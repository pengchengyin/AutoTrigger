//go:build windows

// This benign fixture writes only to AT_FIXTURE_DEST and never touches the registry.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var step int
var button, caption uintptr
var destination string

type windowClass struct {
	Size        uint32
	Style       uint32
	Proc        uintptr
	ClassExtra  int32
	WindowExtra int32
	Instance    uintptr
	Icon        uintptr
	Cursor      uintptr
	Background  uintptr
	Menu        *uint16
	Name        *uint16
	SmallIcon   uintptr
}
type message struct {
	Window  uintptr
	Message uint32
	Pad     uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X, Y    int32
	Private uint32
}

func wide(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	root := filepath.Join(destination, "FixtureApp")
	if err = os.MkdirAll(root, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "FixtureApp.exe"), data, 0755)
}
func main() {
	destination = os.Getenv("AT_FIXTURE_DEST")
	if destination == "" {
		fmt.Fprintln(os.Stderr, "AT_FIXTURE_DEST is required")
		os.Exit(2)
	}
	if len(os.Args) > 1 {
		if os.Args[1] == "--probe" {
			return
		}
		if os.Args[1] == "--silent-fixture" {
			if err := install(); err != nil {
				os.Exit(3)
			}
			return
		}
		os.Exit(2)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	proc := syscall.NewCallback(func(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
		switch msg {
		case 0x111:
			if wparam&0xffff == 101 {
				step++
				switch step {
				case 1:
					user32.NewProc("SetWindowTextW").Call(caption, uintptr(unsafe.Pointer(wide("Choose destination"))))
					user32.NewProc("SetWindowTextW").Call(button, uintptr(unsafe.Pointer(wide("Next >"))))
				case 2:
					user32.NewProc("SetWindowTextW").Call(caption, uintptr(unsafe.Pointer(wide("Ready to install"))))
					user32.NewProc("SetWindowTextW").Call(button, uintptr(unsafe.Pointer(wide("Install"))))
				case 3:
					if install() != nil {
						user32.NewProc("SetWindowTextW").Call(caption, uintptr(unsafe.Pointer(wide("Installation failed"))))
					} else {
						user32.NewProc("SetWindowTextW").Call(caption, uintptr(unsafe.Pointer(wide("Installation complete"))))
					}
					user32.NewProc("SetWindowTextW").Call(button, uintptr(unsafe.Pointer(wide("Finish"))))
				default:
					user32.NewProc("DestroyWindow").Call(hwnd)
				}
			}
			return 0
		case 0x10:
			user32.NewProc("DestroyWindow").Call(hwnd)
			return 0
		case 2:
			user32.NewProc("PostQuitMessage").Call(0)
			return 0
		}
		r, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wparam, lparam)
		return r
	})
	class := windowClass{Proc: proc, Instance: instance, Background: 16, Name: wide("AutoTriggerBenignFixture")}
	class.Size = uint32(unsafe.Sizeof(class))
	user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class.Name)), uintptr(unsafe.Pointer(wide("AutoTrigger fixture setup"))), 0x10cf0000, 160, 160, 480, 280, 0, 0, instance, 0)
	caption, _, _ = user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(wide("STATIC"))), uintptr(unsafe.Pointer(wide("License agreement"))), 0x50000000, 24, 32, 400, 40, hwnd, 0, instance, 0)
	button, _, _ = user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(wide("BUTTON"))), uintptr(unsafe.Pointer(wide("I Agree"))), 0x50000001, 310, 170, 120, 34, hwnd, 101, instance, 0)
	user32.NewProc("ShowWindow").Call(hwnd, 5)
	var m message
	for {
		r, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
}
