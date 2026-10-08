//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// 控件 ID。1001 起，避免与系统命令 ID 冲突。
const (
	idLblInput = 1001 + iota
	idInput
	idBrowseFile
	idBrowseDir
	idLblOutput
	idOutput
	idBrowseOutput
	idLblConfig
	idConfig
	idBrowseConfig
	idMakeConfig
	idEditConfig
	idLblTimeout
	idTimeout
	idLblLogLevel
	idLogLevel
	idLblOptions
	idRecursive
	idNoArchive
	idNoOCR
	idLock
	idRun
	idStop
	idPrereq
	idValidate
	idOpenResult
	idLblCommand
	idCommand
	idCopyCommand
	idLblLog
	idLog
	idStatus
)

// 布局常量，单位为 96 DPI 下的像素（实际值经 scale 换算）。
const (
	uiPadX   = 12
	uiPadY   = 10
	uiRowH   = 24
	uiGapY   = 8
	uiLabelW = 92
	uiGapX   = 6
)

// maxLogBytes 限制界面日志缓冲上限。标准 EDIT 控件在文本过大后会出现不刷新
// 甚至拒绝写入的情况，因此超过上限时保留后半段并重建控件内容。
const maxLogBytes = 160 << 10

var configDialogFilter = []string{
	"JSON 配置文件 (*.json)", "*.json",
	"所有文件 (*.*)", "*.*",
	"",
}

type form struct {
	deps      cli.Dependencies
	hwnd      uintptr
	instance  uintptr
	font      uintptr
	scale     float64
	className *uint16
	title     string
	exePath   string
	exeDir    string
	opts      Options
	ctrls     map[int]uintptr

	mu         sync.Mutex
	logBuf     []byte
	flushed    int
	reset      bool
	pending    int32
	running    bool
	aux        bool
	cancel     context.CancelFunc
	exitCode   int
	resultPath string
}

func newForm(deps cli.Dependencies) *form {
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	exeDir := "."
	if exe != "" {
		exeDir = filepath.Dir(exe)
	}
	f := &form{
		deps:      deps,
		ctrls:     map[int]uintptr{},
		scale:     dpiScale(),
		exePath:   exe,
		exeDir:    exeDir,
		className: utf16Ptr("AutoTriggerMainWindow"),
		title:     fmt.Sprintf("AutoTrigger 图形界面 — 任意文件自动安装/触发工具  v%s", cli.Version),
		opts:      NewOptions(exeDir),
	}
	f.font = uiFont()
	return f
}

// create 注册窗口类并创建主窗口（先隐藏，控件布置完成后再显示，避免出现空窗闪烁）。
func (f *form) create() error {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	f.instance = hInst
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		Style:      csHREDRAW | csVREDRAW,
		WndProc:    syscall.NewCallback(f.wndProc),
		Instance:   hInst,
		Cursor:     cursor,
		Background: color3DFace + 1,
		ClassName:  f.className,
	}
	// 注册失败通常意味着类已存在（同进程重复注册）；窗口创建会给出最终判定。
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(f.className)),
		uintptr(unsafe.Pointer(utf16Ptr(f.title))),
		wsOverlappedWindow,
		cwUseDefault, cwUseDefault,
		uintptr(f.px(900)), uintptr(f.px(680)),
		0, 0, hInst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %v", err)
	}
	f.hwnd = hwnd
	f.createControls()
	f.applyDefaults()
	f.layout()
	return nil
}

func (f *form) show() {
	procShowWindow.Call(f.hwnd, swShow)
	procUpdateWindow.Call(f.hwnd)
	if h := f.ctrls[idInput]; h != 0 {
		procSetFocus.Call(h)
	}
}

func (f *form) loop() {
	var m msgT
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// ---------------------------------------------------------------- 控件

func (f *form) addControl(id int, class, text string, style, exStyle uint32) uintptr {
	h, _, _ := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(utf16Ptr(class))),
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		uintptr(style),
		0, 0, 0, 0,
		f.hwnd, uintptr(id), f.instance, 0)
	if h != 0 {
		send(h, wmSetFont, f.font, 1)
		f.ctrls[id] = h
	}
	return h
}

