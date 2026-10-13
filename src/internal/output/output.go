// Package output writes complete JSON atomically and preserves interruption journals.
package output

import (
	"autotrigger/pkg/schema"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Atomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temp := path + ".tmp"
	f, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp, path)
}
func JSON(w io.Writer, v interface{}, pretty bool) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(v)
}
func Write(path string, result *schema.Result, stdout io.Writer, pretty bool, limit int64) error {
	result.Summary = schema.Summarize(result.Results)
	marshal := func() ([]byte, error) {
		if pretty {
			return json.MarshalIndent(result, "", "  ")
		}
		return json.Marshal(result)
	}
	data, err := marshal()
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		result.Truncated = true
		for i := range result.Results {
			result.Results[i].Attempts = []schema.Attempt{}
		}
		data, err = marshal()
		if err != nil {
			return err
		}
		if int64(len(data)) > limit {
			result.Meta["output_size_warning"] = "fixed file records exceed max_file_size_mb; fixed records retained to preserve summary and contract"
			data, err = marshal()
			if err != nil {
				return err
			}
		}
	}
	data = append(data, '\n')
	if path == "-" {
		_, err = stdout.Write(data)
		return err
	}
	return Atomic(path, data)
}

type Journal struct {
	File   *os.File
	Path   string
	TaskID string
}

func OpenJournal(path, taskID string) (*Journal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		backup := fmt.Sprintf("%s.interrupted-%d", path, time.Now().UnixNano())
		header := strings.TrimSuffix(path, ".journal.ndjson")
		if _, headerErr := os.Stat(header); headerErr == nil && header != path {
			data, readErr := os.ReadFile(header)
			if readErr != nil {
				return nil, readErr
			}
			if err = Atomic(backup+".header.json", data); err != nil {
				return nil, err
			}
			if err = RecoverJournal(backup+".header.json", path, backup+".recovered.json"); err != nil {
				return nil, fmt.Errorf("recover prior journal: %w", err)
			}
		}
		if err = os.Rename(path, backup); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &Journal{File: f, Path: path, TaskID: taskID}, nil
}
func (j *Journal) Append(result schema.FileResult) error {
	row := struct {
		TaskID        string `json:"task_id"`
		SchemaVersion string `json:"schema_version"`
		schema.FileResult
	}{TaskID: j.TaskID, SchemaVersion: schema.Version, FileResult: result}
	if err := JSON(j.File, row, false); err != nil {
		return err
	}
	return j.File.Sync()
}
func (j *Journal) Close(remove bool) error {
	if err := j.File.Close(); err != nil {
		return err
	}
	if remove {
		return os.Remove(j.Path)
	}
	return nil
}
