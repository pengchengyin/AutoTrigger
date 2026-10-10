// Package core schedules samples through injected platform ports.
package core

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/layout"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/logging"
	"autotrigger/internal/resources"
	"autotrigger/internal/verify"
	"autotrigger/pkg/schema"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Engine struct {
	Config          config.Config
	Runner          ports.ProcessRunner
	Detector        ports.Detector
	Trigger         ports.InstallerTrigger
	Opener          ports.FileOpener
	Extractor       ports.ArchiveExtractor
	Mounter         ports.DiskMounter
	Evidence        ports.EvidenceSource
	GUI             ports.GUIController
	Log             ports.Logger
	Pack            resources.Pack
	Result          *schema.Result
	OnFile          func(schema.FileResult) error
	Recursive       bool
	DisableArchives bool
	DumpDir         string
	IgnorePaths     []string
	MissingArchive  bool
	// ResultPath 是结果 JSON 的最终写入路径（`-` 表示写 stdout）。
	// 失败样本的 MSI 详细日志默认留在它旁边，便于现场排查（F15.10）。
	ResultPath string
	Err        error
	temps      []string
}

func (e *Engine) Run(ctx context.Context, input string) error {
	if e.Config.TaskTimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(e.Config.TaskTimeoutSec)*time.Second)
		defer cancel()
	}
	defer func() {
		for _, dir := range e.temps {
			if err := os.RemoveAll(dir); err != nil {
				e.Log.Stage(logging.LevelWarn, logging.StageCleanup, "临时目录清理失败：%s（%v）", dir, err)
			}
		}
	}()
	s, err := os.Stat(input)
	if err != nil {
		return err
	}
	if s.IsDir() {
		entries, err := layout.Scan(input, e.Recursive)
		if err != nil {
			return err
		}
		e.Log.Stage(logging.LevelInfo, logging.StageTask, "开始处理目录：%s（递归=%t，共发现 %d 个文件）", input, e.Recursive, len(entries))
		e.packageRun(ctx, input, entries, 0, &e.Result.LayoutAnalysis)
	} else {
		e.Log.Stage(logging.LevelInfo, logging.StageTask, "开始处理样本：%s（%s）", input, humanSize(s.Size()))
		e.add(e.file(ctx, input, "single_file", 0, map[string]bool{}))
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			e.Log.Stage(logging.LevelWarn, logging.StageTask, "收到中止请求：停止启动新的尝试，已完成部分照常采证并写出结果")
			return context.Canceled
		}
		e.Log.Stage(logging.LevelWarn, logging.StageTask, "任务总时长已达上限（task_timeout_sec=%d 秒）：剩余样本不再处理", e.Config.TaskTimeoutSec)
	}
	return e.Err
}
func (e *Engine) add(f schema.FileResult) {
	e.Result.Results = append(e.Result.Results, f)
	e.Result.Summary = schema.Summarize(e.Result.Results)
	if e.OnFile != nil {
		if err := e.OnFile(f); err != nil {
			e.Err = err
		}
	}
	e.Log.Stage(logging.LevelInfo, logging.StageResult, "%s → %s%s", f.FilePath, schema.StatusText(f.FinalStatus), noteSuffix(f.Notes))
}
func (e *Engine) ignored(path string) bool {
	for _, p := range e.IgnorePaths {
		if strings.EqualFold(path, p) {
			return true
		}
	}
	return false
}

