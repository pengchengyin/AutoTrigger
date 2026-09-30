# Windows 任意文件自动安装/触发工具 PRD

## 1. 文档信息

| 项目 | 内容 |
|------|------|
| 文档名称 | Windows 任意文件自动安装/触发工具 PRD |
| 版本 | v2.6（安装成功优先 · GUI 自动化 · **外部开源 OCR 引擎** · **光盘映像（ISO）专项处理** · **输出可解析（F10.1~F10.3）** · **安装成功判定分级（F15）+ 活动感知超时（F9）+ 第三方集成契约（F19）** · 离线 · Go · **Win7 SP1+ 兼容** · **工具本体单文件 + 完整交付包**） |
| 状态 | 待开发 |
| 目标平台 | **Windows 7 SP1 x64 / 8.1 / 10 / 11 / Server 2008 R2+**（架构预留跨平台扩展） |
| 运行环境 | 离线沙箱环境（工具不负责隔离，不负责检测） |
| 交付形态 | **两级模型**：① **工具本体** = 单个 `AutoTrigger.exe`，免安装、拷贝即用、无需任何语言运行时（用户使用极简）；② **交付包** = 工具本体 + **7-Zip / OCR 引擎等第三方离线安装包** + **部署说明文档** + 可选配置与模板（部署时必须整体提供）。详见 8.1 |
| 环境前置 | ① **7-Zip** ② **OCR 引擎**（默认 PaddleOCR-json）（由镜像/运维按**部署说明文档**预装；**离线安装包随交付包提供**）。工具启动自检，缺失时提示并指向交付包内安装包，**工具自身不代为安装**；另含管理员权限、关闭 UAC、AV 排除目录等镜像准备项，**完整边界见 4.7** |
| 下游依赖 | EDR（负责行为采集与检测） |
| 结果输出 | 结果 JSON（原子写、字段完整、可过 Schema）+ 日志；**stdout 仅机器可读输出、日志走 stderr**，下游可直接解析（F10.1~F10.3、验收 A28） |
| **结果可信度** | 安装成功判定采用**证据分级（S/A/B）+ 基线快照 diff + 反证据**，结果附 `install_confidence`（high/medium/low）；**明确声明能力边界**——判定的是"安装器是否完成了安装动作"，**不等于"软件是否可正常使用"**（F15.8） |
| **第三方集成** | 对外暴露**稳定集成契约**：CLI 参数、退出码语义、stdout/stderr 通道、结果 Schema **只做加法**；提供 `--capabilities --json` 能力协商与单实例锁；**第三方以结果 JSON 为唯一判定依据**（F19、验收 A31） |

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
| C28 | **新增 F17 软件包结构识别与安装入口优先级排序**：压缩包/目录先做结构分析，按 P0~P5 分层（说明文件明示入口 → 安装入口命名 → 标准安装器 → 前置依赖 → 主程序 → 其余），**先 `7z l` 列清单再解压**；新增**排除清单**（卸载/修复/更新/调试类**绝不执行**）；执行策略为**阶段 1 定向优先 + 阶段 2 兜底回退**（非"只跑 install.bat 就不再尝试"） | ① 正确性：盲扫会把 `unins000.exe` 一并执行，**先跑卸载会破坏安装结果**；② 效率：真实入口通常仅 1~5 个，盲扫数百文件浪费且噪音大 |
| C29 | **新增 F18 光盘映像专项处理**：ISO 不再按普通压缩包处理，改为 ① **先判定介质性质**（`malware_lure` 恶意投递结构 / `installer_media` / `os_install_media` / `media_disc` / `data_disc`，**操作系统安装介质与影碟默认不触发**）；② **解压为主、挂载为辅的双轨**（默认 `extract_first`，7-Zip 解析失败自动回退挂载；Win8+ 用系统 `Mount-DiskImage`，Win7 需可选第三方 WinCDEmu）；③ **`autorun.inf` 提升为 P0 入口信号**；④ **MotW 策略可配置**（默认 `native` 不主动改写）；⑤ 新增可选前置探测 F16.6 | ① 7-Zip 对 UDF/蓝光的解析有缺口，仅解压会漏样本；② ISO 内若为操作系统安装盘，执行 `setup.exe` 会**破坏测试虚拟机**；③ ISO + LNK 是典型投递手法，属 EDR 测试重点关注场景 |
| C30 | **定案「输出通道与结果可解析性」**：修正文档中"日志写 stdout"与"禁止污染 stdout 干扰结果解析"的**自相矛盾**（原表述会使 `--dump-config > config.json` 产出非法 JSON）；改为**三通道严格分流**（stdout 仅机器可读输出 / stderr 仅人类可读日志 / 文件承载结果与日志），补充 `--output` 默认路径、`--output -`、`--log-file`；新增结果文件的**可解析性硬要求**（原子写、固定字段恒存在、失败路径也产出结构完整产物、无尾随文本）与**中断留存 journal**（F10.1~F10.3，验收 A28） | 下游流水线以结构化结果驱动，输出通道语义必须唯一确定；原表述存在实现歧义 |
| C31 | **新增安装成功证据分级与基线快照**：原 F14.4/F15 的"四类证据满足任一即成功"过于宽松——安装器退出码 0 存在大量假成功（bootstrapper 解压后立即退出、部分 InstallShield 恒返回 0），且"出现**新增**文件 / **新增**卸载项"在原文中**未定义基线**，无从判定"新增"。改为：① 证据分 **S/A/B 三级**（S：注册表卸载项 diff / 文件系统落盘 diff / MSI 官方成功码 / `MsiInstaller` 事件 11707；A：通用退出码 0 / 完成页被点击；B：安装日志成功关键字）；② **判定前必须先做基线快照**（F15.2）：注册表三处 Uninstall 根（HKLM + Wow6432Node + **HKCU**）+ 文件系统 `watch_dirs` + **`MsiInstaller` 事件日志水位线**；③ `install_completed` 收紧为"**至少 1 条 S 级证据，或 ≥2 条 A 级证据**"；④ 仅弱证据不再判成功，新增状态 `install_unverified` | 原验收口径会让"假成功"被计入已安装，直接污染下游判定 |
| C32 | **新增反证据机制与安装后校验**：原方案只收集正面证据，无法识别"看着成功实则失败/回滚"。新增 **F15.3 反证据**（失败类退出码、GUI 失败页与回滚提示、`MsiInstaller` **11724 卸载事件**、卸载键消失、`watch_dirs` 文件数减少）并规定**反证据优先于正面证据**，冲突时降级为 `install_partial` 并写 `evidence_conflicts`；新增 **F15.4 安装后校验**（校验卸载项指向的主程序存在且为有效 PE，`launch_check` 默认关闭）；新增 **F15.5 已安装检测**（`pre_installed`），解决第三方重复调用时的假阴性 | 结果可靠性核心：只报喜不报忧等于不可用 |
| C33 | **超时策略改为「活动感知 + 分阶段」**：原 F9 仅有单文件固定 120s 超时。安装耗时差异极大（Inno/NSIS 数秒~1 分钟，Office/VS 类 10 分钟以上），固定超时必然"该等的没等、该杀的没杀"。改为四段式：**启动期超时**（进程无任何动作）/ **静默期超时**（无进展信号才计时）/ **单文件硬上限**（默认 600s）/ **任务级上限**；进展信号包括 CPU 时间增长、目标目录写入、子进程新增、窗口与进度变化、注册表变化；超时后**先优雅关闭再终止进程树**，且**超时后仍执行 F15 证据采集**（可能已部分安装） | 直接决定"安装时间过长"时是等还是杀，以及杀完之后结果怎么报 |

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
| DC15 | **输出通道严格分流**：**stdout 只承载机器可读输出**（`--dump-config` 的配置 JSON、`--output -` 的结果 JSON），**日志一律走 stderr + 日志文件**，绝不写 stdout；结果 JSON 采用**原子写**、固定字段恒存在、失败路径亦产出结构完整产物，保证下游可无条件解析（详见 F10.1~F10.3、验收 A28） | F10.1、6、7.1、3.3 |
| DC16 | **安装成功判定以「证据分级 + 基线 diff」为准**：`install_completed` 需 **≥1 条 S 级证据或 ≥2 条 A 级证据**；仅弱证据判 `install_unverified`；检测到明确未完成/回滚判 `install_partial`；结果附 `install_confidence`。**能力边界明确声明**：本工具判定的是"安装器是否完成了安装动作"，**不等于"软件可正常使用"**（依赖缺失、需重启、需激活等不在判定范围） | F15、F10、A29 |
| DC17 | **超时采用活动感知（无进展才计时）**：默认 `startup_timeout_sec: 30`、`idle_timeout_sec: 60`、`timeout_sec: 600`（单文件硬上限）、`task_timeout_sec: 0`（任务级，0 = 不限）；超时后**先优雅关闭、再终止进程树**，且**仍采集安装证据** | F9、7.5、A30 |
| DC18 | **对第三方暴露稳定集成契约**：CLI 参数、退出码语义、stdout/stderr 通道、结果 JSON Schema **只做加法**（枚举只增不改、固定字段恒存在、破坏性变更才升主版本）；单实例锁防并发实例互相污染安装证据 diff；`--capabilities --json` 供调用方做能力/版本协商；**调用方以结果 JSON 为唯一判定依据，不得用退出码推断单文件结果** | F19、A31 |

---

## 2. 背景与目标

### 2.1 背景

软件供应链安全检测部门需要对**入网的任意文件**进行动态行为检测。样本进入沙箱后，缺少一个能够**自动识别文件类型并尝试各种安装/执行方式**的触发工具。检测环节由沙箱内已有的 EDR 负责，本工具只负责"让文件跑起来"。

### 2.2 目标

开发一个运行在 Windows 沙箱内的自动化触发工具，接收**指定的任意文件或文件夹**，自动识别文件类型，**以尽可能完成安装为首要目标**：

1. 优先轮询多种静默安装参数，尽量无人值守装成；
2. 若出现安装向导弹窗，自动识别安装相关按钮/输入项，模拟键鼠完成"同意 → 下一步 → 安装 → 完成"操作链；
3. 尽量多尝试几种方式（静默参数 ↔ 无参数运行）直到取得**安装成功证据**；
4. 安装与触发全程保证样本行为暴露，供 EDR 采集；
5. **每次安装都给出可复核的判定**：证据分级 + 置信度（`install_confidence`），明确区分"装成了 / 疑似装成 / 装了一半 / 本来就装着 / 没装成"；
6. **输出稳定可解析**，且超时、中断、崩溃等异常路径同样产出完整结果——**第三方脚本可直接消费，不需要人工解读**。

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
- **不提供绕过 Windows 安全机制（SmartScreen / MotW 等）的实现**：光盘映像的 MotW 处理仅做**如实记录与可配置的传播行为**（`native` / `preserve` / `clear`，F18.4），"是否让 SmartScreen 参与"由安全团队的测试口径决定，工具不代为决策
- **不触发操作系统安装介质**：含 `sources\install.wim` 等特征的映像默认跳过（F18.1），避免把测试虚拟机装成操作系统
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
| 压缩包递归触发 | 传入 `.zip` / `.7z` / `.rar`，工具用 7-Zip 解压后按 F17 分层递归触发内部可执行文件 |
| **光盘映像触发** | 传入 `.iso`：先判定介质性质 → 解压轨（默认）或挂载轨 → 按 F17 优先级执行；`autorun.inf` 声明与 `.lnk` 目标按 P0 优先 |
| **恶意投递型 ISO** | 映像内仅一个 `.lnk` + 载荷（典型投递结构）：判定为 `malware_lure` 并最高优先执行，供 EDR 采集链式行为 |
| 操作系统安装盘 | 传入 Windows / Linux 安装 ISO：判定为 `os_install_media` 并**默认跳过**，避免在测试机上误装操作系统 |
| 文档类触发 | 传入 `.docx` / `.pdf`，工具调用关联程序打开，触发宏/漏洞利用链 |
| 未知格式兜底 | 传入扩展名未知或文件头与扩展名不符的文件，工具仍尝试执行 |
| 前置缺失 | 目标机未装 7-Zip / OCR 引擎：工具检测后提示用户先安装，不尝试自行安装 |

### 3.3 使用方式

命令行调用为主，便于集成到自动化流水线：

```
AutoTrigger.exe --input "C:\samples\test.exe" --timeout 900 --output "C:\logs\result.json"
AutoTrigger.exe --input "C:\samples\folder" --recursive --timeout 900
AutoTrigger.exe --input "C:\samples" --config "C:\conf\config.json"
AutoTrigger.exe --capabilities --json    # 输出能力清单（版本/枚举/前置可用性），供第三方集成协商
AutoTrigger.exe --input "C:\samples" --output - > result.json   # 结果 JSON 走 stdout，可直接管道消费
AutoTrigger.exe --input "C:\samples" --output - | jq .summary   # 下游流水线直接解析
AutoTrigger.exe --input "C:\samples" --log-file "C:\logs\run.log" --log-level debug
AutoTrigger.exe --dump-config > config.json   # 导出内嵌默认配置，作为配置模板（stdout 只有该 JSON）
AutoTrigger.exe --check-prereq          # 仅做环境自检（7-Zip / OCR 引擎），不处理样本
AutoTrigger.exe --check-prereq --json   # 自检结果以 JSON 输出到 stdout，供镜像构建流水线卡点
AutoTrigger.exe --dump-ui --input "C:\samples\a.exe"   # 调试：导出窗口截图 + UIA 控件树 + OCR 结果，用于扩充模板
AutoTrigger.exe --no-archive --input "C:\samples"      # 无 7-Zip 时跳过压缩包，继续处理其余样本
AutoTrigger.exe --no-ocr --input "C:\samples"          # 无 OCR 引擎时强制仅用 UIA + 模板
```

**输出通道约定**：`stdout` **只承载机器可读输出**（`--dump-config` 的配置 JSON、`--output -` 的结果 JSON、`--check-prereq --json` 的自检 JSON），**所有日志与提示一律走 `stderr` 与日志文件**——因此上面的管道用法可直接被下游解析。详见 **F10.1**。

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
    Snapshot(ctx InstallContext) (*Baseline, error)         // 安装前基线快照（注册表三处 Uninstall / watch_dirs / 事件水位线），F15.2
    Verify(ctx InstallContext, base *Baseline) (*InstallEvidence, error)  // 证据分级 S/A/B + diff + 反证据，F15
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
| **光盘映像挂载** | **`Mount-DiskImage` / `Dismount-DiskImage` 为 Win8 / Server 2012+ 的 Storage 模块**，Win7 没有 | Win7 默认走**解压轨**（`iso.strategy: extract_first`）；若需挂载轨则依赖可选第三方 WinCDEmu 便携版（F16.6） |
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
| **B. 功能性前置（可选）** | **光盘映像挂载后端**（Win8+ 系统自带；Win7 需第三方 WinCDEmu） | 仅当使用 `iso.strategy: mount_*` 时需要 | 挂载轨不可用，自动收敛为解压轨 | 检测 + 提示 + 自动收敛（F16.6）；**不新增退出码** |
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
| F17 | **软件包结构识别与安装入口优先级排序** | P0 | 无关 | M2 |
| F18 | **光盘映像专项处理（ISO/IMG）：介质性质判定 + 解压/挂载双轨** | P0 | 适配层 | M2 |
| F19 | **第三方集成契约（CLI/Schema 稳定面、能力协商、单实例锁、契约测试）** | P0 | 无关 | M1（契约冻结）/ M3（契约测试） |

