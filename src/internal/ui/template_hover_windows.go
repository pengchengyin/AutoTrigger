//go:build windows

package ui

import (
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/windows"
	"context"
	"fmt"
	"image"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	captureFinish = 601 + iota
	captureCancel
	captureUndo
	captureManual
	captureList
	captureStatus
	captureMouse    = 0x8000 + 71
	captureKey      = 0x8000 + 72
	captureHookStop = 0x8000 + 73
)

type hoverResult struct {
	point  image.Point
	target windows.PickTarget
	err    error
}
type captureWork struct {
	item draftCapture
	err  error
	kind string
	shot image.Image
}
type pendingCapture struct {
	point    image.Point
	target   windows.PickTarget
	manual   bool
	region   image.Rectangle
	observed time.Time
	queued   bool
}
type hoverCapture struct {
	pane                                  *templateMakerPane
	hwnd, highlight, mouseHook, keyHook   uintptr
	controls                              map[int]uintptr
	session                               *captureSession
	ctx                                   context.Context
	cancel                                context.CancelFunc
	results                               chan hoverResult
	work                                  chan captureWork
	picking, busy, closing, cancelSession bool
	target                                windows.PickTarget
	targetAt                              time.Time
	lastRequest                           time.Time
	manual, dragging                      bool
	anchor                                image.Point
	region                                image.Rectangle
	frame                                 image.Rectangle
	lastError                             string
	lastNotice                            string
	count                                 int
	hookThread                            uint32
	paused                                bool
	queue                                 []pendingCapture
	snapshotting                          atomic.Bool
	annotation                            uintptr
}

var hoverOnce sync.Once
var hoverClass = utf16Ptr("AutoTriggerHoverCapture")
var highlightClass = utf16Ptr("AutoTriggerCaptureHighlight")
var activeHover *hoverCapture
var hoverPanes = map[uintptr]*hoverCapture{}
var mouseCaptureCallback, keyCaptureCallback uintptr

type captureHookInput struct {
	hwnd       uintptr
	thread     uint32
	buttons    uint32 // Accessed only by the dedicated hook thread.
	closing    atomic.Bool
	annotation atomic.Uintptr
}

var hookInput atomic.Pointer[captureHookInput]

type hookReady struct {
	thread     uint32
	mouse, key uintptr
	err        error
}

func startCaptureHooks(hwnd, instance uintptr) hookReady {
	ready := make(chan hookReady, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		thread, _, _ := kernel32.NewProc("GetCurrentThreadId").Call()
		var m msgT
		// Create the thread message queue before advertising its ID.
		user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 0)
		mouse, _, err := user32.NewProc("SetWindowsHookExW").Call(14, mouseCaptureCallback, instance, 0)
		var key uintptr
		if mouse != 0 {
			key, _, err = user32.NewProc("SetWindowsHookExW").Call(13, keyCaptureCallback, instance, 0)
		}
		if mouse == 0 || key == 0 {
			if mouse != 0 {
				user32.NewProc("UnhookWindowsHookEx").Call(mouse)
			}
			ready <- hookReady{err: err}
			return
		}
		input := &captureHookInput{hwnd: hwnd, thread: uint32(thread)}
		hookInput.Store(input)
		ready <- hookReady{uint32(thread), mouse, key, nil}
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			if m.Message == captureHookStop {
				input.closing.Store(true)
				if key != 0 {
					user32.NewProc("UnhookWindowsHookEx").Call(key)
					key = 0
				}
				if input.buttons == 0 {
					break
				}
				continue
			}
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		hookInput.CompareAndSwap(input, nil)
		user32.NewProc("UnhookWindowsHookEx").Call(mouse)
		if key != 0 {
			user32.NewProc("UnhookWindowsHookEx").Call(key)
		}
	}()
	return <-ready
}

func cursorPoint() image.Point {
	var point struct{ X, Y int32 }
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
	return image.Pt(int(point.X), int(point.Y))
}

