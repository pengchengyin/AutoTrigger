//go:build windows

package ui

import (
	"testing"
	"unicode/utf8"
	"unsafe"
)

// Win32 结构体尺寸/偏移必须与头文件一致。
//
// 这类错位的典型症状是"随机崩溃 / 界面空白 / 对话框打不开"，现场极难定位；
// 用断言把它挡在提交之前。数值按 **64 位** 布局（本项目只产出 amd64 单文件）。
func TestWin32StructLayout(t *testing.T) {
	cases := []struct {
		name      string
		got, want uintptr
	}{
		{"WNDCLASSEXW", unsafe.Sizeof(wndClassEx{}), 80},
		{"MSG", unsafe.Sizeof(msgT{}), 48},
		{"OPENFILENAMEW", unsafe.Sizeof(openFileNameW{}), 152},
		{"BROWSEINFOW", unsafe.Sizeof(browseInfoW{}), 64},
		{"LOGFONTW", unsafe.Sizeof(logFontW{}), 92},
		{"NONCLIENTMETRICSW", unsafe.Sizeof(nonClientMetricsW{}), 504},
		{"MINMAXINFO", unsafe.Sizeof(minMaxInfoT{}), 40},
		{"RECT", unsafe.Sizeof(rectT{}), 16},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s 大小 = %d，期望 %d", c.name, c.got, c.want)
		}
	}

	offsets := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"NONCLIENTMETRICSW.MessageFont", unsafe.Offsetof(nonClientMetricsW{}.MessageFont), 408},
		{"NONCLIENTMETRICSW.Size", unsafe.Offsetof(nonClientMetricsW{}.Size), 0},
		{"OPENFILENAMEW.Flags", unsafe.Offsetof(openFileNameW{}.Flags), 96},
		{"OPENFILENAMEW.Filter", unsafe.Offsetof(openFileNameW{}.Filter), 24},
		{"OPENFILENAMEW.DefaultExt", unsafe.Offsetof(openFileNameW{}.DefaultExt), 104},
		{"MSG.WParam", unsafe.Offsetof(msgT{}.WParam), 16},
		{"BROWSEINFOW.Callback", unsafe.Offsetof(browseInfoW{}.Callback), 40},
		{"WNDCLASSEXW.WndProc", unsafe.Offsetof(wndClassEx{}.WndProc), 8},
	}
	for _, o := range offsets {
		if o.got != o.want {
			t.Errorf("%s 偏移 = %d，期望 %d", o.name, o.got, o.want)
		}
	}
}

// dialogFilterSlice 必须产出 Win32 要求的"以 NUL 分隔、双 NUL 结尾"结构；
// 结尾少了 NUL 会让文件对话框显示乱码或不显示过滤器。
func TestDialogFilterSliceTerminators(t *testing.T) {
	got := dialogFilterSlice([]string{"所有文件", "*.*", ""})
	want := []uint16{'所', '有', '文', '件', 0, '*', '.', '*', 0, 0}
	if len(got) != len(want) {
		t.Fatalf("长度 = %d，期望 %d（%v）", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 项 = %d，期望 %d（%v）", i, got[i], want[i], got)
		}
	}
}

// flushableLen 只截到完整 UTF-8 边界，避免把被写入操作切开的多字节字符
// 渲染成乱码（日志是流式追加的，写入点落在字符中间完全可能）。
func TestFlushableLenKeepsRunesIntact(t *testing.T) {
	full := []byte("abc中文")
	if n := flushableLen(full); n != len(full) {
		t.Fatalf("完整字符串应全量返回：%d != %d", n, len(full))
	}
	// "文" 为 3 字节：截断 1 或 2 字节时应退回到完整的 "abc中"（6 字节）
	for cut := 1; cut <= 2; cut++ {
		part := full[:len(full)-cut]
		n := flushableLen(part)
		if !utf8.Valid(part[:n]) {
			t.Fatalf("截断 %d 字节后返回的前缀不是合法 UTF-8：%q", cut, part[:n])
		}
		if n != 6 {
			t.Fatalf("截断 %d 字节后应保留完整的 \"abc中\"(6 字节)，实得 %d", cut, n)
		}
	}
	if n := flushableLen(nil); n != 0 {
		t.Fatalf("空缓冲应为 0，实得 %d", n)
	}
	ascii := []byte("plain ascii log line\n")
	if n := flushableLen(ascii); n != len(ascii) {
		t.Fatalf("纯 ASCII 应全量返回，实得 %d", n)
	}
}

// 只读多行 EDIT（日志区）走的是 WM_CTLCOLORSTATIC，而不是 WM_CTLCOLOREDIT。
// 若它与 STATIC 标签共用"透明背景 + 3D 面色"方案，控件绘制文字前不再擦除背景，
// 滚动或追加日志时旧字形残留并与新文本叠加，实测表现为"文字重影、互相覆盖"。
// 本表把"日志区必须白底 + 不透明背景"固定下来，防止回归。
func TestCtlColorSchemeRoles(t *testing.T) {
	if bk, brush := ctlColorScheme(roleLog); bk != opaqueBk || brush != colorWindow {
		t.Errorf("日志区配色 = (背景模式 %d, 画刷 %d)，期望 (不透明 %d, 白底 %d)", bk, brush, opaqueBk, colorWindow)
	}
	for name, role := range map[string]ctlRole{"标签": roleLabel, "状态行": roleStatus} {
		if bk, brush := ctlColorScheme(role); bk != transparentBk || brush != color3DFace {
			t.Errorf("%s配色 = (背景模式 %d, 画刷 %d)，期望 (透明 %d, 3D 面色 %d)", name, bk, brush, transparentBk, color3DFace)
		}
	}
}

