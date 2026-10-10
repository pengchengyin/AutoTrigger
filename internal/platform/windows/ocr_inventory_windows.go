//go:build windows

package windows

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/ocr"
	"autotrigger/pkg/schema"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type ocrDefinition struct{ engine, name, executable string }

var ocrDefinitions = []ocrDefinition{
	{"paddleocr_json", "PaddleOCR-json", "PaddleOCR-json.exe"},
	{"rapidocr_json", "RapidOCR-json", "RapidOCR-json.exe"},
	{"umi_ocr", "Umi-OCR", "Umi-OCR.exe"},
	{"tesseract", "Tesseract", "tesseract.exe"},
	{"windows", "Windows OCR", ""},
}

func localOCRExecutables(root, executable string) []string {
	return localOCRExecutablesIn(filepath.Join(root, "third_party", "ocr"), executable)
}

func localOCRExecutablesIn(root, executable string) []string {
	if executable == "" {
		return nil
	}
	paths := []string{}
	filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if strings.HasPrefix(entry.Name(), ".deploy-") || len(strings.Split(rel, string(filepath.Separator))) > 8 {
				return filepath.SkipDir
			}
		} else if strings.EqualFold(entry.Name(), executable) {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return paths
}

func ocrPackages(root string, def ocrDefinition) []string {
	if def.executable == "" {
		return nil
	}
	paths := []string{}
	filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if entry.Name() == "packages" || strings.HasPrefix(entry.Name(), ".deploy-") || len(strings.Split(rel, string(filepath.Separator))) > 4 {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if (ext == ".7z" || ext == ".zip") && strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(def.name)) {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return paths
}

func ocrPackageTarget(root, path string) string {
	rel, _ := filepath.Rel(root, path)
	hash := sha256.Sum256([]byte(strings.ToLower(rel)))
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return filepath.Join(root, "packages", fmt.Sprintf("%s-%x", name, hash[:4]))
}

func (p *Prereqs) inventoryOCR(ctx context.Context, c config.Config, deploy, avx bool, report *schema.Prereq) []schema.OCREngineCheck {
	rows := []schema.OCREngineCheck{}
	root := filepath.Join(p.ExeDir, "third_party", "ocr")
	for _, def := range ocrDefinitions {
		paths := localOCRExecutables(p.ExeDir, def.executable)
		if def.engine == c.OCR.Engine && c.OCR.EnginePath != "auto" {
			paths = append([]string{findExecutable([]string{c.OCR.EnginePath}, p.ExeDir)}, paths...)
		}
		if def.executable != "" {
			if path, err := exec.LookPath(def.executable); err == nil {
				paths = append(paths, path)
			}
			stem := strings.TrimSuffix(def.executable, ".exe")
			if path := findExecutable([]string{filepath.Join(`C:\tools`, stem, def.executable), filepath.Join(`D:\tools`, stem, def.executable), filepath.Join(`C:\Program Files`, stem, def.executable), filepath.Join(`C:\Program Files\Tesseract-OCR`, def.executable)}, p.ExeDir); path != "" {
				paths = append(paths, path)
			}
		}
		packageByPath := map[string]string{}
		for _, pack := range ocrPackages(root, def) {
			target := ocrPackageTarget(root, pack)
			installed := localOCRExecutablesIn(target, def.executable)
			path := ""
			var err error
			if len(installed) == 1 {
				path = installed[0]
			} else if len(installed) > 1 {
				err = fmt.Errorf("部署目录包含多个 %s，无法确定引擎", def.executable)
			}
			if path == "" && err == nil && deploy && c.OCR.Enabled && c.OCR.Engine != "none" {
				if report.ArchiveBackend == nil {
					err = fmt.Errorf("需要可用的 7-Zip 才能部署此包")
				} else {
					path, err = p.deployOCRArchive(ctx, pack, target, def.executable, *report.ArchiveBackend, c.Archive.MaxExtractSizeBytes, false)
					if err == nil {
						report.Issues = append(report.Issues, "已自动部署 OCR 到工具目录："+path)
					}
				}
			}
			if path != "" {
				paths = append(paths, path)
				packageByPath[strings.ToLower(filepath.Clean(path))] = pack
				continue
			}
			row := schema.OCREngineCheck{Engine: def.engine, PackagePath: schema.String(pack), Languages: []string{}, Status: "not_deployed", Detail: "离线包尚未部署（只读检查不会解压）"}
			if !c.OCR.Enabled || c.OCR.Engine == "none" {
				row.Status, row.Detail = "disabled", "OCR 已关闭，未部署或启动此包"
			}
			if err != nil {
				row.Status, row.Detail = "deploy_failed", err.Error()
				report.Issues = append(report.Issues, def.name+" OCR 自动部署失败（"+filepath.Base(pack)+"）："+err.Error())
			}
			rows = append(rows, row)
		}
		seen := map[string]bool{}
		found := false
		for _, path := range paths {
			key := strings.ToLower(filepath.Clean(path))
			if path == "" || seen[key] {
				continue
			}
			seen[key] = true
			found = true
			row := schema.OCREngineCheck{Engine: def.engine, Path: schema.String(path), Languages: []string{}, Status: "missing"}
			if pack := packageByPath[key]; pack != "" {
				row.PackagePath = schema.String(pack)
			}
			if def.engine == "umi_ocr" {
				row.Status, row.Detail = "service_required", "程序已部署；请自行启动本包的本地 HTTP 服务。服务检测单独列出，不能证明此目录版本正在运行。"
				if !c.OCR.Enabled || c.OCR.Engine == "none" {
					row.Status, row.Detail = "disabled", "OCR 已关闭，未启动此程序"
				}
			} else {
				p.probeOCR(ctx, c, def, path, avx, &row)
			}
			if row.Status == "startup_failed" {
				report.Issues = append(report.Issues, def.name+" OCR 能力自检失败（"+path+"）："+row.Detail)
			}
			rows = append(rows, row)
		}
		if def.engine == "umi_ocr" {
			row := schema.OCREngineCheck{Engine: def.engine, Languages: []string{}}
			p.probeOCR(ctx, c, def, "", avx, &row)
			if row.Status == "startup_failed" {
				row.Status = "service_required"
				row.Detail = "本地 HTTP 服务不可用，请先启动 Umi-OCR 并开启接口：" + row.Detail
			}
			rows = append(rows, row)
		} else if !found && len(ocrPackages(root, def)) == 0 {
			row := schema.OCREngineCheck{Engine: def.engine, Languages: []string{}, Status: "missing", Detail: "未发现程序或支持的离线包"}
			if def.engine == "windows" {
				row.Status, row.Detail = "unsupported", "当前版本尚未实现 Windows OCR 适配，不能仅凭系统版本判定可用"
			} else if !c.OCR.Enabled || c.OCR.Engine == "none" {
				row.Status, row.Detail = "disabled", "OCR 已关闭，未启动自检"
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func (p *Prereqs) probeOCR(ctx context.Context, c config.Config, def ocrDefinition, path string, avx bool, row *schema.OCREngineCheck) {
	if !c.OCR.Enabled || c.OCR.Engine == "none" {
		row.Status, row.Detail = "disabled", "OCR 已关闭，未启动自检"
		return
	}
	if def.engine == "paddleocr_json" && !avx {
		row.Status, row.Detail = "unsupported", "CPU 不支持 AVX，未启动 PaddleOCR-json；可选择 RapidOCR-json"
		return
	}
	cfg := c.OCR
	cfg.Engine = def.engine
	// Alternative engines use their supported transport, not the selected engine's TCP port.
	if def.engine != c.OCR.Engine {
		cfg.Transport = "stdio"
		if def.engine == "umi_ocr" {
			cfg.TCPPort = 1224
		}
	}
	e := &ocr.Engine{Config: cfg, Path: path, Runner: p.Runner}
	defer e.Stop()
	info, err := probeOCRCapability(ctx, e)
	row.StartupMS = info.StartupMS
	if err != nil {
		row.Status, row.Detail = "startup_failed", err.Error()
		return
	}
	row.Status, row.Version, row.Languages = "ok", schema.String(info.Version), info.Languages
	row.Detail = "启动及识别请求自检通过"
	if def.engine == "umi_ocr" {
		row.Detail = "本地 HTTP 服务已通过识别请求自检；程序由用户管理，不自动启动或停止"
	}
}

func probeOCRCapability(ctx context.Context, engine *ocr.Engine) (ports.OCRInfo, error) {
	info, err := engine.Start(ctx)
	if err != nil {
		return info, err
	}
	test := image.NewRGBA(image.Rect(0, 0, 96, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 96; x++ {
			test.Set(x, y, color.White)
		}
	}
	_, err = engine.Recognize(ctx, test)
	return info, err
}
