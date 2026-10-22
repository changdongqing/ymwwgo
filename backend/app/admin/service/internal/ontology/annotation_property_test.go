package ontology

import (
	"context"
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 04 期注解属性域内核单测（04 号文档 §8 清单）：
// 层级（50 边多父/环）、自定义注解归一化与 diff（含同属性多值、sort_order 顺排、空数组清空）、
// 校验器 50 规则 + 100 双轨四形态、渲染双轨、存量三服务回归（annotations diff 对类/属性主语通用）、
// 显示名留缝（DisplayLabelPredicates 行为零变化）。

// ---- 层级：50 边复用 BuildClassTree（entityType=40 + edge=50 实例化） ----

func annotationPropertyRow(id uint32, localName string) *ent.OntEntity {
	iri := testBaseIRI + localName
	e := entityRow(id, EntityTypeAnnotationProperty, localName, "")
	e.Iri = &iri
	return e
}

func TestHierarchy_AnnotationPropertyMultiParent(t *testing.T) {
	entities := []*ent.OntEntity{
		annotationPropertyRow(1, "formerNotation"),
		annotationPropertyRow(2, "source"),
		annotationPropertyRow(3, "alias"),
	}
	edges := CollectHierarchyEdges([]*ent.OntAxiom{
		axiomRow(10, AxiomSubAnnotationPropertyOf, 3, 1, "", "", "", 0),
		axiomRow(11, AxiomSubAnnotationPropertyOf, 3, 2, "", "", "", 1),
	}, AxiomSubAnnotationPropertyOf)

	roots := BuildClassTree(context.Background(), ClassHierarchyInput{Entities: entities, Edges: edges}, nil)
	if len(roots) != 2 {
		t.Fatalf("双父属性下 alias 不成根，根数应 2：%d", len(roots))
	}
	for _, root := range roots {
		if root.ID == 3 {
			t.Fatalf("alias(3) 有父属性不应成根")
		}
		if root.ID == 1 && root.ChildCount != 1 {
			t.Fatalf("formerNotation 直接子属性数应 1：%d", root.ChildCount)
		}
	}
}

func TestHierarchy_AnnotationPropertyCycleRejected(t *testing.T) {
	// 现存边：2 的父属性是 1（child=2, parent=1）
	edges := CollectHierarchyEdges([]*ent.OntAxiom{
		axiomRow(10, AxiomSubAnnotationPropertyOf, 2, 1, "", "", "", 0),
	}, AxiomSubAnnotationPropertyOf)

	// 现存边：2 的父属性是 1（child=2, parent=1）；把祖先(1)设为 2 的父属性目标时，
	// 检测 target=1 以 2 为父 → 2 又以 1 为父 → 环；自环 1→1 同样拒绝
	if err := AssertNoCycle(1, edges, []uint32{2}, nil); err == nil {
		t.Fatalf("把子孙属性设为父属性应拒绝")
	} else if !strings.Contains(err.Error(), "环") {
		t.Fatalf("环错误文案应含「环」：%v", err)
	}
	if err := AssertNoCycle(1, edges, []uint32{1}, nil); err == nil {
		t.Fatalf("自环应拒绝")
	}
}

// ---- NormalizeCustomAnnotationInputs：值空白拒绝 / lang 归一 / 三元组有序去重 ----

func TestNormalizeCustomAnnotationInputs(t *testing.T) {
	// 值空白（trim 后空串）拒绝
	if _, err := NormalizeCustomAnnotationInputs([]CustomAnnotationInput{{PropertyID: 7, Value: "   "}}); err == nil {
		t.Fatalf("空白值应拒绝")
	} else if de, ok := err.(*DomainError); !ok || de.Reason != ReasonAnnotationValueInvalid {
		t.Fatalf("应报 ANNOTATION_VALUE_INVALID：%v", err)
	}

	// property_id=0 的行丢弃（前端空行防御）
	inputs, err := NormalizeCustomAnnotationInputs([]CustomAnnotationInput{
		{PropertyID: 0, Value: "x"},
		{PropertyID: 7, Value: " A01 ", Lang: " zh "},
	})
	if err != nil {
		t.Fatalf("合法输入不应报错：%v", err)
	}
	if len(inputs) != 1 || inputs[0].Value != "A01" || inputs[0].Lang != "zh" {
		t.Fatalf("应 trim 并丢弃空谓词行：%+v", inputs)
	}

	// 重复行（property_id, value, lang 三元组相同）去重，只保留一行
	inputs, err = NormalizeCustomAnnotationInputs([]CustomAnnotationInput{
		{PropertyID: 7, Value: "A01"},
		{PropertyID: 7, Value: "A01", Lang: ""},
		{PropertyID: 7, Value: "B02"},
	})
	if err != nil {
		t.Fatalf("去重不应报错：%v", err)
	}
	if len(inputs) != 2 || inputs[0].Value != "A01" || inputs[1].Value != "B02" {
		t.Fatalf("重复三元组应有序去重为两行：%+v", inputs)
	}
}

// ---- AppendCustomAnnotationAxioms：create 侧（新实体引用桥接 + sort_order 从 0 顺排） ----

func TestAppendCustomAnnotationAxioms(t *testing.T) {
	set := &EntityChangeSet{}
	AppendCustomAnnotationAxioms(set, 1, 0, []CustomAnnotationInput{
		{PropertyID: 7, Value: "A01"},
		{PropertyID: 7, Value: "B02", Lang: "en"},
	})
	if len(set.Changes) != 2 {
		t.Fatalf("应产出两行 ADD：%d", len(set.Changes))
	}
	c := set.Changes[0]
	if c.Op != AxiomOpAdd || c.AxiomType != AxiomAnnotationAssertion || c.SubjectNewEntityRef != 1 {
		t.Fatalf("首行应为引用新主语的 100 型 ADD：%+v", c)
	}
	if c.ObjectID != 7 || c.ValueText != "A01" || c.SortOrder != 0 || c.PredicateIRI != "" {
		t.Fatalf("自定义行应 object_id=谓词/predicate_iri 空/sort_order=0：%+v", c)
	}
	if set.Changes[1].Lang != "en" || set.Changes[1].SortOrder != 1 {
		t.Fatalf("第二行 lang/sort_order 不符：%+v", set.Changes[1])
	}
}

// ---- DiffCustomAnnotations：增删改 / 同属性多值 / sort_order 顺排 / 空数组清空 ----

func TestDiffCustomAnnotations(t *testing.T) {
	const me = 100
	// 现状：属性 7 两条值（sort 0/1）、属性 8 一条值（sort 2）
	current := []*ent.OntAxiom{
		axiomRow(1, AxiomAnnotationAssertion, me, 7, "", "A01", "", 0),
		axiomRow(2, AxiomAnnotationAssertion, me, 7, "", "B02", "en", 1),
		axiomRow(3, AxiomAnnotationAssertion, me, 8, "", "C03", "", 2),
		// 内建谓词行不参与本 diff
		axiomRow(4, AxiomAnnotationAssertion, me, 0, PredicateRDFSLabel, "名称", "zh", 3),
	}

	// 无变化 → 零变更
	changes := DiffCustomAnnotations(me, current, []CustomAnnotationInput{
		{PropertyID: 7, Value: "A01"},
		{PropertyID: 7, Value: "B02", Lang: "en"},
		{PropertyID: 8, Value: "C03"},
	})
	if len(changes) != 0 {
		t.Fatalf("无变化应为零变更：%+v", changes)
	}

	// 删（属性 7 的 A01 行，不在目标集）+ 增（属性 8 的 D04 新值，sort 从存量最大 2 后顺排）；
	// (7,B02,en) 与 (8,C03) 键命中存量 → 保留不动
	changes = DiffCustomAnnotations(me, current, []CustomAnnotationInput{
		{PropertyID: 7, Value: "B02", Lang: "en"},
		{PropertyID: 8, Value: "C03"},
		{PropertyID: 8, Value: "D04"},
	})
	if len(changes) != 2 {
		t.Fatalf("应删 1 增 1：%+v", changes)
	}
	if changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 1 {
		t.Fatalf("应删 A01 行：%+v", changes[0])
	}
	add := changes[1]
	if add.Op != AxiomOpAdd || add.ObjectID != 8 || add.ValueText != "D04" || add.SortOrder != 3 {
		t.Fatalf("新增行应从存量最大 sort_order 后顺排（3）：%+v", add)
	}

	// 空数组 = 清空全部自定义注解（全量替换语义）；内建谓词行不动
	changes = DiffCustomAnnotations(me, current, nil)
	if len(changes) != 3 {
		t.Fatalf("清空应删 3 行自定义注解：%d", len(changes))
	}
	for _, c := range changes {
		if c.Op != AxiomOpDelete || c.AxiomID == 4 {
			t.Fatalf("清空只删 object_id 路径行，内建谓词行不动：%+v", changes)
		}
	}
}

// ---- 校验器：50 规则 + 100 双轨四形态 ----

func TestValidator_SubAnnotationPropertyOf(t *testing.T) {
	entities := []*ent.OntEntity{
		annotationPropertyRow(1, "p1"), annotationPropertyRow(2, "p2"),
		classEntity(3, "C"),
	}
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2), declAxiom(12, 3),
		axiomRow(20, AxiomSubAnnotationPropertyOf, 2, 1, "", "", "", 0),
	}
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("合法 50 边不应有问题：%v", problems)
	}

	// 宾语是类 → 类型不符
	problems := validate(t, entities, []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(12, 3),
		axiomRow(21, AxiomSubAnnotationPropertyOf, 1, 3, "", "", "", 0),
	})
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "期望注解属性") {
		t.Fatalf("50 边宾语须为注解属性：%v", problems)
	}
}

