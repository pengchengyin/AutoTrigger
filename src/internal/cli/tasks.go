package cli

import (
	"autotrigger/internal/datadir"
	"autotrigger/internal/remotetask"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func runTaskCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(stdout, "用法：AutoTrigger.exe task submit --input <主机上传文件绝对路径> [--name 名称] [--sha256 校验值] [--id 请求ID]\n      AutoTrigger.exe task status|logs|result|cancel --id <任务ID>\n      AutoTrigger.exe task health|list\n日志可使用 --offset <字节偏移> --limit <最多1048576字节>。命令返回 JSON；桌面端须开启远程提交。")
		return 0
	}
	r := remotetask.Request{Action: args[0]}
	fs := flag.NewFlagSet("task", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&r.ID, "id", "", "task/request ID")
	fs.StringVar(&r.Input, "input", "", "host upload file")
	fs.StringVar(&r.Name, "name", "", "task name")
	fs.StringVar(&r.SHA256, "sha256", "", "expected SHA-256")
	fs.Int64Var(&r.Offset, "offset", 0, "log byte offset")
	fs.IntVar(&r.Limit, "limit", 65536, "log bytes")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "任务命令有多余参数")
		return 2
	}
	switch r.Action {
	case "submit":
		if r.Input == "" {
			fmt.Fprintln(stderr, "请指定 --input")
			return 2
		}
		if r.ID == "" {
			id, err := remotetask.NewID()
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			r.ID = id
		}
	case "status", "logs", "result", "cancel":
		if !remotetask.ValidID(r.ID) {
			fmt.Fprintln(stderr, "请指定有效的 --id")
			return 2
		}
	case "health", "list":
	default:
		fmt.Fprintln(stderr, "未知任务命令，请运行 task --help")
		return 2
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	layout, err := datadir.Load(filepath.Dir(exe))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	response, err := remotetask.Call(ctx, layout.Root, r)
	if err != nil {
		fmt.Fprintf(stderr, "任务通信失败：%v；请求 ID=%s（提交超时时可按相同 ID 重试）\n", err, r.ID)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(response); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !response.OK {
		return 1
	}
	return 0
}
