package verify

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"context"
	"testing"
)

func TestUpgradeProductIdentity(t *testing.T) {
	old := ports.UninstallEntry{Publisher: "Microsoft Corporation", DisplayName: "Microsoft Visual C++ 2019 X64 Minimum Runtime - 14.28.29913", DisplayVersion: "14.28.29913"}
	current := ports.UninstallEntry{Publisher: old.Publisher, DisplayName: "Microsoft Visual C++ 2022 X64 Minimum Runtime - 14.44.35211", DisplayVersion: "14.44.35211"}
	if !sameProduct(old, current) {
		t.Fatal("version replacement not recognized")
	}
	for _, alter := range []func(*ports.UninstallEntry){
		func(r *ports.UninstallEntry) { r.Publisher = "" },
		func(r *ports.UninstallEntry) { r.Publisher = "Other" },
		func(r *ports.UninstallEntry) {
			r.DisplayName = "Microsoft Visual C++ 2022 X86 Minimum Runtime - 14.44.35211"
		},
		func(r *ports.UninstallEntry) {
			r.DisplayName = "Microsoft Visual C++ 2022 X64 Additional Runtime - 14.44.35211"
		},
	} {
		other := current
		alter(&other)
		if sameProduct(old, other) {
			t.Fatal("unrelated or unverified identity accepted", other)
		}
	}
	base := &ports.Baseline{Registry: map[string]ports.UninstallEntry{"old": old}, Dirs: map[string]ports.DirState{}}
	after := &ports.Baseline{Registry: map[string]ports.UninstallEntry{"new": current}, Dirs: map[string]ports.DirState{}}
	cfg := config.Defaults().Verify
	cfg.SettleMaxSec = 0
	v := Verifier{Source: source{b: after}, Config: cfg}
	exit := 0
	o, err := v.Observe(context.Background(), "VC_redist.x64.exe", base, ports.Command{}, &exit, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range o.Evidence {
		if e.ID == "N4" {
			t.Fatal(e)
		}
	}
}
