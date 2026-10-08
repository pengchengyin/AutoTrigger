// Package gui implements semantic arbitration for UIA, template and OCR candidates.
package gui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/resources"
	"autotrigger/pkg/schema"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

type Update = ports.GUIUpdate
type state struct {
	count       int
	lastControl string
	lastHash    [32]byte
	lastClick   time.Time
}
type Automator struct {
	Native    ports.GUIAutomator
	Engine    ports.OCREngine
	Config    config.Config
	Pack      resources.Pack
	Templates *templates.Library
	states    map[uintptr]*state
	rules     []resources.Rule
}

func (a *Automator) Reset() { a.states = map[uintptr]*state{} }

func Normalize(text string) string {
	text = strings.Map(func(r rune) rune {
		if r == '&' {
			return -1
		}
		if r >= 0xff01 && r <= 0xff5e {
			r -= 0xfee0
		}
		if r == 0x3000 {
			return ' '
		}
		return unicode.ToLower(r)
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	text = strings.TrimSpace(strings.Trim(text, "<>»«›‹"))
	return strings.TrimSpace(text)
}
func (a *Automator) Init() error {
	a.states = map[uintptr]*state{}
	a.rules = []resources.Rule{}
	for _, file := range []string{"keywords/buttons.zh-CN.json", "keywords/buttons.en-US.json"} {
		mandatory, err := (resources.Pack{Mode: "builtin"}).Rules(file)
		if err != nil {
			return err
		}
		for _, rule := range mandatory {
			if rule.Action == "deny" {
				a.rules = append(a.rules, rule)
			}
		}
		rules, err := a.Pack.Rules(file)
		if err != nil {
			return err
		}
		a.rules = append(a.rules, rules...)
	}
	if a.Config.GUI.KeywordFile != "buttons.zh-CN.json" && a.Config.GUI.KeywordFile != "buttons.en-US.json" {
		rules, err := a.Pack.Rules(a.Config.GUI.KeywordFile)
		if err != nil {
			return err
		}
		a.rules = append(a.rules, rules...)
	}
	return nil
}
func (a *Automator) semantic(text string) string {
	n := Normalize(text)
	for _, r := range a.rules {
		if r.Action != "deny" {
			continue
		}
		for _, word := range strings.Split(r.Pattern, "|") {
			if n == Normalize(word) {
				return "deny"
			}
		}
	}
	for _, r := range a.rules {
		if r.Action == "deny" {
			continue
		}
		for _, word := range strings.Split(r.Pattern, "|") {
			if n == Normalize(word) {
				return r.Action
			}
		}
	}
	return ""
}
func (a *Automator) signal(text string) string {
	n := Normalize(text)
	for _, r := range a.rules {
		if r.Action != "failure" && r.Action != "network" && r.Action != "reboot" {
			continue
		}
		for _, word := range strings.Split(r.Pattern, "|") {
			if strings.Contains(n, Normalize(word)) {
				return r.Action
			}
		}
	}
	return ""
}
func (a *Automator) Step(ctx context.Context, pids []int) (Update, error) {
	u := Update{Actions: []schema.GUIAction{}, Evidence: []schema.Evidence{}}
	windows, err := a.Native.Windows(pids)
	if err != nil {
		return u, err
	}
	signature := []string{}
	for _, w := range windows {
		controls, err := a.Native.Controls(w)
		if err != nil {
			controls = nil
		}
		signature = append(signature, fmt.Sprintf("%x:%s", w.Handle, w.Title))
		for _, c := range controls {
			signature = append(signature, c.Text+fmt.Sprint(c.Bounds))
			switch a.signal(c.Text) {
			case "failure":
				u.Evidence = append(u.Evidence, schema.Evidence{ID: "N2", Level: "N", Type: "gui_failure", Source: c.Source, Detail: c.Text, At: time.Now()})
			case "network":
				u.RequiresNetwork = true
			case "reboot":
				u.RequiresReboot = true
			}
		}
		candidates := []ports.Control{}
		for _, c := range controls {
			if c.Enabled && (c.Role == "button" || c.Role == "checkbox" || c.Role == "radio") {
				action := a.semantic(c.Text)
				if action != "" && action != "deny" && action != "failure" && action != "network" && action != "reboot" {
					if c.Role == "checkbox" && c.Checked {
						continue
					}
					candidates = append(candidates, c)
				}
			}
		}
		var shot image.Image
		if len(candidates) == 0 {
			shot, err = a.Native.Screenshot(w)
			if err == nil {
				candidates, _ = a.match(ctx, w, shot)
				if len(candidates) == 0 && a.Engine != nil && a.Config.OCR.Enabled {
					started := time.Now()
					texts, e := a.Engine.Recognize(ctx, shot)
					if e == nil {
						for _, t := range texts {
							left, top, right, bottom := t.Box[0].X, t.Box[0].Y, t.Box[2].X, t.Box[2].Y
							h := bottom - top
							action := a.semantic(t.Text)
							if t.Score < a.Config.OCR.MinScore || h < a.Config.OCR.MinTextHeightPX || h > a.Config.OCR.MaxTextHeightPX || right <= left || action == "" || action == "deny" || action == "failure" || action == "network" || action == "reboot" {
								continue
							}
							r := ports.Rect{Left: w.Bounds.Left + left, Top: w.Bounds.Top + top, Right: w.Bounds.Left + right, Bottom: w.Bounds.Top + bottom}
							candidates = append(candidates, ports.Control{ID: fmt.Sprint(t.Box), Text: t.Text, Role: "button", Bounds: r, Window: w, Source: "ocr", Confidence: t.Score, Enabled: true, RecognizeMS: time.Since(started).Milliseconds()})
						}
					}
				}
			}
		}
		if len(candidates) == 0 {
			if a.Config.GUI.DumpFailedShots && shot != nil {
				a.saveFailed(w, shot)
			}
			continue
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			pi, pj := rank(a.semantic(candidates[i].Text)), rank(a.semantic(candidates[j].Text))
			if pi != pj {
				return pi < pj
			}
			return candidates[i].Confidence > candidates[j].Confidence
		})
		c := candidates[0]
		s := a.states[w.Handle]
		if s == nil {
			s = &state{}
			a.states[w.Handle] = s
		}
		if s.count >= a.Config.GUI.MaxActionsPerWindow {
			continue
		}
		if time.Since(s.lastClick) < 750*time.Millisecond {
			continue
		}
		if shot == nil {
			shot, _ = a.Native.Screenshot(w)
		}
		hash := shotHash(shot)
		key := c.ID + ":" + Normalize(c.Text)
		if key == s.lastControl && hash == s.lastHash {
			continue
		}
		if ctx.Err() != nil {
			return u, ctx.Err()
		}
		if err = a.Native.Click(c); err != nil {
			continue
		}
		s.count++
		s.lastControl = key
		s.lastHash = hash
		s.lastClick = time.Now()
		action := a.semantic(c.Text)
		g := schema.GUIAction{Window: w.Title, ControlText: c.Text, Source: c.Source, Action: "click", At: time.Now()}
		if c.Source == "ocr" {
			g.OCRScore = &c.Confidence
			g.RecognizeMS = &c.RecognizeMS
		}
		u.Actions = append(u.Actions, g)
		if action == "finish" {
			u.Evidence = append(u.Evidence, schema.Evidence{ID: "A2", Level: "A", Type: "gui_finish_clicked", Source: c.Source, Detail: c.Text, At: time.Now()})
		}
	}
	u.Signature = strings.Join(signature, "|")
	return u, nil
}
func rank(action string) int {
	switch action {
	case "accept":
		return 0
	case "install":
		return 1
	case "next":
		return 2
	case "ok":
		return 3
	case "finish":
		return 4
	default:
		return 10
	}
}
func shotHash(img image.Image) [32]byte {
	if img == nil {
		return [32]byte{}
	}
	return sha256.Sum256(templates.Gray(img).Pix)
}
func (a *Automator) match(ctx context.Context, w ports.Window, shot image.Image) ([]ports.Control, error) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	out := []ports.Control{}
	if a.Templates == nil {
		return out, nil
	}
	entries, err := a.Templates.Select(w.DPI, w.Theme)
	if err != nil {
		return nil, err
	}
	g := templates.Gray(shot)
	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		if e.Action == "cancel" || e.Action == "uninstall" || e.Action == "no" || strings.Contains(e.Action, "disabled") {
			continue
		}
		r, score, ok := templates.MatchContext(ctx, g, e.Image, a.Config.GUI.TemplateMinSimilarity)
		if !ok {
			continue
		}
		text := e.Action
		switch text {
		case "accept":
			text = "i agree"
		case "checkbox_off":
			text = "i accept"
		}
		if a.semantic(text) == "" || a.semantic(text) == "deny" {
			continue
		}
		out = append(out, ports.Control{ID: e.ID, Text: text, Role: "button", Bounds: ports.Rect{Left: w.Bounds.Left + r.Min.X, Top: w.Bounds.Top + r.Min.Y, Right: w.Bounds.Left + r.Max.X, Bottom: w.Bounds.Top + r.Max.Y}, Window: w, Source: "template", Confidence: score, Enabled: true})
	}
	return out, nil
}
func (a *Automator) saveFailed(w ports.Window, shot image.Image) {
	os.MkdirAll("shots", 0755)
	f, err := os.Create(filepath.Join("shots", fmt.Sprintf("failed_%x_%d.png", w.Handle, time.Now().UnixMilli())))
	if err == nil {
		png.Encode(f, shot)
		f.Close()
	}
}

