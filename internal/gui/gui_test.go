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
	"time"
)

type native struct {
	controls []ports.Control
	clicked  []ports.Control
}

func TestAgreementSelectionAdvancesToNext(t *testing.T) {
	for _, role := range []string{"radio", "checkbox"} {
		n := &native{controls: []ports.Control{
			{ID: "accept", Text: "I accept the agreement", Role: role, Enabled: true},
			{ID: "next", Text: "下一步(&N) >", Role: "button", Enabled: false},
		}}
		a := Automator{Native: n, Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
		if err := a.Init(); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Step(context.Background(), []int{1}); err != nil || len(n.clicked) != 1 || n.clicked[0].ID != "accept" {
			t.Fatalf("%s agreement was not selected: %v, %+v", role, err, n.clicked)
		}
		n.controls[0].Checked = true
		n.controls[1].Enabled = true
		a.states[1].lastClick = time.Now().Add(-time.Second)
		if _, err := a.Step(context.Background(), []int{1}); err != nil || len(n.clicked) != 2 || n.clicked[1].ID != "next" {
			t.Fatalf("%s blocked next after agreement: %v, %+v", role, err, n.clicked)
		}
	}
}

func TestNotepad3ChineseLicensePage(t *testing.T) {
	n := &native{controls: []ports.Control{
		{ID: "agree", Text: "我同意此协议(&A)", Role: "radio", Enabled: true},
		{ID: "decline", Text: "我拒绝此协议(&D)", Role: "radio", Enabled: true, Checked: true},
		{ID: "next", Text: "下一步(&N)", Role: "button", Enabled: false},
	}}
	a := Automator{Native: n, Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	if a.semantic(n.controls[1].Text) != "deny" {
		t.Fatal("declining license was not forbidden")
	}
	if _, err := a.Step(context.Background(), []int{1}); err != nil || len(n.clicked) != 1 || n.clicked[0].ID != "agree" {
		t.Fatalf("license agreement not selected: %v, %+v", err, n.clicked)
	}
	n.controls[0].Checked, n.controls[1].Checked, n.controls[2].Enabled = true, false, true
	a.states[1].lastClick = time.Now().Add(-time.Second)
	if _, err := a.Step(context.Background(), []int{1}); err != nil || len(n.clicked) != 2 || n.clicked[1].ID != "next" {
		t.Fatalf("license next not clicked: %v, %+v", err, n.clicked)
	}
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

func TestLocalizedWizardAccelerators(t *testing.T) {
	for _, text := range []string{"下一步(&N) >", "下一步（Ｎ） ＞", "下一步 (&N)", "&Next >"} {
		n := &native{controls: []ports.Control{{ID: "next", Text: text, Role: "button", Enabled: true}}}
		a := Automator{Native: n, Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
		if err := a.Init(); err != nil {
			t.Fatal(err)
		}
		u, err := a.Step(context.Background(), []int{1})
		if err != nil || len(u.Actions) != 1 || len(n.clicked) != 1 {
			t.Fatalf("label %q: %+v, %v", text, u, err)
		}
		if _, err := a.Step(context.Background(), []int{1}); err != nil || len(n.clicked) != 1 {
			t.Fatalf("label %q clicked repeatedly: %v", text, err)
		}
	}
	a := Automator{Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"取消(&C)", "卸载(&U)", "退出(&X)"} {
		if a.semantic(text) != "deny" {
			t.Fatalf("accelerator bypassed deny: %q", text)
		}
	}
	for _, text := range []string{"下一步(推荐)", "Install Malware (&M)", "下一步(N)其他"} {
		if a.semantic(text) != "" {
			t.Fatalf("unrelated label matched: %q", text)
		}
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
