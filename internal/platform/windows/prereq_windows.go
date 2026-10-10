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
	return p.check(ctx, c, true)
}

func (p *Prereqs) CheckOnly(ctx context.Context, c config.Config) (schema.Prereq, map[string]interface{}, error) {
	return p.check(ctx, c, false)
}

func (p *Prereqs) check(ctx context.Context, c config.Config, deploy bool) (schema.Prereq, map[string]interface{}, error) {
	return p.checkEnvironment(ctx, c, deploy, false)
}

func (p *Prereqs) CheckEnvironment(ctx context.Context, c config.Config, deploy bool) (schema.Prereq, map[string]interface{}, error) {
	return p.checkEnvironment(ctx, c, deploy, true)
}

func (p *Prereqs) checkEnvironment(ctx context.Context, c config.Config, deploy, inventory bool) (schema.Prereq, map[string]interface{}, error) {
	if p.Engine != nil {
		p.Engine.Stop()
		p.Engine = nil
	}
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
		report.Issues = append(report.Issues, "建议以管理员身份运行：当前进程未提权，即使账户属于 Administrators 也可能是标准权限。系统级安装与 HKLM 采证可能不完整；权限拒绝需在管理员会话复测，不能据此推断安装器或工具有缺陷。UAC 处理见 README 使用说明")
	}
	candidates := []string{}
	if c.Archive.SevenZipPath != "auto" {
		candidates = append(candidates, c.Archive.SevenZipPath)
	} else {
		candidates = append(candidates, filepath.Join("third_party", "7zip", "7-Zip", "7z.exe"))
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
	zipPath := findExecutable(candidates, p.ExeDir)
	if zipPath == "" && deploy && (c.Archive.RequireSevenZip || (c.OCR.Enabled && c.OCR.Engine != "none")) {
		var err error
		zipPath, err = p.deploySevenZip(ctx)
		if err != nil {
			report.Issues = append(report.Issues, "7-Zip 自动部署失败："+err.Error())
		} else {
			report.Issues = append(report.Issues, "已自动部署 7-Zip 到工具目录："+zipPath)
		}
	}
	if path := zipPath; path != "" {
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
	if inventory {
		report.OCREngines = p.inventoryOCR(ctx, c, deploy, avx, &report)
	}
	if c.OCR.Enabled && c.OCR.Engine != "none" {
		engine := c.OCR.Engine
		names := map[string]string{"paddleocr_json": "PaddleOCR-json.exe", "rapidocr_json": "RapidOCR-json.exe", "umi_ocr": "Umi-OCR.exe", "tesseract": "tesseract.exe"}
		if engine == "windows" {
			report.Issues = append(report.Issues, "Windows OCR 适配尚未实现，请选择其他受支持的引擎")
		} else if engine == "paddleocr_json" && !avx {
			report.Issues = append(report.Issues, "CPU 未启用 AVX：若部署了 RapidOCR-json 将优先使用它")
			engine = "rapidocr_json"
		}
		exe := names[engine]
		paths := []string{}
		if c.OCR.EnginePath != "auto" && engine == c.OCR.Engine {
			paths = append(paths, c.OCR.EnginePath)
		} else {
			stem := strings.TrimSuffix(exe, ".exe")
			paths = append(paths, filepath.Join("third_party", "ocr", stem, exe), filepath.Join("third_party", "ocr", exe))
			paths = append(paths, localOCRExecutables(p.ExeDir, exe)...)
			if path, err := exec.LookPath(exe); err == nil {
				paths = append(paths, path)
			}
			paths = append(paths, filepath.Join(`C:\tools`, stem, exe), filepath.Join(`D:\tools`, stem, exe), filepath.Join(`C:\Program Files`, stem, exe), filepath.Join(`C:\Program Files\Tesseract-OCR`, exe))
		}
		path := findExecutable(paths, p.ExeDir)
		if path == "" && deploy && (engine == "paddleocr_json" || engine == "rapidocr_json") {
			if report.ArchiveBackend == nil {
				report.Issues = append(report.Issues, "无法自动部署 OCR：需要先准备可用的 7-Zip")
			} else {
				var err error
				path, err = p.deployOCR(ctx, engine, *report.ArchiveBackend, c.Archive.MaxExtractSizeBytes)
				if err != nil {
					report.Issues = append(report.Issues, "OCR 自动部署失败："+err.Error())
				} else {
					report.Issues = append(report.Issues, "已自动部署 OCR 到工具目录："+path)
				}
			}
		}
		report.OCREngine = engine
		report.OCREngineStatus = "missing"
		if path != "" {
			report.OCREnginePath = schema.String(path)
		}
		if engine == "windows" || (engine == "paddleocr_json" && !avx) {
			report.OCREngineStatus = "unsupported"
		} else if path != "" || engine == "umi_ocr" {
			ocrConfig := c.OCR
			ocrConfig.Engine = engine
			e := &ocr.Engine{Config: ocrConfig, Path: path, Runner: p.Runner}
			info, err := probeOCRCapability(ctx, e)
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
			archives, _ := filepath.Glob(filepath.Join(p.ExeDir, "third_party", "ocr", "*.7z"))
			if path == "" && len(archives) > 0 {
				report.Issues = append(report.Issues, "检测到 OCR 离线压缩包，但压缩包不能直接运行。请先完整解压（保留模型和 DLL），再在设置中选择引擎 exe，或解压到工具同级 third_party\\ocr\\PaddleOCR-json（或 RapidOCR-json）目录。")
			}
			report.Issues = append(report.Issues, "未检测到可用 OCR 引擎：请部署 third_party\\ocr\\PaddleOCR-json 或 RapidOCR-json；当前退回 UIA/Win32 + 模板；可用 --no-ocr 显式跳过")
		}
	}
	for i := range report.OCREngines {
		row := &report.OCREngines[i]
		row.Selected = row.Engine == report.OCREngine &&
			((row.Path == nil && report.OCREnginePath == nil) || (row.Path != nil && report.OCREnginePath != nil && strings.EqualFold(*row.Path, *report.OCREnginePath)))
		if row.Engine == "umi_ocr" && report.OCREngine == "umi_ocr" && row.Path == nil && row.PackagePath == nil {
			row.Selected = true
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
