package output

import (
	"autotrigger/pkg/schema"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoverInterruptedJournal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "result.json")
	r := schema.NewResult("old", "windows")
	if err := Write(p, &r, io.Discard, false, 1<<20); err != nil {
		t.Fatal(err)
	}
	j, err := OpenJournal(p+".journal.ndjson", "old")
	if err != nil {
		t.Fatal(err)
	}
	f := schema.NewFile("sample.exe", "loose")
	f.FinalStatus = "failed"
	if err = j.Append(f); err != nil {
		t.Fatal(err)
	}
	j.File.WriteString(`{"task_id":"old"`)
	j.Close(false)
	destination := p + ".recovered.json"
	if err = RecoverJournal(p, j.Path, destination); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(destination)
	var got schema.Result
	if err = json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.TaskID != "old" || got.Error == nil || got.FinishedAt != nil || len(got.Results) != 1 || got.Summary.Total != 1 {
		t.Fatal(got)
	}
	if err = RecoverJournal(p, j.Path, filepath.Join(destination, "bad")); err == nil {
		t.Fatal("write failure hidden")
	}
	next, err := OpenJournal(j.Path, "new")
	if err != nil {
		t.Fatal(err)
	}
	next.Close(false)
	archives, _ := filepath.Glob(j.Path + ".interrupted-*.recovered.json")
	if len(archives) != 1 {
		t.Fatal("automatic recovery missing", archives)
	}
}
