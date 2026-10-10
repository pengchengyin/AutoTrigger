package ui

import (
	"autotrigger/pkg/schema"
	"encoding/json"
	"fmt"
	"path/filepath"
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
	if report == nil {
		rows = rows[:1]
		for _, engine := range []string{"paddleocr_json", "rapidocr_json", "umi_ocr", "tesseract", "windows"} {
			rows = append(rows, dependencyState{Name: ocrDisplayName(engine), Status: "未检查", Kind: "info"})
		}
	}
	if checking {
		if report != nil && len(report.OCREngines) > 0 {
			rows = dependencyStates(report, false)
		}
		for i := range rows {
			rows[i].Status = "检测与部署中"
			rows[i].Kind = "info"
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
	if len(report.OCREngines) > 0 {
		rows = rows[:1]
		for _, engine := range report.OCREngines {
			name := ocrDisplayName(engine.Engine)
			if engine.PackagePath != nil {
				name = strings.TrimSuffix(filepath.Base(*engine.PackagePath), filepath.Ext(*engine.PackagePath))
			}
			if engine.Engine == "umi_ocr" && engine.Path == nil && engine.PackagePath == nil {
				name += " · 本地服务"
			}
			if engine.Selected {
				name += " · 当前选择"
			}
			status, kind := dependencyStatus(engine.Status)
			if engine.Selected {
				status = "当前 · " + strings.TrimPrefix(status, "已安装 · ")
			}
			detail := engine.Detail
			if engine.Path != nil {
				detail = *engine.Path + "；" + detail
			}
			if engine.Version != nil && *engine.Version != "unknown" {
				detail = "版本 " + *engine.Version + "；" + detail
			}
			rows = append(rows, dependencyState{Name: name, Status: status, Kind: kind, Detail: detail})
		}
	}
	return rows
}

func ocrDisplayName(engine string) string {
	name := map[string]string{"paddleocr_json": "PaddleOCR-json", "rapidocr_json": "RapidOCR-json", "umi_ocr": "Umi-OCR", "tesseract": "Tesseract", "windows": "Windows OCR"}[engine]
	if name == "" {
		return engine
	}
	return name
}

func dependencyStatus(status string) (string, string) {
	switch status {
	case "ok":
		return "已安装 · 可用", "ok"
	case "startup_failed":
		return "自检失败", "fail"
	case "deploy_failed":
		return "部署失败", "fail"
	case "not_deployed":
		return "未部署（只读）", "warn"
	case "service_required":
		return "需启动服务", "warn"
	case "unsupported":
		return "不支持 / 未实现", "warn"
	case "disabled":
		return "已关闭", "info"
	case "missing":
		return "未安装 / 不可用", "warn"
	default:
		return "检测异常", "warn"
	}
}

func dependencySummary(report *schema.Prereq, checking bool) string {
	rows := dependencyStates(report, checking)
	if report != nil && len(report.OCREngines) > 0 && !checking {
		available := 0
		for _, engine := range report.OCREngines {
			if engine.Status == "ok" {
				available++
			}
		}
		status, _ := dependencyStatus(report.OCREngineStatus)
		return fmt.Sprintf("7-Zip：%s；当前 OCR：%s；清单 %d/%d 项可用", rows[0].Status, status, available, len(report.OCREngines))
	}
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
