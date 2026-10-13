package datadir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortableLayoutAndLocator(t *testing.T) {
	exe := t.TempDir()
	layout, err := Load(exe)
	if err != nil || layout.Root != exe {
		t.Fatal(layout, err)
	}
	if err = layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"logs", "results", "templates", "third_parties", "config"} {
		if info, e := os.Stat(filepath.Join(exe, dir)); e != nil || !info.IsDir() {
			t.Fatal(dir, e)
		}
	}
	data := t.TempDir()
	if err = Save(exe, data); err != nil {
		t.Fatal(err)
	}
	layout, err = Load(exe)
	if err != nil || layout.Root != data {
		t.Fatal(layout, err)
	}
	if err = Save(exe, ""); err == nil {
		t.Fatal("empty path accepted")
	}
	if err = os.WriteFile(locator(exe), []byte(`{"data_directory":"relative"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(exe); err == nil {
		t.Fatal("invalid locator silently ignored")
	}
}
