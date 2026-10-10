//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/pkg/schema"
	"strconv"
	"strings"
	"unsafe"
)

const (
	uiSidebarW   = 148
	uiHeaderH    = 62
	uiContentPad = 18
	uiCardRadius = 8
)

type taskSectionLayout struct {
	Title string
	Rect  rectT
}

var modernTheme = struct {
	bg          uintptr
	surface     uintptr
	surfaceAlt  uintptr
	border      uintptr
	text        uintptr
	muted       uintptr
	primary     uintptr
	primaryDark uintptr
	primarySoft uintptr
	success     uintptr
	danger      uintptr
	warn        uintptr
}{
	bg:          rgb(244, 249, 255),
	surface:     rgb(255, 255, 255),
	surfaceAlt:  rgb(238, 246, 255),
	border:      rgb(211, 228, 248),
	text:        rgb(24, 42, 70),
	muted:       rgb(89, 112, 141),
	primary:     rgb(30, 116, 255),
	primaryDark: rgb(14, 74, 178),
	primarySoft: rgb(226, 240, 255),
	success:     rgb(15, 163, 95),
	danger:      rgb(224, 54, 74),
	warn:        rgb(222, 132, 30),
}

func (f *form) paint() uintptr {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(f.hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc != 0 {
		f.paintBuffered(hdc, ps.RcPaint)
	}
	procEndPaint.Call(f.hwnd, uintptr(unsafe.Pointer(&ps)))
	return 0
}

func (f *form) paintBuffered(hdc uintptr, dirty rectT) {
	w, h := f.clientSize()
	mem, _, _ := gdi32.NewProc("CreateCompatibleDC").Call(hdc)
	if mem == 0 {
		f.paintWindow(hdc)
		return
	}
	defer gdi32.NewProc("DeleteDC").Call(mem)
	bitmap, _, _ := gdi32.NewProc("CreateCompatibleBitmap").Call(hdc, uintptr(w), uintptr(h))
	if bitmap == 0 {
		f.paintWindow(hdc)
		return
	}
	defer procDeleteObject.Call(bitmap)
	old, _, _ := procSelectObject.Call(mem, bitmap)
	defer procSelectObject.Call(mem, old)
	gdi32.NewProc("IntersectClipRect").Call(mem, uintptr(dirty.Left), uintptr(dirty.Top), uintptr(dirty.Right), uintptr(dirty.Bottom))
	if dirty.Right <= int32(f.px(uiSidebarW)) {
		f.paintSidebar(mem, h)
	} else {
		f.paintWindow(mem)
	}
	gdi32.NewProc("BitBlt").Call(hdc, uintptr(dirty.Left), uintptr(dirty.Top), uintptr(dirty.Right-dirty.Left), uintptr(dirty.Bottom-dirty.Top), mem, uintptr(dirty.Left), uintptr(dirty.Top), 0x00CC0020)
}

func (f *form) paintWindow(hdc uintptr) {
	clientW, clientH := f.clientSize()
	fillSolid(hdc, rectT{Right: int32(clientW), Bottom: int32(clientH)}, modernTheme.bg)
	f.paintSidebar(hdc, clientH)
	f.paintHeader(hdc, clientW)
	switch f.page {
	case pageHome:
		f.paintHome(hdc, clientW, clientH)
	case pageTask, pageNewTask:
		f.paintContentPanel(hdc, clientW, clientH)
	default:
		f.paintPlaceholderPage(hdc, clientW, clientH)
	}
}

func (f *form) paintSidebar(hdc uintptr, clientH int) {
	sideW := f.px(uiSidebarW)
	fillSolid(hdc, rectT{Right: int32(sideW), Bottom: int32(clientH)}, modernTheme.surface)
	fillSolid(hdc, rectT{Left: int32(sideW - f.px(1)), Right: int32(sideW), Bottom: int32(clientH)}, modernTheme.border)

	logo := rectT{Left: int32(f.px(14)), Top: int32(f.px(14)), Right: int32(f.px(42)), Bottom: int32(f.px(42))}
	roundRect(hdc, logo, modernTheme.primary, modernTheme.primary, f.px(7))
	f.paintAppIcon(hdc, logo)
	drawText(hdc, "AutoTrigger", rectT{Left: int32(f.px(50)), Top: int32(f.px(12)), Right: int32(sideW - f.px(8)), Bottom: int32(f.px(30))}, modernTheme.text, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	drawText(hdc, "v"+cliVersionText(), rectT{Left: int32(f.px(50)), Top: int32(f.px(30)), Right: int32(sideW - f.px(8)), Bottom: int32(f.px(48))}, modernTheme.muted, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)

	for i, item := range uiNavItems {
		r := f.navItemRect(i)
		active := item.ID == f.page || (item.ID == pageTask && (f.page == pageNewTask || f.page == pageTaskDetail))
		hovered := item.ID == f.hoverPage
		if active {
			roundRect(hdc, r, modernTheme.primary, modernTheme.primary, f.px(uiCardRadius))
		} else if hovered {
			roundRect(hdc, r, modernTheme.primarySoft, modernTheme.primarySoft, f.px(uiCardRadius))
		}
		color := modernTheme.text
		if active {
			color = modernTheme.surface
		}
		iconRect := rectT{Left: r.Left + int32(f.px(10)), Top: r.Top, Right: r.Left + int32(f.px(32)), Bottom: r.Bottom}
		textRect := rectT{Left: r.Left + int32(f.px(36)), Top: r.Top, Right: r.Right - int32(f.px(8)), Bottom: r.Bottom}
		drawText(hdc, item.Icon, iconRect, color, f.font, dtCenter|dtVCenter|dtSingleLine)
		drawText(hdc, item.Label, textRect, color, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	}

	drawText(hdc, "工具版本："+cliVersionText(), rectT{Left: int32(f.px(14)), Top: int32(clientH - f.px(32)), Right: int32(sideW - f.px(12)), Bottom: int32(clientH - f.px(14))}, modernTheme.muted, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
}

func (f *form) paintHeader(hdc uintptr, clientW int) {
	sideW := f.px(uiSidebarW)
	left := sideW + f.px(uiContentPad)
	top := f.px(14)
	actions := headerActions(f.page)
	actionLeft := clientW - f.px(20)
	for i := range actions {
		r := f.headerActionRect(i, clientW)
		if int(r.Left)-f.px(12) < actionLeft {
			actionLeft = int(r.Left) - f.px(12)
		}
		f.paintActionButton(hdc, r, actions[i])
	}
	drawText(hdc, pageTitle(f.page), rectT{Left: int32(left), Top: int32(top), Right: int32(actionLeft), Bottom: int32(top + f.px(24))}, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	drawText(hdc, pageSubtitle(f.page), rectT{Left: int32(left), Top: int32(top + f.px(28)), Right: int32(actionLeft), Bottom: int32(top + f.px(48))}, modernTheme.muted, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
}

func (f *form) paintActionButton(hdc uintptr, r rectT, action pageAction) {
	fill, stroke, text := modernTheme.surface, modernTheme.border, modernTheme.primaryDark
	if action.Kind == "primary" {
		fill, stroke, text = modernTheme.primary, modernTheme.primary, modernTheme.surface
	}
	roundRect(hdc, r, fill, stroke, f.px(7))
	drawText(hdc, action.Label, r, text, f.font, dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
}

func (f *form) headerActionRect(index, clientW int) rectT {
	w := f.px(92)
	h := f.px(30)
	right := clientW - f.px(20) - index*(w+f.px(8))
	top := f.px(18)
	return rectT{Left: int32(right - w), Top: int32(top), Right: int32(right), Bottom: int32(top + h)}
}

func (f *form) headerActionAt(x, y, clientW int) (pageAction, bool) {
	actions := headerActions(f.page)
	for i, action := range actions {
		if pointInRect(x, y, f.headerActionRect(i, clientW)) {
			return action, true
		}
	}
	return pageAction{}, false
}

func (f *form) paintHome(hdc uintptr, clientW, clientH int) {
	left := f.px(uiSidebarW) + f.px(uiContentPad)
	right := clientW - f.px(uiContentPad)
	top := f.px(uiHeaderH)
	hero := rectT{Left: int32(left), Top: int32(top), Right: int32(right), Bottom: int32(top + f.px(112))}
	roundRect(hdc, hero, modernTheme.surfaceAlt, modernTheme.border, f.px(uiCardRadius))
	icon := rectT{Left: hero.Left + int32(f.px(28)), Top: hero.Top + int32(f.px(20)), Right: hero.Left + int32(f.px(92)), Bottom: hero.Top + int32(f.px(84))}
	roundRect(hdc, icon, modernTheme.primary, modernTheme.primary, f.px(14))
	f.paintAppIcon(hdc, icon)
	drawText(hdc, "Windows 任意文件自动安装/触发工具", rectT{Left: hero.Left + int32(f.px(112)), Top: hero.Top + int32(f.px(24)), Right: hero.Right - int32(f.px(20)), Bottom: hero.Top + int32(f.px(52))}, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	drawText(hdc, "一键触发 · 智能识别 · 自动安装", rectT{Left: hero.Left + int32(f.px(112)), Top: hero.Top + int32(f.px(58)), Right: hero.Right - int32(f.px(20)), Bottom: hero.Top + int32(f.px(82))}, modernTheme.primaryDark, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)

	cards := f.homeStatCards()
	cardTop := int(hero.Bottom) + f.px(14)
	gap := f.px(10)
	cardW := (right - left - gap*3) / 4
	for i, card := range cards {
		x := left + i*(cardW+gap)
		r := rectT{Left: int32(x), Top: int32(cardTop), Right: int32(x + cardW), Bottom: int32(cardTop + f.px(92))}
		f.paintStatCard(hdc, r, card)
	}

	quickTop := cardTop + f.px(132)
	drawText(hdc, "快速入口", rectT{Left: int32(left), Top: int32(quickTop - f.px(26)), Right: int32(right), Bottom: int32(quickTop - f.px(4))}, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine)
	actions := []string{"新建任务", "模板管理", "日志查看", "设置"}
	for i, text := range actions {
		r := f.homeActionRect(i, clientW, clientH)
		roundRect(hdc, r, modernTheme.surface, modernTheme.border, f.px(uiCardRadius))
		icon := rectT{Left: r.Left + int32(f.px(16)), Top: r.Top + int32(f.px(12)), Right: r.Left + int32(f.px(42)), Bottom: r.Top + int32(f.px(38))}
		roundRect(hdc, icon, modernTheme.primarySoft, modernTheme.primarySoft, f.px(5))
		drawText(hdc, []string{"+", "▣", "≡", "{}"}[i], icon, modernTheme.primaryDark, f.font, dtCenter|dtVCenter|dtSingleLine)
		drawText(hdc, text, rectT{Left: r.Left + int32(f.px(16)), Top: r.Top + int32(f.px(44)), Right: r.Right - int32(f.px(12)), Bottom: r.Bottom - int32(f.px(10))}, modernTheme.primaryDark, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	}
	status, color := "环境状态：尚未检查", modernTheme.muted
	if f.checkingPrereq {
		status = "环境状态：检查中"
	} else if f.prereqChecked {
		if f.prereqReport == nil {
			status, color = "环境状态：未取得有效检测报告，请查看日志", modernTheme.danger
		} else if f.prereqCode == 0 {
			status, color = "环境状态：检查通过", modernTheme.success
			if f.prereqWarnings {
				status, color = "环境状态：检查完成，有提示，详情见日志", modernTheme.warn
			}
		} else {
			status, color = "环境状态：存在依赖或配置问题，详情见日志", modernTheme.danger
		}
	}
	topStatus := f.homeActionRect(0, clientW, clientH).Bottom + int32(f.px(16))
	statusRight := right
	if f.prereqChecked || f.checkingPrereq {
		statusRight -= f.px(130)
	}
	drawText(hdc, status, rectT{Left: int32(left), Top: topStatus, Right: int32(statusRight), Bottom: topStatus + int32(f.px(30))}, color, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	if f.prereqChecked || f.checkingPrereq {
		drawText(hdc, dependencySummary(f.prereqReport, f.checkingPrereq), rectT{Left: int32(left), Top: topStatus + int32(f.px(30)), Right: int32(right), Bottom: topStatus + int32(f.px(56))}, modernTheme.text, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	}
}

func (f *form) paintStatCard(hdc uintptr, r rectT, card statCard) {
	roundRect(hdc, r, modernTheme.surface, modernTheme.border, f.px(uiCardRadius))
	accent := statColor(card.Kind)
	dot := rectT{Left: r.Left + int32(f.px(14)), Top: r.Top + int32(f.px(16)), Right: r.Left + int32(f.px(24)), Bottom: r.Top + int32(f.px(26))}
	roundRect(hdc, dot, accent, accent, f.px(5))
	drawText(hdc, card.Label, rectT{Left: r.Left + int32(f.px(32)), Top: r.Top + int32(f.px(10)), Right: r.Right - int32(f.px(12)), Bottom: r.Top + int32(f.px(32))}, modernTheme.muted, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	drawText(hdc, card.Value, rectT{Left: r.Left + int32(f.px(14)), Top: r.Top + int32(f.px(34)), Right: r.Right - int32(f.px(12)), Bottom: r.Top + int32(f.px(62))}, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	drawText(hdc, card.Detail, rectT{Left: r.Left + int32(f.px(14)), Top: r.Top + int32(f.px(62)), Right: r.Right - int32(f.px(12)), Bottom: r.Bottom - int32(f.px(10))}, modernTheme.muted, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
}

func (f *form) paintTaskSurface(hdc uintptr, clientW, clientH int) {
	r := f.taskSurfaceRect(clientW, clientH)
	roundRect(hdc, r, modernTheme.surface, modernTheme.border, f.px(uiCardRadius))
	f.paintTaskSections(hdc, r)
}

func (f *form) taskSurfaceRect(clientW, clientH int) rectT {
	return rectT{
		Left:   int32(f.px(uiSidebarW) + f.px(uiContentPad)),
		Top:    int32(f.px(uiHeaderH)),
		Right:  int32(clientW - f.px(uiContentPad)),
		Bottom: int32(clientH - f.px(uiContentPad)),
	}
}

func (f *form) taskSectionRects(bounds rectT) []taskSectionLayout {
	left := int(bounds.Left) + f.px(12)
	right := int(bounds.Right) - f.px(12)
	top := int(bounds.Top) + f.px(10)
	bottom := int(bounds.Bottom) - f.px(10)
	gap := f.px(8)
	sampleH := f.px(128)
	paramsH := f.px(118)
	commandH := f.px(58)
	logTop := top + sampleH + gap + paramsH + gap + commandH + gap
	if logTop > bottom-f.px(80) {
		logTop = bottom - f.px(80)
	}
	return []taskSectionLayout{
		{Title: "样本与输出", Rect: rectT{Left: int32(left), Top: int32(top), Right: int32(right), Bottom: int32(top + sampleH)}},
		{Title: "执行参数", Rect: rectT{Left: int32(left), Top: int32(top + sampleH + gap), Right: int32(right), Bottom: int32(top + sampleH + gap + paramsH)}},
		{Title: "等价命令行", Rect: rectT{Left: int32(left), Top: int32(top + sampleH + gap + paramsH + gap), Right: int32(right), Bottom: int32(top + sampleH + gap + paramsH + gap + commandH)}},
		{Title: "运行日志", Rect: rectT{Left: int32(left), Top: int32(logTop), Right: int32(right), Bottom: int32(bottom)}},
	}
}

func (f *form) taskSectionContent(r rectT) rectT {
	return rectT{
		Left:   r.Left + int32(f.px(14)),
		Top:    r.Top + int32(f.px(30)),
		Right:  r.Right - int32(f.px(14)),
		Bottom: r.Bottom - int32(f.px(10)),
	}
}

func (f *form) paintTaskSections(hdc uintptr, bounds rectT) {
	for _, s := range f.taskSectionRects(bounds) {
		if int(s.Rect.Bottom-s.Rect.Top) < f.px(36) {
			continue
		}
		r := s.Rect
		roundRect(hdc, r, modernTheme.surfaceAlt, modernTheme.border, f.px(7))
		drawText(hdc, s.Title, rectT{Left: r.Left + int32(f.px(12)), Top: r.Top + int32(f.px(6)), Right: r.Right - int32(f.px(12)), Bottom: r.Top + int32(f.px(26))}, modernTheme.primaryDark, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	}
}

func (f *form) paintContentPanel(hdc uintptr, clientW, clientH int) rectT {
	r := rectT{
		Left:   int32(f.px(uiSidebarW) + f.px(uiContentPad)),
		Top:    int32(f.px(uiHeaderH)),
		Right:  int32(clientW - f.px(uiContentPad)),
		Bottom: int32(clientH - f.px(uiContentPad)),
	}
	roundRect(hdc, r, modernTheme.surface, modernTheme.border, f.px(uiCardRadius))
	return r
}

func (f *form) paintPlaceholderPage(hdc uintptr, clientW, clientH int) {
	r := f.paintContentPanel(hdc, clientW, clientH)
	summary, ok := f.homeSummary()
	if !ok {
		summary = nil
	}
	switch f.page {
	case pageTemplates:
		f.paintTemplateGrid(hdc, r)
	case pageLogs:
		f.paintLogViewer(hdc, r)
	case pageConfig:
		// Native editor and toolbar occupy this surface.
	case pageSettings:
		if f.settingsRecognition {
			f.paintDependencyStates(hdc, r)
		}
	case pageTaskDetail:
		f.paintTaskDetailPage(hdc, r)
	case pageHelp:
		// Selectable help topics and native text viewer.
	default:
		cards := pageCards(f.page, f.currentOptions(), f.exeDir, f.templates, summary, f.isRunning())
		f.paintInfoCards(hdc, r, cards)
	}
}

func (f *form) paintDependencyStates(hdc uintptr, bounds rectT) {
	left, top := bounds.Left+int32(f.px(30)), bounds.Top+int32(f.px(56+214))
	right := bounds.Right - int32(f.px(30))
	drawText(hdc, "第三方工具", rectT{Left: left, Top: top, Right: right, Bottom: top + int32(f.px(22))}, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine)
}

func (f *form) paintInfoCards(hdc uintptr, bounds rectT, cards []infoCard) {
	if len(cards) == 0 {
		return
	}
	pad := f.px(18)
	gap := f.px(12)
	cols := 2
	if int(bounds.Right-bounds.Left) < f.px(640) {
		cols = 1
	}
	cardW := (int(bounds.Right-bounds.Left) - pad*2 - gap*(cols-1)) / cols
	cardH := f.px(142)
	for i, card := range cards {
		col, row := i%cols, i/cols
		left := int(bounds.Left) + pad + col*(cardW+gap)
		top := int(bounds.Top) + pad + row*(cardH+gap)
		if top+cardH > int(bounds.Bottom)-pad {
			break
		}
		r := rectT{Left: int32(left), Top: int32(top), Right: int32(left + cardW), Bottom: int32(top + cardH)}
		f.paintInfoCard(hdc, r, card)
	}
}

func (f *form) paintInfoCard(hdc uintptr, r rectT, card infoCard) {
	roundRect(hdc, r, modernTheme.surfaceAlt, modernTheme.border, f.px(uiCardRadius))
	accent := statColor(card.Kind)
	if f.page == pageTemplates {
		icon := rectT{Left: r.Right - int32(f.px(42)), Top: r.Top + int32(f.px(12)), Right: r.Right - int32(f.px(16)), Bottom: r.Top + int32(f.px(38))}
		roundRect(hdc, icon, modernTheme.primarySoft, modernTheme.border, f.px(4))
		for i := 0; i < 3; i++ {
			fillSolid(hdc, rectT{Left: icon.Left + int32(f.px(6)), Top: icon.Top + int32(f.px(6+i*5)), Right: icon.Right - int32(f.px(6)), Bottom: icon.Top + int32(f.px(8+i*5))}, accent)
		}
	}
	fillSolid(hdc, rectT{Left: r.Left, Top: r.Top, Right: r.Left + int32(f.px(4)), Bottom: r.Bottom}, accent)
	drawText(hdc, card.Title, rectT{Left: r.Left + int32(f.px(16)), Top: r.Top + int32(f.px(12)), Right: r.Right - int32(f.px(12)), Bottom: r.Top + int32(f.px(36))}, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	y := int(r.Top) + f.px(42)
	lineH := f.px(20)
	for _, line := range card.Lines {
		if y+lineH > int(r.Bottom)-f.px(8) {
			break
		}
		drawText(hdc, line, rectT{Left: r.Left + int32(f.px(16)), Top: int32(y), Right: r.Right - int32(f.px(12)), Bottom: int32(y + lineH)}, modernTheme.muted, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		y += lineH
	}
}

func (f *form) paintTemplateGrid(hdc uintptr, bounds rectT) {
	cards := []infoCard{
		{Title: "合并模板库", Lines: []string{"内置为底，自定义按 ID 覆盖", "默认推荐 · merged"}, Kind: "info"},
		{Title: "内置模板库", Lines: []string{"图像模板：" + strconvI(f.templates.Builtin) + " 个", "关键词 / 静默参数 / 识别规则"}, Kind: "ok"},
		{Title: "自定义模板库", Lines: []string{"图像模板：" + strconvI(f.templates.External) + " 个", "只使用外部模板 · external"}, Kind: "warn"},
	}
	w, h := f.clientSize()
	for i, r := range f.templateCardRects(w, h) {
		if r.Right == 0 {
			continue
		}
		f.paintInfoCard(hdc, r, cards[i])
		label := []string{"合并", "内置", "自定义"}[i]
		color := statColor(cards[i].Kind)
		if f.templates.Mode == []string{"merged", "builtin", "external"}[i] {
			label = "当前使用"
		}
		badge := rectT{Left: r.Right - int32(f.px(88)), Top: r.Top + int32(f.px(76)), Right: r.Right - int32(f.px(14)), Bottom: r.Top + int32(f.px(100))}
		roundRect(hdc, badge, modernTheme.surface, color, f.px(4))
		drawText(hdc, label, badge, color, f.font, dtCenter|dtVCenter|dtSingleLine)
	}
}

func (f *form) paintLogViewer(hdc uintptr, bounds rectT) {
	fileH := f.px(32 + 24*maxInt(1, len(f.logFiles)))
	if fileH > f.px(112) {
		fileH = f.px(112)
	}
	left, top := bounds.Left+int32(f.px(18)), bounds.Bottom-int32(f.px(18)+fileH+f.px(28))
	drawText(hdc, "日志与结果文件", rectT{Left: left, Top: top, Right: bounds.Right - int32(f.px(18)), Bottom: top + int32(f.px(28))}, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine)
}

func (f *form) paintSettingsPage(hdc uintptr, bounds rectT) {
	drawText(hdc, "基础设置与依赖", rectT{Left: bounds.Left + int32(f.px(30)), Top: bounds.Top + int32(f.px(16)), Right: bounds.Right, Bottom: bounds.Top + int32(f.px(44))}, modernTheme.primaryDark, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine)
}

type buttonDrawItem struct {
	Type, ID, Item, Action, State uint32
	Window, DC                    uintptr
	Rect                          rectT
	Data                          uintptr
}

var procSetBkColor = gdi32.NewProc("SetBkColor")

func (f *form) paintNativeButton(item *buttonDrawItem) {
	background, foreground := modernTheme.surfaceAlt, modernTheme.primaryDark
	if item.ID == idRun || item.ID == idCfgSave || item.ID == idTemplateUse || item.ID == idSettingsSave || int(item.ID) == idTemplateAll+f.templateTab {
		background, foreground = modernTheme.primary, modernTheme.surface
	}
	if (item.ID == idSettingsBasic && !f.settingsAdvanced && !f.settingsRecognition) || (item.ID == idSettingsRecognition && f.settingsRecognition) || (item.ID == idSettingsAdvanced && f.settingsAdvanced) {
		background, foreground = modernTheme.primary, modernTheme.surface
	}
	if item.State&1 != 0 {
		background = modernTheme.primarySoft
	}
	if item.State&4 != 0 {
		background, foreground = modernTheme.bg, modernTheme.muted
	}
	roundRect(item.DC, item.Rect, background, modernTheme.border, f.px(5))
	drawText(item.DC, windowText(item.Window), item.Rect, foreground, f.font, dtCenter|dtVCenter|dtSingleLine)
	if item.State&0x10 != 0 {
		r := item.Rect
		r.Left += 3
		r.Top += 3
		r.Right -= 3
		r.Bottom -= 3
		procDrawFocusRect.Call(item.DC, uintptr(unsafe.Pointer(&r)))
	}
}

var procDrawFocusRect = user32.NewProc("DrawFocusRect")

func paintToolIcon(dc uintptr, r rectT) {
	w, h := r.Right-r.Left, r.Bottom-r.Top
	x, y := r.Left, r.Top
	points := []pointT{{X: x + w/2, Y: y + h/6}, {X: x + w*5/6, Y: y + h/3}, {X: x + w*3/4, Y: y + h*2/3}, {X: x + w/2, Y: y + h*5/6}, {X: x + w/4, Y: y + h*2/3}, {X: x + w/6, Y: y + h/3}}
	brush, _, _ := procCreateSolidBrush.Call(modernTheme.surface)
	oldBrush, _, _ := procSelectObject.Call(dc, brush)
	pen, _, _ := procGetStockObject.Call(8) // NULL_PEN
	oldPen, _, _ := procSelectObject.Call(dc, pen)
	gdi32.NewProc("Polygon").Call(dc, uintptr(unsafe.Pointer(&points[0])), uintptr(len(points)))
	procSelectObject.Call(dc, oldPen)
	procSelectObject.Call(dc, oldBrush)
	procDeleteObject.Call(brush)
	for row := int32(0); row < 2; row++ {
		for col := int32(0); col < 2; col++ {
			left, top := x+w/3+col*w/6, y+h/3+row*h/6
			fillSolid(dc, rectT{Left: left, Top: top, Right: left + w/7, Bottom: top + h/7}, modernTheme.primary)
		}
	}
}

func (f *form) paintTaskDetailPage(hdc uintptr, bounds rectT) {
	if snap, ok := f.resultSnapshot(); ok {
		name := snap.TaskID
		var selected taskRecord
		for _, record := range f.history {
			if record.Result == snap.Path {
				name = record.Name
				selected = record
				break
			}
		}
		cards := detailCards(snap, selected)
		pad, gap := f.px(18), f.px(12)
		left, right := bounds.Left+int32(pad), bounds.Right-int32(pad)
		status := schemaOutcomeText(snap)
		if snap.PresentationState != "" {
			status = snap.PresentationState
		}
		if len(snap.Rows) == 1 && snap.Error == "" && snap.PresentationState == "" {
			status = snap.Rows[0].Status
		}
		drawText(hdc, valueOr(name, "任务详情"), rectT{Left: left, Top: bounds.Top + int32(f.px(16)), Right: right - int32(f.px(220)), Bottom: bounds.Top + int32(f.px(42))}, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		drawText(hdc, "任务 ID："+valueOr(snap.TaskID, "未提供"), rectT{Left: left, Top: bounds.Top + int32(f.px(42)), Right: right - int32(f.px(220)), Bottom: bounds.Top + int32(f.px(68))}, modernTheme.muted, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		badge := rectT{Left: right - int32(f.px(210)), Top: bounds.Top + int32(f.px(48)), Right: right, Bottom: bounds.Top + int32(f.px(72))}
		roundRect(hdc, badge, modernTheme.surfaceAlt, statColor(outcomeKind(snap)), f.px(6))
		drawText(hdc, status, badge, statColor(outcomeKind(snap)), f.font, dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
		width := (int(bounds.Right-bounds.Left) - 2*pad - 2*gap) / 3
		for i, card := range cards {
			x := int(left) + i*(width+gap)
			f.paintInfoCard(hdc, rectT{Left: int32(x), Top: bounds.Top + int32(f.px(76)), Right: int32(x + width), Bottom: bounds.Top + int32(f.px(220))}, card)
		}
		banner := rectT{Left: left, Top: bounds.Top + int32(f.px(232)), Right: right, Bottom: bounds.Top + int32(f.px(272))}
		roundRect(hdc, banner, modernTheme.surfaceAlt, statColor(outcomeKind(snap)), f.px(6))
		message := status + " · " + schema.SummaryLine(snap.Summary.ByStatus, snap.Summary.Total)
		if snap.PresentationState != "" {
			message = status + " · 最终结果以执行结束后的 JSON 为准"
		}
		if snap.Error != "" && snap.PresentationState == "" {
			message = "任务失败：" + snap.Error
		}
		banner.Left += int32(f.px(12))
		drawText(hdc, message, banner, statColor(outcomeKind(snap)), f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		return
	}
	cards := pageCards(pageTaskDetail, f.currentOptions(), f.exeDir, f.templates, nil, f.isRunning())
	f.paintInfoCards(hdc, bounds, cards)
}

func (f *form) paintHelpPage(hdc uintptr, bounds rectT) {
	leftPane := rectT{Left: bounds.Left + int32(f.px(18)), Top: bounds.Top + int32(f.px(18)), Right: bounds.Left + int32(f.px(210)), Bottom: bounds.Bottom - int32(f.px(18))}
	roundRect(hdc, leftPane, modernTheme.surfaceAlt, modernTheme.border, f.px(7))
	for i, item := range []string{"1. 快速开始", "2. 任务配置说明", "3. 结果文件", "4. 版本与兼容"} {
		y := int(leftPane.Top) + f.px(18+i*34)
		drawText(hdc, item, rectT{Left: leftPane.Left + int32(f.px(14)), Top: int32(y), Right: leftPane.Right - int32(f.px(10)), Bottom: int32(y + f.px(24))}, modernTheme.primaryDark, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	}
	right := rectT{Left: leftPane.Right + int32(f.px(12)), Top: bounds.Top, Right: bounds.Right, Bottom: bounds.Bottom}
	f.paintInfoCards(hdc, right, pageCards(pageHelp, f.currentOptions(), f.exeDir, f.templates, nil, f.isRunning()))
}

func (f *form) paintResultTable(hdc uintptr, bounds rectT, snap resultSnapshot) {
	pad := f.px(18)
	r := rectT{Left: bounds.Left + int32(pad), Top: bounds.Top + int32(pad), Right: bounds.Right - int32(pad), Bottom: bounds.Bottom - int32(pad)}
	roundRect(hdc, r, modernTheme.surfaceAlt, modernTheme.border, f.px(7))
	drawText(hdc, "结果文件列表", rectT{Left: r.Left + int32(f.px(14)), Top: r.Top + int32(f.px(10)), Right: r.Right - int32(f.px(14)), Bottom: r.Top + int32(f.px(34))}, modernTheme.text, f.statusFontOrDefault(), dtLeft|dtVCenter|dtSingleLine)
	headerTop := int(r.Top) + f.px(42)
	rowH := f.px(30)
	cols := []struct {
		title string
		w     int
	}{{"文件名", 260}, {"类型", 110}, {"状态", 150}, {"置信度", 80}}
	x := int(r.Left) + f.px(14)
	for _, c := range cols {
		drawText(hdc, c.title, rectT{Left: int32(x), Top: int32(headerTop), Right: int32(x + f.px(c.w)), Bottom: int32(headerTop + rowH)}, modernTheme.primaryDark, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		x += f.px(c.w)
	}
	y := headerTop + rowH
	if len(snap.Rows) == 0 {
		drawText(hdc, "结果 JSON 中暂无逐文件记录。", rectT{Left: r.Left + int32(f.px(14)), Top: int32(y), Right: r.Right - int32(f.px(14)), Bottom: int32(y + rowH)}, modernTheme.muted, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		return
	}
	for i, row := range snap.Rows {
		if i >= 8 || y+rowH > int(r.Bottom)-f.px(8) {
			break
		}
		if i%2 == 0 {
			fillSolid(hdc, rectT{Left: r.Left + int32(f.px(10)), Top: int32(y), Right: r.Right - int32(f.px(10)), Bottom: int32(y + rowH)}, modernTheme.surface)
		}
		values := []string{row.File, row.Type, row.Status, row.Confidence}
		x = int(r.Left) + f.px(14)
		for ci, c := range cols {
			drawText(hdc, values[ci], rectT{Left: int32(x), Top: int32(y), Right: int32(x + f.px(c.w)), Bottom: int32(y + rowH)}, modernTheme.muted, f.font, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
			x += f.px(c.w)
		}
		y += rowH
	}
}

func (f *form) navItemRect(index int) rectT {
	x := f.px(10)
	y := f.px(70) + index*f.px(37)
	return rectT{Left: int32(x), Top: int32(y), Right: int32(f.px(uiSidebarW - 10)), Bottom: int32(y + f.px(30))}
}

func (f *form) navPageAt(x, y int) pageID {
	for i, item := range uiNavItems {
		if pointInRect(x, y, f.navItemRect(i)) {
			return item.ID
		}
	}
	return pageNone
}

func (f *form) homeActionRect(index, clientW, clientH int) rectT {
	left := f.px(uiSidebarW) + f.px(uiContentPad)
	right := clientW - f.px(uiContentPad)
	top := f.px(uiHeaderH) + f.px(112) + f.px(14) + f.px(132)
	gap := f.px(10)
	w := (right - left - gap*3) / 4
	if w < f.px(120) {
		w = f.px(120)
	}
	x := left + index*(w+gap)
	bottom := top + f.px(86)
	if bottom > clientH-f.px(uiContentPad) {
		bottom = clientH - f.px(uiContentPad)
	}
	return rectT{Left: int32(x), Top: int32(top), Right: int32(x + w), Bottom: int32(bottom)}
}

func (f *form) homeActionAt(x, y, clientW, clientH int) int {
	if f.page != pageHome {
		return -1
	}
	for i := 0; i < 4; i++ {
		if pointInRect(x, y, f.homeActionRect(i, clientW, clientH)) {
			return i
		}
	}
	return -1
}

func (f *form) homeSummary() (*ResultSummary, bool) {
	p := f.resultPathOrDefault()
	s, ok := ReadResultSummary(p)
	if !ok {
		return nil, false
	}
	return &s, true
}

func (f *form) resultSnapshot() (resultSnapshot, bool) {
	path := f.resultPathOrDefault()
	if f.page == pageTaskDetail && f.selectedTaskRunning() {
		record, _ := f.selectedTaskRecord()
		snap, _ := readResultSnapshot(path)
		snap.TaskID, snap.Path, snap.StartedAt = record.ID, path, record.Created
		snap.FinishedAt = nil
		snap.PresentationState = "安装 / 触发进行中"
		return snap, true
	}
	if snap, ok := readResultSnapshot(path); ok {
		return snap, true
	}
	if f.page == pageTaskDetail {
		for _, record := range f.history {
			if record.Result != path {
				continue
			}
			state := "结果文件暂不可用"
			if record.ID == f.activeTask && f.isRunning() {
				state = "安装 / 触发进行中"
			}
			return resultSnapshot{TaskID: record.ID, Path: path, StartedAt: record.Created, PresentationState: state}, true
		}
	}
	return resultSnapshot{}, false
}

func (f *form) resultPathOrDefault() string {
	if f.selectedResult != "" && (f.page == pageTaskDetail || f.page == pageLogs) {
		return f.selectedResult
	}
	p := strings.TrimSpace(f.resultPath)
	if p == "" {
		p = strings.TrimSpace(windowText(f.ctrls[idOutput]))
	}
	if p == "" {
		p = f.opts.Output
	}
	return p
}

func (f *form) statusFontOrDefault() uintptr {
	if f.statusFont != 0 {
		return f.statusFont
	}
	return f.font
}

func statColor(kind string) uintptr {
	switch kind {
	case "ok":
		return modernTheme.success
	case "fail":
		return modernTheme.danger
	case "run":
		return modernTheme.primary
	case "warn":
		return modernTheme.warn
	default:
		return modernTheme.primaryDark
	}
}

func strconvI(v int) string { return strconv.Itoa(v) }

func configPreviewLines() []string {
	body, err := DefaultConfigJSON()
	if err != nil {
		return []string{"无法生成默认配置：" + err.Error()}
	}
	lines := strings.Split(body, "\n")
	if len(lines) > 10 {
		lines = lines[:10]
	}
	return lines
}

func (f *form) logPreviewLines() []string {
	snap, ok := f.resultSnapshot()
	if ok {
		lines := []string{
			"[INFO] 任务ID：" + valueOr(snap.TaskID, "-"),
			"[INFO] " + schema.SummaryLine(snap.Summary.ByStatus, snap.Summary.Total),
			"[INFO] 结果文件：" + snap.Path,
		}
		if snap.Error != "" {
			lines = append(lines, "[ERROR] "+snap.Error)
		}
		return lines
	}
	return []string{
		"[INFO] 尚未读取到结果 JSON",
		"[INFO] 选择样本后可在任务配置页开始执行",
		"[INFO] 日志会写入结果文件同目录的 .log 文件",
	}
}

func schemaOutcomeText(snap resultSnapshot) string {
	return schema.OutcomeText(snapshotOutcome(snap))
}

func outcomeKind(snap resultSnapshot) string {
	if snap.PresentationState == "安装 / 触发进行中" {
		return "run"
	}
	if snap.PresentationState != "" {
		return "warn"
	}
	switch snapshotOutcome(snap) {
	case schema.OutcomeOK:
		return "ok"
	case schema.OutcomeFailed:
		return "fail"
	case schema.OutcomePartial, schema.OutcomeUnverified, schema.OutcomeSkipped:
		return "warn"
	default:
		return "info"
	}
}

func pointInRect(x, y int, r rectT) bool {
	return int32(x) >= r.Left && int32(x) < r.Right && int32(y) >= r.Top && int32(y) < r.Bottom
}

func fillSolid(hdc uintptr, r rectT, color uintptr) {
	brush, _, _ := procCreateSolidBrush.Call(color)
	if brush == 0 {
		return
	}
	defer procDeleteObject.Call(brush)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
}

func roundRect(hdc uintptr, r rectT, fill, stroke uintptr, radius int) {
	brush, _, _ := procCreateSolidBrush.Call(fill)
	pen, _, _ := procCreatePen.Call(psSolid, 1, stroke)
	if brush == 0 || pen == 0 {
		if brush != 0 {
			procDeleteObject.Call(brush)
		}
		if pen != 0 {
			procDeleteObject.Call(pen)
		}
		return
	}
	oldBrush, _, _ := procSelectObject.Call(hdc, brush)
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	procRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius), uintptr(radius))
	if oldBrush != 0 {
		procSelectObject.Call(hdc, oldBrush)
	}
	if oldPen != 0 {
		procSelectObject.Call(hdc, oldPen)
	}
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}

func drawText(hdc uintptr, text string, r rectT, color, font uintptr, flags uint32) {
	if text == "" {
		return
	}
	procSetBkMode.Call(hdc, transparentBk)
	procSetTextColor.Call(hdc, color)
	oldFont := uintptr(0)
	if font != 0 {
		oldFont, _, _ = procSelectObject.Call(hdc, font)
	}
	u := utf16Slice(text)
	if len(u) > 0 {
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), uintptr(unsafe.Pointer(&r)), uintptr(flags))
	}
	if oldFont != 0 {
		procSelectObject.Call(hdc, oldFont)
	}
}

func mousePoint(lparam unsafe.Pointer) (int, int) {
	v := uintptr(lparam)
	x := int(int16(v & 0xFFFF))
	y := int(int16((v >> 16) & 0xFFFF))
	return x, y
}

func cliVersionText() string {
	if cli.Version == "" {
		return "unknown"
	}
	return cli.Version
}
