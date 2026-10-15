// Package cli implements the stable command-line integration surface.
package cli

import (
	"autotrigger/internal/archive"
	"autotrigger/internal/config"
	"autotrigger/internal/core"
	"autotrigger/internal/core/detect"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/datadir"
	"autotrigger/internal/gui"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/logging"
	"autotrigger/internal/output"
	"autotrigger/internal/resources"
	"autotrigger/pkg/schema"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var Version = "0.1.0"

type Dependencies struct {
	HistoryManaged bool
	Runner         ports.ProcessRunner
	Trigger        func(config.Config, resources.Pack) ports.InstallerTrigger
	Opener         ports.FileOpener
	Mounter        ports.DiskMounter
	Evidence       ports.EvidenceSource
	NativeGUI      ports.GUIAutomator
	Lock           ports.Lock
	Prereqs        ports.PrereqManager
	OCR            func() ports.OCREngine
}
type Options struct {
	Input              string
	Output             string
	Config             string
	LogFile            string
	LogLevel           string
	Recursive          bool
	Timeout            int
	NoArchive          bool
	NoOCR              bool
	RequireOCR         bool
	CheckPrereq        bool
	NoDeployDeps       bool
	DumpConfig         bool
	Capabilities       bool
	Version            bool
	JSON               bool
	Pretty             bool
	Lock               bool
	NoLock             bool
	DumpUI             bool
	Out                string
	DumpResources      string
	NoBuiltinTemplates bool
	SelfTest           string
	GUI                bool
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, deps Dependencies) (code int) {
	if len(args) > 0 && args[0] == "task" {
		return runTaskCommand(ctx, args[1:], stdout, stderr)
	}
	// 帮助优先于一切初始化：在此处提前返回，可避免「无参数运行」时先在当前
	// 目录留下结果 JSON 与日志文件（用户此时尚未开始任何任务）。
	if len(args) == 0 {
		printHelp(stderr)
		return 2
	}
	if helpRequested(args) {
		printHelp(stdout)
		return 0
	}
	opts := Options{}
	informationMode := informationRequest(args)
	fs := flag.NewFlagSet("AutoTrigger", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// 参数出错时只给一行摘要并指向 --help，不刷整屏帮助（保持错误信息可读）。
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "用法: AutoTrigger --input <路径> [选项]\n运行 AutoTrigger --help 查看完整帮助。\n")
	}
	fs.StringVar(&opts.Input, "input", "", "sample file or directory")
	fs.StringVar(&opts.Output, "output", "", "result path, or - for JSON stdout")
	fs.StringVar(&opts.Config, "config", "", "configuration file")
	fs.StringVar(&opts.LogFile, "log-file", "", "log path")
	fs.StringVar(&opts.LogLevel, "log-level", "", "debug|info|warn|error")
	fs.BoolVar(&opts.Recursive, "recursive", false, "scan subdirectories")
	fs.IntVar(&opts.Timeout, "timeout", 600, "per-file total deadline; 0 is unlimited")
	fs.BoolVar(&opts.NoArchive, "no-archive", false, "skip archive processing")
	fs.BoolVar(&opts.NoOCR, "no-ocr", false, "disable OCR")
	fs.BoolVar(&opts.RequireOCR, "require-ocr", false, "fail if OCR is unavailable")
	fs.BoolVar(&opts.CheckPrereq, "check-prereq", false, "check offline prerequisites")
	fs.BoolVar(&opts.NoDeployDeps, "no-deploy-deps", false, "check prerequisites without installing offline dependencies")
	fs.BoolVar(&opts.DumpConfig, "dump-config", false, "emit effective configuration JSON")
	fs.BoolVar(&opts.Capabilities, "capabilities", false, "emit capability JSON")
	fs.BoolVar(&opts.Version, "version", false, "emit version JSON")
	fs.BoolVar(&opts.JSON, "json", false, "emit machine-readable report")
	fs.BoolVar(&opts.Pretty, "pretty", false, "indent JSON output")
	fs.BoolVar(&opts.Lock, "lock", true, "machine-wide installation lock")
	fs.BoolVar(&opts.NoLock, "no-lock", false, "disable installation lock")
	fs.BoolVar(&opts.DumpUI, "dump-ui", false, "capture sample windows without clicking")
	fs.StringVar(&opts.Out, "out", "", "UI capture directory")
	fs.StringVar(&opts.DumpResources, "dump-resources", "", "export builtin rules and templates")
	fs.BoolVar(&opts.NoBuiltinTemplates, "no-builtin-templates", false, "disable builtin image templates")
	fs.StringVar(&opts.SelfTest, "self-test", "", "templates: validate source screenshots")
	fs.BoolVar(&opts.GUI, "gui", false, "open the graphical interface (Windows only)")
	cfg := config.Defaults()
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	id := taskID()
	result := schema.NewResult(id, runtime.GOOS)
	result.Meta["tool_version"] = Version
	result.Meta["resource_pack_version"] = resources.Version
	result.ConfigDigest = cfg.Digest()
	resultPath := "autotrigger-result.json"
	if p := argumentValue(args, "output"); p != "" {
		resultPath = p
	}
	var logger *logging.Logger
	var historyFinish func(int)
	finish := func(exit int, err error) int {
		if rawInput := argumentValue(args, "input"); rawInput != "" {
			sample, _ := filepath.Abs(rawInput)
			target, _ := filepath.Abs(resultPath)
			if strings.EqualFold(sample, target) {
				resultPath = "-"
			}
		}
		if err != nil {
			result.Error = &schema.Error{Code: exit, Message: err.Error()}
			// 日志已建立时走统一日志出口：带时间戳与中文级别，**且会写进日志文件**，
			// 保证"失败原因"在日志里查得到；早期错误（如参数解析失败）仍直接写 stderr。
			if logger != nil {
				logger.Stage(logging.LevelError, logging.StageResult, "工具错误：%v", err)
			} else {
				fmt.Fprintf(stderr, "[error] %v\n", err)
			}
		}
		now := time.Now()
		if informationMode && argumentValue(args, "output") == "" {
			return exit
		}
		result.FinishedAt = &now
		result.ConfigDigest = cfg.Digest()
		if writeErr := output.Write(resultPath, &result, stdout, cfg.Output.Pretty, int64(cfg.Output.MaxFileSizeMB)*1024*1024); writeErr != nil {
			fmt.Fprintf(stderr, "[error] writing result: %v\n", writeErr)
			if historyFinish != nil {
				historyFinish(1)
			}
			return 1
		}
		if historyFinish != nil {
			historyFinish(exit)
		}
		return exit
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			code = finish(1, fmt.Errorf("unexpected runtime exception: %v", recovered))
		}
	}()
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printHelp(stdout)
			return 0
		}
		return finish(2, err)
	}
	if fs.NArg() != 0 {
		return finish(2, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")))
	}
	if opts.GUI {
		// 图形界面由启动器（launchUI）负责打开。能走到这里只有两种可能：
		// 与其它参数混用，或当前平台不提供界面（4.8 / F20）。给出一句话说明，
		// 而不是静默忽略 —— 静默忽略会让调用方以为界面已打开。
		return finish(2, fmt.Errorf("--gui must be used alone (equivalent to double-clicking AutoTrigger.exe), and the graphical interface is Windows-only"))
	}
	var warnings []string
	var err error
	layout, err := datadir.Load(exeDir)
	if err != nil {
		return finish(3, err)
	}
	configPath := opts.Config
	if configPath == "" {
		if info, e := os.Stat(layout.Config()); e == nil && !info.IsDir() {
			configPath = layout.Config()
		}
	}
	cfg, warnings, err = config.Load(configPath, filepath.Dir(layout.Config()))
	if err != nil {
		return finish(3, err)
	}
	if checker, ok := deps.Prereqs.(interface{ SetDataRoot(string) }); ok {
		checker.SetDataRoot(layout.Root)
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if !visited["output"] && cfg.Output.Path == config.Defaults().Output.Path {
		cfg.Output.Path = filepath.Join(layout.Results(), "autotrigger-result.json")
	}
	if !visited["output"] && !visited["log-file"] && cfg.Output.LogFile == "" && cfg.Output.Path != "-" {
		cfg.Output.LogFile = filepath.Join(layout.Logs(), filepath.Base(cfg.Output.Path)+".log")
	}
	if visited["timeout"] {
		cfg.TimeoutSec = opts.Timeout
	}
	if visited["output"] {
		cfg.Output.Path = opts.Output
	}
	if visited["log-file"] {
		cfg.Output.LogFile = opts.LogFile
	}
	if visited["log-level"] {
		cfg.Output.LogLevel = opts.LogLevel
	}
	if visited["pretty"] {
		cfg.Output.Pretty = opts.Pretty
	}
	if visited["lock"] {
		cfg.Lock.Enabled = opts.Lock
	}
	if opts.NoLock {
		cfg.Lock.Enabled = false
	}
	if opts.NoArchive {
		cfg.Archive.RequireSevenZip = false
	}
	if opts.NoOCR {
		cfg.OCR.Enabled = false
		cfg.OCR.RequireOCR = false
	}
	if opts.RequireOCR {
		cfg.OCR.RequireOCR = true
	}
	if opts.NoBuiltinTemplates {
		cfg.GUI.BuiltinTemplates = false
	}
	if cfg.Output.Path != "" {
		resultPath = cfg.Output.Path
	}
	if err = cfg.Validate(); err != nil {
		return finish(3, err)
	}
	if opts.Input != "" {
		opts.Input, err = filepath.Abs(opts.Input)
		if err != nil {
			return finish(2, err)
		}
	}
	if cfg.Output.Path == "" && opts.Input != "" {
		if stat, e := os.Stat(opts.Input); e == nil && stat.IsDir() {
			cwd, _ := os.Getwd()
			rel, e := filepath.Rel(opts.Input, cwd)
			if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				resultPath = filepath.Join(os.TempDir(), "AT-results", id+".json")
			}
		}
	}
	if resultPath != "-" {
		resultPath, err = filepath.Abs(resultPath)
		if err != nil {
			return finish(2, err)
		}
	}
	if opts.DumpConfig {
		return writeInfo(stdout, cfg, opts.Pretty, stderr)
	}
	if opts.Version {
		return writeInfo(stdout, map[string]string{"tool_version": Version, "schema_version": schema.Version, "resource_pack_version": resources.Version, "go_version": runtime.Version()}, opts.Pretty, stderr)
	}
	if opts.DumpResources != "" {
		if err = resources.Export(opts.DumpResources); err != nil {
			return finish(1, err)
		}
		return writeInfo(stdout, map[string]interface{}{"resource_pack_version": resources.Version, "exported_to": opts.DumpResources}, opts.Pretty, stderr)
	}
	pack := resources.Pack{Mode: cfg.GUI.TemplatePack, Root: layout.Root}
	library, err := templates.LoadAt(cfg.GUI, layout.Root, layout.Templates())
	if err != nil {
		return finish(3, err)
	}
	if opts.SelfTest != "" {
		if opts.SelfTest != "templates" {
			return finish(2, fmt.Errorf("unknown self-test: %s", opts.SelfTest))
		}
		failures, err := library.SelfTest()
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
		}
		status := map[string]interface{}{"ok": err == nil, "template_count": len(library.Entries), "master_count": library.Masters, "failures": failures}
		writeInfo(stdout, status, true, stderr)
		if err != nil {
			return 1
		}
		return 0
	}
	logPath := cfg.Output.LogFile
	if informationMode && !visited["log-file"] {
		logPath = ""
	}
	if logPath == "" && !informationMode {
		if resultPath == "-" {
			logPath = "autotrigger-result.json.log"
		} else {
			logPath = resultPath + ".log"
		}
	}
	logOut := stderr
	if logPath != "" {
		logPath, err = filepath.Abs(logPath)
		if err != nil {
			return finish(2, err)
		}
		if resultPath != "-" && strings.EqualFold(logPath, resultPath) {
			return finish(2, fmt.Errorf("log and result paths must differ"))
		}
		if opts.Input != "" && (strings.EqualFold(resultPath, opts.Input) || strings.EqualFold(logPath, opts.Input)) {
			return finish(2, fmt.Errorf("output paths must not overwrite the sample"))
		}
		if err = os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			return finish(1, err)
		}
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return finish(1, err)
		}
		defer logFile.Close()
		logOut = io.MultiWriter(stderr, logFile)
	}
	logger = &logging.Logger{Out: logOut, Level: cfg.Output.LogLevel}
	if !informationMode && !deps.HistoryManaged && opts.Input != "" {
		writeHistory := func(exit *int) {
			if historyErr := writeTaskHistory(layout, &result, opts.Input, configPath, logPath, exit); historyErr != nil {
				fmt.Fprintf(stderr, "[warning] 保存命令行任务历史失败：%v\n", historyErr)
			}
		}
		writeHistory(nil)
		historyFinish = func(exit int) { writeHistory(&exit) }
	}
	for _, w := range append(warnings, library.Warnings...) {
		logger.Stage(logging.LevelWarn, logging.StageTask, "配置提示：%s", w)
	}
	if !informationMode {
		logger.Stage(logging.LevelInfo, logging.StageTask, "任务开始：task_id=%s，工具版本 %s，资源包 %s", id, Version, resources.Version)
		logger.Stage(logging.LevelInfo, logging.StageTask, "结果文件：%s", resultPath)
		logger.Stage(logging.LevelInfo, logging.StageTask, "日志文件：%s", logPath)
	} else {
		logger.Stage(logging.LevelInfo, logging.StageTask, "开始环境/能力检查（不创建安装任务产物）")
	}
	if !opts.CheckPrereq && !opts.Capabilities {
		if opts.Input == "" {
			return finish(2, fmt.Errorf("--input is required"))
		}
		if _, err = os.Stat(opts.Input); err != nil {
			return finish(2, fmt.Errorf("input: %w", err))
		}
	}
	if checker, ok := deps.Prereqs.(interface {
		CheckEnvironment(context.Context, config.Config, bool) (schema.Prereq, map[string]interface{}, error)
	}); opts.CheckPrereq && ok {
		result.Prereq, result.PlatformMeta, err = checker.CheckEnvironment(ctx, cfg, !opts.NoDeployDeps && !opts.Capabilities)
	} else if checker, ok := deps.Prereqs.(interface {
		CheckOnly(context.Context, config.Config) (schema.Prereq, map[string]interface{}, error)
	}); (opts.NoDeployDeps || opts.Capabilities) && ok {
		result.Prereq, result.PlatformMeta, err = checker.CheckOnly(ctx, cfg)
	} else {
		result.Prereq, result.PlatformMeta, err = deps.Prereqs.Check(ctx, cfg)
	}
	if deps.OCR != nil {
		if engine := deps.OCR(); engine != nil {
			defer engine.Stop()
		}
	}
	if err != nil {
		return finish(1, err)
	}
	for _, issue := range result.Prereq.Issues {
		level := logging.LevelWarn
		if strings.HasPrefix(issue, "已自动部署 ") {
			level = logging.LevelInfo
		}
		logger.Stage(level, logging.StageTask, "环境提示：%s", issue)
	}
	if opts.Capabilities {
		return writeInfo(stdout, capabilities(cfg, result, library), opts.Pretty, stderr)
	}
	if opts.CheckPrereq {
		if opts.JSON {
			writeInfo(stdout, result.Prereq, opts.Pretty, stderr)
		} else {
			selfCheck := "未通过"
			if result.Prereq.OK {
				selfCheck = "通过"
			}
			logger.Stage(logging.LevelInfo, logging.StageTask, "环境自检：总体 %s（7-Zip=%s，OCR=%s）", selfCheck, result.Prereq.ArchiveBackendStatus, result.Prereq.OCREngineStatus)
		}
		if cfg.Archive.RequireSevenZip && result.Prereq.ArchiveBackend == nil {
			return 4
		}
		if cfg.OCR.RequireOCR && result.Prereq.OCREngineStatus != "ok" {
			return 5
		}
		return 0
	}
	if cfg.OCR.RequireOCR && result.Prereq.OCREngineStatus != "ok" {
		return finish(5, fmt.Errorf("required OCR engine unavailable; see data directory third_parties/ocr and README.md"))
	}
	if cfg.Lock.Enabled {
		unlock, err := deps.Lock.Acquire(ctx, time.Duration(cfg.Lock.WaitSec)*time.Second)
		if err != nil {
			if errors.Is(err, ports.ErrBusy) {
				return finish(6, err)
			}
			return finish(1, err)
		}
		defer unlock()
	}
	var extractor ports.ArchiveExtractor
	if result.Prereq.ArchiveBackend != nil {
		extractor = archive.SevenZip{Path: *result.Prereq.ArchiveBackend, Runner: deps.Runner}
	}
	var ocrEngine ports.OCREngine
	if deps.OCR != nil {
		ocrEngine = deps.OCR()
	}
	automator := &gui.Automator{Native: deps.NativeGUI, Engine: ocrEngine, Config: cfg, Pack: pack, Templates: library}
	if err = automator.Init(); err != nil {
		return finish(3, err)
	}
	defer deps.NativeGUI.Close()
	engine := core.Engine{Config: cfg, Runner: deps.Runner, Detector: detect.Detector{}, Trigger: deps.Trigger(cfg, pack), Opener: deps.Opener, Mounter: deps.Mounter, Extractor: extractor, Evidence: deps.Evidence, GUI: automator, Log: logger, Pack: pack, Result: &result, Recursive: opts.Recursive, ResultPath: resultPath, IgnorePaths: []string{resultPath, logPath, resultPath + ".tmp", resultPath + ".journal.ndjson", exe}}
	engine.DisableArchives = opts.NoArchive
	if opts.DumpUI {
		engine.DumpDir = opts.Out
		if engine.DumpDir == "" {
			engine.DumpDir = filepath.Join("dump", id)
		}
	}
	var journal *output.Journal
	if cfg.Output.Journal && resultPath != "-" {
		journal, err = output.OpenJournal(resultPath+".journal.ndjson", id)
		if err != nil {
			return finish(1, err)
		}
		engine.OnFile = journal.Append
	}
	if resultPath != "-" {
		if err = output.Write(resultPath, &result, stdout, cfg.Output.Pretty, int64(cfg.Output.MaxFileSizeMB)*1024*1024); err != nil {
			if journal != nil {
				journal.Close(false)
			}
			return finish(1, err)
		}
	}
	err = engine.Run(ctx, opts.Input)
	exit := 0
	if err != nil {
		if errors.Is(err, context.Canceled) {
			exit = 130
		} else {
			exit = 1
		}
	}
	if engine.MissingArchive && exit == 0 {
		exit = 4
		err = fmt.Errorf("7-Zip unavailable for archive input; deploy offline package or use --no-archive")
	}
	logRunSummary(logger, &result, exit, time.Since(result.StartedAt))
	exit = finish(exit, err)
	if journal != nil {
		if closeErr := journal.Close(exit == 0 && !cfg.Output.KeepJournalOnSuccess); closeErr != nil {
			logger.Stage(logging.LevelWarn, logging.StageCleanup, "journal 清理失败：%v", closeErr)
		}
	}
	return exit
}

