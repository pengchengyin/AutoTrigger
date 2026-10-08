//go:build windows

package ui

import (
	"runtime"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

// Win32 绑定：只用标准库 syscall，不引入第三方依赖。
// 只声明本界面真正用到的过程与常量，保持可审计（PRD 4.5）。
//
// 平台前提：Windows 7 SP1 x64 及以上（4.6）。结构体布局按 **64 位** 定义，
// 由 win32_windows_test.go 断言尺寸与偏移，防止无意的字段增删悄悄改变布局
// （布局错位会让系统调用读到垃圾参数）。
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
)

var (
	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procDestroyWindow         = user32.NewProc("DestroyWindow")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procGetMessageW           = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessageW      = user32.NewProc("DispatchMessageW")
	procSendMessageW          = user32.NewProc("SendMessageW")
	procPostMessageW          = user32.NewProc("PostMessageW")
	procShowWindow            = user32.NewProc("ShowWindow")
	procUpdateWindow          = user32.NewProc("UpdateWindow")
	procSetWindowTextW        = user32.NewProc("SetWindowTextW")
	procGetWindowTextLengthW  = user32.NewProc("GetWindowTextLengthW")
	procEnableWindow          = user32.NewProc("EnableWindow")
	procMessageBoxW           = user32.NewProc("MessageBoxW")
	procSetFocus              = user32.NewProc("SetFocus")
	procGetClientRect         = user32.NewProc("GetClientRect")
	procMoveWindow            = user32.NewProc("MoveWindow")
	procLoadCursorW           = user32.NewProc("LoadCursorW")
	procGetSysColor           = user32.NewProc("GetSysColor")
	procGetSysColorBrush      = user32.NewProc("GetSysColorBrush")
	procSystemParametersInfo  = user32.NewProc("SystemParametersInfoW")
	procSetProcessDPIAware    = user32.NewProc("SetProcessDPIAware")
	procGetDC                 = user32.NewProc("GetDC")
	procReleaseDC             = user32.NewProc("ReleaseDC")
	procGetDeviceCaps         = gdi32.NewProc("GetDeviceCaps")
	procCreateFontIndirectW   = gdi32.NewProc("CreateFontIndirectW")
	procGetStockObject        = gdi32.NewProc("GetStockObject")
	procSetBkMode             = gdi32.NewProc("SetBkMode")
	procSetTextColor          = gdi32.NewProc("SetTextColor")
	procGetModuleHandleW      = kernel32.NewProc("GetModuleHandleW")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleMode        = kernel32.NewProc("GetConsoleMode")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procGetOpenFileNameW      = comdlg32.NewProc("GetOpenFileNameW")
	procGetSaveFileNameW      = comdlg32.NewProc("GetSaveFileNameW")
	procSHBrowseForFolderW    = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW  = shell32.NewProc("SHGetPathFromIDListW")
	procShellExecuteW         = shell32.NewProc("ShellExecuteW")
	procCoInitializeEx        = ole32.NewProc("CoInitializeEx")
	procCoTaskMemFree         = ole32.NewProc("CoTaskMemFree")
)

// 窗口类与窗口
const (
	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsTabStop          = 0x00010000
	wsGroup            = 0x00020000
	wsBorder           = 0x00800000
	wsVScroll          = 0x00200000
	wsExClientEdge     = 0x00000200

	csHREDRAW    = 0x0002
	csVREDRAW    = 0x0001
	cwUseDefault = 0x80000000

	swHide       = 0
	swShowNormal = 1
	swShow       = 5

	idcArrow       = 32512
	defaultGUIFont = 17 // GetStockObject(DEFAULT_GUI_FONT)

	colorWindow            = 5
	color3DFace            = 15
	colorBtnText           = 18
	logPixelsX             = 88
	spiGetNonClientMetrics = 0x0029

	transparentBk = 1

	mbOK           = 0x0000
	mbYesNo        = 0x0004
	mbIconError    = 0x0010
	mbIconQuestion = 0x0020
	mbIconInfo     = 0x0040
	idYes          = 6
)

// 控件样式
const (
	esAutoHScroll = 0x0080
	esAutoVScroll = 0x0040
	esMultiline   = 0x0004
	esReadOnly    = 0x0800
	esWantReturn  = 0x1000

	bsPushButton   = 0x00000000
	bsAutoCheckBox = 0x00000003

	ssLeftNoWordWrap = 0x0000000C
)

