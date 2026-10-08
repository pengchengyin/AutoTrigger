package cli

import (
	"fmt"
	"io"
)

// helpText 是 --help 与「未提供任何参数」两种场景共用的用法说明。
//
// 通道与退出码约定（PRD F19.1 / 6.1）：
//   - 显式 --help / -h：输出到 stdout，退出码 0（用户主动请求，属成功路径）
//   - 未提供任何参数：输出到 stderr，退出码 2，且不产出结果 JSON 与日志文件
//     （避免在用户当前目录留下无意义的产物；与「参数非法仍产出结果 JSON」的
//     F10.2 R6 刻意区分：那是指已开始处理后的失败）
const helpText = `用法:
  AutoTrigger --input <路径> [选项]
  AutoTrigger --check-prereq [--json]
  AutoTrigger --dump-ui --input <样本> [--out <目录>]
  AutoTrigger --dump-resources <目录> | --self-test templates | --version | --capabilities

基本:
  --input <路径>            待触发/安装的样本文件或目录（除检查类命令外必填）
  --output <路径>           结果 JSON 路径；"-" 表示写 stdout（默认 .\autotrigger-result.json）
  --recursive               目录输入时递归子目录
  --config <路径>           指定配置文件（显式指定时取代同目录 config.json）

前置与能力:
  --check-prereq            检查离线前置（7-Zip / OCR）后退出
  --capabilities            输出机器可读能力清单
  --version                 输出版本与 schema 版本

行为控制:
  --timeout <秒>            单文件硬上限，默认 600；0 为不限
  --no-archive              跳过压缩包处理（不使用 7-Zip）
  --no-ocr                  禁用 OCR
  --require-ocr             OCR 不可用时失败（退出码 5）
  --lock / --no-lock        单实例锁，默认开启（第二个实例返回退出码 6）

调试与资源:
  --dump-config             输出生效配置
  --dump-ui                 抓取窗口截图与 UIA/OCR 数据，不执行点击
  --out <目录>              --dump-ui 输出目录（默认 .\dump\<task_id>）
  --dump-resources <目录>   导出内置资源包（模板与规则）作为修改起点
  --no-builtin-templates    禁用内置图像模板
  --self-test templates     校验每张模板能否在其源截图上匹配

输出与日志:
  --json                    检查类命令以 JSON 输出
  --pretty                  缩进 JSON
  --log-file <路径>         日志路径（默认 <结果路径>.log）
  --log-level <级别>        debug | info | warn | error

帮助:
  -h, --help                显示本帮助并退出

退出码:
  0 正常完成         1 工具运行期异常     2 参数或路径非法
  3 配置解析失败     4 未检测到 7-Zip     5 OCR 缺失且被要求必需
  6 已有实例运行中   130 任务被中止
  退出码只表示工具自身状态；单文件安装结果以结果 JSON 的 final_status 为准。

示例:
  AutoTrigger --input sample.exe
  AutoTrigger --input .\samples --recursive --output result.json
  AutoTrigger --check-prereq --json
  AutoTrigger --input setup.exe --dump-ui --out dump\case1
  AutoTrigger --input setup.exe --no-archive --no-ocr --output -

文档: README.md · docs\图像模板制作说明.md · DEPLOY.md
`

// helpRequested 判断是否显式请求帮助。
// 独立扫描而非依赖 flag 包：flag 对内建 -h 的处理会走「参数错误」路径并返回
// ErrHelp，无法区分「用户想看帮助」与「参数写错了」。
func helpRequested(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-h", "-help", "--help":
			return true
		}
	}
	return false
}

// printHelp 输出用法说明，首行带工具版本。
func printHelp(w io.Writer) {
	fmt.Fprintf(w, "AutoTrigger — Windows 任意文件自动安装/触发工具  (v%s)\n\n%s", Version, helpText)
}
