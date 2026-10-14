package archive

import (
	"autotrigger/internal/core/ports"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractionFailureDiagnostic(t *testing.T) {
	cause := fmt.Errorf("7z.exe exited 2: ")
	err := extractionError("list", ports.Exit{Code: 2, Stdout: "ERROR: archive is truncated"}, cause)
	if !strings.HasPrefix(err.Error(), "extract_failed: list:") || !strings.Contains(err.Error(), "exited 2") || !strings.Contains(err.Error(), "archive is truncated") || !errors.Is(err, cause) {
		t.Fatal(err)
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if got := extractionError("extract", ports.Exit{}, cause); got != cause {
			t.Fatal("cancellation reclassified", got)
		}
	}
	if extractionError("list", ports.Exit{}, nil) != nil {
		t.Fatal("success classified as failure")
	}
	err = extractionError("extract", ports.Exit{Stderr: strings.Repeat("x", 10000) + "Wrong password"}, cause)
	if len(err.Error()) > 4300 || !strings.Contains(err.Error(), "Wrong password") || !strings.Contains(err.Error(), "diagnostics truncated") {
		t.Fatal("diagnostic limit or tail lost")
	}
}

type failingArchiveRunner struct{}
type failingArchiveProcess struct{ done chan ports.Exit }

func (failingArchiveRunner) Start(_ context.Context, cmd ports.Command) (ports.Process, error) {
	fmt.Fprint(cmd.Stdout, "ERROR: archive is truncated")
	done := make(chan ports.Exit, 1)
	done <- ports.Exit{Code: 2}
	return failingArchiveProcess{done}, nil
}
func (p failingArchiveProcess) PID() int                                  { return 1 }
func (p failingArchiveProcess) Done() <-chan ports.Exit                   { return p.done }
func (p failingArchiveProcess) Activity() (ports.Activity, error)         { return ports.Activity{}, nil }
func (p failingArchiveProcess) Terminate(context.Context) (string, error) { return "graceful", nil }
func (p failingArchiveProcess) KillTree() error                           { return nil }
func (p failingArchiveProcess) Close() error                              { return nil }

func TestSevenZipProcessFailuresCarryCodeAndBothChannels(t *testing.T) {
	s := SevenZip{Path: "7z.exe", Runner: failingArchiveRunner{}}
	_, listErr := s.List(context.Background(), "broken.zip")
	extractErr := s.Extract(context.Background(), "broken.zip", t.TempDir(), 100)
	for _, err := range []error{listErr, extractErr} {
		if err == nil || !strings.HasPrefix(err.Error(), "extract_failed:") || !strings.Contains(err.Error(), "exited 2") || !strings.Contains(err.Error(), "archive is truncated") {
			t.Fatal(err)
		}
	}
}

func TestListingAndLimits(t *testing.T) {
	entries, err := ParseList("Path = setup.exe\nSize = 42\nAttributes = A\n\nPath = assets\nSize = 0\nFolder = +\n\n")
	if err != nil || len(entries) != 2 || entries[0].Size != 42 || !entries[1].Directory {
		t.Fatalf("%+v %v", entries, err)
	}
	if err = Size(entries, 41); err == nil {
		t.Fatal("size limit bypassed")
	}
	for _, path := range []string{"../escape.exe", `C:\escape.exe`, `..\escape.exe`, "/escape.exe"} {
		if _, err = ParseList("Path = " + path + "\nSize = 1\n"); err == nil {
			t.Fatal("unsafe path accepted", path)
		}
	}
	if _, err = ParseList("Path = link\nSymbolic Link = ../escape\n"); err == nil {
		t.Fatal("link accepted")
	}
	if _, err = ParseList("Path = x\nSize = -1\n"); err == nil {
		t.Fatal("negative size accepted")
	}
}
func TestExtractionBudget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("1234"), 0600)
	if ValidateTree(dir, 4) != nil {
		t.Fatal("legal tree rejected")
	}
	if ValidateTree(dir, 3) == nil {
		t.Fatal("actual extraction size ignored")
	}
}
