// Package activity applies startup and idle deadlines to process-tree progress.
package activity

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"fmt"
	"time"
)

type Monitor struct {
	Started      time.Time
	LastProgress time.Time
	Last         ports.Activity
	Resets       int
	Seen         bool
	Signal       string
	cfg          config.Config
}

func New(now time.Time, c config.Config) *Monitor {
	return &Monitor{Started: now, LastProgress: now, cfg: c}
}
func (m *Monitor) Update(now time.Time, a ports.Activity, extra string) bool {
	signal := ""
	if a.CPUTimeMS > m.Last.CPUTimeMS && a.CPUTimeMS-m.Last.CPUTimeMS >= uint64(m.cfg.ActivityCPUEpsilonMS) {
		signal = "cpu"
	}
	if a.IOBytes > m.Last.IOBytes && a.IOBytes-m.Last.IOBytes >= m.cfg.ActivityIOEpsilonBytes {
		signal = "io"
	}
	if fmt.Sprint(a.PIDs) != fmt.Sprint(m.Last.PIDs) {
		signal = "process_tree"
	}
	if a.Signature != m.Last.Signature {
		signal = "filesystem_registry"
	}
	if extra != "" {
		signal = extra
	}
	if signal != "" {
		m.Seen = true
		m.LastProgress = now
		m.Resets++
		m.Signal = signal
	}
	m.Last = a
	return signal != ""
}
func (m *Monitor) Timeout(now time.Time) string {
	if !m.Seen && now.Sub(m.Started) >= time.Duration(m.cfg.StartupTimeoutSec)*time.Second {
		return "no_activity"
	}
	if m.Seen && now.Sub(m.LastProgress) >= time.Duration(m.cfg.IdleTimeoutSec)*time.Second {
		return "idle_timeout"
	}
	return ""
}
