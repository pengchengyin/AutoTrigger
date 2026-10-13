package schema

import "testing"

// OverallOutcome 是界面横幅与状态栏的结论来源，档位错分会导致
// "结论行"与"统计行"口径打架（F20.6），因此逐档固定。
func TestOverallOutcomeLadder(t *testing.T) {
	cases := []struct {
		name   string
		byStat map[string]int
		total  int
		want   string
	}{
		{"无样本", map[string]int{}, 0, OutcomeEmpty},
		{"全部安装成功", map[string]int{"installed": 2}, 2, OutcomeOK},
		{"仅未验证成功", map[string]int{"unverified": 1}, 1, OutcomeUnverified},
		{"成功 + 未验证混合", map[string]int{"installed": 1, "unverified": 1}, 2, OutcomeOK},
		{"部分完成", map[string]int{"installed": 1, "partial": 1}, 2, OutcomePartial},
		{"有失败但仍有成功", map[string]int{"installed": 1, "failed": 1}, 2, OutcomePartial},
		{"全失败", map[string]int{"failed": 2}, 2, OutcomeFailed},
		{"全超时", map[string]int{"timeout": 1}, 1, OutcomeFailed},
		{"已安装视为可接受", map[string]int{"already_installed": 1}, 1, OutcomeOK},
		{"已触发视为可接受", map[string]int{"triggered": 1}, 1, OutcomeOK},
		{"全跳过", map[string]int{"skipped": 1}, 1, OutcomeSkipped},
	}
	for _, c := range cases {
		got := OverallOutcome(Summary{Total: c.total, ByStatus: c.byStat})
		if got != c.want {
			t.Errorf("%s：OverallOutcome = %q，期望 %q", c.name, got, c.want)
		}
	}
}

// 结论行与统计行必须措辞一致：若 OverallOutcome 把"未验证"并入"安装成功"，
// 日志会同时出现"总体结论：安装成功"和"安装未验证 1"，使用者会怀疑口径。
func TestUnverifiedOutcomeTextMatchesBucketText(t *testing.T) {
	if got := OutcomeText(OutcomeUnverified); got != "安装成功（未验证）" {
		t.Errorf("未验证档结论文案 = %q", got)
	}
	if got := BucketText("unverified"); got != "安装未验证" {
		t.Errorf("未验证桶文案 = %q", got)
	}
	if OutcomeText(OutcomeUnverified) == OutcomeText(OutcomeOK) {
		t.Error("未验证档不得与确定成功档使用同一文案")
	}
}

// 统计行只列非零项（避免"安装成功 0"这类噪音），且顺序按 BucketOrder。
func TestSummaryLineOmitsZeros(t *testing.T) {
	got := SummaryLine(map[string]int{"installed": 2, "failed": 1, "skipped": 0}, 3)
	want := "样本总数 3｜安装成功 2｜失败 1"
	if got != want {
		t.Errorf("SummaryLine = %q，期望 %q", got, want)
	}
}

// 未知状态 / 桶不得丢字，原样返回，便于排障时看出新枚举值。
func TestUnknownLabelsPassThrough(t *testing.T) {
	if got := StatusText("brand_new_status"); got != "brand_new_status" {
		t.Errorf("未知状态应原样返回，实得 %q", got)
	}
	if got := BucketText("brand_new_bucket"); got != "brand_new_bucket" {
		t.Errorf("未知桶应原样返回，实得 %q", got)
	}
}
