package ontology

import (
	"context"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 03 期层级实例化单测（03 号文档 §8）：40 型边建树（多父双现）+ 环检测（自环/深链）。
// BuildClassTree / AssertNoCycle 本体为泛化实现（01 期实现、02 期参数化复用），
// 此处按 03 期参数（entityType=30，edge=40）走一遍实例化回归。

func dataPropertyTreeInput() ClassHierarchyInput {
	entities := []*ent.OntEntity{
		dataPropEntity(1, "P"),
		dataPropEntity(2, "C"),
		dataPropEntity(3, "D"),
		dataPropEntity(4, "E"),
	}
	axioms := []*ent.OntAxiom{
		axiomRow(10, AxiomSubDataPropertyOf, 2, 1, "", "", "", 0), // C ⊑ P
		axiomRow(11, AxiomSubDataPropertyOf, 3, 1, "", "", "", 0), // D ⊑ P
		axiomRow(12, AxiomSubDataPropertyOf, 4, 2, "", "", "", 0), // E ⊑ C
		axiomRow(13, AxiomSubDataPropertyOf, 4, 3, "", "", "", 0), // E ⊑ D（多父：E 双现）
	}
	return ClassHierarchyInput{
		Entities: entities,
		Edges:    CollectHierarchyEdges(axioms, AxiomSubDataPropertyOf),
	}
}

func TestBuildClassTree_DataPropertyMultiParent(t *testing.T) {
	roots := BuildClassTree(context.Background(), dataPropertyTreeInput(), nil)
	if len(roots) != 1 || roots[0].LocalName != "P" {
		t.Fatalf("根应只有 P：%+v", roots)
	}
	p := roots[0]
	if len(p.Children) != 2 {
		t.Fatalf("P 应有 2 个直接子属性：%+v", p.Children)
	}
	// E 多父双现：C 与 D 下各有 E
	foundE := 0
	for _, c := range p.Children {
		for _, e := range c.Children {
			if e.LocalName == "E" {
				foundE++
			}
		}
	}
	if foundE != 2 {
		t.Fatalf("多父 E 应双现：%d", foundE)
	}
	if p.ChildCount != 2 {
		t.Fatalf("P 的真实直接子属性数应为 2：%d", p.ChildCount)
	}
}

func TestAssertNoCycle_DataPropertySelfAndDeepChain(t *testing.T) {
	in := dataPropertyTreeInput()
	// 自环：P 以自己为父
	if err := AssertNoCycle(1, in.Edges, []uint32{1}, nil); err == nil {
		t.Fatal("自环应拒绝")
	}
	// 深链环：把子孙 E 设为 P 的父属性（E 已是 P 的后代：P ⊑ C ⊑ E）
	if err := AssertNoCycle(1, in.Edges, []uint32{4}, nil); err == nil {
		t.Fatal("把子孙设为父属性应拒绝")
	}
	// 合法新边：E 追加祖先 P 为第二父（P 是 E 的祖先，但方向合法——多父扩展）
	if err := AssertNoCycle(4, in.Edges, []uint32{1}, nil); err != nil {
		t.Fatalf("E 追加父 P 不应报环：%v", err)
	}
}
