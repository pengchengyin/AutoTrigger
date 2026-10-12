// Package ports owns the platform interfaces consumed by the core.
package ports

import (
	"autotrigger/internal/config"
	"autotrigger/pkg/schema"
	"context"
	"errors"
	"image"
	"io"
	"time"
)

var ErrUnsupported = errors.New("platform capability unsupported")
var ErrBusy = errors.New("another AutoTrigger instance holds the machine lock")

type Command struct {
	CommandLine string
	Program     string
	Args        []string
	Dir         string
	Method      string
	MSI         bool
	NonInstall  bool
	LogPath     string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}
type Exit struct {
	Code   int
	Err    error
	Stdout string
	Stderr string
}
type Activity struct {
	CPUTimeMS uint64
	IOBytes   uint64
	PIDs      []int
	Signature string
}
type Process interface {
	PID() int
	Done() <-chan Exit
	Activity() (Activity, error)
	Terminate(context.Context) (string, error)
	KillTree() error
	Close() error
}
type ProcessRunner interface {
	Start(context.Context, Command) (Process, error)
}
type FileOpener interface {
	OpenCommand(string) (Command, error)
	HasAssociation(string) bool
	ResolveShortcut(string) (string, []string, error)
}
type Detector interface {
	Detect(string) (FileType, error)
}
type FileType struct {
	Kind      string
	Extension string
	Disguised bool
	Family    string
	DLL       bool
}
type ArchiveEntry struct {
	Path      string
	Size      int64
	Directory bool
	Link      bool
}
type ArchiveExtractor interface {
	List(context.Context, string) ([]ArchiveEntry, error)
	Extract(context.Context, string, string, int64) error
}
type InstallerTrigger interface {
	Commands(string, FileType, string) ([]Command, error)
}
type DiskMounter interface {
	Mount(context.Context, string) (string, error)
	Dismount(context.Context, string) error
	MotW(string, string, string) error
}

type Rect struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
}

func (r Rect) Width() int  { return r.Right - r.Left }
func (r Rect) Height() int { return r.Bottom - r.Top }
func (r Rect) Contains(x, y int) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

type Window struct {
	Handle uintptr `json:"handle"`
	PID    int     `json:"pid"`
	Title  string  `json:"title"`
	Bounds Rect    `json:"bounds"`
	DPI    int     `json:"dpi"`
	Theme  string  `json:"theme"`
}
type Control struct {
	RuleAction   string  `json:"rule_action,omitempty"`
	UserTemplate string  `json:"user_template,omitempty"`
	ID           string  `json:"id"`
	Text         string  `json:"text"`
	Role         string  `json:"role"`
	Bounds       Rect    `json:"bounds"`
	Window       Window  `json:"window"`
	Source       string  `json:"source"`
	Confidence   float64 `json:"confidence"`
	Enabled      bool    `json:"enabled"`
	Checked      bool    `json:"checked"`
	Native       uintptr `json:"-"`
	RecognizeMS  int64   `json:"recognize_ms"`
}
type GUIAutomator interface {
	Windows([]int) ([]Window, error)
	Controls(Window) ([]Control, error)
	Screenshot(Window) (image.Image, error)
	Click(Control) error
	SetText(Control, string) error
	CloseWindow(Window) error
	Close() error
}
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type OCRText struct {
	Text  string   `json:"text"`
	Box   [4]Point `json:"box"`
	Score float64  `json:"score"`
}
type OCRInfo struct {
	Path      string
	Version   string
	Languages []string
	StartupMS int64
}
type OCREngine interface {
	Name() string
	Start(context.Context) (OCRInfo, error)
	Recognize(context.Context, image.Image) ([]OCRText, error)
	Stop() error
}
type GUIUpdate struct {
	Actions         []schema.GUIAction
	Evidence        []schema.Evidence
	Signature       string
	RequiresNetwork bool
	RequiresReboot  bool
	// Notes 记录本轮没有产生动作的原因（窗口与控件摘要），供调用方按调试级别输出，
	// 便于现场定位"界面看得见却点不动"这类问题。
	Notes []string
}
type GUIController interface {
	Step(context.Context, []int) (GUIUpdate, error)
	Dump(context.Context, []int, string) error
	Reset()
}

type UninstallEntry struct {
	Publisher       string `json:"publisher,omitempty"`
	Key             string `json:"key"`
	DisplayName     string `json:"display_name"`
	DisplayVersion  string `json:"display_version"`
	InstallLocation string `json:"install_location"`
	DisplayIcon     string `json:"display_icon"`
	UninstallString string `json:"uninstall_string"`
}
type DirState struct {
	Files    int      `json:"files"`
	Bytes    int64    `json:"bytes"`
	Modified int64    `json:"modified"`
	Names    []string `json:"names"`
}
type Event struct {
	RecordID uint64
	ID       int
	At       time.Time
	Detail   string
}
type Baseline struct {
	Registry       map[string]UninstallEntry `json:"registry"`
	Dirs           map[string]DirState       `json:"dirs"`
	EventWatermark uint64                    `json:"event_watermark"`
	CapturedAt     time.Time                 `json:"captured_at"`
	Truncated      bool                      `json:"truncated"`
	Warnings       []string                  `json:"warnings"`
}
type EvidenceSource interface {
	Snapshot(context.Context, config.Verify) (*Baseline, error)
	Events(context.Context, uint64) ([]Event, error)
	ValidArtifact(string) bool
}
type Lock interface {
	Acquire(context.Context, time.Duration) (func(), error)
}
type PrereqManager interface {
	Check(context.Context, config.Config) (schema.Prereq, map[string]interface{}, error)
}

// Logger 是日志出口。除通用 Log 外提供 Stage —— 阶段日志统一带 `【阶段】` 前缀，
// 保证"安装到哪一步"在界面与日志文件里都可读且格式一致。
type Logger interface {
	Log(string, string, ...interface{})
	Stage(string, string, string, ...interface{})
}
