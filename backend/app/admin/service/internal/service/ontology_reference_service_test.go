package service

import (
	"context"
	"strings"
	"testing"

	ontologyV1 "go-wind-admin/api/gen/go/ontology/service/v1"

	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// 08 期通用反查服务层契约测试（不触库）：默认集/位置/分页钳制/谓词过滤参数校验/
// 直查快路径判定（§4.2/§4.6）。

func TestResolveReferenceAxiomTypes(t *testing.T) {
	// 缺省 = §3.2 默认集
	got, err := resolveReferenceAxiomTypes(nil)
	if err != nil {
		t.Fatalf("缺省不应报错：%v", err)
	}
	if len(got) != len(ontology.DefaultReferenceAxiomTypes) {
		t.Fatalf("缺省集长度不符：%v", got)
	}

	// 重复值静默去重（§7 #3）
	got, err = resolveReferenceAxiomTypes([]uint32{61, 62, 61})
	if err != nil {
		t.Fatalf("重复值不应报错：%v", err)
	}
	if len(got) != 2 || got[0] != 61 || got[1] != 62 {
		t.Fatalf("重复值去重不符：%v", got)
	}

	// 未知号段 400
	if _, err = resolveReferenceAxiomTypes([]uint32{61, 999}); err == nil {
		t.Fatalf("未知号段应报错")
	} else if !strings.Contains(err.Error(), "unknown axiom type") {
		t.Fatalf("错误文案不符：%v", err)
	}
}

func TestResolveReferencePositions(t *testing.T) {
	got, err := resolveReferencePositions(nil)
	if err != nil || len(got) != 1 || got[0] != ontology.ReferencePositionObject {
		t.Fatalf("缺省应为 [OBJECT]：%v %v", got, err)
	}

	got, err = resolveReferencePositions([]ontologyV1.ReferencePosition{
		ontologyV1.ReferencePosition_REFERENCE_POSITION_PREDICATE,
		ontologyV1.ReferencePosition_REFERENCE_POSITION_UNSPECIFIED,
		ontologyV1.ReferencePosition_REFERENCE_POSITION_PREDICATE,
	})
	if err != nil {
		t.Fatalf("合法位置不应报错：%v", err)
	}
	// UNSPECIFIED 按 OBJECT；重复 PREDICATE 去重 → [PREDICATE, OBJECT]
	if len(got) != 2 || got[0] != ontology.ReferencePositionPredicate || got[1] != ontology.ReferencePositionObject {
		t.Fatalf("位置规范化不符：%v", got)
	}

	if _, err = resolveReferencePositions([]ontologyV1.ReferencePosition{99}); err == nil {
		t.Fatalf("未知位置应报错")
	}
}

func TestClampReferencePage(t *testing.T) {
	cases := []struct{ page, size, wantPage, wantSize int32 }{
		{0, 0, 1, referenceDefaultPageSize},
		{-3, -5, 1, referenceDefaultPageSize},
		{2, 50, 2, 50},
		{1, 999, 1, referenceMaxPageSize},
	}
	for _, c := range cases {
		p, s := clampReferencePage(c.page, c.size)
		if p != int(c.wantPage) || s != int(c.wantSize) {
			t.Fatalf("钳制不符 (%d,%d)：got (%d,%d)", c.page, c.size, p, s)
		}
	}
}

func TestResolveReferencePredicate_Validation(t *testing.T) {
	svc := &OntologyReferenceService{}
	// 非绝对 IRI → 400
	if _, err := svc.resolveReferencePredicate(context.Background(), 1, "not-an-iri", []uint32{61}); err == nil {
		t.Fatalf("非绝对 IRI 应报错")
	}
	// 空 / 绝对 IRI 且不含 62 → 不触库、返回 0
	for _, iri := range []string{"", "http://example.com/p"} {
		id, err := svc.resolveReferencePredicate(context.Background(), 1, iri, []uint32{61, 100})
		if err != nil || id != 0 {
			t.Fatalf("iri=%q 应返回 0 且不报错：%d %v", iri, id, err)
		}
	}
}

func TestDirectPageableMatrix(t *testing.T) {
	objectProperty := uint32(ontology.EntityTypeObjectProperty)
	classType := uint32(ontology.EntityTypeClass)

	cases := []struct {
		name string
		q    referenceQuery
		want bool
	}{
		{
			name: "默认集+OBJECT：含成对号段 → 内存合并",
			q: referenceQuery{
				positions: []ontology.ReferencePosition{ontology.ReferencePositionObject},
				types:     ontology.DefaultReferenceAxiomTypes,
				order:     ontology.ParseRefOrder(""),
			},
			want: false,
		},
		{
			name: "61+OBJECT：无成对号段 → 直查",
			q: referenceQuery{
				positions: []ontology.ReferencePosition{ontology.ReferencePositionObject},
				types:     []uint32{ontology.AxiomObjectPropertyAssertion},
				order:     ontology.ParseRefOrder(""),
			},
			want: true,
		},
		{
			name: "PREDICATE+对象属性 → 直查",
			q: referenceQuery{
				targetType: objectProperty,
				positions:  []ontology.ReferencePosition{ontology.ReferencePositionPredicate},
				types:      ontology.DefaultReferenceAxiomTypes,
				order:      ontology.ParseRefOrder(""),
			},
			want: true,
		},
		{
			name: "PREDICATE+类目标 → 0 条（非直查也不报错）",
			q: referenceQuery{
				targetType: classType,
				positions:  []ontology.ReferencePosition{ontology.ReferencePositionPredicate},
				types:      ontology.DefaultReferenceAxiomTypes,
				order:      ontology.ParseRefOrder(""),
			},
			want: false,
		},
		{
			name: "多位置 → 内存合并",
			q: referenceQuery{
				targetType: objectProperty,
				positions: []ontology.ReferencePosition{
					ontology.ReferencePositionObject,
					ontology.ReferencePositionPredicate,
					ontology.ReferencePositionChainMember,
				},
				types: ontology.DefaultReferenceAxiomTypes,
				order: ontology.ParseRefOrder(""),
			},
			want: false,
		},
		{
			name: "updated_at 排序 → 内存合并（直查只按 id）",
			q: referenceQuery{
				positions: []ontology.ReferencePosition{ontology.ReferencePositionObject},
				types:     []uint32{ontology.AxiomObjectPropertyAssertion},
				order:     ontology.ParseRefOrder(`["-updated_at"]`),
			},
			want: false,
		},
		{
			name: "excludeDeprecated → 内存合并（需节点信息过滤）",
			q: referenceQuery{
				positions:  []ontology.ReferencePosition{ontology.ReferencePositionObject},
				types:      []uint32{ontology.AxiomObjectPropertyAssertion},
				excludeDep: true,
				order:      ontology.ParseRefOrder(""),
			},
			want: false,
		},
		{
			name: "CHAIN_MEMBER 单位置 → 内存合并",
			q: referenceQuery{
				targetType: objectProperty,
				positions:  []ontology.ReferencePosition{ontology.ReferencePositionChainMember},
				types:      []uint32{ontology.AxiomSubPropertyChainOf},
				order:      ontology.ParseRefOrder(""),
			},
			want: false,
		},
	}
	for _, c := range cases {
		if got := c.q.directPageable(); got != c.want {
			t.Fatalf("%s：directPageable=%v want %v", c.name, got, c.want)
		}
	}
}

func TestAssembleReferenceEdges_NodeProjection(t *testing.T) {
	iri := "http://example.com/onto#hasQuantityKind"
	objPropType := uint32(ontology.EntityTypeObjectProperty)
	individualType := uint32(ontology.EntityTypeIndividual)
	dataPropType := uint32(ontology.EntityTypeDataProperty)

	index := &referenceNodeIndex{
		byID: map[uint32]*ent.OntEntity{
			401: {ID: 401, EntityType: &individualType, LocalName: strPtr("DEG_C"), DisplayLabel: strPtr("摄氏度")},
			341: {ID: 341, EntityType: &individualType, LocalName: strPtr("Temperature")},
			310: {ID: 310, EntityType: &objPropType, Iri: &iri, LocalName: strPtr("hasQuantityKind")},
			320: {ID: 320, EntityType: &dataPropType, LocalName: strPtr("conversionMultiplier")},
		},
		byIRI: map[string]*ent.OntEntity{},
	}
	index.byIRI[iri] = index.byID[310]

	rows := []ontology.RefRow{
		{AxiomID: 1, AxiomType: ontology.AxiomObjectPropertyAssertion, SubjectID: 401, ObjectID: 341, PredicateIRI: iri, Position: ontology.ReferencePositionObject},
		{AxiomID: 2, AxiomType: ontology.AxiomDataPropertyAssertion, SubjectID: 341, ObjectID: 320, ValueText: "1.0", Lang: "en", Position: ontology.ReferencePositionObject},
		{AxiomID: 3, AxiomType: ontology.AxiomObjectPropertyAssertion, SubjectID: 401, ObjectID: 341, PredicateIRI: "http://example.com/missing#p", Position: ontology.ReferencePositionPredicate},
		{AxiomID: 4, AxiomType: ontology.AxiomSubPropertyChainOf, SubjectID: 310, ValueText: `["401"]`, Position: ontology.ReferencePositionChainMember},
		{AxiomID: 5, AxiomType: ontology.AxiomDataPropertyRange, SubjectID: 320, ValueText: "http://www.w3.org/2001/XMLSchema#decimal", Position: ontology.ReferencePositionObject},
	}

	edges, err := assembleReferenceEdges(rows, index)
	if err != nil || len(edges) != len(rows) {
		t.Fatalf("组装失败：%v %d", err, len(edges))
	}

	// 61：主语/谓词/宾语三节点齐全
	if edges[0].Subject.GetDisplayLabel() != "摄氏度" || edges[0].Predicate.GetLocalName() != "hasQuantityKind" ||
		edges[0].Object.GetId() != 341 || edges[0].GetPaired() {
		t.Fatalf("61 投影不符：%+v", edges[0])
	}
	// 62：谓词=数据属性实体，宾语为空，字面量+lang
	if edges[1].Predicate.GetId() != 320 || edges[1].Object != nil ||
		edges[1].GetLiteralValue() != "1.0" || edges[1].GetLang() != "en" {
		t.Fatalf("62 投影不符：%+v", edges[1])
	}
	// 61 谓词反解失败：IRI 尾段 + unresolved
	if !edges[2].Predicate.GetUnresolved() || edges[2].Predicate.GetLocalName() != "p" || edges[2].Predicate.GetId() != 0 {
		t.Fatalf("未解析谓词节点不符：%+v", edges[2].Predicate)
	}
	// 37：字面量回显链 JSON，无宾语
	if edges[3].GetLiteralValue() != `["401"]` || edges[3].Object != nil {
		t.Fatalf("37 投影不符：%+v", edges[3])
	}
	// 42 内建轨：无宾语、IRI 文本进字面量
	if edges[4].Object != nil || edges[4].GetLiteralValue() != "http://www.w3.org/2001/XMLSchema#decimal" {
		t.Fatalf("42 内建轨投影不符：%+v", edges[4])
	}
}

func TestFilterDeprecatedReferenceRows(t *testing.T) {
	deprecated := true
	index := &referenceNodeIndex{
		byID: map[uint32]*ent.OntEntity{
			1: {ID: 1},
			2: {ID: 2, Deprecated: &deprecated},
			3: {ID: 3},
		},
		byIRI: map[string]*ent.OntEntity{},
	}
	rows := []ontology.RefRow{
		{AxiomID: 1, SubjectID: 1, ObjectID: 3},
		{AxiomID: 2, SubjectID: 2, ObjectID: 3},
		{AxiomID: 3, SubjectID: 1, ObjectID: 2},
	}
	kept := filterDeprecatedReferenceRows(rows, index)
	if len(kept) != 1 || kept[0].AxiomID != 1 {
		t.Fatalf("deprecated 过滤不符：%+v", kept)
	}
}

func strPtr(v string) *string { return &v }
