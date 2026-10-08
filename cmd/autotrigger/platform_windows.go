//go:build windows

package main

import (
	"autotrigger/internal/cli"
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/windows"
	"autotrigger/internal/resources"
	"autotrigger/internal/ui"
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

// launchUI 决定是否进入图形界面（PRD F20），返回 (退出码, 是否已接管)。
//
// 判定规则刻意做窄，避免影响脚本与第三方集成（F19）：
//   - 双击启动：**无任何参数**，且当前属于"独占的新控制台"（见 ui.LooksLikeDoubleClick）。
//     只有从资源管理器双击才会同时满足；脚本、流水线调用（通常重定向输出）
//     以及在已有终端里手敲命令（会附加到该终端）都仍按原有
//     "无参数 → 用法提示 + 退出码 2" 处理，不会被意外弹窗。
//   - 显式 `--gui`：强制打开界面（Windows 专用）。
func launchUI(args []string) (int, bool) {
	if len(args) == 0 && ui.LooksLikeDoubleClick() {
		return ui.Run(dependencies()), true
	}
	if len(args) == 1 && (args[0] == "--gui" || args[0] == "-gui") {
		return ui.Run(dependencies()), true
	}
	return 0, false
}
