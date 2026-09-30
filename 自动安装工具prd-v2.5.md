# Windows 任意文件自动安装/触发工具 PRD

## 1. 文档信息

| 项目 | 内容 |
|------|------|
| 文档名称 | Windows 任意文件自动安装/触发工具 PRD |
| 版本 | v2.5（安装成功优先 · GUI 自动化 · **外部开源 OCR 引擎** · 离线 · Go · **Win7 SP1+ 兼容** · **工具本体单文件 + 完整交付包**） |
| 状态 | 待开发 |
| 目标平台 | **Windows 7 SP1 x64 / 8.1 / 10 / 11 / Server 2008 R2+**（架构预留跨平台扩展） |
| 运行环境 | 离线沙箱环境（工具不负责隔离，不负责检测） |
| 交付形态 | **两级模型**：① **工具本体** = 单个 `AutoTrigger.exe`，免安装、拷贝即用、无需任何语言运行时（用户使用极简）；② **交付包** = 工具本体 + **7-Zip / OCR 引擎等第三方离线安装包** + **部署说明文档** + 可选配置与模板（部署时必须整体提供）。详见 8.1 |
| 环境前置 | ① **7-Zip** ② **OCR 引擎**（默认 PaddleOCR-json）（由镜像/运维按**部署说明文档**预装；**离线安装包随交付包提供**）。工具启动自检，缺失时提示并指向交付包内安装包，**工具自身不代为安装**；另含管理员权限、关闭 UAC、AV 排除目录等镜像准备项，**完整边界见 4.7** |
| 下游依赖 | EDR（负责行为采集与检测） |

### 1.1 变更说明（相对 v1.0）

| 编号 | 变更 | 原因 |
|------|------|------|
| C1 | 修复触发顺序矛盾，统一口径（v2.1 进一步调整为安装成功优先） | v1.0 中 F3 与 F13 顺序自相矛盾 |
| C2 | 重定义 final_status 枚举，`killed_on_timeout` 视为"已触发" | 超时说明进程已运行、行为已被 EDR 采集 |
| C3 | 补充 `.lnk` 与 `.dll` 触发逻辑 | v1.0 F2 识别了这两类但无处理流程 |
| C4 | 平台抽象层接口补齐 `IInstallerTrigger`，新增 `IConfigProvider`、`IGuiAutomator`、`IInstallVerifier` | v1.0 F4 引用了未定义的接口 |
| C5 | 新增退出码表（6.1）、config.json 结构（7.5）、解压大小限制 | 工程落地必需，v1.0 缺失 |
| C6 | 结果 JSON 增加 `schema_version`、`task_id`、时间戳、尝试输出摘要 | 便于流水线版本兼容与问题排查 |
| C7 | 修复 v1.0 第 5.2 节后半部分至第 11 节的格式损坏 | 原文档表格丢失，影响评审 |
| C8 | 新增开发里程碑（第 10 节）与待确认问题清单（第 12 节） | 便于排期与需求澄清 |
| C9 | 目标调整为**以"安装成功"为首要目标**，静默参数优先、无参数运行 + GUI 自动化兜底，尽量多试 | 需求：尽可能装成功 |
| C10 | 新增 F14：GUI 弹窗识别与键鼠模拟（UIA 优先 + 图像识别兜底） | 安装向导弹窗需自动完成点击/输入 |
| C11 | 新增离线约束：全程不联网，安装器联网等待按超时记录 `requires_network` | 部署环境为离线沙箱 |
| C12 | final_status 新增 `install_completed`，含退出码/完成页/落盘/注册表四类证据；新增 F15 安装结果验证 | "装成了"需可验证 |
| C13 | 新增单一二进制分发约束 | 便于分发到各沙箱节点 |
| C14 | **解压方案定案**：不接受内嵌/运行时释放的降级方案，统一依赖外部 7-Zip | 需求决策 |
| C15 | **语言定案 Go**（静态编译单文件，目标机无需 Go 环境）；补充高内聚低耦合的工程规范与目录结构（7.3） | 需求决策 |
| C16 | 补充 GUI 图像模板初始覆盖清单与模板库迭代机制（F14.2） | 明确模板范围 |
| C17 | **交付形态定案**：交付物即**一个免安装、可直接运行的二进制文件**，不附带安装包、脚本与运行时；**7-Zip 作为环境前置由镜像/运维预装**，工具启动时检测，**缺失则提示用户先安装** | 需求决策 |
| C18 | 新增 OCR 辅助定位：GUI 控件识别升级为「UIA → 图像模板 → OCR」三级策略 | 需求决策 |
| C19 | Windows 内置 OCR 实测结论（2026-09-30）：引擎可用但**仅装 `zh-Hans-CN`、无 `en-US`**，且为**单语言引擎** | 实测验证结果 |
| C20 | **目标平台下探至 Windows 7 SP1 x64 及以上**；由此**锁定 Go 1.20.x 工具链**（Go 1.21+ 已放弃 Win7 支持），新增第 4.6 节 Win7 兼容性约束 + 验收 A20 | 需求决策 |
| C21 | **OCR 引擎选型改为外部开源 OCR（默认 PaddleOCR-json）**：作为**环境前置依赖**由镜像预装，工具检测 + 提示、不代装；进程外调用（stdio / TCP），**不使用 CGO**。Windows 内置 OCR 降为**可选回退后端**（默认关闭） | 需求决策：内置 OCR 语言包受限、Win7 不可用 |
| C22 | OCR 置信度由"启发式打分"升级为**引擎原生 `score` 阈值过滤**（PaddleOCR-json 返回逐文本框置信度） | 外部引擎能力增强 |
| C23 | **澄清运行时依赖边界**：Go 静态编译只消除**语言运行时**依赖，**不消除功能性前置依赖**（7-Zip / OCR 引擎 / VC++ 运行库）与**镜像准备项**（管理员权限、关闭 UAC、AV 排除、可写临时目录）；新增第 4.7 节与验收 A22 | 避免"免安装 = 零环境要求"的误读，明确部署前置清单 |
| C24 | **交付形态修正为两级模型**：**工具本体**是单一二进制（使用极简：拷贝即用、零语言运行时依赖）；**交付包**是完整的部署单元，**包含 7-Zip / OCR 引擎等第三方离线安装包 + 部署说明文档 + 可选配置模板**。修正 v2.4 中"不携带任何安装包"的表述——原表述混淆了"工具形态"与"交付包组成" | 需求澄清：工具使用要简单，但部署所需的一整套东西必须一并交付 |
| C25 | **新增 7.7 OCR 引擎候选评估记录**：按 8 项硬性门槛评估外部资料所列 12 个 OCR 开源项目及评论区补充项（HunyuanOCR / DeepSeek-OCR），结论为**均未优于现选 PaddleOCR-json**（任务类型不匹配：文档结构化 vs 控件定位） | 评审追溯：留档选型依据，避免重复调研 |
| C26 | **新增 7.1.1 语言选型论证（为什么 Go 而非 Python）**：决定性因素是**交付形态**（Python 无真静态编译路径，PyInstaller 单文件运行时释放到 `%TEMP%\_MEIxxxx`、有残留、体积大、启动慢，与 DC1 及"无残留"验收冲突）+ **反病毒友好性**（打包器产物是 AV 误报重灾区，而本工具本身已属高风险）+ **Win7 下 Python 只能锁已 EOL 的 3.8**（Go 为 2023-02 的 1.20.x）。Python 的 OCR/GUI 生态优势已被"OCR 外置进程 + GUI 走原生 Win32/UIA API"两项决策消解 | 评审追溯：语言选择为高频质疑点，留档依据避免重复论证 |
| C27 | **补充配置加载规则（7.5.1）**：明确**四级优先级**（命令行 > `--config` 指定文件 > 同目录 `config.json` > 内嵌默认值）、**深合并语义**（字段级递归覆盖，非整体替换——原文档仅写"覆盖内嵌默认值"，语义缺失会导致实现歧义）、未知字段**忽略 + 告警**（向前兼容）、类型/越界错误以**退出码 3 失败不静默回落**；新增 `--dump-config` 导出生效配置与结果中的 `config_digest` | 工程落地必需：配置合并语义与失败行为必须开发前定死 |

### 1.2 已决策事项（评审确认，不再作为待确认项）

| 编号 | 决策 | 落点 |
|------|------|------|
| DC1 | 交付形态（两级）**工具本体 = 单一可执行文件、免安装、零语言运行时依赖**；**交付包 = 工具本体 + 第三方离线安装包 + 部署说明文档** | 2.3、8.1、8.3 |
| DC2 | 语言：Go（`CGO_ENABLED=0` 静态编译） | 7.1、7.3 |
| DC3 | 解压后端：外部 7-Zip；**不做内嵌降级**；**7-Zip 离线安装包随交付包提供** | 7.4、F6、8.3 |
| DC4 | 主目标：尽可能安装成功，含 GUI 弹窗自动化 | 2.2、F3/F13/F14 |
| DC5 | 环境：离线，全程无外连 | 2.3、6 |
| DC6 | **7-Zip 缺失时的行为**：仅检测 + **明确提示用户先安装**（指向交付包内离线安装包），工具自身不代为安装、不静默降级 | F16、7.4、6.1 |
| DC7 | GUI 识别三级策略：**UIA 首选 → 图像模板匹配 → OCR 辅助兜底**；OCR 是辅助手段，不得成为唯一依赖 | F14.2、F14.6 |
| DC8 | **OCR 引擎：外部开源 OCR 进程（默认 `PaddleOCR-json`）**，环境预置、进程外调用（stdio/TCP）、**禁止 CGO**；Windows 内置 OCR 仅作可选回退后端且默认关闭 | 7.1、7.6、F14.6 |
| DC9 | **平台基线：Windows 7 SP1 x64 及以上**；**Go 工具链锁定 1.20.x**（Go 1.21+ 不支持 Win7） | 4.6、7.1、A20 |
| DC10 | **OCR 引擎与 7-Zip 同策略**：仅检测 + 提示 + 不代装；缺失时 OCR 自动降级为「UIA + 模板」，不中断主流程（`require_ocr: true` 时除外） | F16.3、6.1 |
| DC11 | OCR 置信度采用**引擎原生 `score`** + `min_score` 阈值过滤，取代 v2.3 的启发式打分 | F14.6 |
| DC12 | **运行时依赖边界**：工具本体**零语言运行时依赖**（不需 Go / .NET / JRE / VC++）；**7-Zip 与 OCR 引擎属功能性前置依赖**，由镜像预置；镜像另需满足管理员权限、UAC 关闭、AV 排除目录、可写临时目录四项准备 | 4.7、A22 |
| DC13 | **工具使用极简、部署一并交付**：运维只需两层动作——① 按**部署说明文档**安装交付包内的第三方离线包（7-Zip / OCR 引擎）；② 拷贝 `AutoTrigger.exe` 到运行目录。**工具本体不含任何安装逻辑**（不代为安装、不释放文件、不写注册表） | 8.1、8.3、F16 |
| DC14 | **OCR 引擎维持现选**：`PaddleOCR-json`（默认）+ `RapidOCR-json`（免 AVX 备选），**不引入 Python / PyTorch / GPU 依赖，不引入文档解析类或 VLM 类模型**；外部候选评估结论见 7.7 | 7.1、7.6、7.7 |

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
| 离线 | 工具自身与触发流程**全程不访问网络**：不下载组件、不更新规则、不上报结果。**所需的第三方离线安装包随交付包一并提供**，无需现场联网获取 |
| **工具本体：单文件免安装** | 交付的**可执行文件为单一二进制**（Go 静态编译）：拷贝到任意目录即可运行，**无安装步骤、无外部运行时**（不需 Go / .NET），无注册表写入、无残留。**这是"工具侧使用简单"的核心保证** |
| **交付包：完整部署单元** | 交付包 = 工具本体 + **7-Zip / OCR 引擎等第三方离线安装包** + **部署说明文档** + 可选 `config.json` / `templates/`。运维一次拿到全部所需物料，按文档即可完成部署（详见 8.1 / 8.3） |
| 环境前置 ① 7-Zip | 目标机需**预先安装 7-Zip**（安装包由交付包提供）。工具启动即自检，**缺失时提示用户先安装**并以退出码 4 结束；**工具自身不代为安装** |
| 环境前置 ② OCR 引擎 | 目标机需**预先安装 OCR 引擎**（默认 PaddleOCR-json，绿色解压即用；包由交付包提供）。工具启动自检；缺失时**提示用户安装**并自动降级为「UIA + 模板」（默认不阻断流程） |
| 工具本体不携带第三方文件 | `AutoTrigger.exe` **自身不含**安装包、脚本、动态库、OCR 模型（第三方物料放在交付包的其他目录，**不嵌入 exe**）；同目录可放可选的 `config.json` 与 `templates/`（缺失时使用内嵌默认值） |
| 分发 | 交付包通过共享目录 / 移动介质拷贝到各沙箱节点；**工具本体也可单独拷贝给运维人员做快速验证** |
| 平台基线 | **Windows 7 SP1 x64 及以上**（Win7 无 UIA 增强与新 DPI API，见 4.6） |
| 架构 | 预留 Linux / macOS 扩展能力 |

### 2.4 非目标（明确不做）

- 不负责沙箱隔离与逃逸防护（运行环境已是沙箱）
- 不负责行为监控、日志采集、威胁判定（由 EDR 负责）
- 不负责样本静态分析、反混淆、脱壳
- 不负责网络隔离策略配置；安装器需联网下载组件时不做网络补偿，按超时处理并标记 `requires_network`
- **不自研、不内嵌文件解压实现**（解压能力统一由 7-Zip 提供）
- **不自研、不训练 OCR 模型**（识别能力统一由外部开源 OCR 引擎提供）
- **工具自身不代为安装环境前置依赖**（7-Zip、OCR 引擎）：只检测、只提示；**但交付包内提供这些依赖的离线安装包与部署说明文档**，由运维按文档手工安装（F16、8.1）
- **不引入 CGO、不链接任何原生 OCR SDK**（保持静态单文件；OCR 一律走进程外调用）
- 不追踪样本运行后释放的后续文件与链式行为（只负责"第一跳"，其余由 EDR 捕获）
- 不保证所有样本都能装成功（反沙箱、需联网激活、依赖缺失等由环境侧解决）
- 本期不实现 Linux / macOS，仅预留架构扩展点

