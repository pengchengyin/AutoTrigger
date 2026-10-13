package core

import (
	"autotrigger/internal/config"
	"autotrigger/internal/logging"
	"autotrigger/pkg/schema"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// keepFailureLogs 把未判成功的样本的安装器详细日志留到结果文件旁边（F15.9）。
//
// 动机：msiexec 的 `/L*v` 日志写在临时工作目录里，而临时目录在任务结束时被整体
// 删除 —— 偏偏在"装失败了、要查日志"的时候把日志删掉了。1603 这类通用失败码
// 只有对着详细日志才能定位，所以这里在清理之前先把日志复制出去。
//
// 只处理"未判为安装成功"的样本：成功的样本留着日志只是噪音。
func (e *Engine) keepFailureLogs(sample string, f schema.FileResult) {
	if !e.Config.MSI.KeepVerboseLog || f.FinalStatus == "install_completed" {
		return
	}
	var pending []schema.Attempt
	for _, a := range f.Attempts {
		if a.MSILog != nil && strings.TrimSpace(*a.MSILog) != "" {
			pending = append(pending, a)
		}
	}
	if len(pending) == 0 {
		return
	}
	dir := e.failureLogDir()
	if dir == "" {
		e.Log.Stage(logging.LevelWarn, logging.StageCleanup,
			"结果写入 stdout，无法在旁边保留安装器详细日志；可用 msi.log_dir 指定目录：%s", sample)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.Log.Stage(logging.LevelWarn, logging.StageCleanup, "无法创建日志保留目录 %s：%v", dir, err)
		return
	}
	base := trimExt(filepath.Base(sample))
	kept := 0
	for i, a := range pending {
		src := *a.MSILog
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(dir, fmt.Sprintf("%s-%d-%s.log", base, i+1, sanitizeName(a.Method)))
		if err := copyFile(src, dst); err != nil {
			e.Log.Stage(logging.LevelWarn, logging.StageCleanup, "保留安装器日志失败：%s（%v）", dst, err)
			continue
		}
		kept++
	}
	if kept > 0 {
		e.Log.Stage(logging.LevelInfo, logging.StageCleanup,
			"已保留 %d 份安装器详细日志（失败排查用）：%s", kept, dir)
	}
}

// failureLogDir 返回失败日志的保留目录；返回空串表示无处可放（结果写到 stdout）。
// 优先用配置的 msi.log_dir；否则落在结果文件同目录的 <结果名>-msi-logs\。
func (e *Engine) failureLogDir() string {
	if d := strings.TrimSpace(e.Config.MSI.LogDir); d != "" {
		return config.ExpandEnv(d)
	}
	p := strings.TrimSpace(e.ResultPath)
	if p == "" || p == "-" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), trimExt(filepath.Base(p))+"-msi-logs")
}

// trimExt 去掉扩展名；没有扩展名时原样返回。
func trimExt(name string) string {
	if ext := filepath.Ext(name); ext != "" {
		return strings.TrimSuffix(name, ext)
	}
	return name
}

// sanitizeName 把方法名之类的小字符串清成可安全做文件名的形式。
func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "attempt"
	}
	return out
}

// copyFile 复制文件内容（保留原文件不动，供随后清理临时目录使用）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
