package gui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/resources"
	"context"
	"image"
	"testing"
)

func TestEnabledUserRuleReallyDrivesAutomation(t *testing.T) {
	n := &native{controls: []ports.Control{{ID: "custom", Text: "进入下一阶段", Role: "button", Source: "uia", Enabled: true, Bounds: ports.Rect{Right: 32, Bottom: 24}}}}
	n.controls = append(n.controls, ports.Control{ID: "standard", Text: "Install", Role: "button", Source: "uia", Enabled: true, Bounds: ports.Rect{Left: 100, Right: 132, Bottom: 24}})
	a := Automator{Native: n, Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}, Templates: &templates.Library{}}
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Step(context.Background(), []int{1}); err != nil || len(n.clicked) != 1 || n.clicked[0].ID != "standard" {
		t.Fatal("unselected rule changed original automation", err)
	}
	n.clicked = nil
	a.Reset()
	a.Templates.User = templates.UserIndex{Rules: []templates.UserRule{{ProjectID: "project", ID: "capture", Method: "uia", WindowTitle: "Setup", Name: "进入下一阶段", AutomationID: "custom", Role: "button", Action: "next"}}}
	u, err := a.Step(context.Background(), []int{1})
	if err != nil || len(n.clicked) != 1 || len(u.Actions) != 1 || n.clicked[0].UserTemplate == "" || n.clicked[0].ID != "custom" {
		t.Fatal("selected rule did not drive a real native action", err, u)
	}
	a.Reset()
	n.controls[0].Enabled = false
	n.controls[1].Enabled = false
	a.Step(context.Background(), []int{1})
	if len(n.clicked) != 1 {
		t.Fatal("disabled control bypassed")
	}
}

func TestUserImageRuleThresholdAndDPI(t *testing.T) {
	needle := image.NewGray(image.Rect(0, 0, 4, 4))
	for i := range needle.Pix {
		needle.Pix[i] = uint8(i * 11)
	}
	index := templates.UserIndex{Rules: []templates.UserRule{{ProjectID: "project", ID: "image", Method: "image", WindowTitle: "Setup", DPI: 96, Action: "next", Threshold: 1, Image: needle}}}
	w := ports.Window{Title: "Setup", DPI: 96}
	if got := index.Images(context.Background(), w, needle); len(got) != 1 || got[0].Confidence != 1 {
		t.Fatal(got)
	}
	w.DPI = 144
	if got := index.Images(context.Background(), w, needle); len(got) != 0 {
		t.Fatal("different DPI silently reused")
	}
}