func TestValidator_AnnotationAssertionDualTrack(t *testing.T) {
	entities := []*ent.OntEntity{
		classEntity(1, "A"),
		annotationPropertyRow(2, "formerNotation"),
		classEntity(3, "B"), // 非注解属性实体（object_id 指向它须拒绝）
	}
	decls := []*ent.OntAxiom{declAxiom(10, 1), declAxiom(11, 2), declAxiom(12, 3)}

	// 形态一：内建谓词行（存量路径）通过
	problems := validate(t, entities, append(decls,
		axiomRow(20, AxiomAnnotationAssertion, 1, 0, PredicateRDFSLabel, "类A", "zh", 0)))
	if len(problems) != 0 {
		t.Fatalf("内建谓词行应通过：%v", problems)
	}

	// 形态二：自定义谓词行（object_id → 注解属性实体）通过
	problems = validate(t, entities, append(decls,
		axiomRow(21, AxiomAnnotationAssertion, 1, 2, "", "A01", "", 0)))
	if len(problems) != 0 {
		t.Fatalf("自定义谓词行应通过：%v", problems)
	}

	// 形态三：object_id 指向非注解属性实体 → 拒绝
	problems = validate(t, entities, append(decls,
		axiomRow(22, AxiomAnnotationAssertion, 1, 3, "", "x", "", 0)))
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "注解谓词必须是注解属性实体") {
		t.Fatalf("object_id 指向非注解属性应拒绝：%v", problems)
	}

	// 形态三变体：object_id 指向不存在/已删实体 → 拒绝
	problems = validate(t, entities, append(decls,
		axiomRow(23, AxiomAnnotationAssertion, 1, 99, "", "x", "", 0)))
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "不存在或已删除") {
		t.Fatalf("object_id 指向不存在实体应拒绝：%v", problems)
	}

	// 形态四：object_id 与 predicate_iri 皆空 → 拒绝
	problems = validate(t, entities, append(decls,
		axiomRow(24, AxiomAnnotationAssertion, 1, 0, "", "x", "", 0)))
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "不在白名单内") {
		t.Fatalf("谓词双空应拒绝：%v", problems)
	}

	// 自定义行值空白 → 拒绝
	problems = validate(t, entities, append(decls,
		axiomRow(25, AxiomAnnotationAssertion, 1, 2, "", "  ", "", 0)))
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "注解值为空白") {
		t.Fatalf("空白值应拒绝：%v", problems)
	}
}

