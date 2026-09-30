# 项目长期记忆（自动安装工具 PRD）

## 项目性质
- 本项目当前**只有 PRD 文档 + 少量脚本**，尚无代码。主文档：`自动安装工具prd-v2.11.md`（`自动安装工具prd.md` 是最初稿，保留不动）。
- 主题：离线 Windows 沙箱里"自动触发/安装任意文件样本"的工具，供 EDR 采集行为。Go 单文件、Win7 SP1+、外部 7-Zip 与 OCR 前置。

## 用户约定（务必遵守）
1. **每轮改动后自动提交 git**，commit message 用中文。
   实测：`git-commit` skill 在本机不存在，直接用 `git add -A && git commit -m "..."`。
2. **云函数改动要上传并部署**（本项目暂不涉及）。
3. **文档版本号与文件名必须同步递增**（C37 定案）：任何一次改 PRD 都要
   ① 版本行 `vN` → `vN+1`；② 新增变更记录 `| C.. |`；③ `git mv` 重命名为 `自动安装工具prd-vN+1.md`。
4. 文档写作风格：表格化、每条决策写"为什么"、变更记录里注明落点（如 `F14.7、A32`）。

## 长 Markdown 的安全编辑流程（踩坑后固化）
- ⚠️ **不要用"标题/行首前缀"做 `old_string`**：`| C39 | **...**` 这种前缀被替换会**吞掉整行的行标签**，导致后续文本悬挂（已犯两次）。要用**行中段唯一片段**作锚点，或把整行完整纳入 `old_string`。
- ⚠️ **同一批不要并行发多个 Edit**：本会话多次出现"返回成功但内容回退"，必须**逐个 Edit**，最后用 `grep -c` 逐锚点复核。
- **调整表格行顺序**：先改内容，再用 PowerShell 读行 → 交换数组下标 → `WriteAllText`（保持 **UTF-8 无 BOM + LF**，否则整文件 diff 爆炸）。
- 编辑后统一校验：`for k in 锚点...; do grep -c "$k" 文件; done`。

## 本机/sandbox PowerShell 事实（少走弯路）
- PowerShell 工具**不回显 stdout**：结果要写文件再用 Read 工具看。
- sandbox 中 **`.ps1` 无法执行**（`& script.ps1` 失败且文件改动被回滚）；可用替代：`[System.Management.Automation.Language.Parser]::ParseFile($f,[ref]$toks,[ref]$errs)` 做**纯静态语法校验**。
- 含中文的 `.ps1` **必须 UTF-8 with BOM**（PS 5.1 否则按 ANSI 解码，报大量虚假语法错误）；`.md` 则保持 **无 BOM + LF**。
- `1>` / `2>` 只捕获 PowerShell 流，脚本内 `[Console]::Out/Error` 直写句柄不被捕获；`*>` 可捕获 `Write-Host`。
- 环境：Win10 build 26200 / i7-13700H / PowerShell 5.1；未安装任何 OCR 引擎（PaddleOCR-json 等）。

## 当前文档状态（v2.11）
- `schema_version: 2.8`；`resource_pack_version: 1.0`（新）。
- 已定案：判定口径宽松（DC19）、单实例串行 + 单文件 600s（DC20）、watch_dirs 五处（C35）、内置资源包（C40）。
- 待确认清单第 12 节：Q17~Q20 已关闭，其余 Q2~Q16 部分仍未答复。