func (e *Engine) packageRun(ctx context.Context, root string, entries []ports.ArchiveEntry, depth int, analysis *schema.Layout) bool {
	filtered := []ports.ArchiveEntry{}
	for _, entry := range entries {
		if !e.ignored(filepath.Join(root, entry.Path)) {
			filtered = append(filtered, entry)
		}
	}
	analyzer := layout.Analyzer{Detector: e.Detector, Rules: e.Pack, Config: e.Config}
	candidates, l, err := analyzer.Analyze(root, filtered, true)
	if err != nil {
		e.Err = fmt.Errorf("package analysis: %w", err)
		return false
	}
	e.Log.Stage(logging.LevelInfo, logging.StageEntry, "结构分析完成：候选入口 %d 个，排除 %d 个（排除项见结果 JSON 的 layout_analysis）", len(candidates), len(l.Excluded))
	l.InputKind = analysis.InputKind
	if l.InputKind == "file" {
		l.InputKind = "directory"
	}
	l.MediaKind = analysis.MediaKind
	l.ISOStrategyUsed = analysis.ISOStrategyUsed
	l.MountDrive = analysis.MountDrive
	l.DismountStatus = analysis.DismountStatus
	*analysis = l
	packageScope := depth > 0
	for _, c := range candidates {
		packageScope = packageScope || c.Reason == "entry_p0_manifest"
	}
	converged := false
	phase1 := 0
	for _, c := range candidates {
		if e.Err != nil {
			return converged
		}
		if c.Reason == "excluded_blacklist" {
			f := schema.NewFile(c.Path, e.Config.Verify.JudgmentProfile)
			f.FileSizeBytes = c.Size
			f.DetectedType = c.Type.Kind
			f.SelectionReason = c.Reason
			f.Notes = "excluded_blacklist: " + c.Matched
			e.Log.Stage(logging.LevelInfo, logging.StageEntry, "排除（命中禁执行清单 %s）：%s", c.Matched, c.Path)
			e.add(f)
			continue
		}
		reason := c.Reason
		if !packageScope && c.Priority == 5 {
			analysis.FallbackUsed = true
		}
		skip := ""
		if converged {
			skip = "skipped_after_success"
			reason = skip
			analysis.SkippedAfterSuccess++
		} else if ctx.Err() != nil {
			skip = "task_timeout"
			if errors.Is(ctx.Err(), context.Canceled) {
				skip = "interrupted"
			}
		} else if packageScope && e.Config.PackageLayout.Enabled && e.Config.PackageLayout.Mode == "smart" {
			if c.Priority <= 4 {
				phase1++
				if phase1 > e.Config.PackageLayout.MaxEntriesPhase1 {
					if e.Config.PackageLayout.FallbackToBrute {
						reason = "fallback_p5"
						analysis.FallbackUsed = true
					} else {
						skip = "phase1_entry_limit"
					}
				}
			} else {
				if !e.Config.PackageLayout.FallbackToBrute {
					skip = "fallback_disabled"
				} else {
					analysis.FallbackUsed = true
				}
			}
		}
		if skip != "" {
			f := schema.NewFile(c.Path, e.Config.Verify.JudgmentProfile)
			f.FileSizeBytes = c.Size
			f.DetectedType = c.Type.Kind
			f.SelectionReason = reason
			f.Notes = skip
			e.Log.Stage(logging.LevelInfo, logging.StageEntry, "跳过（%s）：%s", skipText(skip), c.Path)
			e.add(f)
			continue
		}
		e.Log.Stage(logging.LevelInfo, logging.StageEntry, "选用入口：%s（优先级 P%d，判定依据 %s）", c.Path, c.Priority, c.Reason)
		f := e.file(ctx, c.Path, reason, depth, map[string]bool{})
		e.add(f)
		if packageScope && f.FinalStatus == "install_completed" && c.Priority != 3 && e.Config.PackageLayout.Enabled && e.Config.PackageLayout.Mode == "smart" {
			converged = true
			analysis.ConvergedAt = schema.String(c.Relative)
			e.Log.Stage(logging.LevelInfo, logging.StageEntry, "已达成安装成功，按「成功即收敛」策略停止执行本包后续候选")
		}
	}
	return converged
}

