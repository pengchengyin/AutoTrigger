//go:build windows

package windows

import (
	"autotrigger/internal/core/ports"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type deployRunner struct {
	commands []ports.Command
	unsafe   bool
}

func (r *deployRunner) Start(_ context.Context, cmd ports.Command) (ports.Process, error) {
	r.commands = append(r.commands, cmd)
	if strings.HasSuffix(cmd.Program, "-x64.exe") {
		dir := strings.TrimPrefix(cmd.Args[1], "/D=")
		if err := os.WriteFile(filepath.Join(dir, "7z.exe"), []byte("test fixture"), 0600); err != nil {
			return nil, err
		}
	} else if cmd.Args[0] == "l" {
		path := "release/PaddleOCR-json.exe"
		if r.unsafe {
			path = "../outside.exe"
		}
		fmt.Fprint(cmd.Stdout, "Path = "+path+"\nSize = 1\n\n")
	} else if cmd.Args[0] == "x" {
		for _, arg := range cmd.Args {
			if !strings.HasPrefix(arg, "-o") {
				continue
			}
			dir := filepath.Join(strings.TrimPrefix(arg, "-o"), "release")
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, err
			}
			for _, name := range []string{"PaddleOCR-json.exe", "model.dat", "runtime.dll"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
					return nil, err
				}
			}
		}
	}
	done := make(chan ports.Exit, 1)
	done <- ports.Exit{}
	return deployProcess{done: done}, nil
}

type deployProcess struct{ done chan ports.Exit }

func (deployProcess) PID() int                                  { return 0 }
func (p deployProcess) Done() <-chan ports.Exit                 { return p.done }
func (deployProcess) Activity() (ports.Activity, error)         { return ports.Activity{}, nil }
func (deployProcess) Terminate(context.Context) (string, error) { return "test", nil }
func (deployProcess) KillTree() error                           { return nil }
func (deployProcess) Close() error                              { return nil }

func TestOfflineDeploymentIsLocalAndIdempotent(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{"third_party/7zip/7z2603-x64.exe", "third_party/ocr/PaddleOCR-json_v1.4.1.7z"} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture, never executed"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	runner := &deployRunner{}
	p := &Prereqs{Runner: runner, ExeDir: root}
	sevenZip, err := p.deploySevenZip(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sevenZip != filepath.Join(root, "third_party", "7zip", "7-Zip", "7z.exe") {
		t.Fatal(sevenZip)
	}
	exe, err := p.deployOCR(context.Background(), "paddleocr_json", sevenZip, 1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"PaddleOCR-json.exe", "model.dat", "runtime.dll"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(exe), name)); err != nil {
			t.Fatal(err)
		}
	}
	before := len(runner.commands)
	if _, err := p.deploySevenZip(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.deployOCR(context.Background(), "paddleocr_json", sevenZip, 1024); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != before {
		t.Fatal("existing deployment reinstalled")
	}
}

func TestOfflineDeploymentUsesSelectedDataDirectory(t *testing.T) {
	root := t.TempDir()
	exeDir := t.TempDir()
	for _, relative := range []string{"third_parties/7zip/7z2603-x64.exe", "third_parties/ocr/PaddleOCR-json_v1.4.1.7z"} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture, never executed"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := &Prereqs{Runner: &deployRunner{}, ExeDir: exeDir}
	p.SetDataRoot(root)
	zipPath, err := p.deploySevenZip(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if zipPath != filepath.Join(root, "third_parties", "7zip", "7-Zip", "7z.exe") {
		t.Fatal(zipPath)
	}
	ocrPath, err := p.deployOCR(context.Background(), "paddleocr_json", zipPath, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ocrPath, filepath.Join(root, "third_parties", "ocr")) {
		t.Fatal(ocrPath)
	}
	if _, err = os.Stat(filepath.Join(exeDir, "third_party")); !os.IsNotExist(err) {
		t.Fatal("wrote dependency to EXE directory")
	}
}

func TestOfflineDeploymentRejectsUnsafeAndMissingPackages(t *testing.T) {
	root := t.TempDir()
	runner := &deployRunner{unsafe: true}
	p := &Prereqs{Runner: runner, ExeDir: root}
	if _, err := p.deploySevenZip(context.Background()); err == nil || len(runner.commands) != 0 {
		t.Fatal("missing installer must not run")
	}
	dir := filepath.Join(root, "third_party", "ocr")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PaddleOCR-json_test.7z"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.deployOCR(context.Background(), "paddleocr_json", "7z.exe", 1024); err == nil {
		t.Fatal("unsafe package accepted")
	}
	if len(runner.commands) != 1 || runner.commands[0].Args[0] != "l" {
		t.Fatal("unsafe archive extracted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.deployOCR(ctx, "paddleocr_json", "7z.exe", 1024); err != context.Canceled {
		t.Fatalf("cancelled deployment error = %v", err)
	}
	if len(runner.commands) != 1 {
		t.Fatal("cancelled deployment started a process")
	}
}
