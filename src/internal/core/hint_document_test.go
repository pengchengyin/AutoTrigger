package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingEntryHintDocumentIsNotOpened(t *testing.T) {
	e, r := engineFor(t)
	e.Opener = fakeOpener{association: true}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.txt"), []byte("请运行 missing-setup.exe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "action.cmd"), []byte("echo test"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range e.Result.Results {
		if filepath.Base(f.FilePath) == "README.txt" {
			found = true
			if f.FinalStatus != "skipped" || f.Notes != "entry_hint_document" || len(f.Attempts) != 0 {
				t.Fatal(f)
			}
		}
	}
	if !found || len(r.commands) != 1 {
		t.Fatalf("hint opened or script skipped: results=%+v commands=%v", e.Result.Results, r.commands)
	}
}

func TestDirectHintDocumentStillUsesAssociation(t *testing.T) {
	e, r := engineFor(t)
	e.Opener = fakeOpener{association: true}
	if err := e.Run(context.Background(), sample(t, "README.txt", []byte("请运行 missing-setup.exe"))); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 1 || e.Result.Results[0].FinalStatus != "triggered" {
		t.Fatal(e.Result.Results)
	}
}
