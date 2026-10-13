package output

import (
	"autotrigger/pkg/schema"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicOutputAndTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "result.json")
	r := schema.NewResult("test", "windows")
	r.Results = append(r.Results, schema.NewFile("sample.exe", "loose"))
	r.Results[0].Attempts = append(r.Results[0].Attempts, schema.Attempt{Command: string(bytes.Repeat([]byte("x"), 2000))})
	if err := Write(path, &r, &bytes.Buffer{}, false, 1500); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var parsed schema.Result
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.Truncated || len(parsed.Results[0].Attempts) != 0 {
		t.Fatal("truncation broke fixed records")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary result remained")
	}
	if parsed.FinishedAt != nil {
		t.Fatal("interrupted result timestamp lost")
	}
	r.Error = &schema.Error{Code: 2, Message: "输入不存在"}
	if err := Write(path, &r, &bytes.Buffer{}, true, 65536); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !json.Valid(data) || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("invalid UTF-8 JSON output")
	}
}
func TestJournalOwnsTaskAndPreservesPreviousCrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.journal.ndjson")
	j, err := OpenJournal(path, "first")
	if err != nil {
		t.Fatal(err)
	}
	f := schema.NewFile("one.exe", "loose")
	if err = j.Append(f); err != nil {
		t.Fatal(err)
	}
	j.Close(false)
	j, err = OpenJournal(path, "second")
	if err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(path + ".interrupted-*")
	if len(matches) != 1 {
		t.Fatal("prior interrupted journal destroyed")
	}
	if err = j.Close(true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("success journal retained")
	}
}
