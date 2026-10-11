package ui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/gui/templates"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestMultipleDraftProjectsPublishIndependently(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "templates")
	shot := image.NewGray(image.Rect(0, 0, 140, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 140; x++ {
			shot.SetGray(x, y, color.Gray{uint8(x*y + x*7 + y*13)})
		}
	}
	entry := templates.Entry{ID: "next", Family: "common", Action: "next", Lang: "zh-CN", DPI: 100, Theme: "light"}
	a, err := newTemplateProject(root, "软件甲")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newTemplateProject(root, "软件乙")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []templateProject{a, b} {
		if err = saveCapturedTemplate(projectDir(root, p), shot, image.Rect(10, 10, 90, 36), entry); err != nil {
			t.Fatal(err)
		}
	}
	c := config.Defaults().GUI
	c.TemplateDir = target
	c.TemplatePack = "external"
	lib, err := templates.Load(c, root)
	if err != nil || len(lib.Entries) != 0 {
		t.Fatal("unpublished drafts leaked into runtime")
	}
	if err = publishTemplateProject(root, target, a); err != nil {
		t.Fatal(err)
	}
	if err = publishTemplateProject(root, target, b); err != nil {
		t.Fatal(err)
	}
	lib, err = templates.Load(c, root)
	if err != nil || len(lib.Entries) != 2 || lib.Entries[0].ID == lib.Entries[1].ID {
		t.Fatalf("projects not merged safely: %+v, %v", lib, err)
	}
	selected, err := lib.Select(96, "light")
	if err != nil || len(selected) != 2 {
		t.Fatalf("published templates unavailable to execution: %v", err)
	}
	entry.ID = "install"
	entry.Action = "install"
	if err = saveCapturedTemplate(projectDir(root, a), shot, image.Rect(20, 20, 100, 46), entry); err != nil {
		t.Fatal(err)
	}
	lib, _ = templates.Load(c, root)
	if len(lib.Entries) != 2 {
		t.Fatal("draft edit changed live index")
	}
	if err = publishTemplateProject(root, target, a); err != nil {
		t.Fatal(err)
	}
	lib, err = templates.Load(c, root)
	if err != nil || len(lib.Entries) != 3 {
		t.Fatal("republish lost another project or duplicated entries")
	}
	projects, err := listTemplateProjects(root)
	if err != nil || len(projects) != 2 {
		t.Fatal("project persistence failed")
	}
	old, _ := os.ReadFile(filepath.Join(target, "templates.index.json"))
	if err = os.Remove(filepath.Join(projectDir(root, a), "templates.index.json")); err != nil {
		t.Fatal(err)
	}
	if publishTemplateProject(root, target, a) == nil {
		t.Fatal("invalid draft published")
	}
	current, _ := os.ReadFile(filepath.Join(target, "templates.index.json"))
	if string(current) != string(old) {
		t.Fatal("invalid publication damaged working index")
	}
}

func TestEmptyTemplateCannotPublish(t *testing.T) {
	root := t.TempDir()
	p, err := newTemplateProject(root, "空模板")
	if err != nil {
		t.Fatal(err)
	}
	if publishTemplateProject(root, filepath.Join(root, "templates"), p) == nil {
		t.Fatal("empty publication accepted")
	}
}

func TestEmptyIndexCanBeRepopulatedAndPublished(t *testing.T) {
	root := t.TempDir()
	p, err := newTemplateProject(root, "重新发布")
	if err != nil {
		t.Fatal(err)
	}
	draft := projectDir(root, p)
	if err = atomicTemplateFile(filepath.Join(draft, "templates.index.json"), []byte(`{"entries":[]}`)); err != nil {
		t.Fatal(err)
	}
	shot := image.NewGray(image.Rect(0, 0, 120, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 120; x++ {
			shot.SetGray(x, y, color.Gray{uint8(x*y + x*11 + y*3)})
		}
	}
	e := templates.Entry{ID: "next", Family: "common", Action: "next", Lang: "zh-CN", DPI: 100, Theme: "light"}
	if err = saveCapturedTemplate(draft, shot, image.Rect(5, 5, 85, 31), e); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "templates")
	if err = atomicTemplateFile(filepath.Join(target, "templates.index.json"), []byte(`{"entries":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err = publishTemplateProject(root, target, p); err != nil {
		t.Fatal(err)
	}
}
