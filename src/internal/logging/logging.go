// Package logging keeps human-readable messages out of stdout.
//
// 日志行格式固定为：`2006-01-02 15:04:05 [级别] 消息`，级别标签为中文
// （[调试] / [信息] / [警告] / [错误]），过滤仍使用英文级别名（debug/info/warn/error），
// 与配置项 `output.log_level` 和 `--log-level` 的取值保持一致。
package logging

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// 级别名。作为过滤键使用，取值与配置 / CLI 完全一致。
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// 阶段标签。统一以 `【阶段】` 作为消息前缀，让"现在装到哪一步了"一眼可读；
// 界面日志区与日志文件都据此扫读。
const (
	StageTask    = "任务"
	StageDetect  = "识别"
	StageExtract = "解压"
	StageEntry   = "入口"
	StageRun     = "执行"
	StageGUI     = "界面"
	StageVerify  = "判定"
	StageResult  = "结果"
	StageCleanup = "清理"
)

// TimestampLayout 是日志行的时间戳格式：本地时间、人类可读。
// （结果 JSON 里的时间字段仍按 R3 使用 RFC3339。）
const TimestampLayout = "2006-01-02 15:04:05"

var levelRank = map[string]int{LevelDebug: 0, LevelInfo: 1, LevelWarn: 2, LevelError: 3}

var levelTag = map[string]string{LevelDebug: "调试", LevelInfo: "信息", LevelWarn: "警告", LevelError: "错误"}

type Logger struct {
	mu    sync.Mutex
	Out   io.Writer
	Level string
}

// Log 写一行日志。
func (l *Logger) Log(level, format string, args ...interface{}) {
	l.emit(level, fmt.Sprintf(format, args...))
}

// Stage 写一行带阶段前缀的日志：`… [信息] 【执行】已启动…`。
func (l *Logger) Stage(level, stage, format string, args ...interface{}) {
	l.emit(level, "【"+stage+"】"+fmt.Sprintf(format, args...))
}

func (l *Logger) emit(level, msg string) {
	if levelRank[level] < levelRank[l.Level] {
		return
	}
	tag, ok := levelTag[level]
	if !ok {
		tag = level
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.Out, "%s [%s] %s\n", time.Now().Format(TimestampLayout), tag, msg)
}
