package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArgsRequiresInput(t *testing.T) {
	if _, err := (Options{}).Args(); err == nil {
		t.Fatal("空输入应当报错，而不是下发一条没有 --input 的命令")
	}
}

func TestArgsMatchesCLIFlags(t *testing.T) {
	o := Options{
		Input:      `D:\samples\setup.exe`,
		Output:     `D:\out\result.json`,
		Config:     `D:\cfg\config.json`,
		Recursive:  true,
		NoArchive:  true,
		NoOCR:      true,
		Lock:       false,
		TimeoutSec: 30,
		LogLevel:   "debug",
	}
	args, err := o.Args()
	if err != nil {
		t.Fatalf("Args: %v", err)
	}
	want := "--input D:\\samples\\setup.exe --output D:\\out\\result.json --config D:\\cfg\\config.json " +
		"--recursive --no-archive --no-ocr --no-lock --timeout 30 --log-level debug"
	if got := strings.Join(args, " "); got != want {
		t.Fatalf("参数不一致\n got: %s\nwant: %s", got, want)
	}
}

func TestArgsOmitsDefaults(t *testing.T) {
	o := Options{Input: "a.exe", Lock: true, TimeoutSec: DefaultTimeoutSec, LogLevel: DefaultLogLevel}
	args, err := o.Args()
	if err != nil {
		t.Fatalf("Args: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, unwanted := range []string{"--timeout", "--log-level", "--no-lock"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("默认值不应下发 %s：%s", unwanted, joined)
		}
	}
}

func TestArgsRejectsNegativeTimeout(t *testing.T) {
	if _, err := (Options{Input: "a.exe", TimeoutSec: -1}).Args(); err == nil {
		t.Fatal("负超时应当报错")
	}
}

func TestCommandLineQuotesSpaces(t *testing.T) {
	o := Options{Input: `C:\my samples\a b.exe`}
	line, err := o.CommandLine("AutoTrigger.exe")
	if err != nil {
		t.Fatalf("CommandLine: %v", err)
	}
	if !strings.Contains(line, `"C:\my samples\a b.exe"`) {
		t.Fatalf("含空格的路径未加引号：%s", line)
	}
	if !strings.HasPrefix(line, "AutoTrigger.exe --input ") {
		t.Fatalf("命令预览格式异常：%s", line)
	}
}

func TestCommandLinePropagatesInputError(t *testing.T) {
	if _, err := (Options{}).CommandLine("AutoTrigger.exe"); err == nil {
		t.Fatal("缺样本时命令预览应当报错，而不是给出半截命令")
	}
}

func TestWriteConfigFileNoClobber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	wrote, err := WriteConfigFile(path, false)
	if err != nil {
		t.Fatalf("首次写入: %v", err)
	}
	if !wrote {
		t.Fatal("首次写入应报告 wrote=true")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取默认配置: %v", err)
	}
	for _, key := range []string{"timeout_sec", "install_verify", "gui_automation", "ocr"} {
		if !strings.Contains(string(body), key) {
			t.Fatalf("默认配置缺少字段 %q", key)
		}
	}
	// 已有内容时不得被覆盖（否则会抹掉使用者的改动）
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, err = WriteConfigFile(path, false)
	if err != nil {
		t.Fatalf("二次写入: %v", err)
	}
	if wrote {
		t.Fatal("已存在的配置不应被覆盖")
	}
	after, _ := os.ReadFile(path)
	if string(after) != "{}" {
		t.Fatalf("文件内容被改写：%s", after)
	}
	// 显式覆盖时才允许替换
	if _, err := WriteConfigFile(path, true); err != nil {
		t.Fatalf("覆盖写入: %v", err)
	}
	after, _ = os.ReadFile(path)
	if !strings.Contains(string(after), "timeout_sec") {
		t.Fatal("显式覆盖后应写入默认配置")
	}
}

func TestNewOptionsDefaultsMatchCLI(t *testing.T) {
	dir := t.TempDir()
	o := NewOptions(dir)
	if o.Config != "" {
		t.Fatalf("不存在 config.json 时不应预填 --config（会触发退出码 3）：%q", o.Config)
	}
	if o.Output != filepath.Join(dir, DefaultResultName) {
		t.Fatalf("默认结果路径异常：%q", o.Output)
	}
	if !o.Lock || o.TimeoutSec != DefaultTimeoutSec || o.LogLevel != DefaultLogLevel {
		t.Fatalf("默认值与 CLI 不一致：%+v", o)
	}

	// 配置文件存在时才预填
	if _, err := WriteConfigFile(ConfigPath(dir), false); err != nil {
		t.Fatal(err)
	}
	o = NewOptions(dir)
	if o.Config != ConfigPath(dir) {
		t.Fatalf("应预填已存在的配置文件：%q", o.Config)
	}
}
