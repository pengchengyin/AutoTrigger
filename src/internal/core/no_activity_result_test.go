package core

import (
	"autotrigger/internal/core/ports"
	"context"
	"strings"
	"testing"
)

type quietProcess struct{ ports.Process }

func (p quietProcess) Activity() (ports.Activity, error) {
	return ports.Activity{PIDs: []int{p.PID()}}, nil
}

type quietRunner struct{ *fakeRunner }

func (r quietRunner) Start(ctx context.Context, c ports.Command) (ports.Process, error) {
	p, err := r.fakeRunner.Start(ctx, c)
	if err != nil {
		return nil, err
	}
	return quietProcess{p}, nil
}
func TestNoActivityFinalStatusIsNotGenericTimeout(t *testing.T) {
	e, r := engineFor(t)
	r.hang = true
	e.Runner = quietRunner{r}
	e.Trigger = fakeTrigger{count: 1}
	e.Config.StartupTimeoutSec = 1
	e.Config.TimeoutSec = 5
	if err := e.Run(context.Background(), sample(t, "setup.exe", []byte("MZ"))); err != nil {
		t.Fatal(err)
	}
	f := e.Result.Results[0]
	if f.FinalStatus != "failed" || !strings.HasPrefix(f.Notes, "no_activity:") || f.Attempts[0].Error == nil || *f.Attempts[0].Error != "no_activity" {
		t.Fatalf("%+v", f)
	}
	if !r.last.killed {
		t.Fatal("quiet process not cleaned up")
	}
}
