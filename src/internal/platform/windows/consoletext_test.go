//go:build windows

package windows

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func encodeUTF16(s string, order binary.ByteOrder, bom bool) []byte {
	b := []byte{}
	if bom {
		if order == binary.LittleEndian {
			b = append(b, 0xFF, 0xFE)
		} else {
			b = append(b, 0xFE, 0xFF)
		}
	}
	var buf [2]byte
	for _, u := range utf16.Encode([]rune(s)) {
		order.PutUint16(buf[:], u)
		b = append(b, buf[0], buf[1])
	}
	return b
}

// msiexec 的本地化消息是 UTF-16LE 且**不带 BOM**，原样当字符串存下来就是
// "夹 NUL 的乱码"，还会让判定层扫不到关键词。这里把这个回归钉住。
func TestDecodeConsoleTextUTF16WithoutBOM(t *testing.T) {
	const msg = "无法打开此安装程序包。与应用程序供应商确认这是一个有效的 Windows Installer 程序包。\r\n"
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		got := decodeConsoleText(encodeUTF16(msg, order, false))
		if got != msg {
			t.Fatalf("order=%v 解码结果 = %q，期望 %q", order, got, msg)
		}
		if strings.ContainsRune(got, 0) {
			t.Fatalf("order=%v 解码后不应残留 NUL", order)
		}
	}
}

func TestDecodeConsoleTextUTF16WithBOM(t *testing.T) {
	const msg = "安装成功\r\nInstallation completed successfully"
	got := decodeConsoleText(encodeUTF16(msg, binary.LittleEndian, true))
	if got != msg {
		t.Fatalf("带 BOM 的 UTF-16LE 解码结果 = %q，期望 %q", got, msg)
	}
}

// 普通 UTF-8 输出必须原样保留 —— 不能因为新增的嗅探逻辑把正常输出改坏。
func TestDecodeConsoleTextKeepsUTF8(t *testing.T) {
	for _, s := range []string{
		"安装完成，退出码 0\n",
		"plain ascii output",
		"",
		"a",
		"混合 mixed 文本 123 !@#",
	} {
		if got := decodeConsoleText([]byte(s)); got != s {
			t.Errorf("UTF-8 输出被改动：%q -> %q", s, got)
		}
	}
}

// GBK 等双字节代码页不会产生 0x00 字节，不能被误判成 UTF-16。
func TestDecodeConsoleTextDoesNotMistakeGBKForUTF16(t *testing.T) {
	gbk := []byte{0xD3, 0xD0, 0xD0, 0xA7, 0xC9, 0xFA, 0xB3, 0xC9} // "有効生成" 的 GBK 字节
	got := decodeConsoleText(gbk)
	if strings.ContainsRune(got, 0) {
		t.Fatalf("GBK 输出不应触发 UTF-16 分支：%q", got)
	}
	if sniff := sniffUTF16Order(gbk); sniff != nil {
		t.Fatalf("GBK 字节被误判为 UTF-16（order=%v）", sniff)
	}
}

// 奇数长度的 UTF-16 字节流应丢弃最后半个码元而不是 panic。
func TestDecodeConsoleTextOddLength(t *testing.T) {
	b := encodeUTF16("AB", binary.LittleEndian, false)
	b = append(b, 0x41) // 尾巴多出半个码元
	if got := decodeConsoleText(b); !strings.HasPrefix(got, "AB") {
		t.Fatalf("奇数长度解码结果 = %q", got)
	}
}
