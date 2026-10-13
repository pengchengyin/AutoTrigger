package templates

import (
	"autotrigger/internal/core/ports"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UserRule struct {
	Theme        string      `json:"theme,omitempty"`
	ProjectID    string      `json:"project_id"`
	ID           string      `json:"id"`
	Method       string      `json:"method"`
	WindowTitle  string      `json:"window_title"`
	Name         string      `json:"name"`
	AutomationID string      `json:"automation_id,omitempty"`
	Role         string      `json:"role,omitempty"`
	Action       string      `json:"action,omitempty"`
	DPI          int         `json:"dpi"`
	Threshold    float64     `json:"threshold,omitempty"`
	File         string      `json:"file"`
	SHA256       string      `json:"sha256"`
	Image        *image.Gray `json:"-"`
}

type UserIndex struct {
	Managed  bool       `json:"-"`
	Version  int        `json:"version"`
	Projects []string   `json:"projects"`
	Rules    []UserRule `json:"rules"`
}

func SafeUserAction(action string) bool {
	switch action {
	case "next", "install", "accept", "ok", "yes", "finish":
		return true
	}
	return false
}

func LoadUserIndex(root string) (UserIndex, error) {
	index := UserIndex{Version: 1, Projects: []string{}, Rules: []UserRule{}}
	b, err := os.ReadFile(filepath.Join(root, "template-projects", "active.index.json"))
	if os.IsNotExist(err) {
		return index, nil
	}
	if err != nil {
		return index, err
	}
	if err = json.Unmarshal(b, &index); err != nil {
		return index, err
	}
	index.Managed = true
	return index, ValidateUserIndex(root, &index)
}

func ValidateUserIndex(root string, index *UserIndex) error {
	if index.Version != 1 {
		return fmt.Errorf("unsupported user template version")
	}
	projects := map[string]bool{}
	for _, id := range index.Projects {
		if id == "" || id == "." || !safePath(id) || filepath.Base(id) != id || projects[id] {
			return fmt.Errorf("invalid project ID")
		}
		projects[id] = true
	}
	seen := map[string]bool{}
	for i := range index.Rules {
		r := &index.Rules[i]
		key := r.ProjectID + "/" + r.ID
		if !projects[r.ProjectID] || r.ID == "" || !safePath(r.ID) || filepath.Base(r.ID) != r.ID || seen[key] || !safePath(r.File) || r.DPI < 1 || r.Method != "legacy_image" && r.WindowTitle == "" || (r.Method != "uia" && r.Method != "image" && r.Method != "legacy_image") || r.Action != "" && !SafeUserAction(r.Action) {
			return fmt.Errorf("invalid capture rule: %s", key)
		}
		seen[key] = true
		if r.Method == "uia" && r.Name == "" && r.AutomationID == "" {
			return fmt.Errorf("UIA rule lacks identity: %s", key)
		}
		if r.Method == "uia" && r.Role != "button" && r.Role != "checkbox" && r.Role != "radio" {
			return fmt.Errorf("UIA capture is not an actionable control: %s", key)
		}
		if (r.Method == "image" || r.Method == "legacy_image") && (r.Action == "" || r.Threshold <= 0 || r.Threshold > 1) {
			return fmt.Errorf("image rule requires action and threshold: %s", key)
		}
		expected := filepath.ToSlash(filepath.Join("template-projects", r.ProjectID, "captures", r.ID, "image.png"))
		if r.Method != "legacy_image" && r.File != expected || r.Method == "legacy_image" && !strings.HasPrefix(r.File, "template-projects/"+r.ProjectID+"/") {
			return fmt.Errorf("capture path mismatch: %s", key)
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(r.File)))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != r.SHA256 {
			return fmt.Errorf("capture checksum mismatch: %s", key)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(b))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 25000000 {
			return fmt.Errorf("invalid capture PNG: %s", key)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			return err
		}
		r.Image = Gray(img)
	}
	for id := range projects {
		found := false
		for _, r := range index.Rules {
			found = found || r.ProjectID == id
		}
		if !found {
			return fmt.Errorf("project has no enabled rules: %s", id)
		}
	}
	return nil
}

func (index UserIndex) Controls(w ports.Window, controls []ports.Control, semantic func(string) string) []ports.Control {
	out := []ports.Control{}
	for _, rule := range index.Rules {
		if rule.Method != "uia" || rule.WindowTitle != w.Title {
			continue
		}
		action := rule.Action
		if action == "" {
			action = semantic(rule.Name)
		}
		if !SafeUserAction(action) {
			continue
		}
		for _, c := range controls {
			if c.Source != "uia" || !c.Enabled || c.Checked || c.Role != "button" && c.Role != "checkbox" && c.Role != "radio" || semantic(c.Text) == "deny" {
				continue
			}
			if rule.AutomationID != "" && rule.AutomationID != c.ID || rule.Name != "" && rule.Name != c.Text || rule.Role != "" && rule.Role != c.Role {
				continue
			}
			if known := semantic(c.Text); known != "" && known != action {
				continue
			}
			c.RuleAction = action
			c.UserTemplate = rule.ProjectID + "/" + rule.ID
			out = append(out, c)
		}
	}
	return out
}

func (index UserIndex) Images(ctx context.Context, w ports.Window, shot image.Image) []ports.Control {
	out := []ports.Control{}
	if len(index.Rules) == 0 || shot == nil {
		return out
	}
	haystack := Gray(shot)
	for _, rule := range index.Rules {
		if ctx.Err() != nil {
			break
		}
		if rule.Method != "image" && rule.Method != "legacy_image" || rule.WindowTitle != "" && rule.WindowTitle != w.Title || rule.DPI != w.DPI || rule.Theme != "" && rule.Theme != w.Theme || rule.Image == nil {
			continue
		}
		matchCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		r, score, ok := MatchContext(matchCtx, haystack, rule.Image, rule.Threshold)
		cancel()
		if !ok {
			continue
		}
		out = append(out, ports.Control{ID: rule.ProjectID + "/" + rule.ID, Text: rule.Name, Role: "button", Bounds: ports.Rect{Left: w.Bounds.Left + r.Min.X, Top: w.Bounds.Top + r.Min.Y, Right: w.Bounds.Left + r.Max.X, Bottom: w.Bounds.Top + r.Max.Y}, Window: w, Source: "template", Confidence: score, Enabled: true, RuleAction: rule.Action, UserTemplate: rule.ProjectID + "/" + rule.ID})
	}
	return out
}

func (index UserIndex) HasImages(w ports.Window) bool {
	for _, rule := range index.Rules {
		if (rule.Method == "image" || rule.Method == "legacy_image") && (rule.WindowTitle == "" || rule.WindowTitle == w.Title) && rule.DPI == w.DPI && (rule.Theme == "" || rule.Theme == w.Theme) {
			return true
		}
	}
	return false
}
