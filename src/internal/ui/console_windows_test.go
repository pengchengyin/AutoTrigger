//go:build windows

package ui

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestGUIDetachesConsoleWithoutClosingParent(t *testing.T) {
	if os.Getenv("AUTOTRIGGER_CONSOLE_CHILD") == "1" {
		if consoleProcessCount() == 0 {
			os.Exit(10)
		}
		if detachConsole() != nil {
			os.Exit(11)
		}
		if consoleProcessCount() != 0 {
			os.Exit(12)
		}
		if h, _, _ := procGetConsoleWindow.Call(); h != 0 {
			os.Exit(13)
		}
		if detachConsole() != nil {
			os.Exit(14)
		}
		os.Exit(0)
	}
	before, _, _ := procGetConsoleWindow.Call()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := exec.Command(exe, "-test.run=^TestGUIDetachesConsoleWithoutClosingParent$")
	c.Env = append(os.Environ(), "AUTOTRIGGER_CONSOLE_CHILD=1")
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x10}
	if err = c.Run(); err != nil {
		t.Fatal(err)
	}
	after, _, _ := procGetConsoleWindow.Call()
	if before != after {
		t.Fatal("parent terminal changed")
	}
}
