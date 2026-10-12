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
