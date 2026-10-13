//go:build windows

package remotetask

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var advapi = syscall.NewLazyDLL("advapi32.dll")

const maxFrame = 20 << 20

type Server struct {
	h      syscall.Handle
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func PipeName(root string) string {
	absolute, _ := filepath.Abs(root)
	hash := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(absolute))))
	return `\\.\pipe\AutoTrigger-task-` + hex.EncodeToString(hash[:16])
}
func Listen(root string, handler func(context.Context, Request) Response) (*Server, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	sid, err := user.User.Sid.String()
	if err != nil {
		return nil, err
	}
	dacl, err := syscall.UTF16PtrFromString("D:P(A;;GA;;;" + sid + ")S:(ML;;NW;;;ME)")
	if err != nil {
		return nil, err
	}
	var sd uintptr
	ok, _, err := advapi.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW").Call(uintptr(unsafe.Pointer(dacl)), 1, uintptr(unsafe.Pointer(&sd)), 0)
	if ok == 0 {
		return nil, err
	}
	defer kernel.NewProc("LocalFree").Call(sd)
	sa := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), SecurityDescriptor: sd}
	name, _ := syscall.UTF16PtrFromString(PipeName(root))
	h, _, err := kernel.NewProc("CreateNamedPipeW").Call(uintptr(unsafe.Pointer(name)), 3|0x40000000|0x80000, 8, 1, 65536, 65536, 0, uintptr(unsafe.Pointer(&sa)))
	if h == ^uintptr(0) {
		return nil, fmt.Errorf("无法启动任务接收端（可能已有执行端）：%w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{h: syscall.Handle(h), cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer syscall.CloseHandle(s.h)
		for ctx.Err() == nil {
			if err := connectPipe(ctx, s.h); err != nil {
				if ctx.Err() != nil {
					return
				}
				time.Sleep(50 * time.Millisecond)
				continue
			}
			requestCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			var request Request
			err := readFrame(requestCtx, s.h, &request)
			if err == nil {
				response := handler(requestCtx, request)
				if err = writeFrame(requestCtx, s.h, response); err == nil {
					var ack [1]byte
					_ = transfer(requestCtx, s.h, ack[:], false)
				}
			}
			cancel()
			kernel.NewProc("DisconnectNamedPipe").Call(uintptr(s.h))
		}
	}()
	return s, nil
}
func (s *Server) Close() {
	s.once.Do(func() { s.cancel(); kernel.NewProc("CancelIoEx").Call(uintptr(s.h), 0) })
	<-s.done
}

func await(ctx context.Context, h syscall.Handle, o *syscall.Overlapped, err error) (uint32, error) {
	if err != nil && err != syscall.Errno(997) {
		return 0, err
	}
	var n uint32
	for {
		result, _, e := kernel.NewProc("WaitForSingleObject").Call(uintptr(o.HEvent), 100)
		if result == 0 {
			ok, _, e := kernel.NewProc("GetOverlappedResult").Call(uintptr(h), uintptr(unsafe.Pointer(o)), uintptr(unsafe.Pointer(&n)), 0)
			if ok == 0 {
				return 0, e
			}
			return n, nil
		}
		if result != 258 {
			return 0, e
		}
		if ctx.Err() != nil {
			kernel.NewProc("CancelIoEx").Call(uintptr(h), uintptr(unsafe.Pointer(o)))
			kernel.NewProc("GetOverlappedResult").Call(uintptr(h), uintptr(unsafe.Pointer(o)), uintptr(unsafe.Pointer(&n)), 1)
			return 0, ctx.Err()
		}
	}
}
func event() (*syscall.Overlapped, error) {
	h, _, err := kernel.NewProc("CreateEventW").Call(0, 1, 0, 0)
	if h == 0 {
		return nil, err
	}
	return &syscall.Overlapped{HEvent: syscall.Handle(h)}, nil
}
func connectPipe(ctx context.Context, h syscall.Handle) error {
	o, err := event()
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(o.HEvent)
	ok, _, e := kernel.NewProc("ConnectNamedPipe").Call(uintptr(h), uintptr(unsafe.Pointer(o)))
	if ok != 0 || e == syscall.Errno(535) {
		return nil
	}
	_, err = await(ctx, h, o, e)
	return err
}
func transfer(ctx context.Context, h syscall.Handle, b []byte, write bool) error {
	for len(b) > 0 {
		o, err := event()
		if err != nil {
			return err
		}
		var n uint32
		if write {
			err = syscall.WriteFile(h, b, &n, o)
		} else {
			err = syscall.ReadFile(h, b, &n, o)
		}
		if err == syscall.Errno(997) {
			n, err = await(ctx, h, o, err)
		}
		runtime.KeepAlive(b)
		syscall.CloseHandle(o.HEvent)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("任务通信连接已关闭")
		}
		b = b[n:]
	}
	return nil
}
func writeFrame(ctx context.Context, h syscall.Handle, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > maxFrame {
		return fmt.Errorf("任务响应过大")
	}
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(b)))
	if err = transfer(ctx, h, size[:], true); err != nil {
		return err
	}
	return transfer(ctx, h, b, true)
}
func readFrame(ctx context.Context, h syscall.Handle, v interface{}) error {
	var size [4]byte
	if err := transfer(ctx, h, size[:], false); err != nil {
		return err
	}
	n := binary.LittleEndian.Uint32(size[:])
	if n == 0 || n > maxFrame {
		return fmt.Errorf("任务消息长度无效")
	}
	b := make([]byte, n)
	if err := transfer(ctx, h, b, false); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func Call(ctx context.Context, root string, r Request) (Response, error) {
	name, _ := syscall.UTF16PtrFromString(PipeName(root))
	var h syscall.Handle
	var err error
	for {
		h, err = syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, 0x40000000|0x100000, 0)
		if err == nil {
			break
		}
		if err != syscall.Errno(231) {
			return Response{}, fmt.Errorf("桌面任务接收端未开启、未运行或账户无权连接：%w", err)
		}
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	defer syscall.CloseHandle(h)
	if err = writeFrame(ctx, h, r); err != nil {
		return Response{}, err
	}
	var response Response
	err = readFrame(ctx, h, &response)
	if err == nil {
		_ = transfer(ctx, h, []byte{1}, true)
	}
	return response, err
}
func openSnapshot(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, 0x00200000, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
