//go:build windows

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunningDetailOverridesIntermediateResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(path, []byte(`{"task_id":"active","summary":{"total":0,"by_status":{}},"results":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	f := &form{page: pageTaskDetail, selectedResult: path, activeTask: "active", running: true,
		history: []taskRecord{{ID: "active", Result: path, Created: time.Now()}},
		logBuf:  []byte("旧任务结论\n环境检查完成\n"), taskLogBuf: []byte("正在执行安装\n")}
	snap, ok := f.resultSnapshot()
	if !ok || snap.PresentationState != "安装 / 触发进行中" || outcomeKind(snap) != "run" {
		t.Fatalf("intermediate snapshot lost running state: %+v", snap)
	}
	if got := f.resultViewText(); got != "正在执行安装\r\n" {
		t.Fatalf("detail did not show task-only log: %q", got)
	}
	f.appendLog([]byte("自动点击下一步\n"))
	if got := f.resultViewText(); !strings.Contains(got, "自动点击下一步") || strings.Contains(got, "总体结论") || strings.Contains(got, "旧任务") {
		t.Fatalf("live log did not refresh: %q", got)
	}
	f.running = false
	snap, ok = f.resultSnapshot()
	if !ok || snap.PresentationState != "" || !strings.Contains(f.resultViewText(), "总体结论") {
		t.Fatal("finished task did not restore result report")
	}
}

func TestOldDetailDoesNotShowActiveTaskLogs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.json")
	f := &form{page: pageTaskDetail, selectedResult: path, activeTask: "other", running: true,
		history: []taskRecord{{ID: "old", Result: path}}, taskLogBuf: []byte("另一条任务正在安装")}
	if f.selectedTaskRunning() || strings.Contains(f.resultViewText(), "另一条任务") {
		t.Fatal("old detail leaked active task logs")
	}
}
