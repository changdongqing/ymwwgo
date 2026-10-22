package ontology

import (
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 02 期 diff 组件单测（02 号文档 §2.4 / §8）：DiffCharacteristics、DiffChains。

func TestDiffCharacteristics(t *testing.T) {
	const me = uint32(100)
	// 现状：transitive + symmetric
	current := []*ent.OntAxiom{
		axiomRow(1, AxiomObjectPropertyCharacteristic, me, 0, PropertyCharacteristicIRIs[2], "", "", 0),
		axiomRow(2, AxiomObjectPropertyCharacteristic, me, 0, PropertyCharacteristicIRIs[3], "", "", 0),
	}

	// 无变化 → 零变更
	if changes := DiffCharacteristics(me, AxiomObjectPropertyCharacteristic, current, PropertyCharacteristicIRIs[2:4]); len(changes) != 0 {
		t.Fatalf("无变化应为零变更：%+v", changes)
	}

	// 增删：去 symmetric、增 functional → 删行 2 + 增 functional
	changes := DiffCharacteristics(me, AxiomObjectPropertyCharacteristic, current, []string{PropertyCharacteristicIRIs[0], PropertyCharacteristicIRIs[2]})
	if len(changes) != 2 {
		t.Fatalf("应为 2 条变更：%+v", changes)
	}
	if changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 2 {
		t.Fatalf("应删 symmetric 行：%+v", changes[0])
	}
	if changes[1].Op != AxiomOpAdd || changes[1].AxiomType != AxiomObjectPropertyCharacteristic ||
		changes[1].PredicateIRI != PropertyCharacteristicIRIs[0] || changes[1].ObjectID != 0 {
		t.Fatalf("应增 functional 行（object_id 空）：%+v", changes[1])
	}

	// 清空 → 全删
	changes = DiffCharacteristics(me, AxiomObjectPropertyCharacteristic, current, nil)
	if len(changes) != 2 {
		t.Fatalf("清空应删 2 行：%+v", changes)
	}
	for _, c := range changes {
		if c.Op != AxiomOpDelete {
			t.Fatalf("清空只应有删：%+v", c)
		}
	}
}

func TestDiffChains(t *testing.T) {
	const me = uint32(100)
	// 现状：链 #1 = [10,11]；链 #2 = [12,13]
	current := []PropertyChain{
		{AxiomID: 1, MemberIDs: []uint32{10, 11}},
		{AxiomID: 2, MemberIDs: []uint32{12, 13}},
	}

	// 无变化（含 id 且成员未变）→ 零变更
	targets := []PropertyChain{
		{AxiomID: 1, MemberIDs: []uint32{10, 11}},
		{AxiomID: 2, MemberIDs: []uint32{12, 13}},
	}
	if changes := DiffChains(me, current, targets); len(changes) != 0 {
		t.Fatalf("无变化应为零变更：%+v", changes)
	}

	// 成员顺序变化 = 变更（保序语义，不做 distinct 归一）：删旧行 + 增新行
	// 链 #2 未变保留（一并提交，否则会被"缺 id"规则删除）
	changes := DiffChains(me, current, []PropertyChain{
		{AxiomID: 1, MemberIDs: []uint32{11, 10}},
		{AxiomID: 2, MemberIDs: []uint32{12, 13}},
	})
	if len(changes) != 2 {
		t.Fatalf("顺序变化应删 1 增 1：%+v", changes)
	}
	if changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 1 {
		t.Fatalf("应删旧行 1：%+v", changes[0])
	}
	if changes[1].Op != AxiomOpAdd || changes[1].AxiomType != AxiomSubPropertyChainOf ||
		changes[1].SubjectID != me || changes[1].SortOrder != 0 {
		t.Fatalf("增行应带类型/主语/序：%+v", changes[1])
	}
	if changes[1].ValueText != EncodeChainMemberIDs([]uint32{11, 10}) {
		t.Fatalf("增行 value_text 应保序编码：%s", changes[1].ValueText)
	}

	// P∘P 重复成员不误报、不去重：[12,13] → [12,12] 是变更；[12,12] → [12,12] 不动
	if changes = DiffChains(me, current, []PropertyChain{{AxiomID: 1, MemberIDs: []uint32{10, 10}}, {AxiomID: 2, MemberIDs: []uint32{12, 13}}}); len(changes) != 2 {
		t.Fatalf("P∘P 变更应删 1 增 1：%+v", changes)
	}
	if changes = DiffChains(me, []PropertyChain{{AxiomID: 1, MemberIDs: []uint32{10, 10}}}, []PropertyChain{{AxiomID: 1, MemberIDs: []uint32{10, 10}}}); len(changes) != 0 {
		t.Fatalf("P∘P 未变应零变更：%+v", changes)
	}

	// 无 id → 增；缺 id → 删
	changes = DiffChains(me, current, []PropertyChain{
		{AxiomID: 1, MemberIDs: []uint32{10, 11}}, // 保留
		{MemberIDs: []uint32{14, 15, 16}},         // 新增（三成员链）
	})
	if len(changes) != 2 {
		t.Fatalf("应删链 2 + 增新链：%+v", changes)
	}
	deleted, added := changes[0], changes[1]
	if deleted.Op != AxiomOpDelete || deleted.AxiomID != 2 {
		t.Fatalf("未被引用的链 2 应删：%+v", deleted)
	}
	if added.Op != AxiomOpAdd || added.SortOrder != 1 || added.ValueText != EncodeChainMemberIDs([]uint32{14, 15, 16}) {
		t.Fatalf("新链应按提交序落行：%+v", added)
	}

	// 全量清空 → 全删
	changes = DiffChains(me, current, nil)
	if len(changes) != 2 {
		t.Fatalf("清空应删 2 行：%+v", changes)
	}

	// 防御：提交未知行 id（不在现状）按新增处理
	changes = DiffChains(me, nil, []PropertyChain{{AxiomID: 99, MemberIDs: []uint32{10, 11}}})
	if len(changes) != 1 || changes[0].Op != AxiomOpAdd {
		t.Fatalf("未知行 id 应按新增处理：%+v", changes)
	}
}
