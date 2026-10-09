# 项目长期记忆（自动安装工具）

## 项目性质
- **需求已收口（PRD v2.14），已进入开发**：Go 实现位于 `cmd/`、`internal/`、`pkg/`、`tools/`，
  产物 `dist/AutoTrigger.exe`（v0.1.0，Schema 2.8，资源包 0.1.0）。
- 主题：离线 Windows 沙箱中"自动触发/安装任意文件样本"供 EDR 采集行为。Go 1.20.x 单文件、
  Win7 SP1+、外部 7-Zip 与 OCR 前置、安装成功优先。
- 权威文档：`自动安装工具prd-v2.15.md`（唯一 PRD）；另 `docs/图像模板制作说明.md`(D17)、
  `docs/架构说明.md`、`docs/验收状态.md`、`README.md`、`DEPLOY.md`。

## 用户约定（务必遵守）
1. **每轮改动后自动提交 git**，commit message 用中文（`git add -A && git commit -m "..."`）。
2. **文档版本号与文件名必须同步递增**（C37）：改 PRD 就要 ① 版本行 +1；② 新增 `| C.. |`
   变更记录；③ `git mv` 重命名为 `自动安装工具prd-vN+1.md`。
3. 写作风格：表格化、每条决策写"为什么"、变更记录注明落点（如 `F14.7、A32`）。

## 长 Markdown 的安全编辑流程（踩坑后固化）
- ⚠️ **不要用"行首前缀"做 `old_string`**（如 `| C39 | **...`）：会吞掉表格行标签，导致文本悬挂
  （已犯两次）。要用**行中段唯一片段**或将整行完整纳入。
- ⚠️ **同一批不要并行发多个 Edit**：多次出现"返回成功但内容回退"。必须**逐个 Edit**，
  最后 `for k in 锚点...; do grep -c "$k" 文件; done` 逐条复核。
- 调整表格行顺序：PowerShell 读行 → 交换数组下标 → `WriteAllText`（保持 UTF-8 无 BOM + LF）。

## 开发环境事实（编译前必看）
- **Go 位置**：`D:\Go\bin\go.exe`（go1.20.1 windows/amd64，**不在 PATH**）。GOPROXY 默认
  `proxy.golang.org` **不可达**（实测 HTTP 000），需 `goproxy.cn`；CGO_ENABLED 默认 0。
- ⚠️ **`D:\Go\src` 下有 3358 个 `*.old.go`**（覆盖 536 目录，另有 `*.old.md`/`LICENSE.old`），
  疑来自 `D:\AntiySafeDocBackup`（安天安全软件，2024-05-13）。Go 会当成包的一部分，
  `go build` 报 `found packages X and X_test` 或 `import cycle not allowed`，**完全无法编译**。
  绕过（不动原 GOROOT）：另建干净 GOROOT —— `junction` 指向原 `bin`/`pkg`，
  src 用 `find . -type f ! -name "*.old*" -print0 | tar --null -cf - -T -` **只复制不删除**，
  再用 `GOROOT=<新目录>` 编译。
- ⚠️ **执行环境有"批量删除守卫"**（`SAFE_DELETE_BULK_GUARD`）：批量 `Remove-Item`/`rmdir`
  会被**静默拦截且不报错**（只有单文件删除才暴露错误）。清理临时目录时会遇到。
- ⚠️ **`cmd //c "..."` 在本环境 Git Bash 中不生效**（cmd 进入交互模式、命令静默丢失）；
  需要 Windows 命令时改用原生终端工具。
- PowerShell 工具**不回显 stdout**（结果写文件再 Read）；含中文的 `.ps1` 必须 **UTF-8 with BOM**，
  `.md` 保持**无 BOM + LF**。
- 环境：Win10 build 26200 / i7-13700H / PowerShell 5.1；未安装任何 OCR 引擎。

## 当前文档状态（v2.14）
- `schema_version: 2.8`；已定案：判定宽松（DC19）、单实例串行 + 单文件 600s（DC20）、
  watch_dirs 五处（C35）、内置资源包（C40）、跨平台扩展（4.8）、CLI 帮助面（C42）、
  **图形操作界面（DC21 / C43 / F20：双击即用、input 可选文件/文件夹、配置生成与编辑、命令预览）**。
- 退出码：0 完成 / 1 异常 / 2 参数路径非法 / 3 配置错误 / 4 缺 7-Zip / 5 缺 OCR /
  6 已运行 / 130 中止。交付物 D1~D17；验收 **A1~A33**。
- **GUI 关键实现事实**：`internal/ui`（`_windows` 实现 + `_other` stub）；双击判据 =
  `GetConsoleMode` 成功 **且** `GetConsoleProcessList` 进程数为 1；`--gui` 必须单独使用，
  否则退出码 2；界面不复制业务逻辑，只把参数拼成 args 交给 `cli.Run`。
