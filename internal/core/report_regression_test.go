package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestD01DirectBlacklistedInputsNeverStart(t *testing.T) {
	for _, name := range []string{"unins000.exe", "卸载.exe", "update.exe", "crashpad_handler.exe", "checkupdate.exe", "repair.exe", "modify.exe"} {
		t.Run(name, func(t *testing.T) {
			e, r := engineFor(t)
			e.Run(context.Background(), sample(t, name, []byte("MZfixture")))
			f := e.Result.Results[0]
			if len(r.commands) != 0 || len(f.Attempts) != 0 || f.FinalStatus != "skipped" || !strings.HasPrefix(f.Notes, "excluded_blacklist:") {
				t.Fatalf("%+v commands=%v", f, r.commands)
			}
		})
	}
}

func TestD03OrdinaryDirectoryDoesNotConverge(t *testing.T) {
	e, r := engineFor(t)
	r.installAt = 1
	root := t.TempDir()
	for _, name := range []string{"one.exe", "two.exe", "three.exe"} {
		os.WriteFile(filepath.Join(root, name), []byte("MZ"), 0600)
	}
	if err := e.Run(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 9 || e.Result.LayoutAnalysis.ConvergedAt != nil || e.Result.Summary.Total != 3 {
		t.Fatalf("commands=%d results=%+v", len(r.commands), e.Result)
	}
	for _, f := range e.Result.Results {
		if f.SelectionReason == "skipped_after_success" {
			t.Fatal(f)
		}
	}
}
