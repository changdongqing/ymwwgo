package service

import (
	"testing"
	"time"

	"go-wind-admin/onestop/data"
	"go-wind-admin/onestop/data/ent"
	"go-wind-admin/onestop/data/ent/periodicplan"
)

func freqPtr(s string) *periodicplan.FreqType {
	f := periodicplan.FreqType(s)
	return &f
}

// 展开纯函数守卫：钉死 P1 设计 §13 的频率口径——
// 季度 = FREQ_YEARLY months 多值、半月 = FREQ_MONTHLY month_days=[1,15]、
// clamp_to_month_end 月底顺延/跳过、-1=月末、excluded_dates 全天不生成、
// 生效区间含当日。工单调度器与周期计划配置共用这套口径，任何一侧改动
// 都会在这里红。

func strPtr(s string) *string { return &s }

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return d
}

func expandAt(t *testing.T, plan *ent.PeriodicPlan, dates ...string) []string {
	t.Helper()
	ds := make([]time.Time, 0, len(dates))
	for _, d := range dates {
		ds = append(ds, day(t, d))
	}
	var out []string
	for _, at := range expandPlanInstances(plan, ds) {
		out = append(out, at.Format("2006-01-02 15:04"))
	}
	return out
}

func assertList(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("instances = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("instances[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestExpandDailyMultipleTimes(t *testing.T) {
	plan := &ent.PeriodicPlan{
		FreqType: freqPtr("FREQ_DAILY"),
		Times:    []string{"08:00", "14:00"},
	}
	assertList(t, expandAt(t, plan, "2026-10-26"),
		"2026-10-26 08:00", "2026-10-26 14:00")
}

func TestExpandWeekly(t *testing.T) {
	// 2026-10-26 是周一，2026-10-28 是周三，2026-10-30 是周五
	plan := &ent.PeriodicPlan{
		FreqType: freqPtr("FREQ_WEEKLY"),
		Weekdays: []int32{1, 3},
		Times:    []string{"09:30"},
	}
	assertList(t, expandAt(t, plan, "2026-10-26", "2026-10-27", "2026-10-28"), "2026-10-26 09:30", "2026-10-28 09:30")
}

func TestExpandMonthlyHalfMonth(t *testing.T) {
	// 半月维保口径：FREQ_MONTHLY month_days=[1,15]（weekdays 表达不了双周）
	plan := &ent.PeriodicPlan{
		FreqType:  freqPtr("FREQ_MONTHLY"),
		MonthDays: []int32{1, 15},
		Times:     []string{"07:00"},
	}
	assertList(t, expandAt(t, plan, "2026-10-01", "2026-10-15", "2026-10-16"),
		"2026-10-01 07:00", "2026-10-15 07:00")
}

func TestExpandYearlyQuarterly(t *testing.T) {
	// 季度口径：FREQ_YEARLY months 多值（如 [1,4,7,10]）
	plan := &ent.PeriodicPlan{
		FreqType:  freqPtr("FREQ_YEARLY"),
		Months:    []int32{1, 4, 7, 10},
		MonthDays: []int32{5},
		Times:     []string{"10:00"},
	}
	assertList(t, expandAt(t, plan, "2026-01-05", "2026-02-05", "2026-04-05", "2026-07-05", "2026-10-05"),
		"2026-01-05 10:00", "2026-04-05 10:00", "2026-07-05 10:00", "2026-10-05 10:00")
}

func TestExpandMonthEndAndClamp(t *testing.T) {
	// -1 = 当月最后一天；clamp：号数超出当月天数时顺延至月末（2 月的 31 号）
	lastDay := &ent.PeriodicPlan{
		FreqType:  freqPtr("FREQ_MONTHLY"),
		MonthDays: []int32{-1},
		Times:     []string{"23:00"},
	}
	// 2026-02 的最后一天是 28 号
	assertList(t, expandAt(t, lastDay, "2026-02-27", "2026-02-28"), "2026-02-28 23:00")

	clampTrue := &ent.PeriodicPlan{
		FreqType:        freqPtr("FREQ_MONTHLY"),
		MonthDays:       []int32{31},
		ClampToMonthEnd: true,
		Times:           []string{"08:00"},
	}
	// 2 月无 31 号，clamp=true 顺延至 28 号执行（3 月正常 31 号）
	assertList(t, expandAt(t, clampTrue, "2026-02-28", "2026-03-31"),
		"2026-02-28 08:00", "2026-03-31 08:00")

	clampFalse := &ent.PeriodicPlan{
		FreqType:        freqPtr("FREQ_MONTHLY"),
		MonthDays:       []int32{31},
		ClampToMonthEnd: false,
		Times:           []string{"08:00"},
	}
	// clamp=false 当月跳过
	assertList(t, expandAt(t, clampFalse, "2026-02-28", "2026-03-31"), "2026-03-31 08:00")
}

func TestExpandExcludedDatesAndEffectiveRange(t *testing.T) {
	plan := &ent.PeriodicPlan{
		FreqType:      freqPtr("FREQ_DAILY"),
		Times:         []string{"08:00"},
		ExcludedDates: []string{"2026-10-27"},
		// 生效区间含当日：26 号在区间内、29 号在区间外
		EffectiveFrom: timePtrDay("2026-10-26"),
		EffectiveTo:   timePtrDay("2026-10-28"),
	}
	assertList(t, expandAt(t, plan, "2026-10-25", "2026-10-26", "2026-10-27", "2026-10-28", "2026-10-29"),
		"2026-10-26 08:00", "2026-10-28 08:00")
}

func timePtrDay(s string) *time.Time {
	d, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return &d
}

func TestParseHHMM(t *testing.T) {
	if d, err := parseHHMM("08:05"); err != nil || d != 8*time.Hour+5*time.Minute {
		t.Fatalf("parseHHMM(08:05) = %v, %v", d, err)
	}
	for _, bad := range []string{"24:00", "08:60", "0800", "", "ab:cd"} {
		if _, err := parseHHMM(bad); err == nil {
			t.Fatalf("parseHHMM(%q) should fail", bad)
		}
	}
	// "8:5" 会被 Sscanf 宽松解析为 8:05——计划/路线侧入库已有严格 HH:mm 正则
	// 把关（periodic_plan_service 的 timePattern），调度器宽松可接受，此处钉死该口径
	if d, err := parseHHMM("8:5"); err != nil || d != 8*time.Hour+5*time.Minute {
		t.Fatalf("parseHHMM(8:5) lenient parse = %v, %v (want 8h5m)", d, err)
	}
	if _, err := parseHHMM("23:59"); err != nil {
		t.Fatalf("parseHHMM(23:59) failed: %v", err)
	}
}

func TestDeriveOrderNo(t *testing.T) {
	cases := []struct {
		id   uint32
		want string
	}{
		{1, "WO-00000001"},
		{123, "WO-00000123"},
		{99999999, "WO-99999999"},
	}
	for _, c := range cases {
		if got := data.DeriveOrderNo(c.id); got != c.want {
			t.Fatalf("order no of %d = %q, want %q", c.id, got, c.want)
		}
	}
}
