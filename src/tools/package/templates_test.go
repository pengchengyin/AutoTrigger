package main

import (
	"autotrigger/internal/core/ports"
	"autotrigger/internal/gui/templates"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBundledSelfDrawnTemplate(t *testing.T) {
	source := filepath.Join("..", "..", "delivery", "templates")
	dest := filepath.Join(t.TempDir(), "templates")
	n, err := copyBundledTemplates(source, dest)
	if err != nil || n != 4 {
		t.Fatalf("bundle: %d %v", n, err)
	}
	index, err := templates.LoadUserIndex(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Projects) != 1 || len(index.Rules) != 4 {
		t.Fatal("invalid bundled rule count")
	}
	dir := filepath.Join(dest, "template-projects", index.Projects[0])
	var captures struct {
		Items []struct {
			ID           string     `json:"id"`
			SourceFile   string     `json:"source_file"`
			SourceBounds ports.Rect `json:"source_bounds"`
			Bounds       ports.Rect `json:"screen_bounds"`
		} `json:"items"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "captures.index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &captures); err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, r := range index.Rules {
		actions[r.Action] = true
		for _, c := range captures.Items {
			if c.ID == r.ID {
				f, err := os.Open(filepath.Join(dir, filepath.FromSlash(c.SourceFile)))
				if err != nil {
					t.Fatal(err)
				}
				shot, err := png.Decode(f)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
				one := templates.UserIndex{Rules: []templates.UserRule{r}}
				w := ports.Window{Title: r.WindowTitle, DPI: r.DPI, Bounds: c.SourceBounds}
				matches := one.Images(context.Background(), w, shot)
				if len(matches) != 1 || matches[0].Confidence < .99999 || matches[0].Bounds != c.Bounds {
					t.Fatalf("source matching %s: %+v", r.Action, matches)
				}
			}
		}
	}
	for _, action := range []string{"accept", "next", "install", "finish"} {
		if !actions[action] {
			t.Fatalf("missing action %s", action)
		}
	}
	for _, rule := range index.Rules {
		if rule.WindowTitle != "自绘界面安装向导 Synthetic" || rule.DPI != 96 || rule.Threshold != .95 {
			t.Fatal("scope widened")
		}
	}
	if err = os.WriteFile(filepath.Join(dir, filepath.FromSlash(captures.Items[0].SourceFile)), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = copyBundledTemplates(dest, filepath.Join(t.TempDir(), "broken")); err == nil {
		t.Fatal("corrupt source accepted")
	}
}
