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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, dependencies()))
}
