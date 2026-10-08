// Package procutil runs bounded dependency commands through the platform runner.
package procutil

import (
	"autotrigger/internal/core/ports"
	"context"
	"fmt"
	"sync"
	"time"
)

type Capture struct {
	mu        sync.Mutex
	Data      []byte
	Limit     int
	Truncated bool
}

func (b *Capture) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remain := b.Limit - len(b.Data)
	if remain < 0 {
		remain = 0
	}
	if len(p) > remain {
		p = p[:remain]
		b.Truncated = true
	}
	b.Data = append(b.Data, p...)
	return n, nil
}
func (b *Capture) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.Data) }
func Run(ctx context.Context, runner ports.ProcessRunner, cmd ports.Command) (ports.Exit, error) {
	out := &Capture{Limit: 16 * 1024 * 1024}
	stderr := &Capture{Limit: 65536}
	cmd.Stdout = out
	cmd.Stderr = stderr
	p, err := runner.Start(ctx, cmd)
	if err != nil {
		return ports.Exit{}, err
	}
	defer p.Close()
	select {
	case e := <-p.Done():
		e.Stdout = out.String()
		e.Stderr = stderr.String()
		if out.Truncated {
			return e, fmt.Errorf("dependency output exceeded 16 MiB")
		}
		if e.Code != 0 {
			return e, fmt.Errorf("%s exited %d: %s", cmd.Program, e.Code, e.Stderr)
		}
		return e, e.Err
	case <-ctx.Done():
		p.KillTree()
		select {
		case <-p.Done():
		case <-time.After(time.Second):
		}
		return ports.Exit{}, ctx.Err()
	}
}
