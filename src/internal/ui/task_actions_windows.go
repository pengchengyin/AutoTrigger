//go:build windows

package ui

import "os"

func (f *form) selectedTaskRecord() (taskRecord, bool) {
	path := f.selectedResult
	if path == "" {
		path = f.resultPath
	}
	for _, record := range f.history {
		if record.Result == path {
			return record, true
		}
	}
	return taskRecord{}, false
}

func (f *form) selectedTaskRunning() bool {
	record, ok := f.selectedTaskRecord()
	return ok && record.ID == f.activeTask && f.isRunning() && !f.aux
}

func (f *form) updateDetailButtons() {
	record, ok := f.selectedTaskRecord()
	enableWindow(f.ctrls[idDetailRetry], ok && record.Input != "" && !f.isRunning())
	if ok && (record.RemoteState == "queued" || record.RemoteState == "staging" || record.Origin == "cli" && record.ExitCode == nil) {
		enableWindow(f.ctrls[idDetailRetry], false)
	}
	enableWindow(f.ctrls[idDetailStop], f.selectedTaskRunning() || ok && record.RemoteState == "queued" && f.remoteQueue != nil)
}

func (f *form) retrySelectedTask() {
	if f.isRunning() {
		return
	}
	record, ok := f.selectedTaskRecord()
	if !ok || record.Input == "" {
		return
	}
	if _, err := os.Stat(record.Input); err != nil {
		messageBox(f.hwnd, "无法重新安装", "原任务的文件或目录无法访问：\n"+err.Error(), mbOK|mbIconError)
		return
	}
	opts := retryOptions(record, f.currentOptions())
	previousTask := f.activeTask
	name := windowText(f.ctrls[idTaskName])
	setWindowTextRaw(f.ctrls[idTaskName], record.Name)
	f.runTaskOptions(opts)
	setWindowTextRaw(f.ctrls[idTaskName], name)
	if f.activeTask != previousTask {
		f.selectedResult = f.resultPath
		f.refreshResultView()
		f.layout()
	}
}
