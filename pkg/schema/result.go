// Package schema defines the stable, platform-neutral integration contract.
package schema

import "time"

const Version = "2.8"

var Statuses = []string{"install_completed", "install_unverified", "install_partial", "already_installed", "triggered", "killed_on_timeout", "failed", "skipped"}

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
