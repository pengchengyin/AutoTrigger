// Package ui 提供「双击即用」的图形界面（PRD F20）。
//
// 设计约束与取舍：
//   - 界面**不重写任何业务逻辑**：只收集参数、转成与命令行**完全一致**的参数列表，
//     再复用 internal/cli 执行。因此界面的参数校验、退出码语义、结果 JSON
//     （F10）与命令行天然一致，不存在"两套行为"需要分别维护。
//   - 只使用标准库 syscall 绑定 Win32（user32 / gdi32 / comdlg32 / shell32 / ole32），
//     **不引入任何第三方 GUI 依赖**，保持"单文件、离线、零依赖"的交付形态（4.5 / 7.2）。
//   - 本文件为平台无关部分（参数与配置文件辅助），可在任意平台编译与测试。
package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/pkg/schema"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// DefaultTimeoutSec 是 CLI `--timeout` 的默认值（PRD F9.1 / DC20），界面用它判断
	// "是否偏离 CLI 默认值"，从而决定要不要下发 --timeout。
	DefaultTimeoutSec = 600
	// DefaultLogLevel 与配置默认值保持一致（7.5）。
	DefaultLogLevel = "info"
	// DefaultResultName 是界面预填的结果 JSON 文件名（与 CLI 默认同名）。
	DefaultResultName = "autotrigger-result.json"

	// UITimeoutDefaultValue / UITimeoutDefaultUnit 是界面超时输入框的预填值：5 分钟。
	//
	// 注意：**CLI 的 `--timeout` 默认仍是 600 秒**（DC20 已定案，此处不改）。界面之所以
	// 预填 5 分钟，是现场更常用的起步值（C45）；由于 300 ≠ 600，界面每次都会**显式**下发
	// `--timeout 300`，"等价命令行"与"命令预览"都能看到这个数 —— 不存在界面与命令行
	// 静默分叉（F20.3 的例外说明见 PRD）。
	UITimeoutDefaultValue = 5
	UITimeoutDefaultUnit  = UnitMinutes
)

// TimeoutUnit 是超时输入框的单位。取值与 TimeoutUnitLabels 的下标一一对应。
type TimeoutUnit int

const (
	UnitMinutes TimeoutUnit = iota
	UnitSeconds
)

// TimeoutUnitLabels 是单位下拉框的显示文本，顺序与 TimeoutUnit 取值一致。
var TimeoutUnitLabels = []string{"分钟", "秒"}

// LogLevelValues 是日志级别下拉框的取值，与 CLI `--log-level` / 配置 `output.log_level` 完全一致。
var LogLevelValues = []string{"error", "warn", "info", "debug"}

// LogLevelLabels 与 LogLevelValues 一一对应，用于下拉框显示（让级别含义自解释）。
var LogLevelLabels = []string{"error（仅错误）", "warn（警告及以上）", "info（常规）", "debug（调试）"}

// TimeoutSeconds 把「数值 + 单位」换算成 CLI 需要的秒数。
func TimeoutSeconds(value int, unit TimeoutUnit) (int, error) {
	if value < 0 {
		return 0, fmt.Errorf("超时数值不能为负数")
	}
	if unit == UnitMinutes {
		if value > 1<<20 {
			return 0, fmt.Errorf("超时数值过大")
		}
		return value * 60, nil
	}
	return value, nil
}

// SplitTimeout 把秒数拆回「数值 + 单位」，用于把已有参数回填到输入框。
// 能被 60 整除时优先用分钟显示，便于阅读（600 秒 → 10 分钟）。
func SplitTimeout(sec int) (int, TimeoutUnit) {
	if sec > 0 && sec%60 == 0 {
		return sec / 60, UnitMinutes
	}
	return sec, UnitSeconds
}

// LogLevelIndex 返回日志级别在下拉框中的下标；未知级别回落到 info。
func LogLevelIndex(level string) int {
	level = strings.TrimSpace(level)
	for i, v := range LogLevelValues {
		if v == level {
			return i
		}
	}
	return LogLevelIndex(DefaultLogLevel)
}

// Options 是图形界面收集到的参数，与 CLI 参数一一对应。
type Options struct {
	Input      string
	Output     string
	Config     string
	Recursive  bool
	NoArchive  bool
	NoOCR      bool
	Lock       bool
	TimeoutSec int
	LogLevel   string
}

