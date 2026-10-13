package core

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// utf16Bytes 按指定字节序把字符串编成带 BOM 的 UTF-16 字节流，
// 用来复现 msiexec /L*v 的真实产物（实测为 UTF-16LE + BOM FF FE）。
func utf16Bytes(s string, le bool) []byte {
	b := []byte{}
	if le {
		b = append(b, 0xFF, 0xFE)
	} else {
		b = append(b, 0xFE, 0xFF)
	}
	var order binary.ByteOrder = binary.LittleEndian
	if !le {
		order = binary.BigEndian
	}
	var buf [2]byte
	for _, u := range utf16.Encode([]rune(s)) {
		order.PutUint16(buf[:], u)
		b = append(b, buf[0], buf[1])
	}
	return b
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// MSI 详细日志是 UTF-16LE：按 UTF-8 直接转字符串会得到"夹 NUL 的乱码"，
// 导致 B1（安装器日志关键词）证据永远匹配不上。这里把这个回归钉住。
func TestLoadInstallerLogDecodesUTF16(t *testing.T) {
	const body = "=== Verbose logging started ===\r\nAction ended: InstallFinalize. Return value 3.\r\ninstallation completed successfully\r\n"
	for _, le := range []bool{true, false} {
		p := writeTemp(t, "MSI-0.log", utf16Bytes(body, le))
		got := loadInstallerLog(p, installerLogTailBytes)
		if !strings.Contains(got, "installation completed successfully") {
			t.Fatalf("le=%t：解码后应能匹配到关键词，实得 %q", le, got)
		}
		if !strings.Contains(got, "Return value 3.") {
			t.Fatalf("le=%t：解码后应保留失败动作行，实得 %q", le, got)
		}
		if strings.ContainsRune(got, 0) {
			t.Fatalf("le=%t：解码后不应残留 NUL", le)
		}
	}
	if !strings.Contains(loadInstallerLog(writeTemp(t, "a.log", utf16Bytes("安装成功", true)), 1<<10), "安装成功") {
		t.Fatal("UTF-16LE 中文未正确解码")
	}
}

// 无 BOM 的日志（非 MSI 的安装器日志）按 UTF-8 处理，不做任何转换。
func TestLoadInstallerLogPlainUTF8(t *testing.T) {
	p := writeTemp(t, "plain.log", []byte("Installation completed successfully\n"))
	got := loadInstallerLog(p, installerLogTailBytes)
	if !strings.Contains(got, "Installation completed successfully") {
		t.Fatalf("无 BOM 时应按 UTF-8 原样读取，实得 %q", got)
	}
}

// 只读尾部时要落在 UTF-16 码元的偶数边界，否则最后一个字符会被截成半个。
func TestLoadInstallerLogTailAlignment(t *testing.T) {
	head := strings.Repeat("A", 4000)
	p := writeTemp(t, "big.log", utf16Bytes(head+"TAILMARK\r\n", true))
	got := loadInstallerLog(p, 64) // 故意取一个奇数也能除尽的尾部
	if !strings.Contains(got, "TAILMARK") {
		t.Fatalf("尾部截断后应仍能读到结尾内容，实得 %q", got)
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("偶数边界对齐失败，出现了替换字符：%q", got)
	}
}

func TestLoadInstallerLogMissingFile(t *testing.T) {
	if got := loadInstallerLog(filepath.Join(t.TempDir(), "nope.log"), 1<<10); got != "" {
		t.Fatalf("文件不存在应返回空串，实得 %q", got)
	}
}

// 1603 这类通用码只有对着日志才有意义；要把"哪个动作失败""报了什么错"摘出来。
func TestMSIFailureHint(t *testing.T) {
	log := strings.Join([]string{
		"MSI (s) (40:E4) [01:38:38:100]: Note: 1: 2262 2: RemoveFiles ",
		"Action start 1:38:38: InstallFinalize.",
		"MSI (s) (40:E4) [01:38:50:200]: Product: SoloMD -- Error 1603. 安装时发生严重错误",
		"Action ended 1:38:50: InstallFinalize. Return value 3.",
	}, "\r\n")
	hint := msiFailureHint(log)
	if !strings.Contains(hint, "Error 1603") {
		t.Fatalf("应摘出 Error 行，实得 %q", hint)
	}
	if !strings.Contains(hint, "Return value 3.") {
		t.Fatalf("应摘出失败动作行，实得 %q", hint)
	}
	if got := msiFailureHint("nothing wrong here\n"); got != "" {
		t.Fatalf("日志正常时不应编造要点，实得 %q", got)
	}
}

// "Error " 后面必须紧跟数字才算错误行，避免把 "Error handling disabled" 之类当错误。
func TestHasMSIErrorCode(t *testing.T) {
	cases := map[string]bool{
		"Product: X -- Error 1603.":     true,
		"Error 1925: 权限不足":              true,
		"Error handling disabled":       false,
		"No error here":                 false,
		"Action ended. Return value 3.": false,
	}
	for line, want := range cases {
		if got := hasMSIErrorCode(line); got != want {
			t.Errorf("hasMSIErrorCode(%q) = %t，期望 %t", line, got, want)
		}
	}
}

func TestClipRunesKeepsChineseIntact(t *testing.T) {
	if got := clipRunes("安装过程中发生致命错误", 4); got != "安装过程…" {
		t.Fatalf("按字符截断结果 = %q", got)
	}
	if got := clipRunes("abc", 10); got != "abc" {
		t.Fatalf("未超长应原样返回，实得 %q", got)
	}
}
