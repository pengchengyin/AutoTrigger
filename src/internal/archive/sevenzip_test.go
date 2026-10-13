package archive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListingAndLimits(t *testing.T) {
	entries, err := ParseList("Path = setup.exe\nSize = 42\nAttributes = A\n\nPath = assets\nSize = 0\nFolder = +\n\n")
	if err != nil || len(entries) != 2 || entries[0].Size != 42 || !entries[1].Directory {
		t.Fatalf("%+v %v", entries, err)
	}
	if err = Size(entries, 41); err == nil {
		t.Fatal("size limit bypassed")
	}
	for _, path := range []string{"../escape.exe", `C:\escape.exe`, `..\escape.exe`, "/escape.exe"} {
		if _, err = ParseList("Path = " + path + "\nSize = 1\n"); err == nil {
			t.Fatal("unsafe path accepted", path)
		}
	}
	if _, err = ParseList("Path = link\nSymbolic Link = ../escape\n"); err == nil {
		t.Fatal("link accepted")
	}
	if _, err = ParseList("Path = x\nSize = -1\n"); err == nil {
		t.Fatal("negative size accepted")
	}
}
func TestExtractionBudget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("1234"), 0600)
	if ValidateTree(dir, 4) != nil {
		t.Fatal("legal tree rejected")
	}
	if ValidateTree(dir, 3) == nil {
		t.Fatal("actual extraction size ignored")
	}
}
