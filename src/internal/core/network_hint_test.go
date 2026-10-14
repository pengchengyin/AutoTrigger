package core

import (
	"autotrigger/internal/core/ports"
	"autotrigger/internal/resources"
	"context"
	"testing"
)

func TestInstallerOutputNetworkHints(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{{"正在下载组件，请稍候", true}, {"Connecting to server...", true}, {"internet connection required", true}, {"installation failed", false}, {"Waiting for user input", false}, {"", false}} {
		if got := outputRequiresNetwork(resources.Pack{Mode: "builtin"}, tc.text); got != tc.want {
			t.Fatalf("%q network=%t", tc.text, got)
		}
	}
}

type networkOutputRunner struct {
	*fakeRunner
	stdout, stderr string
}

func (r networkOutputRunner) Start(ctx context.Context, c ports.Command) (ports.Process, error) {
	p, err := r.fakeRunner.Start(ctx, c)
	if err == nil && !r.hang {
		f := p.(*fakeProcess)
		exit := <-f.exit
		exit.Stdout, exit.Stderr = r.stdout, r.stderr
		f.exit <- exit
	}
	return p, err
}
func TestSilentNetworkHintsWithoutGUI(t *testing.T) {
	for _, stderr := range []bool{false, true} {
		e, r := engineFor(t)
		e.Config.GUI.Enabled = false
		e.GUI = nil
		runner := networkOutputRunner{fakeRunner: r}
		if stderr {
			runner.stderr = "需要联网下载组件"
		} else {
			runner.stdout = "Connecting to server..."
		}
		e.Runner = runner
		if err := e.Run(context.Background(), sample(t, "setup.exe", []byte("MZ"))); err != nil {
			t.Fatal(err)
		}
		f := e.Result.Results[0]
		if f.PlatformMeta["requires_network"] != true {
			t.Fatalf("silent hint missing: %+v", f)
		}
		if f.FinalStatus == "install_completed" {
			t.Fatal("network hint fabricated installation evidence")
		}
	}
}
