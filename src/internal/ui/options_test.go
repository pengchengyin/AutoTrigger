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

func TestConfigEditorBodyLoadsDiskOrDefaults(t *testing.T) {
	dir := t.TempDir()
	body, resolved, fromDisk, err := ConfigEditorBody("", dir)
	if err != nil {
		t.Fatalf("默认配置正文: %v", err)
	}
	if resolved != ConfigPath(dir) || fromDisk {
		t.Fatalf("不存在 config.json 时应展示内置默认但不标记为落盘：path=%q fromDisk=%v", resolved, fromDisk)
	}
	if !strings.Contains(body, "timeout_sec") || !strings.Contains(body, "gui_automation") {
		t.Fatalf("默认配置正文缺关键字段：%s", body)
	}

	path := ConfigPath(dir)
	if err := os.WriteFile(path, []byte(`{"output":{"log_level":"debug"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	body, resolved, fromDisk, err = ConfigEditorBody("", dir)
	if err != nil {
		t.Fatalf("读取落盘配置: %v", err)
	}
	if resolved != path || !fromDisk || body != `{"output":{"log_level":"debug"}}` {
		t.Fatalf("落盘配置读取异常：body=%q path=%q fromDisk=%v", body, resolved, fromDisk)
	}

	if _, _, _, err = ConfigEditorBody(filepath.Join(dir, "missing.json"), dir); err == nil {
		t.Fatal("显式指定不存在的配置文件应报错")
	}
}

func TestWriteConfigEditorBodyValidatesBeforeOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := `{"output":{"log_level":"info"}}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteConfigEditorBody(path, `{"output":{"log_level":"verbose"}}`, dir); err == nil {
		t.Fatal("无效配置保存前应被拦截")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Fatalf("无效配置不得覆盖原文件：%s", after)
	}

	warnings, err := WriteConfigEditorBody(path, `{"output":{"log_level":"debug"},"unknown_key":1}`, dir)
	if err != nil {
		t.Fatalf("未知字段只应 warning，不应保存失败: %v", err)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "unknown_key") {
		t.Fatalf("未知字段 warning 缺失：%v", warnings)
	}
	after, _ = os.ReadFile(path)
	if !strings.Contains(string(after), `"debug"`) {
		t.Fatalf("有效配置应写入：%s", after)
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
	if !o.Lock || o.LogLevel != DefaultLogLevel {
		t.Fatalf("默认值与 CLI 不一致：%+v", o)
	}
	// 界面的超时预填是 5 分钟（300 秒），**刻意不同于 CLI 的 600 秒**（C45）：
	// 现场更常用 5 分钟起步。因为 300 != 600，界面会显式下发 --timeout 300，
	// 命令预览与实际参数一致，不存在静默分叉（F20.3 的例外说明）。
	wantTimeout, err := TimeoutSeconds(UITimeoutDefaultValue, UITimeoutDefaultUnit)
	if err != nil {
		t.Fatalf("TimeoutSeconds: %v", err)
	}
	if o.TimeoutSec != wantTimeout || o.TimeoutSec != 300 {
		t.Fatalf("界面默认超时应为 5 分钟（300 秒），实得 %d", o.TimeoutSec)
	}
	// 界面默认超时 300 秒与 CLI 默认 600 秒不同，因此必须显式下发 --timeout 300。
	withInput := o
	withInput.Input = "sample.exe"
	args, err := withInput.Args()
	if err != nil {
		t.Fatalf("Args: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--timeout 300") {
		t.Fatalf("界面默认超时偏离 CLI 默认值时必须显式下发 --timeout 300：%s", joined)
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

// 超时「数值 + 单位」换算：错一位就会把 5 分钟变成 5 秒。
func TestTimeoutSeconds(t *testing.T) {
	cases := []struct {
		value int
		unit  TimeoutUnit
		want  int
	}{
		{5, UnitMinutes, 300},
		{10, UnitMinutes, 600},
		{0, UnitMinutes, 0}, // 0 表示不限（F9）
		{90, UnitSeconds, 90},
		{0, UnitSeconds, 0},
	}
	for _, c := range cases {
		got, err := TimeoutSeconds(c.value, c.unit)
		if err != nil {
			t.Fatalf("TimeoutSeconds(%d, %d): %v", c.value, c.unit, err)
		}
		if got != c.want {
			t.Errorf("TimeoutSeconds(%d, %d) = %d，期望 %d", c.value, c.unit, got, c.want)
		}
	}
	if _, err := TimeoutSeconds(-1, UnitMinutes); err == nil {
		t.Error("负数超时应报错")
	}
}

// SplitTimeout 把秒数拆回输入框显示，能被 60 整除时优先显示分钟。
func TestSplitTimeout(t *testing.T) {
	cases := []struct {
		sec      int
		wantVal  int
		wantUnit TimeoutUnit
	}{
		{300, 5, UnitMinutes},
		{600, 10, UnitMinutes},
		{90, 90, UnitSeconds},
		{0, 0, UnitSeconds},
	}
	for _, c := range cases {
		value, unit := SplitTimeout(c.sec)
		if value != c.wantVal || unit != c.wantUnit {
			t.Errorf("SplitTimeout(%d) = (%d, %d)，期望 (%d, %d)", c.sec, value, unit, c.wantVal, c.wantUnit)
		}
	}
}

// 结束横幅是"安装成功 / 失败"的显式提示（F20.6），文案与统计都固定下来。
func TestResultBannerStatesOutcome(t *testing.T) {
	s := &ResultSummary{Total: 3, ByStatus: map[string]int{"installed": 2, "failed": 1}, Path: `D:\out\r.json`}
	lines, outcome := ResultBanner(0, s, `D:\out\r.json`)
	body := strings.Join(lines, "\n")
	if outcome != "partial" {
		t.Fatalf("2 成功 1 失败应为部分成功，实得 %q", outcome)
	}
	for _, want := range []string{"部分成功", "样本总数 3", "安装成功 2", "失败 1", `D:\out\r.json`} {
		if !strings.Contains(body, want) {
			t.Errorf("横幅缺少 %q：\n%s", want, body)
		}
	}

	lines, outcome = ResultBanner(0, &ResultSummary{Total: 1, ByStatus: map[string]int{"installed": 1}}, "")
	if outcome != "ok" || !strings.Contains(strings.Join(lines, "\n"), "安装成功") {
		t.Fatalf("全成功应给出成功结论，实得 %q：%v", outcome, lines)
	}

	// 只有"未验证"成功时必须自成档：结论行不得冒充"安装成功"，
	// 否则会与统计行的"安装未验证 1"口径打架（F20.6）。
	lines, outcome = ResultBanner(0, &ResultSummary{Total: 1, ByStatus: map[string]int{"unverified": 1}}, "")
	body = strings.Join(lines, "\n")
	if outcome != "unverified" {
		t.Fatalf("仅未验证成功应给出 unverified 结论，实得 %q", outcome)
	}
	if !strings.Contains(body, "安装成功（未验证）") || !strings.Contains(body, "安装未验证 1") {
		t.Fatalf("未验证档的结论行与统计行应措辞一致：\n%s", body)
	}

	// 读不到结果 JSON 时按退出码兜底，不得编造安装结论
	lines, outcome = ResultBanner(4, nil, "")
	if strings.Join(lines, "\n") == "" || !strings.Contains(strings.Join(lines, "\n"), "7-Zip") {
		t.Fatalf("缺少结果文件时应按退出码给出说明：%v", lines)
	}
	if len(lines) == 0 || strings.Contains(strings.Join(lines, "\n"), "安装成功") {
		t.Fatalf("读不到结果时不得给出安装结论：%v", lines)
	}
}

// ReadResultSummary 只读 summary 段；路径异常/文件不存在时返回 ok=false。
func TestReadResultSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")
	body := `{"schema_version":"2.8","summary":{"total":2,"by_status":{"installed":1,"timeout":1}},"results":[]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, ok := ReadResultSummary(path)
	if !ok {
		t.Fatal("应能解析出 summary")
	}
	if s.Total != 2 || s.ByStatus["installed"] != 1 || s.ByStatus["timeout"] != 1 {
		t.Fatalf("summary 解析结果异常：%+v", s)
	}
	for _, bad := range []string{"", "-", filepath.Join(dir, "不存在.json")} {
		if _, ok := ReadResultSummary(bad); ok {
			t.Errorf("路径 %q 不应解析成功", bad)
		}
	}
}
