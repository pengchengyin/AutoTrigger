package ui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/gui"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/resources"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func selectedProjects(root string) (templates.UserIndex, error) {
	index := templates.UserIndex{Version: 1, Projects: []string{}, Rules: []templates.UserRule{}}
	b, err := os.ReadFile(filepath.Join(projectRoot(root), "active.index.json"))
	if os.IsNotExist(err) {
		return index, nil
	}
	if err != nil {
		return index, err
	}
	err = json.Unmarshal(b, &index)
	if err == nil && index.Version != 1 {
		err = fmt.Errorf("启用索引版本无效")
	}
	return index, err
}

func initializeTemplateSelection(root string) error {
	unlock, err := acquireHistoryLock(projectRoot(root))
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(projectRoot(root), "active.index.json")
	if _, err = os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := json.Marshal(templates.UserIndex{Version: 1, Projects: []string{}, Rules: []templates.UserRule{}})
	if err != nil {
		return err
	}
	return atomicTemplateFile(path, b)
}

func toggleTemplateProject(root, id string, enabled, running bool) error {
	if running {
		return fmt.Errorf("任务运行中，请结束后再切换模板")
	}
	unlock, err := acquireHistoryLock(projectRoot(root))
	if err != nil {
		return err
	}
	defer unlock()
	index, err := selectedProjects(root)
	if err != nil {
		return err
	}
	ids := []string{}
	found := false
	for _, p := range index.Projects {
		if p == id {
			found = true
			if !enabled {
				continue
			}
		}
		ids = append(ids, p)
	}
	if enabled && !found {
		ids = append(ids, id)
	}
	result := templates.UserIndex{Version: 1, Projects: ids, Rules: []templates.UserRule{}}
	// Preserve already validated immutable snapshots for other selected projects.
	for _, r := range index.Rules {
		if r.ProjectID != id {
			result.Rules = append(result.Rules, r)
		}
	}
	if enabled {
		projects, err := listTemplateProjects(root)
		if err != nil {
			return err
		}
		var project templateProject
		for _, p := range projects {
			if p.ID == id {
				project = p
			}
		}
		if project.ID == "" {
			return fmt.Errorf("模板不存在")
		}
		captures, err := readCaptureIndex(projectDir(root, project), id)
		if err != nil {
			return err
		}
		resolver := gui.Automator{Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
		if err = resolver.Init(); err != nil {
			return err
		}
		for _, c := range captures.Items {
			if !c.Confirmed {
				continue
			}
			action := c.Rule.Action
			if action == "" {
				action = resolver.ActionForText(c.Rule.Name)
			}
			if c.Rule.Method == "uia" {
				if known := resolver.ActionForText(c.Rule.Name); known != "" && known != action {
					return fmt.Errorf("图片 %s 的动作与控件文字冲突", c.Name)
				}
			}
			if !templates.SafeUserAction(action) {
				return fmt.Errorf("图片 %s 尚未标记有效动作，请进入详情选择动作并保存标记", c.Name)
			}
			role := ""
			if c.Control != nil {
				role = c.Control.Role
			}
			result.Rules = append(result.Rules, templates.UserRule{ProjectID: id, ID: c.ID, Method: c.Rule.Method, WindowTitle: c.Window.Title, Name: c.Rule.Name, AutomationID: c.Rule.AutomationID, Role: role, Action: action, DPI: c.Window.DPI, Threshold: c.Rule.MinSimilarity, File: filepath.ToSlash(filepath.Join("template-projects", id, c.File)), SHA256: c.SHA256})
		}
		legacy, err := legacyProjectImages(root, project)
		if err != nil {
			return err
		}
		for _, entry := range legacy {
			if !templates.SafeUserAction(entry.Action) {
				continue
			}
			b, err := os.ReadFile(filepath.Join(projectDir(root, project), filepath.FromSlash(entry.File)))
			if err != nil {
				return err
			}
			result.Rules = append(result.Rules, templates.UserRule{ProjectID: id, ID: entry.ID, Method: "legacy_image", Name: entry.ID, Action: entry.Action, DPI: entry.DPI * 96 / 100, Theme: entry.Theme, Threshold: config.Defaults().GUI.TemplateMinSimilarity, File: filepath.ToSlash(filepath.Join("template-projects", id, entry.File)), SHA256: fmt.Sprintf("%x", sha256.Sum256(b))})
		}
	}
	if enabled {
		if err = templates.ValidateUserIndex(root, &result); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return atomicTemplateFile(filepath.Join(projectRoot(root), "active.index.json"), b)
}

func updateCaptureMark(root string, p templateProject, id, action string, remove bool) error {
	unlock, err := acquireHistoryLock(projectDir(root, p))
	if err != nil {
		return err
	}
	defer unlock()
	index, err := readCaptureIndex(projectDir(root, p), p.ID)
	if err != nil {
		return err
	}
	found := -1
	for i, c := range index.Items {
		if c.ID == id {
			found = i
		}
	}
	if found < 0 {
		return fmt.Errorf("图片不存在")
	}
	if !remove && !templates.SafeUserAction(action) {
		return fmt.Errorf("请选择安全有效的动作")
	}
	item := index.Items[found]
	if remove {
		index.Items = append(index.Items[:found], index.Items[found+1:]...)
	} else {
		index.Items[found].Rule.Action = action
		index.Items[found].Confirmed = true
	}
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	if err = atomicTemplateFile(filepath.Join(projectDir(root, p), "captures.index.json"), b); err != nil {
		return err
	}
	if remove {
		return os.RemoveAll(filepath.Dir(filepath.Join(projectDir(root, p), item.File)))
	}
	return nil
}

func legacyProjectImages(root string, p templateProject) ([]templates.Entry, error) {
	path := filepath.Join(projectDir(root, p), "templates.index.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	lib, err := loadEditableTemplateDir(projectDir(root, p))
	if err != nil {
		return nil, err
	}
	return lib.Entries, nil
}

func updateLegacyMark(root string, p templateProject, id, action string, remove bool) error {
	if !remove && !templates.SafeUserAction(action) {
		return fmt.Errorf("请选择有效动作")
	}
	unlock, err := acquireHistoryLock(projectDir(root, p))
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(projectDir(root, p), "templates.index.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err = json.Unmarshal(b, &doc); err != nil {
		return err
	}
	var entries []map[string]json.RawMessage
	if err = json.Unmarshal(doc["entries"], &entries); err != nil {
		return err
	}
	kept := []map[string]json.RawMessage{}
	found := false
	for _, entry := range entries {
		var entryID string
		if err = json.Unmarshal(entry["id"], &entryID); err != nil {
			return err
		}
		if entryID == id {
			found = true
			if remove {
				continue
			}
			entry["action"], _ = json.Marshal(action)
		}
		kept = append(kept, entry)
	}
	if !found {
		return fmt.Errorf("图片不存在")
	}
	doc["entries"], err = json.Marshal(kept)
	if err != nil {
		return err
	}
	b, err = json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return atomicTemplateFile(path, b)
}
