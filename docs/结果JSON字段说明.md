# AutoTrigger 结果 JSON 字段说明

适用结构版本：`schema_version = "2.8"`。本文说明安装/触发任务输出的 result JSON，不是配置文件或 `task submit`、`task status` 等远程接口的响应结构。

## 1. 如何判断结果

1. 先检查顶层 `error`，判断任务是否发生整体错误。
2. 检查 `truncated`，确认结果是否完整。
3. 遍历 `results`，读取每个样本的 `final_status`、`install_confidence`、`evidence_conflicts` 和各次尝试的 `install_evidence`。
4. 用 `summary` 展示统计，用执行日志排查具体过程。

**任务结束、CLI 退出码为 0、安装进程退出码为 0、自动点击成功，都不等于安装成功。** 安装结论以逐文件状态及实际证据为准；安装成功也不表示已激活或业务功能正常。

类型约定：`integer` 为整数，`number` 为数值，`boolean` 为布尔值，`object` 为对象，`array` 为数组。`string / null` 等表示可空。时间为 RFC3339 字符串，包含时区，可带小数秒。`null` 不等于 `0`、`false` 或空字符串。

路径指执行主机上的路径，不是客户端本机路径。远程任务实际执行冻结的安装包快照，因此 JSON 的 `file_path` 可能是快照位置；GUI 文件信息显示原始提交位置。

## 2. 顶层结构

| 字段 | 类型 | 含义 |
|---|---|---|
| `schema_version` | string | 结果结构版本，当前为 `2.8`。 |
| `task_id` | string | 本次执行任务标识，用于关联结果及日志；不能假定等于远程队列任务 ID。 |
| `config_digest` | string | 本次生效配置摘要，用于核对执行条件。 |
| `error` | object / null | 任务整体错误；为 null 也不代表所有样本成功。 |
| `started_at` | string | 任务开始时间。 |
| `finished_at` | string / null | 任务结束时间；null 表示未提供结束时间。 |
| `platform` | string | 执行平台，例如 `windows`。 |
| `platform_meta` | object | 平台补充信息，键可扩展，例如系统版本、架构、管理员权限及 DPI 感知模式。 |
| `prereq` | object | 解压、挂载、OCR 等前置能力的实际检测结果。 |
| `summary` | object | 逐文件结果统计，不代替错误处理或逐项判定。 |
| `layout_analysis` | object | 输入介质和安装入口筛选过程。 |
| `results` | array | 逐文件结果；可能为空，必须同时检查 error。 |
| `truncated` | boolean | 是否因输出大小限制截断；true 时不可假定数组及证据完整。 |
| `meta` | object | 可扩展运行元数据，例如工具版本、资源版本、临时目录和所属进程。不是固定字段集合。 |

`error` 对象：

| 字段 | 类型 | 含义 |
|---|---|---|
| `code` | integer | 工具记录的任务错误码；不是样本进程退出码。 |
| `message` | string | 原始错误原因。主动取消可能包含 `context canceled`；GUI 会显示友好中文，但 JSON 保留原值。 |

结果顶层没有任务整体 `exit_code` 字段。通过 CLI 启动时，从调用进程取得工具退出码；通过远程任务接口启动时，从对应队列任务记录读取，不要使用 `attempts[].exit_code` 替代。

## 3. 前置能力 prereq

| 字段 | 类型 | 含义 |
|---|---|---|
| `archive_backend` | string / null | 实际解压后端路径或标识。 |
| `sevenzip_version` | string / null | 检测到的 7-Zip 版本。 |
| `archive_backend_status` | string | 解压能力状态。 |
| `mount_backend` | string / null | 实际挂载后端。 |
| `mount_backend_status` | string | 挂载能力状态。 |
| `ocr_engine` | string | 实际选定的 OCR 引擎，例如 paddleocr_json、rapidocr_json、umi_ocr、tesseract、none。引擎标识不保证能力已实现或已通过检查。 |
| `ocr_engine_path` | string / null | 实际 OCR 程序路径。 |
| `ocr_engine_status` | string | 选定 OCR 引擎的能力状态。 |
| `ocr_engine_version` | string / null | 取得的版本；unknown 表示未提供版本信息。 |
| `ocr_engine_languages` | array of string | 检测得到的识别语言列表。 |
| `ocr_startup_ms` | integer | OCR 启动耗时，毫秒。 |
| `ocr_engines` | array，可省略 | 多引擎检查清单；旧结果或非清单检查可能没有此字段。 |
| `ok` | boolean | 前置能力是否满足此次检查要求，不表示安装成功。 |
| `issues` | array of string | 能力缺失、权限建议等问题或警告；非空不一定是致命错误。 |

`ocr_engines[]` 每项：

| 字段 | 类型 | 含义 |
|---|---|---|
| `engine` | string | 引擎类别。 |
| `path` | string / null | 引擎程序路径。 |
| `package_path` | string / null | 对应的离线包路径；null 表示没有关联包。 |
| `status` | string | 此项部署和自检状态。 |
| `version` | string / null | 实际取得的版本。 |
| `languages` | array of string | 实际取得的识别语言列表。 |
| `startup_ms` | integer | 本次启动耗时，毫秒；未启动时为 0。 |
| `detail` | string | 不可用原因或检查补充说明。 |
| `selected` | boolean | 是否对应本次实际选定的引擎，不表示此项自检通过。 |

