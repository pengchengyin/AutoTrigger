// Package config loads embedded defaults and validates incremental overrides.
package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type Config struct {
	TimeoutSec             int           `json:"timeout_sec"`
	StartupTimeoutSec      int           `json:"startup_timeout_sec"`
	IdleTimeoutSec         int           `json:"idle_timeout_sec"`
	TaskTimeoutSec         int           `json:"task_timeout_sec"`
	ProgressIntervalSec    int           `json:"progress_interval_sec"`
	ActivityCPUEpsilonMS   int           `json:"activity_cpu_epsilon_ms"`
	ActivityIOEpsilonBytes uint64        `json:"activity_io_epsilon_bytes"`
	Concurrency            int           `json:"concurrency"`
	AttemptIntervalSec     int           `json:"attempt_interval_sec"`
	MaxAttemptsPerFile     int           `json:"max_attempts_per_file"`
	Archive                Archive       `json:"archive"`
	ISO                    ISO           `json:"iso"`
	PackageLayout          PackageLayout `json:"package_layout"`
	GUI                    GUI           `json:"gui_automation"`
	OCR                    OCR           `json:"ocr"`
	Verify                 Verify        `json:"install_verify"`
	Lock                   Lock          `json:"lock"`
	DLLExportProbeCount    int           `json:"dll_export_probe_count"`
	Output                 Output        `json:"output"`
}

type Archive struct {
	MaxDepth            int    `json:"max_depth"`
	MaxExtractSizeBytes int64  `json:"max_extract_size_bytes"`
	SevenZipPath        string `json:"sevenzip_path"`
	RequireSevenZip     bool   `json:"require_sevenzip"`
	MinVersion          string `json:"min_version"`
	TempRoot            string `json:"temp_root"`
}
type ISO struct {
	Strategy                 string `json:"strategy"`
	AllowOSMedia             bool   `json:"allow_os_media"`
	AllowMediaDisc           bool   `json:"allow_media_disc"`
	AutorunAsP0              bool   `json:"autorun_as_p0"`
	MediaKindsFile           string `json:"media_kinds_file"`
	MountToolPath            string `json:"mount_tool_path"`
	MountTimeoutMS           int    `json:"mount_timeout_ms"`
	DismountRetry            int    `json:"dismount_retry"`
	KeepMountedUntilVerified bool   `json:"keep_mounted_until_verified"`
	KeepExtractedOnSuccess   bool   `json:"keep_extracted_on_success"`
	MotWPolicy               string `json:"motw_policy"`
}
type PackageLayout struct {
	Enabled             bool   `json:"enabled"`
	Mode                string `json:"mode"`
	EntryPatternsFile   string `json:"entry_patterns_file"`
	ExcludePatternsFile string `json:"exclude_patterns_file"`
	PreferShallowest    bool   `json:"prefer_shallowest"`
	PreferArchMatch     bool   `json:"prefer_arch_match"`
	ReadmeHints         bool   `json:"readme_hints"`
	MaxEntriesPhase1    int    `json:"max_entries_phase1"`
	FallbackToBrute     bool   `json:"fallback_to_brute"`
}
type GUI struct {
	Enabled               bool    `json:"enabled"`
	PollIntervalMS        int     `json:"poll_interval_ms"`
	ClickJitterPX         int     `json:"click_jitter_px"`
	MaxActionsPerWindow   int     `json:"max_actions_per_window"`
	KeywordFile           string  `json:"keyword_file"`
	TemplatePack          string  `json:"template_pack"`
	TemplateDir           string  `json:"template_dir"`
	TemplateDPIMode       string  `json:"template_dpi_mode"`
	TemplateTheme         string  `json:"template_theme"`
	TemplateMinSimilarity float64 `json:"template_min_similarity"`
	BuiltinTemplates      bool    `json:"builtin_templates"`
	DumpFailedShots       bool    `json:"dump_failed_shots"`
}
type OCR struct {
	Enabled               bool    `json:"enabled"`
	Engine                string  `json:"engine"`
	EnginePath            string  `json:"engine_path"`
	Transport             string  `json:"transport"`
	TCPPort               int     `json:"tcp_port"`
	StartupTimeoutMS      int     `json:"startup_timeout_ms"`
	RecognizeTimeoutMS    int     `json:"recognize_timeout_ms"`
	KeepAlive             bool    `json:"keep_alive"`
	RestartOnFailure      bool    `json:"restart_on_failure"`
	MaxRestarts           int     `json:"max_restarts"`
	RequireOCR            bool    `json:"require_ocr"`
	MinScore              float64 `json:"min_score"`
	ROIPaddingPX          int     `json:"roi_padding_px"`
	MinTextHeightPX       int     `json:"min_text_height_px"`
	MaxTextHeightPX       int     `json:"max_text_height_px"`
	CacheByScreenshotHash bool    `json:"cache_by_screenshot_hash"`
}
type Verify struct {
	JudgmentProfile    string   `json:"judgment_profile"`
	SettleSec          int      `json:"settle_sec"`
	QuietPolls         int      `json:"quiet_polls"`
	SettleMaxSec       int      `json:"settle_max_sec"`
	WatchDirs          []string `json:"watch_dirs"`
	WatchExclude       []string `json:"watch_exclude"`
	SnapshotDepth      int      `json:"snapshot_depth"`
	SnapshotMaxEntries int      `json:"snapshot_max_entries"`
	WriteBaseline      bool     `json:"write_baseline"`
	MinNewFiles        int      `json:"min_new_files"`
	ShrinkTolerance    int      `json:"shrink_tolerance"`
	RequireArtifact    bool     `json:"require_artifact"`
	LaunchCheck        bool     `json:"launch_check"`
	LaunchCheckSec     int      `json:"launch_check_sec"`
}
type Lock struct {
	Enabled bool `json:"enabled"`
	WaitSec int  `json:"wait_sec"`
}
type Output struct {
	Path                 string `json:"path"`
	Pretty               bool   `json:"pretty"`
	Journal              bool   `json:"journal"`
	KeepJournalOnSuccess bool   `json:"keep_journal_on_success"`
	LogFile              string `json:"log_file"`
	LogLevel             string `json:"log_level"`
	MaxFileSizeMB        int    `json:"max_file_size_mb"`
}

