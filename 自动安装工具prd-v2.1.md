# Windows 任意文件自动安装/触发工具 PRD

## 1. 文档信息

| 项目 | 内容 |
|------|------|
| 文档名称 | Windows 任意文件自动安装/触发工具 PRD |
| 版本 | v2.1（安装成功优先 + GUI 自动化 + 离线单文件交付） |
| 状态 | 待开发 |
| 目标平台 | Windows 10 1809+ / 11 / Server 2016+（架构预留跨平台扩展） |
| 运行环境 | 离线沙箱环境（工具不负责隔离，不负责检测） |
| 交付形态 | 单一可执行文件（无安装、无外部运行时、无外部工具依赖） |
| 下游依赖 | EDR（负责行为采集与检测） |

### 1.1 变更说明（相对 v1.0）

| 编号 | 变更 | 原因 |
|------|------|------|
| C1 | 修复触发顺序矛盾，明确统一口径（v2.1 进一步调整为安装成功优先） | v1.0 中 F3 与 F13 顺序自相矛盾 |
| C2 | 重定义 final_status 枚举，`killed_on_timeout` 视为"已触发" | 超时说明进程已运行、行为已被 EDR 采集 |
| C3 | 补充 `.lnk` 与 `.dll` 触发逻辑 | v1.0 F2 识别了这两类但无处理流程 |
| C4 | 平台抽象层接口补齐 `IInstallerTrigger`、新增 `IConfigProvider` | v1.0 F4 引用了未定义的接口 |
| C5 | 新增退出码表（6.1）、config.json 结构（7.4）、解压大小限制 | 工程落地必需，v1.0 缺失 |
| C6 | 结果 JSON 增加 `schema_version`、`task_id`、时间戳、尝试输出摘要 | 便于流水线版本兼容与问题排查 |
| C7 | 修复 v1.0 第 5.2 节后半部分至第 11 节的格式损坏 | 原文档表格丢失，影响评审 |
| C8 | 新增开发里程碑（第 10 节）与待确认问题清单（第 12 节） | 便于排期与需求澄清 |
| C9 | 目标调整：**以"安装成功"为首要目标**，静默参数优先、无参数运行 + GUI 自动化兜底，尽量多试 | 需求：尽可能装成功，多尝试 |
| C10 | 新增 F14：GUI 弹窗识别与键鼠模拟（UIA 优先 + 图像识别兜底） | 安装向导弹窗需自动完成点击/输入 |
| C11 | 新增离线约束：工具与触发流程全程不联网，安装器联网等待按超时记录 `requires_network` | 部署环境为离线沙箱 |
| C12 | final_status 新增 `install_completed`，含退出码/完成页/落盘/注册表四类证据 | "装成了"需要可验证判定 |
| C13 | 新增单一二进制分发约束：无运行时与外部工具依赖，解压改内嵌库，配置与模板内嵌 | 便于分发到各沙箱节点 |

---

## 2. 背景与目标

### 2.1 背景

软件供应链安全检测部门需要对**入网的任意文件**进行动态行为检测。样本进入沙箱后，缺少一个能够**自动识别文件类型并尝试各种安装/执行方式**的触发工具。检测环节由沙箱内已有的 EDR 负责，本工具只负责"让文件跑起来"。

### 2.2 目标

开发一个运行在 Windows 沙箱内的自动化触发工具，接收**指定的任意文件或文件夹**，自动识别文件类型，**以尽可能完成安装为首要目标**：

1. 优先轮询多种静默安装参数，尽量无人值守装成；
2. 若出现安装向导弹窗，自动识别安装相关按钮/输入项，模拟键鼠完成"同意 → 下一步 → 安装 → 完成"操作链；
3. 尽量多尝试几种方式（静默参数 ↔ 无参数运行）直到取得**安装成功证据**；
4. 安装与触发全程保证样本行为暴露，供 EDR 采集。

### 2.3 交付与部署约束

| 约束 | 要求 |
|------|------|
| 离线 | 工具自身与触发流程**全程不访问网络**：不下载组件、不更新规则、不上报结果 |
| 单文件 | 以**单一可执行文件**交付，目标机解压即用；无外部运行时（不需 .NET Runtime / VC++ Redist）、无外部工具（不需 7z.exe）、无安装步骤 |
| 分发 | 支持通过共享目录 / 移动介质拷贝到各沙箱节点 |
| 架构 | 预留 Linux / macOS 扩展能力 |

### 2.4 非目标（明确不做）

- 不负责沙箱隔离与逃逸防护（运行环境已是沙箱）
- 不负责行为监控、日志采集、威胁判定（由 EDR 负责）
- 不负责样本静态分析、反混淆、脱壳
- 不负责网络隔离策略配置；安装器需联网下载组件时不做网络补偿，按超时处理并标记 `requires_network`
- 不追踪样本运行后释放的后续文件与链式行为（只负责"第一跳"，其余由 EDR 捕获）
- 不保证所有样本都能装成功（反沙箱、需联网激活、依赖缺失等由环境侧解决）
- 本期不实现 Linux / macOS，仅预留架构扩展点

