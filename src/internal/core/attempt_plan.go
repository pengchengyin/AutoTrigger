package core

import (
	"autotrigger/internal/core/ports"
	"fmt"
	"strings"
)

func attemptPlanText(commands []ports.Command) string {
	labels := map[string]string{"silent_install": "静默安装", "no_param_run": "直接运行", "msi": "Windows 安装包执行", "script": "脚本执行", "dll_probe": "动态库入口探测", "lnk_target": "快捷方式目标执行", "document_host": "关联程序打开"}
	counts := map[string]int{}
	order := []string{}
	for _, cmd := range commands {
		if counts[cmd.Method] == 0 {
			order = append(order, cmd.Method)
		}
		counts[cmd.Method]++
	}
	parts := make([]string, 0, len(order))
	for _, method := range order {
		label := labels[method]
		if label == "" {
			label = "其他方式（" + method + "）"
		}
		unit := "次"
		if method == "silent_install" {
			unit = "组参数"
		}
		parts = append(parts, fmt.Sprintf("%s %d %s", label, counts[method], unit))
	}
	return strings.Join(parts, "、")
}
