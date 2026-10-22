package ontology

import (
	"context"
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

const testBaseIRI = "http://example.com/ont#"

func classEntity(id uint32, localName string) *ent.OntEntity {
	iri := testBaseIRI + localName
	e := entityRow(id, EntityTypeClass, localName, "")
	e.Iri = &iri
	return e
}

func declAxiom(id, subjectID uint32) *ent.OntAxiom {
	return axiomRow(id, AxiomDeclaration, subjectID, 0, "", "", "", 0)
}

func validate(t *testing.T, entities []*ent.OntEntity, axioms []*ent.OntAxiom) []error {
	t.Helper()
	return ValidateAxioms(context.Background(), testBaseIRI, entities, axioms, nil)
}

func TestValidator_CleanPasses(t *testing.T) {
	entities := []*ent.OntEntity{classEntity(1, "A"), classEntity(2, "B")}
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1),
		declAxiom(11, 2),
		axiomRow(12, AxiomSubClassOf, 2, 1, "", "", "", 0),
		axiomRow(13, AxiomAnnotationAssertion, 1, 0, PredicateRDFSLabel, "类A", "zh", 0),
		axiomRow(14, AxiomAnnotationAssertion, 1, 0, PredicateSKOSNotation, "A-01", "", 0),
	}
	if problems := validate(t, entities, axioms); len(problems) != 0 {
		t.Fatalf("合法数据不应有问题：%v", problems)
	}
}

func TestValidator_IRISyntax(t *testing.T) {
	e := classEntity(1, "A")
	e.Iri = strPtr("not-an-absolute-iri")
	problems := validate(t, []*ent.OntEntity{e}, []*ent.OntAxiom{declAxiom(10, 1)})
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "不是绝对 IRI") {
		t.Fatalf("应报 IRI 语法问题：%v", problems)
	}

	e2 := classEntity(2, "B")
	e2.Iri = strPtr("http://other.com/#B") // 绝对但不以 base_iri 为前缀
	problems = validate(t, []*ent.OntEntity{classEntity(1, "A"), e2},
		[]*ent.OntAxiom{declAxiom(10, 1), declAxiom(11, 2)})
	var hasPrefix bool
	for _, p := range problems {
		if strings.Contains(p.Error(), "不以本体命名空间") {
			hasPrefix = true
		}
	}
	if !hasPrefix {
		t.Fatalf("应报命名空间前缀问题：%v", problems)
	}
}

func TestValidator_ReferenceMissingOrMismatched(t *testing.T) {
	entities := []*ent.OntEntity{classEntity(1, "A"), classEntity(2, "B")}
	// 引用不存在的实体 99
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2),
		axiomRow(12, AxiomSubClassOf, 2, 99, "", "", "", 0),
	}
	problems := validate(t, entities, axioms)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "不存在") {
		t.Fatalf("应报引用缺失：%v", problems)
	}

	// 引用非类实体（对象属性）
	prop := entityRow(3, EntityTypeObjectProperty, "prop", "")
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2),
		axiomRow(12, AxiomSubClassOf, 2, 3, "", "", "", 0),
	}
	problems = validate(t, append(entities, prop), axioms)
	var mismatched bool
	for _, p := range problems {
		if strings.Contains(p.Error(), "类型不符") {
			mismatched = true
		}
	}
	if !mismatched {
		t.Fatalf("应报类型不匹配：%v", problems)
	}
}

func TestValidator_DeclarationCompleteness(t *testing.T) {
	e1 := classEntity(1, "A")
	// 缺声明
	problems := validate(t, []*ent.OntEntity{e1}, nil)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "声明公理数量为 0") {
		t.Fatalf("应报声明缺失：%v", problems)
	}

	// 双声明
	problems = validate(t, []*ent.OntEntity{e1}, []*ent.OntAxiom{declAxiom(10, 1), declAxiom(11, 1)})
	var dup bool
	for _, p := range problems {
		if strings.Contains(p.Error(), "声明公理数量为 2") {
			dup = true
		}
	}
	if !dup {
		t.Fatalf("应报声明重复：%v", problems)
	}

	// 声明主体不存在（软删未同步的防御）
	problems = validate(t, nil, []*ent.OntAxiom{declAxiom(10, 42)})
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "不存在") {
		t.Fatalf("应报声明主体缺失：%v", problems)
	}
}

func TestValidator_AnnotationPredicateWhitelist(t *testing.T) {
	e1 := classEntity(1, "A")
	// 白名单外谓词（自定义注解属性 04 期前一律拒绝）
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1),
		axiomRow(11, AxiomAnnotationAssertion, 1, 0, "http://example.com/my#tag", "x", "", 0),
	}
	problems := validate(t, []*ent.OntEntity{e1}, axioms)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "不在白名单内") {
		t.Fatalf("应拒绝白名单外谓词：%v", problems)
	}

	// lang 超长（>16 字符）
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1),
		axiomRow(11, AxiomAnnotationAssertion, 1, 0, PredicateRDFSLabel, "x", "abcdefghijklmnopqrstuvwxyz", 0),
	}
	problems = validate(t, []*ent.OntEntity{e1}, axioms)
	var langIssue bool
	for _, p := range problems {
		if strings.Contains(p.Error(), "语言标签超长") {
			langIssue = true
		}
	}
	if !langIssue {
		t.Fatalf("应报 lang 超长：%v", problems)
	}
}

func TestValidator_ValueLexical(t *testing.T) {
	e1 := classEntity(1, "A")
	// deprecated 词法
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1),
		axiomRow(11, AxiomAnnotationAssertion, 1, 0, PredicateOWLDeprecated, "yes", "", 0),
	}
	problems := validate(t, []*ent.OntEntity{e1}, axioms)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "owl:deprecated") {
		t.Fatalf("应报 deprecated 词法：%v", problems)
	}

	// notation 字符集
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1),
		axiomRow(11, AxiomAnnotationAssertion, 1, 0, PredicateSKOSNotation, "A 01!", "", 0),
	}
	problems = validate(t, []*ent.OntEntity{e1}, axioms)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "编码") {
		t.Fatalf("应报 notation 字符集：%v", problems)
	}

	// 合法值
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1),
		axiomRow(11, AxiomAnnotationAssertion, 1, 0, PredicateOWLDeprecated, "false", "", 0),
		axiomRow(12, AxiomAnnotationAssertion, 1, 0, PredicateSKOSNotation, "A_01-b", "", 0),
	}
	if problems := validate(t, []*ent.OntEntity{e1}, axioms); len(problems) != 0 {
		t.Fatalf("合法值不应报错：%v", problems)
	}
}

func TestValidator_UnknownAxiomTypeWarnSkipped(t *testing.T) {
	e1 := classEntity(1, "A")
	// 03 期之后号段（90）尚未启用：WARN 跳过、不报错（30 已于 02 期启用并被校验）
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1),
		axiomRow(11, 90, 1, 0, "", "", "", 0),
	}
	warns := &warnRecorder{}
	problems := ValidateAxioms(context.Background(), testBaseIRI, []*ent.OntEntity{e1}, axioms, warns)
	if len(problems) != 0 {
		t.Fatalf("未知号段应跳过不报错：%v", problems)
	}
	if len(warns.messages) == 0 {
		t.Fatal("未知号段应产生 WARN")
	}
}
