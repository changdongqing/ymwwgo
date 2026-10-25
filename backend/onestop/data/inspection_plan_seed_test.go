package data

import (
	"encoding/json"
	"regexp"
	"testing"
)

// 巡检计划种子守卫：钉死内置计划模板的频率规则与类别归属，防止手工编辑把
// 播种数据改坏。改 seeds/inspection_plans.json 前先读懂这里的约束。
func TestInspectionPlanSeedData(t *testing.T) {
	var seeds []inspectionPlanSeed
	if err := json.Unmarshal(inspectionPlanSeedData, &seeds); err != nil {
		t.Fatalf("unmarshal inspection plan seed json: %v", err)
	}
	if len(seeds) != 9 {
		t.Fatalf("inspection plan seeds = %d, want 9", len(seeds))
	}

	// 与 service_types.json 的联合校验：计划挂的类别必须真实存在且在 0103 子树
	var typeSeeds []serviceTypeSeed
	if err := json.Unmarshal(serviceTypeSeedData, &typeSeeds); err != nil {
		t.Fatalf("unmarshal service type seed json: %v", err)
	}
	codeSet := make(map[string]string, len(typeSeeds))
	for _, ts := range typeSeeds {
		codeSet[ts.Code] = ts.Level
	}

	timePattern := regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	datePattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	freqSeen := make(map[string]bool)

	for _, seed := range seeds {
		level, ok := codeSet[seed.Code]
		if !ok {
			t.Errorf("seed %q: 类别 %q 不在服务类型字典中", seed.Name, seed.Code)
			continue
		}
		if len(seed.Code) < 6 || seed.Code[:4] != "0103" {
			t.Errorf("seed %q: 计划只允许挂在 0103 巡检巡查子树，got %q", seed.Name, seed.Code)
		}
		if level != "LEVEL_MINOR" && level != "LEVEL_DETAIL" {
			t.Errorf("seed %q: 计划只允许挂在小类/细类上，%q 是 %s", seed.Name, seed.Code, level)
		}
		if seed.Name == "" {
			t.Errorf("seed %q: 计划名称为空", seed.Code)
		}

		freqSeen[seed.FreqType] = true
		switch seed.FreqType {
		case "FREQ_DAILY":
		case "FREQ_WEEKLY":
			if len(seed.Weekdays) == 0 {
				t.Errorf("seed %q: 每周计划须至少一个周几", seed.Name)
			}
			for _, wd := range seed.Weekdays {
				if wd < 1 || wd > 7 {
					t.Errorf("seed %q: weekdays %d 越界", seed.Name, wd)
				}
			}
		case "FREQ_MONTHLY", "FREQ_YEARLY":
			if len(seed.MonthDays) == 0 {
				t.Errorf("seed %q: 每月/每年计划须至少一个几号", seed.Name)
			}
			for _, md := range seed.MonthDays {
				if (md < 1 || md > 31) && md != -1 {
					t.Errorf("seed %q: monthDays %d 越界", seed.Name, md)
				}
			}
			if seed.FreqType == "FREQ_YEARLY" {
				if len(seed.Months) == 0 {
					t.Errorf("seed %q: 每年计划须至少一个月份", seed.Name)
				}
				for _, m := range seed.Months {
					if m < 1 || m > 12 {
						t.Errorf("seed %q: months %d 越界", seed.Name, m)
					}
				}
			}
		default:
			t.Errorf("seed %q: 未知周期类型 %q", seed.Name, seed.FreqType)
		}

		if len(seed.Times) == 0 {
			t.Errorf("seed %q: 须至少一个执行时间点", seed.Name)
		}
		for _, tm := range seed.Times {
			if !timePattern.MatchString(tm) {
				t.Errorf("seed %q: 执行时间 %q 非 HH:mm", seed.Name, tm)
			}
		}
		for _, d := range seed.ExcludedDates {
			if !datePattern.MatchString(d) {
				t.Errorf("seed %q: 排除日期 %q 非 YYYY-MM-DD", seed.Name, d)
			}
		}

		if len(seed.SpaceItems)+len(seed.EquipmentItems) == 0 {
			t.Errorf("seed %q: 点表为空（空间/设备合计至少 1 项）", seed.Name)
		}
	}

	// 四种周期类型至少各有一套模板（本期设计的代表性覆盖）
	for _, freq := range []string{"FREQ_DAILY", "FREQ_WEEKLY", "FREQ_MONTHLY", "FREQ_YEARLY"} {
		if !freqSeen[freq] {
			t.Errorf("种子缺少 %s 周期的模板计划", freq)
		}
	}
}
