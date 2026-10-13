//go:build windows

package windows

import (
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/windows/winver"
	"autotrigger/internal/procutil"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type GUI struct{}
type nativeRect struct{ Left, Top, Right, Bottom int32 }

func (r nativeRect) rect() ports.Rect {
	return ports.Rect{Left: int(r.Left), Top: int(r.Top), Right: int(r.Right), Bottom: int(r.Bottom)}
}
func windowText(hwnd uintptr) string {
	buf := make([]uint16, 4096)
	user32.NewProc("GetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}
func (GUI) Windows(pids []int) ([]ports.Window, error) {
	set := map[int]bool{}
	for _, id := range pids {
		set[id] = true
	}
	out := []ports.Window{}
	visit := func(hwnd, lparam uintptr) uintptr {
		var id uint32
		user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&id)))
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
		if !set[int(id)] || visible == 0 {
			return 1
		}
		var r nativeRect
		user32.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&r)))
		if r.Right <= r.Left || r.Bottom <= r.Top {
			return 1
		}
		theme := "light"
		v := winver.Current()
		if v.Major == 6 && v.Minor == 1 {
			theme = "win7classic"
		}
		out = append(out, ports.Window{Handle: hwnd, PID: int(id), Title: windowText(hwnd), Bounds: r.rect(), DPI: winver.DPI(hwnd), Theme: theme})
		return 1
	}
	if err := enumerateWindows(0, false, visit); err != nil {
		return nil, err
	}
	return out, nil
}

type guid struct {
	A    uint32
	B, C uint16
	D    [8]byte
}
type comObject struct{ VT *[96]uintptr }

func comCall(obj uintptr, index int, args ...uintptr) error {
	if obj == 0 {
		return fmt.Errorf("nil COM object")
	}
	all := append([]uintptr{obj}, args...)
	o := (*comObject)(nativeAddress(obj))
	r, _, _ := syscall.SyscallN(o.VT[index], all...)
	if int32(r) < 0 {
		return fmt.Errorf("UIA HRESULT 0x%x", r)
	}
	return nil
}
func release(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}
func bstr(obj uintptr, index int) string {
	var p uintptr
	if comCall(obj, index, uintptr(unsafe.Pointer(&p))) != nil || p == 0 {
		return ""
	}
	defer syscall.NewLazyDLL("oleaut32.dll").NewProc("SysFreeString").Call(p)
	n, _, _ := syscall.NewLazyDLL("oleaut32.dll").NewProc("SysStringLen").Call(p)
	if n > 8192 {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(nativeAddress(p)), int(n)))
}

func (GUI) Controls(w ports.Window) ([]ports.Control, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	result, err := procutil.Run(ctx, Runner{}, ports.Command{Program: exe, Args: []string{"--internal-uia", strconv.FormatUint(uint64(w.Handle), 10)}})
	if err != nil {
		return win32Controls(w), nil
	}
	var controls []ports.Control
	if err = json.Unmarshal([]byte(result.Stdout), &controls); err != nil {
		return win32Controls(w), nil
	}
	if len(controls) == 0 {
		return win32Controls(w), nil
	}
	for _, native := range win32Controls(w) {
		if native.Role == "button" || native.Role == "checkbox" || native.Role == "radio" {
			controls = append(controls, native)
		}
	}
	for i := range controls {
		controls[i].Window = w
	}
	return controls, nil
}

