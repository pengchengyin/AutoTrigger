package ui

import (
	"autotrigger/pkg/schema"
	"encoding/json"
	"strings"
)

type dependencyState struct{ Name, Status, Detail, Kind string }

func parsePrereqReport(body []byte) *schema.Prereq {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields["archive_backend_status"] == nil {
		return nil
	}
	var report schema.Prereq
	if json.Unmarshal(body, &report) != nil {
		return nil
	}
	return &report
}

func dependencyStates(report *schema.Prereq, checking bool) []dependencyState {
	rows := []dependencyState{{Name: "7-Zip", Status: "未检查", Kind: "info"}, {Name: "OCR", Status: "未检查", Kind: "info"}}
	if checking {
		for i := range rows {
			rows[i].Status = "检测与部署中"
		}
		return rows
	}
	if report == nil {
		return rows
	}
	for i, status := range []string{report.ArchiveBackendStatus, report.OCREngineStatus} {
		switch status {
		case "ok":
			rows[i].Status, rows[i].Kind = "已安装 · 可用", "ok"
		case "version_too_old":
			rows[i].Status, rows[i].Kind = "版本偏低", "warn"
		case "startup_failed":
			rows[i].Status, rows[i].Kind = "自检失败", "fail"
		case "disabled":
			rows[i].Status = "已关闭"
		case "missing":
			rows[i].Status, rows[i].Kind = "未安装 / 不可用", "warn"
		default:
			rows[i].Status, rows[i].Kind = "检测异常", "warn"
		}
	}
	if report.ArchiveBackend != nil {
		rows[0].Detail = *report.ArchiveBackend
	}
	if report.SevenZipVersion != nil {
		rows[0].Name += " " + *report.SevenZipVersion
	}
	if report.OCREnginePath != nil {
		rows[1].Detail = *report.OCREnginePath
	}
	if report.OCREngine != "none" && report.OCREngine != "" {
		name := map[string]string{"paddleocr_json": "PaddleOCR-json", "rapidocr_json": "RapidOCR-json", "umi_ocr": "Umi-OCR", "tesseract": "Tesseract"}[report.OCREngine]
		if name == "" {
			name = report.OCREngine
		}
		rows[1].Name = "OCR · " + name
	}
	for i, keyword := range []string{"7-Zip", "OCR"} {
		if rows[i].Detail != "" {
			continue
		}
		for _, issue := range report.Issues {
			if strings.Contains(issue, keyword) && !strings.HasPrefix(issue, "已自动部署 ") {
				rows[i].Detail = issue
				break
			}
		}
	}
	return rows
}

func dependencySummary(report *schema.Prereq, checking bool) string {
	rows := dependencyStates(report, checking)
	return "7-Zip：" + rows[0].Status + "；OCR：" + rows[1].Status
}

func dependencyIssues(report *schema.Prereq) string {
	if report == nil {
		return ""
	}
	issues := []string{}
	for _, issue := range report.Issues {
		if !strings.HasPrefix(issue, "已自动部署 ") {
			issues = append(issues, issue)
		}
	}
	return strings.Join(issues, "；")
}

func dependencyNotice(report *schema.Prereq) string {
	notice := dependencyIssues(report)
	if notice == "" {
		return ""
	}
	for _, issue := range report.Issues {
		if strings.Contains(issue, "失败") {
			notice = issue
			break
		}
	}
	runes := []rune(notice)
	if len(runes) > 32 {
		return string(runes[:32]) + "…（详见日志）"
	}
	return notice
}
