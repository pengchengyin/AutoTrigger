package layout

import (
	"autotrigger/internal/config"
	"autotrigger/internal/resources"
	"testing"
)

func TestBlacklistUsesPurposeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		blocked bool
	}{
		{"unins000.exe", true}, {"uninstall.exe", true},
		{"app-uninstall.exe", true}, {"crashpad_handler.exe", true},
		{"update.exe", true}, {"checkupdate.exe", true},
		{"repair.exe", true}, {"modify.exe", true}, {"卸载.exe", true},
		{"卸载程序.exe", true}, {"A29-03-标准安装-落盘+卸载项.exe", false},
		{"INNO-06-假成功-只解压无卸载项.exe", false},
		{"MSI-06-升级覆盖V1.msi", false}, {"A29-09-覆盖升级-字段变化.exe", false},
		{"updated-editor-setup.exe", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matched, err := ExecutionExclusion(tc.name, resources.Pack{Mode: "builtin"}, config.Defaults())
			if err != nil || (matched != "") != tc.blocked {
				t.Fatalf("blocked=%t matched=%q err=%v", tc.blocked, matched, err)
			}
		})
	}
}
