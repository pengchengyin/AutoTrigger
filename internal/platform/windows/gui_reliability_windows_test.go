//go:build windows

package windows

import (
	"os"
	"testing"
	"unsafe"
)

func TestGUIIntegrityQuery(t *testing.T) {
	if got := processIntegrityRID(os.Getpid()); got < 0x1000 || got > 0x5000 {
		t.Fatalf("current process integrity query failed: %#x", got)
	}
	if got := processIntegrityRID(-1); got != 0 {
		t.Fatalf("invalid process integrity should be unknown: %#x", got)
	}
}

func TestMouseInputMatchesWindowsX64ABI(t *testing.T) {
	if got := unsafe.Sizeof(mouseInput{}); got != 40 {
		t.Fatalf("INPUT structure size: %d", got)
	}
}
