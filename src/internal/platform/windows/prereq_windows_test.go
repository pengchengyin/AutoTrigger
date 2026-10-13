//go:build windows

package windows

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindExecutableRequiresUnpackedOCR(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "third_party", "ocr", "PaddleOCR-json")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "PaddleOCR-json_v1.4.1.7z"), []byte("archive fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join("third_party", "ocr", "PaddleOCR-json", "PaddleOCR-json.exe")}
	if got := findExecutable(paths, root); got != "" {
		t.Fatalf("archive is not a runnable engine: %s", got)
	}
	exe := filepath.Join(dir, "PaddleOCR-json.exe")
	if err := os.WriteFile(exe, []byte("path resolution fixture, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := findExecutable(paths, root); got != exe {
		t.Fatalf("portable engine path = %q, want %q", got, exe)
	}
}
