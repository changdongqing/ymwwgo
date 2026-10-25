package data

import (
	"encoding/json"
	"regexp"
	"testing"
)

// 运送路线种子守卫：钉死内置路线模板的结构（类别归属/时长域/班次时间格式），
// 防止手工编辑把播种数据改坏。改种子前先读懂这里的约束。
func TestTransportRouteSeedData(t *testing.T) {
	var seeds []transportRouteSeed
	if err := json.Unmarshal(transportRouteSeedData, &seeds); err != nil {
		t.Fatalf("unmarshal seed json: %v", err)
	}
	if len(seeds) != 11 {
		t.Fatalf("seed rows = %d, want 11（医院巡回/急送/餐配 + 商写快递/押运 + 园区清运）", len(seeds))
	}

	timePattern := regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	nameSeen := make(map[string]bool, len(seeds))
	scheduled := 0
	for _, seed := range seeds {
		if len(seed.Code) < 6 || seed.Code[:4] != "0106" {
			t.Errorf("seed %q: 运送路线只允许挂在 0106 运送配送子树", seed.Code)
		}
		if seed.Name == "" || seed.Origin == "" || seed.Destination == "" {
			t.Errorf("seed %q: 名称/起点/终点必填", seed.Name)
		}
		if nameSeen[seed.Name] {
			t.Errorf("seed %q: 路线名称重复", seed.Name)
		}
		nameSeen[seed.Name] = true

		if seed.EstDurationMinutes == nil ||
			*seed.EstDurationMinutes == 0 || *seed.EstDurationMinutes > 1440 {
			t.Errorf("seed %q: 单程时长须在 1-1440 分钟", seed.Name)
		}

		viaSeen := make(map[string]bool)
		for _, p := range seed.ViaPoints {
			if p == "" {
				t.Errorf("seed %q: 途经点含空串", seed.Name)
			}
			if viaSeen[p] {
				t.Errorf("seed %q: 途经点重复 %q", seed.Name, p)
			}
			viaSeen[p] = true
		}

		timeSeen := make(map[string]bool)
		for _, tm := range seed.DepartureTimes {
			if !timePattern.MatchString(tm) {
				t.Errorf("seed %q: 班次时间须为 HH:mm，得到 %q", seed.Name, tm)
			}
			if timeSeen[tm] {
				t.Errorf("seed %q: 班次时间重复 %q", seed.Name, tm)
			}
			timeSeen[tm] = true
		}
		if len(seed.DepartureTimes) > 0 {
			scheduled++
		}
	}

	// 定时班次与按需即时两类都要有样例（定时班次是工单对象生成定时任务的来源）
	if scheduled == 0 || scheduled == len(seeds) {
		t.Errorf("定时班次路线数 = %d（共 %d），须同时包含定时班次与按需即时两类", scheduled, len(seeds))
	}
}
