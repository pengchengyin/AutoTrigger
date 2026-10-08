// Package core schedules samples through injected platform ports.
package core

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/layout"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/resources"
	"autotrigger/internal/verify"
	"autotrigger/pkg/schema"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Engine struct {
	Config          config.Config
	Runner          ports.ProcessRunner
	Detector        ports.Detector
	Trigger         ports.InstallerTrigger
	Opener          ports.FileOpener
	Extractor       ports.ArchiveExtractor
	Mounter         ports.DiskMounter
	Evidence        ports.EvidenceSource
	GUI             ports.GUIController
	Log             ports.Logger
	Pack            resources.Pack
	Result          *schema.Result
	OnFile          func(schema.FileResult) error
	Recursive       bool
	DisableArchives bool
	DumpDir         string
	IgnorePaths     []string
	MissingArchive  bool
	Err             error
	temps           []string
}

func (e *Engine) Run(ctx context.Context, input string) error {
	if e.Config.TaskTimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(e.Config.TaskTimeoutSec)*time.Second)
		defer cancel()
	}
	defer func() {
		for _, dir := range e.temps {
			if err := os.RemoveAll(dir); err != nil {
				e.Log.Log("warn", "temporary directory cleanup %s: %v", dir, err)
			}
		}
	}()
	s, err := os.Stat(input)
	if err != nil {
		return err
	}
	if s.IsDir() {
		entries, err := layout.Scan(input, e.Recursive)
		if err != nil {
			return err
		}
		e.packageRun(ctx, input, entries, 0, &e.Result.LayoutAnalysis)
	} else {
		e.add(e.file(ctx, input, "single_file", 0, map[string]bool{}))
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		e.Log.Log("warn", "task timeout reached; pending samples were skipped")
	}
	return e.Err
}
func (e *Engine) add(f schema.FileResult) {
	e.Result.Results = append(e.Result.Results, f)
	e.Result.Summary = schema.Summarize(e.Result.Results)
	if e.OnFile != nil {
		if err := e.OnFile(f); err != nil {
			e.Err = err
		}
	}
	e.Log.Log("info", "%s: %s", f.FilePath, f.FinalStatus)
}
func (e *Engine) ignored(path string) bool {
	for _, p := range e.IgnorePaths {
		if strings.EqualFold(path, p) {
			return true
		}
	}
	return false
}

func (e *Engine) packageRun(ctx context.Context, root string, entries []ports.ArchiveEntry, depth int, analysis *schema.Layout) bool {
	filtered := []ports.ArchiveEntry{}
	for _, entry := range entries {
		if !e.ignored(filepath.Join(root, entry.Path)) {
			filtered = append(filtered, entry)
		}
	}
	analyzer := layout.Analyzer{Detector: e.Detector, Rules: e.Pack, Config: e.Config}
	candidates, l, err := analyzer.Analyze(root, filtered, true)
	if err != nil {
		e.Err = fmt.Errorf("package analysis: %w", err)
		return false
	}
	l.InputKind = analysis.InputKind
	if l.InputKind == "file" {
		l.InputKind = "directory"
	}
	l.MediaKind = analysis.MediaKind
	l.ISOStrategyUsed = analysis.ISOStrategyUsed
	l.MountDrive = analysis.MountDrive
	l.DismountStatus = analysis.DismountStatus
	*analysis = l
	converged := false
	phase1 := 0
	for _, c := range candidates {
		if e.Err != nil {
			return converged
		}
		if c.Reason == "excluded_blacklist" {
			f := schema.NewFile(c.Path, e.Config.Verify.JudgmentProfile)
			f.FileSizeBytes = c.Size
			f.DetectedType = c.Type.Kind
			f.SelectionReason = c.Reason
			f.Notes = "excluded_blacklist: " + c.Matched
			e.add(f)
			continue
		}
		reason := c.Reason
		skip := ""
		if converged {
			skip = "skipped_after_success"
			reason = skip
			analysis.SkippedAfterSuccess++
		} else if ctx.Err() != nil {
			skip = "task_timeout"
			if errors.Is(ctx.Err(), context.Canceled) {
				skip = "interrupted"
			}
		} else if e.Config.PackageLayout.Enabled && e.Config.PackageLayout.Mode == "smart" {
			if c.Priority <= 4 {
				phase1++
				if phase1 > e.Config.PackageLayout.MaxEntriesPhase1 {
					if e.Config.PackageLayout.FallbackToBrute {
						reason = "fallback_p5"
						analysis.FallbackUsed = true
					} else {
						skip = "phase1_entry_limit"
					}
				}
			} else {
				if !e.Config.PackageLayout.FallbackToBrute {
					skip = "fallback_disabled"
				} else {
					analysis.FallbackUsed = true
				}
			}
		}
		if skip != "" {
			f := schema.NewFile(c.Path, e.Config.Verify.JudgmentProfile)
			f.FileSizeBytes = c.Size
			f.DetectedType = c.Type.Kind
			f.SelectionReason = reason
			f.Notes = skip
			e.add(f)
			continue
		}
		f := e.file(ctx, c.Path, reason, depth, map[string]bool{})
		e.add(f)
		if f.FinalStatus == "install_completed" && c.Priority != 3 && e.Config.PackageLayout.Enabled && e.Config.PackageLayout.Mode == "smart" {
			converged = true
			analysis.ConvergedAt = schema.String(c.Relative)
		}
	}
	return converged
}

