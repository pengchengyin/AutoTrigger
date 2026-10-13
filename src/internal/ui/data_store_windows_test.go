//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/datadir"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWindowsUnifiedDataSettingsAndTemplateControls(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := newForm(cli.Dependencies{})
	f.exeDir = t.TempDir()
	f.dataDir = t.TempDir()
	f.dataError = nil
	f.historyDir = f.dataLayout().History()
	f.history = nil
	f.opts.Config = ""
	f.opts.Output = filepath.Join(f.dataLayout().Results(), DefaultResultName)
	if err := f.create(); err != nil {
		t.Fatal(err)
	}
	defer procDestroyWindow.Call(f.hwnd)
	f.show()
	f.setPage(pageSettings)
	if f.ctrls[5901] != 0 {
		t.Fatal("directory convention hint still created")
	}
	for _, id := range []int{idConfig, idBrowseConfig, idStatus} {
		if visible, _, _ := user32.NewProc("IsWindowVisible").Call(f.ctrls[id]); visible != 0 {
			t.Fatal("separate config path still visible")
		}
	}
	if windowText(f.ctrls[idOutputDir]) != f.dataDir || windowText(f.ctrls[idLblOutput]) != "数据目录" {
		t.Fatal("data directory not displayed")
	}
	if !f.saveSettings() {
		t.Fatal(windowText(f.ctrls[idStatus]))
	}
	if !fileExists(f.dataLayout().Config()) || fileExists(filepath.Join(f.exeDir, "config.json")) {
		t.Fatal("config not contained in data directory")
	}
	if err := datadir.Save(f.exeDir, f.dataDir); err != nil {
		t.Fatal(err)
	}
	layout, err := datadir.Load(f.exeDir)
	if err != nil || layout.Root != f.dataDir {
		t.Fatal(layout, err)
	}
	f.setPage(pageTemplates)
	f.performHeaderAction("new-template")
	for id, text := range map[int]string{5400: "模板名称（必填）", 5401: "模板描述（选填，例如：适用软件、窗口或用途）"} {
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(f.ctrls[id])
		if visible == 0 || windowText(f.ctrls[id]) != text {
			t.Fatal("template form label missing", id)
		}
	}
	setWindowTextRaw(f.ctrls[hubName], "Native data layout test")
	f.onCommand(hubSave, bnClicked)
	projects, err := listTemplateProjects(f.dataLayout().Templates())
	if err != nil || len(projects) != 1 {
		t.Fatal(projects, err)
	}
	if _, err = os.Stat(projectRoot(f.exeDir)); !os.IsNotExist(err) {
		t.Fatal("project stored beside EXE instead of templates")
	}
	actions := headerActions(pageTemplates)
	if len(actions) != 3 {
		t.Fatal("missing transfer commands")
	}
	if got := f.taskLogPath(filepath.Join(f.dataLayout().Results(), "result.json")); got != filepath.Join(f.dataDir, "logs", "result.json.log") {
		t.Fatal(got)
	}
}

func TestWindowsStartupDoesNotPromptOrImportLegacyData(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := newForm(cli.Dependencies{})
	f.exeDir = t.TempDir()
	f.dataDir = f.exeDir
	f.dataError = nil
	f.historyDir = f.dataLayout().History()
	f.history = nil
	f.opts.Config = ""
	f.opts.Output = filepath.Join(f.dataLayout().Results(), DefaultResultName)
	legacy := filepath.Join(f.exeDir, "config.json")
	if err := os.WriteFile(legacy, []byte(`{"timeout_sec":123}`), 0600); err != nil {
		t.Fatal(err)
	}
	p, _ := hubFixture(t, f.exeDir, "旧模板")
	if err := os.MkdirAll(filepath.Join(f.exeDir, "third_party"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := f.create(); err != nil {
		t.Fatal(err)
	}
	defer procDestroyWindow.Call(f.hwnd)
	f.show()
	if fileExists(f.dataLayout().Config()) {
		t.Fatal("startup silently migrated old config")
	}
	projects, err := listTemplateProjects(f.templateRoot())
	if err != nil || len(projects) != 0 {
		t.Fatal("startup migrated old templates", projects, err)
	}
	if !fileExists(filepath.Join(projectDir(f.exeDir, p), "project.json")) || !fileExists(legacy) {
		t.Fatal("startup removed legacy data")
	}
}
