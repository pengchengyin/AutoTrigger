package output

import (
	"autotrigger/pkg/schema"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type workspaceOwner struct {
	TaskID string `json:"task_id"`
	PID    int    `json:"pid"`
	Path   string `json:"path"`
}

func RegisterWorkspace(resultPath, dir string, result *schema.Result) error {
	if resultPath == "" || resultPath == "-" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	data, err := json.Marshal(workspaceOwner{TaskID: result.TaskID, PID: os.Getpid(), Path: abs})
	if err != nil {
		return err
	}
	if err = Atomic(abs+".owner.json", data); err != nil {
		return err
	}
	if result.Meta == nil {
		result.Meta = map[string]interface{}{}
	}
	paths, _ := result.Meta["temporary_workspaces"].([]string)
	result.Meta["temporary_workspaces"] = append(paths, abs)
	result.Meta["workspace_owner_pid"] = os.Getpid()
	data, err = json.Marshal(result)
	if err != nil {
		return err
	}
	return Atomic(resultPath, data)
}

func RemoveWorkspace(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.Remove(dir + ".owner.json"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func cleanupRecoveredWorkspaces(result *schema.Result) []string {
	warnings := []string{}
	pidNumber, ok := result.Meta["workspace_owner_pid"].(float64)
	if !ok || pidNumber < 1 || pidNumber != float64(int(pidNumber)) || !ownerExited(int(pidNumber)) {
		return warnings
	}
	paths, ok := result.Meta["temporary_workspaces"].([]interface{})
	if !ok {
		return warnings
	}
	for _, entry := range paths {
		dir, ok := entry.(string)
		if !ok || !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Base(dir), "AT-") {
			continue
		}
		data, err := os.ReadFile(dir + ".owner.json")
		if err != nil {
			continue
		}
		var owner workspaceOwner
		if json.Unmarshal(data, &owner) != nil || owner.TaskID != result.TaskID || owner.PID != int(pidNumber) || owner.Path != dir {
			continue
		}
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			os.Remove(dir + ".owner.json")
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if err = os.RemoveAll(dir); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", dir, err))
			continue
		}
		if err = os.Remove(dir + ".owner.json"); err != nil {
			warnings = append(warnings, err.Error())
		}
	}
	return warnings
}
