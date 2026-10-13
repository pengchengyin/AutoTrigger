//go:build windows

package ui

import (
	_ "embed"
	"encoding/binary"
	"unsafe"
)

//go:embed assets/AutoTrigger.ico
var appIconData []byte

func loadAppIcon(size int) uintptr {
	if len(appIconData) < 6 {
		return 0
	}
	count := int(binary.LittleEndian.Uint16(appIconData[4:6]))
	best, difference := -1, 10000
	for i := 0; i < count; i++ {
		start := 6 + i*16
		if start+16 > len(appIconData) {
			break
		}
		w := int(appIconData[start])
		if w == 0 {
			w = 256
		}
		delta := w - size
		if delta < 0 {
			delta = -delta
		}
		if delta < difference {
			best, difference = start, delta
		}
	}
	if best < 0 {
		return 0
	}
	n := int(binary.LittleEndian.Uint32(appIconData[best+8 : best+12]))
	offset := int(binary.LittleEndian.Uint32(appIconData[best+12 : best+16]))
	if n <= 0 || offset < 0 || offset+n > len(appIconData) {
		return 0
	}
	icon, _, _ := user32.NewProc("CreateIconFromResourceEx").Call(uintptr(unsafe.Pointer(&appIconData[offset])), uintptr(n), 1, 0x30000, uintptr(size), uintptr(size), 0)
	return icon
}

func (f *form) paintAppIcon(dc uintptr, r rectT) {
	if f.appIcon != 0 {
		user32.NewProc("DrawIconEx").Call(dc, uintptr(r.Left), uintptr(r.Top), f.appIcon, uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0, 0, 3)
	}
}