func (p *templateMakerPane) startHoverCapture() {
	if p.owner.isRunning() {
		p.status("请先停止正在运行的任务，或等待任务结束后捕获")
		return
	}
	if p.capturing || activeHover != nil || hookInput.Load() != nil {
		p.status("已有捕获会话，请先结束")
		return
	}
	session, err := newCaptureSession(p.owner.templateRoot(), p.project)
	if err != nil {
		p.status("无法开始捕获：" + err.Error())
		return
	}
	hoverOnce.Do(func() {
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
		for _, v := range []struct {
			name *uint16
			proc uintptr
		}{{hoverClass, syscall.NewCallback(hoverCaptureProc)}, {highlightClass, syscall.NewCallback(captureHighlightProc)}} {
			wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: v.proc, Instance: p.owner.instance, Cursor: cursor, ClassName: v.name}
			procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		}
		mouseCaptureCallback = syscall.NewCallback(captureMouseHook)
		keyCaptureCallback = syscall.NewCallback(captureKeyboardHook)
	})
	ctx, cancel := context.WithCancel(context.Background())
	s := &hoverCapture{pane: p, session: session, ctx: ctx, cancel: cancel, results: make(chan hoverResult, 1), work: make(chan captureWork, 1), controls: map[int]uintptr{}}
	h, _, _ := procCreateWindowExW.Call(8|0x80, uintptr(unsafe.Pointer(hoverClass)), uintptr(unsafe.Pointer(utf16Ptr("连续捕获"))), 0x80000000|0xc00000|0x80000, uintptr(p.owner.px(20)), uintptr(p.owner.px(20)), uintptr(p.owner.px(620)), uintptr(p.owner.px(260)), 0, 0, p.owner.instance, 0)
	if h == 0 {
		cancel()
		p.status("无法创建捕获工具栏")
		return
	}
	s.hwnd = h
	hoverPanes[h] = s
	add := func(id int, class, text string, style uint32, x, y, w, height int) {
		c, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(wsChild|wsVisible|style), uintptr(p.owner.px(x)), uintptr(p.owner.px(y)), uintptr(p.owner.px(w)), uintptr(p.owner.px(height)), h, uintptr(id), p.owner.instance, 0)
		send(c, wmSetFont, p.owner.font, 1)
		s.controls[id] = c
	}
	for i, v := range []struct {
		id   int
		text string
	}{{captureFinish, "结束捕获"}, {captureUndo, "撤销上一项"}, {captureManual, "手动框选"}, {captureCancel, "取消本次"}} {
		add(v.id, "BUTTON", v.text, wsTabStop|0xB, 12+i*148, 12, 140, 30)
	}
	add(captureStatus, "STATIC", "正在识别 · 0 项", 0, 12, 50, 592, 48)
	add(captureList, "LISTBOX", "", wsBorder|wsVScroll|wsTabStop, 12, 102, 592, 108)
	s.highlight, _, _ = procCreateWindowExW.Call(8|0x80|0x20|0x80000|0x8000000, uintptr(unsafe.Pointer(highlightClass)), 0, 0x80000000, 0, 0, 1, 1, 0, 0, p.owner.instance, 0)
	if s.highlight == 0 {
		procDestroyWindow.Call(h)
		p.status("无法创建目标高亮层")
		return
	}
	ok, _, _ := user32.NewProc("SetLayeredWindowAttributes").Call(s.highlight, 0, 255, 1)
	if ok == 0 {
		procDestroyWindow.Call(s.highlight)
		procDestroyWindow.Call(h)
		p.status("无法设置透明高亮层")
		return
	}
	activeHover = s
	hooks := startCaptureHooks(h, p.owner.instance)
	s.mouseHook, s.keyHook, s.hookThread = hooks.mouse, hooks.key, hooks.thread
	err = hooks.err
	if s.mouseHook == 0 || s.keyHook == 0 {
		s.detach()
		procDestroyWindow.Call(h)
		p.status("无法安全拦截输入，捕获未启动：" + err.Error())
		return
	}
	p.capturing = true
	procShowWindow.Call(p.hwnd, swHide)
	procShowWindow.Call(p.owner.hwnd, swHide)
	procShowWindow.Call(h, 4) // Do not steal focus from the observed window.
	user32.NewProc("SetTimer").Call(h, 1, 50, 0)
	s.notice("移动鼠标选择窗口，点击后标记；可切换手动框选")
}

