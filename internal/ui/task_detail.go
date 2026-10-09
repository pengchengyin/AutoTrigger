package ui

import "fmt"

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
	confidence, status, evidence := "未提供", schemaOutcomeText(snap), "未提供"
	if len(snap.Rows) == 1 {
		row := snap.Rows[0]
		if record.Input == "" {
			input = row.Path
		}
		fileType, method = row.Type, row.Method
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
	return []infoCard{
		{Title: "文件信息", Kind: "info", Lines: []string{"路径：" + input, "类型：" + fileType, "大小：" + size, "触发方式：" + method}},
		{Title: "执行信息", Kind: "info", Lines: []string{"开始：" + started, "结束：" + finished, "执行时长：" + duration, "任务退出码：" + exit}},
		{Title: "结果可信度", Kind: "info", Lines: []string{"置信度：" + confidence, "最终状态：" + status, "证据：" + evidence}},
	}
}
