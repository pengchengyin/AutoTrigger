//go:build windows

package ui

import (
	"autotrigger/internal/gui/templates"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	makerWindows = 101 + iota
	makerRefresh
	makerCapture
	makerLoad
	makerID
	makerFamily
	makerAction
	makerLanguage
	makerDPI
	makerTheme
	makerX
	makerY
	makerWidth
	makerHeight
	makerDir
	makerBrowse
	makerSave
	makerStatus
	makerName
	makerImages
	makerWithdraw
	makerDeleteImage
	makerVersions
	makerRollback
	makerRecords
)

type templateMakerPane struct {
	hub                           bool
	owner                         *form
	hwnd                          uintptr
	controls                      map[int]uintptr
	labels                        map[int]uintptr
	shot                          image.Image
	pixels                        []byte
	display                       image.Rectangle
	bounds                        image.Rectangle
	anchor                        image.Point
	dragging, updating, capturing bool
	autoID, origin                string
	projects                      []templateProject
	project                       templateProject
	entries                       []templates.Entry
	releases                      []templateRelease
	captures                      []draftCapture
	capturePreview                bool
}

var makerPanes = map[uintptr]*templateMakerPane{}
var makerClassOnce sync.Once
var makerClassName = utf16Ptr("AutoTriggerTemplateMaker")

func (f *form) showTemplateMaker() {
	f.createTemplateMaker(true)
}

func (f *form) createTemplateMaker(show bool) {
	if f.templateMaker != 0 {
		if show {
			procShowWindow.Call(f.templateMaker, swShow)
			user32.NewProc("SetForegroundWindow").Call(f.templateMaker)
		}
		return
	}
	makerClassOnce.Do(func() {
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
		wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: syscall.NewCallback(templateMakerProc), Instance: f.instance, Cursor: cursor, Background: colorWindow + 1, ClassName: makerClassName}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
	h, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(makerClassName)), uintptr(unsafe.Pointer(utf16Ptr("制作模板"))), wsOverlappedWindow|wsClipChildren, cwUseDefault, cwUseDefault, uintptr(f.px(1040)), uintptr(f.px(720)), f.hwnd, 0, f.instance, 0)
	if h == 0 {
		f.templateNotice("无法打开模板制作窗口")
		return
	}
	p := &templateMakerPane{owner: f, hwnd: h, controls: map[int]uintptr{}, labels: map[int]uintptr{}}
	makerPanes[h], f.templateMaker = p, h
	add := func(id int, class, text string, style uint32) uintptr {
		c, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(wsChild|wsVisible|style), 0, 0, 0, 0, h, uintptr(id), f.instance, 0)
		send(c, wmSetFont, f.font, 1)
		return c
	}
	for _, id := range []int{makerRefresh, makerCapture, makerLoad, makerBrowse, makerSave, makerWithdraw, makerDeleteImage, makerRollback} {
		text := map[int]string{makerRefresh: "新建模板", makerCapture: "开始捕获", makerLoad: "载入 PNG", makerBrowse: "发布", makerSave: "保存图片", makerWithdraw: "取消发布", makerDeleteImage: "删除图片", makerRollback: "回退版本"}[id]
		p.controls[id] = add(id, "BUTTON", text, wsTabStop|0xB)
	}
	p.controls[makerWindows] = add(makerWindows, "COMBOBOX", "", wsTabStop|cbsDropDownList|wsVScroll)
	p.controls[makerName] = add(makerName, "EDIT", "新模板", wsTabStop|wsBorder|esAutoHScroll)
	send(p.controls[makerName], 0x1501, 1, uintptr(unsafe.Pointer(utf16Ptr("模板名称"))))
	p.controls[makerImages] = add(makerImages, "COMBOBOX", "", wsTabStop|cbsDropDownList|wsVScroll)
	p.controls[makerVersions] = add(makerVersions, "COMBOBOX", "", wsTabStop|cbsDropDownList|wsVScroll)
	p.controls[makerRecords] = add(makerRecords, "COMBOBOX", "", wsTabStop|cbsDropDownList|wsVScroll)
	for _, id := range []int{makerID, makerFamily, makerAction, makerLanguage, makerDPI, makerTheme, makerX, makerY, makerWidth, makerHeight, makerDir} {
		label := map[int]string{makerID: "模板 ID", makerFamily: "安装器家族", makerAction: "按钮动作", makerLanguage: "界面语言", makerDPI: "截图 DPI", makerTheme: "截图主题", makerX: "X", makerY: "Y", makerWidth: "宽度", makerHeight: "高度", makerDir: "保存目录"}[id]
		p.labels[id] = add(0, "STATIC", label, 0)
		if id == makerID || id == makerDir || id >= makerX && id <= makerHeight {
			p.controls[id] = add(id, "EDIT", "", wsTabStop|wsBorder|esAutoHScroll)
		} else {
			p.controls[id] = add(id, "COMBOBOX", "", wsTabStop|cbsDropDownList|wsVScroll)
		}
	}
	for id, items := range map[int][]string{
		makerFamily:   {"common", "inno", "nsis", "installshield", "msi", "wixburn", "advinstaller", "installaware", "setupfactory", "wise", "smartinstall", "electron", "gamelauncher"},
		makerAction:   {"next · 下一步", "install · 安装", "accept · 接受协议", "finish · 完成", "yes · 是", "ok · 确定", "browse · 浏览", "close · 关闭", "checkbox_off · 未勾选", "checkbox_on · 已勾选", "pathinput · 路径输入", "dropdown · 下拉", "cancel · 取消（仅记录）", "uninstall · 卸载（仅记录）", "no · 否（仅记录）", "install_disabled · 安装禁用态（仅记录）"},
		makerLanguage: {"zh-CN", "en-US"}, makerDPI: {"100", "125", "150", "200"}, makerTheme: {"light · 浅色", "dark · 深色", "win7classic · Win7 经典"},
	} {
		for _, s := range items {
			send(p.controls[id], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr(s))))
		}
		send(p.controls[id], 0x14e, 0, 0)
	}
	p.controls[makerStatus] = add(makerStatus, "EDIT", "", wsBorder|esMultiline|esReadOnly|wsVScroll)
	dir := f.templates.TemplateDir
	if dir == "" {
		dir = f.templateRoot()
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(f.dataLayout().Root, dir)
	}
	setWindowTextRaw(p.controls[makerDir], dir)
	p.refreshID()
	p.refreshProjects("")
	if len(p.projects) == 0 {
		p.newProject()
	}
	p.updateBoundsFields()
	p.refreshProjectImages()
	p.layout()
	if f.appIcon != 0 {
		send(h, 0x80, 0, f.appIcon)
		send(h, 0x80, 1, f.appIcon)
	}
	if show {
		procShowWindow.Call(h, swShow)
		procUpdateWindow.Call(h)
	}
}