---

## 3. 用户与使用场景

### 3.1 目标用户

- 供应链安全检测工程师
- 自动化检测流水线运维人员

### 3.2 典型场景

| 场景 | 描述 |
|------|------|
| 静默安装成功 | 传入 `.exe` 安装器，工具轮询静默参数完成无人值守安装，产出 `install_completed` |
| 向导式安装 | 安装器只支持 GUI，工具识别弹窗按钮/协议勾选框并模拟点击，走完全流程 |
| 单文件触发 | 传入一个 `.exe`，多种方式依次尝试并记录每次结果 |
| 文件夹批量触发 | 传入一个目录，工具遍历所有文件，逐个识别并触发 |
| 压缩包递归触发 | 传入 `.zip`，工具解压后递归触发内部所有可执行文件 |
| 文档类触发 | 传入 `.docx` / `.pdf`，工具调用关联程序打开，触发宏/漏洞利用链 |
| 未知格式兜底 | 传入扩展名未知或文件头与扩展名不符的文件，工具仍尝试执行 |

### 3.3 使用方式

命令行调用为主，便于集成到自动化流水线：

```
AutoTrigger.exe --input "C:\samples\test.exe" --timeout 120 --output "C:\logs\result.json"
AutoTrigger.exe --input "C:\samples\folder" --recursive --timeout 120
AutoTrigger.exe --input "C:\samples" --config "C:\conf\config.json"
```

参数优先级：命令行参数 > 外部配置文件 > 内嵌默认值。

---

## 4. 跨平台架构设计要求

### 4.1 设计原则

工具从第一天起就按**平台无关核心 + 平台适配层**的方式组织，避免平台相关逻辑散落在业务代码中。

### 4.2 分层结构

| 层级 | 职责 | 平台相关性 |
|------|------|-----------|
| Core（核心层） | 输入解析、任务调度、类型识别、参数探测、超时控制、结果输出 | 平台无关 |
| Platform Abstraction（平台抽象层） | 定义统一接口（见 4.3） | 平台无关接口 |
| Platform Adapter（平台适配层） | Windows 实现（本期）、Linux 实现（后期）、macOS 实现（后期） | 平台相关 |
| CLI（入口层） | 命令行参数解析、调用 Core | 平台无关 |
| Embedded Resources（内嵌资源） | 魔数表、静默参数表、按钮关键词表、图像模板、默认配置 | 平台无关 |

### 4.3 平台抽象层接口定义

```
interface IProcessRunner:
    start(command, args, timeout) -> ProcessHandle
    killTree(ProcessHandle) -> void          // Windows: Job Object / taskkill /T /F
    waitForExit(ProcessHandle, timeout) -> ExitResult

interface IFileOpener:
    openWithDefaultApp(filePath) -> ProcessHandle   // Windows: ShellExecute
    hasAssociation(filePath) -> bool                // 无关联程序时返回 false，供 F7 跳过

interface IArchiveExtractor:
    extract(archivePath, destDir, limits) -> ExtractResult   // limits: 最大层数/最大解压体积
    supportedExtensions() -> list

interface IFileTypeDetector:
    detect(filePath) -> FileType
    magicNumberMap() -> map                  // 平台无关，支持按平台扩展

interface IInstallerTrigger:
    getTriggerCommands(filePath, fileType) -> list[TriggerCommand]
    // Windows: msiexec 系列；Linux(后期): dpkg/rpm；macOS(后期): pkg/dmg

interface IConfigProvider:
    loadConfig(path) -> Config
    // 内嵌默认值 + 外部同名文件覆盖；参数表与类型映射表按平台分文件

interface IGuiAutomator:
    watchWindows(pidTree, interval) -> list[Window]    // 轮询进程树的可见顶层窗口
    findControls(window) -> list[Control]              // UIA 控件树
    click(Control) -> void
    setText(Control, text) -> void
    matchByTemplate(window, templates) -> Control|null // 图像识别兜底（自绘窗口）

interface IInstallVerifier:
    isInstalled(context) -> InstallEvidence | null
    // 退出码 / 完成页 / 落盘证据 / 注册表卸载项（见 F14.4）
```

### 4.4 平台适配矩阵

