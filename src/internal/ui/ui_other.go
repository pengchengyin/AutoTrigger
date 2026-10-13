//go:build !windows

package ui

import (
	"autotrigger/internal/cli"
	"fmt"
	"os"
)

// Run 在非 Windows 平台上不可用。
//
// 界面按"原生窗口"实现（PRD F20），与 GUI 自动化一样属于平台相关能力：
// 新增平台时按 4.8 的扩展方式各自实现，core 与 internal/verify 不受影响。
// 这里保留可编译的实现，保证 GOOS=linux/darwin 的编译卡点（A9 ③）通过。
func Run(deps cli.Dependencies) int {
	fmt.Fprintln(os.Stderr, "[error] 图形界面目前仅在 Windows 上提供；请改用命令行参数运行，见 --help")
	return 2
}

// Available 报告本平台是否提供图形界面。
func Available() bool { return false }
