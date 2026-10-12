package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterialSourceSupportsLegacyAndNewLayout(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err = os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	path := "third_parties/ocr/package.zip"
	old := filepath.Join("dist", "third_party", "ocr", "package.zip")
	if err = os.MkdirAll(filepath.Dir(old), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(old, []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if materialSource(path) != old {
		t.Fatal("legacy source unavailable")
	}
	current := filepath.Join("dist", "third_parties", "ocr", "package.zip")
	if err = os.MkdirAll(filepath.Dir(current), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(current, []byte("current"), 0600); err != nil {
		t.Fatal(err)
	}
	if materialSource(path) != current {
		t.Fatal("new layout not preferred")
	}
}
