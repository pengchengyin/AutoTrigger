//go:build windows

package remotetask

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestNativePipeRoundTripAndSingleExecutor(t *testing.T) {
	q, r := fixture(t)
	server, err := Listen(q.layout.Root, q.HandleContext)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if duplicate, err := Listen(q.layout.Root, q.HandleContext); err == nil {
		duplicate.Close()
		t.Fatal("second executor accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := Call(ctx, q.layout.Root, Request{Action: "health"})
	if err != nil || !response.OK || response.Inbox != Inbox(q.layout.Root) {
		t.Fatal(response, err)
	}
	response, err = Call(ctx, q.layout.Root, r)
	if err != nil || !response.OK || response.Job.ID != r.ID {
		t.Fatal(response, err)
	}
	response, err = Call(ctx, q.layout.Root, Request{Action: "cancel", ID: r.ID})
	if err != nil || response.Job.State != "cancelled" {
		t.Fatal(response, err)
	}
	response, err = Call(ctx, q.layout.Root, Request{Action: "status", ID: r.ID})
	if err != nil || response.Job.State != "cancelled" {
		t.Fatal(response, err)
	}
}
func TestSnapshotDeniesWritesWhileExecuting(t *testing.T) {
	q, r := fixture(t)
	j, err := q.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := Verify(j)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(j.Input, []byte("changed"), 0600); err == nil {
		guard.Close()
		t.Fatal("snapshot writable during run")
	}
	if err = os.WriteFile(j.Config, []byte("{}"), 0600); err == nil {
		guard.Close()
		t.Fatal("config writable during run")
	}
	guard.Close()
}

func TestPipeCloseCancelsIdleClient(t *testing.T) {
	q, _ := fixture(t)
	server, err := Listen(q.layout.Root, q.HandleContext)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := syscall.UTF16PtrFromString(PipeName(q.layout.Root))
	client, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, 0x40000000, 0)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	defer syscall.CloseHandle(client)
	done := make(chan struct{})
	go func() { server.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("idle pipe client prevented shutdown")
	}
}
