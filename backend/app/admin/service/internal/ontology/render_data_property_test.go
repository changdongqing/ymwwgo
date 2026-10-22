package ontology

import (
	"strings"
	"testing"
)

// 03 期渲染分支单测（03 号文档 §4.4）：40~45 + 62 + datatype 短名。

func TestRender_SubAndDomainAndRange(t *testing.T) {
	names := map[uint32]string{2: "P", 3: "C", 1: "A"}
	if got := RenderAxiom(axiomRow(1, AxiomSubDataPropertyOf, 3, 2, "", "", "", 0), names); got != "SubPropertyOf(C P)" {
		t.Fatalf("40 渲染不符：%s", got)
	}
	if got := RenderAxiom(axiomRow(2, AxiomDataPropertyDomain, 2, 1, "", "", "", 0), names); got != "Domain(P A)" {
		t.Fatalf("41 渲染不符：%s", got)
	}
	got := RenderAxiom(axiomRow(3, AxiomDataPropertyRange, 2, 0, "", IRIXSDNamespace+"integer", "", 0), names)
	if got != "Range(P xsd:integer)" {
		t.Fatalf("42 渲染不符：%s", got)
	}
	// 白名单外 IRI 渲染完整 IRI
	got = RenderAxiom(axiomRow(4, AxiomDataPropertyRange, 2, 0, "", "http://example.com/weird", "", 1), names)
	if got != "Range(P http://example.com/weird)" {
		t.Fatalf("白名单外值域应渲染完整 IRI：%s", got)
	}
}

func TestRender_CharacteristicAndPairs(t *testing.T) {
	names := map[uint32]string{2: "P", 3: "C"}
	got := RenderAxiom(axiomRow(1, AxiomDataPropertyCharacteristic, 2, 0, IRIOWLFunctionalProperty, "", "", 0), names)
	if got != "Characteristic(P, functional)" {
		t.Fatalf("43 渲染不符：%s", got)
	}
	// 成对行按无序对去重：仅渲染 subject < object 一侧
	if got := RenderAxiom(axiomRow(2, AxiomEquivalentDataProperties, 3, 2, "", "", "", 0), names); got != "" {
		t.Fatalf("镜像行（subject > object）应渲染空串：%s", got)
	}
	if got := RenderAxiom(axiomRow(3, AxiomEquivalentDataProperties, 2, 3, "", "", "", 0), names); got != "EquivalentDataProperties(P C)" {
		t.Fatalf("44 渲染不符：%s", got)
	}
	if got := RenderAxiom(axiomRow(4, AxiomDisjointDataProperties, 2, 3, "", "", "", 0), names); got != "DisjointDataProperties(P C)" {
		t.Fatalf("45 渲染不符：%s", got)
	}
}

func TestRender_DataPropertyAssertion(t *testing.T) {
	names := map[uint32]string{4: "张三", 2: "age"}
	ax := axiomRow(1, AxiomDataPropertyAssertion, 4, 2, "", "42", "", 0)

	// 裸词形（无值域上下文）
	if got := RenderAxiom(ax, names); got != `DataPropertyAssertion(张三 age "42")` {
		t.Fatalf("62 渲染不符：%s", got)
	}
	// 值域上下文注入：^^datatype 后缀（06 期个体 frame 复用形态）
	datatypeOf := func(propertyID uint32) string { return IRIXSDNamespace + "integer" }
	if got := RenderDataPropertyAssertion(ax, names, datatypeOf); got != `DataPropertyAssertion(张三 age "42"^^xsd:integer)` {
		t.Fatalf("62 带值域渲染不符：%s", got)
	}
	// lang 有值 → @lang（rdf:langString 不落 ^^ 后缀）
	langAx := axiomRow(2, AxiomDataPropertyAssertion, 4, 2, "", "张三", "zh", 0)
	if got := RenderDataPropertyAssertion(langAx, names, datatypeOf); got != `DataPropertyAssertion(张三 age "张三"@zh)` {
		t.Fatalf("62 语言文本渲染不符：%s", got)
	}
	// datatypeOf 返回白名单外 IRI → 不落后缀
	badDatatypeOf := func(uint32) string { return "http://example.com/weird" }
	if got := RenderDataPropertyAssertion(ax, names, badDatatypeOf); !strings.HasSuffix(got, `"42")`) {
		t.Fatalf("白名单外 datatype 不应落后缀：%s", got)
	}
	// 实体名缺省回退 id
	if got := RenderAxiom(ax, nil); got != `DataPropertyAssertion(4 2 "42")` {
		t.Fatalf("62 无实体名渲染不符：%s", got)
	}
}

func TestDatatypeShortName(t *testing.T) {
	cases := map[string]string{
		IRIXSDNamespace + "integer":           "xsd:integer",
		IRIXSDNamespace + "nonNegativeInteger": "xsd:nonNegativeInteger",
		IRIXSDNamespace + "dateTime":          "xsd:dateTime",
		"http://example.com/weird":            "http://example.com/weird",
	}
	for iri, want := range cases {
		if got := DatatypeShortName(iri); got != want {
			t.Fatalf("%s → %s，期望 %s", iri, got, want)
		}
	}
}
