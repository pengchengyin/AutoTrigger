//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/datadir"
	"autotrigger/internal/platform/windows"
	"autotrigger/internal/remotetask"
	"autotrigger/internal/resources"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func newRemoteTestForm(t *testing.T, deps cli.Dependencies) *form {
	t.Helper()
	f := newForm(deps)
	f.exeDir = t.TempDir()
	f.dataDir = t.TempDir()
	f.dataError = nil
	f.remoteEnabled = false
	f.history = nil
	f.historyDir = f.dataLayout().History()
	classID, _ := remotetask.NewID()
	f.className = utf16Ptr("AutoTriggerRemoteTest" + classID)
	if err := f.create(); err != nil {
		t.Fatal(err)
	}
	if !f.saveSettings() {
		t.Fatal(windowText(f.ctrls[idStatus]))
	}
	send(f.ctrls[idRemoteTasks], bmSetCheck, bstChecked, 0)
	if !f.saveSettings() {
		t.Fatal(windowText(f.ctrls[idStatus]))
	}
	return f
}

func TestWindowsRemoteTasksQueueHistoryAndSavedPolicy(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := newRemoteTestForm(t, cli.Dependencies{})
	defer procDestroyWindow.Call(f.hwnd)
	if f.remoteServer == nil {
		t.Fatal("receiver not started")
	}
	input := filepath.Join(remotetask.Inbox(f.dataDir), "queued.exe")
	os.WriteFile(input, []byte("test"), 0600)
	id, _ := remotetask.NewID()
	response, err := remotetask.Call(context.Background(), f.dataDir, remotetask.Request{Action: "submit", ID: id, Input: input, Name: "SSH queued task"})
	if err != nil || !response.OK {
		t.Fatal(response, err)
	}
	f.running = true
	f.pollRemoteTasks()
	f.running = false
	if len(f.history) != 1 || f.history[0].RemoteID != id || f.history[0].RemoteState != "queued" {
		t.Fatal(f.history)
	}
	f.page = pageTaskDetail
	f.selectedResult = f.history[0].Result
	if snap, ok := f.resultSnapshot(); !ok || !snap.StartedAt.IsZero() {
		t.Fatal("queued submission time presented as execution start", snap)
	}
	saved, _, err := config.Load(response.Job.Config, f.exeDir)
	if err != nil || !saved.OCR.Enabled || !saved.Lock.Enabled {
		t.Fatal("remote task did not inherit OCR or machine lock", saved, err)
	}
	saved.OCR.Enabled = false
	body, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(f.dataLayout().Config(), body, 0600); err != nil {
		t.Fatal(err)
	}
	secondID, _ := remotetask.NewID()
	second, err := remotetask.Call(context.Background(), f.dataDir, remotetask.Request{Action: "submit", ID: secondID, Input: input})
	if err != nil || !second.OK {
		t.Fatal(second, err)
	}
	updated, _, err := config.Load(second.Job.Config, f.exeDir)
	if err != nil || updated.OCR.Enabled {
		t.Fatal("saved advanced settings not reloaded on submit", updated, err)
	}
	if _, err = f.remoteQueue.Cancel(secondID); err != nil {
		t.Fatal(err)
	}
	setWindowTextRaw(f.ctrls[idOutputDir], t.TempDir())
	if err = f.applyDataDirectory(); err == nil {
		t.Fatal("data root changed under receiver")
	}
	setWindowTextRaw(f.ctrls[idOutputDir], f.dataDir)
	if _, err = f.remoteQueue.Cancel(id); err != nil {
		t.Fatal(err)
	}
	f.pollRemoteTasks()
	if f.history[0].RemoteState != "cancelled" {
		t.Fatal(f.history)
	}
	f.selectedResult = f.history[0].Result
	f.page = pageTaskDetail
	if snap, ok := f.resultSnapshot(); !ok || !strings.Contains(snap.PresentationState, "已取消") {
		t.Fatal(snap)
	}
	send(f.ctrls[idRemoteTasks], bmSetCheck, 0, 0)
	if !f.saveSettings() {
		t.Fatal(windowText(f.ctrls[idStatus]))
	}
	if f.remoteServer != nil {
		t.Fatal("receiver not stopped")
	}
}