func (s *hoverCapture) notice(text string) {
	if s.lastError != "" && !strings.Contains(text, s.lastError) {
		text += " · 上次操作失败：" + s.lastError
	}
	message := fmt.Sprintf("本次 %d 项 · %s", s.count, text)
	if s.lastNotice != message {
		s.lastNotice = message
		setWindowTextRaw(s.controls[captureStatus], message)
	}
}

func captureToolbarAt(hwnd uintptr, p image.Point) bool {
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
	if visible == 0 {
		return false
	}
	var r rectT
	procGetWindowRect := user32.NewProc("GetWindowRect")
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return p.In(image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom)))
}
func (s *hoverCapture) toolbarAt(p image.Point) bool { return captureToolbarAt(s.hwnd, p) }

func captureMouseHook(code, w uintptr, l unsafe.Pointer) uintptr {
	s := hookInput.Load()
	if int32(code) >= 0 && s != nil {
		point := (*struct {
			X, Y int32
			Data uint32
		})(l)
		p := image.Pt(int(point.X), int(point.Y))
		var bit uint32
		var down bool
		switch w {
		case 0x201:
			bit, down = 1, true
		case 0x202:
			bit = 1
		case 0x204:
			bit, down = 2, true
		case 0x205:
			bit = 2
		case 0x207:
			bit, down = 4, true
		case 0x208:
			bit = 4
		case 0x20b, 0x20c:
			bit = 8
			if point.Data>>16 == 2 {
				bit = 16
			}
			down = w == 0x20b
		}
		if bit != 0 && !down && s.buttons&bit != 0 {
			s.buttons &^= bit
			if s.closing.Load() {
				if s.buttons == 0 {
					user32.NewProc("PostThreadMessageW").Call(uintptr(s.thread), 0x12, 0, 0)
				}
			} else if bit == 1 {
				packed := uintptr(uint64(uint32(p.X)) | uint64(uint32(p.Y))<<32)
				procPostMessageW.Call(s.hwnd, captureMouse, w, packed)
			}
			return 1
		}
		allowed := s.hwnd
		if annotation := s.annotation.Load(); annotation != 0 {
			allowed = annotation
		}
		if bit != 0 && down && !s.closing.Load() && !captureToolbarAt(allowed, p) {
			s.buttons |= bit
			if bit == 1 {
				packed := uintptr(uint64(uint32(p.X)) | uint64(uint32(p.Y))<<32)
				procPostMessageW.Call(s.hwnd, captureMouse, w, packed)
			}
			return 1
		}
	}
	r, _, _ := user32.NewProc("CallNextHookEx").Call(0, code, w, uintptr(l))
	return r
}

func captureKeyboardHook(code, w uintptr, l unsafe.Pointer) uintptr {
	s := hookInput.Load()
	if int32(code) >= 0 && s != nil && s.annotation.Load() != 0 && *(*uint32)(l) == 27 {
		if w == 0x100 || w == 0x104 {
			procPostMessageW.Call(s.annotation.Load(), 0x100, 27, 0)
		}
		return 1
	}
	if int32(code) >= 0 && s != nil && !s.closing.Load() && s.annotation.Load() == 0 {
		key := *(*uint32)(l)
		ctrl, _, _ := user32.NewProc("GetAsyncKeyState").Call(17)
		if key == 27 || key == 13 || key == 119 || key == 90 && ctrl&0x8000 != 0 {
			if w == 0x100 || w == 0x104 {
				procPostMessageW.Call(s.hwnd, captureKey, uintptr(key), 0)
			}
			return 1
		}
	}
	r, _, _ := user32.NewProc("CallNextHookEx").Call(0, code, w, uintptr(l))
	return r
}

func (s *hoverCapture) detach() {
	s.paused = true
	input := hookInput.Load()
	if input != nil && input.hwnd == s.hwnd {
		input.closing.Store(true)
	}
	if activeHover == s {
		activeHover = nil
	}
	s.mouseHook, s.keyHook = 0, 0
	if s.hookThread != 0 {
		user32.NewProc("PostThreadMessageW").Call(uintptr(s.hookThread), captureHookStop, 0, 0)
		s.hookThread = 0
	}
	if s.highlight != 0 {
		procShowWindow.Call(s.highlight, swHide)
	}
	s.cancel()
}

