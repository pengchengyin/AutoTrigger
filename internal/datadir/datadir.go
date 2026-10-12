// Package datadir defines the portable data layout shared by GUI and CLI.
package datadir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Layout struct{ Root string }

func (l Layout) Logs() string         { return filepath.Join(l.Root, "logs") }
func (l Layout) Results() string      { return filepath.Join(l.Root, "results") }
func (l Layout) Templates() string    { return filepath.Join(l.Root, "templates") }
func (l Layout) ThirdParties() string { return filepath.Join(l.Root, "third_parties") }
func (l Layout) Config() string       { return filepath.Join(l.Root, "config", "config.json") }
func (l Layout) History() string      { return filepath.Join(l.Root, "config", "history") }
func (l Layout) Ensure() error {
	for _, dir := range []string{l.Logs(), l.Results(), l.Templates(), l.ThirdParties(), filepath.Dir(l.Config())} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// Only this locator remains beside the EXE when data is stored elsewhere.
func locator(exeDir string) string { return filepath.Join(exeDir, "config", "data-location.json") }
func Load(exeDir string) (Layout, error) {
	root, err := filepath.Abs(exeDir)
	if err != nil {
		return Layout{}, err
	}
	b, err := os.ReadFile(locator(exeDir))
	if os.IsNotExist(err) {
		return Layout{root}, nil
	}
	if err != nil {
		return Layout{}, err
	}
	var doc struct {
		Root string `json:"data_directory"`
	}
	if err = json.Unmarshal(b, &doc); err != nil {
		return Layout{}, err
	}
	if !filepath.IsAbs(doc.Root) || strings.TrimSpace(doc.Root) == "" {
		return Layout{}, fmt.Errorf("数据目录记录无效")
	}
	return Layout{filepath.Clean(doc.Root)}, nil
}
func Save(exeDir, root string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("数据目录不能为空")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(struct {
		Root string `json:"data_directory"`
	}{root}, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(locator(exeDir), b)
}
func AtomicWrite(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".data-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