func (e *Engine) file(ctx context.Context, path, reason string, depth int, seen map[string]bool) schema.FileResult {
	f := schema.NewFile(path, e.Config.Verify.JudgmentProfile)
	f.SelectionReason = reason
	started := time.Now()
	if ctx.Err() != nil {
		f.Notes = "task_timeout"
		return f
	}
	s, err := os.Stat(path)
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = err.Error()
		return f
	}
	f.FileSizeBytes = s.Size()
	if s.Size() == 0 {
		f.Notes = "empty_file"
		e.Log.Stage(logging.LevelWarn, logging.StageDetect, "跳过空文件（0 字节）：%s", path)
		return f
	}
	fileCtx := ctx
	cancel := func() {}
	if e.Config.TimeoutSec > 0 {
		fileCtx, cancel = context.WithDeadline(ctx, started.Add(time.Duration(e.Config.TimeoutSec)*time.Second))
	}
	defer cancel()
	ft, err := e.Detector.Detect(path)
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = err.Error()
		return f
	}
	f.DetectedType = ft.Kind
	f.ExtensionDisguised = ft.Disguised
	if matched, err := layout.ExecutionExclusion(path, e.Pack, e.Config); err != nil {
		f.FinalStatus, f.Notes = "failed", err.Error()
		return f
	} else if matched != "" {
		f.SelectionReason, f.Notes = "excluded_blacklist", "excluded_blacklist: "+matched
		return f
	}
	e.Log.Stage(logging.LevelInfo, logging.StageDetect, "识别结果：%s → %s（%s）", path, ft.Kind, humanSize(s.Size()))
	if ft.Disguised {
		e.Log.Stage(logging.LevelWarn, logging.StageDetect, "扩展名与真实类型不符（疑似伪装），按真实类型 %s 处理：%s", ft.Kind, path)
	}
	hash, err := hashFile(fileCtx, path)
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = "hash: " + err.Error()
		return f
	}
	f.FileHashSHA256 = hash
	if seen[path] {
		f.FinalStatus = "failed"
		f.Notes = "shortcut_cycle"
		return f
	}
	seen[path] = true
	if ft.Kind == "archive" || ft.Kind == "disk_image" {
		return e.container(fileCtx, f, depth, ft.Kind == "disk_image")
	}
	triggerPath := path
	var linkArgs []string
	if ft.Kind == "shortcut" {
		target, args, err := e.Opener.ResolveShortcut(path)
		if err != nil {
			f.FinalStatus = "failed"
			f.Notes = "shortcut: " + err.Error()
			return f
		}
		target, err = filepath.Abs(target)
		if err != nil || target == path || seen[target] {
			f.FinalStatus = "failed"
			f.Notes = "shortcut_cycle or invalid target"
			return f
		}
		f.PlatformMeta["shortcut_target"] = target
		f.PlatformMeta["shortcut_arguments"] = args
		ft, err = e.Detector.Detect(target)
		if err != nil {
			f.FinalStatus = "failed"
			f.Notes = err.Error()
			return f
		}
		triggerPath = target
		linkArgs = args
		if ft.Kind == "shortcut" {
			child := e.file(fileCtx, target, reason, depth, seen)
			child.FilePath = path
			child.FileHashSHA256 = hash
			child.PlatformMeta["shortcut_target"] = target
			return child
		}
	}
	if matched, err := layout.ExecutionExclusion(triggerPath, e.Pack, e.Config); err != nil {
		f.FinalStatus, f.Notes = "failed", err.Error()
		return f
	} else if matched != "" {
		f.SelectionReason, f.Notes = "excluded_blacklist", "excluded_blacklist: "+matched
		return f
	}
	if strings.HasPrefix(ft.Family, "unsupported_") {
		f.Notes = ft.Family
		return f
	}
	if ft.Kind == "unknown" && !e.Opener.HasAssociation(triggerPath) {
		f.FinalStatus, f.Notes = "failed", "untriggerable"
		return f
	}
	e.Log.Stage(logging.LevelInfo, logging.StageVerify, "采集安装前基线快照（注册表卸载项 / 监控目录 / 文件关联），用于判定本次安装是否真的生效：%s", path)
	base, err := e.Evidence.Snapshot(fileCtx, e.Config.Verify)
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = "baseline unavailable: " + err.Error()
		return f
	}
	f.BaselineDigest = schema.String(verify.Digest(base))
	f.PreInstalled = verify.PreInstalled(triggerPath, base)
	if e.Config.Verify.WriteBaseline {
		f.PlatformMeta["baseline"] = base
	}
	for _, w := range base.Warnings {
		e.Log.Stage(logging.LevelWarn, logging.StageVerify, "基线采集告警（%s）：%s", path, w)
	}
	temp, err := e.tempDir()
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = err.Error()
		return f
	}
	var commands []ports.Command
	if ft.Kind == "document" || ft.Kind == "unknown" {
		if !e.Opener.HasAssociation(triggerPath) {
			f.Notes = "no_host_app"
			return f
		}
		cmd, err := e.Opener.OpenCommand(triggerPath)
		if err != nil {
			f.FinalStatus = "failed"
			f.Notes = err.Error()
			return f
		}
		commands = []ports.Command{cmd}
	} else {
		commands, err = e.Trigger.Commands(triggerPath, ft, temp)
		if err != nil {
			f.FinalStatus = "failed"
			f.Notes = err.Error()
			return f
		}
	}
	if len(linkArgs) > 0 {
		if ft.Kind == "script" || ft.Kind == "document" || ft.Kind == "unknown" {
			for i := range commands {
				commands[i].Args = append(commands[i].Args, linkArgs...)
				commands[i].CommandLine = ""
			}
		} else {
			commands = []ports.Command{{Program: triggerPath, Args: linkArgs, Dir: filepath.Dir(triggerPath), Method: "lnk_target"}}
		}
	}
	for i := range commands {
		ext := strings.ToLower(filepath.Ext(triggerPath))
		commands[i].NonInstall = commands[i].NonInstall || ft.Kind == "script" || commands[i].Method == "document_host" || ext == ".com" || ext == ".scr"
	}
	if e.DumpDir != "" {
		for _, cmd := range commands {
			if cmd.Method == "no_param_run" {
				commands = []ports.Command{cmd}
				break
			}
		}
	}
	methods := make([]string, 0, len(commands))
	for _, cmd := range commands {
		methods = append(methods, cmd.Method)
	}
	e.Log.Stage(logging.LevelInfo, logging.StageRun, "准备按 %d 种方式依次尝试安装：%s", len(commands), strings.Join(methods, "、"))
	if e.GUI != nil {
		e.GUI.Reset()
	}
	allEvidence := []schema.Evidence{}
	unresolvedNegatives := []schema.Evidence{}
	sideEffects := false
	everStarted, timedout := false, false
	for i, cmd := range commands {
		if fileCtx.Err() != nil {
			timedout = everStarted
			break
		}
		if i > 0 {
			select {
			case <-fileCtx.Done():
				timedout = everStarted
			case <-time.After(time.Duration(e.Config.AttemptIntervalSec) * time.Second):
			}
			if fileCtx.Err() != nil {
				break
			}
		}
		e.Log.Stage(logging.LevelInfo, logging.StageRun, "第 %d/%d 次尝试：方法=%s，命令=%s", i+1, len(commands), cmd.Method, commandText(cmd))
		a, ev, startedAttempt, timeoutAttempt, reboot, network := e.attempt(fileCtx, cmd, base, triggerPath)
		f.Attempts = append(f.Attempts, a)
		everStarted = everStarted || startedAttempt
		timedout = timedout || timeoutAttempt
		f.RequiresReboot = f.RequiresReboot || reboot
		if cmd.NonInstall {
			f.PlatformMeta["host_program"] = cmd.Program
			f.FinalStatus = "failed"
			if startedAttempt && a.ExitCode != nil && *a.ExitCode == 0 {
				f.FinalStatus = "triggered"
			}
			if timeoutAttempt {
				f.FinalStatus = "killed_on_timeout"
			}
			f.InstallConfidence = nil
			if f.FinalStatus == "triggered" || timeoutAttempt {
				break
			}
			continue
		}
		if network {
			f.PlatformMeta["requires_network"] = true
		}
		allEvidence = append(allEvidence, ev.Evidence...)
		sideEffects = sideEffects || ev.SideEffects
		// A failed parameter attempt must not poison a later successful attempt.
		current := positive(allEvidence)
		for _, item := range ev.Evidence {
			if item.Level == "N" {
				current = append(current, item)
			}
		}
		d := verify.Decide(e.Config.Verify.JudgmentProfile, current, sideEffects, f.PreInstalled, everStarted, timedout, ev.Artifact)
		for _, item := range ev.Evidence {
			if item.Level == "N" {
				unresolvedNegatives = append(unresolvedNegatives, item)
			}
		}
		strong := false
		for _, item := range ev.Evidence {
			strong = strong || item.Level == "S"
		}
		resolved := d.Status == "install_completed" && strong && a.Result == "exited" && a.ExitCode != nil && verify.MSICodeIsSuccess(*a.ExitCode) && !timeoutAttempt
		if resolved {
			unresolvedNegatives = nil
		}
		if len(unresolvedNegatives) > 0 {
			d = verify.Decide(e.Config.Verify.JudgmentProfile, append(positive(allEvidence), unresolvedNegatives...), sideEffects, f.PreInstalled, everStarted, timedout, ev.Artifact)
		}
		f.FinalStatus = d.Status
		f.InstallConfidence = d.Confidence
		f.EvidenceConflicts = d.Conflicts
		confidence := "未知"
		if d.Confidence != nil {
			confidence = *d.Confidence
		}
		e.Log.Stage(logging.LevelInfo, logging.StageVerify, "第 %d/%d 次尝试判定：%s（置信度 %s）", i+1, len(commands), schema.StatusText(d.Status), confidence)
		for _, c := range d.Conflicts {
			e.Log.Stage(logging.LevelWarn, logging.StageVerify, "证据冲突（%s）：%s", c.ID, verify.ConflictText(c))
		}
		if d.Status == "install_completed" || e.DumpDir != "" {
			break
		}
	}
	if len(f.Attempts) == 0 {
		f.FinalStatus = "failed"
		f.Notes = "no attempts completed before file deadline"
		e.Log.Stage(logging.LevelWarn, logging.StageRun, "未能在单文件时限内完成任何一次尝试（timeout_sec=%d）：%s", e.Config.TimeoutSec, path)
	}
	if timedout && f.FinalStatus == "install_completed" {
		f.Notes = "terminated on timeout; installation evidence still collected"
		e.Log.Stage(logging.LevelWarn, logging.StageRun, "安装进程因超时被终止，但仍采集到安装证据：%s", path)
	}
	// 临时目录随任务结束被删；先把手头这份"失败排查依据"搬出去。
	e.keepFailureLogs(path, f)
	return f
}

