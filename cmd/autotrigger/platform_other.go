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