func (p *templateMakerPane) status(s string) {
	setWindowTextRaw(p.controls[makerStatus], s)
	if p.hub {
		p.owner.setStatus(strings.ReplaceAll(s, "\r\n", " · "))
	}
}
func (p *templateMakerPane) value(id int) string {
	return strings.TrimSpace(windowText(p.controls[id]))
}
func (p *templateMakerPane) token(id int) string { return strings.Split(p.value(id), " · ")[0] }
func (p *templateMakerPane) refreshID() {
	current := p.value(makerID)
	next := strings.Join([]string{p.token(makerFamily), p.token(makerAction), p.token(makerLanguage), p.token(makerDPI), p.token(makerTheme)}, "__") + "__" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if current == "" || current == p.autoID {
		setWindowTextRaw(p.controls[makerID], next)
		p.autoID = next
	}
}

func (p *templateMakerPane) setImage(img image.Image, origin string) {
	p.capturePreview = false
	p.shot, p.origin, p.bounds = img, origin, image.Rectangle{}
	b := img.Bounds()
	p.pixels = make([]byte, b.Dx()*b.Dy()*4)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := (y*b.Dx() + x) * 4
			p.pixels[i], p.pixels[i+1], p.pixels[i+2], p.pixels[i+3] = byte(bl>>8), byte(g>>8), byte(r>>8), 255
		}
	}
	p.updateBoundsFields()
	p.layout()
	p.status(fmt.Sprintf("%s\r\n截图：%d × %d 像素", origin, b.Dx(), b.Dy()))
}

