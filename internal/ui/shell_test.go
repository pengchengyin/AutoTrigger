package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNavItemsCoverRequiredPages(t *testing.T) {
	if len(uiNavItems) != 5 {
		t.Fatalf("导航项数量 = %d，期望 5", len(uiNavItems))
	}
	seen := map[pageID]string{}
	for _, item := range uiNavItems {
		if item.Label == "" || item.Icon == "" {
			t.Fatalf("导航项缺少图标或名称：%+v", item)
		}
		if prev, ok := seen[item.ID]; ok {
			t.Fatalf("页面 %d 被重复注册为 %q 和 %q", item.ID, prev, item.Label)
		}
		seen[item.ID] = item.Label
		if pageTitle(item.ID) != item.Label {
			t.Fatalf("pageTitle(%d) = %q，期望 %q", item.ID, pageTitle(item.ID), item.Label)
		}
	}
	if _, ok := seen[pageTaskDetail]; ok {
		t.Fatal("detail must be reached from a task")
	}
	if _, ok := seen[pageConfig]; ok {
		t.Fatal("config must be inside settings")
	}
	if _, ok := seen[pageLogs]; ok {
		t.Fatal("logs must be inside task detail")
	}
}

func TestLogFilterHandlesCLILocalizedLevels(t *testing.T) {
	input := "[INFO] first\n2026-10-09 [信息] 【任务】second\n[警告] missing OCR\n[ERROR] failed\n[错误] failed too"
	for _, test := range []struct {
		level string
		count int
	}{{"INFO", 2}, {"WARN", 1}, {"ERROR", 2}, {"DEBUG", 0}} {
		got := filterLogLines(input, test.level)
		count := 0
		if got != "" {
			count = len(strings.Split(got, "\n"))
		}
		if count != test.count {
			t.Fatalf("%s: %q", test.level, got)
		}
	}
}

func TestHomeStatsUsesRealSummaryOnly(t *testing.T) {
	cards := homeStats(nil, false)
	if len(cards) != 4 {
		t.Fatalf("首页统计卡数量 = %d，期望 4", len(cards))
	}
	for _, c := range cards {
		if c.Value != "0" {
			t.Fatalf("无结果 summary 时不得显示演示数据：%+v", cards)
		}
	}
	if cards[0].Detail != "暂无真实任务数据" {
		t.Fatalf("无真实数据时必须明确标注来源，实得 %q", cards[0].Detail)
	}

	s := &ResultSummary{Total: 5, ByStatus: map[string]int{
		"installed":         2,
		"already_installed": 1,
		"failed":            1,
		"timeout":           1,
	}}
	cards = homeStats(s, false)
	if cards[0].Value != "5" || cards[1].Value != "3" || cards[2].Value != "2" || cards[3].Value != "0" {
		t.Fatalf("summary 统计映射异常：%+v", cards)
	}

	cards = homeStats(s, true)
	if cards[3].Value != "1" || cards[0].Detail != "当前任务运行中" {
		t.Fatalf("运行中状态应单独反映到首页统计：%+v", cards)
	}
}

func TestPageCardsUseRealState(t *testing.T) {
	opts := Options{
		Output:     `D:\out\result.json`,
		Config:     `D:\cfg\config.json`,
		Recursive:  true,
		NoOCR:      true,
		Lock:       true,
		TimeoutSec: 300,
		LogLevel:   "debug",
	}
	tmpl := templateInfo{ResourceVersion: "0.1.0", Mode: "merged", Masters: 0, Builtin: 0, External: 2}
	summary := &ResultSummary{Total: 2, ByStatus: map[string]int{"installed": 1, "failed": 1}}

	templateCards := pageCards(pageTemplates, opts, `D:\tool`, tmpl, nil, false)
	if len(templateCards) == 0 || !hasLineContaining(templateCards, "外部变体：2") {
		t.Fatalf("模板页应展示真实模板计数：%+v", templateCards)
	}

	logCards := pageCards(pageLogs, opts, `D:\tool`, tmpl, summary, false)
	if !hasLineContaining(logCards, `D:\out\result.json`) || !hasLineContaining(logCards, "样本总数 2") {
		t.Fatalf("日志页应展示当前结果路径和 summary：%+v", logCards)
	}

	settingsCards := pageCards(pageSettings, opts, `D:\tool`, tmpl, summary, false)
	for _, want := range []string{"超时：300 秒", "禁用 OCR：是", "单实例锁：开启"} {
		if !hasLineContaining(settingsCards, want) {
			t.Fatalf("设置页缺少 %q：%+v", want, settingsCards)
		}
	}

	detailCards := pageCards(pageTaskDetail, opts, `D:\tool`, tmpl, summary, false)
	if !hasLineContaining(detailCards, "部分成功") {
		t.Fatalf("任务详情页应按结果 summary 推导总体结论：%+v", detailCards)
	}
}

func TestReadResultSnapshotRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	body := `{
  "schema_version":"2.8",
  "task_id":"task-1",
  "started_at":"2026-10-09T10:00:00Z",
  "platform":"windows",
  "summary":{"total":1,"by_status":{"installed":1}},
  "results":[{
    "file_path":"D:\\samples\\setup.exe",
    "detected_type":"exe",
    "final_status":"install_completed",
    "install_confidence":"high",
    "attempts":[{"method":"silent_install","install_evidence":[{"id":"S1","type":"file","detail":"created app.exe"}]}],
    "notes":"done"
  }]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, ok := readResultSnapshot(path)
	if !ok {
		t.Fatal("应能读取结果快照")
	}
	if snap.TaskID != "task-1" || snap.Summary.Total != 1 || len(snap.Rows) != 1 {
		t.Fatalf("结果快照异常：%+v", snap)
	}
	row := snap.Rows[0]
	if row.File != "setup.exe" || row.Type != "exe" || row.Status != "安装成功" || row.Confidence != "high" || row.Method != "silent_install" {
		t.Fatalf("文件行映射异常：%+v", row)
	}
	report := resultSnapshotReport(snap)
	for _, want := range []string{"task-1", "总体结论：安装成功", "silent_install", "S1:created app.exe", "done"} {
		if !strings.Contains(report, want) {
			t.Fatalf("结果报告缺少 %q：\n%s", want, report)
		}
	}
}

func hasLineContaining(cards []infoCard, want string) bool {
	for _, card := range cards {
		if strings.Contains(card.Title, want) {
			return true
		}
		for _, line := range card.Lines {
			if strings.Contains(line, want) {
				return true
			}
		}
	}
	return false
}
