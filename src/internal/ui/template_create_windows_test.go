//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/windows"
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsTemplateMaker(t *testing.T) {
	dir := os.Getenv("AUTOTRIGGER_UI_SHOTS")
	if dir == "" {
		t.Skip("set AUTOTRIGGER_UI_SHOTS for native template creation checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()
	f := newForm(cli.Dependencies{})
	f.className = utf16Ptr("AutoTriggerTemplateMakerRegression" + strconv.FormatInt(time.Now().UnixNano(), 10))
	f.exeDir = t.TempDir()
	f.historyDir = filepath.Join(f.exeDir, "history")
	f.history = nil
	f.opts = NewOptions(f.exeDir)
	f.templates = loadTemplateInfo(f.exeDir)
	if err := f.create(); err != nil {
		t.Fatal(err)
	}
	defer procDestroyWindow.Call(f.hwnd)
	f.show()
	f.setPage(pageTemplates)
	procUpdateWindow.Call(f.hwnd)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	captureNativeWindow(t, f.hwnd, filepath.Join(dir, "templates-create-entry.png"))
	var wr, br rectT
	user32.NewProc("GetWindowRect").Call(f.hwnd, uintptr(unsafe.Pointer(&wr)))
	user32.NewProc("GetWindowRect").Call(f.ctrls[idTemplateCreate], uintptr(unsafe.Pointer(&br)))
	shot, err := (windows.GUI{}).RawScreenshot(ports.Window{Handle: f.hwnd, Bounds: ports.Rect{Left: int(wr.Left), Top: int(wr.Top), Right: int(wr.Right), Bottom: int(wr.Bottom)}})
	if err != nil {
		t.Fatal(err)
	}
	// Also exercise the shipped screenshot worker, without starting an installer.
	tool, err := filepath.Abs(filepath.Join("..", "..", "dist", "AutoTrigger.exe"))
	if err == nil {
		if _, err = os.Stat(tool); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			type workerResult struct {
				data []byte
				err  error
			}
			done := make(chan workerResult, 1)
			go func() {
				data, err := exec.CommandContext(ctx, tool, "--internal-screenshot", strconv.FormatUint(uint64(f.hwnd), 10)).Output()
				done <- workerResult{data, err}
			}()
			var result workerResult
			waiting := true
			for waiting {
				var m msgT
				for {
					available, _, _ := user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
					if available == 0 {
						break
					}
					procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
					procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
				}
				select {
				case result = <-done:
					waiting = false
				default:
					time.Sleep(5 * time.Millisecond)
				}
			}
			if result.err != nil {
				t.Fatalf("screenshot worker: %v", result.err)
			}
			img, err := png.Decode(bytes.NewReader(result.data))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Size() != shot.Bounds().Size() {
				t.Fatal("worker screenshot dimensions differ")
			}
			shot = img
		}
	}
	send(f.ctrls[idTemplateCreate], 0xF5, 0, 0)
	p := makerPanes[f.templateMaker]
	if p == nil {
		t.Fatal("creation button did not open native window")
	}
	defer procDestroyWindow.Call(p.hwnd)
	if enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(p.controls[makerSave]); enabled != 0 {
		t.Fatal("save enabled without screenshot")
	}
	p.setImage(shot, "原生窗口回归截图（不是安装器素材）")
	bounds := image.Rect(int(br.Left-wr.Left), int(br.Top-wr.Top), int(br.Right-wr.Left), int(br.Bottom-wr.Top))
	// Exercise drag mapping before correcting to the exact control pixel bounds.
	d := p.display
	size := shot.Bounds().Size()
	point := func(v image.Point) uintptr {
		x := d.Min.X + v.X*d.Dx()/size.X
		y := d.Min.Y + v.Y*d.Dy()/size.Y
		return uintptr(uint32(x)&0xffff | uint32(y)<<16)
	}
	send(p.hwnd, wmLButtonDown, 1, point(bounds.Min))
	send(p.hwnd, wmMouseMove, 1, point(bounds.Max))
	send(p.hwnd, 0x202, 0, point(bounds.Max))
	if p.bounds.Empty() || p.dragging {
		t.Fatal("drag selection failed")
	}
	for id, v := range map[int]int{makerX: bounds.Min.X, makerY: bounds.Min.Y, makerWidth: bounds.Dx(), makerHeight: bounds.Dy()} {
		setWindowTextRaw(p.controls[id], strconv.Itoa(v))
	}
	if p.bounds != bounds {
		t.Fatalf("precision fields not applied: %v / %v", p.bounds, bounds)
	}
	setWindowTextRaw(p.controls[makerID], "native-regression-only")
	send(p.controls[makerAction], 0x14e, 1, 0)
	send(p.hwnd, 0x111, uintptr(makerAction|cbnSelChange<<16), p.controls[makerAction])
	if p.value(makerID) != "native-regression-only" {
		t.Fatal("manual ID overwritten")
	}
	procUpdateWindow.Call(p.hwnd)
	captureNativeWindow(t, p.hwnd, filepath.Join(dir, "template-maker-selection.png"))
	procMoveWindow.Call(p.hwnd, 10, 10, uintptr(f.px(820)), uintptr(f.px(680)), 1)
	procUpdateWindow.Call(p.hwnd)
	captureNativeWindow(t, p.hwnd, filepath.Join(dir, "template-maker-small.png"))
	out := projectDir(f.exeDir, p.project)
	send(p.controls[makerSave], 0xF5, 0, 0)
	if !strings.Contains(p.value(makerStatus), "已保存图片") {
		t.Fatal(p.value(makerStatus))
	}
	lib, err := loadExternalTemplateDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Entries) != 1 || lib.Entries[0].ID != "native-regression-only" {
		t.Fatal("save did not publish template")
	}
	if failures, err := lib.SelfTest(); err != nil {
		t.Fatalf("%v: %v", err, failures)
	}
	if fileExists(filepath.Join(f.exeDir, "templates", "templates.index.json")) {
		t.Fatal("draft became executable before publishing")
	}
	p.publishProject()
	if !strings.Contains(p.value(makerStatus), "已发布") {
		t.Fatal(p.value(makerStatus))
	}
	lib, err = loadExternalTemplateDir(filepath.Join(f.exeDir, "templates"))
	if err != nil || len(lib.Entries) != 1 {
		t.Fatalf("publication failed: %v", err)
	}
	if len(p.releases) != 1 || int(send(p.controls[makerVersions], 0x146, 0, 0)) != 1 {
		t.Fatal("published version not exposed in native selector")
	}
	send(p.controls[makerRollback], 0xF5, 0, 0)
	if !strings.Contains(p.value(makerStatus), "已恢复发布版本") {
		t.Fatal("rollback button failed: " + p.value(makerStatus))
	}
	captureNativeWindow(t, p.hwnd, filepath.Join(dir, "template-project-published.png"))
	p.captureScreen()
	var overlay uintptr
	for h, s := range screenPanes {
		if s.pane == p {
			overlay = h
		}
	}
	if overlay == 0 {
		t.Fatal("screen overlay did not open")
	}
	captureNativeWindow(t, overlay, filepath.Join(dir, "template-screen-overlay.png"))
	send(overlay, wmLButtonDown, 1, uintptr(100|100<<16))
	send(overlay, wmMouseMove, 1, uintptr(180|126<<16))
	send(overlay, 0x202, 0, uintptr(180|126<<16))
	if p.bounds != image.Rect(100, 100, 180, 126) || p.capturing {
		t.Fatal("direct screen selection did not complete")
	}
	p.captureScreen()
	for h, s := range screenPanes {
		if s.pane == p {
			send(h, 0x100, 27, 0)
		}
	}
	if p.capturing || len(screenPanes) != 0 {
		t.Fatal("cancel left screen overlay active")
	}
	p.deleteProjectImage()
	if len(p.entries) != 0 {
		t.Fatal("draft image deletion failed")
	}
	lib, err = loadExternalTemplateDir(filepath.Join(f.exeDir, "templates"))
	if err != nil || len(lib.Entries) != 1 {
		t.Fatal("draft deletion changed published version")
	}
	p.withdrawProject()
	if p.project.PublishedAt != nil {
		t.Fatal("withdraw did not reset state")
	}
	procDestroyWindow.Call(p.hwnd)
	if f.templateMaker != 0 {
		t.Fatal("closed maker retained stale handle")
	}
}