// 状态行是"安装成功 / 失败"最直接的提示（F20.6）。
// 固定各档位的颜色与 COLORREF 布局：红在低字节，写错会整体偏色。
func TestStatusColors(t *testing.T) {
	if got := rgb(1, 2, 3); got != 0x00030201 {
		t.Fatalf("COLORREF 布局应为 0x00BBGGRR，实得 %#x", got)
	}
	seen := map[uintptr]string{}
	for _, k := range []string{statusKindOK, statusKindUnverified, statusKindFail, statusKindWarn, statusKindRun} {
		c := statusColors(k)
		if other, dup := seen[c]; dup {
			t.Fatalf("状态档位 %s 与 %s 用了同一个颜色 %#x，成功/失败必须一眼可分", k, other, c)
		}
		seen[c] = k
	}
	if statusColors(statusKindOK) != rgb(0, 122, 0) {
		t.Errorf("成功档位应为绿色，实得 %#x", statusColors(statusKindOK))
	}
	if statusColors(statusKindFail) != rgb(196, 0, 0) {
		t.Errorf("失败档位应为红色，实得 %#x", statusColors(statusKindFail))
	}
	if statusColors("不存在的档位") != rgb(0, 0, 0) {
		t.Errorf("未知档位应回落到黑色，实得 %#x", statusColors("不存在的档位"))
	}
}

// WS_CLIPCHILDREN 必须出现在主窗口样式里，否则父窗口重绘时会盖到子控件（日志框）区域。
func TestMainWindowStyleClipsChildren(t *testing.T) {
	if wsOverlappedWindow&wsClipChildren != 0 {
		t.Fatalf("wsOverlappedWindow 已含 WS_CLIPCHILDREN，测试前提失效")
	}
	if wsClipChildren != 0x02000000 {
		t.Fatalf("WS_CLIPCHILDREN 常量 = %#x，期望 0x02000000", wsClipChildren)
	}
}

// 多行 EDIT 只把 CRLF 当换行，**纯 LF 不换行**（日志会挤成一大团）。
// flushLog 按缓冲区分块追加，因此还要覆盖"CR 与 LF 被切到相邻两段"的边界。
func TestNormalizeCRLF(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		prevCR   bool
		want     string
		wantPrev bool
	}{
		{"纯 LF 补上 CR", "a\nb\n", false, "a\r\nb\r\n", false},
		{"已是 CRLF 不重复插入", "a\r\nb", false, "a\r\nb", false},
		// 上一段已写出 CR，本段只需补 LF —— 拼起来仍是完整的一对 CRLF。
		{"跨段 CR+LF 不重复插入 CR", "\n", true, "\n", false},
		{"空段保留上一段的 CR 状态", "", true, "", true},
		{"段内无换行则清除 CR 状态", "abc", true, "abc", false},
		{"裸 CR 结尾挂起等下一段", "a\r", false, "a\r", true},
		{"中文内容不受影响", "识别\n安装成功\n", false, "识别\r\n安装成功\r\n", false},
	}
	for _, c := range cases {
		got, gotPrev := normalizeCRLF(c.in, c.prevCR)
		if got != c.want || gotPrev != c.wantPrev {
			t.Errorf("%s：normalizeCRLF(%q, %v) = (%q, %v)，期望 (%q, %v)", c.name, c.in, c.prevCR, got, gotPrev, c.want, c.wantPrev)
		}
		if !utf8.ValidString(got) {
			t.Errorf("%s：结果不是合法 UTF-8：%q", c.name, got)
		}
	}
}

// 首选项与级别/单位下拉框必须一一对应：错位会把"5 分钟"变成"5 秒"。
func TestDropdownOptionsAlign(t *testing.T) {
	if len(LogLevelLabels) != len(LogLevelValues) {
		t.Fatalf("日志级别下拉框标签 %d 项 != 取值 %d 项", len(LogLevelLabels), len(LogLevelValues))
	}
	if len(TimeoutUnitLabels) != 2 {
		t.Fatalf("超时单位应为 2 项（分钟 / 秒），实得 %d 项", len(TimeoutUnitLabels))
	}
	if int(UnitMinutes) != 0 || int(UnitSeconds) != 1 {
		t.Fatalf("单位取值必须与 TimeoutUnitLabels 下标一致：minutes=%d seconds=%d", UnitMinutes, UnitSeconds)
	}
	if LogLevelIndex("debug") != 3 || LogLevelIndex("info") != 2 || LogLevelIndex("不存在的级别") != LogLevelIndex("info") {
		t.Fatalf("日志级别下标映射错误：debug=%d info=%d 未知=%d", LogLevelIndex("debug"), LogLevelIndex("info"), LogLevelIndex("不存在的级别"))
	}
}
