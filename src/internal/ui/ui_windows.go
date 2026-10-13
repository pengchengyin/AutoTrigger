//go:build windows

package ui

import (
	"autotrigger/internal/cli"
)

// Run 打开图形界面并阻塞到窗口关闭，返回进程退出码。
//
// 前置动作的作用：
//   - lockMainThread：窗口、消息循环与窗口过程必须始终在同一线程执行；
//   - detachConsole：进入 GUI 时脱离控制台，不改变已有用户终端；
//   - SetProcessDPIAware：按真实 DPI 布局与选字体，避免高缩放比下界面发虚；
//   - CoInitializeEx：SHBrowseForFolder 的新式样式需要本线程已初始化 OLE。
func Run(deps cli.Dependencies) int {
	lockMainThread()
	if err := detachConsole(); err != nil {
		messageBox(0, "AutoTrigger", "无法脱离控制台："+err.Error(), mbOK|mbIconError)
		return 1
	}
	procSetProcessDPIAware.Call()
	procCoInitializeEx.Call(0, coInitApartmentThreaded)

	f := newForm(deps)
	if err := f.create(); err != nil {
		messageBox(0, "AutoTrigger",
			"无法创建图形界面："+err.Error()+"\n\n可改用命令行参数运行，见 --help。",
			mbOK|mbIconError)
		return 1
	}
	f.show()
	f.loop()
	return f.exitCode
}

// Available 报告本平台是否提供图形界面。
func Available() bool { return true }
