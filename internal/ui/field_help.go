package ui

import (
	"autotrigger/internal/config"
	"autotrigger/pkg/schema"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

var fieldMeanings = map[string]string{
	"timeout_sec": "单文件执行上限（秒），最大 600。", "startup_timeout_sec": "启动阶段超时（秒）。", "idle_timeout_sec": "无活动超时（秒）。", "task_timeout_sec": "整个任务的超时（秒），0 不另设上限。", "progress_interval_sec": "进度日志间隔（秒）。", "activity_cpu_epsilon_ms": "判断 CPU 活动的最小增量（毫秒）。", "activity_io_epsilon_bytes": "判断 I/O 活动的最小增量（字节）。", "concurrency": "执行并发数；当前为串行，保持 1。", "attempt_interval_sec": "两次触发尝试的间隔（秒）。", "max_attempts_per_file": "每个文件最多尝试次数。",
	"archive": "压缩包处理配置。", "max_depth": "最大嵌套解压层数。", "max_extract_size_bytes": "允许解压的最大总字节数。", "sevenzip_path": "7-Zip 程序路径；auto 自动查找。", "require_sevenzip": "是否要求 7-Zip 能力可用。", "min_version": "最低 7-Zip 版本。", "temp_root": "临时工作目录；空值使用系统临时目录。",
	"iso": "光盘映像处理配置。", "strategy": "extract_first 优先解压 / mount_first 优先挂载 / extract_only 仅解压 / mount_only 仅挂载。", "allow_os_media": "是否允许处理操作系统介质。", "allow_media_disc": "是否允许处理影音光盘。", "autorun_as_p0": "将 autorun 指定入口列为高优先级候选。", "media_kinds_file": "介质识别规则文件。", "mount_tool_path": "挂载工具路径；auto 自动查找。", "mount_timeout_ms": "挂载超时（毫秒）。", "dismount_retry": "卸载挂载介质的重试次数。", "keep_mounted_until_verified": "保留挂载直到结果验证结束。", "keep_extracted_on_success": "成功后是否保留解压目录。", "motw_policy": "下载来源标记处理：native 系统默认 / preserve 保留 / clear 清除。",
	"package_layout": "安装包入口识别配置。", "enabled": "是否启用当前功能。", "mode": "入口识别模式：smart 智能优先 / brute 遍历。", "entry_patterns_file": "安装入口匹配规则。", "exclude_patterns_file": "卸载、修复等入口排除规则。", "prefer_shallowest": "优先选择浅层目录中的入口。", "prefer_arch_match": "优先选择与系统架构匹配的入口。", "readme_hints": "参考说明文件中的安装提示。", "max_entries_phase1": "第一阶段最多尝试入口数量。", "fallback_to_brute": "智能入口未成功时是否回退遍历。",
	"gui_automation": "窗口自动化与模板配置。", "poll_interval_ms": "扫描安装窗口的间隔（毫秒）。", "click_jitter_px": "点击坐标偏移范围（像素）。", "max_actions_per_window": "每个窗口的最多自动操作次数。", "keyword_file": "按钮识别关键词文件。", "template_pack": "模板库：merged 内置为底、外部按 ID 覆盖 / builtin 仅内置 / external 仅外部。", "template_dir": "自定义模板目录，需包含 templates.index.json 和图片。", "template_dpi_mode": "模板 DPI：auto / 100 / 125 / 150 / 200。", "template_theme": "模板主题：auto / light / dark / win7classic。", "template_min_similarity": "模板匹配最低相似度，0~1。", "builtin_templates": "是否启用内置图像模板。", "dump_failed_shots": "识别失败时是否保留截图。",
	"ocr": "文字识别配置。", "engine": "OCR 引擎：paddleocr_json / rapidocr_json / umi_ocr / tesseract / windows / none。", "engine_path": "OCR 程序路径；auto 自动查找。", "transport": "引擎通信方式：stdio / tcp / http。", "tcp_port": "TCP 端口；0 自动选择。", "startup_timeout_ms": "OCR 启动超时（毫秒）。", "recognize_timeout_ms": "一次文字识别超时（毫秒）。", "keep_alive": "是否复用 OCR 进程。", "restart_on_failure": "OCR 异常后是否重启。", "max_restarts": "最多重启次数。", "require_ocr": "是否强制要求 OCR 可用；否则缺失时降级。", "min_score": "文字识别最低置信度，0~1。", "roi_padding_px": "识别区域外围扩展像素。", "min_text_height_px": "接受的最小文字高度（像素）。", "max_text_height_px": "接受的最大文字高度（像素）。", "cache_by_screenshot_hash": "是否复用相同截图的识别结果。",
	"install_verify": "安装结果判定配置。", "judgment_profile": "判定策略：loose 宽松 / strict 严格。", "settle_sec": "安装后等待稳定的时间（秒）。", "quiet_polls": "连续无变化的检查次数。", "settle_max_sec": "最多等待稳定的时间（秒）。", "watch_dirs": "监控安装变化的目录列表。", "watch_exclude": "排除的临时、缓存目录列表。", "snapshot_depth": "文件快照最大目录深度。", "snapshot_max_entries": "快照最多记录条目数。", "write_baseline": "是否写出安装前基线。", "min_new_files": "认可文件变化所需的最少新增文件数。", "shrink_tolerance": "容许文件条目减少的数量。", "require_artifact": "是否要求安装产物证据。", "launch_check": "是否启动安装产物作验证，可能产生额外副作用。", "launch_check_sec": "产物启动验证超时（秒）。",
	"msi": "Windows Installer 详细日志配置。", "keep_verbose_log": "未确认安装成功时是否保留 MSI 详细日志。", "log_dir": "MSI 日志目录；空值使用结果文件旁的日志目录。", "lock": "单实例执行锁配置。", "wait_sec": "等待执行锁的时间（秒）。", "dll_export_probe_count": "DLL 入口探测的最多导出函数数量。", "output": "结果和日志输出配置。", "path": "文件或目录路径。", "pretty": "是否格式化结果 JSON。", "journal": "是否保存逐步执行日志。", "keep_journal_on_success": "成功后是否保留执行日志。", "log_file": "日志文件路径；空值使用结果文件旁的默认日志。", "log_level": "最低日志级别：debug / info / warn / error。", "max_file_size_mb": "结果文件大小限制（MB）。",
	"schema_version": "结果结构版本；集成时允许新增未知字段，不应强依赖固定字段数量。", "task_id": "任务唯一标识，用于关联结果与日志。", "config_digest": "本次生效配置的摘要。", "error": "顶层或单次尝试的错误；null 表示此处没有错误。", "code": "错误码；顶层错误必须独立处理。", "message": "错误原因文本。", "started_at": "任务开始时间，RFC3339，包含时区。", "finished_at": "任务结束时间；尚未结束时可能为 null。", "platform": "执行平台。", "platform_meta": "平台相关补充信息；键可扩展。", "prereq": "第三方能力的实际检测结果。", "archive_backend": "实际解压后端。", "sevenzip_version": "实际 7-Zip 版本。", "archive_backend_status": "解压能力状态。", "mount_backend": "实际挂载后端。", "mount_backend_status": "挂载能力状态。", "ocr_engine": "实际 OCR 引擎。", "ocr_engine_path": "实际 OCR 程序路径。", "ocr_engine_status": "OCR 能力状态。", "ocr_engine_version": "OCR 引擎版本。", "ocr_engine_languages": "支持的识别语言列表。", "ocr_startup_ms": "OCR 启动耗时（毫秒）。", "ok": "前置能力检查是否满足要求。", "issues": "检测到的问题列表。",
	"summary": "逐文件结果的汇总，不能代替顶层错误处理。", "total": "逐文件结果总数，不等于输入目录的全部文件数。", "by_status": "状态桶计数：installed / unverified / partial / already_installed / triggered / timeout / failed / skipped。", "flags": "附加标志计数，例如 requires_reboot、evidence_conflict。", "layout_analysis": "介质与安装入口选择过程。", "input_kind": "输入形态，例如 file、directory。", "media_kind": "介质分类。", "iso_strategy_used": "实际使用的映像处理策略。", "mount_drive": "挂载盘符。", "dismount_status": "卸载挂载介质的结果。", "entries": "选中的入口列表。", "excluded": "排除的入口列表。", "fallback_used": "是否启用了遍历回退。", "converged_at": "停止继续尝试的成功入口。", "skipped_after_success": "成功后跳过的其他入口数量。", "priority": "入口优先级。", "reason": "选择或排除原因。", "matched": "命中的规则。",
	"results": "逐文件结果数组；可能为空，务必同时读取 error。", "file_path": "处理文件的实际路径。", "file_size_bytes": "文件大小，字节。", "file_hash_sha256": "文件 SHA-256。", "detected_type": "业务识别类型，例如 pe_executable、msi、archive；不是仅按扩展名猜测。", "selection_reason": "选择该文件的原因。", "extension_disguised": "扩展名与识别类型是否不一致。", "pre_installed": "执行前是否已检测到安装。", "requires_reboot": "是否提示需要重启。", "attempts": "按顺序记录的实际触发尝试。", "evidence_conflicts": "与安装结论冲突的证据列表。", "install_confidence": "安装结论置信度 high / medium / low；null 为未提供。", "final_status": "最终状态，不可仅凭进程退出码推断安装成功。", "notes": "补充说明。", "baseline_digest": "安装前基线摘要；可能为 null。", "method": "实际触发方式：silent_install 静默 / no_param_run 直接运行 / msi / script / dll_probe / lnk_target / document_host。", "command": "实际执行命令。", "result": "单次尝试结果，不等于安装最终结论。", "exit_code": "单次触发进程退出码；null 为未提供，不是任务整体退出码。", "duration_sec": "本次尝试耗时（秒）。", "termination_mode": "进程自然退出、超时或其他终止方式。", "idle_resets": "活动检测重置空闲计时的次数。", "install_evidence": "支持安装判定的实际证据。", "artifact_check": "安装产物检查状态。", "artifact_runnable": "安装产物是否可运行；null 为未测试。", "gui_actions": "实际窗口自动化操作记录。", "stdout_summary": "标准输出摘要，可能截断。", "stderr_summary": "标准错误摘要，可能截断。", "msi_log": "保留的 MSI 详细日志文件路径。", "id": "证据唯一标识。", "level": "证据等级 S / A / B。", "type": "证据类别。", "source": "证据或识别来源。", "detail": "证据内容。", "at": "发生时间，RFC3339。", "window": "目标窗口。", "control_text": "目标控件文本。", "action": "实际操作。", "ocr_score": "OCR 识别置信度；null 为未使用 OCR。", "recognize_ms": "识别耗时（毫秒）。", "truncated": "结果是否因大小限制被截断；true 时不能假定数组完整。", "meta": "可扩展的运行元数据，包括资源版本等。",
}

func fieldHelp(configHelp bool) string {
	value := reflect.ValueOf(schema.Result{})
	intro := "结果 JSON 字段说明（schema " + schema.Version + "）\r\n先处理顶层 error，再读取 results[].final_status、install_confidence 和证据。未知字段应忽略，null 不等于 0 或 false；任务退出码来自 CLI，不在顶层结果中。\r\n"
	if configHelp {
		value = reflect.ValueOf(config.Defaults())
		intro = "高级配置字段说明\r\nJSON 不支持注释；缺省字段继承内置默认值。下面显示的是内置默认值，不是当前草稿。修改后先校验再保存；路径中的反斜杠需要转义。\r\n"
	}
	var blocks []string
	var walk func(reflect.Type, reflect.Value, string)
	walk = func(t reflect.Type, v reflect.Value, path string) {
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
			v = reflect.Zero(t)
		}
		if t == reflect.TypeOf(time.Time{}) || t.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			key := name
			if path != "" {
				key = path + "." + name
			}
			ft := field.Type
			typeName := jsonFieldType(ft)
			desc := fieldMeanings[name]
			if configHelp && key == "output.path" {
				desc = "结果文件路径；GUI 新建任务会使用独立结果文件。"
			}
			defaultText := ""
			fv := v.Field(i)
			if configHelp && ft.Kind() != reflect.Struct {
				b, _ := json.Marshal(fv.Interface())
				defaultText = "\r\n内置默认值：" + string(b)
			}
			blocks = append(blocks, fmt.Sprintf("%s [%s]\r\n%s%s", key, typeName, desc, defaultText))
			if ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.Struct {
				walk(ft.Elem(), reflect.Zero(ft.Elem()), key+"[]")
			} else {
				walk(ft, fv, key)
			}
		}
	}
	walk(value.Type(), value, "")
	if !configHelp {
		for _, status := range schema.Statuses {
			blocks = append(blocks, "final_status = "+status+"\r\n"+schema.StatusText(status))
		}
	}
	return intro + "\r\n" + strings.Join(blocks, "\r\n\r\n")
}

func jsonFieldType(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		return "可空 " + jsonFieldType(t.Elem())
	}
	if t == reflect.TypeOf(time.Time{}) {
		return "时间字符串"
	}
	switch t.Kind() {
	case reflect.Struct, reflect.Map:
		return "对象"
	case reflect.Slice, reflect.Array:
		return "数组"
	case reflect.String:
		return "字符串"
	case reflect.Bool:
		return "布尔值"
	case reflect.Interface:
		return "可扩展值"
	default:
		return "数值"
	}
}

func filterFieldHelp(body, query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return body
	}
	var out []string
	for _, block := range strings.Split(body, "\r\n\r\n") {
		if strings.Contains(strings.ToLower(block), query) {
			out = append(out, block)
		}
	}
	if len(out) == 0 {
		return "没有匹配字段。"
	}
	return strings.Join(out, "\r\n\r\n")
}
