package ui

import (
	"autotrigger/internal/datadir"
	"autotrigger/internal/remotetask"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataMigrationPreservesRemoteQueueSnapshots(t *testing.T) {
	oldRoot, newRoot := t.TempDir(), t.TempDir()
	q, err := remotetask.New(oldRoot, remotetask.Policy{Config: []byte(`{"timeout_sec":3}`)})
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(remotetask.Inbox(oldRoot), "setup.exe")
	if err = os.WriteFile(input, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	id, _ := remotetask.NewID()
	j, err := q.Submit(remotetask.Request{ID: id, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	if err = migrateData(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	loaded, err := remotetask.New(newRoot, remotetask.Policy{Config: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	jobs := loaded.List()
	if len(jobs) != 1 || jobs[0].ID != j.ID || jobs[0].State != "queued" || !strings.HasPrefix(jobs[0].Input, newRoot) {
		t.Fatal(jobs)
	}
	guard, err := remotetask.Verify(jobs[0])
	if err != nil {
		t.Fatal(err)
	}
	guard.Close()
	if _, err = os.Stat(j.Input); err != nil {
		t.Fatal("migration removed source snapshot", err)
	}
}

func TestDataMigrationRetainsSourceAndRebasesHistory(t *testing.T) {
	old := t.TempDir()
	dest := t.TempDir()
	layout := datadir.Layout{Root: old}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(layout.Results(), "result.json")
	log := filepath.Join(layout.Logs(), "result.json.log")
	for _, path := range []string{result, log, layout.Config()} {
		if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := taskRecord{ID: "task", Result: result, LogPath: log, Config: layout.Config(), Execution: &Options{Output: result, Config: layout.Config()}}
	if err := saveHistory(layout.History(), []taskRecord{r}); err != nil {
		t.Fatal(err)
	}
	if err := migrateData(old, dest); err != nil {
		t.Fatal(err)
	}
	records, err := loadHistory(datadir.Layout{Root: dest}.History())
	if err != nil || len(records) != 1 {
		t.Fatal(err)
	}
	if records[0].Result != filepath.Join(dest, "results", "result.json") || records[0].LogPath != filepath.Join(dest, "logs", "result.json.log") || records[0].Execution.Config != filepath.Join(dest, "config", "config.json") {
		t.Fatal(records)
	}
	if !fileExists(result) {
		t.Fatal("source removed")
	}
	if err = migrateData(old, dest); err == nil {
		t.Fatal("destination overwritten")
	}
	if err = migrateData(old, filepath.Join(old, "nested")); err == nil {
		t.Fatal("nested migration accepted")
	}
}

func TestLegacyMigrationIsCopyOnly(t *testing.T) {
	root := t.TempDir()
	history := t.TempDir()
	p, _ := hubFixture(t, root, "old template")
	if err := os.MkdirAll(filepath.Join(root, "third_party", "ocr"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "third_party", "ocr", "package.zip"), []byte("offline"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "result-old.json.log"), []byte("log"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyData(root, history); err != nil {
		t.Fatal(err)
	}
	projects, err := listTemplateProjects(filepath.Join(root, "templates"))
	if err != nil || len(projects) != 1 || projects[0].ID != p.ID {
		t.Fatal(projects, err)
	}
	if !fileExists(filepath.Join(root, "logs", "result-old.json.log")) || !fileExists(filepath.Join(root, "third_parties", "ocr", "package.zip")) {
		t.Fatal("legacy paths not migrated")
	}
	if !fileExists(filepath.Join(projectDir(root, p), "project.json")) {
		t.Fatal("legacy source removed")
	}
	b, _ := os.ReadFile(filepath.Join(root, "config", "legacy-layout-imported.json"))
	var doc map[string]bool
	if json.Unmarshal(b, &doc) != nil || !doc["imported"] {
		t.Fatal("missing marker")
	}
}

func TestMigrationBackToExeAllowsOnlyLocatorMetadata(t *testing.T) {
	data := t.TempDir()
	exe := t.TempDir()
	if err := (datadir.Layout{Root: data}).Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "config", "config.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := datadir.Save(exe, data); err != nil {
		t.Fatal(err)
	}
	if err := migrateData(data, exe); err != nil {
		t.Fatal(err)
	}
	layout, err := datadir.Load(exe)
	if err != nil || layout.Root != data {
		t.Fatal("locator changed before switch", layout, err)
	}
	if !fileExists(filepath.Join(exe, "config", "config.json")) {
		t.Fatal("config not migrated")
	}
	if err = datadir.Save(exe, exe); err != nil {
		t.Fatal(err)
	}
	layout, err = datadir.Load(exe)
	if err != nil || layout.Root != exe {
		t.Fatal(layout, err)
	}
}
