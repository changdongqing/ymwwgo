package ontology

import (
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 05 期数据类型域内核单测（05 号文档 §8）：定义解析/归一化/DiffKey/五规则校验、
// 派生词法 facet 检查、42 双轨三分支、双轨 diff、定义摘要渲染。

// ---- 词法：05 期新增 16 Kind 正/反例（§8 词法清单） ----

func TestValidateLexical_05NewKinds(t *testing.T) {
	const XSD = IRIXSDNamespace
	cases := []struct {
		iri  string
		pass []string
		fail []string
	}{
		{XSD + "nonPositiveInteger", []string{"0", "-1", "-007", "+0"}, []string{"1", "0.5", "abc", ""}},
		{XSD + "positiveInteger", []string{"1", "007", "+7"}, []string{"0", "-1", "1.5", "abc", ""}},
		{XSD + "negativeInteger", []string{"-1", "-007"}, []string{"0", "1", "-1.5", "abc", ""}},
		{XSD + "unsignedLong", []string{"0", "18446744073709551615"}, []string{"-1", "18446744073709551616", "1.0", "abc", ""}},
		{XSD + "unsignedInt", []string{"0", "4294967295"}, []string{"4294967296", "-1", "abc", ""}},
		{XSD + "unsignedShort", []string{"0", "65535"}, []string{"65536", "-1", "abc", ""}},
		{XSD + "unsignedByte", []string{"0", "255"}, []string{"256", "-1", "abc", ""}},
		{XSD + "normalizedString", []string{"plain text", "  spaced  "}, []string{"no\nnewline", "no\ttab", "no\rcr"}},
		{XSD + "token", []string{"plain", "one space"}, []string{" leading", "trailing ", "double  space", "tab\tinside", "  trim? no"}},
		{XSD + "language", []string{"en", "zh-Hans", "zh-Hant-TW", "x-q", "abcdefgh-12345678"}, []string{"", "toolonglanguagetag", "en-", "en--US", "123", "en_US"}},
		{XSD + "Name", []string{"A", "_x", "ns:name", "a1.b-c:d"}, []string{"1a", ".dot", "-dash", "", "sp ace"}},
		{XSD + "NCName", []string{"A", "_x", "a1.b-c", "中文"}, []string{"ns:name", "1a", ".dot", "", "sp ace"}},
		{XSD + "NMTOKEN", []string{"A", "a1.b-c:d", "123", "_"}, []string{"", "sp ace"}},
		{XSD + "hexBinary", []string{"", "0A", "deadBEEF", "00"}, []string{"0", "abc", "0G", "0a 1b"}},
		{XSD + "base64Binary", []string{"", "QUJD", "QQ==", "QUI=", "QUJ=", "QUJD\r\nREVW", "a GVsbG8="}, []string{"A", "QQ===", "Q U J D !", "QUJD="}},
		{XSD + "dateTimeStamp", []string{"2026-10-22T10:00:00Z", "2026-10-22T10:00:00+08:00", "2026-10-22T10:00:00.5-05:30"}, []string{"2026-10-22T10:00:00", "2026-10-22", "not-a-time"}},
	}
	for _, c := range cases {
		for _, v := range c.pass {
			if !ValidateLexical(c.iri, v) {
				t.Fatalf("%s 词形 %q 应通过", c.iri, v)
			}
		}
		for _, v := range c.fail {
			if ValidateLexical(c.iri, v) {
				t.Fatalf("%s 词形 %q 应拒绝", c.iri, v)
			}
		}
	}
}

// ---- ParseDefinition ----

func TestParseDefinition(t *testing.T) {
	if _, err := ParseDefinition(`{"baseIri":"` + IRIXSDNamespace + `integer","facets":[{"facet":"minInclusive","value":"0"},{"facet":"maxInclusive","value":"100"}]}`); err != nil {
		t.Fatalf("合法定义应解析通过：%v", err)
	}
	// 未知键容忍（§1.4 预留 kind 判别键的向前兼容）
	if _, err := ParseDefinition(`{"kind":"restriction","baseIri":"` + IRIXSDNamespace + `integer","facets":[]}`); err != nil {
		t.Fatalf("未知键应容忍：%v", err)
	}
	// 脏 JSON 各形态 → DATATYPE_DEFINITION_JSON_INVALID
	for _, raw := range []string{
		`{`,
		`not json`,
		`[1,2]`,
		`"string"`,
		`null`,
		`123`,
		`{"facets":[{"value":"0"}]}`,            // facet 项缺 facet 名
		`{"facets":"not-array"}`,                // facets 非数组
	} {
		_, err := ParseDefinition(raw)
		if err == nil {
			t.Fatalf("脏 JSON %q 应拒绝", raw)
		}
		if de, ok := err.(*DomainError); !ok || de.Reason != ReasonDatatypeDefinitionJSONInvalid {
			t.Fatalf("脏 JSON %q 错误 reason 应为 DATATYPE_DEFINITION_JSON_INVALID：%v", raw, err)
		}
	}
}

// ---- NormalizeDefinition：固定键序 / 去重后者覆盖 / trim / 空白剔除 / 保序 ----

func TestNormalizeDefinition(t *testing.T) {
	got := NormalizeDefinition(DatatypeDefinition{
		BaseIRI: "  " + IRIXSDNamespace + "integer  ",
		Facets: []DatatypeFacet{
			{Facet: "maxLength", Value: " 10 "},
			{Facet: "minInclusive", Value: "0"},
			{Facet: "maxLength", Value: "20"}, // 后者覆盖，位置不变
			{Facet: "", Value: "junk"},        // 空白名剔除
			{Facet: "pattern", Value: "   "},  // 空白值剔除
		},
	})
	want := `{"baseIri":"` + IRIXSDNamespace + `integer","facets":[{"facet":"maxLength","value":"20"},{"facet":"minInclusive","value":"0"}]}`
	if got != want {
		t.Fatalf("归一化不符：\n got %s\nwant %s", got, want)
	}
	if NormalizeDefinition(DatatypeDefinition{}) != `{"baseIri":"","facets":[]}` {
		t.Fatalf("空定义应输出固定键序空对象：%s", NormalizeDefinition(DatatypeDefinition{}))
	}
}

// ---- DiffKeyDefinition：facet 调序等值（源修订 #7） ----

func TestDiffKeyDefinition_OrderInsensitive(t *testing.T) {
	a := DatatypeDefinition{
		BaseIRI: IRIXSDNamespace + "integer",
		Facets:  []DatatypeFacet{{Facet: "minInclusive", Value: "0"}, {Facet: "maxInclusive", Value: "100"}},
	}
	b := DatatypeDefinition{
		BaseIRI: IRIXSDNamespace + "integer",
		Facets:  []DatatypeFacet{{Facet: "maxInclusive", Value: "100"}, {Facet: "minInclusive", Value: "0"}},
	}
	if DiffKeyDefinition(a) != DiffKeyDefinition(b) {
		t.Fatalf("facet 调序提交应等值：%s vs %s", DiffKeyDefinition(a), DiffKeyDefinition(b))
	}
	c := a
	c.Facets = []DatatypeFacet{{Facet: "minInclusive", Value: "1"}, {Facet: "maxInclusive", Value: "100"}}
	if DiffKeyDefinition(a) == DiffKeyDefinition(c) {
		t.Fatal("值不同 DiffKey 应不同")
	}
}

// ---- ValidateDefinition：白名单 / 族适配 / 值词法 / 静态冲突五规则 ----

func TestValidateDefinition_Rules(t *testing.T) {
	expectReason := func(t *testing.T, err error, want string) {
		t.Helper()
		if err == nil {
			t.Fatalf("应拒绝（期望 %s）", want)
		}
		de, ok := err.(*DomainError)
		if !ok || de.Reason != want {
			t.Fatalf("错误 reason 应为 %s：%v", want, err)
		}
	}

	// 基类型白名单外 / datatype 实体作基类型（单层派生）/ facets 非空而 base 缺失
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: "http://example.com/ont#myType"}),
		ReasonDatatypeBaseInvalid)
	expectReason(t, ValidateDefinition(DatatypeDefinition{Facets: []DatatypeFacet{{Facet: "pattern", Value: "^a$"}}}),
		ReasonDatatypeBaseInvalid)
	// 空定义（仅声明态）通过
	if err := ValidateDefinition(DatatypeDefinition{}); err != nil {
		t.Fatalf("空定义应通过：%v", err)
	}

	// facet 白名单外 / 族不适配 / 值词法非法
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "string",
		Facets: []DatatypeFacet{{Facet: "whiteSpace", Value: "collapse"}}}), ReasonDatatypeFacetInvalid)
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "integer",
		Facets: []DatatypeFacet{{Facet: "length", Value: "5"}}}), ReasonDatatypeFacetInvalid) // 数值基类型 + length
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "string",
		Facets: []DatatypeFacet{{Facet: "minInclusive", Value: "a"}}}), ReasonDatatypeFacetInvalid) // 字符串基类型 + minInclusive
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "integer",
		Facets: []DatatypeFacet{{Facet: "minInclusive", Value: "abc"}}}), ReasonDatatypeFacetInvalid) // 比较值不在 base 词法空间
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "string",
		Facets: []DatatypeFacet{{Facet: "minLength", Value: "-1"}}}), ReasonDatatypeFacetInvalid) // 非负整数列传负数
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "string",
		Facets: []DatatypeFacet{{Facet: "pattern", Value: "(?P<"}}}), ReasonDatatypeFacetInvalid) // RE2 非法正则
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "decimal",
		Facets: []DatatypeFacet{{Facet: "totalDigits", Value: "2.5"}}}), ReasonDatatypeFacetInvalid)

	// 静态冲突五规则（§3.2）
	intDef := func(facets ...DatatypeFacet) DatatypeDefinition {
		return DatatypeDefinition{BaseIRI: IRIXSDNamespace + "integer", Facets: facets}
	}
	expectReason(t, ValidateDefinition(intDef(
		DatatypeFacet{Facet: "minInclusive", Value: "0"}, DatatypeFacet{Facet: "minExclusive", Value: "5"})),
		ReasonDatatypeFacetConflict) // 规则 1
	expectReason(t, ValidateDefinition(intDef(
		DatatypeFacet{Facet: "maxInclusive", Value: "100"}, DatatypeFacet{Facet: "maxExclusive", Value: "100"})),
		ReasonDatatypeFacetConflict) // 规则 2
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "string", Facets: []DatatypeFacet{
		{Facet: "length", Value: "5"}, {Facet: "minLength", Value: "2"}}}),
		ReasonDatatypeFacetConflict) // 规则 3
	expectReason(t, ValidateDefinition(intDef(
		DatatypeFacet{Facet: "minInclusive", Value: "150"}, DatatypeFacet{Facet: "maxInclusive", Value: "100"})),
		ReasonDatatypeFacetConflict) // 规则 4：min > max
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "decimal",
		Facets: []DatatypeFacet{{Facet: "minExclusive", Value: "2.5"}, {Facet: "maxInclusive", Value: "2.5"}}}),
		ReasonDatatypeFacetConflict) // 规则 4 边界：min == max 且任一 exclusive（decimal 基类型，源 §3.2 示例）
	expectReason(t, ValidateDefinition(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "string", Facets: []DatatypeFacet{
		{Facet: "minLength", Value: "10"}, {Facet: "maxLength", Value: "2"}}}),
		ReasonDatatypeFacetConflict) // 规则 5

	// 合法定义通过（静态合法但语义矛盾允许保存——§2.7）
	if err := ValidateDefinition(intDef(
		DatatypeFacet{Facet: "minInclusive", Value: "0"},
		DatatypeFacet{Facet: "maxInclusive", Value: "100"},
		DatatypeFacet{Facet: "pattern", Value: "^[0-9]{1,3}$"})); err != nil {
		t.Fatalf("合法定义应通过：%v", err)
	}
}

