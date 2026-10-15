Option Explicit
' 以 runas 动词在交互桌面「静默提权」启动 AutoTrigger 桌面端。
' 适用：SSH/服务会话（session 0）无法直接进入用户交互桌面（session 1）时拉起接收端。
' 前提：UAC 已配置为静默提权（ConsentPromptBehaviorAdmin=0、PromptOnSecureDesktop=0）。
' 注意：路径按实际部署修改。
Dim sh
Set sh = CreateObject("Shell.Application")
sh.ShellExecute "C:\Users\Administrator\Desktop\AutoTrigger\AutoTrigger.exe", "--gui", "C:\Users\Administrator\Desktop\AutoTrigger", "runas", 1
