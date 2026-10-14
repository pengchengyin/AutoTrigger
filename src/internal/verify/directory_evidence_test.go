package verify

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"context"
	"strings"
	"testing"
)

func TestFileEvidenceIdentifiesDistinctDirectories(t *testing.T) {
	base := &ports.Baseline{Registry: map[string]ports.UninstallEntry{}, Dirs: map[string]ports.DirState{"old": {Files: 20}}}
	after := &ports.Baseline{Registry: map[string]ports.UninstallEntry{}, Dirs: map[string]ports.DirState{"new": {Files: 20}}}
	cfg := config.Defaults().Verify
	cfg.SettleMaxSec = 0
	v := Verifier{Source: source{b: after}, Config: cfg}
	o, err := v.Observe(context.Background(), "app.exe", base, ports.Command{}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range o.Evidence {
		if e.ID == "S2" || e.ID == "N5" {
			want := "new"
			if e.ID == "N5" {
				want = "old"
			}
			if e.Source != want || !strings.Contains(e.Detail, want) {
				t.Fatal(e)
			}
			seen[e.ID] = true
		}
	}
	if !seen["S2"] || !seen["N5"] {
		t.Fatal(o)
	}
}

func TestA3HasRealDirectoryAndNoPhantomFileChange(t *testing.T) {
	base := &ports.Baseline{Registry: map[string]ports.UninstallEntry{"key": {DisplayName: "App", DisplayVersion: "1"}}, Dirs: map[string]ports.DirState{"actual": {Files: 1, Bytes: 10}}}
	after := &ports.Baseline{Registry: map[string]ports.UninstallEntry{"key": {DisplayName: "App", DisplayVersion: "2"}}, Dirs: map[string]ports.DirState{"actual": {Files: 1, Bytes: 20}}}
	cfg := config.Defaults().Verify
	cfg.SettleMaxSec = 0
	o, err := (Verifier{Source: source{b: after}, Config: cfg}).Observe(context.Background(), "app.exe", base, ports.Command{}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range o.Evidence {
		if e.Type == "filesystem_changed" {
			found = true
			if e.Source != "actual" || !strings.Contains(e.Detail, "10→20") || !strings.Contains(e.Detail, "不代表进程归因") {
				t.Fatal(e)
			}
		}
	}
	if !found {
		t.Fatal("changed directory not diagnosed")
	}
	after.Dirs = base.Dirs
	o, err = (Verifier{Source: source{b: after}, Config: cfg}).Observe(context.Background(), "app.exe", base, ports.Command{}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range o.Evidence {
		if e.Type == "filesystem_changed" {
			t.Fatal("registry change fabricated file evidence", e)
		}
	}
}