func (p *templateMakerPane) loadPNG() {
	path, ok := openFileDialogWith(p.hwnd, "选择真实安装器截图", "", []string{"PNG 截图 (*.png)", "*.png"})
	if !ok {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		p.status(err.Error())
		return
	}
	defer f.Close()
	dim, err := png.DecodeConfig(f)
	if err != nil || dim.Width < 1 || dim.Height < 1 || dim.Width > 10000 || dim.Height > 10000 || dim.Width*dim.Height > 25000000 {
		p.status("PNG 无效或尺寸过大（最多 2500 万像素）")
		return
	}
	if _, err = f.Seek(0, 0); err != nil {
		p.status(err.Error())
		return
	}
	img, err := png.Decode(f)
	if err != nil {
		p.status(err.Error())
		return
	}
	p.setImage(img, "原始截图："+path)
}

func (p *templateMakerPane) updateBoundsFields() {
	p.updating = true
	for id, v := range map[int]int{makerX: p.bounds.Min.X, makerY: p.bounds.Min.Y, makerWidth: p.bounds.Dx(), makerHeight: p.bounds.Dy()} {
		setWindowTextRaw(p.controls[id], strconv.Itoa(v))
	}
	p.updating = false
	enableWindow(p.controls[makerSave], p.shot != nil && !p.bounds.Empty() && !p.capturing)
	procInvalidateRect.Call(p.hwnd, 0, 0)
}

func (p *templateMakerPane) readBounds() (image.Rectangle, error) {
	vals := []int{}
	for _, id := range []int{makerX, makerY, makerWidth, makerHeight} {
		v, err := strconv.Atoi(p.value(id))
		if err != nil || v < 0 || v > 10000 {
			return image.Rectangle{}, fmt.Errorf("裁剪坐标和尺寸必须是 0~10000 的整数")
		}
		vals = append(vals, v)
	}
	return image.Rect(vals[0], vals[1], vals[0]+vals[2], vals[1]+vals[3]), nil
}

func (p *templateMakerPane) save() {
	b, err := p.readBounds()
	if err != nil {
		p.status(err.Error())
		return
	}
	dpi, _ := strconv.Atoi(p.token(makerDPI))
	e := templates.Entry{ID: p.value(makerID), Family: p.token(makerFamily), Action: p.token(makerAction), Lang: p.token(makerLanguage), DPI: dpi, Theme: p.token(makerTheme), Origin: p.origin}
	dir := projectDir(p.owner.templateRoot(), p.project)
	if p.project.ID == "" {
		p.status("请先新建模板")
		return
	}
	p.project.Name = p.value(makerName)
	if err = writeProject(p.owner.templateRoot(), p.project); err != nil {
		p.status(err.Error())
		return
	}
	if err = saveCapturedTemplate(dir, p.shot, b, e); err != nil {
		p.status("保存失败：" + err.Error())
		return
	}
	p.refreshProjects(p.project.ID)
	p.refreshProjectImages()
	setWindowTextRaw(p.controls[makerID], "")
	p.autoID = ""
	p.refreshID()
	p.status(fmt.Sprintf("已保存图片，当前模板共 %d 张。发布后用于后续任务。", len(p.entries)))
}

