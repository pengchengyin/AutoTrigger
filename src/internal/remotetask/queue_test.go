package remotetask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) (*Queue, Request) {
	t.Helper()
	root := t.TempDir()
	q, err := New(root, Policy{Config: []byte(`{"timeout_sec":3}`)})
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(Inbox(root), "setup.exe")
	if err = os.WriteFile(input, []byte("trusted-upload"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	return q, Request{Action: "submit", ID: id, Input: input}
}
func TestSubmitSnapshotIdempotencyAndFIFO(t *testing.T) {
	q, r := fixture(t)
	first, err := q.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != "queued" || first.Input == r.Input || first.Name != "setup.exe" {
		t.Fatal(first)
	}
	again, err := q.Submit(r)
	if err != nil || again.ID != first.ID || len(q.List()) != 1 {
		t.Fatal(again, err)
	}
	if err = os.WriteFile(r.Input, []byte("later-upload"), 0600); err != nil {
		t.Fatal(err)
	}
	guard, err := Verify(first)
	if err != nil {
		t.Fatal("source upload changed snapshot", err)
	}
	guard.Close()
	r.ID, _ = NewID()
	second, err := q.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	job, ok, err := q.Next()
	if err != nil || !ok || job.ID != first.ID {
		t.Fatal(job, ok, err)
	}
	if _, ok, err = q.Next(); err != nil || ok {
		t.Fatal("parallel claim", err)
	}
	if err = q.Complete(first.ID, 0, ""); err != nil {
		t.Fatal(err)
	}
	job, ok, err = q.Next()
	if err != nil || !ok || job.ID != second.ID {
		t.Fatal(job, ok, err)
	}
}
func TestSubmitOutsideUpload(t *testing.T) {
	q, r := fixture(t)
	outside := filepath.Join(t.TempDir(), "安装包 带空格.exe")
	if err := os.WriteFile(outside, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	job, err := q.Submit(Request{ID: r.ID, Input: outside})
	if err != nil {
		t.Fatal(err)
	}
	if job.Source != outside || job.Input == outside {
		t.Fatal("external input not frozen")
	}
	if err = os.WriteFile(outside, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(job.Input)
	if err != nil || string(data) != "x" {
		t.Fatal("snapshot changed", err)
	}
}

func TestRejectInvalidPathAndHashMismatch(t *testing.T) {
	q, r := fixture(t)
	for _, path := range []string{"relative.exe", `\\server\share\setup.exe`, filepath.Join(t.TempDir(), "missing.exe"), t.TempDir()} {
		if _, err := q.Submit(Request{ID: r.ID, Input: path}); err == nil {
			t.Fatalf("invalid input accepted: %s", path)
		}
	}
	r.SHA256 = string(make([]byte, 64))
	if _, err := q.Submit(r); err == nil {
		t.Fatal("invalid hash accepted")
	}
	hash := sha256.Sum256([]byte("wrong"))
	r.SHA256 = hex.EncodeToString(hash[:])
	if _, err := q.Submit(r); err == nil {
		t.Fatal("mismatched hash accepted")
	}
	if len(q.List()) != 0 {
		t.Fatal("failed submissions persisted")
	}
	r.SHA256 = ""
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := q.SubmitContext(ctx, r); err == nil {
		t.Fatal("cancelled copy accepted")
	}
}
func TestRecoveryCancellationAndTampering(t *testing.T) {
	q, r := fixture(t)
	job, err := q.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	q.Next()
	restarted, err := New(q.layout.Root, q.policy)
	if err != nil {
		t.Fatal(err)
	}
	response := restarted.Handle(Request{Action: "status", ID: job.ID})
	if response.Job.State != "interrupted" {
		t.Fatal(response)
	}
	if _, ok, _ := restarted.Next(); ok {
		t.Fatal("interrupted installation replayed")
	}
	r.ID, _ = NewID()
	queued, err := restarted.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Cancel(queued.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := restarted.Next(); ok {
		t.Fatal("cancelled task claimed")
	}
	if err = os.WriteFile(job.Input, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if guard, err := Verify(job); err == nil {
		guard.Close()
		t.Fatal("changed snapshot accepted")
	}
	r.ID, _ = NewID()
	job, err = restarted.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(job.Config, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if guard, err := Verify(job); err == nil {
		guard.Close()
		t.Fatal("changed config accepted")
	}
}
func TestLogsResultsAndRunningCancel(t *testing.T) {
	q, r := fixture(t)
	job, err := q.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	q.Next()
	if err = q.layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(job.Log, []byte("one\ntwo\n"), 0600)
	os.WriteFile(job.Result, []byte(`{"summary":{"total":1}}`), 0600)
	response := q.Handle(Request{Action: "logs", ID: job.ID, Offset: 4, Limit: 4})
	if !response.OK || response.Logs != "two\n" || response.NextOffset != 8 {
		t.Fatal(response)
	}
	if response = q.Handle(Request{Action: "result", ID: job.ID}); !response.OK || len(response.Result) == 0 {
		t.Fatal(response)
	}
	if response = q.Handle(Request{Action: "cancel", ID: job.ID}); !response.OK || !response.Job.CancelRequested {
		t.Fatal(response)
	}
	if err = q.Complete(job.ID, 1, ""); err != nil {
		t.Fatal(err)
	}
	if response = q.Handle(Request{Action: "status", ID: job.ID}); response.Job.State != "cancelled" {
		t.Fatal(response)
	}
}

func TestLogChunksPreserveUTF8(t *testing.T) {
	q, r := fixture(t)
	job, err := q.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	q.layout.Ensure()
	os.WriteFile(job.Log, []byte("安装中\n"), 0600)
	var text string
	var offset int64
	for i := 0; i < 4; i++ {
		response := q.Handle(Request{Action: "logs", ID: job.ID, Offset: offset, Limit: 4})
		if !response.OK {
			t.Fatal(response)
		}
		text += response.Logs
		offset = response.NextOffset
	}
	if text != "安装中\n" {
		t.Fatal(text)
	}
	if response := q.Handle(Request{Action: "logs", ID: job.ID, Offset: 1, Limit: 4}); response.OK {
		t.Fatal("invalid UTF-8 byte boundary accepted")
	}
}