---

## 3. 用户与使用场景

### 3.1 目标用户

- 供应链安全检测工程师
- 自动化检测流水线运维人员
- 镜像/环境构建人员（负责预装 7-Zip 与 OCR 引擎）

### 3.2 典型场景

| 场景 | 描述 |
|------|------|
| 静默安装成功 | 传入 `.exe` 安装器，工具轮询静默参数完成无人值守安装，产出 `install_completed` |
| 向导式安装 | 安装器只支持 GUI，工具识别弹窗按钮/协议勾选框并模拟点击，走完全流程 |
| 自绘界面安装 | 安装器无 UIA 控件树（Electron/CEF 类），图像模板未命中时由外部 OCR 引擎定位按钮文本像素位并点击 |
| 中文界面识别 | 中文安装向导：OCR 引擎（PaddleOCR）对中文界面识别率高，弥补 UIA 覆盖不足 |
| 老系统环境 | Windows 7 沙箱：无 Windows 内置 OCR，UIA 覆盖较弱，主要依赖图像模板 + 外部 OCR |
| 低配/无 AVX 环境 | CPU 不支持 AVX 时无法运行 PaddleOCR-json，改用 RapidOCR-json 引擎（配置切换） |
| 单文件触发 | 传入一个 `.exe`，多种方式依次尝试并记录每次结果 |
| 文件夹批量触发 | 传入一个目录，工具遍历所有文件，逐个识别并触发 |
| 压缩包递归触发 | 传入 `.zip` / `.iso`，工具用 7-Zip 解压后递归触发内部所有可执行文件 |
| 文档类触发 | 传入 `.docx` / `.pdf`，工具调用关联程序打开，触发宏/漏洞利用链 |
| 未知格式兜底 | 传入扩展名未知或文件头与扩展名不符的文件，工具仍尝试执行 |
| 前置缺失 | 目标机未装 7-Zip / OCR 引擎：工具检测后提示用户先安装，不尝试自行安装 |

### 3.3 使用方式

命令行调用为主，便于集成到自动化流水线：

```
AutoTrigger.exe --input "C:\samples\test.exe" --timeout 120 --output "C:\logs\result.json"
AutoTrigger.exe --input "C:\samples\folder" --recursive --timeout 120
AutoTrigger.exe --input "C:\samples" --config "C:\conf\config.json"
AutoTrigger.exe --dump-config > config.json   # 导出内嵌默认配置，作为配置模板
AutoTrigger.exe --check-prereq          # 仅做环境自检（7-Zip / OCR 引擎），不处理样本
AutoTrigger.exe --dump-ui --input "C:\samples\a.exe"   # 调试：导出窗口截图 + UIA 控件树 + OCR 结果，用于扩充模板
AutoTrigger.exe --no-archive --input "C:\samples"      # 无 7-Zip 时跳过压缩包，继续处理其余样本
AutoTrigger.exe --no-ocr --input "C:\samples"          # 无 OCR 引擎时强制仅用 UIA + 模板
```

参数优先级（由高到低）：**命令行参数 > `--config` 指定文件 > 同目录 `config.json` > 内嵌默认值**。合并与校验规则详见 **7.5.1**。

---

## 4. 跨平台架构设计要求

### 4.1 设计原则

工具从第一天起就按**平台无关核心 + 平台适配层**的方式组织，避免平台相关逻辑散落在业务代码中。接口由**使用方（core）定义**，适配层负责实现，核心层不 import 任何平台包。

### 4.2 分层结构

| 层级 | 职责 | 平台相关性 |
|------|------|-----------|
| Core（核心层） | 输入解析、任务调度、类型识别、参数探测、尝试编排、超时控制、结果输出 | 平台无关 |
| Ports（接口层） | 定义统一接口（见 4.3），随 core 包定义 | 平台无关接口 |
| Platform Adapter（平台适配层） | Windows 实现（本期）、Linux / macOS 实现（后期） | 平台相关 |
| Prereq（前置依赖层） | 7-Zip 与 OCR 引擎探测、能力自检与**用户提示** | 平台相关 |
| OCR（识别辅助层） | **外部 OCR 引擎适配**（进程外调用：PaddleOCR-json / RapidOCR-json / Umi-OCR / tesseract）；可选回退到系统内置 OCR | 平台相关 |
| CLI（入口层） | 命令行解析、依赖装配、调用 Core | 平台无关 |
| Embedded Resources（内嵌资源） | 魔数表、静默参数表、按钮关键词表、图像模板、默认配置 | 平台无关 |

### 4.3 接口定义（Go 接口，由 core/ports 包定义）

```
type ProcessRunner interface {
    Start(cmd Command, timeout time.Duration) (ProcessHandle, error)
    KillTree(handle ProcessHandle) error          // Windows: Job Object / taskkill /T /F
    WaitForExit(handle ProcessHandle, timeout time.Duration) (ExitResult, error)
}

type FileOpener interface {
    OpenWithDefaultApp(path string) (ProcessHandle, error)
    HasAssociation(path string) bool              // 无关联程序返回 false，供 F7 跳过
}

type ArchiveExtractor interface {
    Extract(archivePath, destDir string, limits Limits) (ExtractResult, error)  // 后端为外部 7-Zip
    SupportedExtensions() []string
}

type FileTypeDetector interface {
    Detect(path string) (FileType, error)
    MagicNumberMap() map[string]FileType          // 内嵌，平台无关
}

type InstallerTrigger interface {
    TriggerCommands(path string, ft FileType) []TriggerCommand
}

type ConfigProvider interface {
    Load(explicitPath string) (Config, error)     // 默认值 < 文件 < 命令行
}

// Control 为统一控件模型，含：
//   ID / Text / Role / BoundingBox(像素) / Source(uia|template|ocr) / Confidence
type GUIAutomator interface {
    WatchWindows(pidTree []int, interval time.Duration) ([]Window, error)
    FindControls(w Window) ([]Control, error)                    // 一级：UIA 控件树
    MatchByTemplate(w Window, templates []Template) ([]Control, error)  // 二级：图像模板匹配
    LocateByOCR(w Window, keywords []string) ([]Control, error)  // 三级：外部 OCR 文本定位（辅助）
    Click(c Control) error
    SetText(c Control, text string) error
}

// OcrEngine —— v2.4：外部 OCR 引擎的进程外适配（禁止 CGO）
type OcrEngine interface {
    Name() string                                 // paddleocr_json | rapidocr_json | umi_ocr | tesseract | windows | none
    Probe() (OcrInfo, error)                      // 探测：可执行文件路径、版本、是否支持 AVX、可用语言
    Start(ctx context.Context) error              // 启动常驻引擎进程（避免每次识别重复初始化）
    Stop() error                                  // 任务结束回收进程
    Recognize(img Image) ([]OcrText, error)       // 返回 文本 + 四角像素坐标 + 原生 score
    HealthCheck() error                           // 卡死/崩溃探测，供自动重启
}
// OcrText: Text string / Box [4]Point（四角） / Score float64 / Center Point

type InstallVerifier interface {
    Verify(ctx InstallContext) (*InstallEvidence, error)  // 四类证据，见 F15/F14.4
}

type PrereqManager interface {
    CheckPrereqs() (PrereqReport, error)          // 仅检测，不安装
    MissingHints() []string                       // 缺失项的用户提示与安装指引（供 CLI 打印）
}
```

### 4.4 平台适配矩阵

| 能力 | Windows（本期） | Linux（后期） | macOS（后期） |
|------|----------------|--------------|--------------|
| 进程执行 | CreateProcess / Start-Process（Win7 可用） | fork+exec | posix_spawn |
| 进程树终止 | Job Object / taskkill /T | kill process group | kill process group |
| 默认程序打开 | ShellExecute | xdg-open | open |
| 解压 | **外部 7z.exe（环境需预置）** | 7z / tar | 7z / tar |
| 文件头识别 | 魔数表（平台无关，内嵌） | 魔数表 | 魔数表 |
| 安装包触发 | msiexec / 静默参数 | dpkg / rpm / sh | pkg / dmg |
| 脚本触发 | cmd / powershell / wscript | bash / sh / python | bash / zsh / osascript |
| GUI 自动化 | UIA（Win7 为 UIAutomationCore 基础版）+ SendInput + 截图模板匹配 + **外部 OCR** | xdotool（后期） | AppleScript（后期） |
| OCR 引擎 | **外部开源引擎进程（PaddleOCR-json / RapidOCR-json / Umi-OCR / tesseract）**；可选回退 Windows 内置 OCR（仅 Win10 1809+） | 同左（tesseract / PaddleOCR） | 同左 |
| 截屏 | GDI `BitBlt` / `PrintWindow`（后者 Vista+ 可用） | X11 | CGWindowListCreateImage |
| DPI 感知 | Win10 1607+：`GetDpiForWindow`；**Win7/8.1 回退 `GetDeviceCaps(LOGPIXELSX)` + `SetProcessDPIAware`** | — | — |
| 安装证据采集 | 文件系统 + 注册表 | 包管理器数据库 | pkgutil |

### 4.5 跨平台扩展约束

- 核心层不得直接调用任何 Windows API，适配层之外的包不得 import `internal/platform/windows`
- 所有平台相关行为必须通过 4.3 接口调用
- 参数探测表、类型映射表、按钮关键词表、图像模板按平台组织，优先内嵌、支持外部覆盖
- **OCR 引擎按平台适配**：core 只依赖 `OcrEngine` 接口，不感知具体引擎；引擎的探测、启动、通信、回收全部封装在 `internal/ocr` 内
- 结果 JSON 结构保持平台无关，平台特有字段放 `platform_meta`

### 4.6 Windows 7 兼容性约束（v2.4 新增，重要）

平台基线定为 **Windows 7 SP1 x64 及以上**，带来以下硬性约束：

| 约束项 | 说明 | 对策 |
|--------|------|------|
| **Go 工具链** | **Go 1.21+ 已放弃 Win7/8/8.1 支持**（要求 Win10 / Server 2016+）；**Go 1.20.x 是最后一个支持 Win7 的版本** | 构建环境固定 Go 1.20.x；`go.mod` 声明 `go 1.20`；**引入任何第三方库前须确认其最低 Go 版本要求 ≤ 1.20** |
| UIA 能力 | Win7 提供 `UIAutomationCore`（随 .NET 3.0+），对标准 Win32 控件可读；**WPF / 新式框架覆盖度明显弱于 Win10**，部分属性/模式缺失 | UIA 命中失败时**更早**回落到图像模板与 OCR；不依赖 Win10 专有 UIA 特性；UIA 调用全部包在能力探测里 |
| DPI API | Win7 无 `GetDpiForWindow`（Win10 1607+）、无 `SetProcessDpiAwarenessContext`（Win10 1703+） | 运行时按系统版本分支：优先新 API，回退 `GetDeviceCaps(LOGPIXELSX)` 取 DPI + `SetProcessDPIAware`（Vista+ 可用） |
| 截屏 | `PrintWindow` 在 Vista+ 可用；`PW_RENDERFULLCONTENT` 标志为 Win8.1+ | 按版本选择标志；失败时回退 `BitBlt` |
| 进程树终止 | Job Object 自 Vista 起可用 | 无需回退 |
| 运行库 | OCR 引擎（PaddleOCR-json / Umi-OCR）可能依赖 **VC++ 2015-2022 运行库** | 列入镜像前置检查表（F16.5）；若缺失需镜像预置 |
| 哈希 | SHA-256 需 Win7 SP1 及以上 | 平台基线明确为 **SP1**；启动时校验系统版本，低于 SP1 输出告警 |
| 实测要求 | Win7 与 Win10 的 UIA / 截屏 / DPI 行为差异较大 | **必须准备 Win7 x64 实测环境**，验收 A20 在 Win7 上执行 |

### 4.7 目标机运行时依赖边界（v2.4 新增，v2.5 补充交付包对应关系，部署必读）

**结论一句话**：**"免安装"指工具本体不需要安装、不需要语言运行时；但目标虚拟机仍须满足若干功能性前置条件。** 二者不可等同，分发前须按本节逐项核对。

| 类别 | 具体项 | 是否必需 | 缺失后果 | 工具行为 |
|------|--------|---------|---------|---------|
| **A. 语言运行时** | Go 运行时 / .NET / JRE | **完全不需要** | — | `CGO_ENABLED=0` 静态编译，单文件自包含 |
| **A. 系统动态库** | `kernel32` / `user32` / `advapi32` / `ole32` / `shell32` / `gdi32` / `userenv` 等 | 系统自带（XP 起即存在） | 无法启动 | 无需任何处理；**导入表只应出现系统 DLL**，不得出现第三方 DLL |
| **B. 功能性前置** | **7-Zip** | 处理压缩包样本时必需 | 压缩包无法解压 | 检测 + 提示 + 退出码 4；`--no-archive` 可绕过（F16.1/F16.2） |
| **B. 功能性前置** | **OCR 引擎**（默认 PaddleOCR-json） | 建议必需（自绘界面依赖） | 降级为 UIA + 模板，识别率下降 | 检测 + 提示 + 自动降级；`require_ocr: true` 时退出码 5（F16.3） |
| **B. 功能性前置** | **VC++ 2015-2022 运行库** | 取决于所选 OCR 引擎 | OCR 引擎进程无法启动 | 能力自检失败即视为 OCR 不可用并降级（F16.5） |
| **C. 镜像准备项** | **管理员权限** | 必需 | 静默安装、MSI、注册表证据采集失败 | 启动自检权限，告警并继续（第 6 节） |
| **C. 镜像准备项** | **关闭 UAC 安全桌面** | GUI 自动化场景必需 | 提权弹窗无法自动化，安装链中断 | 检测到提权窗口记 `uac_blocked`（F14.5） |
| **C. 镜像准备项** | **AV / EDR 排除目录** | 强烈建议 | 未签名静态单文件易被拦截或直接删除，工具无法运行 | 工具无法自解；由 README 与镜像检查表要求（F16.5） |
| **C. 镜像准备项** | **可写临时目录** | 必需 | 解压与样本落地失败 | 优先 `%TEMP%`，不可写则回退工具同级目录并告警 |
| **D. CPU 能力** | AVX（仅 `paddleocr_json`） | 视引擎而定 | 引擎无法启动 | 启动自检 + 提示切换 `rapidocr_json`，或降级 UIA + 模板（A21） |
| **D. 架构** | x64（`GOARCH=amd64`） | 必需 | 32 位系统无法运行 | 本期只出 amd64；如需 32 位另出 `GOARCH=386` 构建（不在本期范围） |

