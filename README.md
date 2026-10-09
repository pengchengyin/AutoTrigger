# AutoTrigger 自动安装工具

依据《自动安装工具prd-v2.17.md》实现的 Windows 离线工具。工具本体是 Go 1.20.14、CGO 关闭的单一 `AutoTrigger.exe`，同时提供命令行与图形界面，Windows 7 SP1 x64 是构建目标。当前版本 `0.1.0`，结果 Schema `2.8`，资源包 `0.1.0`。

## 图形界面

在资源管理器中**双击 `AutoTrigger.exe`** 即可打开界面（无参数且独占新控制台时自动弹出）；也可用 `AutoTrigger.exe --gui` 显式启动。

用**【选择文件…】**和**【选择文件夹…】**录入样本路径（直接弹系统对话框，不用手打路径），可改结果路径、配置路径、超时、日志级别，以及递归、跳过压缩包、关闭 OCR、单实例锁等开关；支持**一键生成默认 `config.json`**（已存在时不覆盖）与**打开编辑配置**；底部实时显示**等价命令行**，可一键复制去脚本里复跑。

点【开始】后界面在后台调用与命令行**完全相同**的实现：参数校验、退出码语义与结果 JSON 一致，结果文件照常写出，日志区实时滚动；运行中可【停止】触发协作式中止。

从**已有终端、管道或带任意参数**调用时**不会弹窗**，仍走命令行（行为与旧版本一致）。界面只用系统 DLL，不引入任何第三方 GUI 库，产物仍是单个静态 exe；非 Windows 平台不提供界面，会提示改用命令行。

## 使用

```powershell
.\AutoTrigger.exe --help
.\AutoTrigger.exe --version
.\AutoTrigger.exe --check-prereq --json
.\AutoTrigger.exe --input 'D:\samples\setup.exe' --output 'D:\results\result.json'
.\AutoTrigger.exe --input 'D:\samples' --recursive --output 'D:\results\batch.json'
.\AutoTrigger.exe --input 'D:\samples\setup.exe' --no-archive --no-ocr --output -
.\AutoTrigger.exe --capabilities --json
```

**双击** exe（无参数且独占新控制台）会打开图形界面；而**从终端、脚本或管道**不带参数运行则打印用法并以退出码 2 结束，**不产生结果文件与日志**（避免在用户当前目录留下无意义产物）。`--help` / `-h` 输出同样的用法到 stdout 并以 0 退出。

`--output -` 的 stdout 只有一个 JSON 文档。日志、进度和依赖提示写 stderr 与日志文件。PowerShell 5.1 重定向可能将 stdout 转为 UTF-16；第三方集成推荐 `--output result.json`，结果文件始终为 UTF-8 无 BOM。

首次部署按 [DEPLOY.md](DEPLOY.md) 准备 7-Zip 和 OCR。程序只检查前置，不代装、不下载。`--no-archive` 跳过压缩包和映像，`--no-ocr` 关闭 OCR。

## 安装行为

PE 文件先尝试静默参数，再无参数启动。MSI 使用 `/quiet`、`/passive` 和向导模式；脚本通过系统解释器执行。支持快捷方式目标解析、DLL 导出探测、文档关联宿主及扩展名伪装识别。目录/压缩包先分析入口，强制排除卸载、修复、更新、调试和帮助程序；成功后停止本包后续执行。前置依赖先执行，但前置安装成功不会代替主应用的安装成果。

GUI 仅处理样本 Job Object 进程树的可见窗口，按 UIA/Win32 → 模板 → OCR 降级。中英文本归一化、拒绝词、动作上限和同帧去重用于约束点击。UIA 与截图在可终止的辅助进程中运行。工具需要交互式桌面；锁屏、服务会话和 UAC 安全桌面无法保证点击。

每文件的全部尝试共用一份基线；采集 HKLM、WOW6432Node、HKCU 卸载项、监控目录与 MSI 事件。`loose` 为默认判定档，`strict` 可配置；退出码 0 无副作用不会判成功。反证据优先，超时后仍采证。**安装动作完成不等于软件可正常使用**：需要重启、激活、运行库或源介质的情况仍可能不可用。

