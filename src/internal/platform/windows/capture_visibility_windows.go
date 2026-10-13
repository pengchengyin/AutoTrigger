//go:build windows

package windows

import (
	"autotrigger/internal/core/ports"
	"fmt"
	"unsafe"
)

// Capture only real visible desktop pixels, never silently include covering windows.
func CaptureRegionVisible(w ports.Window, region ports.Rect) error {
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(w.Handle)
	iconic, _, _ := user32.NewProc("IsIconic").Call(w.Handle)
	if visible == 0 || iconic != 0 {
		return fmt.Errorf("目标窗口不可见或已最小化")
	}
	metric := func(index uintptr) int {
		value, _, _ := user32.NewProc("GetSystemMetrics").Call(index)
		return int(int32(value))
	}
	left, top := metric(76), metric(77)
	right, bottom := left+metric(78), top+metric(79)
	if region.Left < left || region.Top < top || region.Right > right || region.Bottom > bottom {
		return fmt.Errorf("目标区域超出屏幕，请将窗口移到可见屏幕内")
	}
	h, _, _ := user32.NewProc("GetWindow").Call(w.Handle, 3)
	seen := map[uintptr]bool{}
	for h != 0 {
		if seen[h] || len(seen) > 10000 {
			return fmt.Errorf("无法可靠核实窗口遮挡")
		}
		seen[h] = true
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(h)
		iconic, _, _ := user32.NewProc("IsIconic").Call(h)
		if visible != 0 && iconic == 0 {
			var rect nativeRect
			ok, _, _ := user32.NewProc("GetWindowRect").Call(h, uintptr(unsafe.Pointer(&rect)))
			if ok == 0 {
				return fmt.Errorf("无法核实上层窗口范围")
			}
			b := CaptureWindowBounds(ports.Window{Handle: h, Bounds: rect.rect()})
			if b.Left < region.Right && b.Right > region.Left && b.Top < region.Bottom && b.Bottom > region.Top {
				return fmt.Errorf("目标仍被上层窗口 %q 遮挡，请移开覆盖窗口后重试", windowText(h))
			}
		}
		h, _, _ = user32.NewProc("GetWindow").Call(h, 3)
	}
	return nil
}
