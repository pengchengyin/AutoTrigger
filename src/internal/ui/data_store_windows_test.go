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
	f.legacyPrompted = true
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
	for _, id := range []int{idConfig, idBrowseConfig} {
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
