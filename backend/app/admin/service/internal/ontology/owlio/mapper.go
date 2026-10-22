package owlio

import (
	"sort"
	"strconv"
	"strings"

	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// mapper.go 导出方向（07 号文档 §2.2）：公理行 → 三元组，按 01~06 期行语义总表逐型产三元组。
// 同源纪律：号段常量、词法校验、定义 JSON 与校验器共享 internal/ontology 同一份定义。

// entityTypeIRIs 实体类型 → 声明 rdf:type 宾语 IRI（§3.2 总表首行）。
var entityTypeIRIs = map[uint32]string{
	ontology.EntityTypeClass:              IRIOWLClass,
	ontology.EntityTypeObjectProperty:     IRIOWLObjectProperty,
	ontology.EntityTypeDataProperty:       IRIOWLDataProperty,
	ontology.EntityTypeAnnotationProperty: IRIOWLAnnotationProperty,
	ontology.EntityTypeDatatype:           IRIRDFSDatatype,
	ontology.EntityTypeIndividual:         IRIOWLNamedIndividual,
}

// declaredIRIsOf 反向表：声明 IRI → 实体类型（导入侧判定用）。
var declaredIRIsOf = func() map[string]uint32 {
	out := make(map[string]uint32, len(entityTypeIRIs))
	for t, iri := range entityTypeIRIs {
		out[iri] = t
	}
	return out
}()

// OntologyHeader 导出的本体 header 信息（由服务层从 ent.OntOntology 提取，保持 mapper
// 对 ent 的依赖最小——仅公理/实体行直接用 ent 类型）。
type OntologyHeader struct {
	IRI         string
	Version     uint32
	Description string
}

// RowsToTriples 全量公理行 → 三元组（导出方向）。
//
// entities/axioms 须为本体下未删行全量快照（服务层单条 SELECT 提供）。返回追加的
// header 三元组之前的正文三元组（header 由 AppendHeaderTriples 追加）与告警
// （脏数据防御跳过的行；导出不因个别脏数据失败，warning 进服务日志）。
func RowsToTriples(entities []*ent.OntEntity, axioms []*ent.OntAxiom) ([]Triple, []string) {
	warnings := make([]string, 0, 4)
	warn := func(format string, args ...interface{}) {
		warnings = append(warnings, sprintf(format, args...))
	}

	byID := make(map[uint32]*ent.OntEntity, len(entities))
	for _, e := range entities {
		byID[e.ID] = e
	}
	iriOf := func(id uint32) string {
		if e, ok := byID[id]; ok && e.Iri != nil {
			return *e.Iri
		}
		return ""
	}

	triples := make([]Triple, 0, len(entities)+len(axioms)+8)

	// 1. 实体声明（10 行的行语义由实体行承载，导出按实体行产 rdf:type——每实体一条）
	for _, e := range entities {
		if e.EntityType == nil || e.Iri == nil {
			warn("实体 %d 缺少类型或 IRI，已跳过", e.ID)
			continue
		}
		typeIRI, ok := entityTypeIRIs[*e.EntityType]
		if !ok {
			warn("实体 %d 类型 %d 未知，已跳过", e.ID, *e.EntityType)
			continue
		}
		triples = append(triples, Triple{Subject: Iri(*e.Iri), Predicate: Iri(IRIRDFType), Object: Iri(typeIRI)})
	}

	// 2. 首条 42 值域（62 导出 typed literal 用；sort_order/id 最小，与校验器同口径）
	dtByProperty := propertyDatatypes(axioms, byID)

	// 3. 公理行（sort_order/id 稳定序，保证 60 主类目与 37 链序稳定）
	sorted := make([]*ent.OntAxiom, len(axioms))
	copy(sorted, axioms)
	sort.SliceStable(sorted, func(i, j int) bool { return axiomBefore(sorted[i], sorted[j]) })

	for _, ax := range sorted {
		if ax.AxiomType == nil {
			continue
		}
		subject := ""
		if ax.SubjectID != nil {
			subject = iriOf(*ax.SubjectID)
		}
		if subject == "" {
			warn("公理 %d 主语实体缺失，已跳过", ax.ID)
			continue
		}
		object := ""
		if ax.ObjectID != nil && *ax.ObjectID != 0 {
			object = iriOf(*ax.ObjectID)
		}
		value := ""
		if ax.ValueText != nil {
			value = *ax.ValueText
		}
		lang := ""
		if ax.Lang != nil {
			lang = *ax.Lang
		}
		predicate := ""
		if ax.PredicateIri != nil {
			predicate = *ax.PredicateIri
		}

		switch *ax.AxiomType {
		case ontology.AxiomDeclaration:
			// 声明行语义由实体行承载（步骤 1），不重复输出
		case ontology.AxiomSubClassOf:
			if object == "" {
				warn("公理 %d 缺少宾语，已跳过", ax.ID)
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIRDFSSubClassOf), Object: Iri(object)})
		case ontology.AxiomDisjointWith:
			if object == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIOWLDisjointWith), Object: Iri(object)})
		case ontology.AxiomEquivalentClasses:
			if object == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIOWLEquivalentClass), Object: Iri(object)})
		case ontology.AxiomSubObjectPropertyOf, ontology.AxiomSubDataPropertyOf, ontology.AxiomSubAnnotationPropertyOf:
			if object == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIRDFSSubPropertyOf), Object: Iri(object)})
		case ontology.AxiomObjectPropertyDomain, ontology.AxiomDataPropertyDomain:
			if object == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIRDFSDomain), Object: Iri(object)})
		case ontology.AxiomObjectPropertyRange:
			if object == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIRDFSRange), Object: Iri(object)})
		case ontology.AxiomDataPropertyRange:
			// 42 双轨：内建轨 value_text / 派生轨 datatype 实体 IRI
			iri := value
			if object != "" {
				iri = object
			}
			if iri == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIRDFSRange), Object: Iri(iri)})
		case ontology.AxiomObjectPropertyCharacteristic, ontology.AxiomDataPropertyCharacteristic:
			if predicate == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIRDFType), Object: Iri(predicate)})
		case ontology.AxiomInverseOf:
			if object == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIOWLInverseOf), Object: Iri(object)})
		case ontology.AxiomEquivalentObjectProperties, ontology.AxiomEquivalentDataProperties:
			if object == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIOWLEquivalentProperty), Object: Iri(object)})
		case ontology.AxiomDisjointObjectProperties, ontology.AxiomDisjointDataProperties:
			if object == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(IRIOWLPropertyDisjointWith), Object: Iri(object)})
		case ontology.AxiomSubPropertyChainOf:
			chainTriples, ok := chainToTriples(subject, value, byID, ax.ID, warn)
			if ok {
				triples = append(triples, chainTriples...)
			}
		case ontology.AxiomDatatypeDefinition:
			defTriples, ok := definitionToTriples(subject, value, ax.ID, warn)
			if ok {
				triples = append(triples, defTriples...)
			}
		case ontology.AxiomTypeAssertion:
			if object == "" {
				continue
			}
			triples = append(triples, Triple{
				Subject:   Iri(subject),
				Predicate: Iri(IRIRDFType),
				Object:    Iri(object),
			})
		case ontology.AxiomObjectPropertyAssertion:
			if object == "" || predicate == "" {
				continue
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(predicate), Object: Iri(object)})
		case ontology.AxiomDataPropertyAssertion:
			// 62 行模型谓词存 object_id（数据属性实体）：导出按实体 IRI 产三元组
			predIRI := predicate
			if predIRI == "" {
				predIRI = object
			}
			if predIRI == "" {
				continue
			}
			if lang != "" {
				triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(predIRI), Object: LangLit(value, lang)})
				continue
			}
			if dt := dtByProperty[nilUintOf(ax.ObjectID)]; dt != "" {
				triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(predIRI), Object: TypedLit(value, dt)})
			} else {
				triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(predIRI), Object: Lit(value)})
			}
		case ontology.AxiomAnnotationAssertion:
			// 内建轨 predicate_iri / 自定义轨 object_id（注解属性实体）
			iri := predicate
			if iri == "" {
				iri = object
			}
			if iri == "" || value == "" {
				warn("公理 %d 注解断言缺少谓词或值，已跳过", ax.ID)
				continue
			}
			obj := Lit(value)
			if iri == ontology.PredicateOWLDeprecated {
				obj = TypedLit(value, NSXSD+"boolean") // 源修订 #5：plain literal 对外部工具语义不正确
			} else if lang != "" {
				obj = LangLit(value, lang)
			}
			triples = append(triples, Triple{Subject: Iri(subject), Predicate: Iri(iri), Object: obj})
		default:
			warn("公理 %d 类型 %d 未知，已跳过", ax.ID, *ax.AxiomType)
		}
	}
	return triples, warnings
}

