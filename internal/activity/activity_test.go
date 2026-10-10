package activity

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"testing"
	"time"
)

func TestStartupAndIdle(t *testing.T) {
	c := config.Defaults()
	now := time.Now()
	m := New(now, c)
	if m.Timeout(now.Add(31*time.Second)) != "no_activity" {
		t.Fatal("startup deadline ignored")
	}
	m.Update(now.Add(time.Second), ports.Activity{CPUTimeMS: 201}, "")
	if m.Timeout(now.Add(59*time.Second)) != "" {
		t.Fatal("early idle timeout")
	}
	if m.Timeout(now.Add(62*time.Second)) != "idle_timeout" {
		t.Fatal("idle deadline ignored")
	}
	m.Update(now.Add(62*time.Second), ports.Activity{CPUTimeMS: 202}, "gui_action")
	if m.Timeout(now.Add(63*time.Second)) != "" {
		t.Fatal("GUI progress did not reset idle")
	}
}

func TestD04PassiveHostCPUCannotKeepIdleAlive(t *testing.T) {
	c := config.Defaults()
	now := time.Now()
	m := New(now, c)
	m.PassiveHost = true
	for i := 1; i <= 70; i++ {
		m.Update(now.Add(time.Duration(i)*time.Second), ports.Activity{PIDs: []int{1}, CPUTimeMS: uint64(i) * 500, IOBytes: uint64(i) * 100000}, "")
	}
	if m.Timeout(now.Add(70*time.Second)) != "idle_timeout" {
		t.Fatalf("passive host kept alive: %+v", m)
	}
	m.Update(now.Add(71*time.Second), m.Last, "gui_change")
	if m.Timeout(now.Add(72*time.Second)) != "" {
		t.Fatal("actual GUI change must reset idle")
	}
}
