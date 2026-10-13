//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTaskOutputUsesSelectedFiles(t *testing.T) {
	root := t.TempDir()
	result := filepath.Join(root, "old.json")
	log := filepath.Join(root, "custom.log")
	os.WriteFile(result, []byte(`{"large":9007199254740993,"nested":{"ok":true}}`), 0600)
	os.WriteFile(log, []byte("所选任务日志\n"), 0600)
	f := &form{page: pageTaskDetail, selectedResult: result, history: []taskRecord{{ID: "old", Result: result, LogPath: log}}, logBuf: []byte("其他任务日志"), taskLogBuf: []byte("当前任务日志")}
	if f.detailFilePath() != log || f.detailViewText() != "所选任务日志\r\n" {
		t.Fatal(f.detailViewText())
	}
	f.detailJSON = true
	text := f.detailViewText()
	if f.detailFilePath() != result || !strings.Contains(text, "9007199254740993") || !strings.Contains(text, "\r\n  \"nested\"") {
		t.Fatal(text)
	}
	f.detailJSON = false
	f.history[0].LogPath = ""
	if f.detailFilePath() != result+".log" {
		t.Fatal("legacy history used unrelated settings")
	}
}

func TestWindowsTaskOutputTabs(t *testing.T) {
	dir := os.Getenv("AUTOTRIGGER_UI_SHOTS")
	if dir == "" {
		t.Skip("set AUTOTRIGGER_UI_SHOTS")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()
	f := newForm(cli.Dependencies{})
	f.exeDir = t.TempDir()
	f.historyDir = filepath.Join(f.exeDir, "history")
	result := filepath.Join(f.exeDir, "task.json")
	os.WriteFile(result, []byte(`{"task_id":"selected","summary":{"total":1,"by_status":{"failed":1}},"results":[]}`), 0600)
	os.WriteFile(result+".log", []byte("所选任务：启动\n所选任务：执行中\n"), 0600)
	f.history = []taskRecord{{ID: "selected", Name: "日志标签回归", Result: result}}
	if err := f.create(); err != nil {
		t.Fatal(err)
	}
	defer user32.NewProc("DestroyWindow").Call(f.hwnd)
	f.show()
	f.selectedResult = result
	f.setPage(pageTaskDetail)
	os.MkdirAll(dir, 0755)
	captureNativeWindow(t, f.hwnd, filepath.Join(dir, "detail-execution-log.png"))
	send(f.hwnd, wmCommand, uintptr(idDetailJSONTab), 0)
	if !f.detailJSON || !strings.Contains(windowText(f.ctrls[idResultView]), `"task_id": "selected"`) {
		t.Fatal("JSON tab did not update")
	}
	captureNativeWindow(t, f.hwnd, filepath.Join(dir, "detail-result-json.png"))
	send(f.hwnd, wmCommand, uintptr(idResultHelp), 0)
	if f.helpWindow == 0 {
		t.Fatal("field help missing")
	}
	pane := helpPanes[f.helpWindow]
	setWindowTextRaw(pane.search, "schema_version")
	if !strings.Contains(windowText(pane.body), "schema_version") {
		t.Fatal("field search did not return matching field")
	}
	captureNativeWindow(t, f.helpWindow, filepath.Join(dir, "detail-field-help.png"))
	procDestroyWindow.Call(f.helpWindow)
	send(f.hwnd, wmCommand, uintptr(idDetailLogTab), 0)
	os.WriteFile(result+".log", []byte("所选任务：新增过程日志\n"), 0600)
	f.updateLiveViews()
	if f.detailJSON || !strings.Contains(windowText(f.ctrls[idResultView]), "新增过程日志") {
		t.Fatal("task log did not refresh")
	}
}
