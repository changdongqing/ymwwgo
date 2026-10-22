// Package ontology 本体建模域内核（非 HTTP、非仓储；纯 Go 可单测）。
//
// 六项本体建模功能（类/对象属性/数据属性/注解属性/数据类型/个体）共用的
// 工厂、公理变更事务、层级构建、结构校验与 Manchester 渲染都在此包内增量扩展。
// 依赖方向：本包只依赖 ent 生成类型与标准库（仓储需求以小接口注入，便于单测桩替换）；
// internal/data（repo）与 internal/service（HTTP 编排）依赖它，反向禁止。
// 设计文档：docs/ontology/design/01-阶段0前置与本体类详细设计.md §3.0。
package ontology

import (
	"context"
	"fmt"
)

// 实体类型号段（与 proto OntologyEntityType / 源项目 OntEntityTypeEnum 逐值一致）。
// 02~06 期逐期启用预留号段时无需改 schema 重跑 ent 生成。
const (
	EntityTypeClass              uint32 = 10 // 本体类（OWLClass）
	EntityTypeObjectProperty     uint32 = 20 // 对象属性（02 期）
	EntityTypeDataProperty       uint32 = 30 // 数据属性（03 期）
	EntityTypeAnnotationProperty uint32 = 40 // 注解属性（04 期）
	EntityTypeDatatype           uint32 = 50 // 数据类型（05 期）
	EntityTypeIndividual         uint32 = 60 // 个体对象（06 期）
)

// 公理类型号段（与 proto OntologyAxiomType 逐值一致）。
// 未知号段（03~06 期尚未启用）在校验器中 WARN 跳过、不报错。
const (
	AxiomDeclaration                  uint32 = 10  // 实体声明
	AxiomSubClassOf                   uint32 = 20  // 子类（多继承多条，subject=子类，object=父类）
	AxiomDisjointWith                 uint32 = 21  // 不相交（成对存储：(A,B) 与 (B,A) 各一行）
	AxiomEquivalentClasses            uint32 = 22  // 等价（成对存储，V1 命名类对）
	AxiomSubObjectPropertyOf          uint32 = 30  // 子对象属性（subject=子属性，object_id=父属性）
	AxiomObjectPropertyDomain         uint32 = 31  // 对象属性定义域（subject=属性，object_id=类；多行=多定义域，OWL 交集语义）
	AxiomObjectPropertyRange          uint32 = 32  // 对象属性值域（同上）
	AxiomObjectPropertyCharacteristic uint32 = 33  // 对象属性特征（泛型：subject=属性，predicate_iri=特征 IRI，每特征一行）
	AxiomInverseOf                    uint32 = 34  // 逆属性（成对 (P,Q)+(Q,P) 各一行）
	AxiomEquivalentObjectProperties   uint32 = 35  // 等价属性（成对；frame 单值暴露，存储为成对）
	AxiomDisjointObjectProperties     uint32 = 36  // 不相交属性（成对）
	AxiomSubPropertyChainOf           uint32 = 37  // 属性链（subject=超属性，value_text=链成员实体 id 的 JSON 字符串数组，保序）
	AxiomSubDataPropertyOf            uint32 = 40  // 子数据属性（subject=子属性，object_id=父属性）
	AxiomDataPropertyDomain           uint32 = 41  // 数据属性定义域（subject=属性，object_id=类；多行=多定义域，OWL 交集语义）
	AxiomDataPropertyRange            uint32 = 42  // 数据属性值域（多行=多值域；05 期双轨激活：内建轨 value_text=内建 datatype IRI / 派生轨 object_id=datatype 实体 id，二选一）
	AxiomDataPropertyCharacteristic   uint32 = 43  // 数据属性特征（泛型：subject=属性，predicate_iri=特征 IRI；V1 仅函数性）
	AxiomEquivalentDataProperties     uint32 = 44  // 等价数据属性（成对 (P,Q)+(Q,P) 各一行）
	AxiomDisjointDataProperties       uint32 = 45  // 不相交数据属性（成对）
	AxiomDatatypeDefinition           uint32 = 46  // 数据类型定义（05 期：subject=datatype 实体，value_text=定义 JSON，至多一行；47~49 继续预留）
	AxiomSubAnnotationPropertyOf      uint32 = 50  // 04：子注解属性（subject=子属性，object_id=父属性；多行=多父）
	AxiomTypeAssertion                uint32 = 60  // 06：类型断言（subject=个体，object_id=类型类实体 id，sort_order=类型序，0=主类目）
	AxiomObjectPropertyAssertion      uint32 = 61  // 06：对象属性断言（subject=主语个体，predicate_iri=对象属性 IRI，object_id=宾语个体；不成对存储）
	AxiomDataPropertyAssertion        uint32 = 62  // 数据属性断言（06 期写入方：subject=个体，object_id=数据属性实体，value_text=字面量词形；03 期定稿行语义 + 校验器分支）
	AxiomAnnotationAssertion          uint32 = 100 // 注解断言，全功能共用
)

