package ontology

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// HierarchyEdge 层级边（20 型公理的建树投影）
type HierarchyEdge struct {
	ChildID   uint32 // subject（子类）
	ParentID  uint32 // object（父类）
	SortOrder int32
}

// TreeNode 层级树节点（proto OntologyClassTreeNode 的域内核同构体）
type TreeNode struct {
	ID           uint32
	IRI          string
	LocalName    string
	DisplayLabel string
	Deprecated   bool
	ChildCount   uint32 // 真实直接子类数（骨架树被剪枝时与 len(Children) 存在差值，前端据此渲染 +N）
	Children     []*TreeNode
}

// ClassHierarchyInput 层级构建输入：实体与边的全量投影（读路径一次查询，与源
// AssertedClassHierarchyProvider 同构）。
type ClassHierarchyInput struct {
	Entities []*ent.OntEntity // 未删、entity_type=CLASS
	Edges    []HierarchyEdge  // 未删 20 型公理（指向未声明实体的边在收集侧剔除）
}

// BuildClassTree 全量树构建（设计文档 §4.3；算法与实体类型无关——输入为实体与边的投影，
// 02 期属性树以 entityType=20 的实体集 + 30 型边复用同一实现，函数名沿用 01 期）：
//   - childMap = edges GROUP BY parent，按 sort_order、display_label、id 排序；
//   - roots = 无父类边的实体（孤立类成根）；
//   - 虚拟根 owl:Thing / owl:topObjectProperty 不入库：服务端返回根列表，虚拟根节点由前端固定渲染；
//   - 脏数据成环（历史/手工改库）防御：递归携带 path 集合，成环边跳过 + WARN。
func BuildClassTree(ctx context.Context, in ClassHierarchyInput, log Logger) []*TreeNode {
	byID := make(map[uint32]*ent.OntEntity, len(in.Entities))
	for _, e := range in.Entities {
		byID[e.ID] = e
	}

	// 指向未声明/外部实体的边不参与建树（仍可过校验）；子实体同理必须已声明。
	// 父内子列表按边 sort_order、子 display_label、子 id 稳定排序（多继承同一子只随首条边出现）。
	type childEdge struct {
		childID   uint32
		sortOrder int32
	}
	childMap := make(map[uint32][]childEdge, len(in.Entities))
	hasParent := make(map[uint32]struct{}, len(in.Entities))
	for _, e := range in.Edges {
		if _, ok := byID[e.ChildID]; !ok {
			continue
		}
		if _, ok := byID[e.ParentID]; !ok {
			continue
		}
		childMap[e.ParentID] = append(childMap[e.ParentID], childEdge{childID: e.ChildID, sortOrder: e.SortOrder})
		hasParent[e.ChildID] = struct{}{}
	}
	for parent := range childMap {
		children := childMap[parent]
		keys := make(map[uint32]struct {
			label string
			id    uint32
		}, len(children))
		for _, c := range children {
			keys[c.childID] = struct {
				label string
				id    uint32
			}{label: displayLabelOf(byID[c.childID]), id: c.childID}
		}
		sort.Slice(children, func(i, j int) bool {
			a, b := children[i], children[j]
			if a.sortOrder != b.sortOrder {
				return a.sortOrder < b.sortOrder
			}
			ka, kb := keys[a.childID], keys[b.childID]
			if ka.label != kb.label {
				return ka.label < kb.label
			}
			return ka.id < kb.id
		})
		childMap[parent] = children
	}

	emitted := make(map[uint32]struct{}, len(in.Entities))
	// 多继承下同一实体在树中多现（路径复合 key 由前端承载），不做全局去重；
	// 脏环防御只在单条祖先链（path）内判断。
	var build func(id uint32, path map[uint32]struct{}) *TreeNode
	build = func(id uint32, path map[uint32]struct{}) *TreeNode {
		if _, inPath := path[id]; inPath {
			// 理论已被写路径环检测禁止；脏数据出现环则跳过该边并 WARN
			warnf(log, ctx, "ontology hierarchy: 脏数据成环，跳过实体 %d 所在的环边", id)
			return nil
		}
		path[id] = struct{}{}
		defer delete(path, id)

		e := byID[id]
		node := &TreeNode{
			ID:           e.ID,
			IRI:          nilString(e.Iri),
			LocalName:    nilString(e.LocalName),
			DisplayLabel: nilString(e.DisplayLabel),
			Deprecated:   nilBool(e.Deprecated),
			ChildCount:   uint32(len(childMap[id])), // 真实直接子类数（骨架树剪枝后与 len(Children) 的差值即 +N）
		}
		emitted[id] = struct{}{}
		for _, childEdge := range childMap[id] {
			if child := build(childEdge.childID, path); child != nil {
				node.Children = append(node.Children, child)
			}
		}
		return node
	}

	// 根列表确定性排序（display_label → id），与父内子列表排序语义一致
	roots := make([]*TreeNode, 0, len(in.Entities))
	for _, e := range in.Entities {
		if _, ok := hasParent[e.ID]; ok {
			continue
		}
		if node := build(e.ID, map[uint32]struct{}{}); node != nil {
			roots = append(roots, node)
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].DisplayLabel != roots[j].DisplayLabel {
			return roots[i].DisplayLabel < roots[j].DisplayLabel
		}
		return roots[i].ID < roots[j].ID
	})
	// 脏环可达性防御：成环的实体没有根（互为祖先）时从任何根都走不到，逐个 WARN 暴露
	for _, e := range in.Entities {
		if _, ok := emitted[e.ID]; !ok {
			warnf(log, ctx, "ontology hierarchy: 实体 %d 位于无法从根到达的脏环中，未出现在树里", e.ID)
		}
	}
	return roots
}