// ---- ValidateLexicalDerived：facet 检查语义（§3.2 62 断言 facet 检查规则） ----

func TestValidateLexicalDerived(t *testing.T) {
	ageDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "integer", Facets: []DatatypeFacet{
		{Facet: "minInclusive", Value: "0"}, {Facet: "maxInclusive", Value: "100"},
	}}
	if !ValidateLexicalDerived("50", ageDef) {
		t.Fatal("integer[0,100] 值域下 50 应通过")
	}
	if ValidateLexicalDerived("150", ageDef) {
		t.Fatal("integer[0,100] 值域下 150 应拒绝")
	}
	if ValidateLexicalDerived("abc", ageDef) {
		t.Fatal("integer 基类型词法外应拒绝")
	}

	// big.Rat 精确比较（decimal 族）：0.1 边界
	decDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "decimal", Facets: []DatatypeFacet{
		{Facet: "minInclusive", Value: "0.1"}}}
	if !ValidateLexicalDerived("0.10", decDef) {
		t.Fatal("decimal ≥ 0.1 下 0.10 应通过（big.Rat 等值）")
	}
	if ValidateLexicalDerived("0.0999", decDef) {
		t.Fatal("decimal ≥ 0.1 下 0.0999 应拒绝")
	}

	// float 族
	floatDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "double", Facets: []DatatypeFacet{
		{Facet: "maxExclusive", Value: "1.5"}}}
	if !ValidateLexicalDerived("1.49", floatDef) || ValidateLexicalDerived("1.5", floatDef) {
		t.Fatal("double < 1.5 边界不符")
	}

	// 长度码点 / pattern
	codeDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "string", Facets: []DatatypeFacet{
		{Facet: "minLength", Value: "2"}, {Facet: "maxLength", Value: "4"},
		{Facet: "pattern", Value: `^A\d{2,4}$`}}}
	if !ValidateLexicalDerived("A12", codeDef) {
		t.Fatal("A12 应通过（长度 3 + pattern）")
	}
	if ValidateLexicalDerived("A12345", codeDef) {
		t.Fatal("A12345 应拒绝（长度 6 超上界）")
	}
	if ValidateLexicalDerived("B123", codeDef) {
		t.Fatal("B123 应拒绝（pattern 不命中）")
	}
	// 码点计长（非 ASCII）：中文按 RuneCount 而非字节
	cjkDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "string", Facets: []DatatypeFacet{
		{Facet: "maxLength", Value: "4"}}}
	if !ValidateLexicalDerived("编码一二", cjkDef) {
		t.Fatal("中文 4 码点应通过（utf8.RuneCount）")
	}
	if ValidateLexicalDerived("编码一二三", cjkDef) {
		t.Fatal("中文 5 码点应拒绝")
	}

	// 位数按写形（源修订 #11）：fractionDigits "1.50" → 2
	fracDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "decimal", Facets: []DatatypeFacet{
		{Facet: "fractionDigits", Value: "2"}}}
	if !ValidateLexicalDerived("1.50", fracDef) {
		t.Fatal("1.50 fractionDigits=2 应通过")
	}
	if ValidateLexicalDerived("1.500", fracDef) {
		t.Fatal("1.500 fractionDigits=2 应拒绝（写形 3 位）")
	}
	if !ValidateLexicalDerived("-.5", fracDef) || !ValidateLexicalDerived("5", fracDef) {
		t.Fatal("-.5 / 5 应通过")
	}
	totalDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "decimal", Facets: []DatatypeFacet{
		{Facet: "totalDigits", Value: "4"}}}
	if !ValidateLexicalDerived("-12.34", totalDef) {
		t.Fatal("-12.34 totalDigits=4 应通过（去符号与小数点）")
	}
	if ValidateLexicalDerived("-123.45", totalDef) {
		t.Fatal("-123.45 totalDigits=4 应拒绝")
	}

	// 时间比较：dateTime 墙钟（源修订 #10）/ dateTimeStamp 绝对时刻
	dtDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "dateTime", Facets: []DatatypeFacet{
		{Facet: "minInclusive", Value: "2026-01-01T00:00:00Z"},
		{Facet: "maxInclusive", Value: "2026-12-31T23:59:59Z"}}}
	if !ValidateLexicalDerived("2026-06-01T08:00:00+08:00", dtDef) {
		t.Fatal("dateTime 墙钟比较应通过（08:00+08:00 与 00:00Z 同墙钟日中点）")
	}
	if ValidateLexicalDerived("2025-06-01T00:00:00Z", dtDef) {
		t.Fatal("2025 年应拒绝")
	}
	// dateTime 墙钟的边界体现：08:00+08:00 的墙钟 = 00:00Z 同日零点 → 恰好命中最小界
	boundaryDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "dateTime", Facets: []DatatypeFacet{
		{Facet: "minExclusive", Value: "2026-01-01T00:00:00Z"}}}
	if !ValidateLexicalDerived("2026-01-01T08:00:00+08:00", boundaryDef) {
		t.Fatal("dateTime 墙钟比较：与下界同墙钟时刻但偏移不同应视为相等（minExclusive → 拒绝）")
	}
	// dateTimeStamp 时区感知：+08:00 墙钟 08:00 = 绝对 00:00Z → minExclusive 拒绝
	dtsDef := DatatypeDefinition{BaseIRI: IRIXSDNamespace + "dateTimeStamp", Facets: []DatatypeFacet{
		{Facet: "minExclusive", Value: "2026-01-01T00:00:00Z"}}}
	if ValidateLexicalDerived("2026-01-01T08:00:00+08:00", dtsDef) {
		t.Fatal("dateTimeStamp 绝对时刻比较：08:00+08:00 == 00:00Z 应拒绝")
	}
	if !ValidateLexicalDerived("2026-01-01T08:01:00+08:00", dtsDef) {
		t.Fatal("dateTimeStamp 绝对时刻晚 1 分钟应通过")
	}

	// 未定义 / 白名单外基类型 fail-closed
	if ValidateLexicalDerived("anything", DatatypeDefinition{}) {
		t.Fatal("空定义（无基类型）应 fail-closed 拒绝")
	}
}