// 消息
const (
	wmGetText        = 0x000D
	wmGetTextLength  = 0x000E
	wmDestroy        = 0x0002
	wmSize           = 0x0005
	wmClose          = 0x0010
	wmGetMinMaxInfo  = 0x0024
	wmSetFont        = 0x0030
	wmCommand        = 0x0111
	wmCtlColorEdit   = 0x0133
	wmCtlColorBtn    = 0x0135
	wmCtlColorStatic = 0x0138
	wmCopy           = 0x0301

	bnClicked = 0
	enChange  = 1

	bmGetCheck    = 0x00F0
	bmSetCheck    = 0x00F1
	bstChecked    = 1
	emSetSel      = 0x00B1
	emReplaceSel  = 0x00C2
	emSetReadOnly = 0x00CF
	emSetLimit    = 0x00C5

	// WM_APP 起的自定义消息：仅用于"后台线程 → 界面线程"的单向通知，
	// 界面线程在窗口过程内取数据并刷新控件（跨线程不直接操作窗口）。
	wmAppLog  = 0x8000 + 1
	wmAppDone = 0x8000 + 2
)

// 文件对话框
const (
	ofnOverwritePrompt = 0x00000002
	ofnHideReadOnly    = 0x00000004
	ofnNoChangeDir     = 0x00000008
	ofnPathMustExist   = 0x00000800
	ofnFileMustExist   = 0x00001000
	ofnExplorer        = 0x00080000
	ofnNoDereference   = 0x00100000

	bifReturnOnlyFSDirs = 0x0001
	bifEditBox          = 0x0010
	bifNewDialogStyle   = 0x0040
	bifUseNewUI         = bifEditBox | bifNewDialogStyle

	coInitApartmentThreaded = 0x2
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  uintptr
}

type msgT struct {
	Hwnd    uintptr
	Message uint32
	_       uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
}

type rectT struct {
	Left, Top, Right, Bottom int32
}

type pointT struct {
	X, Y int32
}

type minMaxInfoT struct {
	Reserved     pointT
	MaxSize      pointT
	MaxPosition  pointT
	MinTrackSize pointT
	MaxTrackSize pointT
}

// logFontW 对应 Win32 LOGFONTW（64 位 92 字节）。
type logFontW struct {
	Height         int32
	Width          int32
	Escapement     int32
	Orientation    int32
	Weight         int32
	Italic         byte
	Underline      byte
	StrikeOut      byte
	CharSet        byte
	OutPrecision   byte
	ClipPrecision  byte
	Quality        byte
	PitchAndFamily byte
	FaceName       [32]uint16
}

// nonClientMetricsW 对应 Win32 NONCLIENTMETRICSW（Vista 及以上，64 位 504 字节）。
// 只用其中的 MessageFont —— 系统"界面字体"，随系统 DPI 给出正确字号，
// 比硬编码字号更贴近用户设置。
type nonClientMetricsW struct {
	Size              uint32
	BorderWidth       int32
	ScrollWidth       int32
	ScrollHeight      int32
	CaptionWidth      int32
	CaptionHeight     int32
	CaptionFont       logFontW
	SmCaptionWidth    int32
	SmCaptionHeight   int32
	SmCaptionFont     logFontW
	MenuWidth         int32
	MenuHeight        int32
	MenuFont          logFontW
	StatusFont        logFontW
	MessageFont       logFontW
	PaddedBorderWidth int32
}

// openFileNameW 对应 Win32 OPENFILENAMEW（64 位 152 字节）。
type openFileNameW struct {
	StructSize    uint32
	_             uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	_             uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	_             uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefaultExt    *uint16
	CustData      uintptr
	Hook          uintptr
	TemplateName  *uint16
	ReservedPtr   uintptr
	ReservedDword uint32
	FlagsEx       uint32
}

// browseInfoW 对应 Win32 BROWSEINFOW（64 位 64 字节）。
type browseInfoW struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	_           uint32
	Callback    uintptr
	Param       uintptr
	Image       int32
	_           uint32
}

// fileDialogFilter / saveDialogFilter 是文件对话框的过滤器描述。
// 以"描述, 模式, 描述, 模式, ..., 空串"给出，最终形成 Win32 要求的双 NUL 结尾串。
var fileDialogFilter = []string{
	"所有文件 (*.*)", "*.*",
	"可执行与安装包 (*.exe;*.msi;*.bat;*.cmd;*.ps1;*.lnk)", "*.exe;*.msi;*.bat;*.cmd;*.ps1;*.lnk",
	"压缩与光盘映像 (*.zip;*.7z;*.rar;*.iso)", "*.zip;*.7z;*.rar;*.iso",
	"文档 (*.doc;*.docx;*.xls;*.xlsx;*.pdf)", "*.doc;*.docx;*.xls;*.xlsx;*.pdf",
	"",
}

var saveDialogFilter = []string{
	"JSON 结果文件 (*.json)", "*.json",
	"所有文件 (*.*)", "*.*",
	"",
}

// utf16Ptr 返回字符串的 UTF-16 指针。
// 注意：syscall.UTF16PtrFromString 在字符串含 NUL 时会报错，因此含 NUL 的
// Win32 双 NUL 串必须走 dialogFilterSlice 之类的切片构造方式。
func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	}
	return p
}