func (s *hoverCapture) setFrame(region image.Rectangle) {
	if s.frame == region {
		return
	}
	s.frame = region
	if region.Empty() {
		procShowWindow.Call(s.highlight, swHide)
		return
	}
	// Outside the target pixels; neither the frame nor its client area intercepts input.
	r := region.Inset(-3)
	procMoveWindow.Call(s.highlight, uintptr(int64(r.Min.X)), uintptr(int64(r.Min.Y)), uintptr(r.Dx()), uintptr(r.Dy()), 0)
	// A hollow window region also excludes the centre from cross-thread hit tests;
	// relying only on colour-key transparency is insufficient for WindowFromPoint.
	outer, _, _ := gdi32.NewProc("CreateRectRgn").Call(0, 0, uintptr(r.Dx()), uintptr(r.Dy()))
	inner, _, _ := gdi32.NewProc("CreateRectRgn").Call(3, 3, uintptr(r.Dx()-3), uintptr(r.Dy()-3))
	validRegion := false
	if outer != 0 && inner != 0 {
		combined, _, _ := gdi32.NewProc("CombineRgn").Call(outer, outer, inner, 4)
		if combined != 0 {
			ok, _, _ := user32.NewProc("SetWindowRgn").Call(s.highlight, outer, 1)
			if ok != 0 {
				outer = 0
				validRegion = true
			} // Window owns the region on success.
		}
	}
	if inner != 0 {
		procDeleteObject.Call(inner)
	}
	if outer != 0 {
		procDeleteObject.Call(outer)
	}
	if !validRegion {
		s.lastError = "无法创建可靠的高亮区域，捕获已停止"
		s.close(false)
		return
	}
	procInvalidateRect.Call(s.highlight, 0, 0)
	if !s.snapshotting.Load() {
		procShowWindow.Call(s.highlight, 4)
	}
}

