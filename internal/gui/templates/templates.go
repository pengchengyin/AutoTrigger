// Package templates validates, selects and matches grayscale control captures.
package templates

import (
	"autotrigger/internal/config"
	"autotrigger/internal/resources"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type Entry struct {
	ID         string          `json:"id"`
	Family     string          `json:"family"`
	Action     string          `json:"action"`
	Lang       string          `json:"lang"`
	DPI        int             `json:"dpi"`
	Theme      string          `json:"theme"`
	File       string          `json:"file"`
	Width      int             `json:"width"`
	Height     int             `json:"height"`
	Source     string          `json:"source"`
	Screenshot string          `json:"screenshot"`
	Bounds     image.Rectangle `json:"bounds"`
	Origin     string          `json:"origin"`
	Image      *image.Gray     `json:"-"`
	FS         fs.FS           `json:"-"`
}
type Index struct {
	Version string  `json:"resource_pack_version"`
	Status  string  `json:"status"`
	Masters int     `json:"master_count"`
	Entries []Entry `json:"entries"`
}
type Library struct {
	User          UserIndex
	Entries       []Entry
	Masters       int
	BuiltinCount  int
	ExternalCount int
	Warnings      []string
	cfg           config.GUI
	cache         map[string]*image.Gray
}

func Load(c config.GUI, root string) (*Library, error) {
	l := &Library{Entries: []Entry{}, Warnings: []string{}, cfg: c, cache: map[string]*image.Gray{}}
	byID := map[string]int{}
	load := func(fsys fs.FS, source string) error {
		b, err := fs.ReadFile(fsys, "templates.index.json")
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		var index Index
		if err = json.Unmarshal(b, &index); err != nil {
			return err
		}
		if source == "builtin" {
			l.Masters = index.Masters
		}
		for _, e := range index.Entries {
			if !safePath(e.File) || e.ID == "" {
				return fmt.Errorf("unsafe template entry: %s", e.ID)
			}
			e.Source = source
			e.FS = fsys
			if at, ok := byID[e.ID]; ok {
				l.Entries[at] = e
			} else {
				byID[e.ID] = len(l.Entries)
				l.Entries = append(l.Entries, e)
			}
			if source == "builtin" {
				l.BuiltinCount++
			} else {
				l.ExternalCount++
			}
		}
		return nil
	}
	if c.TemplatePack != "external" && c.BuiltinTemplates {
		sub, err := fs.Sub(resources.FS, "assets/builtin/templates")
		if err != nil {
			return nil, err
		}
		if err = load(sub, "builtin"); err != nil {
			return nil, err
		}
	}
	if c.TemplatePack != "builtin" {
		dir := c.TemplateDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		if err := load(os.DirFS(dir), "external"); err != nil {
			return nil, fmt.Errorf("external templates: %w", err)
		}
	}
	if len(l.Entries) == 0 {
		l.Warnings = append(l.Warnings, "内置图像模板为空：仍会使用 UIA/Win32 与 OCR 识别；如需更高命中率，请按 D17 补齐模板")
	}
	if user, err := LoadUserIndex(root); err == nil {
		l.User = user
	} else {
		l.Warnings = append(l.Warnings, "用户模板加载失败，保留原识别流程："+err.Error())
		if _, e := os.Stat(filepath.Join(root, "template-projects", "active.index.json")); e == nil {
			l.User.Managed = true
		}
	}
	if l.User.Managed {
		kept := l.Entries[:0]
		for _, entry := range l.Entries {
			project := strings.SplitN(entry.ID, "__", 2)[0]
			if entry.Source == "external" && strings.HasPrefix(project, "project-") {
				if _, err := os.Stat(filepath.Join(root, "template-projects", project, "project.json")); err == nil {
					continue
				}
			}
			kept = append(kept, entry)
		}
		l.Entries = kept
	}
	return l, nil
}
func safePath(path string) bool { return fs.ValidPath(path) && !strings.Contains(path, ":") }
func (l *Library) Select(dpi int, theme string) ([]Entry, error) {
	if l.cfg.TemplateDPIMode != "auto" {
		fmt.Sscan(l.cfg.TemplateDPIMode, &dpi)
	} else {
		dpi = int(math.Round(float64(dpi) * 100 / 96))
		best := 100
		for _, v := range []int{125, 150, 200} {
			if abs(v-dpi) < abs(best-dpi) {
				best = v
			}
		}
		dpi = best
	}
	if l.cfg.TemplateTheme != "auto" {
		theme = l.cfg.TemplateTheme
	}
	out := []Entry{}
	for _, e := range l.Entries {
		if e.DPI != dpi || e.Theme != theme {
			continue
		}
		key := e.Source + ":" + e.ID
		g, ok := l.cache[key]
		if !ok {
			f, err := e.FS.Open(e.File)
			if err != nil {
				return nil, err
			}
			img, err := png.Decode(f)
			f.Close()
			if err != nil {
				return nil, err
			}
			var valid bool
			g, valid = img.(*image.Gray)
			if !valid {
				return nil, fmt.Errorf("template %s must be 8-bit grayscale", e.ID)
			}
			if len(l.cache) > 64 {
				l.cache = map[string]*image.Gray{}
			}
			l.cache[key] = g
		}
		e.Image = g
		out = append(out, e)
	}
	return out, nil
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func Gray(img image.Image) *image.Gray {
	b := img.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetGray(x, y, color.GrayModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray))
		}
	}
	return out
}

