//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/internal/datadir"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/remotetask"
	"autotrigger/pkg/schema"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
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
	idTimeoutUnit
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
	idCfgEditor
	idCfgLoad
	idCfgSave
	idCfgImport
	idCfgExport
	idCfgValidate
	idCfgStatus
	idResultView
	idResultRefresh
	idResultOpenFile
	idResultOpenDir
	idResultCopy
	idTemplateAll
	idTemplateBuiltin
	idTemplateExternal
	idTemplateUse
	idLogFiles
	idSettingsSave
	idOCREngineLabel
	idOCREngine
	idLogFilter
	idTaskNameLabel
	idTaskName
	idTaskSearch
	idTaskList
	idTaskView
	idSettingsBasic
	idSettingsAdvanced
	idHelpTopics
	idHelpBody
	idHelpAbout
	idOutputDir
	idSettingsRecognition
	idOCREnginePathLabel
	idOCREnginePath
	idOCREngineBrowse
	idSevenZipLabel
	idSevenZip
	idSevenZipBrowse
	idOCRScoreLabel
	idOCRScore
	idTaskSearchLabel
	idTemplateImport
	idTemplateExport
	idTemplateCheck
	idTemplateBody
	idTemplateStatus
	idTemplateUseBuiltin
	idTemplateUseExternal
	idTemplateDetailsMerged
	idTemplateDetailsBuiltin
	idTemplateDetailsExternal
	idResultHelp
	idConfigHelp
	idTemplateGuide
	idTemplateSelect
	idTemplatePreview
	idTemplateCreate
	idDependencyList
	idDependencyDetails
	idDetailRetry
	idDetailStop
	idDetailLogTab
	idDetailJSONTab
	idDetailPath
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

// 状态行的语义档位，决定文字颜色与提示措辞（F20.6）。
const (
	statusKindInfo       = "info"
	statusKindRun        = "run"
	statusKindOK         = "ok"
	statusKindUnverified = "unverified"
	statusKindWarn       = "warn"
	statusKindFail       = "fail"
)

var configDialogFilter = []string{
	"JSON 配置文件 (*.json)", "*.json",
	"所有文件 (*.*)", "*.*",
	"",
}

type form struct {
	deps                cli.Dependencies
	hwnd                uintptr
	instance            uintptr
	font                uintptr
	statusFont          uintptr
	scale               float64
	className           *uint16
	title               string
	exePath             string
	exeDir              string
	dataDir             string
	dataError           error
	opts                Options
	ctrls               map[int]uintptr
	page                pageID
	hoverPage           pageID
	templates           templateInfo
	configOpen          bool
	templateTab         int
	autoTaskName        string
	logScroll           map[uintptr]*logScrollState
	logSubclass         uintptr
	helpWindow          uintptr
	templateMaker       uintptr
	templateEntries     []templates.Entry
	tooltipTexts        []*uint16
	tooltipEntries      map[int]tooltipEntry
	templateChecking    bool
	templateCheckResult chan templateCheckResult
	terminalBrush       uintptr
	logFiles            []string
	checkingPrereq      bool
	prereqChecked       bool
	prereqCode          int
	prereqWarnings      bool
	prereqLogStart      int
	prereqReport        *schema.Prereq
	dependencyRows      []dependencyState
	prereqOutput        []byte
	appIcon             uintptr
	historyDir          string
	history             []taskRecord
	historyView         []taskRecord
	historyStats        [3]int
	activeTask          string
	selectedResult      string
	hubProjects         []templateProject
	hubSelected         string
	hubEditing          bool
	hubPage             int
	hubCaptures         []draftCapture
	hubLegacy           []templates.Entry
	hubEnabled          map[string]bool
	detailJSON          bool
	settingsAdvanced    bool
	settingsBaseline    *settingsDraftState
	settingsRecognition bool
	loadingSettings     bool
	remoteEnabled       bool
	remoteQueue         *remotetask.Queue
	remoteServer        *remotetask.Server
	remoteID            string
	remoteCancelApplied bool
	runAwaitingDone     bool

	mu         sync.Mutex
	logBuf     []byte
	taskLogBuf []byte
	flushed    int
	reset      bool
	pending    int32
	running    bool
	aux        bool
	cancel     context.CancelFunc
	exitCode   int
	resultPath string

	// 以下字段只在界面线程读写。
	statusKind string
	logPrevCR  bool
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
		deps:       deps,
		ctrls:      map[int]uintptr{},
		scale:      dpiScale(),
		exePath:    exe,
		exeDir:     exeDir,
		className:  utf16Ptr("AutoTriggerMainWindow"),
		title:      fmt.Sprintf("AutoTrigger 图形界面 — 任意文件自动安装/触发工具  v%s", cli.Version),
		opts:       NewOptions(exeDir),
		page:       pageHome,
		hoverPage:  pageNone,
		templates:  loadTemplateInfo(exeDir),
		statusKind: statusKindInfo,
	}
	f.font = uiFont()
	f.statusFont = uiFontBold()
	f.appIcon = loadAppIcon(f.px(64))
	layout, loadErr := datadir.Load(exeDir)
	if loadErr != nil {
		layout = datadir.Layout{Root: exeDir}
		f.dataError = loadErr
	}
	f.dataDir = layout.Root
	f.opts.Output = filepath.Join(layout.Results(), DefaultResultName)
	f.opts.Config = ""
	if fileExists(layout.Config()) {
		f.opts.Config = layout.Config()
	}
	f.historyDir = layout.History()
	f.history, _ = loadHistory(f.historyDir)
	if p, err := loadPreferences(filepath.Dir(layout.Config())); err == nil {
		f.opts.Recursive = p.Recursive
		f.opts.NoArchive = p.NoArchive
		f.remoteEnabled = p.RemoteTasks
	}
	return f
}

// create 注册窗口类并创建主窗口（先隐藏，控件布置完成后再显示，避免出现空窗闪烁）。
func (f *form) create() error {
	if f.dataError != nil {
		return fmt.Errorf("无法读取数据目录：%w", f.dataError)
	}
	if f.dataDir != "" {
		if err := f.dataLayout().Ensure(); err != nil {
			return fmt.Errorf("数据目录不可写：%w", err)
		}
	}
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
		// WS_CLIPCHILDREN：父窗口重绘时跳过子控件区域，避免窗口缩放/滚动时
		// 父窗口底色盖到日志框上（表现为日志区被"擦花"）。
		wsOverlappedWindow|wsClipChildren,
		cwUseDefault, cwUseDefault,
		uintptr(f.px(1020)), uintptr(f.px(700)),
		0, 0, hInst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %v", err)
	}
	f.hwnd = hwnd
	f.createControls()
	f.applyDefaults()
	f.loadSettingsFields()
	f.createWorkflowControls()
	f.loadSettingsFields()
	f.layout()
	send(f.hwnd, 0x80, 1, f.appIcon)
	send(f.hwnd, 0x80, 0, f.appIcon)
	return nil
}

