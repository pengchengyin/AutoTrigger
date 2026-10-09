package ui

import (
	"strings"
	"testing"
	"time"
)

func TestDetailCardsUseRealValues(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(12 * time.Second)
	snap := resultSnapshot{StartedAt: start, FinishedAt: &end, Rows: []resultRow{{Path: "setup.exe", SizeBytes: 1048576, Type: "exe", Status: "已安装（无需安装）", Confidence: "high", Method: "silent_install", Evidence: "S1:真实证据"}}}
	cards := detailCards(snap, taskRecord{})
	text := strings.Join(append(append(cards[0].Lines, cards[1].Lines...), cards[2].Lines...), "\n")
	for _, want := range []string{"1.00 MB", "12.0 秒", "任务退出码：未提供", "置信度：高", "已安装（无需安装）"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	snap.Error = "配置错误"
	if strings.Contains(detailCards(snap, taskRecord{})[2].Lines[1], "已安装") {
		t.Fatal("top-level error must override file status")
	}
}

func TestDetailCardsUnknownAndMultipleConfidence(t *testing.T) {
	if got := detailCards(resultSnapshot{}, taskRecord{})[2].Lines[0]; got != "置信度：未提供" {
		t.Fatal(got)
	}
	snap := resultSnapshot{Rows: []resultRow{{Confidence: "high"}, {Confidence: "low"}}}
	if got := detailCards(snap, taskRecord{})[2].Lines[0]; got != "置信度：按文件查看" {
		t.Fatal(got)
	}
}
