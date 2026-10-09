package core

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/detect"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/resources"
	"autotrigger/pkg/schema"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeProcess struct {
	exit   chan ports.Exit
	killed bool
	hang   bool
}

func (p *fakeProcess) PID() int                { return 123 }
func (p *fakeProcess) Done() <-chan ports.Exit { return p.exit }
func (p *fakeProcess) Activity() (ports.Activity, error) {
	return ports.Activity{PIDs: []int{123}, CPUTimeMS: 1000}, nil
}
func (p *fakeProcess) Terminate(context.Context) (string, error) {
	p.killed = true
	select {
	case p.exit <- ports.Exit{Code: 1}:
	default:
	}
	return "killed_tree", nil
}
func (p *fakeProcess) KillTree() error { p.killed = true; return nil }
func (p *fakeProcess) Close() error    { return nil }

type fakeRunner struct {
	commands  []ports.Command
	fail      bool
	code      int
	hang      bool
	installAt int
	installed bool
	last      *fakeProcess
}

func (r *fakeRunner) Start(_ context.Context, c ports.Command) (ports.Process, error) {
	r.commands = append(r.commands, c)
	if r.fail {
		return nil, errors.New("launch denied")
	}
	if r.installAt > 0 && len(r.commands) >= r.installAt {
		r.installed = true
	}
	p := &fakeProcess{exit: make(chan ports.Exit, 1), hang: r.hang}
	r.last = p
	if !r.hang {
		p.exit <- ports.Exit{Code: r.code, Stdout: "output"}
	}
	return p, nil
}

type fakeTrigger struct {
	count int
	err   error
}

func (t fakeTrigger) Commands(path string, ft ports.FileType, temp string) ([]ports.Command, error) {
	if t.err != nil {
		return nil, t.err
	}
	out := []ports.Command{}
	for i := 0; i < t.count; i++ {
		method := "silent_install"
		if i == t.count-1 {
			method = "no_param_run"
		}
		out = append(out, ports.Command{Program: path, Method: method, MSI: ft.Kind == "msi"})
	}
	return out, nil
}

type fakeEvidence struct {
	runner *fakeRunner
	fail   bool
	pre    bool
}

func (s fakeEvidence) Snapshot(context.Context, config.Verify) (*ports.Baseline, error) {
	if s.fail {
		return nil, errors.New("snapshot denied")
	}
	reg := map[string]ports.UninstallEntry{}
	if s.runner.installed || s.pre {
		reg["hkcu\\demo"] = ports.UninstallEntry{DisplayName: "Demo", DisplayIcon: "demo.exe"}
	}
	return &ports.Baseline{Registry: reg, Dirs: map[string]ports.DirState{}, CapturedAt: time.Now(), Warnings: []string{}}, nil
}
func (s fakeEvidence) Events(context.Context, uint64) ([]ports.Event, error) { return nil, nil }
func (s fakeEvidence) ValidArtifact(string) bool                             { return true }

type fakeOpener struct {
	association bool
	target      string
	err         error
}

func (o fakeOpener) HasAssociation(string) bool { return o.association }
func (o fakeOpener) OpenCommand(path string) (ports.Command, error) {
	return ports.Command{Program: path, Method: "document_host"}, o.err
}
func (o fakeOpener) ResolveShortcut(string) (string, []string, error) { return o.target, nil, o.err }

type fakeLogger struct{}

func (fakeLogger) Log(string, string, ...interface{}) {}

func (fakeLogger) Stage(string, string, string, ...interface{}) {}

type fakeArchive struct {
	entries                 []ports.ArchiveEntry
	err                     error
	listCalls, extractCalls int
}

func (a *fakeArchive) List(context.Context, string) ([]ports.ArchiveEntry, error) {
	a.listCalls++
	return a.entries, a.err
}
func (a *fakeArchive) Extract(_ context.Context, _, root string, _ int64) error {
	a.extractCalls++
	for _, entry := range a.entries {
		if entry.Directory {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(entry.Path))
		os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte("MZfixture"), 0600); err != nil {
			return err
		}
	}
	return a.err
}

type fakeMounter struct {
	root                string
	err                 error
	mounted, dismounted bool
}

func (m *fakeMounter) Mount(context.Context, string) (string, error) {
	m.mounted = true
	return m.root, m.err
}
func (m *fakeMounter) Dismount(context.Context, string) error { m.dismounted = true; return nil }
func (m *fakeMounter) MotW(string, string, string) error      { return nil }

type fakeGUI struct {
	dump  bool
	steps int
}

