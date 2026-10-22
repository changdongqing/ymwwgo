package ontology

import (
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// ---- 测试工具：构造带指针字段的公理行 ----

func axiomRow(id uint32, axiomType, subjectID, objectID uint32, predicate, value, lang string, sortOrder uint32) *ent.OntAxiom {
	ax := &ent.OntAxiom{ID: id, AxiomType: &axiomType, SubjectID: &subjectID, SortOrder: &sortOrder}
	if objectID != 0 {
		ax.ObjectID = &objectID
	}
	if predicate != "" {
		ax.PredicateIri = &predicate
	}
	if value != "" {
		ax.ValueText = &value
	}
	if lang != "" {
		ax.Lang = &lang
	}
	return ax
}

func TestDiffSubjectSideRelations(t *testing.T) {
	const me = 100
	// 现状：父类 A(10)、B(11)，sort_order 0/1
	current := []*ent.OntAxiom{
		axiomRow(1, AxiomSubClassOf, me, 10, "", "", "", 0),
		axiomRow(2, AxiomSubClassOf, me, 11, "", "", "", 1),
	}

	// 无变化 → 零变更
	changes := DiffSubjectSideRelations(me, AxiomSubClassOf, current, []uint32{10, 11})
	if len(changes) != 0 {
		t.Fatalf("无变化应为零变更，实得 %+v", changes)
	}

	// 增删：[A(0),B(1)] → [B,C] —— B 位置前移也走重写，删 A、删 B 旧行、增 B(0)、增 C(1)
	changes = DiffSubjectSideRelations(me, AxiomSubClassOf, current, []uint32{11, 12})
	if len(changes) != 4 {
		t.Fatalf("应为 4 条变更（删 A + 重写 B + 增 C），实得 %+v", changes)
	}
	deletes := map[uint32]bool{}
	adds := map[uint32]int32{}
	for _, c := range changes {
		switch c.Op {
		case AxiomOpDelete:
			deletes[c.AxiomID] = true
		case AxiomOpAdd:
			adds[c.ObjectID] = c.SortOrder
		}
	}
	if !deletes[1] || !deletes[2] {
		t.Fatalf("A/B 旧行都应删除：%+v", changes)
	}
	if adds[11] != 0 || adds[12] != 1 {
		t.Fatalf("B/C 应按提交顺序写 sort_order（B=0, C=1）：%+v", adds)
	}

	// 重排：[A,B] → [B,A]，无增删，但按提交顺序重写 sort_order（删旧行 + 增新行）
	changes = DiffSubjectSideRelations(me, AxiomSubClassOf, current, []uint32{11, 10})
	addCount, deleteCount := 0, 0
	reorderAdds := map[uint32]int32{}
	for _, c := range changes {
		switch c.Op {
		case AxiomOpAdd:
			addCount++
			reorderAdds[c.ObjectID] = c.SortOrder
			if c.SubjectID != me {
				t.Fatalf("ADD 变更应带主语：%+v", c)
			}
		case AxiomOpDelete:
			deleteCount++
		}
	}
	if addCount != 2 || deleteCount != 2 {
		t.Fatalf("重排应删 2 增 2，实得 增%d 删%d：%+v", addCount, deleteCount, changes)
	}
	if reorderAdds[11] != 0 || reorderAdds[10] != 1 {
		t.Fatalf("重排后 sort_order 应与提交顺序一致（B=0, A=1）：%+v", reorderAdds)
	}

	// 清空：全部删除
	changes = DiffSubjectSideRelations(me, AxiomSubClassOf, current, nil)
	if len(changes) != 2 {
		t.Fatalf("清空应删 2 行：%+v", changes)
	}
	for _, c := range changes {
		if c.Op != AxiomOpDelete || c.SubjectID != me {
			t.Fatalf("清空变更应为带主语的 DELETE：%+v", c)
		}
	}
}

func TestDiffSymmetricPairs(t *testing.T) {
	const me = 100
	// 现状：与 10、20 不相交（成对四行）
	current := []SymmetricAxiomPair{
		{OtherID: 10, ForwardAxiomID: 1, BackwardAxiomID: 2},
		{OtherID: 20, ForwardAxiomID: 3, BackwardAxiomID: 4},
	}

	// 无变化
	changes := DiffSymmetricPairs(me, AxiomDisjointWith, current, []uint32{10, 20})
	if len(changes) != 0 {
		t.Fatalf("无变化应为零变更：%+v", changes)
	}

	// 删 10、增 30：删两行 + 增两行
	changes = DiffSymmetricPairs(me, AxiomDisjointWith, current, []uint32{20, 30})
	if len(changes) != 4 {
		t.Fatalf("应删 2 增 2（成对）：%+v", changes)
	}
	deletes := map[uint32]bool{}
	addPairs := map[uint32]int{}
	for _, c := range changes {
		if c.Op == AxiomOpDelete {
			deletes[c.AxiomID] = true
		}
		if c.Op == AxiomOpAdd {
			addPairs[c.SubjectID]++
			if c.SortOrder != 1 {
				t.Fatalf("新增对 sort_order 应为提交序 1：%+v", c)
			}
		}
	}
	if !deletes[1] || !deletes[2] || deletes[3] || deletes[4] {
		t.Fatalf("应只删与 10 的两行：%+v", deletes)
	}
	// me→30 与 30→me 两行
	if addPairs[me] != 1 || addPairs[30] != 1 {
		t.Fatalf("新增应成对（me→30 与 30→me）：%+v", changes)
	}
}

func TestDiffAnnotations(t *testing.T) {
	const me = 100
	current := []*ent.OntAxiom{
		axiomRow(1, AxiomAnnotationAssertion, me, 0, PredicateRDFSLabel, "旧名", "zh", 0),
		axiomRow(2, AxiomAnnotationAssertion, me, 0, PredicateRDFSComment, "旧注释", "zh", 0),
	}

	// 值相等跳过
	changes := DiffAnnotations(me, current, map[string]*string{
		PredicateRDFSLabel: strPtr("旧名"),
	})
	if len(changes) != 0 {
		t.Fatalf("值相等应跳过：%+v", changes)
	}

	// 全量替换：删旧 + 增新（label 带 zh）
	changes = DiffAnnotations(me, current, map[string]*string{
		PredicateRDFSLabel: strPtr("新名"),
	})
	if len(changes) != 2 {
		t.Fatalf("替换应为删 1 增 1：%+v", changes)
	}
	if changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 1 || changes[0].SubjectID != me {
		t.Fatalf("删除变更应带公理 id 与主语：%+v", changes[0])
	}
	if changes[1].Op != AxiomOpAdd || changes[1].ValueText != "新名" || changes[1].Lang != "zh" {
		t.Fatalf("新增 label 应带 zh：%+v", changes[1])
	}

	// 空串 = 清空（protojson 无法区分 null 与缺省，本仓定案：空串清空、缺省不变）
	changes = DiffAnnotations(me, current, map[string]*string{
		PredicateRDFSComment: strPtr(""),
	})
	if len(changes) != 1 || changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 2 {
		t.Fatalf("空串应只删旧行（清空）：%+v", changes)
	}

	// 键不存在 = 不变
	changes = DiffAnnotations(me, current, map[string]*string{})
	if len(changes) != 0 {
		t.Fatalf("键缺失应不变：%+v", changes)
	}
}

func TestDiffDeprecated(t *testing.T) {
	const me = 100
	current := []*ent.OntAxiom{
		axiomRow(1, AxiomAnnotationAssertion, me, 0, PredicateOWLDeprecated, "true", "", 0),
	}

	// 已废弃 → 仍废弃：无变更
	if changes := DiffDeprecated(me, current, true); len(changes) != 0 {
		t.Fatalf("状态不变应为零变更：%+v", changes)
	}

	// false = 移除断言（源修订 #8）
	changes := DiffDeprecated(me, current, false)
	if len(changes) != 1 || changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 1 {
		t.Fatalf("false 应删除断言行：%+v", changes)
	}

	// 未废弃 → 置废弃：补断言（值 "true"，无 lang）
	changes = DiffDeprecated(me, nil, true)
	if len(changes) != 1 || changes[0].Op != AxiomOpAdd ||
		changes[0].PredicateIRI != PredicateOWLDeprecated || changes[0].ValueText != "true" || changes[0].Lang != "" {
		t.Fatalf("true 应补断言：%+v", changes)
	}
}

func TestDiffEquivalentSingleValue(t *testing.T) {
	const me = 100
	// 等价 V1 单值，走 DiffSymmetricPairs（成对存储）
	current := []SymmetricAxiomPair{
		{OtherID: 10, ForwardAxiomID: 1, BackwardAxiomID: 2},
	}

	// 换等价类：10 → 11（删一对增一对）
	changes := DiffSymmetricPairs(me, AxiomEquivalentClasses, current, []uint32{11})
	deletes, adds := 0, 0
	for _, c := range changes {
		if c.Op == AxiomOpDelete {
			deletes++
		}
		if c.Op == AxiomOpAdd {
			adds++
		}
	}
	if deletes != 2 || adds != 2 {
		t.Fatalf("等价单值替换应删一对增一对：%+v", changes)
	}

	// 清空（提交 0/空列表）：删一对
	changes = DiffSymmetricPairs(me, AxiomEquivalentClasses, current, nil)
	if len(changes) != 2 {
		t.Fatalf("清空应删一对：%+v", changes)
	}
}

func strPtr(s string) *string { return &s }
