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
)

var templateControlIDs = []int{idTemplateAll, idTemplateBuiltin, idTemplateExternal, idTemplateUse, idTemplateUseBuiltin, idTemplateUseExternal, idTemplateDetailsMerged, idTemplateDetailsBuiltin, idTemplateDetailsExternal, idTemplateImport, idTemplateExport, idTemplateCheck, idTemplateBody, idTemplateStatus}

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
	f.showAt(idTemplateStatus, left, top+f.px(244), right-left-f.px(122), f.px(28))
	f.showAt(idTemplateBody, left, top+f.px(284), right-left, maxInt(f.px(48), int(r.Bottom)-top-f.px(302)))
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
	setWindowTextRaw(f.ctrls[idTemplateBody], templateDetails(f.templates, mode))
	text := "资源详情 · " + map[string]string{"merged": "全部模板", "builtin": "内置模板", "external": "自定义模板"}[mode]
	if f.templates.Error != "" {
		text = "模板加载失败，原因见下方详情"
	}
	setWindowTextRaw(f.ctrls[idTemplateStatus], text)
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
