//go:build windows

package ui

import (
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestSchedulerFailureKeepsSystemReason(t *testing.T) {
	cause := errors.New("exit status 1")
	msg := "ERROR: invalid task XML; access denied"
	for _, b := range [][]byte{[]byte(msg), schedulerUTF16(msg)} {
		err := schedulerFailure(cause, b)
		if !errors.Is(err, cause) || !strings.Contains(err.Error(), msg) {
			t.Fatal(err)
		}
	}
	if schedulerFailure(cause, nil) != cause {
		t.Fatal("empty diagnostic lost original error")
	}
}

func schedulerUTF16(text string) []byte {
	b := []byte{0xff, 0xfe}
	for _, u := range utf16.Encode([]rune(text)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

func TestSchedulerTextDecodesChineseAndBoundsDiagnostics(t *testing.T) {
	if got := schedulerText(schedulerUTF16("错误：任务 XML 无效")); got != "错误：任务 XML 无效" {
		t.Fatal(got)
	}
	err := schedulerFailure(errors.New("failure"), []byte(strings.Repeat("X", 4000)))
	if len(err.Error()) > 2100 {
		t.Fatal("unbounded scheduler diagnostic")
	}
}

func TestAdministratorStartupDefinition(t *testing.T) {
	exe := `C:\tools & apps\AutoTrigger.exe`
	sid := "S-1-5-21-1234-1001"
	want := startupDefinition(exe, sid)
	b, err := xml.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "&amp;") {
		t.Fatal("executable path not XML escaped")
	}
	var got startupTask
	if err = xml.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.User != sid || got.Principal.User != sid || got.Principal.Logon != "InteractiveToken" || got.Principal.Level != "HighestAvailable" {
		t.Fatalf("wrong logon/elevation policy: %+v", got)
	}
	if got.Principal.ID == "" || got.Actions.Context != got.Principal.ID {
		t.Fatal("missing principal key or mismatched action context")
	}
	if got.Actions.Exec.Command != exe || got.Actions.Exec.Arguments != "--gui" || got.Actions.Exec.Directory == "" {
		t.Fatalf("incorrect action: %+v", got)
	}
	if !got.Settings.Enabled || got.Settings.Limit != "PT0S" || got.Settings.Instances != "IgnoreNew" || got.Settings.Battery || got.Settings.StopBattery {
		t.Fatalf("desktop receiver should remain running: %+v", got.Settings)
	}
}

func TestStartupXMLFileEncodingMatchesDeclaration(t *testing.T) {
	exe := `C:\自动安装工具 & apps\AutoTrigger.exe`
	b, err := startupXMLFile(startupDefinition(exe, "S-1-5-21-1234-1001"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xfe || b[2] != '<' || b[3] != 0 {
		t.Fatal("task import file must be UTF-16LE with BOM")
	}
	text := schedulerText(b)
	if !strings.HasPrefix(text, "<?xml version=\"1.0\" encoding=\"UTF-16\"?>") {
		t.Fatal("encoding declaration mismatch")
	}
	start := strings.Index(text, "<Task")
	if start < 0 {
		t.Fatal("task root missing")
	}
	var got startupTask
	if err = xml.Unmarshal([]byte(text[start:]), &got); err != nil {
		t.Fatal(err)
	}
	if got.Actions.Exec.Command != exe || got.Principal.ID != got.Actions.Context {
		t.Fatalf("task file lost path or principal: %+v", got)
	}
}

func TestStartupStateAcceptsSchedulerNormalizedAccount(t *testing.T) {
	exe, sid := `C:\工具\AutoTrigger.exe`, "S-1-5-21-1234-1001"
	resolve := func(account string) (string, error) {
		if account == `VM\pengc` {
			return sid, nil
		}
		return "", errors.New("unknown account")
	}
	task := startupDefinition(exe, sid)
	task.Principal.User = `VM\pengc`
	task.Actions.Exec.Arguments = " --gui "
	b, err := startupXMLFile(task)
	if err != nil {
		t.Fatal(err)
	}
	if !startupXMLMatches(b, exe, sid, resolve) {
		t.Fatal("scheduler account normalization hid enabled startup")
	}
	task.Settings.Enabled = false
	b, _ = startupXMLFile(task)
	if startupXMLMatches(b, exe, sid, resolve) {
		t.Fatal("disabled task shown as enabled")
	}
	task.Settings.Enabled = true
	task.Principal.User = `VM\other`
	b, _ = startupXMLFile(task)
	if startupXMLMatches(b, exe, sid, resolve) {
		t.Fatal("other account shown as current startup")
	}
}

func TestStartupStateAllowsDefaultEnabledButRejectsDifferentAction(t *testing.T) {
	exe, sid := `C:\tools\AutoTrigger.exe`, "S-1-5-21-1234-1001"
	b, err := xml.Marshal(startupDefinition(exe, sid))
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.ReplaceAll(string(b), "<Enabled>true</Enabled>", ""))
	resolve := func(string) (string, error) { return "", errors.New("unexpected lookup") }
	if !startupXMLMatches(b, exe, sid, resolve) {
		t.Fatal("default enabled task hidden")
	}
	if startupXMLMatches(b, `C:\other\AutoTrigger.exe`, sid, resolve) {
		t.Fatal("wrong executable accepted")
	}
}

func TestStartupStateVMExportWithoutEnabled(t *testing.T) {
	const body = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><URI>\AutoTrigger-Logon-c5e2a2ebf90ed4aa</URI></RegistrationInfo>
  <Principals><Principal id="AutoTriggerUser">
    <UserId>S-1-5-21-2552418914-3197032882-3489908625-500</UserId>
    <LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel>
  </Principal></Principals>
  <Settings>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <IdleSettings><StopOnIdleEnd>true</StopOnIdleEnd><RestartOnIdle>false</RestartOnIdle></IdleSettings>
  </Settings>
  <Triggers><LogonTrigger><UserId>WIN10\Administrator</UserId></LogonTrigger></Triggers>
  <Actions Context="AutoTriggerUser"><Exec>
    <Command>C:\Users\Administrator\Desktop\AutoTrigger\AutoTrigger.exe</Command>
    <Arguments>--gui</Arguments><WorkingDirectory>C:\Users\Administrator\Desktop\AutoTrigger</WorkingDirectory>
  </Exec></Actions>
</Task>`
	for _, data := range [][]byte{[]byte(body), schedulerUTF16(body)} {
		if !startupXMLMatches(data, `C:\Users\Administrator\Desktop\AutoTrigger\AutoTrigger.exe`, "S-1-5-21-2552418914-3197032882-3489908625-500", func(string) (string, error) { return "", errors.New("unexpected lookup") }) {
			t.Fatal("real VM task export did not show enabled")
		}
	}
}

func TestStartupIdentityIsStableAndReadOnly(t *testing.T) {
	one, exe, sid, err := startupIdentity()
	if err != nil {
		t.Fatal(err)
	}
	two, _, _, err := startupIdentity()
	if err != nil || one != two || exe == "" || !strings.HasPrefix(sid, "S-1-") || !strings.HasPrefix(one, "AutoTrigger-Logon-") {
		t.Fatalf("bad startup identity: %s %s %s %v", one, exe, sid, err)
	}
}
