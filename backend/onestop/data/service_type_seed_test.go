package data

import (
	"encoding/json"
	"regexp"
	"testing"
)

// 种子数据守卫：钉死内置字典的结构与报修维修配置模板，防止手工编辑把
// 播种数据改坏（编码/层级/父子链/配置取值域）。改种子前先读懂这里的约束。
func TestServiceTypeSeedData(t *testing.T) {
	var seeds []serviceTypeSeed
	if err := json.Unmarshal(serviceTypeSeedData, &seeds); err != nil {
		t.Fatalf("unmarshal seed json: %v", err)
	}
	if len(seeds) != 299 {
		t.Fatalf("seed rows = %d, want 299 (docs/leixing/biaodan 全量字典)", len(seeds))
	}

	codePattern := regexp.MustCompile(`^([0-9]{2}|[0-9]{4}|[0-9]{6}|[0-9]{8})$`)
	levelOfLen := map[int]string{2: "LEVEL_MAJOR", 4: "LEVEL_MIDDLE", 6: "LEVEL_MINOR", 8: "LEVEL_DETAIL"}

	seen := make(map[string]bool, len(seeds))
	parentSeen := make(map[string]bool, len(seeds))
	minorCfg, detailCfg := 0, 0
	for _, seed := range seeds {
		if !codePattern.MatchString(seed.Code) {
			t.Errorf("seed %q: code must be 2/4/6/8 digits", seed.Code)
		}
		if seen[seed.Code] {
			t.Errorf("seed %q: duplicate code", seed.Code)
		}
		seen[seed.Code] = true

		if want, ok := levelOfLen[len(seed.Code)]; !ok || seed.Level != want {
			t.Errorf("seed %q: level = %q, want %q", seed.Code, seed.Level, want)
		}

		// 父节点必须先于子节点出现且真实存在（播种按序落库依赖这一点）
		if seed.ParentCode != "" {
			if seed.ParentCode != seed.Code[:len(seed.Code)-2] {
				t.Errorf("seed %q: parentCode %q must be code 去掉末两位", seed.Code, seed.ParentCode)
			}
			if !parentSeen[seed.ParentCode] {
				t.Errorf("seed %q: parent %q 未先于子节点出现或不存在", seed.Code, seed.ParentCode)
			}
		}
		parentSeen[seed.Code] = true

		// —— 报修维修配置：只允许出现在 0101 子树，且数值域受控 ——

		isRepair := len(seed.Code) >= 6 && seed.Code[:4] == "0101"
		hasCfg := seed.RepairPost != nil || seed.StandardWorkHours != nil ||
			seed.ResponseTimeLimit != nil || seed.CompletionTimeLimit != nil ||
			seed.Guidance != nil || len(seed.CommonPhrases) > 0 ||
			len(seed.StandardTools) > 0 || len(seed.SpareParts) > 0
		if hasCfg && !isRepair {
			t.Errorf("seed %q: 本期报修配置只允许出现在 0101 报修维修子树", seed.Code)
		}

		// —— 巡检巡查配置：标准巡检项目库只允许出现在 0103 子树 ——

		isInspection := len(seed.Code) >= 6 && seed.Code[:4] == "0103"
		if len(seed.InspectionItems) > 0 && !isInspection {
			t.Errorf("seed %q: 标准巡检项目库只允许出现在 0103 巡检巡查子树", seed.Code)
		}
		if isInspection && len(seed.InspectionItems) > 0 {
			seen := make(map[string]bool)
			for _, item := range seed.InspectionItems {
				if item == "" {
					t.Errorf("seed %q: 标准巡检项目含空串", seed.Code)
				}
				if seen[item] {
					t.Errorf("seed %q: 标准巡检项目重复 %q", seed.Code, item)
				}
				seen[item] = true
			}
		}

		if seed.RepairPost != nil {
			if len(seed.Code) != 6 {
				t.Errorf("seed %q: 维修岗位只配在小类层", seed.Code)
			}
			minorCfg++
		}
		if seed.Guidance != nil && len(seed.Code) != 6 {
			t.Errorf("seed %q: 指导说明只配在小类层", seed.Code)
		}
		if seed.StandardWorkHours != nil {
			if len(seed.Code) != 8 {
				t.Errorf("seed %q: 标准工时只配在细类层", seed.Code)
			}
			if *seed.StandardWorkHours <= 0 {
				t.Errorf("seed %q: 标准工时须为正数", seed.Code)
			}
			detailCfg++
		}
		if seed.ResponseTimeLimit != nil {
			if v := *seed.ResponseTimeLimit; v == 0 || v > 43200 {
				t.Errorf("seed %q: 响应时限须在 1-43200 分钟", seed.Code)
			}
		}
		if seed.CompletionTimeLimit != nil {
			if v := *seed.CompletionTimeLimit; v == 0 || v > 43200 {
				t.Errorf("seed %q: 完成时限须在 1-43200 分钟", seed.Code)
			}
		}
		for _, phrase := range seed.CommonPhrases {
			if phrase == "" {
				t.Errorf("seed %q: 常见报修语句含空串", seed.Code)
			}
		}
	}

	if minorCfg != 11 {
		t.Errorf("小类岗位配置数 = %d, want 11（报修维修全部小类）", minorCfg)
	}
	if detailCfg != 40 {
		t.Errorf("细类工时配置数 = %d, want 40（报修维修全部细类）", detailCfg)
	}

	// 电梯困人是全字典唯一的 SLA 覆盖样例：10 分钟到场、30 分钟完成救援
	if s := findSeed(seeds, "01010501"); s == nil || s.ResponseTimeLimit == nil || *s.ResponseTimeLimit != 10 {
		t.Errorf("seed 01010501 电梯困人：响应时限须为 10 分钟")
	}

	// 标准巡检项目库覆盖钉死：0103 的 8 个小类 + 每日防火巡查/每月防火检查两个细类
	inspectionSeeded := 0
	for _, seed := range seeds {
		if len(seed.InspectionItems) > 0 {
			inspectionSeeded++
		}
	}
	if inspectionSeeded != 10 {
		t.Errorf("标准巡检项目库覆盖节点数 = %d, want 10（0103 八小类 + 两个防火细类）", inspectionSeeded)
	}
	if s := findSeed(seeds, "010301"); s == nil || len(s.InspectionItems) == 0 {
		t.Errorf("seed 010301 消防巡查：须预置标准巡检项目库")
	}
}

func findSeed(seeds []serviceTypeSeed, code string) *serviceTypeSeed {
	for i, seed := range seeds {
		if seed.Code == code {
			return &seeds[i]
		}
	}
	return nil
}
