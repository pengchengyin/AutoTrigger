//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/pkg/schema"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsOCRSelectionSaveAndDraft(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := newForm(cli.Dependencies{})
	f.exeDir, f.dataDir = t.TempDir(), t.TempDir()
	f.dataError = nil
	f.historyDir = f.dataLayout().History()
	f.history = nil
	f.opts.Config = ""
	f.opts.Output = filepath.Join(f.dataLayout().Results(), DefaultResultName)
	if err := f.create(); err != nil {
		t.Fatal(err)
	}
	defer procDestroyWindow.Call(f.hwnd)
	f.setPage(pageSettings)
	path := filepath.Join(f.dataDir, "rapid.exe")
	r := schema.EmptyPrereq()
	r.OCREngines = []schema.OCREngineCheck{{Engine: "rapidocr_json", Status: "ok", Path: &path}}
	f.prereqReport = &r
	if notice := f.selectDetectedOCR(); !strings.Contains(notice, "已自动选择并保存") {
		t.Fatal(notice)
	}
	cfg, _, err := config.Load(f.dataLayout().Config(), filepath.Dir(f.dataLayout().Config()))
	if err != nil || cfg.OCR.Engine != "rapidocr_json" || cfg.OCR.EnginePath != path || cfg.OCR.Transport != "stdio" {
		t.Fatalf("bad saved OCR: %+v %v", cfg.OCR, err)
	}
	if f.settingsChanged() {
		t.Fatal("auto-save left a dirty baseline")
	}
	setWindowTextRaw(f.ctrls[idOCRScore], "0.73")
	r.OCREngines = []schema.OCREngineCheck{{Engine: "paddleocr_json", Status: "ok", Path: schema.String(filepath.Join(f.dataDir, "paddle.exe"))}}
	if notice := f.selectDetectedOCR(); !strings.Contains(notice, "未保存") {
		t.Fatal(notice)
	}
	cfg, _, err = config.Load(f.dataLayout().Config(), filepath.Dir(f.dataLayout().Config()))
	if err != nil || cfg.OCR.Engine != "rapidocr_json" || windowText(f.ctrls[idOCRScore]) != "0.73" {
		t.Fatal("draft overwritten")
	}
	f.discardSettingsDraft()
	send(f.ctrls[idNoOCR], bmSetCheck, bstChecked, 0)
	if notice := f.selectDetectedOCR(); !strings.Contains(notice, "禁用") {
		t.Fatal(notice)
	}
}
