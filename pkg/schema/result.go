// Package schema defines the stable, platform-neutral integration contract.
package schema

import (
	"strconv"
	"strings"
	"time"
)

const Version = "2.8"

var Statuses = []string{"install_completed", "install_unverified", "install_partial", "already_installed", "triggered", "killed_on_timeout", "failed", "skipped"}

// 结果状态的中文标签。仅用于人类可读展示（界面状态栏、日志横幅）；
// 机器可读取值始终是 Statuses 里的英文标识，展示层不得改写（F10.2 / F20.3）。
var statusText = map[string]string{
	"install_completed":  "安装成功",
	"install_unverified": "安装成功（未验证）",
	"install_partial":    "部分完成",
	"already_installed":  "已安装（无需安装）",
	"triggered":          "已触发（未确认安装）",
	"killed_on_timeout":  "超时终止",
	"failed":             "失败",
	"skipped":            "跳过",
}

// StatusText 返回状态的中文标签；未知状态原样返回。
func StatusText(status string) string {
	if t, ok := statusText[status]; ok {
		return t
	}
	return status
}

// 汇总桶（summary.by_status 的键）的中文标签。
var bucketText = map[string]string{
	"installed":         "安装成功",
	"unverified":        "安装未验证",
	"partial":           "部分完成",
	"already_installed": "已安装",
	"triggered":         "已触发",
	"timeout":           "超时终止",
	"failed":            "失败",
	"skipped":           "跳过",
}

// BucketText 返回汇总桶的中文标签；未知桶原样返回。
func BucketText(bucket string) string {
	if t, ok := bucketText[bucket]; ok {
		return t
	}
	return bucket
}

// BucketOrder 是汇总桶的展示顺序：先成功、再待确认、最后失败与跳过，便于扫读。
var BucketOrder = []string{"installed", "unverified", "partial", "already_installed", "triggered", "timeout", "failed", "skipped"}

// SummaryLine 把汇总桶拼成一行中文统计，例如「样本总数 3｜安装成功 2｜失败 1」。
// 只列非零项，避免"安装成功 0"这类噪音；界面结果横幅与 CLI 日志汇总共用同一口径。
func SummaryLine(byStatus map[string]int, total int) string {
	parts := []string{"样本总数 " + strconv.Itoa(total)}
	for _, k := range BucketOrder {
		if n := byStatus[k]; n > 0 {
			parts = append(parts, BucketText(k)+" "+strconv.Itoa(n))
		}
	}
	return strings.Join(parts, "｜")
}

// 总体结论档位。界面据此用颜色与大号文字给出显式提示（F20.6）。
const (
	OutcomeEmpty   = "empty"
	OutcomeOK      = "ok"
	OutcomePartial = "partial"
	OutcomeFailed  = "failed"
	OutcomeSkipped = "skipped"
)

var outcomeText = map[string]string{
	OutcomeEmpty:   "没有可处理的样本",
	OutcomeOK:      "安装成功",
	OutcomePartial: "部分成功",
	OutcomeFailed:  "安装失败",
	OutcomeSkipped: "已跳过",
}

// OutcomeText 返回总体结论的中文标签。
func OutcomeText(outcome string) string {
	if t, ok := outcomeText[outcome]; ok {
		return t
	}
	return outcome
}