func (f *form) label(id int, text string) {
	f.addControl(id, "STATIC", text, wsChild|wsVisible|ssLeftNoWordWrap, 0)
}

func (f *form) edit(id int, text string) {
	f.addControl(id, "EDIT", text, wsChild|wsVisible|wsTabStop|esAutoHScroll, wsExClientEdge)
}

func (f *form) button(id int, text string) {
	f.addControl(id, "BUTTON", text, wsChild|wsVisible|wsTabStop|bsPushButton, 0)
}

func (f *form) check(id int, text string, checked bool) {
	h := f.addControl(id, "BUTTON", text, wsChild|wsVisible|wsTabStop|bsAutoCheckBox, 0)
	if checked && h != 0 {
		send(h, bmSetCheck, bstChecked, 0)
	}
}

func (f *form) createControls() {
	f.label(idLblInput, "样本文件/目录")
	f.edit(idInput, "")
	f.button(idBrowseFile, "选择文件…")
	f.button(idBrowseDir, "选择文件夹…")

	f.label(idLblOutput, "结果 JSON")
	f.edit(idOutput, "")
	f.button(idBrowseOutput, "另存为…")

	f.label(idLblConfig, "配置文件")
	f.edit(idConfig, "")
	f.button(idBrowseConfig, "选择…")
	f.button(idMakeConfig, "生成默认")
	f.button(idEditConfig, "编辑")

	f.label(idLblTimeout, "超时(秒)")
	f.edit(idTimeout, "")
	f.label(idLblLogLevel, "日志级别")
	f.edit(idLogLevel, "")

	f.label(idLblOptions, "选项")
	f.check(idRecursive, "递归子目录", false)
	f.check(idNoArchive, "跳过压缩包", false)
	f.check(idNoOCR, "禁用 OCR", false)
	f.check(idLock, "单实例锁", true)

	f.button(idRun, "运行")
	f.button(idStop, "停止")
	f.button(idPrereq, "检查前置")
	f.button(idValidate, "校验配置")
	f.button(idOpenResult, "打开结果目录")

	f.label(idLblCommand, "等价命令行")
	f.edit(idCommand, "")
	f.button(idCopyCommand, "复制")

	f.label(idLblLog, "运行日志")
	f.addControl(idLog, "EDIT", "",
		wsChild|wsVisible|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esReadOnly|esWantReturn,
		wsExClientEdge)
	if h := f.ctrls[idLog]; h != 0 {
		send(h, emSetLimit, 0x7FFFFFFE, 0)
	}
	f.label(idStatus, "就绪。")
}

func (f *form) applyDefaults() {
	setWindowTextRaw(f.ctrls[idInput], f.opts.Input)
	setWindowTextRaw(f.ctrls[idOutput], f.opts.Output)
	setWindowTextRaw(f.ctrls[idConfig], f.opts.Config)
	setWindowTextRaw(f.ctrls[idTimeout], strconv.Itoa(f.opts.TimeoutSec))
	setWindowTextRaw(f.ctrls[idLogLevel], f.opts.LogLevel)
	f.refreshCommand()
	f.updateButtons()
	if f.opts.Config == "" {
		f.setStatus("就绪。未发现 config.json，将使用内置默认配置（可点「生成默认」创建）。")
	}
}

// ---------------------------------------------------------------- 布局

func (f *form) px(v int) int {
	if f.scale <= 0 {
		return v
	}
	return int(float64(v)*f.scale + 0.5)
}

