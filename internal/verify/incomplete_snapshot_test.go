package verify

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"context"
	"testing"
)

func TestIncompleteSnapshotsCannotInventFileEvidence(t *testing.T) {
	for _, flags := range [][2]bool{{true, false}, {false, true}, {true, true}} {
		base := &ports.Baseline{Truncated: flags[0], Registry: map[string]ports.UninstallEntry{}, Dirs: map[string]ports.DirState{"old": {Files: 20}}}
		after := &ports.Baseline{Truncated: flags[1], Registry: map[string]ports.UninstallEntry{}, Dirs: map[string]ports.DirState{"new": {Files: 20}}}
		cfg := config.Defaults().Verify
		cfg.SettleMaxSec = 0
		v := Verifier{Source: source{b: after}, Config: cfg}
		exit := 1603
		o, err := v.Observe(context.Background(), "app.exe", base, ports.Command{}, &exit, nil, "", after)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range o.Evidence {
			if e.ID == "S2" || e.ID == "A3" || e.ID == "N5" || e.ID == "N6" {
				t.Fatal(e)
			}
		}
		if o.SideEffects {
			t.Fatal("partial scan became installation side effects")
		}
		d := Decide("loose", o.Evidence, o.SideEffects, false, true, false, o.Artifact)
		if d.Status != "failed" {
			t.Fatal(d)
		}
	}
}
