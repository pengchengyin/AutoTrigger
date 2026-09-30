# Windows 任意文件自动安装/触发工具 PRD

## 1. 文档信息

| 项目 | 内容 |
|------|------|
| 文档名称 | Windows 任意文件自动安装/触发工具 PRD |
| 版本 | v2.0（基于 v1.0 评审优化） |
| 状态 | 待开发 |
| 目标平台 | Windows 10 1809+ / 11 / Server 2016+（架构预留跨平台扩展） |
| 运行环境 | 已有沙箱环境（工具不负责隔离，不负责检测） |
| 下游依赖 | EDR（负责行为采集与检测） |

### 1.1 v2.0 变更说明（相对 v1.0）

| 编号 | 变更 | 原因 |
|------|------|------|
| C1 | 统一触发顺序：**无参数直接运行优先，静默参数作为补充** | v1.0 中 F3 与 F13 顺序矛盾 |
| C2 | 重定义 final_status 枚举，`killed_on_timeout` 视为"已触发" | 本工具目标是引爆而非装成功，超时=行为已被 EDR 采集 |
| C3 | 补充 `.lnk`（F3.4）与 `.dll`（F3.3）触发逻辑 | v1.0 F2 识别了这两类但无处理流程 |
| C4 | 平台抽象层接口补齐 `IInstallerTrigger`、新增 `IConfigProvider` | v1.0 F4 引用了未定义的接口 |
| C5 | 新增退出码表（7.2）、config.json 结构（7.3）、解压大小限制 | 工程落地必需，v1.0 缺失 |
| C6 | 结果 JSON 增加 `schema_version`、`started_at`、单次尝试的 stdout 摘要 | 便于流水线版本兼容与问题排查 |
| C7 | 修复 v1.0 第 5.2 节后半部分至第 11 节的格式损坏 | 原文档表格丢失，影响评审 |
| C8 | 新增开发里程碑拆分（第 10 节）与待确认问题清单（第 12 节） | 便于排期与需求澄清 |

---

## 2. 背景与目标

### 2.1 背景

软件供应链安全检测部门需要对**入网的任意文件**进行动态行为检测。样本进入沙箱后，缺少一个能够**自动识别文件类型并尝试各种安装/执行方式**的触发工具。检测环节由沙箱内已有的 EDR 负责，本工具只负责"让文件跑起来"。

### 2.2 目标

开发一个运行在 Windows 沙箱内的自动化触发工具，接收**指定的任意文件或文件夹**，自动识别文件类型，尝试多种安装/执行方式，尽可能触发样本的真实行为，供 EDR 采集。

工具需在架构层面**预留跨平台扩展能力**，便于后期支持 Linux / macOS。

### 2.3 非目标（明确不做）

- 不负责沙箱隔离与逃逸防护（运行环境已是沙箱）
- 不负责行为监控、日志采集、威胁判定（由 EDR 负责）
- 不负责样本静态分析、反混淆、脱壳
- 不负责网络隔离策略配置
- 不追踪样本运行后释放的后续文件与链式行为（只负责"第一跳"，其余由 EDR 捕获）
- 本期不实现 Linux / macOS，仅预留架构扩展点

---

## 3. 用户与使用场景

### 3.1 目标用户

- 供应链安全检测工程师
- 自动化检测流水线运维人员

### 3.2 典型场景

| 场景 | 描述 |
|------|------|
| 单文件触发 | 传入一个 `.exe`，工具尝试无参数运行、静默安装等多种方式 |
| 文件夹批量触发 | 传入一个目录，工具遍历所有文件，逐个识别并触发 |
| 压缩包递归触发 | 传入 `.zip`，工具解压后递归触发内部所有可执行文件 |
| 文档类触发 | 传入 `.docx` / `.pdf`，工具调用关联程序打开，触发宏/漏洞利用链 |
| 未知格式兜底 | 传入扩展名未知或文件头与扩展名不符的文件，工具仍尝试执行 |

### 3.3 使用方式

命令行调用为主，便于集成到自动化流水线：

```
AutoTrigger --input "C:\samples\test.exe" --timeout 120 --output "C:\logs\result.json"
AutoTrigger --input "C:\samples\folder" --recursive --timeout 120
AutoTrigger --input "C:\samples" --config "C:\conf\config.json"
```

参数优先级：命令行参数 > 配置文件 > 内置默认值。

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
    extract(archivePath, destDir) -> ExtractResult
    supportedExtensions() -> list

