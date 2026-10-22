package ontology

import (
	"strconv"
	"strings"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// Manchester OWL Syntax 子集渲染器（设计文档 §3.6）：
// SubClassOf(设备 冷水机组)、DisjointWith(A B)、EquivalentClasses(A B)、
// Declaration(Class(A))、Annotation(rdfs:label "..."@zh) 形式的公理描述串。
// V1 供 frame 详情实时计算展示，不落库（预留 ont_entities 未来加 render_cache 列的升级路径）。

// builtinPredicateShortForms 内建谓词 IRI → 紧缩名（渲染用；其余谓词渲染完整 IRI）。
var builtinPredicateShortForms = map[string]string{
	PredicateRDFSLabel:     "rdfs:label",
	PredicateRDFSComment:   "rdfs:comment",
	PredicateOWLDeprecated: "owl:deprecated",
	PredicateSKOSNotation:  "skos:notation",
}

// EntityDisplayName 实体显示名（label 优先 shortForm 的渲染约定）：display_label 回退 local_name。
func EntityDisplayName(e *ent.OntEntity) string {
	return displayLabelOf(e)
}

// ShortFormPredicate 谓词 IRI 的渲染形式（内建谓词用前缀紧缩名）。
func ShortFormPredicate(iri string) string {
	if sf, ok := builtinPredicateShortForms[iri]; ok {
		return sf
	}
	return iri
}

// RenderAxiom 渲染单条公理。entityNames: 实体 id → 显示名（frame 计算时由实体列表构建；可为 nil，
// 届时以 id 数字渲染）。
func RenderAxiom(ax *ent.OntAxiom, entityNames map[uint32]string) string {
	entityName := func(id uint32) string {
		if n, ok := entityNames[id]; ok && n != "" {
			return n
		}
		return strconvFormatUint(id)
	}
	subject := uint32(0)
	if ax.SubjectID != nil {
		subject = *ax.SubjectID
	}

	if ax.AxiomType == nil {
		return ""
	}
	switch *ax.AxiomType {
	case AxiomDeclaration:
		return "Declaration(Class(" + entityName(subject) + "))"
	case AxiomSubClassOf:
		return "SubClassOf(" + entityName(subject) + " " + entityName(nilUint(ax.ObjectID)) + ")"
	case AxiomDisjointWith:
		return "DisjointWith(" + entityName(subject) + " " + entityName(nilUint(ax.ObjectID)) + ")"
	case AxiomEquivalentClasses:
		return "EquivalentClasses(" + entityName(subject) + " " + entityName(nilUint(ax.ObjectID)) + ")"
	case AxiomSubObjectPropertyOf:
		return "SubPropertyOf(" + entityName(subject) + " " + entityName(nilUint(ax.ObjectID)) + ")"
	case AxiomObjectPropertyDomain:
		return "Domain(" + entityName(subject) + " " + entityName(nilUint(ax.ObjectID)) + ")"
	case AxiomObjectPropertyRange:
		return "Range(" + entityName(subject) + " " + entityName(nilUint(ax.ObjectID)) + ")"
	case AxiomObjectPropertyCharacteristic:
		return "Characteristic(" + entityName(subject) + ", " + CharacteristicShortName(nilString(ax.PredicateIri)) + ")"
	case AxiomInverseOf, AxiomEquivalentObjectProperties, AxiomDisjointObjectProperties:
		// 对称成对行按无序对去重展示：仅渲染 subject < object 一侧（两条镜像行只出一条，
		// 与源 OWLAPI 集合去重语义等价，02 号文档 §4.4）
		object := nilUint(ax.ObjectID)
		if subject > object {
			return ""
		}
		name := ""
		switch *ax.AxiomType {
		case AxiomInverseOf:
			name = "InverseOf"
		case AxiomEquivalentObjectProperties:
			name = "EquivalentProperties"
		default:
			name = "DisjointProperties"
		}
		return name + "(" + entityName(subject) + " " + entityName(object) + ")"
	case AxiomSubPropertyChainOf:
		// 链按 value_text 保序渲染：SubPropertyChainOf(P1 ∘ P2 P)；成员解析失败回退占位
		var b strings.Builder
		b.WriteString("SubPropertyChainOf(")
		if ax.ValueText != nil {
			if members, err := ParseChainMemberIDs(*ax.ValueText); err == nil {
				names := make([]string, 0, len(members))
				for _, m := range members {
					names = append(names, entityName(m))
				}
				b.WriteString(strings.Join(names, " ∘ "))
				b.WriteString(" ")
			} else {
				b.WriteString("<invalid-chain> ∘ ")
			}
		}
		b.WriteString(entityName(subject))
		b.WriteString(")")
		return b.String()
	case AxiomAnnotationAssertion:
		return renderAnnotation(entityName(subject), nilString(ax.PredicateIri), nilString(ax.ValueText), nilString(ax.Lang))
	case AxiomSubDataPropertyOf:
		return "SubPropertyOf(" + entityName(subject) + " " + entityName(nilUint(ax.ObjectID)) + ")"
	case AxiomDataPropertyDomain:
		return "Domain(" + entityName(subject) + " " + entityName(nilUint(ax.ObjectID)) + ")"
	case AxiomDataPropertyRange:
		// 值域渲染 datatype 短名（value_text = 内建 datatype IRI，03 号文档 §4.4）
		return "Range(" + entityName(subject) + " " + DatatypeShortName(nilString(ax.ValueText)) + ")"
	case AxiomDataPropertyCharacteristic:
		return "Characteristic(" + entityName(subject) + ", " + CharacteristicShortName(nilString(ax.PredicateIri)) + ")"
	case AxiomEquivalentDataProperties, AxiomDisjointDataProperties:
		// 成对行按无序对去重展示（同 34/35/36）
		object := nilUint(ax.ObjectID)
		if subject > object {
			return ""
		}
		name := "EquivalentDataProperties"
		if *ax.AxiomType == AxiomDisjointDataProperties {
			name = "DisjointDataProperties"
		}
		return name + "(" + entityName(subject) + " " + entityName(object) + ")"
	case AxiomDataPropertyAssertion:
		// 62 数据断言（06 期个体 frame 复用）：RenderAxiom 单行入口不带值域上下文，
		// 字面量渲染为裸词形（带 lang 则 @lang）；^^datatype 后缀走 RenderDataPropertyAssertion
		return RenderDataPropertyAssertion(ax, entityNames, nil)
	default:
		// 未知号段（03~06 期启用后各自补渲染分支）
		return "Axiom(type=" + strconvFormatUint(*ax.AxiomType) + " subject=" + entityName(subject) + ")"
	}
}

func renderAnnotation(subject, predicate, value, lang string) string {
	var b strings.Builder
	b.WriteString("Annotation(")
	b.WriteString(ShortFormPredicate(predicate))
	b.WriteString(" ")
	b.WriteString(subject)
	b.WriteString(" ")
	b.WriteString(quoteValue(value))
	if lang != "" {
		b.WriteString("@")
		b.WriteString(lang)
	}
	b.WriteString(")")
	return b.String()
}

// LiteralDatatypeOf 62 断言字面量 datatype 推断钩子（数据属性实体 id → datatype IRI）。
// 06 期个体 frame 渲染时由调用方按该属性的值域行（42 型 value_text，缺省 xsd:string）传入。
type LiteralDatatypeOf func(propertyID uint32) string

// RenderDataPropertyAssertion 62 数据断言渲染：DataPropertyAssertion(个体 P "词形"^^datatype)。
// datatypeOf 非 nil 且命中内建白名单时字面量带 ^^datatype 后缀；lang 有值按语言文本渲染（@lang，
// rdf:langString 不落 ^^ 后缀）。RenderAxiom 的 62 分支以 nil 钩子委托至此。
func RenderDataPropertyAssertion(ax *ent.OntAxiom, entityNames map[uint32]string, datatypeOf LiteralDatatypeOf) string {
	if ax == nil || ax.AxiomType == nil || *ax.AxiomType != AxiomDataPropertyAssertion {
		return ""
	}
	entityName := func(id uint32) string {
		if n, ok := entityNames[id]; ok && n != "" {
			return n
		}
		return strconvFormatUint(id)
	}
	subject := uint32(0)
	if ax.SubjectID != nil {
		subject = *ax.SubjectID
	}
	var b strings.Builder
	b.WriteString("DataPropertyAssertion(")
	b.WriteString(entityName(subject))
	b.WriteString(" ")
	b.WriteString(entityName(nilUint(ax.ObjectID)))
	b.WriteString(" ")
	value := nilString(ax.ValueText)
	b.WriteString(quoteValue(value))
	if lang := nilString(ax.Lang); lang != "" {
		b.WriteString("@")
		b.WriteString(lang)
	} else if datatypeOf != nil {
		if dt := datatypeOf(nilUint(ax.ObjectID)); dt != "" {
			if _, ok := IsBuiltinDatatype(dt); ok {
				b.WriteString("^^")
				b.WriteString(DatatypeShortName(dt))
			}
		}
	}
	b.WriteString(")")
	return b.String()
}

// DatatypeShortName datatype IRI 的紧缩渲染名（http://www.w3.org/2001/XMLSchema#integer →
// xsd:integer）；白名单外返回完整 IRI。
func DatatypeShortName(iri string) string {
	if _, ok := IsBuiltinDatatype(iri); ok {
		return "xsd:" + strings.TrimPrefix(iri, IRIXSDNamespace)
	}
	return iri
}

func quoteValue(v string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}

func nilUint(p *uint32) uint32 {
	if p == nil {
		return 0
	}
	return *p
}

func strconvFormatUint(v uint32) string {
	return strconv.FormatUint(uint64(v), 10)
}