// ---- ResolveRangeTargets：三分支（§2.2） ----

func datatypeEntity(id uint32, localName string) *ent.OntEntity {
	iri := testBaseIRI + localName
	e := entityRow(id, EntityTypeDatatype, localName, "")
	e.OntologyID = ontologyIDPtr(1)
	e.Iri = &iri
	return e
}

func classEntityOfOntology(id uint32, localName string, ontologyID uint32) *ent.OntEntity {
	iri := testBaseIRI + localName
	e := entityRow(id, EntityTypeClass, localName, "")
	e.OntologyID = ontologyIDPtr(ontologyID)
	e.Iri = &iri
	return e
}

func ontologyIDPtr(id uint32) *uint32 { return &id }

func TestResolveRangeTargets(t *testing.T) {
	datatype := datatypeEntity(9, "AgeCode")
	classCross := classEntityOfOntology(8, "CrossClass", 2) // 跨本体的类
	lookup := func(iri string) (*ent.OntEntity, error) {
		for _, e := range []*ent.OntEntity{datatype, classCross} {
			if e.Iri != nil && *e.Iri == iri {
				return e, nil
			}
		}
		return nil, nil
	}

	// 内建 → value_text 轨
	targets, err := ResolveRangeTargets([]string{IRIXSDNamespace + "string"}, 1, lookup)
	if err != nil || len(targets) != 1 || targets[0].ValueText == "" || targets[0].ObjectID != 0 {
		t.Fatalf("内建 IRI 应走 value_text 轨：%+v %v", targets, err)
	}
	// 本体 datatype 实体 → object_id 轨
	targets, err = ResolveRangeTargets([]string{testBaseIRI + "AgeCode"}, 1, lookup)
	if err != nil || len(targets) != 1 || targets[0].ObjectID != 9 || targets[0].ValueText != "" {
		t.Fatalf("datatype 实体 IRI 应走 object_id 轨：%+v %v", targets, err)
	}
	// 类型不符（类实体）→ DATATYPE_RANGE_TARGET_INVALID
	_, err = ResolveRangeTargets([]string{testBaseIRI + "CrossClass"}, 1, lookup)
	if de, ok := err.(*DomainError); !ok || de.Reason != ReasonDatatypeRangeTargetInvalid {
		t.Fatalf("非 datatype 实体应 DATATYPE_RANGE_TARGET_INVALID：%v", err)
	}
	// 未知 IRI → DATA_PROPERTY_RANGE_INVALID
	_, err = ResolveRangeTargets([]string{"http://example.com/nowhere#x"}, 1, lookup)
	if de, ok := err.(*DomainError); !ok || de.Reason != ReasonDataPropertyRangeInvalid {
		t.Fatalf("未知 IRI 应 DATA_PROPERTY_RANGE_INVALID：%v", err)
	}
	// 跨本体 → DATATYPE_RANGE_TARGET_INVALID
	_, err = ResolveRangeTargets([]string{testBaseIRI + "CrossClass"}, 9, lookup)
	if de, ok := err.(*DomainError); !ok || de.Reason != ReasonDatatypeRangeTargetInvalid {
		t.Fatalf("跨本体应 DATATYPE_RANGE_TARGET_INVALID：%v", err)
	}
}

