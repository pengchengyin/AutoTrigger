//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/stub"
	"autotrigger/internal/platform/windows"
	"autotrigger/internal/resources"
	"autotrigger/pkg/schema"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Opt-in: creates real native controls and captures the rendered window.
func TestWindowsVisualClicks(t *testing.T) {
	dir := os.Getenv("AUTOTRIGGER_UI_SHOTS")
	if dir == "" {
		t.Skip("set AUTOTRIGGER_UI_SHOTS to run interactive Windows checks")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()
	f := newForm(cli.Dependencies{})
	f.exeDir = t.TempDir()
	f.historyDir = filepath.Join(f.exeDir, "history")
	f.history = nil
	f.deps = cli.Dependencies{Runner: windows.Runner{}, Prereqs: &windows.Prereqs{Runner: windows.Runner{}, ExeDir: f.exeDir}}
	f.opts = NewOptions(f.exeDir)
	f.templates = loadTemplateInfo(f.exeDir)
	if err := f.create(); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(f.exeDir, "check-config.json")
	setWindowTextRaw(f.ctrls[idConfig], configPath)
	if args := strings.Join(f.auxArgs("--check-prereq"), " "); !strings.Contains(args, "--config "+configPath) {
		t.Fatal("environment check ignored selected settings config")
	}
	setWindowTextRaw(f.ctrls[idConfig], "")
	defer user32.NewProc("DestroyWindow").Call(f.hwnd)
	f.show()
	procUpdateWindow.Call(f.hwnd)
	for i := range uiNavItems {
		r := f.navItemRect(i)
		f.onMouseMove(int(r.Left+2), int(r.Top+2))
		var dirty rectT
		if changed, _, _ := user32.NewProc("GetUpdateRect").Call(f.hwnd, uintptr(unsafe.Pointer(&dirty)), 0); changed == 0 || dirty.Right > int32(f.px(uiSidebarW)) {
			t.Fatalf("hover invalidated page content: %+v", dirty)
		}
		procUpdateWindow.Call(f.hwnd)
	}
	send(f.hwnd, 0x02A3, 0, 0)
	if f.hoverPage != pageNone {
		t.Fatal("hover not cleared on leave")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	capture := func(name string) {
		t.Helper()
		procUpdateWindow.Call(f.hwnd)
		captureNativeWindow(t, f.hwnd, filepath.Join(dir, name+".png"))
	}
	click := func(r rectT) {
		x, y := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
		send(f.hwnd, wmLButtonDown, 1, uintptr(uint32(x)&0xffff|uint32(y)<<16))
		send(f.hwnd, 0x202, 0, uintptr(uint32(x)&0xffff|uint32(y)<<16))
		procUpdateWindow.Call(f.hwnd)
	}
	capture("home")
	w, h := f.clientSize()
	click(f.homeActionRect(0, w, h))
	if f.page != pageNewTask {
		t.Fatal("home new task did not navigate")
	}
	capture("task")
	for i, page := range []pageID{pageNewTask, pageTemplates, pageLogs, pageSettings} {
		f.setPage(pageHome)
		click(f.homeActionRect(i, w, h))
		if f.page != page {
			t.Fatalf("home shortcut %d: got %v", i, f.page)
		}
	}
	for i, item := range uiNavItems {
		click(f.navItemRect(i))
		if f.page != item.ID {
			t.Fatalf("navigation %s: got %v", item.Label, f.page)
		}
		capture("page-" + string(rune('0'+i)))
	}
	f.setPage(pageTemplates)
	send(f.ctrls[idTemplateExternal], 0xF5, 0, 0)
	if f.templateTab != 2 {
		t.Fatal("external template tab failed")
	}
	capture("templates-external")
	send(f.ctrls[idTemplateUseExternal], 0xF5, 0, 0)
	if f.page != pageTemplates || !strings.Contains(windowText(f.ctrls[idTemplateBody]), "无法使用自定义模板") {
		t.Fatal("empty external library was accepted")
	}
	send(f.ctrls[idTemplateAll], 0xF5, 0, 0)
	send(f.ctrls[idTemplateUse], 0xF5, 0, 0)
	if f.page != pageSettings || !f.settingsAdvanced {
		t.Fatal("template use did not open config")
	}
	if _, err := ValidateConfigJSONBody(windowText(f.ctrls[idCfgEditor]), f.exeDir); err != nil {
		t.Fatal(err)
	}
	capture("template-config")
	send(f.ctrls[idCfgSave], 0xF5, 0, 0)
	f.setPage(pageTemplates)
	waitTemplateCheck := func() {
		deadline := time.Now().Add(3 * time.Second)
		for f.templateChecking && time.Now().Before(deadline) {
			var message msgT
			for {
				available, _, _ := user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 1)
				if available == 0 {
					break
				}
				procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	f.checkTemplates()
	waitTemplateCheck()
	if f.templateChecking || !strings.Contains(windowText(f.ctrls[idTemplateBody]), "模板校验未通过") {
		t.Fatal("empty templates were reported as passing")
	}
	customDir := t.TempDir()
	beforeDraft := windowText(f.ctrls[idCfgEditor])
	f.importTemplateDir(customDir)
	if f.page != pageTemplates || windowText(f.ctrls[idCfgEditor]) != beforeDraft {
		t.Fatal("invalid import changed configuration")
	}
	writeTemplateFixture(t, customDir, false)
	f.importTemplateDir(customDir)
	if f.page != pageSettings || !strings.Contains(windowText(f.ctrls[idCfgEditor]), "template_dir") {
		t.Fatal("template import did not stage CLI configuration")
	}
	send(f.ctrls[idCfgSave], 0xF5, 0, 0)
	f.setPage(pageTemplates)
	if f.templates.External != 1 {
		t.Fatal("saved custom template inventory missing")
	}
	f.checkTemplates()
	waitTemplateCheck()
	if f.templateChecking || !strings.Contains(windowText(f.ctrls[idTemplateBody]), "校验通过：1") {
		t.Fatal("valid template did not pass source self-test")
	}
	capture("templates-validated")
	parent := t.TempDir()
	f.exportTemplateResourcesTo(parent)
	exports, _ := filepath.Glob(filepath.Join(parent, "AutoTrigger-resources-*", "templates", "templates.index.json"))
	if len(exports) != 1 {
		t.Fatal("builtin resource export missing index")
	}
	send(f.ctrls[idTemplateDetailsBuiltin], 0xF5, 0, 0)
	if !strings.Contains(windowText(f.ctrls[idTemplateBody]), "buttons.zh-CN.json") {
		t.Fatal("builtin rule details missing")
	}
	procMoveWindow.Call(f.hwnd, 0, 0, uintptr(f.px(920)), uintptr(f.px(600)), 1)
	f.layout()
	capture("templates-small")
	procMoveWindow.Call(f.hwnd, 0, 0, uintptr(f.px(1020)), uintptr(f.px(700)), 1)
	f.layout()
	f.setPage(pageSettings)
	send(f.ctrls[idSettingsBasic], 0xF5, 0, 0)
	setWindowTextRaw(f.ctrls[idTimeout], "7")
	send(f.ctrls[idSettingsSave], 0xF5, 0, 0)
	if _, err := os.Stat(filepath.Join(f.exeDir, "config.json")); err != nil {
		t.Fatalf("settings not saved: %v", err)
	}
	capture("settings-saved")
	send(f.ctrls[idSettingsRecognition], 0xF5, 0, 0)
	if !f.settingsRecognition {
		t.Fatal("dependency settings tab failed")
	}
	setWindowTextRaw(f.ctrls[idOCRScore], "0.75")
	send(f.ctrls[idSettingsSave], 0xF5, 0, 0)
	loadedConfig, _, configErr := config.Load(windowText(f.ctrls[idConfig]), f.exeDir)
	if configErr != nil || loadedConfig.OCR.MinScore != 0.75 {
		t.Fatal("visual OCR configuration not saved")
	}
	prefs, prefsErr := loadPreferences(filepath.Dir(f.historyDir))
	if prefsErr != nil || prefs.ConfigPath != windowText(f.ctrls[idConfig]) {
		t.Fatal("settings configuration selection not persisted")
	}
	capture("settings-recognition")
	send(f.ctrls[idSettingsBasic], 0xF5, 0, 0)
	f.setPage(pageTask)
	if f.currentOptions().TimeoutSec != 420 {
		t.Fatal("settings timeout not shared with task")
	}
	fixturePath := filepath.Join(f.exeDir, "history-ui-test.json")
	fixture := schema.Result{TaskID: "ui-test-history", StartedAt: time.Now(), Summary: schema.Summary{Total: 1, ByStatus: map[string]int{"skipped": 1}}}
	finished := fixture.StartedAt.Add(12 * time.Second)
	fixture.FinishedAt = &finished
	fixture.Results = []schema.FileResult{{FilePath: filepath.Join(f.exeDir, "ui-test-only.exe"), FileSizeBytes: 1048576, DetectedType: "exe", FinalStatus: "skipped", Notes: "界面测试样本，未执行安装"}}
	fixtureBytes, _ := json.Marshal(fixture)
	if err := os.WriteFile(fixturePath, fixtureBytes, 0600); err != nil {
		t.Fatal(err)
	}
	record, err := recordFromResult(fixturePath, "界面回归测试（测试记录）")
	if err != nil {
		t.Fatal(err)
	}
	f.addHistory(record)
	capture("task-history")
	stateItem := listItem{State: 2, StateMask: 2}
	send(f.ctrls[idTaskList], 0x102B, 0, uintptr(unsafe.Pointer(&stateItem)))
	send(f.ctrls[idTaskView], 0xF5, 0, 0)
	if f.page != pageTaskDetail || f.selectedResult != fixturePath {
		t.Fatal("history detail navigation failed")
	}
	capture("task-detail")
	procMoveWindow.Call(f.hwnd, 0, 0, uintptr(f.px(920)), uintptr(f.px(600)), 1)
	f.layout()
	capture("task-detail-small")
	procMoveWindow.Call(f.hwnd, 0, 0, uintptr(f.px(1020)), uintptr(f.px(700)), 1)
	f.layout()
	f.performHeaderAction("back-tasks")
	if f.page != pageTask {
		t.Fatal("detail back failed")
	}
	setWindowTextRaw(f.ctrls[idTaskSearch], "not-a-task")
	if len(f.historyView) != 0 {
		t.Fatal("task search not filtering")
	}
	setWindowTextRaw(f.ctrls[idTaskSearch], "")
	logPath := f.relatedLogFiles()[1]
	if err := os.WriteFile(logPath, []byte("[INFO] Windows UI test log\n[ERROR] filter test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.setPage(pageLogs)
	if len(f.logFiles) != 1 {
		t.Fatalf("expected real log file, got %v", f.logFiles)
	}
	f.setComboSel(idLogFilter, 4)
	send(f.hwnd, wmCommand, uintptr(idLogFilter|cbnSelChange<<16), f.ctrls[idLogFilter])
	if text := windowText(f.ctrls[idResultView]); !strings.Contains(text, "filter test") || strings.Contains(text, "Windows UI test log") {
		t.Fatalf("log filtering: %q", text)
	}
	capture("logs-filtered")
	f.setPage(pageHome)
	r := f.headerActionRect(0, w)
	click(r)
	if f.page != pageNewTask {
		t.Fatal("header new task did not navigate")
	}
	f.setPage(pageHome)
	capture("home-after")
	sentinelPath := f.resultPathOrDefault()
	if err := os.WriteFile(sentinelPath, []byte("{\"test_sentinel\":true}"), 0600); err != nil {
		t.Fatal(err)
	}
	click(f.headerActionRect(1, w))
	deadline := time.Now().Add(20 * time.Second)
	for !f.prereqChecked && time.Now().Before(deadline) {
		var message msgT
		for {
			available, _, _ := user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 1)
			if available == 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !f.prereqChecked {
		f.stopRun()
		t.Fatal("environment check did not finish")
	}
	if f.page != pageHome {
		t.Fatal("environment check unexpectedly changed home page")
	}
	for _, id := range []int{idTaskList, idTimeout, idCfgEditor, idOCREngine} {
		if visible, _, _ := user32.NewProc("IsWindowVisible").Call(f.ctrls[id]); visible != 0 {
			t.Fatalf("home retained control %d after environment check", id)
		}
	}
	t.Logf("real environment check exit=%d", f.prereqCode)
	if f.prereqReport == nil || !strings.Contains(windowText(f.ctrls[idStatus]), "7-Zip：") {
		t.Fatal("environment result did not display structured dependency statuses")
	}
	f.setPage(pageSettings)
	send(f.ctrls[idSettingsRecognition], 0xF5, 0, 0)
	capture("settings-dependency-status")
	f.setPage(pageHome)
	if saved, err := os.ReadFile(sentinelPath); err != nil || string(saved) != "{\"test_sentinel\":true}" {
		t.Fatal("environment check overwrote task output")
	}
	f.setPage(pageLogs)
	f.setComboSel(idLogFilter, 0)
	f.refreshResultView()
	capture("environment-log")
	f.setPage(pageHome)
	capture("home-environment")
	procMoveWindow.Call(f.hwnd, 0, 0, uintptr(f.px(920)), uintptr(f.px(600)), 1)
	f.layout()
	capture("home-small")
	f.setPage(pageSettings)
	capture("settings-small")
	f.setPage(pageHelp)
	for i := 0; i < 5; i++ {
		send(f.ctrls[idHelpTopics], 0x186, uintptr(i), 0)
		send(f.hwnd, wmCommand, uintptr(idHelpTopics|1<<16), f.ctrls[idHelpTopics])
		if windowText(f.ctrls[idHelpBody]) == "" {
			t.Fatal("help topic blank")
		}
	}
	capture("help-about")
	// Failure-only adapter exercises CLI dispatch without launching installers.
	a := stub.Adapter{}
	f.deps = cli.Dependencies{Runner: a, Trigger: func(config.Config, resources.Pack) ports.InstallerTrigger { return a }, Opener: a, Mounter: a, Evidence: a, NativeGUI: a, Lock: a, Prereqs: a}
	sample := filepath.Join(f.exeDir, "ui-failure-test.txt")
	if err := os.WriteFile(sample, []byte("UI negative-path test"), 0600); err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 2; iteration++ {
		f.beginNewTask()
		setWindowTextRaw(f.ctrls[idInput], sample)
		setWindowTextRaw(f.ctrls[idTaskName], fmt.Sprintf("不会执行安装的失败回归测试 %d", iteration+1))
		before := len(f.history)
		send(f.ctrls[idRun], 0xF5, 0, 0)
		deadline = time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			var message msgT
			for {
				available, _, _ := user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 1)
				if available == 0 {
					break
				}
				procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
			}
			if len(f.history) > before && f.history[0].ExitCode != nil {
				break
			}
			time.Sleep(15 * time.Millisecond)
		}
		if len(f.history) != before+1 || f.history[0].ExitCode == nil || *f.history[0].ExitCode == 0 {
			t.Fatal("CLI failure did not update task history")
		}
		if snap, ok := readResultSnapshot(f.history[0].Result); !ok || snapshotOutcome(snap) != schema.OutcomeFailed {
			t.Fatal("CLI failure result missing or misclassified")
		}
		if f.history[0].Config != windowText(f.ctrls[idConfig]) {
			t.Fatal("task did not inherit saved CLI config")
		}
		if f.history[0].Result == sentinelPath {
			t.Fatal("task reused previous result file")
		}
		stored, err := loadHistory(f.historyDir)
		if err != nil || len(stored) != len(f.history) {
			t.Fatal("completed task history not persisted")
		}
		if len(stored) > 1 && stored[0].Result == stored[1].Result {
			t.Fatal("consecutive tasks reused result file")
		}
	}
	f.setPage(pageTask)
	for row, record := range f.historyView {
		var text [512]uint16
		item := listItem{SubItem: 1, Text: &text[0], TextMax: int32(len(text))}
		send(f.ctrls[idTaskList], 0x1073, uintptr(row), uintptr(unsafe.Pointer(&item)))
		if actual := syscall.UTF16ToString(text[:]); actual != record.Name {
			t.Fatalf("row %d name=%q, want %q", row, actual, record.Name)
		}
	}
	capture("task-history-after-failure")
}

func captureNativeWindow(t *testing.T, hwnd uintptr, path string) {
	t.Helper()
	var r rectT
	user32.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	dc, _, _ := user32.NewProc("GetWindowDC").Call(hwnd)
	defer user32.NewProc("ReleaseDC").Call(hwnd, dc)
	mem, _, _ := gdi32.NewProc("CreateCompatibleDC").Call(dc)
	defer gdi32.NewProc("DeleteDC").Call(mem)
	bitmap, _, _ := gdi32.NewProc("CreateCompatibleBitmap").Call(dc, uintptr(w), uintptr(h))
	defer procDeleteObject.Call(bitmap)
	old, _, _ := procSelectObject.Call(mem, bitmap)
	if ok, _, _ := user32.NewProc("PrintWindow").Call(hwnd, mem, 0); ok == 0 {
		t.Fatal("PrintWindow failed")
	}
	procSelectObject.Call(mem, old)
	header := struct {
		Size                   uint32
		Width, Height          int32
		Planes, Bits           uint16
		Compression, ImageSize uint32
		XPels, YPels           int32
		Used, Important        uint32
	}{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
	pixels := make([]byte, w*h*4)
	if n, _, _ := gdi32.NewProc("GetDIBits").Call(dc, bitmap, 0, uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&header)), 0); n == 0 {
		t.Fatal("GetDIBits failed")
	}
	for i := 0; i < len(pixels); i += 4 {
		pixels[i], pixels[i+2], pixels[i+3] = pixels[i+2], pixels[i], 255
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = png.Encode(file, &image.RGBA{Pix: pixels, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}); err != nil {
		t.Fatal(err)
	}
	t.Log(path)
}