func (f *form) show() {
	procShowWindow.Call(f.hwnd, swShow)
	procUpdateWindow.Call(f.hwnd)
	if f.remoteEnabled {
		if err := f.startRemoteTasks(); err != nil {
			f.setStatusKind("远程接收未启动："+err.Error(), statusKindWarn)
		}
	}
	if f.page != pageTask {
		return
	}
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
		if f.templateMaker != 0 {
			if handled, _, _ := procIsDialogMessageW.Call(f.templateMaker, uintptr(unsafe.Pointer(&m))); handled != 0 {
				continue
			}
		}
		if f.helpWindow != 0 {
			if handled, _, _ := procIsDialogMessageW.Call(f.helpWindow, uintptr(unsafe.Pointer(&m))); handled != 0 {
				continue
			}
		}
		if handled, _, _ := procIsDialogMessageW.Call(f.hwnd, uintptr(unsafe.Pointer(&m))); handled != 0 {
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

var procIsDialogMessageW = user32.NewProc("IsDialogMessageW")

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
	f.addControl(id, "EDIT", text, wsChild|wsVisible|wsTabStop|esAutoHScroll|0x00800000, 0)
}

func (f *form) button(id int, text string) {
	f.addControl(id, "BUTTON", text, wsChild|wsVisible|wsTabStop|0xB, 0)
}

func (f *form) check(id int, text string, checked bool) {
	h := f.addControl(id, "BUTTON", text, wsChild|wsVisible|wsTabStop|bsAutoCheckBox, 0)
	if checked && h != 0 {
		send(h, bmSetCheck, bstChecked, 0)
	}
}

// combo 创建只读下拉框（CBS_DROPDOWNLIST）并填入选项。
// 用下拉而非文本框，是为了让"日志级别""超时单位"这类有限取值**只能选、不能拼错**。
func (f *form) combo(id int, items []string, sel int) {
	h := f.addControl(id, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropDownList, 0)
	if h == 0 {
		return
	}
	for _, item := range items {
		p := utf16Ptr(item)
		send(h, cbAddString, 0, uintptr(unsafe.Pointer(p)))
	}
	f.setComboSel(id, sel)
}

// setComboSel 选中下拉框的第 sel 项（越界时钳到第一项）。
func (f *form) setComboSel(id, sel int) {
	h := f.ctrls[id]
	if h == 0 {
		return
	}
	if sel < 0 {
		sel = 0
	}
	send(h, cbSetCurSel, uintptr(sel), 0)
}

// comboIndex 返回下拉框当前选中项下标；未选中返回 -1。
func (f *form) comboIndex(id int) int {
	h := f.ctrls[id]
	if h == 0 {
		return -1
	}
	return int(int32(uint32(send(h, cbGetCurSel, 0, 0))))
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

	f.label(idLblTimeout, "超时")
	f.edit(idTimeout, "")
	f.combo(idTimeoutUnit, TimeoutUnitLabels, int(UITimeoutDefaultUnit))
	f.label(idLblLogLevel, "日志级别")
	f.combo(idLogLevel, LogLevelLabels, LogLevelIndex(DefaultLogLevel))

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
	// 状态行用加粗放大字体：安装成功/失败要一眼可见（F20.6）。
	if h, ok := f.ctrls[idStatus]; ok && h != 0 && f.statusFont != 0 {
		send(h, wmSetFont, f.statusFont, 1)
	}

	f.addControl(idCfgEditor, "EDIT", "",
		wsChild|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esWantReturn,
		wsExClientEdge)
	f.button(idCfgLoad, "加载")
	f.button(idCfgSave, "保存")
	f.button(idCfgImport, "导入")
	f.button(idCfgExport, "导出")
	f.button(idCfgValidate, "校验")
	f.button(idConfigHelp, "? 字段说明")
	f.label(idCfgStatus, "配置编辑器尚未加载。")

	f.addControl(idResultView, "EDIT", "",
		wsChild|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esReadOnly|esWantReturn,
		wsExClientEdge)
	f.button(idResultRefresh, "刷新")
	f.button(idResultOpenFile, "打开结果")
	f.button(idResultOpenDir, "打开目录")
	f.button(idResultCopy, "复制")
	f.button(idResultHelp, "?")
	f.button(idDetailLogTab, "执行日志")
	f.button(idDetailJSONTab, "结果 JSON")
	f.button(idDetailPath, "")
	send(f.ctrls[idResultView], emSetLimit, 0x7ffffffe, 0)
	f.button(idTemplateAll, "全部模板")
	f.button(idTemplateBuiltin, "内置模板")
	f.button(idTemplateExternal, "自定义模板")
	f.button(idTemplateUse, "使用")
	f.button(idTemplateUseBuiltin, "使用")
	f.button(idTemplateUseExternal, "使用")
	f.button(idTemplateImport, "+ 导入模板")
	f.button(idTemplateExport, "导出内置资源")
	f.button(idTemplateCheck, "校验模板")
	f.button(idTemplateGuide, "制作说明")
	f.button(idTemplateCreate, "制作模板")
	f.combo(idTemplateSelect, []string{"当前分类没有图像模板"}, 0)
	f.button(idTemplatePreview, "打开图片")
	for _, id := range []int{idTemplateDetailsMerged, idTemplateDetailsBuiltin, idTemplateDetailsExternal} {
		f.button(id, "详情")
	}
	f.label(idTemplateStatus, "")
	f.addControl(idTemplateBody, "EDIT", "", wsChild|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esReadOnly, wsExClientEdge)
	f.button(idSettingsSave, "保存设置")
	f.label(idOCREngineLabel, "OCR 引擎")
	f.combo(idOCREngine, []string{"PaddleOCR-json", "RapidOCR-json", "Umi-OCR", "Tesseract", "Windows OCR", "不使用 OCR"}, 0)
	f.combo(idLogFilter, []string{"全部级别", "DEBUG", "INFO", "WARN", "ERROR"}, 0)
	f.addControl(idLogFiles, "LISTBOX", "", wsChild|wsTabStop|wsVScroll|0x1, wsExClientEdge)
	f.terminalBrush, _, _ = procCreateSolidBrush.Call(rgb(20, 27, 36))
}

func (f *form) applyDefaults() {
	setWindowTextRaw(f.ctrls[idInput], f.opts.Input)
	setWindowTextRaw(f.ctrls[idOutput], f.opts.Output)
	setWindowTextRaw(f.ctrls[idConfig], f.opts.Config)
	send(f.ctrls[idRecursive], bmSetCheck, uintptr(boolCheck(f.opts.Recursive)), 0)
	send(f.ctrls[idNoArchive], bmSetCheck, uintptr(boolCheck(f.opts.NoArchive)), 0)
	// 超时按「数值 + 单位」回填：能被 60 整除时用分钟显示（300 秒 → 5 分钟）。
	value, unit := SplitTimeout(f.opts.TimeoutSec)
	setWindowTextRaw(f.ctrls[idTimeout], strconv.Itoa(value))
	f.setComboSel(idTimeoutUnit, int(unit))
	f.setComboSel(idLogLevel, LogLevelIndex(f.opts.LogLevel))
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

// comboDropH 返回下拉框控件的总高度：单行高度 + 展开列表高度。
// MoveWindow 给的高度决定下拉列表能显示几项，给足行数可避免列表出现滚动条。
func (f *form) comboDropH(rowH, items int) int {
	return rowH + items*f.px(18) + f.px(4)
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
	f.showTaskControls(false)
	f.showConfigControls(false)
	f.showResultControls(false)
	f.hideWorkflowControls()
	procShowWindow.Call(f.ctrls[idLogFiles], swHide)
	procShowWindow.Call(f.ctrls[idSettingsSave], swHide)
	procShowWindow.Call(f.ctrls[idOCREngineLabel], swHide)
	procShowWindow.Call(f.ctrls[idOCREngine], swHide)
	procShowWindow.Call(f.ctrls[idLogFilter], swHide)
	for _, id := range templateControlIDs {
		procShowWindow.Call(f.ctrls[id], swHide)
	}
	switch f.page {
	case pageHome:
		if f.prereqChecked || f.checkingPrereq {
			r := f.homeActionRect(0, clientW, clientH)
			f.showAt(idDependencyDetails, clientW-f.px(uiContentPad+130), int(r.Bottom)+f.px(16), f.px(110), f.px(30))
		}
	case pageSettings:
		f.layoutSettingsPage(clientW, clientH)
	case pageTemplates:
		f.layoutTemplates(clientW, clientH)
	case pageTask:
		f.layoutTaskList(clientW, clientH)
	case pageNewTask:
		f.layoutNewTask(clientW, clientH)
	case pageHelp:
		f.layoutHelp(clientW, clientH)
	case pageConfig:
		f.showConfigControls(true)
		f.layoutConfigControls(clientW, clientH)
	case pageLogs, pageTaskDetail:
		f.showResultControls(true)
		f.layoutResultControls(clientW, clientH)
	default:
		procInvalidateRect.Call(f.hwnd, 0, 1)
	}
	procInvalidateRect.Call(f.hwnd, 0, 1)
	procUpdateWindow.Call(f.hwnd)
}

func (f *form) layoutTaskControls(clientW, clientH int) {
	f.showTaskControls(true)
	rowH, gapY, gapX := f.px(uiRowH), f.px(uiGapY), f.px(uiGapX)
	labelW := f.px(uiLabelW)
	sections := f.taskSectionRects(f.taskSurfaceRect(clientW, clientH))
	if len(sections) < 4 {
		return
	}
	sample := f.taskSectionContent(sections[0].Rect)
	params := f.taskSectionContent(sections[1].Rect)
	command := f.taskSectionContent(sections[2].Rect)
	logs := f.taskSectionContent(sections[3].Rect)
	mx := int(sample.Left)
	right := int(sample.Right)
	y := int(sample.Top)

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

	// 4) 超时（数值 + 单位下拉）/ 日志级别（下拉）
	mx = int(params.Left)
	right = int(params.Right)
	y = int(params.Top)
	x := mx
	lblTimeoutW, timeoutW, unitW := f.px(48), f.px(52), f.px(76)
	f.place(idLblTimeout, x, y+f.px(4), lblTimeoutW, rowH)
	f.place(idTimeout, x+lblTimeoutW, y, timeoutW, rowH)
	x += lblTimeoutW + timeoutW + f.px(4)
	f.place(idTimeoutUnit, x, y, unitW, f.comboDropH(rowH, len(TimeoutUnitLabels)))
	x += unitW + f.px(24)
	lblLevelW, levelW := f.px(66), f.px(160)
	f.place(idLblLogLevel, x, y+f.px(4), lblLevelW, rowH)
	f.place(idLogLevel, x+lblLevelW, y, levelW, f.comboDropH(rowH, len(LogLevelLabels)))
	y += rowH + gapY

	// 5) 选项（独占一行：勾选框文本较长，与下拉框挤在同一行在 800px 最小宽度下会重叠）
	x = mx
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

	// 6) 操作按钮
	x = mx
	for _, item := range []struct {
		id int
		w  int
	}{{idRun, 96}, {idStop, 96}, {idPrereq, 100}, {idValidate, 100}, {idOpenResult, 120}} {
		f.place(item.id, x, y, f.px(item.w), rowH)
		x += f.px(item.w) + gapX
	}

	// 7) 等价命令行
	mx = int(command.Left)
	right = int(command.Right)
	y = int(command.Top)
	copyW := f.px(72)
	f.place(idLblCommand, mx, y+f.px(4), labelW, rowH)
	f.place(idCommand, mx+labelW, y, right-(mx+labelW)-copyW-gapX, rowH)
	f.place(idCopyCommand, right-copyW, y, copyW, rowH)

	// 8) 日志 + 状态
	mx = int(logs.Left)
	right = int(logs.Right)
	y = int(logs.Top)
	statusH := f.px(28)
	f.place(idLblLog, mx, y, labelW, f.px(18))
	logTop := y + f.px(20)
	statusTop := int(logs.Bottom) - statusH
	logH := statusTop - logTop - f.px(6)
	if logH < f.px(24) {
		logH = f.px(24)
	}
	f.place(idLog, mx, logTop, right-mx, logH)
	f.place(idStatus, mx, statusTop, right-mx, statusH)
	procInvalidateRect.Call(f.hwnd, 0, 1)
}

func (f *form) layoutConfigControls(clientW, clientH int) {
	r := f.taskSurfaceRect(clientW, clientH)
	pad := f.px(18)
	left := int(r.Left) + pad
	right := int(r.Right) - pad
	top := int(r.Top) + pad
	rowH := f.px(uiRowH)
	gap := f.px(uiGapX)
	x := left
	for _, item := range []struct {
		id int
		w  int
	}{{idCfgLoad, 76}, {idCfgSave, 76}, {idCfgImport, 76}, {idCfgExport, 76}, {idCfgValidate, 76}} {
		f.place(item.id, x, top, f.px(item.w), rowH)
		x += f.px(item.w) + gap
	}
	statusH := f.px(24)
	editorTop := top + rowH + f.px(10)
	statusTop := int(r.Bottom) - pad - statusH
	editorH := statusTop - editorTop - f.px(8)
	if editorH < f.px(120) {
		editorH = f.px(120)
	}
	f.place(idCfgEditor, left, editorTop, right-left, editorH)
	f.place(idCfgStatus, left, statusTop, right-left, statusH)
	procInvalidateRect.Call(f.hwnd, 0, 1)
}

func (f *form) layoutResultControls(clientW, clientH int) {
	r := f.taskSurfaceRect(clientW, clientH)
	pad := f.px(18)
	left := int(r.Left) + pad
	right := int(r.Right) - pad
	top := int(r.Top) + pad
	if f.page == pageTaskDetail {
		f.layoutDetailMetadata(r)
		f.showAt(idDetailRetry, right-f.px(210), int(r.Top)+f.px(14), f.px(112), f.px(30))
		f.showAt(idDetailStop, right-f.px(90), int(r.Top)+f.px(14), f.px(90), f.px(30))
		f.updateDetailButtons()
		top = int(r.Top) + f.px(286)
	}
	rowH := f.px(uiRowH)
	gap := f.px(uiGapX)
	x := left
	for _, item := range []struct {
		id int
		w  int
	}{{idResultRefresh, 76}, {idResultOpenFile, 96}, {idResultHelp, 28}, {idResultOpenDir, 96}, {idResultCopy, 76}} {
		f.place(item.id, x, top, f.px(item.w), rowH)
		x += f.px(item.w) + gap
	}
	viewTop := top + rowH + f.px(10)
	viewH := int(r.Bottom) - pad - viewTop
	if f.page == pageLogs {
		fileH := f.px(32 + 24*maxInt(1, len(f.logFiles)))
		if fileH > f.px(112) {
			fileH = f.px(112)
		}
		viewH = int(r.Bottom) - pad - viewTop - fileH - f.px(42)
		procShowWindow.Call(f.ctrls[idLogFilter], swShow)
		f.place(idLogFilter, right-f.px(180), top, f.px(180), f.comboDropH(rowH, 5))
		procShowWindow.Call(f.ctrls[idLogFiles], swShow)
		f.place(idLogFiles, left, viewTop+viewH+f.px(42), right-left, fileH)
	}
	if f.page == pageTaskDetail {
		viewTop = top + rowH + f.px(10)
		for _, id := range []int{idResultOpenFile, idResultOpenDir} {
			procShowWindow.Call(f.ctrls[id], swHide)
		}
		f.showAt(idDetailLogTab, left, top, f.px(100), rowH)
		f.showAt(idDetailJSONTab, left+f.px(106), top, f.px(110), rowH)
		f.place(idResultRefresh, right-f.px(204), top, f.px(76), rowH)
		f.place(idResultCopy, right-f.px(122), top, f.px(76), rowH)
		f.place(idResultHelp, right-f.px(34), top, f.px(28), rowH)
		if !f.detailJSON {
			procShowWindow.Call(f.ctrls[idResultHelp], swHide)
		}
		viewH = maxInt(f.px(40), int(r.Bottom)-pad-viewTop-f.px(38))
		f.showAt(idDetailPath, left, viewTop+viewH+f.px(6), right-left, f.px(28))
		setWindowTextRaw(f.ctrls[idDetailPath], f.detailFilePath())
	}
	if viewH < f.px(120) && f.page != pageTaskDetail {
		viewH = f.px(120)
	}
	f.place(idResultView, left, viewTop, right-left, viewH)
	procInvalidateRect.Call(f.hwnd, 0, 1)
}

func (f *form) layoutSettingsControls(w, h int) {
	r := f.taskSurfaceRect(w, h)
	left, top := int(r.Left)+f.px(30), int(r.Top)+f.px(58)
	width := int(r.Right) - left - f.px(30)
	rows := [][2]int{{idLblOutput, idOutputDir}, {idLblTimeout, idTimeout}, {idLblLogLevel, idLogLevel}}
	for i, pair := range rows {
		y := top + f.px(i*48)
		for _, id := range pair {
			procShowWindow.Call(f.ctrls[id], swShow)
		}
		f.place(pair[0], left, y, f.px(110), f.px(30))
		f.place(pair[1], left+f.px(120), y, width-f.px(250), f.px(30))
		if pair[1] == idLogLevel {
			f.place(idLogLevel, left+f.px(120), y, width-f.px(250), f.comboDropH(f.px(30), 4))
		}
	}
	for i, id := range []int{idBrowseOutput, idTimeoutUnit} {
		procShowWindow.Call(f.ctrls[id], swShow)
		height := f.px(30)
		if id == idTimeoutUnit {
			height = f.comboDropH(height, 2)
		}
		f.place(id, int(r.Right)-f.px(150), top+f.px(i*48), f.px(120), height)
	}
	for i, id := range []int{idRecursive, idNoArchive, idNoOCR, idLock} {
		procShowWindow.Call(f.ctrls[id], swShow)
		f.place(id, left+f.px((i%2)*260), top+f.px(220+(i/2)*38), f.px(250), f.px(30))
	}
	procShowWindow.Call(f.ctrls[idOCREngineLabel], swShow)
	procShowWindow.Call(f.ctrls[idOCREngine], swShow)
	f.place(idOCREngineLabel, left, top+f.px(184), f.px(110), f.px(30))
	f.place(idOCREngine, left+f.px(120), top+f.px(184), width-f.px(150), f.comboDropH(f.px(30), 6))
	for i, id := range []int{idPrereq, idValidate, idMakeConfig} {
		procShowWindow.Call(f.ctrls[id], swShow)
		f.place(id, left+f.px(i*150), top+f.px(336), f.px(140), f.px(34))
	}
	procShowWindow.Call(f.ctrls[idStatus], swHide)
	procShowWindow.Call(f.ctrls[idSettingsSave], swShow)
	f.place(idSettingsSave, int(r.Right)-f.px(170), top+f.px(336), f.px(140), f.px(34))
	f.showAt(idRemoteTasks, left, top+f.px(292), width, f.px(30))
}

func (f *form) saveSettings() (saved bool) {
	defer func() {
		if saved {
			f.rememberSettingsDraft()
		}
	}()
	if err := f.applyDataDirectory(); err != nil {
		f.setStatusKind(err.Error(), statusKindFail)
		return false
	}
	opts, err := f.collect()
	if err != nil {
		f.setStatusKind(err.Error(), statusKindFail)
		return false
	}
	body, path, _, err := ConfigEditorBody(opts.Config, filepath.Dir(f.dataLayout().Config()))
	path = f.dataLayout().Config()
	if f.configOpen {
		body = windowText(f.ctrls[idCfgEditor])
		err = nil
	}
	if err != nil {
		f.setStatusKind(err.Error(), statusKindFail)
		return false
	}
	var doc map[string]interface{}
	if err = json.Unmarshal([]byte(body), &doc); err != nil {
		f.setStatusKind(err.Error(), statusKindFail)
		return false
	}
	doc["timeout_sec"] = opts.TimeoutSec
	engines := []string{"paddleocr_json", "rapidocr_json", "umi_ocr", "tesseract", "windows", "none"}
	engineIndex := f.comboIndex(idOCREngine)
	if engineIndex < 0 || engineIndex >= len(engines) {
		f.setStatusKind("请选择 OCR 引擎。", statusKindFail)
		return false
	}
	for _, update := range []struct {
		section, key string
		value        interface{}
	}{
		{"output", "path", opts.Output}, {"output", "log_level", opts.LogLevel}, {"ocr", "enabled", !opts.NoOCR}, {"lock", "enabled", opts.Lock},
		{"gui", "template_dir", f.dataLayout().Templates()},
		{"ocr", "engine", engines[engineIndex]},
		{"ocr", "engine_path", strings.TrimSpace(windowText(f.ctrls[idOCREnginePath]))},
		{"archive", "sevenzip_path", strings.TrimSpace(windowText(f.ctrls[idSevenZip]))},
	} {
		section, ok := doc[update.section].(map[string]interface{})
		if !ok {
			section = map[string]interface{}{}
			doc[update.section] = section
		}
		section[update.key] = update.value
	}
	score, err := strconv.ParseFloat(strings.TrimSpace(windowText(f.ctrls[idOCRScore])), 64)
	if err != nil || score < 0 || score > 1 {
		f.setStatusKind("识别置信度必须是 0 到 1 的数值。", statusKindFail)
		return false
	}
	if section, ok := doc["ocr"].(map[string]interface{}); ok {
		section["min_score"] = score
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err == nil {
		if path == "" {
			path = f.dataLayout().Config()
		}
		_, err = WriteConfigEditorBody(path, string(b), filepath.Dir(f.dataLayout().Config()))
	}
	if err != nil {
		f.setStatusKind(err.Error(), statusKindFail)
		return false
	}
	setWindowTextRaw(f.ctrls[idConfig], path)
	f.configOpen = false
	f.selectedResult = ""
	f.prereqChecked = false
	f.prereqReport = nil
	f.refreshCommand()
	if err = f.persistPreferences(path); err != nil {
		f.setStatusKind("配置已保存，但界面偏好无法保存："+err.Error(), statusKindWarn)
		f.appendLog([]byte("[警告] 界面偏好保存失败：" + err.Error() + "\n"))
		return false
	}
	if err = f.applyRemoteSetting(); err != nil {
		f.setStatusKind("远程任务设置失败："+err.Error(), statusKindFail)
		return false
	}
	f.setStatusKind("设置已保存，下次启动和新建任务会沿用这些设置。", statusKindOK)
	return true
}

func (f *form) applyTemplatePack(mode, dir string) bool {
	body, _, _, err := ConfigEditorBody(strings.TrimSpace(windowText(f.ctrls[idConfig])), filepath.Dir(f.dataLayout().Config()))
	if f.configOpen {
		body, err = windowText(f.ctrls[idCfgEditor]), nil
	}
	if err != nil {
		f.templateNotice("无法使用模板库：" + err.Error())
		return false
	}
	b, err := templateConfigBody(body, mode, dir)
	if err != nil {
		f.templateNotice("无法使用模板库：配置草稿无效，请到设置中修正。\r\n" + err.Error())
		return false
	}
	f.settingsAdvanced = true
	f.settingsRecognition = false
	f.setPage(pageSettings)
	setWindowTextRaw(f.ctrls[idCfgEditor], toCRLF(b))
	f.configOpen = true
	setWindowTextRaw(f.ctrls[idCfgStatus], "模板库已选入配置；保存后用于执行。")
	f.layout()
	return true
}

func (f *form) showTaskControls(visible bool) {
	cmd := uintptr(swHide)
	if visible {
		cmd = swShow
	}
	for _, id := range []int{
		idLblInput, idInput, idBrowseFile, idBrowseDir,
		idLblOutput, idOutput, idBrowseOutput,
		idLblConfig, idConfig, idBrowseConfig, idMakeConfig, idEditConfig,
		idLblTimeout, idTimeout, idTimeoutUnit, idLblLogLevel, idLogLevel,
		idLblOptions, idRecursive, idNoArchive, idNoOCR, idLock,
		idRun, idStop, idPrereq, idValidate, idOpenResult,
		idLblCommand, idCommand, idCopyCommand,
		idLblLog, idLog, idStatus,
	} {
		if h := f.ctrls[id]; h != 0 {
			procShowWindow.Call(h, cmd)
		}
	}
}

func (f *form) showConfigControls(visible bool) {
	cmd := uintptr(swHide)
	if visible {
		cmd = swShow
	}
	for _, id := range []int{idCfgEditor, idCfgLoad, idCfgSave, idCfgImport, idCfgExport, idCfgValidate, idCfgStatus, idConfigHelp} {
		if h := f.ctrls[id]; h != 0 {
			procShowWindow.Call(h, cmd)
		}
	}
}

func (f *form) showResultControls(visible bool) {
	cmd := uintptr(swHide)
	if visible {
		cmd = swShow
	}
	for _, id := range []int{idResultView, idResultRefresh, idResultOpenFile, idResultOpenDir, idResultCopy, idResultHelp, idDetailLogTab, idDetailJSONTab, idDetailPath} {
		if h := f.ctrls[id]; h != 0 {
			procShowWindow.Call(h, cmd)
		}
	}
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
	case wmAppTemplate:
		f.finishTemplateCheck()
		return 0
	case wmAppLog:
		f.flushLog()
		if f.page == pageLogs {
			f.updateLogView(f.ctrls[idResultView], f.filteredLogText())
		}
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

// ctlRole 标识某个子控件在 WM_CTLCOLOR* 里属于哪一类，决定背景与文字色。
type ctlRole int

const (
	roleLabel ctlRole = iota
	roleLog
	roleStatus
)

// ctlColorScheme 返回子控件在 WM_CTLCOLOR* 下应使用的背景模式与画刷色。
//
// 关键坑：**只读 EDIT（日志区）向父窗口发送的是 WM_CTLCOLORSTATIC，而不是
// WM_CTLCOLOREDIT**。若把它和 STATIC 标签一视同仁（透明背景 + 3D 面色画刷），
// 控件在绘制文字前不再擦除背景，滚动或追加日志时旧字形会残留并与新文本叠加，
// 实测表现为日志区“文字重影、互相覆盖”。故日志区单列一档：白底 + 不透明背景。
func ctlColorScheme(role ctlRole) (bkMode, brushIdx uintptr) {
	if role == roleLog {
		return opaqueBk, colorWindow
	}
	return transparentBk, colorWindow
}

// ctlTextColor 返回子控件的文字色：普通标签/按钮用系统色，状态行按语义着色。
func ctlTextColor(role ctlRole, statusKind string) uintptr {
	switch role {
	case roleLog:
		v, _, _ := procGetSysColor.Call(colorWindowText)
		return v
	case roleStatus:
		return statusColors(statusKind)
	default:
		v, _, _ := procGetSysColor.Call(colorBtnText)
		return v
	}
}

// ctlRoleOf 由子控件句柄判断它在配色上属于哪一类。
func (f *form) ctlRoleOf(lparam unsafe.Pointer) ctlRole {
	if lparam == nil {
		return roleLabel
	}
	switch child := uintptr(lparam); {
	case child != 0 && (child == f.ctrls[idLog] || child == f.ctrls[idResultView] || child == f.ctrls[idCfgEditor] || child == f.ctrls[idHelpBody] || child == f.ctrls[idTemplateBody]):
		return roleLog
	case child != 0 && child == f.ctrls[idStatus]:
		return roleStatus
	default:
		return roleLabel
	}
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
	case 0x113:
		if wparam == 44 {
			f.pollRemoteTasks()
			f.updateLiveViews()
			return 0
		}
	case wmPaint:
		return f.paint()
	case wmEraseBkgnd:
		return 1
	case wmClose:
		return f.onClose()
	case wmDestroy:
		if f.remoteServer != nil {
			f.remoteServer.Close()
			f.remoteServer = nil
		}
		user32.NewProc("KillTimer").Call(f.hwnd, 44)
		if f.appIcon != 0 {
			user32.NewProc("DestroyIcon").Call(f.appIcon)
			f.appIcon = 0
		}
		if f.terminalBrush != 0 {
			procDeleteObject.Call(f.terminalBrush)
			f.terminalBrush = 0
		}
		stock, _, _ := procGetStockObject.Call(defaultGUIFont)
		if f.statusFont != 0 && f.statusFont != stock {
			procDeleteObject.Call(f.statusFont)
			f.statusFont = 0
		}
		if f.font != 0 && f.font != stock {
			procDeleteObject.Call(f.font)
			f.font = 0
		}
		procPostQuitMessage.Call(0)
		return 0
	case wmSize:
		f.layout()
		return 0
	case wmGetMinMaxInfo:
		if lparam != nil {
			mmi := (*minMaxInfoT)(lparam)
			mmi.MinTrackSize.X = int32(f.px(920))
			mmi.MinTrackSize.Y = int32(f.px(600))
		}
		return 0
	case wmMouseMove:
		x, y := mousePoint(lparam)
		f.onMouseMove(x, y)
		return 0
	case 0x02A3: // WM_MOUSELEAVE
		f.onMouseMove(-1, -1)
		return 0
	case wmLButtonDown:
		x, y := mousePoint(lparam)
		f.onMouseDown(x, y)
		return 0
	case wmCommand:
		f.onCommand(int(uint16(wparam&0xFFFF)), int(uint16((wparam>>16)&0xFFFF)))
		return 0
	case 0x4E:
		if lparam != nil {
			f.workflowNotify(lparam)
		}
		return 0
	case 0x2B: // WM_DRAWITEM: native buttons keep focus and keyboard semantics.
		if lparam != nil {
			f.paintNativeButton((*buttonDrawItem)(lparam))
			return 1
		}
	case wmCtlColorStatic, wmCtlColorBtn:
		if uintptr(lparam) == f.ctrls[5960] || uintptr(lparam) == f.ctrls[5961] {
			procSetBkMode.Call(wparam, opaqueBk)
			procSetBkColor.Call(wparam, modernTheme.surfaceAlt)
			procSetTextColor.Call(wparam, modernTheme.muted)
			gdi32.NewProc("SetDCBrushColor").Call(wparam, modernTheme.surfaceAlt)
			brush, _, _ := gdi32.NewProc("GetStockObject").Call(18)
			return brush
		}
		if uintptr(lparam) == f.ctrls[idResultView] && (f.page == pageLogs || f.page == pageTaskDetail && !f.detailJSON) {
			procSetBkMode.Call(wparam, opaqueBk)
			procSetBkColor.Call(wparam, rgb(20, 27, 36))
			procSetTextColor.Call(wparam, rgb(204, 231, 215))
			return f.terminalBrush
		}
		// 只读多行 EDIT（日志区）发送的也是 WM_CTLCOLORSTATIC，必须与 STATIC 标签区分；
		// 状态行同理，需要按语义给文字色（成功绿 / 失败红）。
		role := f.ctlRoleOf(lparam)
		bkMode, brushIdx := ctlColorScheme(role)
		procSetBkMode.Call(wparam, bkMode)
		if role == roleLog {
			color, _, _ := procGetSysColor.Call(colorWindow)
			procSetBkColor.Call(wparam, color)
		}
		procSetTextColor.Call(wparam, ctlTextColor(role, f.statusKind))
		return sysColorBrush(brushIdx)
	case wmCtlColorEdit:
		return sysColorBrush(colorWindow)
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, uintptr(lparam))
	return r
}

func (f *form) onMouseMove(x, y int) {
	hover := f.navPageAt(x, y)
	if hover == f.hoverPage {
		return
	}
	previous := f.hoverPage
	f.hoverPage = hover
	if hover != pageNone {
		tracking := struct {
			Size, Flags uint32
			Window      uintptr
			Time        uint32
		}{Flags: 2, Window: f.hwnd}
		tracking.Size = uint32(unsafe.Sizeof(tracking))
		user32.NewProc("TrackMouseEvent").Call(uintptr(unsafe.Pointer(&tracking)))
	}
	for i, item := range uiNavItems {
		if item.ID == previous || item.ID == hover {
			r := f.navItemRect(i)
			procInvalidateRect.Call(f.hwnd, uintptr(unsafe.Pointer(&r)), 0)
		}
	}
}

func (f *form) onMouseDown(x, y int) {
	if f.page == pageTemplates && f.hubSelected == "" && !f.hubEditing {
		w, h := f.clientSize()
		for i, r := range f.hubCardRects(w, h) {
			if pointInRect(x, y, r) {
				at := f.hubPage*4 + i
				if at < len(f.hubProjects) {
					f.openHubProject(f.hubProjects[at].ID)
				}
				return
			}
		}
	}
	clientW, clientH := f.clientSize()
	if action, ok := f.headerActionAt(x, y, clientW); ok {
		f.performHeaderAction(action.ID)
		return
	}
	if p := f.navPageAt(x, y); p != pageNone {
		f.setPage(p)
		return
	}
	switch f.homeActionAt(x, y, clientW, clientH) {
	case 0:
		f.beginNewTask()
	case 1:
		f.setPage(pageTemplates)
	case 2:
		f.setPage(pageTask)
	case 3:
		f.setPage(pageSettings)
	}
}

func (f *form) performHeaderAction(id string) {
	switch id {
	case "import-templates":
		f.templateTransfer(true)
	case "export-templates":
		f.templateTransfer(false)
	case "new-template":
		f.hubSelected = ""
		f.hubEditing = true
		setWindowTextRaw(f.ctrls[hubName], "")
		setWindowTextRaw(f.ctrls[hubDescription], "")
		f.layout()
	case "new-task":
		f.beginNewTask()
	case "back-tasks":
		f.selectedResult = ""
		f.setPage(pageTask)
	case "import-history":
		f.importHistory()
	case "run-task":
		f.setPage(pageNewTask)
		f.startRun()
	case "check-prereq":
		f.checkPrereq()
	case "open-templates":
		f.openTemplateDir()
	case "refresh-templates":
		f.reloadTemplates()
	case "refresh-result":
		f.refreshResultView()
	case "open-result-file":
		f.openResultFile()
	case "open-result-dir":
		f.openResultDir()
	case "load-config":
		f.loadConfigEditor(true)
	case "save-config":
		f.saveConfigEditor()
	case "validate-config":
		if f.page == pageConfig {
			f.validateConfigEditor()
		} else {
			f.setPage(pageTask)
			f.validateConfig()
		}
	}
}

func (f *form) openTemplateDir() {
	dir := strings.TrimSpace(f.templates.TemplateDir)
	if dir == "" {
		dir = filepath.Join(f.exeDir, "templates")
	}
	if _, err := os.Stat(dir); err != nil {
		messageBox(f.hwnd, "模板目录不存在",
			"外部模板目录尚不存在：\n"+dir+"\n\n当前仍会使用内置资源包与 UIA/OCR 识别。",
			mbOK|mbIconInfo)
		return
	}
	if err := shellOpen(dir); err != nil {
		messageBox(f.hwnd, "打开失败", err.Error(), mbOK|mbIconError)
		return
	}
	f.setStatus("已打开模板目录：" + dir)
}

func (f *form) reloadTemplates() {
	f.templates = loadTemplateInfoWithConfig(f.dataLayout().Root, strings.TrimSpace(windowText(f.ctrls[idConfig])))
	f.showTemplateDetails([]string{"merged", "builtin", "external"}[f.templateTab])
	if f.templates.Error != "" {
		f.setStatusKind("模板刷新失败："+f.templates.Error, statusKindFail)
	} else if len(f.templates.Warnings) > 0 {
		f.setStatusKind("模板已刷新，但存在资源警告。", statusKindWarn)
	} else {
		f.setStatusKind("模板已刷新。", statusKindOK)
	}
	procInvalidateRect.Call(f.hwnd, 0, 1)
}

func (f *form) setPage(page pageID) {
	if page == pageConfig {
		page = pageSettings
		f.settingsAdvanced = true
		f.settingsRecognition = false
	}
	if page == pageNone || page == f.page {
		return
	}
	if f.page == pageSettings && !f.confirmLeaveSettings() {
		return
	}
	f.page = page
	f.hoverPage = pageNone
	if page == pageLogs || page == pageTaskDetail {
		send(f.ctrls[idResultView], emSetSel, 0, 0)
		if state := f.logScroll[f.ctrls[idResultView]]; state != nil {
			state.follow = true
			state.tracking = false
		}
	}
	if page == pageTemplates {
		f.templates = loadTemplateInfoWithConfig(f.dataLayout().Root, strings.TrimSpace(windowText(f.ctrls[idConfig])))
		f.showTemplateDetails([]string{"merged", "builtin", "external"}[f.templateTab])
	}
	f.layout()
	if page == pageTask {
		f.refreshTaskList()
		if h := f.ctrls[idTaskSearch]; h != 0 {
			procSetFocus.Call(h)
		}
	} else if page == pageConfig {
		if !f.configOpen {
			f.loadConfigEditor(false)
		}
		if h := f.ctrls[idCfgEditor]; h != 0 {
			procSetFocus.Call(h)
		}
	} else if page == pageLogs || page == pageTaskDetail {
		f.refreshResultView()
		if page == pageLogs {
			send(f.ctrls[idResultView], emSetSel, ^uintptr(0), ^uintptr(0))
			send(f.ctrls[idResultView], emScrollCaret, 0, 0)
			send(f.ctrls[idResultView], 0x115, 7, 0)
		}
	} else if page == pageSettings {
		body, _, _, err := ConfigEditorBody(strings.TrimSpace(windowText(f.ctrls[idConfig])), filepath.Dir(f.dataLayout().Config()))
		if err != nil {
			f.setStatusKind(err.Error(), statusKindFail)
			return
		}
		var doc struct {
			OCR struct {
				Engine string `json:"engine"`
			} `json:"ocr"`
		}
		if err = json.Unmarshal([]byte(body), &doc); err != nil {
			f.setStatusKind(err.Error(), statusKindFail)
			return
		}
		for i, engine := range []string{"paddleocr_json", "rapidocr_json", "umi_ocr", "tesseract", "windows", "none"} {
			if engine == doc.OCR.Engine {
				f.setComboSel(idOCREngine, i)
				break
			}
		}
		f.rememberSettingsDraft()
	}
}

func (f *form) onCommand(id, code int) {
	if f.hubCommand(id, code) {
		return
	}
	if id == idTemplateSelect && code == cbnSelChange {
		f.selectedTemplateDetails()
		return
	}
	if f.workflowCommand(id, code) {
		return
	}
	if id == idLogFilter && code == cbnSelChange {
		setWindowTextRaw(f.ctrls[idResultView], f.filteredLogText())
		return
	}
	if id == idLogFiles && code == 2 {
		index := int(send(f.ctrls[idLogFiles], 0x188, 0, 0))
		if index >= 0 && index < len(f.logFiles) {
			if err := shellOpen(f.logFiles[index]); err != nil {
				f.setStatusKind(err.Error(), statusKindFail)
			}
		}
		return
	}
	switch {
	case code == cbnSelChange && (id == idLogLevel || id == idTimeoutUnit):
		// 下拉框选择变化：刷新等价命令行预览（不改动其它状态）。
		f.refreshCommand()
		return
	case code == enChange && id == idTimeout:
		f.refreshCommand()
		return
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
			if f.page == pageSettings {
				if p, ok := pickFolderDialog(f.hwnd, "选择统一数据目录"); ok {
					setWindowTextRaw(f.ctrls[idOutputDir], p)
				}
				return
			}
			cur := strings.TrimSpace(windowText(f.ctrls[idOutput]))
			if p, ok := saveFileDialog(f.hwnd, "保存结果 JSON 到…", cur, "json"); ok {
				setWindowTextRaw(f.ctrls[idOutput], p)
				f.refreshCommand()
			}
		case idBrowseConfig:
			f.pickInto(idConfig, "选择配置文件（config.json）", configDialogFilter)
			f.configOpen = false
			f.loadSettingsFields()
		case idMakeConfig:
			f.makeConfig()
		case idEditConfig:
			f.editConfig()
		case idRun:
			f.startRun()
		case idStop:
			f.stopRun()
		case idDetailRetry:
			f.retrySelectedTask()
		case idDetailStop:
			if f.selectedTaskRunning() {
				f.stopRun()
			} else if record, ok := f.selectedTaskRecord(); ok && record.RemoteState == "queued" && f.remoteQueue != nil {
				if _, err := f.remoteQueue.Cancel(record.RemoteID); err != nil {
					f.setStatusKind(err.Error(), statusKindFail)
				} else {
					f.syncRemoteHistory(f.remoteQueue.List())
					f.refreshResultView()
				}
			}
		case idPrereq:
			f.checkPrereq()
		case idValidate:
			f.validateConfig()
		case idOpenResult:
			f.openResultDir()
		case idCopyCommand:
			f.copyCommand()
		case idCfgLoad:
			f.loadConfigEditor(true)
		case idCfgSave:
			f.saveConfigEditor()
			f.loadSettingsFields()
		case idCfgImport:
			f.importConfigEditor()
		case idCfgExport:
			f.exportConfigEditor()
		case idCfgValidate:
			f.validateConfigEditor()
		case idResultRefresh:
			if f.page == pageTaskDetail {
				f.resetDetailView()
			}
			f.refreshResultView()
		case idResultOpenFile:
			f.openResultFile()
		case idResultOpenDir:
			f.openResultDir()
		case idResultCopy:
			f.copyResultView()
		case idResultHelp:
			f.showFieldHelp("结果 JSON 字段说明", fieldHelp(false))
		case idDetailLogTab, idDetailJSONTab:
			f.detailJSON = id == idDetailJSONTab
			f.resetDetailView()
			f.refreshResultView()
			f.layout()
		case idDetailPath:
			if err := shellOpen(f.detailFilePath()); err != nil {
				messageBox(f.hwnd, "文件暂不可用", err.Error(), mbOK|mbIconInfo)
			}
		case idConfigHelp:
			f.showFieldHelp("高级配置字段说明", fieldHelp(true))
		case idTemplateGuide:
			f.showFieldHelp("模板制作与使用说明", templateGuide)
		case idTemplateCreate:
			f.showTemplateMaker()
		case idDependencyDetails:
			f.showDependencyDetails()
		case idTemplatePreview:
			f.previewTemplate()
		case idTemplateAll, idTemplateBuiltin, idTemplateExternal:
			f.templateTab = id - idTemplateAll
			f.showTemplateDetails([]string{"merged", "builtin", "external"}[f.templateTab])
			f.layout()
		case idTemplateUse:
			f.applyTemplatePack("merged", "")
		case idTemplateUseBuiltin:
			f.applyTemplatePack("builtin", "")
		case idTemplateUseExternal:
			f.useExternalTemplates()
		case idTemplateDetailsMerged, idTemplateDetailsBuiltin, idTemplateDetailsExternal:
			f.showTemplateDetails([]string{"merged", "builtin", "external"}[id-idTemplateDetailsMerged])
		case idTemplateImport:
			if dir, ok := pickFolderDialog(f.hwnd, "选择包含 templates.index.json 的模板目录"); ok {
				f.importTemplateDir(dir)
			}
		case idTemplateExport:
			f.exportTemplateResources()
		case idTemplateCheck:
			f.checkTemplates()
		case idSettingsSave:
			if !f.saveSettings() {
				messageBox(f.hwnd, "设置保存失败", windowText(f.ctrls[idStatus]), mbOK|mbIconError)
			}
		case idRemoteTasks:
			if f.isChecked(idRemoteTasks) && !f.remoteEnabled {
				if messageBox(f.hwnd, "允许远程提交任务", "允许与桌面执行端相同的 Windows 账户提交安装任务。任务将以本工具权限执行。\n\n请仅向可信客户端授权 SSH，上传目录见 task health。保存设置后生效。是否允许？", mbYesNo|mbIconQuestion) != idYes {
					send(f.ctrls[idRemoteTasks], bmSetCheck, 0, 0)
				}
			}
		case idRecursive, idNoArchive, idNoOCR, idLock:
			f.refreshCommand()
		}
	case code == enChange:
		switch id {
		case idInput, idOutput, idConfig, idTimeout, idLogLevel:
			if id == idInput {
				f.syncTaskName()
			}
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
			if f.remoteID != "" && f.remoteQueue != nil {
				_ = f.remoteQueue.Interrupt(f.remoteID, "桌面执行端已关闭")
			}
			cancel()
		}
	}
	procDestroyWindow.Call(f.hwnd)
	return 0
}

// ---------------------------------------------------------------- 参数与命令预览

func (f *form) collect() (Options, error) {
	raw := strings.TrimSpace(windowText(f.ctrls[idTimeout]))
	if raw == "" {
		return Options{}, fmt.Errorf("超时数值不能为空（填 0 表示不限）")
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return Options{}, fmt.Errorf("超时数值必须是整数（当前为 %q）", raw)
	}
	unit := TimeoutUnit(f.comboIndex(idTimeoutUnit))
	if unit != UnitMinutes && unit != UnitSeconds {
		unit = UnitMinutes
	}
	timeout, err := TimeoutSeconds(value, unit)
	if err != nil {
		return Options{}, err
	}
	// 下拉框只能选，取不到时回落到默认级别，避免出现空级别导致命令行缺参。
	level := DefaultLogLevel
	if i := f.comboIndex(idLogLevel); i >= 0 && i < len(LogLevelValues) {
		level = LogLevelValues[i]
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

func (f *form) currentOptions() Options {
	opts, err := f.collect()
	if err != nil {
		return f.opts
	}
	return opts
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
		path = f.dataLayout().Config()
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
		path = f.dataLayout().Config()
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

func (f *form) loadConfigEditor(showMessage bool) {
	path := strings.TrimSpace(windowText(f.ctrls[idConfig]))
	body, resolved, fromDisk, err := ConfigEditorBody(path, filepath.Dir(f.dataLayout().Config()))
	if err != nil {
		if showMessage {
			messageBox(f.hwnd, "加载失败", err.Error(), mbOK|mbIconError)
		}
		setWindowTextRaw(f.ctrls[idCfgStatus], "加载失败："+err.Error())
		f.setStatusKind("配置加载失败。", statusKindFail)
		return
	}
	if fromDisk {
		setWindowTextRaw(f.ctrls[idConfig], resolved)
	}
	if pretty, formatErr := json.MarshalIndent(json.RawMessage(body), "", "  "); formatErr == nil {
		body = toCRLF(string(pretty))
	}
	setWindowTextRaw(f.ctrls[idCfgEditor], body)
	f.configOpen = true
	f.refreshCommand()
	source := "来自内置默认配置，尚未保存。"
	if fromDisk {
		source = "来自文件：" + resolved
	}
	setWindowTextRaw(f.ctrls[idCfgStatus], source)
	f.setStatus("配置已加载：" + resolved)
}

func (f *form) saveConfigEditor() {
	path := f.dataLayout().Config()
	body := windowText(f.ctrls[idCfgEditor])
	warnings, err := WriteConfigEditorBody(path, body, filepath.Dir(f.dataLayout().Config()))
	if err != nil {
		messageBox(f.hwnd, "保存失败", "配置未保存，原文件保持不变。\n\n"+err.Error(), mbOK|mbIconError)
		setWindowTextRaw(f.ctrls[idCfgStatus], "保存失败："+err.Error())
		f.setStatusKind("配置保存失败。", statusKindFail)
		return
	}
	setWindowTextRaw(f.ctrls[idConfig], path)
	f.refreshCommand()
	status := "已保存并通过校验：" + path
	f.prereqChecked = false
	f.prereqReport = nil
	f.templates = loadTemplateInfoWithConfig(f.dataLayout().Root, path)
	if prefErr := f.persistPreferences(path); prefErr != nil {
		f.setStatusKind("配置已保存，但界面偏好保存失败："+prefErr.Error(), statusKindFail)
		return
	} else if err := f.applyRemoteSetting(); err != nil {
		f.setStatusKind("远程任务设置失败："+err.Error(), statusKindFail)
		return
	}
	if len(warnings) > 0 {
		status = "已保存，存在 warning：" + strings.Join(warnings, "；")
		f.setStatusKind("配置已保存，但存在 warning。", statusKindWarn)
	} else {
		f.setStatusKind("配置已保存并通过校验。", statusKindOK)
	}
	setWindowTextRaw(f.ctrls[idCfgStatus], status)
	if f.page == pageSettings {
		f.rememberSettingsDraft()
	}
}

func (f *form) importConfigEditor() {
	start := f.startDir(strings.TrimSpace(windowText(f.ctrls[idConfig])))
	p, ok := openFileDialogWith(f.hwnd, "导入配置文件", start, configDialogFilter)
	if !ok {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		messageBox(f.hwnd, "导入失败", err.Error(), mbOK|mbIconError)
		return
	}
	setWindowTextRaw(f.ctrls[idConfig], "")
	setWindowTextRaw(f.ctrls[idCfgEditor], toCRLF(string(b)))
	f.configOpen = true
	f.refreshCommand()
	setWindowTextRaw(f.ctrls[idCfgStatus], "已导入："+p+"；保存前可先校验。")
	f.setStatus("配置已导入：" + p)
}

func (f *form) exportConfigEditor() {
	cur := strings.TrimSpace(windowText(f.ctrls[idConfig]))
	if cur == "" {
		cur = f.dataLayout().Config()
	}
	p, ok := saveFileDialogWith(f.hwnd, "导出配置文件", cur, "json", configDialogFilter)
	if !ok {
		return
	}
	body := windowText(f.ctrls[idCfgEditor])
	warnings, err := WriteConfigEditorBody(p, body, filepath.Dir(f.dataLayout().Config()))
	if err != nil {
		messageBox(f.hwnd, "导出失败", "配置未导出。\n\n"+err.Error(), mbOK|mbIconError)
		return
	}
	setWindowTextRaw(f.ctrls[idConfig], p)
	f.refreshCommand()
	status := "已导出并通过校验：" + p
	if len(warnings) > 0 {
		status = "已导出，存在 warning：" + strings.Join(warnings, "；")
	}
	setWindowTextRaw(f.ctrls[idCfgStatus], status)
	f.setStatus("配置已导出：" + p)
}

func (f *form) validateConfigEditor() {
	body := windowText(f.ctrls[idCfgEditor])
	if strings.TrimSpace(body) == "" {
		messageBox(f.hwnd, "没有可校验内容", "请先加载、导入或输入配置 JSON。", mbOK|mbIconInfo)
		return
	}
	warnings, err := ValidateConfigJSONBody(body, f.exeDir)
	if err != nil {
		messageBox(f.hwnd, "校验失败", err.Error(), mbOK|mbIconError)
		setWindowTextRaw(f.ctrls[idCfgStatus], "校验失败："+err.Error())
		f.setStatusKind("配置校验失败。", statusKindFail)
		return
	}
	if len(warnings) > 0 {
		msg := "校验通过，但存在 warning：" + strings.Join(warnings, "；")
		setWindowTextRaw(f.ctrls[idCfgStatus], msg)
		f.setStatusKind("配置校验通过，但存在 warning。", statusKindWarn)
		messageBox(f.hwnd, "校验通过（有警告）", strings.Join(warnings, "\n"), mbOK|mbIconInfo)
		return
	}
	setWindowTextRaw(f.ctrls[idCfgStatus], "校验通过。")
	f.setStatusKind("配置校验通过。", statusKindOK)
}

// ---------------------------------------------------------------- 运行

func (f *form) auxArgs(extra ...string) []string {
	args := append([]string{}, extra...)
	if path := strings.TrimSpace(windowText(f.ctrls[idConfig])); path != "" {
		args = append(args, "--config", path)
	}
	if f.isChecked(idNoOCR) {
		args = append(args, "--no-ocr")
	}
	// Auxiliary checks use a distinct log in the selected data directory.
	args = append(args, "--log-file", filepath.Join(f.dataLayout().Logs(), "environment-"+time.Now().Format("20060102-150405.000000000")+".log"))
	args = append(args, "--output", "-")
	return args
}

func (f *form) checkPrereq() {
	if f.isRunning() {
		return
	}
	f.checkingPrereq = true
	f.prereqReport = nil
	f.prereqChecked = false
	f.prereqOutput = nil
	f.prereqLogStart = len(f.logTextSnapshot())
	f.launch("检测并准备 7-Zip / OCR", f.auxArgs("--check-prereq", "--json", "--pretty"), true)
	procInvalidateRect.Call(f.hwnd, 0, 0)
}

func (f *form) startRun() {
	if f.isRunning() {
		return
	}
	if f.settingsAdvanced && f.configOpen {
		if _, err := ValidateConfigJSONBody(windowText(f.ctrls[idCfgEditor]), f.exeDir); err != nil {
			messageBox(f.hwnd, "配置需要修正", err.Error(), mbOK|mbIconError)
			return
		}
		f.saveConfigEditor()
		if f.statusKind == statusKindFail {
			return
		}
		f.loadSettingsFields()
	}
	if !f.saveSettings() {
		messageBox(f.hwnd, "设置需要修正", windowText(f.ctrls[idStatus]), mbOK|mbIconError)
		return
	}
	opts, err := f.collect()
	if err != nil {
		messageBox(f.hwnd, "参数有误", err.Error(), mbOK|mbIconError)
		return
	}
	f.runTaskOptions(opts)
}

func (f *form) runTaskOptions(opts Options) {
	if f.isRunning() {
		return
	}
	if opts.Input != "" && opts.Output != "-" {
		dir := f.dataLayout().Results()
		if err := os.MkdirAll(dir, 0755); err != nil {
			messageBox(f.hwnd, "无法保存结果", err.Error(), mbOK|mbIconError)
			return
		}
		opts.Output = filepath.Join(dir, "result-"+time.Now().Format("20060102-150405.000000000")+".json")
	}
	args, err := opts.Args()
	if err != nil {
		messageBox(f.hwnd, "参数不完整", err.Error(), mbOK|mbIconError)
		return
	}
	f.resultPath = opts.Output
	if opts.Output != "" && opts.Output != "-" {
		if err := os.MkdirAll(f.dataLayout().Logs(), 0755); err != nil {
			f.setStatusKind(err.Error(), statusKindFail)
			return
		}
		args = append(args, "--log-file", f.taskLogPath(opts.Output))
	}
	f.registerTask(opts)
	f.launch("运行安装/触发", args, false)
}

func (f *form) stopRun() {
	f.mu.Lock()
	cancel := f.cancel
	f.mu.Unlock()
	if cancel == nil {
		return
	}
	if f.remoteID != "" && f.remoteQueue != nil {
		if _, err := f.remoteQueue.Cancel(f.remoteID); err != nil {
			f.appendLog([]byte("远程任务取消状态保存失败：" + err.Error() + "\n"))
		} else {
			f.remoteCancelApplied = true
		}
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
	f.runAwaitingDone = true
	ctx, cancel := context.WithCancel(context.Background())
	f.mu.Lock()
	f.cancel = cancel
	f.running = true
	f.aux = aux
	if !aux {
		f.taskLogBuf = nil
	}
	f.exitCode = 0
	f.mu.Unlock()
	f.refreshTaskList()
	f.updateButtons()

	display := filepath.Base(f.exePath) + " " + strings.Join(quoteEach(args), " ")
	f.setStatusKind("正在执行："+label+" …", statusKindRun)
	fmt.Fprintf(f.logSink(), "\n=== %s ===\n$ %s\n", label, display)

	checking := f.checkingPrereq
	remoteID, remoteQueue := f.remoteID, f.remoteQueue
	go func() {
		sink := f.logSink()
		var report bytes.Buffer
		var stdout io.Writer = sink
		if checking {
			stdout = io.MultiWriter(sink, &report)
		}
		code := 0
		var guard io.Closer
		if remoteID != "" && remoteQueue != nil {
			response := remoteQueue.Handle(remotetask.Request{Action: "status", ID: remoteID})
			var err error
			if response.Job == nil {
				err = fmt.Errorf("远程任务记录不可用")
			} else {
				guard, err = remotetask.VerifyContext(ctx, *response.Job)
			}
			if err != nil {
				fmt.Fprintln(sink, "[错误] 拒绝执行远程任务：", err)
				code = 3
				reason := err.Error()
				if ctx.Err() != nil {
					code = 1
					reason = ""
				}
				_ = remoteQueue.Complete(remoteID, code, reason)
			}
		}
		if code == 0 {
			deps := f.deps
			deps.HistoryManaged = true
			code = cli.Run(ctx, args, stdout, sink, deps)
		}
		if guard != nil {
			guard.Close()
		}
		if remoteID != "" && remoteQueue != nil {
			if err := remoteQueue.Complete(remoteID, code, ""); err != nil {
				fmt.Fprintln(sink, "[错误] 远程任务状态保存失败：", err)
			}
		}
		fmt.Fprintf(sink, "=== 结束：退出码 %d ===\n", code)
		f.mu.Lock()
		f.running = false
		f.cancel = nil
		f.exitCode = code
		if checking {
			f.prereqOutput = append([]byte(nil), report.Bytes()...)
		}
		f.mu.Unlock()
		post(f.hwnd, wmAppDone, uintptr(code), 0)
	}()
}

func (f *form) onRunDone(code int) {
	f.runAwaitingDone = false
	f.mu.Lock()
	aux := f.aux
	resultPath := f.resultPath
	prereqOutput := append([]byte(nil), f.prereqOutput...)
	f.mu.Unlock()
	f.updateButtons()
	f.flushLog()
	if aux {
		wasPrereq := f.checkingPrereq
		if f.checkingPrereq {
			f.prereqChecked = true
			f.prereqCode = code
			f.prereqReport = parsePrereqReport(prereqOutput)
			log := f.logTextSnapshot()
			if f.prereqLogStart < len(log) {
				log = log[f.prereqLogStart:]
			}
			f.prereqWarnings = strings.Contains(log, "[警告]") || strings.Contains(log, "[WARN]")
			f.checkingPrereq = false
		}
		// 前置检查/校验配置这类附属命令没有"安装结果"，不渲染结果横幅，
		// 只按退出码给出档位（0 通过，其余按退出码含义说明）。
		if wasPrereq && f.prereqReport != nil {
			kind := statusKindOK
			if dependencyIssues(f.prereqReport) != "" || !f.prereqReport.OK {
				kind = statusKindWarn
			}
			if code != 0 {
				kind = statusKindFail
			}
			f.setStatusKind(dependencySummary(f.prereqReport, false)+"\r\n"+dependencyNotice(f.prereqReport), kind)
		} else if wasPrereq {
			f.setStatusKind("未取得环境检测结果："+cli.ExitText(code)+"，请查看日志。", statusKindFail)
		} else if code == 0 {
			f.setStatusKind("√ 检查完成（可查看环境检测详情）", statusKindOK)
		} else {
			f.setStatusKind("× "+cli.ExitText(code)+"（退出码 "+strconv.Itoa(code)+"）", statusKindFail)
		}
		procInvalidateRect.Call(f.hwnd, 0, 1)
		f.layout()
		return
	}
	f.completeTask(code)
	f.remoteID = ""
	f.remoteCancelApplied = false
	summary, ok := ReadResultSummary(resultPath)
	var ref *ResultSummary
	if ok {
		ref = &summary
	}
	lines, outcome := ResultBanner(code, ref, resultPath)
	f.appendLog([]byte(strings.Join(lines, "\n")))
	f.flushLog()
	kind, text := finishStatus(code, outcome)
	f.setStatusKind(text, kind)
	if f.page == pageLogs || f.page == pageTaskDetail {
		f.refreshResultView()
	}
}

// finishStatus 把退出码与总体结论翻译成状态行的配色档位与文案。
//
// 这是"安装成功/失败要明显"的落点（F20.6）：成功用绿、失败用红、中止或部分成功用橙，
// 文案写中文，不再让使用者去解读退出码。
func finishStatus(code int, outcome string) (kind, text string) {
	switch {
	case code == 130:
		return statusKindWarn, "! 已中止（结果已写出，可查看结果 JSON）"
	case code != 0:
		return statusKindFail, "× " + cli.ExitText(code) + "（退出码 " + strconv.Itoa(code) + "）"
	}
	switch outcome {
	case schema.OutcomeOK:
		return statusKindOK, "√ 安装成功"
	case schema.OutcomeUnverified:
		return statusKindUnverified, "! 安装成功（未验证）：有安装动作但证据不足，详见结果 JSON"
	case schema.OutcomePartial:
		return statusKindWarn, "! 部分成功：有个别样本失败或超时，详见结果 JSON"
	case schema.OutcomeFailed:
		return statusKindFail, "× 安装失败：详见结果 JSON 与日志"
	case schema.OutcomeSkipped:
		return statusKindWarn, "! 已跳过：没有可执行的安装动作"
	case "":
		// 结果 JSON 读不到（例如输出到 stdout 或路径被改到别处）：只说工具状态，不编造结论。
		return statusKindWarn, "! 运行结束，但未读到结果文件，请查看运行日志"
	default:
		return statusKindWarn, "! 没有可处理的样本"
	}
}

func (f *form) isRunning() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

func (f *form) updateButtons() {
	running := f.isRunning()
	f.updateDetailButtons()
	enableWindow(f.ctrls[idRun], !running)
	enableWindow(f.ctrls[idStop], running)
	for _, id := range []int{idPrereq, idValidate, idMakeConfig, idEditConfig,
		idBrowseFile, idBrowseDir, idBrowseOutput, idBrowseConfig,
		idCfgLoad, idCfgSave, idCfgImport, idCfgExport, idCfgValidate,
		idResultRefresh, idResultOpenFile, idResultOpenDir, idResultCopy} {
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

func (f *form) openResultFile() {
	p := f.resultPathOrDefault()
	if strings.TrimSpace(p) == "" || p == "-" {
		messageBox(f.hwnd, "没有结果文件", "当前结果输出为 stdout 或尚未指定结果路径。", mbOK|mbIconInfo)
		return
	}
	if _, err := os.Stat(p); err != nil {
		messageBox(f.hwnd, "结果文件不存在", "尚未找到结果文件：\n"+p, mbOK|mbIconInfo)
		return
	}
	if err := shellOpen(p); err != nil {
		messageBox(f.hwnd, "打开失败", err.Error(), mbOK|mbIconError)
		return
	}
	f.setStatus("已打开结果文件：" + p)
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

func (f *form) refreshResultView() {
	h := f.ctrls[idResultView]
	if h == 0 {
		return
	}
	if !f.canRefreshLog(h) {
		return
	}
	if f.page != pageLogs {
		setWindowTextRaw(h, f.resultViewText())
		procInvalidateRect.Call(h, 0, 1)
	}
	if f.page == pageLogs {
		f.updateLogView(h, f.filteredLogText())
		send(f.ctrls[idLogFiles], 0x184, 0, 0)
		f.logFiles = nil
		for _, path := range f.relatedLogFiles() {
			if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
				f.logFiles = append(f.logFiles, path)
				line := fmt.Sprintf("%s    %d bytes", filepath.Base(path), stat.Size())
				send(f.ctrls[idLogFiles], 0x180, 0, uintptr(unsafe.Pointer(utf16Ptr(line))))
			}
		}
		if len(f.logFiles) == 0 {
			send(f.ctrls[idLogFiles], 0x180, 0, uintptr(unsafe.Pointer(utf16Ptr("暂无结果或日志文件"))))
		}
	}
	if f.page == pageLogs {
		f.layout()
	}
	f.setStatus("结果视图已刷新。")
}

func (f *form) relatedLogFiles() []string {
	result := f.resultPathOrDefault()
	logPath := result + ".log"
	if cfg, _, err := config.Load(strings.TrimSpace(windowText(f.ctrls[idConfig])), f.exeDir); err == nil && strings.TrimSpace(cfg.Output.LogFile) != "" {
		logPath = cfg.Output.LogFile
	}
	return []string{result, logPath, result + ".journal.ndjson"}
}

func (f *form) filteredLogText() string {
	text := f.logTextSnapshot()
	if text == "" {
		paths := f.relatedLogFiles()
		if len(paths) > 1 {
			if file, err := os.Open(paths[1]); err == nil {
				defer file.Close()
				if stat, err := file.Stat(); err == nil {
					offset := stat.Size() - (96 << 10)
					if offset < 0 {
						offset = 0
					}
					file.Seek(offset, 0)
					buffer := make([]byte, 96<<10)
					n, _ := file.Read(buffer)
					text = string(buffer[:n])
				}
			}
		}
	}
	index := f.comboIndex(idLogFilter)
	if index > 0 && index < 5 {
		level := []string{"", "DEBUG", "INFO", "WARN", "ERROR"}[index]
		text = filterLogLines(text, level)
	}
	if strings.TrimSpace(text) == "" {
		text = "暂无匹配的日志。"
	}
	return toCRLF(text)
}

func (f *form) resultViewText() string {
	if f.page == pageTaskDetail {
		return f.detailViewText()
	}
	var b strings.Builder
	if f.page == pageTaskDetail && f.selectedTaskRunning() {
		f.mu.Lock()
		text := string(f.taskLogBuf)
		f.mu.Unlock()
		if strings.TrimSpace(text) == "" {
			return "运行中，等待执行日志…\r\n"
		}
		return toCRLF(text)
	}
	if f.page == pageLogs {
		b.WriteString("当前会话日志\r\n")
		b.WriteString(strings.Repeat("=", 32) + "\r\n")
		if logText := f.logTextSnapshot(); strings.TrimSpace(logText) != "" {
			b.WriteString(toCRLF(logText))
			if !strings.HasSuffix(b.String(), "\r\n") {
				b.WriteString("\r\n")
			}
		} else {
			b.WriteString("暂无当前会话日志。\r\n")
		}
		b.WriteString("\r\n最近结果\r\n")
		b.WriteString(strings.Repeat("=", 32) + "\r\n")
	}
	snap, ok := f.resultSnapshot()
	if !ok {
		b.WriteString("尚未读取到结果 JSON。\r\n")
		b.WriteString("结果路径：" + f.resultPathOrDefault() + "\r\n")
		b.WriteString("运行任务后点击“刷新”查看任务详情、逐文件结果和证据。\r\n")
		return b.String()
	}
	if snap.PresentationState != "" {
		fmt.Fprintf(&b, "任务 ID：%s\r\n状态：%s\r\n结果路径：%s\r\n\r\n", snap.TaskID, snap.PresentationState, snap.Path)
		if snap.PresentationState == "安装 / 触发进行中" {
			b.WriteString(toCRLF(f.logTextSnapshot()))
		}
		return b.String()
	}
	b.WriteString(resultSnapshotReport(snap))
	return b.String()
}

func (f *form) logTextSnapshot() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.logBuf) == 0 {
		return ""
	}
	const maxView = 96 << 10
	start := 0
	if len(f.logBuf) > maxView {
		start = len(f.logBuf) - maxView
	}
	return string(f.logBuf[start:])
}

func (f *form) copyResultView() {
	h := f.ctrls[idResultView]
	if h == 0 {
		return
	}
	procSetFocus.Call(h)
	send(h, emSetSel, 0, ^uintptr(0))
	send(h, wmCopy, 0, 0)
	f.setStatus("结果视图内容已复制。")
}

func toCRLF(s string) string {
	out, _ := normalizeCRLF(s, false)
	if !strings.HasSuffix(out, "\r\n") {
		out += "\r\n"
	}
	return out
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
	if f.running && !f.aux {
		f.taskLogBuf = append(f.taskLogBuf, p...)
		if len(f.taskLogBuf) > maxLogBytes {
			f.taskLogBuf = append(f.taskLogBuf[:0], f.taskLogBuf[len(f.taskLogBuf)-maxLogBytes/2:]...)
		}
	}
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
	if reset && f.canRefreshLog(h) {
		setWindowTextRaw(h, "")
		f.logPrevCR = false
	}
	if chunk == "" {
		return
	}
	// 多行 EDIT 只认 CRLF：纯 LF 不换行，日志会挤成一团。
	chunk, f.logPrevCR = normalizeCRLF(chunk, f.logPrevCR)
	if f.canRefreshLog(h) {
		f.appendToEdit(h, chunk)
	}
}

// appendToEdit 把文本追加到多行只读编辑控件末尾，并把视图滚到尾部。
// 只读编辑控件在部分系统上会忽略 EM_REPLACESEL，故先临时解除只读。
func (f *form) appendToEdit(h uintptr, s string) {
	send(h, emSetReadOnly, 0, 0)
	send(h, emSetSel, ^uintptr(0), ^uintptr(0))
	p := utf16Ptr(s)
	send(h, emReplaceSel, 0, uintptr(unsafe.Pointer(p)))
	send(h, emSetReadOnly, 1, 0)
	// 追加后跟随尾部，否则新日志可能落在可视区之外。
	send(h, emScrollCaret, 0, 0)
}

func (f *form) setStatus(s string) {
	f.setStatusKind(s, statusKindInfo)
}

// setStatusKind 更新状态行文本与语义配色（成功绿 / 失败红 / 中止橙 / 运行中蓝）。
// 颜色由窗口过程在 WM_CTLCOLORSTATIC 里按下标给出，这里只需改状态 + 触发重绘。
func (f *form) setStatusKind(s, kind string) {
	if kind == "" {
		kind = statusKindInfo
	}
	f.statusKind = kind
	if f.page == pageTemplates && f.ctrls[hubStatus] != 0 {
		setWindowTextRaw(f.ctrls[hubStatus], s)
	}
	h := f.ctrls[idStatus]
	if h == 0 {
		return
	}
	setWindowTextRaw(h, s)
	procInvalidateRect.Call(h, 0, 1)
}

func quoteEach(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, quoteArg(a))
	}
	return out
}
