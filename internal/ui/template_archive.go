package ui

import (
	"archive/zip"
	"autotrigger/internal/gui/templates"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type templateArchiveManifest struct {
	Version int               `json:"version"`
	Hashes  map[string]string `json:"sha256"`
}

func archivePath(name string) bool {
	if !fs.ValidPath(name) || strings.ContainsAny(name, `\:<>"|?*`) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.TrimRight(part, " .") != part {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return false
		}
	}
	return true
}

func validateArchivedProjects(root string) error {
	projects, err := listTemplateProjects(root)
	if err != nil {
		return err
	}
	for _, p := range projects {
		dir := projectDir(root, p)
		index, err := readCaptureIndex(dir, p.ID)
		if err != nil {
			return err
		}
		for _, c := range index.Items {
			if _, err = captureImage(dir, c); err != nil {
				return err
			}
		}
		if fileExists(filepath.Join(dir, "templates.index.json")) {
			if _, err = loadEditableTemplateDir(dir); err != nil {
				return err
			}
		}
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("模板包含链接：%s", path)
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".png") {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			cfg, e := png.DecodeConfig(bytes.NewReader(b))
			if e != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 25000000 {
				return fmt.Errorf("无效或过大的模板图片：%s", path)
			}
			if _, e = png.Decode(bytes.NewReader(b)); e != nil {
				return e
			}
		}
		if d.Name() == "release.json" {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			var release templateRelease
			if e = json.Unmarshal(b, &release); e != nil {
				return e
			}
			for name, hash := range release.Hashes {
				if !archivePath(name) {
					return fmt.Errorf("发布资源路径无效")
				}
				b, e := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
				if e != nil {
					return e
				}
				if fmt.Sprintf("%x", sha256.Sum256(b)) != hash {
					return fmt.Errorf("发布资源已损坏：%s", name)
				}
			}
		}
		return nil
	})
}

