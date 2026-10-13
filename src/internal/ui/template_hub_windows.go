//go:build windows

package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

const (
	hubName = 5100 + iota
	hubDescription
	hubSave
	hubBack
	hubCapture
	hubImages
	hubDelete
	hubAction
	hubMark
	hubPrevious
	hubNext
	hubStatus
)

var hubControlIDs = []int{hubName, hubDescription, hubSave, hubBack, hubCapture, hubImages, hubDelete, hubAction, hubMark, hubPrevious, hubNext, hubStatus, 5400, 5401, 5200, 5201, 5202, 5203, 5300, 5301, 5302, 5303}

func init() { templateControlIDs = append(templateControlIDs, hubControlIDs...) }

func (f *form) ensureHubControls() {
	if f.ctrls[hubName] != 0 {
		return
	}
	f.edit(hubName, "")
	f.edit(hubDescription, "")
	f.label(5400, "模板名称（必填）")
	f.label(5401, "模板描述（选填，例如：适用软件、窗口或用途）")
	f.label(hubStatus, "")
	send(f.ctrls[hubName], 0x1501, 1, uintptr(unsafe.Pointer(utf16Ptr("模板名称"))))
	send(f.ctrls[hubDescription], 0x1501, 1, uintptr(unsafe.Pointer(utf16Ptr("模板描述"))))
	for id, text := range map[int]string{hubSave: "保存模板", hubBack: "返回列表", hubCapture: "开始捕获", hubDelete: "删除图片", hubMark: "保存标记", hubPrevious: "<", hubNext: ">"} {
		f.button(id, text)
	}
	f.combo(hubAction, []string{"选择图片动作", "下一步", "安装", "接受协议", "确定", "是", "完成"}, 0)
	f.addControl(hubImages, "LISTBOX", "", wsChild|wsTabStop|wsVScroll|0x1, wsExClientEdge)
	for i := 0; i < 4; i++ {
		f.check(5200+i, "启用", false)
		f.button(5300+i, "查看")
	}
	for _, id := range hubControlIDs {
		procShowWindow.Call(f.ctrls[id], swHide)
	}
	if err := initializeTemplateSelection(f.templateRoot()); err != nil {
		f.setStatusKind(err.Error(), statusKindFail)
	}
	f.refreshTemplateHub()
}

func (f *form) refreshTemplateHub() {
	projects, err := listTemplateProjects(f.templateRoot())
	if err != nil {
		f.setStatusKind("模板读取失败："+err.Error(), statusKindFail)
		return
	}
	f.hubProjects = projects
	f.hubEnabled = map[string]bool{}
	index, err := selectedProjects(f.templateRoot())
	if err != nil {
		f.setStatusKind("启用索引读取失败："+err.Error(), statusKindFail)
	} else {
		for _, id := range index.Projects {
			f.hubEnabled[id] = true
		}
	}
	if f.hubPage*4 >= len(projects) {
		f.hubPage = maxInt(0, (len(projects)-1)/4)
	}
	f.hubCaptures = nil
	f.hubLegacy = nil
	if p, ok := f.hubProject(); ok {
		index, err := readCaptureIndex(projectDir(f.templateRoot(), p), p.ID)
		if err != nil {
			f.setStatusKind(err.Error(), statusKindFail)
		} else {
			f.hubCaptures = index.Items
		}
		legacy, err := legacyProjectImages(f.templateRoot(), p)
		if err != nil {
			f.setStatusKind(err.Error(), statusKindFail)
		} else {
			f.hubLegacy = legacy
		}
	}
	if f.ctrls[hubImages] != 0 {
		send(f.ctrls[hubImages], 0x184, 0, 0)
		for _, item := range f.hubCaptures {
			label := item.Name + " · " + item.Rule.Method
			if !item.Confirmed {
				label += " · 未确认"
			}
			send(f.ctrls[hubImages], 0x180, 0, uintptr(unsafe.Pointer(utf16Ptr(label))))
		}
		for _, entry := range f.hubLegacy {
			send(f.ctrls[hubImages], 0x180, 0, uintptr(unsafe.Pointer(utf16Ptr(entry.ID+" · 图片模板"))))
		}
	}
}

