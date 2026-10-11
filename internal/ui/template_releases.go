package ui

import (
	"autotrigger/internal/gui/templates"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Release manifests describe immutable copies, never paths into a draft.
type templateRelease struct {
	ProjectID string            `json:"project_id"`
	Version   string            `json:"version"`
	CreatedAt time.Time         `json:"created_at"`
	Entries   []templates.Entry `json:"entries"`
	Hashes    map[string]string `json:"sha256"`
}

func listTemplateReleases(target, projectID string) ([]templateRelease, error) {
	dirs, err := os.ReadDir(target)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var releases []templateRelease
	for _, dir := range dirs {
		if !dir.IsDir() || !strings.HasPrefix(dir.Name(), "published-"+projectID+"-") {
			continue
		}
		data, e := os.ReadFile(filepath.Join(target, dir.Name(), "release.json"))
		if os.IsNotExist(e) {
			continue
		} // Legacy publications have no version manifest.
		if e != nil {
			return nil, e
		}
		var release templateRelease
		if e = json.Unmarshal(data, &release); e != nil {
			return nil, e
		}
		if release.ProjectID != projectID || release.Version != dir.Name() {
			return nil, fmt.Errorf("发布版本标识不一致")
		}
		releases = append(releases, release)
	}
	return releases, nil
}

func restoreTemplateRelease(target, projectID, version string, running bool) error {
	if running {
		return fmt.Errorf("任务运行中，不能切换模板版本")
	}
	unlock, err := acquireHistoryLock(target)
	if err != nil {
		return err
	}
	defer unlock()
	releases, err := listTemplateReleases(target, projectID)
	if err != nil {
		return err
	}
	var selected *templateRelease
	for i := range releases {
		if releases[i].Version == version {
			selected = &releases[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("发布版本不存在")
	}
	if len(selected.Entries) == 0 {
		return fmt.Errorf("发布版本没有图片")
	}
	for _, entry := range selected.Entries {
		if !strings.HasPrefix(entry.ID, projectID+"__") {
			return fmt.Errorf("图片不属于当前模板")
		}
		for _, name := range []string{entry.File, entry.Screenshot} {
			if !fs.ValidPath(name) || strings.Contains(name, ":") || !strings.HasPrefix(name, version+"/") {
				return fmt.Errorf("版本资源路径无效")
			}
			data, e := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
			if e != nil {
				return e
			}
			if selected.Hashes[name] == "" || selected.Hashes[name] != fmt.Sprintf("%x", sha256.Sum256(data)) {
				return fmt.Errorf("版本资源校验失败：%s", name)
			}
		}
	}
	path := filepath.Join(target, "templates.index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err = json.Unmarshal(data, &doc); err != nil {
		return err
	}
	var entries []templates.Entry
	if err = json.Unmarshal(doc["entries"], &entries); err != nil {
		return err
	}
	kept := make([]templates.Entry, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.ID, projectID+"__") {
			kept = append(kept, entry)
		}
	}
	kept = append(kept, selected.Entries...)
	doc["entries"], err = json.Marshal(kept)
	if err != nil {
		return err
	}
	data, err = json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return atomicTemplateFile(path, data)
}
