//go:build windows

package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

func (p *templateMakerPane) refreshProjects(selected string) {
	projects, err := listTemplateProjects(p.owner.exeDir)
	if err != nil {
		p.status(err.Error())
		return
	}
	p.projects = projects
	send(p.controls[makerWindows], 0x14b, 0, 0)
	index := 0
	for i, project := range projects {
		state := "草稿"
		if project.PublishedAt != nil {
			state = "已发布"
			if project.PublishedDigest != projectDigest(p.owner.exeDir, project) {
				state = "已发布 · 草稿待更新"
			}
		}
		send(p.controls[makerWindows], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr(project.Name+" · "+state))))
		if project.ID == selected {
			index = i
		}
	}
	send(p.controls[makerWindows], 0x14e, uintptr(index), 0)
	p.selectProject()
}

func (p *templateMakerPane) selectProject() {
	i := int(send(p.controls[makerWindows], 0x147, 0, 0))
	if i < 0 || i >= len(p.projects) {
		return
	}
	p.project = p.projects[i]
	setWindowTextRaw(p.controls[makerName], p.project.Name)
	setWindowTextRaw(p.controls[makerDir], projectDir(p.owner.exeDir, p.project))
	p.shot, p.pixels, p.bounds = nil, nil, image.Rectangle{}
	p.updateBoundsFields()
	p.refreshProjectImages()
	p.refreshVersions()
	p.refreshCaptureRecords()
	procInvalidateRect.Call(p.hwnd, 0, 0)
}

func (p *templateMakerPane) refreshCaptureRecords() {
	send(p.controls[makerRecords], 0x14b, 0, 0)
	index, err := readCaptureIndex(projectDir(p.owner.exeDir, p.project), p.project.ID)
	p.captures = nil
	if err != nil {
		p.status("捕获记录读取失败：" + err.Error())
		return
	}
	p.captures = index.Items
	for _, item := range index.Items {
		state := "草稿"
		if !item.Confirmed {
			state = "会话未确认"
		}
		label := item.Name + " · " + item.Rule.Method + " · " + state
		send(p.controls[makerRecords], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr(label))))
	}
	if len(index.Items) == 0 {
		send(p.controls[makerRecords], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr("暂无连续捕获记录"))))
	}
	send(p.controls[makerRecords], 0x14e, 0, 0)
}

func (p *templateMakerPane) previewCaptureRecord() {
	i := int(send(p.controls[makerRecords], 0x147, 0, 0))
	if i < 0 || i >= len(p.captures) {
		return
	}
	item := p.captures[i]
	shot, err := captureImage(projectDir(p.owner.exeDir, p.project), item)
	if err != nil {
		p.status("无法查看捕获图片：" + err.Error())
		return
	}
	p.setImage(shot, item.Name)
	p.capturePreview = true
	p.layout()
	enableWindow(p.controls[makerSave], false)
	p.status(fmt.Sprintf("%s · %s\r\n窗口：%s\r\n区域：%d,%d - %d,%d · DPI：%d\r\n规则：%+v", item.Name, item.Rule.Method, item.Window.Title, item.Bounds.Left, item.Bounds.Top, item.Bounds.Right, item.Bounds.Bottom, item.Window.DPI, item.Rule))
}

func (p *templateMakerPane) refreshVersions() {
	send(p.controls[makerVersions], 0x14b, 0, 0)
	releases, err := listTemplateReleases(p.publishedTarget(), p.project.ID)
	p.releases = releases
	if err != nil {
		p.status("版本列表读取失败：" + err.Error())
	}
	for _, release := range releases {
		label := fmt.Sprintf("%s · %d 张 · %s", release.CreatedAt.Local().Format("2006-01-02 15:04:05"), len(release.Entries), release.Version)
		send(p.controls[makerVersions], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr(label))))
	}
	if len(releases) == 0 {
		send(p.controls[makerVersions], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr("暂无可回退的发布版本"))))
	}
	send(p.controls[makerVersions], 0x14e, 0, 0)
	enableWindow(p.controls[makerRollback], err == nil && len(releases) > 0)
}

