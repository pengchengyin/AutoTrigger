//go:build windows

package ui

import "unsafe"

type tooltipInfo struct {
	Size, Flags     uint32
	Window, ID      uintptr
	Rect            rectT
	Instance        uintptr
	Text            *uint16
	Param, Reserved uintptr
}

func (f *form) addTooltip(id int, text string) {
	h, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr("tooltips_class32"))), 0, 0x80000001, 0, 0, 0, 0, f.hwnd, 0, f.instance, 0)
	if h == 0 {
		return
	}
	label := utf16Ptr(text)
	f.tooltipTexts = append(f.tooltipTexts, label)
	tool := tooltipInfo{Size: uint32(unsafe.Sizeof(tooltipInfo{})), Flags: 1 | 16, Window: f.hwnd, ID: f.ctrls[id], Text: label}
	send(h, 0x432, 0, uintptr(unsafe.Pointer(&tool)))
}