---

### 5.2 功能详细说明

#### F1 输入接收与校验

- 支持 `--input` 指定单个文件或文件夹路径
- 支持 `--recursive` 递归遍历文件夹
- 支持 `--timeout` 指定单个文件的**硬上限**时长（秒），默认 600（`0` = 不限）；**它不覆盖静默期超时与任务级超时**（见 F9.1）
- 支持 `--output <path>` 指定结果输出路径（JSON）：默认 `.\autotrigger-result.json`；`--output -` 表示输出到 stdout；父目录不存在时自动创建
- 支持 `--log-file <path>` 指定日志文件（默认与结果文件同名 + `.log`）、`--log-level <debug|info|warn|error>` 控制日志级别
- 支持 `--config` 指定外部配置文件路径（覆盖内嵌默认值，见 7.5）
- 支持 `--check-prereq` 仅执行环境自检（`--json` 时输出机器可读自检结果）、`--dump-config` 导出生效配置、`--no-archive` 跳过压缩包处理、`--no-ocr` 跳过 OCR
- 支持 **`--capabilities --json`** 输出机器可读能力清单（供第三方做版本/能力协商，见 F19.3）、**`--version`** 输出版本与 `schema_version`、**`--lock` / `--no-lock`** 控制单实例锁（见 F19.4）
- **未知命令行参数以退出码 2 失败并提示参数名**（命令行是精确接口，不得静默忽略——与配置文件的"未知字段忽略"刻意不同，见 F19.1）
- 外部配置的加载、合并与校验规则见 **7.5.1**（深合并、未知字段忽略、类型/越界以退出码 3 失败）
- 输入路径不存在或无权限时：记录错误、输出**空结果 JSON（字段完整、`results: []`、`error` 写明原因）+ 错误摘要**（保证下游始终有产物可解析）、退出码 `2`
- 输入为空文件（0 字节）：跳过并记录 `skipped: empty_file`
- **输出通道与结果可解析性要求见 F10.1~F10.3**（stdout 仅机器可读输出、日志走 stderr、结果文件原子写）

#### F2 文件类型识别

识别优先级：**文件头（Magic Number）→ 扩展名 → 综合判断**

> 文件头优先于扩展名；仅当文件头无法识别时才参考扩展名归类。

| 类别 | 扩展名 | 文件头特征 |
|------|--------|-----------|
| PE 可执行 | .exe .dll .scr .com | `MZ` |
| MSI 安装包 | .msi .msp | OLE Compound File |
| 脚本 | .bat .cmd .ps1 .vbs .js .wsf | 文本特征 + 解释器关键字 |
| 压缩包 | .zip .rar .7z .tar .gz | 各格式魔数 |
| **光盘映像** | .iso .img .nrg .mds .cue | ISO9660 / UDF（`CD001` 等）；**走 F18 专项流程**，不按普通压缩包处理 |
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
2. **解压前先 `7z l` 列清单（不落盘）**，交由 **F17** 完成结构识别与入口分层，并预判解压后总大小是否超限
3. 解压到临时目录（目录名含任务 ID）
4. **按 F17 给出的优先级顺序**处理解压后的文件（而非盲目全量平扫）；每个文件重新走 F2 类型识别 → 对应触发逻辑
5. **递归深度限制：默认 3 层（可配置）**，防止无限嵌套
6. **解压后总大小限制：默认 2 GB（可配置）**，超限停止并记录 `extract_size_exceeded`（防 zip 炸弹）
7. 解压失败时记录 `extract_failed` 与 7z 输出摘要，不阻塞后续文件
8. 取得 `install_completed` 后按 **F17.4 提前结束该包**，其余文件记 `skipped_after_success`
9. 退出前清理临时目录（清理失败仅告警）
10. `--no-archive` 或 7-Zip 缺失时跳过压缩包处理，相关文件记 `skipped: prereq_missing`（见 F16）
11. **光盘映像（`.iso` / `.img` 等）不走本流程**——其只读介质语义、`autorun.inf` 入口、"操作系统安装介质"风险与 UDF 解析缺陷需专项处理，见 **F18**

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

#### F9 超时、进程清理与时长控制

**核心原则：超时判定的依据是"有没有进展"，而不是"过了多久"。** 固定墙钟超时对本工具是错误模型——同一个 120 秒，对 NSIS 安装器是大方，对 Office / VS / 大型游戏载体是苛刻；而真正该杀的是"卡住不动"的进程，不是"慢慢在装"的进程。

**F9.1 四段式超时**

| 阶段 | 配置项 | 默认 | 语义 | 触发后的判定 |
|------|--------|------|------|-------------|
| 启动期 | `startup_timeout_sec` | 30 | 进程创建后**没有任何动作**（无窗口、无子进程、无文件写入、CPU 时间几乎不增长） | 视为启动失败，该次尝试记 `failed: no_activity` |
| **静默期** | `idle_timeout_sec` | 60 | **连续 N 秒无任何进展信号**（F9.2） | 判定为卡死，进入 F9.3 终止流程 |
| 单文件硬上限 | `timeout_sec` | **600** | 单文件的**挂号总时长**（含全部参数尝试，非单次尝试） | 强制终止，记 `killed_on_timeout` |
| 任务级 | `task_timeout_sec` | 0（不限） | 整个批量任务的总时长上限 | 优雅中止后续文件、写 journal、以退出码 0 结束（未处理的记 `skipped: task_timeout`） |

> 原 v2.5 的 `timeout_sec: 120` 语义是"单文件总超时"，实测口径偏紧且模型不对，**v2.6 改为单文件硬上限 600s，真正的把关交给静默期超时**。`timeout_sec: 0` 表示单文件不限时（供明确知道某软件很慢的调用方使用）。

**F9.2 进展信号（任一出现即重置静默期计时器）**

| 信号 | 采集方式 | 说明 |
|------|---------|------|
| CPU 时间增长 | `GetProcessTimes`（进程树内所有进程求和） | 最可靠的"还在干活"指标；**推荐主信号** |
| 目标目录写入 | `ReadDirectoryChangesW` 监视 `install_verify.watch_dirs` + 安装目录 | 大文件拷贝阶段 CPU 占用低但文件在增长 |
| 子进程新增 / 退出 | Job Object 通知 + 进程树轮询 | 安装器常 spawn 子安装器（bootstrapper 模式） |
| GUI 变化 | F14 窗口轮询结果（新窗口、控件文本变化、进度条值变化） | 向导推进中 |
| 注册表变化 | 卸载键子键数量变化 | MSI 事务中途 |
| 磁盘 I/O | `GetProcessIoCounters` 字节数增长 | 兜底信号 |

> 设计取舍：**不监控网络**（离线环境无意义）。信号判定阈值可配（`activity_epsilon`），避免把"极小波动"误判为进展。

**F9.3 超时后的处置顺序（先优雅，后强杀）**

1. **优雅请求（3 秒）**：GUI 场景先向安装向导窗口发送 `WM_CLOSE`；控制台程序尝试 `GenerateConsoleCtrlEvent(CTRL_BREAK)`。给安装器走正常清理/回滚逻辑的机会，**避免留下半损坏的安装状态**
2. **温和终止（3 秒）**：`TerminateProcess` 主进程，等待其子进程自行退出
3. **强制终止**：通过 **Job Object 终止整棵进程树**（含脱离的子进程），确保无遗留
4. 每一步的实际结果（`graceful` / `terminated` / `killed_tree`）写入该 attempt 的 `termination_mode`

> 原表述为"结束该文件及其派生进程树"，未区分优雅与强杀。**直接强杀会让 MSI 留下待回滚事务，进而污染后续样本的注册表证据**，因此顺序必须写死。

**F9.4 超时后仍然要判定"装成了没"**

超时 **≠** 没装成。被超时的安装器可能：已完成主体安装只是最后一步卡在"完成"页、或已完成落盘但被"是否重启"对话框挂住。因此：

- 超时终止后**必须照常执行 F15 证据采集**（含基线 diff 与 `settle_sec` 稳定期）
- 若采集到 S 级证据 → `install_completed`（`notes` 注明"超时终结但已取得安装证据"）
- 若有落盘/注册表副作用但未达完成标准 → `install_partial`
- 完全无副作用 → `killed_on_timeout`（沿用原口径，计入触发成功，行为已被 EDR 采集）

**F9.5 进度可观测性**

- 长任务在 **stderr** 每 `progress_interval_sec`（默认 30）输出一次心跳：当前文件、已用时长、最近一次进展信号类型与时间、静止时长
- **心跳绝不写 stdout**（F10.1 红线）
- 心跳的用途是让调用方能区分"工具在处理慢任务"与"工具挂了"——这是第三方集成时最常见的困惑点

**F9.6 协作式中止**

- 收到 `Ctrl+C` / `SIGINT`（或 Windows 控制台关闭事件）时：停止派发新文件 → 对当前文件走 F9.3 流程 → 采集当前证据 → 写 journal → 原子写结果 JSON（`finished_at` 正常填写、`error.code = 130`）→ 退出码 `130`
- 保证"**任何时刻被打断，都有一份可解析的结果产物**"（与 F10.3 呼应）

**F9.7 进程树与资源清理**

- 工具退出前清理所有临时解压目录、卸载本次挂载的光盘映像（F18.3）、回收常驻 OCR 引擎进程、清理原子写遗留 `.tmp`
- **全局看门狗**（兜底，非主机制）：单文件累计时长不得超过 `timeout_sec + 60s`，防止实现缺陷导致挂死阻塞批量任务；看门狗触发时按 F9.3 强制终止并记 `failed: watchdog`（属工具异常，需排查）
- GUI 弹窗处理与 OCR 识别耗时**计入静默期重置**（F14 正在点击 = 有进展），但不豁免单文件硬上限

#### F10 执行结果记录与输出

输出 JSON：任务级汇总 + 文件明细数组，仅写本地文件。**通道划分与可解析性要求见 F10.1~F10.3（P0，下游流水线依赖）**。

```json
{
  "schema_version": "2.7",
  "task_id": "uuid",
  "config_digest": "sha256:...",
  "error": null,
  "started_at": "2026-09-30T09:40:00+08:00",
  "finished_at": "2026-09-30T09:52:31+08:00",
  "platform": "windows",
  "platform_meta": { "os_version": "6.1.7601", "dpi_awareness": "system", "cpu_avx": true },
  "prereq": {
    "archive_backend": "C:\\Program Files\\7-Zip\\7z.exe",
    "sevenzip_version": "25.01",
    "archive_backend_status": "ok",
    "mount_backend": "system",
    "mount_backend_status": "ok",
    "ocr_engine": "paddleocr_json",
    "ocr_engine_path": "C:\\tools\\PaddleOCR-json\\PaddleOCR-json.exe",
    "ocr_engine_status": "ok",
    "ocr_engine_languages": ["ch", "en"],
    "ocr_startup_ms": 680
  },
  "summary": {
    "total": 50,
    "by_status": {
      "installed": 12,
      "unverified": 3,
      "partial": 2,
      "already_installed": 1,
      "triggered": 24,
      "timeout": 5,
      "failed": 2,
      "skipped": 1
    },
    "flags": {
      "requires_reboot": 2,
      "evidence_conflict": 1,
      "pre_installed": 1,
      "artifact_check_failed": 1
    }
  },
  "layout_analysis": {
    "input_kind": "iso",
    "media_kind": "installer_media",
    "iso_strategy_used": "extract",
    "mount_drive": null,
    "dismount_status": "not_applicable",
    "motw_policy": "native",
    "entries": [
      { "path": "install.bat", "priority": "P1", "reason": "entry_p1_named" },
      { "path": "setup.exe", "priority": "P2", "reason": "entry_p2_installer" }
    ],
    "excluded": [
      { "path": "unins000.exe", "reason": "excluded_blacklist", "matched": "unins0" }
    ],
    "fallback_used": false,
    "converged_at": "setup.exe",
    "skipped_after_success": 37
  },
  "results": [
    {
      "file_path": "C:\\samples\\test.exe",
      "file_size_bytes": 10485760,
      "file_hash_sha256": "...",
      "detected_type": "pe_executable",
      "selection_reason": "entry_p2_installer",
      "extension_disguised": false,
      "pre_installed": false,
      "requires_reboot": false,
      "platform_meta": {},
      "attempts": [
        {
          "method": "silent_install",
          "command": "C:\\samples\\test.exe /VERYSILENT /NORESTART",
          "result": "exited",
          "exit_code": 0,
          "duration_sec": 32,
          "termination_mode": "natural",
          "idle_resets": 7,
          "install_evidence": [
            { "id": "A1", "level": "A", "type": "generic_exit_code_zero", "source": "process", "detail": "非 MSI 来源退出码 0（bootstrapper 可能假成功）", "at": "2026-09-30T09:40:32+08:00" }
          ]
        },
        {
          "method": "gui_automation",
          "command": "C:\\samples\\test.exe（无参数 + 弹窗接管）",
          "result": "exited",
          "exit_code": 0,
          "duration_sec": 45,
          "termination_mode": "natural",
          "idle_resets": 11,
          "install_evidence": [
            { "id": "S1", "level": "S", "type": "registry_uninstall_key", "source": "hklm_wow6432", "detail": "新增卸载项 {5F1A...} DisplayName=\"Test App\"", "at": "2026-09-30T09:41:35+08:00" },
            { "id": "S4", "level": "S", "type": "msiinstaller_event", "source": "eventlog", "detail": "EventID 11707 安装完成", "at": "2026-09-30T09:41:36+08:00" },
            { "id": "A2", "level": "A", "type": "gui_finish_clicked", "source": "uia", "detail": "点击「完成」", "at": "2026-09-30T09:41:30+08:00" }
          ],
          "artifact_check": "ok",
          "artifact_runnable": null,
          "gui_actions": [
            { "window": "Setup - Test", "control_text": "我接受", "source": "uia", "action": "click", "at": "2026-09-30T09:41:02+08:00" },
            { "window": "Setup - Test", "control_text": "安装", "source": "template", "action": "click", "at": "2026-09-30T09:41:15+08:00" },
            { "window": "Setup - Test", "control_text": "下一步", "source": "ocr", "ocr_score": 0.94, "recognize_ms": 210, "action": "click", "at": "2026-09-30T09:41:22+08:00" }
          ]
        }
      ],
      "evidence_conflicts": [],
      "install_confidence": "high",
      "final_status": "install_completed",
      "notes": ""
    }
  ],
  "truncated": false
}
```