// AppendHeaderTriples 本体 header 三元组（§2.2：rdfs:comment = description、
// owl:versionInfo = 版本号字符串；导入侧不落库、进忽略清单——round-trip 时报 known-diff）。
func AppendHeaderTriples(triples []Triple, header OntologyHeader) []Triple {
	if header.IRI == "" {
		return triples
	}
	out := make([]Triple, 0, len(triples)+3)
	out = append(out, Triple{Subject: Iri(header.IRI), Predicate: Iri(IRIRDFType), Object: Iri(IRIOWLOntology)})
	if header.Description != "" {
		out = append(out, Triple{Subject: Iri(header.IRI), Predicate: Iri(IRIRDFSComment), Object: Lit(header.Description)})
	}
	if header.Version > 0 {
		out = append(out, Triple{Subject: Iri(header.IRI), Predicate: Iri(IRIOWLVersionInfo), Object: Lit("v" + strconv.FormatUint(uint64(header.Version), 10))})
	}
	return append(out, triples...)
}

// propertyDatatypes 首条 42 行 → 数据属性的 typed literal datatype IRI（内建轨取
// value_text；派生轨取 datatype 实体 IRI；无 42 行/脏行返回空 = plain 缺省）。
func propertyDatatypes(axioms []*ent.OntAxiom, byID map[uint32]*ent.OntEntity) map[uint32]string {
	out := map[uint32]string{}
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyRange {
			continue
		}
		if ax.SubjectID == nil {
			continue
		}
		id := *ax.SubjectID
		if _, seen := out[id]; seen {
			continue
		}
		if v := nilStringOf(ax.ValueText); v != "" {
			if _, ok := ontology.IsBuiltinDatatype(v); ok {
				out[id] = v
				continue
			}
		}
		if id2 := nilUintOf(ax.ObjectID); id2 != 0 {
			if e, ok := byID[id2]; ok && e.Iri != nil {
				out[id] = *e.Iri
				continue
			}
		}
		out[id] = "" // 首行脏数据：占位防止次行顶替（缺省 plain 口径）
	}
	return out
}

