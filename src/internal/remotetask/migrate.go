package remotetask

import (
	"autotrigger/internal/config"
	"autotrigger/internal/datadir"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RebaseCopied operates only on the private migration staging tree.
func RebaseCopied(stage, oldRoot, newRoot string) error {
	entries, err := os.ReadDir(filepath.Join(stage, "config", "remote", "jobs"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	rebase := func(path string) string {
		rel, err := filepath.Rel(oldRoot, path)
		if !filepath.IsAbs(path) || err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return path
		}
		return filepath.Join(newRoot, rel)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !ValidID(entry.Name()) {
			continue
		}
		dir := jobDir(stage, entry.Name())
		b, err := os.ReadFile(filepath.Join(dir, "task.json"))
		if err != nil {
			return err
		}
		var j Job
		if err = json.Unmarshal(b, &j); err != nil {
			return err
		}
		old := Queue{layout: datadir.Layout{Root: oldRoot}}
		if j.ID != entry.Name() || !old.validPaths(j) {
			return fmt.Errorf("迁移任务索引路径无效：%s", entry.Name())
		}
		if j.Version != 1 {
			return fmt.Errorf("迁移任务索引版本无效：%s", entry.Name())
		}
		if j.State != "staging" {
			copied := j
			copied.Input = filepath.Join(dir, "input", filepath.Base(j.Input))
			copied.Config = filepath.Join(dir, "config.json")
			guard, err := Verify(copied)
			if err != nil {
				return fmt.Errorf("迁移快照完整性校验失败：%s：%w", j.ID, err)
			}
			guard.Close()
		}
		j.Source, j.Input, j.Config, j.Result, j.Log = rebase(j.Source), rebase(j.Input), rebase(j.Config), rebase(j.Result), rebase(j.Log)
		if j.State == "staging" || j.State == "running" {
			j.State = "interrupted"
			j.Reason = "迁移前任务未正常结束，未自动重新执行"
			j.Finished = nil
		}
		path := filepath.Join(dir, "config.json")
		body, err := os.ReadFile(path)
		if err == nil {
			cfg, _, err := config.Load(path, oldRoot)
			if err != nil {
				return err
			}
			cfg.Output.Path = rebase(cfg.Output.Path)
			cfg.Output.LogFile = rebase(cfg.Output.LogFile)
			cfg.OCR.EnginePath = rebase(cfg.OCR.EnginePath)
			cfg.Archive.SevenZipPath = rebase(cfg.Archive.SevenZipPath)
			cfg.Archive.TempRoot = rebase(cfg.Archive.TempRoot)
			cfg.GUI.TemplateDir = rebase(cfg.GUI.TemplateDir)
			cfg.MSI.LogDir = rebase(cfg.MSI.LogDir)
			for i, path := range cfg.Verify.WatchDirs {
				cfg.Verify.WatchDirs[i] = rebase(path)
			}
			body, err = json.Marshal(cfg)
			if err != nil {
				return err
			}
			if err = datadir.AtomicWrite(path, body); err != nil {
				return err
			}
			hash := sha256.Sum256(body)
			j.ConfigSHA256 = hex.EncodeToString(hash[:])
		} else if !os.IsNotExist(err) || j.State == "queued" {
			return err
		}
		b, err = json.MarshalIndent(j, "", "  ")
		if err != nil {
			return err
		}
		if err = datadir.AtomicWrite(filepath.Join(dir, "task.json"), b); err != nil {
			return err
		}
	}
	return nil
}
