package ui

import (
	"autotrigger/internal/gui/templates"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveCapturedTemplate(t *testing.T) {
	dir := t.TempDir()
	shot := image.NewRGBA(image.Rect(0, 0, 180, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 180; x++ {
			shot.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x * y), 255})
		}
	}
	e := templates.Entry{ID: "test-only", Family: "common", Action: "next", Lang: "zh-CN", DPI: 100, Theme: "light", Origin: "regression fixture, not installer asset"}
	bounds := image.Rect(14, 20, 94, 46)
	if err := saveCapturedTemplate(dir, shot, bounds, e); err != nil {
		t.Fatal(err)
	}
	lib, err := loadExternalTemplateDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Entries) != 1 || lib.Entries[0].Bounds != bounds {
		t.Fatalf("incorrect entry: %+v", lib.Entries)
	}
	if failures, err := lib.SelfTest(); err != nil {
		t.Fatalf("%v: %v", err, failures)
	}
	f, err := os.Open(filepath.Join(dir, lib.Entries[0].File))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	gray, ok := img.(*image.Gray)
	if !ok {
		t.Fatal("not grayscale")
	}
	if gray.GrayAt(0, 0) != color.GrayModel.Convert(shot.At(14, 20)).(color.Gray) {
		t.Fatal("incorrect crop pixels")
	}
	indexPath := filepath.Join(dir, "templates.index.json")
	before, _ := os.ReadFile(indexPath)
	if err := saveCapturedTemplate(dir, shot, bounds, e); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	after, _ := os.ReadFile(indexPath)
	if !bytes.Equal(before, after) {
		t.Fatal("duplicate overwrote index")
	}
	var doc map[string]json.RawMessage
	json.Unmarshal(before, &doc)
	doc["custom_note"] = json.RawMessage(`"preserve me"`)
	updated, _ := json.Marshal(doc)
	os.WriteFile(indexPath, updated, 0600)
	e.ID = "second-test-only"
	if err := saveCapturedTemplate(dir, shot, bounds, e); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(indexPath)
	json.Unmarshal(after, &doc)
	if string(doc["custom_note"]) != `"preserve me"` {
		t.Fatal("unknown index metadata lost")
	}
	lib, err = loadExternalTemplateDir(dir)
	if err != nil || len(lib.Entries) != 2 {
		t.Fatalf("append failed: %v", err)
	}
}

func TestCapturedTemplateRejectsInvalid(t *testing.T) {
	shot := image.NewGray(image.Rect(0, 0, 200, 100))
	e := templates.Entry{ID: "fixture", Family: "common", Action: "next", Lang: "en-US", DPI: 100, Theme: "light"}
	for _, b := range []image.Rectangle{image.Rect(-1, 0, 79, 26), image.Rect(0, 0, 201, 26), image.Rect(0, 0, 20, 26), image.Rect(0, 0, 80, 49), {}} {
		dir := t.TempDir()
		if err := saveCapturedTemplate(dir, shot, b, e); err == nil {
			t.Errorf("accepted invalid crop: %v", b)
		}
		if _, err := os.Stat(filepath.Join(dir, "templates.index.json")); !os.IsNotExist(err) {
			t.Fatal("invalid crop published index")
		}
	}
	dir := t.TempDir()
	bad := []byte(`{"entries":"corrupted"}`)
	os.WriteFile(filepath.Join(dir, "templates.index.json"), bad, 0600)
	if err := saveCapturedTemplate(dir, shot, image.Rect(0, 0, 80, 26), e); err == nil {
		t.Fatal("corrupt index overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "templates.index.json"))
	if !bytes.Equal(bad, after) {
		t.Fatal("corrupt index changed")
	}
}

func TestTemplateCropCoordinates(t *testing.T) {
	display := templateImageRect(image.Rect(10, 20, 510, 420), image.Pt(1000, 600))
	if display != image.Rect(10, 70, 510, 370) {
		t.Fatalf("bad aspect ratio: %v", display)
	}
	if got := templateImagePoint(image.Pt(260, 220), display, image.Pt(1000, 600)); got != image.Pt(500, 300) {
		t.Fatal(got)
	}
	if got := templateImagePoint(image.Pt(-10, 999), display, image.Pt(1000, 600)); got != image.Pt(0, 600) {
		t.Fatal(got)
	}
}