// ---- DiffRangeRefs：双轨混合 diff（§8 双轨清单） ----

func TestDiffRangeRefs(t *testing.T) {
	const me = 100
	// 现状：内建轨 xsd:integer（sort 0）、派生轨实体 9（sort 1，IRI 反查命中）
	current := []RangeRow{
		{AxiomID: 1, IRI: IRIXSDNamespace + "integer", ValueText: IRIXSDNamespace + "integer", SortOrder: 0},
		{AxiomID: 2, IRI: testBaseIRI + "Age", ObjectID: 9, SortOrder: 1},
	}
	builtin := RangeRef{IRI: IRIXSDNamespace + "string", ValueText: IRIXSDNamespace + "string"}
	derived := RangeRef{IRI: testBaseIRI + "Age", ObjectID: 9}

	// 无变化 → 零变更
	if changes := DiffRangeRefs(me, AxiomDataPropertyRange, current, []RangeRef{
		{IRI: IRIXSDNamespace + "integer", ValueText: IRIXSDNamespace + "integer"}, derived,
	}); len(changes) != 0 {
		t.Fatalf("无变化应为零变更：%+v", changes)
	}

	// 删内建补派生（换轨）：删 1 行 + 增 1 行（object_id 轨形态）
	changes := DiffRangeRefs(me, AxiomDataPropertyRange, current, []RangeRef{builtin, derived})
	var dels, adds int
	var addObjectID uint32
	var addValueText string
	for _, c := range changes {
		switch c.Op {
		case AxiomOpDelete:
			dels++
		case AxiomOpAdd:
			adds++
			addObjectID = c.ObjectID
			addValueText = c.ValueText
		}
	}
	if dels != 1 || adds != 1 {
		t.Fatalf("换轨应删 1 增 1：%+v", changes)
	}
	if addObjectID != builtinObjectIDSentinel || addValueText != IRIXSDNamespace+"string" {
		t.Fatalf("新增行应落内建 value_text 轨：%+v", changes[len(changes)-1])
	}
	if got := sortOrderOfLastAdd(changes); got != 2 { // 保留行最大 sort 1 后顺排
		t.Fatalf("新增行 sort_order 应为 2：%+v", changes)
	}

	// 悬空行（IRI 空）→ 删除侧清理
	dangling := []RangeRow{{AxiomID: 3, IRI: "", ObjectID: 77, SortOrder: 2}}
	changes = DiffRangeRefs(me, AxiomDataPropertyRange, dangling, nil)
	if len(changes) != 1 || changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 3 {
		t.Fatalf("悬空行应删除自愈：%+v", changes)
	}

	// 重排（保留行 sort_order 非严格递增）→ 整段重写
	reordered := []RangeRow{
		{AxiomID: 1, IRI: IRIXSDNamespace + "integer", ValueText: IRIXSDNamespace + "integer", SortOrder: 1},
		{AxiomID: 2, IRI: testBaseIRI + "Age", ObjectID: 9, SortOrder: 0},
	}
	changes = DiffRangeRefs(me, AxiomDataPropertyRange, reordered, []RangeRef{
		{IRI: IRIXSDNamespace + "integer", ValueText: IRIXSDNamespace + "integer"}, derived,
	})
	if len(changes) != 4 { // 删 2 + 增 2（重排重写）
		t.Fatalf("重排应整段重写（删 2 增 2）：%+v", changes)
	}
}

