package core

import (
	"autotrigger/internal/core/ports"
	"context"
	"fmt"
	"strings"
	"testing"
)

type elevationDeniedRunner struct{ *fakeRunner }

func (r elevationDeniedRunner) Start(_ context.Context, cmd ports.Command) (ports.Process, error) {
	r.commands = append(r.commands, cmd)
	return nil, fmt.Errorf("%w: blocked", ports.ErrElevationRequired)
}
func TestElevationFailureDoesNotRetryAllParameters(t *testing.T) {
	e, r := engineFor(t)
	e.Runner = elevationDeniedRunner{r}
	e.Trigger = fakeTrigger{count: 9}
	if err := e.Run(context.Background(), sample(t, "setup.exe", []byte("MZ"))); err != nil {
		t.Fatal(err)
	}
	f := e.Result.Results[0]
	if len(r.commands) != 1 || len(f.Attempts) != 1 || f.FinalStatus != "failed" || !strings.HasPrefix(f.Notes, "requires_elevation:") || !strings.Contains(f.Notes, "管理员") {
		t.Fatalf("%+v commands=%d", f, len(r.commands))
	}
}