// RawControls runs only inside a disposable helper process to bound hostile COM providers.
func (GUI) RawControls(w ports.Window) ([]ports.Control, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 0)
	if int32(hr) < 0 {
		return win32Controls(w), nil
	}
	defer ole32.NewProc("CoUninitialize").Call()
	clsid := guid{0xff48dba4, 0x60ef, 0x4201, [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
	iid := guid{0x30cbe57d, 0xd9d0, 0x452a, [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee}}
	var automation uintptr
	hr, _, _ = ole32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&automation)))
	if int32(hr) < 0 {
		return win32Controls(w), nil
	}
	defer release(automation)
	var element, condition, array uintptr
	if err := comCall(automation, 6, w.Handle, uintptr(unsafe.Pointer(&element))); err != nil {
		return win32Controls(w), nil
	}
	defer release(element)
	if err := comCall(automation, 21, uintptr(unsafe.Pointer(&condition))); err != nil {
		return win32Controls(w), nil
	}
	defer release(condition)
	if err := comCall(element, 6, 4, condition, uintptr(unsafe.Pointer(&array))); err != nil {
		return win32Controls(w), nil
	}
	defer release(array)
	var length int32
	comCall(array, 3, uintptr(unsafe.Pointer(&length)))
	if length > 2048 {
		length = 2048
	}
	out := []ports.Control{}
	for i := int32(0); i < length; i++ {
		var e uintptr
		if comCall(array, 4, uintptr(i), uintptr(unsafe.Pointer(&e))) != nil {
			continue
		}
		var kind, enabled, offscreen int32
		var rect nativeRect
		comCall(e, 21, uintptr(unsafe.Pointer(&kind)))
		comCall(e, 28, uintptr(unsafe.Pointer(&enabled)))
		comCall(e, 38, uintptr(unsafe.Pointer(&offscreen)))
		comCall(e, 43, uintptr(unsafe.Pointer(&rect)))
		text := bstr(e, 23)
		id := bstr(e, 29)
		checked := uiaChecked(e, kind)
		release(e)
		if offscreen != 0 || rect.Right <= rect.Left || rect.Bottom <= rect.Top {
			continue
		}
		role := "text"
		switch kind {
		case 50000:
			role = "button"
		case 50002:
			role = "checkbox"
		case 50004:
			role = "edit"
		case 50013:
			role = "radio"
		case 50012:
			role = "progress"
		}
		if id == "" {
			id = strconv.Itoa(int(i))
		}
		out = append(out, ports.Control{ID: id, Text: text, Role: role, Bounds: rect.rect(), Window: w, Source: "uia", Confidence: 1, Enabled: enabled != 0, Checked: checked})
	}
	if len(out) == 0 {
		return win32Controls(w), nil
	}
	return out, nil
}

func uiaChecked(element uintptr, kind int32) bool {
	var patternID uintptr
	var index int
	var iid guid
	switch kind {
	case 50002:
		patternID, index = 10015, 4 // TogglePattern.CurrentToggleState
		iid = guid{0x94cf8058, 0x9b8d, 0x4ab9, [8]byte{0x8b, 0xfd, 0x4c, 0xd0, 0xa3, 0x3c, 0x8c, 0x70}}
	case 50013:
		patternID, index = 10010, 6 // SelectionItemPattern.CurrentIsSelected
		iid = guid{0xa8efa66a, 0x0fda, 0x421a, [8]byte{0x91, 0x94, 0x38, 0x02, 0x1f, 0x35, 0x78, 0xea}}
	default:
		return false
	}
	var unknown, pattern uintptr
	if comCall(element, 16, patternID, uintptr(unsafe.Pointer(&unknown))) != nil || unknown == 0 {
		return false
	}
	defer release(unknown)
	if comCall(unknown, 0, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&pattern))) != nil || pattern == 0 {
		return false
	}
	defer release(pattern)
	var value int32
	return comCall(pattern, index, uintptr(unsafe.Pointer(&value))) == nil && value == 1
}

func win32Controls(w ports.Window) []ports.Control {
	out := []ports.Control{}
	visit := func(hwnd, lparam uintptr) uintptr {
		vis, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
		if vis == 0 {
			return 1
		}
		var rect nativeRect
		user32.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&rect)))
		enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(hwnd)
		classBuf := make([]uint16, 128)
		user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&classBuf[0])), 128)
		class := syscall.UTF16ToString(classBuf)
		role := "text"
		checked := false
		if class == "Button" || class == "TButton" || class == "TNewButton" || class == "TNewCheckBox" || class == "TNewRadioButton" {
			role = "button"
			style, _, _ := user32.NewProc("GetWindowLongW").Call(hwnd, ^uintptr(15))
			switch style & 0xf {
			case 2, 3, 5, 6:
				role = "checkbox"
				var r uintptr
				user32.NewProc("SendMessageTimeoutW").Call(hwnd, 0xf0, 0, 0, 2, 100, uintptr(unsafe.Pointer(&r)))
				checked = r == 1
			case 4, 9:
				role = "radio"
				var r uintptr
				user32.NewProc("SendMessageTimeoutW").Call(hwnd, 0xf0, 0, 0, 2, 100, uintptr(unsafe.Pointer(&r)))
				checked = r == 1
			}
		}
		if class == "Edit" {
			role = "edit"
		}
		out = append(out, ports.Control{ID: fmt.Sprintf("%x", hwnd), Text: windowText(hwnd), Role: role, Bounds: rect.rect(), Window: w, Source: "win32", Confidence: 1, Enabled: enabled != 0, Checked: checked, Native: hwnd})
		return 1
	}
	enumerateWindows(w.Handle, true, visit)
	return out
}

