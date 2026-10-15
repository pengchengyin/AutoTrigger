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