func (e *Engine) file(ctx context.Context, path, reason string, depth int, seen map[string]bool) schema.FileResult {
	f := schema.NewFile(path, e.Config.Verify.JudgmentProfile)
	f.SelectionReason = reason
	started := time.Now()
	if ctx.Err() != nil {
		f.Notes = "task_timeout"
		return f
	}
	s, err := os.Stat(path)
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = err.Error()
		return f
	}
	f.FileSizeBytes = s.Size()
	if s.Size() == 0 {
		f.Notes = "empty_file"
		return f
	}
	fileCtx := ctx
	cancel := func() {}
	if e.Config.TimeoutSec > 0 {
		fileCtx, cancel = context.WithDeadline(ctx, started.Add(time.Duration(e.Config.TimeoutSec)*time.Second))
	}
	defer cancel()
	ft, err := e.Detector.Detect(path)
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = err.Error()
		return f
	}
	f.DetectedType = ft.Kind
	f.ExtensionDisguised = ft.Disguised
	hash, err := hashFile(fileCtx, path)
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = "hash: " + err.Error()
		return f
	}
	f.FileHashSHA256 = hash
	if seen[path] {
		f.FinalStatus = "failed"
		f.Notes = "shortcut_cycle"
		return f
	}
	seen[path] = true
	if ft.Kind == "archive" || ft.Kind == "disk_image" {
		return e.container(fileCtx, f, depth, ft.Kind == "disk_image")
	}
	triggerPath := path
	var linkArgs []string
	if ft.Kind == "shortcut" {
		target, args, err := e.Opener.ResolveShortcut(path)
		if err != nil {
			f.FinalStatus = "failed"
			f.Notes = "shortcut: " + err.Error()
			return f
		}
		target, err = filepath.Abs(target)
		if err != nil || target == path || seen[target] {
			f.FinalStatus = "failed"
			f.Notes = "shortcut_cycle or invalid target"
			return f
		}
		f.PlatformMeta["shortcut_target"] = target
		f.PlatformMeta["shortcut_arguments"] = args
		ft, err = e.Detector.Detect(target)
		if err != nil {
			f.FinalStatus = "failed"
			f.Notes = err.Error()
			return f
		}
		triggerPath = target
		linkArgs = args
		if ft.Kind == "shortcut" {
			child := e.file(fileCtx, target, reason, depth, seen)
			child.FilePath = path
			child.FileHashSHA256 = hash
			child.PlatformMeta["shortcut_target"] = target
			return child
		}
	}
	base, err := e.Evidence.Snapshot(fileCtx, e.Config.Verify)
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = "baseline unavailable: " + err.Error()
		return f
	}
	f.BaselineDigest = schema.String(verify.Digest(base))
	f.PreInstalled = verify.PreInstalled(triggerPath, base)
	if e.Config.Verify.WriteBaseline {
		f.PlatformMeta["baseline"] = base
	}
	for _, w := range base.Warnings {
		e.Log.Log("warn", "baseline %s: %s", path, w)
	}
	temp, err := e.tempDir()
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = err.Error()
		return f
	}
	var commands []ports.Command
	if ft.Kind == "document" || ft.Kind == "unknown" {
		if !e.Opener.HasAssociation(triggerPath) {
			f.Notes = "no_host_app"
			return f
		}
		cmd, err := e.Opener.OpenCommand(triggerPath)
		if err != nil {
			f.FinalStatus = "failed"
			f.Notes = err.Error()
			return f
		}
		commands = []ports.Command{cmd}
	} else {
		commands, err = e.Trigger.Commands(triggerPath, ft, temp)
		if err != nil {
			f.FinalStatus = "failed"
			f.Notes = err.Error()
			return f
		}
	}
	if len(linkArgs) > 0 {
		commands = []ports.Command{{Program: triggerPath, Args: linkArgs, Dir: filepath.Dir(triggerPath), Method: "lnk_target"}}
	}
	if e.DumpDir != "" {
		for _, cmd := range commands {
			if cmd.Method == "no_param_run" {
				commands = []ports.Command{cmd}
				break
			}
		}
	}
	if e.GUI != nil {
		e.GUI.Reset()
	}
	allEvidence := []schema.Evidence{}
	sideEffects := false
	everStarted, timedout := false, false
	for i, cmd := range commands {
		if fileCtx.Err() != nil {
			timedout = everStarted
			break
		}
		if i > 0 {
			select {
			case <-fileCtx.Done():
				timedout = everStarted
			case <-time.After(time.Duration(e.Config.AttemptIntervalSec) * time.Second):
			}
			if fileCtx.Err() != nil {
				break
			}
		}
		a, ev, startedAttempt, timeoutAttempt, reboot, network := e.attempt(fileCtx, cmd, base, triggerPath)
		f.Attempts = append(f.Attempts, a)
		everStarted = everStarted || startedAttempt
		timedout = timedout || timeoutAttempt
		f.RequiresReboot = f.RequiresReboot || reboot
		if network {
			f.PlatformMeta["requires_network"] = true
		}
		allEvidence = append(allEvidence, ev.Evidence...)
		sideEffects = sideEffects || ev.SideEffects
		// A failed parameter attempt must not poison a later successful attempt.
		current := positive(allEvidence)
		for _, item := range ev.Evidence {
			if item.Level == "N" {
				current = append(current, item)
			}
		}
		d := verify.Decide(e.Config.Verify.JudgmentProfile, current, sideEffects, f.PreInstalled, everStarted, timedout, ev.Artifact)
		f.FinalStatus = d.Status
		f.InstallConfidence = d.Confidence
		f.EvidenceConflicts = d.Conflicts
		if d.Status == "install_completed" || e.DumpDir != "" {
			break
		}
	}
	if len(f.Attempts) == 0 {
		f.FinalStatus = "failed"
		f.Notes = "no attempts completed before file deadline"
	}
	if timedout && f.FinalStatus == "install_completed" {
		f.Notes = "terminated on timeout; installation evidence still collected"
	}
	return f
}

func positive(ev []schema.Evidence) []schema.Evidence {
	out := []schema.Evidence{}
	for _, e := range ev {
		if e.Level != "N" {
			out = append(out, e)
		}
	}
	return out
}
func commandText(c ports.Command) string {
	args := []string{fmt.Sprintf("%q", c.Program)}
	for _, arg := range c.Args {
		args = append(args, fmt.Sprintf("%q", arg))
	}
	return strings.Join(args, " ")
}
func (e *Engine) tempDir() (string, error) {
	root := config.ExpandEnv(e.Config.Archive.TempRoot)
	if root != "" {
		if err := os.MkdirAll(root, 0755); err != nil {
			return "", err
		}
	}
	dir, err := os.MkdirTemp(root, "AT-")
	if err != nil && root == "" {
		e.Log.Log("warn", "TEMP unavailable, falling back to executable directory")
		exe, _ := os.Executable()
		dir, err = os.MkdirTemp(filepath.Dir(exe), "AT-")
	}
	if err == nil {
		e.temps = append(e.temps, dir)
	}
	return dir, err
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	return r.r.Read(p)
}
func hashFile(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, contextReader{ctx: ctx, r: file}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
