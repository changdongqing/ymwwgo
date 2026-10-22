package ontology

import (
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 02 期对象属性结构校验器单测（02 号文档 §4.4 / §8）：30~37 逐类规则。

func propAxiomSet(t *testing.T, entities []*ent.OntEntity, axioms []*ent.OntAxiom) []error {
	return validate(t, entities, axioms)
}

func TestValidator_SubObjectPropertyOf(t *testing.T) {
	p1, p2 := propEntity(1, "P1"), propEntity(2, "P2")
	cls := classEntity(3, "A")

	// 合法：两端均为对象属性
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2),
		axiomRow(12, AxiomSubObjectPropertyOf, 2, 1, "", "", "", 0),
	}
	if problems := propAxiomSet(t, []*ent.OntEntity{p1, p2}, axioms); len(problems) != 0 {
		t.Fatalf("合法子属性公理不应报错：%v", problems)
	}

	// 宾语为类 → 拒绝
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(12, 3),
		axiomRow(13, AxiomSubObjectPropertyOf, 1, 3, "", "", "", 0),
	}
	problems := propAxiomSet(t, []*ent.OntEntity{p1, cls}, axioms)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "期望对象属性") {
		t.Fatalf("子属性宾语非对象属性应拒绝：%v", problems)
	}
}

func TestValidator_DomainRange(t *testing.T) {
	p := propEntity(1, "P")
	cls := classEntity(2, "A")
	other := propEntity(3, "Q")

	// 合法：subject=属性，object=类
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2),
		axiomRow(12, AxiomObjectPropertyDomain, 1, 2, "", "", "", 0),
		axiomRow(13, AxiomObjectPropertyRange, 1, 2, "", "", "", 1),
	}
	if problems := propAxiomSet(t, []*ent.OntEntity{p, cls}, axioms); len(problems) != 0 {
		t.Fatalf("合法域/值域不应报错：%v", problems)
	}

	// object 为对象属性（非类）→ 拒绝
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 3),
		axiomRow(12, AxiomObjectPropertyDomain, 1, 3, "", "", "", 0),
	}
	problems := propAxiomSet(t, []*ent.OntEntity{p, other}, axioms)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "期望类") {
		t.Fatalf("定义域宾语非类应拒绝：%v", problems)
	}

	// subject 为类（非属性）→ 拒绝
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2),
		axiomRow(12, AxiomObjectPropertyRange, 2, 1, "", "", "", 0),
	}
	problems = propAxiomSet(t, []*ent.OntEntity{p, cls}, axioms)
	if len(problems) == 0 || !strings.Contains(problems[0].Error(), "期望对象属性") {
		t.Fatalf("值域主语非对象属性应拒绝：%v", problems)
	}
}

func TestValidator_CharacteristicWhitelist(t *testing.T) {
	p := propEntity(1, "P")

	// 七特征全部合法
	axioms := []*ent.OntAxiom{declAxiom(10, 1)}
	for i, iri := range PropertyCharacteristicIRIs {
		axioms = append(axioms, axiomRow(uint32(20+i), AxiomObjectPropertyCharacteristic, 1, 0, iri, "", "", 0))
	}
	if problems := propAxiomSet(t, []*ent.OntEntity{p}, axioms); len(problems) != 0 {
		t.Fatalf("七特征白名单内不应报错：%v", problems)
	}

	// 白名单外谓词 → 拒绝
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1),
		axiomRow(20, AxiomObjectPropertyCharacteristic, 1, 0, "http://example.com/#Fake", "", "", 0),
	}
	if problems := propAxiomSet(t, []*ent.OntEntity{p}, axioms); len(problems) == 0 || !strings.Contains(problems[0].Error(), "白名单") {
		t.Fatalf("白名单外特征应拒绝：%v", problems)
	}
}

func TestValidator_PropertyChain(t *testing.T) {
	p1, p2, p3 := propEntity(1, "P1"), propEntity(2, "P2"), propEntity(3, "P3")
	cls := classEntity(4, "A")

	assertOne := func(problems []error, keyword string) {
		t.Helper()
		if len(problems) == 0 || !strings.Contains(problems[0].Error(), keyword) {
			t.Fatalf("应报 %s：%v", keyword, problems)
		}
	}

	// 合法：含 P∘P 重复成员、含超属性自身
	axioms := []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2), declAxiom(12, 3),
		axiomRow(20, AxiomSubPropertyChainOf, 1, 0, "", EncodeChainMemberIDs([]uint32{2, 2, 1}), "", 0),
	}
	if problems := propAxiomSet(t, []*ent.OntEntity{p1, p2, p3}, axioms); len(problems) != 0 {
		t.Fatalf("P∘P/含超属性自身的链不应报错：%v", problems)
	}

	// 成员不足 2
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2),
		axiomRow(20, AxiomSubPropertyChainOf, 1, 0, "", EncodeChainMemberIDs([]uint32{2}), "", 0),
	}
	assertOne(propAxiomSet(t, []*ent.OntEntity{p1, p2}, axioms), "不足")

	// 悬空成员 id
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2),
		axiomRow(20, AxiomSubPropertyChainOf, 1, 0, "", EncodeChainMemberIDs([]uint32{2, 99}), "", 0),
	}
	assertOne(propAxiomSet(t, []*ent.OntEntity{p1, p2}, axioms), "不是已声明的对象属性")

	// 非法 JSON
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1),
		axiomRow(20, AxiomSubPropertyChainOf, 1, 0, "", "{bad", "", 0),
	}
	assertOne(propAxiomSet(t, []*ent.OntEntity{p1}, axioms), "不合法")

	// 成员是类（非对象属性）
	axioms = []*ent.OntAxiom{
		declAxiom(10, 1), declAxiom(11, 2), declAxiom(12, 4),
		axiomRow(20, AxiomSubPropertyChainOf, 1, 0, "", EncodeChainMemberIDs([]uint32{2, 4}), "", 0),
	}
	assertOne(propAxiomSet(t, []*ent.OntEntity{p1, p2, cls}, axioms), "不是已声明的对象属性")
}

func TestValidator_SymmetricPairTypes(t *testing.T) {
	p1, p2 := propEntity(1, "P1"), propEntity(2, "P2")
	cls := classEntity(3, "A")

	for _, tc := range []struct {
		name      string
		axiomType uint32
	}{{"逆属性", AxiomInverseOf}, {"等价", AxiomEquivalentObjectProperties}, {"不相交", AxiomDisjointObjectProperties}} {
		// 合法成对
		axioms := []*ent.OntAxiom{
			declAxiom(10, 1), declAxiom(11, 2),
			axiomRow(20, tc.axiomType, 1, 2, "", "", "", 0),
			axiomRow(21, tc.axiomType, 2, 1, "", "", "", 0),
		}
		if problems := propAxiomSet(t, []*ent.OntEntity{p1, p2}, axioms); len(problems) != 0 {
			t.Fatalf("%s成对行不应报错：%v", tc.name, problems)
		}

		// 一端为类 → 拒绝
		axioms = []*ent.OntAxiom{
			declAxiom(10, 1), declAxiom(11, 3),
			axiomRow(20, tc.axiomType, 1, 3, "", "", "", 0),
		}
		problems := propAxiomSet(t, []*ent.OntEntity{p1, cls}, axioms)
		if len(problems) == 0 || !strings.Contains(problems[0].Error(), "期望对象属性") {
			t.Fatalf("%s一端为类应拒绝：%v", tc.name, problems)
		}
	}
}