// 内建注解谓词 IRI（注解断言 V1 白名单；04 期自定义注解属性激活 object_id 双轨后放开）
const (
	PredicateRDFSLabel     = "http://www.w3.org/2000/01/rdf-schema#label"
	PredicateRDFSComment   = "http://www.w3.org/2000/01/rdf-schema#comment"
	PredicateOWLDeprecated = "http://www.w3.org/2002/07/owl#deprecated"
	PredicateSKOSNotation  = "http://www.w3.org/2004/02/skos/core#notation"
)

// IRIOWLThing owl:Thing 虚拟根：不入实体表，由前端固定渲染（源修订 #5，渲染关注点）
const IRIOWLThing = "http://www.w3.org/2002/07/owl#Thing"

// IRIOWLTopObjectProperty owl:topObjectProperty 虚拟根（对象属性层级的顶点，owl:Thing 模式镜像）
const IRIOWLTopObjectProperty = "http://www.w3.org/2002/07/owl#topObjectProperty"

// IRIOWLTopDataProperty owl:topDataProperty 虚拟根（数据属性层级的顶点，OWL 2 规范词汇；
// 03 期：不入库、不可编辑删除、仅可"新建子属性"，前端固定渲染）
const IRIOWLTopDataProperty = "http://www.w3.org/2002/07/owl#topDataProperty"

// IRIOWLTopAnnotationProperty owl:topAnnotationProperty 虚拟根（注解属性层级的顶点，OWL 2 规范词汇；
// 04 期：不入库、不可编辑删除、仅可"新建子属性"，前端固定渲染。与 Protégé 多根+8 内建根的
// 刻意差异见 04 号文档 §2.3——系列统一单虚拟根，内建谓词不入树不入库）
const IRIOWLTopAnnotationProperty = "http://www.w3.org/2002/07/owl#topAnnotationProperty"

// IRIOWLFunctionalProperty 函数性特征 IRI（OWL 2 词汇表；数据属性 V1 唯一支持的特征——
// 逆函数性等六种是对象属性专属，03 号文档 §2.3）。PropertyCharacteristicIRIs 首项引用本常量，单一事实源。
const IRIOWLFunctionalProperty = "http://www.w3.org/2002/07/owl#FunctionalProperty"

// 特征谓词 IRI（OWL 2 词汇表；有序：page/frame 的 characteristics 输出按此声明序——源修订 #5 内化）
var PropertyCharacteristicIRIs = []string{
	IRIOWLFunctionalProperty,                                  // 函数性
	"http://www.w3.org/2002/07/owl#InverseFunctionalProperty", // 逆函数性
	"http://www.w3.org/2002/07/owl#TransitiveProperty",        // 传递性
	"http://www.w3.org/2002/07/owl#SymmetricProperty",         // 对称性
	"http://www.w3.org/2002/07/owl#AsymmetricProperty",        // 反对称性
	"http://www.w3.org/2002/07/owl#ReflexiveProperty",         // 自反性
	"http://www.w3.org/2002/07/owl#IrreflexiveProperty",       // 反自反性
}