// ---- 渲染：50 分支 + 100 双轨 ----

func TestRender_AnnotationPropertyAxioms(t *testing.T) {
	names := map[uint32]string{1: "formerNotation", 2: "alias", 3: "动物"}

	// 50：SubPropertyOf(alias formerNotation)
	ax := axiomRow(1, AxiomSubAnnotationPropertyOf, 2, 1, "", "", "", 0)
	if got := RenderAxiom(ax, names); got != "SubPropertyOf(alias formerNotation)" {
		t.Fatalf("50 渲染不符：%s", got)
	}

	// 100 内建轨：行为零变化
	ax = axiomRow(2, AxiomAnnotationAssertion, 3, 0, PredicateRDFSLabel, "张三", "zh", 0)
	if got := RenderAxiom(ax, names); got != `Annotation(rdfs:label 动物 "张三"@zh)` {
		t.Fatalf("100 内建轨渲染不符：%s", got)
	}

	// 100 自定义轨：谓词渲染为注解属性实体名
	ax = axiomRow(3, AxiomAnnotationAssertion, 3, 1, "", "A01", "", 0)
	if got := RenderAxiom(ax, names); got != `Annotation(formerNotation 动物 "A01")` {
		t.Fatalf("100 自定义轨渲染不符：%s", got)
	}
}