const builtinObjectIDSentinel = uint32(0) // 内建轨 object_id 恒零（二选一不双写）

func sortOrderOfLastAdd(changes []AxiomChange) int32 {
	for i := len(changes) - 1; i >= 0; i-- {
		if changes[i].Op == AxiomOpAdd {
			return changes[i].SortOrder
		}
	}
	return -1
}

// ---- DefinitionSummary 渲染（§4.7 源修订 #5） ----

func TestDefinitionSummary(t *testing.T) {
	got := DefinitionSummary(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "integer", Facets: []DatatypeFacet{
		{Facet: "minInclusive", Value: "0"}, {Facet: "maxInclusive", Value: "100"},
	}})
	if got != "integer [0 ≤ x ≤ 100]" {
		t.Fatalf("区间摘要不符：%s", got)
	}
	got = DefinitionSummary(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "string", Facets: []DatatypeFacet{
		{Facet: "pattern", Value: `^A\d{4}$`}, {Facet: "minLength", Value: "2"},
	}})
	if got != `string ~ /^A\d{4}$/ minLength=2` {
		t.Fatalf("pattern/长度摘要不符：%s", got)
	}
	got = DefinitionSummary(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "integer"})
	if got != "integer" {
		t.Fatalf("无 facet 摘要应为基类型短名：%s", got)
	}
	// 混合 exclusive 区间
	got = DefinitionSummary(DatatypeDefinition{BaseIRI: IRIXSDNamespace + "decimal", Facets: []DatatypeFacet{
		{Facet: "minExclusive", Value: "0"}, {Facet: "maxInclusive", Value: "1"},
	}})
	if got != "decimal [0 < x ≤ 1]" {
		t.Fatalf("exclusive 区间摘要不符：%s", got)
	}
	// 摘要不得出现完整命名空间（未定义态返回空串由调用方兜底）
	if strings.Contains(got, IRIXSDNamespace) {
		t.Fatalf("摘要应使用短名：%s", got)
	}
}
