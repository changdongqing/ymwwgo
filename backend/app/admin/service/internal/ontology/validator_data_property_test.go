package ontology

import (
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 03 期校验器单测（03 号文档 §8）：40~45 结构规则 + 62 数据断言分支四形态。

func dataPropEntity(id uint32, localName string) *ent.OntEntity {
	iri := testBaseIRI + localName
	e := entityRow(id, EntityTypeDataProperty, localName, "")
	e.Iri = &iri
	return e
}

func individualEntity(id uint32, localName string) *ent.OntEntity {
	iri := testBaseIRI + localName
	e := entityRow(id, EntityTypeIndividual, localName, "")
	e.Iri = &iri
	return e
}

// fullScenario 合法最小场景：类 A、数据属性 P/C（C ⊑ P）、个体 I。
func fullScenario() ([]*ent.OntEntity, []*ent.OntAxiom) {
	entities := []*ent.OntEntity{
		classEntity(1, "A"),
		dataPropEntity(2, "P"),
		dataPropEntity(3, "C"),
		individualEntity(4, "I"),
	}
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1),
		declAxiom(11, 2),
		declAxiom(12, 3),
		declAxiom(13, 4),
		axiomRow(20, AxiomSubDataPropertyOf, 3, 2, "", "", "", 0),          // C ⊑ P
		axiomRow(21, AxiomDataPropertyDomain, 2, 1, "", "", "", 0),         // P domain A
		axiomRow(22, AxiomDataPropertyRange, 2, 0, "", IRIXSDNamespace+"integer", "", 0), // P range xsd:integer
		axiomRow(23, AxiomDataPropertyCharacteristic, 2, 0, IRIOWLFunctionalProperty, "", "", 0),
	}
	return entities, axioms
}

func TestValidator_DataProperty_CleanPasses(t *testing.T) {
	entities, axioms := fullScenario()
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("合法数据不应有问题：%v", problems)
	}
}

func TestValidator_SubDataPropertyOf_TypeMismatch(t *testing.T) {
	entities, axioms := fullScenario()
	// 40 型宾语指向类 → 期望数据属性
	axioms = append(axioms, axiomRow(30, AxiomSubDataPropertyOf, 2, 1, "", "", "", 0))
	problems := validate(t, entities, axioms)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "类型不符（期望数据属性）") {
		t.Fatalf("40 型宾语非数据属性应报类型不符：%v", problems)
	}
}

func TestValidator_DataPropertyDomain_MustBeClass(t *testing.T) {
	entities, axioms := fullScenario()
	// 41 型宾语指向数据属性 → 期望类
	axioms = append(axioms, axiomRow(31, AxiomDataPropertyDomain, 2, 3, "", "", "", 0))
	problems := validate(t, entities, axioms)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "类型不符（期望类）") {
		t.Fatalf("41 型宾语非类应报类型不符：%v", problems)
	}
}

func TestValidator_DataPropertyRange_Whitelist(t *testing.T) {
	entities, axioms := fullScenario()
	// 白名单内第二个值域：通过
	axioms = append(axioms, axiomRow(32, AxiomDataPropertyRange, 2, 0, "", IRIXSDNamespace+"string", "", 1))
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("白名单内值域应通过：%v", problems)
	}

	// 白名单外 IRI：拒绝
	axioms = append(axioms, axiomRow(33, AxiomDataPropertyRange, 2, 0, "", "http://example.com/not-a-datatype", "", 2))
	problems := validate(t, entities, axioms)
	if len(problems) == 0 || !strings.Contains(problems[len(problems)-1].Error(), "不是内建数据类型") {
		t.Fatalf("白名单外值域应拒绝：%v", problems)
	}

	// 05 期双轨：object_id 指向非 datatype 实体（fullScenario 的实体 1 是类）→ 类型不符拒绝
	axioms2 := axiomRow(34, AxiomDataPropertyRange, 2, 1, "", IRIXSDNamespace+"string", "", 3)
	problems = validate(t, entities, append(axioms[:len(axioms)-1], axioms2))
	if len(problems) == 0 || !strings.Contains(problems[len(problems)-1].Error(), "数据类型实体") {
		t.Fatalf("42 行 object_id 指向非 datatype 实体应拒绝：%v", problems)
	}
}

func TestValidator_DataPropertyCharacteristic_FunctionalOnly(t *testing.T) {
	entities, axioms := fullScenario()
	// 函数性：通过（fullScenario 已含）；附加传递性（对象属性专属）→ 拒绝
	axioms = append(axioms, axiomRow(35, AxiomDataPropertyCharacteristic, 2, 0, PropertyCharacteristicIRIs[2], "", "", 0))
	problems := validate(t, entities, axioms)
	if len(problems) == 0 || !strings.Contains(problems[len(problems)-1].Error(), "仅支持函数性") {
		t.Fatalf("数据属性非函数性特征应拒绝：%v", problems)
	}
}

