//go:build windows

package ui

import (
	"autotrigger/internal/datadir"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (f *form) dataLayout() datadir.Layout {
	root := f.dataDir
	if root == "" {
		root = f.exeDir
	}
	return datadir.Layout{Root: root}
}
func (f *form) templateRoot() string {
	// Test fixtures created before the data layout retain their isolated roots.
	if f.dataDir == "" {
		return f.exeDir
	}
	return f.dataLayout().Templates()
}
func (f *form) taskLogPath(result string) string {
	return filepath.Join(f.dataLayout().Logs(), filepath.Base(result)+".log")
}

func (f *form) offerLegacyMigration() {
	if f.legacyPrompted || f.dataDir == "" {
		return
	}
	f.legacyPrompted = true
	if !strings.EqualFold(f.dataDir, f.exeDir) || fileExists(filepath.Join(f.dataDir, "config", "legacy-layout-imported.json")) {
		return
	}
	oldHistory := ""
	if dir, e := os.UserConfigDir(); e == nil {
		oldHistory = filepath.Join(dir, "AutoTrigger", "history")
	}
	found := false
	for _, move := range legacyDataMoves(f.exeDir, oldHistory) {
		if _, e := os.Stat(move[0]); e == nil {
			found = true
			break
		}
	}
	if !found {
		return
	}
	if messageBox(f.hwnd, "发现旧版工具数据", "新版统一使用 logs、results、templates、third_parties、config。\n是否复制旧版任务历史、结果、日志、模板项目及 third_party 到对应目录？\n原文件保留；已有新版数据不覆盖。\n请先关闭其他正在执行任务的工具。", 4|mbIconInfo) != 6 {
		return
	}
	if err := migrateLegacyData(f.exeDir, oldHistory); err != nil {
		messageBox(f.hwnd, "旧数据迁移失败", err.Error(), mbOK|mbIconError)
		return
	}
	f.history, _ = loadHistory(f.historyDir)
	if fileExists(f.dataLayout().Config()) {
		setWindowTextRaw(f.ctrls[idConfig], f.dataLayout().Config())
	}
	f.loadSettingsFields()
	f.refreshTaskList()
	if f.ctrls[hubName] != 0 {
		f.refreshTemplateHub()
		f.layout()
	}
	f.setStatusKind("旧数据已复制迁移，原目录保留。", statusKindOK)
}

func (f *form) applyDataDirectory() error {
	requested := strings.TrimSpace(windowText(f.ctrls[idOutputDir]))
	if requested == "" {
		return fmt.Errorf("请选择数据目录")
	}
	root, err := filepath.Abs(requested)
	if err != nil {
		return err
	}
	old := f.dataLayout().Root
	if strings.EqualFold(filepath.Clean(root), filepath.Clean(old)) {
		return f.dataLayout().Ensure()
	}
	if f.isRunning() {
		return fmt.Errorf("任务运行中不能修改数据目录")
	}
	if f.templateMaker != 0 {
		if p := makerPanes[f.templateMaker]; p != nil && p.capturing {
			return fmt.Errorf("请先结束模板捕获")
		}
	}
	choice := messageBox(f.hwnd, "更改数据目录", fmt.Sprintf("原目录：%s\n新目录：%s\n\n是否复制迁移原目录中的 logs、results、templates、third_parties、config？\n请先关闭其他执行实例。\n\n是：复制迁移，保留原数据，目标已有数据时不覆盖。\n否：使用新目录，不迁移；原任务和模板仍留在原目录。\n取消：不更改。", old, root), 3|mbIconInfo)
	if choice == 2 {
		return fmt.Errorf("已取消修改数据目录")
	}
	if choice == 6 {
		if err = migrateData(old, root); err != nil {
			return err
		}
	}
	layout := datadir.Layout{Root: root}
	if err = layout.Ensure(); err != nil {
		return err
	}
	if err = datadir.Save(f.exeDir, root); err != nil {
		return err
	}
	if f.templateMaker != 0 {
		procDestroyWindow.Call(f.templateMaker)
		f.templateMaker = 0
	}
	f.dataDir = root
	f.historyDir = layout.History()
	f.history, err = loadHistory(f.historyDir)
	f.hubSelected = ""
	f.hubEditing = false
	f.hubPage = 0
	f.selectedResult = ""
	f.resultPath = ""
	f.prereqReport = nil
	f.prereqChecked = false
	configPath := ""
	if fileExists(layout.Config()) {
		configPath = layout.Config()
	}
	setWindowTextRaw(f.ctrls[idConfig], configPath)
	setWindowTextRaw(f.ctrls[idOutput], filepath.Join(layout.Results(), DefaultResultName))
	f.configOpen = false
	f.loadSettingsFields()
	f.refreshTaskList()
	if f.ctrls[hubName] != 0 {
		if e := initializeTemplateSelection(f.templateRoot()); e != nil {
			return e
		}
		f.refreshTemplateHub()
	}
	if err != nil {
		return fmt.Errorf("目录已修改，但历史读取失败：%w", err)
	}
	return nil
}

func (f *form) templateTransfer(importing bool) {
	if f.isRunning() {
		messageBox(f.hwnd, "暂时无法迁移", "请结束任务后导入或导出模板。", mbOK|mbIconInfo)
		return
	}
	if f.templateMaker != 0 {
		if p := makerPanes[f.templateMaker]; p != nil && p.capturing {
			messageBox(f.hwnd, "请先结束捕获", "捕获期间不能迁移模板。", mbOK|mbIconInfo)
			return
		}
	}
	var path string
	var ok bool
	if importing {
		path, ok = openFileDialogWith(f.hwnd, "批量导入模板 ZIP", f.dataLayout().Root, []string{"模板压缩包 (*.zip)", "*.zip"})
	} else {
		path, ok = saveFileDialog(f.hwnd, "批量导出全部模板", filepath.Join(f.dataLayout().Root, "AutoTrigger-templates.zip"), "zip")
	}
	if !ok {
		return
	}
	var err error
	if importing {
		err = importTemplateArchive(f.templateRoot(), path)
	} else {
		err = exportTemplateArchive(f.templateRoot(), path)
	}
	if err != nil {
		messageBox(f.hwnd, "模板迁移失败", err.Error(), mbOK|mbIconError)
		f.setStatusKind(err.Error(), statusKindFail)
		return
	}
	if importing {
		f.refreshTemplateHub()
		f.layout()
		f.setStatusKind("模板已导入，请检查并勾选需要启用的模板。", statusKindOK)
	} else {
		f.setStatusKind("模板已导出："+path, statusKindOK)
	}
}