// Executes only a self-created batch file which writes a marker in t.TempDir.
func TestWindowsRemoteTaskEndToEnd(t *testing.T) {
	if os.Getenv("AT_REMOTE_TASK_SMOKE") != "1" {
		t.Skip("set AT_REMOTE_TASK_SMOKE=1 for interactive desktop task smoke")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := interactiveDesktopReady(); err != nil {
		t.Skipf("desktop unavailable: %v", err)
	}
	runner := windows.Runner{}
	deps := cli.Dependencies{Runner: runner, Trigger: func(c config.Config, p resources.Pack) ports.InstallerTrigger {
		return windows.Trigger{Config: c, Pack: p}
	}, Opener: windows.Opener{}, Mounter: windows.Mounter{Runner: runner}, Evidence: windows.Evidence{}, NativeGUI: windows.GUI{}, Lock: windows.MachineLock{}, Prereqs: &windows.Prereqs{Runner: runner, ExeDir: t.TempDir()}}
	f := newRemoteTestForm(t, deps)
	defer procDestroyWindow.Call(f.hwnd)
	cfg := config.Defaults()
	cfg.TimeoutSec = 5
	cfg.GUI.Enabled = false
	cfg.OCR.Enabled = false
	cfg.Verify.WatchDirs = []string{t.TempDir()}
	cfg.MaxAttemptsPerFile = 1
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(f.dataLayout().Config(), body, 0600); err != nil {
		t.Fatal(err)
	}
	f.remoteQueue.Configure(remotetask.Policy{Config: body})
	marker := filepath.Join(t.TempDir(), "executed.txt")
	input := filepath.Join(remotetask.Inbox(f.dataDir), "remote-smoke.cmd")
	if err = os.WriteFile(input, []byte("@echo off\r\necho remote-ok>\""+marker+"\"\r\nexit /b 0\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	id, _ := remotetask.NewID()
	response, err := remotetask.Call(context.Background(), f.dataDir, remotetask.Request{Action: "submit", ID: id, Input: input})
	if err != nil || !response.OK {
		t.Fatal(response, err)
	}
	f.pollRemoteTasks()
	pumpCaptureUntil(t, func() bool { return !f.isRunning() && !f.runAwaitingDone })
	f.pollRemoteTasks()
	response, err = remotetask.Call(context.Background(), f.dataDir, remotetask.Request{Action: "status", ID: id})
	if err != nil || response.Job.State != "completed" || response.Job.ExitCode == nil {
		t.Fatal(response, err)
	}
	if b, err := os.ReadFile(marker); err != nil || !strings.Contains(string(b), "remote-ok") {
		t.Fatal("submitted task did not execute real CLI workflow", err)
	}
	result := f.remoteQueue.Handle(remotetask.Request{Action: "result", ID: id})
	if !result.OK {
		t.Fatal(result)
	}
	log := f.remoteQueue.Handle(remotetask.Request{Action: "logs", ID: id})
	if !log.OK || !strings.Contains(log.Logs, "任务开始") {
		t.Fatal(log)
	}
	if len(f.history) != 1 || f.history[0].RemoteID != id || f.history[0].ExitCode == nil {
		t.Fatal(f.history)
	}
	id, _ = remotetask.NewID()
	response, err = remotetask.Call(context.Background(), f.dataDir, remotetask.Request{Action: "submit", ID: id, Input: input})
	if err != nil || !response.OK {
		t.Fatal(response, err)
	}
	f.pollRemoteTasks()
	response, err = remotetask.Call(context.Background(), f.dataDir, remotetask.Request{Action: "cancel", ID: id})
	if err != nil || !response.OK {
		t.Fatal(response, err)
	}
	f.pollRemoteTasks()
	pumpCaptureUntil(t, func() bool { return !f.isRunning() && !f.runAwaitingDone })
	response = f.remoteQueue.Handle(remotetask.Request{Action: "status", ID: id})
	if response.Job.State != "cancelled" {
		t.Fatal("running cancellation did not persist", response)
	}
}

func TestWindowsRemoteExecutableClient(t *testing.T) {
	tool := os.Getenv("AT_REMOTE_CLIENT_TOOL")
	if tool == "" {
		t.Skip("set AT_REMOTE_CLIENT_TOOL to the built AutoTrigger.exe")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := newRemoteTestForm(t, cli.Dependencies{})
	defer procDestroyWindow.Call(f.hwnd)
	body, err := os.ReadFile(tool)
	if err != nil {
		t.Fatal(err)
	}
	client := filepath.Join(f.exeDir, "AutoTrigger.exe")
	if err = os.WriteFile(client, body, 0700); err != nil {
		t.Fatal(err)
	}
	if err = datadir.Save(f.exeDir, f.dataDir); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(remotetask.Inbox(f.dataDir), "client-upload.exe")
	os.WriteFile(input, []byte("fixture"), 0600)
	id, _ := remotetask.NewID()
	for _, args := range [][]string{{"task", "health"}, {"task", "submit", "--id", id, "--input", input}, {"task", "status", "--id", id}, {"task", "cancel", "--id", id}} {
		command := exec.Command(client, args...)
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		data, err := command.CombinedOutput()
		if err != nil {
			t.Fatal(args, string(data), err)
		}
		var response remotetask.Response
		if err = json.Unmarshal(data, &response); err != nil || !response.OK {
			t.Fatal(string(data), err)
		}
	}
	f.pollRemoteTasks()
	if len(f.history) != 1 || f.history[0].RemoteState != "cancelled" {
		t.Fatal(f.history)
	}
}
