//go:build windows

package main

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/windows"
	"autotrigger/internal/resources"
	"os"
	"path/filepath"
)

func dependencies() cli.Dependencies {
	runner := windows.Runner{}
	exe, _ := os.Executable()
	prereqs := &windows.Prereqs{Runner: runner, ExeDir: filepath.Dir(exe)}
	return cli.Dependencies{Runner: runner, Trigger: func(c config.Config, p resources.Pack) ports.InstallerTrigger {
		return windows.Trigger{Config: c, Pack: p}
	}, Opener: windows.Opener{}, Mounter: windows.Mounter{Runner: runner}, Evidence: windows.Evidence{}, NativeGUI: windows.GUI{}, Lock: windows.MachineLock{}, Prereqs: prereqs, OCR: func() ports.OCREngine {
		if prereqs.Engine == nil {
			return nil
		}
		return prereqs.Engine
	}}
}
func internalHelper() bool { return windows.InternalHelper() }