// utf16Slice 返回不含结尾 NUL 的 UTF-16 切片，便于拼装多段字符串。
func utf16Slice(s string) []uint16 {
	if s == "" {
		return nil
	}
	out, err := syscall.UTF16FromString(s)
	if err != nil || len(out) == 0 {
		return nil
	}
	return out[:len(out)-1]
}

// dialogFilterSlice 把过滤器描述拼成 Win32 所需的以 NUL 分隔、双 NUL 结尾的切片。
func dialogFilterSlice(parts []string) []uint16 {
	out := make([]uint16, 0, 256)
	for _, p := range parts {
		out = append(out, utf16Slice(p)...)
		out = append(out, 0)
	}
	return out
}

func send(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func post(hwnd uintptr, msg uint32, wparam, lparam uintptr) {
	procPostMessageW.Call(hwnd, uintptr(msg), wparam, lparam)
}

// windowText 读取控件文本。
func windowText(hwnd uintptr) string {
	n := int(send(hwnd, wmGetTextLength, 0, 0))
	if n <= 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	send(hwnd, wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf)
}

// setWindowTextRaw 直接调用 SetWindowTextW（编辑控件也可用，等价 WM_SETTEXT）。
func setWindowTextRaw(hwnd uintptr, s string) {
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(s))))
}

func messageBox(owner uintptr, title, text string, flags uintptr) int {
	r, _, _ := procMessageBoxW.Call(owner,
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		flags)
	return int(r)
}

func enableWindow(hwnd uintptr, enabled bool) {
	v := uintptr(0)
	if enabled {
		v = 1
	}
	procEnableWindow.Call(hwnd, v)
}

// sysColorBrush 返回系统颜色画刷（静态控件与按钮的背景）。
func sysColorBrush(index uintptr) uintptr {
	r, _, _ := procGetSysColorBrush.Call(index)
	return r
}

// flushableLen 返回缓冲区中可以安全交给控件的前缀长度。
// 只截到完整 UTF-8 边界，避免把被写入操作切开的多字节字符渲染成乱码。
func flushableLen(b []byte) int {
	for len(b) > 0 {
		r, size := utf8.DecodeLastRune(b)
		if r == utf8.RuneError && size <= 1 {
			b = b[:len(b)-1] // 末尾是不完整序列，退回一个字节
			continue
		}
		break
	}
	return len(b)
}

// consoleProcessCount 返回连接到当前控制台的进程数；没有控制台时返回 0。
func consoleProcessCount() int {
	var pids [8]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return int(n)
}

// stdoutIsTerminal 判断标准输出是否连接着**真实控制台**。
// Go 的 os.Stdout.Stat() 在 Windows 上会把管道也标成 ModeCharDevice，不足以区分，
// 因此用 GetConsoleMode 判定。
func stdoutIsTerminal() bool {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(uintptr(syscall.Stdout), uintptr(unsafe.Pointer(&mode)))
	return r != 0
}

// LooksLikeDoubleClick 判断本次启动是否像"从资源管理器双击"，作为打开界面的依据。
//
// 需要同时满足两个条件，缺一不可：
//  1. 标准输出连着真实控制台 —— 脚本与流水线调用通常重定向输出；
//  2. 该控制台由本进程**独占**（系统刚刚分配）—— 而不是附加到用户已有的终端。
//
// 条件 2 很关键：在 cmd 里手敲 `AutoTrigger.exe` 会附加到已有控制台（计数 ≥2），
// 此时仍按命令行处理，避免脚本调用时突然弹出窗口；需要界面时用 `--gui`。
func LooksLikeDoubleClick() bool {
	if !stdoutIsTerminal() {
		return false
	}
	return consoleProcessCount() == 1
}

// hideConsole 隐藏由本进程独占的控制台窗口（双击启动时会先出现一个黑框）。
//
// 附加在用户已有终端上时**不做任何事** —— GetConsoleWindow 返回的是用户正在
// 使用的那扇窗口，隐藏它会让用户的终端凭空消失。采用隐藏而非 FreeConsole，
// 保留句柄以便异常时仍有输出通道。
func hideConsole() {
	if consoleProcessCount() != 1 {
		return
	}
	h, _, _ := procGetConsoleWindow.Call()
	if h != 0 {
		procShowWindow.Call(h, swHide)
	}
}

// dpiScale 返回系统 DPI 相对 96 的缩放系数，用于把布局像素换算成实际像素。
// 前提：已调用 SetProcessDPIAware（否则只会拿到 96）。
func dpiScale() float64 {
	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		return 1
	}
	defer procReleaseDC.Call(0, dc)
	dpi, _, _ := procGetDeviceCaps.Call(dc, logPixelsX)
	if dpi < 72 || dpi > 480 {
		return 1
	}
	return float64(dpi) / 96.0
}

