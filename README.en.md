# AutoTrigger

[![Latest release](https://img.shields.io/github/v/release/pengchengyin/AutoTrigger?display_name=tag)](https://github.com/pengchengyin/AutoTrigger/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows-blue)](https://github.com/pengchengyin/AutoTrigger)
[![Language](https://img.shields.io/badge/language-Go-00ADD8)](https://github.com/pengchengyin/AutoTrigger/tree/main/src)

**English** · [简体中文](README.md)

**AutoTrigger is an offline Windows software installation automation tool.** It ships with both a native GUI and a CLI. Hand it an installer file or a folder of them, and it will detect the installer type (Inno Setup, NSIS, MSI, WiX Burn and more), drive the setup wizard automatically, and record per-task logs and structured JSON results. Everything runs locally — no network access required, and the delivered binary needs no Go installation.

Topics: Windows installer automation · silent install · unattended setup · GUI automation · OCR · image template matching · software deployment · Go

## Why AutoTrigger

- **One tool, many installers** — built-in adapters for Inno Setup, NSIS, MSI and WiX Burn; image template matching covers non-standard setup dialogs.
- **GUI + CLI** — point-and-click for humans, stable parameters and output contract for scripts and CI.
- **Offline & self-contained** — single-file binary; 7-Zip and OCR dependencies deploy from bundled offline materials.
- **Evidence-based results** — before/after snapshot comparison, evidence grading and counter-evidence arbitration produce high/medium/low confidence, clearly distinguishing *installed / likely installed / partially completed / already present / failed*.
- **Smart timeout** — activity-aware timeouts (progress resets the clock), graceful shutdown first, process-tree cleanup second, evidence still collected.
- **Full results on every path** — timeout, cancellation and crashes still emit parseable result JSON that third-party scripts can consume directly.
- **Unattended operation** — on a dedicated Windows VM with auto-login and admin auto-start, tasks can be submitted remotely over SSH (same Windows account) and logs/results queried remotely.
- **DIY image templates** — capture screenshots of unusual installers, mark click regions and actions, and let matching handle the rest.

## Tested results (historical build)

Admin session, Windows 10.0.26300 x64, tool 0.1.0 / Schema 2.8. Out of 22 real installers across Inno Setup / NSIS / MSI / WiX Burn / other vendors, **18 (81.8%)** produced an "installed" verdict; the remaining 4 were "partially completed". Details and boundaries: [历史测试效果摘要 (Chinese)](docs/历史测试效果摘要.md).

> These numbers come from historical builds and do not represent the overall success rate of the latest version. Task completion or exit code 0 does not mean installation succeeded — always check the status, evidence and logs.

## Quick start

1. **Prepare** — extract the delivery ZIP to a writable directory, keeping the bundled dependency materials.
2. **Launch** — double-click `AutoTrigger.exe`. For system-level installs, run as administrator.
3. **Check environment** — click "检查环境" (Check Environment) on the home page. Missing 7-Zip/OCR engines are deployed from the bundled offline packages.
4. **Run a task** — click "新建任务" (New Task), pick a file or folder, and run.
5. **Inspect results** — open "任务详情" (Task Details) from the task list: status, live log and result JSON.

CLI:

```powershell
.\AutoTrigger.exe --check-prereq --json
.\AutoTrigger.exe --input "D:\installers\setup.exe" --output "D:\results\result.json"
.\AutoTrigger.exe --help
```

Build from source (Go 1.20.x, Windows):

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\src\scripts\build.ps1
```

## Notes

- **Security software**: if Defender or another product flags the tool, verify the source and SHA-256 first; do not blindly whitelist. The tool cannot guarantee zero alerts from every security product.
- **UAC / desktop state**: locked screens and the UAC secure desktop cannot be automated; keep an interactive desktop during GUI automation. The tool never bypasses system security prompts.
- **Compatibility**: targets Windows 7 SP1 x64 and later; Win7 real-hardware validation is not yet complete.

## Documentation (Chinese)

- [快速开始与使用说明（中文 README）](README.md)
- [SSH 远程任务说明](docs/SSH远程任务说明.md)
- [图像模板制作说明](docs/图像模板制作说明.md)
- [结果 JSON 字段说明](docs/结果JSON字段说明.md)
- [架构说明](docs/架构说明.md)

## License

[MIT](LICENSE)