// chainToTriples 37 链导出：value_text JSON id 数组 → owl:propertyChainAxiom + first/rest 集合。
func chainToTriples(subject, value string, byID map[uint32]*ent.OntEntity, axiomID uint32, warn func(string, ...interface{})) ([]Triple, bool) {
	members, err := ontology.ParseChainMemberIDs(value)
	if err != nil || len(members) < 2 {
		warn("公理 %d 的属性链成员不合法，已跳过", axiomID)
		return nil, false
	}
	iris := make([]string, 0, len(members))
	for _, id := range members {
		e, ok := byID[id]
		if !ok || e.Iri == nil || *e.Iri == "" {
			warn("公理 %d 的属性链成员 %d 实体缺失，已跳过", axiomID, id)
			return nil, false
		}
		iris = append(iris, *e.Iri)
	}
	head := Blank("chain" + strconv.FormatUint(uint64(axiomID), 10))
	out := []Triple{{Subject: Iri(subject), Predicate: Iri(IRIOWLPropertyChainAxiom), Object: head}}
	cur := head
	for i, iri := range iris {
		out = append(out, Triple{Subject: cur, Predicate: Iri(IRIRDFFirst), Object: Iri(iri)})
		if i == len(iris)-1 {
			out = append(out, Triple{Subject: cur, Predicate: Iri(IRIRDFRest), Object: Iri(IRIRDFNil)})
		} else {
			next := Blank("chain" + strconv.FormatUint(uint64(axiomID), 10) + "_" + strconv.Itoa(i+1))
			out = append(out, Triple{Subject: cur, Predicate: Iri(IRIRDFRest), Object: next})
			cur = next
		}
	}
	return out, true
}