interface IFileTypeDetector:
    detect(filePath) -> FileType
    magicNumberMap() -> map                  // 平台无关，支持按平台扩展

interface IInstallerTrigger:                 // v2.0 补充定义
    getTriggerCommands(filePath, fileType) -> list[TriggerCommand]
    // Windows: msiexec 系列；Linux(后期): dpkg/rpm；macOS(后期): pkg/dmg

interface IConfigProvider:                   // v2.0 新增
    loadConfig(path) -> Config
    // 静默参数表、类型映射表按平台分文件存放，核心层按当前平台加载
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
| F13 | 参数探测与回退机制 | P0 | 无关 | M1 |

---

### 5.2 功能详细说明

#### F1 输入接收与校验

- 支持 `--input` 指定单个文件或文件夹路径
- 支持 `--recursive` 递归遍历文件夹
- 支持 `--timeout` 指定单个文件的最大触发时长（秒），默认 120
- 支持 `--output` 指定结果输出路径（JSON）
- 支持 `--config` 指定配置文件路径（见 7.3）
- 输入路径不存在或无权限时：记录错误、输出**空结果 JSON + 错误摘要**（保证下游流水线始终有产物可解析）、以退出码 `2` 退出
- 输入为空文件（0 字节）：跳过并记录 `skipped: empty_file`

#### F2 文件类型识别

识别优先级：**文件头（Magic Number）→ 扩展名 → 综合判断**

> v2.0 调整：文件头优先于扩展名。仅当文件头无法识别时，才参考扩展名归类。

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

**脚本识别细则**：`.js` 需通过内容特征区分 Windows Script（`WScript.`/`ActiveXObject` 等）与普通 JavaScript 文本；无法判定时按 Windows Script 处理。

**跨平台要求**：类型识别逻辑放在 Core 层，魔数表平台无关，各平台可扩展自己的类型映射（如 Linux 的 .deb / .rpm）。

#### F3 PE 可执行文件触发

**触发顺序（v2.0 统一，见 C1）**：

1. **无参数直接运行** —— 第一优先级。目的是引爆而非装成功，直接运行最能暴露真实行为
2. **常见静默参数轮询**（见 F13）—— 作为补充，覆盖"直接运行弹 GUI 卡死"导致行为未展开的情况
3. 若上一步骤中进程弹 GUI 卡住被超时终结，超时本身已计入"已触发"

**子类型处理**：

| 子类型 | 触发方式 |
|--------|---------|
| .exe / .scr / .com | 直接执行 |
| .dll | `rundll32.exe "文件",<导出函数>`，依次尝试 `DllMain` 相关入口与导出表前 N 个函数（默认 N=3，可配置） |
| .lnk | 解析目标路径与参数后，对目标文件重新走 F2 类型识别并触发；解析失败则记录 `failed` |

> 注意：即使静默安装失败，**无参数直接运行也必须执行一次**。

**跨平台要求**：通过 `IProcessRunner` 接口执行，Windows 适配层实现 PE 相关逻辑，Linux 适配层后期实现 ELF 触发。

#### F4 安装包触发

对 `.msi` 文件，按以下顺序尝试：

1. `msiexec /i "文件" /quiet /norestart`
2. `msiexec /i "文件" /passive /norestart`
3. `msiexec /i "文件"`（无静默参数，观察 GUI 行为）

对 `.msp` 补丁包：`msiexec /p "文件" /quiet`

每次尝试均记录 msiexec 退出码与日志路径（`%TEMP%\MSI*.LOG`，若生成）。

**跨平台要求**：安装包触发逻辑通过 `IInstallerTrigger` 接口抽象，Windows 实现 MSI，Linux 后期实现 deb/rpm，macOS 后期实现 pkg/dmg。

#### F5 脚本类触发

| 类型 | 执行方式 |
|------|----------|
| .bat / .cmd | `cmd.exe /c "文件"` |
| .ps1 | `powershell.exe -ExecutionPolicy Bypass -NoProfile -File "文件"` |
| .vbs / .js | `wscript.exe "文件"`；若 30 秒内有 GUI 弹窗嫌疑则换 `cscript.exe "文件"` 重试一次 |
| .wsf | `wscript.exe "文件"` |

要求：脚本执行同样受超时控制；脚本解释器与类型的映射通过 `IConfigProvider` 按平台加载，**不硬编码在 Core 层**。

#### F6 压缩包递归处理