// OverallOutcome 由汇总桶推导总体结论。
// 口径：出现失败或超时即不算全胜；仅部分完成也不算成功；只剩"已安装 / 已触发 / 跳过"时按可接受处理。
func OverallOutcome(s Summary) string {
	if s.Total == 0 {
		return OutcomeEmpty
	}
	installed := s.ByStatus["installed"] + s.ByStatus["unverified"]
	partial := s.ByStatus["partial"]
	bad := s.ByStatus["failed"] + s.ByStatus["timeout"]
	done := s.ByStatus["already_installed"] + s.ByStatus["triggered"]
	switch {
	case bad > 0 && installed == 0 && partial == 0 && done == 0:
		return OutcomeFailed
	case bad > 0 || partial > 0:
		return OutcomePartial
	case installed > 0 || done > 0:
		return OutcomeOK
	default:
		return OutcomeSkipped
	}
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Evidence struct {
	ID     string    `json:"id"`
	Level  string    `json:"level"`
	Type   string    `json:"type"`
	Source string    `json:"source"`
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
}

type GUIAction struct {
	Window      string    `json:"window"`
	ControlText string    `json:"control_text"`
	Source      string    `json:"source"`
	Action      string    `json:"action"`
	OCRScore    *float64  `json:"ocr_score"`
	RecognizeMS *int64    `json:"recognize_ms"`
	At          time.Time `json:"at"`
}

type Attempt struct {
	Method           string      `json:"method"`
	Command          string      `json:"command"`
	Result           string      `json:"result"`
	ExitCode         *int        `json:"exit_code"`
	DurationSec      float64     `json:"duration_sec"`
	TerminationMode  string      `json:"termination_mode"`
	IdleResets       int         `json:"idle_resets"`
	InstallEvidence  []Evidence  `json:"install_evidence"`
	ArtifactCheck    string      `json:"artifact_check"`
	ArtifactRunnable *bool       `json:"artifact_runnable"`
	GUIActions       []GUIAction `json:"gui_actions"`
	StdoutSummary    string      `json:"stdout_summary"`
	StderrSummary    string      `json:"stderr_summary"`
	Error            *string     `json:"error"`
}

type FileResult struct {
	FilePath           string                 `json:"file_path"`
	FileSizeBytes      int64                  `json:"file_size_bytes"`
	FileHashSHA256     string                 `json:"file_hash_sha256"`
	DetectedType       string                 `json:"detected_type"`
	SelectionReason    string                 `json:"selection_reason"`
	ExtensionDisguised bool                   `json:"extension_disguised"`
	PreInstalled       bool                   `json:"pre_installed"`
	RequiresReboot     bool                   `json:"requires_reboot"`
	PlatformMeta       map[string]interface{} `json:"platform_meta"`
	Attempts           []Attempt              `json:"attempts"`
	EvidenceConflicts  []Evidence             `json:"evidence_conflicts"`
	InstallConfidence  *string                `json:"install_confidence"`
	FinalStatus        string                 `json:"final_status"`
	Notes              string                 `json:"notes"`
	JudgmentProfile    string                 `json:"judgment_profile"`
	BaselineDigest     *string                `json:"baseline_digest"`
}

func NewFile(path, profile string) FileResult {
	return FileResult{FilePath: path, JudgmentProfile: profile, PlatformMeta: map[string]interface{}{}, Attempts: []Attempt{}, EvidenceConflicts: []Evidence{}, FinalStatus: "skipped"}
}

type Prereq struct {
	ArchiveBackend       *string  `json:"archive_backend"`
	SevenZipVersion      *string  `json:"sevenzip_version"`
	ArchiveBackendStatus string   `json:"archive_backend_status"`
	MountBackend         *string  `json:"mount_backend"`
	MountBackendStatus   string   `json:"mount_backend_status"`
	OCREngine            string   `json:"ocr_engine"`
	OCREnginePath        *string  `json:"ocr_engine_path"`
	OCREngineStatus      string   `json:"ocr_engine_status"`
	OCREngineVersion     *string  `json:"ocr_engine_version"`
	OCREngineLanguages   []string `json:"ocr_engine_languages"`
	OCRStartupMS         int64    `json:"ocr_startup_ms"`
	OK                   bool     `json:"ok"`
	Issues               []string `json:"issues"`
}

func EmptyPrereq() Prereq {
	return Prereq{ArchiveBackendStatus: "missing", MountBackendStatus: "missing", OCREngine: "none", OCREngineStatus: "disabled", OCREngineLanguages: []string{}, Issues: []string{}}
}

type LayoutEntry struct {
	Path     string `json:"path"`
	Priority string `json:"priority"`
	Reason   string `json:"reason"`
	Matched  string `json:"matched"`
}

type Layout struct {
	InputKind           string        `json:"input_kind"`
	MediaKind           *string       `json:"media_kind"`
	ISOStrategyUsed     *string       `json:"iso_strategy_used"`
	MountDrive          *string       `json:"mount_drive"`
	DismountStatus      string        `json:"dismount_status"`
	MotWPolicy          string        `json:"motw_policy"`
	Entries             []LayoutEntry `json:"entries"`
	Excluded            []LayoutEntry `json:"excluded"`
	FallbackUsed        bool          `json:"fallback_used"`
	ConvergedAt         *string       `json:"converged_at"`
	SkippedAfterSuccess int           `json:"skipped_after_success"`
}

func NewLayout(kind, motw string) Layout {
	return Layout{InputKind: kind, DismountStatus: "not_applicable", MotWPolicy: motw, Entries: []LayoutEntry{}, Excluded: []LayoutEntry{}}
}

type Summary struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
	Flags    map[string]int `json:"flags"`
}

func Summarize(files []FileResult) Summary {
	s := Summary{Total: len(files), ByStatus: map[string]int{}, Flags: map[string]int{"requires_reboot": 0, "evidence_conflict": 0, "pre_installed": 0, "artifact_check_failed": 0}}
	buckets := map[string]string{"install_completed": "installed", "install_unverified": "unverified", "install_partial": "partial", "already_installed": "already_installed", "triggered": "triggered", "killed_on_timeout": "timeout", "failed": "failed", "skipped": "skipped"}
	for _, k := range buckets {
		s.ByStatus[k] = 0
	}
	for _, f := range files {
		s.ByStatus[buckets[f.FinalStatus]]++
		if f.RequiresReboot {
			s.Flags["requires_reboot"]++
		}
		if f.PreInstalled {
			s.Flags["pre_installed"]++
		}
		if len(f.EvidenceConflicts) > 0 {
			s.Flags["evidence_conflict"]++
		}
		for _, a := range f.Attempts {
			if a.ArtifactCheck == "failed" {
				s.Flags["artifact_check_failed"]++
				break
			}
		}
	}
	return s
}

type Result struct {
	SchemaVersion  string                 `json:"schema_version"`
	TaskID         string                 `json:"task_id"`
	ConfigDigest   string                 `json:"config_digest"`
	Error          *Error                 `json:"error"`
	StartedAt      time.Time              `json:"started_at"`
	FinishedAt     *time.Time             `json:"finished_at"`
	Platform       string                 `json:"platform"`
	PlatformMeta   map[string]interface{} `json:"platform_meta"`
	Prereq         Prereq                 `json:"prereq"`
	Summary        Summary                `json:"summary"`
	LayoutAnalysis Layout                 `json:"layout_analysis"`
	Results        []FileResult           `json:"results"`
	Truncated      bool                   `json:"truncated"`
	Meta           map[string]interface{} `json:"meta"`
}

func NewResult(id, platform string) Result {
	return Result{SchemaVersion: Version, TaskID: id, StartedAt: time.Now(), Platform: platform, PlatformMeta: map[string]interface{}{}, Prereq: EmptyPrereq(), Summary: Summarize(nil), LayoutAnalysis: NewLayout("file", "native"), Results: []FileResult{}, Meta: map[string]interface{}{}}
}

func String(v string) *string { return &v }
