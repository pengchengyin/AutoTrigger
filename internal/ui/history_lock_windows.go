//go:build windows

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

func acquireHistoryLock(dir string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(dir, "index.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	var overlapped syscall.Overlapped
	dll := syscall.NewLazyDLL("kernel32.dll")
	deadline := time.Now().Add(2 * time.Second)
	for {
		ok, _, err := dll.NewProc("LockFileEx").Call(file.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
		if ok != 0 {
			return func() {
				dll.NewProc("UnlockFileEx").Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
				file.Close()
			}, nil
		}
		if err != syscall.Errno(33) || time.Now().After(deadline) {
			file.Close()
			return nil, fmt.Errorf("历史索引被占用或无法写入：%w", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
