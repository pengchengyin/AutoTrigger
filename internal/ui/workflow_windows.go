//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/internal/core/detect"
	"autotrigger/pkg/schema"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type listColumn struct {
	Mask                                                               uint32
	Format, Width                                                      int32
	Text                                                               *uint16
	TextMax, SubItem, Image, Order, MinWidth, DefaultWidth, IdealWidth int32
}
type listItem struct {
	Mask             uint32
	Item, SubItem    int32
	State, StateMask uint32
	Text             *uint16
	TextMax, Image   int32
	Param            uintptr
	Indent, Group    int32
	Columns          uint32
	ColumnPtr        uintptr
	Formats          uintptr
	GroupIndex       int32
}
type notifyHeader struct {
	Window, ID uintptr
	Code       uint32
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (f *form) createWorkflowControls() {
	f.label(idTaskNameLabel, "任务名称")
	f.edit(idTaskName, "")
	f.edit(idTaskSearch, "")
	f.label(idTaskSearchLabel, "搜索任务")
	f.edit(idOutputDir, filepath.Dir(windowText(f.ctrls[idOutput])))
	setWindowTextRaw(f.ctrls[idLblOutput], "结果保存目录")
	setWindowTextRaw(f.ctrls[idLblInput], "文件 / 文件夹")
	setWindowTextRaw(f.ctrls[idBrowseOutput], "选择目录…")
	f.button(idSettingsRecognition, "识别与依赖")
	f.label(idOCREnginePathLabel, "OCR 程序路径")
	f.edit(idOCREnginePath, "auto")
	f.button(idOCREngineBrowse, "选择程序…")
	f.label(idSevenZipLabel, "7-Zip 程序路径")
	f.edit(idSevenZip, "auto")
	f.button(idSevenZipBrowse, "选择程序…")
	f.label(idOCRScoreLabel, "最低识别置信度")
	f.edit(idOCRScore, "0.6")
	send(f.ctrls[idTaskSearch], 0x1501, 1, uintptr(unsafe.Pointer(utf16Ptr("搜索任务名称、文件或任务 ID"))))
	init := struct{ Size, Classes uint32 }{8, 1}
	syscallComctl.NewProc("InitCommonControlsEx").Call(uintptr(unsafe.Pointer(&init)))
	f.addControl(idTaskList, "SysListView32", "", wsChild|wsTabStop|0x1|0x4|0x8, wsExClientEdge)
	send(f.ctrls[idTaskList], 0x1036, 0, 0x20|0x1)
	f.addControl(idDependencyList, "SysListView32", "", wsChild|wsTabStop|0x1|0x4|0x8, wsExClientEdge)
	send(f.ctrls[idDependencyList], 0x1036, 0, 0x20|0x1|0x10000)
	for i, c := range []struct {
		name  string
		width int
	}{{"工具 / 离线包", 240}, {"检测状态", 150}, {"路径 / 原因", 360}} {
		column := listColumn{Mask: 1 | 2 | 4, Width: int32(f.px(c.width)), Text: utf16Ptr(c.name)}
		send(f.ctrls[idDependencyList], 0x1061, uintptr(i), uintptr(unsafe.Pointer(&column)))
	}
	f.button(idDependencyDetails, "检测详情")
	for i, c := range []struct {
		name  string
		width int
	}{{"任务 ID", 140}, {"任务名称", 140}, {"文件 / 文件夹", 220}, {"状态", 110}, {"创建时间", 150}} {
		column := listColumn{Mask: 1 | 2 | 4, Width: int32(f.px(c.width)), Text: utf16Ptr(c.name)}
		send(f.ctrls[idTaskList], 0x1061, uintptr(i), uintptr(unsafe.Pointer(&column)))
	}
	f.button(idTaskView, "查看详情")
	f.button(idSettingsBasic, "基础设置")
	f.button(idSettingsAdvanced, "高级配置 JSON")
	f.addControl(idHelpTopics, "LISTBOX", "", wsChild|wsTabStop|0x1, wsExClientEdge)
	for _, title := range []string{"快速开始", "任务与历史", "结果状态说明", "环境与依赖", "关于 AutoTrigger"} {
		send(f.ctrls[idHelpTopics], 0x180, 0, uintptr(unsafe.Pointer(utf16Ptr(title))))
	}
	f.addControl(idHelpBody, "EDIT", "", wsChild|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esReadOnly, 0)
	f.button(idHelpAbout, "关于 AutoTrigger")
	f.showHelpTopic(0)
	f.refreshTaskList()
	f.installLiveViews()
	f.addTooltip(idResultHelp, "结果 JSON 的结构与字段含义（第三方对接）")
}

var syscallComctl = syscall.NewLazyDLL("comctl32.dll")

func (f *form) hideWorkflowControls() {
	procShowWindow.Call(f.ctrls[idDependencyList], swHide)
	procShowWindow.Call(f.ctrls[idDependencyDetails], swHide)
	for _, id := range []int{idTaskSearchLabel, idTaskNameLabel, idTaskName, idTaskSearch, idTaskList, idTaskView, idSettingsBasic, idSettingsAdvanced, idHelpTopics, idHelpBody, idHelpAbout, idOutputDir, idSettingsRecognition, idOCREnginePathLabel, idOCREnginePath, idOCREngineBrowse, idSevenZipLabel, idSevenZip, idSevenZipBrowse, idOCRScoreLabel, idOCRScore} {
		procShowWindow.Call(f.ctrls[id], swHide)
	}
}

func (f *form) showAt(id, x, y, w, h int) {
	procShowWindow.Call(f.ctrls[id], swShow)
	f.place(id, x, y, w, h)
}

func (f *form) layoutTaskList(w, h int) {
	r := f.taskSurfaceRect(w, h)
	left, top := int(r.Left)+f.px(18), int(r.Top)+f.px(18)
	width := int(r.Right) - left - f.px(18)
	f.showAt(idTaskSearchLabel, left, top, f.px(80), f.px(32))
	f.showAt(idTaskSearch, left+f.px(88), top, width-f.px(238), f.px(32))
	f.showAt(idTaskView, left+width-f.px(140), top, f.px(140), f.px(32))
	f.showAt(idTaskList, left, top+f.px(48), width, int(r.Bottom)-top-f.px(66))
	send(f.ctrls[idTaskList], 0x101E, 2, uintptr(maxInt(f.px(120), width-f.px(560))))
}

func (f *form) layoutNewTask(w, h int) {
	r := f.taskSurfaceRect(w, h)
	left, top := int(r.Left)+f.px(24), int(r.Top)+f.px(28)
	width := int(r.Right) - left - f.px(24)
	f.showAt(idTaskNameLabel, left, top, f.px(90), f.px(30))
	f.showAt(idTaskName, left+f.px(100), top, width-f.px(100), f.px(32))
	top += f.px(52)
	f.showAt(idLblInput, left, top, f.px(90), f.px(30))
	f.showAt(idInput, left+f.px(100), top, width-f.px(320), f.px(32))
	f.showAt(idBrowseFile, left+width-f.px(210), top, f.px(98), f.px(32))
	f.showAt(idBrowseDir, left+width-f.px(104), top, f.px(104), f.px(32))
	top += f.px(58)
	f.showAt(idRun, left, top, f.px(120), f.px(36))
	f.showAt(idStop, left+f.px(132), top, f.px(100), f.px(36))
	top += f.px(56)
	f.showAt(idStatus, left, top, width, f.px(44))
	f.showAt(idLog, left, top+f.px(50), width, maxInt(f.px(64), int(r.Bottom)-top-f.px(74)))
}

func (f *form) beginNewTask() {
	if f.isRunning() {
		f.setPage(pageNewTask)
		return
	}
	f.selectedResult = ""
	setWindowTextRaw(f.ctrls[idInput], "")
	setWindowTextRaw(f.ctrls[idTaskName], "")
	f.autoTaskName = ""
	f.setPage(pageNewTask)
	f.setStatus("选择本次要处理的文件或文件夹。执行参数使用设置中的配置。")
	procSetFocus.Call(f.ctrls[idInput])
}

func (f *form) refreshTaskList() {
	h := f.ctrls[idTaskList]
	if h == 0 {
		return
	}
	send(h, 0x1009, 0, 0)
	f.historyView = nil
	f.historyStats = [3]int{len(f.history), 0, 0}
	query := strings.ToLower(strings.TrimSpace(windowText(f.ctrls[idTaskSearch])))
	for _, record := range f.history {
		status := "等待结果"
		if record.ID == f.activeTask && f.isRunning() {
			status = "运行中"
		} else if snap, ok := readResultSnapshot(record.Result); ok {
			outcome := snapshotOutcome(snap)
			status = schema.OutcomeText(outcome)
			if outcome == schema.OutcomeOK {
				f.historyStats[1]++
			}
			if outcome == schema.OutcomeFailed {
				f.historyStats[2]++
			}
		} else if record.ExitCode != nil {
			status = "结果文件不可用"
		}
		if query != "" && !strings.Contains(strings.ToLower(record.Name+record.Input+record.ID), query) {
			continue
		}
		values := []string{record.ID, record.Name, record.Input, status, record.Created.Local().Format("2006-01-02 15:04")}
		index := len(f.historyView)
		f.historyView = append(f.historyView, record)
		for col, text := range values {
			item := listItem{Mask: 1, Item: int32(index), SubItem: int32(col), Text: utf16Ptr(text)}
			msg := uint32(0x1074)
			row := uintptr(index)
			if col == 0 {
				msg = 0x104D
				row = 0
			}
			send(h, msg, row, uintptr(unsafe.Pointer(&item)))
		}
	}
}

func (f *form) homeStatCards() []statCard {
	if len(f.history) == 0 {
		summary, _ := f.homeSummary()
		return homeStats(summary, false)
	}
	running := 0
	if f.activeTask != "" && f.isRunning() {
		running = 1
	}
	return []statCard{
		{Label: "任务总数", Value: strconv.Itoa(f.historyStats[0]), Detail: "本机任务记录", Kind: "info"},
		{Label: "成功", Value: strconv.Itoa(f.historyStats[1]), Detail: "结果确认安装完成", Kind: "ok"},
		{Label: "失败", Value: strconv.Itoa(f.historyStats[2]), Detail: "任务结果报告失败", Kind: "fail"},
		{Label: "运行中", Value: strconv.Itoa(running), Detail: "当前执行任务", Kind: "run"},
	}
}

func (f *form) viewSelectedTask() {
	index := int(int32(send(f.ctrls[idTaskList], 0x100C, ^uintptr(0), 2)))
	if index < 0 || index >= len(f.historyView) {
		f.setStatus("请先选择一条任务记录。")
		return
	}
	f.selectedResult = f.historyView[index].Result
	f.setPage(pageTaskDetail)
}

func (f *form) workflowNotify(pointer unsafe.Pointer) {
	header := (*notifyHeader)(pointer)
	if header.ID == idTaskList && int32(header.Code) == -3 {
		f.viewSelectedTask()
	}
	if header.ID == idDependencyList && int32(header.Code) == -3 {
		f.showDependencyDetails()
	}
}

func (f *form) workflowCommand(id, code int) bool {
	if f.ctrls[id] == 0 {
		return false
	}
	if id == idCfgEditor && code == enChange {
		f.configOpen = true
		return true
	}
	if id == idOutputDir && code == enChange {
		if f.loadingSettings {
			return true
		}
		setWindowTextRaw(f.ctrls[idOutput], filepath.Join(strings.TrimSpace(windowText(f.ctrls[idOutputDir])), DefaultResultName))
		f.refreshCommand()
		return true
	}
	if id == idTaskSearch && code == enChange {
		f.refreshTaskList()
		return true
	}
	if id == idHelpTopics && code == 1 {
		f.showHelpTopic(int(send(f.ctrls[idHelpTopics], 0x188, 0, 0)))
		return true
	}
	if code != bnClicked {
		return false
	}
	switch id {
	case idTaskView:
		f.viewSelectedTask()
	case idSettingsBasic:
		f.settingsAdvanced = false
		f.settingsRecognition = false
		f.layout()
	case idSettingsRecognition:
		f.settingsAdvanced = false
		f.settingsRecognition = true
		f.layout()
	case idOCREngineBrowse:
		f.pickInto(idOCREnginePath, "选择 OCR 引擎程序", fileDialogFilter)
	case idSevenZipBrowse:
		f.pickInto(idSevenZip, "选择 7z.exe", fileDialogFilter)
	case idSettingsAdvanced:
		f.settingsAdvanced = true
		f.settingsRecognition = false
		if !f.configOpen {
			f.loadConfigEditor(false)
		}
		f.layout()
	case idHelpAbout:
		f.showHelpTopic(4)
	default:
		return false
	}
	return true
}

func (f *form) importHistory() {
	path, ok := openFileDialogWith(f.hwnd, "选择历史结果 JSON", f.exeDir, configDialogFilter)
	if !ok {
		return
	}
	record, err := recordFromResult(path, "")
	if err != nil {
		messageBox(f.hwnd, "无法读取结果", err.Error(), mbOK|mbIconError)
		return
	}
	f.addHistory(record)
	f.setPage(pageTask)
	f.refreshTaskList()
}

func (f *form) addHistory(record taskRecord) {
	for i, current := range f.history {
		if current.ID == record.ID {
			f.history[i] = record
			f.persistHistory()
			return
		}
	}
	f.history = append([]taskRecord{record}, f.history...)
	f.persistHistory()
}

func (f *form) persistHistory() {
	records, err := mergeHistory(f.historyDir, f.history)
	if err != nil {
		f.appendLog([]byte("[警告] 任务记录保存失败：" + err.Error() + "\n"))
		f.setStatusKind("任务记录保存失败："+err.Error(), statusKindWarn)
	} else {
		f.history = records
	}
	f.refreshTaskList()
}

func (f *form) persistPreferences(path string) error {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return savePreferences(filepath.Dir(f.historyDir), preferences{ConfigPath: path, Recursive: f.isChecked(idRecursive), NoArchive: f.isChecked(idNoArchive)})
}

func (f *form) registerTask(opts Options) {
	id := time.Now().Format("20060102-150405.000000000")
	name := strings.TrimSpace(windowText(f.ctrls[idTaskName]))
	if name == "" {
		name = filepath.Base(opts.Input)
	}
	f.activeTask = id
	input, result, configuration := opts.Input, opts.Output, opts.Config
	if absolute, err := filepath.Abs(input); err == nil {
		input = absolute
	}
	if result != "-" {
		if absolute, err := filepath.Abs(result); err == nil {
			result = absolute
		}
	}
	if configuration != "" {
		if absolute, err := filepath.Abs(configuration); err == nil {
			configuration = absolute
		}
	}
	record := taskRecord{ID: id, Name: name, Input: input, Result: result, Config: configuration, Created: time.Now()}
	if stat, err := os.Stat(input); err == nil {
		if stat.IsDir() {
			record.FileType = "directory"
		} else {
			size := stat.Size()
			record.FileSize = &size
			if ft, err := (detect.Detector{}).Detect(input); err == nil {
				record.FileType = ft.Kind
			}
		}
	}
	f.addHistory(record)
}

func (f *form) completeTask(code int) {
	for i := range f.history {
		if f.history[i].ID == f.activeTask {
			f.history[i].ExitCode = &code
			if snap, ok := readResultSnapshot(f.history[i].Result); ok && snap.TaskID != "" {
				f.history[i].ID = snap.TaskID
			}
			break
		}
	}
	f.activeTask = ""
	f.persistHistory()
}

func (f *form) loadSettingsFields() {
	f.loadingSettings = true
	defer func() { f.loadingSettings = false }()
	cfg, _, err := config.Load(strings.TrimSpace(windowText(f.ctrls[idConfig])), f.exeDir)
	if err != nil {
		f.setStatusKind(err.Error(), statusKindFail)
		return
	}
	if cfg.Output.Path != "" {
		setWindowTextRaw(f.ctrls[idOutput], cfg.Output.Path)
	}
	if f.ctrls[idOCREnginePath] != 0 {
		setWindowTextRaw(f.ctrls[idOCREnginePath], cfg.OCR.EnginePath)
		setWindowTextRaw(f.ctrls[idSevenZip], cfg.Archive.SevenZipPath)
		setWindowTextRaw(f.ctrls[idOCRScore], strconv.FormatFloat(cfg.OCR.MinScore, 'f', -1, 64))
	}
	if f.ctrls[idOutputDir] != 0 {
		setWindowTextRaw(f.ctrls[idOutputDir], filepath.Dir(windowText(f.ctrls[idOutput])))
	}
	if strings.TrimSpace(windowText(f.ctrls[idConfig])) != "" {
		value, unit := SplitTimeout(cfg.TimeoutSec)
		setWindowTextRaw(f.ctrls[idTimeout], strconv.Itoa(value))
		f.setComboSel(idTimeoutUnit, int(unit))
	}
	f.setComboSel(idLogLevel, LogLevelIndex(cfg.Output.LogLevel))
	send(f.ctrls[idNoOCR], bmSetCheck, uintptr(boolCheck(!cfg.OCR.Enabled)), 0)
	send(f.ctrls[idLock], bmSetCheck, uintptr(boolCheck(cfg.Lock.Enabled)), 0)
	for i, engine := range []string{"paddleocr_json", "rapidocr_json", "umi_ocr", "tesseract", "windows", "none"} {
		if engine == cfg.OCR.Engine {
			f.setComboSel(idOCREngine, i)
		}
	}
	f.templates = loadTemplateInfoWithConfig(f.exeDir, strings.TrimSpace(windowText(f.ctrls[idConfig])))
	f.refreshCommand()
}

func boolCheck(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (f *form) layoutSettingsPage(w, h int) {
	r := f.taskSurfaceRect(w, h)
	f.showAt(idSettingsBasic, int(r.Left)+f.px(18), int(r.Top)+f.px(10), f.px(130), f.px(30))
	f.showAt(idSettingsRecognition, int(r.Left)+f.px(156), int(r.Top)+f.px(10), f.px(130), f.px(30))
	f.showAt(idSettingsAdvanced, int(r.Left)+f.px(294), int(r.Top)+f.px(10), f.px(160), f.px(30))
	if f.settingsRecognition {
		f.layoutRecognition(w, h)
		return
	}
	if f.settingsAdvanced {
		f.showConfigControls(true)
		f.layoutConfigControls(w, h)
		for _, id := range []int{idCfgLoad, idCfgSave, idCfgImport, idCfgExport, idCfgValidate} {
			var rect rectT
			user32.NewProc("GetWindowRect").Call(f.ctrls[id], uintptr(unsafe.Pointer(&rect)))
			point := pointT{X: rect.Left, Y: rect.Top}
			user32.NewProc("ScreenToClient").Call(f.hwnd, uintptr(unsafe.Pointer(&point)))
			f.place(id, int(point.X), int(r.Top)+f.px(52), int(rect.Right-rect.Left), f.px(28))
		}
		f.place(idCfgEditor, int(r.Left)+f.px(18), int(r.Top)+f.px(94), int(r.Right-r.Left)-f.px(36), int(r.Bottom-r.Top)-f.px(150))
		f.showAt(idConfigHelp, int(r.Right)-f.px(146), int(r.Top)+f.px(52), f.px(128), f.px(28))
		return
	}
	f.layoutSettingsControls(w, h)
}

func (f *form) layoutHelp(w, h int) {
	r := f.taskSurfaceRect(w, h)
	left, top := int(r.Left)+f.px(18), int(r.Top)+f.px(18)
	f.showAt(idHelpTopics, left, top, f.px(166), f.px(180))
	f.showAt(idHelpBody, left+f.px(188), top, int(r.Right)-left-f.px(206), int(r.Bottom)-top-f.px(18))
	f.showHelpTopic(maxInt(0, int(send(f.ctrls[idHelpTopics], 0x188, 0, 0))))
}

func (f *form) layoutRecognition(w, h int) {
	r := f.taskSurfaceRect(w, h)
	left, top := int(r.Left)+f.px(30), int(r.Top)+f.px(56)
	width := int(r.Right) - left - f.px(30)
	f.showAt(idOCREngineLabel, left, top, f.px(130), f.px(30))
	f.showAt(idOCREngine, left+f.px(140), top, width-f.px(140), f.comboDropH(f.px(30), 6))
	for i, row := range [][3]int{{idOCREnginePathLabel, idOCREnginePath, idOCREngineBrowse}, {idSevenZipLabel, idSevenZip, idSevenZipBrowse}, {idOCRScoreLabel, idOCRScore, 0}} {
		y := top + f.px(44*(i+1))
		f.showAt(row[0], left, y, f.px(130), f.px(30))
		fieldW := width - f.px(140)
		if row[2] != 0 {
			fieldW -= f.px(118)
			f.showAt(row[2], left+width-f.px(108), y, f.px(108), f.px(32))
		}
		f.showAt(row[1], left+f.px(140), y, fieldW, f.px(32))
	}
	f.showAt(idNoOCR, left, top+f.px(176), f.px(200), f.px(30))
	listTop := top + f.px(240)
	buttonTop := int(r.Bottom) - f.px(90)
	f.showAt(idDependencyList, left, listTop, width, maxInt(f.px(48), buttonTop-listTop-f.px(12)))
	send(f.ctrls[idDependencyList], 0x101e, 2, uintptr(maxInt(f.px(100), width-f.px(420))))
	f.refreshDependencyList()
	f.showAt(idPrereq, left, buttonTop, f.px(140), f.px(34))
	f.showAt(idDependencyDetails, left+f.px(152), buttonTop, f.px(110), f.px(34))
	f.showAt(idSettingsSave, left+width-f.px(140), buttonTop, f.px(140), f.px(34))
	f.showAt(idStatus, left, buttonTop+f.px(42), width, f.px(40))
}

func (f *form) refreshDependencyList() {
	rows := dependencyStates(f.prereqReport, f.checkingPrereq)
	equal := len(rows) == len(f.dependencyRows)
	if equal {
		for i := range rows {
			if rows[i] != f.dependencyRows[i] {
				equal = false
				break
			}
		}
	}
	if equal {
		return
	}
	f.dependencyRows = rows
	h := f.ctrls[idDependencyList]
	send(h, 0x1009, 0, 0)
	for i, row := range rows {
		for col, text := range []string{row.Name, row.Status, row.Detail} {
			item := listItem{Mask: 1, Item: int32(i), SubItem: int32(col), Text: utf16Ptr(text)}
			msg, rowIndex := uint32(0x1074), uintptr(i)
			if col == 0 {
				msg, rowIndex = 0x104d, 0
			}
			send(h, msg, rowIndex, uintptr(unsafe.Pointer(&item)))
		}
	}
}

func (f *form) showDependencyDetails() {
	var b strings.Builder
	b.WriteString(dependencySummary(f.prereqReport, f.checkingPrereq) + "\r\n\r\n")
	for _, row := range dependencyStates(f.prereqReport, f.checkingPrereq) {
		fmt.Fprintf(&b, "%s\r\n状态：%s\r\n%s\r\n\r\n", row.Name, row.Status, row.Detail)
	}
	if f.prereqReport != nil {
		for _, issue := range f.prereqReport.Issues {
			b.WriteString(issue + "\r\n")
		}
	}
	f.showFieldHelp("第三方工具检测详情", b.String())
}

func (f *form) showHelpTopic(index int) {
	topics := []string{
		"快速开始\r\n\r\n1. 打开设置，确认结果保存位置、超时和 OCR 引擎，点击保存设置。\r\n2. 在首页点击检查环境，查看缺少的依赖。\r\n3. 点击新建任务，选择文件或文件夹，填写便于辨认的名称。\r\n4. 点击运行。执行参数自动沿用设置。\r\n5. 在任务配置列表选择任务，点击查看详情。",
		"任务与历史\r\n\r\n任务配置页面展示本机执行记录。可以按名称、文件路径或任务 ID 搜索。\r\n\r\n选择记录后点击查看详情，或双击记录。详情读取该任务对应的结果文件。\r\n\r\n导入结果可将已有 CLI 结果 JSON 加入列表。请保留结果文件，删除或移动后无法读取详情。\r\n\r\n新建任务只需要选择文件或文件夹，超时、OCR 和日志级别等参数在设置中统一配置。",
		"如何理解结果\r\n\r\n安装成功：找到支持安装完成的证据。\r\n安装成功（未验证）：现有证据不足以完全确认，请人工检查。\r\n部分完成：部分步骤或文件成功，仍有未完成内容。\r\n已安装：检测到现有安装，不需要再次安装。\r\n已触发：程序已启动，但不能确认安装完成。\r\n失败或超时：任务未按要求完成，详情和日志中可查看原因。\r\n\r\n任务结束或退出码为 0，不代表所有文件均安装成功。",
		"环境与依赖\r\n\r\n本工具需要 Windows 7 SP1 64 位或更新的 Windows。\r\n\r\n点击检查环境后，已有的 7-Zip 和 OCR 会直接复用；缺失时会使用工具同级 third_party 中的离线包，自动部署到工具目录。请保留完整离线包。不会联网下载。\r\n\r\nOCR 压缩包需要完整解压，程序会保留模型和 DLL，再启动引擎验证识别能力。缺包、目录不可写或自检失败时，请查看日志中的修复提示。\r\n\r\n7-Zip 安装器可能要求 UAC 批准。其他运行库不会自动安装。只读检查可运行 --check-prereq --no-deploy-deps。",
		fmt.Sprintf("关于 AutoTrigger\r\n\r\nWindows 任意文件自动安装 / 触发工具\r\n版本：%s\r\n\r\n开发语言：Go\r\n界面：Windows 原生控件\r\n支持离线运行。缺失的 7-Zip 与 OCR 可从同级离线包自动部署。\r\n\r\n图形界面与命令行共用执行逻辑和配置格式。设置保存为 config.json，高级配置可以查看、导入或导出同一份 JSON。\r\n\r\n任务记录位置：%s", cli.Version, f.historyDir),
	}
	if index < 0 || index >= len(topics) {
		return
	}
	send(f.ctrls[idHelpTopics], 0x186, uintptr(index), 0)
	setWindowTextRaw(f.ctrls[idHelpBody], topics[index])
}
