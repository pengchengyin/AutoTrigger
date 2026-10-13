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

func TestDetailEvidenceLinesAndOriginalPath(t *testing.T) {
	path := `C:\样本目录\很长的安装包名称.exe`
	snap := resultSnapshot{Rows: []resultRow{{Path: `C:\config\remote\jobs\id\input\snapshot.exe`, Evidence: "S1:注册表；A1:文件；B1:进程"}}}
	cards := detailCards(snap, taskRecord{Input: path})
	if cards[0].Lines[0] != "路径："+path {
		t.Fatal("snapshot path shown instead of source")
	}
	lines := cards[2].Lines
	if len(lines) != 5 || lines[2] != "证据：S1:注册表" || lines[3] != "A1:文件" || lines[4] != "B1:进程" {
		t.Fatalf("evidence not separated: %v", lines)
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

func TestRunningDetailShowsKnownMetadataOnly(t *testing.T) {
	size := int64(2048)
	record := taskRecord{Input: "setup.exe", FileType: "pe_executable", FileSize: &size}
	cards := detailCards(resultSnapshot{PresentationState: "安装 / 触发进行中"}, record)
	text := strings.Join(append(cards[0].Lines, cards[2].Lines...), "\n")
	for _, want := range []string{"可执行程序", "2048 字节", "等待实际执行记录", "安装 / 触发进行中", "置信度：未提供"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s: %s", want, text)
		}
	}
	if methodText("silent_install") != "静默安装" {
		t.Fatal("method not translated")
	}
}
