package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIncrementalConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "override.json")
	os.WriteFile(path, []byte(`{"timeout_sec":300,"ocr":{"min_score":0.8,"unknown":true}}`), 0600)
	c, w, err := Load(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.TimeoutSec != 300 || c.OCR.MinScore != .8 || c.OCR.Engine != "paddleocr_json" || c.Archive.MaxDepth != 3 {
		t.Fatalf("defaults were lost: %+v", c)
	}
	if len(w) != 1 || !strings.Contains(w[0], "ocr.unknown") {
		t.Fatalf("warnings: %v", w)
	}
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"timeout_sec":111}`), 0600)
	c, _, _ = Load(path, dir)
	if c.TimeoutSec != 300 {
		t.Fatal("explicit config stacked with implicit config")
	}
	c, _, _ = Load("", dir)
	if c.TimeoutSec != 111 {
		t.Fatal("implicit file was not loaded")
	}
}
func TestConfigErrors(t *testing.T) {
	for _, tc := range []struct{ json, key string }{{`{"timeout_sec":"x"}`, "timeout_sec"}, {`{"ocr":null}`, "ocr"}, {`{"ocr":{"min_score":2}}`, "ocr.min_score"}, {`{"timeout_sec":-1}`, "timeout_sec"}, {`{"concurrency":2}`, "concurrency"}, {`{"install_verify":{"judgment_profile":"fast"}}`, "judgment_profile"}, {`{"ocr":{"transport":"remote"}}`, "transport"}, {`{"install_verify":{"launch_check":true}}`, "launch_check"}, {`null`, "object"}, {`[]`, "config"}, {`{`, "config"}} {
		t.Run(tc.key+tc.json, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "c.json")
			os.WriteFile(path, []byte(tc.json), 0600)
			_, _, err := Load(path, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("expected %s: %v", tc.key, err)
			}
		})
	}
	if _, _, err := Load(filepath.Join(t.TempDir(), "absent"), t.TempDir()); err == nil {
		t.Fatal("missing explicit config accepted")
	}
}
func TestDigestAndUnlimitedTimeout(t *testing.T) {
	c := Defaults()
	c.TimeoutSec = 0
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	a := c.Digest()
	c.OCR.MinScore = .8
	if a == c.Digest() {
		t.Fatal("digest unchanged")
	}
	t.Setenv("AT_TEST_VALUE", "example")
	if ExpandEnv(`%AT_TEST_VALUE%/x`) != "example/x" {
		t.Fatal("environment not expanded")
	}
}
