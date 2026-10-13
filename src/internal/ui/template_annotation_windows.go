//go:build windows

package ui

import (
	"autotrigger/internal/core/ports"
	"fmt"
	"image"
	"image/draw"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const annotationSave = 701
const annotationCancel = 702
const annotationAction = 703
const annotationName = 704

func annotationMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var annotationOnce sync.Once
var annotationClass = utf16Ptr("AutoTriggerCaptureAnnotation")
var annotationPanes = map[uintptr]*captureAnnotation{}

type captureAnnotation struct {
	s                  *hoverCapture
	hwnd, action, name uintptr
	item               draftCapture
	shot               image.Image
	pixels             []byte
	selection          image.Rectangle
	anchor, offset     image.Point
	dragging           bool
	bar                int
}

func (s *hoverCapture) openAnnotation(item draftCapture, shot image.Image) error {
	if shot == nil || shot.Bounds().Empty() {
		return fmt.Errorf("截图为空，无法标记")
	}
	annotationOnce.Do(func() {
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
		wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: syscall.NewCallback(annotationProc), Instance: s.pane.owner.instance, Cursor: cursor, ClassName: annotationClass}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
	a := &captureAnnotation{s: s, item: item, shot: shot, bar: s.pane.owner.px(76)}
	b := shot.Bounds()
	a.pixels = make([]byte, b.Dx()*b.Dy()*4)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, _ := shot.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := (y*b.Dx() + x) * 4
			a.pixels[i], a.pixels[i+1], a.pixels[i+2], a.pixels[i+3] = byte(bl>>8), byte(g>>8), byte(r>>8), 255
		}
	}
	var desktop rectT
	user32.NewProc("SystemParametersInfoW").Call(48, 0, uintptr(unsafe.Pointer(&desktop)), 0)
	w, h := annotationMin(maxInt(b.Dx()+40, s.pane.owner.px(600)), int(desktop.Right-desktop.Left)-40), annotationMin(b.Dy()+a.bar+70, int(desktop.Bottom-desktop.Top)-40)
	hwnd, _, err := procCreateWindowExW.Call(8|0x80, uintptr(unsafe.Pointer(annotationClass)), uintptr(unsafe.Pointer(utf16Ptr("标记捕获图片"))), 0x00cf0000|0x100000|wsVScroll, uintptr(int64(desktop.Left+20)), uintptr(int64(desktop.Top+20)), uintptr(w), uintptr(maxInt(h, 240)), 0, 0, s.pane.owner.instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("无法创建标记窗口：%v", err)
	}
	a.hwnd = hwnd
	annotationPanes[hwnd] = a
	s.annotation = hwnd
	add := func(id int, class, text string, style uint32, x, width int) uintptr {
		c, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(wsChild|wsVisible|wsTabStop|style), uintptr(s.pane.owner.px(x)), uintptr(s.pane.owner.px(12)), uintptr(s.pane.owner.px(width)), uintptr(s.pane.owner.px(30)), hwnd, uintptr(id), s.pane.owner.instance, 0)
		send(c, wmSetFont, s.pane.owner.font, 1)
		return c
	}
	a.action = add(annotationAction, "COMBOBOX", "", 3|wsVScroll, 12, 130)
	procMoveWindow.Call(a.action, uintptr(s.pane.owner.px(12)), uintptr(s.pane.owner.px(12)), uintptr(s.pane.owner.px(130)), uintptr(s.pane.owner.px(250)), 1)
	for _, text := range []string{"选择标记动作", "下一步", "安装", "接受协议", "确定", "是", "完成"} {
		send(a.action, 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr(text))))
	}
	send(a.action, 0x14e, 0, 0)
	a.name = add(annotationName, "EDIT", "", esAutoHScroll|wsBorder, 152, 180)
	label := add(705, "STATIC", "名称（选填）", 0, 152, 180)
	procMoveWindow.Call(label, uintptr(s.pane.owner.px(152)), uintptr(s.pane.owner.px(8)), uintptr(s.pane.owner.px(180)), uintptr(s.pane.owner.px(22)), 1)
	actionLabel := add(706, "STATIC", "标记动作", 0, 12, 130)
	procMoveWindow.Call(actionLabel, uintptr(s.pane.owner.px(12)), uintptr(s.pane.owner.px(8)), uintptr(s.pane.owner.px(130)), uintptr(s.pane.owner.px(22)), 1)
	add(annotationSave, "BUTTON", "确定保存", 0, 342, 110)
	add(annotationCancel, "BUTTON", "取消标记", 0, 462, 110)
	for _, id := range []int{annotationAction, annotationName, annotationSave, annotationCancel} {
		control, _, _ := user32.NewProc("GetDlgItem").Call(hwnd, uintptr(id))
		var r rectT
		user32.NewProc("GetWindowRect").Call(control, uintptr(unsafe.Pointer(&r)))
		point := struct{ X, Y int32 }{r.Left, r.Top}
		user32.NewProc("ScreenToClient").Call(hwnd, uintptr(unsafe.Pointer(&point)))
		user32.NewProc("SetWindowPos").Call(control, 0, uintptr(point.X), uintptr(s.pane.owner.px(34)), 0, 0, 0x15)
	}
	send(a.name, 0x1501, 1, uintptr(unsafe.Pointer(utf16Ptr("标记名称（选填）"))))
	if input := hookInput.Load(); input != nil {
		input.annotation.Store(hwnd)
	}
	s.paused = true
	s.setFrame(image.Rectangle{})
	procShowWindow.Call(s.hwnd, swHide)
	a.scrollbars()
	procShowWindow.Call(hwnd, swShow)
	user32.NewProc("SetForegroundWindow").Call(hwnd)
	return nil
}

