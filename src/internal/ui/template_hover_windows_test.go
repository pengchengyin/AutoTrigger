//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/windows"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// The test executable hosts a separate real Win32 process, not a simulated UIA tree.
func init() {
	if windows.InternalHelper() {
		os.Exit(0)
	}
	if len(os.Args) == 2 && os.Args[1] == "--capture-fixture" {
		runCaptureFixture()
		os.Exit(0)
	}
}

// Opt-in only: opens the existing installer, captures its first page and never
// invokes Next/Install/OK/Cancel. The job owns and cleans up only this process tree.
func TestWindowsHoverCaptureInstaller(t *testing.T) {
	installer := os.Getenv("AUTOTRIGGER_CAPTURE_INSTALLER")
	shots := os.Getenv("AUTOTRIGGER_UI_SHOTS")
	if installer == "" || shots == "" {
		t.Skip("set installer and screenshot directory for read-only installer validation")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()
	process, err := (windows.Runner{}).Start(context.Background(), ports.Command{Program: installer, Args: []string{"/CURRENTUSER"}, NonInstall: true})
	if err != nil {
		t.Fatalf("installer could not be opened without elevation: %v", err)
	}
	defer process.Close()
	defer process.KillTree()
	var target ports.Window
	pumpCaptureUntil(t, func() bool {
		activity, _ := process.Activity()
		pids := append(activity.PIDs, process.PID())
		ws, _ := (windows.GUI{}).Windows(pids)
		if len(ws) > 0 {
			target = ws[len(ws)-1]
		}
		return target.Handle != 0
	})
	controls, err := (windows.GUI{}).Controls(target)
	if err != nil {
		t.Fatal(err)
	}
	if permission := windows.CapturePermission(target.PID); permission != nil {
		t.Skipf("真实安装器捕获未验证：%v", permission)
	}
	var buttons []ports.Control
	for _, c := range controls {
		if c.Role == "button" && c.Enabled && c.Bounds.Width() > 0 && c.Bounds.Height() > 0 {
			duplicate := false
			for _, old := range buttons {
				if old.Bounds == c.Bounds {
					duplicate = true
				}
			}
			if duplicate {
				continue
			}
			buttons = append(buttons, c)
		}
	}
	if len(buttons) < 2 {
		t.Fatalf("first installer page has fewer than two buttons: %+v", controls)
	}
	f := newForm(cli.Dependencies{})
	f.exeDir = t.TempDir()
	f.historyDir = filepath.Join(f.exeDir, "history")
	f.history = nil
	f.opts = NewOptions(f.exeDir)
	f.templates = loadTemplateInfo(f.exeDir)
	f.className = utf16Ptr("InstallerCaptureHost" + strconv.FormatInt(time.Now().UnixNano(), 10))
	if err = f.create(); err != nil {
		t.Fatal(err)
	}
	defer procDestroyWindow.Call(f.hwnd)
	f.show()
	f.showTemplateMaker()
	p := makerPanes[f.templateMaker]
	defer procDestroyWindow.Call(p.hwnd)
	p.startHoverCapture()
	s := activeHover
	if s == nil {
		t.Fatal(p.value(makerStatus))
	}
	defer func() {
		if hoverPanes[s.hwnd] != nil {
			s.close(true)
			pumpCaptureUntil(t, func() bool { return hoverPanes[s.hwnd] == nil })
		}
	}()
	user32.NewProc("SetForegroundWindow").Call(target.Handle)
	// Test-only visibility: do not move/resize the installer, and restore its
	// z-order afterwards. The production picker never repositions its targets.
	ok, _, _ := user32.NewProc("SetWindowPos").Call(target.Handle, ^uintptr(0), 0, 0, 0, 0, 0x13)
	if ok == 0 {
		t.Fatal("installer cannot be made visible for read-only validation")
	}
	defer user32.NewProc("SetWindowPos").Call(target.Handle, ^uintptr(1), 0, 0, 0, 0, 0x13)
	if err = os.MkdirAll(shots, 0755); err != nil {
		t.Fatal(err)
	}
	captureInstallerShot(t, target, filepath.Join(shots, "installer-before.png"))
	for i, c := range buttons[:2] {
		if s.manual {
			send(s.controls[captureManual], 0xf5, 0, 0)
		}
		point := image.Pt((c.Bounds.Left+c.Bounds.Right)/2, (c.Bounds.Top+c.Bounds.Bottom)/2)
		if s.toolbarAt(point) {
			t.Fatal("installer target overlaps capture toolbar")
		}
		user32.NewProc("SetCursorPos").Call(uintptr(point.X), uintptr(point.Y))
		pumpCaptureUntil(t, func() bool {
			return s.target.Window.Handle == target.Handle && !s.picking
		})
		method := "uia"
		if s.target.Control == nil || !s.target.Control.Bounds.Contains(point.X, point.Y) {
			method = "image"
			send(s.controls[captureManual], 0xf5, 0, 0)
			user32.NewProc("SetCursorPos").Call(uintptr(c.Bounds.Left), uintptr(c.Bounds.Top))
			user32.NewProc("mouse_event").Call(2, 0, 0, 0, 0)
			pumpCaptureUntil(t, func() bool { return s.dragging })
			user32.NewProc("SetCursorPos").Call(uintptr(c.Bounds.Right), uintptr(c.Bounds.Bottom))
			user32.NewProc("mouse_event").Call(4, 0, 0, 0, 0)
		} else {
			user32.NewProc("mouse_event").Call(2, 0, 0, 0, 0)
			user32.NewProc("mouse_event").Call(4, 0, 0, 0, 0)
		}
		pumpCaptureUntil(t, func() bool { return s.count == i+1 && !s.busy })
		t.Logf("captured %s as %s, bounds=%+v DPI=%d", c.Text, method, c.Bounds, target.DPI)
	}
	after, err := (windows.GUI{}).Controls(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, before := range buttons[:2] {
		found := false
		for _, c := range after {
			if c.Text == before.Text && c.Bounds == before.Bounds && c.Enabled == before.Enabled {
				found = true
			}
		}
		if !found {
			t.Fatalf("installer changed after capture: %s", before.Text)
		}
	}
	captureInstallerShot(t, target, filepath.Join(shots, "installer-after.png"))
	captureNativeWindow(t, s.hwnd, filepath.Join(shots, "installer-captures.png"))
	s.close(false)
	pumpCaptureUntil(t, func() bool { return hoverPanes[s.hwnd] == nil })
	index, err := readCaptureIndex(projectDir(f.exeDir, p.project), p.project.ID)
	if err != nil || len(index.Items) != 2 {
		t.Fatal("installer captures not persisted")
	}
	for _, item := range index.Items {
		if _, err = captureImage(projectDir(f.exeDir, p.project), item); err != nil {
			t.Fatal(err)
		}
	}
}

func captureInstallerShot(t *testing.T, w ports.Window, path string) {
	t.Helper()
	shot, err := (windows.GUI{}).RawScreenshot(ports.Window{Bounds: w.Bounds})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, shot)
	closeErr := f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	t.Log(path)
}

func runCaptureFixture() {
	runtime.LockOSThread()
	procSetProcessDPIAware.Call()
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	name := utf16Ptr("AutoTriggerCaptureFixture" + strconv.Itoa(os.Getpid()))
	clicks := 0
	cb := syscall.NewCallback(func(h uintptr, msg uint32, w uintptr, l unsafe.Pointer) uintptr {
		if msg == 0x111 && int(w>>16) == 0 {
			clicks++
			setWindowTextRaw(h, "AutoTriggerCaptureFixture clicks="+strconv.Itoa(clicks))
			return 0
		}
		if msg == 2 {
			procPostQuitMessage.Call(0)
			return 0
		}
		r, _, _ := procDefWindowProcW.Call(h, uintptr(msg), w, uintptr(l))
		return r
	})
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: cb, Instance: instance, Cursor: cursor, Background: colorWindow + 1, ClassName: name}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	h, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(utf16Ptr("AutoTriggerCaptureFixture clicks=0"))), wsOverlappedWindow, 700, 300, 500, 320, 0, 0, instance, 0)
	for i, v := range []struct{ class, text string }{{"BUTTON", "下一步"}, {"EDIT", "安装路径"}, {"BUTTON", "安装"}} {
		procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(v.class))), uintptr(unsafe.Pointer(utf16Ptr(v.text))), wsChild|wsVisible|wsTabStop|wsBorder, 40, uintptr(30+i*56), 200, 32, h, uintptr(100+i), instance, 0)
	}
	procShowWindow.Call(h, swShow)
	procUpdateWindow.Call(h)
	var msg msgT
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func pumpCaptureUntil(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		var msg msgT
		for {
			available, _, _ := user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
			if available == 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, s := range hoverPanes {
		t.Logf("capture: count=%d busy=%v target=%+v error=%s status=%s", s.count, s.busy, s.target, s.lastError, windowText(s.controls[captureStatus]))
	}
	t.Fatal("native capture operation timed out")
}

func TestWindowsHoverCapture(t *testing.T) {
	shots := os.Getenv("AUTOTRIGGER_UI_SHOTS")
	if shots == "" {
		t.Skip("set AUTOTRIGGER_UI_SHOTS for native UIA and hook regression")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(exe, "--capture-fixture")
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	var targetH uintptr
	pumpCaptureUntil(t, func() bool {
		ws, _ := (windows.GUI{}).Windows([]int{child.Process.Pid})
		if len(ws) == 1 {
			targetH = ws[0].Handle
		}
		return targetH != 0
	})
	button, _, _ := user32.NewProc("GetDlgItem").Call(targetH, 100)
	edit, _, _ := user32.NewProc("GetDlgItem").Call(targetH, 101)
	center := func(h uintptr) image.Point {
		var r rectT
		user32.NewProc("GetWindowRect").Call(h, uintptr(unsafe.Pointer(&r)))
		return image.Pt(int(r.Left+r.Right)/2, int(r.Top+r.Bottom)/2)
	}
	f := newForm(cli.Dependencies{})
	f.exeDir = t.TempDir()
	f.historyDir = filepath.Join(f.exeDir, "history")
	f.history = nil
	f.opts = NewOptions(f.exeDir)
	f.templates = loadTemplateInfo(f.exeDir)
	f.className = utf16Ptr("CaptureHost" + strconv.FormatInt(time.Now().UnixNano(), 10))
	if err = f.create(); err != nil {
		t.Fatal(err)
	}
	defer procDestroyWindow.Call(f.hwnd)
	f.show()
	f.showTemplateMaker()
	p := makerPanes[f.templateMaker]
	if p == nil {
		t.Fatal("maker not created")
	}
	defer procDestroyWindow.Call(p.hwnd)
	f.mu.Lock()
	f.running = true
	f.mu.Unlock()
	send(p.controls[makerCapture], 0xf5, 0, 0)
	if activeHover != nil || p.capturing {
		t.Fatal("capture interfered with running task")
	}
	f.mu.Lock()
	f.running = false
	f.mu.Unlock()
	send(p.controls[makerCapture], 0xf5, 0, 0)
	s := activeHover
	if s == nil {
		t.Fatal("capture failed: " + p.value(makerStatus))
	}
	defer func() {
		if hoverPanes[s.hwnd] != nil {
			s.close(true)
			pumpCaptureUntil(t, func() bool { return hoverPanes[s.hwnd] == nil })
		}
	}()
	if err = os.MkdirAll(shots, 0755); err != nil {
		t.Fatal(err)
	}
	user32.NewProc("SetForegroundWindow").Call(targetH)
	move := func(h uintptr) {
		v := center(h)
		user32.NewProc("SetCursorPos").Call(uintptr(v.X), uintptr(v.Y))
		pumpCaptureUntil(t, func() bool {
			return s.target.Control != nil && s.target.Control.Bounds.Contains(v.X, v.Y) && !s.picking
		})
	}
	move(button)
	if s.target.Control.Role != "button" || s.target.Control.Source != "uia" {
		t.Fatalf("not real UIA: %+v", s.target)
	}
	tool, e := filepath.Abs(filepath.Join("..", "..", "dist", "AutoTrigger.exe"))
	if e == nil {
		if _, e = os.Stat(tool); e == nil {
			point := center(button)
			done := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
				defer cancel()
				data, err := exec.CommandContext(ctx, tool, "--internal-uia-point", strconv.Itoa(point.X)+","+strconv.Itoa(point.Y)).Output()
				var picked windows.PickTarget
				if err == nil {
					err = json.Unmarshal(data, &picked)
				}
				if err == nil && (picked.Control == nil || picked.Control.Role != "button" || picked.Window.Handle != targetH) {
					err = fmt.Errorf("shipped helper returned invalid target: %+v", picked)
				}
				done <- err
			}()
			finished := false
			var helperError error
			pumpCaptureUntil(t, func() bool {
				select {
				case helperError = <-done:
					finished = true
				default:
				}
				return finished
			})
			if helperError != nil {
				t.Fatal(helperError)
			}
		}
	}
	frame := s.frame
	// Repeated refresh while the cursor is stationary must preserve the same region.
	for i := 0; i < 3; i++ {
		s.lastRequest = time.Time{}
		s.tick()
		pumpCaptureUntil(t, func() bool { return !s.picking })
		if s.frame != frame {
			t.Fatal("highlight changed stable target")
		}
		point := center(button)
		under, e := windows.WindowAtPoint(point.X, point.Y)
		if e != nil || under.Handle != targetH {
			t.Fatal("highlight intercepted point detection")
		}
	}
	captureNativeWindow(t, targetH, filepath.Join(shots, "hover-target-before.png"))
	captureNativeWindow(t, s.hwnd, filepath.Join(shots, "hover-capture-toolbar.png"))
	click := func() {
		user32.NewProc("mouse_event").Call(2, 0, 0, 0, 0)
		user32.NewProc("mouse_event").Call(4, 0, 0, 0, 0)
	}
	click()
	pumpCaptureUntil(t, func() bool { return s.count == 1 && !s.busy })
	// Rapid consecutive clicks must be queued, not discarded while writing.
	click()
	click()
	pumpCaptureUntil(t, func() bool { return s.count == 3 && !s.busy && len(s.queue) == 0 })
	for want := 2; want >= 1; want-- {
		send(s.controls[captureUndo], 0xf5, 0, 0)
		expected := want
		pumpCaptureUntil(t, func() bool { return s.count == expected && !s.busy })
	}
	move(edit)
	click()
	pumpCaptureUntil(t, func() bool { return s.count == 2 && !s.busy })
	if strings.Contains(windowText(targetH), "clicks=1") {
		t.Fatal("capture click triggered fixture button")
	}
	if int(send(s.controls[captureList], 0x18b, 0, 0)) != 2 || len(p.captures) != 2 {
		t.Fatal("captures not immediately visible")
	}
	send(s.controls[captureUndo], 0xf5, 0, 0)
	pumpCaptureUntil(t, func() bool { return s.count == 1 && !s.busy })
	// Move the target: the old geometry must not survive a stationary cursor.
	procMoveWindow.Call(targetH, 680, 280, 500, 320, 1)
	move(button)
	if s.frame == frame {
		t.Fatal("moved window kept stale coordinates")
	}
	// UIA does not describe the blank client area as a precise control.
	user32.NewProc("SetCursorPos").Call(1090, 530)
	pumpCaptureUntil(t, func() bool { return !s.picking && s.target.Control == nil })
	send(s.controls[captureManual], 0xf5, 0, 0)
	user32.NewProc("SetCursorPos").Call(1030, 510)
	user32.NewProc("mouse_event").Call(2, 0, 0, 0, 0)
	pumpCaptureUntil(t, func() bool { return s.dragging })
	user32.NewProc("SetCursorPos").Call(1130, 540)
	user32.NewProc("mouse_event").Call(4, 0, 0, 0, 0)
	pumpCaptureUntil(t, func() bool { return s.count == 2 && !s.busy })
	if p.captures[1].Rule.Method != "image" {
		t.Fatal("manual fallback mislabeled UIA")
	}
	captureNativeWindow(t, s.hwnd, filepath.Join(shots, "hover-two-captures.png"))
	send(s.controls[captureFinish], 0xf5, 0, 0)
	pumpCaptureUntil(t, func() bool { return hoverPanes[s.hwnd] == nil })
	if activeHover != nil || p.capturing || s.mouseHook != 0 || s.keyHook != 0 {
		t.Fatal("hooks not released")
	}
	index, err := readCaptureIndex(projectDir(f.exeDir, p.project), p.project.ID)
	if err != nil || len(index.Items) != 2 || !index.Items[0].Confirmed || !index.Items[1].Confirmed {
		t.Fatal("finish did not persist captures")
	}
	p.previewCaptureRecord()
	captureNativeWindow(t, p.hwnd, filepath.Join(shots, "hover-draft-after.png"))
	// Cancel a new session without deleting the confirmed session.
	p.startHoverCapture()
	s = activeHover
	if s == nil {
		t.Fatal("second session failed")
	}
	user32.NewProc("SetForegroundWindow").Call(targetH)
	move(button)
	click()
	pumpCaptureUntil(t, func() bool { return s.count == 1 && !s.busy })
	send(s.controls[captureCancel], 0xf5, 0, 0)
	pumpCaptureUntil(t, func() bool { return hoverPanes[s.hwnd] == nil })
	index, err = readCaptureIndex(projectDir(f.exeDir, p.project), p.project.ID)
	if err != nil || len(index.Items) != 2 {
		t.Fatal("cancel removed prior session")
	}
	// Escape works without giving focus to the toolbar.
	p.startHoverCapture()
	s = activeHover
	if s == nil {
		t.Fatal("escape session failed")
	}
	user32.NewProc("keybd_event").Call(27, 0, 0, 0)
	user32.NewProc("keybd_event").Call(27, 0, 2, 0)
	pumpCaptureUntil(t, func() bool { return hoverPanes[s.hwnd] == nil })
	if windowText(targetH) != "AutoTriggerCaptureFixture clicks=0" {
		t.Fatal("fixture received capture actions: " + windowText(targetH))
	}
	// Exiting mid-drag must still swallow the matching release, not leak it.
	p.startHoverCapture()
	s = activeHover
	if s == nil {
		t.Fatal("mid-drag session failed")
	}
	user32.NewProc("SetForegroundWindow").Call(targetH)
	move(button)
	send(s.controls[captureManual], 0xf5, 0, 0)
	user32.NewProc("mouse_event").Call(2, 0, 0, 0, 0)
	pumpCaptureUntil(t, func() bool { return s.dragging })
	user32.NewProc("keybd_event").Call(27, 0, 0, 0)
	user32.NewProc("keybd_event").Call(27, 0, 2, 0)
	pumpCaptureUntil(t, func() bool { return hoverPanes[s.hwnd] == nil })
	if hookInput.Load() == nil {
		t.Fatal("hook stopped before swallowing outstanding release")
	}
	user32.NewProc("mouse_event").Call(4, 0, 0, 0, 0)
	pumpCaptureUntil(t, func() bool { return hookInput.Load() == nil })
	if windowText(targetH) != "AutoTriggerCaptureFixture clicks=0" {
		t.Fatal("release leaked to fixture")
	}
	if _, err = (windows.GUI{}).RawScreenshot(ports.Window{}); err == nil {
		t.Fatal("invalid screenshot dimensions accepted")
	}
	// The isolated helper reports invalid/unavailable targets rather than making up a rectangle.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = (windows.GUI{}).PickAtPoint(ctx, -99999, -99999); err == nil {
		t.Fatal("cancelled UIA request succeeded")
	}
}