> `error`：仅在未正常完成时给出对象（`{"code": 2, "message": "input path not found: C:\\samples"}`），正常为 `null`；`truncated`：结果被体积上限截断时为 `true`（见 F10.2 R9）。两者均为**恒存在的固定字段**（R2）。

**final_status 枚举（v2.6 依据 F15.7 决策表重定义）**：

| 取值 | 定义 | `by_status` 归类 | 可信度 |
|------|------|-----------------|--------|
| `install_completed` | **≥1 条 S 级证据**（或 ≥2 条 A 级且含 A1/A2），且无反证据 | `installed` | high / medium |
| `install_unverified` | 仅弱证据（单条 A 级或 B 级），无反证据 —— **疑似装成但未经证实** | `unverified` | low |
| `install_partial` | 有安装副作用但存在反证据（失败页 / 回滚 / 卸载事件），**明确未完成** | `partial` | low |
| `already_installed` | 基线中已存在该软件，本次 diff 为空 —— **环境本就具备** | `already_installed` | high |
| `triggered` | 进程成功启动并运行结束，diff 为空且非已安装（非安装类样本的正常结果） | `triggered` | — |
| `killed_on_timeout` | 进程成功启动但被超时终结，且无任何安装副作用 —— 行为已被 EDR 采集，视为有效触发 | `timeout` | — |
| `failed` | 所有方式均无法启动进程；或存在反证据且无正面副作用 | `failed` | — |
| `skipped` | 无可用触发方式（空文件 / 文档无宿主程序 / 前置依赖缺失 / 任务级超时未处理等） | `skipped` | — |

**summary 结构说明（v2.6 重组，消除原有歧义）**：

- `by_status`：**互斥桶**，恒满足 `installed + unverified + partial + already_installed + triggered + timeout + failed + skipped == total`
- `flags`：**非互斥计数**，可与其他桶叠加，用于快速筛出需关注的记录
- 原 v2.5 的平铺写法无法判断 `triggered` 是否已包含 `timeout`，故重组为两级结构；下游迁移时按 `by_status.*` 取值

> 枚举变更说明：v2.0 起将 `killed_on_timeout` 并入触发成功口径；v2.1 新增 `install_completed`；v2.3 起 `gui_actions` 记录识别来源 `source`；**v2.4 起记录 OCR 原生 `ocr_score` 与识别耗时，并把 `prereq` 扩展为完整的引擎探测结果**；**v2.5 起 `prereq` 增加挂载后端探测结果，`layout_analysis` 增加 F18 的光盘映像字段**；**v2.6 起按 F15.7 重定义枚举（新增 `install_unverified` / `install_partial` / `already_installed`），`install_evidence` 由字符串升级为结构化数组（含 `id` / `level` / `type` / `source` / `detail` / `at`），新增 `install_confidence` / `evidence_conflicts` / `pre_installed` / `requires_reboot` / `artifact_check`，`summary` 重组为 `by_status` + `flags`**。

**F10.1 输出通道划分（三通道严格分流）**

工具的输出**只有三条通道，职责互不重叠**：

| 通道 | 承载内容 | 说明 |
|------|---------|------|
| **stdout** | **仅机器可读输出**：① `--dump-config` 的配置 JSON；② `--output -` 时的结果 JSON；③ `--check-prereq --json` 的自检结果 JSON。**其余情况 stdout 为空** | 可直接管道给下游程序（`jq` / 流水线解析），**保证解析零干扰** |
| **stderr** | **仅人类可读日志**（时间戳、级别、阶段、进度、提示）。默认级别 `info`，由 `--log-level` / `output.log_level` 控制 | 进度与统计摘要**只出现在这里**，绝不写 stdout |
| **文件** | ① 结果 JSON（`--output` 指定，**默认 `.\autotrigger-result.json`**）；② 日志文件（`--log-file`，默认与结果文件同名 + `.log`） | 结果文件路径与日志文件路径均在 stderr 与日志中打印绝对路径，便于排障 |

> ⚠️ **实现红线**：**任何日志、进度条、统计摘要都不得写入 stdout**。这条是"结果可解析"的前提——历史上正是"日志写 stdout"与"禁止污染 stdout"两条表述并存，导致 `AutoTrigger.exe --dump-config > config.json` 会产出非法 JSON。验收 A28 第 ① 条专卡此项。

**结果输出路径的取值规则**：

| 取值 | 行为 |
|------|------|
| `--output <绝对/相对路径>` | 写入指定文件（父目录不存在时**自动创建**） |
| `--output -` | 结果 JSON 写入 **stdout**（此时 stdout 只有这一个 JSON 文档），供管道直接消费 |
| 未指定 | 默认写入**当前工作目录** `.\autotrigger-result.json`；**默认路径不得落在输入目录内**，避免下一轮遍历把结果文件当成输入样本 |
| 结果文件名冲突 | 默认名已存在时**直接覆盖**（同一工作目录的重复运行视为同一任务位置）；需要并存时由调用方用 `--output` 指定 |

**F10.2 结果 JSON 可解析性硬要求（P0）**

| 编号 | 要求 | 原因 |
|------|------|------|
| R1 | **UTF-8 无 BOM**，输出**单个完整 JSON 文档**（非 JSONL / 非拼接） | BOM 会让部分解析器首字符报错 |
| R2 | **固定字段恒存在**，无值一律用 `null`，**不用"缺字段"表达无值** | 下游与 Schema 校验无需判空字段缺失 |
| R3 | 数字为 JSON number（不写成字符串）；时间为 **RFC3339 带时区**；路径按 JSON 规则转义反斜杠 | 避免"看起来像 JSON 实际要二次清洗" |
| R4 | **无尾随文本**：文件末尾不得追加统计摘要、耗时、提示语 | 尾随内容会使严格解析器失败 |
| R5 | **原子写**：先写 `<path>.tmp` → flush + fsync → `os.Rename` 覆盖目标文件 | 被超时强杀/崩溃时不会留下半截 JSON |
| R6 | **失败路径同样产出结构完整的结果 JSON**：输入非法（退出码 2）、配置错误（3）、前置缺失（4/5）均输出**字段完整、`results` 可为空数组**的结果，并在 `error` 字段写明原因 | 保证下游"永远有可解析产物"，不因失败分支拿到 0 字节文件 |
| R7 | `finished_at` 为 `null` 表示**任务未正常结束**（被强杀或崩溃） | 与 F10.3 的 journal 配合判断中断 |
| R8 | 输出格式由 `output.pretty` 控制，**默认 `false`（紧凑单行）**；`--pretty` 可切换为缩进格式（仅供人工阅读，两者语义完全等价） | 紧凑格式体积小、便于整体读取 |
| R9 | 结果文件体积上限 `output.max_file_size_mb`（默认 64）：超限时**截断 `attempts` 明细**（保留 `final_status` 与汇总）并置 `truncated: true` | 千级文件 + `gui_actions` 明细会显著膨胀；截断不改结构、不破坏解析 |
| R10 | 必须可通过 **D7 的结果 JSON Schema** 校验，且能被 `json.Unmarshal` 一次性解析成功 | 可解析性的最终判据 |

**F10.3 中断留存 journal（可选，保证长任务不丢数据）**

批量任务可能运行数小时（数千文件 × 单文件超时）。若在末尾崩溃，仅靠"结束时一次性写结果"会**丢掉全部已完成记录**。因此：

- `output.journal`（默认 `true`）：在结果文件旁写 **`<结果文件名>.journal.ndjson`**，每完成一个文件**追加一行 NDJSON**（该文件的完整结果对象），逐行 flush
- journal **只作中断诊断与数据找回**，**不是主结果**：主结果仍是 F10.2 的单一 JSON 文档
- 任务正常结束且结果 JSON 原子写成功后：`output.keep_journal_on_success`（默认 `false`）→ **删除 journal**（符合 6 节"无残留"要求）；置 `true` 则保留供审计
- 任务异常中断（被强杀/崩溃）时：结果 JSON 可能不存在或 `finished_at: null`，此时 journal 保留，**下游可解析 NDJSON 找回已完成文件的结果**
- `output.journal: false` 时完全不写 journal（纯单文件产物，适合结果文件必然完整的小批量场景）

> 下游推荐的判定顺序：① 读结果 JSON（存在且可解析）→ 以它为准；② 若结果缺失或 `finished_at: null` → 回退读 `.journal.ndjson`，并按其中 `task_id` 与 `schema_version` 校验归属。

**结果文件的 Schema 兼容策略**：

- `schema_version` 为**语义化字符串**（当前 `"2.7"`）；**只做加法**（新增字段）不视为破坏性变更
- **破坏性变更**（删除字段、改字段含义、改枚举取值）才升主版本号，并在第 1 节变更记录中说明迁移方式
- **下游必须容忍未知字段**（忽略而非报错），与本工具对配置文件的兼容策略一致（7.5.1 ③）
- **退出码与结果 JSON 是两条独立通道**：退出码只表示"工具自身"的状态（6.1），单文件结果**只在 JSON 里**；下游判定样本结果时**以 JSON 为准**，不要用退出码推断（批量场景下退出码 0 不等于全部装成功）

#### F11 文件夹批量遍历

- 递归遍历输入目录下所有文件（`--recursive` 控制）
- **遍历前先由 F17 做结构识别与入口分层**——一个目录本身就是"软件包"，优先处理识别出的安装入口，取得 `install_completed` 后提前结束该目录
- 按类型排序处理：PE/安装包优先 → 脚本 → 文档 → 未知最后（F17 命中的入口整体前置）
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

**F14.4 GUI 视角的完成信号（不再单独决定最终状态）**

GUI 自动化只能观察到"界面走到哪一步"，**这不是安装成功的充分证据**，仅作为 F15 判定中的 A2 级/反证据 N2 来源：

| 界面信号 | 对应 F15 证据 |
|---------|-------------|
| 点击了"完成 / Finish"向导页 | A2（中证据，需与 A1 互证） |
| 界面出现"安装失败 / 无法安装 / 回滚 / Rollback" | **N2（反证据）**，触发降级 |
| 界面长时间停在"正在下载组件" | 离线环境必然卡住 → 记录 `requires_network`，交由 F9 静默期超时处理 |
| 界面出现重启提示（"需要重启才能完成"） | 置 `requires_reboot: true`，并**不再等待**（见 F15.6 例外） |

> **最终判定一律以 F15 为准**（证据分级 + 基线 diff + 反证据）。原 v2.5 "四类证据满足任一即成功" 的写法已废止——它把 `安装器退出码 0` 与 `注册表新增卸载项` 等同看待，会让大量假成功被计入已安装。

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

**定位**：F15 是"结果可信度"的唯一出口。它回答两个问题——① **软件到底装成了没**（判定）；② **这个判定有多可信**（置信度）。判定不采信安装器的"自我声明"，而依据**操作系统层面的客观变化**与**安装前后的基线差异**。

**F15.1 证据分级（取代 v2.5 的"四类证据满足任一即成功"）**

| 级别 | 编号 | 证据 | 采集方式 | 可靠性说明 |
|------|------|------|---------|-----------|
| **S（强）** | S1 | **注册表卸载项新增** | 三处 Uninstall 根（`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`、其 `WOW6432Node` 分支、**`HKCU` 同名路径**）与基线 diff 出新子键，且含非空 `DisplayName` | 安装最权威的系统级记录。**必须含 HKCU**，否则 per-user 安装（现代安装器默认行为）全部漏判 |
| **S** | S2 | **文件系统落盘 diff** | `install_verify.watch_dirs` 下新增目录/文件（相对基线），新增实体数 ≥ `min_new_files`（默认 1），且不在临时目录内 | 覆盖**绿色/便携软件**（无卸载项）；**必须有基线才成立** |
| **S** | S3 | **MSI 官方成功码** | 退出码 ∈ `{0, 3010, 1641}` **且**进程确为 `msiexec.exe` 或 MSI 事务链（`service: msiserver`） | **必须限定来源**——通用 exe 的 0 不得按 S 级计 |
| **S** | S4 | **`MsiInstaller` 事件 11707** | Application 日志源 `MsiInstaller` 新增 **11707**（安装完成），且时间晚于基线水位线 | 操作系统独立记录，客观性最高；仅 MSI 系安装器会产生 |
| **A（中）** | A1 | 通用安装器退出码 0 | 非 MSI 来源的进程退出码为 0 | **bootstrapper 假成功高发**（解压后立即退出、InstallShield 常恒返回 0），**不得单独判定** |
| **A** | A2 | 完成页被点击 | F14 `gui_actions` 中存在"完成/finish"语义按钮的点击记录 | 属"操作动作"而非"系统结果"，需与 A1 互相印证 |
| **A** | A3 | 卸载项字段变化 | 基线中**已存在**的卸载项其 `DisplayVersion` / `InstallLocation` 发生变化 | 用于**覆盖安装 / 升级**类样本（非新增键） |
| **B（弱）** | B1 | 安装日志成功关键字 | `%TEMP%\MSI*.LOG` 或安装目录日志中出现 `successfully installed` 等 | 文案不可控，易误判，**仅辅助** |
| **B** | B2 | 窗口标题出现完成语义 | F14 窗口轮询记录的标题变化 | 同上 |
| **B** | B3 | 安装器自有语义码 | 厂商自定义退出码（如 InstallShield 内部码） | 需厂商文档，不作通用依据 |

**F15.2 基线快照与差异比对（判定成立的前提）**

原文档写"安装期间出现**新增**文件 / **新增**卸载项"，但**未定义基线**——没有"安装前"的快照，就无从判定"新增"。v2.6 补齐：

| 项 | 要求 |
|------|------|
| **快照时机** | **每个文件处理之前**各做一次（保证副作用可归因到具体文件）。**同一文件的多次尝试共用同一份基线**——否则第一次尝试的副作用会被误当作第二次的基线，导致漏判（实现红线） |
| 注册表快照 | 三处 Uninstall 根的**子键名集合** + 每项 `DisplayName` / `DisplayVersion` / `InstallLocation` / `UninstallString` |
| 文件系统快照 | `watch_dirs` 内**深度 `snapshot_depth`（默认 2）** 的目录名集合 + 每目录文件数与最新 mtime；**不枚举全部文件**（避免 Program Files 下数万文件的性能灾难） |
| 事件日志水位线 | 记录 `MsiInstaller` 当前最新记录 ID（O(1)），用于判定"基线之后是否新发生" |
| 性能预算 | 单次快照 < **500ms**；`snapshot_max_entries`（默认 20000）保护，超限降级为**仅目录层**并告警 |
| 体积控制 | 基线明细是否写入结果由 `verify.write_baseline` 控制（默认 `false`，只写 `baseline_digest` + 计数），避免结果体积爆炸 |
| **扫描范围可配** | `watch_dirs` 默认 `C:\Program Files`、`C:\Program Files (x86)`；建议按样本情况追加 `%ProgramData%`、`%LOCALAPPDATA%`、`%APPDATA%`（否则 per-user 与数据型安装漏判） |

