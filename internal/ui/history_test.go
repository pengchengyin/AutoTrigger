package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
