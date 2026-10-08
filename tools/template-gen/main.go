// template-gen derives variants exclusively from validated real installer captures.
package main

import (
	"autotrigger/internal/gui/templates"
	"autotrigger/internal/output"
	"autotrigger/internal/resources"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type source struct {
	Screenshot string                                 `json:"screenshot"`
	Bounds     struct{ Left, Top, Right, Bottom int } `json:"bounds"`
	DPI        int                                    `json:"dpi"`
	Theme      string                                 `json:"theme"`
	Installer  string                                 `json:"installer"`
	Origin     string                                 `json:"capture_origin"`
}

func main() {
	masters := flag.String("masters", "masters", "real installer master captures")
	out := flag.String("out", "internal/resources/assets/builtin/templates", "generated template directory")
	release := flag.Bool("release", false, "require >=50 masters and >=180 variants")
	flag.Parse()
	if err := generate(*masters, *out, *release); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func generate(root, dest string, release bool) error {
	index := templates.Index{Version: resources.Version, Status: "source_validated", Entries: []templates.Entry{}}
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".png") && !strings.Contains(filepath.ToSlash(path), "/screenshots/") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no real installer captures in %s; see docs/图像模板制作说明.md", root)
	}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".png")
		parts := strings.Split(name, "__")
		if len(parts) != 5 || parts[3] != "100" || parts[4] != "light" {
			return fmt.Errorf("invalid master name: %s", name)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			return err
		}
		master, ok := img.(*image.Gray)
		if !ok {
			return fmt.Errorf("master %s must be 8-bit grayscale", name)
		}
		w, h := master.Bounds().Dx(), master.Bounds().Dy()
		if w < 24 || w > 240 || h < 16 || h > 48 {
			return fmt.Errorf("master %s dimensions outside 24..240 x 16..48", name)
		}
		b, err := os.ReadFile(strings.TrimSuffix(path, ".png") + ".source.json")
		if err != nil {
			return fmt.Errorf("master %s requires .source.json provenance: %w", name, err)
		}
		var meta source
		if err = json.Unmarshal(b, &meta); err != nil {
			return err
		}
		if meta.DPI != 100 || meta.Theme != "light" || meta.Origin != "real_installer" || meta.Installer == "" {
			return fmt.Errorf("master %s requires real installer, 100%% DPI, light theme provenance", name)
		}
		if !safe(meta.Screenshot) {
			return fmt.Errorf("unsafe screenshot path: %s", meta.Screenshot)
		}
		f, err = os.Open(filepath.Join(root, filepath.FromSlash(meta.Screenshot)))
		if err != nil {
			return err
		}
		screenshot, err := png.Decode(f)
		f.Close()
		if err != nil {
			return err
		}
		shot := templates.Gray(screenshot)
		r := image.Rect(meta.Bounds.Left, meta.Bounds.Top, meta.Bounds.Right, meta.Bounds.Bottom)
		if r.Dx() != w || r.Dy() != h || !r.In(shot.Bounds()) {
			return fmt.Errorf("master %s source bounds mismatch", name)
		}
		crop := templates.Gray(shot.SubImage(r))
		if templates.Similarity(crop, master, 0, 0) < .95 {
			return fmt.Errorf("master %s does not match its source crop", name)
		}
		index.Masters++
		for _, dpi := range []int{100, 125, 150, 200} {
			for _, theme := range []string{"light", "dark", "win7classic"} {
				width, height := int(math.Round(float64(w)*float64(dpi)/100)), int(math.Round(float64(h)*float64(dpi)/100))
				variant := themeImage(templates.Resize(master, width, height), theme)
				id := strings.Join([]string{parts[0], parts[1], parts[2], fmt.Sprint(dpi), theme}, "__")
				file := parts[0] + "/" + id + ".png"
				sourceFile := "sources/" + id + ".png"
				scaledShot := themeImage(templates.Resize(shot, int(math.Round(float64(shot.Bounds().Dx())*float64(dpi)/100)), int(math.Round(float64(shot.Bounds().Dy())*float64(dpi)/100))), theme)
				x, y := int(math.Round(float64(r.Min.X)*float64(dpi)/100)), int(math.Round(float64(r.Min.Y)*float64(dpi)/100))
				transformedBounds := image.Rect(x, y, x+width, y+height)
				if !transformedBounds.In(scaledShot.Bounds()) {
					return fmt.Errorf("variant bounds outside source: %s", id)
				}
				for yy := 0; yy < height; yy++ {
					for xx := 0; xx < width; xx++ {
						scaledShot.SetGray(x+xx, y+yy, variant.GrayAt(xx, yy))
					}
				}
				if err = writePNG(filepath.Join(dest, filepath.FromSlash(file)), variant); err != nil {
					return err
				}
				if err = writePNG(filepath.Join(dest, filepath.FromSlash(sourceFile)), scaledShot); err != nil {
					return err
				}
				origin := "real_installer_crop"
				if dpi != 100 || theme != "light" {
					origin = "derived_variant_requires_target_environment_validation"
				}
				index.Entries = append(index.Entries, templates.Entry{ID: id, Family: parts[0], Action: parts[1], Lang: parts[2], DPI: dpi, Theme: theme, File: file, Width: width, Height: height, Source: "builtin", Screenshot: sourceFile, Bounds: transformedBounds, Origin: origin})
			}
		}
	}
	if release && (index.Masters < 50 || len(index.Entries) < 180) {
		return fmt.Errorf("release resource minimum unmet: %d masters, %d variants", index.Masters, len(index.Entries))
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	if err = output.Atomic(filepath.Join(dest, "templates.index.json"), data); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "generated %d masters, %d variants; target DPI/theme validation still required\n", index.Masters, len(index.Entries))
	return nil
}
func safe(path string) bool {
	return path != "" && !filepath.IsAbs(path) && !strings.Contains(path, "..") && !strings.Contains(path, ":")
}
func themeImage(src *image.Gray, theme string) *image.Gray {
	dst := image.NewGray(src.Bounds())
	copy(dst.Pix, src.Pix)
	for i, p := range dst.Pix {
		if theme == "dark" {
			dst.Pix[i] = 255 - p
		} else if theme == "win7classic" {
			dst.Pix[i] = uint8(int(p) / 16 * 16)
		}
	}
	if theme == "win7classic" {
		b := dst.Bounds()
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.SetGray(x, b.Min.Y, color.Gray{Y: 255})
			dst.SetGray(x, b.Max.Y-1, color.Gray{Y: 64})
		}
	}
	return dst
}
func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
