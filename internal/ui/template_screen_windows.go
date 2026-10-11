//go:build windows

package ui

import (
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/windows"
	"autotrigger/internal/platform/windows/winver"
	"fmt"
	"image"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var screenClassOnce sync.Once
var screenClass = utf16Ptr("AutoTriggerScreenSelection")
var screenPanes = map[uintptr]*screenSelection{}

type screenSelection struct {
	pane     *templateMakerPane
	origin   image.Point
	anchor   image.Point
	rect     image.Rectangle
	dragging bool
	accepted bool
}

func (p *templateMakerPane) captureScreen() {
	if p.capturing {
		return
	}
	left, _, _ := user32.NewProc("GetSystemMetrics").Call(76)
	top, _, _ := user32.NewProc("GetSystemMetrics").Call(77)
	w, _, _ := user32.NewProc("GetSystemMetrics").Call(78)
	h, _, _ := user32.NewProc("GetSystemMetrics").Call(79)
	if w == 0 || h == 0 || uint64(w)*uint64(h) > 25000000 {
		p.status("屏幕范围过大，无法截图")
		return
	}
	origin := image.Pt(int(int32(left)), int(int32(top)))
	p.capturing = true
	procShowWindow.Call(p.hwnd, swHide)
	procShowWindow.Call(p.owner.hwnd, swHide)
	time.Sleep(120 * time.Millisecond)
	shot, err := (windows.GUI{}).RawScreenshot(ports.Window{Bounds: ports.Rect{Left: origin.X, Top: origin.Y, Right: origin.X + int(w), Bottom: origin.Y + int(h)}})
	if err != nil {
		p.capturing = false
		procShowWindow.Call(p.owner.hwnd, swShow)
		procShowWindow.Call(p.hwnd, swShow)
		p.status("截图失败：" + err.Error())
		return
	}
	p.setImage(shot, "屏幕框选")
	screenClassOnce.Do(func() {
		cursor, _, _ := procLoadCursorW.Call(0, 32515)
		wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: syscall.NewCallback(screenSelectionProc), Instance: p.owner.instance, Cursor: cursor, ClassName: screenClass}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
	overlay, _, _ := procCreateWindowExW.Call(8|0x80, uintptr(unsafe.Pointer(screenClass)), uintptr(unsafe.Pointer(utf16Ptr("截图标记"))), 0x80000000, uintptr(int64(origin.X)), uintptr(int64(origin.Y)), w, h, 0, 0, p.owner.instance, 0)
	if overlay == 0 {
		p.capturing = false
		procShowWindow.Call(p.owner.hwnd, swShow)
		procShowWindow.Call(p.hwnd, swShow)
		p.status("无法打开截图框选层")
		return
	}
	screenPanes[overlay] = &screenSelection{pane: p, origin: origin}
	procShowWindow.Call(overlay, swShow)
	user32.NewProc("SetForegroundWindow").Call(overlay)
	procSetFocus.Call(overlay)
}

func screenSelectionProc(h uintptr, msg uint32, w uintptr, l unsafe.Pointer) uintptr {
	s := screenPanes[h]
	if s != nil {
		p := s.pane
		switch msg {
		case wmEraseBkgnd:
			return 1
		case wmPaint:
			var ps paintStruct
			dc, _, _ := procBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			b := p.shot.Bounds()
			info := makerBitmapInfo{Size: 40, Width: int32(b.Dx()), Height: -int32(b.Dy()), Planes: 1, Bits: 32}
			gdi32.NewProc("StretchDIBits").Call(dc, 0, 0, uintptr(b.Dx()), uintptr(b.Dy()), 0, 0, uintptr(b.Dx()), uintptr(b.Dy()), uintptr(unsafe.Pointer(&p.pixels[0])), uintptr(unsafe.Pointer(&info)), 0, 0xcc0020)
			if !s.rect.Empty() {
				r := rectT{Left: int32(s.rect.Min.X), Top: int32(s.rect.Min.Y), Right: int32(s.rect.Max.X), Bottom: int32(s.rect.Max.Y)}
				brush, _, _ := procCreateSolidBrush.Call(modernTheme.primary)
				user32.NewProc("FrameRect").Call(dc, uintptr(unsafe.Pointer(&r)), brush)
				procDeleteObject.Call(brush)
			}
			procEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			return 0
		case wmLButtonDown:
			x, y := mousePoint(l)
			s.anchor = image.Pt(x, y)
			s.dragging = true
			user32.NewProc("SetCapture").Call(h)
			return 0
		case wmMouseMove, 0x202:
			if s.dragging {
				x, y := mousePoint(l)
				s.rect = image.Rectangle{Min: s.anchor, Max: image.Pt(x, y)}.Canon().Intersect(p.shot.Bounds())
				procInvalidateRect.Call(h, 0, 0)
				if msg == 0x202 {
					s.dragging = false
					user32.NewProc("ReleaseCapture").Call()
					if !s.rect.Empty() {
						s.accepted = true
						procDestroyWindow.Call(h)
					}
				}
			}
			return 0
		case 0x100:
			if w == 27 {
				procDestroyWindow.Call(h)
				return 0
			}
		case 0x205, 0x10:
			procDestroyWindow.Call(h)
			return 0
		case 0x215:
			s.dragging = false
			return 0
		case 0x82:
			delete(screenPanes, h)
			p.capturing = false
			procShowWindow.Call(h, swHide)
			if s.accepted {
				p.bounds = s.rect
				point := s.origin.Add(s.rect.Min).Add(image.Pt(s.rect.Dx()/2, s.rect.Dy()/2))
				packed := uintptr(uint64(uint32(point.X)) | uint64(uint32(point.Y))<<32)
				under, _, _ := user32.NewProc("WindowFromPoint").Call(packed)
				dpi := winver.DPI(under) * 100 / 96
				for i, v := range []int{100, 125, 150, 200} {
					if v == dpi {
						send(p.controls[makerDPI], 0x14e, uintptr(i), 0)
					}
				}
				p.updateBoundsFields()
				p.status(fmt.Sprintf("已标记 %d × %d 像素", p.bounds.Dx(), p.bounds.Dy()))
			} else {
				p.status("已取消截图")
			}
			procShowWindow.Call(p.owner.hwnd, swShow)
			procShowWindow.Call(p.hwnd, swShow)
			user32.NewProc("SetForegroundWindow").Call(p.hwnd)
		}
	}
	r, _, _ := procDefWindowProcW.Call(h, uintptr(msg), w, uintptr(l))
	return r
}
