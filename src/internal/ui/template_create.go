package ui

import (
	"autotrigger/internal/gui/templates"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// Save new captures separately; publish the index only after both PNGs exist.
func saveCapturedTemplate(dir string, shot image.Image, bounds image.Rectangle, entry templates.Entry) (err error) {
	if shot == nil || bounds.Empty() || !bounds.In(shot.Bounds()) {
		return fmt.Errorf("请在截图范围内框选完整按钮")
	}
	if bounds.Dx() < 24 || bounds.Dx() > 240 || bounds.Dy() < 16 || bounds.Dy() > 48 {
		return fmt.Errorf("按钮尺寸应为 24~240 × 16~48 像素，当前为 %d × %d", bounds.Dx(), bounds.Dy())
	}
	if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Family) == "" || strings.TrimSpace(entry.Action) == "" || strings.TrimSpace(entry.Lang) == "" {
		return fmt.Errorf("请填写模板 ID、安装器家族、动作和语言")
	}
	if entry.DPI != 100 && entry.DPI != 125 && entry.DPI != 150 && entry.DPI != 200 {
		return fmt.Errorf("DPI 必须是 100、125、150 或 200")
	}
	if entry.Theme != "light" && entry.Theme != "dark" && entry.Theme != "win7classic" {
		return fmt.Errorf("请选择有效主题")
	}
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("请选择保存目录")
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	unlock, err := acquireHistoryLock(dir)
	if err != nil {
		return fmt.Errorf("模板目录无法锁定：%w", err)
	}
	defer unlock()
	indexPath := filepath.Join(dir, "templates.index.json")
	original, readErr := os.ReadFile(indexPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	doc := map[string]json.RawMessage{}
	var entries []json.RawMessage
	if readErr == nil {
		if _, err = loadEditableTemplateDir(dir); err != nil {
			return fmt.Errorf("原模板库无效，不覆盖：%w", err)
		}
		if err = json.Unmarshal(original, &doc); err != nil {
			return err
		}
		if err = json.Unmarshal(doc["entries"], &entries); err != nil {
			return err
		}
	}
	for _, raw := range entries {
		var existing templates.Entry
		if err = json.Unmarshal(raw, &existing); err != nil {
			return err
		}
		if existing.ID == entry.ID {
			return fmt.Errorf("模板 ID 已存在，请修改 ID，原模板不会被覆盖")
		}
	}
	assets, err := os.MkdirTemp(dir, "capture-")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(assets)
		}
	}()
	writePNG := func(name string, img image.Image) error {
		var b bytes.Buffer
		if e := png.Encode(&b, img); e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(assets, name), b.Bytes(), 0600)
	}
	gray := templates.Gray(shot)
	if err = writePNG("button.png", templates.Gray(gray.SubImage(bounds.Sub(shot.Bounds().Min)))); err != nil {
		return err
	}
	if err = writePNG("source.png", shot); err != nil {
		return err
	}
	entry.File = filepath.Base(assets) + "/button.png"
	entry.Screenshot = filepath.Base(assets) + "/source.png"
	entry.Bounds, entry.Width, entry.Height = bounds, bounds.Dx(), bounds.Dy()
	entry.Source, entry.Image, entry.FS = "external", nil, nil
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	entries = append(entries, raw)
	doc["entries"], err = json.Marshal(entries)
	if err != nil {
		return err
	}
	if len(doc["resource_pack_version"]) == 0 {
		doc["resource_pack_version"] = json.RawMessage(`"0.1.0"`)
	}
	doc["status"] = json.RawMessage(`"user_captured"`)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".templates-index-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	current, currentErr := os.ReadFile(indexPath)
	if (currentErr != nil && !os.IsNotExist(currentErr)) || !bytes.Equal(current, original) {
		return fmt.Errorf("模板索引在保存期间发生变化，请重新保存")
	}
	return os.Rename(tmp.Name(), indexPath)
}

func templateImageRect(area image.Rectangle, size image.Point) image.Rectangle {
	if area.Empty() || size.X <= 0 || size.Y <= 0 {
		return image.Rectangle{}
	}
	scale := float64(area.Dx()) / float64(size.X)
	if other := float64(area.Dy()) / float64(size.Y); other < scale {
		scale = other
	}
	w, h := int(float64(size.X)*scale), int(float64(size.Y)*scale)
	x, y := area.Min.X+(area.Dx()-w)/2, area.Min.Y+(area.Dy()-h)/2
	return image.Rect(x, y, x+w, y+h)
}

func templateImagePoint(p image.Point, display image.Rectangle, size image.Point) image.Point {
	if display.Empty() {
		return image.Point{}
	}
	x := (p.X - display.Min.X) * size.X / display.Dx()
	y := (p.Y - display.Min.Y) * size.Y / display.Dy()
	if x < 0 {
		x = 0
	}
	if x > size.X {
		x = size.X
	}
	if y < 0 {
		y = 0
	}
	if y > size.Y {
		y = size.Y
	}
	return image.Pt(x, y)
}