能力状态常见值：`ok` 可用、`missing` 未发现、`not_deployed` 未部署、`deploy_failed` 部署失败、`startup_failed` 自检失败、`service_required` 需要启动本地服务、`unsupported` 不兼容或未实现、`disabled` 已关闭。不同能力不一定使用所有取值，未知状态应保留展示，不能默认为可用。

## 4. 汇总 summary

| 字段 | 类型 | 含义 |
|---|---|---|
| `total` | integer | 逐文件结果总数，不等于输入目录全部文件数。 |
| `by_status` | object | 各状态桶的样本数量，映射见下一节。 |
| `flags` | object | 附加标志计数，允许后续新增键。 |

`flags` 当前包含：

| 键 | 含义 |
|---|---|
| `requires_reboot` | 提示需要重启的样本数。 |
| `evidence_conflict` | 存在冲突证据的样本数。 |
| `pre_installed` | 执行前已检测到安装的样本数。 |
| `artifact_check_failed` | 至少一次尝试产物检查失败的样本数，不是失败尝试总次数。 |

这些标志可以重叠，不能相加作为样本总数。

## 5. 逐文件结果 results[]

| 字段 | 类型 | 含义 |
|---|---|---|
| `file_path` | string | 实际处理文件路径，可能位于解压目录或远程任务快照目录。 |
| `file_size_bytes` | integer | 文件大小，字节。 |
| `file_hash_sha256` | string | 文件 SHA-256；空字符串表示没有提供，不能当作有效校验值。 |
| `detected_type` | string | 内容识别类型，如 pe_executable、pe_dll、msi、script、archive、disk_image、shortcut、document、unknown。不只是扩展名。 |
| `selection_reason` | string | 选择该文件的原因，例如单文件输入或安装入口筛选。 |
| `extension_disguised` | boolean | 扩展名与实际识别类型是否不一致。 |
| `pre_installed` | boolean | 执行前是否已检测到安装。 |
| `requires_reboot` | boolean | 是否提示需要重启。 |
| `platform_meta` | object | 此文件的平台相关补充信息，键可扩展。 |
| `attempts` | array | 按顺序保存的实际执行尝试；空数组表示没有记录尝试。 |
| `evidence_conflicts` | array of Evidence | 与安装结论冲突的证据，结构见第 7 节。 |
| `install_confidence` | string / null | high 高、medium 中、low 低；null 表示未提供，不等于低。 |
| `final_status` | string | 该样本最终结论，枚举见下表。 |
| `notes` | string | 补充说明，包括跳过或失败原因等。 |
| `judgment_profile` | string | loose 宽松或 strict 严格判定策略。 |
| `baseline_digest` | string / null | 安装前基线摘要；null 表示未提供。 |

最终状态及汇总桶：

| final_status | summary.by_status 键 | 解释 |
|---|---|---|
| `install_completed` | `installed` | 安装判定成功；仍应结合证据、置信度和重启提示。 |
| `install_unverified` | `unverified` | 有安装动作或相关信号，但未充分验证成功，不能当作已确认安装。 |
| `install_partial` | `partial` | 部分完成，不能按完整成功处理。 |
| `already_installed` | `already_installed` | 执行前已检测到安装，并非本次新增安装成功。 |
| `triggered` | `triggered` | 已触发或启动，不表示安装成功。 |
| `killed_on_timeout` | `timeout` | 执行超时后终止，已有系统变更不保证回滚。 |
| `failed` | `failed` | 失败，应检查顶层错误、尝试错误、证据及日志。 |
| `skipped` | `skipped` | 已跳过，原因见 notes 和相关记录。 |

机器对接使用上述英文标识，不依赖 GUI 中文标签。任务主动取消没有独立的 final_status 枚举，需结合顶层错误和远程任务状态判断，不自行假定存在 `cancelled` 样本状态。

## 6. 单次尝试 results[].attempts[]

| 字段 | 类型 | 含义 |
|---|---|---|
| `method` | string | 实际执行方式，见下表。多条相同方式可能对应不同静默参数。 |
| `command` | string | 实际命令的记录文本；不要将此字符串直接作为重新执行指令。 |
| `result` | string | 本轮执行结果，例如 exited、failed、killed_on_timeout；不是最终安装结论。 |
| `exit_code` | integer / null | 本轮被执行进程退出码；null 表示没有取得，不等于 0。 |
| `duration_sec` | number | 本轮耗时，秒，可带小数。 |
| `termination_mode` | string | 进程终止方式，例如 natural 自然退出、graceful 温和终止；以实际值及日志为准。 |
| `idle_resets` | integer | 活动检测重置空闲计时的次数，不表示安装进度或成功次数。 |
| `install_evidence` | array of Evidence | 本轮安装判定证据。 |
| `artifact_check` | string | 产物检查状态，如 ok、failed、unknown。 |
| `artifact_runnable` | boolean / null | 产物运行检查结果；null 表示未提供或未检查，不能视为可运行。 |
| `gui_actions` | array | 本轮实际记录的自动化操作，见第 8 节。 |
| `stdout_summary` | string | 标准输出摘要，可能截断，不是完整日志。 |
| `stderr_summary` | string | 标准错误摘要，可能截断，不是完整日志。 |
| `msi_log` | string / null | 保留的 MSI 详细日志路径；null 表示未提供。 |
| `error` | string / null | 本轮原始错误文本，注意与顶层 error 对象不同。 |

