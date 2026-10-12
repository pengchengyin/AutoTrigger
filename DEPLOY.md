# 离线部署说明

工具本体免安装，不需要 Go、Python、.NET、JRE 或 VC++ 语言运行时。7-Zip 与 OCR 是独立功能性前置；OCR 是否需要 VC++ 由其发行包决定。此版本为开发交付，完整生产验收状态见 `docs/验收状态.md`。

## 交付目录

运行数据默认放在 EXE 同级的 logs、results、templates、third_parties、config 中。在 GUI 设置中修改数据目录后，程序与安装脚本均读取 EXE 同级 config/data-location.json；配置文件默认是数据目录 config/config.json。旧 third_party 和根目录 config.json 可通过首次 GUI 提示复制迁移，原文件保留。脚本仍支持显式配置路径和部署路径，第三方参数的显式覆盖不变。

```text
AutoTrigger.exe
README.md
DEPLOY.md
config.example.json
result.schema.json
manifest.json
docs/
scripts/install-windows.bat
scripts/install-windows.ps1
scripts/verify-ocr.ps1
resources/
third_parties/7zip/7z2603-x64.exe
third_parties/ocr/PaddleOCR-json_v1.4.1_windows_x64.7z
third_parties/ocr/RapidOCR-json_v0.2.0.7z
```

`manifest.json` 记录文件 SHA-256、长度、版本、物料是否齐备和模板未验收状态。分发前用 Get-FileHash 核对。离线物料不得只提供下载网址；打包器的正式模式会拒绝缺少第三方离线包或真实模板的交付。

## 一键部署（推荐）

交付包的 `scripts\` 下带有一键安装脚本，把第三方离线物料变成"开箱即用"：

```text
scripts\install-windows.bat     ← 双击入口（转发给 .ps1，结束时保留窗口便于看结论与退出码）
scripts\install-windows.ps1     ← 实际逻辑（也可直接带参数调用）
```

脚本做四件事：

| 步骤 | 动作 |
|---|---|
| 1 | **7-Zip**：先复用系统上已有的 `7z.exe`（`-SevenZipExe` → `config.json` → 注册表 → `PATH` → 常见安装目录 → 交付包内便携目录）；确实没有才用 `third_parties\7zip\7z*-x64.exe` 静默安装 `/S /D=<目录>` |
| 2 | **OCR**：用上一步得到的 `7z.exe` 把 `third_parties\ocr\*.7z` 释放到 `<OCRRoot>\PaddleOCR-json` 与 `<OCRRoot>\RapidOCR-json`（保留 `models` 与 DLL，并压平压缩包内的单层根目录） |
| 3 | **config.json**：在数据目录 `config\config.json` 写**最小增量**配置（显式 `archive.sevenzip_path`；`ocr.engine_path` 保持 `auto`），覆盖前自动备份为 `config.json.bak-<时间戳>` |
| 4 | **自检**：调 `AutoTrigger.exe --check-prereq --json` 复核，结论与退出码与工具本体同源 |

```powershell
# 默认部署到工具目录；安装器可能仍要求 UAC 批准
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\install-windows.ps1

