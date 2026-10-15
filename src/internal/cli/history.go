package cli

import (
	"autotrigger/internal/datadir"
	"autotrigger/internal/output"
	"autotrigger/pkg/schema"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// Each CLI invocation owns its files, avoiding cross-process index updates.
func writeTaskHistory(layout datadir.Layout, result *schema.Result, input, configPath, logPath string, exit *int) error {
	path := filepath.Join(layout.Results(), "cli-"+result.TaskID+".json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err = output.Atomic(path, data); err != nil {
		return err
	}
	record := map[string]interface{}{
		"origin": "cli",
		"id":     result.TaskID, "name": filepath.Base(input), "input": input,
		"result": path, "config": configPath, "created": result.StartedAt,
		"log_path": logPath,
	}
	if exit != nil {
		record["exit_code"] = *exit
		if source, openErr := os.Open(logPath); openErr == nil {
			defer source.Close()
			archive := filepath.Join(layout.Logs(), "cli-"+result.TaskID+".log")
			if err = os.MkdirAll(layout.Logs(), 0755); err != nil {
				return err
			}
			dest, createErr := os.Create(archive + ".tmp")
			if createErr != nil {
				return createErr
			}
			_, copyErr := io.Copy(dest, source)
			closeErr := dest.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if err = os.Rename(archive+".tmp", archive); err != nil {
				return err
			}
			record["log_path"] = archive
		} else if !os.IsNotExist(openErr) {
			return openErr
		}
	}
	b, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(layout.History(), "cli")
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	target := filepath.Join(dir, result.TaskID+".json")
	if err = os.WriteFile(target+".tmp", b, 0600); err != nil {
		return err
	}
	return os.Rename(target+".tmp", target)
}
