package ui

import (
	"autotrigger/pkg/schema"
	"fmt"
	"strings"
)

func schemaOutcomeText(snap resultSnapshot) string {
	if taskCancelledError(snap.Error) {
		return "任务已取消"
	}
	return schema.OutcomeText(snapshotOutcome(snap))
}

func taskCancelledError(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	return text == "context canceled" || text == "content canceled" || strings.HasSuffix(text, ": context canceled")
}

func taskFailureMessage(message string) string {
	if taskCancelledError(message) {
		return "任务已取消，已产生的安装变更不会自动回滚。"
	}
	return "任务失败：" + message
}

func fileTypeText(kind string) string {
	labels := map[string]string{"exe": "可执行程序", "pe_executable": "可执行程序", "pe_dll": "动态链接库", "dll": "动态链接库", "msi": "Windows 安装包", "script": "脚本", "archive": "压缩包", "disk_image": "光盘映像", "shortcut": "快捷方式", "document": "文档", "directory": "文件夹", "unknown": "未知文件"}
	return valueOr(labels[kind], valueOr(kind, "未提供"))
}

func methodText(method string) string {
	labels := map[string]string{"silent_install": "静默安装", "no_param_run": "直接运行", "msi": "Windows 安装包执行", "script": "脚本执行", "dll_probe": "动态库入口探测", "lnk_target": "快捷方式目标", "document_host": "文档关联程序", "shell_open": "系统关联打开", "-": "等待实际执行记录"}
	return valueOr(labels[method], valueOr(method, "等待实际执行记录"))
}

func detailCards(snap resultSnapshot, record taskRecord) []infoCard {
	started, finished, duration, exit := "未提供", "未提供", "未提供", "未提供"
	if !snap.StartedAt.IsZero() {
		started = snap.StartedAt.Local().Format("2006-01-02 15:04:05")
	}
	if snap.FinishedAt != nil {
		finished = snap.FinishedAt.Local().Format("2006-01-02 15:04:05")
		if !snap.StartedAt.IsZero() && !snap.FinishedAt.Before(snap.StartedAt) {
			duration = fmt.Sprintf("%.1f 秒", snap.FinishedAt.Sub(snap.StartedAt).Seconds())
		}
	}
	if record.ExitCode != nil {
		exit = fmt.Sprint(*record.ExitCode)
	}
	input, fileType, size, method := valueOr(record.Input, "未提供"), "未提供", "未提供", "未提供"
	if record.FileType != "" {
		fileType = fileTypeText(record.FileType)
	}
	if record.FileSize != nil {
		size = fmt.Sprintf("%d 字节（%.2f MB）", *record.FileSize, float64(*record.FileSize)/(1024*1024))
	}
	if record.FileType == "directory" {
		size = "不适用（文件夹）"
	}
	if snap.PresentationState != "" {
		method = "等待实际执行记录"
	}
	confidence, status, evidence := "未提供", schemaOutcomeText(snap), "未提供"
	if len(snap.Rows) == 1 {
		row := snap.Rows[0]
		if record.Input == "" {
			input = row.Path
		}
		fileType, method = fileTypeText(row.Type), methodText(row.Method)
		if row.SizeBytes > 0 {
			size = fmt.Sprintf("%.2f MB", float64(row.SizeBytes)/(1024*1024))
		}
		if snap.Error == "" {
			status = row.Status
		}
		switch row.Confidence {
		case "high":
			confidence = "高"
		case "medium":
			confidence = "中"
		case "low":
			confidence = "低"
		}
		if row.Evidence != "-" && row.Evidence != "" {
			evidence = row.Evidence
		}
	} else if len(snap.Rows) > 1 {
		fileType = fmt.Sprintf("%d 个文件", len(snap.Rows))
		confidence, evidence, method = "按文件查看", "见下方详细记录", "按文件查看"
	}
	if snap.PresentationState != "" {
		status = snap.PresentationState
	}
	evidenceLines := []string{"置信度：" + confidence, "最终状态：" + status}
	for i, part := range strings.Split(evidence, "；") {
		prefix := ""
		if i == 0 {
			prefix = "证据："
		}
		evidenceLines = append(evidenceLines, prefix+strings.TrimSpace(part))
	}
	return []infoCard{
		{Title: "文件信息", Kind: "info", Lines: []string{"路径：" + input, "类型：" + fileType, "大小：" + size, "触发方式：" + method}},
		{Title: "执行信息", Kind: "info", Lines: []string{"开始：" + started, "结束：" + finished, "执行时长：" + duration, "任务退出码：" + exit}},
		{Title: "结果可信度", Kind: "info", Lines: evidenceLines},
	}
}