// XSD 命名空间前缀（内建 datatype IRI 与渲染短名共用）
const IRIXSDNamespace = "http://www.w3.org/2001/XMLSchema#"

// rdf:langString（语言文本字面量的 datatype；不进值域白名单——语言文本由 62 行 lang 列承载，
// 03 号文档 §3.2）
const IRIRDFLangString = "http://www.w3.org/1999/02/22-rdf-syntax-ns#langString"

// DatatypeKind 内建 datatype 的词法校验分派键（03 号文档 §3.2/§4.4；Go 侧不承载展示文案，
// 中文标签归前端 i18n 的 DATATYPE_LABELS——转译差异记录 #2）。
// 03 期 10 键 + 05 期扩充 16 键（对齐 OWLAPI OWL2Datatype 枚举的 XSD 成员，05 号文档 §3.2）。
type DatatypeKind uint8

const (
	KindString        DatatypeKind = iota // xsd:string：任意 Unicode 文本
	KindBoolean                           // xsd:boolean：true/false/1/0
	KindDecimal                           // xsd:decimal：可选符号 + 数字 + 可选小数
	KindInteger                           // xsd:integer 族（integer/long/int/short/byte）：64 位整数解析
	KindNonNegInteger                     // xsd:nonNegativeInteger：整数且 ≥ 0
	KindFloat                             // xsd:float / xsd:double：ParseFloat（含科学计数）
	KindDate                              // xsd:date：YYYY-MM-DD 可选时区后缀
	KindDateTime                          // xsd:dateTime：RFC3339 同构（要求时区）
	KindTime                              // xsd:time：HH:MM:SS(.fff)?(Z|±HH:MM)?
	KindAnyURI                            // xsd:anyURI：非空且无控制字符
	// 05 期新增（词法实现见 lexical.go）
	KindNonPosInteger    // xsd:nonPositiveInteger：整数且 ≤ 0
	KindPositiveInteger  // xsd:positiveInteger：整数且 > 0
	KindNegativeInteger  // xsd:negativeInteger：整数且 < 0
	KindUnsignedLong     // xsd:unsignedLong：ParseUint 64 位（0 ~ 2^64-1）
	KindUnsignedInt      // xsd:unsignedInt：≤ 2^32-1
	KindUnsignedShort    // xsd:unsignedShort：≤ 65535
	KindUnsignedByte     // xsd:unsignedByte：≤ 255
	KindNormalizedString // xsd:normalizedString：无换行/回车/制表
	KindToken            // xsd:token：normalizedString 且首尾无空白、无连续空白
	KindLanguage         // xsd:language：[a-zA-Z]{1,8}(-[a-zA-Z0-9]{1,8})*
	KindName             // xsd:Name：NCName 放宽冒号（首字符 字母/_/:）
	KindNCName           // xsd:NCName：首字符 字母/_，后续 字母/数字/./-/_
	KindNMTOKEN          // xsd:NMTOKEN：名称字符任意混合
	KindHexBinary        // xsd:hexBinary：[0-9A-Fa-f]* 且偶长度
	KindBase64Binary     // xsd:base64Binary：RFC 2045 词法（可夹空白）
	KindDateTimeStamp    // xsd:dateTimeStamp：RFC3339 且必须带时区（03 期 dateTime 同口径）
)

// DatatypeFamily 内建 datatype 的词法族（05 号文档 §3.2）：facet 适用族矩阵的判定键，
// 与前端 FAMILY_GROUPS 分组元数据同源维护（前端常量 FACET_META 镜像同一矩阵）。
type DatatypeFamily string