func (a *captureAnnotation) scrollbars() {
	var r rectT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&r)))
	for axis, v := range []struct{ max, page, pos int }{{a.shot.Bounds().Dx(), int(r.Right), a.offset.X}, {a.shot.Bounds().Dy(), maxInt(1, int(r.Bottom)-a.bar), a.offset.Y}} {
		si := scrollInfo{Mask: 7, Max: int32(v.max - 1), Page: uint32(v.page), Pos: int32(v.pos)}
		si.Size = uint32(unsafe.Sizeof(si))
		user32.NewProc("SetScrollInfo").Call(a.hwnd, uintptr(axis), uintptr(unsafe.Pointer(&si)), 1)
	}
}

func (a *captureAnnotation) close() {
	if input := hookInput.Load(); input != nil {
		input.annotation.Store(0)
	}
	a.s.annotation = 0
	a.s.paused = false
	procDestroyWindow.Call(a.hwnd)
	procShowWindow.Call(a.s.hwnd, 4)
	a.s.notice("继续选择窗口，或切换手动框选")
}

func (a *captureAnnotation) save() {
	index := int(send(a.action, 0x147, 0, 0))
	actions := []string{"", "next", "install", "accept", "ok", "yes", "finish"}
	if index <= 0 || index >= len(actions) || a.selection.Empty() {
		messageBox(a.hwnd, "标记未完成", "请选择动作，并在图片中拖动框选目标区域。", mbOK|mbIconInfo)
		return
	}
	region := a.selection.Intersect(image.Rect(0, 0, a.shot.Bounds().Dx(), a.shot.Bounds().Dy()))
	item := a.item
	item.Control = nil
	item.Visible = nil
	item.Rule = captureRule{Method: "image", Action: actions[index], MinSimilarity: 0.95}
	item.Name = strings.TrimSpace(windowText(a.name))
	if item.Name == "" {
		item.Name = windowText(a.action)
	}
	item.Bounds = ports.Rect{Left: item.Bounds.Left + region.Min.X, Top: item.Bounds.Top + region.Min.Y, Right: item.Bounds.Left + region.Max.X, Bottom: item.Bounds.Top + region.Max.Y}
	img := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(img, img.Bounds(), a.shot, a.shot.Bounds().Min.Add(region.Min), draw.Src)
	a.close()
	a.s.busy = true
	a.s.paused = true
	go func() {
		saved, err := a.s.session.save(item, img)
		a.s.work <- captureWork{item: saved, err: err, kind: "save"}
	}()
}

