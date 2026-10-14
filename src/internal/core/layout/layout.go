// Package layout prioritizes package entrances and always excludes destructive helpers.
package layout

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/resources"
	"autotrigger/pkg/schema"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Candidate struct {
	Path     string
	Relative string
	Type     ports.FileType
	Priority int
	Reason   string
	Matched  string
	Size     int64
	Args     []string
}
type Analyzer struct {
	Detector ports.Detector
	Rules    resources.Pack
	Config   config.Config
}

func SafeRelative(name string) bool {
	p := strings.ReplaceAll(name, "\\", "/")
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, ":") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func (a Analyzer) Analyze(root string, entries []ports.ArchiveEntry, readHints bool) ([]Candidate, schema.Layout, error) {
	l := schema.NewLayout("directory", a.Config.ISO.MotWPolicy)
	var candidates []Candidate
	builtin, err := (resources.Pack{Mode: "builtin"}).Rules("exclude_entries.json")
	if err != nil {
		return nil, l, err
	}
	extra, err := a.Rules.Rules(a.Config.PackageLayout.ExcludePatternsFile)
	if err != nil {
		return nil, l, err
	}
	excludes := append(builtin, extra...)
	entryRules, err := a.Rules.Rules(a.Config.PackageLayout.EntryPatternsFile)
	if err != nil {
		return nil, l, err
	}
	for _, e := range entries {
		if e.Directory {
			continue
		}
		if !SafeRelative(e.Path) || e.Link {
			return nil, l, fmt.Errorf("unsafe package entry: %s", e.Path)
		}
		rel := filepath.FromSlash(strings.ReplaceAll(e.Path, "\\", "/"))
		path := filepath.Join(root, rel)
		ft := ports.FileType{Kind: "unknown", Extension: strings.ToLower(filepath.Ext(rel))}
		ft.Kind = kindFromExtension(ft.Extension)
		if root != "" {
			if _, err := os.Stat(path); err == nil {
				ft, err = a.Detector.Detect(path)
				if err != nil {
					return nil, l, err
				}
			}
		}
		c := Candidate{Path: path, Relative: rel, Type: ft, Priority: 5, Reason: "fallback_p5", Size: e.Size}
		name := strings.ToLower(filepath.Base(rel))
		for _, r := range excludes {
			if r.ID == "help" {
				continue
			}
			rx, err := regexp.Compile("(?i)" + r.Pattern)
			if err != nil {
				return nil, l, fmt.Errorf("exclude %s: %w", r.ID, err)
			}
			if rx.MatchString(name) {
				c.Reason = "excluded_blacklist"
				c.Matched = r.Pattern
				break
			}
		}
		if c.Reason == "excluded_blacklist" {
			l.Excluded = append(l.Excluded, schema.LayoutEntry{Path: rel, Reason: c.Reason, Matched: c.Matched})
			candidates = append(candidates, c)
			continue
		}
		for _, r := range entryRules {
			rx, err := regexp.Compile("(?i)" + r.Pattern)
			if err != nil {
				return nil, l, err
			}
			if rx.MatchString(name) {
				switch r.Action {
				case "P1":
					c.Priority = 1
					c.Reason = "entry_p1_named"
				case "P3":
					c.Priority = 3
					c.Reason = "entry_p3_prereq"
				case "P4":
					c.Priority = 4
					c.Reason = "entry_p4_main"
				}
			}
		}
		if c.Priority == 5 && (ft.Kind == "msi" || ft.Family != "") {
			c.Priority = 2
			c.Reason = "entry_p2_installer"
		}
		if c.Priority == 5 && ft.Kind == "pe_executable" && strings.Count(strings.ReplaceAll(rel, "\\", "/"), "/") == 0 {
			c.Priority = 4
			c.Reason = "entry_p4_main"
		}
		if !executable(ft.Kind) && ft.Kind != "archive" && ft.Kind != "disk_image" {
			c.Priority, c.Reason = 5, "non_executable_entry"
		}
		candidates = append(candidates, c)
	}
	if readHints {
		a.applyHints(root, candidates)
	}
	kind, err := a.MediaKind(entries, root)
	if err != nil {
		return nil, l, err
	}
	if kind == "malware_lure" {
		for i := range candidates {
			c := &candidates[i]
			if c.Reason != "excluded_blacklist" && c.Type.Kind == "shortcut" && filepath.Dir(c.Relative) == "." {
				c.Priority, c.Reason = 0, "entry_p0_lure"
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		x, y := candidates[i], candidates[j]
		if x.Priority != y.Priority {
			return x.Priority < y.Priority
		}
		if a.Config.PackageLayout.PreferArchMatch {
			ax, ay := archRank(x.Relative), archRank(y.Relative)
			if ax != ay {
				return ax < ay
			}
		}
		if a.Config.PackageLayout.PreferShallowest {
			dx, dy := strings.Count(filepath.ToSlash(x.Relative), "/"), strings.Count(filepath.ToSlash(y.Relative), "/")
			if dx != dy {
				return dx < dy
			}
		}
		return strings.ToLower(x.Relative) < strings.ToLower(y.Relative)
	})
	// Prerequisites run first and cannot themselves converge the main package.
	if a.Config.PackageLayout.Enabled && a.Config.PackageLayout.Mode == "smart" {
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Priority == 3 && candidates[j].Priority != 3 })
	}
	for _, c := range candidates {
		if c.Reason != "excluded_blacklist" {
			l.Entries = append(l.Entries, schema.LayoutEntry{Path: c.Relative, Priority: fmt.Sprintf("P%d", c.Priority), Reason: c.Reason})
		}
	}
	return candidates, l, nil
}

