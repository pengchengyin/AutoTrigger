package core

import (
	"encoding/binary"
	"io"
	"os"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// installerLogTailBytes 是读取安装器详细日志时保留的尾部大小。
// 失败原因总在日志尾部（最后失败的动作 + 返回码），取太大只是浪费内存。
const installerLogTailBytes = 128 << 10

// loadInstallerLog 读取安装器详细日志并转成 UTF-8 文本。
//
// 为什么不直接 string(data)：msiexec 的 `/L*v` 日志是 **UTF-16LE（BOM FF FE）**。
// 按 UTF-8 直接转字符串会得到"每个字符夹一个 NUL"的乱码，后果有两个：
//  1. 判定层的 B1 证据（扫描日志里的 "installation completed successfully"、"安装成功"
//     等关键词）**永远匹配不上** —— 等于 MSI 这条最主流的路径白丢一整类证据；
//  2. 结果 JSON 的 `attempts[].stdout_summary` 里塞满 NUL 与乱码，人不可读。
//
// 因此按 BOM 判定编码（无 BOM 按 UTF-8）。只读尾部 tail 字节以适应大日志，
// 且 UTF-16 的截断点必须落在**偶数边界**，否则最后一个码元会被截成半个。
func loadInstallerLog(path string, tail int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	order := probeUTF16Order(f)
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	size := st.Size()
	off := int64(0)
	if order != nil {
		off = 2 // 跳过 BOM
	}
	if size-off > tail {
		off = size - tail
		if order != nil && off%2 != 0 {
			off++
		}
	}
	if size <= off {
		return ""
	}
	buf := make([]byte, size-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return ""
	}
	if order == nil {
		return strings.ToValidUTF8(string(buf), "\uFFFD")
	}
	units := make([]uint16, 0, len(buf)/2)
	for i := 0; i+1 < len(buf); i += 2 {
		units = append(units, order.Uint16(buf[i:]))
	}
	return string(utf16.Decode(units))
}

// probeUTF16Order 读前两字节判定是否有 UTF-16 BOM；无 BOM 返回 nil（按 UTF-8 处理）。
// 用 ReadAt 定位读取，不依赖文件句柄的当前位置。
func probeUTF16Order(f *os.File) binary.ByteOrder {
	var head [2]byte
	if n, _ := io.ReadFull(f, head[:]); n < 2 {
		return nil
	}
	switch {
	case head[0] == 0xFF && head[1] == 0xFE:
		return binary.LittleEndian
	case head[0] == 0xFE && head[1] == 0xFF:
		return binary.BigEndian
	}
	return nil
}

// msiFailureHint 从 MSI 详细日志里挑出最有诊断价值的一到两行。
//
// 1603 这类码本身不说明问题：真正的原因写在日志里的"哪个动作返回 3"与"Error xxxx"上。
// 摘出来直接打进日志，现场就不必再去翻整个日志文件。
func msiFailureHint(text string) string {
	var failingAction, errorLine string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.Contains(line, "Return value 3."):
			failingAction = line
		case hasMSIErrorCode(line):
			errorLine = line
		}
	}
	parts := make([]string, 0, 2)
	if errorLine != "" {
		parts = append(parts, clipRunes(errorLine, 200))
	}
	if failingAction != "" {
		parts = append(parts, clipRunes(failingAction, 200))
	}
	return strings.Join(parts, " ｜ ")
}

// hasMSIErrorCode 判断一行是否是 MSI 的错误行（"Error " 后紧跟数字，如 Error 1603）。
func hasMSIErrorCode(line string) bool {
	const marker = "Error "
	i := strings.Index(line, marker)
	if i < 0 {
		return false
	}
	rest := line[i+len(marker):]
	return rest != "" && rest[0] >= '0' && rest[0] <= '9'
}

// clipRunes 按字符（而非字节）截断，避免把中文切坏。
func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