func Defaults() Config {
	var c Config
	if err := json.Unmarshal(defaultJSON, &c); err != nil {
		panic(err)
	}
	return c
}

func Load(explicit, exeDir string) (Config, []string, error) {
	c := Defaults()
	path := explicit
	if path == "" {
		path = filepath.Join(exeDir, "config.json")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return c, nil, nil
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, nil, fmt.Errorf("config %s: %w", path, err)
	}
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(b, &raw); err != nil {
		return c, nil, fmt.Errorf("config %s: %w", path, err)
	}
	if raw == nil {
		return c, nil, fmt.Errorf("config must be an object")
	}
	var warnings []string
	if err = merge(reflect.ValueOf(&c).Elem(), raw, "", &warnings); err != nil {
		return c, warnings, err
	}
	return c, warnings, c.Validate()
}

func merge(v reflect.Value, raw map[string]json.RawMessage, prefix string, warnings *[]string) error {
	fields := map[string]int{}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fields[t.Field(i).Tag.Get("json")] = i
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		path := prefix + k
		i, ok := fields[k]
		if !ok {
			*warnings = append(*warnings, "配置中出现未知字段（已忽略，请核对拼写或删除）："+path)
			continue
		}
		f := v.Field(i)
		if string(raw[k]) == "null" {
			return fmt.Errorf("%s: null is not a valid %s", path, f.Kind())
		}
		if f.Kind() == reflect.Struct {
			var child map[string]json.RawMessage
			if err := json.Unmarshal(raw[k], &child); err != nil {
				return fmt.Errorf("%s: expected object: %w", path, err)
			}
			if err := merge(f, child, path+".", warnings); err != nil {
				return err
			}
		} else {
			if err := json.Unmarshal(raw[k], f.Addr().Interface()); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}
	}
	return nil
}