func (f *form) place(id, x, y, w, h int) {
	h2 := f.ctrls[id]
	if h2 == 0 {
		return
	}
	if w < f.px(20) {
		w = f.px(20)
	}
	if h < f.px(16) {
		h = f.px(16)
	}
	procMoveWindow.Call(h2, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
}

func (f *form) clientSize() (int, int) {
	var r rectT
	procGetClientRect.Call(f.hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	if w <= 0 {
		w = f.px(900)
	}
	if h <= 0 {
		h = f.px(680)
	}
	return w, h
}

func (f *form) layout() {
	clientW, clientH := f.clientSize()
	mx, my := f.px(uiPadX), f.px(uiPadY)
	rowH, gapY, gapX := f.px(uiRowH), f.px(uiGapY), f.px(uiGapX)
	labelW := f.px(uiLabelW)
	right := clientW - mx
	y := my

	// 1) 样本
	btnFileW, btnDirW := f.px(88), f.px(104)
	f.place(idLblInput, mx, y+f.px(4), labelW, rowH)
	f.place(idInput, mx+labelW, y, right-(mx+labelW)-btnFileW-btnDirW-gapX*2, rowH)
	f.place(idBrowseFile, right-btnDirW-btnFileW-gapX, y, btnFileW, rowH)
	f.place(idBrowseDir, right-btnDirW, y, btnDirW, rowH)
	y += rowH + gapY

	// 2) 结果
	btnW := f.px(88)
	f.place(idLblOutput, mx, y+f.px(4), labelW, rowH)
	f.place(idOutput, mx+labelW, y, right-(mx+labelW)-btnW-gapX, rowH)
	f.place(idBrowseOutput, right-btnW, y, btnW, rowH)
	y += rowH + gapY

	// 3) 配置
	b1, b2, b3 := f.px(72), f.px(88), f.px(64)
	f.place(idLblConfig, mx, y+f.px(4), labelW, rowH)
	f.place(idConfig, mx+labelW, y, right-(mx+labelW)-b1-b2-b3-gapX*3, rowH)
	f.place(idBrowseConfig, right-b3-b2-b1-gapX*2, y, b1, rowH)
	f.place(idMakeConfig, right-b3-b2-gapX, y, b2, rowH)
	f.place(idEditConfig, right-b3, y, b3, rowH)
	y += rowH + gapY

	// 4) 超时 / 日志级别 / 选项
	x := mx
	lblTimeoutW, timeoutW := f.px(64), f.px(56)
	f.place(idLblTimeout, x, y+f.px(4), lblTimeoutW, rowH)
	f.place(idTimeout, x+lblTimeoutW, y, timeoutW, rowH)
	x += lblTimeoutW + timeoutW + f.px(18)
	lblLevelW, levelW := f.px(66), f.px(72)
	f.place(idLblLogLevel, x, y+f.px(4), lblLevelW, rowH)
	f.place(idLogLevel, x+lblLevelW, y, levelW, rowH)
	x += lblLevelW + levelW + f.px(22)
	lblOptW := f.px(40)
	f.place(idLblOptions, x, y+f.px(4), lblOptW, rowH)
	x += lblOptW
	cbH := f.px(22)
	for _, item := range []struct {
		id  int
		w   int
		ttl string
	}{{idRecursive, 96, ""}, {idNoArchive, 120, ""}, {idNoOCR, 96, ""}, {idLock, 104, ""}} {
		f.place(item.id, x, y+f.px(1), f.px(item.w), cbH)
		x += f.px(item.w)
	}
	y += rowH + gapY

	// 5) 操作按钮
	x = mx
	for _, item := range []struct {
		id int
		w  int
	}{{idRun, 96}, {idStop, 96}, {idPrereq, 100}, {idValidate, 100}, {idOpenResult, 120}} {
		f.place(item.id, x, y, f.px(item.w), rowH)
		x += f.px(item.w) + gapX
	}
	y += rowH + gapY

	// 6) 等价命令行
	copyW := f.px(72)
	f.place(idLblCommand, mx, y+f.px(4), labelW, rowH)
	f.place(idCommand, mx+labelW, y, right-(mx+labelW)-copyW-gapX, rowH)
	f.place(idCopyCommand, right-copyW, y, copyW, rowH)
	y += rowH + gapY

	// 7) 日志 + 状态
	statusH := f.px(20)
	f.place(idLblLog, mx, y, labelW, f.px(18))
	logTop := y + f.px(20)
	logH := clientH - my - statusH - logTop - f.px(8)
	if logH < f.px(90) {
		logH = f.px(90)
	}
	f.place(idLog, mx, logTop, right-mx, logH)
	f.place(idStatus, mx, logTop+logH+f.px(6), right-mx, statusH)
}

// ---------------------------------------------------------------- 窗口过程

// wndProc 是窗口过程。
//
// 参数类型说明：全部参数均为指针大小，且 lparam 直接声明为 unsafe.Pointer ——
// 既避免 uintptr→unsafe.Pointer 的转换（go vet 会报 possible misuse，且
// //go:nocheckptr 并不能豁免），也便于直接解引用（如 WM_GETMINMAXINFO 传入的
// MINMAXINFO*）。回调以方法值绑定到本 form，因此消息天然归属该实例；
// 本工具每进程只有一个窗口与一次类注册，无需 GWLP_USERDATA 中转。
func (f *form) wndProc(hwnd uintptr, msg uint32, wparam uintptr, lparam unsafe.Pointer) uintptr {
	switch msg {
	case wmAppLog:
		f.flushLog()
		atomic.StoreInt32(&f.pending, 0)
		f.mu.Lock()
		more := f.flushed < len(f.logBuf)
		f.mu.Unlock()
		if more && atomic.CompareAndSwapInt32(&f.pending, 0, 1) {
			post(f.hwnd, wmAppLog, 0, 0)
		}
		return 0
	case wmAppDone:
		f.onRunDone(int(int32(uint32(wparam))))
		return 0
	}
	return f.handle(hwnd, msg, wparam, lparam)
}

func (f *form) handle(hwnd uintptr, msg uint32, wparam uintptr, lparam unsafe.Pointer) (result uintptr) {
	// 窗口过程里的 panic 会直接终止进程，这里兜底并保留日志。
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[error] 界面异常: %v\n", r)
			result = 0
		}
	}()
	switch msg {
	case wmClose:
		return f.onClose()
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	case wmSize:
		f.layout()
		return 0
	case wmGetMinMaxInfo:
		if lparam != nil {
			mmi := (*minMaxInfoT)(lparam)
			mmi.MinTrackSize.X = int32(f.px(800))
			mmi.MinTrackSize.Y = int32(f.px(540))
		}
		return 0
	case wmCommand:
		f.onCommand(int(uint16(wparam&0xFFFF)), int(uint16((wparam>>16)&0xFFFF)))
		return 0
	case wmCtlColorStatic, wmCtlColorBtn:
		textColor, _, _ := procGetSysColor.Call(colorBtnText)
		procSetBkMode.Call(wparam, transparentBk)
		procSetTextColor.Call(wparam, textColor)
		return sysColorBrush(color3DFace)
	case wmCtlColorEdit:
		return sysColorBrush(colorWindow)
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, uintptr(lparam))
	return r
}

