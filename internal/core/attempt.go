package core

import (
	"autotrigger/internal/activity"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/verify"
	"autotrigger/pkg/schema"
	"context"
	"fmt"
	"os"
	"time"
)

func (e *Engine) attempt(ctx context.Context, cmd ports.Command, base *ports.Baseline, path string) (schema.Attempt, verify.Observation, bool, bool, bool, bool) {
	start := time.Now()
	a := schema.Attempt{Method: cmd.Method, Command: commandText(cmd), Result: "failed", TerminationMode: "natural", InstallEvidence: []schema.Evidence{}, GUIActions: []schema.GUIAction{}, ArtifactCheck: "unknown"}
	obs := verify.Observation{Evidence: []schema.Evidence{}}
	p, err := e.Runner.Start(ctx, cmd)
	if err != nil {
		a.Error = schema.String(err.Error())
		a.DurationSec = time.Since(start).Seconds()
		return a, obs, false, false, false, false
	}
	defer p.Close()
	defer p.KillTree()
	e.Log.Log("info", "attempt %s pid=%d file=%s", cmd.Method, p.PID(), path)
	m := activity.New(start, e.Config)
	m.Last.PIDs = []int{p.PID()}
	tick := time.NewTicker(time.Duration(e.Config.GUI.PollIntervalMS) * time.Millisecond)
	defer tick.Stop()
	heartbeat := time.NewTicker(time.Duration(e.Config.ProgressIntervalSec) * time.Second)
	defer heartbeat.Stop()
	guiEvidence := []schema.Evidence{}
	timedout, reboot, network := false, false, false
	guiSignature := ""
	filesystemSignature := verify.Digest(base)
	lastSnapshot := time.Now()
	var exit ports.Exit
	terminated := false
	finish := func(reason string) {
		timedout = reason != "dump_ui" && reason != "requires_reboot"
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		mode, err := p.Terminate(cleanupCtx)
		cancel()
		if err != nil {
			e.Log.Log("warn", "process termination %s: %v", path, err)
			p.KillTree()
		}
		a.TerminationMode = mode
		a.Result = "killed_on_timeout"
		if reason == "no_activity" {
			a.Result = "failed"
		}
		if reason == "dump_ui" || reason == "requires_reboot" {
			a.Result = "exited"
		}
		a.Error = schema.String(reason)
		select {
		case exit = <-p.Done():
		case <-time.After(time.Second):
		}
		terminated = true
	}
loop:
	for {
		select {
		case exit = <-p.Done():
			a.Result = "exited"
			break loop
		case <-ctx.Done():
			finish("deadline_or_interrupt")
			break loop
		case <-heartbeat.C:
			e.Log.Log("info", "progress file=%s elapsed=%.1fs idle=%.1fs last_signal=%s", path, time.Since(start).Seconds(), time.Since(m.LastProgress).Seconds(), m.Signal)
		case <-tick.C:
			progress, err := p.Activity()
			if err != nil {
				progress = ports.Activity{PIDs: []int{p.PID()}}
			}
			if len(progress.PIDs) == 0 {
				progress.PIDs = []int{p.PID()}
			}
			if e.DumpDir != "" && e.GUI != nil {
				if err := e.GUI.Dump(ctx, progress.PIDs, e.DumpDir); err == nil {
					finish("dump_ui")
					break loop
				}
			}
			extra := ""
			if e.GUI != nil && e.Config.GUI.Enabled && e.DumpDir == "" {
				update, err := e.GUI.Step(ctx, progress.PIDs)
				if err != nil {
					e.Log.Log("debug", "GUI: %v", err)
				} else {
					a.GUIActions = append(a.GUIActions, update.Actions...)
					if len(update.Actions) > 0 {
						extra = "gui_action"
					}
					if update.Signature != guiSignature {
						extra = "gui_change"
						guiSignature = update.Signature
					}
					guiEvidence = append(guiEvidence, update.Evidence...)
					reboot = reboot || update.RequiresReboot
					network = network || update.RequiresNetwork
				}
			}
			if time.Since(lastSnapshot) >= 2*time.Second {
				if current, err := e.Evidence.Snapshot(ctx, e.Config.Verify); err == nil {
					filesystemSignature = verify.Digest(current)
				}
				lastSnapshot = time.Now()
			}
			progress.Signature = filesystemSignature
			m.Update(time.Now(), progress, extra)
			if reboot {
				finish("requires_reboot")
				break loop
			}
			if why := m.Timeout(time.Now()); why != "" {
				finish(why)
				break loop
			}
		}
	}
	a.IdleResets = m.Resets
	a.DurationSec = time.Since(start).Seconds()
	a.StdoutSummary = exit.Stdout
	a.StderrSummary = exit.Stderr
	if !terminated {
		a.ExitCode = &exit.Code
		if exit.Err != nil {
			a.Error = schema.String(exit.Err.Error())
		}
	}
	if cmd.LogPath != "" {
		a.Command += fmt.Sprintf(" [log=%s]", cmd.LogPath)
		if data, err := os.ReadFile(cmd.LogPath); err == nil {
			if len(data) > 65536 {
				data = data[len(data)-65536:]
			}
			a.StdoutSummary += string(data)
		}
	}
	// A later successful GUI finish supersedes earlier transient failure-page observations.
	finishAt := time.Time{}
	for _, ev := range guiEvidence {
		if ev.ID == "A2" && ev.At.After(finishAt) {
			finishAt = ev.At
		}
	}
	filtered := []schema.Evidence{}
	for _, ev := range guiEvidence {
		if ev.ID == "N2" && !finishAt.IsZero() && ev.At.Before(finishAt) {
			continue
		}
		filtered = append(filtered, ev)
	}
	verifyCtx, cancel := context.WithTimeout(context.Background(), time.Duration(e.Config.Verify.SettleMaxSec+2)*time.Second)
	defer cancel()
	verifier := verify.Verifier{Source: e.Evidence, Config: e.Config.Verify}
	obs, err = verifier.Observe(verifyCtx, path, base, cmd, a.ExitCode, filtered, a.StdoutSummary+"\n"+a.StderrSummary)
	if err != nil {
		a.Error = schema.String("verification: " + err.Error())
		obs.Evidence = filtered
	}
	a.InstallEvidence = obs.Evidence
	a.ArtifactCheck = obs.Artifact
	return a, obs, true, timedout, reboot || obs.RequiresReboot, network
}