func (p *templateMakerPane) rollbackProject() {
	i := int(send(p.controls[makerVersions], 0x147, 0, 0))
	if i < 0 || i >= len(p.releases) {
		return
	}
	release := p.releases[i]
	if err := restoreTemplateRelease(p.publishedTarget(), p.project.ID, release.Version, p.owner.isRunning()); err != nil {
		p.status("回退失败：" + err.Error())
		return
	}
	now := time.Now()
	p.project.PublishedAt = &now
	// An old release is independent of the current draft, which remains editable.
	p.project.PublishedDigest = "release:" + release.Version
	if err := writeProject(p.owner.exeDir, p.project); err != nil {
		p.status("版本已切换，但项目状态保存失败：" + err.Error())
		return
	}
	p.owner.reloadTemplates()
	p.refreshProjects(p.project.ID)
	p.status("已恢复发布版本 " + release.Version + "；当前草稿未改变，后续任务使用恢复的资源。")
}

func (p *templateMakerPane) newProject() {
	name := p.value(makerName)
	if p.project.ID != "" && name == p.project.Name {
		name = fmt.Sprintf("%s %d", name, len(p.projects)+1)
	}
	if name == "" {
		name = fmt.Sprintf("模板 %d", len(p.projects)+1)
	}
	project, err := newTemplateProject(p.owner.exeDir, name)
	if err != nil {
		p.status(err.Error())
		return
	}
	p.refreshProjects(project.ID)
	setWindowTextRaw(p.controls[makerID], "")
	p.autoID = ""
	p.refreshID()
	enableWindow(p.controls[makerCapture], true)
}

func (p *templateMakerPane) refreshProjectImages() {
	send(p.controls[makerImages], 0x14b, 0, 0)
	p.entries = nil
	if p.project.ID != "" && fileExists(filepath.Join(projectDir(p.owner.exeDir, p.project), "templates.index.json")) {
		lib, err := loadEditableTemplateDir(projectDir(p.owner.exeDir, p.project))
		if err != nil {
			p.status(err.Error())
			return
		}
		p.entries = lib.Entries
		for _, e := range p.entries {
			send(p.controls[makerImages], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr(e.ID+" · "+e.Action))))
		}
	}
	if len(p.entries) == 0 {
		send(p.controls[makerImages], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr("暂无可发布图片"))))
	}
	send(p.controls[makerImages], 0x14e, 0, 0)
	enableWindow(p.controls[makerBrowse], len(p.entries) > 0)
	enableWindow(p.controls[makerDeleteImage], len(p.entries) > 0)
	enableWindow(p.controls[makerWithdraw], p.project.PublishedAt != nil)
	p.status(fmt.Sprintf("%s · %d 张图片", p.project.Name, len(p.entries)))
	enableWindow(p.controls[makerCapture], p.project.ID != "")
	if len(p.entries) > 0 {
		p.previewProjectImage()
	}
}

func (p *templateMakerPane) deleteProjectImage() {
	i := int(send(p.controls[makerImages], 0x147, 0, 0))
	if i < 0 || i >= len(p.entries) {
		return
	}
	dir := projectDir(p.owner.exeDir, p.project)
	unlock, err := acquireHistoryLock(dir)
	if err != nil {
		p.status(err.Error())
		return
	}
	defer unlock()
	path := filepath.Join(dir, "templates.index.json")
	b, err := os.ReadFile(path)
	if err != nil {
		p.status(err.Error())
		return
	}
	var doc map[string]json.RawMessage
	if err = json.Unmarshal(b, &doc); err != nil {
		p.status(err.Error())
		return
	}
	var entries []json.RawMessage
	if err = json.Unmarshal(doc["entries"], &entries); err != nil {
		p.status(err.Error())
		return
	}
	kept := []json.RawMessage{}
	for _, raw := range entries {
		var e struct {
			ID string `json:"id"`
		}
		if err = json.Unmarshal(raw, &e); err != nil {
			p.status(err.Error())
			return
		}
		if e.ID != p.entries[i].ID {
			kept = append(kept, raw)
		}
	}
	doc["entries"], _ = json.Marshal(kept)
	b, err = json.MarshalIndent(doc, "", "  ")
	if err == nil {
		err = atomicTemplateFile(path, b)
	}
	if err != nil {
		p.status(err.Error())
		return
	}
	p.refreshProjectImages()
	p.status("已从草稿删除图片，已发布版本保持不变。")
}