// definitionToTriples 46 定义导出：定义 JSON → owl:equivalentClass restriction 子图
// （facet 值按基类型 typed literal——源修订 #3，避免 round-trip datatype 漂移）。
func definitionToTriples(subject, value string, axiomID uint32, warn func(string, ...interface{})) ([]Triple, bool) {
	def, err := ontology.ParseDefinition(value)
	if err != nil {
		warn("公理 %d 的数据类型定义不合法，已跳过", axiomID)
		return nil, false
	}
	if strings.TrimSpace(def.BaseIRI) == "" {
		warn("公理 %d 的数据类型定义缺少基类型，已跳过", axiomID)
		return nil, false
	}
	dt := Blank("dtdef" + strconv.FormatUint(uint64(axiomID), 10))
	out := []Triple{
		{Subject: Iri(subject), Predicate: Iri(IRIOWLEquivalentClass), Object: dt},
		{Subject: dt, Predicate: Iri(IRIRDFType), Object: Iri(IRIRDFSDatatype)},
		{Subject: dt, Predicate: Iri(IRIOWLOnDatatype), Object: Iri(strings.TrimSpace(def.BaseIRI))},
	}
	if len(def.Facets) == 0 {
		return out, true
	}
	base, _ := ontology.IsBuiltinDatatype(strings.TrimSpace(def.BaseIRI))
	// withRestrictions 集合：dt → cell0 → first facet0 / rest cell1 … cellN rest rdf:nil
	cells := make([]Term, len(def.Facets))
	facetNodes := make([]Term, len(def.Facets))
	for i := range def.Facets {
		cells[i] = Blank(sprintf("dtdef%d_c%d", axiomID, i))
		facetNodes[i] = Blank(sprintf("dtdef%d_e%d", axiomID, i))
	}
	out = append(out, Triple{Subject: dt, Predicate: Iri(IRIOWLWithRestrictions), Object: cells[0]})
	for i, cell := range cells {
		if i < len(cells)-1 {
			out = append(out, Triple{Subject: cell, Predicate: Iri(IRIRDFRest), Object: cells[i+1]})
		} else {
			out = append(out, Triple{Subject: cell, Predicate: Iri(IRIRDFRest), Object: Iri(IRIRDFNil)})
		}
		out = append(out, Triple{Subject: cell, Predicate: Iri(IRIRDFFirst), Object: facetNodes[i]})
		// facet 节点：<facetIRI> literal
		facetIRI := NSXSD + def.Facets[i].Facet
		out = append(out, Triple{Subject: facetNodes[i], Predicate: Iri(facetIRI), Object: facetLiteral(base, def.Facets[i])})
	}
	return out, true
}

// facetLiteral facet 值输出形态：数值比较 facet 按基类型 typed；位数 facet typed xsd:integer；
// 字符串 facet plain。
func facetLiteral(base ontology.BuiltinDatatype, f ontology.DatatypeFacet) Term {
	switch f.Facet {
	case "minInclusive", "minExclusive", "maxInclusive", "maxExclusive":
		if base.IRI != "" {
			return TypedLit(f.Value, base.IRI)
		}
		return Lit(f.Value)
	case "totalDigits", "fractionDigits":
		return TypedLit(f.Value, NSXSD+"integer")
	default:
		return Lit(f.Value)
	}
}

func axiomBefore(a, b *ent.OntAxiom) bool {
	soA, soB := int32(0), int32(0)
	if a.SortOrder != nil {
		soA = int32(*a.SortOrder)
	}
	if b.SortOrder != nil {
		soB = int32(*b.SortOrder)
	}
	if soA != soB {
		return soA < soB
	}
	return a.ID < b.ID
}

func nilStringOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nilUintOf(p *uint32) uint32 {
	if p == nil {
		return 0
	}
	return *p
}
