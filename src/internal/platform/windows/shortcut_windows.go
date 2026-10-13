//go:build windows

package windows

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// IShellLink has no GetRelativePath; read the bounded MS-SHLLINK StringData.
// https://learn.microsoft.com/openspecs/windows_protocols/ms-shllink/17b69472-0f34-4bcf-b290-eccdb8de224b
func shortcutRelative(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4*1024*1024+1))
	if err != nil {
		return "", err
	}
	bad := fmt.Errorf("invalid shortcut StringData")
	if len(b) < 76 || len(b) > 4*1024*1024 || binary.LittleEndian.Uint32(b) != 76 {
		return "", bad
	}
	flags := binary.LittleEndian.Uint32(b[20:24])
	if flags&8 == 0 {
		return "", nil
	}
	at := 76
	if flags&1 != 0 {
		if at+2 > len(b) {
			return "", bad
		}
		at += 2 + int(binary.LittleEndian.Uint16(b[at:]))
	}
	if flags&2 != 0 {
		if at+4 > len(b) {
			return "", bad
		}
		size := int(binary.LittleEndian.Uint32(b[at:]))
		if size < 28 || size > len(b)-at {
			return "", bad
		}
		at += size
	}
	for _, bit := range []uint32{4, 8} {
		if flags&bit == 0 {
			continue
		}
		if at+2 > len(b) {
			return "", bad
		}
		n := int(binary.LittleEndian.Uint16(b[at:]))
		at += 2
		bytes := n
		if flags&128 != 0 {
			bytes *= 2
		}
		if bytes > len(b)-at {
			return "", bad
		}
		data := b[at : at+bytes]
		at += bytes
		if bit != 8 {
			continue
		}
		if flags&128 != 0 {
			words := make([]uint16, n)
			for i := range words {
				words[i] = binary.LittleEndian.Uint16(data[i*2:])
			}
			return syscall.UTF16ToString(words), nil
		}
		if n == 0 {
			return "", nil
		}
		words := make([]uint16, n*2+1)
		r, _, _ := kernel32.NewProc("MultiByteToWideChar").Call(0, 0, uintptr(unsafe.Pointer(&data[0])), uintptr(n), uintptr(unsafe.Pointer(&words[0])), uintptr(len(words)))
		if r == 0 {
			return "", bad
		}
		return syscall.UTF16ToString(words[:int(r)]), nil
	}
	return "", nil
}

func relocatedShortcutTarget(link, original, relative string) string {
	if original != "" {
		if s, err := os.Stat(original); err == nil && !s.IsDir() {
			return original
		}
	}
	if relative != "" && !filepath.IsAbs(relative) && filepath.VolumeName(relative) == "" {
		candidate := filepath.Clean(filepath.Join(filepath.Dir(link), relative))
		if s, err := os.Stat(candidate); err == nil && !s.IsDir() {
			return candidate
		}
	}
	return original
}
