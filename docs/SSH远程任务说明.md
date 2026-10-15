# SSH 远程任务说明

## 1. 工作方式

SSH 负责传输文件和提交请求；安装、截图、UIA、OCR 由主机上已登录桌面的 AutoTrigger 执行。不要在 SSH 会话中直接运行旧的 `--input` 命令来替代桌面任务提交，它不能保证访问交互桌面。

工具不安装 OpenSSH、不修改防火墙、不自动登录、不关闭 UAC，也不新增网络监听端口。桌面端与 SSH 命令通过本机命名管道通信。

## 2. 主机准备

### 2.1 无人值守 Windows 配置清单

适用于专门运行安装测试、可还原快照的 Windows 10/11 虚拟机。这里的“后台运行”是指不需要人员持续操作，**不是无桌面的 Windows 服务**。完整链路为：虚拟机启动 → 指定账户自动登录 → 管理员权限启动 AutoTrigger → SSH 提交任务 → 桌面执行 GUI/UIA/OCR。SSH 服务能连接，不代表桌面已经可用。

自动登录、降低 UAC 通知和取消自动锁定会降低主机安全性。仅在隔离测试网络、可信客户端、专用账户及授权策略允许时采用；不建议用于日常办公电脑、存有敏感信息的主机或直接暴露公网的机器。先记录原设置、创建虚拟机快照，结束测试后恢复。不要关闭 Defender、添加安全软件排除或绕过组织策略来换取无人值守。

| 配置项 | 专用虚拟机目标值 | 必要性与说明 |
|---|---|---|
| 自动登录 | 使用微软 Sysinternals Autologon 配置指定账户 | 无人值守重启后恢复桌面时需要；人工登录的场景可不配置。账户须与 SSH 和 AutoTrigger 执行账户一致。 |
| AutoTrigger 管理员自启 | 勾选“登录 Windows 后以管理员权限自动启动”并保存 | 当前账户登录后启动，不是登录前启动的系统服务；设置时须以管理员身份运行。固定 EXE 路径，移动前先关闭旧位置自启。 |
| SSH 接收 | 勾选“允许通过 SSH 提交任务”并保存 | 与工具自启是两个独立开关；仅允许可信客户端使用同一 Windows 账户提交。 |
| OpenSSH Server | 已安装，sshd 正在运行且启动类型为“自动” | 设置入站访问范围和密钥认证；无需为了远程任务关闭防火墙。 |
| OCR 与解压依赖 | 检查环境/检查前置通过，保存可用 OCR 设置 | 自检通过才代表相应引擎可用；Umi-OCR 的 HTTP 服务需用户另行启动，不因工具自启而自动启动。 |
| UAC 通知 | 默认保留；确有确认阻塞且获授权时，可选“从不通知” | 仅为隔离虚拟机的可选降级方案，不保证所有安装器均无需人工确认，也不等同于完整关闭 UAC。 |
| 电源、睡眠与休眠 | 测试电源计划下关闭自动睡眠和休眠；建议屏幕关闭时间为“从不” | 关闭显示器不必然锁屏，但截图能力依赖虚拟显示环境；固定分辨率与缩放并实测。不要直接套用到日常笔记本。 |
| 自动锁定 | 测试期间避免空闲锁定、屏保锁定及动态锁 | “不显示锁屏图片”不等于桌面不会锁定；域策略、MDM、安全软件的强制锁定不得擅自绕过。 |
| 虚拟化与会话 | 虚拟机不被宿主机挂起，保留可用虚拟显示器与交互桌面 | 客体系统不睡眠仍可能被宿主机暂停。RDP 断开后的截图、分辨率和桌面状态必须单独验证。 |

### 2.2 工具与 SSH 接收端

1. 建议首期使用 Windows 10/11，安装并启动 OpenSSH Server，按管理要求配置防火墙、SSH 密钥和允许登录的账户。参考[微软 OpenSSH 部署说明](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh_install_firstuse)。Win7 的 SSH 部署与实机兼容未验证。
2. 使用指定 Windows 账户登录桌面，打开 AutoTrigger；系统级安装按需以管理员身份启动工具。不建议将 SSH 管理入口直接暴露在公网。
3. 在设置中完成数据目录、超时、OCR 与依赖配置，检查环境。桌面端必须保持运行。
4. 勾选基础设置中的“允许通过 SSH 提交任务”，核对权限提示并保存。默认关闭，不允许其他 Windows 账户提交。
5. 在该账户的终端执行以下命令，确认接收端及上传目录：

