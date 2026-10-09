package verify

import (
	"strconv"

	"autotrigger/pkg/schema"
)

// Windows Installer（msiexec）常见退出码的中文释义。
//
// 现场只看裸数字（尤其 1603）无法判断"是权限问题、包本身有问题，还是前置条件不满足"，
// 因此把最常见的码翻成一句中文直接打进日志（F10.1）。未收录的码原样显示，不影响判定。
//
// 注意：这里只做**展示**，判定口径不读本表 —— 是否算成功仍由 MSICodeIsSuccess 决定，
// 两者都以此文件为唯一事实来源，避免"日志说成功、判定说失败"。
var msiCodeText = map[int]string{
	1601: "Windows Installer 服务不可用（服务被禁用或损坏）",
	1602: "用户取消了安装",
	1603: "安装过程中发生致命错误（常见：未以管理员身份运行、前置条件不满足、自定义操作失败、磁盘空间不足；详因见 MSI 详细日志）",
	1605: "该产品未安装（无法卸载或修复）",
	1618: "另一个安装正在进行中（稍后重试即可）",
	1619: "无法打开安装包（文件损坏或不可读）",
	1620: "安装包无效（不是合法的 MSI）",
	1622: "无法打开日志文件",
	1625: "被系统策略禁止安装（组策略限制）",
	1633: "本平台不支持该安装包（常见：64 位包装到 32 位系统）",
	1638: "已安装该产品的其他版本",
	1639: "命令行参数无效",
	1641: "安装成功，并已启动重启",
	1642: "无法启动 Windows Installer 服务",
	3010: "安装成功，需要重启才能生效",
}

// MSICodeText 返回 msiexec 退出码的中文释义；未收录时返回空串。
func MSICodeText(code int) string { return msiCodeText[code] }

// MSICodeIsSuccess 判断 msiexec 退出码是否表示安装成功（含"需重启"）。
func MSICodeIsSuccess(code int) bool {
	return code == 0 || code == 3010 || code == 1641
}

// MSICodeNote 把退出码渲染成「1603：中文含义」；未收录时只给数字。
//
// 用冒号而不是括号：释义本身常含括号（如「安装包无效（不是合法的 MSI）」），
// 再套一层会写成「1620（安装包无效（…））」，嵌套括号读起来很别扭。
func MSICodeNote(code int) string {
	if t := MSICodeText(code); t != "" {
		return strconv.Itoa(code) + "：" + t
	}
	return strconv.Itoa(code)
}

// ConflictText 把证据冲突渲染成一句中文，说明"为什么判成这样"。
//
// 冲突码是排查入口，裸码（如 N1：1603）对现场没有意义；这里补上人类可读的解释，
// 但**不改变冲突本身的判定语义**（conflicts 数组仍原样写入结果 JSON）。
func ConflictText(c schema.Evidence) string {
	switch c.ID {
	case "N1":
		if code, err := strconv.Atoi(c.Detail); err == nil {
			return "安装进程返回失败退出码 " + MSICodeNote(code)
		}
		return "安装进程返回失败退出码 " + c.Detail
	case "N2":
		return "安装界面出现失败或回滚提示（" + c.Detail + "）"
	case "N3":
		return "安装完成后又被卸载（MSI 卸载事件）"
	case "N4":
		return "已安装的注册表项被移除（疑似回滚）：" + c.Detail
	case "N5":
		return "监控目录实体数量减少（疑似回滚）：" + c.Detail
	}
	return c.Detail
}
