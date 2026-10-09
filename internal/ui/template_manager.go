package ui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/resources"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Patch only the existing CLI fields, preserving unrelated editor content.
func templateConfigBody(body, mode, dir string) (string, error) {
	if mode != "merged" && mode != "builtin" && mode != "external" {
		return "", fmt.Errorf("无效模板库：%s", mode)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return "", err
	}
	if doc == nil {
		return "", fmt.Errorf("配置必须是 JSON 对象")
	}
	var gui map[string]json.RawMessage
	if raw := doc["gui_automation"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &gui); err != nil {
			return "", err
		}
	}
	if gui == nil {
		gui = map[string]json.RawMessage{}
	}
	gui["template_pack"], _ = json.Marshal(mode)
	gui["builtin_templates"], _ = json.Marshal(mode != "external")
	if dir != "" {
		gui["template_dir"], _ = json.Marshal(dir)
	}
	doc["gui_automation"], _ = json.Marshal(gui)
	b, err := json.MarshalIndent(doc, "", "  ")
	return string(b), err
}

func loadExternalTemplateDir(dir string) (*templates.Library, error) {
	if _, err := os.Stat(filepath.Join(dir, "templates.index.json")); err != nil {
		return nil, fmt.Errorf("请选择包含 templates.index.json 的模板目录：%w", err)
	}
	cfg := config.Defaults()
	cfg.GUI.TemplatePack, cfg.GUI.TemplateDir = "external", dir
	lib, err := templates.Load(cfg.GUI, dir)
	if err != nil {
		return nil, err
	}
	if len(lib.Entries) == 0 {
		return nil, fmt.Errorf("模板索引为空，请先按模板制作说明准备 PNG 和索引")
	}
	for _, entry := range lib.Entries {
		if entry.Width < 24 || entry.Width > 240 || entry.Height < 16 || entry.Height > 48 {
			return nil, fmt.Errorf("模板 %s：尺寸应为 24~240 × 16~48", entry.ID)
		}
		header, err := entry.FS.Open(entry.File)
		if err != nil {
			return nil, fmt.Errorf("模板 %s：%w", entry.ID, err)
		}
		dimensions, err := png.DecodeConfig(header)
		header.Close()
		if err != nil || dimensions.Width != entry.Width || dimensions.Height != entry.Height {
			return nil, fmt.Errorf("模板 %s：PNG 无效或尺寸与索引不一致", entry.ID)
		}
		file, err := entry.FS.Open(entry.File)
		if err != nil {
			return nil, fmt.Errorf("模板 %s：%w", entry.ID, err)
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("模板 %s：无效 PNG：%w", entry.ID, err)
		}
		if _, ok := img.(*image.Gray); !ok {
			return nil, fmt.Errorf("模板 %s：必须是 8-bit 灰度 PNG", entry.ID)
		}
		if img.Bounds().Dx() != entry.Width || img.Bounds().Dy() != entry.Height {
			return nil, fmt.Errorf("模板 %s：图片尺寸与索引不一致", entry.ID)
		}
	}
	return lib, nil
}

func templateDetails(info templateInfo, mode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "资源版本：%s\r\n已保存的模板库：%s\r\n自定义目录：%s\r\n", info.ResourceVersion, info.Mode, info.TemplateDir)
	if mode != "external" {
		b.WriteString("\r\n内置规则与关键词：\r\n")
		fs.WalkDir(resources.FS, "assets/builtin", func(path string, file fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !file.IsDir() && !strings.Contains(path, "/templates/") {
				fmt.Fprintf(&b, "  %s\r\n", strings.TrimPrefix(path, "assets/builtin/"))
			}
			return nil
		})
	}
	if info.Error != "" {
		fmt.Fprintf(&b, "加载错误：%s\r\n", info.Error)
	}
	for _, warning := range info.Warnings {
		fmt.Fprintf(&b, "提示：%s\r\n", warning)
	}
	count := 0
	for _, entry := range info.Entries {
		if mode == "builtin" && entry.Source != "builtin" || mode == "external" && entry.Source != "external" {
			continue
		}
		count++
		fmt.Fprintf(&b, "\r\n%s [%s]\r\n家族：%s · 动作：%s · 语言：%s\r\nDPI：%d%% · 主题：%s · 尺寸：%d × %d\r\n文件：%s\r\n", entry.ID, entry.Source, entry.Family, entry.Action, entry.Lang, entry.DPI, entry.Theme, entry.Width, entry.Height, entry.File)
	}
	if count == 0 {
		b.WriteString("\r\n当前分类没有图像模板。UIA / OCR 是否可用以环境检查为准。\r\n内置关键词和规则不等同于图像模板。")
	}
	return b.String()
}
