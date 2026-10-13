// Package resources embeds and merges the offline rule and template pack.
package resources

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const Version = "0.1.0"

//go:embed assets/builtin
var FS embed.FS

type Rule struct {
	ID      string   `json:"id"`
	Pattern string   `json:"pattern"`
	Action  string   `json:"action"`
	Args    []string `json:"args"`
	Kind    string   `json:"kind"`
	Offset  int      `json:"offset"`
	Hex     string   `json:"hex"`
	Source  string   `json:"source"`
}
type Pack struct {
	Mode     string
	Root     string
	Warnings []string
}

func (p Pack) Rules(name string) ([]Rule, error) {
	var out []Rule
	if p.Mode != "external" {
		b, err := FS.ReadFile("assets/builtin/" + name)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &out); err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Source = "builtin"
		}
	}
	if p.Mode == "builtin" {
		return out, nil
	}
	b, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(name)))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var extra []Rule
	if err = json.Unmarshal(b, &extra); err != nil {
		return nil, fmt.Errorf("resource %s: %w", name, err)
	}
	byID := map[string]int{}
	for i, r := range out {
		byID[r.ID] = i
	}
	for _, r := range extra {
		if r.ID == "" {
			return nil, fmt.Errorf("resource %s: rule id required", name)
		}
		r.Source = "external"
		if i, ok := byID[r.ID]; ok {
			out[i] = r
		} else {
			byID[r.ID] = len(out)
			out = append(out, r)
		}
	}
	return out, nil
}

func Export(dir string) error {
	return fs.WalkDir(FS, "assets/builtin", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "assets/builtin")
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		b, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0644)
	})
}
