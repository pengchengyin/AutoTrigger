package verify

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/pkg/schema"
	"context"
	"testing"
)

func TestD02MSILogStatusMustBeNumericSuccess(t *testing.T) {
	for _, text := range []string{"安装成功或错误状态: 1603。", "安装成功或错误状态: 1602。", "Installation success or error status: 1620.", "安装成功或错误状态: 未知"} {
		if got := installerLogSuccess(text); got != "" {
			t.Fatalf("false B1: %q", got)
		}
	}
	for _, text := range []string{"安装成功或错误状态: 0。", "Installation success or error status: 3010.", "Installation success or error status: 1641.", "Installation completed successfully"} {
		if installerLogSuccess(text) == "" {
			t.Fatalf("missing B1: %q", text)
		}
	}
}

func TestD06TransientInstallationRemovedBeforeSettle(t *testing.T) {
	empty := &ports.Baseline{Registry: map[string]ports.UninstallEntry{}, Dirs: map[string]ports.DirState{}}
	transient := &ports.Baseline{Registry: map[string]ports.UninstallEntry{"new": {DisplayName: "Demo"}}, Dirs: map[string]ports.DirState{"newdir": {Files: 1}}}
	c := config.Defaults().Verify
	c.SettleMaxSec = 0
	v := Verifier{Source: source{b: empty}, Config: c}
	exit := 0
	o, err := v.Observe(context.Background(), "demo.exe", empty, ports.Command{}, &exit, nil, "", transient)
	if err != nil {
		t.Fatal(err)
	}
	d := Decide("loose", o.Evidence, o.SideEffects, false, true, false, o.Artifact)
	if d.Status != "install_partial" || len(d.Conflicts) != 2 {
		t.Fatalf("%+v %+v", o, d)
	}
}

func TestD05TimeoutWeakEvidenceCannotCompleteInstallation(t *testing.T) {
	d := Decide("loose", []schema.Evidence{{ID: "A3", Level: "A"}}, true, false, true, true, "failed")
	if d.Status != "install_partial" {
		t.Fatal(d)
	}
}
