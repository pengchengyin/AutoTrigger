//go:build windows

package windows

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/pkg/schema"
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

type inventoryRunner struct {
	mu        sync.Mutex
	commands  []ports.Command
	failRapid bool
}

func (r *inventoryRunner) Start(_ context.Context, cmd ports.Command) (ports.Process, error) {
	r.mu.Lock()
	r.commands = append(r.commands, cmd)
	r.mu.Unlock()
	if strings.EqualFold(filepath.Base(cmd.Program), "PaddleOCR-json.exe") || strings.EqualFold(filepath.Base(cmd.Program), "RapidOCR-json.exe") {
		if r.failRapid && strings.EqualFold(filepath.Base(cmd.Program), "RapidOCR-json.exe") {
			return nil, fmt.Errorf("regression fixture: runtime DLL missing")
		}
		duplicate := func(f *os.File) (*os.File, error) {
			process, err := syscall.GetCurrentProcess()
			if err != nil {
				return nil, err
			}
			var handle syscall.Handle
			if err = syscall.DuplicateHandle(process, syscall.Handle(f.Fd()), process, &handle, 0, false, syscall.DUPLICATE_SAME_ACCESS); err != nil {
				return nil, err
			}
			return os.NewFile(uintptr(handle), "inventory-test-pipe"), nil
		}
		input, err := duplicate(cmd.Stdin.(*os.File))
		if err != nil {
			return nil, err
		}
		output, err := duplicate(cmd.Stdout.(*os.File))
		if err != nil {
			input.Close()
			return nil, err
		}
		fmt.Fprintln(output, "OCR init completed.")
		go func() {
			defer input.Close()
			defer output.Close()
			scan := bufio.NewScanner(input)
			for scan.Scan() {
				fmt.Fprintln(output, `{"code":101,"data":[]}`)
			}
		}()
		return &inventoryProcess{done: make(chan ports.Exit, 1)}, nil
	}
	if len(cmd.Args) > 0 && (cmd.Args[0] == "l" || cmd.Args[0] == "x") {
		pack := cmd.Args[len(cmd.Args)-1]
		name := "PaddleOCR-json.exe"
		if strings.HasPrefix(filepath.Base(pack), "RapidOCR-json") {
			name = "RapidOCR-json.exe"
		}
		if cmd.Args[0] == "l" {
			fmt.Fprintf(cmd.Stdout, "Path = release/%s\nSize = 1\n\n", name)
		} else {
			for _, arg := range cmd.Args {
				if strings.HasPrefix(arg, "-o") {
					stage := strings.TrimPrefix(arg, "-o")
					target := filepath.Join(stage, "release")
					if err := os.MkdirAll(target, 0755); err != nil {
						return nil, err
					}
					for _, f := range []string{name, "runtime.dll", "model.dat"} {
						if err := os.WriteFile(filepath.Join(target, f), []byte("fixture"), 0600); err != nil {
							return nil, err
						}
					}
					// Keep packaging files outside the executable directory as well.
					if err := os.WriteFile(filepath.Join(stage, "README.txt"), []byte("fixture"), 0600); err != nil {
						return nil, err
					}
				}
			}
		}
	} else if len(cmd.Args) > 0 && cmd.Args[0] == "i" {
		fmt.Fprintln(cmd.Stdout, "7-Zip 26.03")
	}
	done := make(chan ports.Exit, 1)
	done <- ports.Exit{}
	return deployProcess{done: done}, nil
}

type inventoryProcess struct {
	done chan ports.Exit
	once sync.Once
}

func (p *inventoryProcess) PID() int                                  { return 0 }
func (p *inventoryProcess) Done() <-chan ports.Exit                   { return p.done }
func (p *inventoryProcess) Activity() (ports.Activity, error)         { return ports.Activity{}, nil }
func (p *inventoryProcess) Terminate(context.Context) (string, error) { return "fixture", p.KillTree() }
func (p *inventoryProcess) KillTree() error                           { p.once.Do(func() { p.done <- ports.Exit{} }); return nil }
func (p *inventoryProcess) Close() error                              { return nil }

func inventoryFixture(t *testing.T) (string, *Prereqs, *inventoryRunner, config.Config, schema.Prereq) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "third_party", "ocr")
	os.MkdirAll(dir, 0755)
	for _, name := range []string{"PaddleOCR-json_v1.7z", "PaddleOCR-json_v2.zip", "RapidOCR-json_v0.2.0.7z"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture, never executed"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := &inventoryRunner{}
	p := &Prereqs{Runner: r, ExeDir: root}
	c := config.Defaults()
	c.Archive.MaxExtractSizeBytes = 1024
	report := schema.EmptyPrereq()
	report.ArchiveBackend = schema.String("test-7z.exe")
	return root, p, r, c, report
}

