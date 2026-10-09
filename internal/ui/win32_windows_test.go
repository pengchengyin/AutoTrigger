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
func TestCtlColorSchemeKeepsLogOpaque(t *testing.T) {
	bk, fg, brush := ctlColorScheme(true)
	if bk != opaqueBk {
		t.Errorf("日志区背景模式 = %d，期望不透明(%d)", bk, opaqueBk)
	}
	if fg != colorWindowText || brush != colorWindow {
		t.Errorf("日志区配色 = (文字 %d, 画刷 %d)，期望 (%d, %d)", fg, brush, colorWindowText, colorWindow)
	}
	if bk, fg, brush = ctlColorScheme(false); bk != transparentBk || fg != colorBtnText || brush != color3DFace {
		t.Errorf("标签/按钮配色 = (%d, %d, %d)，期望 (%d, %d, %d)", bk, fg, brush, transparentBk, colorBtnText, color3DFace)
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
