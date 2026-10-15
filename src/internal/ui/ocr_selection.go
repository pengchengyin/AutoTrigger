package ui

import (
	"autotrigger/pkg/schema"
	"strings"
)

func availableOCR(report *schema.Prereq, current, path string) (schema.OCREngineCheck, bool) {
	if report == nil {
		return schema.OCREngineCheck{}, false
	}
	valid := func(row schema.OCREngineCheck) bool {
		if row.Status != "ok" {
			return false
		}
		switch row.Engine {
		case "umi_ocr":
			return true
		case "paddleocr_json", "rapidocr_json", "tesseract":
			return row.Path != nil && strings.TrimSpace(*row.Path) != ""
		}
		return false
	}
	for _, row := range report.OCREngines {
		if valid(row) && row.Engine == current && (row.Path != nil && strings.EqualFold(*row.Path, path) || row.Selected && (path == "auto" || path == "")) {
			return row, true
		}
	}
	for _, engine := range []string{current, "paddleocr_json", "rapidocr_json", "umi_ocr", "tesseract"} {
		for _, row := range report.OCREngines {
			if row.Engine == engine && valid(row) {
				return row, true
			}
		}
	}
	return schema.OCREngineCheck{}, false
}

func ocrTransport(engine string) string {
	if engine == "umi_ocr" {
		return "http"
	}
	return "stdio"
}