// logRunSummary 在日志末尾输出中文汇总，让"装了几个、成没成"一眼可见。
// 与图形界面结束横幅同口径（F20.6）：总耗时、退出码、各状态计数、总体结论。
func logRunSummary(logger *logging.Logger, result *schema.Result, exit int, elapsed time.Duration) {
	s := result.Summary
	logger.Stage(logging.LevelInfo, logging.StageResult, "本次运行结束：总耗时 %.1f 秒，退出码 %d —— %s", elapsed.Seconds(), exit, ExitText(exit))
	// 结论先于计数：先给判断，再给依据，避免"安装成功 0…总体结论 安装成功"这类误读。
	logger.Stage(logging.LevelInfo, logging.StageResult, "总体结论：%s", schema.OutcomeText(schema.OverallOutcome(s)))
	logger.Stage(logging.LevelInfo, logging.StageResult, "样本统计：%s", schema.SummaryLine(s.ByStatus, s.Total))
}

func writeInfo(stdout io.Writer, v interface{}, pretty bool, stderr io.Writer) int {
	if err := output.JSON(stdout, v, pretty); err != nil {
		fmt.Fprintf(stderr, "[error] JSON output: %v\n", err)
		return 1
	}
	return 0
}
func taskID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	id[6] = id[6]&0xf | 0x40
	id[8] = id[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}
