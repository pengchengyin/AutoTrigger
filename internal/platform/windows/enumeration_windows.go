//go:build windows

package windows

import (
	"sync"
	"syscall"
)

var enumerations = struct {
	sync.Mutex
	next     uintptr
	visitors map[uintptr]func(uintptr, uintptr) uintptr
}{visitors: make(map[uintptr]func(uintptr, uintptr) uintptr)}

// Go callback trampolines are never reclaimed. Keep one trampoline and scope
// the visitor to the synchronous native enumeration, including nested calls.
var enumerationCallback = syscall.NewCallback(func(hwnd, token uintptr) uintptr {
	enumerations.Lock()
	visit := enumerations.visitors[token]
	enumerations.Unlock()
	if visit == nil {
		return 0
	}
	return visit(hwnd, token)
})

func enumerateWindows(parent uintptr, children bool, visit func(uintptr, uintptr) uintptr) error {
	enumerations.Lock()
	for {
		enumerations.next++
		if enumerations.next != 0 && enumerations.visitors[enumerations.next] == nil {
			break
		}
	}
	token := enumerations.next
	enumerations.visitors[token] = visit
	enumerations.Unlock()
	defer func() {
		enumerations.Lock()
		delete(enumerations.visitors, token)
		enumerations.Unlock()
	}()
	if children {
		// EnumChildWindows' return value is not an error indicator.
		user32.NewProc("EnumChildWindows").Call(parent, enumerationCallback, token)
		return nil
	}
	return callOK(user32.NewProc("EnumWindows"), enumerationCallback, token)
}
