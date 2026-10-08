package core

import (
	"autotrigger/internal/archive"
	"autotrigger/internal/core/layout"
	"autotrigger/internal/core/ports"
	"autotrigger/pkg/schema"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func (e *Engine) container(ctx context.Context, f schema.FileResult, depth int, iso bool) schema.FileResult {
	if e.DisableArchives {
		f.Notes = "prereq_missing: archives disabled"
		return f
	}
	if depth >= e.Config.Archive.MaxDepth {
		f.Notes = "max_archive_depth"
		return f
	}
	analyzer := layout.Analyzer{Detector: e.Detector, Rules: e.Pack, Config: e.Config}
	root := ""
	analysis := schema.NewLayout("archive", e.Config.ISO.MotWPolicy)
	if iso {
		analysis.InputKind = "iso"
	}
	mounted := false
	keep := false
	defer func() {
		if mounted {
			status := "failed"
			for i := 0; i < e.Config.ISO.DismountRetry; i++ {
				cleanup, cancel := context.WithTimeout(context.Background(), time.Duration(e.Config.ISO.MountTimeoutMS)*time.Millisecond)
				err := e.Mounter.Dismount(cleanup, f.FilePath)
				cancel()
				if err == nil {
					status = "ok"
					break
				}
				time.Sleep(time.Second)
			}
			analysis.DismountStatus = status
			if status == "failed" {
				e.Log.Log("warn", "dismount_failed: %s", f.FilePath)
			}
		}
		f.PlatformMeta["layout_analysis"] = analysis
		e.Result.LayoutAnalysis = analysis
	}()
	extract := func() error {
		if e.Extractor == nil {
			e.MissingArchive = true
			return fmt.Errorf("prereq_missing: 7-Zip not available")
		}
		entries, err := e.Extractor.List(ctx, f.FilePath)
		if err != nil {
			return err
		}
		if err = archive.Size(entries, e.Config.Archive.MaxExtractSizeBytes); err != nil {
			return err
		}
		if iso {
			kind, err := analyzer.MediaKind(entries)
			if err != nil {
				return err
			}
			analysis.MediaKind = schema.String(kind)
			if kind == "os_install_media" && !e.Config.ISO.AllowOSMedia {
				return errSkipMedia("os_media")
			}
			if kind == "media_disc" && !e.Config.ISO.AllowMediaDisc {
				return errSkipMedia("media_disc")
			}
		}
		_, pre, err := analyzer.Analyze("", entries, false)
		if err != nil {
			return err
		}
		e.Log.Log("info", "archive preflight %s: %d entries, %d excluded", f.FilePath, len(pre.Entries), len(pre.Excluded))
		root, err = e.tempDir()
		if err != nil {
			return err
		}
		if err = e.Extractor.Extract(ctx, f.FilePath, root, e.Config.Archive.MaxExtractSizeBytes); err != nil {
			return err
		}
		strategy := "extract"
		analysis.ISOStrategyUsed = &strategy
		return nil
	}
	mount := func() error {
		if e.Mounter == nil {
			return ports.ErrUnsupported
		}
		mountCtx, cancel := context.WithTimeout(ctx, time.Duration(e.Config.ISO.MountTimeoutMS)*time.Millisecond)
		defer cancel()
		drive, err := e.Mounter.Mount(mountCtx, f.FilePath)
		if err != nil {
			return err
		}
		mounted = true
		root = drive
		analysis.MountDrive = &drive
		analysis.DismountStatus = "pending"
		strategy := "mount"
		analysis.ISOStrategyUsed = &strategy
		entries, err := layout.Scan(root, true)
		if err != nil {
			return err
		}
		kind, err := analyzer.MediaKind(entries)
		if err != nil {
			return err
		}
		analysis.MediaKind = &kind
		if kind == "os_install_media" && !e.Config.ISO.AllowOSMedia {
			return errSkipMedia("os_media")
		}
		if kind == "media_disc" && !e.Config.ISO.AllowMediaDisc {
			return errSkipMedia("media_disc")
		}
		return nil
	}
	var err error
	strategy := e.Config.ISO.Strategy
	if !iso {
		strategy = "extract_only"
	}
	switch strategy {
	case "mount_only":
		err = mount()
	case "mount_first":
		err = mount()
		if err != nil && canSwitchTrack(err) {
			err = extract()
			if err == nil {
				analysis.ISOStrategyUsed = schema.String("mount→extract")
			}
		}
	default:
		err = extract()
		if err != nil && canSwitchTrack(err) && strategy == "extract_first" {
			err = mount()
			if err == nil {
				analysis.ISOStrategyUsed = schema.String("extract→mount")
			}
		}
	}
	if err != nil {
		f.Notes = err.Error()
		if !isSkipMedia(err) && !e.MissingArchive {
			f.FinalStatus = "failed"
		}
		f.PlatformMeta["layout_analysis"] = analysis
		e.Result.LayoutAnalysis = analysis
		return f
	}
	if iso && e.Mounter != nil {
		if err = e.Mounter.MotW(f.FilePath, root, e.Config.ISO.MotWPolicy); err != nil {
			f.FinalStatus = "failed"
			f.Notes = "motw_policy_failed: " + err.Error()
			return f
		}
	}
	entries, err := layout.Scan(root, true)
	if err != nil {
		f.FinalStatus = "failed"
		f.Notes = err.Error()
		return f
	}
	before := len(e.Result.Results)
	converged := e.packageRun(ctx, root, entries, depth+1, &analysis)
	f.FinalStatus = "triggered"
	f.Notes = "container processed"
	if converged {
		f.FinalStatus = "install_completed"
		f.InstallConfidence = schema.String("high")
		for _, child := range e.Result.Results[before:] {
			if child.FinalStatus == "install_completed" {
				f.InstallConfidence = child.InstallConfidence
				f.RequiresReboot = f.RequiresReboot || child.RequiresReboot
			}
		}
		keep = iso && !mounted && e.Config.ISO.KeepExtractedOnSuccess
	}
	if keep {
		for i, dir := range e.temps {
			if dir == root {
				e.temps = append(e.temps[:i], e.temps[i+1:]...)
				break
			}
		}
		f.PlatformMeta["retained_extract_dir"] = root
	}
	if !mounted && !keep {
		if err = os.RemoveAll(root); err != nil {
			e.Log.Log("warn", "cleanup %s: %v", root, err)
		}
	}
	f.PlatformMeta["layout_analysis"] = analysis
	e.Result.LayoutAnalysis = analysis
	return f
}

type errSkipMedia string

func (e errSkipMedia) Error() string { return string(e) }
func isSkipMedia(err error) bool     { _, ok := err.(errSkipMedia); return ok }

func canSwitchTrack(err error) bool {
	if isSkipMedia(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := err.Error()
	return !strings.Contains(message, "extract_size_exceeded") && !strings.Contains(message, "unsafe") && !strings.Contains(message, "links are not supported")
}
