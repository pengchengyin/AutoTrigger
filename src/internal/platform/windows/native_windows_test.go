//go:build windows

package windows

import (
	"autotrigger/internal/core/ports"
	"autotrigger/internal/procutil"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("AT_NATIVE_TEST_HELPER")
	if mode == "" {
		return
	}
	if mode == "console-check" {
		h, _, _ := kernel32.NewProc("GetConsoleWindow").Call()
		fmt.Printf("console=%d", h)
		os.Exit(0)
	}
	if mode == "child" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	if mode == "parent" {
		executable, _ := os.Executable()
		child := exec.Command(executable, "-test.run=^TestProcessHelper$")
		child.Env = append(os.Environ(), "AT_NATIVE_TEST_HELPER=child")
		if err := child.Start(); err != nil {
			os.Exit(8)
		}
		time.Sleep(300 * time.Millisecond)
		os.Exit(0)
	}
}
func TestSuspendedJobCapturesOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exit, err := procutil.Run(ctx, Runner{}, ports.Command{Program: systemExe("cmd.exe"), Args: []string{"/d", "/c", "echo native-output"}})
	if err != nil {
		t.Fatal(err)
	}
	if exit.Code != 0 || !strings.Contains(exit.Stdout, "native-output") {
		t.Fatal(exit)
	}
}

func TestHiddenHelperCapturesOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exit, err := procutil.Run(ctx, Runner{}, ports.Command{Program: systemExe("cmd.exe"), Args: []string{"/d", "/c", "echo hidden-output"}, HideWindow: true})
	if err != nil || exit.Code != 0 || !strings.Contains(exit.Stdout, "hidden-output") {
		t.Fatal(exit, err)
	}
}

func TestHiddenHelperHasNoConsole(t *testing.T) {
	t.Setenv("AT_NATIVE_TEST_HELPER", "console-check")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exit, err := procutil.Run(ctx, Runner{}, ports.Command{Program: exe, Args: []string{"-test.run=^TestProcessHelper$"}, HideWindow: true})
	if err != nil || exit.Code != 0 || exit.Stdout != "console=0" {
		t.Fatal(exit, err)
	}
}
func TestJobKeepsChildrenAfterParentExit(t *testing.T) {
	t.Setenv("AT_NATIVE_TEST_HELPER", "parent")
	executable, _ := os.Executable()
	p, err := (Runner{}).Start(context.Background(), ports.Command{Program: executable, Args: []string{"-test.run=^TestProcessHelper$"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	defer p.KillTree()
	time.Sleep(800 * time.Millisecond)
	select {
	case <-p.Done():
		t.Fatal("parent exit discarded still-running child")
	default:
	}
	a, err := p.Activity()
	if err != nil || len(a.PIDs) < 1 {
		t.Fatalf("child not tracked: %+v %v", a, err)
	}
	if err = p.KillTree(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("job termination did not finish")
	}
	a, err = p.Activity()
	if err == nil && len(a.PIDs) != 0 {
		t.Fatal("process tree leaked", a)
	}
}
func TestGlobalMutexRejectsSecondInstance(t *testing.T) {
	release, err := (MachineLock{}).Acquire(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	go func() {
		unlock, err := (MachineLock{}).Acquire(context.Background(), 0)
		if unlock != nil {
			unlock()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != ports.ErrBusy {
			t.Fatalf("expected busy, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("mutex contention hung")
	}
}
