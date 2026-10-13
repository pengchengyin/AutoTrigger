package ui

import (
	"autotrigger/pkg/schema"
	"strings"
	"testing"
)

func TestPrereqStatusesDoNotInferSuccessFromExit(t *testing.T) {
	report := schema.EmptyPrereq()
	report.OK = true // Optional OCR can be missing even when the check exits zero.
	report.ArchiveBackendStatus = "ok"
	report.OCREngineStatus = "missing"
	rows := dependencyStates(&report, false)
	if rows[0].Kind != "ok" || rows[1].Kind != "warn" || !strings.Contains(rows[1].Status, "未安装") {
		t.Fatal(rows)
	}
	report.OCREngineStatus = "startup_failed"
	if dependencyStates(&report, false)[1].Status != "自检失败" {
		t.Fatal("startup failure mislabeled")
	}
	report.OCREngineStatus = "disabled"
	if dependencyStates(&report, false)[1].Status != "已关闭" {
		t.Fatal("disabled OCR mislabeled")
	}
	if dependencyStates(nil, true)[0].Status != "检测与部署中" {
		t.Fatal("missing pending state")
	}
	if parsePrereqReport([]byte(`{"task_id":"failed-check","error":{"code":3}}`)) != nil {
		t.Fatal("task error accepted as prereq report")
	}
	if parsePrereqReport([]byte(`{"archive_backend_status":"ok","ocr_engine_status":"ok"}`)) == nil {
		t.Fatal("valid report rejected")
	}
}

func TestPrereqIssuesKeepActualFailures(t *testing.T) {
	report := schema.EmptyPrereq()
	report.Issues = []string{"已自动部署 OCR 到工具目录：test", "OCR 能力自检失败：models missing"}
	if got := dependencyIssues(&report); got != report.Issues[1] {
		t.Fatal(got)
	}
}

func TestOCRInventoryShowsEveryRowAndActualSelectedStatus(t *testing.T) {
	r := schema.EmptyPrereq()
	r.ArchiveBackendStatus = "ok"
	r.OCREngine = "paddleocr_json"
	r.OCREngineStatus = "startup_failed"
	r.OCREngines = []schema.OCREngineCheck{
		{Engine: "paddleocr_json", Status: "startup_failed", Detail: "DLL missing", Selected: true},
		{Engine: "rapidocr_json", Status: "ok", PackagePath: schema.String("RapidOCR-json_v0.2.0.7z")},
		{Engine: "umi_ocr", Status: "service_required"},
		{Engine: "tesseract", Status: "missing"},
		{Engine: "windows", Status: "unsupported"},
	}
	rows := dependencyStates(&r, false)
	if len(rows) != 6 || !strings.Contains(rows[1].Name, "当前选择") || rows[1].Detail != "DLL missing" || rows[2].Kind != "ok" || rows[3].Status != "需启动服务" {
		t.Fatal(rows)
	}
	if summary := dependencySummary(&r, false); !strings.Contains(summary, "当前 OCR：自检失败") || !strings.Contains(summary, "1/5") {
		t.Fatal(summary)
	}
	if len(dependencyStates(&r, true)) != 6 {
		t.Fatal("recheck hid engine inventory")
	}
}
