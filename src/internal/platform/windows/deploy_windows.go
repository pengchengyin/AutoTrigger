//go:build windows

package windows

import (
	"autotrigger/internal/archive"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/procutil"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func (p *Prereqs) deploySevenZip(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	unlock, err := (MachineLock{}).Acquire(ctx, 2*time.Minute)
	if err != nil {
		return "", err
	}
	defer unlock()
	target := filepath.Join(p.thirdPartyDir(), "7zip", "7-Zip")
	exe := filepath.Join(target, "7z.exe")
	if findExecutable([]string{exe}, p.dataBase()) != "" {
		return exe, nil
	}
	packages, _ := filepath.Glob(filepath.Join(p.thirdPartyDir(), "7zip", "7z*-x64.exe"))
	if len(packages) == 0 {
		return "", fmt.Errorf("缺少 %s 离线安装包；不会联网下载", filepath.Join(p.thirdPartyDir(), "7zip", "7z*-x64.exe"))
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return "", err
	}
	installer := packages[len(packages)-1]
	_, err = procutil.Run(ctx, p.Runner, ports.Command{Program: installer, Args: []string{"/S", "/D=" + target}, CommandLine: syscall.EscapeArg(installer) + " /S /D=" + target})
	if errors.Is(err, syscall.Errno(740)) {
		err = runElevatedDependency(ctx, installer, "/S /D="+target)
	}
	if err != nil {
		return "", err
	}
	if findExecutable([]string{exe}, p.dataBase()) == "" {
		return "", fmt.Errorf("安装器结束但没有生成 %s", exe)
	}
	return exe, nil
}

func (p *Prereqs) deployOCR(ctx context.Context, engine, sevenZip string, limit int64) (string, error) {
	stem := "PaddleOCR-json"
	if engine == "rapidocr_json" {
		stem = "RapidOCR-json"
	}
	root := filepath.Join(p.thirdPartyDir(), "ocr")
	target := filepath.Join(root, stem)
	exe := filepath.Join(target, stem+".exe")
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if findExecutable([]string{exe}, p.dataBase()) != "" {
		return exe, nil
	}
	packages, _ := filepath.Glob(filepath.Join(root, stem+"*.7z"))
	if len(packages) == 0 {
		return "", fmt.Errorf("缺少 %s 离线压缩包；不会联网下载", stem)
	}
	return p.deployOCRArchive(ctx, packages[len(packages)-1], target, stem+".exe", sevenZip, limit, true)
}

func (p *Prereqs) deployOCRArchive(ctx context.Context, packagePath, target, executable, sevenZip string, limit int64, flatten bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	unlock, err := (MachineLock{}).Acquire(ctx, 2*time.Minute)
	if err != nil {
		return "", err
	}
	defer unlock()
	root := filepath.Join(p.thirdPartyDir(), "ocr")
	if paths := localOCRExecutablesIn(target, executable); len(paths) == 1 {
		return paths[0], nil
	}
	stage, err := os.MkdirTemp(root, ".deploy-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	backend := archive.SevenZip{Path: sevenZip, Runner: p.Runner}
	entries, err := backend.List(ctx, packagePath)
	if err != nil {
		return "", err
	}
	if err = archive.Size(entries, limit); err != nil {
		return "", err
	}
	if err = backend.Extract(ctx, packagePath, stage, limit); err != nil {
		return "", err
	}
	source := ""
	err = filepath.WalkDir(stage, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), executable) {
			if source != "" {
				return fmt.Errorf("引擎包含多个 %s，无法确定部署目录", executable)
			}
			source = filepath.Dir(path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if source == "" {
		return "", fmt.Errorf("解压后未找到 %s", executable)
	}
	exeRelative, err := filepath.Rel(stage, filepath.Join(source, executable))
	if err != nil {
		return "", err
	}
	if flatten && source != stage {
		children, err := os.ReadDir(stage)
		if err != nil {
			return "", err
		}
		if len(children) != 1 || !children[0].IsDir() || source != filepath.Join(stage, children[0].Name()) {
			return "", fmt.Errorf("OCR 发行包目录结构不明确，拒绝丢弃引擎目录以外的模型或 DLL")
		}
	}
	if !flatten {
		source = stage
	} else {
		exeRelative = executable
	}
	// Preserve any existing installation instead of overwriting user files.
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("%s 已存在但引擎不完整，请修复该目录后重试", target)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", err
	}
	if err = os.Rename(source, target); err != nil {
		return "", err
	}
	return filepath.Join(target, exeRelative), nil
}

// Installer manifests can require UAC even for a tool-local installation.
func runElevatedDependency(ctx context.Context, program, args string) error {
	info := struct {
		Size, Mask                        uint32
		Window                            uintptr
		Verb, File, Parameters, Directory *uint16
		Show                              int32
		Instance, IDList                  uintptr
		Class                             *uint16
		ClassKey                          uintptr
		HotKey                            uint32
		Icon, Process                     uintptr
	}{Mask: 0x40 | 0x400, Verb: utf("runas"), File: utf(program), Parameters: utf(args), Directory: utf(filepath.Dir(program))}
	info.Size = uint32(unsafe.Sizeof(info))
	if err := callOK(syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteExW"), uintptr(unsafe.Pointer(&info))); err != nil {
		return fmt.Errorf("7-Zip 安装需要管理员批准：%w", err)
	}
	if info.Process == 0 {
		return fmt.Errorf("未取得 7-Zip 安装进程句柄")
	}
	defer syscall.CloseHandle(syscall.Handle(info.Process))
	for {
		result, _, err := kernel32.NewProc("WaitForSingleObject").Call(info.Process, 100)
		if result == 0 {
			var code uint32
			if err := callOK(kernel32.NewProc("GetExitCodeProcess"), info.Process, uintptr(unsafe.Pointer(&code))); err != nil {
				return err
			}
			if code != 0 {
				return fmt.Errorf("7-Zip 安装返回退出码 %d", code)
			}
			return nil
		}
		if result == 0xffffffff {
			return err
		}
		if ctx.Err() != nil {
			kernel32.NewProc("TerminateProcess").Call(info.Process, 1)
			return ctx.Err()
		}
	}
}
