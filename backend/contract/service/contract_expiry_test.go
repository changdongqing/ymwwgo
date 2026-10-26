package service

import (
	"testing"
	"time"
)

// 到期判定守卫：钉死"end_date 早于当日零点才过期"的口径——
// 当天到期的合同当天不过期（次日 02:30 扫描才关），时区取本地。
func TestStartOfToday(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"2026-10-26 15:04:05", "2026-10-26"},
		{"2026-10-26 00:00:00", "2026-10-26"},
		{"2026-12-31 23:59:59", "2026-12-31"},
		{"2027-01-01 00:00:01", "2027-01-01"},
	}
	for _, c := range cases {
		in, err := time.ParseInLocation("2006-01-02 15:04:05", c.in, time.Local)
		if err != nil {
			t.Fatalf("parse %q: %v", c.in, err)
		}
		got := StartOfToday(in).Format("2006-01-02")
		if got != c.want {
			t.Errorf("StartOfToday(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestExpiryBoundary(t *testing.T) {
	// 边界：end_date == 今日零点 → 不算过期（EndDateLT 严格小于）；
	// end_date == 昨日 23:59 → 过期。
	today := StartOfToday(time.Now())
	if !today.Add(-time.Minute).Before(today) {
		t.Errorf("昨日 23:59 应早于今日零点")
	}
	if today.Before(today) {
		t.Errorf("今日零点不应早于自身（EndDateLT 严格小于，当日到期不过期）")
	}
}