func (c Config) Validate() error {
	nonnegative := map[string]int{"timeout_sec": c.TimeoutSec, "task_timeout_sec": c.TaskTimeoutSec, "activity_cpu_epsilon_ms": c.ActivityCPUEpsilonMS, "attempt_interval_sec": c.AttemptIntervalSec, "archive.max_depth": c.Archive.MaxDepth, "gui_automation.click_jitter_px": c.GUI.ClickJitterPX, "ocr.max_restarts": c.OCR.MaxRestarts, "ocr.roi_padding_px": c.OCR.ROIPaddingPX, "install_verify.settle_sec": c.Verify.SettleSec, "install_verify.snapshot_depth": c.Verify.SnapshotDepth, "install_verify.shrink_tolerance": c.Verify.ShrinkTolerance, "lock.wait_sec": c.Lock.WaitSec}
	for k, v := range nonnegative {
		if v < 0 || v > 864000 {
			return fmt.Errorf("%s: expected 0..864000", k)
		}
	}
	positive := map[string]int{"startup_timeout_sec": c.StartupTimeoutSec, "idle_timeout_sec": c.IdleTimeoutSec, "progress_interval_sec": c.ProgressIntervalSec, "max_attempts_per_file": c.MaxAttemptsPerFile, "gui_automation.poll_interval_ms": c.GUI.PollIntervalMS, "gui_automation.max_actions_per_window": c.GUI.MaxActionsPerWindow, "ocr.startup_timeout_ms": c.OCR.StartupTimeoutMS, "ocr.recognize_timeout_ms": c.OCR.RecognizeTimeoutMS, "ocr.min_text_height_px": c.OCR.MinTextHeightPX, "ocr.max_text_height_px": c.OCR.MaxTextHeightPX, "package_layout.max_entries_phase1": c.PackageLayout.MaxEntriesPhase1, "iso.mount_timeout_ms": c.ISO.MountTimeoutMS, "iso.dismount_retry": c.ISO.DismountRetry, "install_verify.quiet_polls": c.Verify.QuietPolls, "install_verify.settle_max_sec": c.Verify.SettleMaxSec, "install_verify.snapshot_max_entries": c.Verify.SnapshotMaxEntries, "install_verify.min_new_files": c.Verify.MinNewFiles, "install_verify.launch_check_sec": c.Verify.LaunchCheckSec, "dll_export_probe_count": c.DLLExportProbeCount, "output.max_file_size_mb": c.Output.MaxFileSizeMB}
	for k, v := range positive {
		if v < 1 || v > 864000 {
			return fmt.Errorf("%s: expected 1..864000", k)
		}
	}
	if c.Concurrency != 1 {
		return fmt.Errorf("concurrency: this release requires 1 to preserve evidence attribution")
	}
	if c.Archive.MaxExtractSizeBytes < 1 {
		return fmt.Errorf("archive.max_extract_size_bytes: must be positive")
	}
	if c.Verify.SettleMaxSec < c.Verify.SettleSec {
		return fmt.Errorf("install_verify.settle_max_sec: must be >= settle_sec")
	}
	if c.OCR.TCPPort < 0 || c.OCR.TCPPort > 65535 {
		return fmt.Errorf("ocr.tcp_port: expected 0..65535")
	}
	if c.OCR.MinTextHeightPX > c.OCR.MaxTextHeightPX {
		return fmt.Errorf("ocr.min_text_height_px: exceeds max_text_height_px")
	}
	for k, v := range map[string]float64{"ocr.min_score": c.OCR.MinScore, "gui_automation.template_min_similarity": c.GUI.TemplateMinSimilarity} {
		if v < 0 || v > 1 {
			return fmt.Errorf("%s: expected [0,1]", k)
		}
	}
	enums := []struct{ key, value, allowed string }{{"install_verify.judgment_profile", c.Verify.JudgmentProfile, "loose strict"}, {"iso.strategy", c.ISO.Strategy, "extract_first mount_first extract_only mount_only"}, {"iso.motw_policy", c.ISO.MotWPolicy, "native preserve clear"}, {"package_layout.mode", c.PackageLayout.Mode, "smart brute"}, {"gui_automation.template_pack", c.GUI.TemplatePack, "merged builtin external"}, {"gui_automation.template_dpi_mode", c.GUI.TemplateDPIMode, "auto 100 125 150 200"}, {"gui_automation.template_theme", c.GUI.TemplateTheme, "auto light dark win7classic"}, {"ocr.engine", c.OCR.Engine, "paddleocr_json rapidocr_json umi_ocr tesseract windows none"}, {"ocr.transport", c.OCR.Transport, "stdio tcp http"}, {"output.log_level", c.Output.LogLevel, "debug info warn error"}}
	for _, e := range enums {
		found := false
		for _, s := range strings.Fields(e.allowed) {
			if s == e.value {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s: expected one of %s", e.key, e.allowed)
		}
	}
	if c.Verify.LaunchCheck {
		return fmt.Errorf("install_verify.launch_check: not supported in this release (DC19)")
	}
	return nil
}

func (c Config) Digest() string {
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", sum)
}

func ExpandEnv(path string) string {
	for {
		start := strings.IndexByte(path, '%')
		if start < 0 {
			break
		}
		end := strings.IndexByte(path[start+1:], '%')
		if end < 0 {
			break
		}
		end += start + 1
		key := path[start+1 : end]
		value, ok := os.LookupEnv(key)
		if !ok {
			for _, entry := range os.Environ() {
				parts := strings.SplitN(entry, "=", 2)
				if strings.EqualFold(parts[0], key) {
					value = parts[1]
					ok = true
					break
				}
			}
		}
		if !ok {
			break
		}
		path = path[:start] + value + path[end+1:]
	}
	return os.ExpandEnv(path)
}