```powershell
.\AutoTrigger.exe task health
```

SSH 登录账户必须与运行桌面执行端的账户相同。该能力允许提交的代码以桌面端权限运行，启用后只应向可信客户端提供该账户的访问权限；管理员模式下风险尤其高。命名管道仅允许同账户访问并拒绝远程管道连接，不提供跨账户授权或任意命令执行接口。

### 2.3 Windows 配置步骤

以下命令在虚拟机内的**管理员 PowerShell** 执行；不是直接粘贴到 CMD，也不要在 SSH 会话中把启动 GUI 当作自动登录的替代方案。文档只提供配置方法，AutoTrigger 不会自动执行这些系统修改。

**自动登录**

从[微软 Autologon 页面](https://learn.microsoft.com/en-us/sysinternals/downloads/autologon)获取工具，以管理员身份运行，在图形界面填写专用 Windows 账户、计算机名/域及登录密码，点击 Enable。使用真实登录密码，不是 Windows Hello PIN；不要把密码放进脚本、命令行、截图或仓库。Autologon 将凭据保存在 LSA secret 中，但本机管理员仍可提取解密，不能当作密码保险箱。修改账户密码后需更新配置；域策略、登录提示或其他身份要求可能使自动登录失效。关闭时使用 Autologon 的 Disable，并重启确认。

**OpenSSH Server**

按[微软 OpenSSH 部署说明](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh_install_firstuse)安装 Server 组件。安装可选功能可能需要 Windows Update 或匹配系统版本的离线组件源，AutoTrigger 不代为下载或安装。安装后设置服务：

```powershell
Set-Service -Name sshd -StartupType Automatic
Start-Service -Name sshd
Get-Service -Name sshd | Select-Object Name, Status, StartType
```

预期为 Running / Automatic。防火墙仅允许管理网段或指定客户端访问 SSH 端口，优先使用密钥认证。不要删除全部防火墙规则或对公网开放管理员口令登录。

**电源、睡眠与休眠**

先在“设置 → 系统 → 电源和睡眠/屏幕和睡眠”中调整测试用电源计划。固定供电的测试虚拟机可参考：

```powershell
powercfg /change monitor-timeout-ac 0
powercfg /change standby-timeout-ac 0
powercfg /change hibernate-timeout-ac 0
```

这里的 0 表示从不，仅修改当前电源计划的接通电源项；存在电池供电时，还需评估对应 DC 项。若决定完全禁用休眠，可执行 `powercfg /hibernate off`；它也会影响依赖休眠文件的功能，例如快速启动。恢复时执行 `powercfg /hibernate on`，并按记录恢复原电源计划数值，不要把“从不”作为所有主机的通用默认值。

**避免自动锁定**

检查“设置 → 账户 → 登录选项”中离开后重新登录要求、动态锁，以及屏幕保护程序的“在恢复时显示登录屏幕”。仅在测试授权范围内取消会锁定桌面的选项；不同 Windows 版本的入口和可选项可能不同。如果组织设置了“交互式登录：计算机不活动限制”、屏保超时或 MDM 锁定策略，应申请专用测试策略，而不是强制覆盖。仅禁用锁屏背景页不能解决会话锁定。

**UAC 可选调整**

先以管理员权限运行 AutoTrigger 并试装，只有仍被权限确认阻塞、且已批准降低测试机防护时，再打开“控制面板 → 用户账户 → 更改用户账户控制设置”，将滑块设为“从不通知”，确认系统提示，并按系统要求重启。该选项对管理员提升通知的影响见[微软 UAC 说明](https://learn.microsoft.com/en-us/windows-server/security/user-account-control/how-user-account-control-works)；它不赋予标准用户管理员权限，不保证消除组织强制策略、安全软件拦截或安装器自带的确认。不要为了此流程额外修改 EnableLUA、绕过安全桌面或关闭防病毒。测试完成后恢复原通知级别。

**AutoTrigger 自启与 OCR**

以管理员身份打开固定路径的 `AutoTrigger.exe`，先检查环境，确认 OCR 的实际引擎、路径和自检结果。在基础设置中分别勾选 SSH 接收和管理员登录自启，点击保存；有未保存 OCR 配置时一并核对。自启使用当前账户的交互式计划任务，不保存登录密码，也不替用户配置 Autologon。Umi-OCR 等外部服务如果另有启动要求，须单独安排并验证。自启读取异常时查看数据目录 `config/autostart-state.json`；它含本机路径和账户信息，提交排查前可脱敏。

### 2.4 重启与无人值守验收

1. 重启虚拟机，不手动登录或先用 RDP“唤醒”桌面，确认 Autologon 能进入指定账户。
2. 从虚拟机控制台确认 AutoTrigger 自动启动、管理员自启状态正确、SSH 接收已开启且设置已保存。
3. 在远程客户端通过 SSH 调用主机的 `AutoTrigger.exe task health`，确认 ok=true；有 waiting_reason 时先解决会话或桌面问题，不把服务可达当成可执行。
4. 上传一个已知安全的测试样本，提交任务并连续查询 status、logs、result，核对 OCR/GUI 动作、文件变化和最终证据。task.completed 或命令退出码 0 本身不等于安装成功。
5. 在预期的闲置时间后再次提交，验证没有睡眠或自动锁定；若实际通过 RDP 运维，还需在断开后重新验证截图、鼠标坐标及 OCR。不要假设所有虚拟机和显示驱动表现一致。
6. 遇到 UAC 安全桌面、Defender 告警、会话不可交互或自启失败时保留日志并停止该轮验收，不自动放行或反复重放安装。恢复快照前先备份结果和诊断材料。

本清单是部署建议，不代表所有 Windows 版本、虚拟化平台或真实安装器已通过无人值守验收。自动登录及低防护配置应限定使用期限，停用时同时恢复 UAC、锁定与电源策略；不再使用远程入口时关闭工具接收和不需要的 SSH 服务。

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

### 4.1 响应字段

以下是 `AutoTrigger.exe task ...` 返回的 JSON 外层结构。除 `ok` 外，字段按命令及实际内容出现；空值通常省略，而不是固定输出 null。

| 字段 | 类型 | 含义 |
|---|---|---|
| `ok` | boolean | 本次请求是否成功。不是安装成功标志。false 时先处理 error。 |
| `error` | string，可省略 | 请求或业务失败原因。参数错误、通信失败可能仅写标准错误，不返回此 JSON。 |
| `task` | object，可省略 | 单个远程任务记录，字段见下节。失败响应中即使出现此对象，也不能据此认为已接收任务。 |
| `tasks` | array，可省略 | list 返回的任务记录数组；没有任务时省略，响应可能只有 `{"ok":true}`。 |
| `inbox` | string，可省略 | health 返回的可选上传目录，不限制安装包必须位于此目录。 |
| `waiting_reason` | string，可省略 | 执行端当前等待原因。health 可返回；status/logs/result 仅在查询任务排队时返回。它不是安装失败原因。 |
| `logs` | string，可省略 | logs 返回的本次 UTF-8 日志片段。没有新内容时可能省略，不是完整日志文件。 |
| `next_offset` | integer，可省略 | 下次 logs 查询使用的字节偏移，不是字符数量；值为 0 时省略。 |
| `result` | object，可省略 | result 返回的安装/触发结果 JSON 对象，不是路径或 JSON 字符串，字段见[结果 JSON 字段说明](结果JSON字段说明.md)。 |

| 命令 | 成功时主要字段 |
|---|---|
| `health` | ok、inbox、可选 waiting_reason |
| `submit` / `cancel` | ok、task |
| `list` | ok、可选 tasks |
| `status` | ok、task、可选 waiting_reason |
| `logs` | ok、task、可选 logs / next_offset / waiting_reason |
| `result` | ok、task、result、可选 waiting_reason |

`task result` 的 ok=true 只表示成功读取有效 JSON，不表示里面所有样本安装成功，也不保证任务已经完成。请先核对 task.state，再处理 result.error、result.truncated 和 result.results[].final_status。

### 4.2 任务记录 task / tasks[]

任务元数据结构版本当前为 `version=1`，与安装结果的 `schema_version=2.8` 不同。所有路径均属于执行主机。时间采用 RFC3339 格式，包含时区，可带小数秒。

| 字段 | 类型 | 含义 |
|---|---|---|
| `version` | integer | 远程任务记录格式版本，当前为 1。 |
| `id` | string | 32 位小写十六进制远程任务 ID，用于 status/logs/result/cancel 和幂等重试，不等于结果中的 CLI task_id。 |
| `name` | string | 任务名称；未指定时使用源文件名。 |
| `source` | string | 原始提交文件的解析后路径。 |
| `input` | string | 冻结的安装包快照路径，实际执行使用此文件，不直接使用 source。 |
| `sha256` | string | 安装包快照 SHA-256。 |
| `config_sha256` | string | 配置快照 SHA-256。 |
| `config` | string | 此任务使用的配置快照文件路径。 |
| `result_path` | string | 预定结果文件路径；出现路径不表示文件已经生成。 |
| `log_path` | string | 预定过程日志路径；排队时文件可能尚不存在。 |
| `created` | string | 创建时间。 |
| `started` | string，可省略 | 实际开始时间；未开始时省略。 |
| `finished` | string，可省略 | 结束时间；尚未结束时省略。 |
| `state` | string | 调度生命周期状态，见第 5 节，不是逐样本安装状态。 |
| `reason` | string，可省略 | 取消、中断或执行异常等补充原因。保留原始诊断信息，可能包含英文。 |
| `cancel_requested` | boolean，可省略 | 是否请求取消；false 时省略。true 不表示进程已经停止，应继续查询 state。 |
| `exit_code` | integer，可省略 | 执行工具返回的真实退出码；尚未取得时省略，不可默认为 0。即使为 0 也需检查安装结果。 |
| `recursive` | boolean | 此任务继承的递归处理设置。 |
| `no_archive` | boolean | 此任务是否跳过解压处理。 |

### 4.3 日志与结果读取约定

首次读取日志使用 `--offset 0`；后续成功响应有 next_offset 时，按该值继续。没有 next_offset 时保留原偏移。没有 logs 不代表执行结束，需结合 task.state 判断；运行期间可以定期查询。不要用日志字符串长度推算偏移，中文 UTF-8 字符占多个字节。

常见响应示例（仅展示关键字段，非完整任务对象）：

```json
{
  "ok": true,
  "task": {
    "id": "7745273878c29e8a829b78c13138ae73",
    "state": "running"
  },
  "logs": "正在执行安装任务\r\n",
  "next_offset": 1234
}
```

对接时先检查命令退出码和 ok，再读取字段；允许可选字段缺省和未来新增字段。completed、cancelled、interrupted 都应结合结果与日志处理，不能直接当成安装成功或系统变更已回滚。

## 5. 状态与运行限制

| 状态 | 含义 |
|---|---|
| `staging` | 正在复制并冻结文件；同 ID 正在接收时不能另建重复任务 |
| `queued` | 已接收，等待桌面空闲和可交互 |
| `running` | 已开始处理；不代表安装成功 |
| `completed` | 执行流程结束，必须查看退出码和结果 JSON |
| `cancelled` | 已取消；已运行的安装器可能已经产生部分变更 |
| `interrupted` | 执行端异常退出、桌面失去交互能力或快照无效；不自动重放 |

本地和远程安装任务不会在同一桌面端同时执行；远程任务按队列顺序调度，模板捕获期间等待。远程任务强制使用既有机器锁。

执行前检查实际桌面会话及输入桌面。锁屏、未连接或 UAC 安全桌面期间不启动排队任务；运行中的远程任务发现桌面不可交互时请求停止，保留明确原因，不自动越过安全确认。RDP 断开后的截图及操作能力尚需实机复测。不要在自动化执行期间同时操作鼠标或切换安装窗口。

关闭工具后不再接受新请求，已排队任务保留；重新打开并开启接收后继续调度。异常退出时，运行中任务标记中断，不自动重新安装。需要修改数据目录时，先关闭接收并保存设置，再使用原迁移功能；复制迁移同步远程任务路径与配置快照校验，保留原数据。

任务元数据 `task.json`（version=1）、配置快照和安装包快照位于 `config/remote/jobs/<ID>`，独立日志位于 `logs/remote-<ID>.json.log`，结果位于 `results/remote-<ID>.json`。快照会占用额外磁盘空间，目前没有自动清理功能。工具已提供管理员登录自启，但 Windows 自动登录和外部服务自启仍由部署人员配置，见第 2 节。记录达到 10000 项时拒绝新增，请停用接收、备份后由管理员归档已结束任务，不能删除排队或运行任务。

## 6. 本轮验证

已进行本机命名管道通信、单执行端限制、队列落盘、幂等提交、安装包与配置 SHA-256 校验、执行期间禁止修改快照、取消、异常恢复和 GUI 历史同步测试。受控批处理脚本通过真实桌面调度与现有 CLI 执行，生成日志和结果，未安装第三方软件。

尚未完成不同主机间实际 SSH/SFTP、跨账户拒绝、管理员与非管理员同账户连接、锁屏/UAC/RDP 断开场景、Win7/Win10 多版本及实际安装器的完整验收。上述限制不因本机测试通过而消除。
