package core

import (
	"autotrigger/internal/activity"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/logging"
	"autotrigger/internal/verify"
	"autotrigger/pkg/schema"
	"context"
	"fmt"
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
		e.Log.Stage(logging.LevelWarn, logging.StageRun, "无法启动安装进程（方法=%s）：%v", cmd.Method, err)
		return a, obs, false, false, false, false
	}
	defer p.Close()
	defer p.KillTree()
	e.Log.Stage(logging.LevelInfo, logging.StageRun, "已启动安装进程：方法=%s，PID=%d", cmd.Method, p.PID())
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
			e.Log.Stage(logging.LevelWarn, logging.StageRun, "安装进程未正常退出，已强制结束进程树（%s）：%v", path, err)
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
			e.Log.Stage(logging.LevelInfo, logging.StageRun, "安装进行中：已用时 %.0f 秒，静默 %.0f 秒，最近信号=%s", time.Since(start).Seconds(), time.Since(m.LastProgress).Seconds(), m.Signal)
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
					e.Log.Stage(logging.LevelDebug, logging.StageGUI, "界面识别/点击未产生动作：%v", err)
				} else {
					a.GUIActions = append(a.GUIActions, update.Actions...)
					for _, act := range update.Actions {
						e.Log.Stage(logging.LevelInfo, logging.StageGUI, "自动点击：%s（识别来源 %s，窗口 %q）", act.ControlText, act.Source, act.Window)
					}
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
	if a.ExitCode != nil {
		e.Log.Stage(logging.LevelInfo, logging.StageRun, "本轮尝试结束：%s，退出码=%d，耗时 %.1f 秒", attemptResultText(a.Result), *a.ExitCode, a.DurationSec)
	} else {
		e.Log.Stage(logging.LevelInfo, logging.StageRun, "本轮尝试结束：%s，耗时 %.1f 秒", attemptResultText(a.Result), a.DurationSec)
	}
	if cmd.LogPath != "" {
		a.Command += fmt.Sprintf(" [log=%s]", cmd.LogPath)
		a.MSILog = schema.String(cmd.LogPath)
		// MSI 的 /L*v 日志是 UTF-16LE，必须按 BOM 解码后再用（详见 loadInstallerLog）。
		if text := loadInstallerLog(cmd.LogPath, installerLogTailBytes); text != "" {
			if len(text) > 65536 {
				text = text[len(text)-65536:]
			}
			a.StdoutSummary += text
			// 失败时把日志里最有诊断价值的一两行摘出来：1603 这类码本身不说明问题。
			if cmd.MSI && a.ExitCode != nil && !verify.MSICodeIsSuccess(*a.ExitCode) {
				if hint := msiFailureHint(text); hint != "" {
					e.Log.Stage(logging.LevelWarn, logging.StageVerify, "MSI 详细日志要点：%s", hint)
				}
			}
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
	if reboot {
		e.Log.Stage(logging.LevelWarn, logging.StageGUI, "安装程序提示需要重启才能完成，按配置停止后续尝试")
	}
	return a, obs, true, timedout, reboot || obs.RequiresReboot, network
}

// attemptResultText 把尝试结果翻译成中文说明（结果 JSON 里仍保留英文原值）。
func attemptResultText(result string) string {
	switch result {
	case "exited":
		return "安装进程已自行退出"
	case "killed_on_timeout":
		return "超时被强制终止"
	case "failed":
		return "长时间无活动，判定为无有效安装行为"
	default:
		return result
	}
}
