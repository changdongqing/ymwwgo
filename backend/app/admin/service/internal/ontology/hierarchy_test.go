package ontology

import (
	"context"
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// ---- 测试工具 ----

func entityRow(id, entityType uint32, localName, displayLabel string) *ent.OntEntity {
	e := &ent.OntEntity{ID: id, EntityType: &entityType, LocalName: &localName}
	if displayLabel != "" {
		e.DisplayLabel = &displayLabel
	}
	return e
}

type warnRecorder struct {
	messages []string
}

func (w *warnRecorder) Warnf(_ context.Context, format string, args ...any) {
	w.messages = append(w.messages, strings.TrimSpace(format))
	_ = args
}

// 场景：Thing（虚拟根，前端渲染）→ A → {B, C}；B → D（多继承下 C 也是 D 的父类：菱形）
func buildSampleTree() ClassHierarchyInput {
	entities := []*ent.OntEntity{
		entityRow(1, EntityTypeClass, "A", ""),
		entityRow(2, EntityTypeClass, "B", ""),
		entityRow(3, EntityTypeClass, "C", ""),
		entityRow(4, EntityTypeClass, "D", "迪"),
		entityRow(5, EntityTypeClass, "E", ""), // 孤立类（无父无边）→ 根
	}
	edges := []HierarchyEdge{
		{ChildID: 2, ParentID: 1, SortOrder: 0}, // B → A
		{ChildID: 3, ParentID: 1, SortOrder: 1}, // C → A
		{ChildID: 4, ParentID: 2, SortOrder: 0}, // D → B
		{ChildID: 4, ParentID: 3, SortOrder: 0}, // D → C（多继承）
	}
	return ClassHierarchyInput{Entities: entities, Edges: edges}
}

func findNode(nodes []*TreeNode, id uint32) *TreeNode {
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
		for _, c := range n.Children {
			if found := findNode([]*TreeNode{c}, id); found != nil {
				return found
			}
		}
	}
	return nil
}

func TestBuildClassTree(t *testing.T) {
	in := buildSampleTree()
	roots := BuildClassTree(context.Background(), in, nil)

	// 根 = A、E（B/C/D 均有父）
	if len(roots) != 2 || roots[0].ID != 1 || roots[1].ID != 5 {
		t.Fatalf("根应为 A、E（display_label 排序）：%+v", rootIDs(roots))
	}

	// A 下有 B、C
	nodeA := roots[0]
	if len(nodeA.Children) != 2 || nodeA.Children[0].ID != 2 || nodeA.Children[1].ID != 3 {
		t.Fatalf("A 的子类应为 B、C：%+v", nodeA.Children)
	}

	// 多继承双现：D 同时出现在 B 与 C 下
	nodeB := nodeA.Children[0]
	nodeC := nodeA.Children[1]
	if len(nodeB.Children) != 1 || nodeB.Children[0].ID != 4 {
		t.Fatalf("B 下应有 D：%+v", nodeB.Children)
	}
	if len(nodeC.Children) != 1 || nodeC.Children[0].ID != 4 {
		t.Fatalf("C 下应双现 D（多继承）：%+v", nodeC.Children)
	}

	// 节点字段与 childCount
	if nodeB.ChildCount != 1 || nodeA.ChildCount != 2 {
		t.Fatalf("childCount 错误：A=%d B=%d", nodeA.ChildCount, nodeB.ChildCount)
	}
	if nodeB.Children[0].DisplayLabel != "迪" {
		t.Fatalf("display_label 应透传：%s", nodeB.Children[0].DisplayLabel)
	}
}

func TestBuildClassTree_IgnoresUndeclaredEdges(t *testing.T) {
	in := buildSampleTree()
	// 指向未声明实体（99）的边不参与建树
	in.Edges = append(in.Edges, HierarchyEdge{ChildID: 2, ParentID: 99})
	// 子实体未声明的边同样忽略
	in.Edges = append(in.Edges, HierarchyEdge{ChildID: 98, ParentID: 1})
	roots := BuildClassTree(context.Background(), in, nil)

	if len(roots) != 2 {
		t.Fatalf("未声明实体的边被忽略后根不变：%+v", rootIDs(roots))
	}
	if node := findNode(roots, 2); node == nil {
		t.Fatal("B 应仍在树中")
	}
}

