package gui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/resources"
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"
)

type native struct {
	controls []ports.Control
	clicked  []ports.Control
}

func (n *native) Windows([]int) ([]ports.Window, error) {
	return []ports.Window{{Handle: 1, Title: "Setup", Bounds: ports.Rect{Left: 0, Top: 0, Right: 500, Bottom: 300}, DPI: 96}}, nil
}
func (n *native) Controls(w ports.Window) ([]ports.Control, error) {
	for i := range n.controls {
		n.controls[i].Window = w
	}
	return n.controls, nil
}
func (n *native) Screenshot(ports.Window) (image.Image, error) {
	return image.NewGray(image.Rect(0, 0, 500, 300)), nil
}
func (n *native) Click(c ports.Control) error         { n.clicked = append(n.clicked, c); return nil }
func (n *native) SetText(ports.Control, string) error { return nil }
func (n *native) CloseWindow(ports.Window) error      { return nil }
func (n *native) Close() error                        { return nil }
func TestArbitrationAndForbiddenButtons(t *testing.T) {
	n := &native{controls: []ports.Control{{ID: "cancel", Text: "取消", Role: "button", Enabled: true}, {ID: "next", Text: "&Next >", Role: "button", Enabled: true}, {ID: "install", Text: "Install", Role: "button", Enabled: true}}}
	a := Automator{Native: n, Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	u, err := a.Step(context.Background(), []int{1})
	if err != nil || len(u.Actions) != 1 || len(n.clicked) != 1 || n.clicked[0].Text != "Install" {
		t.Fatalf("%+v %v", u, err)
	}
	a.Step(context.Background(), []int{1})
	if len(n.clicked) != 1 {
		t.Fatal("same frame clicked twice")
	}
	for _, text := range []string{"Cancel", "取消", "Uninstall", "否", "Exit"} {
		if a.semantic(text) != "deny" {
			t.Fatal("blacklist missed", text)
		}
	}
	if a.semantic("Install Malware") != "" {
		t.Fatal("OCR substring became command")
	}
}
func TestNormalize(t *testing.T) {
	if Normalize("  Ｎｅｘｔ  & > ") != "next" {
		t.Fatal(Normalize("  Ｎｅｘｔ  & > "))
	}
}

func TestExternalRulesCannotOverrideMandatoryDeny(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "keywords"), 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`[{"id":"external_allow","pattern":"Cancel|Uninstall|取消|退出","action":"install"}]`)
	if err := os.WriteFile(filepath.Join(dir, "keywords", "buttons.en-US.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	a := Automator{Config: config.Defaults(), Pack: resources.Pack{Mode: "external", Root: dir}}
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Cancel", "Uninstall", "取消", "退出"} {
		if a.semantic(text) != "deny" {
			t.Fatalf("external rules bypassed mandatory deny: %s", text)
		}
	}
}