MSI 的 `/L*v` 详细日志与 msiexec 的本地化消息都是 UTF-16，工具会先解码再使用（否则日志关键词无法匹配），失败时退出码会附中文释义。**未判安装成功的 MSI 样本**会把各次尝试的详细日志复制到结果文件同目录的 `<结果名>-msi-logs\`，便于排查 1603 这类通用失败码；`msi.keep_verbose_log` 可关，`msi.log_dir` 可改路径，结果 JSON 的 `attempts[].msi_log` 记录原始路径。

## 参数

| 参数 | 含义 |
|---|---|
| `--input` / `--recursive` | 单文件或目录；递归目录扫描 |
| `--timeout` | 单文件全部尝试硬上限，默认 600 秒，0 不限 |
| `--output` / `--pretty` | 结果路径或 `-`；是否缩进 |
| `--config` / `--dump-config` | 增量配置；导出生效配置 |
| `--log-file` / `--log-level` | 日志路径与级别 |
| `--no-archive` / `--no-ocr` / `--require-ocr` | 跳过能力或要求 OCR 可用 |
| `--lock` / `--no-lock` | 默认机器级单实例锁；第二实例退出 6 |
| `--dump-ui --out <目录>` | 启动输入并采集窗口、UIA、OCR，不自动推进向导 |
| `--dump-resources <目录>` | 导出内置规则与模板索引 |
| `--no-builtin-templates` | 停用内置图像模板 |
| `--self-test templates` | 逐张源图匹配，空模板集也会失败 |
| `--capabilities --json` / `--version` | 能力、当前资源数量和版本 JSON |
| `--help` / `-h` | 用法、参数分组、退出码表与示例；写 stdout 并退出 0 |
| `--gui` | 打开图形界面；须单独使用（等价双击 exe） |

未知 CLI 参数退出 2 并提示参数名，只附一行用法摘要并指向 `--help`，不刷整屏帮助。未提供任何参数时输出完整用法到 stderr 并以 2 退出，不产出结果文件与日志。配置未知字段告警并忽略，类型与范围错误退出 3。只支持串行 `concurrency: 1`，防止证据归因交叉。默认结果是当前目录的 `autotrigger-result.json`；当前目录位于输入目录内时，默认移至 `%TEMP%\AT-results\<task_id>.json`，绝对路径写日志。显式输出不得覆盖输入文件。

## 输出契约

`pkg/schema/result.schema.json` 是结果 Schema。固定字段恒存在，无值为 null；数组为空时写 `[]`。结果文件先写 `.tmp`、fsync、再原子替换。处理开始先写 `finished_at: null` 的完整结果，每完成一文件追加并 fsync `.journal.ndjson`，正常结束后删除 journal。旧中断 journal 会改名保留。stdout 模式不写 journal。Ctrl+C 退出 130 并完成清理、采证和结果输出。

| 退出码 | 工具状态 |
|---|---|
| 0 | 处理完成，具体文件结果看 JSON |
| 1 | 工具异常或当前平台不支持 |
| 2 | 参数或输入非法 |
| 3 | 配置/资源错误 |
| 4 | 输入需要 7-Zip 而未部署 |
| 5 | 要求 OCR 但不可用 |
| 6 | 单实例锁被占用 |
| 130 | 协作式中断 |

`summary.by_status` 为互斥桶，计数之和等于 total；`summary.flags` 可重叠。压缩包/映像自身与内部文件各有记录，total 包含容器；调用方统计应用安装数量时应排除容器记录。仅 `high` 可直接采信；`medium` 应抽样复核，`low` / `install_partial` 应二次核查。

## 构建与验证

```bash
AT_GO_BINARY=/path/to/go1.20.14/bin/go bash scripts/build.sh
```

Windows 使用 `scripts/build.ps1 -Go <go.exe>`。构建包含 gofmt/vet、测试、Windows/Linux/macOS 编译和 PE 系统 DLL 导入核验。发布模式 `AT_RELEASE=1` 或 `-Release` 额外要求真实模板数量、源图自检和 golangci-lint。开发构建允许尚无图像母版的资源包，并在 capabilities 如实报告。

`scripts/smoke-windows.ps1` 使用测试目录内的无害向导；`-Gui` 验证真实点击链，`-Archive` 验证 ZIP 解压安装。样本源码在 `testdata/wizard`，仅写 `AT_FIXTURE_DEST`，不修改注册表。先用 `GOOS=windows GOARCH=amd64 go build -o dist/wizard-fixture.exe ./testdata/wizard` 构建夹具。Win7、真实 MSI/Inno/NSIS/InstallShield 和 ISO 挂载须按 [验收状态](docs/验收状态.md) 补充实机验收。

将三份官方离线物料放入 `dist/third_party` 后，运行 `go run ./tools/package`，生成 `dist/AutoTrigger-0.1.0.zip` 与同名目录。清单含每个文件的 SHA-256；开发交付明确 `release_ready: false`。`go run ./tools/package --release` 会拒绝不齐备的离线物料与真实模板；不覆盖已有交付目录，需更换 `--out`。

## 当前边界

- 尚无真实安装器母版，内置模板数量为 0。模板加载、增量覆盖、生成和源图自检工具已实现，不能宣称达到 A32 的 50 母版/180 变体。UIA 和可用 OCR 可继续运行。
- Linux/macOS 仅提供编译 stub，执行明确返回 unsupported。
- Win7 构建兼容已约束，尚无 Win7 实测证据。Win7 ISO 挂载的 WinCDEmu 适配未启用，使用解压轨。
- OCR 支持 Paddle/Rapid stdio、Paddle 环回 TCP、Umi 本地 HTTP 和 Tesseract；Rapid v0.2.0 不支持 TCP。Windows 内置 OCR 后端保留配置值但当前不支持。Tesseract 每次调用其 CLI，不提供常驻服务。
- 强制结束当前工具进程后自动清理“历史挂载归属”的机制尚未启用；正常/协作中断均卸载本次映像。工具不会卸载归属不明或本来已挂载的映像。
- 文件快照受 450ms 与条目数预算限制；截断时禁用文件数量减少反证据并告警。第三方软件或后台安装器可造成系统证据归因噪声。
- `launch_check` 本期关闭；`write_baseline` 默认关闭。超时之后最多增加进程清理与证据收敛的有限时间，外部调用超时须留余量。

官方物料来源：[7-Zip](https://www.7-zip.org/download.html)、[PaddleOCR-json](https://github.com/hiroi-sora/PaddleOCR-json/releases)、[RapidOCR-json](https://github.com/hiroi-sora/RapidOCR-json/releases)。工具只通过本地进程和环回通信工作。
