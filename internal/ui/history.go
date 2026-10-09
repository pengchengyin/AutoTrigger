package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type taskRecord struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Input    string    `json:"input"`
	Result   string    `json:"result"`
	Config   string    `json:"config"`
	Created  time.Time `json:"created"`
	ExitCode *int      `json:"exit_code,omitempty"`
}

func loadHistory(dir string) ([]taskRecord, error) {
	b, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []taskRecord
	if err = json.Unmarshal(b, &records); err != nil {
		return nil, err
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Created.After(records[j].Created) })
	return records, nil
}

func saveHistory(dir string, records []taskRecord) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "index.json")
	if err = os.WriteFile(path+".tmp", b, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func recordFromResult(path, name string) (taskRecord, error) {
	snap, ok := readResultSnapshot(path)
	if !ok || snap.TaskID == "" {
		return taskRecord{}, fmt.Errorf("请选择本工具生成的有效结果 JSON")
	}
	input := ""
	if len(snap.Rows) > 0 {
		input = snap.Rows[0].Path
	}
	if name == "" {
		name = filepath.Base(input)
		if input == "" {
			name = snap.TaskID
		}
	}
	return taskRecord{ID: snap.TaskID, Name: name, Input: input, Result: path, Created: snap.StartedAt}, nil
}
