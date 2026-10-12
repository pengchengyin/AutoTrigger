package verify

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Files summarizes shallow directories without recursively enumerating every file.
func Files(ctx context.Context, c config.Verify) (map[string]ports.DirState, bool, []string) {
	out := map[string]ports.DirState{}
	warnings := []string{}
	truncated := false
	deadline := time.Now().Add(450 * time.Millisecond)
	count := 0
	excluded := func(path string) bool {
		for _, r := range c.WatchExclude {
			root := config.ExpandEnv(r)
			if !strings.Contains(root, "%") && pathWithin(path, root) {
				return true
			}
		}
		return false
	}
	var walk func(string, int)
	walk = func(dir string, depth int) {
		if ctx.Err() != nil || time.Now().After(deadline) || count >= c.SnapshotMaxEntries {
			truncated = true
			return
		}
		if excluded(dir) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, dir+": "+err.Error())
				truncated = true
			}
			return
		}
		s := ports.DirState{Names: []string{}}
		count++
		for _, e := range entries {
			if excluded(filepath.Join(dir, e.Name())) {
				continue
			}
			if count >= c.SnapshotMaxEntries || time.Now().After(deadline) {
				truncated = true
				break
			}
			if e.Type()&os.ModeSymlink != 0 || e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			count++
			s.Files++
			s.Bytes += info.Size()
			if info.ModTime().UnixNano() > s.Modified {
				s.Modified = info.ModTime().UnixNano()
			}
			if strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
				s.Names = append(s.Names, e.Name())
			}
		}
		out[dir] = s
		if depth < c.SnapshotDepth {
			for _, e := range entries {
				if e.IsDir() && e.Type()&os.ModeSymlink == 0 {
					walk(filepath.Join(dir, e.Name()), depth+1)
				}
			}
		}
	}
	for _, root := range c.WatchDirs {
		expanded := config.ExpandEnv(root)
		if strings.Contains(expanded, "%") {
			warnings = append(warnings, "无法解析监控目录（含未展开的环境变量）："+root)
			continue
		}
		walk(expanded, 0)
	}
	if truncated {
		warnings = append(warnings, "文件系统快照不完整（权限、条目数或 450ms 预算限制）：文件差异证据已禁用")
	}
	return out, truncated, warnings
}
