package ontology

import (
	"context"
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 02 期对象属性域内核单测（02 号文档 §8）：链编解码/投影/反查、特征短名。

func propEntity(id uint32, localName string) *ent.OntEntity {
	iri := testBaseIRI + localName
	e := entityRow(id, EntityTypeObjectProperty, localName, "")
	e.Iri = &iri
	return e
}

func TestParseChainMemberIDs(t *testing.T) {
	// 往返：编码 → 解析一致（保序）
	members := []uint32{7, 3, 7} // P∘P 重复成员保序
	encoded := EncodeChainMemberIDs(members)
	if encoded != `["7","3","7"]` {
		t.Fatalf("编码应为 JSON 字符串数组：%s", encoded)
	}
	parsed, err := ParseChainMemberIDs(encoded)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(parsed) != 3 || parsed[0] != 7 || parsed[1] != 3 || parsed[2] != 7 {
		t.Fatalf("往返不一致：%v", parsed)
	}

	// 数字数组形态（直改库）兼容
	if parsed, err = ParseChainMemberIDs(`[7,3]`); err != nil || len(parsed) != 2 || parsed[0] != 7 {
		t.Fatalf("数字数组应兼容：%v %v", parsed, err)
	}

	// 空串 / 非法 JSON / 非法元素 → 错误
	for _, bad := range []string{``, `not-json`, `["abc"]`, `12`, `[true]`} {
		if _, err := ParseChainMemberIDs(bad); err == nil {
			t.Fatalf("非法输入 %q 应报错", bad)
		}
	}
}

func TestPropertyChainsOf(t *testing.T) {
	me := uint32(5)
	other := uint32(9)
	axioms := []*ent.OntAxiom{
		axiomRow(1, AxiomSubPropertyChainOf, me, 0, "", EncodeChainMemberIDs([]uint32{6, 7}), "", 0),
		axiomRow(2, AxiomSubPropertyChainOf, me, 0, "", "", "", 1),                                   // 空链（脏）
		axiomRow(3, AxiomSubPropertyChainOf, me, 0, "", "{bad", "", 2),                               // 解析失败（脏）
		axiomRow(4, AxiomSubPropertyChainOf, other, 0, "", EncodeChainMemberIDs([]uint32{6}), "", 0), // 他人链
		axiomRow(5, AxiomSubClassOf, me, 6, "", "", "", 0),                                           // 非 37 型
	}
	chains := PropertyChainsOf(context.Background(), axioms, me, nil)
	if len(chains) != 3 {
		t.Fatalf("应投影 3 条本属性链行（含脏行）：%+v", chains)
	}
	if chains[0].AxiomID != 1 || len(chains[0].MemberIDs) != 2 || chains[0].MemberIDs[0] != 6 {
		t.Fatalf("链 1 投影不符：%+v", chains[0])
	}
	if chains[1].AxiomID != 2 || chains[1].MemberIDs != nil {
		t.Fatalf("空链应保留条目且成员为空：%+v", chains[1])
	}
	if chains[2].AxiomID != 3 || chains[2].MemberIDs != nil {
		t.Fatalf("解析失败行应保留条目且成员为空：%+v", chains[2])
	}
}

func TestChainReferrers(t *testing.T) {
	me := uint32(5)
	axioms := []*ent.OntAxiom{
		axiomRow(1, AxiomSubPropertyChainOf, 9, 0, "", EncodeChainMemberIDs([]uint32{6, 5}), "", 0), // 命中（尾成员）
		axiomRow(2, AxiomSubPropertyChainOf, 8, 0, "", EncodeChainMemberIDs([]uint32{5, 6}), "", 1), // 命中（首成员）
		axiomRow(3, AxiomSubPropertyChainOf, 7, 0, "", EncodeChainMemberIDs([]uint32{6, 7}), "", 2), // 不含
		axiomRow(4, AxiomSubPropertyChainOf, 6, 0, "", "{bad", "", 3),                               // 解析失败：WARN 跳过
	}
	warns := &warnRecorder{}
	refs := ChainReferrers(context.Background(), axioms, me, warns)
	if len(refs) != 2 {
		t.Fatalf("应命中 2 处链引用：%+v", refs)
	}
	if refs[0].SuperPropertyID != 9 || refs[0].AxiomID != 1 || refs[1].SuperPropertyID != 8 {
		t.Fatalf("命中行不符：%+v", refs)
	}
	if len(warns.messages) != 1 {
		t.Fatalf("解析失败行应 WARN 一次：%+v", warns.messages)
	}
}

func TestCharacteristicShortName(t *testing.T) {
	cases := map[string]string{
		"http://www.w3.org/2002/07/owl#TransitiveProperty":        "transitive",
		"http://www.w3.org/2002/07/owl#InverseFunctionalProperty": "inverseFunctional",
		"http://www.w3.org/2002/07/owl#SymmetricProperty":         "symmetric",
		"http://www.w3.org/2002/07/owl#FunctionalProperty":        "functional",
		"http://www.w3.org/2002/07/owl#ReflexiveProperty":         "reflexive",
		"http://www.w3.org/2002/07/owl#IrreflexiveProperty":       "irreflexive",
		"http://www.w3.org/2002/07/owl#AsymmetricProperty":        "asymmetric",
		"http://www.w3.org/2002/07/owl#UnknownThing":              "http://www.w3.org/2002/07/owl#UnknownThing",
	}
	for iri, want := range cases {
		if got := CharacteristicShortName(iri); got != want {
			t.Fatalf("%s → %s，期望 %s", iri, got, want)
		}
	}
}

func TestPropertyCharacteristicIRIs_DeclaredOrder(t *testing.T) {
	// 声明序即渲染/输出序（源修订 #5）：函数性 → 逆函数性 → 传递性 → 对称性 → 反对称性 → 自反性 → 反自反性
	want := []string{"FunctionalProperty", "InverseFunctionalProperty", "TransitiveProperty",
		"SymmetricProperty", "AsymmetricProperty", "ReflexiveProperty", "IrreflexiveProperty"}
	if len(PropertyCharacteristicIRIs) != len(want) {
		t.Fatalf("特征数量应为 7：%d", len(PropertyCharacteristicIRIs))
	}
	for i, suffix := range want {
		if !strings.HasSuffix(PropertyCharacteristicIRIs[i], "#"+suffix) {
			t.Fatalf("声明序第 %d 位应为 %s：%s", i, suffix, PropertyCharacteristicIRIs[i])
		}
	}
}
