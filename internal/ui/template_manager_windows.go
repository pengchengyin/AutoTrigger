//go:build windows

package ui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/resources"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

var templateControlIDs = []int{idTemplateAll, idTemplateBuiltin, idTemplateExternal, idTemplateUse, idTemplateUseBuiltin, idTemplateUseExternal, idTemplateDetailsMerged, idTemplateDetailsBuiltin, idTemplateDetailsExternal, idTemplateImport, idTemplateExport, idTemplateCheck, idTemplateBody, idTemplateStatus, idTemplateGuide, idTemplateSelect, idTemplatePreview, idTemplateCreate}

type templateCheckResult struct {
	text string
	err  error
}

const wmAppTemplate = 0x8000 + 3

func (f *form) templateCardRects(w, h int) []rectT {
	r := f.taskSurfaceRect(w, h)
	left, top := int(r.Left)+f.px(18), int(r.Top)+f.px(58)
	width := (int(r.Right) - left - f.px(18+24)) / 3
	indices := []int{0, 1, 2}
	if f.templateTab != 0 {
		indices = []int{f.templateTab}
	}
	out := make([]rectT, 3)
	for i, index := range indices {
		x := left + i*(width+f.px(12))
		out[index] = rectT{Left: int32(x), Top: int32(top), Right: int32(x + width), Bottom: int32(top + f.px(172))}
	}
	return out
}

func (f *form) layoutTemplates(w, h int) {
	r := f.taskSurfaceRect(w, h)
	left, right, top := int(r.Left)+f.px(18), int(r.Right)-f.px(18), int(r.Top)
	for i, id := range []int{idTemplateAll, idTemplateBuiltin, idTemplateExternal} {
		f.showAt(id, left+f.px(i*106), top+f.px(12), f.px(98), f.px(30))
	}
	f.showAt(idTemplateImport, right-f.px(240), top+f.px(12), f.px(110), f.px(30))
	f.showAt(idTemplateExport, right-f.px(122), top+f.px(12), f.px(122), f.px(30))
	for i, card := range f.templateCardRects(w, h) {
		if card.Right == 0 {
			continue
		}
		f.showAt([]int{idTemplateUse, idTemplateUseBuiltin, idTemplateUseExternal}[i], int(card.Left)+f.px(14), int(card.Bottom)-f.px(42), f.px(72), f.px(28))
		f.showAt(idTemplateDetailsMerged+i, int(card.Left)+f.px(94), int(card.Bottom)-f.px(42), f.px(72), f.px(28))
	}
	f.showAt(idTemplateCheck, right-f.px(110), top+f.px(244), f.px(110), f.px(28))
	f.showAt(idTemplateGuide, right-f.px(218), top+f.px(244), f.px(100), f.px(28))
	f.showAt(idTemplateCreate, right-f.px(326), top+f.px(244), f.px(100), f.px(28))
	f.showAt(idTemplateStatus, left, top+f.px(244), right-left-f.px(338), f.px(28))
	f.showAt(idTemplateSelect, left, top+f.px(282), right-left-f.px(120), f.comboDropH(f.px(28), 8))
	f.showAt(idTemplatePreview, right-f.px(110), top+f.px(282), f.px(110), f.px(28))
	f.showAt(idTemplateBody, left, top+f.px(322), right-left, maxInt(f.px(48), int(r.Bottom)-top-f.px(340)))
	if windowText(f.ctrls[idTemplateBody]) == "" {
		f.showTemplateDetails([]string{"merged", "builtin", "external"}[f.templateTab])
	}
	if f.templateChecking {
		procEnableWindow.Call(f.ctrls[idTemplateCheck], 0)
	} else {
		procEnableWindow.Call(f.ctrls[idTemplateCheck], 1)
	}
}

