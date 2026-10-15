package cli

import (
	"autotrigger/internal/datadir"
	"autotrigger/pkg/schema"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIHistoryPreservesIndependentResults(t *testing.T) {
	layout := datadir.Layout{Root: t.TempDir()}
	logPath := filepath.Join(layout.Root, "shared.log")
	for _, id := range []string{"first", "second"} {
		if err := os.WriteFile(logPath, []byte(id), 0600); err != nil {
			t.Fatal(err)
		}
		r := schema.NewResult(id, "windows")
		if err := writeTaskHistory(layout, &r, "sample.exe", "config.json", logPath, nil); err != nil {
			t.Fatal(err)
		}
		code := 130
		if err := writeTaskHistory(layout, &r, "sample.exe", "config.json", logPath, &code); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(layout.History(), "cli", id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var record struct {
			ID, Result string
			ExitCode   *int `json:"exit_code"`
		}
		if err = json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		if record.ID != id || record.ExitCode == nil || *record.ExitCode != 130 {
			t.Fatalf("bad record: %+v", record)
		}
		data, err = os.ReadFile(record.Result)
		if err != nil {
			t.Fatal(err)
		}
		var saved schema.Result
		if err = json.Unmarshal(data, &saved); err != nil || saved.TaskID != id || saved.Truncated {
			t.Fatalf("bad result: %v %+v", err, saved)
		}
	}
	for _, id := range []string{"first", "second"} {
		data, err := os.ReadFile(filepath.Join(layout.Logs(), "cli-"+id+".log"))
		if err != nil || string(data) != id {
			t.Fatalf("history log overwritten: %s %v", data, err)
		}
	}
}
