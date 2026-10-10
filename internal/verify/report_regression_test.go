package verify

import "testing"

func TestD02MSILogStatusMustBeNumericSuccess(t *testing.T) {
	for _, text := range []string{"安装成功或错误状态: 1603。", "安装成功或错误状态: 1602。", "Installation success or error status: 1620.", "安装成功或错误状态: 未知"} {
		if got := installerLogSuccess(text); got != "" {
			t.Fatalf("false B1: %q", got)
		}
	}
	for _, text := range []string{"安装成功或错误状态: 0。", "Installation success or error status: 3010.", "Installation success or error status: 1641.", "Installation completed successfully"} {
		if installerLogSuccess(text) == "" {
			t.Fatalf("missing B1: %q", text)
		}
	}
}