**构建期硬性校验（防止"意外破坏零依赖"）**

1. 构建命令必须显式含 `CGO_ENABLED=0`；**任何引入 CGO 的第三方依赖一律拒绝**（CI 断言该环境变量）
2. 产物须核验 PE **导入表只含系统 DLL**——`dumpbin /dependents AutoTrigger.exe`（或 PowerShell 读 PE 导入表）中一旦出现 `vcruntime*.dll` / `msvcp*.dll` / 其他第三方 DLL，即视为**构建失败**
3. 禁止 `-buildmode=c-shared`、禁止 `plugin`、禁止运行期释放 DLL
4. 在**干净镜像**（无 Go、无 VC++ 运行库、无网络、未装任何运行时）冒烟一次——对应验收 A14 / A22

> **职责划分**：A 类无需任何操作；B 类的**离线安装包随交付包提供**，由运维按部署说明文档安装，工具侧由 F16 负责检测与提示；C 类属镜像构建职责，建议在镜像构建流程末尾以 `AutoTrigger.exe --check-prereq` 作卡点校验，通过后再批量分发。
>
> **交付物对应关系**：工具本体（`AutoTrigger.exe`）只解决 A 类；**B 类与 C 类所需的物料和操作说明全部由交付包承载**——见 8.1 交付包结构与 8.3 交付物清单。

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
| F6 | 压缩包递归处理（7-Zip 后端） | P0 | 适配层 | M2 |
| F7 | 文档类触发 | P1 | 适配层 | M2 |
| F8 | 未知格式兜底触发 | P1 | 适配层 | M2 |
| F9 | 超时与进程清理 | P0 | 适配层 | M1 |
| F10 | 执行结果记录与输出 | P0 | 无关 | M1 |
| F11 | 文件夹批量遍历 | P0 | 无关 | M2 |
| F12 | 嵌套/链式触发 | P1 | 无关 | M2 |
| F13 | 参数探测与多轮尝试机制 | P0 | 无关 | M1 |
| F14 | GUI 弹窗识别与键鼠模拟（含外部 OCR 辅助） | P0 | 适配层 | M2 |
| F15 | 安装结果验证与证据采集 | P0 | 适配层 | M2 |
| F16 | 前置依赖自检与用户提示（7-Zip / OCR 引擎） | P0 | 适配层 | M1 |

---

### 5.2 功能详细说明

#### F1 输入接收与校验

- 支持 `--input` 指定单个文件或文件夹路径
- 支持 `--recursive` 递归遍历文件夹
- 支持 `--timeout` 指定单个文件的最大触发时长（秒），默认 120
- 支持 `--output` 指定结果输出路径（JSON）
- 支持 `--config` 指定外部配置文件路径（覆盖内嵌默认值，见 7.5）
- 支持 `--check-prereq` 仅执行环境自检、`--dump-config` 导出生效配置、`--no-archive` 跳过压缩包处理、`--no-ocr` 跳过 OCR
- 外部配置的加载、合并与校验规则见 **7.5.1**（深合并、未知字段忽略、类型/越界以退出码 3 失败）
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

**内嵌要求**：魔数表随 exe 内嵌（`go:embed`），不依赖 libmagic 等原生库。

#### F3 PE 可执行文件触发

**触发顺序（安装成功优先）**：

1. **静默参数轮询**（见 F13）—— 最可能无人值守完成安装
2. **无参数直接运行** —— 触发安装向导 GUI，由 F14 识别弹窗并模拟键鼠推进
3. 上述每条均为独立尝试，尽量多试；全部尝试期间 F14 持续监控弹窗
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
| .ps1 | `powershell.exe -ExecutionPolicy Bypass -NoProfile -File "文件"`（Win7 默认 PS 2.0，指令兼容 2.0） |
| .vbs / .js | `wscript.exe "文件"`；若 30 秒内疑似弹 GUI，改 `cscript.exe "文件"` 重试一次 |
| .wsf | `wscript.exe "文件"` |

要求：脚本执行同样受超时控制；解释器映射通过配置加载，不硬编码在 Core 层。

#### F6 压缩包递归处理（7-Zip 后端）

1. **解压后端为外部 7-Zip 命令行**（`7z x -o<destDir> -y "<archive>"`），可用性由 F16 保证；**不自研、不内嵌解压实现，也不做运行时释放 7z.exe 的降级方案**
2. 解压到临时目录（目录名含任务 ID），遍历解压后所有文件
3. 对每个文件重新走 F2 类型识别 → 对应触发逻辑
4. **递归深度限制：默认 3 层（可配置）**，防止无限嵌套
5. **解压后总大小限制：默认 2 GB（可配置）**，超限停止并记录 `extract_size_exceeded`（防 zip 炸弹）
6. 解压失败时记录 `extract_failed` 与 7z 输出摘要，不阻塞后续文件
7. 退出前清理临时目录（清理失败仅告警）
8. `--no-archive` 或 7-Zip 缺失时跳过压缩包处理，相关文件记 `skipped: prereq_missing`（见 F16）

#### F7 文档类触发

- 通过 `FileOpener.HasAssociation` 判断关联程序：无则记录 `skipped: no_host_app` 并跳过
- 有关联则打开文件，由预装 Office / PDF 阅读器触发宏或漏洞链；若弹出"启用宏/允许编辑"类确认框，由 F14 接管点击
- 超时后强制结束关联进程树；记录关联程序进程名到 `platform_meta`

#### F8 未知格式兜底触发

按以下顺序尝试：

1. 文件头为 `MZ` → 按 PE 执行（F3）
2. 文件头为 ZIP/OLE → 按压缩包（F6）或文档（F7）处理
3. 否则尝试 `OpenWithDefaultApp`（依赖系统关联）
4. 全部失败记 `failed: untriggerable`，不阻塞

**设计原则：宁可误触发，不可漏触发。**

#### F9 超时与进程清理

- 每个文件的触发流程有独立超时（默认 120 秒，可配置）
- 超时后：记录当前阶段与已尝试参数 → 结束该文件及其派生进程树 → 该次尝试记 `killed_on_timeout`（属"已触发"）→ 继续下一个文件
- 工具退出前清理所有临时解压目录，并回收常驻 OCR 引擎进程
- **全局看门狗**：单文件累计时长不得超过 `timeout × 尝试次数上限 + 60s`，防止挂死阻塞批量任务
- GUI 弹窗处理与 OCR 识别耗时计入单文件总超时

#### F10 执行结果记录与输出

输出 JSON：任务级汇总 + 文件明细数组，仅写本地文件。

```json
{
  "schema_version": "2.5",
  "task_id": "uuid",
  "config_digest": "sha256:...",
  "started_at": "2026-09-30T09:40:00+08:00",
  "finished_at": "2026-09-30T09:52:31+08:00",
  "platform": "windows",
  "platform_meta": { "os_version": "6.1.7601", "dpi_awareness": "system", "cpu_avx": true },
  "prereq": {
    "archive_backend": "C:\\Program Files\\7-Zip\\7z.exe",
    "sevenzip_version": "23.01",
    "archive_backend_status": "ok",
    "ocr_engine": "paddleocr_json",
    "ocr_engine_path": "C:\\tools\\PaddleOCR-json\\PaddleOCR-json.exe",
    "ocr_engine_status": "ok",
    "ocr_engine_languages": ["ch", "en"],
    "ocr_startup_ms": 680
  },
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
            { "window": "Setup - Test", "control_text": "我接受", "source": "uia", "action": "click", "at": "2026-09-30T09:41:02+08:00" },
            { "window": "Setup - Test", "control_text": "安装", "source": "template", "action": "click", "at": "2026-09-30T09:41:15+08:00" },
            { "window": "Setup - Test", "control_text": "下一步", "source": "ocr", "ocr_score": 0.94, "recognize_ms": 210, "action": "click", "at": "2026-09-30T09:41:22+08:00" }
          ]
        }
      ],
      "final_status": "install_completed",
      "notes": ""
    }
  ]
}
```

**final_status 枚举**：

| 取值 | 定义 | 汇总归类 |
|------|------|---------|
| `install_completed` | 出现 F15/F14.4 任一安装成功证据 | installed |
| `triggered` | 进程成功启动并运行结束，但无安装成功证据（非安装类样本的正常结果） | triggered |
| `killed_on_timeout` | 进程成功启动但被超时终结 —— 行为已被 EDR 采集，视为有效触发 | triggered（另计 timeout） |
| `failed` | 所有方式均无法启动进程 | failed |
| `skipped` | 无可用触发方式（空文件 / 文档无宿主程序 / 前置依赖缺失等） | skipped |

> 说明：v2.0 起将 `killed_on_timeout` 并入触发成功口径；v2.1 新增 `install_completed`；v2.3 起 `gui_actions` 记录识别来源 `source`；**v2.4 起记录 OCR 原生 `ocr_score` 与识别耗时，并把 `prereq` 扩展为完整的引擎探测结果**。

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
4. 每次尝试间隔 2 秒（可配置）；单文件最大尝试次数可配置
5. 任一尝试取得安装成功证据即提前结束该文件后续尝试
6. 参数表按平台组织，优先内嵌、支持外部覆盖

#### F14 GUI 弹窗识别与键鼠模拟

**目标**：安装向导弹窗出现时，自动识别安装相关按钮/输入项并模拟键鼠，驱动向导走到完成。

**F14.1 窗口监控**

- 每次触发尝试期间，后台协程轮询该进程树的可见顶层窗口（默认每 500ms 一次，可配置）
- 发现新窗口即进入控件识别流程；窗口关闭后回到监控状态

**F14.2 控件识别（三级策略）**

| 级别 | 手段 | 适用场景 | 说明 |
|------|------|---------|------|
| 一级 | **UI Automation（UIA）** | 标准 Win32 / WPF / WinForms 向导 | 读取控件树，取文本、AutomationId、屏幕坐标与角色，语义最准确。**Win7 上覆盖度弱于 Win10**，失败更早回落 |
| 二级 | **图像模板匹配** | 已知安装器家族、UIA 缺失 | 截图 + 按钮模板匹配定位；模板集内嵌、支持外部热更新 |
| 三级 | **外部 OCR 引擎文本定位（辅助）** | 自绘界面、模板未命中、未知安装器、**Win7 环境** | 离线识别窗口内文本及其**像素包围盒**，按语义关键词定位可点击项（详见 F14.6） |

**融合与仲裁规则**

1. **优先级**：UIA 命中 > 模板命中 > OCR 命中；同级内按语义优先级（如"安装"优先于"下一步"）与置信度排序
2. **OCR 引导模板匹配**：OCR 先给出候选文本包围盒，在其外扩 ROI（默认 ±12px）内做模板精修，既提速又提高准确率
3. **串行降级**：某级在合理时间内未取到候选时，才进入下一级，避免无谓开销
4. **Win7 权重调整**：平台基线含 Win7 时，UIA 覆盖度较低，**模板与 OCR 的权重相对提高**（实现上体现为 UIA 更早回落、模板匹配范围放宽）
5. 全部级别均不可用（无 UIA 树、无模板、OCR 未启用或引擎缺失）→ 记录 `gui_identify_failed`，按超时处理

**图像模板初始覆盖清单（建议）**

| 梯队 | 安装器家族 | 覆盖策略 |
|------|-----------|---------|
| 第一梯队（必做） | Inno Setup、NSIS、InstallShield、Windows Installer 标准 UI（MSI）、WiX Burn | Win10 上 UIA 基本可覆盖；**Win7 上须实际验证**，模板各存 2~3 张关键按钮 |
| 第二梯队（建议做） | Advanced Installer、InstallAware、Setup Factory、Wise Installation Studio、Smart Install Maker | 老式向导自绘比例高，需模板支撑 |
| 第三梯队（按需） | Electron/CEF 自绘安装器（多家国产软件）、游戏启动器型安装器 | 基本无 UIA 树，**以模板 + OCR 协同识别为主** |

**初始模板集建议为 8~12 张**：三大主流向导的「下一步 / 安装 / 完成 / 我接受（协议）」按钮各 1~2 张，加上第二梯队 2~3 个代表性安装器的关键按钮。模板按"安装器家族 + 界面语言"命名，避免重复。

**模板库迭代机制**

- 工具提供 `--dump-ui` 调试模式：输出窗口截图 + UIA 控件树 JSON + **OCR 识别结果（含四角坐标与 score）**，供人工标注后补充模板
- 模板目录支持外部覆盖（`gui_automation.template_dir`），便于现场热更新，无需重新发版
- 一次识别失败不视为永久失败，下次任务仍会重试匹配

**F14.3 控件语义判定（关键词表内嵌，中英双语起步）**

| 类别 | 关键词示例 | 动作 |
|------|-----------|------|
| 推进类（白名单） | 下一步 / 安装 / 是 / 确定 / 我接受 / Next / Install / Yes / OK / Agree | 点击 |
| 完成类 | 完成 / Finish / Done | 点击后本窗口流程结束 |
| 协议复选框 | 我接受协议 / I accept | 勾选 |
| 路径输入框 | 安装路径 / Destination Folder | 保留默认路径，异常时恢复默认值 |
| 拒绝类（黑名单，禁止点击） | 取消 / 退出 / 否 / 卸载 / Cancel / Exit / No / Uninstall | 不点击，仅记录 |