type bitmapInfo struct {
	Size        uint32
	Width       int32
	Height      int32
	Planes      uint16
	Bits        uint16
	Compression uint32
	SizeImage   uint32
	XPels       int32
	YPels       int32
	Used        uint32
	Important   uint32
}

func (GUI) Screenshot(w ports.Window) (image.Image, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	result, err := procutil.Run(ctx, Runner{}, ports.Command{Program: exe, Args: []string{"--internal-screenshot", strconv.FormatUint(uint64(w.Handle), 10)}})
	if err != nil {
		return nil, err
	}
	return png.Decode(strings.NewReader(result.Stdout))
}

func (GUI) RawScreenshot(w ports.Window) (image.Image, error) {
	width, height := w.Bounds.Width(), w.Bounds.Height()
	if width < 1 || height < 1 || width > 10000 || height > 10000 {
		return nil, fmt.Errorf("invalid screenshot dimensions %dx%d", width, height)
	}
	screen, _, _ := user32.NewProc("GetDC").Call(0)
	if screen == 0 {
		return nil, fmt.Errorf("desktop DC unavailable")
	}
	defer user32.NewProc("ReleaseDC").Call(0, screen)
	dc, _, _ := gdi32.NewProc("CreateCompatibleDC").Call(screen)
	if dc == 0 {
		return nil, fmt.Errorf("memory DC unavailable")
	}
	defer gdi32.NewProc("DeleteDC").Call(dc)
	bmp, _, _ := gdi32.NewProc("CreateCompatibleBitmap").Call(screen, uintptr(width), uintptr(height))
	if bmp == 0 {
		return nil, fmt.Errorf("bitmap allocation failed")
	}
	defer gdi32.NewProc("DeleteObject").Call(bmp)
	old, _, _ := gdi32.NewProc("SelectObject").Call(dc, bmp)
	defer gdi32.NewProc("SelectObject").Call(dc, old)
	ok, _, _ := user32.NewProc("PrintWindow").Call(w.Handle, dc, winver.Current().PrintFlags())
	if ok == 0 {
		ok, _, _ = gdi32.NewProc("BitBlt").Call(dc, 0, 0, uintptr(width), uintptr(height), screen, uintptr(w.Bounds.Left), uintptr(w.Bounds.Top), 0x40cc0020)
	}
	if ok == 0 {
		return nil, fmt.Errorf("window capture failed")
	}
	buf := make([]byte, width*height*4)
	info := bitmapInfo{Size: 40, Width: int32(width), Height: -int32(height), Planes: 1, Bits: 32}
	gdi32.NewProc("SelectObject").Call(dc, old)
	lines, _, _ := gdi32.NewProc("GetDIBits").Call(dc, bmp, 0, uintptr(height), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&info)), 0)
	if lines == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(buf); i += 4 {
		img.Pix[i] = buf[i+2]
		img.Pix[i+1] = buf[i+1]
		img.Pix[i+2] = buf[i]
		img.Pix[i+3] = 255
	}
	return img, nil
}

type mouseInput struct {
	Type   uint32
	Pad    uint32
	DX, DY int32
	Data   uint32
	Flags  uint32
	Time   uint32
	Pad2   uint32
	Extra  uintptr
}

