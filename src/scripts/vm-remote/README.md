# SSH 远程下发安装任务（测试虚拟机运维脚本）

本目录是**运维/测试工具**，不是产品交付物。用于在无法直接操作虚拟机桌面时，
把 AutoTrigger 桌面端拉起到交互会话，再通过命名管道下发安装任务。

## 前置条件

1. 虚拟机已装 AutoTrigger，且 `preferences.json` 中 `remote_tasks: true`（设置里「允许通过 SSH 提交任务」）。
2. UAC 为静默提权：`ConsentPromptBehaviorAdmin=0`、`PromptOnSecureDesktop=0`，`Application Information` 服务运行中。
3. 桌面端**必须以管理员身份运行**，见下文「为什么必须提权」。

## 用法

```bash
# 1) 把两个脚本复制到虚拟机（路径随实际部署调整）
scp start-desktop-elevated.* administrator@<host>:C:/Users/Administrator/AppData/Local/Temp/

# 2) 注册交互计划任务（InteractiveToken 才能落在 Console 会话），再运行
schtasks /create /tn "AutoTriggerGui" /tr "C:\Users\Administrator\AppData\Local\Temp\start-desktop-elevated.bat" \
    /sc once /st 00:00 /ru <account> /rp <password> /it /f
schtasks /run /tn "AutoTriggerGui"

# 3) 确认接收端在线（ok=true 才表示桌面端在跑）
AutoTrigger.exe task health
```

之后即可 `task submit --input <主机上绝对路径> [--name 名称] [--sha256 值]`，
并用 `task status|logs|result|cancel --id <ID>` 查询。

## 为什么必须提权（实测结论）

- 需要管理员的安装器（如 Inno Setup 且 `PrivilegesRequired=admin`）其窗口完整性级别为 High（`0x3000`），
  未提权的工具是 Medium（`0x2000`），点击会被 Windows UIPI 拦截。
  现象：结果 JSON 出现 `requires_elevation`、判定失败，`prereq.issues` 给出「建议以管理员身份运行」。
- **`schtasks /rl highest` 从非提权进程注册无效**：任务 XML 中确实写入 `<RunLevel>HighestAvailable</RunLevel>`，
  但任务内 `whoami /groups` 仍是 `Medium Mandatory Level`（受限令牌）。
- 因此本脚本改用「交互计划任务 + `Shell.Application.ShellExecute(..., "runas", ...)`」：
  UAC 静默提权时不会弹窗，实测任务内探针返回 `High Mandatory Level`，GUI 自动化（本机为 UIA）即可正常点击安装器向导。

## 说明

- 脚本中的绝对路径（`C:\Users\Administrator\Desktop\AutoTrigger\`）按实际部署修改。
- 计划任务使用 `/rp` 会保存账户口令；若不适合，可在已提权的 GUI 中勾选产品自带的「管理员自启」
  （注册登录触发的 `HighestAvailable` 计划任务），该功能本身也要求先以管理员身份运行工具。
- 上传样本建议先用 `.uploading` 后缀，传完改名再提交，并用 `--sha256` 校验。