func positive(ev []schema.Evidence) []schema.Evidence {
	out := []schema.Evidence{}
	for _, e := range ev {
		if e.Level != "N" {
			out = append(out, e)
		}
	}
	return out
}
func commandText(c ports.Command) string {
	args := []string{fmt.Sprintf("%q", c.Program)}
	for _, arg := range c.Args {
		args = append(args, fmt.Sprintf("%q", arg))
	}
	return strings.Join(args, " ")
}

// noteSuffix 把备注拼成日志尾巴，便于一眼看出"为什么是这个结论"。
func noteSuffix(note string) string {
	if strings.TrimSpace(note) == "" {
		return ""
	}
	return "（" + note + "）"
}

// skipText 把跳过原因翻译成中文说明（结果 JSON 里仍保留英文原值）。
func skipText(skip string) string {
	switch skip {
	case "skipped_after_success":
		return "本包已安装成功，后续候选不再执行"
	case "task_timeout":
		return "任务总时长已到上限"
	case "interrupted":
		return "已收到中止请求"
	case "phase1_entry_limit":
		return "超过阶段 1 入口数量上限且未启用兜底枚举"
	case "fallback_disabled":
		return "未启用兜底枚举（package_layout.fallback_to_brute=false）"
	default:
		return skip
	}
}

// humanSize 把字节数格式化成人可读的大小（如 1.2 MB）。
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTP"[exp])
}
func (e *Engine) tempDir() (string, error) {
	root := config.ExpandEnv(e.Config.Archive.TempRoot)
	if root != "" {
		if err := os.MkdirAll(root, 0755); err != nil {
			return "", err
		}
	}
	dir, err := os.MkdirTemp(root, "AT-")
	if err != nil && root == "" {
		e.Log.Stage(logging.LevelWarn, logging.StageCleanup, "系统临时目录不可用，回退到 exe 同目录建临时工作区")
		exe, _ := os.Executable()
		dir, err = os.MkdirTemp(filepath.Dir(exe), "AT-")
	}
	if err == nil {
		e.temps = append(e.temps, dir)
	}
	return dir, err
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	return r.r.Read(p)
}
func hashFile(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, contextReader{ctx: ctx, r: file}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
