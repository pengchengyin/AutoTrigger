# Windows 任意文件自动安装/触发工具 PRD

## 1. 文档信息

| 项目 | 内容 |
|------|------|
| 文档名称 | Windows 任意文件自动安装/触发工具 PRD |
| 版本 | v1.0 |
| 状态 | 待开发 |
| 目标平台 | Windows 10 / 11 / Server 2016+（架构预留跨平台扩展） |
| 运行环境 | 已有沙箱环境（工具不负责隔离，不负责检测） |
| 下游依赖 | EDR（负责行为采集与检测） |

---

## 2. 背景与目标

### 2.1 背景

软件供应链安全检测部门需要对**入网的任意文件**进行动态行为检测。当前流程中，样本进入沙箱后，缺少一个能够**自动识别文件类型并尝试各种安装/执行方式**的触发工具。检测环节由沙箱内已有的 EDR 负责，本工具只负责“让文件跑起来”。

### 2.2 目标

开发一个运行在 Windows 沙箱内的自动化触发工具，接收**指定的任意文件或文件夹**，自动识别文件类型，尝试多种安装/执行方式，尽可能触发样本的真实行为，供 EDR 采集。

工具需在架构层面**预留跨平台扩展能力**，便于后期支持 Linux / macOS。

### 2.3 非目标（明确不做）

- 不负责沙箱隔离与逃逸防护（运行环境已是沙箱）
- 不负责行为监控、日志采集、威胁判定（由 EDR 负责）
- 不负责样本静态分析、反混淆、脱壳
- 不负责网络隔离策略配置
- 本期不实现 Linux / macOS，仅预留架构扩展点

---

## 3. 用户与使用场景

### 3.1 目标用户

- 供应链安全检测工程师
- 自动化检测流水线运维人员

### 3.2 典型场景

| 场景 | 描述 |
|------|------|
| 单文件触发 | 传入一个 `.exe`，工具尝试静默安装、无参数运行等多种方式 |
| 文件夹批量触发 | 传入一个目录，工具遍历所有文件，逐个识别并触发 |
| 压缩包递归触发 | 传入 `.zip`，工具解压后递归触发内部所有可执行文件 |
| 文档类触发 | 传入 `.docx` / `.pdf`，工具调用关联程序打开，触发宏/漏洞利用链 |
| 未知格式兜底 | 传入扩展名未知或文件头与扩展名不符的文件，工具仍尝试执行 |

### 3.3 使用方式

命令行调用为主，便于集成到自动化流水线：
```
AutoTrigger --input "C:\samples\test.exe" --timeout 120 --output "C:\logs\result.json"
AutoTrigger --input "C:\samples\folder" --recursive --timeout 120
```

---

## 4. 跨平台架构设计要求

### 4.1 设计原则

工具从第一天起就按**平台无关核心 + 平台适配层**的方式组织，避免平台相关逻辑散落在业务代码中。

### 4.2 分层结构

| 层级 | 职责 | 平台相关性 |
|------|------|-----------|
| Core（核心层） | 输入解析、任务调度、类型识别、参数探测、超时控制、结果输出 | 平台无关 |
| Platform Abstraction（平台抽象层） | 定义统一接口：进程执行、进程树终止、文件关联打开、解压、文件头识别 | 平台无关接口 |
| Platform Adapter（平台适配层） | Windows 实现（本期）、Linux 实现（后期）、macOS 实现（后期） | 平台相关 |
| CLI（入口层） | 命令行参数解析、调用 Core | 平台无关 |

### 4.3 平台抽象层接口定义（示意）
```
interface IProcessRunner:
start(command, args, timeout) -> ProcessHandle
killTree(ProcessHandle) -> void
waitForExit(ProcessHandle, timeout) -> ExitResult

interface IFileOpener:
openWithDefaultApp(filePath) -> ProcessHandle

interface IArchiveExtractor:
extract(archivePath, destDir) -> ExtractResult
supportedExtensions() -> list

interface IFileTypeDetector:
detect(filePath) -> FileType
magicNumberMap() -> map
```

### 4.4 平台适配矩阵

