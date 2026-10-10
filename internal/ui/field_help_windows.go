//go:build windows

package ui

import (
	"sync"
	"syscall"
	"unsafe"
)

type helpPane struct {
	owner                *form
	window, search, body uintptr
	text                 string
	px                   func(int) int
}

var helpPanes = map[uintptr]*helpPane{}
var helpClassOnce sync.Once
var helpClassName = utf16Ptr("AutoTriggerFieldHelp")

func helpWindowProc(h uintptr, msg uint32, w uintptr, l unsafe.Pointer) uintptr {
	p := helpPanes[h]
	if p != nil {
		switch msg {
		case wmSize:
			var r rectT
			procGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
			pad := p.px(16)
			procMoveWindow.Call(p.search, uintptr(pad), uintptr(pad), uintptr(maxInt(1, int(r.Right)-2*pad)), uintptr(p.px(30)), 1)
			procMoveWindow.Call(p.body, uintptr(pad), uintptr(p.px(58)), uintptr(maxInt(1, int(r.Right)-2*pad)), uintptr(maxInt(1, int(r.Bottom)-p.px(74))), 1)
			return 0
		case 0x111:
			if w&0xffff == 2 && w>>16 == 0 {
				procDestroyWindow.Call(h)
				return 0
			}
			if int(w&0xffff) == 1 && int(w>>16) == enChange {
				setWindowTextRaw(p.body, filterFieldHelp(p.text, windowText(p.search)))
				return 0
			}
		case wmCtlColorStatic, wmCtlColorEdit:
			procSetBkMode.Call(w, opaqueBk)
			color, _, _ := procGetSysColor.Call(colorWindow)
			procSetBkColor.Call(w, color)
			color, _, _ = procGetSysColor.Call(colorWindowText)
			procSetTextColor.Call(w, color)
			return sysColorBrush(colorWindow)
		case 0x82:
			if p.owner.helpWindow == h {
				p.owner.helpWindow = 0
			}
			delete(helpPanes, h)
		}
	}
	r, _, _ := procDefWindowProcW.Call(h, uintptr(msg), w, uintptr(l))
	return r
}

func (f *form) showFieldHelp(title, body string) {
	helpClassOnce.Do(func() {
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
		wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: syscall.NewCallback(helpWindowProc), Instance: f.instance, Cursor: cursor, Background: colorWindow + 1, ClassName: helpClassName}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
	if f.helpWindow != 0 {
		procDestroyWindow.Call(f.helpWindow)
	}
	h, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(helpClassName)), uintptr(unsafe.Pointer(utf16Ptr(title))), wsOverlappedWindow|wsClipChildren, cwUseDefault, cwUseDefault, uintptr(f.px(800)), uintptr(f.px(620)), f.hwnd, 0, f.instance, 0)
	if h == 0 {
		f.setStatusKind("无法打开字段说明窗口。", statusKindFail)
		return
	}
	p := &helpPane{window: h, text: body, px: f.px, owner: f}
	helpPanes[h] = p
	f.helpWindow = h
	p.search, _, _ = procCreateWindowExW.Call(wsExClientEdge, uintptr(unsafe.Pointer(utf16Ptr("EDIT"))), 0, wsChild|wsVisible|wsTabStop|esAutoHScroll, 0, 0, 0, 0, h, 1, f.instance, 0)
	p.body, _, _ = procCreateWindowExW.Call(wsExClientEdge, uintptr(unsafe.Pointer(utf16Ptr("EDIT"))), 0, wsChild|wsVisible|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esReadOnly, 0, 0, 0, 0, h, 2, f.instance, 0)
	send(p.search, wmSetFont, f.font, 1)
	send(p.body, wmSetFont, f.font, 1)
	send(p.search, 0x1501, 1, uintptr(unsafe.Pointer(utf16Ptr("搜索字段名或中文含义"))))
	send(p.body, emSetLimit, 0x7ffffffe, 0)
	setWindowTextRaw(p.body, body)
	send(h, wmSize, 0, 0)
	procShowWindow.Call(h, swShow)
	procUpdateWindow.Call(h)
}