func (p *templateMakerPane) layout() {
	var r rectT
	procGetClientRect.Call(p.hwnd, uintptr(unsafe.Pointer(&r)))
	px := p.owner.px
	w, h := int(r.Right), int(r.Bottom)
	pad := px(16)
	side := px(300)
	left := w - side - pad
	move := func(c uintptr, x, y, width, height int) {
		procMoveWindow.Call(c, uintptr(maxInt(0, x)), uintptr(maxInt(0, y)), uintptr(maxInt(1, width)), uintptr(maxInt(1, height)), 1)
	}
	move(p.controls[makerWindows], pad, pad, px(230), px(280))
	move(p.controls[makerName], pad+px(240), pad, maxInt(px(80), w-2*pad-px(576)), px(30))
	for i, id := range []int{makerRefresh, makerCapture, makerLoad} {
		move(p.controls[id], w-pad-px(328)+i*px(112), pad, px(104), px(30))
	}
	row := 0
	for _, id := range []int{makerID, makerFamily, makerAction, makerLanguage, makerDPI, makerTheme} {
		y := px(60 + row*52)
		move(p.labels[id], left, y, side, px(18))
		height := px(27)
		if id != makerID {
			height = px(230)
		}
		move(p.controls[id], left, y+px(20), side, height)
		row++
	}
	for i, id := range []int{makerX, makerY, makerWidth, makerHeight} {
		x := left + (i%2)*px(154)
		y := px(382 + (i/2)*52)
		move(p.labels[id], x, y, px(140), px(18))
		move(p.controls[id], x, y+px(20), px(140), px(27))
	}
	move(p.labels[makerDir], left, px(488), side, px(18))
	move(p.controls[makerDir], left, px(510), side, px(28))
	send(p.controls[makerDir], 0xcf, 1, 0)
	move(p.controls[makerSave], left, px(550), px(140), px(32))
	move(p.controls[makerBrowse], left+px(152), px(550), px(148), px(32))
	move(p.controls[makerWithdraw], left, px(590), px(140), px(28))
	move(p.controls[makerRollback], left+px(152), px(590), px(148), px(28))
	move(p.controls[makerVersions], pad, h-px(120), maxInt(1, left-pad-px(16)), px(180))
	move(p.controls[makerImages], pad, px(58), maxInt(1, left-pad-px(120)), px(220))
	move(p.controls[makerRecords], pad, px(94), maxInt(1, left-pad-px(16)), px(220))
	move(p.controls[makerDeleteImage], left-px(112), px(58), px(96), px(28))
	move(p.controls[makerStatus], pad, h-px(84), maxInt(1, left-pad-px(16)), px(68))
	if p.shot != nil {
		area := image.Rect(pad, px(134), left-pad, h-px(136))
		p.display = templateImageRect(area, p.shot.Bounds().Size())
		if p.capturePreview && p.display.Dx() > p.shot.Bounds().Dx() {
			size := p.shot.Bounds().Size()
			center := area.Min.Add(area.Size().Div(2))
			p.display = image.Rectangle{Min: center.Sub(size.Div(2)), Max: center.Sub(size.Div(2)).Add(size)}
		}
	}
	procInvalidateRect.Call(p.hwnd, 0, 1)
}

type makerBitmapInfo struct {
	Size                   uint32
	Width, Height          int32
	Planes, Bits           uint16
	Compression, ImageSize uint32
	XPels, YPels           int32
	Used, Important        uint32
}