const (
	FamilyNumericDecimal DatatypeFamily = "NUMERIC_DECIMAL" // 数值·十进制（decimal + integer 全族）
	FamilyNumericFloat   DatatypeFamily = "NUMERIC_FLOAT"   // 数值·浮点（float/double）
	FamilyString         DatatypeFamily = "STRING"          // 字符串（string + 词法收紧族）
	FamilyBinary         DatatypeFamily = "BINARY"          // 二进制（hex/base64）
	FamilyTemporal       DatatypeFamily = "TEMPORAL"        // 时间（date/dateTime/dateTimeStamp/time）
	FamilyOther          DatatypeFamily = "OTHER"           // 其他（boolean/anyURI）
)

// BuiltinDatatype 内建 XSD datatype 白名单项：IRI + 词法校验分派键 + 词法族。
type BuiltinDatatype struct {
	IRI    string
	Kind   DatatypeKind
	Family DatatypeFamily
}

// BuiltinDatatypes 内建 XSD datatype 白名单（05 期接管扩展：15 → 31 项，对齐 OWLAPI
// OWL2Datatype 枚举的 29 个 XSD 成员 + xsd:date/xsd:time 自建词法特例；03 期既有 15 项
// IRI/Kind 不变，存量零影响）。刻意剔除并记档（05 号文档 §2.4）：rdfs:Literal（top datatype，
// 等于不设值域行）、rdf:langString（语言文本走 62 行 lang 列）、rdf:PlainLiteral/rdf:XMLLiteral、
// owl:real/owl:rational（业务场景被 decimal/double 覆盖）、gYear/duration/QName 等枚举外扩展类型。
var BuiltinDatatypes = []BuiltinDatatype{
	{IRI: IRIXSDNamespace + "string", Kind: KindString, Family: FamilyString},
	{IRI: IRIXSDNamespace + "normalizedString", Kind: KindNormalizedString, Family: FamilyString},
	{IRI: IRIXSDNamespace + "token", Kind: KindToken, Family: FamilyString},
	{IRI: IRIXSDNamespace + "language", Kind: KindLanguage, Family: FamilyString},
	{IRI: IRIXSDNamespace + "Name", Kind: KindName, Family: FamilyString},
	{IRI: IRIXSDNamespace + "NCName", Kind: KindNCName, Family: FamilyString},
	{IRI: IRIXSDNamespace + "NMTOKEN", Kind: KindNMTOKEN, Family: FamilyString},
	{IRI: IRIXSDNamespace + "boolean", Kind: KindBoolean, Family: FamilyOther},
	{IRI: IRIXSDNamespace + "decimal", Kind: KindDecimal, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "integer", Kind: KindInteger, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "long", Kind: KindInteger, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "int", Kind: KindInteger, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "short", Kind: KindInteger, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "byte", Kind: KindInteger, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "nonNegativeInteger", Kind: KindNonNegInteger, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "nonPositiveInteger", Kind: KindNonPosInteger, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "positiveInteger", Kind: KindPositiveInteger, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "negativeInteger", Kind: KindNegativeInteger, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "unsignedLong", Kind: KindUnsignedLong, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "unsignedInt", Kind: KindUnsignedInt, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "unsignedShort", Kind: KindUnsignedShort, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "unsignedByte", Kind: KindUnsignedByte, Family: FamilyNumericDecimal},
	{IRI: IRIXSDNamespace + "float", Kind: KindFloat, Family: FamilyNumericFloat},
	{IRI: IRIXSDNamespace + "double", Kind: KindFloat, Family: FamilyNumericFloat},
	{IRI: IRIXSDNamespace + "hexBinary", Kind: KindHexBinary, Family: FamilyBinary},
	{IRI: IRIXSDNamespace + "base64Binary", Kind: KindBase64Binary, Family: FamilyBinary},
	{IRI: IRIXSDNamespace + "date", Kind: KindDate, Family: FamilyTemporal},
	{IRI: IRIXSDNamespace + "dateTime", Kind: KindDateTime, Family: FamilyTemporal},
	{IRI: IRIXSDNamespace + "dateTimeStamp", Kind: KindDateTimeStamp, Family: FamilyTemporal},
	{IRI: IRIXSDNamespace + "time", Kind: KindTime, Family: FamilyTemporal},
	{IRI: IRIXSDNamespace + "anyURI", Kind: KindAnyURI, Family: FamilyOther},
}

