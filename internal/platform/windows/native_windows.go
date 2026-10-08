//go:build windows

// Package windows implements the Windows 7-compatible platform ports.
package windows

import (
	"autotrigger/internal/core/ports"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var user32 = syscall.NewLazyDLL("user32.dll")
var advapi32 = syscall.NewLazyDLL("advapi32.dll")
var ole32 = syscall.NewLazyDLL("ole32.dll")
var gdi32 = syscall.NewLazyDLL("gdi32.dll")

type Buffer struct {
	mu sync.Mutex
	b  []byte
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(b.b) < 4096 {
		remain := 4096 - len(b.b)
		if len(p) > remain {
			p = p[:remain]
		}
		b.b = append(b.b, p...)
	}
	return n, nil
}
func (b *Buffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.b) }
func utf(s string) *uint16       { p, _ := syscall.UTF16PtrFromString(s); return p }

// nativeAddress reinterprets Win32 return addresses pointing to foreign memory.
func nativeAddress(value uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&value)) }
func callOK(p *syscall.LazyProc, args ...uintptr) error {
	r, _, e := p.Call(args...)
	if r == 0 {
		if e == syscall.Errno(0) {
			return fmt.Errorf("%s returned false", p.Name)
		}
		return fmt.Errorf("%s: %w", p.Name, e)
	}
	return nil
}

type Runner struct{}
type process struct {
	cmd         *exec.Cmd
	job         syscall.Handle
	handle      syscall.Handle
	done        chan ports.Exit
	out, errout *Buffer
	once        sync.Once
	closed      sync.Once
}
type jobLimits struct {
	PerProcess    int64
	PerJob        int64
	Flags         uint32
	Pad           uint32
	MinWorking    uintptr
	MaxWorking    uintptr
	Active        uint32
	Pad2          uint32
	Affinity      uintptr
	Priority      uint32
	Scheduling    uint32
	IO            [6]uint64
	ProcessMemory uintptr
	JobMemory     uintptr
	PeakProcess   uintptr
	PeakJob       uintptr
}

