//go:build windows

package output

import (
	"autotrigger/pkg/schema"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorkspaceRecoveryRequiresExitedOwner(t *testing.T) {
	root := t.TempDir()
	dir, err := os.MkdirTemp(root, "AT-")
	if err != nil {
		t.Fatal(err)
	}
	r := schema.NewResult("task", "windows")
	if err = RegisterWorkspace(filepath.Join(root, "r.json"), dir, &r); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(r)
	json.Unmarshal(data, &r)
	cleanupRecoveredWorkspaces(&r)
	if _, err = os.Stat(dir); err != nil {
		t.Fatal("active workspace deleted", err)
	}
	p := exec.Command("cmd.exe", "/c", "exit", "0")
	if err = p.Run(); err != nil {
		t.Fatal(err)
	}
	pid := p.Process.Pid
	if !ownerExited(pid) {
		t.Skip("exited PID reused or inaccessible")
	}
	r.Meta["workspace_owner_pid"] = float64(pid)
	data, _ = json.Marshal(workspaceOwner{TaskID: "wrong", PID: pid, Path: dir})
	os.WriteFile(dir+".owner.json", data, 0600)
	cleanupRecoveredWorkspaces(&r)
	if _, err = os.Stat(dir); err != nil {
		t.Fatal("mismatched ownership deleted", err)
	}
	data, _ = json.Marshal(workspaceOwner{TaskID: r.TaskID, PID: pid, Path: dir})
	os.WriteFile(dir+".owner.json", data, 0600)
	if warnings := cleanupRecoveredWorkspaces(&r); len(warnings) > 0 {
		t.Fatal(warnings)
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("abandoned workspace retained", err)
	}
}