| 能力 | Windows（本期） | Linux（后期） | macOS（后期） |
|------|----------------|--------------|--------------|
| 进程执行 | CreateProcess / Start-Process | fork+exec | posix_spawn |
| 进程树终止 | Job Object / taskkill /T | kill process group | kill process group |
| 默认程序打开 | ShellExecute | xdg-open | open |
| 解压 | 7z.exe | 7z / unzip / tar | 7z / unzip / tar |
| 文件头识别 | 魔数表（平台无关） | 魔数表 | 魔数表 |
| 安装包触发 | msiexec / 静默参数 | dpkg / rpm / sh | pkg / dmg |
| 脚本触发 | cmd / powershell / wscript | bash / sh / python | bash / zsh / osascript |

### 4.5 跨平台扩展约束

- 核心层不得直接调用任何 Windows API
- 所有平台相关行为必须通过 Platform Abstraction 接口调用
- 参数探测表、文件类型映射表按平台分文件存放，核心层按当前平台加载
- 结果 JSON 结构保持平台无关，平台特有字段放在 `platform_meta` 中

---

## 5. 功能需求

### 5.1 功能总览

| 编号 | 功能模块 | 优先级 | 平台相关性 |
|------|----------|--------|-----------|
| F1 | 输入接收与校验 | P0 | 无关 |
| F2 | 文件类型识别 | P0 | 无关 |
| F3 | 可执行文件触发 | P0 | 适配层 |
| F4 | 安装包触发 | P0 | 适配层 |
| F5 | 脚本类触发 | P0 | 适配层 |
| F6 | 压缩包递归处理 | P0 | 适配层 |
| F7 | 文档类触发 | P1 | 适配层 |
| F8 | 未知格式兜底触发 | P1 | 适配层 |
| F9 | 超时与进程清理 | P0 | 适配层 |
| F10 | 执行结果记录与输出 | P0 | 无关 |
| F11 | 文件夹批量遍历 | P0 | 无关 |
| F12 | 嵌套/链式触发 | P1 | 无关 |
| F13 | 参数探测与回退机制 | P0 | 无关 |

---

### 5.2 功能详细说明

#### F1 输入接收与校验

- 支持 `--input` 指定单个文件或文件夹路径
- 支持 `--recursive` 递归遍历文件夹
- 支持 `--timeout` 指定单个文件的最大触发时长（秒），默认 120
- 支持 `--output` 指定结果输出路径（JSON）
- 输入路径不存在或无权限时，记录错误并退出，不阻塞流水线

#### F2 文件类型识别

识别优先级：**扩展名 → 文件头（Magic Number）→ 综合判断**

| 类别 | 扩展名 | 文件头特征 |
|------|--------|-----------|
| PE 可执行 | .exe .dll .scr .com | `MZ` |
| MSI 安装包 | .msi .msp | OLE Compound File |
| 脚本 | .bat .cmd .ps1 .vbs .js .wsf | 文本特征 |
| 压缩包 | .zip .rar .7z .tar .gz .iso | 各格式魔数 |
| 文档 | .doc .docx .xls .xlsx .pdf .rtf .ppt .pptx | OLE / ZIP / `%PDF` |
| 快捷方式 | .lnk | `4C 00 00 00` |
| 未知 | 其他 | 兜底 |

**关键要求**：文件头与扩展名不一致时，**以文件头为准**，并记录“扩展名伪装”标记。

**跨平台要求**：类型识别逻辑放在 Core 层，魔数表平台无关，但各平台可扩展自己的类型映射（如 Linux 的 .deb / .rpm）。

#### F3 可执行文件触发

对 `.exe` / `.scr` / `.com` 等 PE 文件，按以下顺序尝试：

1. 直接运行（无参数）—— 最基础的引爆方式
2. 常见静默参数轮询（见 F13）
3. 若全部失败或超时，强制结束进程，记录“未能完整触发”

> 注意：即使静默安装失败，**无参数直接运行也必须执行一次**，因为目的是触发行为而非装成功。

**跨平台要求**：通过 `IProcessRunner` 接口执行，Windows 适配层实现 PE 相关逻辑，Linux 适配层后期实现 ELF 触发。

#### F4 安装包触发

对 `.msi` 文件：

1. `msiexec /i "文件" /quiet /norestart`
2. `msiexec /i "文件" /passive /norestart`
3. `msiexec /i "文件"`（无静默参数，观察 GUI 行为）

对 `.msp` 补丁包：`msiexec /p "文件" /quiet`

