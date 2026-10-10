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

func TestD17NonInstallationDoesNotCollectGlobalBaseline(t *testing.T) {
	e, r := engineFor(t)
	e.Evidence = fakeEvidence{runner: r, fail: true}
	e.Opener = fakeOpener{association: true}
	e.Run(context.Background(), sample(t, "script.cmd", []byte("echo test")))
	f := e.Result.Results[0]
	if f.FinalStatus != "triggered" || f.BaselineDigest != nil {
		t.Fatal(f)
	}
}

func TestD15HelpDocumentsAreNotGlobalBlacklist(t *testing.T) {
	for _, name := range []string{"manual.pdf", "help.chm", "manual.hlp"} {
		e, r := engineFor(t)
		e.Opener = fakeOpener{association: true}
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, name), []byte("document"), 0600)
		e.Run(context.Background(), root)
		f := e.Result.Results[0]
		if len(r.commands) != 1 || f.FinalStatus != "triggered" || f.SelectionReason == "excluded_blacklist" {
			t.Fatal(f)
		}
	}
}

func TestD11NonInstallationSamplesHaveTriggerStatus(t *testing.T) {
	for _, name := range []string{"script.bat", "script.ps1", "script.vbs", "script.js", "trigger.scr", "trigger.com"} {
		e, r := engineFor(t)
		r.installAt = 1
		data := []byte("echo test")
		if strings.HasSuffix(name, ".scr") || strings.HasSuffix(name, ".com") {
			data = []byte("MZ")
		}
		e.Run(context.Background(), sample(t, name, data))
		f := e.Result.Results[0]
		if f.FinalStatus != "triggered" || f.InstallConfidence != nil || len(f.Attempts) != 1 || len(f.Attempts[0].InstallEvidence) != 0 {
			t.Fatal(f)
		}
	}
}

func TestD10UnknownAndForeignFilesNeverLaunchOpenWith(t *testing.T) {
	for _, data := range [][]byte{[]byte("random data"), {0x7f, 'E', 'L', 'F'}, {0xcf, 0xfa, 0xed, 0xfe}} {
		e, r := engineFor(t)
		e.Run(context.Background(), sample(t, "sample.bin", data))
		f := e.Result.Results[0]
		if len(r.commands) != 0 || len(f.Attempts) != 0 {
			t.Fatal(f)
		}
		if string(data) == "random data" && (f.FinalStatus != "failed" || f.Notes != "untriggerable") {
			t.Fatal(f)
		}
		if string(data) != "random data" && f.FinalStatus != "skipped" {
			t.Fatal(f)
		}
	}
}

func TestD05DocumentHostCannotGenerateInstallationEvidence(t *testing.T) {
	e, r := engineFor(t)
	r.installAt = 1
	e.Opener = fakeOpener{association: true}
	e.Run(context.Background(), sample(t, "sample.pdf", []byte("%PDF")))
	f := e.Result.Results[0]
	if f.FinalStatus != "triggered" || f.InstallConfidence != nil || len(f.Attempts[0].InstallEvidence) != 0 {
		t.Fatal(f)
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
