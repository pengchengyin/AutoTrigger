package ocr

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"context"
	"fmt"
	"image"
	"testing"
)

type hiddenRunner struct{ commands []ports.Command }
type completedProbe struct {
	ports.Process
	done chan ports.Exit
}

func (p completedProbe) Done() <-chan ports.Exit { return p.done }
func (p completedProbe) Close() error            { return nil }
func (r *hiddenRunner) Start(_ context.Context, c ports.Command) (ports.Process, error) {
	r.commands = append(r.commands, c)
	if c.Stdin != nil {
		return nil, fmt.Errorf("test stops before starting persistent engine")
	}
	text := ""
	if c.Args[0] == "--version" {
		text = "tesseract 5\n"
	} else if c.Args[0] == "--list-langs" {
		text = "eng\n"
	} else {
		text = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"
	}
	if _, err := c.Stdout.Write([]byte(text)); err != nil {
		return nil, err
	}
	done := make(chan ports.Exit, 1)
	done <- ports.Exit{}
	return completedProbe{done: done}, nil
}

func TestOCRCommandsHideConsole(t *testing.T) {
	for _, name := range []string{"paddleocr_json", "rapidocr_json", "tesseract"} {
		t.Run(name, func(t *testing.T) {
			cfg := config.Defaults().OCR
			cfg.Engine = name
			cfg.Transport = "stdio"
			runner := &hiddenRunner{}
			engine := &Engine{Config: cfg, Path: "ocr.exe", Runner: runner}
			defer engine.Stop()
			_, err := engine.Start(context.Background())
			if name == "tesseract" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = engine.Recognize(context.Background(), image.NewGray(image.Rect(0, 0, 32, 24))); err != nil {
					t.Fatal(err)
				}
			}
			if len(runner.commands) == 0 {
				t.Fatal("no command issued")
			}
			for _, cmd := range runner.commands {
				if !cmd.HideWindow {
					t.Fatal("OCR console not hidden", cmd.Args)
				}
			}
		})
	}
}

func TestNativeConfidenceAndBoxParsing(t *testing.T) {
	data := []byte(`{"code":100,"data":[{"text":"安装","box":[[10,20],[80,20],[80,40],[10,40]],"score":0.94},{"text":"bad","box":[[1,1]],"score":1}]}`)
	got, err := Parse(data)
	if err != nil || len(got) != 1 || got[0].Score != .94 || got[0].Box[2].Y != 40 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err = Parse([]byte(`{"code":200,"data":"missing"}`)); err == nil {
		t.Fatal("engine failure swallowed")
	}
	if got, err = Parse([]byte(`{"code":101,"data":[]}`)); err != nil || len(got) != 0 {
		t.Fatal("empty recognition mishandled")
	}
}
func TestTesseractTSV(t *testing.T) {
	got, err := ParseTSV("level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n5\t1\t1\t1\t1\t1\t10\t20\t30\t15\t90\tInstall\n")
	if err != nil || len(got) != 1 || got[0].Score != .9 || got[0].Text != "Install" {
		t.Fatalf("%+v %v", got, err)
	}
}
