package data

import (
	"encoding/json"
	"regexp"
	"testing"
)

// 种子数据守卫：钉死内置类型字典的结构与统一合同配置模板，防止手工编辑把
// 播种数据改坏（编码/层级/父子链/模板取值域/各大类配置矩阵）。
// 约束与 docs/connn/contract-type-and-contract-design.md §3.4 同源。
func TestContractTypeSeedData(t *testing.T) {
	var seeds []contractTypeSeed
	if err := json.Unmarshal(contractTypeSeedData, &seeds); err != nil {
		t.Fatalf("unmarshal seed json: %v", err)
	}
	if len(seeds) != 22 {
		t.Fatalf("seed rows = %d, want 22（6 大类 + 16 细类，docs/connn §3.4）", len(seeds))
	}

	codePattern := regexp.MustCompile(`^([0-9]{2}|[0-9]{4})$`)
	validPayMethods := map[string]bool{
		"PAY_ONCE": true, "PAY_MONTHLY": true, "PAY_QUARTERLY": true,
		"PAY_NODE": true, "PAY_PROGRESS": true,
	}
	validWidgets := map[string]bool{
		"WIDGET_TEXT": true, "WIDGET_TEXTAREA": true, "WIDGET_SELECT": true, "WIDGET_DATE": true,
	}

	seen := make(map[string]bool, len(seeds))
	parentSeen := make(map[string]bool, len(seeds))
	majorCount, detailCount := 0, 0
	for _, seed := range seeds {
		if !codePattern.MatchString(seed.Code) {
			t.Errorf("seed %q: code must be 2/4 digits", seed.Code)
		}
		if seen[seed.Code] {
			t.Errorf("seed %q: duplicate code", seed.Code)
		}
		seen[seed.Code] = true

		wantLevel := "LEVEL_MAJOR"
		if len(seed.Code) == 4 {
			wantLevel = "LEVEL_DETAIL"
		}
		if seed.Level != wantLevel {
			t.Errorf("seed %q: level = %q, want %q", seed.Code, seed.Level, wantLevel)
		}

		// 父节点必须先于子节点出现且真实存在（播种按序落库依赖这一点）
		if seed.ParentCode != "" {
			if seed.ParentCode != seed.Code[:2] {
				t.Errorf("seed %q: parentCode %q must be code 前 2 位", seed.Code, seed.ParentCode)
			}
			if !parentSeen[seed.ParentCode] {
				t.Errorf("seed %q: parent %q 未先于子节点出现或不存在", seed.Code, seed.ParentCode)
			}
		}
		parentSeen[seed.Code] = true

		// —— 统一合同配置模板：分层纪律与数值域 ——
		// 模板只配在大类层（细类整组回退大类；细类覆写留给管理页）
		hasTemplate := seed.PartyBLabel != nil || seed.PayMethod != nil || len(seed.PayNodes) > 0 ||
			seed.FeeDirection != nil || len(seed.DiffFields) > 0 || len(seed.RequiredAttach) > 0 ||
			seed.Guidance != nil
		if hasTemplate && len(seed.Code) != 2 {
			t.Errorf("seed %q: 合同配置模板只配在大类层", seed.Code)
		}

		if seed.PayMethod != nil && !validPayMethods[*seed.PayMethod] {
			t.Errorf("seed %q: 非法付款方式 %q", seed.Code, *seed.PayMethod)
		}
		if seed.FeeDirection != nil {
			switch *seed.FeeDirection {
			case "DIRECTION_PAYABLE", "DIRECTION_RECEIVABLE":
			default:
				t.Errorf("seed %q: 非法费用方向 %q", seed.Code, *seed.FeeDirection)
			}
		}

		// 付款节点：名称非空、比例 1-100、总和 100
		nodeSeen := make(map[string]bool)
		ratioSum := int32(0)
		for _, node := range seed.PayNodes {
			if node.Name == "" {
				t.Errorf("seed %q: 付款节点名称含空串", seed.Code)
			}
			if nodeSeen[node.Name] {
				t.Errorf("seed %q: 付款节点重复 %q", seed.Code, node.Name)
			}
			nodeSeen[node.Name] = true
			if node.RatioPercent < 1 || node.RatioPercent > 100 {
				t.Errorf("seed %q: 付款节点比例须在 1-100（%s=%d）", seed.Code, node.Name, node.RatioPercent)
			}
			ratioSum += node.RatioPercent
		}
		if len(nodeSeen) > 0 && ratioSum != 100 {
			t.Errorf("seed %q: 付款节点比例总和 = %d, want 100", seed.Code, ratioSum)
		}

		// 差异字段：key/label 非空、控件值域、下拉须有选项
		fieldSeen := make(map[string]bool)
		for _, field := range seed.DiffFields {
			if field.Key == "" || field.Label == "" {
				t.Errorf("seed %q: 差异字段 key/label 含空串", seed.Code)
			}
			if fieldSeen[field.Key] {
				t.Errorf("seed %q: 差异字段 key 重复 %q", seed.Code, field.Key)
			}
			fieldSeen[field.Key] = true
			if !validWidgets[field.Widget] {
				t.Errorf("seed %q: 非法差异字段控件 %q", seed.Code, field.Widget)
			}
			if field.Widget == "WIDGET_SELECT" && len(field.Options) == 0 {
				t.Errorf("seed %q: 下拉差异字段 %s 缺选项", seed.Code, field.Key)
			}
		}

		for _, name := range seed.RequiredAttach {
			if name == "" {
				t.Errorf("seed %q: 必备附件清单含空串", seed.Code)
			}
		}

		if len(seed.Code) == 2 {
			majorCount++
		} else {
			detailCount++
		}
	}

	if majorCount != 6 {
		t.Errorf("大类数 = %d, want 6（外包/施工/租赁/服务/维保/采购）", majorCount)
	}
	if detailCount != 16 {
		t.Errorf("细类数 = %d, want 16", detailCount)
	}

	// 关键取值样例：原型的 6 类型硬编码升级口径
	// 02 施工：按节点付款（基础30/主体40/竣工25/质保5）
	if s := findContractSeed(seeds, "02"); s == nil || s.PayMethod == nil || *s.PayMethod != "PAY_NODE" {
		t.Errorf("seed 02 施工合同：付款方式须为 PAY_NODE")
	}
	// 03 租赁：按季付款 + 应收（我方为出租方）
	if s := findContractSeed(seeds, "03"); s == nil || s.PayMethod == nil || *s.PayMethod != "PAY_QUARTERLY" ||
		s.FeeDirection == nil || *s.FeeDirection != "DIRECTION_RECEIVABLE" {
		t.Errorf("seed 03 租赁合同：须为按季付款 + 应收（原型口径：出租方为我方）")
	}
	// 06 采购：按进度付款（预付30/到货60/验收10）+ 供应商称谓
	if s := findContractSeed(seeds, "06"); s == nil || s.PayMethod == nil || *s.PayMethod != "PAY_PROGRESS" ||
		s.PartyBLabel == nil || *s.PartyBLabel != "供应商" {
		t.Errorf("seed 06 采购合同：须为按进度付款 + 乙方称谓=供应商")
	}
	// 01 外包：按月付款 + 三方公司称谓
	if s := findContractSeed(seeds, "01"); s == nil || s.PayMethod == nil || *s.PayMethod != "PAY_MONTHLY" ||
		s.PartyBLabel == nil || *s.PartyBLabel != "三方公司" {
		t.Errorf("seed 01 外包合同：须为按月付款 + 乙方称谓=三方公司")
	}
	// 每个大类都必须带差异字段定义（类型驱动动态表单的种子基线）
	for _, seed := range seeds {
		if len(seed.Code) == 2 && len(seed.DiffFields) == 0 {
			t.Errorf("seed %q: 大类缺差异字段定义", seed.Code)
		}
	}
}

func findContractSeed(seeds []contractTypeSeed, code string) *contractTypeSeed {
	for i, seed := range seeds {
		if seed.Code == code {
			return &seeds[i]
		}
	}
	return nil
}
