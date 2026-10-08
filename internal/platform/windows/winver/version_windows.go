//go:build windows

// Package winver centralizes OS capability and DPI compatibility decisions.
package winver

import (
	"fmt"
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var gdi32 = syscall.NewLazyDLL("gdi32.dll")

type Version struct {
	Major       uint32
	Minor       uint32
	Build       uint32
	ServicePack uint16
}
type versionInfo struct {
	Size     uint32
	Major    uint32
	Minor    uint32
	Build    uint32
	Platform uint32
	CSD      [128]uint16
	SPMajor  uint16
	SPMinor  uint16
	Suite    uint16
	Product  byte
	Reserved byte
}

func Current() Version {
	v := versionInfo{}
	v.Size = uint32(unsafe.Sizeof(v))
	syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&v)))
	return Version{v.Major, v.Minor, v.Build, v.SPMajor}
}
func (v Version) String() string    { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Build) }
func (v Version) NativeMount() bool { return v.Major > 6 || (v.Major == 6 && v.Minor >= 2) }
func (v Version) PrintFlags() uintptr {
	if v.Major > 6 || (v.Major == 6 && v.Minor >= 3) {
		return 2
	}
	return 0
}
func EnableDPI() string {
	p := user32.NewProc("SetProcessDpiAwarenessContext")
	if p.Find() == nil {
		ok, _, _ := p.Call(^uintptr(3))
		if ok != 0 {
			return "per_monitor_v2"
		}
	}
	ok, _, _ := user32.NewProc("SetProcessDPIAware").Call()
	if ok != 0 {
		return "system"
	}
	return "unchanged"
}
func DPI(hwnd uintptr) int {
	p := user32.NewProc("GetDpiForWindow")
	if p.Find() == nil {
		v, _, _ := p.Call(hwnd)
		if v > 0 {
			return int(v)
		}
	}
	dc, _, _ := user32.NewProc("GetDC").Call(0)
	if dc == 0 {
		return 96
	}
	defer user32.NewProc("ReleaseDC").Call(0, dc)
	v, _, _ := gdi32.NewProc("GetDeviceCaps").Call(dc, 88)
	if v == 0 {
		return 96
	}
	return int(v)
}

func cpuID(eax, ecx uint32) (a, b, c, d uint32)
func xgetbv() (a, d uint32)
func AVX() (bool, bool) {
	_, _, c, _ := cpuID(1, 0)
	if c&(1<<27) == 0 || c&(1<<28) == 0 {
		return false, false
	}
	a, _ := xgetbv()
	if a&6 != 6 {
		return false, false
	}
	_, b, _, _ := cpuID(7, 0)
	return true, b&(1<<5) != 0
}