func TestValidator_EquivalentDisjointDataProperties(t *testing.T) {
	entities, axioms := fullScenario()
	// 等价/不相交（成对）：通过
	axioms = append(axioms,
		axiomRow(36, AxiomEquivalentDataProperties, 2, 3, "", "", "", 0),
		axiomRow(37, AxiomEquivalentDataProperties, 3, 2, "", "", "", 0),
		axiomRow(38, AxiomDisjointDataProperties, 2, 3, "", "", "", 0),
		axiomRow(39, AxiomDisjointDataProperties, 3, 2, "", "", "", 0),
	)
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("等价/不相交成对应通过：%v", problems)
	}

	// 44 型宾语指向类 → 期望数据属性
	axioms = append(axioms, axiomRow(40, AxiomEquivalentDataProperties, 2, 1, "", "", "", 0))
	problems := validate(t, entities, axioms)
	if len(problems) == 0 || !strings.Contains(problems[len(problems)-1].Error(), "类型不符（期望数据属性）") {
		t.Fatalf("44 型宾语非数据属性应拒绝：%v", problems)
	}
}

func TestValidator_DataPropertyAssertion_StringLiteral(t *testing.T) {
	entities, axioms := fullScenario()
	// 属性 C 无值域行 → 缺省 xsd:string："编码-001" 合法
	axioms = append(axioms, axiomRow(50, AxiomDataPropertyAssertion, 4, 3, "", "编码-001", "", 0))
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("缺省 string 词形应通过：%v", problems)
	}
}

func TestValidator_DataPropertyAssertion_RangeInferred(t *testing.T) {
	entities, axioms := fullScenario()
	// 属性 P 值域 xsd：integer："42" 合法；"abc" 非法词形拒绝
	axioms = append(axioms, axiomRow(51, AxiomDataPropertyAssertion, 4, 2, "", "42", "", 0))
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("integer 词形应通过：%v", problems)
	}

	entities2, axioms2 := fullScenario()
	axioms2 = append(axioms2, axiomRow(52, AxiomDataPropertyAssertion, 4, 2, "", "abc", "", 0))
	problems := validate(t, entities2, axioms2)
	if len(problems) == 0 || !strings.Contains(problems[len(problems)-1].Error(), "不符合") {
		t.Fatalf("非法词形应拒绝：%v", problems)
	}
}

func TestValidator_DataPropertyAssertion_LangLiteral(t *testing.T) {
	entities, axioms := fullScenario()
	// lang 有值 → rdf:langString 语义：仅要求 value_text 非空，不做值域词法校验
	axioms = append(axioms, axiomRow(53, AxiomDataPropertyAssertion, 4, 2, "", "不是数字", "zh", 0))
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("语言文本字面量应放行（仅要求非空）：%v", problems)
	}

	// lang 有值但 value_text 为空 → 拒绝
	entities2, axioms2 := fullScenario()
	axioms2 = append(axioms2, axiomRow(54, AxiomDataPropertyAssertion, 4, 2, "", "", "zh", 0))
	if problems := validate(t, entities2, axioms2); len(problems) == 0 {
		t.Fatal("空语言文本应拒绝")
	}
}

func TestValidator_DataPropertyAssertion_EntityTypes(t *testing.T) {
	entities, axioms := fullScenario()
	// 主语非个体（类）→ 拒绝
	axioms = append(axioms, axiomRow(55, AxiomDataPropertyAssertion, 1, 2, "", "x", "", 0))
	problems := validate(t, entities, axioms)
	if !hasProblem(problems, "期望个体") {
		t.Fatalf("62 主语非个体应拒绝：%v", problems)
	}

	// 谓词非数据属性（类）→ 拒绝
	entities2, axioms2 := fullScenario()
	axioms2 = append(axioms2, axiomRow(56, AxiomDataPropertyAssertion, 4, 1, "", "x", "", 0))
	problems = validate(t, entities2, axioms2)
	if !hasProblem(problems, "期望数据属性") {
		t.Fatalf("62 谓词非数据属性应拒绝：%v", problems)
	}
}

// hasProblem 问题列表中任一错误信息含子串
func hasProblem(problems []error, substr string) bool {
	for _, p := range problems {
		if strings.Contains(p.Error(), substr) {
			return true
		}
	}
	return false
}

func TestValidator_ClassUsageIncludesDataPropertyDomain(t *testing.T) {
	// 类 usage 集扩容：41 在 ClassUsageAxiomTypes 中（01 号文档预埋的 03 期执行点）
	found := false
	for _, v := range ClassUsageAxiomTypes {
		if v == AxiomDataPropertyDomain {
			found = true
		}
	}
	if !found {
		t.Fatal("ClassUsageAxiomTypes 应含 41（数据属性定义域）")
	}
}