// PruneNavTree 骨架树剪枝（源修订 #15 内化为 V1 必做）：
// keepInNav(node, depth) = depth <= NavKeepDepth || 该节点有真实直接子类——
// 深度 1（根类）、2（域入口）内无条件保留；更深的"有父类且无子类"叶子剪掉。
// 有真实子类的深层节点是导航入口，保留但其叶子子类被剪（体现为 ChildCount 与 len(Children)
// 的差值，前端 +N 角标）——判定用剪枝前的真实子类数（ChildCount），不能用剪枝后的
// len(Children)，否则深层分支会被整枝折叠、违背"叶子升格"交互（验收 #7）。
func PruneNavTree(roots []*TreeNode) []*TreeNode {
	var prune func(node *TreeNode, depth int) *TreeNode
	prune = func(node *TreeNode, depth int) *TreeNode {
		kept := make([]*TreeNode, 0, len(node.Children))
		for _, child := range node.Children {
			if c := prune(child, depth+1); c != nil {
				kept = append(kept, c)
			}
		}
		node.Children = kept
		if depth > NavKeepDepth && node.ChildCount == 0 {
			return nil
		}
		return node
	}

	out := make([]*TreeNode, 0, len(roots))
	for _, root := range roots {
		if node := prune(root, 1); node != nil {
			out = append(out, node)
		}
	}
	return out
}

// SubtreeIDs 子树闭包（分页"仅看子树"用）：迭代 DFS 沿 20 型边收集后代闭包（含自身），
// visited 去重防脏环。服务端计算，前端只传 rootId 一个参数。
func SubtreeIDs(rootID uint32, edges []HierarchyEdge) []uint32 {
	childMap := make(map[uint32][]uint32, len(edges))
	for _, e := range edges {
		childMap[e.ParentID] = append(childMap[e.ParentID], e.ChildID)
	}

	visited := map[uint32]struct{}{rootID: {}}
	stack := []uint32{rootID}
	out := make([]uint32, 0, len(visited))
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, cur)
		for _, child := range childMap[cur] {
			if _, seen := visited[child]; !seen {
				visited[child] = struct{}{}
				stack = append(stack, child)
			}
		}
	}
	return out
}

