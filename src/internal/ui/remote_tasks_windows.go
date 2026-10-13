//go:build windows

package ui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/detect"
	"autotrigger/internal/datadir"
	"autotrigger/internal/remotetask"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const idRemoteTasks = 5902

func remoteStateText(state string) string {
	return map[string]string{"staging": "接收文件中", "queued": "排队中", "running": "运行中", "completed": "执行结束（查看结果）", "cancelled": "已取消", "interrupted": "已中断，未自动重试"}[state]
}
func (f *form) remotePolicy() (remotetask.Policy, error) {
	return savedRemotePolicy(f.dataLayout(), f.exeDir)
}
func savedRemotePolicy(layout datadir.Layout, exeDir string) (remotetask.Policy, error) {
	cfg, _, err := config.Load(layout.Config(), exeDir)
	if err != nil {
		return remotetask.Policy{}, err
	}
	cfg.Lock.Enabled = true
	b, err := json.Marshal(cfg)
	if err != nil {
		return remotetask.Policy{}, err
	}
	prefs, err := loadPreferences(filepath.Dir(layout.Config()))
	if err != nil {
		return remotetask.Policy{}, err
	}
	return remotetask.Policy{Config: b, Recursive: prefs.Recursive, NoArchive: prefs.NoArchive}, nil
}
func (f *form) startRemoteTasks() error {
	if f.remoteServer != nil {
		return nil
	}
	policy, err := f.remotePolicy()
	if err != nil {
		return err
	}
	var holder atomic.Pointer[remotetask.Queue]
	layout, exeDir := f.dataLayout(), f.exeDir
	server, err := remotetask.Listen(layout.Root, func(ctx context.Context, r remotetask.Request) remotetask.Response {
		queue := holder.Load()
		if queue == nil {
			return remotetask.Response{Error: "任务接收端正在初始化，请稍后重试"}
		}
		if r.Action == "submit" {
			policy, err := savedRemotePolicy(layout, exeDir)
			if err != nil {
				return remotetask.Response{Error: "已保存的设置不可用：" + err.Error()}
			}
			queue.Configure(policy)
		}
		return queue.HandleContext(ctx, r)
	})
	if err != nil {
		return err
	}
	queue, err := remotetask.New(f.dataLayout().Root, policy)
	if err != nil {
		server.Close()
		return err
	}
	holder.Store(queue)
	f.remoteQueue, f.remoteServer = queue, server
	f.remoteEnabled = true
	return nil
}
func (f *form) applyRemoteSetting() error {
	requested := f.isChecked(idRemoteTasks)
	var err error
	if !requested && f.remoteID != "" {
		err = fmt.Errorf("远程任务运行中，请先停止任务后关闭接收")
	} else if requested {
		err = f.startRemoteTasks()
		if err == nil {
			var policy remotetask.Policy
			policy, err = f.remotePolicy()
			if err == nil {
				f.remoteQueue.Configure(policy)
			}
		}
	} else if f.remoteServer != nil {
		f.remoteServer.Close()
		f.remoteServer = nil
		f.remoteQueue = nil
		f.remoteEnabled = false
	}
	if err != nil {
		checked := uintptr(0)
		if f.remoteEnabled {
			checked = bstChecked
		}
		send(f.ctrls[idRemoteTasks], bmSetCheck, checked, 0)
		_ = f.persistPreferences(f.dataLayout().Config())
	}
	return err
}

// Check the actual input desktop and this executor's session, not the SSH session.
func interactiveDesktopReady() error {
	var session uint32
	ok, _, err := kernel32.NewProc("ProcessIdToSessionId").Call(uintptr(os.Getpid()), uintptr(unsafe.Pointer(&session)))
	if ok == 0 || session == 0 {
		return fmt.Errorf("执行端不在交互用户会话：%v", err)
	}
	wts := syscall.NewLazyDLL("wtsapi32.dll")
	var data unsafe.Pointer
	var size uint32
	ok, _, err = wts.NewProc("WTSQuerySessionInformationW").Call(0, uintptr(session), 8, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return fmt.Errorf("无法核实桌面会话：%v", err)
	}
	state := uint32(1)
	if data != nil && size >= 4 {
		state = *(*uint32)(data)
	}
	wts.NewProc("WTSFreeMemory").Call(uintptr(data))
	if state != 0 {
		return fmt.Errorf("桌面会话未连接，等待恢复")
	}
	desktop, _, err := user32.NewProc("OpenInputDesktop").Call(0, 0, 0x101)
	if desktop == 0 {
		return fmt.Errorf("桌面已锁定或权限确认中：%v", err)
	}
	defer user32.NewProc("CloseDesktop").Call(desktop)
	var name [128]uint16
	var required uint32
	ok, _, err = user32.NewProc("GetUserObjectInformationW").Call(desktop, 2, uintptr(unsafe.Pointer(&name[0])), unsafe.Sizeof(name), uintptr(unsafe.Pointer(&required)))
	if ok == 0 || syscall.UTF16ToString(name[:]) != "Default" {
		return fmt.Errorf("当前不是可交互的默认桌面：%v", err)
	}
	return nil
}

