package main

import (
	"autotrigger/internal/cli"
	"context"
	"os"
	"os/signal"
)

func main() {
	if internalHelper() {
		return
	}
	args := os.Args[1:]
	// 图形界面优先：双击启动（无参数且连着真实控制台）或显式 --gui。
	// 其余情况一律走命令行，脚本与第三方集成（F19）行为完全不变。
	if code, ok := launchUI(args); ok {
		os.Exit(code)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	os.Exit(cli.Run(ctx, args, os.Stdout, os.Stderr, dependencies()))
}
