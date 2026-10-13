package ui

import (
	"autotrigger/internal/gui/templates"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type templateProject struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Description     string     `json:"description,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at,omitempty"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	PublishedDigest string     `json:"published_digest,omitempty"`
}

func projectDigest(root string, p templateProject) string {
	b, err := os.ReadFile(filepath.Join(projectDir(root, p), "templates.index.json"))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func projectRoot(root string) string                   { return filepath.Join(root, "template-projects") }
func projectDir(root string, p templateProject) string { return filepath.Join(projectRoot(root), p.ID) }

func loadEditableTemplateDir(dir string) (*templates.Library, error) {
	b, err := os.ReadFile(filepath.Join(dir, "templates.index.json"))
	if err != nil {
		return nil, err
	}
	var index templates.Index
	if err = json.Unmarshal(b, &index); err != nil {
		return nil, err
	}
	if len(index.Entries) == 0 {
		return &templates.Library{Entries: []templates.Entry{}}, nil
	}
	return loadExternalTemplateDir(dir)
}

func writeProject(root string, p templateProject) error {
	if filepath.Base(p.ID) != p.ID || p.ID == "." || p.ID == ".." || strings.ContainsAny(p.ID, `\/:*?"<>|`) || p.ID == "" || strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("模板名称不能为空，标识必须有效")
	}
	p.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return atomicTemplateFile(filepath.Join(projectDir(root, p), "project.json"), b)
}

func atomicTemplateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".publish-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func newTemplateProject(root, name string) (templateProject, error) {
	p := templateProject{ID: "project-" + time.Now().Format("20060102-150405.000000000"), Name: strings.TrimSpace(name)}
	return p, writeProject(root, p)
}

func listTemplateProjects(root string) ([]templateProject, error) {
	dirs, err := os.ReadDir(projectRoot(root))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []templateProject
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(projectRoot(root), d.Name(), "project.json"))
		if err != nil {
			return nil, err
		}
		var p templateProject
		if err = json.Unmarshal(b, &p); err != nil {
			return nil, err
		}
		if p.ID != d.Name() || p.Name == "" {
			return nil, fmt.Errorf("模板项目无效：%s", d.Name())
		}
		out = append(out, p)
	}
	return out, nil
}

// Publish immutable assets first, then replace only the selected project's entries.
func publishTemplateProject(root, target string, p templateProject) (err error) {
	draft := projectDir(root, p)
	draftAbs, _ := filepath.Abs(draft)
	targetAbs, _ := filepath.Abs(target)
	if strings.EqualFold(draftAbs, targetAbs) {
		return fmt.Errorf("发布目录不能是草稿目录")
	}
	unlockDraft, err := acquireHistoryLock(draft)
	if err != nil {
		return err
	}
	defer unlockDraft()
	lib, err := loadEditableTemplateDir(draft)
	if err != nil {
		return err
	}
	if len(lib.Entries) == 0 {
		return fmt.Errorf("请先保存至少一张标记图片")
	}
	if failures, e := lib.SelfTest(); e != nil {
		return fmt.Errorf("模板校验未通过：%v：%w", failures, e)
	}
	if err = os.MkdirAll(target, 0755); err != nil {
		return err
	}
	unlock, err := acquireHistoryLock(target)
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(target, "templates.index.json")
	doc := map[string]json.RawMessage{}
	var entries []templates.Entry
	if b, e := os.ReadFile(path); e == nil {
		if _, err = loadEditableTemplateDir(target); err != nil {
			return err
		}
		if err = json.Unmarshal(b, &doc); err != nil {
			return err
		}
		if err = json.Unmarshal(doc["entries"], &entries); err != nil {
			return err
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	prefix := p.ID + "__"
	kept := make([]templates.Entry, 0, len(entries)+len(lib.Entries))
	for _, e := range entries {
		if !strings.HasPrefix(e.ID, prefix) {
			kept = append(kept, e)
		}
	}
	assets, err := os.MkdirTemp(target, "published-"+p.ID+"-")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(assets)
		}
	}()
	for i, entry := range lib.Entries {
		for _, file := range []struct{ source, name string }{{entry.File, fmt.Sprintf("%d-button.png", i)}, {entry.Screenshot, fmt.Sprintf("%d-source.png", i)}} {
			b, e := os.ReadFile(filepath.Join(draft, filepath.FromSlash(file.source)))
			if e != nil {
				return e
			}
			if e = os.WriteFile(filepath.Join(assets, file.name), b, 0600); e != nil {
				return e
			}
		}
		entry.ID = prefix + entry.ID
		entry.File = filepath.Base(assets) + fmt.Sprintf("/%d-button.png", i)
		entry.Screenshot = filepath.Base(assets) + fmt.Sprintf("/%d-source.png", i)
		kept = append(kept, entry)
	}
	doc["entries"], err = json.Marshal(kept)
	if err != nil {
		return err
	}
	if doc["resource_pack_version"] == nil {
		doc["resource_pack_version"] = json.RawMessage(`"0.1.0"`)
	}
	doc["status"] = json.RawMessage(`"user_published"`)
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	// The immutable release belongs to this asset directory. The live index is
	// the commit point; failed publication removes the entire uncommitted release.
	release := templateRelease{ProjectID: p.ID, CreatedAt: time.Now().UTC(), Entries: append([]templates.Entry(nil), kept[len(kept)-len(lib.Entries):]...), Hashes: map[string]string{}}
	for _, entry := range release.Entries {
		for _, name := range []string{entry.File, entry.Screenshot} {
			data, e := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
			if e != nil {
				return e
			}
			release.Hashes[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		}
	}
	release.Version = filepath.Base(assets)
	manifest, e := json.MarshalIndent(release, "", "  ")
	if e != nil {
		return e
	}
	if e = atomicTemplateFile(filepath.Join(assets, "release.json"), manifest); e != nil {
		return e
	}
	return atomicTemplateFile(path, b)
}
