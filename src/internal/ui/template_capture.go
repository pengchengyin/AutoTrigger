package ui

import (
	"autotrigger/internal/core/ports"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type captureRule struct {
	Action        string  `json:"action,omitempty"`
	Method        string  `json:"method"`
	AutomationID  string  `json:"automation_id,omitempty"`
	ControlType   int32   `json:"control_type,omitempty"`
	Name          string  `json:"name,omitempty"`
	ClassName     string  `json:"class_name,omitempty"`
	MinSimilarity float64 `json:"min_similarity,omitempty"`
}

type draftCapture struct {
	ID           string         `json:"id"`
	ProjectID    string         `json:"project_id"`
	SessionID    string         `json:"session_id"`
	Name         string         `json:"name"`
	CreatedAt    time.Time      `json:"created_at"`
	Confirmed    bool           `json:"confirmed"`
	Window       ports.Window   `json:"window"`
	Bounds       ports.Rect     `json:"screen_bounds"`
	Control      *ports.Control `json:"control,omitempty"`
	Visible      *bool          `json:"visible,omitempty"`
	Rule         captureRule    `json:"rule"`
	File         string         `json:"file"`
	SHA256       string         `json:"sha256"`
	SourceFile   string         `json:"source_file,omitempty"`
	SourceSHA256 string         `json:"source_sha256,omitempty"`
	SourceBounds *ports.Rect    `json:"source_bounds,omitempty"`
}

type captureIndex struct {
	Version   int            `json:"version"`
	ProjectID string         `json:"project_id"`
	Items     []draftCapture `json:"items"`
}

type captureSession struct {
	root    string
	project templateProject
	id      string
	ids     []string
	closed  bool
}

func captureID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func newCaptureSession(root string, p templateProject) (*captureSession, error) {
	// Use the existing validation without rewriting the project metadata.
	if filepath.Base(p.ID) != p.ID || p.ID == "" || p.ID == "." || p.ID == ".." || !fs.ValidPath(p.ID) {
		return nil, fmt.Errorf("模板标识无效")
	}
	b, err := os.ReadFile(filepath.Join(projectDir(root, p), "project.json"))
	if err != nil {
		return nil, err
	}
	var stored templateProject
	if err = json.Unmarshal(b, &stored); err != nil {
		return nil, err
	}
	if stored.ID != p.ID {
		return nil, fmt.Errorf("模板项目不一致")
	}
	id, err := captureID()
	if err != nil {
		return nil, err
	}
	return &captureSession{root: root, project: p, id: id}, nil
}

func readCaptureIndex(dir, projectID string) (captureIndex, error) {
	index := captureIndex{Version: 1, ProjectID: projectID, Items: []draftCapture{}}
	b, err := os.ReadFile(filepath.Join(dir, "captures.index.json"))
	if os.IsNotExist(err) {
		return index, nil
	}
	if err != nil {
		return index, err
	}
	if err = json.Unmarshal(b, &index); err != nil {
		return index, err
	}
	if index.Version != 1 || index.ProjectID != projectID {
		return index, fmt.Errorf("捕获清单格式或所属模板无效")
	}
	seen := map[string]bool{}
	for _, item := range index.Items {
		if seen[item.ID] || !captureHex(item.ID, 32) || !captureHex(item.SessionID, 32) || !captureHex(item.SHA256, 64) || item.ProjectID != projectID || !fs.ValidPath(item.File) || item.File != "captures/"+item.ID+"/image.png" || (item.Rule.Method != "uia" && item.Rule.Method != "image") {
			return index, fmt.Errorf("捕获条目无效：%s", item.ID)
		}
		seen[item.ID] = true
	}
	return index, nil
}

func captureHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (s *captureSession) save(item draftCapture, shot image.Image) (draftCapture, error) {
	return s.saveWithSource(item, shot, nil)
}

func (s *captureSession) saveWithSource(item draftCapture, shot, source image.Image) (draftCapture, error) {
	if s.closed {
		return item, fmt.Errorf("捕获会话已结束")
	}
	if shot == nil || shot.Bounds().Dx() != item.Bounds.Width() || shot.Bounds().Dy() != item.Bounds.Height() || item.Bounds.Width() < 1 || item.Bounds.Height() < 1 || int64(item.Bounds.Width())*int64(item.Bounds.Height()) > 25000000 {
		return item, fmt.Errorf("截图为空、尺寸不一致或过大")
	}
	if item.Rule.Method != "uia" && item.Rule.Method != "image" {
		return item, fmt.Errorf("识别方式无效")
	}
	if item.Rule.Method == "uia" && (item.Control == nil || item.Control.Source != "uia" || item.Control.Bounds != item.Bounds) {
		return item, fmt.Errorf("缺少可靠 UIA 控件信息")
	}
	if item.Rule.Method == "image" && (item.Rule.MinSimilarity <= 0 || item.Rule.MinSimilarity > 1) {
		return item, fmt.Errorf("图像匹配阈值无效")
	}
	var original bytes.Buffer
	if source != nil {
		b := item.SourceBounds
		if b == nil || b.Width() != source.Bounds().Dx() || b.Height() != source.Bounds().Dy() || b.Width() < 1 || b.Height() < 1 || int64(b.Width())*int64(b.Height()) > 25000000 || item.Bounds.Left < b.Left || item.Bounds.Top < b.Top || item.Bounds.Right > b.Right || item.Bounds.Bottom > b.Bottom {
			return item, fmt.Errorf("原图与标记范围不一致")
		}
		if err := png.Encode(&original, source); err != nil {
			return item, err
		}
	} else {
		item.SourceBounds = nil
		item.SourceFile = ""
		item.SourceSHA256 = ""
	}
	dir := projectDir(s.root, s.project)
	unlock, err := acquireHistoryLock(dir)
	if err != nil {
		return item, err
	}
	defer unlock()
	index, err := readCaptureIndex(dir, s.project.ID)
	if err != nil {
		return item, err
	}
	id, err := captureID()
	if err != nil {
		return item, err
	}
	item.ID, item.ProjectID, item.SessionID, item.CreatedAt, item.Confirmed = id, s.project.ID, s.id, time.Now().UTC(), false
	if item.Name == "" {
		item.Name = "区域 " + fmt.Sprint(len(index.Items)+1)
	}
	item.File = "captures/" + id + "/image.png"
	if source != nil {
		item.SourceFile = "captures/" + id + "/source.png"
		item.SourceSHA256 = fmt.Sprintf("%x", sha256.Sum256(original.Bytes()))
	}
	var data bytes.Buffer
	if err = png.Encode(&data, shot); err != nil {
		return item, err
	}
	item.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data.Bytes()))
	asset := filepath.Join(dir, "captures", id)
	if err = os.MkdirAll(filepath.Dir(asset), 0755); err != nil {
		return item, err
	}
	// Exclusive creation prevents even an ID collision from replacing old pixels.
	if err = os.Mkdir(asset, 0700); err != nil {
		return item, fmt.Errorf("无法创建独立捕获目录，不覆盖已有资源：%w", err)
	}
	if err = atomicTemplateFile(filepath.Join(asset, "image.png"), data.Bytes()); err != nil {
		os.RemoveAll(asset)
		return item, err
	}
	if source != nil {
		if err = atomicTemplateFile(filepath.Join(asset, "source.png"), original.Bytes()); err != nil {
			os.RemoveAll(asset)
			return item, err
		}
	}
	index.Items = append(index.Items, item)
	b, err := json.MarshalIndent(index, "", "  ")
	if err == nil {
		err = atomicTemplateFile(filepath.Join(dir, "captures.index.json"), b)
	}
	if err != nil {
		os.RemoveAll(asset)
		return item, err
	}
	s.ids = append(s.ids, id)
	return item, nil
}