func captureHighlightProc(h uintptr, msg uint32, w uintptr, l unsafe.Pointer) uintptr {
	if msg == wmEraseBkgnd {
		return 1
	}
	if msg == 0x84 {
		return ^uintptr(0)
	} // HTTRANSPARENT
	if msg == wmPaint {
		var ps paintStruct
		dc, _, _ := procBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		var r rectT
		procGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
		fillSolid(dc, r, 0)
		for i := int32(0); i < 3; i++ {
			outline := rectT{Left: i, Top: i, Right: r.Right - i, Bottom: r.Bottom - i}
			brush, _, _ := procCreateSolidBrush.Call(modernTheme.primary)
			user32.NewProc("FrameRect").Call(dc, uintptr(unsafe.Pointer(&outline)), brush)
			procDeleteObject.Call(brush)
		}
		procEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(h, uintptr(msg), w, uintptr(l))
	return r
}

func (s *hoverCapture) tick() {
	select {
	case result := <-s.results:
		s.picking = false
		point := cursorPoint()
		current, e := windows.WindowAtPoint(point.X, point.Y)
		if !s.closing && e == nil && result.err == nil && current.Handle == result.target.Window.Handle && result.target.CaptureError != "" {
			s.lastError = result.target.CaptureError
			s.close(false)
			return
		}
		if !s.closing && !s.manual && !s.toolbarAt(point) && e == nil && result.err == nil && current.Handle == result.target.Window.Handle && current.PID == result.target.Window.PID && current.Bounds == result.target.Window.Bounds && result.target.Window.PID != os.Getpid() {
			s.target = result.target
			s.targetAt = time.Now()
			if result.target.Control != nil && result.target.Control.Bounds.Contains(point.X, point.Y) {
				c := result.target.Control
				s.setFrame(image.Rect(c.Bounds.Left, c.Bounds.Top, c.Bounds.Right, c.Bounds.Bottom))
				role := map[string]string{"button": "按钮", "edit": "输入框", "checkbox": "复选框", "radio": "单选项", "combobox": "下拉框", "text": "文本"}[c.Role]
				if role == "" {
					role = "控件"
				}
				name := c.Text
				if name == "" {
					name = "未提供名称"
				}
				s.notice(fmt.Sprintf("UIA · %s · %s", role, name))
			} else {
				s.target.Control = nil
				s.setFrame(image.Rectangle{})
				s.notice("未识别到可靠控件，可手动框选")
			}
		} else if !s.manual {
			s.target = windows.PickTarget{}
			s.setFrame(image.Rectangle{})
			if result.err != nil {
				s.notice(result.err.Error() + "；可手动框选")
			}
		}
	default:
	}
	select {
	case work := <-s.work:
		s.busy = false
		if work.kind == "preview" && work.err != nil {
			procShowWindow.Call(s.hwnd, 4)
		}
		if work.kind == "save" || work.err != nil && work.kind == "preview" {
			s.paused = false
		}
		if work.kind == "preview" && work.err == nil && !s.closing {
			if err := s.openAnnotation(work.item, work.shot); err != nil {
				procShowWindow.Call(s.hwnd, 4)
				s.paused = false
				s.lastError = err.Error()
				s.notice(err.Error())
			}
			return
		}
		s.count = len(s.session.ids)
		if !s.paused && !s.frame.Empty() {
			procShowWindow.Call(s.highlight, 4)
		}
		if work.err != nil {
			s.lastError = work.err.Error()
			s.notice("操作失败：" + work.err.Error())
			if work.kind == "finish" {
				s.closing = false
				return
			}
		} else if work.kind == "finish" {
			procDestroyWindow.Call(s.hwnd)
			return
		} else {
			s.lastError = ""
			s.refreshList()
			s.pane.refreshCaptureRecords()
			s.notice("已保存至草稿")
		}
	default:
	}
	if !s.busy && len(s.queue) > 0 && (!s.closing || !s.cancelSession) {
		choice := s.queue[0]
		s.queue = s.queue[1:]
		s.captureChoice(choice)
		return
	}
	if s.closing {
		if !s.busy {
			s.finalize()
		}
		return
	}
	if s.paused {
		return
	}
	if s.manual {
		if s.dragging {
			s.region = image.Rectangle{Min: s.anchor, Max: cursorPoint()}.Canon()
			s.setFrame(s.region)
		}
		return
	}
	point := cursorPoint()
	if s.toolbarAt(point) {
		return
	}
	currentWindow, windowErr := windows.WindowAtPoint(point.X, point.Y)
	if windowErr != nil || currentWindow.PID == os.Getpid() {
		s.target = windows.PickTarget{}
		s.setFrame(image.Rectangle{})
		return
	}
	s.target = windows.PickTarget{Window: currentWindow}
	s.targetAt = time.Now()
	b := windows.CaptureWindowBounds(currentWindow)
	s.setFrame(image.Rect(b.Left, b.Top, b.Right, b.Bottom))
	s.notice("选择窗口 · " + currentWindow.Title)
}

func (s *hoverCapture) choose(point image.Point) {
	if s.closing || s.paused {
		return
	}
	choice := pendingCapture{point: point, target: s.target, manual: s.manual, region: s.region, observed: s.targetAt}
	if s.busy {
		if len(s.queue) >= 8 {
			s.lastError = "待保存捕获已达 8 项，本次未入队，请稍候重试"
			s.notice(s.lastError)
			return
		}
		choice.queued = true
		s.queue = append(s.queue, choice)
		s.notice(fmt.Sprintf("已加入待保存队列 · %d 项", len(s.queue)))
		return
	}
	s.captureChoice(choice)
}

func (s *hoverCapture) captureChoice(choice pendingCapture) {
	point, target := choice.point, choice.target
	current, err := windows.WindowAtPoint(point.X, point.Y)
	if err != nil || current.PID == os.Getpid() {
		s.notice("目标窗口无效")
		return
	}
	if err = windows.CapturePermission(current.PID); err != nil {
		s.lastError = err.Error()
		s.close(false)
		return
	}
	item := draftCapture{Window: current, Rule: captureRule{Method: "image", MinSimilarity: 0.95}}
	if choice.manual {
		region := choice.region.Intersect(image.Rect(current.Bounds.Left, current.Bounds.Top, current.Bounds.Right, current.Bounds.Bottom))
		if region.Empty() || region != choice.region {
			s.notice("框选区域须在同一目标窗口内")
			return
		}
		item.Bounds = ports.Rect{Left: region.Min.X, Top: region.Min.Y, Right: region.Max.X, Bottom: region.Max.Y}
	} else {
		if !choice.queued && time.Since(choice.observed) > 1500*time.Millisecond || target.Window.Handle != current.Handle || target.Window.PID != current.PID || target.Window.Bounds != current.Bounds {
			s.notice("目标已变化或尚未识别，请稍候或手动框选")
			return
		}
		item.Name = current.Title
		item.Bounds = windows.CaptureWindowBounds(current)
	}
	if item.Bounds.Width() < 1 || item.Bounds.Height() < 1 || item.Bounds.Width() > 10000 || item.Bounds.Height() > 10000 || int64(item.Bounds.Width())*int64(item.Bounds.Height()) > 25000000 {
		s.lastError = "捕获区域为空或过大"
		s.notice(s.lastError)
		return
	}
	s.busy = true
	s.paused = true
	if !choice.queued {
		s.lastError = ""
	}
	s.notice("正在保存截图和控件信息")
	// Hide our surfaces and raise only z-order; never move, resize, activate or click the installer.
	procShowWindow.Call(s.highlight, swHide)
	procShowWindow.Call(s.hwnd, swHide)
	user32.NewProc("SetWindowPos").Call(current.Handle, 0, 0, 0, 0, 0, 0x13)
	// Frames are outside the crop. Capture is performed off-thread so hooks keep pumping.
	go func() {
		check, e := windows.WindowAtPoint(point.X, point.Y)
		if e == nil && (check.Handle != current.Handle || check.Bounds != current.Bounds || check.PID != current.PID) {
			e = fmt.Errorf("截图前目标窗口已变化，请重新捕获（位置 %v，原窗口 %d/%d %v，现窗口 %d/%d %v）", point, current.Handle, current.PID, current.Bounds, check.Handle, check.PID, check.Bounds)
		}
		if e == nil && item.Rule.Method == "uia" {
			fresh, pe := (windows.GUI{}).PickAtPoint(context.Background(), point.X, point.Y)
			if pe != nil {
				e = pe
			} else if fresh.Control == nil || fresh.Window.Handle != current.Handle || fresh.Window.PID != current.PID || fresh.Window.Bounds != current.Bounds || fresh.Control.Bounds != item.Bounds || fresh.Control.ID != item.Control.ID || fresh.Control.Text != item.Control.Text || fresh.Control.Enabled != item.Control.Enabled || fresh.ControlType != item.Rule.ControlType || fresh.ClassName != item.Rule.ClassName {
				e = fmt.Errorf("UIA 控件在点击后已变化，请重新捕获")
			}
		}
		var preview image.Image
		if e == nil {
			s.snapshotting.Store(true)
			procShowWindow.Call(s.highlight, swHide)
			time.Sleep(20 * time.Millisecond)
			var shot image.Image
			se := windows.CaptureRegionVisible(current, item.Bounds)
			if se == nil {
				shot, se = (windows.GUI{}).RawScreenshot(ports.Window{Bounds: item.Bounds})
			}
			if se == nil {
				se = windows.CaptureRegionVisible(current, item.Bounds)
			}
			s.snapshotting.Store(false)
			if se != nil {
				e = se
			} else {
				after, ae := windows.WindowAtPoint(point.X, point.Y)
				if ae != nil || after.Handle != current.Handle || after.PID != current.PID || after.Bounds != current.Bounds {
					e = fmt.Errorf("截图过程中目标窗口已变化，本次未保存")
				} else {
					preview = shot
				}
			}
		}
		s.work <- captureWork{item: item, err: e, kind: "preview", shot: preview}
	}()
}

func (s *hoverCapture) refreshList() {
	send(s.controls[captureList], 0x184, 0, 0)
	index, err := readCaptureIndex(projectDir(s.pane.owner.templateRoot(), s.pane.project), s.pane.project.ID)
	if err != nil {
		s.notice(err.Error())
		return
	}
	for _, item := range index.Items {
		if item.SessionID == s.session.id {
			label := item.Rule.Method + " · " + item.Name
			send(s.controls[captureList], 0x180, 0, uintptr(unsafe.Pointer(utf16Ptr(label))))
		}
	}
}

func (s *hoverCapture) undo() {
	if s.busy || s.closing {
		s.notice("请等待当前保存结束")
		return
	}
	s.busy = true
	go func() { s.work <- captureWork{err: s.session.undo(), kind: "undo"} }()
}

func (s *hoverCapture) toggleManual() {
	s.manual = !s.manual
	s.dragging = false
	s.setFrame(image.Rectangle{})
	if s.manual {
		setWindowTextRaw(s.controls[captureManual], "自动选窗")
		s.notice("拖动框选窗口区域，松开后标记")
	} else {
		setWindowTextRaw(s.controls[captureManual], "手动框选")
		s.notice("移动鼠标选择窗口，点击后标记")
	}
}
func (s *hoverCapture) close(cancel bool) {
	if s.closing {
		return
	}
	if a := annotationPanes[s.annotation]; a != nil {
		a.close()
	}
	s.closing = true
	s.cancelSession = cancel
	if cancel {
		s.queue = nil
	}
	s.detach()
	s.notice("正在结束捕获")
	if !s.busy && len(s.queue) == 0 {
		s.finalize()
	}
}
func (s *hoverCapture) finalize() {
	s.busy = true
	go func() { s.work <- captureWork{err: s.session.finish(s.cancelSession), kind: "finish"} }()
}

func hoverCaptureProc(h uintptr, msg uint32, w uintptr, l unsafe.Pointer) uintptr {
	s := hoverPanes[h]
	if s != nil {
		switch msg {
		case wmEraseBkgnd:
			return 1
		case wmPaint:
			var ps paintStruct
			dc, _, _ := procBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			var r rectT
			procGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
			fillSolid(dc, r, modernTheme.surface)
			procEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
			return 0
		case 0x2b:
			if l != nil {
				item := *(*buttonDrawItem)(l)
				item.ID = 0
				s.pane.owner.paintNativeButton(&item)
			}
			return 1
		case 0x113:
			s.tick()
			return 0
		case captureMouse:
			packed := uint64(uintptr(l))
			point := image.Pt(int(int32(uint32(packed))), int(int32(uint32(packed>>32))))
			if w == 0x201 {
				if s.manual {
					s.anchor = point
					s.dragging = true
					s.region = image.Rectangle{}
				}
			} else if w == 0x202 {
				if s.manual && s.dragging {
					s.dragging = false
					s.region = image.Rectangle{Min: s.anchor, Max: point}.Canon()
					s.setFrame(s.region)
					s.choose(point.Sub(image.Pt(1, 1)))
				} else if !s.manual {
					s.choose(point)
				}
			}
			return 0
		case captureKey:
			switch w {
			case 27, 13:
				s.close(false)
			case 90:
				s.undo()
			case 119:
				s.toggleManual()
			}
			return 0
		case 0x111:
			if int(w>>16) == 0 {
				switch int(w & 0xffff) {
				case captureFinish:
					s.close(false)
				case captureCancel:
					s.close(true)
				case captureUndo:
					s.undo()
				case captureManual:
					s.toggleManual()
				}
			}
			return 0
		case 0x10:
			s.close(false)
			return 0
		case 0x82:
			s.detach()
			user32.NewProc("KillTimer").Call(h, 1)
			if s.highlight != 0 {
				procDestroyWindow.Call(s.highlight)
			}
			delete(hoverPanes, h)
			s.pane.capturing = false
			s.pane.refreshCaptureRecords()
			procShowWindow.Call(s.pane.owner.hwnd, swShow)
			if !s.pane.hub {
				procShowWindow.Call(s.pane.hwnd, swShow)
			} else {
				s.pane.owner.refreshTemplateHub()
				s.pane.owner.layout()
			}
			if s.cancelSession {
				s.pane.status("已取消本次捕获，原有草稿和已发布版本未改变")
			} else {
				s.pane.status(fmt.Sprintf("捕获结束，本次保留 %d 项", s.count))
			}
			if s.lastError != "" {
				s.pane.status("捕获已停止：" + s.lastError)
			}
		}
	}
	r, _, _ := procDefWindowProcW.Call(h, uintptr(msg), w, uintptr(l))
	return r
}