// IsBuiltinDatatype IRI 是否命中内建白名单（命中返回分派键；rdf:langString 不在其中）。
func IsBuiltinDatatype(iri string) (BuiltinDatatype, bool) {
	for _, d := range BuiltinDatatypes {
		if d.IRI == iri {
			return d, true
		}
	}
	return BuiltinDatatype{}, false
}

// IRIXMLSchemaString 缺省 datatype xsd:string（62 断言无值域行时的推断缺省，03 号文档 §2.6）
const IRIXMLSchemaString = IRIXSDNamespace + "string"

// BuiltinAnnotationPredicates 内建注解谓词白名单（校验器注解断言双轨之一）
var BuiltinAnnotationPredicates = []string{
	PredicateRDFSLabel,
	PredicateRDFSComment,
	PredicateOWLDeprecated,
	PredicateSKOSNotation,
}

// DisplayLabelPredicates 显示名渲染谓词候选（04 号文档 §2.6 常量留缝，对应 Protégé
// OWLRendererPreferences 的注解偏好列表）：PickDisplayLabel 按序取首个命中谓词的首选值。
// V1 单元素（语义与 01~03 期硬编码 rdfs:label 完全等价）；后续把 skos:prefLabel
// 纳入显示名候选时在此追加，UI 不开放配置（06 期多语言场景驱动再开）。
var DisplayLabelPredicates = []string{
	PredicateRDFSLabel,
}

// ClassUsageAxiomTypes 类删除保护（usage 反查）的引用公理类型集。
// 02 期对象属性定义域/值域（31/32）、03 期数据属性定义域（41）已并入；
// 06 期个体断型（60）兑现预埋点并入（01 号文档 §4.5"06 期个体断型（60）落地时往切片加号段"）。
var ClassUsageAxiomTypes = []uint32{
	AxiomSubClassOf,
	AxiomDisjointWith,
	AxiomEquivalentClasses,
	AxiomObjectPropertyDomain,
	AxiomObjectPropertyRange,
	AxiomDataPropertyDomain,
	AxiomTypeAssertion,
}

// ObjectPropertyUsageAxiomTypes 对象属性删除保护（usage 反查）object 侧的引用公理类型集。
// 链成员（37）按 value_text 包含判定、06 期个体断言（61）按 predicate_iri 等值，
// 不走 object_id 命中，各自单独处理（02 号文档 §4.3；61 行处理在对象属性服务 06 期增量）。
var ObjectPropertyUsageAxiomTypes = []uint32{
	AxiomSubObjectPropertyOf,
	AxiomInverseOf,
	AxiomEquivalentObjectProperties,
	AxiomDisjointObjectProperties,
}

// DataPropertyUsageAxiomTypes 数据属性删除保护（usage 反查）object 侧的引用公理类型集：
// 40 子属性边 / 44 等价 / 45 不相交 / 06 期个体数据断言（62，object_id=属性实体路径命中——
// 03 号文档 §4.3 预告兑现）。
var DataPropertyUsageAxiomTypes = []uint32{
	AxiomSubDataPropertyOf,
	AxiomEquivalentDataProperties,
	AxiomDisjointDataProperties,
	AxiomDataPropertyAssertion,
}

// AnnotationPropertyUsageAxiomTypes 注解属性删除保护（usage 反查）object 侧的引用公理类型集：
// 50 子注解属性边（P 是父属性）+ 100 自定义注解断言（P 是谓词，object_id 路径——100 的
// 内建谓词行 object_id 为 NULL，不会命中 object_id 检索）。
// 注解属性无域值域/特征/等价/不相交，两类引用即全集（04 号文档 §4.4）。
var AnnotationPropertyUsageAxiomTypes = []uint32{
	AxiomSubAnnotationPropertyOf,
	AxiomAnnotationAssertion,
}