func (f *form) onCommand(id, code int) {
	switch {
	case code == bnClicked:
		switch id {
		case idBrowseFile:
			f.pickInto(idInput, "选择待安装/触发的样本文件", fileDialogFilter)
		case idBrowseDir:
			if p, ok := pickFolderDialog(f.hwnd, "选择样本文件夹"); ok {
				setWindowTextRaw(f.ctrls[idInput], p)
				f.refreshCommand()
			}
		case idBrowseOutput:
			cur := strings.TrimSpace(windowText(f.ctrls[idOutput]))
			if p, ok := saveFileDialog(f.hwnd, "保存结果 JSON 到…", cur, "json"); ok {
				setWindowTextRaw(f.ctrls[idOutput], p)
				f.refreshCommand()
			}
		case idBrowseConfig:
			f.pickInto(idConfig, "选择配置文件（config.json）", configDialogFilter)
		case idMakeConfig:
			f.makeConfig()
		case idEditConfig:
			f.editConfig()
		case idRun:
			f.startRun()
		case idStop:
			f.stopRun()
		case idPrereq:
			f.checkPrereq()
		case idValidate:
			f.validateConfig()
		case idOpenResult:
			f.openResultDir()
		case idCopyCommand:
			f.copyCommand()
		case idRecursive, idNoArchive, idNoOCR, idLock:
			f.refreshCommand()
		}
	case code == enChange:
		switch id {
		case idInput, idOutput, idConfig, idTimeout, idLogLevel:
			f.refreshCommand()
		}
	}
}