1. 解压到临时目录（优先使用 7-Zip 命令行，支持 zip/rar/7z/iso 等）
2. 遍历解压后所有文件
3. 对每个文件重新走 F2 类型识别 → 对应触发逻辑
4. **递归深度限制：默认 3 层（可配置）**，防止 zip 炸弹或无限嵌套
5. **解压后总大小限制：默认 2 GB（可配置）**，超限停止解压并记录 `extract_size_exceeded`（v2.0 新增，对应风险清单中 zip 炸弹对策）
6. 解压失败时记录错误，不阻塞后续文件
7. 临时目录命名含任务 ID，便于退出时定向清理

**跨平台要求**：通过 `IArchiveExtractor` 接口抽象，各平台可实现不同解压后端。

#### F7 文档类触发

- 通过 `IFileOpener.hasAssociation` 判断系统是否有关联程序：无则记录 `skipped: no_host_app` 并跳过
- 有关联则 `openWithDefaultApp` 打开文件，由预装的 Office / PDF 阅读器触发宏或漏洞链
- 超时后强制结束关联进程树
- 打开前后记录关联程序的进程名，写入 `platform_meta`

**跨平台要求**：通过 `IFileOpener` 接口抽象，Windows 用 ShellExecute，Linux 用 xdg-open，macOS 用 open。

#### F8 未知格式兜底触发

当扩展名未知或无法归类时，按以下顺序尝试：

1. 文件头为 `MZ` → 按 PE 执行（F3）
2. 文件头为 ZIP/OLE → 按压缩包（F6）或文档（F7）处理
3. 否则尝试 `openWithDefaultApp`（依赖系统关联）
4. 全部失败则记录 `failed: untriggerable`，不阻塞

**设计原则：宁可误触发，不可漏触发。**

#### F9 超时与进程清理

- 每个文件的触发流程有独立超时（默认 120 秒，可配置）
- 超时后：
  1. 记录当前执行阶段与已尝试的参数
  2. 结束该文件及其派生的整个进程树（使用 Job Object 或 `taskkill /T /F`）
  3. 该次尝试记为 `killed_on_timeout`（**属于"已触发"**，见 5.3 final_status 定义）
  4. 继续处理下一个文件
- 工具退出前，清理所有临时解压目录；清理失败仅告警不影响退出码
- 全局看门狗：单个文件累计触发时长不得超过 `timeout × 尝试次数上限 + 60s`，防止异常挂死阻塞整个批量任务

**跨平台要求**：通过 `IProcessRunner.killTree` 接口抽象，Windows 用 Job Object，Linux/macOS 用进程组。

#### F10 执行结果记录与输出

输出 JSON 格式，顶层为任务级汇总 + 文件明细数组：

```json
{
  "schema_version": "2.0",
  "task_id": "uuid",
  "started_at": "2026-09-30T09:40:00+08:00",
  "finished_at": "2026-09-30T09:52:31+08:00",
  "platform": "windows",
  "summary": {
    "total": 50,
    "triggered": 42,
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
          "method": "direct_run",
          "command": "C:\\samples\\test.exe",
          "result": "killed_on_timeout",
          "duration_sec": 120,
          "stdout_head": "（前 2KB 输出摘要，可选）"
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
  ]
}
```

**final_status 枚举（v2.0 重定义，见 C2）**：

| 取值 | 定义 | 汇总归类 |
|------|------|---------|
| `triggered` | 至少一次尝试成功启动进程且运行结束（含退出码非 0） | triggered |
| `killed_on_timeout` | 进程成功启动但被超时终结 —— **行为已被 EDR 采集，视为有效触发** | triggered |
| `failed` | 所有方式均无法启动进程 | failed |
| `skipped` | 无可用触发方式（空文件 / 文档无宿主程序等） | skipped |

> 说明：v1.0 将 `timeout` 单列为负面结果，与本工具"引爆"的目标语义冲突，v2.0 将超时并入触发成功口径；汇总中单列 `timeout` 计数供运维参考。

#### F11 文件夹批量遍历

- 递归遍历输入目录下所有文件（`--recursive` 控制）
- 按文件类型排序处理：PE/安装包优先 → 脚本 → 文档 → 未知最后
- 单文件失败不影响后续文件
- 任务级汇总写入结果 JSON 的 `summary` 字段（见 F10）
- 批量处理默认**串行**执行（可配置并发度），避免干扰 EDR 采集

#### F12 嵌套/链式触发