| 能力 | Windows（本期） | Linux（后期） | macOS（后期） |
|------|----------------|--------------|--------------|
| 进程执行 | CreateProcess / Start-Process | fork+exec | posix_spawn |
| 进程树终止 | Job Object / taskkill /T | kill process group | kill process group |
| 默认程序打开 | ShellExecute | xdg-open | open |
| 解压 | 内嵌解压库（可选外部 7z 释放） | 内嵌库 / tar | 内嵌库 / tar |
| 文件头识别 | 魔数表（平台无关，内嵌） | 魔数表 | 魔数表 |
| 安装包触发 | msiexec / 静默参数 | dpkg / rpm / sh | pkg / dmg |
| 脚本触发 | cmd / powershell / wscript | bash / sh / python | bash / zsh / osascript |
| GUI 自动化 | UIA + SendInput + 截图模板匹配 | xdotool（后期） | AppleScript（后期） |
| 安装证据采集 | 文件系统 + 注册表 | 包管理器数据库 | pkgutil |

### 4.5 跨平台扩展约束

- 核心层不得直接调用任何 Windows API
- 所有平台相关行为必须通过 Platform Abstraction 接口调用
- 参数探测表、文件类型映射表、按钮关键词表、图像模板按平台组织，优先内嵌、支持外部覆盖
- 结果 JSON 结构保持平台无关，平台特有字段放在 `platform_meta` 中

---

## 5. 功能需求

### 5.1 功能总览

| 编号 | 功能模块 | 优先级 | 平台相关性 | 里程碑 |
|------|----------|--------|-----------|--------|
| F1 | 输入接收与校验 | P0 | 无关 | M1 |
| F2 | 文件类型识别 | P0 | 无关 | M1 |
| F3 | PE 可执行文件触发（exe/scr/com/dll/lnk） | P0 | 适配层 | M1 |
| F4 | 安装包触发 | P0 | 适配层 | M1 |
| F5 | 脚本类触发 | P0 | 适配层 | M1 |
| F6 | 压缩包递归处理 | P0 | 适配层 | M2 |
| F7 | 文档类触发 | P1 | 适配层 | M2 |
| F8 | 未知格式兜底触发 | P1 | 适配层 | M2 |
| F9 | 超时与进程清理 | P0 | 适配层 | M1 |
| F10 | 执行结果记录与输出 | P0 | 无关 | M1 |
| F11 | 文件夹批量遍历 | P0 | 无关 | M2 |
| F12 | 嵌套/链式触发 | P1 | 无关 | M2 |
| F13 | 参数探测与多轮尝试机制 | P0 | 无关 | M1 |
| F14 | GUI 弹窗识别与键鼠模拟 | P0 | 适配层 | M2 |
| F15 | 安装结果验证与证据采集 | P0 | 适配层 | M2 |

---

### 5.2 功能详细说明

#### F1 输入接收与校验

- 支持 `--input` 指定单个文件或文件夹路径
- 支持 `--recursive` 递归遍历文件夹
- 支持 `--timeout` 指定单个文件的最大触发时长（秒），默认 120
- 支持 `--output` 指定结果输出路径（JSON）
- 支持 `--config` 指定外部配置文件路径（覆盖内嵌默认值，见 7.4）
- 输入路径不存在或无权限时：记录错误、输出**空结果 JSON + 错误摘要**（保证下游始终有产物可解析）、退出码 `2`
- 输入为空文件（0 字节）：跳过并记录 `skipped: empty_file`

#### F2 文件类型识别

识别优先级：**文件头（Magic Number）→ 扩展名 → 综合判断**

> 文件头优先于扩展名；仅当文件头无法识别时才参考扩展名归类。

| 类别 | 扩展名 | 文件头特征 |
|------|--------|-----------|
| PE 可执行 | .exe .dll .scr .com | `MZ` |
| MSI 安装包 | .msi .msp | OLE Compound File |
| 脚本 | .bat .cmd .ps1 .vbs .js .wsf | 文本特征 + 解释器关键字 |
| 压缩包 | .zip .rar .7z .tar .gz .iso | 各格式魔数 |
| 文档 | .doc .docx .xls .xlsx .pdf .rtf .ppt .pptx | OLE / ZIP / `%PDF` |
| 快捷方式 | .lnk | `4C 00 00 00` |
| 未知 | 其他 | 兜底 |

**关键要求**：文件头与扩展名不一致时，**以文件头为准**，并记录 `extension_disguised: true`。

**脚本识别细则**：`.js` 需通过内容特征区分 Windows Script（`WScript.`/`ActiveXObject`）与普通 JS 文本；无法判定时按 Windows Script 处理。

**内嵌要求**：魔数表随 exe 内嵌，不依赖 libmagic 等原生库。

#### F3 PE 可执行文件触发

**触发顺序（v2.1：安装成功优先）**：

1. **静默参数轮询**（见 F13）—— 最可能无人值守完成安装
2. **无参数直接运行** —— 触发安装向导 GUI，由 F14 识别弹窗并模拟键鼠推进
3. 上述每一条均为独立尝试，尽量多试；全部尝试期间 F14 持续监控弹窗
4. 每次尝试后由 F15 判定是否已取得安装成功证据；一旦成功可提前结束该文件

**子类型处理**：

