package owlio

import (
	"regexp"
	"strings"

	"go-wind-admin/app/admin/service/internal/ontology"
)

// 导入方向（07 号文档 §2.3~§2.6、§3.2 映射总表）：三元组 → ImportPlan。
//
// 三阶段：① header/声明/blank 子图分类 → ② 证据收集与定型（§2.5 类型上下文 +
// subPropertyOf 传导 + range 分派；冲突即 OWL_IMPORT_TYPE_CONFLICT）→
// ③ 逐公理分派落草稿（预校验失败进忽略清单，不落行——"零校验失败不变式"）。

// localNamePattern 与 internal/ontology（entity_factory.go）同款正则的本地副本
// （域内核包内私有常量；两处必须同源维护——07 号文档 §4.1 复用清单）。
var localNamePattern = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_-]*$`)

// importPairKey 无序对键（成对公理去重展开：单条 triple 与 AllDisjoint 展开共用一个集合，
// 保证存量成对行导出两条 triple 再导入时不重复翻倍——round-trip 零漂移）。
type importPairKey struct {
	axiomType uint32
	a, b      string
}

// importAxiomKey 普通公理去重键（同文件完全重复的 triple 防御；保留多行语义，仅拦同键重复）。
type importAxiomKey struct {
	axiomType uint32
	subject   string
	object    string
	predicate string
	valueText string
	lang      string
}

// importCtx 导入上下文。
type importCtx struct {
	plan        *ImportPlan
	ontologyIRI string
	fileName    string
	allTriples  []Triple

	// 实体注册与定型
	iriIndex   map[string]int                 // IRI → Entities 下标
	declared   map[string]uint32              // IRI → 显式声明类型
	evidence   map[string]map[uint32]struct{} // IRI → 类型证据集合
	finalTypes map[string]uint32              // 定型结果（补声明类型）
	finalReasons map[string]string            // IRI → 首个证据理由（autoDeclared 报告）

	// 处理跳过集
	headerSubjects      map[string]struct{} // 本体 header 主语（owl:Ontology 主体）
	annAxiomSubjects    map[string]struct{} // 公理级注解重写声明形态的主语
	exprBlanks          map[string]struct{} // 表达式空白节点（关联子图）
	dtRestrictionBlanks map[string]struct{} // 46 datatype restriction 放行形态的空白节点

	axioms    []AxiomDraft
	axiomSeen map[importAxiomKey]struct{}
	pairSeen  map[importPairKey]struct{}

	// 先收后用（62 词形预校验依赖 42 值域与 46 定义）
	rangeByProperty map[string][]rangeRefDraft // 数据属性 IRI → 文件序值域
	subPropEdges    [][2]string                // rdfs:subPropertyOf 边（传导用）

	sortSeq map[string]int // 个体 IRI → 60 行序计数
}

type rangeRefDraft struct {
	IRI string // 内建轨 IRI 或派生轨 datatype 实体 IRI（定型后二选一落行）
}

// TriplesToPlan 三元组 → 导入计划（§3.2 映射总表反向；07 号文档 §4.3 步骤 3）。
// fileName 供缺省本体名/命名空间回退推断。
func TriplesToPlan(triples []Triple, fileName string) (*ImportPlan, error) {
	ctx := &importCtx{
		plan: &ImportPlan{
			Report: ImportReport{
				EntityStats:     map[string]int{},
				AxiomStats:      map[uint32]int{},
				IgnoredByReason: map[string]int{},
			},
		},
		iriIndex:            map[string]int{},
		declared:            map[string]uint32{},
		evidence:            map[string]map[uint32]struct{}{},
		finalTypes:          map[string]uint32{},
		finalReasons:        map[string]string{},
		headerSubjects:      map[string]struct{}{},
		annAxiomSubjects:    map[string]struct{}{},
		exprBlanks:          map[string]struct{}{},
		dtRestrictionBlanks: map[string]struct{}{},
		axiomSeen:           map[importAxiomKey]struct{}{},
		pairSeen:            map[importPairKey]struct{}{},
		rangeByProperty:     map[string][]rangeRefDraft{},
		sortSeq:             map[string]int{},
	}
	ctx.fileName = strings.TrimSpace(fileName)

	if err := ctx.collectHeaderAndDeclarations(triples); err != nil {
		return nil, err
	}
	ctx.classifyBlanks(triples)
	ctx.collectEvidence(triples)
	if err := ctx.finalizeTypes(); err != nil {
		return nil, err
	}
	if err := ctx.assembleAxioms(triples); err != nil {
		return nil, err
	}
	if err := ctx.finishPlan(); err != nil {
		return nil, err
	}
	return ctx.plan, nil
}

// entityTypeLabel 实体类型 → 报告标签。
func entityTypeLabel(t uint32) string {
	switch t {
	case ontology.EntityTypeClass:
		return "CLASS"
	case ontology.EntityTypeObjectProperty:
		return "OBJECT_PROPERTY"
	case ontology.EntityTypeDataProperty:
		return "DATA_PROPERTY"
	case ontology.EntityTypeAnnotationProperty:
		return "ANNOTATION_PROPERTY"
	case ontology.EntityTypeDatatype:
		return "DATATYPE"
	case ontology.EntityTypeIndividual:
		return "INDIVIDUAL"
	}
	return sprintf("TYPE_%d", t)
}

// ---- 阶段 0：header 与显式声明 ----

func (c *importCtx) collectHeaderAndDeclarations(triples []Triple) error {
	// 1. owl:Ontology 主体（首个）
	for _, t := range triples {
		if t.Subject.Kind == TermIRI && t.Predicate.Value == IRIRDFType && t.Object.Kind == TermIRI && t.Object.Value == IRIOWLOntology {
			c.ontologyIRI = t.Subject.Value
			break
		}
	}
	if c.ontologyIRI != "" {
		c.headerSubjects[c.ontologyIRI] = struct{}{}
	}

	// 2. 显式声明收集（同 IRI 多类型 → OWL_IMPORT_TYPE_CONFLICT）
	for _, t := range triples {
		if t.Subject.Kind != TermIRI || t.Predicate.Value != IRIRDFType || t.Object.Kind != TermIRI {
			continue
		}
		et, ok := declaredIRIsOf[t.Object.Value]
		if !ok {
			continue
		}
		if prev, declared := c.declared[t.Subject.Value]; declared {
			if prev != et {
				return ontology.NewError(ontology.ReasonOWLImportTypeConflict,
					"文件中实体 %s 的类型声明冲突，无法导入", t.Subject.Value)
			}
			continue
		}
		c.declared[t.Subject.Value] = et
	}

	// 3. 公理级注解重写声明形态（owl:annotatedSource/Property/Target 同主语 → 整组丢弃）
	annSeen := map[string]struct{}{}
	for _, t := range triples {
		if t.Subject.Kind != TermIRI {
			continue
		}
		if t.Predicate.Value == IRIOWLAnnotatedSource || t.Predicate.Value == IRIOWLAnnotatedProperty || t.Predicate.Value == IRIOWLAnnotatedTarget {
			annSeen[t.Subject.Value] = struct{}{}
		}
	}
	for iri := range annSeen {
		c.annAxiomSubjects[iri] = struct{}{}
		c.plan.Report.addIgnored("公理级注解丢弃", sprintf("带注解的公理组 %s（公理级注解能力未实现，注解随组丢弃）", iri))
	}

	// 4. header 注解（description 收集；versionInfo/其余进忽略清单——round-trip known-diff）
	if c.ontologyIRI != "" {
		for _, t := range triples {
			if t.Subject.Kind != TermIRI || t.Subject.Value != c.ontologyIRI {
				continue
			}
			switch t.Predicate.Value {
			case IRIRDFType:
			case IRIRDFSComment:
				if t.Object.Kind == TermLiteral && c.plan.Description == "" {
					c.plan.Description = t.Object.Value
				}
			case IRIOWLImports:
				c.plan.Report.ImportsCount++
				c.plan.Report.addIgnored("owl:imports 未解析", sprintf("依赖文档 %s 不解析（V1 不解析 imports）", t.Object.Value))
			default:
				c.plan.Report.addIgnored("本体 header 注解", sprintf("%s %s（导入不落库）", shortPredicate(t.Predicate.Value), literalText(t.Object)))
			}
		}
	}
	c.plan.OntologyIRI = c.ontologyIRI
	return nil
}

// ---- 阶段 1：blank 子图分类（表达式识别 / 46 放行形态） ----

func (c *importCtx) classifyBlanks(triples []Triple) {
	byBlank := map[string][]int{}
	for i, t := range triples {
		if t.Subject.Kind == TermBlank {
			byBlank[t.Subject.Value] = append(byBlank[t.Subject.Value], i)
		}
	}

	for label, idxs := range byBlank {
		hitExpr := false
		isDT := false
		for _, i := range idxs {
			t := triples[i]
			if t.Predicate.Value == IRIOWLOnDatatype {
				isDT = true
			}
			if t.Predicate.Value == IRIRDFType && t.Object.Value == IRIOWLRestriction {
				hitExpr = true
			}
			if isExpressionVocabulary(t.Predicate.Value) {
				hitExpr = true
			}
		}
		if isDT && !hitExpr {
			c.dtRestrictionBlanks[label] = struct{}{} // 46 唯一放行形态
		} else if hitExpr {
			c.exprBlanks[label] = struct{}{}
		}
	}

	// 传播：表达式 blank 的子图 blank（first/rest 成员、嵌套子节点）一并标记
	worklist := make([]string, 0, len(c.exprBlanks))
	for label := range c.exprBlanks {
		worklist = append(worklist, label)
	}
	for len(worklist) > 0 {
		label := worklist[0]
		worklist = worklist[1:]
		for _, i := range byBlank[label] {
			obj := triples[i].Object
			if obj.Kind == TermBlank {
				if _, done := c.exprBlanks[obj.Value]; !done {
					c.exprBlanks[obj.Value] = struct{}{}
					worklist = append(worklist, obj.Value)
				}
			}
		}
		for _, t := range triples {
			if t.Subject.Kind == TermBlank && t.Object.Kind == TermBlank && t.Object.Value == label {
				if _, done := c.exprBlanks[t.Subject.Value]; !done {
					c.exprBlanks[t.Subject.Value] = struct{}{}
					worklist = append(worklist, t.Subject.Value)
				}
			}
		}
	}
}

// ---- 阶段 2：类型证据收集 ----

func (c *importCtx) collectEvidence(triples []Triple) {
	for _, t := range triples {
		if !c.isCandidateAxiom(t) {
			continue
		}
		subject := t.Subject.Value
		predicate := t.Predicate.Value

		switch predicate {
		case IRIRDFType:
			if t.Object.Kind != TermIRI {
				continue
			}
			if _, isChar := charOnlyIRIs[t.Object.Value]; isChar {
				if t.Object.Value != ontology.IRIOWLFunctionalProperty { // FunctionalProperty 两类属性共享，无证据
					c.addEvidence(subject, ontology.EntityTypeObjectProperty, "对象属性特征声明")
				}
				continue
			}
			if _, isDecl := declaredIRIsOf[t.Object.Value]; isDecl {
				continue // 声明已在阶段 0 收集
			}
			// 60 类型断言：主语个体证据 / 宾语类证据
			c.addEvidence(subject, ontology.EntityTypeIndividual, "类型断言主语")
			if !isReservedIRI(t.Object.Value) {
				c.addEvidence(t.Object.Value, ontology.EntityTypeClass, "类型断言宾语")
			}
		case IRIRDFSSubClassOf:
			c.addEvidence(subject, ontology.EntityTypeClass, "子类公理主语")
			if t.Object.Kind == TermIRI && !isReservedIRI(t.Object.Value) {
				c.addEvidence(t.Object.Value, ontology.EntityTypeClass, "子类公理宾语")
			}
		case IRIOWLDisjointWith:
			c.addEvidence(subject, ontology.EntityTypeClass, "不相交公理主语")
			if t.Object.Kind == TermIRI {
				c.addEvidence(t.Object.Value, ontology.EntityTypeClass, "不相交公理宾语")
			}
		case IRIOWLEquivalentClass:
			c.addEvidence(subject, ontology.EntityTypeClass, "等价类公理主语")
			if t.Object.Kind == TermIRI && !isReservedIRI(t.Object.Value) {
				c.addEvidence(t.Object.Value, ontology.EntityTypeClass, "等价类公理宾语")
			}
		case IRIRDFSSubPropertyOf:
			if t.Object.Kind == TermIRI {
				c.subPropEdges = append(c.subPropEdges, [2]string{subject, t.Object.Value})
			}
		case IRIRDFSDomain:
			if t.Object.Kind == TermIRI && !isReservedIRI(t.Object.Value) {
				c.addEvidence(t.Object.Value, ontology.EntityTypeClass, "定义域宾语")
			}
		case IRIRDFSRange:
			// 宾语形态分派：白名单内建 IRI / datatype 实体 → 主语 DATA_PROPERTY
			if t.Object.Kind != TermIRI {
				continue
			}
			if _, ok := ontology.IsBuiltinDatatype(t.Object.Value); ok {
				c.addEvidence(subject, ontology.EntityTypeDataProperty, "值域为内建数据类型")
			} else if c.declared[t.Object.Value] == ontology.EntityTypeDatatype {
				c.addEvidence(subject, ontology.EntityTypeDataProperty, "值域为数据类型实体")
			}
		case IRIOWLInverseOf:
			c.addEvidence(subject, ontology.EntityTypeObjectProperty, "逆属性主语")
			if t.Object.Kind == TermIRI {
				c.addEvidence(t.Object.Value, ontology.EntityTypeObjectProperty, "逆属性宾语")
			}
		case IRIOWLPropertyChainAxiom:
			c.addEvidence(subject, ontology.EntityTypeObjectProperty, "属性链主语")
			if t.Object.Kind == TermBlank {
				for _, m := range c.collectList(t.Object.Value, triples) {
					if m.Kind == TermIRI {
						c.addEvidence(m.Value, ontology.EntityTypeObjectProperty, "属性链成员")
					}
				}
			}
		default:
			if isBuiltinAnnotationPredicate(predicate) {
				continue // 内建谓词不入库（04 号文档决策三），主语类型证据留给声明
			}
			if isReservedIRI(predicate) {
				continue
			}
			// 断言/注解谓词：谓词显式声明为注解属性 → 100 object_id 轨（主语任意实体，无证据）
			if c.declared[predicate] == ontology.EntityTypeAnnotationProperty {
				continue
			}
			switch t.Object.Kind {
			case TermLiteral:
				// 62 候选：谓词 DATA_PROPERTY 证据、主语 INDIVIDUAL 证据
				c.addEvidence(subject, ontology.EntityTypeIndividual, "数据属性断言主语")
				c.addEvidence(predicate, ontology.EntityTypeDataProperty, "数据属性断言谓词")
			case TermIRI:
				// 61 候选（谓词为注解属性时的 IRI 值在分派阶段进忽略清单）
				c.addEvidence(subject, ontology.EntityTypeIndividual, "对象属性断言主语")
				c.addEvidence(predicate, ontology.EntityTypeObjectProperty, "对象属性断言谓词")
				if !isReservedIRI(t.Object.Value) {
					c.addEvidence(t.Object.Value, ontology.EntityTypeIndividual, "对象属性断言宾语")
				}
			case TermBlank:
				// 匿名节点宾语：谓词按对象属性补声明；主语证据同 61（分派阶段进忽略清单）
				c.addEvidence(subject, ontology.EntityTypeIndividual, "断言主语")
				c.addEvidence(predicate, ontology.EntityTypeObjectProperty, "断言谓词（匿名节点宾语）")
			}
		}
	}

	// AllDisjoint 结构证据（members 集合成员）
	for _, t := range triples {
		if t.Subject.Kind != TermBlank || t.Predicate.Value != IRIRDFType ||
			(t.Object.Value != IRIOWLAllDisjointClasses && t.Object.Value != IRIOWLAllDisjointProperties) {
			continue
		}
		kind := ontology.EntityTypeClass
		reason := "AllDisjoint 类成员"
		if t.Object.Value == IRIOWLAllDisjointProperties {
			kind = ontology.EntityTypeObjectProperty
			reason = "AllDisjoint 属性成员"
		}
		for _, t2 := range triples {
			if t2.Subject.Kind == TermBlank && t2.Subject.Value == t.Subject.Value && t2.Predicate.Value == IRIOWLMembers && t2.Object.Kind == TermBlank {
				for _, m := range c.collectList(t2.Object.Value, triples) {
					if m.Kind == TermIRI {
						c.addEvidence(m.Value, kind, reason)
					}
				}
			}
		}
	}
}

// isCandidateAxiom 该 triple 是否进入公理分派（IRI 主语、非 header/注解组/表达式子图）。
// 宾语是表达式 blank 的公理三元组**不在此拒**——交给分派主循环按谓词进"类表达式公理"忽略清单
// （证据收集仍吸收主语侧类型，保证类实体定型）。
func (c *importCtx) isCandidateAxiom(t Triple) bool {
	if t.Subject.Kind != TermIRI {
		return false // blank 主语是子图结构，不作为公理
	}
	if t.Subject.Value == c.ontologyIRI {
		return false
	}
	if _, ok := c.annAxiomSubjects[t.Subject.Value]; ok {
		return false
	}
	if _, ok := c.exprBlanks[t.Subject.Value]; ok {
		return false
	}
	switch t.Predicate.Value {
	case IRIOWLAnnotatedSource, IRIOWLAnnotatedProperty, IRIOWLAnnotatedTarget, IRIOWLImports:
		return false
	}
	return true
}

// collectList first/rest 集合展开（从集合头 blank 开始；成员保序）。
func (c *importCtx) collectList(head string, triples []Triple) []Term {
	firstBySubject := map[string]Term{}
	restBySubject := map[string]Term{}
	for _, t := range triples {
		if t.Subject.Kind != TermBlank {
			continue
		}
		switch t.Predicate.Value {
		case IRIRDFFirst:
			if _, ok := firstBySubject[t.Subject.Value]; !ok {
				firstBySubject[t.Subject.Value] = t.Object
			}
		case IRIRDFRest:
			if _, ok := restBySubject[t.Subject.Value]; !ok {
				restBySubject[t.Subject.Value] = t.Object
			}
		}
	}
	var out []Term
	node := head
	visited := map[string]struct{}{}
	for {
		if _, seen := visited[node]; seen {
			break // 防环
		}
		visited[node] = struct{}{}
		item, has := firstBySubject[node]
		if !has {
			break
		}
		out = append(out, item)
		rest, has := restBySubject[node]
		if !has {
			break
		}
		if rest.Kind != TermBlank {
			break // rdf:nil 或脏形态
		}
		node = rest.Value
	}
	return out
}

// addEvidence 记录类型证据（已显式声明的 IRI 不再吸收证据——显式类型与证据的冲突由
// 分派阶段的行级预校验拒绝，不阻断整个导入；未声明 IRI 的多证据冲突才是文件自身矛盾）。
func (c *importCtx) addEvidence(iri string, t uint32, reason string) {
	if _, ok := c.declared[iri]; ok {
		return
	}
	set, ok := c.evidence[iri]
	if !ok {
		set = map[uint32]struct{}{}
		c.evidence[iri] = set
	}
	set[t] = struct{}{}
	if _, exists := c.finalReasons[iri]; !exists {
		c.finalReasons[iri] = reason
	}
}

// isBuiltinAnnotationPredicate 内建注解谓词判定。
func isBuiltinAnnotationPredicate(iri string) bool {
	for _, p := range ontology.BuiltinAnnotationPredicates {
		if p == iri {
			return true
		}
	}
	return false
}

// isReservedIRI OWL/RDFS/RDF 系统词汇判定（不可作为实体 IRI 补声明）。
func isReservedIRI(iri string) bool {
	return strings.HasPrefix(iri, NSOWL) || strings.HasPrefix(iri, NSRDFS) || strings.HasPrefix(iri, NSRDF)
}

// charOnlyIRIs 特征声明 IRI 集合（rdf:type 特征形态）。
var charOnlyIRIs = func() map[string]struct{} {
	out := map[string]struct{}{}
	for _, iri := range ontology.PropertyCharacteristicIRIs {
		out[iri] = struct{}{}
	}
	return out
}()

// literalText 字面量的报告展示词形。
func literalText(t Term) string {
	if t.Kind != TermLiteral {
		return t.Value
	}
	if t.Lang != "" {
		return sprintf("%q@%s", t.Value, t.Lang)
	}
	return sprintf("%q", t.Value)
}

// shortPredicate 谓词 IRI 的短名（报告展示）。
func shortPredicate(iri string) string {
	if idx := lastByteIndex(iri, '#'); idx >= 0 {
		return iri[idx+1:]
	}
	return lastIRISegment(iri)
}
