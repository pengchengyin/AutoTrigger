# SSH 远程任务说明

## 1. 工作方式

SSH 负责传输文件和提交请求；安装、截图、UIA、OCR 由主机上已登录桌面的 AutoTrigger 执行。不要在 SSH 会话中直接运行旧的 `--input` 命令来替代桌面任务提交，它不能保证访问交互桌面。

工具不安装 OpenSSH、不修改防火墙、不自动登录、不关闭 UAC，也不新增网络监听端口。桌面端与 SSH 命令通过本机命名管道通信。

## 2. 主机准备

1. 建议首期使用 Windows 10/11，安装并启动 OpenSSH Server，按管理要求配置防火墙、SSH 密钥和允许登录的账户。参考[微软 OpenSSH 部署说明](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh_install_firstuse)。Win7 的 SSH 部署与实机兼容未验证。
2. 使用指定 Windows 账户登录桌面，打开 AutoTrigger；系统级安装按需以管理员身份启动工具。不建议将 SSH 管理入口直接暴露在公网。
3. 在设置中完成数据目录、超时、OCR 与依赖配置，检查环境。桌面端必须保持运行。
4. 勾选基础设置中的“允许通过 SSH 提交任务”，核对权限提示并保存。默认关闭，不允许其他 Windows 账户提交。
5. 在该账户的终端执行以下命令，确认接收端及上传目录：

```powershell
.\AutoTrigger.exe task health
```

SSH 登录账户必须与运行桌面执行端的账户相同。该能力允许提交的代码以桌面端权限运行，启用后只应向可信客户端提供该账户的访问权限；管理员模式下风险尤其高。命名管道仅允许同账户访问并拒绝远程管道连接，不提供跨账户授权或任意命令执行接口。

## 3. 上传并提交

文件可以放在目标主机的任意本地目录，只要桌面执行端账户有读取权限。`task health` 返回的 `inbox`（数据目录中的 `config/remote/inbox`）仅为可选上传位置，不是提交范围限制。

使用 SFTP、SCP 或 WinSCP 上传，也可以提交主机已有的安装包。建议先使用 `.uploading` 后缀，传完后改回正式文件名，再提交；必要时计算 SHA-256 并通过 `--sha256` 核对。接受不超过 4 GiB 的普通文件，文件夹需先打包；要求本地绝对路径，解析链接后仍须为本地路径，不接受 UNC 网络路径。提交时复制文件为任务快照，后续修改原文件不影响已接收任务。

安装器依赖同级 CAB、数据文件或其他资源时，应把完整安装目录打包上传，不要只提交入口 EXE/MSI。`task.source` 保留原上传路径，`task.input` 和结果中的样本路径指向实际执行的快照或其解压内容。

以下示例假设主机程序位于 `D:\AutoTrigger\AutoTrigger.exe`，数据目录也为 `D:\AutoTrigger`。将 `installer@host` 替换为实际连接信息。示例使用 Bash 客户端及 Windows 默认命令行；其他 Shell 或包含空格的程序路径需按其规则调整引号。

```bash
ssh installer@host 'D:\AutoTrigger\AutoTrigger.exe task health'
ssh installer@host 'D:\AutoTrigger\AutoTrigger.exe task submit --input D:\AutoTrigger\config\remote\inbox\setup.exe --name RemoteInstall'
```

`submit` 返回 JSON，其中 `task.id` 是远程任务 ID。`ok=true` 仅表示已经接收并落盘，不能表示安装成功。提交端复制安装包与当前已保存的有效配置为任务快照；后续覆盖上传文件或修改设置不改变已接收的任务。OCR 和其他参数继承保存的设置，远程请求不能指定独立配置、关闭机器锁或传入任意命令。

SSH 断开不终止已接受任务。提交命令超时时，错误信息会给出请求 ID，可使用相同 `--id`、输入路径及校验值重新提交，避免重复安装。中断任务不会自动重试；确认结果和主机状态后，使用新请求 ID 重新提交。

## 4. 查询与停止

