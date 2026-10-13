package gui

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/resources"
	"context"
	"image"
	"testing"
)

type captureOCR struct{ calls int }

func (*captureOCR) Name() string                                 { return "test" }
func (*captureOCR) Start(context.Context) (ports.OCRInfo, error) { return ports.OCRInfo{}, nil }
func (*captureOCR) Stop() error                                  { return nil }
func (e *captureOCR) Recognize(context.Context, image.Image) ([]ports.OCRText, error) {
	e.calls++
	return []ports.OCRText{{Text: "下一步", Score: 1, Box: [4]ports.Point{{X: 20, Y: 20}, {X: 100, Y: 20}, {X: 100, Y: 40}, {X: 20, Y: 40}}}}, nil
}
func TestOCRFallbackWithoutSemanticUIAControls(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		n := &native{controls: []ports.Control{{ID: "close", Text: "关闭", Role: "button", Enabled: true}}}
		if disabled {
			n.controls = append(n.controls, ports.Control{ID: "next", Text: "下一步", Role: "button", Enabled: false, Bounds: ports.Rect{Left: 20, Top: 20, Right: 100, Bottom: 40}})
		}
		e := &captureOCR{}
		a := Automator{Native: n, Engine: e, Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
		if err := a.Init(); err != nil {
			t.Fatal(err)
		}
		u, err := a.Step(context.Background(), []int{1})
		if err != nil || e.calls != 1 {
			t.Fatal(err, e.calls)
		}
		if disabled && len(u.Actions) != 0 {
			t.Fatal("OCR bypassed disabled native button")
		}
		if !disabled && (len(u.Actions) != 1 || u.Actions[0].Source != "ocr") {
			t.Fatal(u)
		}
	}
}
