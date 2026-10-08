# 离线部署说明

工具本体免安装，不需要 Go、Python、.NET、JRE 或 VC++ 语言运行时。7-Zip 与 OCR 是独立功能性前置；OCR 是否需要 VC++ 由其发行包决定。此版本为开发交付，完整生产验收状态见 `docs/验收状态.md`。

## 交付目录

```text
AutoTrigger.exe
README.md
DEPLOY.md
config.example.json
result.schema.json
manifest.json
docs/
scripts/
resources/
third_party/7zip/7z2603-x64.exe
third_party/ocr/PaddleOCR-json_v1.4.1_windows_x64.7z
third_party/ocr/RapidOCR-json_v0.2.0.7z
```

`manifest.json` 记录文件 SHA-256、长度、版本、物料是否齐备和模板未验收状态。分发前用 Get-FileHash 核对。离线物料不得只提供下载网址；打包器的正式模式会拒绝缺少第三方离线包或真实模板的交付。

## 镜像准备

目标 Windows 7 SP1 x64 或更新系统。准备可交互桌面、管理员运行权限、可写 TEMP 和样本/结果目录。镜像管理员按测试口径配置 UAC 与 AV/EDR 排除目录；工具不修改这些配置，也不替用户关闭系统保护。不能从无交互桌面的服务会话保证 GUI 安装。

1. 核对交付包 manifest 的 SHA-256。
2. 以管理员身份安装离线 7-Zip，示例：`third_party\7zip\7z2603-x64.exe /S`。程序仅提示此操作，不自动执行。最低版本默认 24.09；低版本告警但继续。
3. 使用 7-Zip 将 PaddleOCR-json 包完整解压到 `C:\tools\PaddleOCR-json`，保留 exe 同目录的 DLL、models 和配置。不要只复制 OCR exe。
4. CPU 没有启用 AVX 时改用 RapidOCR-json，将其完整解压到 `C:\tools\RapidOCR-json`。发行包有自带模型配置时，应按发行包启动说明确认；运行 `scripts\verify-ocr.ps1` 做端到端验证。
5. 仅当 OCR 实际启动提示缺少 VC++ 时，由镜像管理员安装适用于该 Win7 镜像的离线运行库。当前交付不默认包含 VC++，工具不会下载。
6. 拷贝 `AutoTrigger.exe`，执行 `--check-prereq --json`。7-Zip 缺失返回 4，必需 OCR 缺失返回 5。默认可在 OCR 缺失时降级。

## 最小增量配置

将 `config.json` 放在 exe 同目录，或用 `--config` 指定。显式配置取代同目录配置，不叠加；字段递归合并。

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

运行 `AutoTrigger.exe --dump-config` 可导出所有实际默认值；`config.example.json` 为该命令生成的完整模板。关键范围如下：

| 字段 | 默认和范围 |
|---|---|
| `timeout_sec` / `task_timeout_sec` | 600 / 0，0 表示不限 |
| `startup_timeout_sec` / `idle_timeout_sec` | 30 / 60，正整数秒 |
| `concurrency` | 本期仅 1 |
| `max_attempts_per_file` / `attempt_interval_sec` | 8 / 2，正整数 / 非负秒 |
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
