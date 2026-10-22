package owlio

import "fmt"

// sprintf 包内格式化捷径。
func sprintf(format string, args ...interface{}) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// ImportPlan / ImportReport 模型（07 号文档 §4.1 plan.go；§4.3 报告契约）。
//
// 草稿行以"实体下标"互引（SubjectRef/ObjectRef/PredicateRef/MemberRefs）：
// execute 阶段实体逐条 insert 回填自增 id 后按下标重写为真实 id（§2.4 步骤 3/4）。

// EntityDraft 导入实体草稿（ont_entities 行）。
type EntityDraft struct {
	EntityType uint32 // internal/ontology.EntityType* 常量
	IRI        string
	LocalName  string
}

// AxiomDraft 导入公理草稿（ont_axioms 行；引用端为 Entities 下标，-1 = 无）。
type AxiomDraft struct {
	AxiomType uint32

	SubjectRef int // Entities 下标（必 ≥0）

	ObjectRef    int    // 宾语实体下标（-1 = 无）
	PredicateIRI string // 61 断言谓词 / 33·43 特征 / 100 内建轨谓词
	PredicateRef int    // 100 object_id 轨谓词（注解属性实体下标；-1 = 无）
	MemberRefs   []int  // 37 属性链成员下标（保序；仅 37 型）

	ValueText string // 37 链由 MemberRefs 承载，落库时服务层转 JSON id 数组；其余按行语义
	Lang      string
	SortOrder int32
}

// AutoDeclaredRef 自动补声明明细（报告；明细上限 200 条）。
type AutoDeclaredRef struct {
	IRI        string
	EntityType string // CLASS / OBJECT_PROPERTY / DATA_PROPERTY / ANNOTATION_PROPERTY / DATATYPE / INDIVIDUAL
	Reason     string
}

// RenamedLocalName 局部名替补明细（报告）。
type RenamedLocalName struct {
	IRI       string
	LocalName string
	Original  string
}

// IgnoredSample 忽略清单样例。
type IgnoredSample struct {
	Reason  string
	Detail  string // 人类可读明细（公理形态 / 原因）
}

// ImportReport 预检报告（前端展示契约，proto 消息对齐 §4.3）。
type ImportReport struct {
	ReportID     string
	SourceFormat string // 探测结果（RDF/XML / Turtle）

	SuggestedName        string
	SuggestedBaseIRI     string
	SuggestedOntologyIRI string
	SuggestedDescription string

	EntityStats map[string]int // "CLASS"/"OBJECT_PROPERTY"/... → 行数
	AxiomStats  map[uint32]int // axiom_type → 行数

	AutoDeclared      []AutoDeclaredRef
	RenamedLocalNames []RenamedLocalName
	Warnings          []string

	IgnoredCount    int
	IgnoredByReason map[string]int
	IgnoredSamples  []IgnoredSample

	ImportsCount int // 检测到的 owl:imports 三元组数（不解析，报告提示）
}

// ImportPlan preview 产物：落库所需的一切 + 报告。存 ImportPlanStore（TTL 30 分钟）。
type ImportPlan struct {
	SourceFormat Format

	OntologyIRI string // 文件 owl:Ontology 主体 IRI（可空）
	Description string // header rdfs:comment

	Entities []EntityDraft
	Axioms   []AxiomDraft

	Report ImportReport
}

// addWarning 报告追加 warning。
func (r *ImportReport) addWarning(format string, args ...interface{}) {
	r.Warnings = append(r.Warnings, sprintf(format, args...))
}

// addIgnored 忽略清单计数 + 样例（样例上限 50，明细在 byReason 聚合里完整可见）。
func (r *ImportReport) addIgnored(reason, detail string) {
	r.IgnoredCount++
	r.IgnoredByReason[reason]++
	if len(r.IgnoredSamples) < 50 {
		r.IgnoredSamples = append(r.IgnoredSamples, IgnoredSample{Reason: reason, Detail: detail})
	}
}