func TestOCRInventoryDeploysAllVersionsAndProbesAll(t *testing.T) {
	root, p, r, c, report := inventoryFixture(t)
	rows := p.inventoryOCR(context.Background(), c, true, true, &report)
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Engine]++
		if row.Engine == "paddleocr_json" || row.Engine == "rapidocr_json" {
			if row.Status != "ok" || row.Path == nil || row.PackagePath == nil {
				t.Fatalf("not tested: %+v", row)
			}
			for _, name := range []string{"runtime.dll", "model.dat"} {
				if _, err := os.Stat(filepath.Join(filepath.Dir(*row.Path), name)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(*row.Path)), "README.txt")); err != nil {
				t.Fatal("package root contents lost", err)
			}
		}
	}
	if counts["paddleocr_json"] != 2 || counts["rapidocr_json"] != 1 || counts["umi_ocr"] != 1 || counts["tesseract"] != 1 || counts["windows"] != 1 {
		t.Fatal(counts)
	}
	before := 0
	for _, cmd := range r.commands {
		if len(cmd.Args) > 0 && cmd.Args[0] == "x" {
			before++
		}
	}
	p.inventoryOCR(context.Background(), c, true, true, &report)
	after := 0
	for _, cmd := range r.commands {
		if len(cmd.Args) > 0 && cmd.Args[0] == "x" {
			after++
		}
	}
	if before != 3 || after != before {
		t.Fatalf("not idempotent: %d / %d", before, after)
	}
	if len(localOCRExecutables(root, "PaddleOCR-json.exe")) != 2 {
		t.Fatal("deployed version discovery failed")
	}
}

func TestOCRInventoryReadonlyDisabledAndIndependentFailures(t *testing.T) {
	root, p, r, c, report := inventoryFixture(t)
	rows := p.inventoryOCR(context.Background(), c, false, true, &report)
	for _, row := range rows {
		if row.PackagePath != nil && row.Status != "not_deployed" {
			t.Fatal(row)
		}
	}
	for _, cmd := range r.commands {
		if len(cmd.Args) > 0 && (cmd.Args[0] == "x" || cmd.Args[0] == "l") {
			t.Fatal("readonly deployed a package")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "third_party", "ocr", "packages")); !os.IsNotExist(err) {
		t.Fatal("readonly wrote deployment")
	}
	c.OCR.Enabled = false
	r.commands = nil
	p.inventoryOCR(context.Background(), c, true, true, &report)
	if len(r.commands) != 0 {
		t.Fatal("disabled OCR started processes")
	}
	c.OCR.Enabled = true
	r.failRapid = true
	rows = p.inventoryOCR(context.Background(), c, true, true, &report)
	ok, failed := 0, 0
	for _, row := range rows {
		if row.Engine == "paddleocr_json" && row.Status == "ok" {
			ok++
		}
		if row.Engine == "rapidocr_json" && row.Status == "startup_failed" && strings.Contains(row.Detail, "DLL missing") {
			failed++
		}
	}
	if ok != 2 || failed != 1 {
		t.Fatalf("independent probe failure lost: %d/%d", ok, failed)
	}
	r.failRapid = false
	r.commands = nil
	rows = p.inventoryOCR(context.Background(), c, false, false, &report)
	for _, row := range rows {
		if row.Engine == "paddleocr_json" && row.Status != "unsupported" {
			t.Fatal("AVX restriction not reported")
		}
	}
	for _, cmd := range r.commands {
		if strings.EqualFold(filepath.Base(cmd.Program), "PaddleOCR-json.exe") {
			t.Fatal("Paddle started without AVX")
		}
	}
}

func TestInventoryDoesNotReplaceRequiredSelectedEngine(t *testing.T) {
	root, p, r, c, _ := inventoryFixture(t)
	zip := filepath.Join(root, "test-7z.exe")
	if err := os.WriteFile(zip, []byte("fixture, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Archive.SevenZipPath = zip
	c.OCR.Engine = "rapidocr_json"
	c.OCR.RequireOCR = true
	r.failRapid = true
	report, _, err := p.CheckEnvironment(context.Background(), c, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.OCREngine != "rapidocr_json" || report.OCREngineStatus != "startup_failed" || report.OK || p.Engine != nil {
		t.Fatalf("selected failure replaced: %+v", report)
	}
	selected := false
	for _, row := range report.OCREngines {
		if row.Engine == "rapidocr_json" && row.Selected {
			selected = true
		}
	}
	if !selected {
		t.Fatal("selected failed engine missing from inventory")
	}
}