// DatatypeUsageAxiomTypes 数据类型删除保护（usage 反查）object 侧的引用公理类型集：
// 42 值域行派生轨（数据属性以本 datatype 作值域，object_id 路径命中；内建轨 value_text 行不会
// 命中 object_id 检索）。datatype 无入边公理的其余形态——定义 46 行以 datatype 为主语、
// 引用的 base 是 IRI 常量，均不构成"被引用"（05 号文档 §4.5）。
var DatatypeUsageAxiomTypes = []uint32{
	AxiomDataPropertyRange,
}

// 变更日志操作名（ont_change_logs.operation；03~06 期按需追加）
const (
	OpCreateClass = "CREATE_CLASS"
	OpUpdateClass = "UPDATE_CLASS"
	OpDeleteClass = "DELETE_CLASS"

	OpCreateObjectProperty = "CREATE_OBJECT_PROPERTY"
	OpUpdateObjectProperty = "UPDATE_OBJECT_PROPERTY"
	OpDeleteObjectProperty = "DELETE_OBJECT_PROPERTY"

	OpCreateDataProperty = "CREATE_DATA_PROPERTY"
	OpUpdateDataProperty = "UPDATE_DATA_PROPERTY"
	OpDeleteDataProperty = "DELETE_DATA_PROPERTY"

	OpCreateAnnotationProperty = "CREATE_ANNOTATION_PROPERTY"
	OpUpdateAnnotationProperty = "UPDATE_ANNOTATION_PROPERTY"
	OpDeleteAnnotationProperty = "DELETE_ANNOTATION_PROPERTY"

	OpCreateDatatype = "CREATE_DATATYPE"
	OpUpdateDatatype = "UPDATE_DATATYPE"
	OpDeleteDatatype = "DELETE_DATATYPE"

	OpCreateIndividual = "CREATE_INDIVIDUAL"
	OpUpdateIndividual = "UPDATE_INDIVIDUAL"
	OpDeleteIndividual = "DELETE_INDIVIDUAL"
)

// NavKeepDepth 骨架树无条件保留深度：1=根类、2=域入口；更深的"有父类且无子类"叶子剪掉
// （域入口本身可能是叶子，如工种/计量单位，故按深度无条件保留）。
const NavKeepDepth = 2