func (f *form) onClose() uintptr {
	if f.isRunning() {
		if messageBox(f.hwnd, "任务进行中",
			"当前正在运行安装/触发任务。\n\n关闭窗口会中止任务，并终止已启动的进程树。确定要关闭吗？",
			mbYesNo|mbIconQuestion) != idYes {
			return 0
		}
		f.mu.Lock()
		cancel := f.cancel
		f.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	procDestroyWindow.Call(f.hwnd)
	return 0
}

// ---------------------------------------------------------------- 参数与命令预览

func (f *form) collect() (Options, error) {
	timeout := DefaultTimeoutSec
	if raw := strings.TrimSpace(windowText(f.ctrls[idTimeout])); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Options{}, fmt.Errorf("超时秒数必须是整数（当前为 %q）", raw)
		}
		timeout = n
	}
	level := strings.TrimSpace(windowText(f.ctrls[idLogLevel]))
	switch level {
	case "", "debug", "info", "warn", "error":
	default:
		return Options{}, fmt.Errorf("日志级别只能是 debug / info / warn / error（当前为 %q）", level)
	}
	return Options{
		Input:      strings.TrimSpace(windowText(f.ctrls[idInput])),
		Output:     strings.TrimSpace(windowText(f.ctrls[idOutput])),
		Config:     strings.TrimSpace(windowText(f.ctrls[idConfig])),
		Recursive:  f.isChecked(idRecursive),
		NoArchive:  f.isChecked(idNoArchive),
		NoOCR:      f.isChecked(idNoOCR),
		Lock:       f.isChecked(idLock),
		TimeoutSec: timeout,
		LogLevel:   level,
	}, nil
}

func (f *form) refreshCommand() {
	h := f.ctrls[idCommand]
	if h == 0 {
		return
	}
	opts, err := f.collect()
	if err != nil {
		setWindowTextRaw(h, "（"+err.Error()+"）")
		return
	}
	line, err := opts.CommandLine(filepath.Base(f.exePath))
	if err != nil {
		setWindowTextRaw(h, "（"+err.Error()+"）")
		return
	}
	setWindowTextRaw(h, line)
}

func (f *form) isChecked(id int) bool {
	h := f.ctrls[id]
	if h == 0 {
		return false
	}
	return send(h, bmGetCheck, 0, 0) == bstChecked
}

// pickInto 用文件对话框选择路径并写入指定输入框；
// 起始目录取自该输入框的当前值，便于连续处理同一批样本。
func (f *form) pickInto(id int, title string, filter []string) {
	start := ""
	if cur := strings.TrimSpace(windowText(f.ctrls[id])); cur != "" {
		start = f.startDir(cur)
	}
	if p, ok := openFileDialogWith(f.hwnd, title, start, filter); ok {
		setWindowTextRaw(f.ctrls[id], p)
		f.refreshCommand()
	}
}

// startDir 返回用于对话框初始目录的路径：目录本身，或文件所在目录。
func (f *form) startDir(p string) string {
	if p == "" {
		return ""
	}
	if info, err := os.Stat(p); err == nil {
		if info.IsDir() {
			return p
		}
		return filepath.Dir(p)
	}
	return filepath.Dir(p)
}

// ---------------------------------------------------------------- 配置文件

func (f *form) makeConfig() {
	path := strings.TrimSpace(windowText(f.ctrls[idConfig]))
	if path == "" {
		path = ConfigPath(f.exeDir)
	}
	overwrite := false
	if _, err := os.Stat(path); err == nil {
		if messageBox(f.hwnd, "配置文件已存在",
			path+"\n\n已存在同名文件，是否用默认配置覆盖？", mbYesNo|mbIconQuestion) != idYes {
			setWindowTextRaw(f.ctrls[idConfig], path)
			f.refreshCommand()
			return
		}
		overwrite = true
	}
	if _, err := WriteConfigFile(path, overwrite); err != nil {
		messageBox(f.hwnd, "生成失败", err.Error(), mbOK|mbIconError)
		return
	}
	setWindowTextRaw(f.ctrls[idConfig], path)
	f.refreshCommand()
	f.setStatus("已写入默认配置：" + path + "（可点「编辑」用记事本修改）")
}

