package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/pkg/schema"
	"context"
)

func TestTrimExtAndSanitizeName(t *testing.T) {
	for in, want := range map[string]string{
		"SoloMD_4.14.10_x64_en-US.msi": "SoloMD_4.14.10_x64_en-US",
		"noext":                        "noext",
		"a.b.c":                        "a.b",
		".hidden":                      "",
	} {
		if got := trimExt(in); got != want {
			t.Errorf("trimExt(%q) = %q，期望 %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"msi":         "msi",
		"silent_inst": "silent_inst",
		"a b/c:d":     "a_b_c_d",
		"尝试/安装":       "_____",
		"":            "attempt",
	} {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q，期望 %q", in, got, want)
		}
	}
}

type d18MSITrigger struct{}

func (d18MSITrigger) Commands(path string, _ ports.FileType, temp string) ([]ports.Command, error) {
	log := filepath.Join(temp, "MSI-0.log")
	text := "Windows Installer 已安装产品。安装成功或错误状态: 1603。\r\nAction ended: InstallFinalize. Return value 3."
	b := []byte{0xff, 0xfe}
	for _, r := range text {
		b = append(b, byte(r), byte(r>>8))
	}
	if err := os.WriteFile(log, b, 0600); err != nil {
		return nil, err
	}
	return []ports.Command{{Program: path, Method: "msi", MSI: true, LogPath: log}}, nil
}

func TestD18FailedMSILogSurvivesFullTaskCleanup(t *testing.T) {
	e, r := engineFor(t)
	r.code = 1603
	e.Trigger = d18MSITrigger{}
	e.ResultPath = filepath.Join(t.TempDir(), "result.json")
	path := sample(t, "fatal.msi", []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})
	if err := e.Run(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	f := e.Result.Results[0]
	if f.FinalStatus != "failed" {
		t.Fatal(f)
	}
	kept := filepath.Join(e.failureLogDir(), "fatal-1-msi.log")
	if text := loadInstallerLog(kept, installerLogTailBytes); !strings.Contains(text, "1603") || strings.ContainsRune(text, 0) {
		t.Fatalf("retained log unreadable: %q", text)
	}
	if _, err := os.Stat(*f.Attempts[0].MSILog); !os.IsNotExist(err) {
		t.Fatal("task temp not cleaned", err)
	}
}

// 默认落在结果文件同目录；结果写 stdout（"-"）时无处可放，必须返回空串而不是乱造路径。
func TestFailureLogDirPlacement(t *testing.T) {
	c := config.Defaults()
	e := &Engine{Config: c, ResultPath: filepath.Join("D:", "out", "autotrigger-result.json")}
	if got, want := e.failureLogDir(), filepath.Join("D:", "out", "autotrigger-result-msi-logs"); got != want {
		t.Fatalf("默认目录 = %q，期望 %q", got, want)
	}
	e.ResultPath = "-"
	if got := e.failureLogDir(); got != "" {
		t.Fatalf("结果写 stdout 时应返回空串，实得 %q", got)
	}
	c.MSI.LogDir = "D:/deploy/msi-logs"
	e = &Engine{Config: c, ResultPath: "-"}
	if got := e.failureLogDir(); got != "D:/deploy/msi-logs" {
		t.Fatalf("配置的 log_dir 应优先，实得 %q", got)
	}
}

// 安装失败（未判成功）时，MSI 详细日志必须被搬出临时目录 ——
// 否则"要查日志"的时候日志正好被清理掉了。
func TestKeepFailureLogsCopiesOnFailure(t *testing.T) {
	work := t.TempDir()
	src := filepath.Join(work, "MSI-0.log")
	const body = "Action ended: InstallFinalize. Return value 3."
	if err := os.WriteFile(src, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &Engine{Config: config.Defaults(), Log: fakeLogger{}, ResultPath: filepath.Join(work, "result.json")}
	f := schema.FileResult{FinalStatus: "install_partial", Attempts: []schema.Attempt{{Method: "msi", MSILog: schema.String(src)}}}
	e.keepFailureLogs(filepath.Join(work, "SoloMD.msi"), f)

	dst := filepath.Join(work, "result-msi-logs", "SoloMD-1-msi.log")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("失败样本的 MSI 日志未被保留：%v", err)
	}
	if string(got) != body {
		t.Fatalf("保留内容 = %q，期望 %q", got, body)
	}
}

// 成功样本不留日志，避免批量跑完堆一地的噪音文件。
func TestKeepFailureLogsSkipsCompleted(t *testing.T) {
	work := t.TempDir()
	src := filepath.Join(work, "MSI-0.log")
	os.WriteFile(src, []byte("ok"), 0o600)
	e := &Engine{Config: config.Defaults(), Log: fakeLogger{}, ResultPath: filepath.Join(work, "result.json")}
	f := schema.FileResult{FinalStatus: "install_completed", Attempts: []schema.Attempt{{Method: "msi", MSILog: schema.String(src)}}}
	e.keepFailureLogs(filepath.Join(work, "a.msi"), f)
	if _, err := os.Stat(filepath.Join(work, "result-msi-logs")); !os.IsNotExist(err) {
		t.Fatal("判成功的样本不应产生日志保留目录")
	}
}

// 开关关闭时完全不动作。
func TestKeepFailureLogsRespectsConfig(t *testing.T) {
	work := t.TempDir()
	src := filepath.Join(work, "MSI-0.log")
	os.WriteFile(src, []byte("x"), 0o600)
	c := config.Defaults()
	c.MSI.KeepVerboseLog = false
	e := &Engine{Config: c, Log: fakeLogger{}, ResultPath: filepath.Join(work, "result.json")}
	f := schema.FileResult{FinalStatus: "failed", Attempts: []schema.Attempt{{Method: "msi", MSILog: schema.String(src)}}}
	e.keepFailureLogs(filepath.Join(work, "a.msi"), f)
	if _, err := os.Stat(filepath.Join(work, "result-msi-logs")); !os.IsNotExist(err) {
		t.Fatal("关闭开关后不应保留任何日志")
	}
}

// 源文件已被删掉（例如尝试根本没跑起来）时不应报错，也不能留下空壳文件。
func TestKeepFailureLogsMissingSource(t *testing.T) {
	work := t.TempDir()
	e := &Engine{Config: config.Defaults(), Log: fakeLogger{}, ResultPath: filepath.Join(work, "result.json")}
	f := schema.FileResult{FinalStatus: "failed", Attempts: []schema.Attempt{{Method: "msi", MSILog: schema.String(filepath.Join(work, "gone.log"))}}}
	e.keepFailureLogs(filepath.Join(work, "a.msi"), f)
	entries, err := os.ReadDir(filepath.Join(work, "result-msi-logs"))
	if err == nil && len(entries) != 0 {
		t.Fatalf("源日志缺失时不应产出文件，实得 %d 个", len(entries))
	}
}

// 结果写 stdout 时无处安放，必须警告而不是静默丢弃。
func TestKeepFailureLogsWarnsWhenNoPlace(t *testing.T) {
	work := t.TempDir()
	src := filepath.Join(work, "MSI-0.log")
	os.WriteFile(src, []byte("x"), 0o600)
	e := &Engine{Config: config.Defaults(), Log: fakeLogger{}, ResultPath: "-"}
	f := schema.FileResult{FinalStatus: "failed", Attempts: []schema.Attempt{{Method: "msi", MSILog: schema.String(src)}}}
	e.keepFailureLogs(filepath.Join(work, "a.msi"), f) // 只要求不 panic，且不产生相对目录
	if _, err := os.Stat("autotrigger-result-msi-logs"); !os.IsNotExist(err) {
		t.Fatal("结果写 stdout 时不得在当前目录乱造日志目录")
	}
}

func TestSanitizedNameHasNoPathSeparators(t *testing.T) {
	got := sanitizeName("../evil\\name")
	if strings.ContainsAny(got, `/\`) {
		t.Fatalf("清理后仍含路径分隔符：%q", got)
	}
}