# 先看它会做什么，不做任何改动
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\install-windows.ps1 -Check
```

**权限**：非管理员也能跑通——脚本会把 7-Zip 装到交付包内的 `third_parties\7zip\7-Zip\`、把 OCR 释放到用户可写目录。但**7-Zip 的安装包会把它自身的安装路径写进注册表**（`HKLM`/`HKCU` 的 `SOFTWARE\7-Zip` 与卸载项），这是该安装器的固有行为，不是脚本附加的。若不想动注册表，可自行解压 `third_parties\7zip\7z*-x64.exe`（它本身就是 7z 自解压包，解开即得便携版），再用 `-SevenZipExe` 指向解出来的 `7z.exe`。

**参数与默认值**：

| 参数 | 默认 | 说明 |
|---|---|---|
| `-Root` | 自动探测（脚本上级目录 → 脚本目录 → 上级目录\dist） | 含 `AutoTrigger.exe` 的交付根目录 |
| `-SevenZipExe` | 空 | 指定已有 `7z.exe`，跳过发现与安装 |
| `-SevenZipDir` | `<Root>\third_parties\7zip\7-Zip` | 7-Zip 安装目标目录 |
| `-OCRRoot` | `<Root>\third_parties\ocr` | OCR 释放根目录 |
| `-Engine` | `auto`（两个引擎都装） | `auto` / `both` / `paddleocr_json` / `rapidocr_json` / `none` |
| `-ConfigPath` | `<Root>\config.json` | 写出的配置文件路径 |
| `-SkipSevenZip` / `-SkipOCR` / `-NoConfig` | 关 | 分别跳过对应步骤 |
| `-Force` | 关 | 目标已存在也重新释放（OCR 目标目录先清空） |
| `-Check` / `-DryRun` | 关 | 只探测 / 只演示，不落盘任何改动 |
| `-Json` | 关 | 结构化结果走 stdout，人类可读报告走 stderr（对齐 F10.1 三通道约定） |

退出码：`0` 前置就绪 / `4` 缺 7-Zip / `5` 缺 OCR（要求部署但未就绪）/ `1` 其他错误。比工具本体默认更严格——工具在 `ocr.require_ocr: false` 时 OCR 缺失也返回 0，本脚本要求"要装就必须装好"，便于镜像构建阶段做卡点。

**自动部署与脚本用途**：日常使用无需先运行脚本。GUI“检查环境 / 检查前置”和 CLI `--check-prereq` 会逐个发现、部署并自检 `third_parties\ocr` 下支持的便携 `.7z` / `.zip` 包，同一引擎的多个版本也独立处理；普通安装任务仅处理当前选定的引擎。`--check-prereq --no-deploy-deps` 仅检测已有程序与服务，未解压包标记未部署；`--capabilities` 不部署。程序不会覆盖已有配置、联网下载或自动安装 VC++ 运行库；工具目录不可写、物料缺失、能力自检失败均明确提示。7-Zip 安装器可能写注册表或弹出 UAC。

脚本仍保留用于镜像批量准备、`-Check` / `-DryRun`、指定目录和 `-Force` 修复。脚本默认把两个 OCR 引擎解压到数据目录 `third_parties\ocr`，`engine_path` 保持 `auto`，兼容不同 CPU。程序完整环境检查将各包释放到 `third_parties\ocr\packages\<包名与路径摘要>\`，保留整个发行包目录树，重复检查直接复用，不覆盖现有版本；`auto` 能递归发现这些程序。自定义目录仍需配置显式路径。

支持发现的便携包名以 `PaddleOCR-json`、`RapidOCR-json`、`Umi-OCR` 或 `Tesseract` 开头。PaddleOCR-json / RapidOCR-json 启动后实际发送识别请求，Tesseract 还检查语言数据；CPU 无 AVX 时不启动 PaddleOCR-json。Umi-OCR 包只自动解压，其本地 HTTP 服务由用户自行启动，程序列表与服务检测分开显示。Windows OCR 适配尚未实现，清单会明确标记，不伪装成可用。GUI 的“设置 → 识别与依赖”可查看可滚动完整清单，首页“检测详情”可搜索、复制各项路径与失败原因。JSON 新增可选 `ocr_engines` 清单；原标量 OCR 字段仍描述实际选中的引擎，不以替代引擎通过替代当前配置自检。

## 手工部署（等价步骤）

下述步骤就是脚本内部所做的事，适合必须手工核对每一环、或不希望工具代劳的场合。

### 镜像准备

安装中遇到管理员权限确认时，处理选项、关闭通知步骤、风险与恢复方法见 [使用说明：UAC 权限确认](README.md#安装时出现管理员权限确认uac)。日常电脑优先以管理员身份运行工具，不要求关闭 UAC。

目标 Windows 7 SP1 x64 或更新系统。准备可交互桌面、管理员运行权限、可写 TEMP 和样本/结果目录。镜像管理员按测试口径配置 UAC 与 AV/EDR 排除目录；工具不修改这些配置，也不替用户关闭系统保护。不能从无交互桌面的服务会话保证 GUI 安装。

1. 核对交付包 manifest 的 SHA-256。
2. 安装离线 7-Zip：默认可交由工具自动部署到 `third_parties\7zip\7-Zip`；手工执行时用 `/S /D=<工具目录>\third_parties\7zip\7-Zip`。最低版本默认 24.09；低版本告警但继续。安装器可能要求 UAC，便携版也可直接配置 `archive.sevenzip_path`。
3. 使用 7-Zip 将 PaddleOCR-json 包完整解压到 `C:\tools\PaddleOCR-json`，保留 exe 同目录的 DLL、models 和配置。不要只复制 OCR exe。
4. CPU 没有启用 AVX 时改用 RapidOCR-json，将其完整解压到 `C:\tools\RapidOCR-json`。发行包有自带模型配置时，应按发行包启动说明确认；运行 `scripts\verify-ocr.ps1` 做端到端验证。
5. 仅当 OCR 实际启动提示缺少 VC++ 时，由镜像管理员安装适用于该 Win7 镜像的离线运行库。当前交付不默认包含 VC++，工具不会下载。
6. 拷贝 `AutoTrigger.exe`，执行 `--check-prereq --json`。7-Zip 缺失返回 4，必需 OCR 缺失返回 5。默认可在 OCR 缺失时降级。

## 最小增量配置

将配置放在数据目录 `config\config.json`，或用 `--config` 指定。显式配置取代数据目录配置，不叠加；字段递归合并。

```json
{
  "archive": {"sevenzip_path": "C:\\Program Files\\7-Zip\\7z.exe"},
  "ocr": {
    "engine": "paddleocr_json",
    "engine_path": "C:\\tools\\PaddleOCR-json\\PaddleOCR-json.exe",
    "require_ocr": false
  }
}
```

> OCR 装在数据目录 `third_parties\ocr\<引擎名>\` 或常见 `C:\tools\<引擎名>\` / `D:\tools\<引擎名>\` 时，
> 建议写 `"engine_path": "auto"` 而**不要**写死路径：CPU 未启用 AVX 时工具会自动把引擎
> 切到 `rapidocr_json`，写死路径会让这个回落指到错误的 exe 上。`scripts\install-windows.ps1`
> 默认就是写 `auto` + 同时部署两个引擎。

运行 `AutoTrigger.exe --dump-config` 可导出所有实际默认值；`config.example.json` 为该命令生成的完整模板。关键范围如下：

| 字段 | 默认和范围 |
|---|---|
| `timeout_sec` / `task_timeout_sec` | 600 / 0，0 表示不限 |
| `startup_timeout_sec` / `idle_timeout_sec` | 30 / 60，正整数秒 |
| `concurrency` | 本期仅 1 |
| `max_attempts_per_file` / `attempt_interval_sec` | 9 / 2，正整数 / 非负秒；旧配置显式写 8 时仍保持上限 8，完整参数轮询需改为至少 9 |
| `archive.max_depth` / `max_extract_size_bytes` | 3 / 2 GiB；预检加实际解压监控 |
| `iso.strategy` | extract_first、extract_only、mount_first、mount_only |
| `iso.allow_os_media` / `allow_media_disc` | 均 false |
| `iso.motw_policy` | native、preserve、clear；只读挂载盘不能写 ADS 时明确失败 |
| `ocr.engine` / `transport` | paddleocr_json；stdio 或环回 TCP；Umi 使用 HTTP |
| `ocr.min_score` | 0.6，[0,1]；低分不点击 |
| `install_verify.judgment_profile` | loose 或 strict |
| `install_verify.watch_dirs` / `watch_exclude` | 默认五处安装目录，排除临时/缓存目录；支持 `%VAR%` |
| `install_verify.snapshot_depth` / `snapshot_max_entries` | 2 / 20000 |
| `install_verify.settle_sec` / `quiet_polls` / `settle_max_sec` | 3 / 3 / 30 |
| `install_verify.require_artifact` / `launch_check` | true / false；本期 launch_check=true 报配置错误 |
| `msi.keep_verbose_log` / `msi.log_dir` | true / 空；未判安装成功的 MSI 样本会把 `/L*v` 日志复制到 `log_dir`（留空 = 结果文件同目录的 `<结果名>-msi-logs\`），装失败时靠它定位 1603 之类的原因 |
| `lock.enabled` / `wait_sec` | true / 0；调用方自己串行排队 |
| `output.journal` / `keep_journal_on_success` | true / false |
| `output.log_level` / `max_file_size_mb` | info / 64；超限先删除 attempts 明细，保留固定记录 |
| `gui_automation.template_pack` | merged / builtin / external |
| `gui_automation.template_min_similarity` | 0.86，[0,1] |

模板为外部增量时提供 `templates/templates.index.json`，按 ID 覆盖。规则资源放 exe 同目录对应的 `keywords/`、`params/` 和 JSON 文件；排除清单的内置禁止项始终强制保留。外部规则文件损坏不会静默回落。

RapidOCR-json v0.2.0 只支持 stdio；需要 TCP 时选择 PaddleOCR-json。当前交付两份官方 OCR 压缩包的 stdio 已在本机实际启动并识别中英文，Paddle TCP 也已实测；仍需目标 Win7 镜像复测。`archive.require_sevenzip: false` 只取消强制前置要求，有可用 7-Zip 时仍解压；明确跳过归档用 `--no-archive`。

## 调用方

```powershell
& .\AutoTrigger.exe --input 'D:\samples\app.exe' --output 'D:\out\app.json'
$toolExit = $LASTEXITCODE
$result = Get-Content -Raw -Encoding UTF8 'D:\out\app.json' | ConvertFrom-Json
$result.results | Select-Object file_path, final_status, install_confidence, requires_reboot
```

退出码只表示工具状态。文件的安装状态以 JSON 为准，`install_completed` 不保证软件可用。外部超时应大于内部任务上限或按文件数估算，并额外预留终止及收敛时间。中断时先读主结果；`finished_at: null` 时按 task_id/schema_version 恢复 journal。

Win7 的 OCR 与 GUI、DPI、Job Object、真实安装器和离线整包部署均需在干净 VM 复测。不要将当前 Windows 主机的通过结果标成 Win7 通过。
