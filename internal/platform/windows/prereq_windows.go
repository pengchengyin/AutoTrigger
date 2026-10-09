//go:build windows

package windows

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/ocr"
	"autotrigger/internal/platform/windows/winver"
	"autotrigger/internal/procutil"
	"autotrigger/pkg/schema"
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type Prereqs struct {
	Runner ports.ProcessRunner
	Engine *ocr.Engine
	ExeDir string
}

func (p *Prereqs) Check(ctx context.Context, c config.Config) (schema.Prereq, map[string]interface{}, error) {
	report := schema.EmptyPrereq()
	v := winver.Current()
	avx, avx2 := winver.AVX()
	awareness := winver.EnableDPI()
	admin, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("IsUserAnAdmin").Call()
	meta := map[string]interface{}{"os_version": v.String(), "service_pack": v.ServicePack, "cpu_avx": avx, "cpu_avx2": avx2, "dpi_awareness": awareness, "administrator": admin != 0, "architecture": "amd64"}
	if v.Major < 6 || (v.Major == 6 && v.Minor < 1) || (v.Major == 6 && v.Minor == 1 && v.ServicePack < 1) {
		report.Issues = append(report.Issues, "系统版本过低：需要 Windows 7 SP1 x64 或更高版本")
	}
	if admin == 0 {
		report.Issues = append(report.Issues, "建议以管理员身份运行：安装动作与 HKLM 采证在标准权限下可能不完整")
	}
	candidates := []string{}
	if c.Archive.SevenZipPath != "auto" {
		candidates = append(candidates, c.Archive.SevenZipPath)
	} else {
		for _, key := range []string{"SOFTWARE\\7-Zip", "SOFTWARE\\WOW6432Node\\7-Zip"} {
			h, err := regOpen(syscall.HKEY_LOCAL_MACHINE, key)
			if err == nil {
				candidates = append(candidates, filepath.Join(regString(h, "Path"), "7z.exe"))
				syscall.RegCloseKey(h)
			}
		}
		if path, err := exec.LookPath("7z.exe"); err == nil {
			candidates = append(candidates, path)
		}
		candidates = append(candidates, `C:\Program Files\7-Zip\7z.exe`, `C:\Program Files (x86)\7-Zip\7z.exe`)
	}
	if path := findExecutable(candidates, p.ExeDir); path != "" {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		result, err := procutil.Run(probeCtx, p.Runner, ports.Command{Program: path, Args: []string{"i"}})
		cancel()
		if err == nil {
			report.ArchiveBackend = schema.String(path)
			report.ArchiveBackendStatus = "ok"
			rx := regexp.MustCompile(`7-Zip(?: \([^)]*\))? ([0-9]+\.[0-9]+)`)
			if m := rx.FindStringSubmatch(result.Stdout); len(m) > 1 {
				report.SevenZipVersion = schema.String(m[1])
				if versionLess(m[1], c.Archive.MinVersion) {
					report.ArchiveBackendStatus = "version_too_old"
					report.Issues = append(report.Issues, "7-Zip version below "+c.Archive.MinVersion)
				}
			}
		} else {
			report.Issues = append(report.Issues, "7-Zip capability check failed: "+err.Error())
		}
	}
	if report.ArchiveBackend == nil {
		report.Issues = append(report.Issues, "7-Zip missing: install third_party\\7zip\\7z*-x64.exe /S, or msiexec /i 7z*-x64.msi /quiet /norestart; bypass with --no-archive")
	}
	if v.NativeMount() {
		report.MountBackend = schema.String("system")
		report.MountBackendStatus = "ok"
	} else {
		report.MountBackendStatus = "unsupported"
		report.Issues = append(report.Issues, "Win7 环境：光盘映像请走解压轨；可选的 WinCDEmu 挂载适配器尚未验证")
	}
	if c.OCR.Enabled && c.OCR.Engine != "none" {
		engine := c.OCR.Engine
		names := map[string]string{"paddleocr_json": "PaddleOCR-json.exe", "rapidocr_json": "RapidOCR-json.exe", "umi_ocr": "Umi-OCR.exe", "tesseract": "tesseract.exe"}
		if engine == "paddleocr_json" && !avx {
			report.Issues = append(report.Issues, "CPU 未启用 AVX：若部署了 RapidOCR-json 将优先使用它")
			engine = "rapidocr_json"
		}
		exe := names[engine]
		paths := []string{}
		if c.OCR.EnginePath != "auto" {
			paths = append(paths, c.OCR.EnginePath)
		} else {
			if path, err := exec.LookPath(exe); err == nil {
				paths = append(paths, path)
			}
			stem := strings.TrimSuffix(exe, ".exe")
			paths = append(paths, filepath.Join(`C:\tools`, stem, exe), filepath.Join(`D:\tools`, stem, exe), filepath.Join(`C:\Program Files`, stem, exe), filepath.Join(`C:\Program Files\Tesseract-OCR`, exe))
		}
		path := findExecutable(paths, p.ExeDir)
		report.OCREngine = engine
		report.OCREngineStatus = "missing"
		if path != "" || engine == "umi_ocr" {
			ocrConfig := c.OCR
			ocrConfig.Engine = engine
			e := &ocr.Engine{Config: ocrConfig, Path: path, Runner: p.Runner}
			info, err := e.Start(ctx)
			if err == nil {
				test := image.NewRGBA(image.Rect(0, 0, 96, 32))
				for y := 0; y < 32; y++ {
					for x := 0; x < 96; x++ {
						test.Set(x, y, color.White)
					}
				}
				_, err = e.Recognize(ctx, test)
			}
			if err == nil {
				p.Engine = e
				report.OCREnginePath = schema.String(path)
				report.OCREngineStatus = "ok"
				report.OCREngineVersion = schema.String(info.Version)
				report.OCREngineLanguages = info.Languages
				report.OCRStartupMS = info.StartupMS
			} else {
				e.Stop()
				report.OCREngineStatus = "startup_failed"
				report.Issues = append(report.Issues, "OCR 能力自检失败："+err.Error())
			}
		}
		if report.OCREngineStatus != "ok" {
			report.Issues = append(report.Issues, "未检测到可用 OCR 引擎：请部署 third_party\\ocr\\PaddleOCR-json 或 RapidOCR-json；当前退回 UIA/Win32 + 模板；可用 --no-ocr 显式跳过")
		}
	}
	report.OK = (!c.Archive.RequireSevenZip || report.ArchiveBackend != nil) && (!c.OCR.RequireOCR || !c.OCR.Enabled || report.OCREngineStatus == "ok")
	return report, meta, nil
}
func findExecutable(paths []string, root string) string {
	for _, path := range paths {
		if path == "" {
			continue
		}
		path = config.ExpandEnv(path)
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if s, err := os.Stat(path); err == nil && !s.IsDir() {
			return path
		}
	}
	return ""
}
func versionLess(a, b string) bool {
	parse := func(v string) []int {
		out := []int{}
		for _, s := range strings.Split(v, ".") {
			n, _ := strconv.Atoi(s)
			out = append(out, n)
		}
		return out
	}
	x, y := parse(a), parse(b)
	for i := 0; i < len(x) || i < len(y); i++ {
		u, v := 0, 0
		if i < len(x) {
			u = x[i]
		}
		if i < len(y) {
			v = y[i]
		}
		if u != v {
			return u < v
		}
	}
	return false
}

func FileVersion(path string) string {
	versionDLL := syscall.NewLazyDLL("version.dll")
	size, _, _ := versionDLL.NewProc("GetFileVersionInfoSizeW").Call(uintptr(unsafe.Pointer(utf(path))), 0)
	if size == 0 {
		return "unknown"
	}
	buf := make([]byte, size)
	if callOK(versionDLL.NewProc("GetFileVersionInfoW"), uintptr(unsafe.Pointer(utf(path))), 0, size, uintptr(unsafe.Pointer(&buf[0]))) != nil {
		return "unknown"
	}
	var p uintptr
	var n uint32
	r, _, _ := versionDLL.NewProc("VerQueryValueW").Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(utf("\\"))), uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&n)))
	if r == 0 || n < 16 {
		return "unknown"
	}
	words := unsafe.Slice((*uint32)(nativeAddress(p)), 4)
	return fmt.Sprintf("%d.%d.%d.%d", words[2]>>16, words[2]&0xffff, words[3]>>16, words[3]&0xffff)
}
