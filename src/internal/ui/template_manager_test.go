package ui

import (
	"autotrigger/internal/gui/templates"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemplateFixture(t *testing.T, dir string, colored bool) {
	t.Helper()
	var img image.Image
	gray := image.NewGray(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			gray.SetGray(x, y, color.Gray{Y: uint8((x*17 + y*29) % 256)})
		}
	}
	img = gray
	if colored {
		img = image.NewRGBA(gray.Bounds())
	}
	for _, name := range []string{"button.png", "source.png"} {
		file, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, img)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	index := templates.Index{Version: "test-only", Entries: []templates.Entry{{ID: "test-only-button", Family: "generic", Action: "next", Lang: "zh-CN", DPI: 100, Theme: "light", File: "button.png", Screenshot: "source.png", Width: 24, Height: 16}}}
	b, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(dir, "templates.index.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateConfigPreservesOtherFields(t *testing.T) {
	body := `{"ocr":{"engine":"rapidocr_json"},"gui_automation":{"template_pack":"external","template_dir":"original","builtin_templates":false,"template_min_similarity":0.9},"extra":{"large":9007199254740993}}`
	for _, mode := range []string{"merged", "builtin", "external"} {
		patched, err := templateConfigBody(body, mode, "")
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]json.RawMessage
		json.Unmarshal([]byte(patched), &doc)
		if !strings.Contains(string(doc["extra"]), "9007199254740993") || !strings.Contains(string(doc["ocr"]), "rapidocr_json") {
			t.Fatal("unrelated fields lost")
		}
		var gui struct {
			Pack    string  `json:"template_pack"`
			Dir     string  `json:"template_dir"`
			Builtin bool    `json:"builtin_templates"`
			Score   float64 `json:"template_min_similarity"`
		}
		json.Unmarshal(doc["gui_automation"], &gui)
		if gui.Pack != mode || gui.Dir != "original" || gui.Score != 0.9 || gui.Builtin != (mode != "external") {
			t.Fatalf("wrong patch: %+v", gui)
		}
	}
	for _, body := range []string{"null", "[]", "bad", `{"gui_automation":[]}`} {
		if _, err := templateConfigBody(body, "merged", ""); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestExternalTemplateValidation(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadExternalTemplateDir(dir); err == nil {
		t.Fatal("missing index accepted")
	}
	writeTemplateFixture(t, dir, false)
	lib, err := loadExternalTemplateDir(dir)
	if err != nil || len(lib.Entries) != 1 {
		t.Fatalf("load: %v", err)
	}
	if _, err := lib.SelfTest(); err != nil {
		t.Fatal(err)
	}
	preview, err := templatePreviewFile(lib.Entries[0], t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, _ := os.ReadFile(filepath.Join(dir, "button.png"))
	copy, _ := os.ReadFile(preview)
	if string(source) != string(copy) {
		t.Fatal("preview altered original template bytes")
	}
	writeTemplateFixture(t, dir, true)
	if _, err := loadExternalTemplateDir(dir); err == nil {
		t.Fatal("colored PNG accepted")
	}
	writeTemplateFixture(t, dir, false)
	if err := os.Remove(filepath.Join(dir, "button.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := loadExternalTemplateDir(dir); err == nil {
		t.Fatal("missing PNG accepted")
	}
}

func TestTemplateImportRejectsInvalidMetadata(t *testing.T) {
	for _, kind := range []string{"duplicate", "dpi", "theme", "family", "unsafe-path"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			writeTemplateFixture(t, dir, false)
			path := filepath.Join(dir, "templates.index.json")
			data, _ := os.ReadFile(path)
			var index templates.Index
			if err := json.Unmarshal(data, &index); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "duplicate":
				index.Entries = append(index.Entries, index.Entries[0])
			case "dpi":
				index.Entries[0].DPI = 91
			case "theme":
				index.Entries[0].Theme = "unknown"
			case "family":
				index.Entries[0].Family = ""
			case "unsafe-path":
				index.Entries[0].File = "../outside.png"
			}
			data, _ = json.Marshal(index)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadExternalTemplateDir(dir); err == nil {
				t.Fatal("invalid template accepted")
			}
		})
	}
}

func TestTemplateInventoryIndependentOfActiveMode(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFixture(t, dir, false)
	path := filepath.Join(dir, "config.json")
	b, _ := json.Marshal(map[string]interface{}{"gui_automation": map[string]interface{}{"template_pack": "builtin", "template_dir": dir}})
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	info := loadTemplateInfoWithConfig(dir, path)
	if info.Error != "" || info.Mode != "builtin" || info.External != 1 {
		t.Fatalf("inventory: %+v", info)
	}
	if !strings.Contains(templateDetails(info, "external"), "test-only-button") || strings.Contains(templateDetails(info, "builtin"), "test-only-button") {
		t.Fatal("category filtering failed")
	}
}
