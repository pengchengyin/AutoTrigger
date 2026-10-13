package ui

import (
	"autotrigger/internal/datadir"
	"autotrigger/internal/remotetask"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func copyDataTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("不迁移链接：%s", path)
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		out := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0755)
		}
		if d.Name() == "index.lock" || strings.HasSuffix(d.Name(), ".tmp") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("不是普通文件：%s", path)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, in)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}

func rebaseDataPath(path, oldRoot, newRoot string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(oldRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join(newRoot, rel)
}

// Copy first, commit directories last. Never remove source data or overwrite a destination.
func migrateData(oldRoot, newRoot string) (err error) {
	oldRoot, _ = filepath.Abs(oldRoot)
	newRoot, _ = filepath.Abs(newRoot)
	if strings.EqualFold(oldRoot, newRoot) {
		return nil
	}
	for _, pair := range [][2]string{{oldRoot, newRoot}, {newRoot, oldRoot}} {
		rel, e := filepath.Rel(pair[0], pair[1])
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("迁移目录不能互相包含")
		}
	}
	if err = os.MkdirAll(newRoot, 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(newRoot, ".migration-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	names := []string{"logs", "results", "templates", "third_parties", "config"}
	locatorOnly := false
	for _, name := range names {
		dest := filepath.Join(newRoot, name)
		entries, e := os.ReadDir(dest)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if name == "config" && len(entries) == 1 && entries[0].Name() == "data-location.json" && !entries[0].IsDir() {
			locatorOnly = true
		}
		if len(entries) > 0 && !(name == "config" && locatorOnly) {
			return fmt.Errorf("目标目录已有数据，不覆盖：%s", dest)
		}
		src := filepath.Join(oldRoot, name)
		if _, e = os.Stat(src); os.IsNotExist(e) {
			continue
		} else if e != nil {
			return e
		}
		if err = copyDataTree(src, filepath.Join(stage, name)); err != nil {
			return err
		}
	}
	os.Remove(filepath.Join(stage, "config", "data-location.json"))
	if locatorOnly {
		b, e := os.ReadFile(filepath.Join(newRoot, "config", "data-location.json"))
		if e != nil {
			return e
		}
		if err = datadir.AtomicWrite(filepath.Join(stage, "config", "data-location.json"), b); err != nil {
			return err
		}
	}
	records, err := loadHistory(datadir.Layout{Root: stage}.History())
	if err != nil {
		return err
	}
	for i := range records {
		r := &records[i]
		if r.RemoteID != "" {
			r.Input = rebaseDataPath(r.Input, oldRoot, newRoot)
		}
		r.Result = rebaseDataPath(r.Result, oldRoot, newRoot)
		r.LogPath = rebaseDataPath(r.LogPath, oldRoot, newRoot)
		r.Config = rebaseDataPath(r.Config, oldRoot, newRoot)
		if r.Execution != nil {
			if r.RemoteID != "" {
				r.Execution.Input = rebaseDataPath(r.Execution.Input, oldRoot, newRoot)
			}
			r.Execution.Output = rebaseDataPath(r.Execution.Output, oldRoot, newRoot)
			r.Execution.Config = rebaseDataPath(r.Execution.Config, oldRoot, newRoot)
		}
	}
	if len(records) > 0 {
		if err = saveHistory(datadir.Layout{Root: stage}.History(), records); err != nil {
			return err
		}
	}
	cfg := datadir.Layout{Root: stage}.Config()
	if b, e := os.ReadFile(cfg); e == nil {
		var doc map[string]interface{}
		if err = json.Unmarshal(b, &doc); err != nil {
			return err
		}
		for _, pair := range [][2]string{{"output", "path"}, {"output", "log_file"}, {"ocr", "engine_path"}, {"archive", "sevenzip_path"}, {"gui", "template_dir"}} {
			if section, ok := doc[pair[0]].(map[string]interface{}); ok {
				if value, ok := section[pair[1]].(string); ok {
					section[pair[1]] = rebaseDataPath(value, oldRoot, newRoot)
				}
			}
		}
		b, err = json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		if err = datadir.AtomicWrite(cfg, b); err != nil {
			return err
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	committed := []string{}
	if err = remotetask.RebaseCopied(stage, oldRoot, newRoot); err != nil {
		return err
	}
	locatorBackup := filepath.Join(stage, ".locator-backup")
	backupSaved := false
	defer func() {
		if err != nil {
			for _, name := range committed {
				os.Rename(filepath.Join(newRoot, name), filepath.Join(stage, name))
			}
			if backupSaved {
				os.Rename(locatorBackup, filepath.Join(newRoot, "config"))
			}
		}
	}()
	for _, name := range names {
		src := filepath.Join(stage, name)
		if _, e := os.Stat(src); os.IsNotExist(e) {
			continue
		}
		dest := filepath.Join(newRoot, name)
		if name == "config" && locatorOnly {
			if err = os.Rename(dest, locatorBackup); err != nil {
				return err
			}
			backupSaved = true
		} else if e := os.Remove(dest); e != nil && !os.IsNotExist(e) {
			return e
		}
		if err = os.Rename(src, dest); err != nil {
			return err
		}
		committed = append(committed, name)
	}
	return nil
}

func legacyDataMoves(root, history string) [][2]string {
	moves := [][2]string{{filepath.Join(root, "third_party"), filepath.Join(root, "third_parties")}, {filepath.Join(root, "template-projects"), filepath.Join(root, "templates", "template-projects")}, {filepath.Join(root, "config.json"), filepath.Join(root, "config", "config.json")}, {history, filepath.Join(root, "config", "history")}}
	for _, pattern := range []string{"result-*.json*", "autotrigger-result.json*"} {
		files, _ := filepath.Glob(filepath.Join(root, pattern))
		for _, path := range files {
			folder := "results"
			if strings.HasSuffix(path, ".log") {
				folder = "logs"
			}
			moves = append(moves, [2]string{path, filepath.Join(root, folder, filepath.Base(path))})
		}
	}
	return moves
}

func migrateLegacyData(root, history string) (err error) {
	stage, err := os.MkdirTemp(root, ".legacy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	moves := legacyDataMoves(root, history)
	ready := [][2]string{}
	for i, move := range moves {
		if move[0] == "" {
			continue
		}
		info, e := os.Stat(move[0])
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if entries, e := os.ReadDir(move[1]); e == nil && len(entries) == 0 {
		} else if _, e := os.Stat(move[1]); e == nil {
			return fmt.Errorf("新版数据已存在，不能覆盖：%s", move[1])
		} else if !os.IsNotExist(e) {
			return e
		}
		staged := filepath.Join(stage, fmt.Sprint(i))
		if info.IsDir() {
			if err = copyDataTree(move[0], staged); err != nil {
				return err
			}
		} else {
			b, e := os.ReadFile(move[0])
			if e != nil {
				return e
			}
			if err = datadir.AtomicWrite(staged, b); err != nil {
				return err
			}
		}
		ready = append(ready, [2]string{staged, move[1]})
	}
	translate := func(path string) string {
		if !filepath.IsAbs(path) {
			clean := filepath.Clean(filepath.FromSlash(path))
			if strings.HasPrefix(clean, "third_party"+string(filepath.Separator)) {
				return filepath.Join("third_parties", strings.TrimPrefix(clean, "third_party"+string(filepath.Separator)))
			}
		}
		for _, move := range moves {
			if strings.EqualFold(filepath.Clean(path), filepath.Clean(move[0])) {
				return move[1]
			}
			if info, e := os.Stat(move[0]); e == nil && info.IsDir() {
				if next := rebaseDataPath(path, move[0], move[1]); next != path {
					return next
				}
			}
		}
		return path
	}
	for _, move := range ready {
		if move[1] == filepath.Join(root, "config", "history") {
			records, e := loadHistory(move[0])
			if e != nil {
				return e
			}
			for i := range records {
				r := &records[i]
				r.Result = translate(r.Result)
				r.LogPath = translate(r.LogPath)
				r.Config = translate(r.Config)
				if r.Execution != nil {
					r.Execution.Output = translate(r.Execution.Output)
					r.Execution.Config = translate(r.Execution.Config)
				}
			}
			if e = saveHistory(move[0], records); e != nil {
				return e
			}
		}
		if move[1] == filepath.Join(root, "config", "config.json") {
			b, e := os.ReadFile(move[0])
			if e != nil {
				return e
			}
			var doc map[string]interface{}
			if e = json.Unmarshal(b, &doc); e != nil {
				return e
			}
			for _, pair := range [][2]string{{"output", "path"}, {"output", "log_file"}, {"ocr", "engine_path"}, {"archive", "sevenzip_path"}, {"gui", "template_dir"}} {
				if section, ok := doc[pair[0]].(map[string]interface{}); ok {
					if value, ok := section[pair[1]].(string); ok {
						section[pair[1]] = translate(value)
					}
				}
			}
			b, e = json.MarshalIndent(doc, "", "  ")
			if e != nil {
				return e
			}
			if e = datadir.AtomicWrite(move[0], b); e != nil {
				return e
			}
		}
	}
	committed := 0
	defer func() {
		if err != nil {
			for i := committed - 1; i >= 0; i-- {
				os.Rename(ready[i][1], ready[i][0])
			}
		}
	}()
	for _, move := range ready {
		if err = os.MkdirAll(filepath.Dir(move[1]), 0755); err != nil {
			return err
		}
		if e := os.Remove(move[1]); e != nil && !os.IsNotExist(e) {
			return e
		}
		if err = os.Rename(move[0], move[1]); err != nil {
			return err
		}
		committed++
	}
	return datadir.AtomicWrite(filepath.Join(root, "config", "legacy-layout-imported.json"), []byte(`{"imported":true}`))
}
