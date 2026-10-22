package ontology

import (
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 03 期 diff 组件单测（03 号文档 §8）：DiffSubjectSideValues（值域按 value_text 集合 diff，
// 新增行 sort_order 从存量最大值后顺排——源修订 #2）、DiffCharacteristics 43 型实例化。

func rangeRow(id uint32, subjectID uint32, value string, sortOrder uint32) *ent.OntAxiom {
	return axiomRow(id, AxiomDataPropertyRange, subjectID, 0, "", value, "", sortOrder)
}

func TestDiffSubjectSideValues_NoChange(t *testing.T) {
	const me = uint32(100)
	current := []*ent.OntAxiom{
		rangeRow(1, me, IRIXSDNamespace+"string", 0),
		rangeRow(2, me, IRIXSDNamespace+"integer", 1),
	}
	if changes := DiffSubjectSideValues(me, AxiomDataPropertyRange, current, []string{IRIXSDNamespace + "string", IRIXSDNamespace + "integer"}); len(changes) != 0 {
		t.Fatalf("无变化应为零变更：%+v", changes)
	}
}

func TestDiffSubjectSideValues_AddAppendsAfterExistingMax(t *testing.T) {
	const me = uint32(100)
	current := []*ent.OntAxiom{
		rangeRow(1, me, IRIXSDNamespace+"string", 0),
		rangeRow(2, me, IRIXSDNamespace+"integer", 1),
	}
	// 追加 date：存量行不动，新增行 sort_order=2（存量最大值后顺排）
	changes := DiffSubjectSideValues(me, AxiomDataPropertyRange, current,
		[]string{IRIXSDNamespace + "string", IRIXSDNamespace + "integer", IRIXSDNamespace + "date"})
	if len(changes) != 1 {
		t.Fatalf("应只增 1 行：%+v", changes)
	}
	c := changes[0]
	if c.Op != AxiomOpAdd || c.ValueText != IRIXSDNamespace+"date" || c.SortOrder != 2 {
		t.Fatalf("新增行应 sort_order=2：%+v", c)
	}
}

func TestDiffSubjectSideValues_MixedEditReadBackMatchesSubmit(t *testing.T) {
	const me = uint32(100)
	// 验收 #7 源修订 #2 场景：[string, integer] → 提交 [integer, date] → 回读序 = 提交序
	current := []*ent.OntAxiom{
		rangeRow(1, me, IRIXSDNamespace+"string", 0),
		rangeRow(2, me, IRIXSDNamespace+"integer", 1),
	}
	changes := DiffSubjectSideValues(me, AxiomDataPropertyRange, current,
		[]string{IRIXSDNamespace + "integer", IRIXSDNamespace + "date"})
	if len(changes) != 2 {
		t.Fatalf("应 1 删 1 增：%+v", changes)
	}
	deletes, adds := 0, 0
	for _, c := range changes {
		switch c.Op {
		case AxiomOpDelete:
			deletes++
			if c.AxiomID != 1 { // string 行删
				t.Fatalf("应删 string 行 1：%+v", c)
			}
		case AxiomOpAdd:
			adds++
			if c.ValueText != IRIXSDNamespace+"date" || c.SortOrder != 2 {
				t.Fatalf("date 应 sort_order=2（保留行 integer=1 之后）：%+v", c)
			}
		}
	}
	if deletes != 1 || adds != 1 {
		t.Fatalf("应恰 1 删 1 增：%+v", changes)
	}
	// 落库后按 sort_order 回读：integer(sort=1) < date(sort=2) → [integer, date] = 提交序
	// （保留行 sort_order 不动 + 新增行顺排的组合保证，源修订 #2 的核心断言）
}

func TestDiffSubjectSideValues_ReorderRewritesAll(t *testing.T) {
	const me = uint32(100)
	current := []*ent.OntAxiom{
		rangeRow(1, me, IRIXSDNamespace+"string", 0),
		rangeRow(2, me, IRIXSDNamespace+"integer", 1),
	}
	// 纯重排提交 [integer, string]：保留行相对序与提交序冲突 → 整段重写（删全部 + 按提交序重建）
	changes := DiffSubjectSideValues(me, AxiomDataPropertyRange, current,
		[]string{IRIXSDNamespace + "integer", IRIXSDNamespace + "string"})
	if len(changes) != 4 {
		t.Fatalf("重排应 2 删 2 增：%+v", changes)
	}
	for i, c := range changes {
		if i < 2 {
			if c.Op != AxiomOpDelete {
				t.Fatalf("前两条应为删：%+v", c)
			}
			continue
		}
		if c.Op != AxiomOpAdd || c.SortOrder != int32(i-2) {
			t.Fatalf("重建行应按提交序 sort_order=%d：%+v", i-2, c)
		}
	}
}

func TestDiffSubjectSideValues_ClearAndEmpty(t *testing.T) {
	const me = uint32(100)
	current := []*ent.OntAxiom{
		rangeRow(1, me, IRIXSDNamespace+"string", 0),
		rangeRow(2, me, IRIXSDNamespace+"integer", 1),
	}
	// 清空：全删、无新增
	changes := DiffSubjectSideValues(me, AxiomDataPropertyRange, current, nil)
	if len(changes) != 2 {
		t.Fatalf("清空应删 2 行：%+v", changes)
	}
	// 空现状 + 空目标：零变更
	if changes := DiffSubjectSideValues(me, AxiomDataPropertyRange, nil, nil); len(changes) != 0 {
		t.Fatalf("空对空应零变更：%+v", changes)
	}
}

func TestDiffSubjectSideValues_DuplicateDefence(t *testing.T) {
	const me = uint32(100)
	// 脏数据：同 value 两行 → 保留首行、多余行删；目标重复项只补一行
	current := []*ent.OntAxiom{
		rangeRow(1, me, IRIXSDNamespace+"string", 0),
		rangeRow(2, me, IRIXSDNamespace+"string", 1),
	}
	changes := DiffSubjectSideValues(me, AxiomDataPropertyRange, current, []string{IRIXSDNamespace + "string"})
	if len(changes) != 1 || changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 2 {
		t.Fatalf("同 value 多行应保留首行删多余：%+v", changes)
	}

	changes = DiffSubjectSideValues(me, AxiomDataPropertyRange, nil, []string{IRIXSDNamespace + "integer", IRIXSDNamespace + "integer"})
	if len(changes) != 1 {
		t.Fatalf("目标重复项应只补一行：%+v", changes)
	}
}

func TestDiffCharacteristics_DataPropertyType43(t *testing.T) {
	const me = uint32(100)
	current := []*ent.OntAxiom{
		axiomRow(1, AxiomDataPropertyCharacteristic, me, 0, IRIOWLFunctionalProperty, "", "", 0),
	}
	// 勾选 → 勾选：零变更（单复选框退化场景）
	if changes := DiffCharacteristics(me, AxiomDataPropertyCharacteristic, current, []string{IRIOWLFunctionalProperty}); len(changes) != 0 {
		t.Fatalf("函数性保持应零变更：%+v", changes)
	}
	// 取消勾选 → 删一行
	changes := DiffCharacteristics(me, AxiomDataPropertyCharacteristic, current, nil)
	if len(changes) != 1 || changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 1 {
		t.Fatalf("取消函数性应删行 1：%+v", changes)
	}
	// 新增勾选 → 增一行（43 型）
	changes = DiffCharacteristics(me, AxiomDataPropertyCharacteristic, nil, []string{IRIOWLFunctionalProperty})
	if len(changes) != 1 || changes[0].AxiomType != AxiomDataPropertyCharacteristic ||
		changes[0].PredicateIRI != IRIOWLFunctionalProperty {
		t.Fatalf("应增 43 型函数性行：%+v", changes)
	}
}