// Match uses sampled mean absolute error as a gate before normalized correlation.
func Match(haystack, needle *image.Gray, min float64) (image.Rectangle, float64, bool) {
	return MatchContext(context.Background(), haystack, needle, min)
}

func MatchContext(ctx context.Context, haystack, needle *image.Gray, min float64) (image.Rectangle, float64, bool) {
	w, h := needle.Bounds().Dx(), needle.Bounds().Dy()
	if w < 1 || h < 1 || w > haystack.Bounds().Dx() || h > haystack.Bounds().Dy() {
		return image.Rectangle{}, 0, false
	}
	best := 0.0
	var rect image.Rectangle
	for y := 0; y <= haystack.Bounds().Dy()-h; y++ {
		for x := 0; x <= haystack.Bounds().Dx()-w; x++ {
			if x%16 == 0 && ctx.Err() != nil {
				return image.Rectangle{}, 0, false
			}
			sum, n := 0, 0
			for sy := 0; sy < h; sy += 4 {
				for sx := 0; sx < w; sx += 4 {
					sum += abs(int(haystack.GrayAt(x+sx, y+sy).Y) - int(needle.GrayAt(sx, sy).Y))
					n++
				}
			}
			if float64(sum)/float64(n) > 42 {
				continue
			}
			score := Similarity(haystack, needle, x, y)
			if score > best {
				best = score
				rect = image.Rect(x, y, x+w, y+h)
				if best > .99999 {
					return rect, best, true
				}
			}
		}
	}
	return rect, best, best >= min
}
func Similarity(a, b *image.Gray, ox, oy int) float64 {
	n := float64(b.Bounds().Dx() * b.Bounds().Dy())
	if n == 0 {
		return 0
	}
	var sa, sb, saa, sbb, sab, mae float64
	for y := 0; y < b.Bounds().Dy(); y++ {
		for x := 0; x < b.Bounds().Dx(); x++ {
			av, bv := float64(a.GrayAt(x+ox, y+oy).Y), float64(b.GrayAt(x, y).Y)
			sa += av
			sb += bv
			saa += av * av
			sbb += bv * bv
			sab += av * bv
			mae += math.Abs(av - bv)
		}
	}
	den := math.Sqrt((saa - sa*sa/n) * (sbb - sb*sb/n))
	if den < .0001 {
		return 1 - mae/(255*n)
	}
	corr := (sab - sa*sb/n) / den
	return math.Max(0, math.Min(corr, 1-mae/(255*n)))
}
func Resize(src *image.Gray, width, height int) *image.Gray {
	dst := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		fy := float64(y) * float64(src.Bounds().Dy()-1) / math.Max(1, float64(height-1))
		y0 := int(fy)
		y1 := int(math.Min(float64(y0+1), float64(src.Bounds().Dy()-1)))
		wy := fy - float64(y0)
		for x := 0; x < width; x++ {
			fx := float64(x) * float64(src.Bounds().Dx()-1) / math.Max(1, float64(width-1))
			x0 := int(fx)
			x1 := int(math.Min(float64(x0+1), float64(src.Bounds().Dx()-1)))
			wx := fx - float64(x0)
			p0 := float64(src.GrayAt(x0, y0).Y)*(1-wx) + float64(src.GrayAt(x1, y0).Y)*wx
			p1 := float64(src.GrayAt(x0, y1).Y)*(1-wx) + float64(src.GrayAt(x1, y1).Y)*wx
			dst.SetGray(x, y, color.Gray{Y: uint8(math.Round(p0*(1-wy) + p1*wy))})
		}
	}
	return dst
}
func (l *Library) SelfTest() ([]string, error) {
	failures := []string{}
	if len(l.Entries) == 0 {
		return failures, fmt.Errorf("templates unavailable: collect real installer masters before template acceptance")
	}
	for _, e := range l.Entries {
		if !safePath(e.Screenshot) {
			failures = append(failures, e.ID+": missing source screenshot")
			continue
		}
		f, err := e.FS.Open(e.File)
		if err != nil {
			failures = append(failures, e.ID+": "+err.Error())
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			failures = append(failures, e.ID+": invalid PNG")
			continue
		}
		needle, ok := img.(*image.Gray)
		if !ok {
			failures = append(failures, e.ID+": not grayscale")
			continue
		}
		f, err = e.FS.Open(e.Screenshot)
		if err != nil {
			failures = append(failures, e.ID+": source screenshot unavailable")
			continue
		}
		shot, err := png.Decode(f)
		f.Close()
		if err != nil {
			failures = append(failures, e.ID+": source PNG invalid")
			continue
		}
		_, score, ok := Match(Gray(shot), needle, .95)
		if !ok {
			failures = append(failures, fmt.Sprintf("%s: source matching %.4f < .95", e.ID, score))
		}
	}
	if len(failures) > 0 {
		return failures, fmt.Errorf("%d templates failed source validation", len(failures))
	}
	return failures, nil
}