// AssertNoCycle 写路径前置环检测（独立于校验器，与源分工一致——源也是编译器外独立 assertNoCycle）：
// 构建 子->父 邻接表（当前未删 20 型边 + 本次新增），从每个目标父沿父链向上走（记录起点供报错定位），
// 回到 targetID 即拒绝 HIERARCHY_CYCLIC；自环 targetID→targetID 同样被捕获。
// entityNames 供报错展示（id → localName/displayLabel），可为 nil。
func AssertNoCycle(targetID uint32, parentEdges []HierarchyEdge, targetParentIDs []uint32, entityNames map[uint32]string) error {
	name := func(id uint32) string {
		if n, ok := entityNames[id]; ok && n != "" {
			return n
		}
		return strconv.FormatUint(uint64(id), 10)
	}

	parentOf := make(map[uint32][]uint32, len(parentEdges)+len(targetParentIDs))
	for _, e := range parentEdges {
		parentOf[e.ChildID] = append(parentOf[e.ChildID], e.ParentID)
	}
	// 本次新增的目标父集（全量新父集；与现存边重叠时重复邻接无碍，只判可达性）
	for _, p := range targetParentIDs {
		parentOf[targetID] = append(parentOf[targetID], p)
	}

	for _, origin := range targetParentIDs {
		// 自环：target 以自己为父类
		if origin == targetID {
			return NewError(ReasonHierarchyCyclic, "不允许形成继承环：%s 不能以自己为父类", name(targetID))
		}

		// 从 origin 沿父链向上走；per-origin visited 防脏数据既有环导致的死循环
		if path, cyclic := ancestorPath(origin, targetID, parentOf, map[uint32]struct{}{}); cyclic {
			// path = [origin, ..., target]：target 已是 origin 的祖先
			rendered := make([]string, 0, len(path))
			for _, id := range path {
				rendered = append(rendered, name(id))
			}
			return NewError(ReasonHierarchyCyclic,
				"不允许形成继承环：%s 已是 %s 的祖先（%s）",
				name(targetID), name(origin), strings.Join(rendered, " → "))
		}
	}
	return nil
}

// ancestorPath 从 cur 沿父链向上找 target，返回 [cur, ..., target] 路径（找到时）。
func ancestorPath(cur, target uint32, parentOf map[uint32][]uint32, visited map[uint32]struct{}) ([]uint32, bool) {
	if cur == target {
		return []uint32{cur}, true
	}
	if _, seen := visited[cur]; seen {
		return nil, false
	}
	visited[cur] = struct{}{}
	for _, p := range parentOf[cur] {
		if sub, ok := ancestorPath(p, target, parentOf, visited); ok {
			return append([]uint32{cur}, sub...), true
		}
	}
	return nil, false
}

// CollectHierarchyEdges 从公理行投影建树边（只取未删的指定层级边型：类树 20 / 属性树 30；
// subject/object 任一缺失跳过）。01 期实现硬编码 20 型，02 期参数化（02 号文档 §2.3）。
func CollectHierarchyEdges(axioms []*ent.OntAxiom, edgeType uint32) []HierarchyEdge {
	edges := make([]HierarchyEdge, 0, len(axioms))
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != edgeType {
			continue
		}
		if ax.SubjectID == nil || ax.ObjectID == nil {
			continue
		}
		edges = append(edges, HierarchyEdge{
			ChildID:   *ax.SubjectID,
			ParentID:  *ax.ObjectID,
			SortOrder: nilInt32(ax.SortOrder),
		})
	}
	return edges
}

// sortChildIDs 已内联进 BuildClassTree（排序键=边 sort_order → 子 display_label → 子 id）。

func displayLabelOf(e *ent.OntEntity) string {
	if e == nil || e.DisplayLabel == nil || *e.DisplayLabel == "" {
		if e != nil && e.LocalName != nil {
			return *e.LocalName
		}
		return ""
	}
	return *e.DisplayLabel
}

func nilString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nilBool(p *bool) bool {
	return p != nil && *p
}

func nilInt32(p *uint32) int32 {
	if p == nil {
		return 0
	}
	return int32(*p)
}
