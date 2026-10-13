package output

import (
	"autotrigger/pkg/schema"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// RecoverJournal reads only complete synced records, never inventing missing work.
func RecoverJournal(headerPath, journalPath, destination string) error {
	header, err := os.ReadFile(headerPath)
	if err != nil {
		return err
	}
	var result schema.Result
	if err = json.Unmarshal(header, &result); err != nil {
		return err
	}
	if result.FinishedAt != nil {
		return nil
	}
	if result.TaskID == "" || result.SchemaVersion != schema.Version {
		return fmt.Errorf("unsupported journal header")
	}
	f, err := os.Open(journalPath)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	result.Results = []schema.FileResult{}
	for {
		line, readErr := r.ReadBytes('\n')
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
		var row struct {
			TaskID        string `json:"task_id"`
			SchemaVersion string `json:"schema_version"`
			schema.FileResult
		}
		if err = json.Unmarshal(line, &row); err != nil {
			return fmt.Errorf("invalid complete journal record: %w", err)
		}
		if row.TaskID != result.TaskID || row.SchemaVersion != result.SchemaVersion || row.FilePath == "" {
			return fmt.Errorf("journal task/schema mismatch")
		}
		result.Results = append(result.Results, row.FileResult)
	}
	if result.Meta == nil {
		result.Meta = map[string]interface{}{}
	}
	result.Meta["recovered_at"] = time.Now().Format(time.RFC3339Nano)
	result.Meta["recovered_from_journal"] = journalPath
	result.Error = &schema.Error{Code: 1, Message: "任务异常结束：仅恢复 journal 中已完成记录，未执行的样本未知"}
	if err = Write(destination, &result, io.Discard, true, 64*1024*1024); err != nil {
		return err
	}
	if warnings := cleanupRecoveredWorkspaces(&result); len(warnings) > 0 {
		result.Meta["workspace_cleanup_warnings"] = warnings
		return Write(destination, &result, io.Discard, true, 64*1024*1024)
	}
	return nil
}
