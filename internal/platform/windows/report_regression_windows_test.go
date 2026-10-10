//go:build windows

package windows

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestD08RelocatedUnicodeShortcut(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "links")
	os.MkdirAll(dir, 0755)
	target := filepath.Join(root, "安装.exe")
	os.WriteFile(target, []byte("MZ"), 0600)
	link := filepath.Join(dir, "安装.lnk")
	b := make([]byte, 76)
	binary.LittleEndian.PutUint32(b, 76)
	copy(b[4:], []byte{1, 0x14, 2, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46})
	binary.LittleEndian.PutUint32(b[20:], 8|128)
	words, _ := syscall.UTF16FromString(`..\安装.exe`)
	words = words[:len(words)-1]
	buf := make([]byte, 2+len(words)*2)
	binary.LittleEndian.PutUint16(buf, uint16(len(words)))
	for i, w := range words {
		binary.LittleEndian.PutUint16(buf[2+i*2:], w)
	}
	os.WriteFile(link, append(b, buf...), 0600)
	relative, err := shortcutRelative(link)
	if err != nil {
		t.Fatal(err)
	}
	if got := relocatedShortcutTarget(link, `C:\nonexistent\安装.exe`, relative); got != target {
		t.Fatalf("%q", got)
	}
	got, _, err := (Opener{}).ResolveShortcut(link)
	if err != nil || got != target {
		t.Fatalf("COM target %q %v", got, err)
	}
	os.WriteFile(link, b[:30], 0600)
	if _, err = shortcutRelative(link); err == nil {
		t.Fatal("truncated link accepted")
	}
}
