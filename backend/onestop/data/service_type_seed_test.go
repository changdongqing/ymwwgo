package data

import (
	"encoding/json"
	"regexp"
	"testing"
)

// 种子数据守卫：钉死内置字典的结构与统一履约配置模板，防止手工编辑把
// 播种数据改坏（编码/层级/父子链/配置取值域/触发方式矩阵）。
// 改种子前先读懂这里的约束；矩阵与迁移 00012 的 SCHEDULED 回填前缀清单、
// docs/yizhanshi/onestop-unified-service-type-config.md §7 三处须同步维护。
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

	// SCHEDULED 播种矩阵（与迁移 00012 UPDATE 前缀一致）
	scheduledPrefixes := []string{
		"0102", "0103",
		"010401", "01040301", "01040303", "01040401", "010405",
		"010503", "010505", "010506",
		"01060201", "01060301", "01060502",
		"01080402",
		"01090102", "010903",
		"011001", "01100201",
		"01120402", "01120403",
	}
	isScheduled := func(code string) bool {
		for _, p := range scheduledPrefixes {
			if len(code) >= len(p) && code[:len(p)] == p {
				return true
			}
		}
		return false
	}

	seen := make(map[string]bool, len(seeds))
	parentSeen := make(map[string]bool, len(seeds))
	minorPost, detailWorkHours, standardItemsSeeded, scheduledCount := 0, 0, 0, 0
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

		// —— 统一履约模板：分层纪律与数值域 ——

		// 执行岗位只配在小类（工种层）：报修 11 + 运送 5 + 统一期新增 17 个计划型小类
		if seed.ExecutionPost != nil {
			if len(seed.Code) != 6 {
				t.Errorf("seed %q: 执行岗位只配在小类层", seed.Code)
			}
			minorPost++
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
			detailWorkHours++
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
				t.Errorf("seed %q: 常见语句含空串", seed.Code)
			}
		}

		// —— 触发方式矩阵：只允许 TRIGGER_SCHEDULED 显式标记，
		//    且只出现在矩阵前缀命中的小类/细类（其余留空 = NULL 默认按需）——
		if seed.TriggerMode != nil {
			if *seed.TriggerMode != "TRIGGER_SCHEDULED" {
				t.Errorf("seed %q: 种子只显式标记 TRIGGER_SCHEDULED（按需走 NULL 默认），got %q", seed.Code, *seed.TriggerMode)
			}
			if len(seed.Code) != 6 && len(seed.Code) != 8 {
				t.Errorf("seed %q: 触发方式只配在小类/细类", seed.Code)
			}
			if !isScheduled(seed.Code) {
				t.Errorf("seed %q: 不在 SCHEDULED 播种矩阵内却标记了计划生成", seed.Code)
			}
			scheduledCount++
		}

		// —— 标准作业项目库：非空、组内不重复，只配在计划型（SCHEDULED）小类/细类
		//    （0103 的每日/每月防火检查两个细类带各自点表，是细类层的既有样例）——
		if len(seed.StandardItems) > 0 {
			if len(seed.Code) != 6 && len(seed.Code) != 8 {
				t.Errorf("seed %q: 标准作业项目库只配在小类/细类层", seed.Code)
			}
			if !isScheduled(seed.Code) {
				t.Errorf("seed %q: 标准作业项目库只配在计划生成（SCHEDULED）子树", seed.Code)
			}
			itemSeen := make(map[string]bool)
			for _, item := range seed.StandardItems {
				if item == "" {
					t.Errorf("seed %q: 标准作业项目含空串", seed.Code)
				}
				if itemSeen[item] {
					t.Errorf("seed %q: 标准作业项目重复 %q", seed.Code, item)
				}
				itemSeen[item] = true
			}
			standardItemsSeeded++
		}

		// —— 验收方案：值域与结构（沿用 0101/0106 预置）——
		validModes := map[string]bool{"ACCEPT_SIGN": true, "ACCEPT_SCAN": true, "ACCEPT_PHOTO": true, "ACCEPT_DUAL": true}
		for _, mode := range seed.AcceptanceModes {
			if !validModes[mode] {
				t.Errorf("seed %q: 非法验收方式 %q", seed.Code, mode)
			}
		}
		if len(seed.AcceptanceModes) == 0 && len(seed.AcceptanceItems) > 0 {
			t.Errorf("seed %q: 配了验收项却没有验收方式", seed.Code)
		}
		accSeen := make(map[string]bool)
		for _, item := range seed.AcceptanceItems {
			if item.Name == "" {
				t.Errorf("seed %q: 验收项名称含空串", seed.Code)
			}
			if accSeen[item.Name] {
				t.Errorf("seed %q: 验收项重复 %q", seed.Code, item.Name)
			}
			accSeen[item.Name] = true
		}
		if seed.DefaultPriority != nil {
			switch *seed.DefaultPriority {
			case "PRIORITY_NORMAL", "PRIORITY_URGENT", "PRIORITY_EMERGENCY":
			default:
				t.Errorf("seed %q: 非法优先级 %q", seed.Code, *seed.DefaultPriority)
			}
		}

		// —— 专项属性：值域与分层（沿用 0106 预置口径）——
		if seed.TemperatureRequirement != nil {
			switch *seed.TemperatureRequirement {
			case "TEMP_NORMAL", "TEMP_COLD", "TEMP_FROZEN", "TEMP_WARM":
			default:
				t.Errorf("seed %q: 非法温控要求 %q", seed.Code, *seed.TemperatureRequirement)
			}
			if len(seed.Code) != 6 && seed.Code != "01060403" {
				t.Errorf("seed %q: 温控要求只配在小类层（营养餐细类 01060403 为唯一细类覆盖样例）", seed.Code)
			}
		}
		for _, cargo := range seed.CargoTypes {
			if cargo == "" {
				t.Errorf("seed %q: 运送物品细目含空串", seed.Code)
			}
		}
		if len(seed.CargoTypes) > 0 && len(seed.Code) != 6 {
			t.Errorf("seed %q: 物品细目只配在小类层", seed.Code)
		}
	}

	if minorPost != 33 {
		t.Errorf("小类执行岗位配置数 = %d, want 33（报修 11 + 运送 5 + 统一期计划型小类 17）", minorPost)
	}
	if detailWorkHours != 52 {
		t.Errorf("细类工时配置数 = %d, want 52（报修维修 40 + 运送配送 12 个细类）", detailWorkHours)
	}
	if scheduledCount != 105 {
		t.Errorf("SCHEDULED 标记节点数 = %d, want 105（§7 矩阵：80 细类 + 25 小类）", scheduledCount)
	}
	if standardItemsSeeded != 27 {
		t.Errorf("标准作业项目库覆盖节点数 = %d, want 27（0103 既有 10 + 统一期新增 17 个计划型小类）", standardItemsSeeded)
	}

	// 电梯困人是全字典唯一的 SLA 覆盖样例：10 分钟到场、30 分钟完成救援
	if s := findSeed(seeds, "01010501"); s == nil || s.ResponseTimeLimit == nil || *s.ResponseTimeLimit != 10 {
		t.Errorf("seed 01010501 电梯困人：响应时限须为 10 分钟")
	}

	// —— 运送配送配置覆盖钉死：0106 全部 5 小类 + 12 细类 ——
	transportMinor, transportDetail := 0, 0
	for _, seed := range seeds {
		if len(seed.Code) == 6 && seed.Code[:4] == "0106" {
			if seed.ExecutionPost == nil {
				t.Errorf("seed %q: 运送小类缺执行岗位", seed.Code)
			}
			if seed.DefaultPriority == nil || len(seed.AcceptanceModes) == 0 {
				t.Errorf("seed %q: 运送小类缺优先级或验收方式", seed.Code)
			}
			transportMinor++
		}
		if len(seed.Code) == 8 && seed.Code[:4] == "0106" {
			if seed.StandardWorkHours == nil {
				t.Errorf("seed %q: 运送细类缺标准工时", seed.Code)
			}
			transportDetail++
		}
	}
	if transportMinor != 5 {
		t.Errorf("运送小类配置数 = %d, want 5（0106 全部小类）", transportMinor)
	}
	if transportDetail != 12 {
		t.Errorf("运送细类配置数 = %d, want 12（0106 全部细类）", transportDetail)
	}

	// 关键取值样例：标本冷链、急送标本加急、商写押运需审批、营养餐保温
	if s := findSeed(seeds, "010602"); s == nil || s.TemperatureRequirement == nil || *s.TemperatureRequirement != "TEMP_COLD" {
		t.Errorf("seed 010602 标本配送：温控要求须为 TEMP_COLD")
	}
	if s := findSeed(seeds, "01060202"); s == nil || s.DefaultPriority == nil || *s.DefaultPriority != "PRIORITY_URGENT" {
		t.Errorf("seed 01060202 急送标本：优先级须为 PRIORITY_URGENT")
	}
	if s := findSeed(seeds, "01060501"); s == nil || s.RequiresApproval == nil || !*s.RequiresApproval {
		t.Errorf("seed 01060501 搬入搬出审批押运：须要求事前审批")
	}
	if s := findSeed(seeds, "01060403"); s == nil || s.TemperatureRequirement == nil || *s.TemperatureRequirement != "TEMP_WARM" {
		t.Errorf("seed 01060403 营养餐配送：温控要求须为 TEMP_WARM")
	}

	// —— 通用履约配置覆盖钉死：0101 全部 11 小类优先级基线 + 电梯困人特急覆盖 ——
	repairPriority := 0
	for _, seed := range seeds {
		if len(seed.Code) == 6 && seed.Code[:4] == "0101" && seed.DefaultPriority != nil {
			if *seed.DefaultPriority != "PRIORITY_NORMAL" {
				t.Errorf("seed %q: 报修小类优先级基线须为 PRIORITY_NORMAL", seed.Code)
			}
			if len(seed.AcceptanceModes) == 0 {
				t.Errorf("seed %q: 报修小类缺完工验收方式", seed.Code)
			}
			repairPriority++
		}
	}
	if repairPriority != 11 {
		t.Errorf("报修小类优先级配置数 = %d, want 11", repairPriority)
	}
	if s := findSeed(seeds, "01010501"); s == nil || s.DefaultPriority == nil || *s.DefaultPriority != "PRIORITY_EMERGENCY" {
		t.Errorf("seed 01010501 电梯困人：优先级须为 PRIORITY_EMERGENCY")
	}

	// —— 统一期关键样例：维保小类（计划生成 + 岗位 + 项目库）、
	//    定时班次运送细类（计划生成）、急送细类（未标记 = 按需）——
	if s := findSeed(seeds, "010201"); s == nil || s.TriggerMode == nil ||
		s.ExecutionPost == nil || len(s.StandardItems) == 0 {
		t.Errorf("seed 010201 电梯维保：须预置计划生成 + 执行岗位 + 标准作业项目库")
	}
	if s := findSeed(seeds, "01060301"); s == nil || s.TriggerMode == nil {
		t.Errorf("seed 01060301 静配巡回配送：须标记计划生成（定时班次）")
	}
	if s := findSeed(seeds, "01060202"); s != nil && s.TriggerMode != nil {
		t.Errorf("seed 01060202 急送标本：按需发起不得标记计划生成")
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