将示例中的 `TASK_ID` 替换为返回的 32 位任务 ID：

```bash
ssh installer@host 'D:\AutoTrigger\AutoTrigger.exe task list'
ssh installer@host 'D:\AutoTrigger\AutoTrigger.exe task status --id TASK_ID'
ssh installer@host 'D:\AutoTrigger\AutoTrigger.exe task logs --id TASK_ID --offset 0 --limit 65536'
ssh installer@host 'D:\AutoTrigger\AutoTrigger.exe task result --id TASK_ID'
ssh installer@host 'D:\AutoTrigger\AutoTrigger.exe task cancel --id TASK_ID'
```

接收端业务响应返回 JSON；参数或通信失败写入标准错误。命令退出码 0 表示请求成功，1 表示通信或业务失败，2 表示参数错误。不会改变原 `--input` 命令的退出码契约。

- `status`：任务状态、真实执行退出码、文件路径及等待原因。
- `logs`：当前日志片段和 `next_offset`；用返回的字节偏移继续查询，每次 4 字节至 1 MiB，按 UTF-8 字符边界返回，不包含其他任务日志。
- `result`：已有结果文件的 JSON；尚未生成或不完整时明确返回失败。
- `cancel`：排队任务立即取消；运行任务记录取消请求，桌面端调用原有中止流程。请求成功不等于已经停止，请继续查询。

远程任务也出现在任务配置列表，可查看详情、实时日志及结果；排队任务可通过详情中的停止按钮取消。远程任务 ID 与结果 JSON 内的 CLI `task_id` 是两种标识，使用 `task.id` 查询远程任务，使用结果中的 `task_id` 对接已有结果契约。

## 5. 状态与运行限制

| 状态 | 含义 |
|---|---|
| `queued` | 已接收，等待桌面空闲和可交互 |
| `running` | 已开始处理；不代表安装成功 |
| `completed` | 执行流程结束，必须查看退出码和结果 JSON |
| `cancelled` | 已取消；已运行的安装器可能已经产生部分变更 |
| `interrupted` | 执行端异常退出、桌面失去交互能力或快照无效；不自动重放 |

本地和远程安装任务不会在同一桌面端同时执行；远程任务按队列顺序调度，模板捕获期间等待。远程任务强制使用既有机器锁。

执行前检查实际桌面会话及输入桌面。锁屏、未连接或 UAC 安全桌面期间不启动排队任务；运行中的远程任务发现桌面不可交互时请求停止，保留明确原因，不自动越过安全确认。RDP 断开后的截图及操作能力尚需实机复测。不要在自动化执行期间同时操作鼠标或切换安装窗口。

关闭工具后不再接受新请求，已排队任务保留；重新打开并开启接收后继续调度。异常退出时，运行中任务标记中断，不自动重新安装。需要修改数据目录时，先关闭接收并保存设置，再使用原迁移功能；复制迁移同步远程任务路径与配置快照校验，保留原数据。

任务元数据 `task.json`（version=1）、配置快照和安装包快照位于 `config/remote/jobs/<ID>`，独立日志位于 `logs/remote-<ID>.json.log`，结果位于 `results/remote-<ID>.json`。快照会占用额外磁盘空间；本轮没有自动清理或登录自启动功能。记录达到 10000 项时拒绝新增，请停用接收、备份后由管理员归档已结束任务，不能删除排队或运行任务。

## 6. 本轮验证

已进行本机命名管道通信、单执行端限制、队列落盘、幂等提交、安装包与配置 SHA-256 校验、执行期间禁止修改快照、取消、异常恢复和 GUI 历史同步测试。受控批处理脚本通过真实桌面调度与现有 CLI 执行，生成日志和结果，未安装第三方软件。

尚未完成不同主机间实际 SSH/SFTP、跨账户拒绝、管理员与非管理员同账户连接、锁屏/UAC/RDP 断开场景、Win7/Win10 多版本及实际安装器的完整验收。上述限制不因本机测试通过而消除。