- 压缩包解压后的文件递归处理（F6），深度与大小受限（见 F6 第 4/5 条）
- 安装过程中若释放新文件到临时目录，不主动追踪（由 EDR 捕获）
- 脚本中若调用其他程序，不主动拦截（由 EDR 捕获）
- 本工具只负责"第一跳"，后续链式行为交给 EDR

#### F13 参数探测与回退机制

维护一个静默参数映射表（按安装器指纹识别或直接轮询），作为 F3 第 2 步的补充触发：

| 安装器类型 | 参数 |
|-----------|------|
| Inno Setup | `/VERYSILENT /NORESTART /SUPPRESSMSGBOXES` |
| NSIS | `/S` |
| InstallShield | `/s /v"/qn"` |
| WiX / MSI | `/quiet /norestart` |
| 通用 | `/silent` `/quiet` `--silent` `--quiet` `-s` |

**回退规则（v2.0 统一口径，见 C1）**：

1. 无参数直接运行**始终先执行**（F3 第 1 步），不受参数表影响
2. 直接运行结束后，再按参数表顺序轮询静默参数，任一方式进程成功启动即记录
3. 每次尝试之间间隔 2 秒（可配置），避免资源竞争
4. 参数表按平台分文件存放（Windows 用上述表，Linux 后期维护 deb/rpm/sh 参数表），通过 `IConfigProvider` 加载，支持外部编辑覆盖

---

## 6. 非功能需求

| 类别 | 要求 |
|------|------|
| 性能 | 单文件触发不超过配置超时；批量处理并发度可配置（默认串行，避免干扰 EDR 采集） |
| 稳定性 | 单文件失败不影响整体流程；全局看门狗防单文件挂死（F9） |
| 兼容性 | Windows 10 1809+ / Windows 11 / Server 2016+；架构预留 Linux / macOS |
| 权限 | 需以管理员权限运行；启动时自检权限，无管理员权限则告警并继续（部分触发方式会失败） |
| 日志 | 除 JSON 结果外，输出可读日志到 stdout 和文件（含时间戳、级别、文件名、阶段） |
| 可配置 | 超时、并发度、递归深度、解压大小上限、尝试间隔、参数表均可通过配置文件或命令行覆盖 |
| 无残留 | 退出前清理临时目录；不修改系统默认文件关联 |
| 可移植 | 核心层不依赖 Windows API；平台相关逻辑全部走适配层 |

### 6.1 工具自身退出码表（v2.0 新增）

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
| 主语言 | C# / .NET 8（推荐：进程控制强、跨平台好）或 Go（单一二进制分发方便） |
| 跨平台策略 | Core 层用平台无关语言编写；Platform Adapter 按平台分别实现 |
| 进程树管理 | Windows：Job Object；Linux/macOS：进程组 |
| 解压 | 优先 7-Zip 命令行（三平台均有），可扩展其他后端 |
| 文件头识别 | 自行实现魔数表（约 20 条，工作量小、无原生依赖），不引入 libmagic |
| 输出格式 | JSON（主结果）+ 文本日志（辅助） |
| 配置格式 | JSON，按平台分文件存放参数表与类型映射 |

### 7.2 退出码

见 6.1。

### 7.3 config.json 结构（v2.0 新增）

```json
{
  "timeout_sec": 120,
  "concurrency": 1,
  "attempt_interval_sec": 2,
  "archive": {
    "max_depth": 3,
    "max_extract_size_bytes": 2147483648,
    "backend": "7z"
  },
  "silent_params_file": "silent_params.windows.json",
  "file_type_map_file": "filetypes.windows.json",
  "dll_export_probe_count": 3,
  "log_level": "info"
}
```

---

## 8. 交付物

| 编号 | 交付物 |
|------|--------|
| D1 | 可执行工具（Windows 版；Linux / macOS 预留接口与空实现） |
| D2 | 配置文件模板（config.json） |
| D3 | 静默参数映射表（按平台分文件，可外部编辑） |
| D4 | 平台抽象层接口定义文档 |
| D5 | 使用说明文档（README，含退出码表） |
| D6 | 测试样本集与预期结果对照表 |
| D7 | 结果 JSON Schema 文件（供下游流水线校验） |

---

## 9. 验收标准

