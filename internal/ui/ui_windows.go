//go:build windows

package ui

import (
	"autotrigger/internal/cli"
)

// Run 打开图形界面并阻塞到窗口关闭，返回进程退出码。
//
// 前置动作的作用：
//   - lockMainThread：窗口、消息循环与窗口过程必须始终在同一线程执行；
//   - hideConsole：双击启动时系统会先分配一个控制台窗口，隐藏它以只留界面；
//   - SetProcessDPIAware：按真实 DPI 布局与选字体，避免高缩放比下界面发虚；
//   - CoInitializeEx：SHBrowseForFolder 的新式样式需要本线程已初始化 OLE。
func Run(deps cli.Dependencies) int {
	lockMainThread()
	hideConsole()
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
