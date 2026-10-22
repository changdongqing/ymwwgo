package ontology

import (
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 05 期校验器断言（05 号文档 §8）：46 规则四拒绝、42 双轨、62 词法链四形态。

// datatypeScenario datatype 实体 + 声明（46 定义 JSON 可选）
func datatypeScenario(id uint32, localName string, definitionJSON string) ([]*ent.OntEntity, []*ent.OntAxiom) {
	e := datatypeEntity(id, localName)
	axioms := []*ent.OntAxiom{declAxiom(id*10, id)}
	if definitionJSON != "" {
		axioms = append(axioms, axiomRow(id*10+1, AxiomDatatypeDefinition, id, 0, "", definitionJSON, "", 0))
	}
	return []*ent.OntEntity{e}, axioms
}

func TestValidator_DatatypeDefinition_Rules(t *testing.T) {
	expectContains := func(t *testing.T, problems []error, want string) {
		t.Helper()
		if len(problems) == 0 {
			t.Fatalf("应拒绝（期望含 %q）", want)
		}
		found := false
		for _, p := range problems {
			if strings.Contains(p.Error(), want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("应含 %q：%v", want, problems)
		}
	}

	// 合法定义（integer [0,100]）通过
	entities, axioms := datatypeScenario(1, "AgeCode",
		`{"baseIri":"`+IRIXSDNamespace+`integer","facets":[{"facet":"minInclusive","value":"0"},{"facet":"maxInclusive","value":"100"}]}`)
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("合法定义应通过：%v", problems)
	}

	// subject 非 datatype 实体（类挂 46 行）
	entities, axioms = datatypeScenario(1, "AgeCode", "")
	entities = append(entities, classEntity(2, "A"))
	axioms = append(axioms, declAxiom(20, 2),
		axiomRow(30, AxiomDatatypeDefinition, 2, 0, "",
			`{"baseIri":"`+IRIXSDNamespace+`integer","facets":[]}`, "", 0))
	expectContains(t, validate(t, entities, axioms), "期望数据类型")

	// base 白名单外（直改库兜底）
	_, axioms = datatypeScenario(1, "AgeCode",
		`{"baseIri":"`+testBaseIRI+`Other","facets":[]}`)
	expectContains(t, validate(t, entities[:1], axioms), "必须是内建数据类型")

	// 脏 JSON
	_, axioms = datatypeScenario(1, "AgeCode", `not-json`)
	expectContains(t, validate(t, entities[:1], axioms), "定义不合法")

	// 未知 facet（白名单外，直改库兜底）
	_, axioms = datatypeScenario(1, "AgeCode",
		`{"baseIri":"`+IRIXSDNamespace+`string","facets":[{"facet":"unknownFacet","value":"1"}]}`)
	expectContains(t, validate(t, entities[:1], axioms), "定义不合法")

	// 静态冲突（直改库兜底）
	_, axioms = datatypeScenario(1, "AgeCode",
		`{"baseIri":"`+IRIXSDNamespace+`integer","facets":[{"facet":"minInclusive","value":"150"},{"facet":"maxInclusive","value":"100"}]}`)
	expectContains(t, validate(t, entities[:1], axioms), "定义不合法")

	// 多行 46（直改库脏数据）不走错误——读取取首行、服务层 diff 自愈（源修订 #12）
	entities, axioms = datatypeScenario(1, "AgeCode",
		`{"baseIri":"`+IRIXSDNamespace+`integer","facets":[]}`)
	axioms = append(axioms,
		axiomRow(22, AxiomDatatypeDefinition, 1, 0, "",
			`{"baseIri":"`+IRIXSDNamespace+`string","facets":[]}`, "", 1))
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("多行 46 不应报错（自愈路径）：%v", problems)
	}
}

