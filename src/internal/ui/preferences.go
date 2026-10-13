package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// GUI preferences preserve CLI-only options without adding config fields.
type preferences struct {
	ConfigPath  string `json:"config_path"`
	Recursive   bool   `json:"recursive"`
	NoArchive   bool   `json:"no_archive"`
	RemoteTasks bool   `json:"remote_tasks,omitempty"`
}

func loadPreferences(dir string) (preferences, error) {
	b, err := os.ReadFile(filepath.Join(dir, "preferences.json"))
	if os.IsNotExist(err) {
		return preferences{}, nil
	}
	if err != nil {
		return preferences{}, err
	}
	var p preferences
	err = json.Unmarshal(b, &p)
	return p, err
}

func savePreferences(dir string, p preferences) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "preferences.json")
	if err = os.WriteFile(path+".tmp", b, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
