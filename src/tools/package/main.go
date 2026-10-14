// package assembles the offline delivery and records material provenance and hashes.
package main

import (
	"archive/zip"
	"autotrigger/internal/config"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/output"
	"autotrigger/internal/resources"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type fileRecord struct {
	Path   string `json:"path"`
	Size   int64  `json:"size_bytes"`
	SHA256 string `json:"sha256"`
}
type material struct {
	Path          string `json:"path"`
	URL           string `json:"source_url"`
	ExpectedBytes int64  `json:"expected_bytes"`
	Present       bool   `json:"complete"`
	Source        string `json:"-"`
}

func materialSource(root, path string) string {
	current := filepath.Join(root, "dist", filepath.FromSlash(path))
	if _, err := os.Stat(current); err == nil {
		return current
	}
	return filepath.Join(root, "dist", filepath.FromSlash(strings.Replace(path, "third_parties/", "third_party/", 1)))
}

func main() {
	out := flag.String("out", "dist/AutoTrigger-0.1.0", "delivery directory")
	release := flag.Bool("release", false, "require all offline material and real templates")
	flag.Parse()
	if err := assemble(*out, *release); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func assemble(dest string, release bool) error {
	sourceRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	if filepath.Base(sourceRoot) != "src" {
		return fmt.Errorf("run packaging from the src directory")
	}
	root := filepath.Dir(sourceRoot)
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(root, dest)
	}
	absolute, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	runtimeDir := filepath.Join(root, "dist")
	if absolute == runtimeDir || !strings.HasPrefix(absolute, runtimeDir+string(filepath.Separator)) {
		return fmt.Errorf("delivery output must be a new directory inside dist")
	}
	materials := []material{{Path: "third_parties/7zip/7z2603-x64.exe", URL: "https://github.com/ip7z/7zip/releases/download/26.03/7z2603-x64.exe", ExpectedBytes: 0}, {Path: "third_parties/ocr/PaddleOCR-json_v1.4.1_windows_x64.7z", URL: "https://github.com/hiroi-sora/PaddleOCR-json/releases/download/v1.4.1/PaddleOCR-json_v1.4.1_windows_x64.7z", ExpectedBytes: 92736768}, {Path: "third_parties/ocr/RapidOCR-json_v0.2.0.7z", URL: "https://github.com/hiroi-sora/RapidOCR-json/releases/download/v0.2.0/RapidOCR-json_v0.2.0.7z", ExpectedBytes: 73461693}}
	missing := []string{}
	for i, m := range materials {
		materials[i].Source = materialSource(root, m.Path)
		s, e := os.Stat(materials[i].Source)
		materials[i].Present = e == nil && !s.IsDir() && (m.ExpectedBytes == 0 || s.Size() == m.ExpectedBytes)
		if !materials[i].Present {
			missing = append(missing, m.Path)
		}
	}
	library, err := templates.LoadAt(config.Defaults().GUI, filepath.Join(root, "dist"), filepath.Join(root, "dist", "templates"))
	if err != nil {
		return err
	}
	if release && (len(missing) > 0 || library.Masters < 50 || library.BuiltinCount < 180) {
		return fmt.Errorf("release blocked: missing materials=%v, real masters=%d, variants=%d", missing, library.Masters, library.BuiltinCount)
	}
	if _, err = os.Stat(dest); err == nil {
		return fmt.Errorf("delivery directory already exists; choose a new --out path to preserve earlier material")
	}
	if err = os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	for src, target := range map[string]string{"dist/AutoTrigger.exe": "AutoTrigger.exe", "README.md": "README.md", "src/pkg/schema/result.schema.json": "result.schema.json"} {
		if err = copyFile(filepath.Join(root, filepath.FromSlash(src)), filepath.Join(dest, target)); err != nil {
			return err
		}
	}
	for _, dir := range []string{"docs"} {
		if err = copyTree(filepath.Join(root, dir), filepath.Join(dest, dir)); err != nil {
			return err
		}
	}
	bundledRules, err := copyBundledTemplates(filepath.Join(sourceRoot, "delivery", "templates"), filepath.Join(dest, "templates"))
	if err != nil {
		return err
	}
	if err = resources.Export(filepath.Join(dest, "resources")); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(config.Defaults(), "", "  ")
	if err = output.Atomic(filepath.Join(dest, "config.example.json"), data); err != nil {
		return err
	}
	for _, m := range materials {
		if m.Present {
			if err = copyFile(m.Source, filepath.Join(dest, filepath.FromSlash(m.Path))); err != nil {
				return err
			}
		}
	}
	files := []fileRecord{}
	err = filepath.WalkDir(dest, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dest, path)
		files = append(files, fileRecord{Path: filepath.ToSlash(rel), Size: n, SHA256: hex.EncodeToString(h.Sum(nil))})
		return nil
	})
	if err != nil {
		return err
	}
	manifest := map[string]interface{}{"tool_version": "0.1.0", "schema_version": "2.8", "resource_pack_version": resources.Version, "built_at": time.Now(), "go_version_requirement": "1.20.x", "files": files, "materials": materials, "missing_materials": missing, "release_ready": false, "win7_runtime_verified": false, "builtin_template_variants": library.BuiltinCount, "real_master_count": library.Masters, "limitations": []string{"real installer template captures not available", "Win7 clean VM acceptance pending", "see docs/验收状态.md"}}
	manifest["bundled_user_template_rules"] = bundledRules
	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = output.Atomic(filepath.Join(dest, "manifest.json"), data); err != nil {
		return err
	}
	if err = zipTree(dest, dest+".zip"); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "delivery: %s.zip; complete offline materials=%t; release_ready=false\n", dest, len(missing) == 0)
	return nil
}
func copyFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}
func zipTree(root, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		method := uint16(zip.Deflate)
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".7z" || ext == ".zip" {
			method = zip.Store
		}
		out, err := z.CreateHeader(&zip.FileHeader{Name: filepath.ToSlash(rel), Method: method})
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(out, in)
		return err
	})
	closeErr := z.Close()
	fileErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return fileErr
}
