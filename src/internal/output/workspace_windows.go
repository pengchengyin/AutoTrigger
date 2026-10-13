//go:build windows

package output

import "syscall"

func ownerExited(pid int) bool {
	k := syscall.NewLazyDLL("kernel32.dll")
	h, _, err := k.NewProc("OpenProcess").Call(0x100000, 0, uintptr(pid))
	if h == 0 {
		return err == syscall.Errno(87)
	}
	defer k.NewProc("CloseHandle").Call(h)
	r, _, _ := k.NewProc("WaitForSingleObject").Call(h, 0)
	return r == 0
}