func (f *form) editConfig() {
	path := strings.TrimSpace(windowText(f.ctrls[idConfig]))
	if path == "" {
		path = ConfigPath(f.exeDir)
	}
	if _, err := os.Stat(path); err != nil {
		if _, werr := WriteConfigFile(path, false); werr != nil {
			messageBox(f.hwnd, "无法创建默认配置", werr.Error(), mbOK|mbIconError)
			return
		}
		setWindowTextRaw(f.ctrls[idConfig], path)
		f.refreshCommand()
	}
	if err := shellOpen(path); err != nil {
		messageBox(f.hwnd, "打开失败", err.Error(), mbOK|mbIconError)
		return
	}
	f.setStatus("已用系统默认程序打开配置文件；保存后点「校验配置」确认无误。")
}

func (f *form) validateConfig() {
	path := strings.TrimSpace(windowText(f.ctrls[idConfig]))
	if path == "" {
		messageBox(f.hwnd, "未指定配置文件",
			"请先选择或生成 config.json。\n\n留空时工具会按内置默认配置运行，无需校验。",
			mbOK|mbIconInfo)
		return
	}
	f.launch("校验配置", f.auxArgs("--config", path, "--dump-config", "--pretty"), true)
}

// ---------------------------------------------------------------- 运行

func (f *form) auxArgs(extra ...string) []string {
	args := append([]string{}, extra...)
	// 附属命令不改动交付目录：日志写到临时目录。
	args = append(args, "--log-file", filepath.Join(os.TempDir(), "autotrigger-ui-aux.log"))
	return args
}

func (f *form) checkPrereq() {
	f.launch("检查前置（7-Zip / OCR）", f.auxArgs("--check-prereq", "--json", "--pretty"), true)
}

func (f *form) startRun() {
	if f.isRunning() {
		return
	}
	opts, err := f.collect()
	if err != nil {
		messageBox(f.hwnd, "参数有误", err.Error(), mbOK|mbIconError)
		return
	}
	args, err := opts.Args()
	if err != nil {
		messageBox(f.hwnd, "参数不完整", err.Error(), mbOK|mbIconError)
		return
	}
	f.resultPath = opts.Output
	f.launch("运行安装/触发", args, false)
}

func (f *form) stopRun() {
	f.mu.Lock()
	cancel := f.cancel
	f.mu.Unlock()
	if cancel == nil {
		return
	}
	fmt.Fprint(f.logSink(), "=== 请求中止：先优雅关闭，超时后终止进程树（F9.6）===\n")
	cancel()
}

// launch 在后台 goroutine 中执行一次 cli.Run；界面状态由 wmAppDone 收敛。
//
// 与命令行共用同一个 cli.Run，因此校验规则、退出码语义与结果 JSON（F10）
// 完全一致，界面不会引入第二套行为。
func (f *form) launch(label string, args []string, aux bool) {
	if f.isRunning() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.mu.Lock()
	f.cancel = cancel
	f.running = true
	f.aux = aux
	f.exitCode = 0
	f.mu.Unlock()
	f.updateButtons()

	display := filepath.Base(f.exePath) + " " + strings.Join(quoteEach(args), " ")
	fmt.Fprintf(f.logSink(), "\n=== %s ===\n$ %s\n", label, display)

	go func() {
		sink := f.logSink()
		code := cli.Run(ctx, args, sink, sink, f.deps)
		fmt.Fprintf(sink, "=== 结束：退出码 %d ===\n", code)
		f.mu.Lock()
		f.running = false
		f.cancel = nil
		f.exitCode = code
		f.mu.Unlock()
		post(f.hwnd, wmAppDone, uintptr(code), 0)
	}()
}

func (f *form) onRunDone(code int) {
	f.mu.Lock()
	aux := f.aux
	f.mu.Unlock()
	f.updateButtons()
	f.flushLog()
	if aux {
		f.setStatus(fmt.Sprintf("检查完成（退出码 %d）。", code))
		return
	}
	switch code {
	case 0:
		f.setStatus("运行完成（退出码 0）。可点「打开结果目录」查看结果 JSON 与日志。")
	case 130:
		f.setStatus("任务已中止（退出码 130）。")
	case 4:
		f.setStatus("未检测到 7-Zip（退出码 4）：请先部署离线包，或勾选「跳过压缩包」。")
	case 5:
		f.setStatus("OCR 不可用且被要求必需（退出码 5）：请部署 OCR 引擎或去掉「必需」配置。")
	case 6:
		f.setStatus("已有实例在运行（退出码 6）：请等待其结束后重试。")
	case 2:
		f.setStatus("参数或路径非法（退出码 2）：详见日志。")
	case 3:
		f.setStatus("配置解析失败（退出码 3）：详见日志。")
	default:
		f.setStatus(fmt.Sprintf("运行结束（退出码 %d）：详见日志。", code))
	}
}

