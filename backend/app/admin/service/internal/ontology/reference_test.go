package ontology

import (
	"context"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 08 期通用反查域内核单测：默认集矩阵、成对去重、37 链成员匹配、排序分页、参数解析。
// 索引/分页 SQL 行为依赖真实 PostgreSQL，转端到端验收（铁律 4 禁内存库）。

// ---- 号段矩阵（08 号文档 §3.2：默认集逐行钉死，防矩阵漂移） ----

func TestDefaultReferenceAxiomTypes_Matrix(t *testing.T) {
	// §3.2 反查可用性矩阵的 ✓ 行（排除 33/43/46、42 内建轨与 100-IRI 轨）
	expected := []uint32{
		AxiomSubClassOf, AxiomDisjointWith, AxiomEquivalentClasses,
		AxiomSubObjectPropertyOf, AxiomObjectPropertyDomain, AxiomObjectPropertyRange,
		AxiomInverseOf, AxiomEquivalentObjectProperties, AxiomDisjointObjectProperties,
		AxiomSubPropertyChainOf,
		AxiomSubDataPropertyOf, AxiomDataPropertyDomain, AxiomDataPropertyRange,
		AxiomEquivalentDataProperties, AxiomDisjointDataProperties,
		AxiomSubAnnotationPropertyOf,
		AxiomTypeAssertion, AxiomObjectPropertyAssertion, AxiomDataPropertyAssertion,
		AxiomAnnotationAssertion,
	}
	if len(DefaultReferenceAxiomTypes) != len(expected) {
		t.Fatalf("默认集长度漂移：got %d want %d", len(DefaultReferenceAxiomTypes), len(expected))
	}
	for i, want := range expected {
		if DefaultReferenceAxiomTypes[i] != want {
			t.Fatalf("默认集第 %d 项漂移：got %d want %d（完整：%v）", i, DefaultReferenceAxiomTypes[i], want, DefaultReferenceAxiomTypes)
		}
	}
	// 显式排除项不得进默认集
	for _, excluded := range []uint32{
		AxiomDeclaration, AxiomObjectPropertyCharacteristic, AxiomDataPropertyCharacteristic,
		AxiomDatatypeDefinition,
	} {
		if HasAxiomType(DefaultReferenceAxiomTypes, excluded) {
			t.Fatalf("默认集不应包含 %d", excluded)
		}
	}
}

func TestAxiomTypeSets_Consistent(t *testing.T) {
	for _, set := range [][]uint32{DefaultReferenceAxiomTypes, PairedAxiomTypes, PredicateReferenceAxiomTypes} {
		for _, v := range set {
			if !IsKnownAxiomType(v) {
				t.Fatalf("集合含未登记号段 %d", v)
			}
		}
	}
	// 成对号段必须都在默认集内（默认请求就要去重）
	for _, v := range PairedAxiomTypes {
		if !HasAxiomType(DefaultReferenceAxiomTypes, v) {
			t.Fatalf("成对号段 %d 不在默认集内", v)
		}
	}
	// 37 在册但只能走链成员位
	if !HasAxiomType(DefaultReferenceAxiomTypes, AxiomSubPropertyChainOf) {
		t.Fatalf("37 应在默认集内（CHAIN_MEMBER 位生效）")
	}
}

func TestIntersectAxiomTypes(t *testing.T) {
	got := IntersectAxiomTypes([]uint32{61, 62, 20, 61}, []uint32{20, 61, 37})
	want := []uint32{61, 20}
	if len(got) != len(want) {
		t.Fatalf("交集长度不符：%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("交集顺序/内容不符：got %v want %v", got, want)
		}
	}
	if r := IntersectAxiomTypes(nil, []uint32{1}); r != nil {
		t.Fatalf("空输入应返回 nil：%v", r)
	}
}

// ---- 成对公理去重（21/22/34/35/36/44/45） ----

func TestMergePairedRows(t *testing.T) {
	const target = uint32(5)
	// 正常成对：宾语侧 (3 → 5) 行 10；主语侧 (5 → 3) 行 9 → 保留较小行号 9
	primary := []RefRow{
		{AxiomID: 10, AxiomType: AxiomDisjointWith, SubjectID: 3, ObjectID: target},
		// 脏数据：同一对两行（11/12）→ 收敛为一行（11）
		{AxiomID: 12, AxiomType: AxiomEquivalentClasses, SubjectID: 4, ObjectID: target},
		{AxiomID: 11, AxiomType: AxiomEquivalentClasses, SubjectID: 4, ObjectID: target},
		// 缺对称伙伴（历史脏数据）→ 保留单边
		{AxiomID: 13, AxiomType: AxiomInverseOf, SubjectID: 6, ObjectID: target},
	}
	counterparts := []RefRow{
		{AxiomID: 9, AxiomType: AxiomDisjointWith, SubjectID: target, ObjectID: 3},
	}
	merged := MergePairedRows(primary, counterparts)
	if len(merged) != 3 {
		t.Fatalf("无序对去重后应为 3 条：%+v", merged)
	}
	byType := make(map[uint32]RefRow, len(merged))
	for _, row := range merged {
		if !row.Paired {
			t.Fatalf("去重结果应标记 paired：%+v", row)
		}
		byType[row.AxiomType] = row
	}
	if row := byType[AxiomDisjointWith]; row.AxiomID != 9 || row.SubjectID != target || row.ObjectID != 3 {
		t.Fatalf("应保留较小行号的对称行：%+v", row)
	}
	if row := byType[AxiomEquivalentClasses]; row.AxiomID != 11 {
		t.Fatalf("同对多行应保留最小行号：%+v", row)
	}
	if row := byType[AxiomInverseOf]; row.AxiomID != 13 {
		t.Fatalf("缺对称行应保留单边：%+v", row)
	}
}

// ---- 37 属性链成员反查 ----

func TestChainRefRows(t *testing.T) {
	const target = uint32(5)
	axioms := []*ent.OntAxiom{
		axiomRow(1, AxiomSubPropertyChainOf, 9, 0, "", EncodeChainMemberIDs([]uint32{6, 5}), "", 0), // 命中尾成员
		axiomRow(2, AxiomSubPropertyChainOf, 8, 0, "", EncodeChainMemberIDs([]uint32{5, 5}), "", 1), // 成员重复仍只出一行
		axiomRow(3, AxiomSubPropertyChainOf, 7, 0, "", EncodeChainMemberIDs([]uint32{6, 7}), "", 2), // 不含目标
		axiomRow(4, AxiomSubPropertyChainOf, 6, 0, "", "{bad", "", 3),                               // 解析失败：WARN 跳过
		axiomRow(5, AxiomObjectPropertyAssertion, 6, 5, "http://ex/p", "", "", 0),                   // 非 37 行忽略
	}
	warns := &warnRecorder{}
	rows := ChainRefRows(context.Background(), axioms, target, warns)
	if len(rows) != 2 {
		t.Fatalf("应命中 2 条链行：%+v", rows)
	}
	if rows[0].AxiomID != 1 || rows[0].SubjectID != 9 || rows[0].Position != ReferencePositionChainMember {
		t.Fatalf("链行 1 投影不符：%+v", rows[0])
	}
	if rows[0].ValueText == "" {
		t.Fatalf("链行应回显 value_text 原文：%+v", rows[0])
	}
	if rows[1].AxiomID != 2 {
		t.Fatalf("链行 2 投影不符：%+v", rows[1])
	}
	if len(warns.messages) != 1 {
		t.Fatalf("解析失败行应 WARN 一次：%+v", warns.messages)
	}
}

// ---- 排序与分页 ----

func TestParseRefOrder(t *testing.T) {
	cases := []struct {
		raw  string
		want []RefOrderField
	}{
		{"", []RefOrderField{{Field: "id", Desc: true}}},
		{"{bad", []RefOrderField{{Field: "id", Desc: true}}},
		{`["id"]`, []RefOrderField{{Field: "id", Desc: false}}},
		{`["-updated_at","-id"]`, []RefOrderField{{Field: "updated_at", Desc: true}, {Field: "id", Desc: true}}},
		{`["created_at", "local_name"]`, []RefOrderField{{Field: "id", Desc: true}}}, // 白名单外 → 默认
	}
	for _, c := range cases {
		got := ParseRefOrder(c.raw)
		if len(got) != len(c.want) {
			t.Fatalf("order %q 解析长度不符：%+v", c.raw, got)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("order %q 解析不符：got %+v want %+v", c.raw, got, c.want)
			}
		}
	}
}