- 同屏多个推进类按钮时按预置优先级选取（如"安装"优先于"下一步"）
- 点击坐标加入微小随机偏移（`click_jitter_px`），避免固定坐标点击被反自动化识别
- 单窗口内同类动作设置次数上限，防止死循环点击
- **文本归一化**：UIA 名称、模板名、OCR 文本统一按"去首尾空白 → 去除助记符 `&` → 全角转半角 → 折叠连续空白 → 大小写不敏感"后参与关键词匹配

**F14.4 安装成功判定证据（满足任一即成功）**

1. 安装器进程退出码为 0（MSI `3010` 亦视为成功）
2. 出现并点击了"完成"向导页
3. 文件系统证据：安装期间 Program Files / Program Files (x86) / 自定义安装目录出现新增文件
4. 注册表证据：`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall` 下新增卸载项

**F14.5 边界与约束**

- UAC 安全桌面无法自动化：沙箱镜像应关闭 UAC 弹窗或以最高权限运行（见第 11 节风险）
- 全部键鼠动作写入 `gui_actions`（窗口标题 / 控件文本 / 识别来源 / 动作 / 时间戳），可回放审计
- 安装器进入"正在下载组件"状态（离线环境必然卡住）：记录 `requires_network`，按超时处理
- 批量模式下需保证前台桌面可用（GUI 自动化依赖真实桌面会话）
- **高 DPI 缩放**：截图得到的像素坐标与系统逻辑坐标须按窗口 DPI 比例换算；**Win10 1607+ 用 `GetDpiForWindow`，Win7/8.1 回退 `GetDeviceCaps(LOGPIXELSX)` + `SetProcessDPIAware`**；点击前校验目标坐标落在窗口矩形内
- **OCR 使用边界**：仅在 UIA 与模板均未命中时启用；OCR 属于辅助手段，其失效不得导致整体流程失败

**F14.6 外部 OCR 引擎辅助定位（v2.4 重写，P0）**

**定位目标**：窗口内按钮、复选框、输入框标签等**可见文本**，据此推算可点击像素位置。

**引擎选型（默认 PaddleOCR-json）**

| 引擎 | 项目 / 许可 | Win7 | 优缺点 | 配置值 |
|------|------------|------|--------|--------|
| **`paddleocr_json`（默认）** | PaddleOCR-json（开源） | ✅ | **首选**：基于 PaddleOCR C++ 版（PP-OCRv3/v4），中英日韩识别率高；**绿色解压即用、零依赖**；输出 JSON 含**四角坐标 + 原生 score**；支持 stdio 与 TCP；初始化约 0.6s。**需 CPU 支持 AVX 指令集** | `paddleocr_json` |
| `rapidocr_json` | RapidOCR-json（开源） | ✅ | 姊妹项目，**不需要 AVX**，老 CPU / 虚拟化环境适用；速度中等；接口与 PaddleOCR-json 一致，可无缝切换 | `rapidocr_json` |
| `umi_ocr` | Umi-OCR（MIT） | ✅（x64） | 完整 GUI 软件（内置 PaddleOCR-json / RapidOCR-json 引擎，可切换），体积较大（约 500MB）；提供**命令行调用**与 **HTTP 接口**（`127.0.0.1:1224/api/ocr`）；解压即用、完全离线 | `umi_ocr` |
| `tesseract` | Tesseract（Apache-2.0） | ✅ | 兼容性最广、有正式安装包；**中文与复杂排版识别率明显弱于 PaddleOCR**，界面小字号文本需图像预处理（放大、二值化） | `tesseract` |
| `windows`（可选回退） | Windows.Media.Ocr（系统内置） | ❌ | 零外部依赖，**但仅 Win10 1809+ 可用**；实测为单语言引擎且语言包受限（见下方实测记录）；**默认关闭** | `windows` |
| `none` | — | — | 关闭 OCR，仅用 UIA + 模板（等价于 `--no-ocr`） | `none` |

**推荐部署**：镜像预置 **PaddleOCR-json**（首选）；若目标机 CPU 无 AVX 或为老式虚拟机，追加 **RapidOCR-json**；两者均无时退化为 UIA + 模板。

**v2.3 实测记录（保留下来的重要背景）**

v2.3 曾采用 Windows 内置 OCR，实测（2026-09-30）发现：

| 探测项 | 实测结果 | 结论 |
|--------|---------|------|
| `OcrEngine.AvailableRecognizerLanguages` | 仅 `zh-Hans-CN`，Count = 1 | 引擎实际只支持中文 |
| `TryCreateFromLanguage('en-US')` | **null（不可用）** | 该环境无法识别英文界面 |
| `MaxImageDimension` | 10000 | 4K 截图无压力 |

→ 内置 OCR **受语言包限制且为单语言引擎，Win7 上根本不存在**，故 v2.4 改为外部开源引擎为主，内置 OCR 降为可选回退。

**外部引擎调用方式（进程外，禁止 CGO）**

1. **stdio 模式（默认）**：由工具启动引擎子进程，通过 **stdin 发送 JSON 请求、stdout 读取 JSON 结果**，请求示例 `{"image_path": "C:\\tmp\\shot.png"}` 或 Base64 图像
2. **TCP 模式（可选）**：启动时传 `-port=<端口> -addr=loopback`，工具通过环回 TCP 请求（适合需要复用的场景）
3. **HTTP 模式（仅 Umi-OCR）**：POST `http://127.0.0.1:1224/api/ocr`
4. 引擎进程**常驻复用**（初始化一次，任务期间反复识别）；任务结束或工具退出时回收
5. 引擎返回结构（PaddleOCR-json 系）：
   ```json
   {"code":100,"data":[{"box":[[13,5],[161,5],[161,27],[13,27]],"score":0.9996,"text":"安装"}]}
   ```
   `box` 为**四角像素坐标**，中心点即 `((x1+x3)/2, (y1+y3)/2)`，可直接作为点击坐标来源

**识别与点击流程**

1. 仅截取**活动窗口客户区**（缩小范围、降低噪声），按 DPI 换算像素后写入临时 PNG（或转 Base64）
2. 调用 `OcrEngine.Recognize` 获取「文本 + 四角坐标 + score」列表
3. **先按 `min_score`（默认 0.6）过滤低置信结果**（引擎原生 score，比启发式可靠）
4. 文本按 F14.3 规则归一化后与关键词表比对，判定语义类别
5. **白名单/推进类命中**：取包围盒 → 外扩 `roi_padding_px` 形成 ROI → 优先在 ROI 内做模板精修（命中则用模板中心）；未命中模板则以包围盒中心为点击点
6. **黑名单命中**：仅记录，绝不点击（黑名单优先级高于一切）
7. 最终点击点 = 目标中心 + 随机偏移（`click_jitter_px`），落点须在窗口矩形内
8. 命中记录写入 `gui_actions`：`source: "ocr"`、`ocr_score`、`recognize_ms`

**置信度与仲裁**

- 主判据为**引擎原生 `score`**，配合文本**完整词匹配**（非子串模糊命中）
- 包围盒合理性校验：宽高须在合理范围（文字行高 `min_text_height_px`~`max_text_height_px`，默认 10~80px）
- 同窗多候选：先比语义优先级，再比 score
- **界面语种与引擎语言不匹配时**（如中文界面用纯英文模型识别），结果仅记录、**不触发点击**

**性能与缓存**

- 单次识别目标耗时 ≤ 800ms（含截屏与进程通信）；对同一窗口截图做哈希，画面未变化时复用上次结果
- 引擎初始化耗时在结果 JSON 的 `prereq.ocr_startup_ms` 中记录，便于性能回归
- OCR 与 UIA/模板的识别流程串行执行，避免同时占用前台资源影响样本行为

**降级与容错**

- 引擎不存在 / 启动失败 / 健康检查失败 → 记录 `ocr_unavailable`，自动降级为「UIA + 模板」，**不中断流程**（`require_ocr: true` 时除外，见 F16.3）
- 引擎进程卡死或崩溃 → 按 `restart_on_failure` 自动重启（上限可配），超限则本次任务内禁用 OCR
- 单次识别超时（`recognize_timeout_ms`，默认 3000ms）→ 放弃本次识别，不阻塞 GUI 流程
- 同一窗口 OCR 触发动作次数计入 `max_actions_per_window`，防止误识别导致重复点击

#### F15 安装结果验证与证据采集

- 每次尝试结束后调用 `InstallVerifier.Verify`，采集四类证据（同 F14.4）
- 证据写入对应 attempt 的 `install_evidence`，并决定 `final_status` 是否升级为 `install_completed`
- 未取得证据但进程正常运行结束 → `triggered`；仅被超时终结 → `killed_on_timeout`
- 采集应在安装器退出后再等待一个稳定期（默认 3 秒，可配置），避免写入延迟导致误判

#### F16 前置依赖自检与用户提示（7-Zip / OCR 引擎）

**定位**：**工具本体为免安装单文件，自身不代为安装任何环境前置依赖**。前置依赖由镜像构建阶段或运维负责预置；**其离线安装包与安装步骤由交付包提供**（见 8.1 / 8.3），工具只做"检测 → 提示（指向交付包内安装包）→ 明确退出/降级"。

**F16.1 7-Zip 检测**

- **探测顺序**：配置指定路径（`archive.sevenzip_path`，非 `auto` 时优先）→ 注册表 `HKLM\SOFTWARE\7-Zip\Path`（含 `Wow6432Node`）→ `PATH` 查找 → 常见默认路径（`C:\Program Files\7-Zip\7z.exe`、`C:\Program Files (x86)\7-Zip\7z.exe`）
- 命中后记录路径与版本到结果 JSON 的 `prereq` 字段

**F16.2 7-Zip 缺失时的行为**

1. **不静默降级**（不内嵌解压实现）、**不代为安装**（不调用任何安装包）
2. 在 stdout 与日志输出**明确的用户提示**，内容至少包含：缺失项、影响范围、安装方式（含官方离线包文件名与静默安装命令示例）、绕过方式
3. **输入中不含压缩包** → 继续正常处理其余样本，仅在 `prereq.archive_backend_status` 标记 `missing` 并在汇总处告警
4. **输入中含压缩包** → 相关文件记 `skipped: prereq_missing`；若非 `--no-archive`，任务结束时以**退出码 4** 结束
5. `--no-archive` → 显式跳过压缩包处理，继续跑其余样本，不报错

**提示文案要求（示例）**

```
[ERROR] 前置依赖缺失：未检测到 7-Zip
        影响：压缩包类样本（zip / 7z / rar / tar / gz / iso 等）无法解压处理
        解决方式（离线环境，任选其一）：
          0) 使用交付包内的离线安装包（推荐，无需联网）：
             <交付包根目录>\third_party\7zip\7z<版本>-x64.msi
          1) 以管理员身份运行离线安装包：msiexec /i 7z<版本>-x64.msi /quiet /norestart
          2) 运行离线安装包：7z<版本>-x64.exe /S
          3) 已解压的绿色版：将 7z.exe 目录加入 PATH，或在 config.json 中设置 archive.sevenzip_path
        临时绕过（跳过压缩包，继续处理其余样本）：AutoTrigger.exe --no-archive ...
        仅检查环境：AutoTrigger.exe --check-prereq
```

**F16.3 OCR 引擎检测（v2.4 重写，P0）**

- **探测方式**：
  1. 配置指定路径（`ocr.engine_path`，非 `auto` 时优先）
  2. `PATH` 查找引擎可执行文件名（`PaddleOCR-json.exe` / `RapidOCR-json.exe` / `Umi-OCR.exe` / `tesseract.exe`）
  3. 常见默认路径（如 `C:\tools\PaddleOCR-json\`、`C:\Program Files\Tesseract-OCR\`）
  4. 命中后**实际启动一次**做能力自检（取版本、语言、初始化耗时），而非仅看文件存在
- **能力自检项**：
  - 引擎可执行文件路径与版本
  - 语言支持（PaddleOCR 系默认含 `ch` / `en`）
  - **AVX 指令集检测**（`paddleocr_json` 必需）：CPU 不支持时给出"请改用 `rapidocr_json`"的明确提示
  - 初始化耗时（`prereq.ocr_startup_ms`）
- **缺失时的行为**：
  - 默认（`require_ocr: false`）：**提示用户安装 + 自动降级**为「UIA + 模板」，记 `prereq.ocr_engine_status: missing`，**不影响退出码**
  - `require_ocr: true`：视为必需前置，缺失时以**退出码 5** 结束
- **提示文案要求（示例）**

```
[WARN] 前置依赖缺失：未检测到 OCR 引擎
       影响：无 UIA 控件树的自绘安装器界面无法通过 OCR 辅助定位，将仅使用 UIA + 图像模板
       解决方式（离线环境，任选其一）：
         0) 使用交付包内的 OCR 引擎包（推荐，无需联网）：
            解压 <交付包根目录>\third_party\ocr\PaddleOCR-json_v<版本>.7z 到任意目录（绿色免安装），
            然后在 config.json 中设置 ocr.engine_path 指向 PaddleOCR-json.exe
         1) 解压 PaddleOCR-json 发行包到任意目录（绿色免安装），
            然后在 config.json 中设置 ocr.engine_path 指向 PaddleOCR-json.exe
         2) 将解压目录加入 PATH
         3) 若 CPU 不支持 AVX，请改用 RapidOCR-json 并设置 ocr.engine = "rapidocr_json"
       临时绕过（强制仅用 UIA + 模板）：AutoTrigger.exe --no-ocr ...
       仅检查环境：AutoTrigger.exe --check-prereq