**F15.3 反证据（negative evidence）与冲突处理**

只收集正面证据无法识别"看着成功其实失败/回滚"。反证据与正面证据**同级对待，且冲突时反证据优先**：

| 编号 | 反证据 | 判定条件 |
|------|--------|---------|
| N1 | **失败类退出码** | MSI `1602`（用户取消）/ `1603`（致命错误）/ `1605`（未安装）/ `1619`（包打不开）/ `1620`（无效包）/ `1638`（已有其他版本）；或任意非零且不在成功白名单内 |
| N2 | **GUI 失败/回滚页** | F14 识别到失败语义（`失败` / `无法安装` / `failed` / `error` / `回滚` / `rollback`）**且未被后续成功页覆盖** |
| N3 | **`MsiInstaller` 11724 卸载事件** | 水位线之后出现 **11724**（移除完成）**且晚于 11707** —— 直接说明装上的东西被卸掉了 |
| N4 | **卸载项消失** | 基线中已存在的卸载项在安装后消失，**且无同名/同 `DisplayName` 的新键补充**（用于区分"升级覆盖"与"被卸掉"） |
| N5 | **watch_dirs 实体减少** | 监控目录内文件/目录数显著减少（超过 `shrink_tolerance`，默认 0） |
| N6 | **新增目录在稳定期后被删除** | 指向回滚（MSI 事务回滚会清理已复制文件） |

**冲突处理规则**：

- 出现任何反证据时：**不得判定 `install_completed`**，即使存在 S 级正面证据
- 反证据 + 存在正面副作用（有落盘或注册表变化）→ `install_partial`，并把冲突明细写入 `evidence_conflicts`
- 反证据 + 无任何正面副作用 → `failed`（或 `killed_on_timeout`，视终止原因）
- **升级豁免**：若 `pre_installed: true` 且新版卸载项 `DisplayVersion` 高于基线，则 N4 不视为反证据（属正常升级路径）

**F15.4 安装后校验（`artifact check`）**

| 模式 | 配置 | 默认 | 行为 |
|------|------|------|------|
| 轻量（默认） | `verify.require_artifact: true` | ✅ 开 | 从新增卸载项的 `InstallLocation` / `DisplayIcon` 或新增文件中挑选候选主程序，校验**文件存在且为有效 PE**（`MZ` + PE 头，可选版本资源可读） |
| 重量 | `verify.launch_check: false` | 关 | **实际启动**该主程序，等待 `launch_check_sec`（默认 10），确认进程存活或出现主窗口 |

- 结果记 `artifact_check: "ok" / "failed" / "unknown"` 与 `artifact_runnable`（重量模式才有值）
- 校验失败**不推翻**已成立的 S 级证据，但**降低置信度**并写明原因（典型：安装器装了但缺依赖运行库）
- 重量模式的取舍：能区分"安装器假成功"与"真的可用"，且**对 EDR 反而是额外的样本行为**；代价是可能弹窗（由 F14 接管）、可能需要重启。**默认关闭**，由测试口径决定是否开

**F15.5 已安装检测与重复调用（回答"第三方反复调用会怎样"）**

第三方集成时最危险的可靠性陷阱是**重复调用导致假阴性**：同一个软件装第二次时，因为已经装好了，diff 为空 → 工具报"没装上"。

- 处理某文件**之前**，若基线中已存在与之匹配的卸载项（按文件名/厂商/版本启发式匹配）或 `watch_dirs` 已有同名目录 → 记 **`pre_installed: true`**
- 处理后 diff 为空 **且** `pre_installed: true` → 判 **`already_installed`**，而非 `triggered`
- 处理后 diff 为空、`pre_installed: false` 且无反证据 → `triggered`（进程跑完了但确实没装成）
- `pre_installed` 为**恒存在字段**，第三方可据此区分"本来就装着"与"本次装上了"

**F15.6 稳定性等待（`settle_sec`，由固定值改为自适应）**

安装器退出**早于**文件真正落盘是常见现象（MSI 异步提交、子进程仍在写盘）。v2.6 改为**静默期收敛**：

- 退出后至少等待 `settle_sec`（默认 3s），之后**每 500ms 复查一次基线 diff**；若**连续 `quiet_polls`（默认 3）次无新变化**即认为稳定
- 收敛上限 `settle_max_sec`（默认 30s），超时即按当前状态判定
- 例外：退出码为 `3010` / `1641`（需重启）时，**部分落盘可能推迟到重启后** —— 此时无法取得 S2 级证据，**以 S3/S4 为准**，并置 `requires_reboot: true`

**F15.7 判定决策表**

| 条件 | `final_status` | `install_confidence` |
|------|---------------|---------------------|
| 有 ≥1 条 S 级证据，且无反证据 | `install_completed` | `high` |
| 有 ≥2 条 A 级证据（其中至少 1 条为 A1 或 A2），且无反证据 | `install_completed` | `medium` |
| 仅 1 条 A 级，或仅有 B 级证据，无反证据 | `install_unverified` | `low` |
| 有反证据，且存在正面副作用（落盘/注册表有变化） | `install_partial` | `low` |
| diff 为空，`pre_installed: true` | `already_installed` | `high` |
| diff 为空，`pre_installed: false`，进程正常结束 | `triggered` | — |
| 被超时终结且无任何副作用 | `killed_on_timeout` | — |
| 进程无法启动 / 反证据且无副作用 | `failed` | — |
| 无可触发方式（空文件 / 文档无宿主 / 前置缺失） | `skipped` | — |

- `install_confidence` **恒存在**（非安装判定为 `null`）
- 结果同时给出 `install_evidence`（结构化数组，见 F10）与 `evidence_conflicts`，**让下游能自行复核，而不是只拿到一个布尔值**

**F15.8 可靠性边界声明（必须与结果一起交付给使用方）**

**本工具判定的是"安装器是否完成了安装动作"，不等于"软件是否可正常使用"。** 这是能力边界，不是实现缺陷，README 与 DEPLOY.md 均须写明：

**已知假阳性来源**（工具可能报"装成了"但实际不可用）：
- 绿色/便携软件、自解压包、下载器 stub（有落盘但无实际安装）
- 安装器写入卸载项后**因缺依赖/需激活/需重启**而实际不可用
- MSI `3010`：判定为成功，但**重启前功能不完整**
- 反沙箱安装器故意制造"假成功"表象

**已知假阴性来源**（工具可能报"没装上"但实际已可用）：
- 安装到 `watch_dirs` 之外的目录（未纳入监控范围）
- 纯 per-user 且未把 `%LOCALAPPDATA%` / `%APPDATA%` 纳入 `watch_dirs`
- 同版本重复安装（diff 为空）—— 由 F15.5 `already_installed` 缓解，但需 `pre_installed` 匹配成功
- 安装器先卸旧装新，且新旧 `DisplayName` 不同（diff 被掩盖，靠 F15.3 N4 升级豁免逻辑区分）

**因此，"结果可靠"的准确含义是**：① 证据链**可复现、可审计**（每条证据都写明来源与时间）；② 置信度**如实标注**；③ 已知失效模式**明确列出**。**而不是"绝对真值"**。

**下游建议采信策略**：`high` 直接采信；`medium` 抽样复核；`low` 与 `install_partial` 必须人工或 EDR 二次确认；`already_installed` 视为"环境已具备"，不计入本次安装成果。

**F15.9 与 F14.4 的关系**

F14.4 是 GUI 自动化视角下的"看到完成页"（对应 A2 级证据）；**最终判定一律以 F15 为准**，F14.4 不再单独决定 `final_status`。

#### F16 前置依赖自检与用户提示（7-Zip / OCR 引擎）

**定位**：**工具本体为免安装单文件，自身不代为安装任何环境前置依赖**。前置依赖由镜像构建阶段或运维负责预置；**其离线安装包与安装步骤由交付包提供**（见 8.1 / 8.3），工具只做"检测 → 提示（指向交付包内安装包）→ 明确退出/降级"。

**F16.1 7-Zip 检测**

- **探测顺序**：配置指定路径（`archive.sevenzip_path`，非 `auto` 时优先）→ 注册表 `HKLM\SOFTWARE\7-Zip\Path`（含 `Wow6432Node`）→ `PATH` 查找 → 常见默认路径（`C:\Program Files\7-Zip\7z.exe`、`C:\Program Files (x86)\7-Zip\7z.exe`）
- 命中后记录路径与版本到结果 JSON 的 `prereq` 字段
- **版本要求：≥ `archive.min_version`（默认 `24.09`）**。理由：① ISO/UDF 解析缺陷（蓝光 `FileType=249` 等）在较新版本才修；② 更早版本存在嵌套归档 MotW 处理缺陷（CVE-2025-0411，24.09 修复）。低于该版本 → **告警但继续运行**（不阻断），并在 `prereq.archive_backend_status` 记 `version_too_old`
- 7-Zip 官方**仍支持 Windows 7**（含 26.x 版本），Win7 环境无需降级选版本

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
- **`--json` 时改为把结构化自检结果输出到 stdout**（单 JSON 文档，字段与结果 JSON 的 `prereq` 一致 + `ok: true/false` + `issues: []`），便于镜像构建流水线做**机器可判定的卡点**；报告本身仍走 stderr（F10.1）
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
| **7-Zip 版本** | **≥ 24.09**（`archive.min_version`） | ISO/UDF 解析缺陷、嵌套归档 MotW 缺陷（CVE-2025-0411）未修 | F16.1；低于要求时**告警但继续**，记 `version_too_old` |
| （可选）光盘映像挂载后端 | 可选（仅 `iso.strategy: mount_*` 需要） | 挂载轨不可用，自动收敛为解压轨 | F16.6；Win8+ 系统自带，Win7 需第三方 |

**F16.6 光盘映像挂载后端探测（可选前置，仅挂载轨需要）**

