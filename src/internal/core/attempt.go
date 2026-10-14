package core

import (
	"autotrigger/internal/activity"
	"autotrigger/internal/core/layout"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/logging"
	"autotrigger/internal/verify"
	"autotrigger/pkg/schema"
	"context"
	"errors"
	"fmt"
	"time"
)

func (e *Engine) attempt(ctx context.Context, cmd ports.Command, base *ports.Baseline, path string) (schema.Attempt, verify.Observation, bool, bool, bool, bool) {
	start := time.Now()
	a := schema.Attempt{Method: cmd.Method, Command: commandText(cmd), Result: "failed", TerminationMode: "natural", InstallEvidence: []schema.Evidence{}, GUIActions: []schema.GUIAction{}, ArtifactCheck: "unknown"}
	obs := verify.Observation{Evidence: []schema.Evidence{}}
	for _, target := range []string{path, cmd.Program} {
		matched, err := layout.ExecutionExclusion(target, e.Pack, e.Config)
		if err != nil || matched != "" {
			a.Error = schema.String(fmt.Sprintf("execution excluded: %s %v", matched, err))
			return a, obs, false, false, false, false
		}
	}
	p, err := e.Runner.Start(ctx, cmd)
	if err != nil {
		a.Error = schema.String(err.Error())
		if errors.Is(err, ports.ErrElevationRequired) {
			a.Error = schema.String("requires_elevation: 当前权限无法启动安装器，请以管理员身份运行工具后重试。" + err.Error())
		}
		a.DurationSec = time.Since(start).Seconds()
		e.Log.Stage(logging.LevelWarn, logging.StageRun, "无法启动安装进程（方法=%s）：%v", cmd.Method, err)
		return a, obs, false, false, false, false
	}
	defer p.Close()
	defer p.KillTree()
	e.Log.Stage(logging.LevelInfo, logging.StageRun, "已启动安装进程：方法=%s，PID=%d", cmd.Method, p.PID())
	m := activity.New(start, e.Config)
	m.PassiveHost = cmd.Method == "document_host"
	m.Last.PIDs = []int{p.PID()}
	m.Last.Signature = verify.Digest(base)
	tick := time.NewTicker(time.Duration(e.Config.GUI.PollIntervalMS) * time.Millisecond)
	defer tick.Stop()
	heartbeat := time.NewTicker(time.Duration(e.Config.ProgressIntervalSec) * time.Second)
	defer heartbeat.Stop()
	guiEvidence := []schema.Evidence{}
	timedout, reboot, network := false, false, false
	guiSignature := ""
	guiErrorText := ""
	filesystemSignature := verify.Digest(base)
	lastSnapshot := time.Now()
	observed := &ports.Baseline{Registry: map[string]ports.UninstallEntry{}, Dirs: map[string]ports.DirState{}}
	var exit ports.Exit
	terminated := false
	lastSample := start
	checkSamplingGap := func() {
		now := time.Now()
		if gap, unexpected := unexpectedSamplingGap(lastSample, now, time.Duration(e.Config.GUI.PollIntervalMS)*time.Millisecond); unexpected {
			e.Log.Stage(logging.LevelWarn, logging.StageRun, "进度采样间隔异常：%.1f 秒。可能发生系统挂起或识别阻塞；本次耗时未自动扣除此间隔，超时仍按原配置执行。", gap.Seconds())
		}
		lastSample = now
	}
	finish := func(reason string) {
		checkSamplingGap()
		timedout = reason != "dump_ui" && reason != "requires_reboot" && reason != "requires_elevation"
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		mode, err := p.Terminate(cleanupCtx)
		cancel()
		if err != nil {
			e.Log.Stage(logging.LevelWarn, logging.StageRun, "安装进程未正常退出，已强制结束进程树（%s）：%v", path, err)
			p.KillTree()
		}
		a.TerminationMode = mode
		a.Result = "killed_on_timeout"
		if reason == "no_activity" || reason == "requires_elevation" {
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
			checkSamplingGap()
			a.Result = "exited"
			break loop
		case <-ctx.Done():
			finish("deadline_or_interrupt")
			break loop
		case <-heartbeat.C:
			e.Log.Stage(logging.LevelInfo, logging.StageRun, "执行等待中：已用时 %.0f 秒，无进展 %.0f 秒，最近信号=%s，监控PID=%v（活动信号不代表安装成功）", time.Since(start).Seconds(), time.Since(m.LastProgress).Seconds(), m.Signal, m.Last.PIDs)
		case <-tick.C:
			checkSamplingGap()
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
					if errors.Is(err, ports.ErrElevationRequired) {
						a.GUIActions = append(a.GUIActions, update.Actions...)
						guiEvidence = append(guiEvidence, update.Evidence...)
						e.Log.Stage(logging.LevelWarn, logging.StageGUI, "权限阻止自动操作，停止本样本：%v", err)
						finish("requires_elevation")
						a.Error = schema.String("requires_elevation: 请以管理员身份运行工具后重新执行。" + err.Error())
						break loop
					}
					// 首次出现按警告提示（如 UIPI 权限拦截、窗口无响应），重复出现降为调试，避免刷屏。
					if err.Error() != guiErrorText {
						guiErrorText = err.Error()
						e.Log.Stage(logging.LevelWarn, logging.StageGUI, "界面识别/点击未产生动作：%v", err)
					} else {
						e.Log.Stage(logging.LevelDebug, logging.StageGUI, "界面识别/点击未产生动作：%v", err)
					}
				} else {
					guiErrorText = ""
					for _, note := range update.Notes {
						e.Log.Stage(logging.LevelDebug, logging.StageGUI, "界面识别详情：%s", note)
					}
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
			if !cmd.NonInstall && cmd.Method != "document_host" && time.Since(lastSnapshot) >= 2*time.Second {
				if current, err := e.Evidence.Snapshot(ctx, e.Config.Verify); err == nil {
					filesystemSignature = verify.Digest(current)
					for key, entry := range current.Registry {
						if _, exists := base.Registry[key]; !exists {
							observed.Registry[key] = entry
						}
					}
					if !current.Truncated {
						for dir, state := range current.Dirs {
							if _, exists := base.Dirs[dir]; !exists {
								observed.Dirs[dir] = state
							}
						}
					}
				}
				lastSnapshot = time.Now()
			}
			if cmd.Method != "document_host" {
				progress.Signature = filesystemSignature
			}
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
		if cmd.MSI {
			e.Log.Stage(logging.LevelInfo, logging.StageRun, "Windows Installer 退出状态：%s", verify.MSICodeNote(*a.ExitCode))
		}
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
	network = network || outputRequiresNetwork(e.Pack, a.StdoutSummary+"\n"+a.StderrSummary)
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
	if cmd.NonInstall || cmd.Method == "document_host" {
		return a, obs, true, timedout, reboot, network
	}
	verifyCtx, cancel := context.WithTimeout(context.Background(), time.Duration(e.Config.Verify.SettleMaxSec+2)*time.Second)
	defer cancel()
	verifier := verify.Verifier{Source: e.Evidence, Config: e.Config.Verify}
	obs, err = verifier.Observe(verifyCtx, path, base, cmd, a.ExitCode, filtered, a.StdoutSummary+"\n"+a.StderrSummary, observed)
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
