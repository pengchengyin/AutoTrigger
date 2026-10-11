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
	procInvalidateRect.Call(p.hwnd, 0, 0)
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
		send(p.controls[makerImages], 0x143, 0, uintptr(unsafe.Pointer(utf16Ptr("尚未保存标记图片"))))
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
