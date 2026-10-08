//go:build windows

package ocr_test

import (
	"autotrigger/internal/config"
	"autotrigger/internal/ocr"
	"autotrigger/internal/platform/windows"
	"context"
	"image"
	"os"
	"testing"
	"time"
)

func TestOfflineEngineIntegration(t *testing.T) {
	path := os.Getenv("AT_OCR_TEST_PATH")
	if path == "" {
		t.Skip("set AT_OCR_TEST_PATH to a fully deployed offline engine")
	}
	cfg := config.Defaults().OCR
	cfg.Engine = os.Getenv("AT_OCR_TEST_ENGINE")
	if cfg.Engine == "" {
		cfg.Engine = "rapidocr_json"
	}
	if transport := os.Getenv("AT_OCR_TEST_TRANSPORT"); transport != "" {
		cfg.Transport = transport
	}
	cfg.StartupTimeoutMS = 30000
	engine := &ocr.Engine{Config: cfg, Path: path, Runner: &windows.Runner{}}
	defer engine.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	info, err := engine.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != path || info.StartupMS <= 0 {
		t.Fatalf("invalid startup report: %+v", info)
	}
	img := image.NewGray(image.Rect(0, 0, 240, 80))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for i := 0; i < 2; i++ {
		if _, err := engine.Recognize(ctx, img); err != nil {
			t.Fatalf("recognition %d: %v", i, err)
		}
	}
	t.Logf("%s/%s startup=%dms, repeated recognition passed", cfg.Engine, cfg.Transport, info.StartupMS)
}