```

- **部署建议**：OCR 引擎与 7-Zip 一并纳入镜像前置检查表（F16.5）；`--check-prereq` 输出引擎类型、路径、版本、语言与 AVX 支持情况
- **参考工具**：`scripts/verify-ocr.ps1`（v2.3 用于验证 Windows 内置 OCR；**M1 阶段需扩展为多引擎探测与端到端验证**）

**F16.4 `--check-prereq`**

- 单独执行环境自检并输出人类可读报告（供镜像构建/部署前验证），不含样本处理
- 报告内容：系统版本与 SP、CPU AVX、7-Zip 路径与版本、OCR 引擎类型/路径/版本/语言/初始化耗时
- 全部满足 → 退出码 0；7-Zip 缺失 → 退出码 4；OCR 引擎缺失且 `require_ocr: true` → 退出码 5

**F16.5 前置依赖清单（部署检查表）**

| 依赖 | 必需性 | 缺失后果 | 检测/处理 |
|------|--------|---------|----------|
| 7-Zip | 必需（除非 `--no-archive`） | 压缩包样本无法处理；含压缩包时退出码 4 | F16.1 / F16.2 |
| OCR 引擎（`paddleocr_json` 默认） | 建议必需（否则 OCR 降级） | 自绘界面无法 OCR 定位，降级为 UIA + 模板 | F16.3 |
| CPU AVX 指令集 | 仅 `paddleocr_json` 需要 | 引擎无法启动，需改用 `rapidocr_json` | F16.3 能力自检 |
| VC++ 2015-2022 运行库 | 部分引擎可能依赖（Win7 尤其） | 引擎启动失败 | 镜像预置；`--check-prereq` 验证 |
| 可写临时目录（`%TEMP%`） | 必需（镜像准备项） | 解压与样本落地失败 | 工具自检，不可写时回退同级目录并告警 |
| AV / EDR 排除目录 | 强烈建议（镜像准备项，**工具无法自检**） | 未签名单文件可能被拦截或删除，工具无法运行 | 写入 D9 镜像构建指引与 README（4.7 C 类） |
| 管理员权限 / 关闭 UAC | 必需（镜像准备项） | 静默安装失败、GUI 自动化被提权弹窗中断 | 工具启动自检权限并告警；UAC 由镜像侧关闭（F14.5） |
| （可选）tesseract / Umi-OCR | 可选 | 对应后端不可用 | F16.3 |

> 该"检测 + 提示 + 不代装"策略同样适用于后期新增的任何前置依赖。**完整的依赖边界（含语言运行时要不要、镜像准备项有哪些）见 4.7**；表中 7-Zip / OCR 引擎 / VC++ 运行库的**离线安装包由交付包提供**（8.3 B 组），安装步骤见 **DEPLOY.md（D14）**。

---

## 6. 非功能需求

| 类别 | 要求 |
|------|------|
| 性能 | 单文件触发不超过配置超时；批量并发度可配置（默认串行）；单次 OCR 识别 ≤ 800ms；引擎常驻复用，避免重复初始化 |
| 稳定性 | 单文件失败不影响整体流程；全局看门狗防挂死（F9）；无协程泄漏；OCR / 模板任一失败均可降级；OCR 引擎崩溃可自动重启 |
| 兼容性 | **Windows 7 SP1 x64 / 8.1 / 10 / 11 / Server 2008 R2+**；架构预留 Linux / macOS |
| 权限 | 以管理员权限运行（静默安装需要）；启动时自检权限，无管理员权限则告警并继续；沙箱镜像建议关闭 UAC 弹窗以允许 GUI 自动化 |
| 日志 | 除 JSON 结果外，输出可读日志到 stdout 与文件（时间戳、级别、文件名、阶段）；日志与 JSON 结果分流，不混写 stdout |
| 可配置 | 超时、并发度、递归深度、解压大小上限、尝试间隔与次数、参数表、按钮关键词表、**OCR 引擎与路径/传输方式/置信度阈值**、模板目录均可配置 |
| 无残留 | 退出前清理临时目录、**回收 OCR 引擎子进程**；不修改系统默认文件关联；**不写入注册表、不产生安装残留** |
| 可移植 | 核心层不依赖 Windows API；平台相关逻辑全部走适配层；**Win7 / Win10 差异（DPI、UIA、截屏）全部封装在适配层** |
| 可维护 | 见 7.3 工程规范：接口由使用方定义、依赖注入、`gofmt`/`go vet`/`golangci-lint` 零 error、core 层单测覆盖率 ≥ 70% |
| 离线 | 全程不访问网络（OCR 走本地进程 / 环回 TCP）；前置缺失时只提示、不下载、不安装。**部署所需的第三方物料全部随交付包离线提供**（8.2 / 8.3 B 组） |
| 分发 | **工具本体**：单一可执行文件、免安装，拷贝即用、无语言运行时依赖（不需 Go / .NET / JRE / VC++）；**交付包**：整包分发，含 `third_party\` 离线物料与 `DEPLOY.md` 部署文档，运维照文档部署（8.1 / 8.2） |
| 运行时依赖边界 | 工具本体**零语言运行时依赖**；**7-Zip 与 OCR 引擎属功能性前置依赖**，其离线安装包由交付包提供、由运维安装；另有管理员权限、UAC 关闭、AV 排除目录、可写临时目录四项镜像准备要求。部署前按 **4.7 清单**逐项核对（验收 A22 / A23） |

### 6.1 工具自身退出码表

| 退出码 | 含义 |
|--------|------|
| 0 | 全部文件处理完成（无论单文件结果如何） |
| 1 | 工具自身运行期异常崩溃 |
| 2 | 输入参数/路径非法，未开始处理 |
| 3 | 配置文件解析失败 |
| 4 | **必需前置依赖缺失（未检测到 7-Zip）** —— 工具已提示用户先安装，不代为安装 |
| 5 | **OCR 引擎缺失且 `require_ocr: true`** —— 已提示用户安装（默认 `require_ocr: false` 时降级运行，不返回此码） |

---

## 7. 技术方案

### 7.1 技术选型

| 组件 | 方案 |
|------|------|
| 语言 | **Go（定案）**：`CGO_ENABLED=0` 静态编译单文件，目标机**无需 Go 环境** |
| **工具链版本** | **锁定 Go 1.20.x** —— Go 1.21 起已放弃 Windows 7/8/8.1 支持（要求 Win10 / Server 2016+）。`go.mod` 声明 `go 1.20`；**选型第三方库前必须确认其最低 Go 版本 ≤ 1.20** |
| 构建 | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go1.20.x build -trimpath -ldflags "-s -w" -o AutoTrigger.exe ./cmd/autotrigger` |
| 跨平台策略 | Core 层平台无关；Platform Adapter 分平台实现（`_windows.go` 构建标签隔离） |
| 进程树管理 | Windows：Job Object（Vista+ 可用）；Linux/macOS：进程组 |
| 文件解压 | **外部 7-Zip 命令行**（环境需预置）；工具**仅检测与提示**，不引入解压库、不使用 CGO |
| 文件头识别 | 自实现魔数表（约 20 条），`go:embed` 内嵌 |
| GUI 自动化 | Windows UIA（`syscall` + COM 中继库调用；**Win7 走 UIAutomationCore 基础能力**）+ SendInput 键鼠注入 + 自实现模板匹配（纯 Go，不引入 OpenCV/CGO） |
| **OCR 辅助** | **外部开源 OCR 引擎进程（默认 PaddleOCR-json）**：绿色免安装、离线；通过 **stdio 或环回 TCP** 通信；返回**文本 + 四角像素坐标 + 原生 score**；Windows 内置 OCR 仅作可选回退（`ocr.engine=windows`，默认关闭）；**禁止 CGO、禁止链接原生 OCR SDK** |
| 截屏 | GDI `BitBlt` / `PrintWindow`（按系统版本选择 `PW_RENDERFULLCONTENT` 等标志） |
| DPI 处理 | Win10 1607+ 用 `GetDpiForWindow`；**Win7/8.1 回退 `GetDeviceCaps(LOGPIXELSX)` + `SetProcessDPIAware`** |
| 结果输出 | JSON（主结果）+ 文本日志（辅助），仅写本地文件 |
| 配置与资源 | 默认值、参数表、关键词表、图像模板 `go:embed` 内嵌；支持外部同名文件覆盖 |
| 静态检查 | `gofmt` / `go vet` / `golangci-lint`（errcheck、govet、staticcheck、revive、gocritic） |

#### 7.1.1 语言选型论证：为什么是 Go 而不是 Python

语言定案为 Go 的**决定性因素不是性能，而是交付形态**——它是本项目唯一一条 Python 无法满足、且无法通过工程手段绕开的硬约束。

**① 交付形态（硬约束，一票决定）**

需求明确：工具本体是**单个 exe，拷贝即用，目标机不装任何东西**（DC1、7.2），并要求**退出前清理临时目录、不产生残留**（第 6 节）。

Python 没有真正的静态编译路径，只能靠 PyInstaller / Nuitka 打包，而其产物形态与本要求直接冲突：

| 冲突点 | Python 打包产物 | 本工具要求 |
|---|---|---|
| 单文件真实性 | PyInstaller 单文件模式 = bootloader + 压缩归档，**运行时解压到 `%TEMP%\_MEIxxxxxx`** | 自包含、无释放行为 |
| 残留 | 释放目录在异常退出/强杀时可能残留 | 无残留、可清理 |
| 体积 | 30~100 MB+（含解释器与标准库） | 目标 ≤ 30 MB |
| 启动延迟 | 解释器冷启动数百 ms；打包产物首次启动 **1~3 秒** | 需对每个样本做 500ms 级窗口轮询，开销累积 |

Go 以 `CGO_ENABLED=0` 真静态链接，一个 exe、**无自解压、无临时释放、无残留**，一次性满足全部四项。

**② 反病毒友好性（本项目特有，权重很高）**

本工具的行为特征（批量执行任意文件、模拟键鼠、写注册表、触发安装）在 EDR/AV 眼中已与恶意软件高度重合，且为未签名单文件——**这本身就是 11 节已列的高风险项**。

在这个前提下再选 Python 打包，等于**在已很高的拦截风险上叠加一层**：PyInstaller / Nuitka 的 bootloader 是历史上被恶意样本大量滥用的载体，此类"打包器产物"是 AV 误报重灾区。Go 静态二进制无打包器特征、无释放行为，误报率显著更低。

**③ Win7 约束下，Python 的版本处境比 Go 更差**

| | 最后支持 Win7 的版本 | 该版本状态 |
|---|---|---|
| Go | **1.20.x**（2023-02 发布） | 相对新，生态可用性尚可 |
| Python | **3.8.10**（官方文档原话："If you require Windows 7 support, please install Python 3.8"；3.9+ 安装器要求 Win8.1+） | **已于 2024-10 结束维护（EOL）** |

锁在已 EOL 的 3.8 上意味着：拿不到安全修复；大量现代库（NumPy 2.x、新版 opencv-python 等）已放弃 3.8 支持。社区虽有 PythonWin7 一类第三方重编译包可让 3.9+ 跑在 Win7，但那是**非官方维护产物**，引入供应链风险，不适合交付给安全团队使用。

**④ 并发模型与图像处理**

需并行处理：500ms 窗口轮询 + OCR 引擎通信 + 看门狗超时 + 日志写入；其中**模板匹配与截图像素运算是 CPU 密集**。

- Go：goroutine + channel + `select` + `context`，超时/取消/并发为一等公民
- Python：受 GIL 限制，CPU 密集的图像处理多线程被串行化；要真并行只能上 multiprocessing，而**多进程与 PyInstaller 单文件是经典冲突**（子进程重新解压、自身路径解析失败），复杂度陡增

**⑤ 工程规范与类型安全**（对应需求"代码要规范、高内聚低耦合"）

Go 的强类型、接口隐式实现、标准化工具链内建（`gofmt`/`go vet`/`golangci-lint`/`go test -cover`）天然贴合该要求，零配置成本。Python 也能做到（mypy + ruff + pytest + coverage），但需外部拼装。更关键的是：本工具适配层大量涉及 **`syscall` / COM 调用与 unsafe 指针操作**，编译器静态检查的价值远高于普通项目——这类代码的错误若推到运行时才发现，代价是现场不可复现的偶发故障。

**Python 的强项为何在本项目中已被消解（客观评估）**

| Python 的强项 | 在本项目的实际情况 |
|---|---|
| OCR / 图像生态（PaddleOCR、OpenCV、NumPy） | 已决策 **OCR 走独立外部进程**（PaddleOCR-json 绿色包，DC8），Python 生态优势用不上；模板匹配自实现（纯 Go）以规避 CGO |
| GUI 自动化库成熟（pywinauto / pyautogui） | 这类库本质是 **Win32 / UIA API 的封装**；本工具同样直接调用同一套 API（`syscall` + COM 中继库，见 7.1 表），能力等价而非更低；且 Win7 上的 UIA 覆盖度**取决于系统能力而非语言库**，换 Python 不会变好 |
| 开发速度快、迭代方便 | 本项目为一次性开发、长期在离线环境无人值守运行的工具：交付形态与运行稳定性的收益 > 开发迭代速度的收益 |

**结论**：Go 胜出的三条理由是 **交付形态（真静态单文件 / 无释放 / 无残留）+ 反病毒友好性 + Win7 下版本代价更小**。

**反向条件**：若 Q10 确认目标环境**不含 Win7**，Go 可升至 1.21+ 以获取更新的生态；但 ①② 两条理由与 Win7 无关，**即使不覆盖 Win7，语言仍应维持 Go**。

### 7.2 工具本体打包要求（单文件）

> 本节约束的是**工具本体**（`AutoTrigger.exe`）；**交付包的完整组成见 8.1 / 8.3**。两者不可混为一谈：exe 追求"单文件、零依赖"，交付包追求"物料齐全、照文档即可部署"。