func (a Analyzer) applyHints(root string, candidates []Candidate) {
	for at, c := range candidates {
		name := strings.ToLower(filepath.Base(c.Relative))
		autorun := name == "autorun.inf" && a.Config.ISO.AutorunAsP0
		readme := a.Config.PackageLayout.ReadmeHints && (strings.Contains(name, "readme") || strings.Contains(name, "安装说明") || strings.Contains(name, "使用说明"))
		if !autorun && !readme {
			continue
		}
		f, err := os.Open(c.Path)
		if err != nil {
			continue
		}
		buf := make([]byte, 65536)
		n, _ := f.Read(buf)
		f.Close()
		text := strings.ToLower(string(buf[:n]))
		if !executable(c.Type.Kind) && (autorun || readme && entryHintText(text)) {
			candidates[at].Reason = "entry_hint_document"
		}
		for i := range candidates {
			target := &candidates[i]
			if target.Reason == "excluded_blacklist" || !executable(target.Type.Kind) {
				continue
			}
			base := strings.ToLower(filepath.Base(target.Relative))
			if readme && strings.Contains(text, base) && (strings.Contains(text, "双击") || strings.Contains(text, "运行") || strings.Contains(text, "执行") || strings.Contains(text, "run ")) {
				target.Priority = 0
				target.Reason = "entry_p0_manifest"
			}
			if autorun {
				for _, line := range strings.Split(text, "\n") {
					parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
					if len(parts) != 2 || (parts[0] != "open" && parts[0] != "shellexecute") {
						continue
					}
					value := strings.TrimSpace(parts[1])
					if strings.HasPrefix(value, "\"") {
						end := strings.Index(value[1:], "\"")
						if end >= 0 {
							value = value[1 : end+1]
						}
					} else {
						if at := strings.IndexAny(value, " \t"); at >= 0 {
							value = value[:at]
						}
					}
					value = strings.ReplaceAll(value, "\\", "/")
					rel := filepath.ToSlash(target.Relative)
					if SafeRelative(value) && (strings.EqualFold(value, rel) || strings.EqualFold(filepath.Join(filepath.Dir(c.Relative), filepath.FromSlash(value)), target.Relative)) {
						target.Priority = 0
						target.Reason = "entry_p0_manifest"
					}
				}
			}
		}
	}
}

