//go:build windows

package ui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func (f *form) selectDetectedOCR() string {
	if f.isChecked(idNoOCR) {
		return "OCR 已由用户禁用，未自动开启。"
	}
	engines := []string{"paddleocr_json", "rapidocr_json", "umi_ocr", "tesseract", "windows", "none"}
	index := f.comboIndex(idOCREngine)
	if index < 0 || index >= len(engines) {
		return ""
	}
	path := strings.TrimSpace(windowText(f.ctrls[idOCREnginePath]))
	row, ok := availableOCR(f.prereqReport, engines[index], path)
	if !ok {
		return "未找到自检通过的 OCR，已保留原设置。"
	}
	newPath := "auto"
	if row.Path != nil && *row.Path != "" {
		newPath = *row.Path
	}
	// Preserve the selected transport if that exact configuration passed its probe.
	if row.Selected && f.prereqReport.OCREngineStatus == "ok" && engines[index] == row.Engine && (path == newPath || path == "auto") {
		return "当前 OCR 已通过自检，保留原设置。"
	}
	dirty := f.settingsChanged()
	for i, engine := range engines {
		if engine == row.Engine {
			f.setComboSel(idOCREngine, i)
		}
	}
	setWindowTextRaw(f.ctrls[idOCREnginePath], newPath)
	f.detectedOCREngine, f.detectedOCRPath = row.Engine, newPath
	if dirty {
		return fmt.Sprintf("已选择可用 OCR：%s；存在未保存设置，请点击保存设置后生效。", row.Engine)
	}
	body, _, _, err := ConfigEditorBody(strings.TrimSpace(windowText(f.ctrls[idConfig])), filepath.Dir(f.dataLayout().Config()))
	var doc map[string]interface{}
	if err == nil {
		err = json.Unmarshal([]byte(body), &doc)
	}
	if err == nil {
		section, ok := doc["ocr"].(map[string]interface{})
		if !ok {
			section = map[string]interface{}{}
			doc["ocr"] = section
		}
		section["enabled"], section["engine"], section["engine_path"], section["transport"] = true, row.Engine, newPath, ocrTransport(row.Engine)
		var b []byte
		b, err = json.MarshalIndent(doc, "", "  ")
		if err == nil {
			_, err = WriteConfigEditorBody(f.dataLayout().Config(), string(b), filepath.Dir(f.dataLayout().Config()))
		}
	}
	if err != nil {
		return fmt.Sprintf("已选择 %s，但保存失败：%v；请在设置中重试保存。", row.Engine, err)
	}
	setWindowTextRaw(f.ctrls[idConfig], f.dataLayout().Config())
	f.detectedOCREngine, f.detectedOCRPath = "", ""
	f.loadSettingsFields()
	f.rememberSettingsDraft()
	return fmt.Sprintf("已自动选择并保存可用 OCR：%s。", row.Engine)
}
