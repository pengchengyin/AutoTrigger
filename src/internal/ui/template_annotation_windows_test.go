//go:build windows

package ui

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/core/ports"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWindowsAnnotationCancelAndOriginalPixelSave(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := newForm(cli.Dependencies{})
	f.exeDir = t.TempDir()
	f.dataDir = t.TempDir()
	f.dataError = nil
	if err := f.create(); err != nil {
		t.Fatal(err)
	}
	defer procDestroyWindow.Call(f.hwnd)
	p, err := newTemplateProject(f.templateRoot(), "标记测试")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newCaptureSession(f.templateRoot(), p)
	if err != nil {
		t.Fatal(err)
	}
	s := &hoverCapture{pane: &templateMakerPane{owner: f, project: p}, session: session, controls: map[int]uintptr{}, work: make(chan captureWork, 1)}
	shot := image.NewRGBA(image.Rect(0, 0, 120, 80))
	item := draftCapture{Window: ports.Window{DPI: 144}, Bounds: ports.Rect{Left: -200, Top: 50, Right: -80, Bottom: 130}, Rule: captureRule{Method: "image"}}
	if err = s.openAnnotation(item, shot); err != nil {
		t.Fatal(err)
	}
	annotationPanes[s.annotation].close()
	if len(session.ids) != 0 || s.paused || s.annotation != 0 {
		t.Fatal("cancel persisted preview or left capture paused")
	}
	for i := 0; i < 2; i++ {
		if err = s.openAnnotation(item, shot); err != nil {
			t.Fatal(err)
		}
		a := annotationPanes[s.annotation]
		a.selection = image.Rect(10, 15, 50, 45)
		send(a.action, 0x14e, 1, 0)
		setWindowTextRaw(a.name, "下一步标记")
		a.save()
		work := <-s.work
		s.busy = false
		s.paused = false
		if work.err != nil {
			t.Fatal(work.err)
		}
		if work.item.Bounds != (ports.Rect{Left: -190, Top: 65, Right: -150, Bottom: 95}) || work.item.Rule.Action != "next" || work.item.Rule.Method != "image" || work.item.Window.DPI != 144 {
			t.Fatal(work.item)
		}
		file, err := os.Open(filepath.Join(projectDir(f.templateRoot(), p), work.item.File))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil || img.Bounds().Size() != image.Pt(40, 30) {
			t.Fatal("original pixel dimensions not preserved", err)
		}
	}
	if err = session.undo(); err != nil || len(session.ids) != 1 {
		t.Fatal("undo", err)
	}
	if err = session.finish(false); err != nil {
		t.Fatal(err)
	}
}