func annotationProc(h uintptr, msg uint32, w uintptr, l unsafe.Pointer) uintptr {
	a := annotationPanes[h]
	if a != nil {
		switch msg {
		case wmEraseBkgnd:
			return 1
		case wmPaint:
			var ps paintStruct
			dc, _, _ := procBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			var r rectT
			procGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
			fillSolid(dc, r, modernTheme.surface)
			b := a.shot.Bounds()
			info := makerBitmapInfo{Size: 40, Width: int32(b.Dx()), Height: -int32(b.Dy()), Planes: 1, Bits: 32}
			gdi32.NewProc("StretchDIBits").Call(dc, uintptr(-int64(a.offset.X)), uintptr(int64(a.bar-a.offset.Y)), uintptr(b.Dx()), uintptr(b.Dy()), 0, 0, uintptr(b.Dx()), uintptr(b.Dy()), uintptr(unsafe.Pointer(&a.pixels[0])), uintptr(unsafe.Pointer(&info)), 0, 0xcc0020)
			if !a.selection.Empty() {
				rr := a.selection.Sub(a.offset).Add(image.Pt(0, a.bar))
				brush, _, _ := procCreateSolidBrush.Call(modernTheme.primary)
				rect := rectT{Left: int32(rr.Min.X), Top: int32(rr.Min.Y), Right: int32(rr.Max.X), Bottom: int32(rr.Max.Y)}
				user32.NewProc("FrameRect").Call(dc, uintptr(unsafe.Pointer(&rect)), brush)
				procDeleteObject.Call(brush)
			}
			fillSolid(dc, rectT{Right: r.Right, Bottom: int32(a.bar)}, modernTheme.surface)
			procEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			return 0
		case wmSize:
			a.scrollbars()
			return 0
		case 0x114, 0x115:
			axis := uintptr(0)
			pos, max := a.offset.X, a.shot.Bounds().Dx()
			if msg == 0x115 {
				axis = 1
				pos, max = a.offset.Y, a.shot.Bounds().Dy()
			}
			var r rectT
			procGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
			page := int(r.Right)
			if axis == 1 {
				page = int(r.Bottom) - a.bar
			}
			switch w & 0xffff {
			case 0:
				pos -= 24
			case 1:
				pos += 24
			case 2:
				pos -= page
			case 3:
				pos += page
			case 4, 5:
				si := scrollInfo{Mask: 16}
				si.Size = uint32(unsafe.Sizeof(si))
				user32.NewProc("GetScrollInfo").Call(h, axis, uintptr(unsafe.Pointer(&si)))
				pos = int(si.Track)
			}
			pos = maxInt(0, annotationMin(pos, max-page))
			if axis == 0 {
				a.offset.X = pos
			} else {
				a.offset.Y = pos
			}
			a.scrollbars()
			procInvalidateRect.Call(h, 0, 0)
			return 0
		case 0x201, 0x200, 0x202:
			packed := uintptr(l)
			p := image.Pt(int(int16(packed&0xffff)), int(int16((packed>>16)&0xffff)))
			if msg == 0x201 && p.Y >= a.bar {
				a.dragging = true
				a.anchor = p.Sub(image.Pt(0, a.bar)).Add(a.offset)
				user32.NewProc("SetCapture").Call(h)
			}
			if a.dragging {
				point := p.Sub(image.Pt(0, a.bar)).Add(a.offset)
				a.selection = (image.Rectangle{Min: a.anchor, Max: point}).Canon().Intersect(image.Rect(0, 0, a.shot.Bounds().Dx(), a.shot.Bounds().Dy()))
				procInvalidateRect.Call(h, 0, 0)
			}
			if msg == 0x202 {
				a.dragging = false
				user32.NewProc("ReleaseCapture").Call()
			}
			return 0
		case 0x111:
			if w>>16 == 0 {
				switch int(w & 0xffff) {
				case annotationSave:
					a.save()
				case annotationCancel:
					a.close()
				}
			}
			return 0
		case 0x100:
			if w == 27 {
				a.close()
				return 0
			}
		case 0x10:
			a.close()
			return 0
		case 0x82:
			delete(annotationPanes, h)
		}
	}
	r, _, _ := procDefWindowProcW.Call(h, uintptr(msg), w, uintptr(l))
	return r
}
