//go:build windows

package windows

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/procutil"
	"autotrigger/internal/resources"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestD08RelocatedUnicodeShortcut(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "links")
	os.MkdirAll(dir, 0755)
	target := filepath.Join(root, "安装.exe")
	os.WriteFile(target, []byte("MZ"), 0600)
	link := filepath.Join(dir, "安装.lnk")
	b := make([]byte, 76)
	binary.LittleEndian.PutUint32(b, 76)
	copy(b[4:], []byte{1, 0x14, 2, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46})
	binary.LittleEndian.PutUint32(b[20:], 8|128)
	words, _ := syscall.UTF16FromString(`..\安装.exe`)
	words = words[:len(words)-1]
	buf := make([]byte, 2+len(words)*2)
	binary.LittleEndian.PutUint16(buf, uint16(len(words)))
	for i, w := range words {
		binary.LittleEndian.PutUint16(buf[2+i*2:], w)
	}
	os.WriteFile(link, append(b, buf...), 0600)
	relative, err := shortcutRelative(link)
	if err != nil {
		t.Fatal(err)
	}
	if got := relocatedShortcutTarget(link, `C:\nonexistent\安装.exe`, relative); got != target {
		t.Fatalf("%q", got)
	}
	got, _, err := (Opener{}).ResolveShortcut(link)
	if err != nil || got != target {
		t.Fatalf("COM target %q %v", got, err)
	}
	os.WriteFile(link, b[:30], 0600)
	if _, err = shortcutRelative(link); err == nil {
		t.Fatal("truncated link accepted")
	}
}

func TestD20SilentParametersIncludeShortSAndRespectLimit(t *testing.T) {
	c := config.Defaults()
	trigger := Trigger{Config: c, Pack: resources.Pack{Mode: "builtin"}}
	commands, err := trigger.Commands("setup.exe", ports.FileType{Kind: "pe_executable"}, t.TempDir())
	if err != nil || len(commands) != 9 || len(commands[7].Args) != 1 || commands[7].Args[0] != "-s" || commands[8].Method != "no_param_run" {
		t.Fatalf("%+v %v", commands, err)
	}
	trigger.Config.MaxAttemptsPerFile = 8
	commands, err = trigger.Commands("setup.exe", ports.FileType{Kind: "pe_executable"}, t.TempDir())
	if err != nil || len(commands) != 8 || commands[7].Method != "no_param_run" {
		t.Fatalf("%+v %v", commands, err)
	}
}

func TestD12UTF16BatchRunsWithoutChangingOriginal(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "script.bat")
	text := "@echo off\r\necho UTF16-BATCH>\"%~dp0sentinel.txt\"\r\n"
	words, _ := syscall.UTF16FromString(text)
	b := []byte{0xff, 0xfe}
	for _, w := range words[:len(words)-1] {
		b = append(b, byte(w), byte(w>>8))
	}
	os.WriteFile(path, b, 0600)
	trigger := Trigger{Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
	commands, err := trigger.Commands(path, ports.FileType{Kind: "script"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exit, err := procutil.Run(ctx, Runner{}, commands[0])
	if err != nil || exit.Code != 0 {
		t.Fatalf("%+v %v", exit, err)
	}
	if _, err = os.Stat(filepath.Join(root, "sentinel.txt")); err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(path)
	if string(current) != string(b) {
		t.Fatal("original batch changed")
	}
}

func TestD09ExtensionlessPEUsesExactExistingPath(t *testing.T) {
	exe, _ := os.Executable()
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "无扩展名文件")
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	x, err := procutil.Run(ctx, Runner{}, ports.Command{Program: path, Args: []string{"-test.run=^$"}})
	if err != nil || x.Code != 0 {
		t.Fatalf("%+v %v", x, err)
	}
}