| 子类型 | 触发方式 |
|--------|---------|
| .exe / .scr / .com | 直接执行 |
| .dll | `rundll32.exe "文件",<导出函数>`，依次尝试导出表前 N 个函数（默认 N=3，可配置） |
| .lnk | 解析目标路径与参数后，对目标重新走 F2 类型识别并触发；解析失败记 `failed` |

> 注意：即使静默参数全部失败，**无参数直接运行也必须执行一次**，并由 GUI 自动化兜底。

#### F4 安装包触发

对 `.msi`，按以下顺序尝试：

1. `msiexec /i "文件" /quiet /norestart`
2. `msiexec /i "文件" /passive /norestart`（进度条式，可能弹窗，由 F14 接管）
3. `msiexec /i "文件"`（完整 GUI，由 F14 接管）

对 `.msp` 补丁包：`msiexec /p "文件" /quiet`

要求：每次尝试记录 msiexec 退出码；退出码 `3010`（需重启）视为安装成功；`%TEMP%\MSI*.LOG` 若生成则记录路径。

#### F5 脚本类触发

| 类型 | 执行方式 |
|------|----------|
| .bat / .cmd | `cmd.exe /c "文件"` |
| .ps1 | `powershell.exe -ExecutionPolicy Bypass -NoProfile -File "文件"` |
| .vbs / .js | `wscript.exe "文件"`；若 30 秒内疑似弹 GUI，改 `cscript.exe "文件"` 重试一次 |
| .wsf | `wscript.exe "文件"` |

要求：脚本执行同样受超时控制；解释器映射通过 `IConfigProvider` 加载，不硬编码在 Core 层。

#### F6 压缩包递归处理

1. 解压到临时目录（**使用内嵌解压库**，覆盖 zip/7z/rar/tar/gz；ISO 能力不足时按 7.3 降级方案处理）
2. 遍历解压后所有文件
3. 对每个文件重新走 F2 类型识别 → 对应触发逻辑
4. **递归深度限制：默认 3 层（可配置）**，防止无限嵌套
5. **解压后总大小限制：默认 2 GB（可配置）**，超限停止并记录 `extract_size_exceeded`（防 zip 炸弹）
6. 解压失败时记录错误，不阻塞后续文件
7. 临时目录命名含任务 ID，便于退出时定向清理

#### F7 文档类触发

- 通过 `IFileOpener.hasAssociation` 判断关联程序：无则记录 `skipped: no_host_app` 并跳过
- 有关联则打开文件，由预装 Office / PDF 阅读器触发宏或漏洞链；若弹出"启用宏/允许编辑"类确认框，由 F14 接管点击
- 超时后强制结束关联进程树；记录关联程序进程名到 `platform_meta`

#### F8 未知格式兜底触发

按以下顺序尝试：

1. 文件头为 `MZ` → 按 PE 执行（F3）
2. 文件头为 ZIP/OLE → 按压缩包（F6）或文档（F7）处理
3. 否则尝试 `openWithDefaultApp`（依赖系统关联）
4. 全部失败记 `failed: untriggerable`，不阻塞

**设计原则：宁可误触发，不可漏触发。**

#### F9 超时与进程清理

- 每个文件的触发流程有独立超时（默认 120 秒，可配置）
- 超时后：记录当前阶段与已尝试参数 → 结束该文件及其派生进程树（Job Object / `taskkill /T /F`）→ 该次尝试记 `killed_on_timeout`（属"已触发"）→ 继续下一个文件
- 工具退出前清理所有临时解压目录；清理失败仅告警不影响退出码
- **全局看门狗**：单文件累计时长不得超过 `timeout × 尝试次数上限 + 60s`，防止挂死阻塞批量任务
- GUI 弹窗处理耗时计入单文件总超时

#### F10 执行结果记录与输出

输出 JSON：任务级汇总 + 文件明细数组，仅写本地文件。

```json
{
  "schema_version": "2.1",
  "task_id": "uuid",
  "started_at": "2026-09-30T09:40:00+08:00",
  "finished_at": "2026-09-30T09:52:31+08:00",
  "platform": "windows",
  "summary": {
    "total": 50,
    "installed": 12,
    "triggered": 30,
    "timeout": 5,
    "failed": 2,
    "skipped": 1
  },
  "results": [
    {
      "file_path": "C:\\samples\\test.exe",
      "file_size_bytes": 10485760,
      "file_hash_sha256": "...",
      "detected_type": "pe_executable",
      "extension_disguised": false,
      "platform_meta": {},
      "attempts": [
        {
          "method": "silent_install",
          "command": "C:\\samples\\test.exe /VERYSILENT /NORESTART",
          "result": "exited",
          "exit_code": 0,
          "duration_sec": 32,
          "install_evidence": "registry_uninstall_key"
        },
        {
          "method": "gui_automation",
          "command": "C:\\samples\\test.exe（无参数 + 弹窗接管）",
          "result": "exited",
          "exit_code": 0,
          "duration_sec": 45,
          "gui_actions": [
            { "window": "Setup - Test", "control_text": "我接受", "action": "click", "at": "2026-09-30T09:41:02+08:00" },
            { "window": "Setup - Test", "control_text": "安装", "action": "click", "at": "2026-09-30T09:41:15+08:00" }
          ]
        }
      ],
      "final_status": "install_completed",
      "notes": ""
    }
  ]
}
```

