//go:build windows

package windows

import (
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/windows/winver"
	"autotrigger/internal/procutil"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// PickTarget is a read-only observation; it never invokes a provider action.
type PickTarget struct {
	Window       ports.Window   `json:"window"`
	Control      *ports.Control `json:"control,omitempty"`
	ControlType  int32          `json:"control_type,omitempty"`
	ClassName    string         `json:"class_name,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	CaptureError string         `json:"capture_error,omitempty"`
}

// Exclude invisible DWM resize margins when available; retain Win7 fallback.
func CaptureWindowBounds(w ports.Window) ports.Rect {
	var r nativeRect
	proc := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
	if proc.Find() == nil {
		result, _, _ := proc.Call(w.Handle, 9, uintptr(unsafe.Pointer(&r)), unsafe.Sizeof(r))
		b := r.rect()
		if result == 0 && b.Width() > 0 && b.Height() > 0 && b.Left >= w.Bounds.Left && b.Top >= w.Bounds.Top && b.Right <= w.Bounds.Right && b.Bottom <= w.Bounds.Bottom {
			return b
		}
	}
	return w.Bounds
}

// Capturing is not a way to bypass UIPI or the secure desktop. Keep the
// input-interception guarantee limited to observable, same-or-lower privilege targets.
func CapturePermission(pid int) error {
	own, target := processIntegrityRID(os.Getpid()), processIntegrityRID(pid)
	if own == 0 || target == 0 {
		return fmt.Errorf("无法核实目标窗口权限，已停止捕获")
	}
	if target > own {
		return fmt.Errorf("目标窗口权限高于本工具（目标 0x%x，本工具 0x%x），无法保证点击拦截；请以相同权限运行工具后重试", target, own)
	}
	return nil
}

func WindowAtPoint(x, y int) (ports.Window, error) {
	packed := uintptr(uint64(uint32(x)) | uint64(uint32(y))<<32)
	h, _, _ := user32.NewProc("WindowFromPoint").Call(packed)
	h, _, _ = user32.NewProc("GetAncestor").Call(h, 2)
	valid, _, _ := user32.NewProc("IsWindow").Call(h)
	if valid == 0 {
		return ports.Window{}, fmt.Errorf("鼠标位置没有有效窗口")
	}
	var rect nativeRect
	if err := callOK(user32.NewProc("GetWindowRect"), h, uintptr(unsafe.Pointer(&rect))); err != nil {
		return ports.Window{}, err
	}
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(h, uintptr(unsafe.Pointer(&pid)))
	return ports.Window{Handle: h, PID: int(pid), Title: windowText(h), Bounds: rect.rect(), DPI: winver.DPI(h)}, nil
}

func (GUI) PickAtPoint(ctx context.Context, x, y int) (PickTarget, error) {
	exe, err := os.Executable()
	if err != nil {
		return PickTarget{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()
	result, err := procutil.Run(ctx, Runner{}, ports.Command{Program: exe, HideWindow: true, Args: []string{"--internal-uia-point", strconv.Itoa(x) + "," + strconv.Itoa(y)}})
	if err != nil {
		return PickTarget{}, fmt.Errorf("UIA 点识别失败或超时：%w", err)
	}
	var target PickTarget
	if err = json.Unmarshal([]byte(result.Stdout), &target); err != nil {
		return target, err
	}
	return target, nil
}

// RawPickAtPoint must run in the disposable helper, never the UI/hook thread.
func (GUI) RawPickAtPoint(x, y int) (target PickTarget, err error) {
	target.Window, err = WindowAtPoint(x, y)
	if err != nil {
		return target, err
	}
	if permission := CapturePermission(target.Window.PID); permission != nil {
		target.CaptureError = permission.Error()
		return target, nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 0)
	if int32(hr) < 0 {
		target.Reason = "UIA 初始化失败，请手动框选"
		return target, nil
	}
	defer ole32.NewProc("CoUninitialize").Call()
	clsid := guid{0xff48dba4, 0x60ef, 0x4201, [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
	iid := guid{0x30cbe57d, 0xd9d0, 0x452a, [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee}}
	var automation, element uintptr
	hr, _, _ = ole32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&automation)))
	if int32(hr) < 0 {
		target.Reason = "UIA 不可用，请手动框选"
		return target, nil
	}
	defer release(automation)
	packed := uintptr(uint64(uint32(x)) | uint64(uint32(y))<<32)
	if err = comCall(automation, 7, packed, uintptr(unsafe.Pointer(&element))); err != nil {
		target.Reason = err.Error()
		return target, nil
	}
	defer release(element)
	var kind, enabled, offscreen, pid int32
	var rect nativeRect
	for _, property := range []struct {
		index int
		out   uintptr
	}{
		{20, uintptr(unsafe.Pointer(&pid))}, {21, uintptr(unsafe.Pointer(&kind))},
		{28, uintptr(unsafe.Pointer(&enabled))}, {38, uintptr(unsafe.Pointer(&offscreen))}, {43, uintptr(unsafe.Pointer(&rect))},
	} {
		if err = comCall(element, property.index, property.out); err != nil {
			target.Reason = "控件已失效：" + err.Error()
			return target, nil
		}
	}
	target.ClassName = bstr(element, 30)
	target.ControlType = kind
	bounds := rect.rect()
	if int(pid) != target.Window.PID || offscreen != 0 || bounds == target.Window.Bounds || !bounds.Contains(x, y) || bounds.Width() <= 0 || bounds.Height() <= 0 ||
		bounds.Left < target.Window.Bounds.Left || bounds.Top < target.Window.Bounds.Top || bounds.Right > target.Window.Bounds.Right || bounds.Bottom > target.Window.Bounds.Bottom || kind == 50032 || kind == 50033 {
		target.Reason = "未识别到可靠的控件区域，请手动框选"
		return target, nil
	}
	role := map[int32]string{50000: "button", 50002: "checkbox", 50003: "combobox", 50004: "edit", 50013: "radio", 50020: "text"}[kind]
	if role == "" {
		role = "control-" + strconv.Itoa(int(kind))
	}
	target.Control = &ports.Control{ID: bstr(element, 29), Text: bstr(element, 23), Role: role, Bounds: bounds, Window: target.Window, Source: "uia", Enabled: enabled != 0, Checked: uiaChecked(element, kind), Confidence: 1}
	return target, nil
}
