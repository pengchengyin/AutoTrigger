//go:build !windows

package main

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/stub"
	"autotrigger/internal/resources"
)

func dependencies() cli.Dependencies {
	a := stub.Adapter{}
	return cli.Dependencies{Runner: a, Trigger: func(config.Config, resources.Pack) ports.InstallerTrigger { return a }, Opener: a, Mounter: a, Evidence: a, NativeGUI: a, Lock: a, Prereqs: a}
}
func internalHelper() bool { return false }

// launchUI 在非 Windows 平台上不提供图形界面（PRD F20：界面以原生 Win32 实现）。
// 返回 ok=false，调用方继续走命令行，行为与既有版本一致。
func launchUI([]string) (int, bool) { return 0, false }
