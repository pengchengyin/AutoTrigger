package templates

import (
	"context"
	"image"
	"image/color"
	"testing"
)

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
