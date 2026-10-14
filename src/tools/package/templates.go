package main

import (
	"autotrigger/internal/gui/templates"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// Delivery templates are reviewed, versioned assets, not local user data.
func copyBundledTemplates(source, dest string) (int, error) {
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	index, err := templates.LoadUserIndex(source)
	if err != nil {
		return 0, err
	}
	if !index.Managed || len(index.Rules) == 0 {
		return 0, fmt.Errorf("bundled templates require a valid enabled index")
	}
	for _, id := range index.Projects {
		dir := filepath.Join(source, "template-projects", id)
		var captures struct {
			ProjectID string `json:"project_id"`
			Items     []struct {
				ID           string `json:"id"`
				Confirmed    bool   `json:"confirmed"`
				File         string `json:"file"`
				SHA256       string `json:"sha256"`
				SourceFile   string `json:"source_file"`
				SourceSHA256 string `json:"source_sha256"`
			} `json:"items"`
		}
		b, e := os.ReadFile(filepath.Join(dir, "captures.index.json"))
		if e != nil {
			return 0, e
		}
		if e = json.Unmarshal(b, &captures); e != nil {
			return 0, e
		}
		if captures.ProjectID != id || len(captures.Items) == 0 {
			return 0, fmt.Errorf("invalid bundled capture index")
		}
		for _, c := range captures.Items {
			if !c.Confirmed {
				return 0, fmt.Errorf("unconfirmed bundled capture")
			}
			for _, asset := range []struct{ file, hash string }{{c.File, c.SHA256}, {c.SourceFile, c.SourceSHA256}} {
				expected := "captures/" + c.ID + "/image.png"
				if asset.file == c.SourceFile {
					expected = "captures/" + c.ID + "/source.png"
				}
				if asset.file != expected {
					return 0, fmt.Errorf("invalid bundled image path")
				}
				data, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(asset.file)))
				if e != nil {
					return 0, e
				}
				if fmt.Sprintf("%x", sha256.Sum256(data)) != asset.hash {
					return 0, fmt.Errorf("bundled image checksum mismatch")
				}
				cfg, e := png.DecodeConfig(bytes.NewReader(data))
				if e != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 25000000 {
					return 0, fmt.Errorf("invalid bundled image")
				}
				if _, e = png.Decode(bytes.NewReader(data)); e != nil {
					return 0, e
				}
			}
		}
	}
	err = filepath.WalkDir(source, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle cannot contain links")
		}
		if !d.IsDir() && !strings.HasSuffix(d.Name(), ".json") && !strings.HasSuffix(d.Name(), ".png") {
			return fmt.Errorf("unexpected bundled template file: %s", d.Name())
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if err = copyTree(source, dest); err != nil {
		return 0, err
	}
	return len(index.Rules), nil
}