**跨平台要求**：安装包触发逻辑通过 `IInstallerTrigger` 接口抽象，Windows 实现 MSI，Linux 后期实现 deb/rpm，macOS 后期实现 pkg/dmg。

#### F5 脚本类触发

| 类型 | 执行方式 |
|------|----------|
| .bat / .cmd | `cmd.exe /c "文件"` |
| .ps1 | `powershell.exe -ExecutionPolicy Bypass -NoProfile -File "文件"` |
| .vbs / .js | `wscript.exe "文件"` 或 `cscript.exe "文件"` |
| .wsf | `wscript.exe "文件"` |

要求：脚本执行同样受超时控制。

**跨平台要求**：脚本类型与解释器映射按平台分文件配置，Core 层按平台加载。

#### F6 压缩包递归处理

1. 解压到临时目录（优先使用 7-Zip 命令行，支持 zip/rar/7z/iso 等）
2. 遍历解压后所有文件
3. 对每个文件重新走 F2 类型识别 → 对应触发逻辑
4. 递归深度限制：默认 3 层，防止 zip 炸弹或无限嵌套
5. 解压失败时记录错误，不阻塞后续文件

**跨平台要求**：通过 `IArchiveExtractor` 接口抽象，各平台可实现不同解压后端。

#### F7 文档类触发

- 使用系统默认关联程序打开文件：`Start-Process "文件"`
- 若沙箱内预装 Office / PDF 阅读器，则由其触发宏或漏洞链
- 超时后强制结束关联进程树
- 若系统无关联程序，记录“无可用宿主程序”并跳过

**跨平台要求**：通过 `IFileOpener` 接口抽象，Windows 用 ShellExecute，Linux 用 xdg-open，macOS 用 open。

#### F8 未知格式兜底触发

当扩展名未知或无法归类时，按以下顺序尝试：

1. 若文件头为 `MZ`，按 PE 执行（F3）
2. 若文件头为 ZIP/OLE，按压缩包或文档处理
3. 否则尝试 `Start-Process "文件"`（依赖系统关联）
4. 全部失败则记录“无法触发”，不阻塞

**设计原则：宁可误触发，不可漏触发。**

#### F9 超时与进程清理

- 每个文件的触发流程有独立超时（默认 120 秒，可配置）
- 超时后：
  1. 记录当前执行阶段与已尝试的参数
  2. 结束该文件及其派生的整个进程树（使用 Job Object 或 `taskkill /T /F`）
  3. 继续处理下一个文件
- 工具退出前，清理所有临时解压目录

**跨平台要求**：通过 `IProcessRunner.killTree` 接口抽象，Windows 用 Job Object，Linux/macOS 用进程组。

#### F10 执行结果记录与输出

输出 JSON 格式，每个文件一条记录，字段包括：

```json
{
  "file_path": "C:\\samples\\test.exe",
  "file_hash_sha256": "...",
  "detected_type": "pe_executable",
  "extension_disguised": false,
  "platform": "windows",
  "platform_meta": {},
  "attempts": [
    {
      "method": "direct_run",
      "command": "C:\\samples\\test.exe",
      "result": "timeout",
      "duration_sec": 120
    },
    {
      "method": "silent_install",
      "command": "C:\\samples\\test.exe /S",
      "result": "exited",
      "exit_code": 0,
      "duration_sec": 8
    }
  ],
  "final_status": "triggered",
  "notes": ""
}
```
final_status 枚举：

triggered：至少一种方式成功启动并运行到结束

timeout：所有尝试均超时

failed：所有方式均无法启动

skipped：无可用触发方式（如文档无宿主程序）

F11 文件夹批量遍历
递归遍历输入目录下所有文件

按文件类型排序处理（PE/安装包优先，文档次之，未知最后）

单文件失败不影响后续文件

汇总输出总览：总数、成功数、超时数、失败数

F12 嵌套/链式触发
压缩包解压后的文件递归处理（F6）

安装过程中若释放新文件到临时目录，不主动追踪（由 EDR 捕获）

脚本中若调用其他程序，不主动拦截（由 EDR 捕获）

本工具只负责“第一跳”，后续链式行为交给 EDR。

F13 参数探测与回退机制
维护一个静默参数映射表，按顺序轮询：