func className(hwnd uintptr) string {
	buf := make([]uint16, 128)
	user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

// processIntegrityRID 返回进程令牌的完整性级别 RID（0x1000 低 / 0x2000 中 /
// 0x3000 高）；查询失败返回 0，表示未知、不做限制。
func processIntegrityRID(pid int) uint32 {
	const processQueryLimitedInformation = 0x1000
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	var tok syscall.Handle
	if e := callOK(advapi32.NewProc("OpenProcessToken"), uintptr(h), 0x0008, uintptr(unsafe.Pointer(&tok))); e != nil {
		return 0
	}
	defer syscall.CloseHandle(tok)
	var ret uint32
	buf := make([]byte, 64)
	if e := callOK(advapi32.NewProc("GetTokenInformation"), uintptr(tok), 25, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&ret))); e != nil {
		return 0
	}
	sid := *(*uintptr)(unsafe.Pointer(&buf[0]))
	if sid == 0 {
		return 0
	}
	base := uintptr(unsafe.Pointer(&buf[0]))
	if sid < base || sid-base > uintptr(len(buf)-8) {
		return 0
	}
	offset := int(sid - base)
	count := int(buf[offset+1])
	if count == 0 {
		return 0
	}
	end := offset + 8 + count*4
	if end > len(buf) {
		return 0
	}
	return binary.LittleEndian.Uint32(buf[end-4 : end])
}

func (GUI) Click(c ports.Control) error {
	if !c.Enabled || c.Bounds.Width() < 1 || c.Bounds.Height() < 1 {
		return fmt.Errorf("control disabled or empty")
	}
	x, y := (c.Bounds.Left+c.Bounds.Right)/2, (c.Bounds.Top+c.Bounds.Bottom)/2
	if !c.Window.Bounds.Contains(x, y) {
		return fmt.Errorf("click outside owned window")
	}
	valid, _, _ := user32.NewProc("IsWindow").Call(c.Window.Handle)
	if valid == 0 {
		return fmt.Errorf("window closed")
	}
	var owner uint32
	user32.NewProc("GetWindowThreadProcessId").Call(c.Window.Handle, uintptr(unsafe.Pointer(&owner)))
	if owner == 0 || int(owner) != c.Window.PID {
		return fmt.Errorf("sample window owner changed")
	}
	if own, target := processIntegrityRID(os.Getpid()), processIntegrityRID(int(owner)); target > own && own != 0 {
		return fmt.Errorf("窗口 %q 的权限高于本工具（目标 0x%x，本工具 0x%x），输入可能被 Windows UIPI 拦截；请以管理员身份运行本工具后重试", c.Window.Title, target, own)
	}
	var probe uintptr
	if e := callOK(user32.NewProc("SendMessageTimeoutW"), c.Window.Handle, 0, 0, 0, 2, 300, uintptr(unsafe.Pointer(&probe))); e != nil {
		return fmt.Errorf("窗口 %q 拒绝消息或无响应（%v），无法点击", c.Window.Title, e)
	}
	nativeButton := c.Native != 0 && (c.Role == "button" || c.Role == "checkbox" || c.Role == "radio")
	if nativeButton {
		child, _, _ := user32.NewProc("IsChild").Call(c.Window.Handle, c.Native)
		enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(c.Native)
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(c.Native)
		if child == 0 || enabled == 0 || visible == 0 {
			return fmt.Errorf("native button is not a visible enabled child of the sample window")
		}
	}
	// 首选真实鼠标点击。第三方安装器的按钮常是自定义窗口类（Inno 的 TNewButton、
	// 部分 skinned 控件），它们不实现 BM_CLICK：消息能送达却毫无反应，只会让界面
	// 停在原地。SendInput 注入系统级输入流，对标准控件与自定义控件都生效。
	mouseErr := mouseClick(c, x, y)
	if mouseErr == nil || !nativeButton {
		return mouseErr
	}
	// 回退 1：直接向控件发 BM_CLICK。仅标准 Button 类有效——自定义按钮类
	// （Inno 的 TNewButton 等）不实现 BM_CLICK，消息送达成功却毫无反应，
	// 当作点击成功会造成"向导原地卡死"的假象。
	if className(c.Native) == "Button" {
		var result uintptr
		if e := callOK(user32.NewProc("SendMessageTimeoutW"), c.Native, 0xf5, 0, 0, 2, 500, uintptr(unsafe.Pointer(&result))); e == nil {
			return nil
		}
	}
	return fmt.Errorf("窗口 %q 的按钮 %q（类 %s）无法点击：%v；若安装器以管理员权限运行而本工具未提权，Windows UIPI 会拒绝一切注入，请以管理员身份运行本工具", c.Window.Title, c.Text, className(c.Native), mouseErr)
}