func (p *templateMakerPane) paint() {
	var ps paintStruct
	dc, _, _ := procBeginPaint.Call(p.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(p.hwnd, uintptr(unsafe.Pointer(&ps)))
	var r rectT
	procGetClientRect.Call(p.hwnd, uintptr(unsafe.Pointer(&r)))
	mem, _, _ := gdi32.NewProc("CreateCompatibleDC").Call(dc)
	if mem != 0 {
		bitmap, _, _ := gdi32.NewProc("CreateCompatibleBitmap").Call(dc, uintptr(r.Right), uintptr(r.Bottom))
		if bitmap != 0 {
			old, _, _ := procSelectObject.Call(mem, bitmap)
			target := dc
			dc = mem
			defer func() {
				d := ps.RcPaint
				gdi32.NewProc("BitBlt").Call(target, uintptr(d.Left), uintptr(d.Top), uintptr(d.Right-d.Left), uintptr(d.Bottom-d.Top), mem, uintptr(d.Left), uintptr(d.Top), 0xcc0020)
				procSelectObject.Call(mem, old)
				procDeleteObject.Call(bitmap)
				gdi32.NewProc("DeleteDC").Call(mem)
			}()
		} else {
			gdi32.NewProc("DeleteDC").Call(mem)
		}
	}
	fillSolid(dc, r, rgb(245, 247, 250))
	if p.shot == nil {
		drawText(dc, "尚未采集截图", rectT{Left: int32(p.owner.px(30)), Top: int32(p.owner.px(100)), Right: r.Right - int32(p.owner.px(340)), Bottom: int32(p.owner.px(150))}, rgb(95, 105, 120), p.owner.font, dtCenter)
		return
	}
	b := p.shot.Bounds()
	d := p.display
	if d.Empty() {
		return
	}
	info := makerBitmapInfo{Size: 40, Width: int32(b.Dx()), Height: -int32(b.Dy()), Planes: 1, Bits: 32}
	gdi32.NewProc("StretchDIBits").Call(dc, uintptr(d.Min.X), uintptr(d.Min.Y), uintptr(d.Dx()), uintptr(d.Dy()), 0, 0, uintptr(b.Dx()), uintptr(b.Dy()), uintptr(unsafe.Pointer(&p.pixels[0])), uintptr(unsafe.Pointer(&info)), 0, 0xcc0020)
	if selection := p.bounds.Intersect(image.Rect(0, 0, b.Dx(), b.Dy())); !selection.Empty() {
		x := d.Min.X + selection.Min.X*d.Dx()/b.Dx()
		y := d.Min.Y + selection.Min.Y*d.Dy()/b.Dy()
		br := rectT{Left: int32(x), Top: int32(y), Right: int32(d.Min.X + selection.Max.X*d.Dx()/b.Dx()), Bottom: int32(d.Min.Y + selection.Max.Y*d.Dy()/b.Dy())}
		brush, _, _ := procCreateSolidBrush.Call(rgb(20, 110, 230))
		user32.NewProc("FrameRect").Call(dc, uintptr(unsafe.Pointer(&br)), brush)
		procDeleteObject.Call(brush)
	}
}

func templateMakerProc(h uintptr, msg uint32, w uintptr, l unsafe.Pointer) uintptr {
	p := makerPanes[h]
	if p != nil {
		switch msg {
		case wmSize:
			p.layout()
			return 0
		case 0x24:
			// Keep all form fields accessible at the minimum window size.
			v := (*[10]int32)(l)
			v[6], v[7] = int32(p.owner.px(820)), int32(p.owner.px(680))
			return 0
		case wmPaint:
			p.paint()
			return 0
		case wmEraseBkgnd:
			return 1
		case 0x2B:
			if l != nil {
				item := *(*buttonDrawItem)(l)
				id := item.ID
				item.ID = 0
				if int(id) == makerSave {
					item.ID = idCfgSave
				}
				p.owner.paintNativeButton(&item)
				return 1
			}
		case wmLButtonDown:
			x, y := mousePoint(l)
			point := image.Pt(x, y)
			if p.shot != nil && point.In(p.display) {
				p.anchor = templateImagePoint(point, p.display, p.shot.Bounds().Size())
				p.dragging = true
				p.bounds = image.Rectangle{}
				user32.NewProc("SetCapture").Call(h)
			}
			return 0
		case wmMouseMove, 0x202:
			if p.dragging {
				x, y := mousePoint(l)
				point := templateImagePoint(image.Pt(x, y), p.display, p.shot.Bounds().Size())
				p.bounds = image.Rectangle{Min: p.anchor, Max: point}.Canon()
				p.updateBoundsFields()
				if msg == 0x202 {
					p.dragging = false
					user32.NewProc("ReleaseCapture").Call()
					p.status(fmt.Sprintf("已框选：%d × %d 像素", p.bounds.Dx(), p.bounds.Dy()))
				}
			}
			return 0
		case 0x215:
			p.dragging = false
			return 0
		case 0x111:
			id, code := int(w&0xffff), int(w>>16)
			if id == 2 && code == 0 {
				procDestroyWindow.Call(h)
				return 0
			}
			if code == 0 {
				switch id {
				case makerRefresh:
					p.newProject()
				case makerCapture:
					p.startHoverCapture()
				case makerLoad:
					p.loadPNG()
				case makerBrowse:
					p.publishProject()
				case makerWithdraw:
					p.withdrawProject()
				case makerRollback:
					p.rollbackProject()
				case makerDeleteImage:
					p.deleteProjectImage()
				case makerSave:
					p.save()
				}
			}
			if code == cbnSelChange && id >= makerFamily && id <= makerTheme {
				p.refreshID()
			}
			if code == cbnSelChange && id == makerWindows {
				p.selectProject()
			}
			if code == cbnSelChange && id == makerImages {
				p.previewProjectImage()
			}
			if code == cbnSelChange && id == makerRecords {
				p.previewCaptureRecord()
			}
			if code == enChange && id >= makerX && id <= makerHeight && !p.updating {
				if b, err := p.readBounds(); err == nil {
					p.bounds = b
					p.status(fmt.Sprintf("已框选：%d × %d 像素", b.Dx(), b.Dy()))
					enableWindow(p.controls[makerSave], p.shot != nil && !b.Empty() && !p.capturing)
					procInvalidateRect.Call(h, 0, 0)
				}
			}
			return 0
		case wmCtlColorStatic, wmCtlColorEdit:
			procSetBkMode.Call(w, opaqueBk)
			procSetBkColor.Call(w, 0xffffff)
			procSetTextColor.Call(w, 0x202020)
			return sysColorBrush(colorWindow)
		case 0x82:
			if p.owner.templateMaker == h {
				p.owner.templateMaker = 0
			}
			delete(makerPanes, h)
		}
	}
	r, _, _ := procDefWindowProcW.Call(h, uintptr(msg), w, uintptr(l))
	return r
}