// Reason 域错误 reason 常量（与 ontology_error.proto 的 OntologyErrorReason 枚举名逐字一致，
// 服务层按此映射为 Kratos 错误，前端按 reason 字符串分流）
const (
	ReasonOntologyNotFound        = "ONTOLOGY_NOT_FOUND"
	ReasonOntologyNameDuplicate   = "ONTOLOGY_NAME_DUPLICATE"
	ReasonOntologyBaseIRIInvalid  = "ONTOLOGY_BASE_IRI_INVALID"
	ReasonOntologyVersionConflict = "ONTOLOGY_VERSION_CONFLICT"
	ReasonOntologyIRIDuplicate    = "ONTOLOGY_IRI_DUPLICATE"
	ReasonEntityNotFound          = "ENTITY_NOT_FOUND"
	ReasonEntityIRIDuplicate      = "ENTITY_IRI_DUPLICATE"
	ReasonLocalNameInvalid        = "LOCAL_NAME_INVALID"
	ReasonEntityTypeMismatch      = "ENTITY_TYPE_MISMATCH"
	ReasonAxiomInvalid            = "AXIOM_INVALID"
	ReasonHierarchyCyclic         = "HIERARCHY_CYCLIC"
	ReasonEntityInUse             = "ENTITY_IN_USE"

	// 02 期对象属性（源 1-071-003 段）
	ReasonPropertyDomainInvalid         = "PROPERTY_DOMAIN_INVALID"
	ReasonPropertyRangeInvalid          = "PROPERTY_RANGE_INVALID"
	ReasonPropertyChainInvalid          = "PROPERTY_CHAIN_INVALID"
	ReasonPropertyInverseSelfInvalid    = "PROPERTY_INVERSE_SELF_INVALID"
	ReasonPropertySelfReferenceInvalid  = "PROPERTY_SELF_REFERENCE_INVALID"
	ReasonPropertyCharacteristicInvalid = "PROPERTY_CHARACTERISTIC_INVALID"
	ReasonPropertyTargetInvalid         = "PROPERTY_TARGET_INVALID"

	// 03 期数据属性（源 1-071-004 段）
	ReasonDataPropertyDomainInvalid         = "DATA_PROPERTY_DOMAIN_INVALID"
	ReasonDataPropertyRangeInvalid          = "DATA_PROPERTY_RANGE_INVALID"
	ReasonDataPropertySelfReferenceInvalid  = "DATA_PROPERTY_SELF_REFERENCE_INVALID"
	ReasonDataPropertyTargetInvalid         = "DATA_PROPERTY_TARGET_INVALID"
	ReasonDataPropertyCharacteristicInvalid = "DATA_PROPERTY_CHARACTERISTIC_INVALID"

	// 04 期注解属性（源 1-071-005 段）
	ReasonAnnotationPropertyTargetInvalid = "ANNOTATION_PROPERTY_TARGET_INVALID"
	ReasonAnnotationValueInvalid          = "ANNOTATION_VALUE_INVALID"

	// 05 期数据类型（源 1-071-007 段）
	ReasonDatatypeBaseInvalid           = "DATATYPE_BASE_INVALID"            // 基类型必须是内建数据类型（白名单外 / facets 非空而 base 缺失 / datatype 实体作基类型）
	ReasonDatatypeFacetInvalid          = "DATATYPE_FACET_INVALID"           // facet 名白名单外 / 族不适配 / 值词法非法
	ReasonDatatypeFacetConflict         = "DATATYPE_FACET_CONFLICT"          // 静态冲突五规则（§3.2）
	ReasonDatatypeRangeTargetInvalid    = "DATATYPE_RANGE_TARGET_INVALID"    // 42 派生轨：IRI 命中实体但类型不符或跨本体
	ReasonDatatypeDefinitionJSONInvalid = "DATATYPE_DEFINITION_JSON_INVALID" // 46 行 value_text 非 JSON / 形态不符

	// 06 期个体与赋码（源 1-071-006 段）
	ReasonIndividualMainClassRequired = "INDIVIDUAL_MAIN_CLASS_REQUIRED" // 个体必须指定主类目
	ReasonIndividualCodeSourceInvalid = "INDIVIDUAL_CODE_SOURCE_INVALID" // 主类目无编码，无法自动赋码
	ReasonIndividualCodeDuplicate     = "INDIVIDUAL_CODE_DUPLICATE"      // 个体编码已存在
	ReasonIndividualCodePrefixInvalid = "INDIVIDUAL_CODE_PREFIX_INVALID" // 编码含非法字符
	ReasonIndividualCodeRequired      = "INDIVIDUAL_CODE_REQUIRED"       // 个体编码不能为空（MANUAL 空码）
	ReasonIndividualSerialConflict    = "INDIVIDUAL_SERIAL_CONFLICT"     // 取号并发冲突（重试耗尽）
	ReasonIndividualTargetInvalid     = "INDIVIDUAL_TARGET_INVALID"      // 目标不是个体（61 断言目标校验）
)

// DomainError 域内核错误：携带 reason 与用户可读明细。
// 域内核不 import 生成的 Kratos 错误包（依赖纪律），由 HTTP 服务层映射。
type DomainError struct {
	Reason  string
	Message string
}

func (e *DomainError) Error() string { return e.Message }

func NewError(reason, format string, args ...any) *DomainError {
	return &DomainError{Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// Logger 域内核日志接口（WARN：脏环防御 / 未知公理类型跳过）。
// *bLogger.Helper 天然满足；单测传 nil 安全（内部判空）。
type Logger interface {
	Warnf(ctx context.Context, format string, args ...any)
}

// warnf nil 安全的 WARN。
func warnf(l Logger, ctx context.Context, format string, args ...any) {
	if l == nil {
		return
	}
	l.Warnf(ctx, format, args...)
}