func (Runner) Start(ctx context.Context, c ports.Command) (ports.Process, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if strings.ContainsRune(c.Program, 0) {
		return nil, fmt.Errorf("invalid program path")
	}
	job, _, err := kernel32.NewProc("CreateJobObjectW").Call(0, 0)
	if job == 0 {
		return nil, err
	}
	limits := jobLimits{Flags: 0x2000}
	if err = callOK(kernel32.NewProc("SetInformationJobObject"), job, 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits)); err != nil {
		syscall.CloseHandle(syscall.Handle(job))
		return nil, err
	}
	p := &process{job: syscall.Handle(job), done: make(chan ports.Exit, 1), out: &Buffer{}, errout: &Buffer{}}
	cmd := exec.Command(c.Program, c.Args...)
	cmd.Dir = c.Dir
	cmd.Stdin = c.Stdin
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr
	if cmd.Stdout == nil {
		cmd.Stdout = p.out
	}
	if cmd.Stderr == nil {
		cmd.Stderr = p.errout
	}
	cmd.WaitDelay = 500 * time.Millisecond
	// Suspend before user code runs so child processes cannot escape job assignment.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x4 | 0x200, HideWindow: false, CmdLine: c.CommandLine}
	p.cmd = cmd
	if err = cmd.Start(); err != nil {
		syscall.CloseHandle(p.job)
		return nil, err
	}
	h, e := syscall.OpenProcess(0x1f0fff, false, uint32(cmd.Process.Pid))
	if e != nil {
		cmd.Process.Kill()
		cmd.Wait()
		syscall.CloseHandle(p.job)
		return nil, e
	}
	p.handle = h
	if err = callOK(kernel32.NewProc("AssignProcessToJobObject"), job, uintptr(h)); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		p.Close()
		return nil, fmt.Errorf("job assignment (nested jobs on Win7 are unsupported): %w", err)
	}
	status, _, _ := syscall.NewLazyDLL("ntdll.dll").NewProc("NtResumeProcess").Call(uintptr(h))
	if int32(status) < 0 {
		p.KillTree()
		cmd.Wait()
		p.Close()
		return nil, fmt.Errorf("resume process: NTSTATUS %x", status)
	}
	go func() {
		waitErr := cmd.Wait()
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		if code >= 0 {
			waitErr = nil
		}
		for {
			ids, e := p.pids()
			if e != nil || len(ids) == 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		p.done <- ports.Exit{Code: code, Err: waitErr, Stdout: p.out.String(), Stderr: p.errout.String()}
		close(p.done)
	}()
	return p, nil
}
func (p *process) PID() int                { return p.cmd.Process.Pid }
func (p *process) Done() <-chan ports.Exit { return p.done }
func (p *process) pids() ([]int, error) {
	buf := make([]uintptr, 4098)
	r, _, e := kernel32.NewProc("QueryInformationJobObject").Call(uintptr(p.job), 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))*unsafe.Sizeof(buf[0]), 0)
	if r == 0 {
		return nil, e
	}
	n := uint32(buf[0] >> 32)
	if int(n) > len(buf)-1 {
		n = uint32(len(buf) - 1)
	}
	ids := make([]int, 0, n)
	for i := 0; i < int(n); i++ {
		ids = append(ids, int(buf[1+i]))
	}
	return ids, nil
}
func (p *process) Activity() (ports.Activity, error) {
	a := ports.Activity{}
	ids, err := p.pids()
	if err != nil {
		return a, err
	}
	a.PIDs = ids
	for _, id := range ids {
		h, err := syscall.OpenProcess(0x400, false, uint32(id))
		if err != nil {
			continue
		}
		var creation, exit, kernel, user syscall.Filetime
		ok, _, _ := kernel32.NewProc("GetProcessTimes").Call(uintptr(h), uintptr(unsafe.Pointer(&creation)), uintptr(unsafe.Pointer(&exit)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
		if ok != 0 {
			a.CPUTimeMS += ((uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) + (uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))) / 10000
		}
		var io [6]uint64
		ok, _, _ = kernel32.NewProc("GetProcessIoCounters").Call(uintptr(h), uintptr(unsafe.Pointer(&io[0])))
		if ok != 0 {
			a.IOBytes += io[3] + io[4] + io[5]
		}
		syscall.CloseHandle(h)
	}
	return a, nil
}
func (p *process) KillTree() error {
	return callOK(kernel32.NewProc("TerminateJobObject"), uintptr(p.job), 1)
}
func (p *process) Terminate(ctx context.Context) (string, error) {
	ids, _ := p.pids()
	pidSet := map[int]bool{}
	for _, id := range ids {
		pidSet[id] = true
	}
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		var id uint32
		user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&id)))
		if pidSet[int(id)] {
			user32.NewProc("PostMessageW").Call(hwnd, 0x10, 0, 0)
		}
		return 1
	})
	user32.NewProc("EnumWindows").Call(cb, 0)
	kernel32.NewProc("GenerateConsoleCtrlEvent").Call(1, uintptr(p.PID()))
	if waitEmpty(ctx, p, 3*time.Second) {
		return "graceful", nil
	}
	kernel32.NewProc("TerminateProcess").Call(uintptr(p.handle), 1)
	if waitEmpty(ctx, p, 3*time.Second) {
		return "terminated", nil
	}
	return "killed_tree", p.KillTree()
}
func waitEmpty(ctx context.Context, p *process, d time.Duration) bool {
	until := time.Now().Add(d)
	for time.Now().Before(until) {
		ids, e := p.pids()
		if e != nil || len(ids) == 0 {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
	return false
}
func (p *process) Close() error {
	p.closed.Do(func() {
		if p.job != 0 {
			syscall.CloseHandle(p.job)
		}
		if p.handle != 0 {
			syscall.CloseHandle(p.handle)
		}
	})
	return nil
}

type MachineLock struct{}

func (MachineLock) Acquire(ctx context.Context, wait time.Duration) (func(), error) {
	runtime.LockOSThread()
	name := utf("Global\\AutoTrigger.Installation.Lock.v1")
	h, _, err := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		runtime.UnlockOSThread()
		return nil, err
	}
	start := time.Now()
	for {
		r, _, _ := kernel32.NewProc("WaitForSingleObject").Call(h, 0)
		if r == 0 || r == 0x80 {
			return func() {
				kernel32.NewProc("ReleaseMutex").Call(h)
				syscall.CloseHandle(syscall.Handle(h))
				runtime.UnlockOSThread()
			}, nil
		}
		if ctx.Err() != nil {
			syscall.CloseHandle(syscall.Handle(h))
			runtime.UnlockOSThread()
			return nil, ctx.Err()
		}
		if time.Since(start) >= wait {
			syscall.CloseHandle(syscall.Handle(h))
			runtime.UnlockOSThread()
			return nil, ports.ErrBusy
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func Helper(ctx context.Context, runner ports.ProcessRunner, cmd ports.Command) (ports.Exit, error) {
	p, err := runner.Start(ctx, cmd)
	if err != nil {
		return ports.Exit{}, err
	}
	defer p.Close()
	select {
	case e := <-p.Done():
		if e.Code != 0 {
			return e, fmt.Errorf("%s exited %d: %s", cmd.Program, e.Code, e.Stderr)
		}
		return e, e.Err
	case <-ctx.Done():
		p.KillTree()
		select {
		case <-p.Done():
		case <-time.After(time.Second):
		}
		return ports.Exit{}, ctx.Err()
	}
}
func systemExe(name string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = "C:\\Windows"
	}
	return root + "\\System32\\" + name
}
