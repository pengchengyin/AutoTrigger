package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestHistoryLoadsCLIWithoutGUIIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cli"), 0755); err != nil {
		t.Fatal(err)
	}
	record := taskRecord{ID: "cli-one", Name: "sample", Result: "unique.json", Created: time.Now()}
	write := func() {
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "cli", "one.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	loaded, err := loadHistory(dir)
	if err != nil || len(loaded) != 1 || loaded[0].ExitCode != nil {
		t.Fatalf("running history: %+v %v", loaded, err)
	}
	if _, err = mergeHistory(dir, loaded); err != nil {
		t.Fatal(err)
	}
	code := 0
	record.ExitCode = &code
	write()
	loaded, err = loadHistory(dir)
	if err != nil || len(loaded) != 1 || loaded[0].ExitCode == nil || *loaded[0].ExitCode != 0 {
		t.Fatalf("finished history: %+v %v", loaded, err)
	}
}

func TestHistoryPersistenceKeepsIndependentResults(t *testing.T) {
	dir := t.TempDir()
	records := []taskRecord{{ID: "older", Name: "one", Result: "first.json", Created: time.Unix(100, 0)}, {ID: "newer", Name: "two", Result: "second.json", Created: time.Unix(200, 0)}}
	if err := saveHistory(dir, records); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadHistory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0].ID != "newer" || loaded[1].Result != "first.json" {
		t.Fatalf("history changed: %+v", loaded)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadHistory(dir); err == nil {
		t.Fatal("corrupt index accepted")
	}
}

func TestHistoryImportRejectsArbitraryJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := recordFromResult(path, ""); err == nil {
		t.Fatal("non-result accepted as task")
	}
}

func TestMergeHistoryPreservesOtherWindowTasks(t *testing.T) {
	dir := t.TempDir()
	first := taskRecord{ID: "pending-one", Name: "one", Result: "one.json", Created: time.Unix(100, 0)}
	if _, err := mergeHistory(dir, []taskRecord{first}); err != nil {
		t.Fatal(err)
	}
	code := 0
	completed := first
	completed.ID, completed.ExitCode = "cli-one", &code
	second := taskRecord{ID: "two", Name: "two", Result: "two.json", Created: time.Unix(200, 0)}
	if _, err := mergeHistory(dir, []taskRecord{completed, second}); err != nil {
		t.Fatal(err)
	}
	// The original window still has the unfinished one-record snapshot.
	records, err := mergeHistory(dir, []taskRecord{first})
	if err != nil || len(records) != 2 || records[0].ID != "two" || records[1].ID != "cli-one" || records[1].ExitCode == nil {
		t.Fatalf("stale window erased completed history: %+v, %v", records, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := mergeHistory(dir, []taskRecord{second}); err == nil {
		t.Fatal("corrupt history must not be overwritten")
	}
	body, _ := os.ReadFile(filepath.Join(dir, "index.json"))
	if string(body) != "broken" {
		t.Fatal("corrupt index overwritten")
	}
}

func TestMergeHistoryConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	var writers sync.WaitGroup
	for _, id := range []string{"first", "second"} {
		writers.Add(1)
		go func(id string) {
			defer writers.Done()
			if _, err := mergeHistory(dir, []taskRecord{{ID: id, Result: id + ".json", Created: time.Now()}}); err != nil {
				t.Error(err)
			}
		}(id)
	}
	writers.Wait()
	stored, err := loadHistory(dir)
	if err != nil || len(stored) != 2 {
		t.Fatalf("concurrent tasks lost: %+v, %v", stored, err)
	}
}