func TestValidator_DataPropertyRange_DualTrack(t *testing.T) {
	// 场景：类 1、数据属性 2、datatype 实体 3（integer [0,100]）
	entities := []*ent.OntEntity{classEntity(1, "A"), dataPropEntity(2, "P"), datatypeEntity(3, "Age")}
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2), declAxiom(12, 3),
		axiomRow(20, AxiomDatatypeDefinition, 3, 0, "",
			`{"baseIri":"`+IRIXSDNamespace+`integer","facets":[{"facet":"minInclusive","value":"0"},{"facet":"maxInclusive","value":"100"}]}`, "", 0),
		axiomRow(21, AxiomDataPropertyRange, 2, 3, "", "", "", 0),                       // 派生轨
		axiomRow(22, AxiomDataPropertyRange, 2, 0, "", IRIXSDNamespace+"string", "", 1), // 内建轨
	}
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("双轨混合值域应通过（二选一不双写）：%v", problems)
	}

	// 派生轨指向非 datatype 实体（类）→ 拒绝
	bad := append(axioms[:len(axioms)-1], axiomRow(23, AxiomDataPropertyRange, 2, 1, "", "", "", 1))
	if problems := validate(t, entities, bad); len(problems) == 0 || !strings.Contains(problems[len(problems)-1].Error(), "数据类型实体") {
		t.Fatalf("值域指向类实体应拒绝：%v", problems)
	}

	// 派生轨 value_text 残留（双写尝试，直改库）→ object_id 优先按派生轨判，不因 value_text 报白名单错
	dual := append(axioms[:len(axioms)-1],
		axiomRow(21, AxiomDataPropertyRange, 2, 3, "", IRIXSDNamespace+"integer", "", 0))
	if problems := validate(t, entities, dual); len(problems) != 0 {
		t.Fatalf("object_id 优先：并存不应报内建白名单错：%v", problems)
	}
}

// TestValidator_DataPropertyAssertion_LexicalChain 62 词法链四形态（§8）：
// 内建值域（03 期回归）/ 派生有定义（满足与不满足 facet）/ 派生未定义放行 / 缺省 xsd:string。
func TestValidator_DataPropertyAssertion_LexicalChain(t *testing.T) {
	ageDefJSON := `{"baseIri":"` + IRIXSDNamespace + `integer","facets":[{"facet":"minInclusive","value":"0"},{"facet":"maxInclusive","value":"100"}]}`

	scenario := func(rangeRows ...*ent.OntAxiom) ([]*ent.OntEntity, []*ent.OntAxiom) {
		entities := []*ent.OntEntity{
			individualEntity(1, "I"),
			dataPropEntity(2, "P"),
			datatypeEntity(3, "Age"),
		}
		axioms := []*ent.OntAxiom{declAxiom(10, 1), declAxiom(11, 2), declAxiom(12, 3)}
		axioms = append(axioms, rangeRows...)
		return entities, axioms
	}

	assertOK := func(t *testing.T, entities []*ent.OntEntity, axioms []*ent.OntAxiom, value string) {
		t.Helper()
		axioms = append(axioms, axiomRow(30, AxiomDataPropertyAssertion, 1, 2, "", value, "", 0))
		if problems := validate(t, entities, axioms); len(problems) != 0 {
			t.Fatalf("值 %q 应通过：%v", value, problems)
		}
	}
	assertReject := func(t *testing.T, entities []*ent.OntEntity, axioms []*ent.OntAxiom, value string) {
		t.Helper()
		axioms = append(axioms, axiomRow(30, AxiomDataPropertyAssertion, 1, 2, "", value, "", 0))
		problems := validate(t, entities, axioms)
		if len(problems) == 0 || !strings.Contains(problems[len(problems)-1].Error(), "词法") {
			t.Fatalf("值 %q 应被词法拒绝：%v", value, problems)
		}
	}

	// 形态一：内建值域（03 期行为回归）
	entities, axioms := scenario(axiomRow(21, AxiomDataPropertyRange, 2, 0, "", IRIXSDNamespace+"integer", "", 0))
	assertOK(t, entities, axioms, "42")
	assertReject(t, entities, axioms, "abc")

	// 形态二：派生有定义——满足 / 不满足 facet
	entities, axioms = scenario(
		axiomRow(20, AxiomDatatypeDefinition, 3, 0, "", ageDefJSON, "", 0),
		axiomRow(21, AxiomDataPropertyRange, 2, 3, "", "", "", 0))
	assertOK(t, entities, axioms, "50")
	assertReject(t, entities, axioms, "150")

	// 形态三：派生未定义 → 放行
	entities, axioms = scenario(axiomRow(21, AxiomDataPropertyRange, 2, 3, "", "", "", 0))
	assertOK(t, entities, axioms, "anything-goes")

	// 形态四：无 42 行 → 缺省 xsd:string（无词法约束，任意词形通过）
	entities, axioms = scenario()
	assertOK(t, entities, axioms, "任意文本")
	assertOK(t, entities, axioms, "150") // 不按 integer 校验——缺省路径验证
}