func argumentValue(args []string, key string) string {
	for i, arg := range args {
		if strings.HasPrefix(arg, "--"+key+"=") {
			return strings.TrimPrefix(arg, "--"+key+"=")
		}
		if arg == "--"+key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func informationRequest(args []string) bool {
	for _, arg := range args {
		for _, key := range []string{"capabilities", "check-prereq", "version", "dump-config", "dump-resources", "self-test"} {
			if arg == "--"+key || arg == "-"+key || strings.HasPrefix(arg, "--"+key+"=") && !strings.HasSuffix(arg, "=false") {
				return true
			}
		}
	}
	return false
}
func capabilities(c config.Config, r schema.Result, l *templates.Library) map[string]interface{} {
	result := capabilityReport(c, r, l)
	result["supported_evidence_ids"] = []string{"S1", "S2", "S3", "S4", "A1", "A2", "A3", "B1", "N1", "N2", "N3", "N4", "N5", "N6"}
	return result
}

func capabilityReport(c config.Config, r schema.Result, l *templates.Library) map[string]interface{} {
	return map[string]interface{}{"tool_version": Version, "schema_version": schema.Version, "resource_pack_version": resources.Version, "platform": runtime.GOOS, "os_version": r.PlatformMeta["os_version"], "supported_final_status": schema.Statuses, "supported_evidence_ids": []string{"S1", "S2", "S3", "S4", "A1", "A2", "A3", "B1", "N1", "N2", "N3", "N4", "N5"}, "supported_confidence": []string{"high", "medium", "low"}, "supported_methods": []string{"silent_install", "no_param_run", "script", "msi", "dll_probe", "lnk_target", "document_host"}, "capabilities": map[string]interface{}{"archive_backend": r.Prereq.ArchiveBackend != nil, "mount_backend": r.Prereq.MountBackendStatus == "ok", "ocr": r.Prereq.OCREngineStatus == "ok", "gui_automation": c.GUI.Enabled, "builtin_resources": true, "builtin_templates": l.BuiltinCount > 0, "launch_check": false, "win7_runtime_verified": false}, "resources": map[string]interface{}{"template_pack": c.GUI.TemplatePack, "builtin_template_variants": l.BuiltinCount, "external_template_entries": l.ExternalCount, "master_count": l.Masters, "template_acceptance_ready": l.Masters >= 50 && l.BuiltinCount >= 180}, "limits": map[string]int{"max_attempts_per_file": c.MaxAttemptsPerFile, "timeout_sec": c.TimeoutSec, "idle_timeout_sec": c.IdleTimeoutSec, "task_timeout_sec": c.TaskTimeoutSec}, "prereq": r.Prereq}
}
