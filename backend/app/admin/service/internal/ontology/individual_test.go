package ontology

import (
	"context"
	"strings"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 06 期单测（06 号文档 §8 域内核部分）：取号器纯函数、三区 diff、校验器 60/61 规则。
// 取号 SQL 与并发行为依赖真行锁，转端到端验收（铁律 4 禁内存库）。

// ---- serial.go：格式化与字符集 ----

func TestFormatCode(t *testing.T) {
	if got := FormatCode("A0111", 1); got != "A0111-0001" {
		t.Fatalf("FormatCode 基础补零失败：%s", got)
	}
	if got := FormatCode("A0111", 9999); got != "A0111-9999" {
		t.Fatalf("FormatCode 四位边界失败：%s", got)
	}
	if got := FormatCode("A0111", 10000); got != "A0111-10000" {
		t.Fatalf("FormatCode 超 9999 应自然扩位：%s", got)
	}
	if got := FormatCode("110105", 123); got != "110105-0123" {
		t.Fatalf("FormatCode 纯数字前缀（行政区划码）失败：%s", got)
	}
}

func TestFormatLocalName(t *testing.T) {
	if got := FormatLocalName(1); got != "i000001" {
		t.Fatalf("FormatLocalName 基础补零失败：%s", got)
	}
	if got := FormatLocalName(999999); got != "i999999" {
		t.Fatalf("FormatLocalName 六位边界失败：%s", got)
	}
	if got := FormatLocalName(1000000); got != "i1000000" {
		t.Fatalf("FormatLocalName 超 999999 应自然扩位：%s", got)
	}
}

func TestValidateCodeCharset(t *testing.T) {
	valid := []string{"A0111-0001", "110105-0123", "a.b_c-d", "X1"}
	for _, v := range valid {
		if err := ValidateCodeCharset(v); err != nil {
			t.Fatalf("合法编码 %s 被拒绝：%v", v, err)
		}
	}
	invalid := []string{"$ind", " A0111", "A0111 ", ".abc", "-abc", "_abc", "", "编/码", "a/b"}
	for _, v := range invalid {
		if err := ValidateCodeCharset(v); err == nil {
			t.Fatalf("非法编码 %s 未被拒绝", v)
		} else if de, ok := err.(*DomainError); !ok || de.Reason != ReasonIndividualCodePrefixInvalid {
			t.Fatalf("非法编码 %s 应返回 INDIVIDUAL_CODE_PREFIX_INVALID：%v", v, err)
		}
	}
}

// ---- DiffTypeAssertions：类型断言专用全量替换 ----

func TestDiffTypeAssertions(t *testing.T) {
	const me = uint32(200)
	// 现状：主类目=10（sort 0），次类型=11（sort 1）
	current := []*ent.OntAxiom{
		axiomRow(1, AxiomTypeAssertion, me, 10, "", "", "", 0),
		axiomRow(2, AxiomTypeAssertion, me, 11, "", "", "", 1),
	}

	// 完全一致 → 零变更
	if changes := DiffTypeAssertions(me, current, []uint32{10, 11}); len(changes) != 0 {
		t.Fatalf("无变化应为零变更：%+v", changes)
	}

	// 主类目提升（次序类 11 提升为主，10 退为次）：顺序变化即删全部增全部（sort_order 重建）
	changes := DiffTypeAssertions(me, current, []uint32{11, 10})
	if len(changes) != 4 {
		t.Fatalf("主类目提升应删 2 增 2：%+v", changes)
	}
	for _, c := range changes[:2] {
		if c.Op != AxiomOpDelete {
			t.Fatalf("前两条应为删：%+v", c)
		}
	}
	adds := changes[2:]
	for i, c := range adds {
		if c.Op != AxiomOpAdd || c.AxiomType != AxiomTypeAssertion || c.SubjectID != me {
			t.Fatalf("增行形态不符：%+v", c)
		}
		if c.SortOrder != int32(i) {
			t.Fatalf("增行 sort_order 应按新序重建：%+v", c)
		}
	}
	if adds[0].ObjectID != 11 || adds[1].ObjectID != 10 {
		t.Fatalf("增行应按提交序（11 主 / 10 次）：%+v", adds)
	}

	// 追加类型（存量顺序未变）→ 统一全量替换语义：删 2 增 3（幂等重建，代价 O(n) 可接受；
	// 不复用 DiffSubjectSideRelations 的"只删缺增新"，避免主类目提升时失真——§2.4）
	changes = DiffTypeAssertions(me, current, []uint32{10, 11, 12})
	if len(changes) != 5 {
		t.Fatalf("追加类型应删 2 增 3（全量替换语义）：%+v", changes)
	}
}

// ---- DiffObjectAssertions：61 键比对 ----

func TestDiffObjectAssertions(t *testing.T) {
	const me = uint32(200)
	const pLocated = "https://example.com#locatedIn"
	// 现状：me locatedIn 300
	current := []*ent.OntAxiom{
		axiomRow(1, AxiomObjectPropertyAssertion, me, 300, pLocated, "", "", 0),
	}

	// 无变化 → 零变更
	changes := DiffObjectAssertions(me, current, []ObjectAssertionInput{{PredicateIRI: pLocated, TargetID: 300}})
	if len(changes) != 0 {
		t.Fatalf("无变化应为零变更：%+v", changes)
	}

	// 同谓词多目标（同属性多值）：保留旧行、增新行
	changes = DiffObjectAssertions(me, current, []ObjectAssertionInput{
		{PredicateIRI: pLocated, TargetID: 300},
		{PredicateIRI: pLocated, TargetID: 301},
	})
	if len(changes) != 1 || changes[0].Op != AxiomOpAdd || changes[0].ObjectID != 301 {
		t.Fatalf("同属性多值应只增一行：%+v", changes)
	}

	// 换目标：删旧行 + 增新行
	changes = DiffObjectAssertions(me, current, []ObjectAssertionInput{{PredicateIRI: pLocated, TargetID: 301}})
	if len(changes) != 2 || changes[0].Op != AxiomOpDelete || changes[0].AxiomID != 1 {
		t.Fatalf("换目标应删 1 增 1：%+v", changes)
	}

	// 空 = 清空（全量替换语义，06 号文档 §12 #5）
	changes = DiffObjectAssertions(me, current, nil)
	if len(changes) != 1 || changes[0].Op != AxiomOpDelete {
		t.Fatalf("空提交应清空断言区：%+v", changes)
	}
}

// ---- DiffDataAssertions：62 键比对（含 lang 区分） ----

func TestDiffDataAssertions(t *testing.T) {
	const me = uint32(200)
	const pName = uint32(31)
	const pAge = uint32(32)
	current := []*ent.OntAxiom{
		axiomRow(1, AxiomDataPropertyAssertion, me, pName, "", "张三", "zh", 0),
		axiomRow(2, AxiomDataPropertyAssertion, me, pName, "", "张三", "", 1),
		axiomRow(3, AxiomDataPropertyAssertion, me, pAge, "", "42", "", 2),
	}

	// 全量一致（含 lang 区分）→ 零变更
	changes := DiffDataAssertions(me, current, []DataAssertionInput{
		{PropertyID: pName, Value: "张三", Lang: "zh"},
		{PropertyID: pName, Value: "张三"},
		{PropertyID: pAge, Value: "42"},
	})
	if len(changes) != 0 {
		t.Fatalf("无变化（lang 区分）应为零变更：%+v", changes)
	}

	// 同属性多值：增一行
	changes = DiffDataAssertions(me, current, []DataAssertionInput{
		{PropertyID: pName, Value: "张三", Lang: "zh"},
		{PropertyID: pName, Value: "张三"},
		{PropertyID: pName, Value: "ZS"},
		{PropertyID: pAge, Value: "42"},
	})
	if len(changes) != 1 || changes[0].Op != AxiomOpAdd ||
		changes[0].ObjectID != pName || changes[0].ValueText != "ZS" {
		t.Fatalf("同属性多值应只增一行：%+v", changes)
	}
	if changes[0].SortOrder != 3 {
		t.Fatalf("增行 sort_order 应从存量最大值后顺排：%+v", changes[0])
	}

	// 删一个值（同值无 lang 行）+ 改一个值（42→43）：删 2 增 1
	changes = DiffDataAssertions(me, current, []DataAssertionInput{
		{PropertyID: pName, Value: "张三", Lang: "zh"},
		{PropertyID: pAge, Value: "43"},
	})
	if len(changes) != 3 {
		t.Fatalf("应删 2 增 1（lang 空行删 + 42 行删 + 43 行增）：%+v", changes)
	}

	// 空 = 清空（编码行隔离由服务层在 current 侧完成，本函数对空输入一律全删）
	changes = DiffDataAssertions(me, current, nil)
	if len(changes) != 3 {
		t.Fatalf("空提交应清空全部行：%+v", changes)
	}
}

// ---- 校验器：60/61 规则 ----

func indEntity(id uint32, entityType uint32, localName string) *ent.OntEntity {
	iri := "https://ex.com/ont#" + localName
	return &ent.OntEntity{ID: id, EntityType: &entityType, LocalName: &localName, Iri: &iri}
}

func declarationRows(ids ...uint32) []*ent.OntAxiom {
	rows := make([]*ent.OntAxiom, 0, len(ids))
	for i, id := range ids {
		rows = append(rows, axiomRow(uint32(i+1), AxiomDeclaration, id, 0, "", "", "", 0))
	}
	return rows
}

func TestValidatorTypeAssertion(t *testing.T) {
	const base = "https://ex.com/ont#"
	// 60 规则：object 非 CLASS 拒绝
	entities := []*ent.OntEntity{
		indEntity(100, EntityTypeIndividual, "甲"),
		indEntity(200, EntityTypeIndividual, "乙"),
	}
	axioms := append(declarationRows(100, 200),
		axiomRow(3, AxiomTypeAssertion, 100, 200, "", "", "", 0), // 宾语是个体不是类
	)
	problems := ValidateAxioms(context.Background(), base, entities, axioms, nil)
	if len(problems) == 0 {
		t.Fatal("60 宾语非类应被拒绝")
	}

	// 防御项：个体无 60 行应被拒绝（直改库兜底）
	axioms = declarationRows(100, 200)
	problems = ValidateAxioms(context.Background(), base, entities, axioms, nil)
	found := false
	for _, p := range problems {
		if strings.Contains(p.Error(), "缺少类型断言") {
			found = true
		}
	}
	if !found {
		t.Fatalf("个体无 60 行应触发防御项：%v", problems)
	}

	// 合法：个体挂类
	entities = []*ent.OntEntity{
		indEntity(100, EntityTypeIndividual, "甲"),
		indEntity(200, EntityTypeClass, "学生"),
	}
	axioms = append(declarationRows(100, 200),
		axiomRow(3, AxiomTypeAssertion, 100, 200, "", "", "", 0),
	)
	problems = ValidateAxioms(context.Background(), base, entities, axioms, nil)
	if len(problems) != 0 {
		t.Fatalf("合法断型不应报错：%v", problems)
	}
}

func TestValidatorObjectPropertyAssertion(t *testing.T) {
	const base = "https://ex.com/ont#"
	// 61 规则：谓词未声明拒绝
	entities := []*ent.OntEntity{
		indEntity(100, EntityTypeIndividual, "甲"),
		indEntity(200, EntityTypeIndividual, "乙"),
		indEntity(20, EntityTypeObjectProperty, "locatedIn"),
	}
	axioms := append(declarationRows(100, 200, 20),
		axiomRow(4, AxiomTypeAssertion, 100, 100, "", "", "", 0), // 甲挂甲？不合法——改挂自身 id 100 是个体，这里仅为触发谓词错误
		axiomRow(5, AxiomObjectPropertyAssertion, 100, 200, base+"未声明属性", "", "", 0),
	)
	problems := ValidateAxioms(context.Background(), base, entities, axioms, nil)
	found := false
	for _, p := range problems {
		if strings.Contains(p.Error(), "不是本本体已声明的对象属性") {
			found = true
		}
	}
	if !found {
		t.Fatalf("61 谓词未声明应被拒绝：%v", problems)
	}

	// 两端非个体拒绝（宾语换成类实体）
	entities = []*ent.OntEntity{
		indEntity(100, EntityTypeIndividual, "甲"),
		indEntity(200, EntityTypeClass, "学生"),
		indEntity(20, EntityTypeObjectProperty, "locatedIn"),
	}
	axioms = append(declarationRows(100, 200, 20),
		axiomRow(4, AxiomTypeAssertion, 100, 200, "", "", "", 0),
		axiomRow(5, AxiomObjectPropertyAssertion, 100, 200, base+"locatedIn", "", "", 0), // 宾语是类
	)
	problems = ValidateAxioms(context.Background(), base, entities, axioms, nil)
	found = false
	for _, p := range problems {
		if strings.Contains(p.Error(), "类型不符（期望个体）") {
			found = true
		}
	}
	if !found {
		t.Fatalf("61 宾语非个体应被拒绝：%v", problems)
	}

	// 合法：谓词已声明、两端个体（自引用亦合法——自指断言无语言层禁令）
	entities = []*ent.OntEntity{
		indEntity(100, EntityTypeIndividual, "甲"),
		indEntity(20, EntityTypeObjectProperty, "locatedIn"),
		indEntity(300, EntityTypeClass, "地点"),
	}
	axioms = append(declarationRows(100, 20, 300),
		axiomRow(3, AxiomTypeAssertion, 100, 300, "", "", "", 0),
		axiomRow(4, AxiomObjectPropertyAssertion, 100, 100, base+"locatedIn", "", "", 0),
	)
	problems = ValidateAxioms(context.Background(), base, entities, axioms, nil)
	if len(problems) != 0 {
		t.Fatalf("合法 61 断言不应报错：%v", problems)
	}
}

// ---- 校验器：notation 字符集定案回填（§3.3） ----

func TestValidatorNotationCharset(t *testing.T) {
	const base = "https://ex.com/ont#"
	mk := func(notation string) ([]*ent.OntEntity, []*ent.OntAxiom) {
		entities := []*ent.OntEntity{
			indEntity(100, EntityTypeClass, "A"),
		}
		axioms := append(declarationRows(100),
			axiomRow(2, AxiomAnnotationAssertion, 100, 0, PredicateSKOSNotation, notation, "", 0),
		)
		return entities, axioms
	}

	// 新定案：允许点号、纯数字前缀
	for _, ok := range []string{"A.0111", "110105"} {
		entities, axioms := mk(ok)
		if problems := ValidateAxioms(context.Background(), base, entities, axioms, nil); len(problems) != 0 {
			t.Fatalf("notation %s 应通过新字符集：%v", ok, problems)
		}
	}
	// 非法：首字符点/连字符/下划线、含 $、含空格
	for _, bad := range []string{".abc", "-abc", "_abc", "$ind", "a b"} {
		entities, axioms := mk(bad)
		if problems := ValidateAxioms(context.Background(), base, entities, axioms, nil); len(problems) == 0 {
			t.Fatalf("notation %s 应被拒绝", bad)
		}
	}
}