func exportTemplateArchive(root, path string) (err error) {
	if err = os.MkdirAll(root, 0755); err != nil {
		return err
	}
	if err = validateArchivedProjects(root); err != nil {
		return err
	}
	if err = os.MkdirAll(root, 0755); err != nil {
		return err
	}
	unlock, err := acquireHistoryLock(projectRoot(root))
	if err != nil {
		if err = os.MkdirAll(projectRoot(root), 0755); err != nil {
			return err
		}
		unlock, err = acquireHistoryLock(projectRoot(root))
		if err != nil {
			return err
		}
	}
	defer unlock()
	abs, _ := filepath.Abs(path)
	rootAbs, _ := filepath.Abs(root)
	if rel, e := filepath.Rel(rootAbs, abs); e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("导出文件不能放在模板目录内")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".templates-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	writer := zip.NewWriter(file)
	manifest := templateArchiveManifest{Version: 1, Hashes: map[string]string{}}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("模板不能包含链接：%s", path)
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "active.index.json" || d.Name() == "index.lock" || strings.HasSuffix(d.Name(), ".tmp") {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("无效模板文件：%s", path)
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		name := filepath.ToSlash(rel)
		if !archivePath(name) || name == "manifest.json" {
			return fmt.Errorf("无效模板路径：%s", name)
		}
		in, e := os.Open(path)
		if e != nil {
			return e
		}
		defer in.Close()
		entry, e := writer.Create(name)
		if e != nil {
			return e
		}
		hash := sha256.New()
		if _, e = io.Copy(io.MultiWriter(entry, hash), in); e != nil {
			return e
		}
		manifest.Hashes[name] = fmt.Sprintf("%x", hash.Sum(nil))
		return nil
	})
	if err != nil {
		writer.Close()
		return err
	}
	if len(manifest.Hashes) == 0 {
		writer.Close()
		return fmt.Errorf("暂无可导出的模板")
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		writer.Close()
		return err
	}
	entry, err := writer.Create("manifest.json")
	if err == nil {
		_, err = entry.Write(b)
	}
	closeErr := writer.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func importTemplateArchive(root, path string) (err error) {
	if info, e := os.Lstat(projectRoot(root)); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("模板项目目录不能是链接")
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	if len(reader.File) > 10000 {
		return fmt.Errorf("模板包文件数量超过上限")
	}
	if err = os.MkdirAll(root, 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(root, ".import-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	names := map[string]bool{}
	var total uint64
	for _, entry := range reader.File {
		name := entry.Name
		key := strings.ToLower(name)
		if !archivePath(name) || names[key] || entry.Mode()&os.ModeSymlink != 0 || entry.FileInfo().IsDir() || !entry.Mode().IsRegular() {
			return fmt.Errorf("压缩包包含危险或重复路径：%s", name)
		}
		if key == "template-projects/active.index.json" || strings.HasSuffix(key, "/index.lock") || key == "index.lock" {
			return fmt.Errorf("不能导入启用状态或锁文件")
		}
		names[key] = true
		total += entry.UncompressedSize64
		if entry.UncompressedSize64 > 256<<20 || total > 1<<30 {
			return fmt.Errorf("模板包解压大小超过上限（单文件256MB，总计1GB）")
		}
		source, e := entry.Open()
		if e != nil {
			return e
		}
		dest := filepath.Join(stage, filepath.FromSlash(name))
		if e = os.MkdirAll(filepath.Dir(dest), 0755); e != nil {
			source.Close()
			return e
		}
		target, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			source.Close()
			return e
		}
		n, e := io.Copy(target, io.LimitReader(source, int64(entry.UncompressedSize64)+1))
		source.Close()
		closeErr := target.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if uint64(n) != entry.UncompressedSize64 {
			return fmt.Errorf("压缩包大小异常")
		}
	}
	b, err := os.ReadFile(filepath.Join(stage, "manifest.json"))
	if err != nil {
		return fmt.Errorf("缺少模板包校验清单：%w", err)
	}
	var manifest templateArchiveManifest
	if err = json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	if manifest.Version != 1 || len(manifest.Hashes) == 0 || len(manifest.Hashes)+1 != len(names) {
		return fmt.Errorf("模板包清单不完整或版本不支持")
	}
	checked := map[string]bool{}
	for name, hash := range manifest.Hashes {
		key := strings.ToLower(name)
		if checked[key] {
			return fmt.Errorf("校验清单存在重复路径")
		}
		checked[key] = true
		if !archivePath(name) || name == "manifest.json" || !names[strings.ToLower(name)] {
			return fmt.Errorf("校验路径无效")
		}
		b, e := os.ReadFile(filepath.Join(stage, filepath.FromSlash(name)))
		if e != nil {
			return e
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != hash {
			return fmt.Errorf("模板资源损坏：%s", name)
		}
	}
	if err = validateArchivedProjects(stage); err != nil {
		return err
	}
	os.Remove(filepath.Join(stage, "manifest.json"))
	if err = os.MkdirAll(projectRoot(root), 0755); err != nil {
		return err
	}
	unlock, err := acquireHistoryLock(projectRoot(root))
	if err != nil {
		return err
	}
	defer unlock()
	// Projects commit individually; other top-level resources retain their original paths.
	entries, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	moves := [][2]string{}
	for _, d := range entries {
		if d.Name() == "template-projects" {
			children, e := os.ReadDir(filepath.Join(stage, d.Name()))
			if e != nil {
				return e
			}
			for _, child := range children {
				moves = append(moves, [2]string{filepath.Join(stage, d.Name(), child.Name()), filepath.Join(root, d.Name(), child.Name())})
			}
		} else {
			moves = append(moves, [2]string{filepath.Join(stage, d.Name()), filepath.Join(root, d.Name())})
		}
	}
	for _, move := range moves {
		if _, e := os.Lstat(move[1]); e == nil {
			return fmt.Errorf("同名模板或资源已存在，不覆盖：%s", filepath.Base(move[1]))
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	committed := 0
	defer func() {
		if err != nil {
			for i := committed - 1; i >= 0; i-- {
				os.Rename(moves[i][1], moves[i][0])
			}
		}
	}()
	for _, move := range moves {
		if err = os.Rename(move[0], move[1]); err != nil {
			return err
		}
		committed++
	}
	active := filepath.Join(projectRoot(root), "active.index.json")
	if _, err = os.Stat(active); os.IsNotExist(err) {
		b, e := json.Marshal(templates.UserIndex{Version: 1, Projects: []string{}, Rules: []templates.UserRule{}})
		if e != nil {
			return e
		}
		err = atomicTemplateFile(active, b)
	}
	if err != nil {
		return err
	}
	return nil
}
