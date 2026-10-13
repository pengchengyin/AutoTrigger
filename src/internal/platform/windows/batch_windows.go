//go:build windows

package windows

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var batchSourceDir = regexp.MustCompile(`(?i)%~dp0`)

func normalizeBatch(path, temp string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var head [2]byte
	n, _ := f.ReadAt(head[:], 0)
	if n != 2 || (head != [2]byte{0xff, 0xfe} && head != [2]byte{0xfe, 0xff}) {
		return path, nil
	}
	b, err := io.ReadAll(io.LimitReader(f, 8*1024*1024+1))
	if err != nil {
		return "", err
	}
	if len(b) > 8*1024*1024 || len(b)%2 != 0 {
		return "", fmt.Errorf("UTF-16 batch is oversized or truncated")
	}
	var order binary.ByteOrder = binary.LittleEndian
	if head[0] == 0xfe {
		order = binary.BigEndian
	}
	words := make([]uint16, (len(b)-2)/2)
	for i := range words {
		words[i] = order.Uint16(b[2+i*2:])
	}
	text := string(utf16.Decode(words))
	if strings.ContainsRune(text, 0) {
		return "", fmt.Errorf("UTF-16 batch contains NUL")
	}
	dir := strings.ReplaceAll(filepath.Dir(path)+string(filepath.Separator), "%", "%%")
	text = batchSourceDir.ReplaceAllStringFunc(text, func(string) string { return dir })
	words, err = syscall.UTF16FromString(text)
	if err != nil {
		return "", err
	}
	words = words[:len(words)-1]
	if len(words) == 0 {
		return "", fmt.Errorf("UTF-16 batch contains no commands")
	}
	var substituted int32
	proc := kernel32.NewProc("WideCharToMultiByte")
	size, _, _ := proc.Call(1, 0x400, uintptr(unsafe.Pointer(&words[0])), uintptr(len(words)), 0, 0, 0, uintptr(unsafe.Pointer(&substituted)))
	if size == 0 || substituted != 0 {
		return "", fmt.Errorf("UTF-16 batch cannot be represented in the system OEM code page")
	}
	out := make([]byte, int(size))
	r, _, _ := proc.Call(1, 0x400, uintptr(unsafe.Pointer(&words[0])), uintptr(len(words)), uintptr(unsafe.Pointer(&out[0])), size, 0, uintptr(unsafe.Pointer(&substituted)))
	if r == 0 || substituted != 0 {
		return "", fmt.Errorf("UTF-16 batch conversion would corrupt characters")
	}
	name := filepath.Join(temp, "normalized"+strings.ToLower(filepath.Ext(path)))
	if err = os.WriteFile(name, out, 0600); err != nil {
		return "", err
	}
	return name, nil
}