- **Win8 / Server 2012+**：系统自带 `Mount-DiskImage` / `Dismount-DiskImage`（Storage 模块，需 PowerShell 可用），**无需任何额外安装**；ISO 挂载通常不需提权（失败时可提权重试）
- **Win7**：无原生 ISO 挂载能力 → 探测**第三方便携版**（`WinCDEmu` 便携版 `vmnt.exe` / `batchmnt.exe`；LGPL、免安装、无需重启、XP~Win11 通用）
- **探测顺序**：`iso.mount_tool_path`（非 `auto` 时优先）→ `PATH` → 常见目录（如 `C:\tools\WinCDEmu\`）
- **缺失时不报错**（挂载轨属可选能力）：记 `prereq.mount_backend_status: missing`，`iso.strategy` 自动按 `extract_only` 行为运行并告警
- **不新增退出码**——挂载后端缺失不进 6.1 退出码表；仅在 `iso.strategy` 被显式设为 `mount_*` 且后端缺失时，记 `mount_failed` 并在任务汇总告警

> 该"检测 + 提示 + 不代装"策略同样适用于后期新增的任何前置依赖。**完整的依赖边界（含语言运行时要不要、镜像准备项有哪些）见 4.7**；表中 7-Zip / OCR 引擎 / VC++ 运行库的**离线安装包由交付包提供**（8.3 B 组），安装步骤见 **DEPLOY.md（D14）**。

#### F17 软件包结构识别与安装入口优先级排序

**解决的问题**：当输入是包含数百个文件的软件包（压缩包或目录）时，逐个盲扫有两个后果：

1. **效率低**——真正的安装入口通常只有 1~5 个，其余是 DLL、资源、文档
2. **正确性风险（更重要）**——包内常含 `unins000.exe` / `卸载.exe` 等**卸载程序**。盲扫会把它们当作普通 PE 执行，**先跑卸载会破坏刚建立的安装结果**，直接污染 `install_completed` 判定

软件包的目录结构本身携带了强信号（作者用什么文件作为安装入口、哪些是配套工具），应当**先读结构再决定触发表**，而不是平等对待每个文件。

**适用输入**：① 压缩包（F6 解压前后）② 用户直接提供的目录（F11 遍历前）③ **光盘映像（F18 的两条轨均可产出清单）**。**单文件输入不适用**本模块——无结构可分析，直接走 F2 类型识别。

**F17.1 结构预扫描**

- **压缩包：先列清单再解压**——`7z l "<archive>"` 输出文件清单与各条目大小，用于：
  - 解压前即完成入口识别与总量估算
  - 预判解压后总大小是否超限（配合 F6 的 2 GB 上限），超限则直接记 `extract_size_exceeded` 且**不落盘**
- **目录**：直接扫描目录树（深度上限可配置）
- **光盘映像**：轨 A 同样用 `7z l` 列清单（不落盘）；轨 B 直接扫描挂载盘（见 F18.2 / F18.5）
- 产出内部结构模型：文件路径、大小、扩展名、文件头类型、相对包根的层级深度

**F17.2 入口识别与优先级分层**

按"作者意图明确度"排序，优先级数字越小越先执行：

| 优先 | 类型 | 判据 | 示例 |
|------|------|------|------|
| **P0** | 说明文件明示入口 / **`autorun.inf` 声明** | `readme` / `安装说明` / `使用说明` 文本中出现"运行/双击/执行 `<文件名>`"模式，**且该文件确实存在**；或 `autorun.inf` 的 `open=` / `shellexecute=` 指向的文件存在且未被黑名单排除（见 F18.5） | `安装说明.txt` → "双击 install.bat 开始安装"；`autorun.inf` → `open=setup.exe` |
| **P1** | 安装入口命名 | 文件名（去扩展名、转小写）含 `install` / `setup` / `boot` / `安装` / `部署` / `一键` | `install.bat`、`setup.exe`、`一键安装.exe` |
| **P2** | 标准安装器（文件头） | 文件头识别为 MSI / NSIS / Inno Setup / InstallShield / WiX Burn | `setup.exe`（NSIS 特征） |
| **P3** | 前置依赖包 | 名称含 `vcredist` / `dotnet` / `directx` / `prereq` / `依赖` / `环境` | `vcredist_x64.exe` |
| **P4** | 主程序 / 启动器 | 与包名（或包内顶层目录名）同名的 exe、路径层级最浅的 exe、名称含 `启动` / `launch` / `main` | `软件名.exe` |
| **P5** | 其余可执行 | 目录深处的 exe / dll / 脚本 | — |

同级之内的附加排序规则：

- **架构匹配优先**——同时存在 `setup_x64.exe` 与 `setup_x86.exe` 时，优先匹配当前系统架构
- **路径浅者优先**（`prefer_shallowest`）——越靠近包根的越可能是入口
- 嵌套压缩包内的入口，**每深一层降一档**

**F17.3 排除清单（黑名单，绝不执行）**

| 类别 | 匹配关键字 |
|------|-----------|
| **卸载程序** | `uninstall` / `unins0` / `卸载` / `反安装` / `remove` |
| 修复 / 修改安装 | `repair` / `修复` / `modify` |
| 崩溃上报 / 调试 | `crashpad` / `crashreporter` / `werfault` / `debug` |
| 更新检查器 | `update` / `updater` / `checkupdate` / `升级` |
| 帮助 / 示例 | `.chm` / `.hlp` / `.pdf` |

> **这是本模块最重要的收益之一**：`unins000.exe` 在盲扫模式下会被当作普通 PE 执行，若先于安装执行会直接破坏安装结果（甚至删掉刚落盘的文件）。黑名单在**阶段 1 与阶段 2 均强制生效，不可通过配置关闭**。
> 被排除的文件在结果中记 `selection_reason: excluded_blacklist`，保留可审计性。

**F17.4 执行策略：定向优先 + 兜底回退**

> 注意：本模块**不是**"只跑 install.bat，其余一律不试"。目标是"更快成功 + 避免自伤"，而非"减少覆盖面"。

**阶段 1——定向尝试**

1. 按 P0 → P4 顺序逐个执行识别出的入口
2. P0（说明文件明示）**仅提升优先级，不单独构成执行依据**——readme 提示可能过时或指错文件，仍要求目标文件真实存在且未被黑名单排除
3. **P3 前置依赖包先于主安装器执行**（简单前置优先，不构建完整依赖图）
4. 每个入口仍走完整的 F13 多轮参数尝试 + F14 GUI 接管
5. **一旦取得 `install_completed`（F15 证据）→ 立即结束整个包**，其余文件记 `skipped_after_success`

**阶段 2——兜底回退**（`fallback_to_brute`，默认 `true`）

1. 阶段 1 未取得安装成功证据时，回退到**原有的全量遍历行为**（P5 及其余可执行文件）
2. 阶段 2 依然受：黑名单约束、单文件超时（F9）、全局看门狗、`max_attempts_per_file` 约束
3. 设 `fallback_to_brute: false` 可关闭兜底——仅建议用于包结构高度规范、且对效率极敏感的场景

> **设计取舍**：本模块**不改变**"宁可误触发，不可漏触发"（F8）的原则。定向阶段负责把最可能的入口排前并尽早收敛，兜底阶段保证不遗漏。因此收益是**"更快成功 + 不破坏安装结果"**，而不是"缩小尝试范围"。

**F17.5 结果记录**

- **每个文件**新增 `selection_reason`：`entry_p0_manifest` / `entry_p1_named` / `entry_p2_installer` / `entry_p3_prereq` / `entry_p4_main` / `fallback_p5` / `excluded_blacklist` / `skipped_after_success`
- **任务级**新增 `layout_analysis`：识别出的入口清单（含分层与理由）、被排除清单、是否进入兜底阶段、最终收敛点（在哪个入口成功）

#### F18 光盘映像专项处理（ISO / IMG）

**解决的问题**：光盘映像是**一类特殊输入**，与普通压缩包有四个本质差异，**不能简单套用 F6**：

1. **介质语义不同**——光盘是**只读介质**，且带 `autorun.inf` 这一"作者声明的入口"；挂载后 `GetDriveType()` 会返回 `DRIVE_CDROM`，个别安装器会据此分支
2. **解析能力有缺口**——7-Zip 对 ISO/UDF 的支持**并非全覆盖**：部分 UDF 2.5 映像（目录项 CRC 长度字段写法非标准）、蓝光影碟（`BDMV\STREAM\*.m2ts` 的 ICB `FileType=249`）会直接报 `cannot open file as archive`。**只走解压轨会漏样本**
3. **存在"不该触发"的介质**——操作系统安装盘（含 `sources\install.wim`）执行 `setup.exe` 会**开始安装操作系统**，直接破坏测试虚拟机。这不是"装不成"，而是"把机器搞坏"
4. **MotW 传播行为随系统版本变化**——Win11 22H2 起挂载 ISO 会把 Mark-of-the-Web 传播到内部文件（历史上 ISO 挂载是经典的 MotW 绕过手法），直接影响 SmartScreen 是否拦截安装器、进而影响能否装成功

**F18.1 介质性质判定（先行分类，决定后续处理）**

进入触发前先按**内容特征**判定介质性质（规则表外部可扩充：`media_kinds.json`）：

| 性质 | 判定特征 | 处理 |
|------|---------|------|
| `malware_lure`（恶意投递结构，**重点场景**） | 根目录仅 1~3 个文件，含 `.lnk` / `.js` / `.vbs` / `.hta` / `.wsf`，或"与映像同名的快捷方式 + 隐藏载荷"组合 | **最高优先执行**（`.lnk` 解析目标后重走 F2/F3）；ISO+LNK 是近年最典型的投递手法，也是 EDR 测试最关心的场景 |
| `installer_media` | 含 `setup.exe` / `install.exe` / `*.msi`，或 `autorun.inf` 指向入口 | 正常处理：按 F17 分层执行 |
| `os_install_media`（**默认不触发**） | 含 `sources\install.wim` / `install.esd` / `boot\bcd` / `sources\setup.exe` | **默认跳过**，记 `skipped: os_media`；需 `iso.allow_os_media: true` 才触发。**理由：执行它会开始安装操作系统、破坏测试机，且与"触发样本行为"的目标无关** |
| `media_disc`（影碟） | 含 `BDMV\` / `VIDEO_TS\` / `*.m2ts` / `*.vob` | 默认跳过，记 `skipped: media_disc`（7-Zip 对蓝光有已知解析缺陷）；`iso.allow_media_disc: true` 可放开 |
| `data_disc` | 其他 | 走通用 F17 流程 |

判定结果记入 `layout_analysis.media_kind` 供下游审计。

> 判定**只依据清单内容与文件特征，不依据映像文件名**——ISO 的文件名往往无意义（如 `abc.iso` x2）。

**F18.2 两条执行轨：解压为主（默认），挂载为辅**

| | **轨 A 解压**（`extract_*`，默认） | **轨 B 挂载**（`mount_*`） |
|---|---|---|
| 做法 | `7z l` 列清单（不落盘）→ F17 分析 → 解压到**短路径临时目录** → 按优先级执行 | `Mount-DiskImage` 挂载取盘符 → 扫描挂载盘做 F17 分析 → **直接从盘符执行**入口 |
| 优点 | 无驱动依赖、**Win7 可用**、介质**可写**（个别安装器需往介质写日志/许可文件）、无盘符与卸载残留、与 F6/F17 流程完全一致；**7-Zip 默认不传播 MotW** | **恢复真实光盘介质语义**：拿到真实盘符与 `DRIVE_CDROM` 类型、`autorun.inf` 与原始只读介质关系完整、安装期间源介质始终可用 |
| 缺点 | 丢失介质语义；若解压目录被清理，依赖"源介质"的 MSI 按需安装/修复会失效 | 需 Win8+ 原生支持（Win7 需第三方驱动）、占用盘符、**必须可靠卸载**、只读介质下个别安装器反而失败 |
| 环境要求 | 仅需 7-Zip | Win8 / Server 2012+ 用系统 `Mount-DiskImage`（**ISO 挂载通常不需提权**，失败时提权重试）；Win7 需可选第三方（WinCDEmu 便携版，见 F16.6） |

**默认 `iso.strategy: extract_first`** —— 兼顾 Win7 + "尽量装成功"（无 MotW 传播、介质可写、无残留）。`mount_first` 供需要真实介质语义的样本集选用；`extract_only` / `mount_only` 可强制单轨。

**自动切换**：轨 A 在 `7z l` 或解压阶段报"无法打开"（UDF 兼容缺陷 / 加密映像 / 多重会话）时，**自动回退轨 B**（若挂载后端可用），结果中记 `iso_strategy_used: "extract→mount"`；两轨皆失败记 `iso_unhandled: unsupported_image`，不阻塞后续样本。

**F18.3 挂载轨的生命周期与清理（轨 B 的可靠性重点）**

1. **挂载**：`Mount-DiskImage -ImagePath <本地绝对路径>` —— **不支持 UNC 路径**，网络位置需先拷贝到本地再挂载
2. **取盘符**：`Mount-DiskImage -PassThru | Get-Volume` 取 `DriveLetter`，**不假设盘符**
3. **安装期间保持挂载**：`keep_mounted_until_verified: true`（默认）——直到 F15 判定完成（含 `settle_sec` 稳定期）才卸载。**禁止在安装器仍在运行时卸载**，否则安装中断
4. **卸载**：`Dismount-DiskImage -ImagePath <同一路径>`（比按盘符 / 设备路径更稳）。失败时依次：结束占用挂载盘文件的进程 → 短延时约 1s → 重试（`dismount_retry` 默认 3 次）→ 仍失败记 `dismount_failed`，并在任务收尾统一兜底
5. **异常退出兜底**：工具启动时执行一次**遗留挂载清理**（`Get-DiskImage | Where Attached`），**仅卸载能与本次任务标识对应的映像**；无法确认归属的只记录告警，不强行卸载（避免动到其他任务的挂载）
6. **挂载失败**（盘符冲突、映像已在挂载、稀疏映像、USB 读卡器占用等）→ 记 `mount_failed` 并回落轨 A

**F18.4 MotW（Mark-of-the-Web）处理策略**

MotW 直接影响 SmartScreen：带 MotW 的未签名安装器可能被"Windows 已保护你的电脑"拦截，**该拦截会中断安装**。

| 策略 | 行为 | 适用 |
|------|------|------|
| `native`（**默认**） | **不主动改写**，保持各路径原生行为：解压轨下 7-Zip 默认不传播 MotW；挂载轨在 Win11 22H2+ 会传播 MotW | 尊重环境真实行为，不人为干预 |
| `preserve` | 把映像自身的 `Zone.Identifier` **显式传播**到内部文件 | 需要还原"从网络下载后挂载"的真实场景（含 SmartScreen 拦截链） |
| `clear` | 清除内部文件的 `Zone.Identifier` | 需要排除 SmartScreen 干扰、聚焦样本自身行为时 |

- 实际生效值记入 `layout_analysis.motw_policy`，用于解释"这次为什么没装成/为什么被拦"
- **明确边界（同步写入 2.4 非目标）**：本工具**不提供绕过 Windows 安全机制的实现**；上述策略只是**如实记录与可配置的传播行为**。"是否需要 SmartScreen 参与"属测试口径问题，由安全团队决定（见 Q14）

**F18.5 与 F17 的衔接（三处补充）**

1. **`autorun.inf` 提升为 P0 级入口信号**——读取 `[AutoRun]` 段的 `open=` / `shellexecute=` / `icon=`，指向文件存在且未被黑名单排除时按 P0 处理（这正是"作者声明入口"，比 readme 更权威）。**只读取，不依赖系统自动运行**（现代 Windows 默认不执行光盘 autorun）
2. **清单来源按轨区分**——轨 A 用 `7z l` 输出；轨 B 直接扫描挂载盘；两者交给 F17 同一套分层逻辑
3. **编码容错**——老光盘常用 Joliet/GBK 且可能出现乱码文件名。F17 的关键字匹配（`安装说明`、`setup` 等）需**做编码容错与宽松匹配**，避免因乱码漏识别；不确定时按 P1 命名规则与 P5 兜底，**不直接跳过**

**F18.6 工程细节**

| 事项 | 要求 |
|------|------|
| **短路径临时目录** | 解压根目录默认使用短路径（如 `C:\AT\t\<taskID>\`，可由 `archive.temp_root` 指定）——映像内常含深层长路径，避免 `MAX_PATH` 相关失败 |
| 解压目录保留策略 | `iso.keep_extracted_on_success: false`（默认，退出前清理）。设为 `true` 时**保留解压目录**并在结果中记录路径，用于"安装器按需从源读取"或人工复核；**此时不计入"无残留"，README 须说明** |
| 映像内文件哈希 | 对**被执行**的映像内文件同样记录 SHA-256（与普通样本一致，便于下游比对） |
| 深度与体积限制 | 复用 F6 的递归深度（默认 3 层）与解压体积上限（默认 2 GB）；轨 A **在解压前用 `7z l` 预判总量**，超限不落盘 |
| 嵌套 | ISO 内含压缩包 / 压缩包内含 ISO 走 F12 链式调用，统一受深度上限约束 |

**F18.7 结果记录**

`layout_analysis` 新增字段：

| 字段 | 说明 |
|------|------|
| `media_kind` | `malware_lure` / `installer_media` / `os_install_media` / `media_disc` / `data_disc` |
| `iso_strategy_used` | 实际生效的轨：`extract` / `mount` / `extract→mount`（回退） |
| `mount_drive` | 挂载盘符（仅轨 B） |
| `dismount_status` | `ok` / `failed` / `not_applicable` |
| `motw_policy` | 实际生效的 MotW 策略 |

#### F19 第三方集成契约（P0）

**定位**：本工具会被第三方流水线/脚本调用，用于安装任意软件。调用方关心的不是内部实现，而是"**我调它、读它的输出，能不能稳定地做出判断**"。因此把对外契约单独立章——**没有契约的可靠性只是巧合**。

**F19.1 CLI 与通道契约（稳定面）**

- **稳定面**（一经发布即为契约，变更须升主版本）：`--input` / `--output` / `--config` / `--recursive` / `--timeout` / `--no-archive` / `--no-ocr` / `--check-prereq` / `--dump-config` / `--dump-ui` / `--version` / `--capabilities` / `--log-file` / `--log-level` / `--lock`
- **参数解析规则固定**：`--key value` 与 `--key=value` 等价；**未知命令行参数报错退出（退出码 2）而不是忽略** —— 这与配置文件的"未知字段忽略 + 告警"**刻意不同**：命令行是精确接口（拼错参数必须暴露），配置文件是向前兼容接口
- **通道契约**：stdout 仅机器可读产物；stderr 仅日志与进度；结果与日志落文件（F10.1，红线）
- **退出码契约**：见 6.1，**只表示工具自身状态**；单文件结果**只在 JSON 里**，调用方不得用退出码推断

**F19.2 结果 Schema 契约**

| 条款 | 内容 |
|------|------|
| 兼容策略 | **只做加法**：可新增字段；已发布字段**不改名、不改语义、不删除**；枚举值**只增不改**；破坏性变更才升 `schema_version` 主版本 |
| 字段保证 | 固定字段**恒存在**，无值一律 `null`（F10.2 R2）——下游不必判空 |
| 顺序 | **JSON 对象字段顺序不保证**，下游不得依赖顺序；数组顺序**有语义**（按时间/优先级），可依赖 |
| 类型 | 数值/布尔/时间为**原生类型**，非字符串 |
| 最小决策集 | `final_status` + `install_confidence` + `requires_reboot` 是下游做决策的**最小字段集**，跨版本稳定 |
| Schema 文件 | 随交付提供（D7），建议调用方在 CI 中用它对结果做强校验 |

**F19.3 能力协商与版本**

- `--version`：输出工具版本与 `schema_version`
- **`--capabilities --json`（新增）**：输出机器可读能力清单，供调用方**先探测再适配**，而不是硬编码假设：

```json
{
  "tool_version": "1.0.0",
  "schema_version": "2.8",
  "platform": "windows",
  "os_version": "6.1.7601",
  "supported_final_status": ["install_completed", "install_unverified", "install_partial", "already_installed", "triggered", "killed_on_timeout", "failed", "skipped"],
  "supported_evidence_ids": ["S1", "S2", "S3", "S4", "A1", "A2", "A3", "B1", "B2", "B3"],
  "supported_confidence": ["high", "medium", "low"],
  "supported_methods": ["silent_install", "no_param_run", "gui_automation", "script", "msi", "dll_probe", "lnk_target", "document_host"],
  "capabilities": {
    "archive_backend": true,
    "mount_backend": true,
    "ocr": true,
    "gui_automation": true,
    "launch_check": false
  },
  "limits": { "max_attempts_per_file": 8, "timeout_sec": 600, "idle_timeout_sec": 60, "task_timeout_sec": 0 }
}
```

**F19.4 幂等、可重入与并发**

| 项 | 规则 | 理由 |
|------|------|------|
| **单实例锁** | `--lock`（默认开启）：同一机器同一时刻只允许一个实例执行**会修改系统状态**的操作；第二个实例按 `lock_wait_sec`（默认 0，即不等待）排队或直接以**退出码 6** 返回 | **并发实例会互相污染注册表/文件系统 diff，导致证据归因错误**——这是最隐蔽的可靠性杀手 |
| 结果文件 | 同名**直接覆盖**（原子写）；需要保留历史由调用方自行改名/归档 | 与 F10.1 `--output` 规则一致 |
| **重复调用** | 第二次调用同一软件：已安装则得 `already_installed`（F15.5）；覆盖安装则凭 A3/S1 证据得 `install_completed` | 消除"装第二次报失败"的假阴性 |
| **失败重试** | 工具**不做跨运行自动重试**；`max_attempts_per_file` 指"**单次运行内**的多参数尝试"，不是"跨运行重试" | 避免放大行为、无限延长总时长；重试策略由调用方掌握 |
| 中断可恢复 | 中断时结果 JSON 仍有原子写产物，journal 保留已完成记录（F10.3 / F9.6） | 长批量任务不丢数据 |

**F19.5 结果稳定性的前提（回答"为什么同样的输入结果不同"）**

- 结果受**环境**影响：AV/EDR 拦截、UAC 状态、管理员权限、`watch_dirs` 配置差异、是否有其他 MSI 在进行（MSI `1618`）、磁盘空间、补丁级别。**这些不是工具的不确定性，而是环境差异**
- 因此结果中**恒记录** `prereq` / `platform_meta` / `config_digest`；调用方**应把它们与结果一起归档**，用于解释"上次成功这次失败"
- 调用方若要求**可比性**，必须保证三点一致：**相同镜像基线 + 相同配置（`config_digest` 相同）+ 相同权限上下文**
- **明确不承诺**：不同环境（不同 AV 策略 / 补丁级别 / 权限）下同一输入必然得到同一结果。这属于能力边界，不是缺陷

**F19.6 集成模板与契约测试**

- **集成示例**：cmd / PowerShell 示例，覆盖①单文件安装 ②目录批量 ③读取结果判定 ④超时设置 ⑤失败分支处理；附 `jq` / `ConvertFrom-Json` 解析片段
- **契约测试套件**：一组 golden 结果 JSON + Schema，第三方在集成前运行，验证自己的解析逻辑对新版本仍成立
- **`--self-test`（可选）**：用内置最小样本验证自身链路（不依赖外部物料），便于第三方部署后自检

**F19.7 建议的调用方式**

```bat
:: ① 先探测能力（部署后一次性）
AutoTrigger.exe --capabilities --json

