package ui

import (
	"autotrigger/internal/core/ports"
	"autotrigger/internal/gui/templates"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureSourcePreservation(t *testing.T) {
	root := t.TempDir()
	p, err := newTemplateProject(root, "原图")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newCaptureSession(root, p)
	if err != nil {
		t.Fatal(err)
	}
	b := ports.Rect{Left: -100, Top: 20, Right: 100, Bottom: 120}
	item := draftCapture{Bounds: ports.Rect{Left: -90, Top: 30, Right: -70, Bottom: 50}, SourceBounds: &b, Rule: captureRule{Method: "image", Action: "next", MinSimilarity: .95}}
	saved, err := s.saveWithSource(item, image.NewRGBA(image.Rect(0, 0, 20, 20)), image.NewRGBA(image.Rect(0, 0, 200, 100)))
	if err != nil {
		t.Fatal(err)
	}
	dir := projectDir(root, p)
	preview, err := capturePreview(dir, saved)
	if err != nil || preview.Bounds().Size() != image.Pt(200, 100) {
		t.Fatalf("source preview: %v", err)
	}
	patch, err := captureImage(dir, saved)
	if err != nil || patch.Bounds().Size() != image.Pt(20, 20) {
		t.Fatalf("match patch: %v", err)
	}
	if err = os.WriteFile(filepath.Join(dir, filepath.FromSlash(saved.SourceFile)), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = captureImage(dir, saved); err == nil {
		t.Fatal("corrupt source accepted")
	}
	if err = s.undo(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "captures", saved.ID)); !os.IsNotExist(err) {
		t.Fatal("undo left source behind")
	}
}

func TestCaptureSessionIsolation(t *testing.T) {
	root := t.TempDir()
	p, err := newTemplateProject(root, "连续捕获")
	if err != nil {
		t.Fatal(err)
	}
	old, err := newCaptureSession(root, p)
	if err != nil {
		t.Fatal(err)
	}
	item := draftCapture{Bounds: ports.Rect{Left: -50, Top: 120, Right: 50, Bottom: 150}, Rule: captureRule{Method: "image", MinSimilarity: .95}}
	shot := image.NewRGBA(image.Rect(0, 0, 100, 30))
	existing, err := old.save(item, shot)
	if err != nil {
		t.Fatal(err)
	}
	if err = old.finish(false); err != nil {
		t.Fatal(err)
	}
	dir := projectDir(root, p)
	legacy := []byte(`{"entries":[]}`)
	if err = os.WriteFile(filepath.Join(dir, "templates.index.json"), legacy, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := newCaptureSession(root, p)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.save(item, shot)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.save(item, shot)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || a.ID == existing.ID || a.ProjectID != p.ID {
		t.Fatal("capture IDs not unique")
	}
	if err = s.undo(); err != nil {
		t.Fatal(err)
	}
	index, err := readCaptureIndex(dir, p.ID)
	if err != nil || len(index.Items) != 2 {
		t.Fatalf("undo failed: %v", err)
	}
	if index.Items[0].ID != existing.ID || !index.Items[0].Confirmed {
		t.Fatal("undo changed saved content")
	}
	if err = s.finish(true); err != nil {
		t.Fatal(err)
	}
	index, err = readCaptureIndex(dir, p.ID)
	if err != nil || len(index.Items) != 1 || index.Items[0].ID != existing.ID {
		t.Fatal("cancel did not isolate session")
	}
	current, _ := os.ReadFile(filepath.Join(dir, "templates.index.json"))
	if string(current) != string(legacy) {
		t.Fatal("capture changed legacy/published index")
	}
	if _, err = captureImage(dir, existing); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, filepath.FromSlash(existing.File)), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = captureImage(dir, existing); err == nil {
		t.Fatal("corrupt capture preview accepted")
	}
	if _, err = s.save(item, shot); err == nil {
		t.Fatal("closed session accepted capture")
	}
}

func TestCaptureLeavesPublishedVersionsUnchanged(t *testing.T) {
	root := t.TempDir()
	p, err := newTemplateProject(root, "已发布")
	if err != nil {
		t.Fatal(err)
	}
	shot := image.NewGray(image.Rect(0, 0, 100, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 100; x++ {
			shot.SetGray(x, y, color.Gray{uint8(x*13 + y*17 + x*y)})
		}
	}
	entry := templates.Entry{ID: "next", Family: "common", Action: "next", Lang: "zh-CN", DPI: 100, Theme: "light"}
	if err = saveCapturedTemplate(projectDir(root, p), shot, image.Rect(10, 10, 90, 40), entry); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "templates")
	if err = publishTemplateProject(root, target, p); err != nil {
		t.Fatal(err)
	}
	releases, err := listTemplateReleases(target, p.ID)
	if err != nil || len(releases) != 1 {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(target, "templates.index.json"), filepath.Join(target, releases[0].Version, "release.json"), filepath.Join(projectDir(root, p), "templates.index.json")}
	for _, e := range releases[0].Entries {
		paths = append(paths, filepath.Join(target, filepath.FromSlash(e.File)), filepath.Join(target, filepath.FromSlash(e.Screenshot)))
	}
	before := map[string]string{}
	for _, path := range paths {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		before[path] = string(b)
	}
	s, err := newCaptureSession(root, p)
	if err != nil {
		t.Fatal(err)
	}
	item := draftCapture{Bounds: ports.Rect{Right: 100, Bottom: 50}, Rule: captureRule{Method: "image", MinSimilarity: .95}}
	if _, err = s.save(item, shot); err != nil {
		t.Fatal(err)
	}
	if err = s.undo(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.save(item, shot); err != nil {
		t.Fatal(err)
	}
	if err = s.finish(true); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		b, e := os.ReadFile(path)
		if e != nil || string(b) != before[path] {
			t.Fatalf("capture changed existing resource %s: %v", path, e)
		}
	}
}

func TestCaptureValidationAndWriteFailure(t *testing.T) {
	root := t.TempDir()
	p, err := newTemplateProject(root, "异常")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newCaptureSession(root, p)
	if err != nil {
		t.Fatal(err)
	}
	item := draftCapture{Bounds: ports.Rect{Right: 60, Bottom: 30}, Rule: captureRule{Method: "uia"}}
	if _, err = s.save(item, image.NewRGBA(image.Rect(0, 0, 60, 30))); err == nil {
		t.Fatal("UIA rule accepted without control")
	}
	item.Rule = captureRule{Method: "image", MinSimilarity: .95}
	if _, err = s.save(item, nil); err == nil {
		t.Fatal("nil screenshot accepted")
	}
	if _, err = s.save(item, image.NewRGBA(image.Rect(0, 0, 20, 20))); err == nil {
		t.Fatal("wrong screenshot dimensions accepted")
	}
	if err = os.Mkdir(filepath.Join(projectDir(root, p), "captures.index.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = s.save(item, image.NewRGBA(image.Rect(0, 0, 60, 30))); err == nil {
		t.Fatal("write failure hidden")
	}
	if len(s.ids) != 0 {
		t.Fatal("failed write registered undo item")
	}
}

func TestUIACapturePreservesRuleAndCoordinates(t *testing.T) {
	root := t.TempDir()
	p, err := newTemplateProject(root, "UIA")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newCaptureSession(root, p)
	if err != nil {
		t.Fatal(err)
	}
	rect := ports.Rect{Left: -300, Top: 100, Right: -150, Bottom: 140}
	c := ports.Control{ID: "next", Text: "下一步", Role: "button", Bounds: rect, Source: "uia", Enabled: true}
	item := draftCapture{Name: c.Text, Bounds: rect, Control: &c, Window: ports.Window{DPI: 144}, Rule: captureRule{Method: "uia", AutomationID: c.ID, ControlType: 50000}}
	saved, err := s.save(item, image.NewRGBA(image.Rect(0, 0, 150, 40)))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finish(false); err != nil {
		t.Fatal(err)
	}
	index, err := readCaptureIndex(projectDir(root, p), p.ID)
	if err != nil || len(index.Items) != 1 {
		t.Fatal(err)
	}
	got := index.Items[0]
	if got.ID != saved.ID || got.Bounds != rect || got.Window.DPI != 144 || got.Rule.Method != "uia" || got.Rule.MinSimilarity != 0 || !got.Confirmed {
		t.Fatal("UIA rule converted to image or coordinates changed")
	}
}
