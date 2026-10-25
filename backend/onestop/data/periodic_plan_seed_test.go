package data

import (
	"encoding/json"
	"regexp"
	"testing"
)

// 周期计划种子守卫：钉死内置计划模板的频率规则与类别归属（触发方式须为
// 计划生成），防止手工编辑把播种数据改坏。改 seeds/periodic_plans.json 前
// 先读懂这里的约束。
func TestPeriodicPlanSeedData(t *testing.T) {
	var seeds []periodicPlanSeed
	if err := json.Unmarshal(periodicPlanSeedData, &seeds); err != nil {
		t.Fatalf("unmarshal periodic plan seed json: %v", err)
	}
	if len(seeds) != 28 {
		t.Fatalf("periodic plan seeds = %d, want 28（巡检期 9 + 统一期 19）", len(seeds))
	}

	// 与 service_types.json 的联合校验：计划挂的类别必须真实存在、为小类/细类，
	// 且已在种子中标记 TRIGGER_SCHEDULED（挂载门控的种子侧口径）
	var typeSeeds []serviceTypeSeed
	if err := json.Unmarshal(serviceTypeSeedData, &typeSeeds); err != nil {
		t.Fatalf("unmarshal service type seed json: %v", err)
	}
	typeByCode := make(map[string]*serviceTypeSeed, len(typeSeeds))
	for i, ts := range typeSeeds {
		typeByCode[ts.Code] = &typeSeeds[i]
	}

	timePattern := regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	datePattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	freqSeen := make(map[string]bool)
	nameSeen := make(map[string]bool, len(seeds))

	for _, seed := range seeds {
		ts, ok := typeByCode[seed.Code]
		if !ok {
			t.Errorf("seed %q: 类别 %q 不在服务类型字典中", seed.Name, seed.Code)
			continue
		}
		if ts.Level != "LEVEL_MINOR" && ts.Level != "LEVEL_DETAIL" {
			t.Errorf("seed %q: 计划只允许挂在小类/细类上，%q 是 %s", seed.Name, seed.Code, ts.Level)
		}
		if ts.TriggerMode == nil || *ts.TriggerMode != "TRIGGER_SCHEDULED" {
			t.Errorf("seed %q: 类别 %q 未标记 TRIGGER_SCHEDULED（计划只能挂计划生成类类别）", seed.Name, seed.Code)
		}
		if seed.Name == "" {
			t.Errorf("seed %q: 计划名称为空", seed.Code)
		}
		if nameSeen[seed.Name] {
			t.Errorf("seed %q: 计划名称重复", seed.Name)
		}
		nameSeen[seed.Name] = true

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
		if len(seed.Times) > 10 {
			t.Errorf("seed %q: 执行时间点至多 10 个", seed.Name)
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
		for _, item := range append(append([]string{}, seed.SpaceItems...), seed.EquipmentItems...) {
			if item == "" {
				t.Errorf("seed %q: 点表项含空串", seed.Name)
			}
		}
	}

	// 四种周期类型至少各有一套模板（代表性覆盖）
	for _, freq := range []string{"FREQ_DAILY", "FREQ_WEEKLY", "FREQ_MONTHLY", "FREQ_YEARLY"} {
		if !freqSeen[freq] {
			t.Errorf("种子缺少 %s 周期的模板计划", freq)
		}
	}
}