// ---- 存量三服务回归：自定义注解 diff 对类/对象属性/数据属性主语通用（横切能力统一断言，
// 不改 01~03 期既有测试文件——源修订 #2 内化） ----

func TestDiffCustomAnnotations_LegacySubjects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		subj   uint32
		types  []uint32
		entity *ent.OntEntity
	}{
		{"class", 100, []uint32{AxiomDeclaration}, entityRow(100, EntityTypeClass, "A", "")},
		{"objectProperty", 200, []uint32{AxiomDeclaration}, entityRow(200, EntityTypeObjectProperty, "P", "")},
		{"dataProperty", 300, []uint32{AxiomDeclaration}, entityRow(300, EntityTypeDataProperty, "D", "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := []*ent.OntAxiom{
				axiomRow(1, AxiomAnnotationAssertion, tc.subj, 7, "", "old", "", 0),
				axiomRow(2, AxiomAnnotationAssertion, tc.subj, 0, PredicateRDFSLabel, "x", "zh", 1),
			}
			changes := DiffCustomAnnotations(tc.subj, current, []CustomAnnotationInput{{PropertyID: 7, Value: "new"}})
			if len(changes) != 2 {
				t.Fatalf("应删旧值行 + 增新值行：%+v", changes)
			}
			if changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 1 {
				t.Fatalf("应删旧自定义行（内建行不动）：%+v", changes[0])
			}
			add := changes[1]
			if add.Op != AxiomOpAdd || add.SubjectID != tc.subj || add.ObjectID != 7 || add.ValueText != "new" {
				t.Fatalf("新增行字段不符：%+v", add)
			}
		})
	}
}

// ---- 显示名留缝：DisplayLabelPredicates 行为零变化（自定义注解行结构上不参与渲染缓存） ----

func TestPickDisplayLabel_CustomRowsIgnored(t *testing.T) {
	axioms := []*ent.OntAxiom{
		axiomRow(1, AxiomAnnotationAssertion, 1, 7, "", "自定义值", "", 0), // object_id 路径行
		axiomRow(2, AxiomAnnotationAssertion, 1, 0, PredicateRDFSLabel, "label值", "zh", 1),
	}
	if got := PickDisplayLabel(axioms); got == nil || *got != "label值" {
		t.Fatalf("显示名只认 predicate_iri 路径的 rdfs:label：%v", got)
	}
	if got := PickDisplayLabel([]*ent.OntAxiom{
		axiomRow(1, AxiomAnnotationAssertion, 1, 7, "", "自定义值", "", 0),
	}); got != nil {
		t.Fatalf("无内建 label 行应取 nil（自定义行不顶替）：%v", *got)
	}
}
