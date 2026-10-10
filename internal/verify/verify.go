// Package verify arbitrates installation evidence independently of its platform source.
package verify

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/pkg/schema"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Observation struct {
	Evidence       []schema.Evidence
	SideEffects    bool
	Artifact       string
	PreInstalled   bool
	RequiresReboot bool
	BaselineDigest string
}
type Verifier struct {
	Source ports.EvidenceSource
	Config config.Verify
}

func Digest(b *ports.Baseline) string {
	// Capture time is deliberately excluded from content identity.
	data, _ := json.Marshal(struct {
		Registry  map[string]ports.UninstallEntry
		Dirs      map[string]ports.DirState
		Watermark uint64
	}{b.Registry, b.Dirs, b.EventWatermark})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum)
}

func PreInstalled(path string, b *ports.Baseline) bool {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	for _, suffix := range []string{"_setup", "-setup", " setup", "_installer", "-installer", "_x64", "_x86"} {
		name = strings.TrimSuffix(name, suffix)
	}
	if len(name) < 3 || name == "setup" || name == "install" || name == "installer" || name == "main" {
		return false
	}
	for _, r := range b.Registry {
		if strings.Contains(strings.ToLower(r.DisplayName), name) {
			return true
		}
	}
	for dir := range b.Dirs {
		if strings.EqualFold(filepath.Base(dir), name) {
			return true
		}
	}
	return false
}

func (v Verifier) Observe(ctx context.Context, path string, base *ports.Baseline, cmd ports.Command, exit *int, gui []schema.Evidence, output string, observed ...*ports.Baseline) (Observation, error) {
	after, history, err := v.settle(ctx)
	if err != nil {
		return Observation{}, err
	}
	o := Observation{Evidence: []schema.Evidence{}, Artifact: "unknown", PreInstalled: PreInstalled(path, base), BaselineDigest: Digest(base)}
	add := func(id, level, typ, source, detail string) {
		o.Evidence = append(o.Evidence, schema.Evidence{ID: id, Level: level, Type: typ, Source: source, Detail: detail, At: time.Now()})
	}
	removedRegistry, removedDirs := map[string]bool{}, map[string]bool{}
	for _, snapshot := range append(observed, history...) {
		for key, entry := range snapshot.Registry {
			_, existed := base.Registry[key]
			_, remains := after.Registry[key]
			if !existed && !remains && entry.DisplayName != "" && !removedRegistry[key] {
				replaced := false
				for _, current := range after.Registry {
					if strings.EqualFold(current.DisplayName, entry.DisplayName) {
						replaced = true
						break
					}
				}
				if !replaced {
					add("N4", "N", "registry_key_removed", key, "新增卸载项在稳定期消失："+entry.DisplayName)
					o.SideEffects = true
					removedRegistry[key] = true
				}
			}
		}
		if snapshot.Truncated || after.Truncated {
			continue
		}
		for dir := range snapshot.Dirs {
			_, existed := base.Dirs[dir]
			_, remains := after.Dirs[dir]
			if !existed && !remains && !removedDirs[dir] {
				add("N6", "N", "new_directory_removed", dir, "新增目录在稳定期消失，疑似回滚")
				o.SideEffects = true
				removedDirs[dir] = true
			}
		}
	}
	artifacts := []string{}
	newReg := map[string]ports.UninstallEntry{}
	for k, r := range after.Registry {
		old, exists := base.Registry[k]
		if !exists && r.DisplayName != "" {
			o.SideEffects = true
			newReg[k] = r
			add("S1", "S", "registry_uninstall_key", k, r.DisplayName+" "+r.DisplayVersion)
		} else if exists && r != old {
			o.SideEffects = true
			add("A3", "A", "registry_changed", k, r.DisplayName+" "+r.DisplayVersion)
		}
		if !exists || r != old {
			if r.DisplayIcon != "" {
				icon := strings.Trim(strings.Split(r.DisplayIcon, ",")[0], "\"")
				artifacts = append(artifacts, config.ExpandEnv(icon))
			}
			if r.InstallLocation != "" {
				for dir, state := range after.Dirs {
					if pathWithin(dir, config.ExpandEnv(r.InstallLocation)) {
						for _, n := range state.Names {
							artifacts = append(artifacts, filepath.Join(dir, n))
						}
					}
				}
			}
		}
	}
	for k, r := range base.Registry {
		if _, ok := after.Registry[k]; ok {
			continue
		}
		replacement := false
		for _, n := range after.Registry {
			if strings.EqualFold(n.DisplayName, r.DisplayName) {
				replacement = true
				break
			}
		}
		if !replacement {
			add("N4", "N", "registry_key_removed", k, r.DisplayName)
		}
	}
	newFiles, removed := 0, 0
	for dir, s := range after.Dirs {
		old, exists := base.Dirs[dir]
		if !exists {
			newFiles += 1 + s.Files
			o.SideEffects = true
		} else {
			if s.Files > old.Files {
				newFiles += s.Files - old.Files
				o.SideEffects = true
			}
			if s.Files < old.Files {
				removed += old.Files - s.Files
			}
			if s.Bytes != old.Bytes || s.Modified != old.Modified {
				o.SideEffects = true
			}
		}
		if !exists || s.Files != old.Files || s.Bytes != old.Bytes || s.Modified != old.Modified {
			for _, n := range s.Names {
				artifacts = append(artifacts, filepath.Join(dir, n))
			}
		}
	}
	for dir, s := range base.Dirs {
		if _, ok := after.Dirs[dir]; !ok {
			removed += 1 + s.Files
		}
	}
	if newFiles >= v.Config.MinNewFiles {
		add("S2", "S", "filesystem_diff", "filesystem", fmt.Sprintf("新增 %d 个文件或目录实体", newFiles))
	} else if o.SideEffects && len(newReg) == 0 {
		add("A3", "A", "filesystem_changed", "filesystem", "监控文件或目录发生变化")
	}
	if !base.Truncated && !after.Truncated && removed > v.Config.ShrinkTolerance && len(newReg) == 0 {
		add("N5", "N", "filesystem_shrunk", "filesystem", fmt.Sprintf("移除 %d 个文件或目录实体", removed))
	}
	if exit != nil {
		if cmd.MSI && MSICodeIsSuccess(*exit) {
			add("S3", "S", "msi_success_code", "msiexec", fmt.Sprint(*exit))
			o.RequiresReboot = *exit == 3010 || *exit == 1641
		} else if *exit == 0 {
			add("A1", "A", "generic_exit_code_zero", "process", "进程正常退出（退出码 0），不单独证明安装成功")
		} else {
			add("N1", "N", "failure_exit_code", "process", fmt.Sprint(*exit))
		}
	}
	if events, err := v.Source.Events(ctx, base.EventWatermark); err == nil {
		var lastSuccess uint64
		for _, e := range events {
			if e.ID == 11707 {
				lastSuccess = e.RecordID
				add("S4", "S", "msiinstaller_event", "eventlog", e.Detail)
			}
		}
		for _, e := range events {
			if e.ID == 11724 && e.RecordID > lastSuccess {
				add("N3", "N", "msi_uninstalled", "eventlog", e.Detail)
			}
		}
	}
	o.Evidence = append(o.Evidence, gui...)
	if detail := installerLogSuccess(output); detail != "" {
		add("B1", "B", "installer_log_success", "installer_log", detail)
	}
	if v.Config.RequireArtifact && o.SideEffects {
		o.Artifact = "failed"
		for _, p := range artifacts {
			if v.Source.ValidArtifact(p) {
				o.Artifact = "ok"
				break
			}
		}
	}
	return o, nil
}

func pathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (v Verifier) settle(ctx context.Context) (*ports.Baseline, []*ports.Baseline, error) {
	started := time.Now()
	max := time.Duration(v.Config.SettleMaxSec) * time.Second
	quiet := 0
	previous := ""
	var last *ports.Baseline
	history := []*ports.Baseline{}
	for {
		b, err := v.Source.Snapshot(ctx, v.Config)
		if err != nil {
			if last != nil {
				return last, history, nil
			}
			return nil, history, err
		}
		last = b
		history = append(history, b)
		digest := Digest(b)
		if digest == previous {
			quiet++
		} else {
			quiet = 0
		}
		previous = digest
		if time.Since(started) >= max || (time.Since(started) >= time.Duration(v.Config.SettleSec)*time.Second && quiet >= v.Config.QuietPolls) {
			return b, history, nil
		}
		select {
		case <-ctx.Done():
			return last, history, nil
		case <-time.After(500 * time.Millisecond):
		}
	}
}

type Decision struct {
	Status     string
	Confidence *string
	Conflicts  []schema.Evidence
}

func Decide(profile string, ev []schema.Evidence, sideEffects, preInstalled, started, timedout bool, artifact string) Decision {
	d := Decision{Status: "failed", Conflicts: []schema.Evidence{}}
	seen := map[string]bool{}
	s, a, b := 0, 0, 0
	meaningfulA := false
	for _, e := range ev {
		if e.Level == "N" {
			d.Conflicts = append(d.Conflicts, e)
			continue
		}
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		switch e.Level {
		case "S":
			s++
		case "A":
			a++
			if e.ID == "A2" || e.ID == "A3" {
				meaningfulA = true
			}
		case "B":
			b++
		}
	}
	confidence := func(c string) { d.Confidence = schema.String(c) }
	if len(d.Conflicts) > 0 {
		if sideEffects || s+a+b > 0 {
			d.Status = "install_partial"
			confidence("low")
		} else if timedout {
			d.Status = "killed_on_timeout"
		}
		return d
	}
	if !sideEffects && preInstalled && started {
		d.Status = "already_installed"
		confidence("high")
		return d
	}
	if s > 0 {
		d.Status = "install_completed"
		confidence("high")
		if artifact == "failed" {
			confidence("medium")
		}
		return d
	}
	if timedout {
		if sideEffects {
			d.Status = "install_partial"
			confidence("low")
		} else {
			d.Status = "killed_on_timeout"
		}
		return d
	}
	complete := a >= 2 && (seen["A1"] || seen["A2"])
	if profile == "loose" {
		complete = (a >= 1 && sideEffects) || (a >= 2 && meaningfulA) || b >= 2
	}
	if complete {
		d.Status = "install_completed"
		confidence("medium")
		if a == 0 || artifact == "failed" {
			confidence("low")
		}
		return d
	}
	if timedout {
		if sideEffects {
			d.Status = "install_partial"
			confidence("low")
		} else {
			d.Status = "killed_on_timeout"
		}
		return d
	}
	if a > 0 || b > 0 {
		d.Status = "install_unverified"
		confidence("low")
		return d
	}
	if started {
		d.Status = "triggered"
	}
	return d
}
