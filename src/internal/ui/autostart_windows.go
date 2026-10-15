//go:build windows

package ui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
)

const idAutoStart = 5903

type startupTask struct {
	XMLName   xml.Name `xml:"Task"`
	Version   string   `xml:"version,attr"`
	Namespace string   `xml:"xmlns,attr"`
	User      string   `xml:"Triggers>LogonTrigger>UserId"`
	Principal struct {
		ID    string `xml:"id,attr"`
		User  string `xml:"UserId"`
		Logon string `xml:"LogonType"`
		Level string `xml:"RunLevel"`
	} `xml:"Principals>Principal"`
	Settings struct {
		Instances   string `xml:"MultipleInstancesPolicy"`
		Battery     bool   `xml:"DisallowStartIfOnBatteries"`
		StopBattery bool   `xml:"StopIfGoingOnBatteries"`
		Enabled     bool   `xml:"Enabled"`
		Limit       string `xml:"ExecutionTimeLimit"`
	} `xml:"Settings"`
	Actions struct {
		Context string `xml:"Context,attr"`
		Exec    struct {
			Command   string `xml:"Command"`
			Arguments string `xml:"Arguments"`
			Directory string `xml:"WorkingDirectory"`
		} `xml:"Exec"`
	} `xml:"Actions"`
}

func startupDefinition(exe, sid string) startupTask {
	t := startupTask{Version: "1.2", Namespace: "http://schemas.microsoft.com/windows/2004/02/mit/task", User: sid}
	t.Principal.ID, t.Actions.Context = "AutoTriggerUser", "AutoTriggerUser"
	t.Actions.Exec.Command, t.Actions.Exec.Arguments, t.Actions.Exec.Directory = exe, "--gui", filepath.Dir(exe)
	t.Principal.User, t.Principal.Logon, t.Principal.Level = sid, "InteractiveToken", "HighestAvailable"
	t.Settings.Instances, t.Settings.Enabled, t.Settings.Limit = "IgnoreNew", true, "PT0S"
	return t
}

func startupXMLFile(task startupTask) ([]byte, error) {
	b, err := xml.MarshalIndent(task, "", "  ")
	if err != nil {
		return nil, err
	}
	// schtasks imports task files as Unicode; match the declaration to the bytes.
	text := "<?xml version=\"1.0\" encoding=\"UTF-16\"?>\r\n" + string(b)
	units := utf16.Encode([]rune(text))
	data := make([]byte, 2+len(units)*2)
	data[0], data[1] = 0xff, 0xfe
	for i, unit := range units {
		data[2+i*2], data[3+i*2] = byte(unit), byte(unit>>8)
	}
	return data, nil
}

func startupIdentity() (string, string, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", "", err
	}
	u, err := user.Current()
	if err != nil {
		return "", "", "", err
	}
	hash := sha256.Sum256([]byte(strings.ToLower(exe) + "|" + u.Uid))
	return fmt.Sprintf("AutoTrigger-Logon-%x", hash[:8]), exe, u.Uid, nil
}

func schedulerCommand(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var systemDir [syscall.MAX_PATH]uint16
	n, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemDirectoryW").Call(uintptr(unsafe.Pointer(&systemDir[0])), uintptr(len(systemDir)))
	if n == 0 || n >= uintptr(len(systemDir)) {
		return nil, fmt.Errorf("获取系统目录失败：%v", err)
	}
	cmd := exec.CommandContext(ctx, filepath.Join(syscall.UTF16ToString(systemDir[:]), "schtasks.exe"), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, runErr := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return output, fmt.Errorf("计划任务命令超时：%w", ctx.Err())
	}
	if runErr != nil {
		return output, schedulerFailure(runErr, output)
	}
	return output, nil
}

func schedulerFailure(err error, output []byte) error {
	detail := strings.TrimSpace(schedulerText(output))
	if len([]rune(detail)) > 2000 {
		detail = string([]rune(detail)[:2000]) + "..."
	}
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w；系统详情：%s", err, detail)
}

func schedulerText(b []byte) string {
	if len(b) >= 2 && (bytes.HasPrefix(b, []byte{0xff, 0xfe}) || b[1] == 0) {
		if bytes.HasPrefix(b, []byte{0xff, 0xfe}) {
			b = b[2:]
		}
		units := make([]uint16, len(b)/2)
		for i := range units {
			units[i] = uint16(b[i*2]) | uint16(b[i*2+1])<<8
		}
		return string(utf16.Decode(units))
	}
	if utf8.Valid(b) || len(b) == 0 {
		return string(b)
	}
	convert := syscall.NewLazyDLL("kernel32.dll").NewProc("MultiByteToWideChar")
	// schtasks console diagnostics use the system OEM code page.
	n, _, _ := convert.Call(1, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return strings.ToValidUTF8(string(b), "?")
	}
	units := make([]uint16, n)
	convert.Call(1, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&units[0])), n)
	return string(utf16.Decode(units))
}

func startupEnabled() bool {
	name, exe, sid, err := startupIdentity()
	if err != nil {
		return false
	}
	b, err := schedulerCommand("/Query", "/TN", name, "/XML")
	if err != nil {
		return false
	}
	b = []byte(schedulerText(b))
	// schtasks may declare UTF-16 even though its redirected output was decoded.
	start := bytes.Index(b, []byte("<Task"))
	if start < 0 {
		return false
	}
	var t startupTask
	return xml.Unmarshal(b[start:], &t) == nil && t.Settings.Enabled && strings.EqualFold(t.Actions.Exec.Command, exe) && t.Actions.Exec.Arguments == "--gui" && t.Principal.User == sid && t.Principal.Logon == "InteractiveToken" && t.Principal.Level == "HighestAvailable"
}

func requireStartupAdministrator() error {
	var token syscall.Token
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return err
	}
	if err := syscall.OpenProcessToken(process, syscall.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	var elevated, size uint32
	ok, _, err := syscall.NewLazyDLL("advapi32.dll").NewProc("GetTokenInformation").Call(uintptr(token), 20, uintptr(unsafe.Pointer(&elevated)), 4, uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return err
	}
	if elevated == 0 {
		return fmt.Errorf("请先以管理员身份运行 AutoTrigger，再保存管理员自启设置；不会自动绕过 UAC")
	}
	return nil
}

func setStartupEnabled(enabled bool) error {
	if err := requireStartupAdministrator(); err != nil {
		return err
	}
	name, exe, sid, err := startupIdentity()
	if err != nil {
		return err
	}
	if !enabled {
		_, err = schedulerCommand("/Delete", "/TN", name, "/F")
	} else {
		b, e := startupXMLFile(startupDefinition(exe, sid))
		if e != nil {
			return e
		}
		file, e := os.CreateTemp("", "AutoTrigger-logon-*.xml")
		if e != nil {
			return e
		}
		defer os.Remove(file.Name())
		_, e = file.Write(b)
		closeErr := file.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		_, err = schedulerCommand("/Create", "/TN", name, "/XML", file.Name(), "/F")
	}
	if err != nil {
		return fmt.Errorf("计划任务设置失败，请确认管理员权限及 Task Scheduler 服务可用：%w", err)
	}
	return nil
}