func (f *form) hubProject() (templateProject, bool) {
	for _, p := range f.hubProjects {
		if p.ID == f.hubSelected {
			return p, true
		}
	}
	return templateProject{}, false
}

func (f *form) openHubProject(id string) {
	f.hubSelected = id
	f.hubEditing = false
	f.refreshTemplateHub()
	if p, ok := f.hubProject(); ok {
		setWindowTextRaw(f.ctrls[hubName], p.Name)
		setWindowTextRaw(f.ctrls[hubDescription], p.Description)
	}
	if pane := makerPanes[f.templateMaker]; pane != nil {
		pane.shot = nil
	}
	f.layout()
}

func (f *form) hubCardRects(w, h int) []rectT {
	r := f.taskSurfaceRect(w, h)
	pad, gap := f.px(18), f.px(16)
	width := (int(r.Right-r.Left) - 2*pad - gap) / 2
	height := f.px(174)
	out := []rectT{}
	for i := 0; i < 4 && f.hubPage*4+i < len(f.hubProjects); i++ {
		x := int(r.Left) + pad + (i%2)*(width+gap)
		y := int(r.Top) + pad + (i/2)*(height+gap)
		out = append(out, rectT{Left: int32(x), Top: int32(y), Right: int32(x + width), Bottom: int32(y + height)})
	}
	return out
}

func (f *form) layoutTemplateHub(w, h int) {
	f.ensureHubControls()
	r := f.taskSurfaceRect(w, h)
	left, right, top := int(r.Left)+f.px(18), int(r.Right)-f.px(18), int(r.Top)+f.px(18)
	f.showAt(hubStatus, left, int(r.Bottom)-f.px(40), right-left-f.px(100), f.px(26))
	if f.hubEditing || f.hubSelected != "" {
		f.showAt(5400, left, top, right-left-f.px(220), f.px(22))
		f.showAt(5401, left, top+f.px(66), right-left, f.px(22))
		top += f.px(26)
		f.showAt(hubName, left, top, right-left-f.px(220), f.px(32))
		f.showAt(hubSave, right-f.px(212), top, f.px(100), f.px(32))
		f.showAt(hubBack, right-f.px(104), top, f.px(104), f.px(32))
		f.showAt(hubDescription, left, top+f.px(66), right-left, f.px(32))
		if f.hubEditing {
			return
		}
		f.showAt(hubCapture, left, top+f.px(110), f.px(120), f.px(32))
		f.showAt(hubDelete, left+f.px(130), top+f.px(110), f.px(110), f.px(32))
		f.showAt(hubAction, left+f.px(250), top+f.px(110), f.px(170), f.comboDropH(f.px(32), 7))
		f.showAt(hubMark, left+f.px(430), top+f.px(110), f.px(100), f.px(32))
		f.showAt(hubImages, left, top+f.px(158), f.px(250), maxInt(f.px(40), int(r.Bottom)-top-f.px(212)))
		for _, id := range []int{hubCapture, hubDelete, hubMark} {
			enableWindow(f.ctrls[id], !f.isRunning())
		}
		return
	}
	for i, card := range f.hubCardRects(w, h) {
		p := f.hubProjects[f.hubPage*4+i]
		f.showAt(5200+i, int(card.Right)-f.px(88), int(card.Top)+f.px(14), f.px(76), f.px(28))
		checked := uintptr(0)
		if f.hubEnabled[p.ID] {
			checked = bstChecked
		}
		send(f.ctrls[5200+i], bmSetCheck, checked, 0)
		f.showAt(5300+i, int(card.Left)+f.px(16), int(card.Bottom)-f.px(44), f.px(78), f.px(28))
	}
	f.showAt(hubPrevious, right-f.px(88), int(r.Bottom)-f.px(40), f.px(36), f.px(26))
	f.showAt(hubNext, right-f.px(42), int(r.Bottom)-f.px(40), f.px(36), f.px(26))
	enableWindow(f.ctrls[hubPrevious], f.hubPage > 0)
	enableWindow(f.ctrls[hubNext], (f.hubPage+1)*4 < len(f.hubProjects))
}