func TestSortAndPageRefRows(t *testing.T) {
	rows := []RefRow{
		{AxiomID: 3}, {AxiomID: 1}, {AxiomID: 2}, {AxiomID: 5}, {AxiomID: 4},
	}
	SortRefRows(rows, ParseRefOrder(`["-id"]`))
	for i, want := range []uint32{5, 4, 3, 2, 1} {
		if rows[i].AxiomID != want {
			t.Fatalf("倒序第 %d 项不符：%+v", i, rows)
		}
	}
	SortRefRows(rows, ParseRefOrder(`["id"]`))
	for i, want := range []uint32{1, 2, 3, 4, 5} {
		if rows[i].AxiomID != want {
			t.Fatalf("升序第 %d 项不符：%+v", i, rows)
		}
	}

	page := PageRefRows(rows, 2, 2)
	if len(page) != 2 || page[0].AxiomID != 3 || page[1].AxiomID != 4 {
		t.Fatalf("第 2 页内容不符：%+v", page)
	}
	if out := PageRefRows(rows, 99, 2); len(out) != 0 {
		t.Fatalf("越界页应为空：%+v", out)
	}
	if out := PageRefRows(rows, 0, 0); len(out) != 5 {
		t.Fatalf("page/pageSize 缺省应按 1/20 处理：%+v", out)
	}
}

func TestRefRowOf_NilColumns(t *testing.T) {
	ax := &ent.OntAxiom{ID: 7}
	row := RefRowOf(ax, ReferencePositionObject)
	if row.AxiomID != 7 || row.AxiomType != 0 || row.SubjectID != 0 || row.ObjectID != 0 {
		t.Fatalf("nil 列应取零值：%+v", row)
	}
	if row.Position != ReferencePositionObject || row.Paired {
		t.Fatalf("位置/成对标默认值不符：%+v", row)
	}
}
