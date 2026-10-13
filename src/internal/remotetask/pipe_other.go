//go:build !windows

package remotetask

import (
	"context"
	"fmt"
	"os"
)

func Call(context.Context, string, Request) (Response, error) {
	return Response{}, fmt.Errorf("任务提交命令须在 Windows 主机的 SSH 会话中运行")
}
func openSnapshot(path string) (*os.File, error) { return os.Open(path) }
