package ontology

import (
	"context"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 02 期属性树单测（02 号文档 §2.3 / §8）：层级算法泛化的属性实例化回归——
// 同一 BuildClassTree 实现吃 30 型边；多父双现、孤立属性成根、自环/深链环检测、防御环忽略。

func propTreeAxiomsToEdges(axioms []*ent.OntAxiom) []HierarchyEdge {
	return CollectHierarchyEdges(axioms, AxiomSubObjectPropertyOf)
}

func TestPropertyTree_MultiParentAndRoots(t *testing.T) {
	// topObjectProperty（虚拟根，前端渲染）
	//   ├─ P（孤立属性 → 根）
	//   └─ A → {B, C}；B → C（C 多父双现）
	entities := []*ent.OntEntity{
		propEntity(1, "P"),
		propEntity(2, "A"),
		propEntity(3, "B"),
		propEntity(4, "C"),
	}
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2), declAxiom(12, 3), declAxiom(13, 4),
		axiomRow(20, AxiomSubObjectPropertyOf, 2, 5, "", "", "", 0), // A → P
		axiomRow(21, AxiomSubObjectPropertyOf, 3, 2, "", "", "", 0), // B → A
		axiomRow(22, AxiomSubObjectPropertyOf, 4, 2, "", "", "", 1), // C → A
		axiomRow(23, AxiomSubObjectPropertyOf, 4, 3, "", "", "", 0), // C → B（多父）
		// 混入 20 型与 31 型公理：建树必须只吃 30 型边
		axiomRow(24, AxiomSubClassOf, 2, 1, "", "", "", 0),
		axiomRow(25, AxiomObjectPropertyDomain, 2, 1, "", "", "", 0),
	}
	// 注：边 20 的父写 id 5（未声明实体）验证"指向未声明实体的边剔除"——A 实际为根
	roots := BuildClassTree(context.Background(), ClassHierarchyInput{
		Entities: entities,
		Edges:    propTreeAxiomsToEdges(axioms),
	}, nil)

	if len(roots) != 2 {
		t.Fatalf("根应为 P 与 A（20/31 型与未声明父边剔除）：%d", len(roots))
	}
	var aNode *TreeNode
	for _, r := range roots {
		if r.ID == 2 {
			aNode = r
		}
	}
	if aNode == nil {
		t.Fatalf("A 应为根：%+v", roots)
	}
	if len(aNode.Children) != 2 {
		t.Fatalf("A 下应有 B、C：%+v", aNode.Children)
	}
	// C 在 A 与 B 下双现（多父不全局去重）
	var bNode *TreeNode
	for _, c := range aNode.Children {
		if c.ID == 3 {
			bNode = c
		}
	}
	if bNode == nil || len(bNode.Children) != 1 || bNode.Children[0].ID != 4 {
		t.Fatalf("B 下应有 C（双现路径一）：%+v", bNode)
	}
	if findNode(roots, 4) == nil {
		t.Fatal("C 应可从根到达（双现路径二）")
	}
}

func TestAssertNoCycle_PropertyEdges(t *testing.T) {
	// 现状：B → A；提交 A → B（深链环）与 P → P（自环）
	edges := []HierarchyEdge{{ChildID: 2, ParentID: 1}}
	names := map[uint32]string{1: "A", 2: "B", 3: "P"}

	if err := AssertNoCycle(1, edges, []uint32{2}, names); err == nil {
		t.Fatal("把子孙设为父属性应拒绝")
	} else if de, ok := err.(*DomainError); !ok || de.Reason != ReasonHierarchyCyclic {
		t.Fatalf("应报 HIERARCHY_CYCLIC：%v", err)
	}

	if err := AssertNoCycle(3, nil, []uint32{3}, names); err == nil {
		t.Fatal("自环应拒绝")
	}

	// 正向：B → A 合法
	if err := AssertNoCycle(2, edges, []uint32{1}, names); err != nil {
		t.Fatalf("合法父子应通过：%v", err)
	}
}

func TestPropertyTree_DirtyCycleDefended(t *testing.T) {
	// 脏数据成环（直改库）：A→B→A；建树跳过环边 + WARN，B 不可达（无根可达）WARN 暴露
	entities := []*ent.OntEntity{propEntity(1, "A"), propEntity(2, "B")}
	axioms := []*ent.OntAxiom{
		axiomRow(20, AxiomSubObjectPropertyOf, 1, 2, "", "", "", 0),
		axiomRow(21, AxiomSubObjectPropertyOf, 2, 1, "", "", "", 0),
	}
	warns := &warnRecorder{}
	roots := BuildClassTree(context.Background(), ClassHierarchyInput{
		Entities: entities,
		Edges:    propTreeAxiomsToEdges(axioms),
	}, warns)
	if len(roots) != 0 {
		t.Fatalf("纯环无根：%+v", roots)
	}
	if len(warns.messages) < 2 {
		t.Fatalf("脏环应产生 WARN（跳过边 + 不可达暴露）：%+v", warns.messages)
	}
}