| 要求 | 说明 |
|------|------|
| 产物 | **单个 `AutoTrigger.exe`**，放任意目录可运行，**免安装、无需 Go 环境**，交付包内其余文件均非其运行必需 |
| 无运行时依赖 | `CGO_ENABLED=0` 静态编译，目标机无需 Go / .NET / JRE / VC++ 运行库 |
| **CGO 强制校验** | 构建脚本与 CI 显式断言 `CGO_ENABLED=0`；任何拉入 CGO 的依赖直接拒绝（详见 4.7 构建期硬性校验） |
| **导入表核验** | 产物 PE 导入表**只允许系统 DLL**，不得出现 `vcruntime*.dll` / `msvcp*.dll` 等；核验命令见 4.7，异常即构建失败（验收 A22） |
| **反病毒友好性** | 交付前用目标镜像的 AV/EDR 实扫一次；未签名单文件若被拦截，需准备排除目录或代码签名（见 11 节风险） |
| 无内嵌第三方物料 | `AutoTrigger.exe` **自身不内嵌**安装包、脚本、动态库、OCR 模型（避免体积膨胀与授权问题）；第三方物料以**独立文件**形式放在交付包的 `third_party\` 下，由部署环节安装 |
| 可独立冒烟 | 单独拷出 exe 即可启动、跑通不依赖第三方的能力（如 PE 触发、DPI/截屏），便于快速验证与排障 |
| 无网络依赖 | 校验与版本信息不得联网查询；OCR 走本地进程 / 环回 TCP |
| 体积控制 | 目标 ≤ 30 MB（模板与关键词表资源压缩内嵌） |
| 可选同目录文件（均非必需） | `config.json`（外部配置覆盖）、`templates/`（额外图像模板）；缺失时使用内嵌默认值 |
| Win7 构建验证 | 产物须在 **Win7 SP1 x64** 实机/虚拟机冒烟（对应验收 A20） |
| 验证 | 在**未安装 Go 及任何运行时、无网络**的干净镜像上，仅拷贝单个 exe 即可完成全部功能（对应验收 A14） |

### 7.3 工程规范与架构约束（高内聚、低耦合）

**目录结构（建议）**

```
cmd/autotrigger/main.go          # 仅做依赖装配与启动，业务逻辑为零
internal/core/                   # 平台无关核心：调度、类型识别、尝试编排、结果模型
internal/core/ports/             # 接口定义（使用方定义接口，随 core 走）
internal/platform/windows/       # Windows 适配：进程/Job Object、UIA、注册表、截屏、DPI、文件系统证据
internal/platform/windows/winver/# 系统版本判定与 API 能力分支（Win7 / Win10 差异集中在此）
internal/platform/linux/         # 空实现占位（编译期可选）
internal/prereq/                 # 7-Zip / OCR 引擎探测、能力自检与用户提示（不执行安装）
internal/ocr/                    # OCR 引擎适配：进程生命周期、stdio/TCP/HTTP 通信、结果解析、崩溃重启
internal/gui/                    # GUI 识别编排：UIA/模板/OCR 三级仲裁、键鼠注入
internal/resources/              # go:embed 内嵌资源（魔数表/参数表/关键词表/模板）
internal/config/                 # 配置加载与合并（默认值 < 文件 < 命令行）
internal/logging/                # 日志接口与实现
pkg/schema/                      # 对外稳定的结果 JSON 类型（供下游复用）
testdata/                        # 测试样本与期望结果
```

**架构约束**

| 约束 | 说明 |
|------|------|
| 依赖方向 | `cmd → core → ports`；适配层实现 ports，**core 永不 import platform / ocr / gui 具体实现** |
| 接口归属 | 接口由使用方（core）定义，禁止"实现方定义接口" |
| 依赖注入 | 统一在 `main` 装配；core 内不得 `new` 具体实现、不得使用包级全局状态 |
| 禁止 CGO | 全部依赖纯 Go 或 `syscall`/中继库，保证静态单文件 |
| 识别策略可插拔 | UIA / 模板 / OCR 三级识别实现同一 `Control` 模型；**OCR 引擎可配置切换（PaddleOCR-json / RapidOCR-json / Umi-OCR / tesseract / 内置 / 关闭）**，新增引擎不改 core 编排逻辑 |
| 平台能力分支集中 | Win7 与 Win10 的 API 差异（DPI、截屏标志、UIA 特性）统一封装在 `platform/windows/winver`，禁止散落 `if win7` 判断 |
| 外部进程管理 | OCR 引擎等外部进程统一经 `ProcessRunner` 启停，纳入 Job Object 与全局清理流程，保证无残留子进程 |
| 错误处理 | `fmt.Errorf("...: %w", err)` 包裹 + `errors.Is/As` 判定；不用 panic 控制流程；错误须带上下文（文件、阶段） |
| 并发 | 统一 `context.Context` 传递取消与超时；`errgroup` 控制并发；`defer cancel()` 防泄漏 |
| 配置与资源 | 全部经 `config` / `resources` 包访问，禁止散落硬编码常量与路径 |
| 命名与注释 | 导出标识符必须有 doc comment；单文件建议 ≤ 400 行；包职责单一 |
| 测试 | core 层单测覆盖率 ≥ 70%；适配层以表驱动测试覆盖关键路径；Windows API / OCR 交互通过接口 mock，测试不依赖真实安装器与真实 OCR 引擎 |
| 静态检查 | CI（或提交前脚本）执行 `gofmt -l`、`go vet`、`golangci-lint run`，要求零 error |
| 日志 | 统一 logger 接口；禁止 `fmt.Println` 直接写 stdout 干扰结果解析 |

### 7.4 7-Zip 前置依赖（检测 + 提示，不代装）

1. **能力来源**：压缩包支持（zip / 7z / rar / tar / gz / iso 等）**完全由外部 7-Zip 提供**，工具不自研、不内嵌解压实现，也不做"运行时释放内嵌 7z.exe"的降级
2. **部署前提**：**由镜像构建或运维预先安装 7-Zip**；工具**不随包分发安装包、不代为安装**
3. **探测顺序**：配置指定路径 → 注册表 `HKLM\SOFTWARE\7-Zip\Path`（含 Wow6432Node）→ `PATH` → 常见默认路径
4. **缺失处理**：输出 F16.2 规定的用户提示 → 含压缩包样本时以退出码 `4` 结束；可用 `--no-archive` 跳过压缩包继续处理其余样本
5. **部署建议**：镜像构建阶段预置 7-Zip 并在构建流程末尾执行 `AutoTrigger.exe --check-prereq` 作为卡点校验

### 7.5 config.json 结构（全部字段有内嵌默认值，可选覆盖）

```json
{
  "timeout_sec": 120,
  "concurrency": 1,
  "attempt_interval_sec": 2,
  "max_attempts_per_file": 8,
  "archive": {
    "max_depth": 3,
    "max_extract_size_bytes": 2147483648,
    "sevenzip_path": "auto",
    "require_sevenzip": true
  },
  "gui_automation": {
    "enabled": true,
    "poll_interval_ms": 500,
    "click_jitter_px": 3,
    "max_actions_per_window": 10,
    "keyword_file": "buttons.zh-CN.json",
    "template_dir": "templates"
  },
  "ocr": {
    "enabled": true,
    "engine": "paddleocr_json",
    "engine_path": "auto",
    "transport": "stdio",
    "tcp_port": 0,
    "startup_timeout_ms": 10000,
    "recognize_timeout_ms": 3000,
    "keep_alive": true,
    "restart_on_failure": true,
    "max_restarts": 2,
    "require_ocr": false,
    "min_score": 0.6,
    "roi_padding_px": 12,
    "min_text_height_px": 10,
    "max_text_height_px": 80,
    "cache_by_screenshot_hash": true
  },
  "install_verify": {
    "settle_sec": 3,
    "watch_dirs": ["C:\\Program Files", "C:\\Program Files (x86)"]
  },
  "dll_export_probe_count": 3,
  "log_level": "info"
}
```

> - `sevenzip_path` / `ocr.engine_path` 取值为 `auto` 时按 F16.1 / F16.3 的探测顺序自动查找
> - `require_sevenzip: false` 等价于默认启用 `--no-archive` 行为
> - `ocr.engine` 可选 `paddleocr_json`（默认）/ `rapidocr_json` / `umi_ocr` / `tesseract` / `windows`（Win10+ 可选回退）/ `none`
> - `ocr.transport` 可选 `stdio`（默认）/ `tcp` / `http`（仅 Umi-OCR）
> - `ocr.require_ocr: false` 表示 OCR 缺失时**降级而非失败**；设为 `true` 则缺失时以退出码 5 结束
> - `ocr.min_score` 为**引擎原生置信度阈值**，低于该值的识别结果不参与点击

#### 7.5.1 配置加载、覆盖与校验规则

**① 配置来源与优先级（由高到低）**

| 层级 | 来源 | 说明 |
|------|------|------|
| 1 | **命令行参数** | `--timeout` / `--output` / `--recursive` / `--no-archive` / `--no-ocr` 等，仅覆盖其对应的字段 |
| 2 | **`--config <path>` 指定的文件** | 显式指定时**取代**同目录 `config.json`（二者不叠加） |
| 3 | **同目录 `config.json`** | 未指定 `--config` 时自动查找 exe 同目录；不存在则跳过本级 |
| 4 | **内嵌默认值** | 编译期 `go:embed` 固化，保证零配置可直接运行 |

**② 合并语义：深合并（字段级递归覆盖），不是整体替换**

外部配置**只需写要改的字段**，未出现的字段保持下层取值：

```json
{ "timeout_sec": 300 }
```

上例仅把超时从 120 改为 300，`archive` / `ocr` / `gui_automation` 等字段仍取内嵌默认值。**嵌套对象逐层深合并**——只写 `"ocr": { "min_score": 0.8 }` 时，`ocr` 下的其他字段不受影响。

> ⚠️ **实现红线**：若按"整体替换"实现，上例会使 `ocr` 等字段全部变为零值，属严重缺陷。验收 A24 第 ① 条专门卡此项。

**③ 未知字段：忽略 + 启动告警，不失败**

出于向前兼容——新版本新增的字段在旧版工具上应被忽略而非报错。告警写入日志（如 `WARN unknown config key: ocr.min_scores`），便于发现拼写错误。

**④ 类型错误与取值越界：报错退出（退出码 3）**

| 情况 | 行为 |
|------|------|
| 文件不存在 / 不可读 | 退出码 3，**不静默回落默认值**（否则用户会误以为配置已生效） |
| JSON 语法错误 | 退出码 3，输出出错位置 |
| 字段类型不符（如 `timeout_sec: "abc"`） | 退出码 3，输出**字段路径**与期望类型 |
| 取值越界（`timeout_sec <= 0`、`concurrency < 1`、`min_score` 不在 `[0,1]` 等） | 退出码 3，输出字段路径与合法范围 |

**⑤ `--dump-config`：导出生效配置**

由于全部字段可选、默认值内嵌在 exe 中，使用者无法直观得知"有哪些可配项、当前实际生效值"。`--dump-config` 将**合并后的完整生效配置**以 JSON 输出到 stdout（不处理样本、不写结果文件），用途：

- **生成配置模板**：重定向到 `config.json` 后按需修改
- **排障确认**：核对自己改的字段是否真的生效

> 导出格式为**严格 JSON（不支持注释）**；各字段含义与取值范围见 `DEPLOY.md`（D14）与 `config.example.json`（D2）。

**⑥ 结果可追溯**

结果 JSON 中记录 `config_digest` —— **生效配置的 SHA-256**（按规范化后的键值计算，不受缩进与字段顺序影响），用于复现异常时核对"当时的配置是否与现在一致"。

### 7.6 OCR 引擎接入约定（v2.4 新增）

| 约定 | 说明 |
|------|------|
| 通信方式 | 优先 **stdio**（stdin 发 JSON、stdout 读 JSON）；可选环回 TCP（`-port` / `-addr=loopback`）；Umi-OCR 支持 HTTP |
| 进程生命周期 | **常驻复用**：任务开始启动一次 → 多次识别 → 任务结束回收；禁止每次识别都启停进程 |
| 启动超时 | `startup_timeout_ms`（默认 10s）；超时视为引擎不可用并降级 |
| 识别超时 | `recognize_timeout_ms`（默认 3s）；超时放弃本次识别，不阻塞 GUI 流程 |
| 健康检查 | 定期或被调用前检查进程存活；卡死/崩溃按 `restart_on_failure` 重启（上限 `max_restarts`），超限后本次任务内禁用 OCR |
| 输入格式 | 优先传**图片文件路径**（避免大 Base64 传输开销）；需要时支持 Base64 |
| 结果解析 | 统一解析为 `[]OcrText{Text, Box[4], Score}`；不同引擎的输出差异在 `internal/ocr` 内归一化 |
| 语言 | PaddleOCR 系默认含中英；语言配置写入 `prereq.ocr_engine_languages`；**界面语种与引擎语言不匹配时结果不参与点击** |
| 资源占用 | 引擎进程独立于样本进程，避免相互影响；OCR 处理与样本运行尽量错峰（GUI 空闲时识别） |
| 安全 | 仅本地进程 / 环回通信，**绝不外连**；引擎可执行文件由镜像提供，工具不校验签名但记录路径与版本 |

### 7.7 OCR 引擎候选评估记录（v2.5 新增）

**目的**：记录为什么最终选 `PaddleOCR-json`，以及外部候选（含 2026-09 评审的外部资料《2026年最硬核的12个OCR开源项目》所列 12 个项目及评论区补充的混元/DeepSeek OCR）为何不采用，供评审与后续复评追溯。

**评估维度（硬性门槛，任一不满足即出局）**

| # | 门槛 | 说明 |
|---|------|------|
| 1 | **离线可用** | 全程无外连；模型与依赖随交付包分发（当前环境无网络） |
| 2 | **Win7 SP1 x64 可运行** | 与平台基线一致（同 Go 1.20.x 的约束原因） |
| 3 | **纯 CPU 可接受的延迟** | 无 GPU 保证；单次识别目标 ≤ 800ms（`性能`要求见第 6 节） |
| 4 | **进程外可调用** | CLI / stdio / 环回 TCP / HTTP 任一；**禁止 CGO、禁止链接原生 SDK** |
| 5 | **返回逐词/行像素坐标 + 置信度** | 点击定位的硬需求（缺坐标即无法用于 GUI 自动化） |
| 6 | **中文界面识别可用** | 样本以中文安装向导为主 |
| 7 | **轻量可分发** | 不引入 Python/JVM/.NET 等额外运行时；体积可控 |
| 8 | **许可可用于本场景** | 无商用限制，或已获授权 |

**候选评估表**

| 项目 | 类型 | 关键限制 | 判定 |
|------|------|---------|------|
| **PaddleOCR-json** | 引擎（PaddleOCR 的 C++ 独立进程封装） | 需 CPU 支持 AVX | ✅ **采用（默认）** |
| **RapidOCR-json** | 引擎（同门姊妹项目） | 速度略慢 | ✅ **采用（备选，免 AVX）** |
| **Umi-OCR** | 应用（内置上述两引擎，带 CLI/HTTP） | 约 500MB 的 GUI 软件，偏重 | 备选 |
| **Tesseract** | 引擎（CLI） | 中文/小字号界面识别明显弱；Win7 需特定旧版本 | 备选（兜底） |
| **PaddleOCR（官方 Python 版）** | 框架 + Python 管线 | 需 PaddlePaddle + Python 3.9~3.13 完整环境（数百 MB 起），与"零语言运行时"冲突 | 不采用（**其算法/模型正是 PaddleOCR-json 所用**，能力不损失） |
| **MinerU** | 文档解析（PDF → Markdown/JSON） | Python + torch + 0.9B VLM；目标是**长文档结构化**，不产出控件级坐标 | 不采用 |
| **GOT-OCR 2.0** | 通用 VLM OCR | 需 GPU/torch；面向文档理解（公式、图表、乐谱），非控件定位 | 不采用 |
| **Surya** | 文档 OCR 工具包 | Python 3.10+ + PyTorch；**权重 cc-by-nc-sa-4.0、代码 GPL-3.0**，商用有条件 | 不采用 |
| **PDF-Extract-Kit** | 文档解析工具箱 | Python + 多个重型模型（LayoutLMv3 / YOLOv8 / UniMERNet） | 不采用 |
| **Versatile-OCR-Program** | 教育材料结构化 | **依赖 Google Vision / MathPix 等云端服务** | 不采用（违反离线） |
| **Text-Grab** | Windows 桌面小工具 | 基于 **Windows 内置 OCR**（Win7 无此组件，且为单语言引擎） | 不采用 |
| **Tesseract.js** | 浏览器 WASM 库 | 需 JS 运行时；识别率有限 | 不采用 |
| **Tesseract (.NET 封装)** | .NET 绑定 | 需 .NET 运行时，与 Go 单文件冲突（引擎本身已由上一项覆盖） | 不采用 |
| **Dango-Translator** | 屏幕翻译工具 | Python GUI 应用，非可复用的 OCR 引擎 | 不采用 |
| **HunyuanOCR** | 1B 端到端 VLM | 生成式解码，CPU 延迟高；需 torch/vLLM，或自行用 llama.cpp 转 GGUF 构建；Tencent Hunyuan 自定义许可 | 暂不采用（**列入观察项**） |
| **DeepSeek-OCR** | VLM | 官方 vLLM 路径需 CUDA 11.8+ / 16GB 显存；**纯 CPU 慢 10~50 倍**；需 Python 3.12 + PyTorch | 不采用 |

**结论：该资料中的 12 个项目（及评论区补充项）均未优于现选方案。** 根因是**任务类型不匹配**：

- 资料面向 **文档数字化/结构化**（PDF → Markdown、公式、表格、长文档），产出是"可读、可检索的文本"，因此普遍是 **Python + 大模型 + GPU** 栈
- 本工具需要的是 **GUI 控件定位**——截屏后取按钮文字（"下一步/安装/我接受/Install"）的**像素包围盒**并据此点击，要求**小、快、纯 CPU、离线、进程外、带坐标**
- 本工具需要的不是"更强的文档理解"，而是"更稳的小字识别 + 坐标输出"，传统轻量检测/识别流水线（PaddleOCR-json）正是这类形态

**唯一值得持续观察的方向**：小参数 VLM（HunyuanOCR 1B、DeepSeek-OCR 等）经 GGUF/llama.cpp 量化后在纯 CPU 上运行，理论上可用单一模型替代"UIA + 模板 + OCR"三件套的组合。当前障碍明确—— **生成式解码延迟（秒级 vs 目标 800ms）**、Win7 支持未知、需额外构建 llama.cpp 运行时。故本期不做，已记入第 13 节后续方向；`internal/ocr` 按可插拔设计，将来新增引擎不改 core 编排逻辑。

---

## 8. 交付物

### 8.1 交付物两级模型（v2.5 新增，重要）

**"单一二进制"是工具本体的形态，不是交付包的全部。** 二者分属不同层次，务必区分：

| 层次 | 内容 | 面向对象 | 使用方式 | 必需性 |
|------|------|---------|---------|--------|
| **① 工具本体（运行时单元）** | **单个 `AutoTrigger.exe`** | 使用工具的人 | 拷贝到任意目录直接运行，**无安装、无语言运行时依赖** | 跑工具时必需 |
| **② 交付包（发布单元）** | 工具本体 **+ 7-Zip / OCR 引擎 / VC++ 运行库离线安装包 + 部署说明文档 + 可选配置与模板** | 部署运维人员 | 按 `DEPLOY.md` 装完前置依赖，再把 exe 放到运行目录 | **部署时必需** |

**设计意图**：让"**用**工具的人只需处理一个文件"，同时让"**部署**的人一次拿到全部物料、不用联网找安装包"。工具本体**不含任何安装逻辑**（不代为安装、不释放文件、不写注册表）——前置依赖的安装由运维按文档手工完成。

**三层职责对照**

| 环节 | 由谁负责 | 动作 |
|------|---------|------|
| 工具使用 | 检测工程师 / 流水线 | 拷 exe → 直接跑（见 D5 README） |
| 前置依赖安装 | 运维 / 镜像构建人员 | 用交付包内离线包安装 7-Zip、OCR 引擎（见 D14 DEPLOY.md） |
| 部署校验 | 运维 / 镜像构建人员 | `AutoTrigger.exe --check-prereq` 作卡点，通过后再批量分发（见 F16.4） |

### 8.2 交付包目录结构（建议）

```
AutoTrigger-<版本>-win64\
├─ AutoTrigger.exe                    # ★ 工具本体：唯一必需的可执行文件，可单独拷出使用
├─ README.md                          # 使用说明（CLI 用法、退出码、结果 JSON 说明）
├─ DEPLOY.md                          # ★ 部署说明（照做式步骤：装前置 → 校验 → 拷贝 exe）
├─ config.example.json                # 配置模板（可选覆盖，缺失用内嵌默认）
├─ templates\                         # 图像模板（可选覆盖，缺失用内嵌默认）
├─ third_party\                       # ★ 第三方离线物料（全部离线，无需联网获取）
│   ├─ 7zip\      7z<版本>-x64.msi
│   ├─ ocr\       PaddleOCR-json_v<版本>.7z   （默认引擎，需 AVX）
│   │              RapidOCR-json_v<版本>.7z   （无 AVX CPU 的备选，接口一致）
│   └─ vcredist\  VC_redist.x64.exe           （Win7 场景可能需要）
├─ scripts\
│   ├─ verify-ocr.ps1                 # OCR 环境探测与端到端验证
│   └─ check-prereq.cmd               # 一键自检（包装 `AutoTrigger.exe --check-prereq`）
└─ docs\                              # 架构说明、测试样本说明、结果 Schema
```

> 说明：`third_party\` 中的包**只供运维安装使用**，工具本体不会读取或执行它们；`templates\` 与 `config.example.json` 属可选覆盖项，删除不影响运行。

### 8.3 交付物清单

**A 组：工具本体**

| 编号 | 交付物 |
|------|--------|
| D1 | **单一可执行文件** `AutoTrigger.exe`（Go 1.20.x 静态编译，**免安装、拷贝即用**，目标机无需任何语言运行时；**须在 Win7 SP1 x64 与 Win10 上分别验证**） |

**B 组：交付包内第三方离线物料**（离线环境必需，随包分发）

| 编号 | 交付物 | 说明 |
|------|--------|------|
| D11 | **7-Zip 离线安装包**（`7z<版本>-x64.msi` / `.exe`） | 解压后端，**必需**（除非 `--no-archive`）；附版本号与 SHA-256 记录 |
| D12 | **OCR 引擎离线包**：`PaddleOCR-json`（默认）+ `RapidOCR-json`（无 AVX 备选） | 绿色解压即用；建议内部归档固定版本，避免上游变更导致识别行为漂移 |
| D13 | **VC++ 2015-2022 运行库离线安装包**（`VC_redist.x64.exe`，可选） | Win7 场景 OCR 引擎可能需要；按目标镜像实际情况决定是否随包 |

**C 组：文档、配置与脚本**

| 编号 | 交付物 |
|------|--------|
| **D14** | **部署说明文档 `DEPLOY.md`（新增，P0）**：照做式步骤——① 前置检查（系统版本 / 权限 / AV 排除）；② 安装 7-Zip（命令行示例）；③ 部署 OCR 引擎（解压路径 + `config.json` 指向）；④ 安装 VC++ 运行库（如需）；⑤ `--check-prereq` 校验；⑥ 拷贝 exe 到运行目录；⑦ 常见问题（AV 拦截、AVX 缺失）。**要求：运维只读此文档即可独立完成部署** |
| D5 | 使用说明文档（README：**工具本体免安装使用方式**、CLI 参数、退出码表、`--check-prereq` / `--dump-ui` / `--no-archive` / `--no-ocr` 用法、结果 JSON 说明） |
| D9 | 环境前置检查表与镜像构建指引（依赖清单、离线包放置路径、管理员权限 / UAC / AV 排除目录 / 可写临时目录、`--check-prereq` 卡点用法；对应 4.7 与 8.2） |
| D2 | 外部覆盖配置模板 `config.example.json`（**可选**；含全部字段与内嵌默认值，**可由 `AutoTrigger.exe --dump-config` 生成**；字段含义与取值范围由 D14 说明，外部配置**只需写要修改的字段**，见 7.5.1） |
| D3 | 可外部编辑的参数/关键词表（silent_params.windows.json、buttons.zh-CN.json）与图像模板目录（含 F14.2 初始模板集，均为可选覆盖） |
| D4 | 平台抽象层接口定义文档（对应 4.3）+ 架构说明（对应 7.3）+ **Win7/Win10 差异说明（对应 4.6）** |
| D6 | 测试样本集与预期结果对照表（重点覆盖：静默成功 / 向导成功 / **自绘界面 OCR 定位** / 需联网 / 装不成功 / 前置缺失 六类；**并在 Win7 环境复测**） |
| D7 | 结果 JSON Schema 文件（供下游流水线校验） |
| D8 | 构建与检查脚本（Makefile / build.ps1：**Go 1.20.x 静态构建** + gofmt/vet/lint + 单测覆盖率 + **导入表核验**） |
| D10 | **OCR 环境验证脚本** `scripts/verify-ocr.ps1`（现有版本验证 Windows 内置 OCR；**M1 需扩展为多引擎探测 + 端到端识别验证**，输出文本与四角像素坐标） |

---

## 9. 验收标准

| 编号 | 验收项 | 通过标准 |
|------|--------|---------|
| A1 | PE 文件触发顺序 | 静默参数轮询首先执行（≥3 种方式），随后回退无参数直接运行并由 GUI 自动化接管 |
| A2 | MSI 静默安装 | 能正确调用 msiexec，记录退出码与 MSI 日志路径，3010 判定为成功 |
| A3 | 压缩包递归 | 能通过 7-Zip 解压并触发内部可执行文件，递归深度与解压大小均受控 |
| A4 | 超时控制 | 超时后进程树被完整清理不残留；该文件记 `killed_on_timeout` |
| A5 | 未知格式兜底 | 扩展名伪装文件能被文件头识别并正确触发 |
| A6 | 批量处理 | 文件夹内 50 个文件全部处理完毕，单文件失败不影响整体，summary 计数正确 |
| A7 | 结果输出 | JSON 含 `schema_version` 与 `prereq`（含 OCR 引擎探测结果），每次尝试含 method/result/duration，可通过 D7 Schema 校验 |
| A8 | EDR 兼容 | 工具运行期间 EDR 能正常采集到样本行为，无冲突 |
| A9 | 跨平台架构 | Core 层无直接平台 API 调用；适配层接口清晰可扩展；**Win7/Win10 差异集中在适配层** |
| A10 | 快捷方式/DLL | `.lnk` 能解析目标并触发；`.dll` 能通过 rundll32 尝试导出函数 |
| A11 | 退出码 | 参数非法→2，配置错误→3，7-Zip 缺失→4，OCR 缺失且 require_ocr→5，正常完成→0 |
| A12 | GUI 自动化安装 | 对向导式安装器样本（Inno Setup / NSIS / InstallShield），能通过 UIA 完成"同意 → 下一步 → 安装 → 完成"点击链 |
| A13 | 安装成功判定 | 静默与 GUI 两条路径均能产出 `install_completed` 并附证据，且**无误报**（未装成功不得标记） |
| A14 | 工具本体免安装单文件 | 在**未安装 Go / 任何语言运行时、无网络**的干净 Windows 镜像上，仅拷贝单个 exe 即可启动并跑通**不依赖第三方前置**的能力（PE/MSI/脚本触发、截屏、DPI）；**全程无安装步骤、无注册表残留**。压缩包解压与 OCR 定位属于 4.7 B 类前置，不在此项要求内 |
| A15 | 离线合规 | 全程无外连行为（含 OCR 引擎，仅本地进程/环回通信）；安装器联网等待场景标记 `requires_network` 并按超时结束 |
| A16 | 7-Zip 前置依赖处理 | ① 已预置时功能正常；② **未预置时输出明确安装提示（含离线安装命令）并以退出码 4 结束，全程不尝试自行安装**；③ `--no-archive` 可跳过压缩包继续处理其余样本；④ `--check-prereq` 报告准确 |
| A17 | 代码质量 | `gofmt`/`go vet`/`golangci-lint` 零 error；core 层单测覆盖率 ≥ 70%；构建产物为单文件且 `CGO_ENABLED=0`；**使用 Go 1.20.x 构建** |
| A18 | **外部 OCR 引擎接入** | ① 引擎可被正确探测（路径/版本/语言/AVX）；② 常驻复用（一次任务内初始化 1 次，多次识别无重复启动）；③ 引擎缺失/崩溃时自动降级或重启，**不中断主流程**；④ 识别结果含四角坐标与原生 score，且低于 `min_score` 的结果不参与点击 |
| A19 | OCR 安全边界 | OCR 识别到"取消/卸载"等黑名单文本时**不产生点击动作**；OCR 点击次数受单窗口动作上限约束 |
| A20 | **Win7 兼容性** | 在 **Windows 7 SP1 x64** 上：① exe 可直接运行（无需任何运行时）；② 截屏、DPI 换算、Job Object 进程树清理正常；③ UIA 或模板/OCR 至少一级能完成一次向导式安装；④ `--check-prereq` 报告正确 |
| A21 | 无 AVX 环境降级 | CPU 不支持 AVX 时：① 启动自检给出明确提示；② 切换到 `rapidocr_json` 后功能正常；③ 两者皆无时降级为 UIA + 模板且不报错 |
| A22 | **运行时依赖边界** | ① 构建环境强制 `CGO_ENABLED=0`（CI 断言，防误引入 CGO）；② 产物 PE **导入表只含系统 DLL**（`dumpbin /dependents` 核验，无 `vcruntime*` / `msvcp*` / 第三方 DLL）；③ 在**未装 Go / VC++ 运行库**的干净镜像上运行正常；④ 缺失 7-Zip / OCR 引擎时的行为与 4.7 表逐项一致（提示 + 退出码 4/5 或降级） |
| A23 | **交付包完整性与部署文档可用性** | ① 交付包解压后含 8.2 规定的全部目录与文件（exe、`third_party\` 内 7-Zip 与 OCR 包、`DEPLOY.md`、`README.md`、`scripts\`）；② **在一台干净虚拟机（无 Go、无 7-Zip、无 OCR、无网络）上，运维仅按 `DEPLOY.md` 操作即可完成全部前置安装**；③ 完成后 `--check-prereq` 返回退出码 0；④ 随即可用 exe 完成一次安装触发；⑤ 全流程**不需要联网获取任何物料** |
| A24 | **配置加载与覆盖（7.5.1）** | ① 外部配置只写 `{"timeout_sec": 300}` 时，**其余字段仍为内嵌默认值**（验证深合并，非整体替换）；② 嵌套只写 `{"ocr":{"min_score":0.8}}` 时 `ocr` 下其他字段不受影响；③ 命令行 `--timeout 60` 覆盖配置文件中的 300；④ `--config` 与同目录 `config.json` 同时存在时，前者生效且**不叠加**；⑤ 文件不存在 / JSON 语法错误 / 类型不符 / 取值越界均以**退出码 3** 失败并指明字段路径，**不静默回落默认值**；⑥ 未知字段被忽略且产生告警日志；⑦ `--dump-config` 输出的配置重新加载后行为一致；⑧ 结果 JSON 中 `config_digest` 随配置变化而改变 |

---

## 10. 开发里程碑

| 里程碑 | 内容 | 覆盖功能 |
|--------|------|---------|
| M0 环境与选型验证（新增） | ① **Go 1.20.x 工具链与依赖库可用性确认**；② **Win7 SP1 x64 运行验证**（截屏 / DPI / Job Object / UIA 能力基线）；③ **OCR 引擎接入 spike**（PaddleOCR-json 探测、stdio 通信、四角坐标解析、AVX 检测） | 4.6、7.6、D10 |
| M1 核心链路与前置 | CLI + 配置加载 + 类型识别 + PE/MSI/脚本触发 + 多轮参数尝试 + 超时清理 + JSON 输出 + **7-Zip / OCR 引擎自检与用户提示** | F1 F2 F3 F4 F5 F9 F10 F13 F16 |
| M2 安装成功能力 | GUI 弹窗自动化（UIA + 模板 + **OCR 引擎三级识别**）+ 安装证据判定 + 文件夹遍历 + 压缩包递归 + 文档/未知兜底 | F6 F7 F8 F11 F12 F14 F15 |
| M3 打磨与交付 | 单文件静态构建、退出码完善、日志打磨、Schema 校验、测试样本集、README 与 **DEPLOY.md 部署文档**、**交付包打包含 third_party 离线物料**、**Win7 + Win10 双环境冒烟**、**导入表与依赖边界核验（A22）**、**交付包完整性验证（A23）**、**配置加载与覆盖验证（A24）**、lint 与覆盖率达标 | D1~D14、A8~A24 |

---

## 11. 风险与对策

| 风险 | 对策 |
|------|------|
| 样本反沙箱检测，装死不触发 | 工具层面无法解决，需沙箱环境加固；工具确保触发动作本身完整 |
| 安装器需联网下载组件 | 离线环境按超时处理并标记 `requires_network`，不做网络补偿 |
| 安装程序卡在 GUI 等待输入 | 超时强制结束并记录；有 F14 时优先由 GUI 自动化推进 |
| 压缩包嵌套过深或 zip 炸弹 | 递归深度限制（默认 3 层）+ 解压大小限制（默认 2 GB） |
| **目标机未预置 7-Zip** | 启动自检 + **明确提示用户先安装**（含**交付包内离线安装包路径** + 静默安装命令 + 绕过方式）+ 退出码 4；`--no-archive` 可跳过压缩包继续处理；镜像构建阶段用 `--check-prereq` 卡点 |
| **目标机未预置 OCR 引擎** | 启动自检 + **明确提示安装**（指向交付包内 OCR 引擎包与解压方式）+ 自动降级为 UIA + 模板；需要强制时用 `require_ocr: true` + 退出码 5；`--no-ocr` 可显式关闭 |
| **交付包物料缺失或版本不匹配**（离线环境无处补） | 交付包目录结构固化（8.2）+ 打包校验（A23）；`third_party\` 内容记录版本号与 SHA-256；`DEPLOY.md` 内列出全部物料清单与校验方式 |
| **Go 1.21+ 不支持 Win7（必须锁 1.20.x）** | 构建环境固定 Go 1.20.x；`go.mod` 声明 `go 1.20`；引入第三方库前核查其最低 Go 版本；CI 中固定工具链版本 |
| **Go 1.20 生态较旧，部分新库不可用** | 优先选择无依赖或低版本要求的库；必要时自行实现（如魔数表、模板匹配已计划自实现）；在 M0 阶段集中验证 |
| **CPU 不支持 AVX 导致 PaddleOCR-json 无法运行** | 启动能力自检（AVX 检测）；提示改用 `rapidocr_json`；两者皆无则降级 UIA + 模板（A21） |
| **Win7 环境 UIA 覆盖度低** | 明确"UIA 更早回落"策略；提高模板与 OCR 权重；Win7 实机验证模板覆盖清单（A20） |
| **Win7 缺少新 DPI / 截屏 API** | 集中在 `winver` 包做版本分支：`GetDpiForWindow` → `GetDeviceCaps`；`PrintWindow` 标志按版本选择；`BitBlt` 兜底 |
| **未签名单文件被 AV/EDR 拦截或删除**（本工具行为特征明显，触发率不低） | 镜像将工具目录加入 AV 排除项（4.7 C 类准备项，写入 F16.5 与 README）；交付前实扫验证；必要时预留代码签名流程；**注意与"EDR 需正常采集样本行为"平衡——排除只针对工具自身进程/路径** |
| **OCR 引擎依赖 VC++ 运行库（Win7 尤其）** | 列入镜像前置检查表（F16.5）；`--check-prereq` 实际启动一次引擎做能力自检，而非仅检查文件存在 |
| **OCR 引擎初始化慢 / 每次启动开销大** | 引擎**常驻复用**（任务级生命周期）；记录 `ocr_startup_ms` 便于回归；结果缓存按截图哈希复用 |
| **OCR 引擎进程卡死或崩溃** | 识别超时（3s）+ 健康检查 + 自动重启（上限 2 次）+ 超限本次任务内禁用；引擎进程纳入 Job Object 统一清理，避免残留 |
| **OCR 误识别导致误点** | 原生 `score` 阈值过滤 + 完整词匹配 + 包围盒尺寸校验 + 黑名单优先级最高 + 单窗口动作上限 + 全量留痕审计（F14.6） |
| **界面语种与 OCR 模型不匹配导致乱码/误识别** | 界面语种与引擎语言不匹配时结果仅记录、**不触发点击**（F16.3 / F14.6） |
| **高 DPI 缩放导致 OCR/模板点击坐标偏移** | 截图坐标按窗口 DPI 换算（Win7 用 `GetDeviceCaps` 回退）；点击前校验目标坐标在窗口矩形内 |
| UAC 安全桌面拦截自动化点击 | UIA 无法作用于安全桌面；沙箱镜像关闭 UAC 弹窗或工具随系统自启于最高权限 |
| GUI 自动化误点（取消/卸载类按钮） | 黑名单关键词禁止点击（F14.3），**OCR 结果同样受黑名单约束**；全部键鼠动作留痕可回放审计 |
| GUI 自动化在无桌面会话（Session 0）失效 | 明确要求前台桌面可用；批量模式默认串行，避免抢焦点 |
| 自绘安装器无 UIA 树导致识别失败 | 图像模板兜底 + **外部 OCR 文本定位** + `--dump-ui`（含 OCR 结果）采集补模板 + 模板目录外部热更新 |
| 安装成功误判（退出码 0 但实际未装） | F15 多重证据（落盘/注册表/完成页）+ 退出后稳定期 |
| Go 生态调用 Windows API / COM | 使用 `syscall` + 成熟中继库，禁止 CGO；对 UIA / OCR 交互做接口隔离以便 mock 测试 |
| 工具自身被样本攻击 | 运行在沙箱内，崩溃不波及宿主机；建议工具与样本进程隔离 |
| EDR 与工具进程冲突 | 工具自身进程可加入 EDR 白名单，避免误报 |
| 跨平台抽象过度设计 | 本期只实现 Windows 适配层，接口保持最小可用，后期按需扩展 |
| 工具异常挂死阻塞流水线 | 全局看门狗（F9）+ 退出码区分崩溃（6.1） |

---

## 12. 待确认问题清单（开发前需澄清）

| 编号 | 问题 | 影响 |
|------|------|------|
| Q2 | `.dll` 触发用 rundll32 探测导出函数的数量与选取策略（按导出表顺序 / 关键字优先 / 随机）？ | 影响 F3 与 EDR 采集效果 |
| Q3 | 批量场景下单文件结束后是否需要"冷却间隔"（如 5 秒），让 EDR 完成行为上报落盘？ | 影响 F11 与 EDR 对接效果 |
| Q4 | 结果 JSON 的交付方式？（离线环境无上传通道：本地文件 / 共享目录 / 人工拷贝） | 影响 F10 输出通道 |
| Q5 | 是否需要保留同一文件多次任务的尝试历史（用于"触发方式优先级动态调整"）？ | 影响结果存储设计 |
| Q6 | 安装成功是否需要更强证据（如启动安装后的程序验证可用性）？ | 影响 F15 判定精度与工作量 |
| **Q7** | **OCR 引擎最终选型**：镜像统一预置 `PaddleOCR-json`（推荐，需 AVX）还是 `RapidOCR-json`（免 AVX）？是否需要同时预置两者以覆盖不同 CPU？ | 影响 7.6、F16.3 与镜像体积 |
| **Q8** | OCR 引擎的**版本与离线包来源**如何固定？（建议：内部归档一份发行包并记录版本号，避免随上游变更导致行为漂移） | 影响镜像构建与可复现性 |
| **Q9** | 是否需要保留 Windows 内置 OCR 作为回退后端？（Win10+ 环境零依赖，但语言包受限、实测仅中文可用） | 影响 F14.6 工作量 |
| **Q10** | **目标环境是否确实包含 Windows 7？** 若实际只在 Win10+ 运行，可改用 Go 1.21+（生态更新、库选择更多）并简化 4.6 的兼容处理 | 影响 7.1 工具链、4.6 与 A20 工作量 |
| **Q11** | 自绘安装器（Electron/CEF 类）在样本中的占比？若占比高，OCR 与模板的投入应先于 UIA 深化 | 影响 M2 优先级排序 |
| **Q12** | 目标虚拟机能否**为工具目录配置 AV/EDR 排除项**？（若不能，需考虑代码签名或改用其他分发方式） | 影响分发可行性与镜像构建流程（4.7 C 类） |

> 已决策项（不再讨论）：交付形态（免安装单文件）、语言（Go）、解压后端（外部 7-Zip，不降级、不代装）、7-Zip 缺失处理（检测 + 提示）、**OCR 引擎（外部开源引擎，默认 PaddleOCR-json，进程外调用，不代装）**、**平台基线（Win7 SP1 x64 及以上，Go 1.20.x）**、GUI 识别策略（UIA → 模板 → OCR 三级）、主目标（安装成功优先）、环境（离线）—— 见 1.2。

---

## 13. 后续可扩展方向（非本期范围）

- 实现 Linux 适配层（deb / rpm / sh / ELF 触发）
- 实现 macOS 适配层（pkg / dmg / Mach-O 触发）
- 支持自定义触发脚本插件（针对特定样本类型写触发逻辑）
- 支持与 EDR API 联动，触发后自动拉取行为报告（需环境提供通道）
- 支持触发方式优先级动态调整（基于历史成功率）
- 支持分布式批量触发（多沙箱并行）
- GUI 自动化模板库众包扩充（按安装器家族沉淀模板）
- **OCR 引擎抽象层扩展更多后端（如多语言/multi-model 引擎、轻量移动端模型）**
- **观察小参数 VLM OCR（HunyuanOCR 1B / DeepSeek-OCR 等）经 GGUF/llama.cpp 量化后在纯 CPU 的可用性**：若延迟能压到亚秒级且支持 Win7，可作为 `ocr.engine` 的新后端接入，用单模型替代"UIA + 模板 + OCR"组合（见 7.7 评估记录）
- OCR 结果与模板/UIA 的融合策略持续调优（按键位区域先验、按钮样式聚类）
- 触发成功率看板（按安装器家族统计安装成功 / 触发成功比例）

---

文档结束
