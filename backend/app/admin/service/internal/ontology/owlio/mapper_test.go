package owlio

import (
	"sort"
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// mapper 测试（07 号文档 §8：§3.2 总表逐型正例 + round-trip 零漂移）。

func strPtr(v string) *string { return &v }
func u32Ptr(v uint32) *uint32 { return &v }

func mkEnt(id, entityType uint32, iri, localName string) *ent.OntEntity {
	return &ent.OntEntity{ID: id, EntityType: u32Ptr(entityType), Iri: strPtr(iri), LocalName: strPtr(localName)}
}

func mkAx(id, axiomType, subject, object uint32, predicate, value, lang string, sortOrder int32) *ent.OntAxiom {
	ax := &ent.OntAxiom{
		ID: id, AxiomType: u32Ptr(axiomType), SubjectID: u32Ptr(subject),
		PredicateIri: nil, ValueText: nil, Lang: nil,
		SortOrder: u32Ptr(uint32(sortOrder)),
	}
	if object != 0 {
		ax.ObjectID = u32Ptr(object)
	}
	if predicate != "" {
		ax.PredicateIri = strPtr(predicate)
	}
	if value != "" {
		ax.ValueText = strPtr(value)
	}
	if lang != "" {
		ax.Lang = strPtr(lang)
	}
	return ax
}

func TestRowsToTriplesAxiomForms(t *testing.T) {
	const ns = "http://example.com/ns#"
	entities := []*ent.OntEntity{
		mkEnt(1, ontology.EntityTypeClass, ns+"A", "A"),
		mkEnt(2, ontology.EntityTypeClass, ns+"B", "B"),
		mkEnt(3, ontology.EntityTypeObjectProperty, ns+"p", "p"),
		mkEnt(4, ontology.EntityTypeObjectProperty, ns+"q", "q"),
		mkEnt(5, ontology.EntityTypeObjectProperty, ns+"r", "r"),
		mkEnt(6, ontology.EntityTypeDataProperty, ns+"dp", "dp"),
		mkEnt(7, ontology.EntityTypeAnnotationProperty, ns+"ap", "ap"),
		mkEnt(8, ontology.EntityTypeDatatype, ns+"dt", "dt"),
		mkEnt(9, ontology.EntityTypeIndividual, ns+"i1", "i1"),
		mkEnt(10, ontology.EntityTypeIndividual, ns+"i2", "i2"),
	}
	axioms := []*ent.OntAxiom{
		mkAx(1, ontology.AxiomSubClassOf, 1, 2, "", "", "", 0),
		mkAx(2, ontology.AxiomDisjointWith, 1, 2, "", "", "", 0),
		mkAx(3, ontology.AxiomEquivalentClasses, 1, 2, "", "", "", 0),
		mkAx(4, ontology.AxiomSubObjectPropertyOf, 3, 4, "", "", "", 0),
		mkAx(5, ontology.AxiomObjectPropertyDomain, 3, 1, "", "", "", 0),
		mkAx(6, ontology.AxiomObjectPropertyRange, 3, 2, "", "", "", 0),
		mkAx(7, ontology.AxiomObjectPropertyCharacteristic, 3, 0, ontology.IRIOWLFunctionalProperty, "", "", 0),
		mkAx(8, ontology.AxiomInverseOf, 3, 4, "", "", "", 0),
		mkAx(9, ontology.AxiomEquivalentObjectProperties, 3, 4, "", "", "", 0),
		mkAx(10, ontology.AxiomDisjointObjectProperties, 3, 4, "", "", "", 0),
		mkAx(11, ontology.AxiomSubPropertyChainOf, 3, 0, "", "[4,5]", "", 0),
		mkAx(12, ontology.AxiomSubDataPropertyOf, 6, 3, "", "", "", 0),
		mkAx(13, ontology.AxiomDataPropertyDomain, 6, 1, "", "", "", 0),
		mkAx(14, ontology.AxiomDataPropertyRange, 6, 0, "", ontology.IRIXMLSchemaString, "", 0),
		mkAx(15, ontology.AxiomDataPropertyCharacteristic, 6, 0, ontology.IRIOWLFunctionalProperty, "", "", 0),
		mkAx(16, ontology.AxiomSubAnnotationPropertyOf, 7, 3, "", "", "", 0),
		mkAx(17, ontology.AxiomDatatypeDefinition, 8, 0, "", `{"baseIri":"http://www.w3.org/2001/XMLSchema#integer","facets":[{"facet":"minInclusive","value":"0"}]}`, "", 0),
		mkAx(18, ontology.AxiomTypeAssertion, 9, 1, "", "", "", 0),
		mkAx(19, ontology.AxiomTypeAssertion, 9, 2, "", "", "", 1),
		mkAx(20, ontology.AxiomObjectPropertyAssertion, 9, 10, ns+"p", "", "", 0),
		mkAx(21, ontology.AxiomDataPropertyAssertion, 9, 6, "", "值", "zh", 0),
		mkAx(22, ontology.AxiomDataPropertyAssertion, 9, 6, "", "plain", "", 0),
		mkAx(23, ontology.AxiomAnnotationAssertion, 9, 0, ontology.PredicateRDFSLabel, "甲", "zh", 0),
		mkAx(24, ontology.AxiomAnnotationAssertion, 9, 0, ontology.PredicateSKOSNotation, "A0111", "", 0),
		mkAx(25, ontology.AxiomAnnotationAssertion, 9, 0, ontology.PredicateOWLDeprecated, "true", "", 0),
		mkAx(26, ontology.AxiomAnnotationAssertion, 9, 7, "", "自定义注解值", "", 0),
	}

	triples, warnings := RowsToTriples(entities, axioms)
	if len(warnings) != 0 {
		t.Fatalf("导出不应有告警：%v", warnings)
	}

	// 检查若干关键形态
	predSet := map[string]int{}
	for _, tr := range triples {
		predSet[tr.Predicate.Value]++
	}
	expect := map[string]int{
		IRIRDFSSubClassOf:           1,
		IRIOWLDisjointWith:          1,
		IRIOWLEquivalentClass:       2, // 22 类等价 + 46 datatype 定义
		IRIRDFSSubPropertyOf:        3, // 30 + 40 + 50 各一条
		IRIRDFSDomain:               2,
		IRIRDFSRange:                2, // 32 + 42
		IRIOWLInverseOf:             1,
		IRIOWLEquivalentProperty:    1,
		IRIOWLPropertyDisjointWith:  1,
		IRIOWLPropertyChainAxiom:    1,
		IRIOWLWithRestrictions:      1,
		ns + "p":                    1, // 61 断言
		ns + "dp":                   2, // 62 断言（lang + plain）
		ontology.PredicateRDFSLabel: 1,
		ontology.PredicateSKOSNotation: 1,
	}
	for iri, want := range expect {
		if predSet[iri] != want {
			t.Errorf("谓词 %s 计数 %d != %d", iri, predSet[iri], want)
		}
	}

	// 37 链成员保序（first/rest 收集后走链）
	firsts, rests := map[string]string{}, map[string]string{}
	cur := ""
	for _, tr := range triples {
		if tr.Predicate.Value == IRIOWLPropertyChainAxiom {
			cur = tr.Object.Value
		}
		if tr.Subject.Kind == TermBlank && tr.Predicate.Value == IRIRDFFirst {
			firsts[tr.Subject.Value] = tr.Object.Value
		}
		if tr.Subject.Kind == TermBlank && tr.Predicate.Value == IRIRDFRest {
			rests[tr.Subject.Value] = tr.Object.Value
		}
	}
	var chain []string
	for steps := 0; cur != "" && cur != IRIRDFNil && steps < 8; steps++ {
		chain = append(chain, firsts[cur])
		cur = rests[cur]
	}
	if len(chain) != 2 || chain[0] != ns+"q" || chain[1] != ns+"r" {
		t.Fatalf("37 链保序失败：%v", chain)
	}

	// 46 定义：facet 值按基类型 typed（源修订 #3）
	foundFacet := false
	for _, tr := range triples {
		if tr.Subject.Kind == TermBlank && tr.Predicate.Value == NSXSD+"minInclusive" {
			foundFacet = true
			if tr.Object.Datatype != NSXSD+"integer" || tr.Object.Value != "0" {
				t.Fatalf("facet 值应为基类型 typed literal：%+v", tr.Object)
			}
		}
	}
	if !foundFacet {
		t.Fatalf("缺少 withRestrictions facet 三元组")
	}

	// header 追加
	withHeader := AppendHeaderTriples(triples, OntologyHeader{IRI: "http://example.com/ont#", Version: 3, Description: "测试本体"})
	if len(withHeader) != len(triples)+3 {
		t.Fatalf("header 应追加 3 条三元组")
	}
}

// semKey 公理行的语义七元组键（round-trip 比较两侧行集合）。
type semKey struct {
	axiomType          uint32
	subject            string
	other              string // object 实体 IRI / 谓词 IRI（62 的属性、100 的谓词）/ 链成员列表
	value, lang        string
	sortOrder          int32
}

func semKeyOfEntRow(ax *ent.OntAxiom, byID map[uint32]*ent.OntEntity) semKey {
	iri := func(id *uint32) string {
		if id == nil {
			return ""
		}
		if e, ok := byID[*id]; ok && e.Iri != nil {
			return *e.Iri
		}
		return ""
	}
	k := semKey{
		axiomType: derefU32p(ax.AxiomType),
		subject:   iri(ax.SubjectID),
		value:     derefStrp(ax.ValueText),
		lang:      derefStrp(ax.Lang),
	}
	if ax.SortOrder != nil {
		k.sortOrder = int32(*ax.SortOrder)
	}
	if ax.ObjectID != nil && *ax.ObjectID != 0 {
		k.other = iri(ax.ObjectID)
	}
	if ax.PredicateIri != nil {
		k.other = *ax.PredicateIri
	}
	if k.axiomType == ontology.AxiomDataPropertyAssertion && ax.ObjectID != nil {
		k.other = iri(ax.ObjectID)
	}
	return k
}

func semKeyOfDraft(d AxiomDraft, plan *ImportPlan) semKey {
	iriOf := func(ref int) string {
		if ref < 0 || ref >= len(plan.Entities) {
			return ""
		}
		return plan.Entities[ref].IRI
	}
	k := semKey{
		axiomType: d.AxiomType,
		subject:   iriOf(d.SubjectRef),
		value:     d.ValueText,
		lang:      d.Lang,
		sortOrder: d.SortOrder,
	}
	if d.MemberRefs != nil {
		members := make([]string, 0, len(d.MemberRefs))
		for _, r := range d.MemberRefs {
			members = append(members, iriOf(r))
		}
		sort.Strings(members)
		for _, m := range members {
			k.other += m + "\x00"
		}
		return k
	}
	if d.PredicateIRI != "" {
		k.other = d.PredicateIRI
	} else if d.PredicateRef >= 0 {
		k.other = iriOf(d.PredicateRef)
	} else if d.ObjectRef >= 0 {
		k.other = iriOf(d.ObjectRef)
	}
	if d.AxiomType == ontology.AxiomDataPropertyAssertion && d.ObjectRef >= 0 {
		k.other = iriOf(d.ObjectRef)
	}
	return k
}

func derefU32p(p *uint32) uint32 {
	if p == nil {
		return 0
	}
	return *p
}

func derefStrp(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// TestRoundTripZeroDrift round-trip 闭环：行 → 三元组 → Turtle → 解析 → 计划草稿，行集合语义相等。
func TestRoundTripZeroDrift(t *testing.T) {
	const ns = "http://example.com/ont#"
	entities := []*ent.OntEntity{
		mkEnt(1, ontology.EntityTypeClass, ns+"A", "A"),
		mkEnt(2, ontology.EntityTypeClass, ns+"B", "B"),
		mkEnt(3, ontology.EntityTypeObjectProperty, ns+"p", "p"),
		mkEnt(4, ontology.EntityTypeObjectProperty, ns+"q", "q"),
		mkEnt(5, ontology.EntityTypeDataProperty, ns+"dp", "dp"),
		mkEnt(6, ontology.EntityTypeAnnotationProperty, ns+"ap", "ap"),
		mkEnt(7, ontology.EntityTypeDatatype, ns+"dt", "dt"),
		mkEnt(8, ontology.EntityTypeIndividual, ns+"i1", "i1"),
		mkEnt(9, ontology.EntityTypeIndividual, ns+"i2", "i2"),
		mkEnt(10, ontology.EntityTypeAnnotationProperty, ns+"ap2", "ap2"),
	}
	axioms := []*ent.OntAxiom{
		// 类
		mkAx(1, ontology.AxiomSubClassOf, 1, 2, "", "", "", 0),
		mkAx(2, ontology.AxiomDisjointWith, 1, 2, "", "", "", 0),
		mkAx(3, ontology.AxiomDisjointWith, 2, 1, "", "", "", 0), // 成对存储的反向行
		mkAx(4, ontology.AxiomEquivalentClasses, 1, 2, "", "", "", 0),
		mkAx(5, ontology.AxiomEquivalentClasses, 2, 1, "", "", "", 0),
		// 对象属性
		mkAx(6, ontology.AxiomSubObjectPropertyOf, 3, 4, "", "", "", 0),
		mkAx(7, ontology.AxiomObjectPropertyDomain, 3, 1, "", "", "", 0),
		mkAx(8, ontology.AxiomObjectPropertyRange, 3, 2, "", "", "", 0),
		mkAx(9, ontology.AxiomObjectPropertyCharacteristic, 3, 0, ontology.IRIOWLFunctionalProperty, "", "", 0),
		mkAx(10, ontology.AxiomInverseOf, 3, 4, "", "", "", 0),
		mkAx(11, ontology.AxiomInverseOf, 4, 3, "", "", "", 0),
		// 数据属性
		mkAx(12, ontology.AxiomDataPropertyDomain, 5, 1, "", "", "", 0),
		mkAx(13, ontology.AxiomDataPropertyRange, 5, 0, "", ontology.IRIXMLSchemaString, "", 0),
		// 数据类型
		mkAx(14, ontology.AxiomDatatypeDefinition, 7, 0, "", `{"baseIri":"http://www.w3.org/2001/XMLSchema#integer","facets":[{"facet":"minInclusive","value":"0"}]}`, "", 0),
		// 注解属性
		mkAx(15, ontology.AxiomSubAnnotationPropertyOf, 6, 10, "", "", "", 0),
		// 个体
		mkAx(16, ontology.AxiomTypeAssertion, 8, 1, "", "", "", 0),
		mkAx(17, ontology.AxiomTypeAssertion, 8, 2, "", "", "", 1),
		mkAx(18, ontology.AxiomObjectPropertyAssertion, 8, 9, ns+"p", "", "", 0),
		mkAx(19, ontology.AxiomDataPropertyAssertion, 8, 5, "", "文本值", "zh", 0),
		mkAx(20, ontology.AxiomDataPropertyAssertion, 8, 5, "", "42", "", 0),
		mkAx(21, ontology.AxiomAnnotationAssertion, 8, 0, ontology.PredicateRDFSLabel, "个体一", "zh", 0),
		mkAx(22, ontology.AxiomAnnotationAssertion, 8, 0, ontology.PredicateSKOSNotation, "A0111", "", 0),
		mkAx(23, ontology.AxiomAnnotationAssertion, 8, 0, ontology.PredicateOWLDeprecated, "true", "", 0),
		mkAx(24, ontology.AxiomAnnotationAssertion, 8, 6, "", "自定义值", "", 0),
	}

	for _, format := range []Format{FormatTurtle, FormatRDFXML} {
		t.Run(string(format), func(t *testing.T) {
			triples, warnings := RowsToTriples(entities, axioms)
			if len(warnings) != 0 {
				t.Fatalf("导出告警：%v", warnings)
			}
			header := OntologyHeader{IRI: "http://example.com/ontology#", Version: 2, Description: "round-trip 样例"}
			triples = AppendHeaderTriples(triples, header)

			content, err := Write(triples, format, header.IRI)
			if err != nil {
				t.Fatalf("write: %v", err)
			}
			parsed, err := Parse(content)
			if err != nil {
				t.Fatalf("reparse(%s): %v\ncontent:\n%s", format, err, content)
			}
			plan, err := TriplesToPlan(parsed, "sample.owl")
			if err != nil {
				t.Fatalf("plan: %v", err)
			}
			if plan.Report.IgnoredCount != 1 || plan.Report.IgnoredByReason["本体 header 注解"] != 1 {
				t.Fatalf("round-trip 忽略项应仅 header 注解（versionInfo known-diff）：%v（%d）",
					plan.Report.IgnoredByReason, plan.Report.IgnoredCount)
			}
			if plan.Report.ImportsCount != 0 {
				t.Fatalf("header versionInfo 不应计为 imports")
			}

			// 实体集合：IRI + 类型相等
			gotEntities := map[string]uint32{}
			for _, e := range plan.Entities {
				gotEntities[e.IRI] = e.EntityType
			}
			for _, e := range entities {
				wantType := *e.EntityType
				gotType, ok := gotEntities[*e.Iri]
				if !ok {
					t.Fatalf("缺失实体 %s", *e.Iri)
				}
				if gotType != wantType {
					t.Fatalf("实体 %s 类型 %d != %d", *e.Iri, gotType, wantType)
				}
			}
			if len(gotEntities) != len(entities) {
				t.Fatalf("实体数量 %d != %d", len(gotEntities), len(entities))
			}

			// 公理集合语义相等（成对行 (A,B)+(B,A) 经无序对展开后行数不增）
			byID := map[uint32]*ent.OntEntity{}
			for _, e := range entities {
				byID[e.ID] = e
			}
			wantRows := map[semKey]int{}
			for _, ax := range axioms {
				wantRows[semKeyOfEntRow(ax, byID)]++
			}
			// 原样例的声明由实体行承载（10 行不入 axioms 列表）；导入侧为每实体补声明行（修订 #6）
			for _, e := range entities {
				wantRows[semKey{axiomType: ontology.AxiomDeclaration, subject: *e.Iri}]++
			}
			gotRows := map[semKey]int{}
			for _, d := range plan.Axioms {
				gotRows[semKeyOfDraft(d, plan)]++
			}
			for k, n := range wantRows {
				if gotRows[k] != n {
					t.Fatalf("公理漂移 %+v want %d got %d", k, n, gotRows[k])
				}
			}
			for k, n := range gotRows {
				if wantRows[k] != n {
					t.Fatalf("公理多出 %+v got %d want %d", k, n, wantRows[k])
				}
			}
		})
	}
}

func TestTriplesToPlanImportForms(t *testing.T) {
	t.Run("n-ary 两两展开计数", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix ex: <http://e#> .
			[ a owl:AllDisjointClasses ; owl:members ( ex:A ex:B ex:C ) ] .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		plan, err := TriplesToPlan(triples, "t.ttl")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		n := 0
		for _, d := range plan.Axioms {
			if d.AxiomType == ontology.AxiomDisjointWith {
				n++
			}
		}
		if n != 6 { // 3 成员 × 2 有序对
			t.Fatalf("AllDisjointClasses(3) 应展开 6 行，got %d", n)
		}
	})

	t.Run("文件单条成对公理落双行", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix ex: <http://e#> .
			ex:p owl:inverseOf ex:q .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		plan, err := TriplesToPlan(triples, "t.ttl")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		n := 0
		for _, d := range plan.Axioms {
			if d.AxiomType == ontology.AxiomInverseOf {
				n++
			}
		}
		if n != 2 {
			t.Fatalf("inverseOf 应落 (P,Q)+(Q,P) 双行，got %d", n)
		}
	})

	t.Run("类型冲突阻断", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix ex: <http://e#> .
			ex:X a owl:Class , owl:ObjectProperty .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if _, err := TriplesToPlan(triples, "t.ttl"); err == nil {
			t.Fatalf("同 IRI 双类型声明应阻断")
		} else if got := err.Error(); !contains(got, "类型声明冲突") {
			t.Fatalf("错误消息不符：%s", got)
		}
	})

	t.Run("localName 替补与 62 词形失败忽略", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
			@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
			@prefix ex: <http://e#> .
			ex:123456 a owl:NamedIndividual ; a ex:C .
			ex:C a owl:Class .
			ex:dp rdfs:range xsd:integer .
			ex:dp a owl:DatatypeProperty .
			ex:123456 ex:dp "不是数字" .
			ex:123456 ex:dp "42" .
			ex:123456 rdfs:label "数字开头局部名"@zh .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		plan, err := TriplesToPlan(triples, "t.ttl")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if len(plan.Report.RenamedLocalNames) != 1 || plan.Report.RenamedLocalNames[0].LocalName != "e1" {
			t.Fatalf("localName 替补不符：%+v", plan.Report.RenamedLocalNames)
		}
		// 词形失败进忽略清单（"不是数字"），"42" 落行
		if plan.Report.IgnoredByReason["词形与值域不符"] != 1 {
			t.Fatalf("词形失败应进忽略清单：%+v", plan.Report.IgnoredByReason)
		}
		n62 := 0
		for _, d := range plan.Axioms {
			if d.AxiomType == ontology.AxiomDataPropertyAssertion {
				n62++
			}
		}
		if n62 != 1 {
			t.Fatalf("62 行应恰 1 行落库，got %d", n62)
		}
	})

	t.Run("表达式公理进忽略清单", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
			@prefix ex: <http://e#> .
			ex:A rdfs:subClassOf [ a owl:Restriction ; owl:onProperty ex:p ; owl:someValuesFrom ex:B ] .
			ex:A a owl:Class . ex:B a owl:Class . ex:p a owl:ObjectProperty .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		plan, err := TriplesToPlan(triples, "t.ttl")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if plan.Report.IgnoredByReason["类表达式公理"] != 1 {
			t.Fatalf("someValuesFrom 应进忽略清单：%+v", plan.Report.IgnoredByReason)
		}
		for _, d := range plan.Axioms {
			if d.AxiomType == ontology.AxiomSubClassOf {
				t.Fatalf("含表达式的 subClassOf 不应落行")
			}
		}
	})

	t.Run("46 定义导入与 ParseDefinition 往返", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
			@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
			@prefix ex: <http://e#> .
			ex:SmallInt a rdfs:Datatype ;
				owl:equivalentClass [
					a rdfs:Datatype ;
					owl:onDatatype xsd:integer ;
					owl:withRestrictions ( [ xsd:minInclusive "0"^^xsd:integer ; xsd:maxInclusive "100"^^xsd:integer ] )
				] .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		plan, err := TriplesToPlan(triples, "t.ttl")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		var found *AxiomDraft
		for i := range plan.Axioms {
			if plan.Axioms[i].AxiomType == ontology.AxiomDatatypeDefinition {
				found = &plan.Axioms[i]
			}
		}
		if found == nil {
			t.Fatalf("46 定义行未落草稿：%+v", plan.Report.IgnoredByReason)
		}
		def, err := ontology.ParseDefinition(found.ValueText)
		if err != nil {
			t.Fatalf("ParseDefinition: %v", err)
		}
		if def.BaseIRI != "http://www.w3.org/2001/XMLSchema#integer" || len(def.Facets) != 2 {
			t.Fatalf("定义往返不符：%+v", def)
		}
	})

	t.Run("skos:notation 剥 lang 与 deprecated 非布尔忽略", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix skos: <http://www.w3.org/2004/02/skos/core#> .
			@prefix ex: <http://e#> .
			ex:i a owl:NamedIndividual ; a ex:C ;
				skos:notation "A0111"@zh ;
				owl:deprecated "maybe" .
			ex:C a owl:Class .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		plan, err := TriplesToPlan(triples, "t.ttl")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if plan.Report.IgnoredByReason["owl:deprecated 值非布尔"] != 1 {
			t.Fatalf("非布尔 deprecated 应忽略：%+v", plan.Report.IgnoredByReason)
		}
		for _, d := range plan.Axioms {
			if d.AxiomType == ontology.AxiomAnnotationAssertion && d.PredicateIRI == ontology.PredicateSKOSNotation {
				if d.Lang != "" {
					t.Fatalf("notation 的 lang 应剥离")
				}
			}
		}
		if len(plan.Report.Warnings) == 0 {
			t.Fatalf("剥 lang 应有 warning")
		}
	})

	t.Run("100 双轨分流（内建谓词 vs 注解属性实体）", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
			@prefix ex: <http://e#> .
			ex:ap a owl:AnnotationProperty .
			ex:C a owl:Class ;
				rdfs:label "内建轨" ;
				ex:ap "object_id 轨" .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		plan, err := TriplesToPlan(triples, "t.ttl")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		builtin, custom := 0, 0
		for _, d := range plan.Axioms {
			if d.AxiomType != ontology.AxiomAnnotationAssertion {
				continue
			}
			if d.PredicateIRI == ontology.PredicateRDFSLabel && d.PredicateRef < 0 {
				builtin++
			}
			if d.PredicateRef >= 0 && plan.Entities[d.PredicateRef].IRI == "http://e#ap" {
				custom++
			}
		}
		if builtin != 1 || custom != 1 {
			t.Fatalf("100 双轨分流失败：builtin=%d custom=%d", builtin, custom)
		}
	})

	t.Run("imports 计数与空文件", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix ex: <http://e#> .
			<http://ont.example.com#> a owl:Ontology ; owl:imports <http://other.example.com/core> .
			ex:A a owl:Class .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		plan, err := TriplesToPlan(triples, "t.ttl")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if plan.Report.ImportsCount != 1 {
			t.Fatalf("imports 计数不符：%d", plan.Report.ImportsCount)
		}
		if plan.OntologyIRI != "http://ont.example.com#" {
			t.Fatalf("ontology IRI 提取不符：%s", plan.OntologyIRI)
		}

		if _, err := TriplesToPlan(nil, "empty.ttl"); err == nil || !contains(err.Error(), "未发现可导入的实体") {
			t.Fatalf("空文件应报 OWL_IMPORT_EMPTY：%v", err)
		}
	})

	t.Run("众数命名空间推断", func(t *testing.T) {
		triples, err := ParseTurtle([]byte(`
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix ex: <http://main.example.com/ns#> .
			ex:A a owl:Class . ex:B a owl:Class .
			<http://outlier.example.com/x#O> a owl:Class .
		`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		plan, err := TriplesToPlan(triples, "t.ttl")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if plan.Report.SuggestedBaseIRI != "http://main.example.com/ns#" {
			t.Fatalf("众数命名空间推断不符：%s", plan.Report.SuggestedBaseIRI)
		}
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