func TestBuildClassTree_DirtyCycleDefended(t *testing.T) {
	// 脏数据环：A → B → A（互为父类，无从根到达）；E 独立
	entities := []*ent.OntEntity{
		entityRow(1, EntityTypeClass, "A", ""),
		entityRow(2, EntityTypeClass, "B", ""),
		entityRow(5, EntityTypeClass, "E", ""),
	}
	edges := []HierarchyEdge{
		{ChildID: 1, ParentID: 2},
		{ChildID: 2, ParentID: 1},
	}
	warns := &warnRecorder{}
	roots := BuildClassTree(context.Background(), ClassHierarchyInput{Entities: entities, Edges: edges}, warns)

	if len(roots) != 1 || roots[0].ID != 5 {
		t.Fatalf("环上实体不可达，根应只剩 E：%+v", rootIDs(roots))
	}
	if len(warns.messages) == 0 {
		t.Fatal("脏环应产生 WARN 日志")
	}
}

func TestPruneNavTree(t *testing.T) {
	in := buildSampleTree()
	// 深层叶子：B 下加无子叶 F（深度 3）
	in.Edges = append(in.Edges, HierarchyEdge{ChildID: 6, ParentID: 2, SortOrder: 1})
	in.Entities = append(in.Entities, entityRow(6, EntityTypeClass, "F", ""))
	// 域入口叶子：A 的直接子类 G（深度 2，无子）——按深度无条件保留
	in.Edges = append(in.Edges, HierarchyEdge{ChildID: 7, ParentID: 1, SortOrder: 2})
	in.Entities = append(in.Entities, entityRow(7, EntityTypeClass, "G", ""))

	roots := PruneNavTree(BuildClassTree(context.Background(), in, nil))

	nodeA := roots[0]
	if len(nodeA.Children) != 3 {
		t.Fatalf("深度 2 的域入口叶子 G 应无条件保留：%+v", childIDs(nodeA))
	}
	nodeB := findNode(roots, 2)
	// B 的 children：D、F 均为深度 3 无子叶子 → 剪除
	if len(nodeB.Children) != 0 {
		t.Fatalf("深度 3 的叶子 D/F 应被剪除：%+v", childIDs(nodeB))
	}
	// 但 childCount 保留真实值：B 的真实直接子类 = D、F = 2
	if nodeB.ChildCount != 2 {
		t.Fatalf("childCount 应保留真实值 2（+N 角标依据）：%d", nodeB.ChildCount)
	}
	// 深度 2 的 C 无条件保留（其子 D 被剪）
	nodeC := findNode(roots, 3)
	if nodeC == nil || len(nodeC.Children) != 0 || nodeC.ChildCount != 1 {
		t.Fatalf("深度 2 的 C 应保留且 childCount=1：%+v", nodeC)
	}
	// 全树任何位置都不应再有 D（深度 3 叶子）
	if findNode(roots, 4) != nil {
		t.Fatal("D（深度 3 叶子）应被剪除")
	}
}

func TestPruneNavTree_KeepsDeepBranchNodes(t *testing.T) {
	// 深层分支：A(1) → B(2) → D(4) → D2(8)；D 有真实子类 → D 保留为导航入口
	//（其叶子 D2 被剪，体现为 ChildCount=1 与空 children 的 +N 差值），
	// 而无子的深层叶子 F(6) 整个剪掉。验收 #7"在叶子下新建类后该叶子升格进骨架树"。
	in := buildSampleTree()
	in.Edges = append(in.Edges,
		HierarchyEdge{ChildID: 6, ParentID: 2, SortOrder: 1}, // F → B（深度 3 叶子）
		HierarchyEdge{ChildID: 8, ParentID: 4, SortOrder: 0}, // D2 → D
	)
	in.Entities = append(in.Entities,
		entityRow(6, EntityTypeClass, "F", ""),
		entityRow(8, EntityTypeClass, "D2", ""),
	)

	roots := PruneNavTree(BuildClassTree(context.Background(), in, nil))

	// B 的 children：F 剪除、D（深层分支，有真实子类）保留
	nodeB := findNode(roots, 2)
	if nodeB == nil || len(nodeB.Children) != 1 || nodeB.Children[0].ID != 4 || nodeB.ChildCount != 2 {
		t.Fatalf("B 应保留且仅保留分支子 D（childCount=2）：%+v", nodeB)
	}
	// D 是深层分支（有真实子类）→ 保留为导航入口，叶子 D2 剪除
	nodeD := findNode(roots, 4)
	if nodeD == nil {
		t.Fatal("深层分支 D（有真实子类）应保留在骨架树中")
	}
	if len(nodeD.Children) != 0 || nodeD.ChildCount != 1 {
		t.Fatalf("D 应 children 空、childCount=1（+N 角标）：%+v", nodeD)
	}
	if findNode(roots, 8) != nil {
		t.Fatal("D2（深度 4 叶子）应被剪除")
	}
	if findNode(roots, 6) != nil {
		t.Fatal("F（深度 3 叶子）应被剪除")
	}
}