**final_status 枚举（v2.1）**：

| 取值 | 定义 | 汇总归类 |
|------|------|---------|
| `install_completed` | 出现 F15/F14.4 任一安装成功证据 | installed |
| `triggered` | 进程成功启动并运行结束，但无安装成功证据（非安装类样本的正常结果） | triggered |
| `killed_on_timeout` | 进程成功启动但被超时终结 —— 行为已被 EDR 采集，视为有效触发 | triggered（另计 timeout） |
| `failed` | 所有方式均无法启动进程 | failed |
| `skipped` | 无可用触发方式（空文件 / 文档无宿主程序等） | skipped |

> 说明：v1.0 将 `timeout` 单列为负面结果，与触发语义冲突，v2.0 起并入触发成功口径；v2.1 新增 `install_completed`，区分"跑起来了"与"装成了"。

#### F11 文件夹批量遍历

- 递归遍历输入目录下所有文件（`--recursive` 控制）
- 按类型排序处理：PE/安装包优先 → 脚本 → 文档 → 未知最后
- 单文件失败不影响后续文件；任务级汇总写入 `summary`（见 F10）
- 默认**串行**执行（并发度可配置），避免干扰 EDR 采集与 GUI 自动化抢焦点
- 批量模式下 GUI 自动化要求前台桌面可用（见 11 风险）

#### F12 嵌套/链式触发

- 压缩包解压后的文件递归处理（F6），深度与大小受限
- 安装过程中释放到临时目录的新文件不主动追踪（由 EDR 捕获）
- 脚本中调用其他程序不主动拦截（由 EDR 捕获）
- 本工具只负责"第一跳"，后续链式行为交给 EDR

#### F13 参数探测与多轮尝试机制

静默参数映射表（可外部覆盖）：

| 安装器类型 | 参数 |
|-----------|------|
| Inno Setup | `/VERYSILENT /NORESTART /SUPPRESSMSGBOXES` |
| NSIS | `/S` |
| InstallShield | `/s /v"/qn"` |
| WiX / MSI | `/quiet /norestart` |
| 通用 | `/silent` `/quiet` `--silent` `--quiet` `-s` |

**尝试规则（安装成功优先）**：

1. 按参数表顺序轮询静默参数，每次尝试期间 F14 同步监控弹窗兜底
2. 静默参数全部失败后，回退**无参数直接运行**，由 F14 接管向导推进
3. "成功"判定以 F15 安装成功证据为准，而非进程退出码本身（退出码 0 但无落盘证据存疑；MSI 3010 视为成功）
4. 每次尝试间隔 2 秒（可配置）；单文件最大尝试次数可配置（默认覆盖全部参数 + 无参数运行各 1 次）
5. 任一尝试取得安装成功证据即提前结束该文件的后续尝试
6. 参数表按平台组织，优先内嵌、支持外部覆盖

#### F14 GUI 弹窗识别与键鼠模拟（新增）

**目标**：安装向导弹窗出现时，自动识别安装相关按钮/输入项并模拟键鼠，驱动向导走到完成，实现"无人值守安装成功"。

**F14.1 窗口监控**

- 每次触发尝试期间，后台线程轮询该进程树的可见顶层窗口（默认每 500ms 一次，可配置）
- 发现新窗口即进入控件识别流程；窗口关闭后回到监控状态

**F14.2 控件识别（两级策略）**

1. **首选 UI Automation（UIA）**：读取控件树，获取按钮/输入框/复选框的文本、AutomationId 与屏幕坐标
2. **兜底图像识别**：窗口未暴露 UIA 树（自绘界面）时，截图 + 按钮模板匹配定位（模板库内嵌，可外部扩充）

**F14.3 控件语义判定（关键词表内嵌，中英双语起步）**

| 类别 | 关键词示例 | 动作 |
|------|-----------|------|
| 推进类（白名单） | 下一步 / 安装 / 是 / 确定 / 我接受 / Next / Install / Yes / OK / Agree | 点击 |
| 完成类 | 完成 / Finish / Done | 点击后本窗口流程结束 |
| 协议复选框 | 我接受协议 / I accept | 勾选 |
| 路径输入框 | 安装路径 / Destination Folder | 保留默认路径，异常时恢复默认值 |
| 拒绝类（黑名单，禁止点击） | 取消 / 退出 / 否 / 卸载 / Cancel / Exit / No / Uninstall | 不点击，仅记录 |