func (a *Automator) Dump(ctx context.Context, pids []int, dir string) error {
	windows, err := a.Native.Windows(pids)
	if err != nil {
		return err
	}
	if len(windows) == 0 {
		return fmt.Errorf("no sample windows visible")
	}
	for _, sub := range []string{"screenshots", "uia", "ocr"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return err
		}
	}
	for i, w := range windows {
		name := fmt.Sprintf("win_%04d", i+1)
		shot, err := a.Native.Screenshot(w)
		if err != nil {
			return err
		}
		f, err := os.Create(filepath.Join(dir, "screenshots", name+".png"))
		if err != nil {
			return err
		}
		err = png.Encode(f, shot)
		f.Close()
		if err != nil {
			return err
		}
		controls, _ := a.Native.Controls(w)
		if controls == nil {
			controls = []ports.Control{}
		}
		if err = writeJSON(filepath.Join(dir, "uia", name+".json"), controls); err != nil {
			return err
		}
		texts := []ports.OCRText{}
		if a.Engine != nil {
			if got, e := a.Engine.Recognize(ctx, shot); e == nil {
				texts = got
			}
		}
		if err = writeJSON(filepath.Join(dir, "ocr", name+".json"), texts); err != nil {
			return err
		}
	}
	return writeJSON(filepath.Join(dir, "manifest.json"), map[string]interface{}{"captured_at": time.Now(), "windows": windows, "coordinate_space": "physical_screen_pixels", "dpi_format": "native_dpi_96_equals_100_percent"})
}
func writeJSON(path string, v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
