package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/resources"
	"autotrigger/pkg/schema"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type pageID int

const pageNone pageID = -1

const (
	pageHome pageID = iota
	pageTask
	pageTemplates
	pageLogs
	pageConfig
	pageSettings
	pageTaskDetail
	pageHelp
	pageNewTask
)

type navItem struct {
	ID    pageID
	Icon  string
	Label string
}

var uiNavItems = []navItem{
	{ID: pageHome, Icon: "⌂", Label: "首页"},
	{ID: pageTask, Icon: "⚙", Label: "任务配置"},
	{ID: pageTemplates, Icon: "▣", Label: "模板管理"},
	{ID: pageLogs, Icon: "≡", Label: "日志查看"},
	{ID: pageSettings, Icon: "◎", Label: "设置"},
	{ID: pageHelp, Icon: "?", Label: "帮助与关于"},
}

func pageTitle(id pageID) string {
	if id == pageNewTask {
		return "新建任务"
	}
	if id == pageTaskDetail {
		return "任务详情"
	}
	for _, item := range uiNavItems {
		if item.ID == id {
			return item.Label
		}
	}
	return "未知页面"
}

func pageSubtitle(id pageID) string {
	switch id {
	case pageHome:
		return "真实任务概览、环境提示与快捷入口"
	case pageTask:
		return ""
	case pageTemplates:
		return "内置资源包与外部模板的查看入口"
	case pageLogs:
		return "任务日志、结果文件与诊断材料入口"
	case pageConfig:
		return "加载、编辑、校验和导出配置"
	case pageSettings:
		return "工作目录、输出目录与前置依赖检查"
	case pageTaskDetail:
		return "任务状态、证据、置信度和结果文件"
	case pageHelp:
		return "使用说明、版本、依赖与兼容性信息"
	default:
		return ""
	}
}

type statCard struct {
	Label  string
	Value  string
	Detail string
	Kind   string
}

type templateInfo struct {
	ResourceVersion string
	Mode            string
	TemplateDir     string
	Masters         int
	Builtin         int
	External        int
	Warnings        []string
	Error           string
}

type infoCard struct {
	Title string
	Lines []string
	Kind  string
}

func filterLogLines(text, level string) string {
	level = strings.ToUpper(level)
	tags := map[string]string{"DEBUG": "调试", "INFO": "信息", "WARN": "警告", "ERROR": "错误"}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToUpper(line), "["+level+"]") || (tags[level] != "" && strings.Contains(line, "["+tags[level]+"]")) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

type pageAction struct {
	ID    string
	Label string
	Kind  string
}

type resultRow struct {
	File       string
	Path       string
	Type       string
	Status     string
	Confidence string
	Method     string
	Evidence   string
	Notes      string
}

type resultSnapshot struct {
	TaskID     string
	StartedAt  time.Time
	FinishedAt *time.Time
	Summary    ResultSummary
	Rows       []resultRow
	Error      string
	Path       string
}

func snapshotOutcome(snap resultSnapshot) string {
	if snap.Error != "" {
		return schema.OutcomeFailed
	}
	return schema.OverallOutcome(schema.Summary{Total: snap.Summary.Total, ByStatus: snap.Summary.ByStatus})
}

func headerActions(id pageID) []pageAction {
	switch id {
	case pageHome:
		return []pageAction{{ID: "new-task", Label: "+ 新建任务", Kind: "primary"}, {ID: "check-prereq", Label: "检查环境", Kind: "normal"}}
	case pageTask:
		return []pageAction{{ID: "new-task", Label: "+ 新建任务", Kind: "primary"}, {ID: "import-history", Label: "导入结果", Kind: "normal"}}
	case pageNewTask:
		return []pageAction{{ID: "back-tasks", Label: "返回列表", Kind: "normal"}}
	case pageTemplates:
		return []pageAction{{ID: "refresh-templates", Label: "刷新", Kind: "normal"}, {ID: "open-templates", Label: "打开模板目录", Kind: "normal"}}
	case pageLogs:
		return nil
	case pageConfig:
		return []pageAction{{ID: "save-config", Label: "保存", Kind: "primary"}, {ID: "load-config", Label: "加载", Kind: "normal"}, {ID: "validate-config", Label: "校验", Kind: "normal"}}
	case pageSettings:
		return nil
	case pageTaskDetail:
		return []pageAction{{ID: "back-tasks", Label: "返回列表", Kind: "normal"}}
	default:
		return nil
	}
}