- 同屏多个推进类按钮时按预置优先级选取（如"安装"优先于"下一步"）
- 点击坐标加入微小随机偏移，避免固定坐标点击被反自动化识别
- 单窗口内同类动作重复触发时设置次数上限，防止死循环点击

**F14.4 安装成功判定证据（满足任一即判定成功）**

1. 安装器进程退出码为 0（MSI `3010`"重启待定"亦视为成功）
2. 出现并点击了"完成"向导页
3. 文件系统证据：安装期间 Program Files / Program Files (x86) / 自定义安装目录出现新增文件
4. 注册表证据：`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall` 下新增卸载项

**F14.5 边界与约束**

- UAC 安全桌面无法自动化：沙箱镜像应关闭 UAC 弹窗或以最高权限运行（见第 11 节风险）
- 全部键鼠动作写入 `gui_actions`（窗口标题 / 控件文本 / 动作 / 时间戳），可回放审计
- 安装器进入"正在下载组件"状态（离线环境必然卡住）：记录 `requires_network`，按超时处理
- 批量模式下需保证前台桌面可用（GUI 自动化依赖真实桌面会话）

#### F15 安装结果验证与证据采集（新增）

- 每次尝试结束后调用 `IInstallVerifier.isInstalled`，采集四类证据（同 F14.4）
- 证据写入对应 attempt 的 `install_evidence` 字段，并决定 `final_status` 是否升级为 `install_completed`
- 未取得证据但进程正常运行结束 → `triggered`；仅被超时终结 → `killed_on_timeout`
- 证据采集应在安装器进程退出后再等待一个稳定期（默认 3 秒，可配置）以避免写入延迟导致误判

---

## 6. 非功能需求

| 类别 | 要求 |
|------|------|
| 性能 | 单文件触发不超过配置超时；批量并发度可配置（默认串行，避免干扰 EDR 与 GUI 自动化） |
| 稳定性 | 单文件失败不影响整体流程；全局看门狗防挂死（F9） |
| 兼容性 | Windows 10 1809+ / Windows 11 / Server 2016+；架构预留 Linux / macOS |
| 权限 | 以管理员权限运行（静默安装需要）；启动时自检权限，无管理员权限则告警并继续；沙箱镜像建议关闭 UAC 弹窗以允许 GUI 自动化 |
| 日志 | 除 JSON 结果外，输出可读日志到 stdout 与文件（时间戳、级别、文件名、阶段） |
| 可配置 | 超时、并发度、递归深度、解压大小上限、尝试间隔与次数、参数表、按钮关键词表均可配置 |
| 无残留 | 退出前清理临时目录（含释放的临时组件）；不修改系统默认文件关联 |
| 可移植 | 核心层不依赖 Windows API；平台相关逻辑全部走适配层 |
| **离线** | 全程不访问网络，所有依赖内嵌于 exe；安装器联网等待按超时记录 `requires_network` |
| **分发** | **单一可执行文件**：无安装步骤、无外部运行时（不需 .NET Runtime / VC++ Redist）、无外部工具（不需 7z.exe），解压即用，支持介质拷贝分发 |

### 6.1 工具自身退出码表

| 退出码 | 含义 |
|--------|------|
| 0 | 全部文件处理完成（无论单文件结果如何） |
| 1 | 工具自身运行期异常崩溃 |
| 2 | 输入参数/路径非法，未开始处理 |
| 3 | 配置文件解析失败 |

---

## 7. 技术方案

### 7.1 技术选型

| 组件 | 建议 |
|------|------|
| 主语言 | **Go（首选）**：单命令静态编译为单文件 exe，无运行时依赖、分发简单；**C# / .NET 8** 亦可（需 `PublishSingleFile` + self-contained，若启用 AOT 需先验证与 UIA 的 COM 互操作兼容性） |
| 跨平台策略 | Core 层平台无关；Platform Adapter 分平台实现 |
| 进程树管理 | Windows：Job Object；Linux/macOS：进程组 |
| 文件解压 | **内嵌解压库**（Go：`mholt/archives` / `bodgit/sevenzip`；C#：SharpCompress），覆盖 zip/7z/rar/tar/gz，**不依赖外部 7z.exe**；ISO/加密 RAR 支持不足时按 7.3 降级 |
| 文件头识别 | 自行实现魔数表（约 20 条），随 exe 内嵌，不引入 libmagic |
| GUI 自动化 | Windows UIA（Go：`go-ole` / `wca`；C#：原生 COM）+ SendInput 键鼠注入 + 模板匹配兜底；**避免 CGO**，模板匹配自行实现或纯语言实现 |
| 结果输出 | JSON（主结果）+ 文本日志（辅助），仅写本地文件 |
| 配置与资源 | 默认值、参数表、关键词表、图像模板内嵌于 exe；支持外部同名文件覆盖（现场热修） |

### 7.2 单文件打包要求