// Change only items owned by this session, under the same draft lock as saves.
func (s *captureSession) change(ids []string, confirm bool) error {
	dir := projectDir(s.root, s.project)
	unlock, err := acquireHistoryLock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	index, err := readCaptureIndex(dir, s.project.ID)
	if err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	kept := make([]draftCapture, 0, len(index.Items))
	var removed []string
	for _, item := range index.Items {
		if wanted[item.ID] && item.SessionID == s.id {
			if !confirm {
				removed = append(removed, item.ID)
				continue
			}
			item.Confirmed = true
		}
		kept = append(kept, item)
	}
	index.Items = kept
	b, err := json.MarshalIndent(index, "", "  ")
	if err == nil {
		err = atomicTemplateFile(filepath.Join(dir, "captures.index.json"), b)
	}
	if err != nil {
		return err
	}
	for _, id := range removed {
		_ = os.RemoveAll(filepath.Join(dir, "captures", id))
	}
	return nil
}

func (s *captureSession) undo() error {
	if s.closed {
		return fmt.Errorf("捕获会话已结束")
	}
	if len(s.ids) == 0 {
		return fmt.Errorf("本次会话没有可撤销的捕获")
	}
	if err := s.change(s.ids[len(s.ids)-1:], false); err != nil {
		return err
	}
	s.ids = s.ids[:len(s.ids)-1]
	return nil
}

func (s *captureSession) finish(cancel bool) error {
	if s.closed {
		return nil
	}
	if err := s.change(s.ids, !cancel); err != nil {
		return err
	}
	s.closed = true
	return nil
}

func captureImage(dir string, item draftCapture) (image.Image, error) {
	if item.SourceFile != "" || item.SourceBounds != nil || item.SourceSHA256 != "" {
		if _, err := captureSource(dir, item); err != nil {
			return nil, err
		}
	}
	if !fs.ValidPath(item.File) || item.File != "captures/"+item.ID+"/image.png" {
		return nil, fmt.Errorf("捕获资源路径无效")
	}
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(item.File)))
	if err != nil {
		return nil, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != item.SHA256 {
		return nil, fmt.Errorf("捕获图片完整性校验失败")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 25000000 {
		return nil, fmt.Errorf("捕获图片无效或过大")
	}
	return png.Decode(bytes.NewReader(b))
}

func captureSource(dir string, item draftCapture) (image.Image, error) {
	bounds := item.SourceBounds
	if item.SourceFile != "captures/"+item.ID+"/source.png" || !captureHex(item.SourceSHA256, 64) || bounds == nil {
		return nil, fmt.Errorf("原图索引无效")
	}
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(item.SourceFile)))
	if err != nil {
		return nil, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != item.SourceSHA256 {
		return nil, fmt.Errorf("原图完整性校验失败")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width != bounds.Width() || cfg.Height != bounds.Height() || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 25000000 || item.Bounds.Left < bounds.Left || item.Bounds.Top < bounds.Top || item.Bounds.Right > bounds.Right || item.Bounds.Bottom > bounds.Bottom {
		return nil, fmt.Errorf("原图尺寸或标记范围无效")
	}
	return png.Decode(bytes.NewReader(b))
}

func capturePreview(dir string, item draftCapture) (image.Image, error) {
	if _, err := captureImage(dir, item); err != nil {
		return nil, err
	}
	if item.SourceFile != "" {
		return captureSource(dir, item)
	}
	return captureImage(dir, item)
}