func entryHintText(text string) bool {
	command := strings.Contains(text, "双击") || strings.Contains(text, "运行") || strings.Contains(text, "执行") || strings.Contains(text, "run ")
	if !command {
		return false
	}
	for _, extension := range []string{".exe", ".msi", ".bat", ".cmd", ".ps1", ".lnk"} {
		if strings.Contains(text, extension) {
			return true
		}
	}
	return false
}

func (a Analyzer) MediaKind(entries []ports.ArchiveEntry, roots ...string) (string, error) {
	rules, err := a.Rules.Rules(a.Config.ISO.MediaKindsFile)
	if err != nil {
		return "", err
	}
	for _, kind := range []string{"os_install_media", "media_disc"} {
		for _, r := range rules {
			if r.Kind != kind {
				continue
			}
			rx, err := regexp.Compile("(?i)" + r.Pattern)
			if err != nil {
				return "", err
			}
			for _, e := range entries {
				if rx.MatchString(strings.ReplaceAll(e.Path, "\\", "/")) {
					return kind, nil
				}
			}
		}
	}
	rootCount := 0
	lure := false
	shortcut, payload, installer := false, false, false
	for _, e := range entries {
		p := strings.ReplaceAll(e.Path, "\\", "/")
		if e.Directory {
			continue
		}
		ft := ports.FileType{Kind: kindFromExtension(strings.ToLower(filepath.Ext(p)))}
		if len(roots) > 0 && roots[0] != "" {
			ft, err = a.Detector.Detect(filepath.Join(roots[0], filepath.FromSlash(p)))
			if err != nil {
				return "", err
			}
		}
		payload = payload || ft.Kind == "pe_executable" || ft.Kind == "script"
		for _, r := range rules {
			if r.Kind != "installer_media" || !executable(ft.Kind) || ft.Kind == "shortcut" {
				continue
			}
			rx, err := regexp.Compile("(?i)" + r.Pattern)
			if err != nil {
				return "", err
			}
			installer = installer || rx.MatchString(p)
		}
		if !e.Directory && !strings.Contains(p, "/") {
			rootCount++
			switch strings.ToLower(filepath.Ext(p)) {
			case ".lnk", ".js", ".vbs", ".hta", ".wsf":
				lure = true
			}
			shortcut = shortcut || ft.Kind == "shortcut"
		}
	}
	if !installer && ((rootCount >= 1 && rootCount <= 3 && lure) || (shortcut && payload)) {
		return "malware_lure", nil
	}
	if installer {
		return "installer_media", nil
	}
	return "data_disc", nil
}
func archRank(path string) int {
	n := strings.ToLower(path)
	if strings.Contains(n, "x64") || strings.Contains(n, "amd64") || strings.Contains(n, "win64") {
		return 0
	}
	if strings.Contains(n, "x86") || strings.Contains(n, "win32") {
		return 2
	}
	return 1
}
func executable(kind string) bool {
	return kind == "pe_executable" || kind == "pe_dll" || kind == "msi" || kind == "script" || kind == "shortcut"
}
func kindFromExtension(ext string) string {
	switch ext {
	case ".exe", ".com", ".scr":
		return "pe_executable"
	case ".dll":
		return "pe_dll"
	case ".msi", ".msp":
		return "msi"
	case ".bat", ".cmd", ".ps1", ".js", ".vbs", ".wsf", ".hta":
		return "script"
	case ".lnk":
		return "shortcut"
	case ".iso", ".img", ".nrg", ".mds", ".cue":
		return "disk_image"
	case ".zip", ".7z", ".rar", ".tar", ".gz":
		return "archive"
	default:
		return "unknown"
	}
}

func Scan(root string, recursive bool) ([]ports.ArchiveEntry, error) {
	out := []ports.ArchiveEntry{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			if !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		s, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, ports.ArchiveEntry{Path: rel, Size: s.Size(), Link: d.Type()&os.ModeSymlink != 0})
		return nil
	})
	return out, err
}