执行方式：

| method | 中文含义 |
|---|---|
| `silent_install` | 使用静默参数执行安装包。 |
| `no_param_run` | 不带安装参数直接运行。 |
| `msi` | Windows Installer 安装包执行。 |
| `script` | 脚本执行。 |
| `dll_probe` | 动态库入口探测。 |
| `lnk_target` | 快捷方式目标执行。 |
| `document_host` | 文档关联程序打开。 |

## 7. 证据 Evidence

适用于 `install_evidence[]` 和 `evidence_conflicts[]`。

| 字段 | 类型 | 含义 |
|---|---|---|
| `id` | string | 证据规则标识，例如 S1、A1、N1；不是每次事件独一无二的 ID，同一规则可出现多次。 |
| `level` | string | 证据等级/类别，包含 S、A、B 正向证据及 N 负向证据。 |
| `type` | string | 证据具体类别。 |
| `source` | string | 证据来源，例如注册表、进程、日志或 GUI。 |
| `detail` | string | 实际采证内容和说明；应与 id、type 一起阅读。 |
| `at` | string | 证据采集时间。 |

不能简单按证据数量判成功，不能忽略冲突证据，也不能将后台活动、一次鼠标点击或安装前已存在的文件当作本次安装成功的充分证据。判定策略以配置和实际业务规则为准。

## 8. 自动化操作 gui_actions[]

| 字段 | 类型 | 含义 |
|---|---|---|
| `window` | string | 目标窗口标题，不是窗口句柄。 |
| `control_text` | string | 控件文本；图像模板目标可能没有文本。 |
| `source` | string | 识别来源，例如 uia、win32、template、ocr。 |
| `action` | string | 记录的实际操作，例如 click；不能假定这是 next、accept 等模板业务动作。 |
| `ocr_score` | number / null | OCR 置信度；未使用 OCR 时为 null，不代表模板匹配分数。 |
| `recognize_ms` | integer / null | 记录的识别耗时，毫秒；null 表示未提供。 |
| `at` | string | 操作时间。 |

操作记录只表示提交了操作，不保证界面已跳转，更不能代替安装完成证据。用户模板的规则 ID、分数等命中细节以实际调试日志为准，不假定本对象包含未定义字段。

## 9. 输入与入口分析 layout_analysis

| 字段 | 类型 | 含义 |
|---|---|---|
| `input_kind` | string | 输入形态，例如 file、directory。 |
| `media_kind` | string / null | 介质分类，未提供时为 null。 |
| `iso_strategy_used` | string / null | 实际使用的光盘映像处理策略。 |
| `mount_drive` | string / null | 实际挂载盘符。 |
| `dismount_status` | string | 挂载介质卸载结果；not_applicable 表示不适用。 |
| `motw_policy` | string | 下载来源标记策略，如 native、preserve、clear。 |
| `entries` | array | 选中的入口列表。 |
| `excluded` | array | 排除的入口列表。 |
| `fallback_used` | boolean | 是否使用遍历回退。 |
| `converged_at` | string / null | 使后续尝试收敛停止的入口。 |
| `skipped_after_success` | integer | 成功后跳过的其他入口数量。 |

`entries[]`、`excluded[]` 每项：

| 字段 | 类型 | 含义 |
|---|---|---|
| `path` | string | 候选入口路径。 |
| `priority` | string | 入口优先级标识。 |
| `reason` | string | 选择或排除原因。 |
| `matched` | string | 命中的筛选规则。 |

## 10. 第三方对接建议

- 解析结构化 JSON，不解析中文日志作为状态接口；未知字段应允许并忽略，未知状态应保留原值并提示需要适配。
- 区分字段不存在、null、空数组和实际的 0 / false；不要用默认值伪造未取得的信息。
- 结果截断、任务中断或顶层错误时，已有部分结果不应被当作完整任务结果。
- 工具可能保留 journal 恢复记录，但它不等同于最终 result JSON，不直接混合解析。
- 路径、命令、日志摘要和证据可能含主机或用户信息，对外传输前按需要脱敏。
- 多样本逐项处理，不只读取 results[0]；状态统计和附加标志采用各自口径。

字段契约来源：[结果 JSON Schema](../src/pkg/schema/result.schema.json)、[Go 结果结构](../src/pkg/schema/result.go)。功能及判定规则见 [PRD](自动安装工具prd-v2.17.md)，远程接口使用见 [SSH 远程任务说明](SSH远程任务说明.md)。本文更新时需与这些定义保持一致。