func (f *form) showTemplateDetails(mode string) {
	send(f.ctrls[idTemplateSelect], 0x14b, 0, 0)
	f.templateEntries = nil
	for _, entry := range f.templates.Entries {
		if mode == "builtin" && entry.Source != "builtin" || mode == "external" && entry.Source != "external" {
			continue
		}
		f.templateEntries = append(f.templateEntries, entry)
		send(f.ctrls[idTemplateSelect], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr(entry.ID+" · "+entry.Source))))
	}
	if len(f.templateEntries) == 0 {
		send(f.ctrls[idTemplateSelect], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr("当前分类没有图像模板"))))
	}
	f.setComboSel(idTemplateSelect, 0)
	enableWindow(f.ctrls[idTemplatePreview], len(f.templateEntries) > 0)
	setWindowTextRaw(f.ctrls[idTemplateBody], templateDetails(f.templates, mode))
	text := "资源详情 · " + map[string]string{"merged": "全部模板", "builtin": "内置模板", "external": "自定义模板"}[mode]
	if f.templates.Error != "" {
		text = "模板加载失败，原因见下方详情"
	}
	setWindowTextRaw(f.ctrls[idTemplateStatus], text)
}

var templateGuide = toCRLF(`模板制作与使用

1. 使用现有模板
选择模板库并点击使用，进入设置保存。merged 保留内置规则并按 ID 合并自定义图像模板；builtin 忽略外部；external 仅用外部。当前内置图像索引为空，不能将内置规则当作安装按钮图片。

2. 导入
选择包含 templates.index.json 的目录。每个条目须有唯一 id、family、action、lang、dpi、theme、file、width、height。file 必须是目录内的相对路径。图片须为 8-bit 灰度 PNG，宽 24~240、高 16~48。保存配置后生效，源文件不会被移动或覆盖。

3. 制作真实按钮图片
点击模板管理中的“制作模板”。自行打开真实安装器，选择窗口后抓取截图，或载入已有原始 PNG。在截图中拖动框选按钮，用 X、Y、宽度、高度精确调整范围。选择家族、动作、实际语言、DPI 和主题，检查模板 ID，选择保存目录后点击“保存并使用”。按钮自动转为灰度图，原始截图和索引一同保存；现有库追加条目，重复 ID 不覆盖。最后到设置保存配置，才用于执行。
采集不会启动或点击安装器。紧贴控件外接矩形，不要手工重绘；建议母版使用 100% DPI、浅色主题。中文和英文、勾选和未勾选、可用和禁用状态分别采集，禁用态仅作参考。

4. 预览与校验
下拉选择实际模板可查看字段和打开图片。校验会在后台匹配源截图，缺少图片、源截图或匹配失败会列出原因。通过源截图自检不代表已经通过真实安装器端到端验证。

5. 导出与现场排障
导出内置资源会创建新目录，包含规则和 templates 子目录。空模板索引需要自行补齐真实素材。命令行 --dump-ui 可采集窗口素材，--dump-resources 可导出资源，--self-test templates 可自检；详细制作规范见交付文档 docs/图像模板制作说明.md。`)

func (f *form) selectedTemplateDetails() {
	index := f.comboIndex(idTemplateSelect)
	if index < 0 || index >= len(f.templateEntries) {
		return
	}
	entry := f.templateEntries[index]
	setWindowTextRaw(f.ctrls[idTemplateBody], fmt.Sprintf("模板 ID：%s\r\n来源：%s\r\n家族：%s\r\n动作：%s\r\n语言：%s\r\nDPI：%d%%\r\n主题：%s\r\n尺寸：%d × %d\r\n图片：%s\r\n源截图：%s\r\n素材来源：%s", entry.ID, entry.Source, entry.Family, entry.Action, entry.Lang, entry.DPI, entry.Theme, entry.Width, entry.Height, entry.File, entry.Screenshot, entry.Origin))
}

