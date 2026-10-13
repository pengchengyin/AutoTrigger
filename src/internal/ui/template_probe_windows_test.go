//go:build windows

package ui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/gui"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/platform/windows"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLocalTemplateProbe(t *testing.T) {
	root, sample := os.Getenv("AT_TEMPLATE_PROBE_ROOT"), os.Getenv("AT_TEMPLATE_PROBE_SAMPLE")
	if root == "" || sample == "" {
		t.Skip("explicit local read-only recognition probe")
	}
	index, err := templates.LoadUserIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(sample)
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	time.Sleep(2 * time.Second)
	ws, err := (windows.GUI{}).Windows([]int{child.Process.Pid})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rules=%d windows=%d pid=%d", len(index.Rules), len(ws), child.Process.Pid)
	for _, w := range ws {
		shot, e := (windows.GUI{}).Screenshot(w)
		t.Logf("window=%+v screenshot error=%v", w, e)
		if e != nil {
			continue
		}
		for _, rule := range index.Rules {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			rect, score, ok := templates.MatchContext(ctx, templates.Gray(shot), rule.Image, rule.Threshold)
			t.Logf("rule=%s action=%s rect=%v score=%f match=%t timeout=%v eligible=%t", rule.ID, rule.Action, rect, score, ok, ctx.Err(), index.HasImages(w))
			cancel()
		}
	}
	if os.Getenv("AT_TEMPLATE_PROBE_CLICK") == "1" {
		cfg := config.Defaults()
		cfg.OCR.Enabled = false
		a := gui.Automator{Native: windows.GUI{}, Config: cfg, Templates: &templates.Library{User: index}}
		a.Reset()
		for _, expected := range []string{"accept", "next"} {
			u, e := a.Step(context.Background(), []int{child.Process.Pid})
			t.Logf("expected=%s actions=%+v notes=%v error=%v", expected, u.Actions, u.Notes, e)
			if e != nil || len(u.Actions) != 1 {
				t.Fatalf("expected one real %s click", expected)
			}
			if !strings.Contains(strings.Join(u.Notes, " "), "动作="+expected+"，点击已提交") {
				t.Fatalf("wrong rule used: %v", u.Notes)
			}
			time.Sleep(300 * time.Millisecond)
			current, e := (windows.GUI{}).Windows([]int{child.Process.Pid})
			if e != nil || len(current) != 1 {
				t.Fatal("sample window lost")
			}
			shot, e := (windows.GUI{}).Screenshot(current[0])
			if e != nil {
				t.Fatal(e)
			}
			for _, c := range index.Images(context.Background(), current[0], shot) {
				if c.RuleAction == expected {
					t.Fatalf("%s click did not change real window", expected)
				}
			}
		}
	}
}