| 要求 | 说明 |
|------|------|
| 产物 | 单个 `.exe`，放任意目录可运行，无需安装 |
| 无运行时依赖 | Go 默认满足；.NET 需 self-contained 发布（体积换依赖） |
| 无外部工具依赖 | 解压、哈希、GUI 自动化全部内置实现 |
| 无网络依赖 | 校验与版本信息不得联网查询 |
| 体积控制 | 目标 ≤ 100 MB；图像模板使用压缩资源内嵌 |
| 验证 | 在未安装任何运行时的干净镜像上冒烟测试（对应验收 A13） |

### 7.3 压缩包能力降级策略

1. 首选内嵌解压库覆盖 zip / 7z / rar / tar / gz（覆盖绝大多数样本）
2. ISO 或加密 RAR 等内嵌库支持不足的格式：将 7z 精简版二进制以**嵌入资源**形式随 exe 打包，首次需要时释放到临时目录调用，退出时清理
3. 仍失败则记录 `extract_failed` 并跳过，不阻塞流程

### 7.4 config.json 结构（外部覆盖用，全部字段有内嵌默认值）

```json
{
  "timeout_sec": 120,
  "concurrency": 1,
  "attempt_interval_sec": 2,
  "max_attempts_per_file": 8,
  "archive": {
    "max_depth": 3,
    "max_extract_size_bytes": 2147483648,
    "backend": "embedded"
  },
  "gui_automation": {
    "enabled": true,
    "poll_interval_ms": 500,
    "click_jitter_px": 3,
    "max_actions_per_window": 10,
    "keyword_file": "buttons.zh-CN.json",
    "template_dir": "templates"
  },
  "install_verify": {
    "settle_sec": 3,
    "watch_dirs": ["C:\\Program Files", "C:\\Program Files (x86)"]
  },
  "dll_export_probe_count": 3,
  "log_level": "info"
}
```

---

## 8. 交付物

| 编号 | 交付物 |
|------|--------|
| D1 | **单一可执行文件** `AutoTrigger.exe`（Windows 版，自包含无运行时依赖；Linux / macOS 预留接口与空实现） |
| D2 | 外部覆盖配置模板（config.json，可选） |
| D3 | 可外部编辑的参数/关键词表（silent_params.windows.json、buttons.zh-CN.json）与图像模板目录 |
| D4 | 平台抽象层接口定义文档 |
| D5 | 使用说明文档（README，含退出码表与离线部署说明） |
| D6 | 测试样本集与预期结果对照表（重点覆盖静默成功 / 向导成功 / 需联网 / 装不成功四类） |
| D7 | 结果 JSON Schema 文件（供下游流水线校验） |

---

## 9. 验收标准

| 编号 | 验收项 | 通过标准 |
|------|--------|---------|
| A1 | PE 文件触发顺序 | 静默参数轮询首先执行（≥3 种方式），随后回退无参数直接运行并由 GUI 自动化接管 |
| A2 | MSI 静默安装 | 能正确调用 msiexec，记录退出码与 MSI 日志路径，3010 判定为成功 |
| A3 | 压缩包递归 | 能解压并触发内部可执行文件，递归深度与解压大小均受控 |
| A4 | 超时控制 | 超时后进程树被完整清理不残留；该文件记 `killed_on_timeout` |
| A5 | 未知格式兜底 | 扩展名伪装文件能被文件头识别并正确触发 |
| A6 | 批量处理 | 文件夹内 50 个文件全部处理完毕，单文件失败不影响整体，summary 计数正确 |
| A7 | 结果输出 | JSON 含 `schema_version`，每次尝试含 method/result/duration，可通过 D7 Schema 校验 |
| A8 | EDR 兼容 | 工具运行期间 EDR 能正常采集到样本行为，无冲突 |
| A9 | 跨平台架构 | Core 层无直接平台 API 调用；平台适配层接口清晰可扩展 |
| A10 | 快捷方式/DLL | `.lnk` 能解析目标并触发；`.dll` 能通过 rundll32 尝试导出函数 |
| A11 | 退出码 | 参数非法→2，配置错误→3，正常完成→0 |
| A12 | GUI 自动化安装 | 对向导式安装器样本（如 Inno Setup），能通过 UIA/图像识别完成"同意 → 下一步 → 安装 → 完成"点击链 |
| A13 | 安装成功判定 | 静默安装与 GUI 自动化两条路径均能产出 `install_completed` 并附证据，且无误报（未装成功不得标记） |
| A14 | 单一二进制分发 | 在**未安装任何运行时、无网络**的干净 Windows 镜像上，仅拷贝单个 exe 即可完成全部功能（含解压与 GUI 自动化），无缺失依赖报错 |
| A15 | 离线合规 | 全程无外连行为（可用网络监控确认），安装器联网等待场景标记 `requires_network` 并按超时结束 |

---

## 10. 开发里程碑