func (f *form) previewTemplate() {
	index := f.comboIndex(idTemplateSelect)
	if index < 0 || index >= len(f.templateEntries) {
		return
	}
	entry := f.templateEntries[index]
	dir := filepath.Join(os.TempDir(), "AutoTrigger-template-preview")
	path, err := templatePreviewFile(entry, dir)
	if err == nil {
		err = shellOpen(path)
	}
	if err != nil {
		f.templateNotice("无法打开模板图片：" + err.Error())
	}
}

func (f *form) templateNotice(text string) {
	setWindowTextRaw(f.ctrls[idTemplateStatus], text)
	setWindowTextRaw(f.ctrls[idTemplateBody], text+"\r\n\r\n"+templateDetails(f.templates, []string{"merged", "builtin", "external"}[f.templateTab]))
}

func (f *form) importTemplateDir(dir string) {
	lib, err := loadExternalTemplateDir(dir)
	if err != nil {
		f.templateNotice("导入失败：" + err.Error())
		return
	}
	if f.applyTemplatePack("merged", dir) {
		setWindowTextRaw(f.ctrls[idCfgStatus], fmt.Sprintf("已导入 %d 个自定义模板；保存配置后生效（内置为底、自定义按 ID 覆盖）。", len(lib.Entries)))
	}
}

func (f *form) useExternalTemplates() {
	if _, err := loadExternalTemplateDir(f.templates.TemplateDir); err != nil {
		f.templateNotice("无法使用自定义模板：" + err.Error())
		return
	}
	f.applyTemplatePack("external", "")
}

func (f *form) exportTemplateResources() {
	dir, ok := pickFolderDialog(f.hwnd, "选择内置资源导出位置")
	if !ok {
		return
	}
	f.exportTemplateResourcesTo(dir)
}

func (f *form) exportTemplateResourcesTo(parent string) {
	dir, err := os.MkdirTemp(parent, "AutoTrigger-resources-")
	if err == nil {
		err = resources.Export(dir)
	}
	if err != nil {
		f.templateNotice("导出失败：" + err.Error())
		return
	}
	f.templateNotice("内置资源已导出到：" + dir + "\r\n模板目录：" + filepath.Join(dir, "templates"))
}

func (f *form) checkTemplates() {
	if f.templateChecking {
		return
	}
	cfg, _, err := config.Load(strings.TrimSpace(windowText(f.ctrls[idConfig])), f.exeDir)
	if err != nil {
		f.templateNotice("校验失败：" + err.Error())
		return
	}
	cfg.GUI.TemplatePack = []string{"merged", "builtin", "external"}[f.templateTab]
	cfg.GUI.BuiltinTemplates = true
	f.templateChecking = true
	f.templateCheckResult = make(chan templateCheckResult, 1)
	results, hwnd, root := f.templateCheckResult, f.hwnd, f.exeDir
	f.templateNotice("正在校验模板图片和源截图匹配…")
	procEnableWindow.Call(f.ctrls[idTemplateCheck], 0)
	go func() {
		lib, err := templates.Load(cfg.GUI, root)
		text := ""
		if err == nil && len(lib.Entries) == 0 {
			err = fmt.Errorf("没有可校验的图像模板，请先导入自定义模板；规则资源不计入图像模板")
		}
		if err == nil {
			failures, testErr := lib.SelfTest()
			err = testErr
			if err == nil {
				text = fmt.Sprintf("校验通过：%d 个图像模板均匹配源截图。", len(lib.Entries))
			} else {
				text = strings.Join(failures, "\r\n")
			}
		}
		results <- templateCheckResult{text: text, err: err}
		post(hwnd, wmAppTemplate, 0, 0)
	}()
}

func (f *form) finishTemplateCheck() {
	if f.templateCheckResult == nil {
		return
	}
	select {
	case result := <-f.templateCheckResult:
		f.templateChecking = false
		procEnableWindow.Call(f.ctrls[idTemplateCheck], 1)
		if result.err != nil {
			result.text = "模板校验未通过：" + result.err.Error() + "\r\n" + result.text
		}
		f.templateNotice(result.text)
	default:
	}
}
