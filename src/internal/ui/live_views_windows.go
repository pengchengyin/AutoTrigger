//go:build windows

package ui

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

type logScrollState struct{ follow, tracking bool }
type scrollInfo struct {
	Size, Mask uint32
	Min, Max   int32
	Page       uint32
	Pos, Track int32
}

func (f *form) syncTaskName() {
	if f.page != pageNewTask {
		return
	}
	input := strings.TrimSpace(windowText(f.ctrls[idInput]))
	current := strings.TrimSpace(windowText(f.ctrls[idTaskName]))
	if current != "" && current != f.autoTaskName {
		return
	}
	name := ""
	if input != "" {
		name = filepath.Base(filepath.Clean(input))
	}
	f.autoTaskName = name
	setWindowTextRaw(f.ctrls[idTaskName], name)
}

func editAtBottom(h uintptr) bool {
	s := scrollInfo{Mask: 0x17}
	s.Size = uint32(unsafe.Sizeof(s))
	ok, _, _ := user32.NewProc("GetScrollInfo").Call(h, 1, uintptr(unsafe.Pointer(&s)))
	return ok == 0 || int64(s.Pos)+int64(s.Page) >= int64(s.Max)+1
}

func (f *form) installLiveViews() {
	f.logScroll = map[uintptr]*logScrollState{}
	f.logSubclass = syscall.NewCallback(func(h uintptr, msg uint32, w uintptr, l unsafe.Pointer, id, ref uintptr) uintptr {
		state := f.logScroll[h]
		if state != nil && msg == 0x115 && w&0xffff == 5 {
			state.tracking = true
		}
		result, _, _ := syscallComctl.NewProc("DefSubclassProc").Call(h, uintptr(msg), w, uintptr(l))
		if state != nil {
			if msg == 0x115 || msg == 0x20a || msg == 0x101 {
				state.follow = editAtBottom(h)
				if msg == 0x115 && (w&0xffff == 8 || w&0xffff == 4) {
					state.tracking = false
				}
			}
			if msg == 0x202 || msg == 0xa2 {
				state.tracking = false
				state.follow = editAtBottom(h)
			}
		}
		if msg == 0x82 {
			syscallComctl.NewProc("RemoveWindowSubclass").Call(h, f.logSubclass, 1)
			delete(f.logScroll, h)
		}
		return result
	})
	for _, id := range []int{idLog, idResultView} {
		h := f.ctrls[id]
		f.logScroll[h] = &logScrollState{follow: true}
		syscallComctl.NewProc("SetWindowSubclass").Call(h, f.logSubclass, 1, 0)
	}
	user32.NewProc("SetTimer").Call(f.hwnd, 44, 500, 0)
}

func (f *form) canRefreshLog(h uintptr) bool {
	capture, _, _ := user32.NewProc("GetCapture").Call()
	var start, end uint32
	send(h, 0xb0, uintptr(unsafe.Pointer(&start)), uintptr(unsafe.Pointer(&end)))
	state := f.logScroll[h]
	return capture != h && start == end && (state == nil || state.follow && !state.tracking)
}

func (f *form) updateLogView(h uintptr, text string) {
	if !f.canRefreshLog(h) || windowText(h) == text {
		return
	}
	send(h, 0xb, 0, 0)
	setWindowTextRaw(h, text)
	send(h, emSetSel, ^uintptr(0), ^uintptr(0))
	send(h, emScrollCaret, 0, 0)
	send(h, 0xb, 1, 0)
	send(h, 0x115, 7, 0)
	procInvalidateRect.Call(h, 0, 1)
}

func (f *form) updateLiveViews() {
	switch f.page {
	case pageLogs:
		f.updateLogView(f.ctrls[idResultView], f.filteredLogText())
	case pageNewTask:
		f.updateLogView(f.ctrls[idLog], toCRLF(f.logTextSnapshot()))
	case pageTaskDetail:
		if _, ok := f.resultSnapshot(); ok {
			f.updateLogView(f.ctrls[idResultView], f.resultViewText())
		}
		if f.isRunning() {
			procInvalidateRect.Call(f.hwnd, 0, 0)
		}
	}
}