// NewOptions 返回界面启动时的默认参数。
// resultDefault 为空时使用 exeDir 下的默认结果文件名。
func NewOptions(exeDir string) Options {
	timeout, err := TimeoutSeconds(UITimeoutDefaultValue, UITimeoutDefaultUnit)
	if err != nil {
		timeout = DefaultTimeoutSec
	}
	o := Options{
		Output:     filepath.Join(exeDir, DefaultResultName),
		Lock:       true,
		TimeoutSec: timeout,
		LogLevel:   DefaultLogLevel,
	}
	// 仅当配置文件确实存在时才预填：--config 指向不存在的文件会被判为配置错误
	// （退出码 3），空串则让工具按同目录 config.json 的既有规则自行发现。
	if p := ConfigPath(exeDir); fileExists(p) {
		o.Config = p
	}
	return o
}

// Args 把界面参数转成与 CLI 一致的参数列表。
//
// 只输出"偏离 CLI 默认值"的开关（例如 timeout 恰为 CLI 默认 600 秒时不下发 --timeout），
// 让命令预览与日志保持简洁；但会下发 --no-lock，因为锁默认开启，需要显式关闭。
// 界面预填的超时是 300 秒（5 分钟，见 UITimeoutDefaultValue），与 CLI 默认的 600 秒不同，
// 因此默认情况下会**显式**下发 `--timeout 300` —— 预览与实际参数一致，不会静默分叉。
func (o Options) Args() ([]string, error) {
	input := strings.TrimSpace(o.Input)
	if input == "" {
		return nil, fmt.Errorf("请先选择样本文件或目录")
	}
	args := []string{"--input", input}
	if s := strings.TrimSpace(o.Output); s != "" {
		args = append(args, "--output", s)
	}
	if s := strings.TrimSpace(o.Config); s != "" {
		args = append(args, "--config", s)
	}
	if o.Recursive {
		args = append(args, "--recursive")
	}
	if o.NoArchive {
		args = append(args, "--no-archive")
	}
	if o.NoOCR {
		args = append(args, "--no-ocr")
	}
	if !o.Lock {
		args = append(args, "--no-lock")
	}
	if o.TimeoutSec != DefaultTimeoutSec {
		if o.TimeoutSec < 0 {
			return nil, fmt.Errorf("超时秒数不能为负数")
		}
		args = append(args, "--timeout", strconv.Itoa(o.TimeoutSec))
	}
	if lv := strings.TrimSpace(o.LogLevel); lv != "" && lv != DefaultLogLevel {
		args = append(args, "--log-level", lv)
	}
	return args, nil
}

// CommandLine 返回等价的命令行文本，用于界面上的"命令预览/复制"。
// 它让使用者能直接把界面里录好的参数搬进脚本或第三方集成（F19）。
func (o Options) CommandLine(exeName string) (string, error) {
	args, err := o.Args()
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(exeName))
	for _, a := range args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " "), nil
}

// quoteArg 在参数含空白或引号时加双引号，与 cmd.exe 的解析一致。
func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// ConfigPath 返回工具默认加载的配置文件路径（exe 同目录 config.json）。
func ConfigPath(exeDir string) string {
	return filepath.Join(exeDir, "config.json")
}

