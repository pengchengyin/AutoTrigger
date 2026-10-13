package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterialSourceSupportsLegacyAndNewLayout(t *testing.T) {
	root := t.TempDir()
	path := "third_parties/ocr/package.zip"
	old := filepath.Join(root, "dist", "third_party", "ocr", "package.zip")
	if err := os.MkdirAll(filepath.Dir(old), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if materialSource(root, path) != old {
		t.Fatal("legacy source unavailable")
	}
	current := filepath.Join(root, "dist", "third_parties", "ocr", "package.zip")
	if err := os.MkdirAll(filepath.Dir(current), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte("current"), 0600); err != nil {
		t.Fatal(err)
	}
	if materialSource(root, path) != current {
		t.Fatal("new layout not preferred")
	}
}

func TestDeliveryKeepsSourceAndRuntimeSeparate(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "src")
	if err = os.MkdirAll(sourceRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(sourceRoot); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		"dist/AutoTrigger.exe":                 "packaging fixture; never executed",
		"README.md":                            "quick start",
		"src/pkg/schema/result.schema.json":    "{}",
		"docs/guide.md":                        "template guide",
		"src/scripts/build.ps1":                "developer-only fixture",
		"dist/third_party/7zip/7z2603-x64.exe": "offline fixture; never executed",
	} {
		path = filepath.Join(root, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(root, "dist", "delivery")
	if err = assemble(filepath.Join("dist", "delivery"), false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AutoTrigger.exe", "README.md", "result.schema.json", "manifest.json", "docs/guide.md", "third_parties/7zip/7z2603-x64.exe"} {
		if _, err = os.Stat(filepath.Join(dest, filepath.FromSlash(name))); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, name := range []string{"DEPLOY.md", "scripts", "src"} {
		if _, err = os.Stat(filepath.Join(dest, name)); !os.IsNotExist(err) {
			t.Fatal("removed delivery content still copied", name, err)
		}
	}
	archive, err := zip.OpenReader(dest + ".zip")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name == "DEPLOY.md" || strings.HasPrefix(entry.Name, "scripts/") {
			t.Fatal("obsolete content in ZIP", entry.Name)
		}
	}
	if err = assemble(dest, false); err == nil {
		t.Fatal("existing delivery overwritten")
	}
	if err = assemble(dest+"-release", true); err == nil {
		t.Fatal("incomplete offline materials accepted for release")
	}
	if err = assemble(filepath.Join("src", "invalid-delivery"), false); err == nil {
		t.Fatal("delivery allowed inside source")
	}
}
