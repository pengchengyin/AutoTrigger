//go:build windows

package ui

import (
	"encoding/xml"
	"strings"
	"testing"
)

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
	if got.Command != exe || got.Arguments != "--gui" || got.Directory == "" {
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
