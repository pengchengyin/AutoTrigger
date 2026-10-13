package ui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/gui/templates"
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestInitializeTemplateSelectionFreshDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "templates")
	if err := initializeTemplateSelection(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projectRoot(root), "active.index.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	index, err := selectedProjects(root)
	if err != nil || index.Version != 1 || len(index.Projects) != 0 || len(index.Rules) != 0 {
		t.Fatal(index, err)
	}
	if err := initializeTemplateSelection(root); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("initialization changed existing selection", err)
	}
}

func hubFixture(t *testing.T, root, name string) (templateProject, draftCapture) {
	t.Helper()
	p, err := newTemplateProject(root, name)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newCaptureSession(root, p)
	if err != nil {
		t.Fatal(err)
	}
	w := ports.Window{Title: "Setup", DPI: 96}
	c := ports.Control{ID: "custom", Text: "进入下一阶段", Role: "button", Enabled: true, Source: "uia", Bounds: ports.Rect{Right: 32, Bottom: 24}, Window: w}
	item, err := s.save(draftCapture{Name: c.Text, Window: w, Bounds: c.Bounds, Control: &c, Rule: captureRule{Method: "uia", Name: c.Text, AutomationID: c.ID, Action: "next"}}, image.NewGray(image.Rect(0, 0, 32, 24)))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(false); err != nil {
		t.Fatal(err)
	}
	return p, item
}

func TestTemplateCardSelectionUsesRealSnapshots(t *testing.T) {
	root := t.TempDir()
	p, a := hubFixture(t, root, "模板一")
	q, _ := hubFixture(t, root, "模板二")
	if err := toggleTemplateProject(root, p.ID, true, false); err != nil {
		t.Fatal(err)
	}
	if err := toggleTemplateProject(root, q.ID, true, false); err != nil {
		t.Fatal(err)
	}
	lib, err := templates.Load(config.Defaults().GUI, root)
	if err != nil || len(lib.User.Rules) != 2 {
		t.Fatal(err)
	}
	indexPath := filepath.Join(projectRoot(root), "active.index.json")
	before, _ := os.ReadFile(indexPath)
	if err = toggleTemplateProject(root, p.ID, false, true); err == nil {
		t.Fatal("running task allowed state mutation")
	}
	after, _ := os.ReadFile(indexPath)
	if !bytes.Equal(before, after) {
		t.Fatal("failed switch changed index")
	}
	if err = toggleTemplateProject(root, p.ID, false, false); err != nil {
		t.Fatal(err)
	}
	if err = updateCaptureMark(root, p, a.ID, "", true); err != nil {
		t.Fatal(err)
	}
	if len(lib.User.Rules) != 2 || lib.User.Rules[0].Image == nil {
		t.Fatal("running snapshot changed")
	}
	newLib, err := templates.Load(config.Defaults().GUI, root)
	if err != nil || len(newLib.User.Rules) != 1 {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(projectDir(root, p), a.File)); !os.IsNotExist(err) {
		t.Fatal("deleted capture still exists")
	}
	beforeEmpty, _ := os.ReadFile(indexPath)
	if err = toggleTemplateProject(root, p.ID, true, false); err == nil {
		t.Fatal("empty template activated")
	}
	after, _ = os.ReadFile(indexPath)
	if !bytes.Equal(beforeEmpty, after) {
		t.Fatal("empty enable changed snapshot")
	}
}

func TestUnknownCaptureActionDoesNotPretendEnabled(t *testing.T) {
	root := t.TempDir()
	p, item := hubFixture(t, root, "需要标记")
	if err := updateCaptureMark(root, p, item.ID, "uninstall", false); err == nil {
		t.Fatal("unsafe action accepted")
	}
	if err := toggleTemplateProject(root, p.ID, true, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projectDir(root, p), item.File)
	os.WriteFile(path, []byte("broken"), 0600)
	lib, err := templates.Load(config.Defaults().GUI, root)
	if err != nil || len(lib.User.Rules) != 0 || len(lib.Warnings) == 0 {
		t.Fatal("broken resources did not safely fall back", err)
	}
	if err = toggleTemplateProject(root, p.ID, false, false); err != nil {
		t.Fatal("broken template could not be deselected", err)
	}
}

func TestLegacyPicturesRemainUsableWithoutChangingRelease(t *testing.T) {
	root := t.TempDir()
	p, err := newTemplateProject(root, "旧模板")
	if err != nil {
		t.Fatal(err)
	}
	shot := image.NewGray(image.Rect(0, 0, 140, 80))
	for i := range shot.Pix {
		shot.Pix[i] = color.Gray{Y: uint8(i * 7)}.Y
	}
	entry := templates.Entry{ID: "next", Family: "common", Action: "next", Lang: "zh-CN", DPI: 100, Theme: "light"}
	if err = saveCapturedTemplate(projectDir(root, p), shot, image.Rect(10, 10, 90, 36), entry); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "templates")
	if err = publishTemplateProject(root, target, p); err != nil {
		t.Fatal(err)
	}
	releaseIndex, _ := os.ReadFile(filepath.Join(target, "templates.index.json"))
	if err = initializeTemplateSelection(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults().GUI
	lib, err := templates.Load(cfg, root)
	if err != nil || len(lib.Entries) != 0 {
		t.Fatal("unselected legacy project bypassed checkbox", err)
	}
	if err = toggleTemplateProject(root, p.ID, true, false); err != nil {
		t.Fatal(err)
	}
	lib, err = templates.Load(cfg, root)
	if err != nil || len(lib.User.Rules) != 1 || lib.User.Rules[0].Method != "legacy_image" {
		t.Fatal(err)
	}
	if err = toggleTemplateProject(root, p.ID, false, false); err != nil {
		t.Fatal(err)
	}
	if err = updateLegacyMark(root, p, "next", "", true); err != nil {
		t.Fatal(err)
	}
	indexAfter, _ := os.ReadFile(filepath.Join(target, "templates.index.json"))
	if !bytes.Equal(releaseIndex, indexAfter) {
		t.Fatal("immutable publication changed")
	}
}
