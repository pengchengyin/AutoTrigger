//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/internal/gui"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/platform/windows"
	"autotrigger/internal/resources"
	"context"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

func TestWindowsTemplateHub(t *testing.T) {
	shots := os.Getenv("AUTOTRIGGER_UI_SHOTS")
	if shots == "" {
		t.Skip("set AUTOTRIGGER_UI_SHOTS")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()
	f := newForm(cli.Dependencies{})
	f.exeDir = t.TempDir()
	f.dataDir = ""
	f.historyDir = filepath.Join(f.exeDir, "history")
	f.history = nil
	if err := f.create(); err != nil {
		t.Fatal(err)
	}
	defer procDestroyWindow.Call(f.hwnd)
	f.show()
	f.setPage(pageTemplates)
	for _, id := range []int{idTemplateAll, idTemplateBuiltin, idTemplateExternal, idTemplateBody} {
		if visible, _, _ := user32.NewProc("IsWindowVisible").Call(f.ctrls[id]); visible != 0 {
			t.Fatal("old library controls still visible")
		}
	}
	f.performHeaderAction("new-template")
	setWindowTextRaw(f.ctrls[hubName], "真实按钮模板")
	setWindowTextRaw(f.ctrls[hubDescription], "Win32 窗口连续捕获回归")
	send(f.hwnd, wmCommand, uintptr(hubSave), 0)
	if f.hubSelected == "" || f.hubEditing {
		t.Fatal("inline project creation failed")
	}
	projectID := f.hubSelected
	for _, name := range []string{"协议按钮模板", "安装页模板", "完成页模板", "备用模板"} {
		f.performHeaderAction("new-template")
		setWindowTextRaw(f.ctrls[hubName], name)
		setWindowTextRaw(f.ctrls[hubDescription], "待采集的独立草稿")
		send(f.hwnd, wmCommand, uintptr(hubSave), 0)
	}
	f.openHubProject(projectID)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(exe, "--capture-fixture")
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	var target uintptr
	pumpCaptureUntil(t, func() bool {
		ws, _ := (windows.GUI{}).Windows([]int{child.Process.Pid})
		if len(ws) == 1 {
			target = ws[0].Handle
		}
		return target != 0
	})
	button, _, _ := user32.NewProc("GetDlgItem").Call(target, 100)
	send(button, 0xc, 0, uintptr(unsafe.Pointer(utf16Ptr("自定义推进"))))
	user32.NewProc("SetWindowPos").Call(target, ^uintptr(0), 0, 0, 0, 0, 3)
	send(f.hwnd, wmCommand, uintptr(hubCapture), 0)
	s := activeHover
	if s == nil {
		t.Fatal("continuous capture workspace did not open")
	}
	defer func() {
		if hoverPanes[s.hwnd] != nil {
			s.close(true)
			pumpCaptureUntil(t, func() bool { return hoverPanes[s.hwnd] == nil })
		}
	}()
	user32.NewProc("SetForegroundWindow").Call(target)
	var rect rectT
	user32.NewProc("GetWindowRect").Call(button, uintptr(unsafe.Pointer(&rect)))
	point := image.Pt(int(rect.Left+rect.Right)/2, int(rect.Top+rect.Bottom)/2)
	user32.NewProc("SetCursorPos").Call(uintptr(point.X), uintptr(point.Y))
	pumpCaptureUntil(t, func() bool {
		user32.NewProc("SetCursorPos").Call(uintptr(point.X), uintptr(point.Y))
		return s.target.Window.PID == child.Process.Pid && s.target.Control != nil && s.target.Control.Text == "自定义推进" && !s.picking
	})
	s.choose(point)
	pumpCaptureUntil(t, func() bool { return s.count == 1 && !s.busy })
	if !strings.HasSuffix(windowText(target), "clicks=0") {
		t.Fatal("capture activated fixture")
	}
	s.close(false)
	pumpCaptureUntil(t, func() bool { return hoverPanes[s.hwnd] == nil })
	if len(f.hubCaptures) != 1 {
		t.Fatal("captured item missing from inline detail")
	}
	if visible, _, _ := user32.NewProc("IsWindowVisible").Call(f.templateMaker); visible != 0 {
		t.Fatal("legacy maker unexpectedly shown")
	}
	send(f.ctrls[hubImages], 0x186, 0, 0)
	f.hubCommand(hubImages, 1)
	f.setComboSel(hubAction, 1)
	send(f.hwnd, wmCommand, uintptr(hubMark), 0)
	os.MkdirAll(shots, 0755)
	captureNativeWindow(t, f.hwnd, filepath.Join(shots, "template-detail.png"))
	send(f.hwnd, wmCommand, uintptr(hubBack), 0)
	send(f.ctrls[5200], 0xf5, 0, 0)
	if !f.hubEnabled[projectID] {
		t.Fatal("checkbox did not enable template")
	}
	captureNativeWindow(t, f.hwnd, filepath.Join(shots, "template-cards.png"))
	lib, err := templates.Load(config.Defaults().GUI, f.exeDir)
	if err != nil || len(lib.User.Rules) != 1 {
		t.Fatal(err)
	}
	a := gui.Automator{Native: windows.GUI{}, Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}, Templates: lib}
	if err = a.Init(); err != nil {
		t.Fatal(err)
	}
	user32.NewProc("SetForegroundWindow").Call(target)
	pumpCaptureUntil(t, func() bool { return hookInput.Load() == nil })
	var targetRect rectT
	user32.NewProc("GetWindowRect").Call(target, uintptr(unsafe.Pointer(&targetRect)))
	user32.NewProc("SetCursorPos").Call(uintptr(targetRect.Left+200), uintptr(targetRect.Top+15))
	user32.NewProc("mouse_event").Call(2, 0, 0, 0, 0)
	user32.NewProc("mouse_event").Call(4, 0, 0, 0, 0)
	pumpCaptureUntil(t, func() bool {
		foreground, _, _ := user32.NewProc("GetForegroundWindow").Call()
		return foreground == target
	})
	u, err := a.Step(context.Background(), []int{child.Process.Pid})
	if err != nil || len(u.Actions) != 1 {
		t.Fatal("selected template did not drive native fixture", err, u)
	}
	pumpCaptureUntil(t, func() bool { return strings.HasSuffix(windowText(target), "clicks=1") })
	f.openHubProject(projectID)
	send(f.ctrls[hubImages], 0x186, 0, 0)
	send(f.hwnd, wmCommand, uintptr(hubDelete), 0)
	if len(f.hubCaptures) != 0 || f.hubEnabled[projectID] {
		t.Fatal("delete did not update count/selection")
	}
	if f.templateMaker != 0 {
		procDestroyWindow.Call(f.templateMaker)
	}
}
