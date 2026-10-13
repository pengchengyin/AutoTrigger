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

type tooltipEntry struct {
	handle uintptr
	text   string
	label  *uint16
}

func (f *form) addTooltip(id int, text string) {
	if f.tooltipEntries == nil {
		f.tooltipEntries = map[int]tooltipEntry{}
	}
	entry := f.tooltipEntries[id]
	if entry.handle != 0 && entry.text == text {
		return
	}
	if entry.handle != 0 {
		label := utf16Ptr(text)
		tool := tooltipInfo{Size: uint32(unsafe.Sizeof(tooltipInfo{})), Flags: 1 | 16, Window: f.hwnd, ID: f.ctrls[id], Text: label}
		send(entry.handle, 0x439, 0, uintptr(unsafe.Pointer(&tool)))
		f.tooltipEntries[id] = tooltipEntry{entry.handle, text, label}
		return
	}
	h, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr("tooltips_class32"))), 0, 0x80000001, 0, 0, 0, 0, f.hwnd, 0, f.instance, 0)
	if h == 0 {
		return
	}
	label := utf16Ptr(text)
	f.tooltipEntries[id] = tooltipEntry{h, text, label}
	tool := tooltipInfo{Size: uint32(unsafe.Sizeof(tooltipInfo{})), Flags: 1 | 16, Window: f.hwnd, ID: f.ctrls[id], Text: label}
	send(h, 0x432, 0, uintptr(unsafe.Pointer(&tool)))
	send(h, 0x418, 0, uintptr(f.px(600)))
	send(h, 0x403, 2, 30000)
}