func TestSubtreeIDs(t *testing.T) {
	in := buildSampleTree()
	edges := in.Edges

	// B 的子树闭包 = {B, D}（含自身）
	ids := SubtreeIDs(2, edges)
	if len(ids) != 2 || !containsUint(ids, 2) || !containsUint(ids, 4) {
		t.Fatalf("B 的子树闭包应为 {B, D}：%v", ids)
	}

	// A 的子树闭包 = {A, B, C, D}，菱形 D 去重
	ids = SubtreeIDs(1, edges)
	if len(ids) != 4 {
		t.Fatalf("A 的子树闭包应为 {A,B,C,D}（菱形去重）：%v", ids)
	}

	// 根子树含全部
	ids = SubtreeIDs(5, edges)
	if len(ids) != 1 {
		t.Fatalf("孤立类 E 的子树应只含自身：%v", ids)
	}
}

func TestSubtreeIDs_DirtyCycle(t *testing.T) {
	// 脏环：X → Y → X，从 X 出发不发散
	edges := []HierarchyEdge{
		{ChildID: 1, ParentID: 2},
		{ChildID: 2, ParentID: 1},
		{ChildID: 2, ParentID: 3},
	}
	ids := SubtreeIDs(3, edges)
	if len(ids) != 3 || !containsUint(ids, 1) || !containsUint(ids, 2) {
		t.Fatalf("脏环下闭包应去重为 {3,2,1}：%v", ids)
	}
}

func TestAssertNoCycle(t *testing.T) {
	// 现状：A(1) → 无父；B(2) → A；C(3) → B（深链 A ← B ← C）
	edges := []HierarchyEdge{
		{ChildID: 2, ParentID: 1},
		{ChildID: 3, ParentID: 2},
	}
	names := map[uint32]string{1: "A", 2: "B", 3: "C", 4: "D"}

	// 合法：把 D 挂到 C 下（D 无环）
	if err := AssertNoCycle(4, edges, []uint32{3}, names); err != nil {
		t.Fatalf("合法挂载不应报错：%v", err)
	}

	// 深链成环：把 A 挂到 C 下（A 已是 C 的祖先）
	err := AssertNoCycle(1, edges, []uint32{3}, names)
	if err == nil {
		t.Fatal("深链成环应被拒绝")
	}
	de := err.(*DomainError)
	if de.Reason != ReasonHierarchyCyclic {
		t.Fatalf("reason 应为 HIERARCHY_CYCLIC：%v", err)
	}
	if !strings.Contains(de.Message, "A 已是 C 的祖先") {
		t.Fatalf("报错应指明祖先关系：%s", de.Message)
	}
	if !strings.Contains(de.Message, "C → B → A") {
		t.Fatalf("报错应含环路径定位：%s", de.Message)
	}

	// 自环：A 以自己为父类
	err = AssertNoCycle(1, edges, []uint32{1}, names)
	if err == nil || !strings.Contains(err.Error(), "不能以自己为父类") {
		t.Fatalf("自环应被捕获：%v", err)
	}

	// 菱形多继承下的环：B 已挂 A；把 A 挂到 B 下（经 B 的另一父链）也应被捕获
	edges = append(edges, HierarchyEdge{ChildID: 2, ParentID: 4}) // B 多父：A、D
	err = AssertNoCycle(4, edges, []uint32{2}, names)
	if err == nil || !strings.Contains(err.Error(), "D 已是 B 的祖先") {
		t.Fatalf("多继承环应被捕获：%v", err)
	}

	// create 场景：targetID=0（新实体无任何入边）永不成环
	if err := AssertNoCycle(0, edges, []uint32{3}, nil); err != nil {
		t.Fatalf("create 场景不应报环：%v", err)
	}
}

func rootIDs(nodes []*TreeNode) []uint32 {
	out := make([]uint32, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.ID)
	}
	return out
}

func childIDs(n *TreeNode) []uint32 {
	out := make([]uint32, 0, len(n.Children))
	for _, c := range n.Children {
		out = append(out, c.ID)
	}
	return out
}

func containsUint(ids []uint32, v uint32) bool {
	for _, id := range ids {
		if id == v {
			return true
		}
	}
	return false
}
