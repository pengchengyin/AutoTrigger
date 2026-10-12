//go:build windows

package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func (f *form) detailFilePath() string {
	if f.detailJSON {
		return f.resultPathOrDefault()
	}
	if record, ok := f.selectedTaskRecord(); ok && record.LogPath != "" {
		return record.LogPath
	}
	return f.resultPathOrDefault() + ".log"
}

func (f *form) resetDetailView() {
	h := f.ctrls[idResultView]
	send(h, emSetSel, 0, 0)
	if state := f.logScroll[h]; state != nil {
		state.follow = true
		state.tracking = false
	}
}

func (f *form) detailViewText() string {
	if !f.detailJSON && f.selectedTaskRunning() {
		f.mu.Lock()
		text := string(f.taskLogBuf)
		f.mu.Unlock()
		if strings.TrimSpace(text) == "" {
			return "运行中，等待执行日志…\r\n"
		}
		return toCRLF(text)
	}
	path := f.detailFilePath()
	file, err := os.Open(path)
	if err != nil {
		return "文件暂不可用：" + err.Error()
	}
	defer file.Close()
	if !f.detailJSON {
		stat, err := file.Stat()
		if err != nil {
			return err.Error()
		}
		offset := stat.Size() - (256 << 10)
		if offset > 0 {
			if _, err = file.Seek(offset, 0); err != nil {
				return err.Error()
			}
		}
		data, err := io.ReadAll(io.LimitReader(file, 256<<10))
		if err != nil {
			return err.Error()
		}
		if offset > 0 {
			if i := bytes.IndexByte(data, '\n'); i >= 0 {
				data = data[i+1:]
			}
		}
		if len(data) == 0 {
			return "暂无执行日志。"
		}
		return toCRLF(string(data))
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<20)+1))
	if err != nil {
		return err.Error()
	}
	if len(data) > 64<<20 {
		return "结果 JSON 超过 64 MB，请点击下方路径查看文件。"
	}
	var formatted bytes.Buffer
	if err = json.Indent(&formatted, data, "", "  "); err != nil {
		return fmt.Sprintf("结果 JSON 尚未就绪或格式无效：%v", err)
	}
	return toCRLF(formatted.String())
}
