package verify

import (
	"strings"
	"testing"

	"autotrigger/pkg/schema"
)

// 成功码只有 0 / 3010（需重启）/ 1641（安装后启动重启），别的都不能算成功。
func TestMSICodeIsSuccess(t *testing.T) {
	for _, code := range []int{0, 3010, 1641} {
		if !MSICodeIsSuccess(code) {
			t.Errorf("%d 应判为安装成功", code)
		}
	}
	for _, code := range []int{1603, 1618, 1619, 1625, 1633, 1638, 1, -1} {
		if MSICodeIsSuccess(code) {
			t.Errorf("%d 不得判为安装成功", code)
		}
	}
}

// 现场只看到裸数字无法判断"是权限问题还是包有问题"，必须有中文释义。
func TestMSICodeNote(t *testing.T) {
	if got := MSICodeNote(1603); !strings.Contains(got, "1603") || !strings.Contains(got, "致命错误") {
		t.Fatalf("1603 应带中文释义，实得 %q", got)
	}
	if got := MSICodeNote(3010); !strings.Contains(got, "重启") {
		t.Fatalf("3010 应说明需重启，实得 %q", got)
	}
	if got := MSICodeNote(99999); got != "99999" {
		t.Fatalf("未收录的码应原样显示，实得 %q", got)
	}
}

// 冲突码是排查入口：N1:1603 这种裸码对现场没有意义，要带上人类可读的解释。
func TestConflictText(t *testing.T) {
	n1 := schema.Evidence{ID: "N1", Detail: "1603"}
	got := ConflictText(n1)
	if !strings.Contains(got, "1603") || !strings.Contains(got, "致命错误") {
		t.Fatalf("N1 应翻译成带释义的失败说明，实得 %q", got)
	}
	for id, want := range map[string]string{
		"N2": "失败或回滚提示",
		"N3": "又被卸载",
		"N4": "注册表项被移除",
		"N5": "实体数量减少",
	} {
		got := ConflictText(schema.Evidence{ID: id, Detail: "d"})
		if !strings.Contains(got, want) {
			t.Errorf("%s 的说明 = %q，应包含 %q", id, got, want)
		}
	}
	// 未知码原样返回，便于排障时看出新增了证据类型。
	if got := ConflictText(schema.Evidence{ID: "N9", Detail: "raw"}); got != "raw" {
		t.Fatalf("未知冲突码应原样返回，实得 %q", got)
	}
	// 非数字的 N1 详情（理论上不会出现）也不能丢信息。
	if got := ConflictText(schema.Evidence{ID: "N1", Detail: "crashed"}); !strings.Contains(got, "crashed") {
		t.Fatalf("N1 非数字详情应保留原文，实得 %q", got)
	}
}