func (p *templateMakerPane) previewProjectImage() {
	i := int(send(p.controls[makerImages], 0x147, 0, 0))
	if i < 0 || i >= len(p.entries) {
		return
	}
	e := p.entries[i]
	f, err := os.Open(filepath.Join(projectDir(p.owner.exeDir, p.project), filepath.FromSlash(e.Screenshot)))
	if err != nil {
		p.status(err.Error())
		return
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		p.status(err.Error())
		return
	}
	p.setImage(img, e.Origin)
	p.bounds = e.Bounds
	p.updateBoundsFields()
	enableWindow(p.controls[makerSave], false)
	p.status(fmt.Sprintf("已保存图片：%s · %s", e.ID, e.Action))
}

func (p *templateMakerPane) publishedTarget() string {
	dir := p.owner.templates.TemplateDir
	if dir == "" {
		dir = "templates"
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(p.owner.exeDir, dir)
	}
	return dir
}

func (p *templateMakerPane) publishProject() {
	p.project.Name = p.value(makerName)
	if err := writeProject(p.owner.exeDir, p.project); err != nil {
		p.status(err.Error())
		return
	}
	if p.owner.isRunning() {
		p.status("请等待当前任务结束后发布")
		return
	}
	body, path, _, err := ConfigEditorBody(windowText(p.owner.ctrls[idConfig]), p.owner.exeDir)
	if err != nil {
		p.status(err.Error())
		return
	}
	target := p.publishedTarget()
	body, err = templateConfigBody(body, "merged", target)
	if err != nil {
		p.status(err.Error())
		return
	}
	var draft string
	if p.owner.configOpen {
		draft, err = templateConfigBody(windowText(p.owner.ctrls[idCfgEditor]), "merged", target)
		if err != nil {
			p.status("高级配置草稿无效：" + err.Error())
			return
		}
	}
	if _, err = ValidateConfigJSONBody(body, p.owner.exeDir); err != nil {
		p.status(err.Error())
		return
	}
	if err = publishTemplateProject(p.owner.exeDir, target, p.project); err != nil {
		p.status("发布失败：" + err.Error())
		return
	}
	if _, err = WriteConfigEditorBody(path, body, p.owner.exeDir); err != nil {
		p.status("图片已发布，但启用配置失败：" + err.Error())
		return
	}
	if draft != "" {
		setWindowTextRaw(p.owner.ctrls[idCfgEditor], toCRLF(draft))
	}
	setWindowTextRaw(p.owner.ctrls[idConfig], path)
	now := time.Now()
	p.project.PublishedAt = &now
	p.project.PublishedDigest = projectDigest(p.owner.exeDir, p.project)
	if err = writeProject(p.owner.exeDir, p.project); err != nil {
		p.status("发布成功，但状态记录失败：" + err.Error())
		return
	}
	p.owner.reloadTemplates()
	p.refreshProjects(p.project.ID)
	p.status("已发布，后续任务自动参与匹配；新截图需再次发布。")
}

func (p *templateMakerPane) withdrawProject() {
	if p.owner.isRunning() {
		p.status("请等待当前任务结束后取消发布")
		return
	}
	target := p.publishedTarget()
	unlock, err := acquireHistoryLock(target)
	if err != nil {
		p.status(err.Error())
		return
	}
	defer unlock()
	path := filepath.Join(target, "templates.index.json")
	b, err := os.ReadFile(path)
	if err != nil {
		p.status(err.Error())
		return
	}
	var doc map[string]json.RawMessage
	if err = json.Unmarshal(b, &doc); err != nil {
		p.status(err.Error())
		return
	}
	var entries []json.RawMessage
	if err = json.Unmarshal(doc["entries"], &entries); err != nil {
		p.status(err.Error())
		return
	}
	kept := []json.RawMessage{}
	for _, raw := range entries {
		var entry struct {
			ID string `json:"id"`
		}
		if err = json.Unmarshal(raw, &entry); err != nil {
			p.status(err.Error())
			return
		}
		if !strings.HasPrefix(entry.ID, p.project.ID+"__") {
			kept = append(kept, raw)
		}
	}
	doc["entries"], _ = json.Marshal(kept)
	b, err = json.MarshalIndent(doc, "", "  ")
	if err == nil {
		err = atomicTemplateFile(path, b)
	}
	if err != nil {
		p.status(err.Error())
		return
	}
	p.project.PublishedAt = nil
	p.project.PublishedDigest = ""
	if err = writeProject(p.owner.exeDir, p.project); err != nil {
		p.status(err.Error())
		return
	}
	p.owner.reloadTemplates()
	p.refreshProjects(p.project.ID)
	p.status("已取消发布，草稿和图片保留。")
}
