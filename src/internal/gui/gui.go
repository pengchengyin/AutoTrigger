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
	"errors"
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
	retries     int
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
	// Localized Win32 labels often end with an accelerator, e.g. Next(&N).
	if len(text) >= 3 && text[len(text)-3] == '(' && text[len(text)-1] == ')' {
		key := text[len(text)-2]
		if key >= 'a' && key <= 'z' {
			text = strings.TrimSpace(text[:len(text)-3])
		}
	}
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

func (a *Automator) controlAction(c ports.Control) string {
	if a.semantic(c.Text) == "deny" {
		return "deny"
	}
	if c.UserTemplate != "" && templates.SafeUserAction(c.RuleAction) {
		return c.RuleAction
	}
	return a.semantic(c.Text)
}

func (a *Automator) ActionForText(text string) string { return a.semantic(text) }
func (a *Automator) Step(ctx context.Context, pids []int) (Update, error) {
	u := Update{Actions: []schema.GUIAction{}, Evidence: []schema.Evidence{}, Notes: []string{}}
	windows, err := a.Native.Windows(pids)
	if err != nil {
		return u, err
	}
	signature := []string{}
	visible := map[uintptr]bool{}
	var clickError error
	for _, w := range windows {
		visible[w.Handle] = true
		controls, err := a.Native.Controls(w)
		if err != nil {
			controls = nil
		}
		signature = append(signature, fmt.Sprintf("%x:%s", w.Handle, w.Title))
		texts := []string{}
		for _, c := range controls {
			signature = append(signature, c.Text+fmt.Sprint(c.Bounds))
			if c.Text != "" && c.Role != "text" && len(texts) < 16 {
				label := []rune(c.Text)
				if len(label) > 120 {
					label = label[:120]
				}
				texts = append(texts, fmt.Sprintf("%s/%s/可用=%t/选中=%t", string(label), c.Role, c.Enabled, c.Checked))
			}
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
		if a.Templates != nil {
			candidates = append(candidates, a.Templates.User.Controls(w, controls, a.semantic)...)
		}
		for _, c := range controls {
			if c.Enabled && (c.Role == "button" || c.Role == "checkbox" || c.Role == "radio") {
				action := a.semantic(c.Text)
				if action != "" && action != "deny" && action != "failure" && action != "network" && action != "reboot" {
					if (c.Role == "checkbox" || c.Role == "radio") && c.Checked {
						continue
					}
					candidates = append(candidates, c)
				}
			}
		}
		var shot image.Image
		if a.Templates != nil && a.Templates.User.HasImages(w) {
			shot, err = a.Native.Screenshot(w)
			if err == nil && shot != nil {
				candidates = append(candidates, a.Templates.User.Images(ctx, w, shot)...)
			} else {
				u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q 用户图片匹配截图失败：%v", w.Title, err))
			}
		}
		if len(candidates) == 0 {
			if shot == nil {
				shot, err = a.Native.Screenshot(w)
			}
			if err != nil {
				u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q 截图失败，无法进行模板/OCR 回退：%v", w.Title, err))
			}
			if err == nil {
				candidates, _ = a.match(ctx, w, shot)
				if len(candidates) == 0 && a.Engine != nil && a.Config.OCR.Enabled {
					started := time.Now()
					texts, e := a.Engine.Recognize(ctx, shot)
					if e != nil {
						u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q OCR 识别失败：%v", w.Title, e))
					} else {
						u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q 已调用 OCR：识别文本 %d 项", w.Title, len(texts)))
					}
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
				} else if len(candidates) == 0 {
					u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q 未命中模板，OCR 未启用或引擎未就绪", w.Title))
				}
			}
		}
		filtered := candidates[:0]
		for _, candidate := range candidates {
			blocked := false
			if candidate.Source == "ocr" || candidate.Source == "template" {
				x := (candidate.Bounds.Left + candidate.Bounds.Right) / 2
				y := (candidate.Bounds.Top + candidate.Bounds.Bottom) / 2
				for _, control := range controls {
					if (!control.Enabled || a.semantic(control.Text) == "deny" || control.Checked && (control.Role == "checkbox" || control.Role == "radio")) && x >= control.Bounds.Left && x < control.Bounds.Right && y >= control.Bounds.Top && y < control.Bounds.Bottom {
						blocked = true
					}
				}
			}
			if !blocked {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
		if len(candidates) == 0 {
			if a.Config.GUI.DumpFailedShots && shot != nil {
				a.saveFailed(w, shot)
			}
			u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q（PID %d）未识别到可点击控件：%s", w.Title, w.PID, strings.Join(texts, "、")))
			continue
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if (candidates[i].UserTemplate != "") != (candidates[j].UserTemplate != "") {
				return candidates[i].UserTemplate != ""
			}
			pi, pj := rank(a.controlAction(candidates[i])), rank(a.controlAction(candidates[j]))
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
			u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q 已达到单窗口点击上限 %d 次，本轮不再点击", w.Title, a.Config.GUI.MaxActionsPerWindow))
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
			if c.Role == "checkbox" || c.Role == "radio" || s.retries >= 2 {
				u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q 的控件 %q 已点击且无变化，不重复切换选项或超过重试上限", w.Title, c.Text))
				continue
			}
			if time.Since(s.lastClick) < 3*time.Second {
				u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q（句柄 %#x，PID %d）的控件 %q 刚点击过且画面无变化，稍候重试", w.Title, w.Handle, w.PID, c.Text))
				continue
			}
			// 画面长时间无变化仍按上限有节制地重试：点击可能被前台切换、
			// 焦点抢占或覆盖窗口等瞬态原因吞掉，永久放弃会让向导卡死。
			u.Notes = append(u.Notes, fmt.Sprintf("窗口 %q（句柄 %#x，PID %d，范围 %v）的控件 %q（范围 %v）点击后画面无变化，重试点击（本窗口第 %d 次，上限 %d）", w.Title, w.Handle, w.PID, w.Bounds, c.Text, c.Bounds, s.count+1, a.Config.GUI.MaxActionsPerWindow))
		}
		if ctx.Err() != nil {
			return u, ctx.Err()
		}
		if err = a.Native.Click(c); err != nil {
			clickError = fmt.Errorf("窗口 %q 的按钮 %q（%s）点击失败：%w", w.Title, c.Text, c.Source, err)
			if errors.Is(err, ports.ErrElevationRequired) {
				return u, clickError
			}
			continue
		}
		if key == s.lastControl && hash == s.lastHash {
			s.retries++
		} else {
			s.retries = 0
		}
		s.count++
		s.lastControl = key
		s.lastHash = hash
		s.lastClick = time.Now()
		action := a.controlAction(c)
		if c.UserTemplate != "" {
			u.Notes = append(u.Notes, fmt.Sprintf("用户模板命中：%s，方式=%s，分数=%.4f，动作=%s，点击已提交", c.UserTemplate, c.Source, c.Confidence, action))
		}
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
	for handle := range a.states {
		if !visible[handle] {
			delete(a.states, handle)
		}
	}
	u.Signature = strings.Join(signature, "|")
	if len(u.Actions) == 0 && clickError != nil {
		return u, clickError
	}
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
func (a *Automator) saveShot(w ports.Window, shot image.Image, kind string) {
	os.MkdirAll("shots", 0755)
	f, err := os.Create(filepath.Join("shots", fmt.Sprintf("%s_%x_%d.png", kind, w.Handle, time.Now().UnixMilli())))
	if err == nil {
		png.Encode(f, shot)
		f.Close()
	}
}

func (a *Automator) saveFailed(w ports.Window, shot image.Image) {
	a.saveShot(w, shot, "failed")
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
