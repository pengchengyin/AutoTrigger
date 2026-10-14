package verify

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"context"
	"testing"
)

func TestNonMSIInstallerRebootCodes(t *testing.T) {
	for _, method := range []string{"silent_install", "no_param_run"} {
		for _, code := range []int{3010, 1641} {
			base := &ports.Baseline{Registry: map[string]ports.UninstallEntry{}, Dirs: map[string]ports.DirState{}}
			cfg := config.Defaults().Verify
			cfg.SettleMaxSec = 0
			v := Verifier{Source: source{b: base}, Config: cfg}
			o, err := v.Observe(context.Background(), "setup.exe", base, ports.Command{Method: method}, &code, nil, "")
			if err != nil || !o.RequiresReboot {
				t.Fatalf("%s %d %+v %v", method, code, o, err)
			}
			for _, e := range o.Evidence {
				if e.ID == "N1" || e.ID == "S3" {
					t.Fatalf("non-MSI code misclassified: %+v", e)
				}
			}
			d := Decide("loose", o.Evidence, o.SideEffects, false, true, false, o.Artifact)
			if d.Status == "install_completed" {
				t.Fatal("exit code alone counted as installed")
			}
		}
	}
	for _, cmd := range []ports.Command{{Method: "script", NonInstall: true}, {Method: "silent_install", NonInstall: true}, {Method: "dll_probe"}, {}} {
		base := &ports.Baseline{Registry: map[string]ports.UninstallEntry{}, Dirs: map[string]ports.DirState{}}
		cfg := config.Defaults().Verify
		cfg.SettleMaxSec = 0
		code := 3010
		o, err := (Verifier{Source: source{b: base}, Config: cfg}).Observe(context.Background(), "app.exe", base, cmd, &code, nil, "")
		if err != nil || o.RequiresReboot {
			t.Fatalf("non-installer got reboot semantics: %+v %v", o, err)
		}
	}
}
