//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"
)

func TestWindowsSettingsDraftTracking(t *testing.T) {
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
	f.setPage(pageSettings)
	if f.settingsChanged() {
		t.Fatal("fresh settings incorrectly dirty")
	}
	var remote, save rectT
	user32.NewProc("GetWindowRect").Call(f.ctrls[idRemoteTasks], uintptr(unsafe.Pointer(&remote)))
	user32.NewProc("GetWindowRect").Call(f.ctrls[idSettingsSave], uintptr(unsafe.Pointer(&save)))
	if remote.Bottom > save.Top {
		t.Fatal("SSH switch is not above Save")
	}
	var startup rectT
	user32.NewProc("GetWindowRect").Call(f.ctrls[idAutoStart], uintptr(unsafe.Pointer(&startup)))
	if startup.Top < remote.Bottom || startup.Bottom > save.Top {
		t.Fatal("administrator startup switch overlaps SSH or Save")
	}
	initialStartup := f.isChecked(idAutoStart)
	value := uintptr(bstChecked)
	if initialStartup {
		value = 0
	}
	send(f.ctrls[idAutoStart], bmSetCheck, value, 0)
	if !f.settingsChanged() {
		t.Fatal("startup change not detected")
	}
	f.discardSettingsDraft()
	if f.settingsChanged() || f.isChecked(idAutoStart) != initialStartup {
		t.Fatal("startup discard failed")
	}
	if shots := os.Getenv("AUTOTRIGGER_UI_SHOTS"); shots != "" {
		if err := os.MkdirAll(shots, 0755); err != nil {
			t.Fatal(err)
		}
		f.show()
		captureNativeWindow(t, f.hwnd, filepath.Join(shots, "settings-save-order.png"))
	}
	send(f.ctrls[idRemoteTasks], bmSetCheck, bstChecked, 0)
	if !f.settingsChanged() {
		t.Fatal("SSH change not detected")
	}
	if f.remoteEnabled {
		t.Fatal("unsaved SSH switch enabled receiver")
	}
	f.discardSettingsDraft()
	if f.settingsChanged() || f.isChecked(idRemoteTasks) {
		t.Fatal("discard did not restore checkbox")
	}
	setWindowTextRaw(f.ctrls[idOCRScore], "0.72")
	if !f.settingsChanged() {
		t.Fatal("recognition change not detected")
	}
	f.discardSettingsDraft()
	f.configOpen = true
	setWindowTextRaw(f.ctrls[idCfgEditor], f.settingsBaseline.Editor)
	if f.settingsChanged() {
		t.Fatal("opening JSON editor is not a change")
	}
	setWindowTextRaw(f.ctrls[idCfgEditor], "{invalid")
	if !f.settingsChanged() {
		t.Fatal("invalid JSON draft not detected")
	}
	f.discardSettingsDraft()
	if f.settingsChanged() {
		t.Fatal("discard JSON did not restore baseline")
	}
	setWindowTextRaw(f.ctrls[idTimeout], "321")
	if !f.saveSettings() {
		t.Fatal("saving settings failed")
	}
	if f.settingsChanged() {
		t.Fatal("saved settings still dirty")
	}
	f.setPage(pageHome)
	if f.page != pageHome {
		t.Fatal("clean settings blocked navigation")
	}
}
