// Package owlio OWL 导入导出的 RDF 读写层与三元组映射层（07 号文档 §2.1/§2.2）。
//
// 统一三元组中间表示：两个 parser（RDF/XML / Turtle）都产出 []Triple，
// 两个 writer 都消费 []Triple；mapper.go 在三元组与本体公理行（01~06 期行语义）之间
// 双向映射。包内除 mapper/plan 依赖 internal/ontology 与 ent 外，读写器为纯函数、可独立单测。
package owlio

// TermKind RDF 项类别。
type TermKind uint8

const (
	TermIRI TermKind = iota // IRI 全文存 Value
	TermLiteral             // 字面量词形存 Value；Datatype 为 datatype IRI（空 = plain/rdf:langString）
	TermBlank               // 空白节点标签存 Value
)

// Term RDF 项（IRI / 字面量 / 空白节点）。
type Term struct {
	Kind     TermKind
	Value    string
	Datatype string // 字面量 datatype IRI（空 = plain；lang 非空时语义为 rdf:langString）
	Lang     string // 语言标签（可空）
}

// Triple 一条 RDF 三元组。
type Triple struct {
	Subject   Term
	Predicate Term
	Object    Term
}

// 常用 RDF 词汇命名空间（前缀按 OWL 生态惯例）。
const (
	NSRDF  = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	NSRDFS = "http://www.w3.org/2000/01/rdf-schema#"
	NSOWL  = "http://www.w3.org/2002/07/owl#"
	NSXSD  = "http://www.w3.org/2001/XMLSchema#"
	NSSKOS = "http://www.w3.org/2004/02/skos/core#"

	XMLNSLang = "http://www.w3.org/XML/1998/namespace" // xml:lang 的命名空间
)

// 解析器 / 映射器关注的词汇 IRI。
const (
	IRIRDFType       = NSRDF + "type"
	IRIRDFFirst      = NSRDF + "first"
	IRIRDFRest       = NSRDF + "rest"
	IRIRDFNil        = NSRDF + "nil"
	IRIRDFRDF        = NSRDF + "RDF"           // rdf:RDF 根元素
	IRIRDFDescription = NSRDF + "Description"  // rdf:Description 节点元素
	IRIRDFAbout      = NSRDF + "about"
	IRIRDFResource   = NSRDF + "resource"
	IRIRDFNodeID     = NSRDF + "nodeID"
	IRIRDFDatatype   = NSRDF + "datatype"
	IRIRDFParseType  = NSRDF + "parseType"
	IRIRDFID         = NSRDF + "ID"

	IRIRDFSSubClassOf      = NSRDFS + "subClassOf"
	IRIRDFSSubPropertyOf   = NSRDFS + "subPropertyOf"
	IRIRDFSDomain          = NSRDFS + "domain"
	IRIRDFSRange           = NSRDFS + "range"
	IRIRDFSLabel           = NSRDFS + "label"
	IRIRDFSComment         = NSRDFS + "comment"
	IRIRDFSDatatype        = NSRDFS + "Datatype"

	IRIOWLClass              = NSOWL + "Class"
	IRIOWLObjectProperty     = NSOWL + "ObjectProperty"
	IRIOWLDataProperty       = NSOWL + "DatatypeProperty" // OWL 2 词汇为 owl:DatatypeProperty（Protégé 识别键）
	IRIOWLAnnotationProperty = NSOWL + "AnnotationProperty"
	IRIOWLOntology           = NSOWL + "Ontology"
	IRIOWLNamedIndividual    = NSOWL + "NamedIndividual"
	IRIOWLRestriction        = NSOWL + "Restriction"
	IRIOWLAllDisjointClasses    = NSOWL + "AllDisjointClasses"
	IRIOWLAllDisjointProperties = NSOWL + "AllDisjointProperties"
	IRIOWLMembers            = NSOWL + "members"
	IRIOWLDisjointWith       = NSOWL + "disjointWith"
	IRIOWLEquivalentClass    = NSOWL + "equivalentClass"
	IRIOWLInverseOf          = NSOWL + "inverseOf"
	IRIOWLEquivalentProperty = NSOWL + "equivalentProperty"
	IRIOWLPropertyDisjointWith = NSOWL + "propertyDisjointWith"
	IRIOWLPropertyChainAxiom = NSOWL + "propertyChainAxiom"
	IRIOWLOnDatatype         = NSOWL + "onDatatype"
	IRIOWLWithRestrictions   = NSOWL + "withRestrictions"
	IRIOWLImports            = NSOWL + "imports"
	IRIOWLVersionInfo        = NSOWL + "versionInfo"
	IRIOWLDeprecated         = NSOWL + "deprecated"
	IRIOWLAnnotatedSource    = NSOWL + "annotatedSource"
	IRIOWLAnnotatedProperty  = NSOWL + "annotatedProperty"
	IRIOWLAnnotatedTarget    = NSOWL + "annotatedTarget"

	IRISKOSNotation = NSSKOS + "notation"
)