| 里程碑 | 内容 | 覆盖功能 |
|--------|------|---------|
| M1 核心触发链路 | CLI + 配置加载 + 类型识别 + PE/MSI/脚本触发 + 多轮参数尝试 + 超时清理 + JSON 输出 | F1 F2 F3 F4 F5 F9 F10 F13 |
| M2 安装成功能力 | GUI 弹窗自动化 + 安装证据判定 + 文件夹遍历 + 压缩包递归 + 文档/未知兜底 | F6 F7 F8 F11 F12 F14 F15 |
| M3 打磨与交付 | 单文件打包、退出码完善、日志打磨、Schema 校验、测试样本集、README、离线环境冒烟 | D1~D7、A8~A15 |

---

## 11. 风险与对策

| 风险 | 对策 |
|------|------|
| 样本反沙箱检测，装死不触发 | 工具层面无法解决，需沙箱环境加固；工具确保触发动作本身完整 |
| 安装器需联网下载组件 | 离线环境按超时处理并标记 `requires_network`，不做网络补偿 |
| 安装程序卡在 GUI 等待输入 | 超时强制结束并记录；有 F14 时优先由 GUI 自动化推进 |
| 压缩包嵌套过深或 zip 炸弹 | 递归深度限制（默认 3 层）+ 解压大小限制（默认 2 GB） |
| 静默参数抑制行为或直接失败 | 静默与无参数两种模式均作为独立尝试执行，尽量多试（F3/F13） |
| UAC 安全桌面拦截自动化点击 | UIA 无法作用于安全桌面；沙箱镜像关闭 UAC 弹窗或工具随系统自启于最高权限 |
| GUI 自动化误点（取消/卸载/格式化类按钮） | 黑名单关键词禁止点击（F14.3）；全部键鼠动作留痕可回放审计 |
| GUI 自动化在无桌面会话（Session 0）失效 | 明确要求前台桌面可用；批量模式默认串行，避免抢焦点 |
| 安装成功误判（退出码 0 但实际未装） | 需 F15 多重证据（落盘/注册表/完成页），并设置退出后稳定期 |
| 单文件打包与 GUI/解压库冲突 | Go 避免 CGO；.NET 若用 AOT 需先验证 UIA COM 互操作；解压降级见 7.3 |
| 工具自身被样本攻击 | 运行在沙箱内，崩溃不波及宿主机；建议工具与样本进程隔离 |
| EDR 与工具进程冲突 | 工具自身进程可加入 EDR 白名单，避免误报 |
| 跨平台抽象过度设计 | 本期只实现 Windows 适配层，接口保持最小可用，后期按需扩展 |
| 工具异常挂死阻塞流水线 | 全局看门狗（F9）+ 退出码区分崩溃（6.1） |

---

## 12. 待确认问题清单（开发前需澄清）

| 编号 | 问题 | 影响 |
|------|------|------|
| Q1 | 内嵌解压库对 ISO / 加密 RAR 支持不足时，是否接受"内嵌 7z 二进制资源、运行时释放调用"的折中方案？ | 影响 F6 与 7.3 实现 |
| Q2 | `.dll` 触发用 rundll32 探测导出函数的数量与选取策略（按导出表顺序 / 关键字优先 / 随机）？ | 影响 F3 与 EDR 采集效果 |
| Q3 | 批量场景下单文件结束后是否需要"冷却间隔"（如 5 秒），让 EDR 完成行为上报落盘？ | 影响 F11 与 EDR 对接效果 |
| Q4 | 结果 JSON 的交付方式？（离线环境无上传通道：本地文件 / 共享目录 / 人工拷贝） | 影响 F10 输出通道 |
| Q5 | 是否需要保留同一文件多次任务的尝试历史（用于后续"触发方式优先级动态调整"）？ | 影响结果存储设计 |
| Q6 | GUI 图像识别模板库初始覆盖范围？（建议先覆盖 Inno Setup / NSIS / InstallShield 三大主流向导） | 影响 F14.2 工作量 |
| Q7 | 安装成功是否需要更强证据（如启动安装后的程序验证可用性）？ | 影响 F15 判定精度与工作量 |
| Q8 | 语言选型最终定 Go 还是 C#/.NET？（Go 单文件更省事，C# 的 UIA 支持更成熟） | 影响整体技术栈与排期 |

---

## 13. 后续可扩展方向（非本期范围）

- 实现 Linux 适配层（deb / rpm / sh / ELF 触发）
- 实现 macOS 适配层（pkg / dmg / Mach-O 触发）
- 支持自定义触发脚本插件（针对特定样本类型写触发逻辑）
- 支持与 EDR API 联动，触发后自动拉取行为报告（需环境提供通道）
- 支持触发方式优先级动态调整（基于历史成功率）
- 支持分布式批量触发（多沙箱并行）
- GUI 自动化模板库众包扩充（按安装器家族沉淀模板）

---

文档结束