| 编号 | 验收项 | 通过标准 |
|------|--------|---------|
| A1 | 单 PE 文件触发 | 无参数直接运行首先执行，随后按顺序尝试静默参数（≥3 种方式） |
| A2 | MSI 静默安装 | 能正确调用 msiexec 并记录退出码与 MSI 日志路径 |
| A3 | 压缩包递归 | 能解压并触发内部可执行文件，递归深度与解压大小均受控 |
| A4 | 超时控制 | 超时后进程树被完整清理，不残留；该文件记为 `killed_on_timeout` |
| A5 | 未知格式兜底 | 扩展名伪装文件能被文件头识别并正确触发 |
| A6 | 批量处理 | 文件夹内 50 个文件全部处理完毕，单文件失败不影响整体，summary 计数正确 |
| A7 | 结果输出 | JSON 含 `schema_version`，每次尝试含 method/result/duration，可通过 D7 Schema 校验 |
| A8 | EDR 兼容 | 工具运行期间 EDR 能正常采集到样本行为，无冲突 |
| A9 | 跨平台架构 | Core 层无直接平台 API 调用；平台适配层接口清晰可扩展 |
| A10 | 快捷方式/DLL | `.lnk` 能解析目标并触发；`.dll` 能通过 rundll32 尝试导出函数 |
| A11 | 退出码 | 参数非法→2，配置错误→3，正常完成→0 |

---

## 10. 开发里程碑（v2.0 新增）

| 里程碑 | 内容 | 覆盖功能 |
|--------|------|---------|
| M1 核心触发链路 | CLI + 配置加载 + 类型识别 + PE/MSI/脚本触发 + 超时清理 + JSON 输出 | F1 F2 F3 F4 F5 F9 F10 F13 |
| M2 批量与扩展触发 | 文件夹遍历 + 压缩包递归 + 文档/未知兜底触发 | F6 F7 F8 F11 F12 |
| M3 打磨交付 | 退出码完善、日志打磨、Schema 校验、测试样本集、README | D1~D7、A8~A11 |

---

## 11. 风险与对策

| 风险 | 对策 |
|------|------|
| 样本反沙箱检测，装死不触发 | 工具层面无法解决，需沙箱环境加固；工具确保触发动作本身完整 |
| 安装程序卡在 GUI 等待输入 | 超时机制强制结束，记录 `killed_on_timeout`（行为已被采集） |
| 压缩包嵌套过深或 zip 炸弹 | 递归深度限制（默认 3 层）+ 解压大小限制（默认 2 GB） |
| 静默参数导致样本行为被抑制 | 无参数直接运行始终作为第一优先级执行 |
| 工具自身被样本攻击 | 运行在沙箱内，工具崩溃不影响宿主机；建议工具与样本进程隔离 |
| EDR 与工具进程冲突 | 工具自身进程可加入 EDR 白名单，避免误报 |
| 跨平台抽象过度设计 | 本期只实现 Windows 适配层，接口保持最小可用，后期按需扩展 |
| 工具异常挂死阻塞流水线 | 全局看门狗（F9）+ 退出码区分崩溃（6.1） |

---

## 12. 待确认问题清单（v2.0 新增，开发前需澄清）

| 编号 | 问题 | 影响 |
|------|------|------|
| Q1 | 沙箱内是否预装 7-Zip？若未预装，工具是否需自带 7z.exe（随包分发）？ | 影响交付物与 F6 实现 |
| Q2 | `.dll` 触发的 rundll32 探测导出函数数量与选取策略（按导出表顺序 or 随机）？ | 影响 F3.3 与 EDR 采集效果 |
| Q3 | 批量场景下单文件触发结束后是否需要"冷却间隔"（如 5 秒），让 EDR 完成行为上报落盘？ | 影响 F11 与 EDR 对接效果 |
| Q4 | 结果 JSON 是否需要上报到某个流水线 API，还是仅写本地文件？ | 影响 F10 输出通道 |
| Q5 | 是否需要对同一文件保留多次任务的尝试历史（用于后续"触发方式优先级动态调整"扩展）？ | 影响结果存储设计 |

---

## 13. 后续可扩展方向（非本期范围）

- 实现 Linux 适配层（deb / rpm / sh / ELF 触发）
- 实现 macOS 适配层（pkg / dmg / Mach-O 触发）
- 支持自定义触发脚本插件（用户可针对特定样本类型写触发逻辑）
- 支持从 URL 下载样本后触发
- 支持与 EDR API 联动，触发后自动拉取行为报告
- 支持触发方式优先级动态调整（基于历史成功率）
- 支持分布式批量触发（多沙箱并行）

---

文档结束