func homeStats(summary *ResultSummary, running bool) []statCard {
	source := "暂无真实任务数据"
	total, success, failed, runningCount := 0, 0, 0, 0
	if summary != nil {
		source = "来自最近结果 JSON"
		total = summary.Total
		success = summary.ByStatus["installed"] + summary.ByStatus["already_installed"]
		failed = summary.ByStatus["failed"] + summary.ByStatus["timeout"]
	}
	if running {
		source = "当前任务运行中"
		runningCount = 1
	}
	return []statCard{
		{Label: "任务总数", Value: strconv.Itoa(total), Detail: source, Kind: "info"},
		{Label: "成功", Value: strconv.Itoa(success), Detail: "安装完成或已安装", Kind: "ok"},
		{Label: "失败", Value: strconv.Itoa(failed), Detail: "执行失败或超时", Kind: "fail"},
		{Label: "运行中", Value: strconv.Itoa(runningCount), Detail: "界面后台执行状态", Kind: "run"},
	}
}

func readResultSnapshot(path string) (resultSnapshot, bool) {
	p := strings.TrimSpace(path)
	if p == "" || p == "-" {
		return resultSnapshot{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return resultSnapshot{}, false
	}
	var doc schema.Result
	if err := json.Unmarshal(data, &doc); err != nil {
		return resultSnapshot{}, false
	}
	snap := resultSnapshot{
		TaskID:     doc.TaskID,
		StartedAt:  doc.StartedAt,
		FinishedAt: doc.FinishedAt,
		Summary:    ResultSummary{Total: doc.Summary.Total, ByStatus: doc.Summary.ByStatus, Path: p},
		Path:       p,
	}
	if snap.Summary.ByStatus == nil {
		snap.Summary.ByStatus = map[string]int{}
	}
	if doc.Error != nil {
		snap.Error = doc.Error.Message
	}
	for _, r := range doc.Results {
		conf := ""
		if r.InstallConfidence != nil {
			conf = *r.InstallConfidence
		}
		method, evidence := resultAttemptSummary(r.Attempts)
		snap.Rows = append(snap.Rows, resultRow{
			File:       filepath.Base(r.FilePath),
			Path:       r.FilePath,
			Type:       valueOr(r.DetectedType, "-"),
			Status:     schema.StatusText(r.FinalStatus),
			Confidence: valueOr(conf, "-"),
			Method:     method,
			Evidence:   evidence,
			Notes:      r.Notes,
		})
	}
	return snap, true
}

func resultAttemptSummary(attempts []schema.Attempt) (method, evidence string) {
	if len(attempts) == 0 {
		return "-", "-"
	}
	last := attempts[len(attempts)-1]
	method = valueOr(last.Method, "-")
	var parts []string
	for _, e := range last.InstallEvidence {
		if e.ID == "" && e.Detail == "" {
			continue
		}
		part := valueOr(e.ID, e.Type)
		if e.Detail != "" {
			part += ":" + e.Detail
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return method, "-"
	}
	return method, strings.Join(parts, "；")
}

func resultSnapshotReport(snap resultSnapshot) string {
	var b strings.Builder
	b.WriteString("任务 ID：" + valueOr(snap.TaskID, "-") + "\r\n")
	if !snap.StartedAt.IsZero() {
		b.WriteString("开始时间：" + snap.StartedAt.Format(time.RFC3339) + "\r\n")
	}
	if snap.FinishedAt != nil {
		b.WriteString("结束时间：" + snap.FinishedAt.Format(time.RFC3339) + "\r\n")
	}
	b.WriteString("总体结论：" + schema.OutcomeText(snapshotOutcome(snap)) + "\r\n")
	b.WriteString("样本统计：" + schema.SummaryLine(snap.Summary.ByStatus, snap.Summary.Total) + "\r\n")
	if snap.Error != "" {
		b.WriteString("错误信息：" + snap.Error + "\r\n")
	}
	b.WriteString("结果文件：" + snap.Path + "\r\n")
	b.WriteString("\r\n文件明细\r\n")
	if len(snap.Rows) == 0 {
		b.WriteString("  结果 JSON 中暂无逐文件记录。\r\n")
		return b.String()
	}
	for i, row := range snap.Rows {
		b.WriteString(strconv.Itoa(i+1) + ". " + valueOr(row.File, "-") + "\r\n")
		b.WriteString("   路径：" + valueOr(row.Path, "-") + "\r\n")
		b.WriteString("   类型：" + valueOr(row.Type, "-") + "    状态：" + valueOr(row.Status, "-") + "    置信度：" + valueOr(row.Confidence, "-") + "\r\n")
		b.WriteString("   方法：" + valueOr(row.Method, "-") + "\r\n")
		b.WriteString("   证据：" + valueOr(row.Evidence, "-") + "\r\n")
		if row.Notes != "" {
			b.WriteString("   备注：" + row.Notes + "\r\n")
		}
	}
	return b.String()
}

func loadTemplateInfo(exeDir string) templateInfo {
	return loadTemplateInfoWithConfig(exeDir, "")
}

func loadTemplateInfoWithConfig(exeDir, path string) templateInfo {
	cfg, warnings, err := config.Load(path, exeDir)
	if err != nil {
		return templateInfo{Error: err.Error()}
	}
	dir := cfg.GUI.TemplateDir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(exeDir, dir)
	}
	info := templateInfo{ResourceVersion: resources.Version, Mode: cfg.GUI.TemplatePack, TemplateDir: dir, Warnings: warnings}
	lib, err := templates.Load(cfg.GUI, exeDir)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Masters = lib.Masters
	info.Builtin = lib.BuiltinCount
	info.External = lib.ExternalCount
	info.Warnings = append(info.Warnings, lib.Warnings...)
	return info
}

func pageCards(id pageID, opts Options, exeDir string, tmpl templateInfo, summary *ResultSummary, running bool) []infoCard {
	resultPath := strings.TrimSpace(opts.Output)
	if resultPath == "" {
		resultPath = filepath.Join(exeDir, DefaultResultName)
	}
	configPath := strings.TrimSpace(opts.Config)
	configMode := "未显式指定；运行时按同目录 config.json，再回落到内嵌默认值"
	if configPath != "" {
		configMode = configPath
	}
	switch id {
	case pageTemplates:
		lines := []string{
			"资源包版本：" + valueOr(tmpl.ResourceVersion, "unknown"),
			"模板合并模式：" + valueOr(tmpl.Mode, "merged"),
			"外部目录：" + valueOr(tmpl.TemplateDir, "-"),
			"真实母版：" + strconv.Itoa(tmpl.Masters),
			"内置变体：" + strconv.Itoa(tmpl.Builtin),
			"外部变体：" + strconv.Itoa(tmpl.External),
		}
		if tmpl.Error != "" {
			lines = append(lines, "加载错误："+tmpl.Error)
		}
		lines = append(lines, tmpl.Warnings...)
		return []infoCard{
			{Title: "模板资源状态", Lines: lines, Kind: templateKind(tmpl)},
			{Title: "使用说明", Lines: []string{"当前页只展示真实资源包状态。模板选择和管理操作将在接入外部模板目录后开放。"}, Kind: "info"},
		}
	case pageLogs:
		lines := []string{"结果 JSON：" + resultPath, "完整日志：" + resultPath + ".log", "MSI 失败日志目录：" + strings.TrimSuffix(resultPath, filepath.Ext(resultPath)) + "-msi-logs"}
		if summary != nil {
			lines = append(lines, schema.SummaryLine(summary.ByStatus, summary.Total))
		} else {
			lines = append(lines, "尚未读取到结果 JSON；运行任务后会显示最近 summary。")
		}
		return []infoCard{
			{Title: "日志与结果文件", Lines: lines, Kind: "info"},
			{Title: "打开方式", Lines: []string{"任务配置页的“打开结果目录”会打开当前结果文件所在目录。"}, Kind: "run"},
		}
	case pageConfig:
		return []infoCard{
			{Title: "当前配置来源", Lines: []string{configMode}, Kind: "info"},
			{Title: "可用操作", Lines: []string{"任务配置页提供“生成默认”“编辑”“校验配置”。生成默认配置不会在未确认时覆盖已有文件。"}, Kind: "ok"},
			{Title: "配置约束", Lines: []string{"结构、字段、默认值和深合并规则以 PRD 与 internal/config 为准。"}, Kind: "warn"},
		}
	case pageSettings:
		lockText := "关闭"
		if opts.Lock {
			lockText = "开启"
		}
		return []infoCard{
			{Title: "基础设置", Lines: []string{"工作目录：" + exeDir, "结果路径：" + resultPath, "超时：" + strconv.Itoa(opts.TimeoutSec) + " 秒", "日志级别：" + opts.LogLevel}, Kind: "info"},
			{Title: "执行开关", Lines: []string{"递归：" + yesNo(opts.Recursive), "跳过压缩包：" + yesNo(opts.NoArchive), "禁用 OCR：" + yesNo(opts.NoOCR), "单实例锁：" + lockText}, Kind: "run"},
			{Title: "前置依赖", Lines: []string{"点首页或任务配置页的“检查前置”获取 7-Zip / OCR 的真实探测结果。"}, Kind: "warn"},
		}
	case pageTaskDetail:
		if running {
			return []infoCard{{Title: "当前任务", Lines: []string{"任务正在运行；完成后会从结果 JSON 读取状态、统计和证据摘要。"}, Kind: "run"}}
		}
		if summary != nil {
			return []infoCard{
				{Title: "最近任务摘要", Lines: []string{schema.OutcomeText(schema.OverallOutcome(schema.Summary{Total: summary.Total, ByStatus: summary.ByStatus})), schema.SummaryLine(summary.ByStatus, summary.Total), "结果文件：" + resultPath}, Kind: "ok"},
				{Title: "证据详情", Lines: []string{"本阶段先展示 summary；逐样本阶段、证据和置信度将在接入任务详情页控件后展示。"}, Kind: "info"},
			}
		}
		return []infoCard{{Title: "暂无任务结果", Lines: []string{"运行任务后，这里会展示最近结果 JSON 中的任务状态与统计。"}, Kind: "info"}}
	case pageHelp:
		return []infoCard{
			{Title: "版本", Lines: []string{"工具版本：" + cli.Version, "资源包版本：" + resources.Version, "结果 Schema：" + schema.Version}, Kind: "info"},
			{Title: "入口", Lines: []string{"双击 exe 或 --gui 打开界面；脚本和流水线请继续使用 CLI 参数。"}, Kind: "run"},
			{Title: "兼容性", Lines: []string{"目标平台：Windows 7 SP1 x64 及以上；GUI 使用原生 Win32 控件和局部自绘。"}, Kind: "ok"},
		}
	default:
		return []infoCard{{Title: pageTitle(id), Lines: []string{"本页将在对应业务数据接入后开放。"}, Kind: "info"}}
	}
}

func valueOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func yesNo(v bool) string {
	if v {
		return "是"
	}
	return "否"
}

func templateKind(t templateInfo) string {
	switch {
	case t.Error != "":
		return "fail"
	case t.Builtin+t.External == 0:
		return "warn"
	default:
		return "ok"
	}
}