// mouseClick 把目标窗口置于前台后在该控件中心注入真实鼠标点击。
func mouseClick(c ports.Control, x, y int) error {
	if !setForeground(c.Window.Handle) {
		fg, _, _ := user32.NewProc("GetForegroundWindow").Call()
		return fmt.Errorf("cannot focus sample window (foreground is %q)", windowText(fg))
	}
	if c.Native != 0 && c.Role == "checkbox" && c.Checked {
		return nil
	}
	user32.NewProc("SetCursorPos").Call(uintptr(x), uintptr(y))
	time.Sleep(30 * time.Millisecond) // 让光标先落位，避免个别控件把"移动+点击"当成拖拽
	foreground, _, _ := user32.NewProc("GetForegroundWindow").Call()
	var cursor struct{ X, Y int32 }
	ok, _, _ := user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&cursor)))
	if foreground != c.Window.Handle || ok == 0 || int(cursor.X) != x || int(cursor.Y) != y {
		return fmt.Errorf("focus or cursor changed before input; click cancelled")
	}
	inputs := [2]mouseInput{{Flags: 2}, {Flags: 4}}
	n, _, e := user32.NewProc("SendInput").Call(2, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	if n != 2 {
		return fmt.Errorf("SendInput: %w", e)
	}
	return nil
}

// setForeground does not inject keys into another application's input queue.
func setForeground(hwnd uintptr) bool {
	user32.NewProc("SetForegroundWindow").Call(hwnd)
	fg, _, _ := user32.NewProc("GetForegroundWindow").Call()
	return fg == hwnd
}
func (GUI) SetText(c ports.Control, text string) error {
	if c.Native == 0 || c.Role != "edit" {
		return ports.ErrUnsupported
	}
	var result uintptr
	return callOK(user32.NewProc("SendMessageTimeoutW"), c.Native, 0xc, 0, uintptr(unsafe.Pointer(utf(text))), 2, 500, uintptr(unsafe.Pointer(&result)))
}
func (GUI) CloseWindow(w ports.Window) error {
	return callOK(user32.NewProc("PostMessageW"), w.Handle, 0x10, 0, 0)
}
func (GUI) Close() error { return nil }

// InternalHelper serves read-only window capture requests before normal CLI startup.
func InternalHelper() bool {
	if len(os.Args) == 3 && os.Args[1] == "--internal-uia-point" {
		xy := strings.Split(os.Args[2], ",")
		if len(xy) != 2 {
			os.Exit(2)
		}
		x, ex := strconv.Atoi(xy[0])
		y, ey := strconv.Atoi(xy[1])
		if ex != nil || ey != nil {
			os.Exit(2)
		}
		winver.EnableDPI()
		target, err := (GUI{}).RawPickAtPoint(x, y)
		if err != nil {
			os.Exit(1)
		}
		if json.NewEncoder(os.Stdout).Encode(target) != nil {
			os.Exit(1)
		}
		return true
	}
	if len(os.Args) != 3 || (os.Args[1] != "--internal-uia" && os.Args[1] != "--internal-screenshot") {
		return false
	}
	handle, err := strconv.ParseUint(os.Args[2], 10, 64)
	if err != nil {
		os.Exit(2)
	}
	winver.EnableDPI()
	var rect nativeRect
	user32.NewProc("GetWindowRect").Call(uintptr(handle), uintptr(unsafe.Pointer(&rect)))
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(uintptr(handle), uintptr(unsafe.Pointer(&pid)))
	w := ports.Window{Handle: uintptr(handle), PID: int(pid), Title: windowText(uintptr(handle)), Bounds: rect.rect(), DPI: winver.DPI(uintptr(handle))}
	if os.Args[1] == "--internal-uia" {
		controls, err := (GUI{}).RawControls(w)
		if err != nil {
			os.Exit(1)
		}
		json.NewEncoder(os.Stdout).Encode(controls)
	} else {
		img, err := (GUI{}).RawScreenshot(w)
		if err != nil {
			os.Exit(1)
		}
		if err = png.Encode(os.Stdout, img); err != nil {
			os.Exit(1)
		}
	}
	return true
}