func (f *form) isRunning() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

func (f *form) updateButtons() {
	running := f.isRunning()
	enableWindow(f.ctrls[idRun], !running)
	enableWindow(f.ctrls[idStop], running)
	for _, id := range []int{idPrereq, idValidate, idMakeConfig, idEditConfig,
		idBrowseFile, idBrowseDir, idBrowseOutput, idBrowseConfig} {
		enableWindow(f.ctrls[id], !running)
	}
}

func (f *form) openResultDir() {
	p := strings.TrimSpace(windowText(f.ctrls[idOutput]))
	if p == "" {
		p = f.opts.Output
	}
	dir := filepath.Dir(p)
	if _, err := os.Stat(dir); err != nil {
		messageBox(f.hwnd, "目录不存在", "结果目录尚不存在：\n"+dir, mbOK|mbIconInfo)
		return
	}
	if err := shellOpen(dir); err != nil {
		messageBox(f.hwnd, "打开失败", err.Error(), mbOK|mbIconError)
		return
	}
	f.setStatus("已打开目录：" + dir)
}

func (f *form) copyCommand() {
	h := f.ctrls[idCommand]
	if h == 0 {
		return
	}
	procSetFocus.Call(h)
	send(h, emSetSel, 0, ^uintptr(0))
	send(h, wmCopy, 0, 0)
	f.setStatus("等价命令行已复制到剪贴板，可直接用于脚本或第三方集成。")
}

// ---------------------------------------------------------------- 日志

type logSink struct{ f *form }

func (s *logSink) Write(p []byte) (int, error) {
	s.f.appendLog(p)
	return len(p), nil
}

func (f *form) logSink() *logSink { return &logSink{f: f} }

func (f *form) appendLog(p []byte) {
	if len(p) == 0 {
		return
	}
	f.mu.Lock()
	f.logBuf = append(f.logBuf, p...)
	if len(f.logBuf) > maxLogBytes {
		keep := maxLogBytes / 2
		f.logBuf = append(f.logBuf[:0], f.logBuf[len(f.logBuf)-keep:]...)
		f.flushed = 0
		f.reset = true
	}
	f.mu.Unlock()
	if atomic.CompareAndSwapInt32(&f.pending, 0, 1) {
		post(f.hwnd, wmAppLog, 0, 0)
	}
}

func (f *form) flushLog() {
	h := f.ctrls[idLog]
	if h == 0 {
		return
	}
	f.mu.Lock()
	reset := f.reset
	if reset {
		f.reset = false
		f.flushed = 0
	}
	end := flushableLen(f.logBuf)
	chunk := ""
	if end > f.flushed {
		chunk = string(f.logBuf[f.flushed:end])
		f.flushed = end
	}
	f.mu.Unlock()
	if reset {
		setWindowTextRaw(h, "")
	}
	if chunk == "" {
		return
	}
	f.appendToEdit(h, chunk)
}

// appendToEdit 把文本追加到多行只读编辑控件末尾。
// 只读编辑控件在部分系统上会忽略 EM_REPLACESEL，故先临时解除只读。
func (f *form) appendToEdit(h uintptr, s string) {
	send(h, emSetReadOnly, 0, 0)
	send(h, emSetSel, ^uintptr(0), ^uintptr(0))
	p := utf16Ptr(s)
	send(h, emReplaceSel, 0, uintptr(unsafe.Pointer(p)))
	send(h, emSetReadOnly, 1, 0)
}

func (f *form) setStatus(s string) {
	setWindowTextRaw(f.ctrls[idStatus], s)
}

func quoteEach(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, quoteArg(a))
	}
	return out
}
