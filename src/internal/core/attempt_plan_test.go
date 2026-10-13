package core

import (
	"autotrigger/internal/core/ports"
	"testing"
)

func TestAttemptPlanText(t *testing.T) {
	commands := make([]ports.Command, 8)
	for i := range commands {
		commands[i].Method = "silent_install"
	}
	commands = append(commands, ports.Command{Method: "no_param_run"})
	if got := attemptPlanText(commands); got != "静默安装 8 组参数、直接运行 1 次" {
		t.Fatal(got)
	}
	if got := attemptPlanText([]ports.Command{{Method: "script"}, {Method: "custom"}, {Method: "script"}}); got != "脚本执行 2 次、其他方式（custom） 1 次" {
		t.Fatal(got)
	}
	if got := attemptPlanText(nil); got != "" {
		t.Fatal(got)
	}
}