:: ② 执行安装，结果写指定文件；外部超时须大于工具内部上限
AutoTrigger.exe --input "D:\samples\app.exe" --output "D:\out\r.json" --timeout 900

:: ③ 判定（只看 JSON，不看退出码）
::    final_status == "install_completed" && install_confidence == "high"  → 直接采信
```

| 建议项 | 做法 |
|--------|------|
| 外部超时 | 必须**大于**工具内部上限（`task_timeout_sec`，或按 文件数 × `timeout_sec` 估算），否则外部先杀会导致拿不到结果 JSON（虽有 journal 但需额外处理） |
| 判定顺序 | 先读结果 JSON → 若缺失或 `finished_at: null` → 回退读 journal；**单文件结果以 `final_status` 为准，不看退出码** |
| 重试条件 | 仅对 `failed` / `install_partial` / `killed_on_timeout` 且在确认环境正常后重试；`install_unverified` 应**先复核再决定** |
| 归档 | 结果 JSON + 日志 + journal（若有）三件一起归档，附 `config_digest` |

---

## 6. 非功能需求

| 类别 | 要求 |
|------|------|
| 性能 | 单文件触发不超过配置超时；批量并发度可配置（默认串行）；单次 OCR 识别 ≤ 800ms；引擎常驻复用，避免重复初始化；**安装证据快照单次 < 500ms**（F15.2） |
| 稳定性 | 单文件失败不影响整体流程；**四段式超时 + 活动感知**（F9）；全局看门狗防挂死；无协程泄漏；OCR / 模板任一失败均可降级；OCR 引擎崩溃可自动重启；**协作式中止后仍产出完整结果 JSON**（F9.6） |
| **结果可靠性** | 安装成功判定采用**证据分级（S/A/B）+ 基线 diff + 反证据**，`install_completed` 需 ≥1 条 S 级或 ≥2 条 A 级证据；结果附 `install_confidence`；**假阳性与假阴性来源在 F15.8 逐条列明**；不得把"安装动作完成"表述为"软件可用" |
| **幂等与并发** | 单实例锁（F19.4）防并发实例互相污染证据 diff；结果文件同名覆盖（原子写）；重复调用已安装软件返回 `already_installed`；**工具不做跨运行自动重试** |
| 兼容性 | **Windows 7 SP1 x64 / 8.1 / 10 / 11 / Server 2008 R2+**；架构预留 Linux / macOS |
| 权限 | 以管理员权限运行（静默安装需要）；启动时自检权限，无管理员权限则告警并继续；沙箱镜像建议关闭 UAC 弹窗以允许 GUI 自动化 |
| 日志 | **通道严格分流**：日志与提示一律写 **`stderr` + 日志文件**（时间戳、级别、文件名、阶段），**绝不写 `stdout`**；`stdout` 只承载机器可读输出（`--dump-config` 配置 JSON、`--output -` 结果 JSON、`--check-prereq --json` 自检 JSON），保证下游可直接解析（F10.1） |
| 可配置 | 超时、并发度、递归深度、解压大小上限、尝试间隔与次数、参数表、按钮关键词表、**OCR 引擎与路径/传输方式/置信度阈值**、模板目录、**光盘映像策略（解压/挂载轨、OS 介质与影碟是否允许触发、MotW 策略、挂载与卸载重试）**均可配置 |
| 无残留 | 退出前清理临时目录、**卸载本次挂载的光盘映像**（F18.3）、**回收 OCR 引擎子进程**、**清理原子写遗留的 `.tmp` 文件**与 journal（`output.keep_journal_on_success: false` 时，见 F10.3）；不修改系统默认文件关联；**不写入注册表、不产生安装残留** |
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
| **6** | **检测到已有实例正在运行**（`--lock` 生效且 `lock_wait_sec` 内未取得锁）—— 防止并发实例互相污染安装证据 diff（F19.4） |
| **130** | **任务被协作式中止**（`Ctrl+C` / 控制台关闭事件）—— 结果 JSON 已原子写出，`error.code = 130`，journal 保留（F9.6） |

> **退出码只表示工具自身状态，不表示单文件安装结果**；批量场景下退出码 `0` **不等于**全部装成功，单文件结果一律以结果 JSON 的 `final_status` 为准（F19.1 / F10.1）。

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
| 结果输出 | JSON（主结果，**原子写**，字段完整恒存在，可过 D7 Schema）+ 文本日志（辅助），仅写本地文件；**stdout 仅机器可读输出、日志走 stderr**（F10.1~F10.3） |
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
internal/platform/windows/       # Windows 适配：进程/Job Object、UIA、注册表、截屏、DPI、**基线快照与证据采集（注册表 Uninstall 三处 / 文件系统 / MsiInstaller 事件）**
internal/platform/windows/winver/# 系统版本判定与 API 能力分支（Win7 / Win10 差异集中在此）
internal/platform/linux/         # 空实现占位（编译期可选）
internal/verify/                 # **安装证据编排（F15）**：基线快照时机与复用、diff 比对、证据分级映射、反证据仲裁、置信度计算、静默期收敛（平台无关）
internal/activity/               # **进展信号采集（F9.2）**：CPU 时间/IO/子进程/目录写入/GUI 变化的统一抽象（平台无关编排 + 适配层实现）
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
6. **版本要求**：**≥ `archive.min_version`（默认 24.09）**，低于该版本告警但继续（见 F16.1）
7. **ISO / UDF 的已知能力边界**：7-Zip 支持 ISO9660 / UDF 的**解包**，但对部分非标准 UDF 2.5 映像与蓝光影碟（`FileType=249`）会报"无法打开"——**这属于解析缺陷，不是配置问题**。因此光盘映像按 **F18 双轨**处理：解压失败时自动回退到系统挂载

### 7.5 config.json 结构（全部字段有内嵌默认值，可选覆盖）

```json
{
  "timeout_sec": 600,
  "startup_timeout_sec": 30,
  "idle_timeout_sec": 60,
  "task_timeout_sec": 0,
  "progress_interval_sec": 30,
  "activity_cpu_epsilon_ms": 200,
  "activity_io_epsilon_bytes": 65536,
  "concurrency": 1,
  "attempt_interval_sec": 2,
  "max_attempts_per_file": 8,
  "archive": {
    "max_depth": 3,
    "max_extract_size_bytes": 2147483648,
    "sevenzip_path": "auto",
    "require_sevenzip": true,
    "min_version": "24.09",
    "temp_root": ""
  },
  "iso": {
    "strategy": "extract_first",
    "allow_os_media": false,
    "allow_media_disc": false,
    "autorun_as_p0": true,
    "media_kinds_file": "media_kinds.json",
    "mount_tool_path": "auto",
    "mount_timeout_ms": 15000,
    "dismount_retry": 3,
    "keep_mounted_until_verified": true,
    "keep_extracted_on_success": false,
    "motw_policy": "native"
  },
  "package_layout": {
    "enabled": true,
    "mode": "smart",
    "entry_patterns_file": "install_entries.json",
    "exclude_patterns_file": "exclude_entries.json",
    "prefer_shallowest": true,
    "prefer_arch_match": true,
    "readme_hints": true,
    "max_entries_phase1": 10,
    "fallback_to_brute": true
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
    "quiet_polls": 3,
    "settle_max_sec": 30,
    "watch_dirs": ["C:\\Program Files", "C:\\Program Files (x86)", "%ProgramData%", "%LOCALAPPDATA%", "%APPDATA%"],
    "snapshot_depth": 2,
    "snapshot_max_entries": 20000,
    "write_baseline": false,
    "min_new_files": 1,
    "shrink_tolerance": 0,
    "require_artifact": true,
    "launch_check": false,
    "launch_check_sec": 10
  },
  "lock": {
    "enabled": true,
    "wait_sec": 0
  },
  "dll_export_probe_count": 3,
  "output": {
    "path": "",
    "pretty": false,
    "journal": true,
    "keep_journal_on_success": false,
    "log_file": "",
    "log_level": "info",
    "max_file_size_mb": 64
  }
}
```

> - `sevenzip_path` / `ocr.engine_path` 取值为 `auto` 时按 F16.1 / F16.3 的探测顺序自动查找
> - `require_sevenzip: false` 等价于默认启用 `--no-archive` 行为
> - `ocr.engine` 可选 `paddleocr_json`（默认）/ `rapidocr_json` / `umi_ocr` / `tesseract` / `windows`（Win10+ 可选回退）/ `none`
> - `ocr.transport` 可选 `stdio`（默认）/ `tcp` / `http`（仅 Umi-OCR）
> - `ocr.require_ocr: false` 表示 OCR 缺失时**降级而非失败**；设为 `true` 则缺失时以退出码 5 结束
> - `ocr.min_score` 为**引擎原生置信度阈值**，低于该值的识别结果不参与点击
> - `archive.temp_root` 为空时自动选择**短路径**（ISO 解压必须使用短路径，见 F18.6）；`archive.min_version` 为 7-Zip 最低版本要求（见 F16.1）
> - `iso.strategy` 可选 `extract_first`（默认）/ `mount_first` / `extract_only` / `mount_only`；轨 A 解析失败时自动回退轨 B（F18.2）
> - `iso.allow_os_media` / `iso.allow_media_disc` 默认 `false`：**操作系统安装介质与影碟默认不触发**（F18.1），避免误装操作系统、破坏测试机
> - `iso.motw_policy` 可选 `native`（默认，不主动改写）/ `preserve` / `clear`（F18.4）
> - `iso.keep_extracted_on_success: true` 会保留解压目录（此时不计入"无残留"，README 须说明）
> - `package_layout.mode` 可选 `smart`（默认，F17 定向优先 + 兜底回退）/ `brute`（关闭 F17，退化为全量平扫）；`package_layout.enabled: false` 等价于 `mode: brute`
> - **F17.3 的排除清单（卸载/修复/更新/调试类）不受配置影响，两种模式下均强制生效**，不可关闭
> - `output.path` 为空时使用默认路径 `.\autotrigger-result.json`（**不得落在输入目录内**）；`--output` 可覆盖，取 `-` 表示写 stdout
> - `output.log_file` 为空时使用 `<结果文件名>.log`；`output.log_level` 可选 `debug` / `info`（默认）/ `warn` / `error`，可用 `--log-level` 覆盖
> - `output.journal` 控制是否写 `<结果文件名>.journal.ndjson`（逐文件追加，供中断找回）；`keep_journal_on_success` 决定正常结束后是否保留（默认 `false`，即删除，符合第 6 节"无残留"）
> - `output.pretty` 仅影响缩进（语义等价），默认 `false`（紧凑单行）；`max_file_size_mb` 超限时截断 `attempts` 明细并置 `truncated: true`
> - **超时（F9）**：`timeout_sec` 为**单文件硬上限**（默认 600，`0` = 不限）；`startup_timeout_sec` 为进程无任何动作的容忍时长；**`idle_timeout_sec` 才是真正的把关**——连续无进展信号超过该值即判定卡死；`task_timeout_sec` 为任务级上限（`0` = 不限）；`activity_*_epsilon_*` 为进展信号的判定阈值，用于避免把极小波动误判为进展
> - **安装验证（F15）**：`settle_sec` / `quiet_polls` / `settle_max_sec` 共同定义"静默期收敛"（退出后等待稳定，而非固定 sleep）；`watch_dirs` 支持 `%VAR%` 展开，**建议追加 `%LOCALAPPDATA%` / `%APPDATA%` / `%ProgramData%`，否则 per-user 安装与数据型安装会漏判**；`snapshot_depth` 控制文件系统快照深度（不枚举全部文件，控性能）；`write_baseline` 决定基线明细是否写入结果（默认 `false`，只写 digest + 计数）；`min_new_files` / `shrink_tolerance` 为落盘判定的灵敏度；`require_artifact` 校验卸载项指向的主程序**存在且为有效 PE**；`launch_check` 为**重量校验**（真正启动一次），默认关闭
> - **单实例锁（F19.4）**：`lock.enabled` 默认 `true`，`lock.wait_sec: 0` 表示不等待、直接以退出码 6 返回；**并发实例会互相污染安装证据 diff，除非确认无状态交叉，否则不建议关闭**
> - **`--timeout` 命令行参数**只覆盖 `timeout_sec`（单文件硬上限），不会覆盖 `idle_timeout_sec` / `task_timeout_sec`

#### 7.5.1 配置加载、覆盖与校验规则

**① 配置来源与优先级（由高到低）**

| 层级 | 来源 | 说明 |
|------|------|------|
| 1 | **命令行参数** | `--timeout` / `--output` / `--log-file` / `--log-level` / `--recursive` / `--pretty` / `--no-archive` / `--no-ocr` 等，仅覆盖其对应的字段 |
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
│   ├─ vcredist\  VC_redist.x64.exe           （Win7 场景可能需要）
│   └─ mount\     WinCDEmu 便携版             （可选：仅 Win7 需要挂载轨时使用）
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
| D11 | **7-Zip 离线安装包**（`7z<版本>-x64.msi` / `.exe`） | 解压后端，**必需**（除非 `--no-archive`）；**版本须 ≥ 24.09**（ISO/UDF 解析与 MotW 修复，见 F16.1）；附版本号与 SHA-256 记录 |
| D12 | **OCR 引擎离线包**：`PaddleOCR-json`（默认）+ `RapidOCR-json`（无 AVX 备选） | 绿色解压即用；建议内部归档固定版本，避免上游变更导致识别行为漂移 |
| D13 | **VC++ 2015-2022 运行库离线安装包**（`VC_redist.x64.exe`，可选） | Win7 场景 OCR 引擎可能需要；按目标镜像实际情况决定是否随包 |
| **D15** | **（可选）光盘映像挂载工具离线包**：WinCDEmu 便携版（LGPL） | **仅 Win7 需要挂载轨时随包**；Win8+ 使用系统自带 `Mount-DiskImage`，无需此项（F16.6） |

**C 组：文档、配置与脚本**

| 编号 | 交付物 |
|------|--------|
| **D14** | **部署说明文档 `DEPLOY.md`（新增，P0）**：照做式步骤——① 前置检查（系统版本 / 权限 / AV 排除）；② 安装 7-Zip（命令行示例）；③ 部署 OCR 引擎（解压路径 + `config.json` 指向）；④ 安装 VC++ 运行库（如需）；⑤ `--check-prereq` 校验；⑥ 拷贝 exe 到运行目录；⑦ 常见问题（AV 拦截、AVX 缺失）；**⑧（可选）Win7 挂载轨：部署 WinCDEmu 便携版并设置 `iso.mount_tool_path`**；**⑨ 光盘映像相关配置说明（`iso.*` 各字段与 MotW 策略）**。**要求：运维只读此文档即可独立完成部署** |
| D5 | 使用说明文档（README：**工具本体免安装使用方式**、CLI 参数（含 `--output` / `--output -` / `--log-file` / `--log-level` / `--pretty`）、**输出通道说明（F10.1）**、退出码表、`--check-prereq`（含 `--json`）/ `--dump-ui` / `--no-archive` / `--no-ocr` 用法、结果 JSON 说明） |
| D9 | 环境前置检查表与镜像构建指引（依赖清单、离线包放置路径、管理员权限 / UAC / AV 排除目录 / 可写临时目录、`--check-prereq` 卡点用法；对应 4.7 与 8.2） |
| D2 | 外部覆盖配置模板 `config.example.json`（**可选**；含全部字段与内嵌默认值，**可由 `AutoTrigger.exe --dump-config` 生成**；字段含义与取值范围由 D14 说明，外部配置**只需写要修改的字段**，见 7.5.1） |
| D3 | 可外部编辑的参数/关键词表（`silent_params.windows.json`、`buttons.zh-CN.json`、**`install_entries.json`（F17.2 安装入口命名模式）**、**`exclude_entries.json`（F17.3 排除清单模式）**、**`media_kinds.json`（F18.1 光盘映像介质性质判定规则）**）与图像模板目录（含 F14.2 初始模板集，均为可选覆盖） |
| D4 | 平台抽象层接口定义文档（对应 4.3）+ 架构说明（对应 7.3）+ **Win7/Win10 差异说明（对应 4.6）** |
| D6 | 测试样本集与预期结果对照表（重点覆盖：静默成功 / 向导成功 / **自绘界面 OCR 定位** / **光盘映像（含 `malware_lure` 与 `os_install_media` 两类）** / 需联网 / 装不成功 / 前置缺失 七类；**并在 Win7 环境复测**） |
| D7 | 结果 JSON Schema 文件（供下游流水线校验；须覆盖 `error` / `truncated` / `finished_at: null` 等失败与中断形态，见 F10.2 R6/R7；A28 ④⑧ 以其为判据） |
| D8 | 构建与检查脚本（Makefile / build.ps1：**Go 1.20.x 静态构建** + gofmt/vet/lint + 单测覆盖率 + **导入表核验**） |
| D10 | **OCR 环境验证脚本** `scripts/verify-ocr.ps1`（现有版本验证 Windows 内置 OCR；**M1 需扩展为多引擎探测 + 端到端识别验证**，输出文本与四角像素坐标） |
| **D16** | **第三方集成契约包**（F19）：`CONTRACT.md`（CLI 稳定面、退出码语义、Schema 兼容策略、能力协商说明）、**契约测试套件**（golden 结果 JSON + Schema 校验脚本）、**集成示例**（cmd / PowerShell，覆盖单文件、批量、结果判定、超时设置、失败分支）、`--capabilities` 输出样例 |

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
| A25 | **软件包结构识别与入口优先（F17）** | ① 对含 `install.bat` + `setup.exe` + `unins000.exe` + 数十个 DLL 的测试包：**`unins000.exe` 全程未被执行**（结果中记 `excluded_blacklist`）；② 定向阶段按 P0~P4 顺序执行，取得 `install_completed` 后其余文件记 `skipped_after_success`，**不再全量扫描**；③ 阶段 1 全部失败时**回退执行 P5 及其余可执行文件**（验证 `fallback_to_brute: true`）；④ 压缩包场景**先 `7z l` 列清单**，解压前即输出入口分层；⑤ `setup_x64.exe` 与 `setup_x86.exe` 并存时优先匹配当前系统架构；⑥ P0 的 readme 提示指向不存在的文件时**不产生任何执行动作**；⑦ `mode: brute` 下行为与传统全量遍历一致，但**黑名单仍强制生效** |
| A26 | **光盘映像介质判定与安全拦截（F18.1 / F18.5）** | ① 含 `sources\install.wim` 的 Windows 安装 ISO：**判定为 `os_install_media` 且全程不执行任何文件**，结果记 `skipped: os_media`（`allow_os_media: false` 时）；② 含 `BDMV\STREAM\*.m2ts` 的影碟 ISO 记 `skipped: media_disc`；③ 仅含一个 `.lnk` + 载荷的 ISO 判定为 `malware_lure` 且**优先执行该 `.lnk`**；④ 含 `autorun.inf`（`open=setup.exe`）的 ISO：`setup.exe` 被列为 **P0** 并按先于其他入口执行；⑤ 乱码文件名（GBK/Joliet）场景下 `setup.exe` 仍能被识别为 P1/P2，**不因乱码整体跳过**；⑥ 所有判定结果出现在 `layout_analysis.media_kind` |
| A27 | **光盘映像双轨与挂载清理（F18.2 / F18.3）** | ① 常规 ISO 在 `extract_first` 下走解压轨，`iso_strategy_used: "extract"`；② 构造一个 7-Zip 无法解析的映像（或用损坏/非标准 UDF 样本）：**自动回退挂载轨**，`iso_strategy_used: "extract→mount"`，且**后续仍能正常触发入口**；③ 挂载轨安装期间**保持挂载**，`install_completed` 判定与 `settle_sec` 稳定期结束后才卸载，`dismount_status: "ok"`；④ **异常终止（强杀）后重新启动工具，遗留挂载被自动清理**（或无法确认归属时只告警不误卸）；⑤ `mount_only` 且挂载后端缺失时：告警 + 记录 `mount_failed`，**不新增退出码**，其余样本继续处理；⑥ Win7 上默认走解压轨可完整跑通（不依赖挂载后端）；⑦ `iso.keep_extracted_on_success: false` 时解压目录被清理，`true` 时目录保留且路径写入结果 |
| A28 | **输出可解析性（F10.1~F10.3）** | ① `--dump-config > f.json` 与 `--output - > f.json` 产出的文件**均能被严格 JSON 解析器一次解析成功**（stdout 无任何日志/进度/提示混入），`--output - \| jq` 可直接工作；② 结果文件为 **UTF-8 无 BOM、单个完整文档、无尾随文本**，固定字段（`schema_version` / `task_id` / `error` / `results` / `truncated` 等）**恒存在**，无值字段为 `null`；③ 输出到 `--output` 指定路径与 `--output -` 的**内容语义完全一致**；④ 输入不存在（退出码 2）/ 配置错误（3）/ 7-Zip 缺失（4）三条失败路径**同样产出结构完整的结果 JSON**（`results: []` + `error.code` 与 `error.message`），且可过 D7 Schema；⑤ 未指定 `--output` 时结果落在**当前工作目录** `.\autotrigger-result.json`，且**不落在输入目录内**；⑥ 任务被强杀（模拟）时目标路径**不存在半截 JSON**（原子写生效），且 `.journal.ndjson` 保留、可逐行解析并含已完成文件结果；⑦ 正常结束后 journal 被删除（`keep_journal_on_success: false`），无 `.tmp` 残留；⑧ 超过 `max_file_size_mb` 的结果置 `truncated: true` 且**仍可解析、可通过 Schema 校验**；⑨ `--check-prereq --json` 输出可被机器解析（供镜像构建流水线卡点） |
| A29 | **安装成功判定的可靠性（F15）** | ① **假成功识别**：构造一个"只解压落盘到 `%TEMP%`、退出码 0、无卸载项"的 bootstrapper 样本 → **不得判 `install_completed`**（至多 `install_unverified`）；② **S 级证据成立**：标准 MSI/Inno 安装成功 → `install_completed` + `install_confidence: high` + `install_evidence` 内含 S 级条目；③ **基线生效**：`watch_dirs` 预先放置同名目录时，**不得**把已有文件误判为"新增落盘"；④ **HKCU 覆盖**：per-user 安装（仅写 `HKCU\...\Uninstall`）→ **能判定 `install_completed`**；⑤ **反证据优先**：安装器退出码 0 但 GUI 出现失败页/日志含回滚 → 判 `install_partial` 且 `evidence_conflicts` 非空，**不得**判成功；⑥ **MsiInstaller 事件**：MSI 安装后基线水位线之后出现 11707 → 记入 S4；随后执行卸载（11724）→ 判 `install_partial`/`failed`；⑦ **同一文件多次尝试共用基线**：第 1 次尝试已落盘部分文件、第 2 次成功时，**仍能正确判定**（不因基线被刷新而漏判）；⑧ 结果中 `install_confidence` **恒存在**（非安装判定为 `null`） |
| A30 | **活动感知超时与超时后判定（F9）** | ① **长安装不被误杀**：构造持续写入文件的慢安装器（总时长 > `idle_timeout_sec` 且 < `timeout_sec`）→ **不触发静默期超时**，正常完成；② **卡死被识别**：构造启动后无任何动作的进程 → 在 `startup_timeout_sec`（±20%）内判定 `failed: no_activity`；构造运行中突然静止的进程 → 在 `idle_timeout_sec`（±20%）内判定卡死并进入终止流程；③ **终止顺序**：终止过程依次为优雅（`WM_CLOSE`）→ 温和 → Job Object 强杀，`termination_mode` 如实记录；④ **超时后仍采集证据**：安装已完成主体、仅卡在"完成"页而被超时终结的样本 → **仍判 `install_completed`**（notes 注明超时终结）；⑤ `timeout_sec: 0` 时单文件不限时；⑥ `task_timeout_sec` 生效：超时后停止派发新文件、未处理项记 `skipped: task_timeout`、结果 JSON 正常写出；⑦ 心跳按 `progress_interval_sec` 出现在 **stderr**，`stdout` 无任何心跳内容；⑧ `Ctrl+C` 中断 → 退出码 130、结果 JSON 完整（含 `error.code: 130`）、journal 保留 |
| A31 | **第三方集成契约（F19）** | ① 传入**未知命令行参数** → 以退出码 2 失败并提示参数名（**不得静默忽略**）；② `--capabilities --json` 输出可被解析，且其 `supported_final_status` 与实际可返回的枚举值**完全一致**；③ `--version` 与 `--capabilities` 中的 `schema_version` 一致；④ **单实例锁**：并发启动两个实例 → 第二个以**退出码 6** 返回（或按 `lock_wait_sec` 等待），且**不产生交叉污染的安装证据**；⑤ **重复调用同一已安装软件** → 得 `already_installed`（或凭 A3/S1 得 `install_completed`），**不得**报成 `triggered`/装不上；⑥ **Schema 兼容**：用上一版本的 golden 结果 JSON 跑当前 Schema 校验**必须通过**（只做加法）；⑦ `summary.by_status` 各项之和**恒等于 `total`**；⑧ 契约测试套件与集成示例随交付提供，且示例脚本可直接跑通 |

---

## 10. 开发里程碑

| 里程碑 | 内容 | 覆盖功能 |
|--------|------|---------|
| M0 环境与选型验证（新增） | ① **Go 1.20.x 工具链与依赖库可用性确认**；② **Win7 SP1 x64 运行验证**（截屏 / DPI / Job Object / UIA 能力基线）；③ **OCR 引擎接入 spike**（PaddleOCR-json 探测、stdio 通信、四角坐标解析、AVX 检测） | 4.6、7.6、D10 |
| M1 核心链路与前置 | CLI + 配置加载 + 类型识别 + PE/MSI/脚本触发 + 多轮参数尝试 + **活动感知超时与四段式超时（F9.1~F9.6）** + **输出通道与结果可解析性（stdout/stderr/文件分流、原子写、F10.1~F10.3）** + **7-Zip / OCR 引擎自检与用户提示** + **对外契约冻结（F19.1~F19.4：CLI 稳定面、Schema 契约、`--capabilities`、单实例锁）** | F1 F2 F3 F4 F5 F9 F10 F13 F16 F19 |
| M2 安装成功能力 | GUI 弹窗自动化（UIA + 模板 + **OCR 引擎三级识别**）+ **安装证据分级与基线快照（F15.1~F15.6）** + 反证据与冲突处理 + 安装后校验 + 已安装检测 + 文件夹遍历 + 压缩包递归 + **F17 结构识别与入口优先排序** + **F18 光盘映像专项（介质判定 + 解压/挂载双轨）** + 文档/未知兜底 | F6 F7 F8 F11 F12 F14 F15 F17 F18 |
| M3 打磨与交付 | 单文件静态构建、退出码完善、日志打磨、Schema 校验、测试样本集、README 与 **DEPLOY.md 部署文档**、**交付包打包含 third_party 离线物料**、**Win7 + Win10 双环境冒烟**、**导入表与依赖边界核验（A22）**、**交付包完整性验证（A23）**、**配置加载与覆盖验证（A24）**、**输出可解析性验证（A28）**、**判定可靠性与超时策略验证（A29/A30）**、**契约测试套件与集成示例（D16、A31）**、lint 与覆盖率达标 | D1~D16、A8~A31 |

---

## 11. 风险与对策

| 风险 | 对策 |
|------|------|
| 样本反沙箱检测，装死不触发 | 工具层面无法解决，需沙箱环境加固；工具确保触发动作本身完整 |
| 安装器需联网下载组件 | 离线环境按超时处理并标记 `requires_network`，不做网络补偿 |
| 安装程序卡在 GUI 等待输入 | 超时强制结束并记录；有 F14 时优先由 GUI 自动化推进 |
| 压缩包嵌套过深或 zip 炸弹 | 递归深度限制（默认 3 层）+ 解压大小限制（默认 2 GB）；**F17.1 在解压前用 `7z l` 预判总量，超限则不落盘** |
| **盲扫执行到卸载程序，破坏已建立的安装结果** | F17.3 排除清单**强制生效、不可配置关闭**（卸载/修复/更新检查/调试类绝不执行）；被排除项记 `excluded_blacklist` 保留审计；验收 A25 ① 专卡此项 |
| **包结构不规范导致入口漏识别**（如 `i.bat`、`a.exe`、无 readme） | F17 只调整**优先级**、不缩小**覆盖面**：阶段 2 兜底回退到全量遍历（`fallback_to_brute` 默认 `true`）；入口命名模式表 `install_entries.json` 支持外部扩充 |
| 压缩包解压前预判失误（清单与实际不符） | `7z l` 结果仅作排序与预估；实际解压后仍重跑结构扫描，以磁盘实际内容为准 |
| **7-Zip 无法解析部分 ISO**（非标准 UDF 2.5 目录项 CRC、蓝光 `FileType=249`、加密/多重会话映像） | 不把它当"解压失败"处理：**自动回退挂载轨**（F18.2）；两轨皆失败记 `iso_unhandled: unsupported_image`；镜像建议使用 **≥ 24.09** 的 7-Zip（F16.1） |
| **误触发操作系统安装盘，把测试机装成系统** | F18.1 判定 `os_install_media`（`sources\install.wim` / `install.esd` 等）**默认跳过**，需显式 `allow_os_media: true` 才触发；判定结果入 `layout_analysis.media_kind` 可审计（A26 ①） |
| **挂载轨残留**（异常退出未卸载、盘符被占用、文件被占用导致卸载失败） | ① 安装期间保持挂载、判定完成后才卸载（`keep_mounted_until_verified`）；② 卸载前结束占用进程 + 短延时重试（默认 3 次）；③ 工具启动时清理**可确认归属**的遗留挂载，无法确认的只告警不误卸；④ 挂载与卸载全部纳入 Job Object 与全局看门狗（F18.3） |
| **挂载轨不可用**（Win7 无原生 API、盘符冲突、稀疏映像、映像已在挂载） | 默认策略本就是解压轨（`extract_first`），Win7 无此依赖；挂载失败记 `mount_failed` 并回落解压轨；**不新增退出码**（F16.6） |
| **MotW 传播差异导致 SmartScreen 拦截安装器**（Win11 22H2+ 挂载 ISO 会传播 MotW） | `iso.motw_policy` 可配置并在结果中记录实际生效值，便于解释失败原因；**工具不提供绕过实现**，是否让 SmartScreen 参与由安全团队定（F18.4 / Q14） |
| **解压目录被清理后，按需从源安装的 MSI 修复/卸载失效** | `iso.keep_extracted_on_success: true` 可保留解压目录并在结果中记录路径（此时不计入"无残留"，README 说明）；默认 `false` 适用于只做"触发"的场景 |
| **映像内长路径导致解压或执行失败（`MAX_PATH`）** | 解压根目录固定使用**短路径**（`archive.temp_root`，如 `C:\AT\t\<taskID>\`）（F18.6） |
| **老光盘文件名乱码导致入口漏识别**（Joliet/GBK 编码） | F18.5 要求关键字匹配做**编码容错与宽松匹配**；不确定时按 P1 命名规则与 P5 兜底，不直接跳过（A26 ⑤） |
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
| **日志/进度混入 stdout，导致下游解析结果失败** | **三通道严格分流**（F10.1）：stdout 只承载机器可读输出，日志一律走 stderr + 日志文件；禁用 `fmt.Println` 写 stdout（7.3）；验收 A28 ① 用 `--output - \| jq` 与 `--dump-config > f.json` 双用例卡死 |
| **任务被强杀/崩溃导致结果文件损坏或全量丢失** | 结果文件**原子写**（`.tmp` → rename，无半截 JSON）；逐文件追加 `journal.ndjson` 供中断找回（F10.3）；`finished_at: null` 作为"未正常结束"的判据；失败路径亦产出结构完整产物（F10.2 R6） |
| GUI 自动化在无桌面会话（Session 0）失效 | 明确要求前台桌面可用；批量模式默认串行，避免抢焦点 |
| 自绘安装器无 UIA 树导致识别失败 | 图像模板兜底 + **外部 OCR 文本定位** + `--dump-ui`（含 OCR 结果）采集补模板 + 模板目录外部热更新 |
| **安装成功误判（退出码 0 但实际未装）** | **证据分级（F15.1）**：退出码 0 仅算 A 级，不得单独判定；`install_completed` 需 S 级或双 A 级；**基线快照（F15.2）** 才使"新增"可判定；**反证据优先（F15.3）** |
| **假阳性：把"看起来装成了"当成"装成了"** | 证据分级 + `install_confidence` 如实标注；仅弱证据降级为 `install_unverified`；**能力边界在 F15.8 明示**（判定"安装动作完成"≠"软件可用"） |
| **假阴性：已安装的软件被报成"装不上"（第三方重复调用）** | `pre_installed` 检测 + `already_installed` 状态（F15.5）；升级场景由 A3 + N4 升级豁免处理 |
| **并发实例互相污染注册表/文件系统 diff** | **单实例锁（F19.4）**：`--lock` 默认开启，第二实例以退出码 6 返回；这是"最隐蔽的可靠性杀手" |
| **长安装被误杀（固定超时不适合大软件）** | **活动感知超时（F9.1/F9.2）**：无进展才计时；单文件硬上限由 120s 放宽到 600s（可配、`0` = 不限） |
| **进程卡死未被识别（白等）** | 静默期超时（默认 60s 无进展即判卡死）+ 六类进展信号（CPU 时间 / 目录写入 / 子进程 / GUI / 注册表 / IO） |
| **超时后仍有部分安装但结果未反映** | F9.4：**超时后仍执行 F15 证据采集**；有副作用未完成 → `install_partial`；已完成 → 仍判 `install_completed` 并注明 |
| **直接强杀导致 MSI 留待回滚事务，污染后续样本证据** | 终止顺序写死为**优雅（`WM_CLOSE`）→ 温和 → Job Object 强杀**（F9.3），`termination_mode` 留痕 |
| **下游用退出码推断单文件结果而误判** | F19.1 明确契约：退出码只表示工具自身状态，**单文件结果只在 JSON**；A31 ① 卡未知参数、A28 口径同步 |
| **同输入不同结果，调用方无法解释** | F19.5：结果恒记 `prereq` / `platform_meta` / `config_digest`；**明确不承诺跨环境一致**，并要求三点可比性（镜像 / 配置 / 权限） |
| **证据采集自身开销过大（深扫 Program Files）** | 快照**只记目录层 + 计数**（`snapshot_depth` 默认 2），`snapshot_max_entries` 上限保护，单次 < 500ms；基线明细默认不写入结果 |
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
| Q4 | 结果 JSON 的**交付方式**？（离线环境无上传通道：本地文件 / 共享目录 / 人工拷贝。**输出通道语义已定案**见 DC15 / F10.1，此问仅剩"结果文件如何出沙箱"） | 影响 F10 输出落点与 DEPLOY.md |
| Q5 | 是否需要保留同一文件多次任务的尝试历史（用于"触发方式优先级动态调整"）？ | 影响结果存储设计 |
| Q6 | 安装成功是否需要更强证据（如启动安装后的程序验证可用性）？ | 影响 F15 判定精度与工作量 |
| **Q7** | **OCR 引擎最终选型**：镜像统一预置 `PaddleOCR-json`（推荐，需 AVX）还是 `RapidOCR-json`（免 AVX）？是否需要同时预置两者以覆盖不同 CPU？ | 影响 7.6、F16.3 与镜像体积 |
| **Q8** | OCR 引擎的**版本与离线包来源**如何固定？（建议：内部归档一份发行包并记录版本号，避免随上游变更导致行为漂移） | 影响镜像构建与可复现性 |
| **Q9** | 是否需要保留 Windows 内置 OCR 作为回退后端？（Win10+ 环境零依赖，但语言包受限、实测仅中文可用） | 影响 F14.6 工作量 |
| **Q10** | **目标环境是否确实包含 Windows 7？** 若实际只在 Win10+ 运行，可改用 Go 1.21+（生态更新、库选择更多）并简化 4.6 的兼容处理 | 影响 7.1 工具链、4.6 与 A20 工作量 |
| **Q11** | 自绘安装器（Electron/CEF 类）在样本中的占比？若占比高，OCR 与模板的投入应先于 UIA 深化 | 影响 M2 优先级排序 |
| **Q12** | 目标虚拟机能否**为工具目录配置 AV/EDR 排除项**？（若不能，需考虑代码签名或改用其他分发方式） | 影响分发可行性与镜像构建流程（4.7 C 类） |
| **Q13** | **光盘映像的默认处理轨**是否确认为解压优先（`extract_first`）？**挂载轨（轨 B）本期就做，还是先留接口、只做解压轨**？ | 影响 F18.2 的 M2 工作量与 A27 验收范围 |
| **Q14** | **MotW 策略默认值**（`native` / `preserve` / `clear`）由谁定？即：EDR 测试是否**希望 SmartScreen 拦截链也参与**（`preserve`）还是排除其干扰（`clear`）？ | 影响 F18.4 默认值与测试口径（安全团队决策项） |
| **Q15** | ① 样本集中是否有**操作系统安装 ISO**（需明确是否允许触发）？② 是否存在 7-Zip 无法解析、必须靠挂载的映像（决定 Win7 是否需要随包 WinCDEmu）？ | 影响 F18.1 默认拦截策略、D15 交付物与镜像部署说明 |
| **Q16** | 下游流水线**何时消费结果**：任务全部结束后整体读取结果 JSON，还是需要**过程中流式读取**（决定是否额外提供 `--ndjson` 逐条输出模式）？另：结果文件默认落在**当前工作目录**是否可接受，还是需固定为共享目录？ | 影响 F10 输出模式与调用方约定 |
| **Q17** | **"安装成功"的验收口径由谁定**：判定标准从严后（需 S 级证据），会出现一批"实际可用但只拿到 A 级证据"的软件被记为 `install_unverified`。测试团队**希望更严（宁缺毋滥）还是更宽（宁可多报）**？另：是否需要开启 `verify.launch_check`（真正启动一次验证可用性）？ | 影响 F15.7 判定阈值与 F15.4 默认值 |
| **Q18** | 默认 `install_verify.watch_dirs` 是否**追加 `%LOCALAPPDATA%` / `%APPDATA%` / `%ProgramData%`**？若样本以 per-user 安装或"绿色版解压即用"为主，必须追加，否则大量漏判 | 影响 F15.2 快照范围与判定准确率 |
| **Q19** | 第三方是否为**多实例并发调用**场景？`--lock` 默认开启会串行化调用（第二实例退出码 6）。若调用方流水线本来就并行派发，需要协商队列策略（等待时长 / 调用方自建队列 / 关闭锁并接受归因风险） | 影响 F19.4 锁策略与调用方集成方式 |
| **Q20** | 单文件硬上限默认 **600s** 是否合适？若样本中确有大软件（Office / VS / 大型游戏载体），需上调或用 `0` 不限；同时 `task_timeout_sec` 是否需要一个默认值（当前 `0` = 不限，长批量任务可能跑到数小时） | 影响 F9.1 默认值与调用方外部超时设置 |

> 已决策项（不再讨论）：交付形态（免安装单文件）、语言（Go）、解压后端（外部 7-Zip，不降级、不代装）、7-Zip 缺失处理（检测 + 提示）、**OCR 引擎（外部开源引擎，默认 PaddleOCR-json，进程外调用，不代装）**、**平台基线（Win7 SP1 x64 及以上，Go 1.20.x）**、GUI 识别策略（UIA → 模板 → OCR 三级）、**光盘映像处理（专项流程 F18：介质性质判定 + 解压/挂载双轨 + 解压轨优先）**、**安装成功判定（F15：证据分级 S/A/B + 基线 diff + 反证据，`install_completed` 需强证据，附 `install_confidence`）**、**超时策略（F9：活动感知 + 四段式，`idle_timeout_sec` 为主把关）**、**输出与集成契约（F10.1~F10.3 + F19：三通道分流、只做加法、单实例锁、第三方以结果 JSON 为准）**、主目标（安装成功优先）、环境（离线）—— 见 1.2。

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
- **扩展光盘映像挂载能力**：VHD / VHDX / IMG / DMG 等虚拟磁盘类样本（挂载后同样按 F17 结构分析触发；VHD 挂载需管理员权限，与 ISO 不同）
- **物理光驱介质支持**：直接从物理 DVD 驱动器（`DRIVE_CDROM`）读取并触发，覆盖"必须真光盘"的样本
- **MotW / SmartScreen 拦截链的行为采集**：把"被 SmartScreen 拦截"本身作为一类可观测结果纳入统计（需与安全团队确认口径）

---

文档结束