func (f *form) paintTemplateHub(dc uintptr, r rectT) {
	if f.hubEditing {
		return
	}
	if f.hubSelected != "" {
		p := makerPanes[f.templateMaker]
		area := image.Rect(int(r.Left)+f.px(286), int(r.Top)+f.px(202), int(r.Right)-f.px(18), int(r.Bottom)-f.px(54))
		if p == nil || p.shot == nil {
			drawText(dc, "选择图片查看", rectT{Left: int32(area.Min.X), Top: int32(area.Min.Y), Right: int32(area.Max.X), Bottom: int32(area.Max.Y)}, modernTheme.muted, f.font, dtCenter|dtVCenter)
			return
		}
		d := templateImageRect(area, p.shot.Bounds().Size())
		if d.Dx() > p.shot.Bounds().Dx() {
			size := p.shot.Bounds().Size()
			center := area.Min.Add(area.Size().Div(2))
			d = image.Rectangle{Min: center.Sub(size.Div(2)), Max: center.Sub(size.Div(2)).Add(size)}
		}
		b := p.shot.Bounds()
		info := makerBitmapInfo{Size: 40, Width: int32(b.Dx()), Height: -int32(b.Dy()), Planes: 1, Bits: 32}
		gdi32.NewProc("StretchDIBits").Call(dc, uintptr(d.Min.X), uintptr(d.Min.Y), uintptr(d.Dx()), uintptr(d.Dy()), 0, 0, uintptr(b.Dx()), uintptr(b.Dy()), uintptr(unsafe.Pointer(&p.pixels[0])), uintptr(unsafe.Pointer(&info)), 0, 0xcc0020)
		if !p.bounds.Empty() {
			mark := rectT{Left: int32(d.Min.X + p.bounds.Min.X*d.Dx()/b.Dx()), Top: int32(d.Min.Y + p.bounds.Min.Y*d.Dy()/b.Dy()), Right: int32(d.Min.X + p.bounds.Max.X*d.Dx()/b.Dx()), Bottom: int32(d.Min.Y + p.bounds.Max.Y*d.Dy()/b.Dy())}
			fillSolid(dc, rectT{Left: mark.Left, Top: mark.Top, Right: mark.Right, Bottom: mark.Top + 2}, modernTheme.primary)
			fillSolid(dc, rectT{Left: mark.Left, Top: mark.Bottom - 2, Right: mark.Right, Bottom: mark.Bottom}, modernTheme.primary)
			fillSolid(dc, rectT{Left: mark.Left, Top: mark.Top, Right: mark.Left + 2, Bottom: mark.Bottom}, modernTheme.primary)
			fillSolid(dc, rectT{Left: mark.Right - 2, Top: mark.Top, Right: mark.Right, Bottom: mark.Bottom}, modernTheme.primary)
		}
		return
	}
	w, h := f.clientSize()
	if len(f.hubProjects) == 0 {
		drawText(dc, "暂无模板", r, modernTheme.muted, f.font, dtCenter|dtVCenter)
		return
	}
	for i, card := range f.hubCardRects(w, h) {
		p := f.hubProjects[f.hubPage*4+i]
		roundRect(dc, card, modernTheme.surface, modernTheme.border, f.px(6))
		text := card
		text.Left += int32(f.px(16))
		text.Right -= int32(f.px(100))
		text.Top += int32(f.px(14))
		text.Bottom = text.Top + int32(f.px(28))
		drawText(dc, p.Name, text, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		text.Right = card.Right - int32(f.px(16))
		text.Top += int32(f.px(40))
		text.Bottom = card.Bottom - int32(f.px(80))
		drawText(dc, valueOr(p.Description, "未填写描述"), text, modernTheme.muted, f.font, dtLeft|0x10)
		index, err := readCaptureIndex(projectDir(f.templateRoot(), p), p.ID)
		count := 0
		for _, item := range index.Items {
			if item.Confirmed {
				count++
			}
		}
		legacy, legacyErr := legacyProjectImages(f.templateRoot(), p)
		count += len(legacy)
		label := fmt.Sprintf("已标记图片：%d", count)
		if err != nil || legacyErr != nil {
			label = "图片索引异常"
		}
		text.Top = card.Bottom - int32(f.px(78))
		text.Bottom = text.Top + int32(f.px(24))
		drawText(dc, label, text, modernTheme.primaryDark, f.font, dtLeft|dtVCenter|dtSingleLine)
	}
}

func (f *form) hubPane() *templateMakerPane {
	f.createTemplateMaker(false)
	p := makerPanes[f.templateMaker]
	if p != nil {
		p.hub = true
		p.refreshProjects(f.hubSelected)
	}
	return p
}

func (f *form) pauseHubProject() error {
	if f.isRunning() {
		return fmt.Errorf("任务运行中，请结束后再编辑")
	}
	if f.hubEnabled[f.hubSelected] {
		if err := toggleTemplateProject(f.templateRoot(), f.hubSelected, false, false); err != nil {
			return err
		}
		f.hubEnabled[f.hubSelected] = false
	}
	return nil
}

func (f *form) hubCommand(id, code int) bool {
	if f.page != pageTemplates {
		return false
	}
	if code == bnClicked && (id >= 5200 && id < 5204 || id >= 5300 && id < 5304) {
		i := id - 5200
		if id >= 5300 {
			i = id - 5300
		}
		at := f.hubPage*4 + i
		if at >= len(f.hubProjects) {
			return true
		}
		p := f.hubProjects[at]
		if id >= 5300 {
			f.openHubProject(p.ID)
		} else {
			enabled := f.isChecked(id)
			if err := toggleTemplateProject(f.templateRoot(), p.ID, enabled, f.isRunning()); err != nil {
				messageBox(f.hwnd, "无法切换模板", err.Error(), mbOK|mbIconInfo)
				f.setStatusKind(err.Error(), statusKindFail)
			} else if enabled {
				f.setStatusKind("已启用："+p.Name+"（供后续任务匹配）", statusKindOK)
			} else {
				f.setStatus("已停用：" + p.Name)
			}
			f.refreshTemplateHub()
			f.layout()
		}
		return true
	}
	if id == hubImages && code == 1 {
		i := int(send(f.ctrls[hubImages], 0x188, 0, 0))
		if i < 0 || i >= len(f.hubCaptures)+len(f.hubLegacy) {
			return true
		}
		p := f.hubPane()
		if p == nil {
			return true
		}
		var shot image.Image
		var err error
		name, action := "", ""
		if i < len(f.hubCaptures) {
			item := f.hubCaptures[i]
			name = item.Name
			action = item.Rule.Action
			shot, err = capturePreview(projectDir(f.templateRoot(), p.project), item)
		} else {
			entry := f.hubLegacy[i-len(f.hubCaptures)]
			name = entry.ID
			action = entry.Action
			var file *os.File
			file, err = os.Open(filepath.Join(projectDir(f.templateRoot(), p.project), filepath.FromSlash(entry.File)))
			if err == nil {
				shot, err = png.Decode(file)
				file.Close()
			}
		}
		if err != nil {
			p.shot = nil
			f.setStatusKind(err.Error(), statusKindFail)
		} else {
			p.setImage(shot, name)
			p.bounds = image.Rectangle{}
			if i < len(f.hubCaptures) {
				item := f.hubCaptures[i]
				if item.SourceBounds != nil {
					b := item.SourceBounds
					p.bounds = image.Rect(item.Bounds.Left-b.Left, item.Bounds.Top-b.Top, item.Bounds.Right-b.Left, item.Bounds.Bottom-b.Top)
					f.setStatusKind("原图完整 · 标记区域已显示", statusKindOK)
				} else {
					f.setStatusKind("旧记录：只保存了标记图，原图需重新捕获", statusKindWarn)
				}
			}
		}
		f.setComboSel(hubAction, 0)
		for at, candidate := range []string{"", "next", "install", "accept", "ok", "yes", "finish"} {
			if candidate == action {
				f.setComboSel(hubAction, at)
			}
		}
		procInvalidateRect.Call(f.hwnd, 0, 0)
		return true
	}
	if code != bnClicked {
		return false
	}
	switch id {
	case hubPrevious:
		f.hubPage = maxInt(0, f.hubPage-1)
		f.layout()
	case hubNext:
		if (f.hubPage+1)*4 < len(f.hubProjects) {
			f.hubPage++
		}
		f.layout()
	case hubBack:
		f.hubSelected = ""
		f.hubEditing = false
		f.refreshTemplateHub()
		f.layout()
	case hubSave:
		name := strings.TrimSpace(windowText(f.ctrls[hubName]))
		if name == "" {
			f.setStatusKind("请填写模板名称", statusKindFail)
			return true
		}
		p, ok := f.hubProject()
		var err error
		if !ok {
			p, err = newTemplateProject(f.templateRoot(), name)
		}
		if err == nil {
			p.Name = name
			p.Description = strings.TrimSpace(windowText(f.ctrls[hubDescription]))
			err = writeProject(f.templateRoot(), p)
		}
		if err != nil {
			f.setStatusKind(err.Error(), statusKindFail)
		} else {
			f.hubEditing = false
			f.hubSelected = p.ID
			f.refreshTemplateHub()
			f.layout()
		}
	case hubCapture:
		if err := f.pauseHubProject(); err != nil {
			f.setStatusKind(err.Error(), statusKindFail)
			return true
		}
		p := f.hubPane()
		if p != nil {
			p.startHoverCapture()
		}
	case hubDelete, hubMark:
		i := int(send(f.ctrls[hubImages], 0x188, 0, 0))
		if i < 0 || i >= len(f.hubCaptures)+len(f.hubLegacy) {
			f.setStatus("请先选择图片")
			return true
		}
		if err := f.pauseHubProject(); err != nil {
			f.setStatusKind(err.Error(), statusKindFail)
			return true
		}
		p, ok := f.hubProject()
		if !ok {
			return true
		}
		actions := []string{"", "next", "install", "accept", "ok", "yes", "finish"}
		action := ""
		at := f.comboIndex(hubAction)
		if at >= 0 && at < len(actions) {
			action = actions[at]
		}
		var err error
		if i < len(f.hubCaptures) {
			err = updateCaptureMark(f.templateRoot(), p, f.hubCaptures[i].ID, action, id == hubDelete)
		} else {
			err = updateLegacyMark(f.templateRoot(), p, f.hubLegacy[i-len(f.hubCaptures)].ID, action, id == hubDelete)
		}
		if err != nil {
			f.setStatusKind(err.Error(), statusKindFail)
		} else {
			f.refreshTemplateHub()
			if id == hubDelete {
				if pane := makerPanes[f.templateMaker]; pane != nil {
					pane.shot = nil
				}
			} else {
				send(f.ctrls[hubImages], 0x186, uintptr(i), 0)
				f.hubCommand(hubImages, 1)
			}
			f.layout()
			f.setStatus("草稿已更新；重新勾选模板后用于后续任务")
		}
	default:
		return false
	}
	return true
}
