package templates

import (
	"context"
	"image"
	"image/color"
	"testing"
	"time"
)

func TestLowContrastCheckboxWithinBudget(t *testing.T) {
	screen := image.NewGray(image.Rect(0, 0, 566, 409))
	for i := range screen.Pix {
		screen.Pix[i] = 240
	}
	needle := image.NewGray(image.Rect(0, 0, 34, 37))
	for y := 0; y < 37; y++ {
		for x := 0; x < 34; x++ {
			v := uint8(240)
			if x >= 7 && x <= 27 && y >= 9 && y <= 29 && (x <= 8 || x >= 26 || y <= 10 || y >= 28) {
				v = 20
			}
			needle.SetGray(x, y, color.Gray{Y: v})
			screen.SetGray(x+36, y+267, color.Gray{Y: v})
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r, score, ok := MatchContext(ctx, screen, needle, .95)
	if !ok || score < .99999 || r != image.Rect(36, 267, 70, 304) {
		t.Fatalf("checkbox lost: %v %.4f %t %v", r, score, ok, ctx.Err())
	}
}

func TestMatchAndCancellation(t *testing.T) {
	screen := image.NewGray(image.Rect(0, 0, 240, 100))
	for i := range screen.Pix {
		screen.Pix[i] = 255
	}
	needle := image.NewGray(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			value := color.Gray{Y: uint8((x*11 + y*23) % 230)}
			needle.SetGray(x, y, value)
			screen.SetGray(x+120, y+40, value)
		}
	}
	r, score, ok := Match(screen, needle, .95)
	if !ok || score < .999 || r != image.Rect(120, 40, 160, 60) {
		t.Fatalf("match=%v score=%v ok=%t", r, score, ok)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, ok := MatchContext(ctx, screen, needle, .95); ok {
		t.Fatal("cancelled matching returned a click candidate")
	}
	if _, _, ok := Match(needle, screen, .95); ok {
		t.Fatal("oversized template matched")
	}
}

func TestResizePreservesDimensions(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 40, 20))
	for i := range src.Pix {
		src.Pix[i] = 127
	}
	dst := Resize(src, 80, 40)
	if dst.Bounds() != image.Rect(0, 0, 80, 40) || dst.GrayAt(50, 30).Y != 127 {
		t.Fatal("resize changed constant image or dimensions")
	}
}