func (f *form) syncRemoteHistory(jobs []remotetask.Job) {
	changed := false
	for _, job := range jobs {
		if job.State == "staging" {
			continue
		}
		index := -1
		for i, record := range f.history {
			if record.RemoteID == job.ID {
				index = i
				break
			}
		}
		if index >= 0 && f.history[index].Input == job.Source && f.history[index].RemoteState == job.State && f.history[index].RemoteReason == job.Reason && sameExit(f.history[index].ExitCode, job.ExitCode) {
			continue
		}
		var record taskRecord
		if index >= 0 {
			record = f.history[index]
		} else {
			record = taskRecord{ID: job.ID, RemoteID: job.ID, Name: job.Name, Input: job.Source, Result: job.Result, LogPath: job.Log, Config: job.Config, Created: job.Created}
			if stat, err := os.Stat(job.Input); err == nil {
				size := stat.Size()
				record.FileSize = &size
				if ft, err := (detect.Detector{}).Detect(job.Input); err == nil {
					record.FileType = ft.Kind
				}
			}
		}
		record.Input = job.Source
		record.RemoteState, record.RemoteReason, record.ExitCode = job.State, job.Reason, job.ExitCode
		record.RemoteStarted, record.RemoteFinished = job.Started, job.Finished
		if index >= 0 {
			f.history[index] = record
		} else {
			f.history = append(f.history, record)
		}
		changed = true
	}
	if changed {
		f.persistHistory()
		procInvalidateRect.Call(f.hwnd, 0, 0)
	}
}
func sameExit(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (f *form) pollRemoteTasks() {
	if f.remoteQueue == nil {
		return
	}
	jobs := f.remoteQueue.List()
	f.syncRemoteHistory(jobs)
	if f.remoteID != "" {
		if f.isRunning() && !f.remoteCancelApplied {
			if err := interactiveDesktopReady(); err != nil {
				if e := f.remoteQueue.Interrupt(f.remoteID, "桌面不可交互，已停止任务："+err.Error()); e != nil {
					f.appendLog([]byte("远程任务中断状态保存失败：" + e.Error() + "\n"))
				}
				f.stopRun()
				f.remoteCancelApplied = true
			}
		}
		for _, job := range jobs {
			if job.ID == f.remoteID && job.CancelRequested && !f.remoteCancelApplied {
				f.stopRun()
				f.remoteCancelApplied = true
			}
		}
		return
	}
	if f.isRunning() || f.runAwaitingDone || f.activeTask != "" {
		f.remoteQueue.Waiting("正在执行其他任务，等待空闲")
		return
	}
	if activeHover != nil || hookInput.Load() != nil {
		f.remoteQueue.Waiting("模板捕获中，等待捕获结束")
		return
	}
	if err := interactiveDesktopReady(); err != nil {
		f.remoteQueue.Waiting(err.Error())
		return
	}
	f.remoteQueue.Waiting("")
	job, ok, err := f.remoteQueue.Next()
	if err != nil {
		f.setStatusKind("任务队列无法更新："+err.Error(), statusKindFail)
		return
	}
	if !ok {
		return
	}
	cfg, _, err := config.Load(job.Config, f.exeDir)
	if err != nil {
		_ = f.remoteQueue.Complete(job.ID, 3, "配置快照不可用："+err.Error())
		return
	}
	opts := Options{Input: job.Input, Output: job.Result, Config: job.Config, Recursive: job.Recursive, NoArchive: job.NoArchive, Lock: true, TimeoutSec: cfg.TimeoutSec, LogLevel: cfg.Output.LogLevel}
	args, err := opts.Args()
	if err != nil {
		_ = f.remoteQueue.Complete(job.ID, 2, err.Error())
		return
	}
	if err = f.dataLayout().Ensure(); err != nil {
		_ = f.remoteQueue.Complete(job.ID, 1, err.Error())
		return
	}
	args = append(args, "--log-file", job.Log, "--lock")
	f.remoteID = job.ID
	f.remoteCancelApplied = false
	f.activeTask = job.ID
	f.resultPath = job.Result
	for i := range f.history {
		if f.history[i].RemoteID == job.ID {
			f.history[i].ID = job.ID
			f.history[i].Execution = &opts
			f.history[i].RemoteState = "running"
			f.history[i].RemoteStarted = job.Started
		}
	}
	f.persistHistory()
	f.launch("远程任务："+job.Name, args, false)
}
