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

// IRIOWLTopObjectProperty owl:topObjectProperty 虚拟根（对象属性层级的顶点，owl:Thing 模式镜像；
// 03 期镜像 owl:topDataProperty）
const IRIOWLTopObjectProperty = "http://www.w3.org/2002/07/owl#topObjectProperty"

// 特征谓词 IRI（OWL 2 词汇表；有序：page/frame 的 characteristics 输出按此声明序——源修订 #5 内化）
var PropertyCharacteristicIRIs = []string{
	"http://www.w3.org/2002/07/owl#FunctionalProperty",        // 函数性
	"http://www.w3.org/2002/07/owl#InverseFunctionalProperty", // 逆函数性
	"http://www.w3.org/2002/07/owl#TransitiveProperty",        // 传递性
	"http://www.w3.org/2002/07/owl#SymmetricProperty",         // 对称性
	"http://www.w3.org/2002/07/owl#AsymmetricProperty",        // 反对称性
	"http://www.w3.org/2002/07/owl#ReflexiveProperty",         // 自反性
	"http://www.w3.org/2002/07/owl#IrreflexiveProperty",       // 反自反性
}

// BuiltinAnnotationPredicates 内建注解谓词白名单（校验器注解断言双轨之一）
var BuiltinAnnotationPredicates = []string{
	PredicateRDFSLabel,
	PredicateRDFSComment,
	PredicateOWLDeprecated,
	PredicateSKOSNotation,
}

// ClassUsageAxiomTypes 类删除保护（usage 反查）的引用公理类型集。
// 02 期对象属性定义域/值域（31/32）已并入；03 期数据属性定义域（41）、06 期个体断型（60）
// 落地时各自往切片加号段（源修订 #12/#14 的缺口已在本仓前置内化）。
var ClassUsageAxiomTypes = []uint32{
	AxiomSubClassOf,
	AxiomDisjointWith,
	AxiomEquivalentClasses,
	AxiomObjectPropertyDomain,
	AxiomObjectPropertyRange,
}

// ObjectPropertyUsageAxiomTypes 对象属性删除保护（usage 反查）object 侧的引用公理类型集。
// 链成员（37）按 value_text 包含判定、06 期个体断言（61）按 predicate_iri 等值，
// 不走 object_id 命中，各自单独处理（02 号文档 §4.3）。
var ObjectPropertyUsageAxiomTypes = []uint32{
	AxiomSubObjectPropertyOf,
	AxiomInverseOf,
	AxiomEquivalentObjectProperties,
	AxiomDisjointObjectProperties,
}

// 变更日志操作名（ont_change_logs.operation；03~06 期按需追加）
const (
	OpCreateClass = "CREATE_CLASS"
	OpUpdateClass = "UPDATE_CLASS"
	OpDeleteClass = "DELETE_CLASS"

	OpCreateObjectProperty = "CREATE_OBJECT_PROPERTY"
	OpUpdateObjectProperty = "UPDATE_OBJECT_PROPERTY"
	OpDeleteObjectProperty = "DELETE_OBJECT_PROPERTY"
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
