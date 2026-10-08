package layout

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/detect"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/resources"
	"os"
	"path/filepath"
	"testing"
)

func analyzer() Analyzer {
	return Analyzer{Detector: detect.Detector{}, Rules: resources.Pack{Mode: "builtin"}, Config: config.Defaults()}
}
func TestEntrancesAndMandatoryExclusion(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"setup_x64.exe", "setup_x86.exe", "install.bat", "unins000.exe", "repair.exe", "update.exe", "vcredist_x64.exe", "app.exe", "readme.txt"} {
		data := []byte("MZ")
		if name == "readme.txt" {
			data = []byte("运行 install.bat 开始安装")
		}
		os.WriteFile(filepath.Join(root, name), data, 0600)
	}
	entries, _ := Scan(root, true)
	c, l, err := analyzer().Analyze(root, entries, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Excluded) != 3 {
		t.Fatalf("excluded: %v", l.Excluded)
	}
	firstX64, firstX86 := -1, -1
	hint := false
	for i, e := range c {
		if e.Relative == "install.bat" {
			hint = e.Priority == 0
		}
		if e.Relative == "setup_x64.exe" {
			firstX64 = i
		}
		if e.Relative == "setup_x86.exe" {
			firstX86 = i
		}
	}
	if !hint || firstX64 >= firstX86 {
		t.Fatal("priority or architecture ordering failed", c)
	}
	a := analyzer()
	a.Config.PackageLayout.Mode = "brute"
	_, l, err = a.Analyze(root, entries, true)
	if err != nil || len(l.Excluded) != 3 {
		t.Fatal("brute mode disabled blacklist")
	}
}
func TestMediaClassification(t *testing.T) {
	for _, tc := range []struct {
		paths []string
		kind  string
	}{{[]string{"setup.exe", "sources/install.wim"}, "os_install_media"}, {[]string{"BDMV/STREAM/a.m2ts", "setup.exe"}, "media_disc"}, {[]string{"notice.lnk", "hidden.dat"}, "malware_lure"}, {[]string{"setup.exe", "assets/a", "assets/b", "assets/c"}, "installer_media"}, {[]string{"a.txt"}, "data_disc"}} {
		entries := []ports.ArchiveEntry{}
		for _, p := range tc.paths {
			entries = append(entries, ports.ArchiveEntry{Path: p})
		}
		kind, err := analyzer().MediaKind(entries)
		if err != nil || kind != tc.kind {
			t.Fatalf("%v => %s %v", tc.paths, kind, err)
		}
	}
}
func TestAutorunDoesNotExecuteShellPayload(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "autorun.inf"), []byte("[autorun]\nopen=setup.exe & uninstall.exe\n"), 0600)
	os.WriteFile(filepath.Join(root, "setup.exe"), []byte("MZ"), 0600)
	os.WriteFile(filepath.Join(root, "uninstall.exe"), []byte("MZ"), 0600)
	entries, _ := Scan(root, true)
	got, _, err := analyzer().Analyze(root, entries, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.Relative == "setup.exe" && c.Priority != 0 {
			t.Fatal("autorun entrance ignored")
		}
		if len(c.Args) != 0 {
			t.Fatal("autorun shell text became executable arguments")
		}
	}
}
