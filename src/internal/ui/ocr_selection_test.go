package ui

import (
	"autotrigger/pkg/schema"
	"testing"
)

func TestAvailableOCR(t *testing.T) {
	paddle := schema.OCREngineCheck{Engine: "paddleocr_json", Status: "ok", Path: schema.String(`C:\ocr\paddle.exe`)}
	rapid := schema.OCREngineCheck{Engine: "rapidocr_json", Status: "ok", Path: schema.String(`C:\ocr\rapid.exe`)}
	for _, tt := range []struct {
		name, current, path, want string
		rows                      []schema.OCREngineCheck
	}{
		{"keep working current", "rapidocr_json", *rapid.Path, "rapidocr_json", []schema.OCREngineCheck{paddle, rapid}},
		{"failed current", "paddleocr_json", "auto", "rapidocr_json", []schema.OCREngineCheck{{Engine: "paddleocr_json", Status: "startup_failed"}, rapid}},
		{"preferred fallback", "none", "auto", "paddleocr_json", []schema.OCREngineCheck{rapid, paddle}},
		{"service not started", "none", "auto", "", []schema.OCREngineCheck{{Engine: "umi_ocr", Status: "service_required"}}},
		{"missing executable", "none", "auto", "", []schema.OCREngineCheck{{Engine: "rapidocr_json", Status: "ok"}}},
		{"http selftest passed", "none", "auto", "umi_ocr", []schema.OCREngineCheck{{Engine: "umi_ocr", Status: "ok"}}},
		{"unsupported", "none", "auto", "", []schema.OCREngineCheck{{Engine: "windows", Status: "ok"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := schema.EmptyPrereq()
			r.OCREngines = tt.rows
			got, ok := availableOCR(&r, tt.current, tt.path)
			if ok != (tt.want != "") || got.Engine != tt.want {
				t.Fatalf("got %+v %v, want %s", got, ok, tt.want)
			}
		})
	}
	if _, ok := availableOCR(nil, "none", "auto"); ok {
		t.Fatal("nil report selected engine")
	}
}
