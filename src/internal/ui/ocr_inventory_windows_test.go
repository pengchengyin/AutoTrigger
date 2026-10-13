//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/pkg/schema"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestWindowsOCRInventory(t *testing.T) {
	dir, tool := os.Getenv("AUTOTRIGGER_UI_SHOTS"), os.Getenv("AUTOTRIGGER_OCR_INVENTORY_EXE")
	if dir == "" || tool == "" {
		t.Skip("set UI shots and OCR inventory executable for real multi-engine checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, "--check-prereq", "--no-deploy-deps", "--json", "--output=-", "--log-file", filepath.Join(dir, "ocr-inventory-readonly.log"))
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("real inventory failed: %v", err)
	}
	var report schema.Prereq
	if err = json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	available := map[string]bool{}
	for _, row := range report.OCREngines {
		if row.Status == "ok" {
			available[row.Engine] = true
		}
	}
	if !available["paddleocr_json"] || !available["rapidocr_json"] {
		t.Fatalf("real packs did not pass: %s", data)
	}
	if err = os.WriteFile(filepath.Join(dir, "ocr-inventory-real.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()
	f := newForm(cli.Dependencies{})
	f.className = utf16Ptr("AutoTriggerOCRInventoryRegression")
	f.exeDir = t.TempDir()
	f.historyDir = filepath.Join(f.exeDir, "history")
	f.history = nil
	f.opts = NewOptions(f.exeDir)
	f.templates = loadTemplateInfo(f.exeDir)
	if err = f.create(); err != nil {
		t.Fatal(err)
	}
	defer procDestroyWindow.Call(f.hwnd)
	f.prereqChecked, f.prereqReport = true, &report
	f.show()
	f.layout()
	captureNativeWindow(t, f.hwnd, filepath.Join(dir, "home-ocr-inventory.png"))
	send(f.ctrls[idDependencyDetails], 0xf5, 0, 0)
	pane := helpPanes[f.helpWindow]
	if pane == nil {
		t.Fatal("home detail button did not open inventory")
	}
	if len(dependencyStates(&report, false)) != len(report.OCREngines)+1 {
		t.Fatal("inventory rows lost")
	}
	captureNativeWindow(t, f.helpWindow, filepath.Join(dir, "ocr-inventory-details.png"))
	procDestroyWindow.Call(f.helpWindow)
	f.settingsRecognition = true
	f.settingsAdvanced = false
	f.setPage(pageSettings)
	if got := int(send(f.ctrls[idDependencyList], 0x1004, 0, 0)); got != len(report.OCREngines)+1 {
		t.Fatalf("visible list lost engines: %d", got)
	}
	captureNativeWindow(t, f.hwnd, filepath.Join(dir, "settings-ocr-inventory.png"))
	procMoveWindow.Call(f.hwnd, 10, 10, uintptr(f.px(920)), uintptr(f.px(600)), 1)
	captureNativeWindow(t, f.hwnd, filepath.Join(dir, "settings-ocr-inventory-small.png"))
}
