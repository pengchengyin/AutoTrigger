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
	"autotrigger/internal/config"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// DefaultTimeoutSec 与 CLI 的 --timeout 默认值保持一致（PRD F9.1 / DC20）。
	DefaultTimeoutSec = 600
	// DefaultLogLevel 与配置默认值保持一致（7.5）。
	DefaultLogLevel = "info"
	// DefaultResultName 是界面预填的结果 JSON 文件名（与 CLI 默认同名）。
	DefaultResultName = "autotrigger-result.json"
)

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
	o := Options{
		Output:     filepath.Join(exeDir, DefaultResultName),
		Lock:       true,
		TimeoutSec: DefaultTimeoutSec,
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
// 只输出"偏离默认值"的开关（例如 timeout 等于默认 600 时不下发 --timeout），
// 让命令预览与日志保持简洁；但会下发 --no-lock，因为锁默认开启，需要显式关闭。
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

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