// uiFont 返回系统界面字体（随 DPI 得到合适字号），失败时退回默认 GUI 字体。
func uiFont() uintptr {
	var ncm nonClientMetricsW
	ncm.Size = uint32(unsafe.Sizeof(ncm))
	r, _, _ := procSystemParametersInfo.Call(spiGetNonClientMetrics, uintptr(ncm.Size), uintptr(unsafe.Pointer(&ncm)), 0)
	if r != 0 && saneLogFont(&ncm.MessageFont) {
		if font, _, _ := procCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&ncm.MessageFont))); font != 0 {
			return font
		}
	}
	font, _, _ := procGetStockObject.Call(defaultGUIFont)
	return font
}

// saneLogFont 校验系统返回的字体描述是否可信。
// 结构体布局一旦错位，这里读到的会是垃圾值 —— 该检查把"布局错误"转化为
// "退回默认字体"，而不是界面字体错乱或启动失败。
func saneLogFont(lf *logFontW) bool {
	if lf.Height > -6 || lf.Height < -200 {
		return false
	}
	n := 0
	for _, c := range lf.FaceName {
		if c == 0 {
			break
		}
		n++
	}
	return n >= 2
}

// openFileDialogWith 打开"选择文件"对话框；用户取消返回 ok=false。
// filterParts 决定文件类型下拉内容，由调用方按用途给出。
func openFileDialogWith(owner uintptr, title, initialDir string, filterParts []string) (string, bool) {
	buf := make([]uint16, 4096)
	filter := dialogFilterSlice(filterParts)
	ofn := openFileNameW{
		StructSize:  uint32(unsafe.Sizeof(openFileNameW{})),
		Owner:       owner,
		Filter:      &filter[0],
		FilterIndex: 1,
		File:        &buf[0],
		MaxFile:     uint32(len(buf)),
		InitialDir:  optionalPtr(initialDir),
		Title:       utf16Ptr(title),
		Flags:       ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir | ofnHideReadOnly | ofnNoDereference,
	}
	if r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return "", false
	}
	runtime.KeepAlive(filter)
	return syscall.UTF16ToString(buf), true
}

// saveFileDialog 打开"另存为"对话框；用户取消返回 ok=false。
// initial 会预填到文件名框（超长则截断，最长 4095 个 UTF-16 码元），不改变原文件。
func saveFileDialog(owner uintptr, title, initial, defaultExt string) (string, bool) {
	buf := make([]uint16, 4096)
	copy(buf, utf16Slice(initial))
	filter := dialogFilterSlice(saveDialogFilter)
	ofn := openFileNameW{
		StructSize:  uint32(unsafe.Sizeof(openFileNameW{})),
		Owner:       owner,
		Filter:      &filter[0],
		FilterIndex: 1,
		File:        &buf[0],
		MaxFile:     uint32(len(buf)),
		Title:       utf16Ptr(title),
		DefaultExt:  optionalPtr(defaultExt),
		Flags:       ofnExplorer | ofnPathMustExist | ofnNoChangeDir | ofnOverwritePrompt,
	}
	if r, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return "", false
	}
	runtime.KeepAlive(filter)
	return syscall.UTF16ToString(buf), true
}

// pickFolderDialog 打开"选择文件夹"对话框；用户取消返回 ok=false。
// 需要 OLE 已在本线程初始化（Run 中调用 CoInitializeEx），否则新式样式会退化为旧样式。
func pickFolderDialog(owner uintptr, title string) (string, bool) {
	display := make([]uint16, 260)
	bi := browseInfoW{
		Owner:       owner,
		DisplayName: &display[0],
		Title:       utf16Ptr(title),
		Flags:       bifReturnOnlyFSDirs | bifUseNewUI,
	}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", false
	}
	defer procCoTaskMemFree.Call(pidl)
	path := make([]uint16, 260)
	if r, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0]))); r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(path), true
}

// shellOpen 用系统默认程序打开路径（文档、目录、程序皆可）。
func shellOpen(path string) error {
	op := utf16Ptr("open")
	target := utf16Ptr(path)
	r, _, err := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(op)), uintptr(unsafe.Pointer(target)), 0, 0, swShowNormal)
	runtime.KeepAlive(op)
	runtime.KeepAlive(target)
	if r <= 32 {
		return err
	}
	return nil
}

// optionalPtr 空串返回 nil（Win32 视作"未指定"），避免对话框被空初始目录干扰。
func optionalPtr(s string) *uint16 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return utf16Ptr(s)
}

// lockMainThread 把当前 goroutine 固定到一个 OS 线程上。
// 窗口、消息循环与窗口过程必须始终在同一线程执行，否则控件会失效或崩溃。
func lockMainThread() { runtime.LockOSThread() }