安装器类型	参数
Inno Setup	/VERYSILENT /NORESTART /SUPPRESSMSGBOXES
NSIS	/S
InstallShield	/s /v"/qn"
WiX / MSI	/quiet /norestart
通用	/silent /quiet --silent --quiet -s
回退规则：

按顺序尝试，任一方式进程成功启动并退出（无论退出码），记录并进入下一文件

若全部静默参数均失败，必须回退到无参数直接运行

每次尝试之间有短暂间隔（如 2 秒），避免资源竞争

跨平台要求：参数表按平台分文件存放，Windows 使用上述表，Linux 后期维护 deb/rpm/sh 参数表。

6. 非功能需求
类别	要求
性能	单文件触发不超过配置超时；批量处理并发度可配置（默认串行，避免干扰 EDR 采集）
稳定性	单文件失败不影响整体流程；工具自身崩溃后可通过退出码识别
兼容性	Windows 10 1809+ / Windows 11 / Server 2016+；架构预留 Linux / macOS
权限	需以管理员权限运行（安装包静默安装通常需要）
日志	除 JSON 结果外，输出可读日志到 stdout 和文件，便于排查
可配置	超时、并发度、递归深度、参数表均可通过配置文件或命令行覆盖
无残留	退出前清理临时目录；不修改系统默认关联
可移植	核心层不依赖 Windows API；平台相关逻辑全部走适配层
7. 技术选型建议
组件	建议
主语言	C# / .NET（推荐，跨平台支持好，进程控制强）或 Go（跨平台编译方便）
跨平台策略	Core 层用平台无关语言编写；Platform Adapter 按平台分别实现
进程树管理	Windows：Job Object；Linux/macOS：进程组
解压	优先 7-Zip 命令行（三平台均有），可扩展其他后端
文件头识别	自行实现魔数表，或使用 TrID / libmagic 绑定
输出格式	JSON（主结果）+ 文本日志（辅助）
配置格式	JSON 或 YAML，按平台分文件存放参数表与类型映射
若追求更好的进程控制与跨平台能力，推荐 C# / .NET；若追求单一二进制分发与跨平台编译，推荐 Go。

8. 交付物
可执行工具（Windows 版；Linux / macOS 预留接口与空实现）

配置文件模板（config.json）

静默参数映射表（按平台分文件，可外部编辑）

平台抽象层接口定义文档

使用说明文档（README）

测试样本集与预期结果对照表

9. 验收标准
编号	验收项	通过标准
A1	单 PE 文件触发	能识别并按顺序尝试至少 3 种方式
A2	MSI 静默安装	能正确调用 msiexec 并记录退出码
A3	压缩包递归	能解压并触发内部可执行文件，递归深度受控
A4	超时控制	超时后进程树被完整清理，不残留
A5	未知格式兜底	扩展名伪装文件能被正确识别并尝试触发
A6	批量处理	文件夹内 50 个文件全部处理完毕，单文件失败不影响整体
A7	结果输出	JSON 结构完整，包含每次尝试的 method/result/duration
A8	EDR 兼容	工具运行期间 EDR 能正常采集到样本行为，无冲突
A9	跨平台架构	Core 层无直接平台 API 调用；平台适配层接口清晰可扩展
10. 风险与对策
风险	对策
样本反沙箱检测，装死不触发	工具层面无法解决，需沙箱环境加固；工具确保触发动作本身完整
安装程序卡在 GUI 等待输入	超时机制强制结束，记录“未完整触发”
压缩包嵌套过深或 zip 炸弹	递归深度限制 + 解压大小限制
静默参数导致样本行为被抑制	必须保留“无参数直接运行”作为回退
工具自身被样本攻击	运行在沙箱内，工具崩溃不影响宿主机；建议工具与样本进程隔离
EDR 与工具进程冲突	工具自身进程可加入 EDR 白名单，避免误报
跨平台抽象过度设计	本期只实现 Windows 适配层，接口保持最小可用，后期按需扩展
11. 后续可扩展方向（非本期范围）
实现 Linux 适配层（deb / rpm / sh / ELF 触发）

实现 macOS 适配层（pkg / dmg / Mach-O 触发）

支持自定义触发脚本插件（用户可针对特定样本类型写触发逻辑）

支持从 URL 下载样本后触发

支持与 EDR API 联动，触发后自动拉取行为报告

支持触发方式优先级动态调整（基于历史成功率）

支持分布式批量触发（多沙箱并行）

文档结束