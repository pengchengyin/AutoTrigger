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