// DefaultConfigJSON 返回覆盖全部字段的默认配置（缩进 JSON），
// 内容与 `AutoTrigger --dump-config` 等价，供界面"生成默认配置"使用。
func DefaultConfigJSON() (string, error) {
	b, err := json.MarshalIndent(config.Defaults(), "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// WriteConfigFile 写入一份覆盖全部字段的默认配置。
// overwrite 为 false 时，若文件已存在则不动（返回 wrote=false），避免抹掉使用者的改动。
func WriteConfigFile(path string, overwrite bool) (bool, error) {
	if fileExists(path) && !overwrite {
		return false, nil
	}
	body, err := DefaultConfigJSON()
	if err != nil {
		return false, err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, err
		}
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// ConfigEditorBody 返回配置编辑页应展示的 JSON 文本。
// explicit 为空时优先加载 exe 同目录 config.json；该文件不存在则展示内置默认配置，
// 但不把它伪装成已经落盘的用户配置。
func ConfigEditorBody(explicit, exeDir string) (body, resolvedPath string, fromDisk bool, err error) {
	resolvedPath = strings.TrimSpace(explicit)
	explicitPath := resolvedPath != ""
	if resolvedPath == "" {
		resolvedPath = ConfigPath(exeDir)
	}
	if fileExists(resolvedPath) {
		b, readErr := os.ReadFile(resolvedPath)
		if readErr != nil {
			return "", resolvedPath, false, readErr
		}
		return string(b), resolvedPath, true, nil
	}
	if explicitPath {
		return "", resolvedPath, false, fmt.Errorf("配置文件不存在：%s", resolvedPath)
	}
	body, err = DefaultConfigJSON()
	if err != nil {
		return "", resolvedPath, false, err
	}
	return body, resolvedPath, false, nil
}

// ValidateConfigJSONBody 校验一段尚未保存的配置 JSON。
// 复用 config.Load 的深合并、未知键 warning 与 Validate 规则，避免 GUI 出现第二套校验口径。
func ValidateConfigJSONBody(body, exeDir string) ([]string, error) {
	tmp, err := os.CreateTemp("", "autotrigger-ui-config-*.json")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	_, writeErr := tmp.WriteString(body)
	closeErr := tmp.Close()
	defer os.Remove(name)
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	_, warnings, err := config.Load(name, exeDir)
	return warnings, err
}

// WriteConfigEditorBody 先校验编辑器内容，再写入目标路径。
// 保存前校验能避免无效配置覆盖原有有效配置。
func WriteConfigEditorBody(path, body, exeDir string) ([]string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		p = ConfigPath(exeDir)
	}
	warnings, err := ValidateConfigJSONBody(body, exeDir)
	if err != nil {
		return warnings, err
	}
	if dir := filepath.Dir(p); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return warnings, err
		}
	}
	return warnings, os.WriteFile(p, []byte(body), 0o644)
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ResultSummary 是运行结束后展示用的结果摘要（取自结果 JSON 的 summary 段）。
type ResultSummary struct {
	Total    int
	ByStatus map[string]int
	Path     string
}

// ReadResultSummary 读取结果 JSON 的 summary。
// `--output -`、路径被改到别处或文件尚未写出时返回 ok=false，
// 此时界面只按退出码给结论（不编造统计）。
func ReadResultSummary(path string) (ResultSummary, bool) {
	p := strings.TrimSpace(path)
	if p == "" || p == "-" {
		return ResultSummary{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ResultSummary{}, false
	}
	var doc struct {
		Summary struct {
			Total    int            `json:"total"`
			ByStatus map[string]int `json:"by_status"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return ResultSummary{}, false
	}
	s := ResultSummary{Total: doc.Summary.Total, ByStatus: doc.Summary.ByStatus, Path: p}
	if s.ByStatus == nil {
		s.ByStatus = map[string]int{}
	}
	return s, true
}

// bannerWidth 是结果横幅的分隔线宽度（按字符计）。
const bannerWidth = 64

// ResultBanner 生成运行结束时追加到日志区的中文结果横幅。
//
// 第二个返回值是总体结论档位（schema.Outcome*），供状态栏决定颜色与文案；
// 读不到 summary 时按退出码兜底，只给"工具自身状态"，不编造安装结论。
func ResultBanner(exit int, s *ResultSummary, resultPath string) ([]string, string) {
	rule := strings.Repeat("━", bannerWidth)
	lines := []string{"", rule}
	outcome := ""
	if s != nil {
		outcome = schema.OverallOutcome(schema.Summary{Total: s.Total, ByStatus: s.ByStatus})
		lines = append(lines, "  "+schema.OutcomeText(outcome))
		if s.Total > 0 {
			lines = append(lines, "  "+schema.SummaryLine(s.ByStatus, s.Total))
		}
	} else {
		lines = append(lines, "  "+cli.ExitText(exit))
	}
	// 用「——」而不是括号：退出码中文说明里自带括号（如「处理完成（具体结果见结果 JSON）」），
	// 再套一层会写成「退出码 0（处理完成（…））」，嵌套括号读起来很别扭。
	lines = append(lines, "  工具状态：退出码 "+strconv.Itoa(exit)+" —— "+cli.ExitText(exit))
	if p := strings.TrimSpace(resultPath); p != "" && p != "-" {
		lines = append(lines, "  结果文件："+p)
		lines = append(lines, "  完整日志："+p+".log")
	}
	lines = append(lines, rule, "")
	return lines, outcome
}