func (g *fakeGUI) Reset() {}
func (g *fakeGUI) Step(context.Context, []int) (ports.GUIUpdate, error) {
	g.steps++
	return ports.GUIUpdate{Actions: []schema.GUIAction{}, Evidence: []schema.Evidence{}, Signature: "same-window"}, nil
}
func (g *fakeGUI) Dump(context.Context, []int, string) error { g.dump = true; return nil }
func engineFor(t *testing.T) (*Engine, *fakeRunner) {
	t.Helper()
	r := &fakeRunner{}
	c := config.Defaults()
	c.AttemptIntervalSec = 0
	c.Verify.SettleMaxSec = 0
	c.GUI.PollIntervalMS = 10
	c.Archive.TempRoot = t.TempDir()
	result := schema.NewResult("test", "test")
	e := &Engine{Config: c, Runner: r, Detector: detect.Detector{}, Trigger: fakeTrigger{count: 4}, Opener: fakeOpener{}, Evidence: fakeEvidence{runner: r}, Log: fakeLogger{}, Pack: resources.Pack{Mode: "builtin"}, Result: &result}
	return e, r
}
func sample(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestFileAttemptsAndBaselineReuse(t *testing.T) {
	e, r := engineFor(t)
	r.installAt = 2
	path := sample(t, "Demo_setup.exe", []byte("MZfixture"))
	if err := e.Run(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	f := e.Result.Results[0]
	if len(f.Attempts) != 2 || f.FinalStatus != "install_completed" || f.PreInstalled {
		t.Fatalf("%+v", f)
	}
	if f.BaselineDigest == nil || f.FileHashSHA256 == "" {
		t.Fatal("audit fields missing")
	}
	sum := 0
	for _, count := range e.Result.Summary.ByStatus {
		sum += count
	}
	if sum != e.Result.Summary.Total {
		t.Fatal("summary buckets do not partition total")
	}
}
func TestZeroExitCannotFakeInstallation(t *testing.T) {
	e, r := engineFor(t)
	path := sample(t, "demo.exe", []byte("MZfixture"))
	e.Run(context.Background(), path)
	f := e.Result.Results[0]
	if f.FinalStatus != "install_unverified" || len(r.commands) != 4 || r.commands[3].Method != "no_param_run" {
		t.Fatalf("%+v %v", f, r.commands)
	}
}
func TestFailedParameterDoesNotPoisonSuccess(t *testing.T) {
	e, r := engineFor(t)
	r.code = 1
	r.installAt = 2
	e.Trigger = changingTrigger{runner: r}
	path := sample(t, "demo.exe", []byte("MZfixture"))
	e.Run(context.Background(), path)
	if e.Result.Results[0].FinalStatus != "install_completed" {
		t.Fatal(e.Result.Results[0])
	}
}

type changingTrigger struct{ runner *fakeRunner }

func (c changingTrigger) Commands(path string, _ ports.FileType, _ string) ([]ports.Command, error) {
	c.runner.code = 0
	return []ports.Command{{Program: path, Method: "silent_install"}, {Program: path, Method: "no_param_run"}}, nil
}
func TestFileErrorPaths(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(*Engine, *fakeRunner)
		data   []byte
		status string
	}{{"launch", func(e *Engine, r *fakeRunner) { r.fail = true }, []byte("MZ"), "failed"}, {"baseline", func(e *Engine, r *fakeRunner) { e.Evidence = fakeEvidence{runner: r, fail: true} }, []byte("MZ"), "failed"}, {"empty", func(*Engine, *fakeRunner) {}, nil, "skipped"}, {"host", func(e *Engine, r *fakeRunner) { e.Opener = fakeOpener{} }, []byte("%PDF"), "skipped"}, {"document", func(e *Engine, r *fakeRunner) { e.Opener = fakeOpener{association: true} }, []byte("%PDF"), "install_unverified"}, {"trigger", func(e *Engine, r *fakeRunner) { e.Trigger = fakeTrigger{err: ports.ErrUnsupported} }, []byte("MZ"), "failed"}, {"already", func(e *Engine, r *fakeRunner) { e.Evidence = fakeEvidence{runner: r, pre: true} }, []byte("MZ"), "already_installed"}} {
		t.Run(tc.name, func(t *testing.T) {
			e, r := engineFor(t)
			tc.setup(e, r)
			path := sample(t, "Demo.exe", tc.data)
			e.Run(context.Background(), path)
			if f := e.Result.Results[0]; f.FinalStatus != tc.status {
				t.Fatalf("expected %s: %+v", tc.status, f)
			}
		})
	}
}
func TestFolderConvergesAndExcludesUninstaller(t *testing.T) {
	e, r := engineFor(t)
	r.installAt = 1
	root := t.TempDir()
	for _, name := range []string{"setup.exe", "unins000.exe", "other.exe"} {
		os.WriteFile(filepath.Join(root, name), []byte("MZ"), 0600)
	}
	e.Run(context.Background(), root)
	if len(r.commands) != 1 || e.Result.LayoutAnalysis.ConvergedAt == nil {
		t.Fatal("package did not converge", r.commands)
	}
	excluded, skipped := false, false
	for _, f := range e.Result.Results {
		excluded = excluded || f.SelectionReason == "excluded_blacklist"
		skipped = skipped || f.SelectionReason == "skipped_after_success"
	}
	if !excluded || !skipped {
		t.Fatal(e.Result.Results)
	}
}
func TestFolderFallback(t *testing.T) {
	e, r := engineFor(t)
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "deep"), 0755)
	os.WriteFile(filepath.Join(root, "deep", "helper.dll"), []byte("MZ"), 0600)
	e.Recursive = true
	e.Run(context.Background(), root)
	if !e.Result.LayoutAnalysis.FallbackUsed || len(r.commands) != 4 {
		t.Fatal("fallback missed")
	}
	e2, r2 := engineFor(t)
	e2.Recursive = true
	e2.Config.PackageLayout.FallbackToBrute = false
	e2.Run(context.Background(), root)
	if len(r2.commands) != 0 {
		t.Fatal("disabled fallback ran")
	}
}
func TestArchivePreflightAndRecursion(t *testing.T) {
	e, r := engineFor(t)
	r.installAt = 1
	a := &fakeArchive{entries: []ports.ArchiveEntry{{Path: "setup.exe", Size: 9}, {Path: "unins000.exe", Size: 9}}}
	e.Extractor = a
	path := sample(t, "demo.zip", []byte{0x50, 0x4b, 3, 4})
	e.Run(context.Background(), path)
	if a.listCalls != 1 || a.extractCalls != 1 || len(e.Result.Results) != 3 {
		t.Fatalf("%+v %+v", a, e.Result.Results)
	}
	for _, dir := range e.temps {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("extraction residue", dir)
		}
	}
	e2, _ := engineFor(t)
	e2.Extractor = &fakeArchive{entries: a.entries}
	e2.Config.Archive.MaxExtractSizeBytes = 1
	e2.Run(context.Background(), path)
	if !strings.Contains(e2.Result.Results[0].Notes, "extract_size_exceeded") {
		t.Fatal(e2.Result.Results)
	}
}
func TestISOProtectionAndMountFallback(t *testing.T) {
	path := sample(t, "disc.iso", []byte("ISO"))
	e, _ := engineFor(t)
	a := &fakeArchive{entries: []ports.ArchiveEntry{{Path: "sources/install.wim", Size: 1}, {Path: "setup.exe", Size: 1}}}
	e.Extractor = a
	e.Run(context.Background(), path)
	if a.extractCalls != 0 || e.Result.Results[0].Notes != "os_media" {
		t.Fatal("OS media was extracted/executed")
	}
	e2, r := engineFor(t)
	e2.Extractor = &fakeArchive{err: errors.New("cannot open archive")}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "setup.exe"), []byte("MZ"), 0600)
	m := &fakeMounter{root: root}
	e2.Mounter = m
	r.installAt = 1
	e2.Run(context.Background(), path)
	if !m.mounted || !m.dismounted {
		t.Fatal("mount lifecycle missed")
	}
}
func TestMissingArchiveAndExplicitDisable(t *testing.T) {
	path := sample(t, "a.zip", []byte{0x50, 0x4b, 3, 4})
	e, _ := engineFor(t)
	e.Run(context.Background(), path)
	if !e.MissingArchive || e.Result.Results[0].FinalStatus != "skipped" {
		t.Fatal(e.Result.Results)
	}
	e2, _ := engineFor(t)
	e2.Config.Archive.RequireSevenZip = false
	e2.DisableArchives = true
	e2.Run(context.Background(), path)
	if e2.MissingArchive {
		t.Fatal("explicit skip became prerequisite error")
	}
	e3, r3 := engineFor(t)
	e3.Config.Archive.RequireSevenZip = false
	e3.Extractor = &fakeArchive{entries: []ports.ArchiveEntry{{Path: "setup.exe", Size: 9}}}
	r3.installAt = 1
	e3.Run(context.Background(), path)
	if len(r3.commands) == 0 || e3.MissingArchive {
		t.Fatal("optional prerequisite disabled an available archive backend")
	}
}
func TestTimeoutTerminatesAndStillCollectsEvidence(t *testing.T) {
	e, r := engineFor(t)
	r.hang = true
	r.installAt = 1
	e.Config.TimeoutSec = 1
	e.Trigger = fakeTrigger{count: 1}
	g := &fakeGUI{}
	e.GUI = g
	path := sample(t, "demo.exe", []byte("MZ"))
	e.Run(context.Background(), path)
	f := e.Result.Results[0]
	if !r.last.killed || f.FinalStatus != "install_completed" || g.steps == 0 {
		t.Fatalf("%+v", f)
	}
	if f.Attempts[0].ExitCode != nil {
		t.Fatal("tool-generated termination exit code treated as installer failure")
	}
}
func TestDumpAndCancellation(t *testing.T) {
	e, r := engineFor(t)
	r.hang = true
	e.Trigger = fakeTrigger{count: 2}
	g := &fakeGUI{}
	e.GUI = g
	e.DumpDir = t.TempDir()
	path := sample(t, "demo.exe", []byte("MZ"))
	e.Run(context.Background(), path)
	if !g.dump || len(r.commands) != 1 || r.commands[0].Method != "no_param_run" {
		t.Fatal("UI dump launched silent attempts or clicked")
	}
	e2, _ := engineFor(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e2.Run(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