// expressionVocabulary 表达式识别词汇表（§3.2 表达式识别规则：宾语是空白节点且该节点的
// 类型/属性命中本表 → 该三元组及关联子图进忽略清单；owl:onDatatype/withRestrictions 不在其中
// ——46 datatype restriction 是唯一放行形态。subClassOf/equivalentClass/disjointWith/rdf:type
// 本身不在此表：表达式判定在 mapper 的"宾语为空白节点"专用逻辑里做）。
var expressionVocabulary = map[string]struct{}{
	NSOWL + "Restriction":             {},
	NSOWL + "onProperty":              {},
	NSOWL + "someValuesFrom":          {},
	NSOWL + "allValuesFrom":           {},
	NSOWL + "hasValue":                {},
	NSOWL + "hasSelf":                 {},
	NSOWL + "cardinality":             {},
	NSOWL + "minCardinality":          {},
	NSOWL + "maxCardinality":          {},
	NSOWL + "qualifiedCardinality":    {},
	NSOWL + "minQualifiedCardinality": {},
	NSOWL + "maxQualifiedCardinality": {},
	NSOWL + "unionOf":                 {},
	NSOWL + "intersectionOf":          {},
	NSOWL + "complementOf":            {},
	NSOWL + "oneOf":                   {},
	NSOWL + "onDataRange":             {},
	NSOWL + "datatypeComplementOf":    {},
}

// XSD facet IRI → facet 本地名（withRestrictions 元素属性元素用；与 internal/ontology
// FacetWhitelist 词形天然一致，无需再映射）。
var xsdFacetIRIs = map[string]string{
	NSXSD + "length":         "length",
	NSXSD + "minLength":      "minLength",
	NSXSD + "maxLength":      "maxLength",
	NSXSD + "pattern":        "pattern",
	NSXSD + "minInclusive":   "minInclusive",
	NSXSD + "minExclusive":   "minExclusive",
	NSXSD + "maxInclusive":   "maxInclusive",
	NSXSD + "maxExclusive":   "maxExclusive",
	NSXSD + "totalDigits":    "totalDigits",
	NSXSD + "fractionDigits": "fractionDigits",
	// 白名单外但 XSD 官方词汇：识别出名字后交 FacetWhitelist 拒收（进忽略清单）
	NSXSD + "whiteSpace": "whiteSpace",
	NSXSD + "langRange":  "langRange",
}

// Iri 构造 IRI 项。
func Iri(v string) Term { return Term{Kind: TermIRI, Value: v} }

// Lit 构造 plain literal（无 datatype 无 lang）。
func Lit(v string) Term { return Term{Kind: TermLiteral, Value: v} }

// TypedLit 构造 typed literal。
func TypedLit(v, datatype string) Term { return Term{Kind: TermLiteral, Value: v, Datatype: datatype} }

// LangLit 构造带语言标签的 literal。
func LangLit(v, lang string) Term { return Term{Kind: TermLiteral, Value: v, Lang: lang} }

// Blank 构造空白节点项。
func Blank(label string) Term { return Term{Kind: TermBlank, Value: label} }

// isExpressionVocabulary 判定 IRI 是否命中表达式词汇表。
func isExpressionVocabulary(iri string) bool {
	_, ok := expressionVocabulary[iri]
	return ok
}

// lastIRISegment 取 IRI 最后一个 '#' 后段，无 '#' 取最后一个 '/' 后段（§2.3 #3 localName 提取口径）。
func lastIRISegment(iri string) string {
	if idx := lastByteIndex(iri, '#'); idx >= 0 {
		return iri[idx+1:]
	}
	return iri[lastByteIndex(iri, '/')+1:]
}

func lastByteIndex(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}
