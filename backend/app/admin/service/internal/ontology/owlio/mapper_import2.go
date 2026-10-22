package owlio

import (
	"sort"
	"strings"

	"go-wind-admin/app/admin/service/internal/ontology"
)

// 导入方向下半场：定型 → 实体注册 → 公理草稿组装（预校验）→ 报告收尾。

// finalizeTypes 证据合并定型：显式声明 > subPropertyOf 传导 > 证据唯一 > 缺省推断。
// 未声明 IRI 的多类型证据冲突 → OWL_IMPORT_TYPE_CONFLICT（文件自身矛盾）。
func (c *importCtx) finalizeTypes() error {
	c.finalReasons = map[string]string{}
	propTypes := map[string]uint32{} // 属性端点的传导/缺省结果

	// 1. subPropertyOf 连通分量传导（迭代到不动点：两端类型必须一致）
	for changed := true; changed; {
		changed = false
		for _, edge := range c.subPropEdges {
			a, b := edge[0], edge[1]
			ta, oka := c.propTypeHint(a, propTypes)
			tb, okb := c.propTypeHint(b, propTypes)
			if oka && !okb {
				if _, declared := c.declared[b]; !declared {
					propTypes[b] = ta
					changed = true
				}
			} else if okb && !oka {
				if _, declared := c.declared[a]; !declared {
					propTypes[a] = tb
					changed = true
				}
			}
		}
	}

	// 2. 缺省推断：传导/证据后仍无类型的属性端点按对象属性补声明（§2.5 宽容口径——
	//    rdfs:domain/range 主语、subPropertyOf 无证据分量；值域为 datatype 形态的已在证据阶段定向）
	hinted := func(iri string) bool {
		if _, ok := c.declared[iri]; ok {
			return true
		}
		if _, ok := propTypes[iri]; ok {
			return true
		}
		if _, ok := c.evidence[iri]; ok {
			return true
		}
		return false
	}
	for _, edge := range c.subPropEdges {
		for _, iri := range edge {
			if !hinted(iri) {
				propTypes[iri] = ontology.EntityTypeObjectProperty
			}
		}
	}
	for _, t := range c.allTriples {
		if !c.isCandidateAxiom(t) {
			continue
		}
		if t.Predicate.Value == IRIRDFSDomain || t.Predicate.Value == IRIRDFSRange {
			if !hinted(t.Subject.Value) {
				propTypes[t.Subject.Value] = ontology.EntityTypeObjectProperty
			}
		}
	}

	// 3. 汇总定型：证据集合唯一的 IRI 按证据定型；多证据冲突 → error
	iris := make([]string, 0, len(c.evidence))
	for iri := range c.evidence {
		iris = append(iris, iri)
	}
	sort.Strings(iris)
	for _, iri := range iris {
		if _, ok := c.declared[iri]; ok {
			continue
		}
		if t, ok := propTypes[iri]; ok {
			c.finalTypes[iri] = t
			c.autoDeclared(iri, t, c.finalReasons[iri])
			continue
		}
		set := c.evidence[iri]
		if len(set) == 1 {
			for t := range set {
				c.finalTypes[iri] = t
			}
			c.autoDeclared(iri, c.finalTypes[iri], c.finalReasons[iri])
			continue
		}
		labels := make([]string, 0, len(set))
		for t := range set {
			labels = append(labels, entityTypeLabel(t))
		}
		sort.Strings(labels)
		return ontology.NewError(ontology.ReasonOWLImportTypeConflict,
			"文件中实体 %s 的类型声明冲突（%s），无法导入", iri, strings.Join(labels, " / "))
	}
	// 传导/缺省结果统一入 finalTypes
	props := make([]string, 0, len(propTypes))
	for iri := range propTypes {
		props = append(props, iri)
	}
	sort.Strings(props)
	for _, iri := range props {
		if _, ok := c.declared[iri]; ok {
			continue
		}
		if _, ok := c.finalTypes[iri]; !ok {
			c.finalTypes[iri] = propTypes[iri]
			c.autoDeclared(iri, propTypes[iri], c.finalReasons[iri])
		}
	}
	return nil
}

// propTypeHint 传导可用的类型提示：显式属性声明 / 已传导值 / 唯一证据。
func (c *importCtx) propTypeHint(iri string, propTypes map[string]uint32) (uint32, bool) {
	if t, ok := c.declared[iri]; ok {
		switch t {
		case ontology.EntityTypeObjectProperty, ontology.EntityTypeDataProperty, ontology.EntityTypeAnnotationProperty:
			return t, true
		}
		return 0, false
	}
	if t, ok := propTypes[iri]; ok {
		return t, true
	}
	if set, ok := c.evidence[iri]; ok && len(set) == 1 {
		for t := range set {
			return t, true
		}
	}
	return 0, false
}

// autoDeclared 补声明报告登记（上限 200 条明细）。
func (c *importCtx) autoDeclared(iri string, t uint32, reason string) {
	if reason == "" {
		reason = "公理引用未声明"
	}
	if len(c.plan.Report.AutoDeclared) < 200 {
		c.plan.Report.AutoDeclared = append(c.plan.Report.AutoDeclared, AutoDeclaredRef{
			IRI: iri, EntityType: entityTypeLabel(t), Reason: reason,
		})
	}
}

// typeOf IRI 的最终实体类型（定型后调用；未注册返回 false）。
func (c *importCtx) typeOf(iri string) (uint32, bool) {
	if t, ok := c.declared[iri]; ok {
		return t, true
	}
	if t, ok := c.finalTypes[iri]; ok {
		return t, true
	}
	return 0, false
}

// registerEntities 统一注册实体（声明 + 补声明；文件出现序），每实体补 DECLARATION(10) 草稿。
func (c *importCtx) registerEntities() {
	seen := map[string]struct{}{}
	var ordered []string
	for _, t := range c.allTriples {
		if t.Subject.Kind == TermIRI {
			if _, ok := seen[t.Subject.Value]; !ok {
				seen[t.Subject.Value] = struct{}{}
				ordered = append(ordered, t.Subject.Value)
			}
		}
		if t.Object.Kind == TermIRI {
			if _, ok := seen[t.Object.Value]; !ok {
				seen[t.Object.Value] = struct{}{}
				ordered = append(ordered, t.Object.Value)
			}
		}
	}
	for _, iri := range ordered {
		if iri == c.ontologyIRI {
			continue // 本体主体不是实体
		}
		if isReservedIRI(iri) {
			continue // 系统词汇不建实体
		}
		if _, ok := ontology.IsBuiltinDatatype(iri); ok {
			continue // 内建 datatype 白名单跳过（§2.5 #2）
		}
		if _, exists := c.iriIndex[iri]; exists {
			continue
		}
		t, ok := c.declared[iri]
		if !ok {
			t, ok = c.finalTypes[iri]
		}
		if !ok {
			continue // 无类型（防御跳过）
		}
		c.entityIndex(iri, t)
		c.appendDeclaration(iri, t)
	}
}

// entityIndex 实体注册（IRI 保真；localName 提取/替补；同 IRI 幂等）。返回下标。
func (c *importCtx) entityIndex(iri string, entityType uint32) int {
	if idx, ok := c.iriIndex[iri]; ok {
		return idx
	}
	original := lastIRISegment(iri)
	localName := original
	if !localNamePattern.MatchString(localName) {
		localName = sprintf("e%d", len(c.plan.Entities)+1)
		if original == "" {
			original = "<空>"
		}
		c.plan.Report.RenamedLocalNames = append(c.plan.Report.RenamedLocalNames, RenamedLocalName{
			IRI: iri, LocalName: localName, Original: original,
		})
	}
	c.plan.Entities = append(c.plan.Entities, EntityDraft{EntityType: entityType, IRI: iri, LocalName: localName})
	idx := len(c.plan.Entities) - 1
	c.iriIndex[iri] = idx
	return idx
}

// appendDeclaration DECLARATION(10) 草稿 + 统计（修订 #6：为每个存活实体补声明行）。
func (c *importCtx) appendDeclaration(iri string, t uint32) {
	c.appendAxiom(AxiomDraft{
		AxiomType:  ontology.AxiomDeclaration,
		SubjectRef: c.iriIndex[iri],
		ObjectRef:  -1, PredicateRef: -1,
	})
	c.plan.Report.EntityStats[entityTypeLabel(t)]++
	c.plan.Report.AxiomStats[ontology.AxiomDeclaration]++
}

// appendAxiom 草稿追加（同键重复防御）。
func (c *importCtx) appendAxiom(d AxiomDraft) bool {
	key := importAxiomKey{
		axiomType: d.AxiomType,
		subject:   c.iriOf(d.SubjectRef),
		object:    c.iriOf(d.ObjectRef),
		predicate: d.PredicateIRI + "\x00" + c.iriOf(d.PredicateRef),
		valueText: d.ValueText,
		lang:      d.Lang,
	}
	if _, ok := c.axiomSeen[key]; ok {
		return false
	}
	c.axiomSeen[key] = struct{}{}
	c.axioms = append(c.axioms, d)
	return true
}

func (c *importCtx) iriOf(ref int) string {
	if ref < 0 || ref >= len(c.plan.Entities) {
		return ""
	}
	return c.plan.Entities[ref].IRI
}

// refOf IRI → 实体下标（未注册 -1）。
func (c *importCtx) refOf(iri string) int {
	if idx, ok := c.iriIndex[iri]; ok {
		return idx
	}
	return -1
}

// appendPair 成对公理无序对登记（组装完成时统一展开）。
func (c *importCtx) appendPair(axiomType uint32, a, b string) {
	if a == b {
		c.plan.Report.addIgnored("自引用公理", sprintf("%s 与自身建立成对关系，已忽略", a))
		return
	}
	x, y := a, b
	if x > y {
		x, y = y, x
	}
	c.pairSeen[importPairKey{axiomType: axiomType, a: x, b: y}] = struct{}{}
}

// expandPairs 成对公理展开为 (A,B)+(B,A) 双行。
func (c *importCtx) expandPairs() {
	keys := make([]importPairKey, 0, len(c.pairSeen))
	for k := range c.pairSeen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].axiomType != keys[j].axiomType {
			return keys[i].axiomType < keys[j].axiomType
		}
		if keys[i].a != keys[j].a {
			return keys[i].a < keys[j].a
		}
		return keys[i].b < keys[j].b
	})
	for _, k := range keys {
		for _, dir := range [][2]string{{k.a, k.b}, {k.b, k.a}} {
			c.appendAxiom(AxiomDraft{
				AxiomType:  k.axiomType,
				SubjectRef: c.refOf(dir[0]),
				ObjectRef:  c.refOf(dir[1]),
				PredicateRef: -1,
			})
		}
	}
}

// assembleAxioms 公理分派主循环（定型后；42 值域先收、62 后组——词形推断依赖）。
func (c *importCtx) assembleAxioms(triples []Triple) error {
	c.allTriples = triples
	c.registerEntities()

	var pending62 []Triple

	for _, t := range triples {
		if !c.isCandidateAxiom(t) {
			continue
		}
		subject := t.Subject.Value
		predicate := t.Predicate.Value

		switch predicate {
		case IRIRDFType:
			if t.Object.Kind == TermBlank {
				c.plan.Report.addIgnored("类表达式公理", sprintf("%s rdf:type [类表达式]", subject))
				continue
			}
			if t.Object.Kind != TermIRI {
				continue
			}
			if _, isChar := charOnlyIRIs[t.Object.Value]; isChar {
				c.assembleCharacteristic(subject, t.Object.Value)
				continue
			}
			if _, isDecl := declaredIRIsOf[t.Object.Value]; isDecl {
				continue
			}
			c.assembleTypeAssertion(subject, t.Object.Value)
		case IRIRDFSSubClassOf:
			if t.Object.Kind != TermIRI {
				c.plan.Report.addIgnored("类表达式公理", sprintf("%s rdfs:subClassOf [类表达式]", subject))
				continue
			}
			c.assembleBinary(ontology.AxiomSubClassOf, subject, t.Object.Value, "子类")
		case IRIOWLDisjointWith:
			if t.Object.Kind == TermBlank {
				c.plan.Report.addIgnored("类表达式公理", sprintf("%s owl:disjointWith [类表达式]", subject))
				continue
			}
			if t.Object.Kind != TermIRI {
				continue
			}
			c.assembleClassPair(ontology.AxiomDisjointWith, subject, t.Object.Value, "不相交类")
		case IRIOWLEquivalentClass:
			// 46 定义放行形态优先：主语为 datatype 实体且宾语为 datatype restriction blank
			if t.Object.Kind == TermBlank {
				if st, _ := c.typeOf(subject); st == ontology.EntityTypeDatatype {
					if def, ok := c.parseDatatypeRestriction(t.Object.Value, triples); ok {
						c.assembleDefinition(subject, def)
						continue
					}
				}
				c.plan.Report.addIgnored("类表达式公理", sprintf("%s owl:equivalentClass [非 restriction 形态]", subject))
				continue
			}
			if t.Object.Kind != TermIRI {
				continue
			}
			c.assembleClassPair(ontology.AxiomEquivalentClasses, subject, t.Object.Value, "等价类")
		case IRIRDFSSubPropertyOf:
			if t.Object.Kind != TermIRI {
				c.plan.Report.addIgnored("属性层级两端类型不符", sprintf("%s rdfs:subPropertyOf [非命名属性]", subject))
				continue
			}
			c.assembleSubPropertyOf(subject, t.Object.Value)
		case IRIRDFSDomain:
			if t.Object.Kind != TermIRI {
				c.plan.Report.addIgnored("定义域宾语不是类", sprintf("%s rdfs:domain [非命名类]", subject))
				continue
			}
			c.assembleDomainRange(subject, t.Object.Value, true)
		case IRIRDFSRange:
			if t.Object.Kind != TermIRI {
				c.plan.Report.addIgnored("值域 IRI 不在白名单", sprintf("%s rdfs:range [非命名形态]", subject))
				continue
			}
			c.assembleRange(subject, t.Object.Value)
		case IRIOWLInverseOf:
			if t.Object.Kind == TermBlank {
				c.plan.Report.addIgnored("属性公理两端类型不符", sprintf("%s owl:inverseOf [非命名属性]", subject))
				continue
			}
			if t.Object.Kind != TermIRI {
				continue
			}
			c.assembleObjectPropertyPair(ontology.AxiomInverseOf, subject, t.Object.Value, "逆属性")
		case IRIOWLEquivalentProperty:
			if t.Object.Kind == TermBlank {
				c.plan.Report.addIgnored("属性公理两端类型不符", sprintf("%s owl:equivalentProperty [非命名属性]", subject))
				continue
			}
			if t.Object.Kind != TermIRI {
				continue
			}
			c.assemblePropertyPair(ontology.AxiomEquivalentObjectProperties, ontology.AxiomEquivalentDataProperties, subject, t.Object.Value)
		case IRIOWLPropertyDisjointWith:
			if t.Object.Kind == TermBlank {
				c.plan.Report.addIgnored("属性公理两端类型不符", sprintf("%s owl:propertyDisjointWith [非命名属性]", subject))
				continue
			}
			if t.Object.Kind != TermIRI {
				continue
			}
			c.assemblePropertyPair(ontology.AxiomDisjointObjectProperties, ontology.AxiomDisjointDataProperties, subject, t.Object.Value)
		case IRIOWLPropertyChainAxiom:
			if t.Object.Kind != TermBlank {
				continue
			}
			c.assembleChain(subject, t.Object.Value, triples)
		case IRIOWLOnDatatype, IRIOWLWithRestrictions, IRIRDFFirst, IRIRDFRest:
			// blank 子图内部词汇（主语为 IRI 的脏形态：datatype 实体直接挂 onDatatype 无定义语义，跳过）
		default:
			if isBuiltinAnnotationPredicate(predicate) {
				c.assembleBuiltinAnnotation(subject, predicate, t.Object)
				continue
			}
			if isReservedIRI(predicate) {
				c.plan.Report.addIgnored("保留词汇引用", sprintf("%s %s %s", subject, shortPredicate(predicate), literalText(t.Object)))
				continue
			}
			switch t.Object.Kind {
			case TermLiteral:
				// 谓词定型分派：注解属性 → 100 object_id 轨；数据属性（或未定型按 DATA 证据）→ 62
				if pt, ok := c.typeOf(predicate); ok && pt == ontology.EntityTypeAnnotationProperty {
					c.assembleCustomAnnotation(subject, predicate, t.Object)
					continue
				}
				pending62 = append(pending62, t)
			case TermIRI:
				if pt, ok := c.typeOf(predicate); ok && pt == ontology.EntityTypeAnnotationProperty {
					c.plan.Report.addIgnored("IRI 型注解值", sprintf("%s %s %s", subject, shortPredicate(predicate), t.Object.Value))
					continue
				}
				c.assembleObjectAssertion(subject, predicate, t.Object.Value)
			case TermBlank:
				c.plan.Report.addIgnored("匿名节点宾语断言", sprintf("%s %s [匿名节点]", subject, shortPredicate(predicate)))
			}
		}
	}

	// AllDisjoint 展开（两两有序对）
	c.assembleAllDisjoints(triples)
	// 42 值域落草稿（主语定型为数据属性的收集项）
	c.flushRanges()
	// 62 数据断言（42 已收集完毕后统一处理，词形预校验按首条 42 推断）
	for _, t := range pending62 {
		c.assembleDataAssertion(t)
	}
	// 成对公理展开
	c.expandPairs()

	c.plan.Axioms = c.axioms
	return nil
}

// parseDatatypeRestriction 46 放行形态解析：blank 子图（onDatatype + 可选 withRestrictions）
// → 定义；混合表达式词 → 不放行（交由等价类的类表达式忽略分支）。
func (c *importCtx) parseDatatypeRestriction(label string, triples []Triple) (ontology.DatatypeDefinition, bool) {
	def := ontology.DatatypeDefinition{}
	onDatatype := ""
	restrictionsHead := ""
	hitExpr := false
	for _, t := range triples {
		if t.Subject.Kind != TermBlank || t.Subject.Value != label {
			continue
		}
		switch t.Predicate.Value {
		case IRIOWLOnDatatype:
			if t.Object.Kind == TermIRI {
				onDatatype = t.Object.Value
			}
		case IRIOWLWithRestrictions:
			if t.Object.Kind == TermBlank {
				restrictionsHead = t.Object.Value
			}
		case IRIRDFType:
			if t.Object.Value != IRIRDFSDatatype {
				hitExpr = true
			}
		case IRIRDFFirst, IRIRDFRest:
		default:
			if isExpressionVocabulary(t.Predicate.Value) {
				hitExpr = true
			}
		}
	}
	if onDatatype == "" || hitExpr {
		return def, false
	}
	def.BaseIRI = onDatatype
	if restrictionsHead != "" {
		for _, cell := range c.collectList(restrictionsHead, triples) {
			if cell.Kind != TermBlank {
				continue // facet 必须是 restriction 节点
			}
			for _, t := range triples {
				if t.Subject.Kind != TermBlank || t.Subject.Value != cell.Value {
					continue
				}
				facetName, ok := xsdFacetIRIs[t.Predicate.Value]
				if !ok || t.Object.Kind != TermLiteral {
					continue
				}
				def.Facets = append(def.Facets, ontology.DatatypeFacet{Facet: facetName, Value: t.Object.Value})
			}
		}
	}
	return def, true
}

// assembleDefinition 46 定义草稿（校验失败 → 该行忽略清单，datatype 实体保留放行轨——§7 #10）。
func (c *importCtx) assembleDefinition(subjectIRI string, def ontology.DatatypeDefinition) {
	if err := ontology.ValidateDefinition(def); err != nil {
		c.plan.Report.addIgnored("数据类型定义非法", sprintf("%s：%s", subjectIRI, err.Error()))
		return
	}
	c.appendAxiom(AxiomDraft{
		AxiomType:  ontology.AxiomDatatypeDefinition,
		SubjectRef: c.refOf(subjectIRI),
		ObjectRef:  -1, PredicateRef: -1,
		ValueText: ontology.NormalizeDefinition(def),
	})
}

// assembleTypeAssertion 60 行草稿（sort_order = 文件内该个体第 i 条类型断言，0 起首位主类目）。
func (c *importCtx) assembleTypeAssertion(subjectIRI, classIRI string) {
	st, sok := c.typeOf(subjectIRI)
	ot, ook := c.typeOf(classIRI)
	if !sok || st != ontology.EntityTypeIndividual {
		c.plan.Report.addIgnored("类型断言主语非个体", sprintf("%s rdf:type %s", subjectIRI, classIRI))
		return
	}
	if !ook || ot != ontology.EntityTypeClass {
		c.plan.Report.addIgnored("类型断言宾语非类", sprintf("%s rdf:type %s", subjectIRI, classIRI))
		return
	}
	seq := c.sortSeq[subjectIRI]
	c.sortSeq[subjectIRI] = seq + 1
	c.appendAxiom(AxiomDraft{
		AxiomType:  ontology.AxiomTypeAssertion,
		SubjectRef: c.refOf(subjectIRI),
		ObjectRef:  c.refOf(classIRI),
		PredicateRef: -1,
		SortOrder:  int32(seq),
	})
}

// assembleBinary 20 子类行草稿（两端均为命名类）。
func (c *importCtx) assembleBinary(axiomType uint32, subjectIRI, objectIRI, label string) {
	if isReservedIRI(objectIRI) {
		c.plan.Report.addIgnored("保留词汇引用", sprintf("%s %s %s", subjectIRI, shortPredicate(label), objectIRI))
		return
	}
	st, sok := c.typeOf(subjectIRI)
	if !sok || st != ontology.EntityTypeClass {
		c.plan.Report.addIgnored("公理主语类型不符", sprintf("%s 的%s公理主语不是类", subjectIRI, label))
		return
	}
	ot, ook := c.typeOf(objectIRI)
	if !ook || ot != ontology.EntityTypeClass {
		c.plan.Report.addIgnored("公理宾语类型不符", sprintf("%s 的%s公理宾语不是类", subjectIRI, label))
		return
	}
	c.appendAxiom(AxiomDraft{
		AxiomType:  axiomType,
		SubjectRef: c.refOf(subjectIRI),
		ObjectRef:  c.refOf(objectIRI),
		PredicateRef: -1,
	})
}

// assembleClassPair 21/22 类成对公理（两端均为类才落行）。
func (c *importCtx) assembleClassPair(axiomType uint32, aIRI, bIRI, label string) {
	at, aok := c.typeOf(aIRI)
	bt, bok := c.typeOf(bIRI)
	if !aok || at != ontology.EntityTypeClass {
		c.plan.Report.addIgnored("公理两端类型不符", sprintf("%s 的%s公理主语不是类", aIRI, label))
		return
	}
	if !bok || bt != ontology.EntityTypeClass {
		c.plan.Report.addIgnored("公理两端类型不符", sprintf("%s 的%s公理宾语不是类", aIRI, label))
		return
	}
	c.appendPair(axiomType, aIRI, bIRI)
}

// assembleObjectPropertyPair 34 逆属性（两端均为对象属性才落行）。
func (c *importCtx) assembleObjectPropertyPair(axiomType uint32, aIRI, bIRI, label string) {
	at, aok := c.typeOf(aIRI)
	bt, bok := c.typeOf(bIRI)
	if !aok || !bok || at != ontology.EntityTypeObjectProperty || bt != ontology.EntityTypeObjectProperty {
		c.plan.Report.addIgnored("属性公理两端类型不符", sprintf("%s 的%s两端须为对象属性（%s / %s）",
			aIRI, label, typeOrUnknown(at, aok), typeOrUnknown(bt, bok)))
		return
	}
	c.appendPair(axiomType, aIRI, bIRI)
}

// assemblePropertyPair 35/36/44/45 分派（两端同型才落行）。
func (c *importCtx) assemblePropertyPair(objType, dataType uint32, aIRI, bIRI string) {
	at, aok := c.typeOf(aIRI)
	bt, bok := c.typeOf(bIRI)
	if !aok || !bok || at != bt {
		c.plan.Report.addIgnored("属性公理两端类型不符", sprintf("%s / %s（%s / %s）", aIRI, bIRI, typeOrUnknown(at, aok), typeOrUnknown(bt, bok)))
		return
	}
	switch at {
	case ontology.EntityTypeObjectProperty:
		c.appendPair(objType, aIRI, bIRI)
	case ontology.EntityTypeDataProperty:
		c.appendPair(dataType, aIRI, bIRI)
	default:
		c.plan.Report.addIgnored("属性公理两端类型不符", sprintf("%s / %s", aIRI, bIRI))
	}
}

// assembleSubPropertyOf 30/40/50 分派（两端类型一致才落行）。
func (c *importCtx) assembleSubPropertyOf(childIRI, parentIRI string) {
	ct, cok := c.typeOf(childIRI)
	pt, pok := c.typeOf(parentIRI)
	if !cok || !pok || ct != pt {
		c.plan.Report.addIgnored("属性层级两端类型不符", sprintf("%s rdfs:subPropertyOf %s（%s / %s）",
			childIRI, parentIRI, typeOrUnknown(ct, cok), typeOrUnknown(pt, pok)))
		return
	}
	var axiomType uint32
	switch ct {
	case ontology.EntityTypeObjectProperty:
		axiomType = ontology.AxiomSubObjectPropertyOf
	case ontology.EntityTypeDataProperty:
		axiomType = ontology.AxiomSubDataPropertyOf
	case ontology.EntityTypeAnnotationProperty:
		axiomType = ontology.AxiomSubAnnotationPropertyOf
	default:
		c.plan.Report.addIgnored("属性层级两端类型不符", sprintf("%s rdfs:subPropertyOf %s", childIRI, parentIRI))
		return
	}
	c.appendAxiom(AxiomDraft{
		AxiomType:  axiomType,
		SubjectRef: c.refOf(childIRI),
		ObjectRef:  c.refOf(parentIRI),
		PredicateRef: -1,
	})
}

// assembleDomainRange 31/41 分派（主语属性类型；41 的宾语可以是任意 IRI——类证据已收集）。
func (c *importCtx) assembleDomainRange(subjectIRI, classIRI string, isDomain bool) {
	st, ok := c.typeOf(subjectIRI)
	if !ok {
		c.plan.Report.addIgnored("公理主语类型不符", sprintf("%s 的域/值域主语未定型", subjectIRI))
		return
	}
	if isDomain {
		ct, ook := c.typeOf(classIRI)
		if !ook || ct != ontology.EntityTypeClass {
			c.plan.Report.addIgnored("定义域宾语不是类", sprintf("%s rdfs:domain %s", subjectIRI, classIRI))
			return
		}
	}
	var axiomType uint32
	switch st {
	case ontology.EntityTypeObjectProperty:
		axiomType = ontology.AxiomObjectPropertyDomain
	case ontology.EntityTypeDataProperty:
		axiomType = ontology.AxiomDataPropertyDomain
	default:
		c.plan.Report.addIgnored("定义域主语类型不符", sprintf("%s rdfs:domain %s", subjectIRI, classIRI))
		return
	}
	c.appendAxiom(AxiomDraft{
		AxiomType:  axiomType,
		SubjectRef: c.refOf(subjectIRI),
		ObjectRef:  c.refOf(classIRI),
		PredicateRef: -1,
	})
}

// assembleRange rdfs:range 按主语属性类型分派（31/32 vs 41/42 双轨）：
// 对象属性 → 32 行（宾语须类实体）；数据属性 → 42 行（内建轨 value_text / 派生轨 object ref；
// 白名单外 → 忽略清单——§3.2 总表）。数据属性的值域同时进 rangeByProperty 供 62 词形推断。
func (c *importCtx) assembleRange(subjectIRI, objectIRI string) {
	st, ok := c.typeOf(subjectIRI)
	if !ok {
		c.plan.Report.addIgnored("公理主语类型不符", sprintf("%s rdfs:range %s（主语未定型）", subjectIRI, objectIRI))
		return
	}
	switch st {
	case ontology.EntityTypeObjectProperty:
		ot, ook := c.typeOf(objectIRI)
		if !ook || ot != ontology.EntityTypeClass {
			c.plan.Report.addIgnored("值域宾语不是类", sprintf("%s rdfs:range %s", subjectIRI, objectIRI))
			return
		}
		c.appendAxiom(AxiomDraft{
			AxiomType:  ontology.AxiomObjectPropertyRange,
			SubjectRef: c.refOf(subjectIRI),
			ObjectRef:  c.refOf(objectIRI),
			PredicateRef: -1,
		})
	case ontology.EntityTypeDataProperty:
		if _, builtin := ontology.IsBuiltinDatatype(objectIRI); builtin {
			c.rangeByProperty[subjectIRI] = append(c.rangeByProperty[subjectIRI], rangeRefDraft{IRI: objectIRI})
			return
		}
		if ot, ook := c.typeOf(objectIRI); ook && ot == ontology.EntityTypeDatatype {
			c.rangeByProperty[subjectIRI] = append(c.rangeByProperty[subjectIRI], rangeRefDraft{IRI: objectIRI})
			return
		}
		c.plan.Report.addIgnored("值域 IRI 不在白名单", sprintf("%s rdfs:range %s", subjectIRI, objectIRI))
	default:
		c.plan.Report.addIgnored("值域主语类型不符", sprintf("%s rdfs:range %s", subjectIRI, objectIRI))
	}
}

// flushRanges 42 行草稿落库形态（内建轨 value_text / 派生轨 object ref；主语已由
// assembleRange 确认为数据属性）。
func (c *importCtx) flushRanges() {
	props := make([]string, 0, len(c.rangeByProperty))
	for p := range c.rangeByProperty {
		props = append(props, p)
	}
	sort.Strings(props)
	for _, p := range props {
		st, ok := c.typeOf(p)
		if !ok || st != ontology.EntityTypeDataProperty {
			continue // 主语不是数据属性：值域行忽略
		}
		for i, r := range c.rangeByProperty[p] {
			d := AxiomDraft{AxiomType: ontology.AxiomDataPropertyRange, SubjectRef: c.refOf(p), ObjectRef: -1, PredicateRef: -1, SortOrder: int32(i)}
			if _, builtin := ontology.IsBuiltinDatatype(r.IRI); builtin {
				d.ValueText = r.IRI
			} else {
				d.ObjectRef = c.refOf(r.IRI)
			}
			c.appendAxiom(d)
		}
	}
}

// assembleCharacteristic 33/43 特征行。
func (c *importCtx) assembleCharacteristic(subjectIRI, charIRI string) {
	st, ok := c.typeOf(subjectIRI)
	if !ok {
		c.plan.Report.addIgnored("特征主语类型未知", sprintf("%s rdf:type %s", subjectIRI, shortPredicate(charIRI)))
		return
	}
	switch st {
	case ontology.EntityTypeObjectProperty:
		if !isPropertyCharacteristicIRI(charIRI) {
			c.plan.Report.addIgnored("未知对象属性特征", sprintf("%s rdf:type %s", subjectIRI, charIRI))
			return
		}
		c.appendAxiom(AxiomDraft{
			AxiomType:  ontology.AxiomObjectPropertyCharacteristic,
			SubjectRef: c.refOf(subjectIRI),
			ObjectRef:  -1,
			PredicateIRI: charIRI, PredicateRef: -1,
		})
	case ontology.EntityTypeDataProperty:
		if charIRI != ontology.IRIOWLFunctionalProperty {
			c.plan.Report.addIgnored("数据属性仅支持函数性", sprintf("%s rdf:type %s", subjectIRI, shortPredicate(charIRI)))
			return
		}
		c.appendAxiom(AxiomDraft{
			AxiomType:  ontology.AxiomDataPropertyCharacteristic,
			SubjectRef: c.refOf(subjectIRI),
			ObjectRef:  -1,
			PredicateIRI: charIRI, PredicateRef: -1,
		})
	default:
		c.plan.Report.addIgnored("特征主语类型不符", sprintf("%s rdf:type %s", subjectIRI, shortPredicate(charIRI)))
	}
}

func isPropertyCharacteristicIRI(iri string) bool {
	for _, v := range ontology.PropertyCharacteristicIRIs {
		if v == iri {
			return true
		}
	}
	return false
}

// assembleChain 37 链草稿（成员全部对象属性，保序）。
func (c *importCtx) assembleChain(subjectIRI, head string, triples []Triple) {
	st, ok := c.typeOf(subjectIRI)
	if !ok || st != ontology.EntityTypeObjectProperty {
		c.plan.Report.addIgnored("属性链主语类型不符", sprintf("%s owl:propertyChainAxiom (…)", subjectIRI))
		return
	}
	refs := make([]int, 0, 4)
	for _, m := range c.collectList(head, triples) {
		if m.Kind != TermIRI {
			c.plan.Report.addIgnored("属性链成员非法", sprintf("%s 的链成员不是命名属性", subjectIRI))
			return
		}
		mt, mok := c.typeOf(m.Value)
		if !mok || mt != ontology.EntityTypeObjectProperty {
			c.plan.Report.addIgnored("属性链成员类型不符", sprintf("%s 的链成员 %s 不是对象属性", subjectIRI, m.Value))
			return
		}
		refs = append(refs, c.refOf(m.Value))
	}
	if len(refs) < 2 {
		c.plan.Report.addIgnored("属性链成员不足", sprintf("%s 的链成员数 %d 不足（至少 2）", subjectIRI, len(refs)))
		return
	}
	c.appendAxiom(AxiomDraft{
		AxiomType:  ontology.AxiomSubPropertyChainOf,
		SubjectRef: c.refOf(subjectIRI),
		ObjectRef:  -1, PredicateRef: -1,
		MemberRefs: refs,
	})
}

// assembleAllDisjoints AllDisjointClasses/AllDisjointProperties 展开（两两有序对——§2.6 #1）。
func (c *importCtx) assembleAllDisjoints(triples []Triple) {
	for _, t := range triples {
		if t.Subject.Kind != TermBlank || t.Predicate.Value != IRIRDFType ||
			(t.Object.Value != IRIOWLAllDisjointClasses && t.Object.Value != IRIOWLAllDisjointProperties) {
			continue
		}
		for _, t2 := range triples {
			if t2.Subject.Kind != TermBlank || t2.Subject.Value != t.Subject.Value || t2.Predicate.Value != IRIOWLMembers {
				continue
			}
			var iris []string
			valid := true
			for _, m := range c.collectList(t2.Object.Value, triples) {
				if m.Kind != TermIRI {
					valid = false
					break
				}
				iris = append(iris, m.Value)
			}
			if !valid || len(iris) < 2 {
				c.plan.Report.addIgnored("AllDisjoint 成员非法", sprintf("成员数 %d 或含匿名节点", len(iris)))
				continue
			}
			if t.Object.Value == IRIOWLAllDisjointClasses {
				for _, pair := range combinations2(iris) {
					c.assembleClassPair(ontology.AxiomDisjointWith, pair[0], pair[1], "AllDisjoint 不相交类")
				}
			} else {
				for _, pair := range combinations2(iris) {
					c.assemblePropertyPair(ontology.AxiomDisjointObjectProperties, ontology.AxiomDisjointDataProperties, pair[0], pair[1])
				}
			}
		}
	}
}

// combinations2 全部两两组合（保序）。
func combinations2(items []string) [][2]string {
	out := make([][2]string, 0, len(items)*(len(items)-1)/2)
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			out = append(out, [2]string{items[i], items[j]})
		}
	}
	return out
}

// assembleBuiltinAnnotation 100 内建轨草稿（deprecated 归一 / notation 剥 lang / 空白值拦截）。
func (c *importCtx) assembleBuiltinAnnotation(subjectIRI, predicate string, obj Term) {
	if obj.Kind == TermIRI {
		c.plan.Report.addIgnored("IRI 型注解值", sprintf("%s %s %s", subjectIRI, shortPredicate(predicate), obj.Value))
		return
	}
	if obj.Kind != TermLiteral {
		return
	}
	value := obj.Value
	lang := obj.Lang
	if predicate == ontology.PredicateOWLDeprecated {
		// 布尔词形归一（§2.6 #6）：true/false/1/0；其余进忽略清单
		switch value {
		case "true", "1":
			value = "true"
		case "false", "0":
			value = "false"
		default:
			c.plan.Report.addIgnored("owl:deprecated 值非布尔", sprintf("%s owl:deprecated %s", subjectIRI, literalText(obj)))
			return
		}
		lang = ""
	}
	if predicate == ontology.PredicateSKOSNotation && lang != "" {
		c.plan.Report.Warnings = append(c.plan.Report.Warnings,
			sprintf("skos:notation 不支持语言标签，已剥离（%s）", subjectIRI))
		lang = ""
	}
	if strings.TrimSpace(value) == "" {
		c.plan.Report.addIgnored("注解值为空白", sprintf("%s %s", subjectIRI, shortPredicate(predicate)))
		return
	}
	c.appendAxiom(AxiomDraft{
		AxiomType:  ontology.AxiomAnnotationAssertion,
		SubjectRef: c.refOf(subjectIRI),
		ObjectRef:  -1,
		PredicateIRI: predicate, PredicateRef: -1,
		ValueText: value,
		Lang:      lang,
	})
}

// assembleCustomAnnotation 100 object_id 轨草稿（谓词 = 注解属性实体）。
func (c *importCtx) assembleCustomAnnotation(subjectIRI, predicateIRI string, obj Term) {
	if strings.TrimSpace(obj.Value) == "" {
		c.plan.Report.addIgnored("注解值为空白", sprintf("%s %s", subjectIRI, shortPredicate(predicateIRI)))
		return
	}
	c.appendAxiom(AxiomDraft{
		AxiomType:  ontology.AxiomAnnotationAssertion,
		SubjectRef: c.refOf(subjectIRI),
		ObjectRef:  -1,
		PredicateRef: c.refOf(predicateIRI),
		ValueText:  obj.Value,
		Lang:       obj.Lang,
	})
}

// assembleObjectAssertion 61 行草稿（谓词对象属性 + 两端个体）。
func (c *importCtx) assembleObjectAssertion(subjectIRI, predicateIRI, objectIRI string) {
	st, sok := c.typeOf(subjectIRI)
	ot, ook := c.typeOf(objectIRI)
	pt, pok := c.typeOf(predicateIRI)
	if pok && pt != ontology.EntityTypeObjectProperty {
		c.plan.Report.addIgnored("断言谓词类型不符", sprintf("%s 的谓词 %s 不是对象属性", subjectIRI, predicateIRI))
		return
	}
	if !sok || st != ontology.EntityTypeIndividual {
		c.plan.Report.addIgnored("断言主语不是个体", sprintf("%s %s %s", subjectIRI, shortPredicate(predicateIRI), objectIRI))
		return
	}
	if !ook || ot != ontology.EntityTypeIndividual {
		c.plan.Report.addIgnored("断言宾语不是个体", sprintf("%s %s %s", subjectIRI, shortPredicate(predicateIRI), objectIRI))
		return
	}
	c.appendAxiom(AxiomDraft{
		AxiomType:  ontology.AxiomObjectPropertyAssertion,
		SubjectRef: c.refOf(subjectIRI),
		ObjectRef:  c.refOf(objectIRI),
		PredicateIRI: predicateIRI, PredicateRef: -1,
	})
}

// assembleDataAssertion 62 行草稿（按首条 42 推断 datatype 的词形预校验；lang 轨豁免；
// typed datatype 偏差 → warning 落行——§2.6 #3）。
func (c *importCtx) assembleDataAssertion(t Triple) {
	subjectIRI, predicateIRI, obj := t.Subject.Value, t.Predicate.Value, t.Object
	st, sok := c.typeOf(subjectIRI)
	if !sok || st != ontology.EntityTypeIndividual {
		c.plan.Report.addIgnored("断言主语不是个体", sprintf("%s %s %s", subjectIRI, shortPredicate(predicateIRI), literalText(obj)))
		return
	}
	pt, pok := c.typeOf(predicateIRI)
	if pok && pt != ontology.EntityTypeDataProperty {
		c.plan.Report.addIgnored("断言谓词不是数据属性", sprintf("%s 的谓词 %s 不是数据属性", subjectIRI, predicateIRI))
		return
	}

	// lang 轨豁免（§2.6 #5）：带语言标签的字面量不受值域词法闸门约束
	if obj.Lang != "" {
		c.appendAxiom(AxiomDraft{
			AxiomType:  ontology.AxiomDataPropertyAssertion,
			SubjectRef: c.refOf(subjectIRI),
			ObjectRef:  c.refOf(predicateIRI),
			PredicateRef: -1,
			ValueText:  obj.Value,
			Lang:       obj.Lang,
		})
		return
	}

	// 62 行模型无 datatype 列：typed literal 的 datatype 与推断值域偏差时按推断落行 + warning
	inferredIRI, def, passthrough := c.inferDatatype(predicateIRI)
	if obj.Datatype != "" && obj.Datatype != ontology.IRIRDFLangString &&
		inferredIRI != "" && obj.Datatype != inferredIRI {
		c.plan.Report.Warnings = append(c.plan.Report.Warnings,
			sprintf("字面量声明 %s，按值域 %s 落行（%s）", shortPredicate(obj.Datatype), shortPredicate(inferredIRI), subjectIRI))
	}

	if !passthrough {
		valid := false
		if def != nil {
			valid = ontology.ValidateLexicalDerived(obj.Value, *def)
		} else {
			valid = ontology.ValidateLexical(inferredIRI, obj.Value)
		}
		if !valid {
			c.plan.Report.addIgnored("词形与值域不符",
				sprintf("字面量 %s 不在 %s 的词法空间（%s）", literalText(obj), inferredIRI, subjectIRI))
			return
		}
	}
	c.appendAxiom(AxiomDraft{
		AxiomType:  ontology.AxiomDataPropertyAssertion,
		SubjectRef: c.refOf(subjectIRI),
		ObjectRef:  c.refOf(predicateIRI),
		PredicateRef: -1,
		ValueText:  obj.Value,
	})
}

// inferDatatype 按首条 42 行推断 62 的 datatype（03 号文档 §2.6 口径）：
// 无 42 行 → 缺省 xsd:string；内建轨 → IRI；派生轨 → (datatype 实体 IRI, 定义)；
// 派生轨未定义 → passthrough（无词法空间可校验，与校验器放行口径一致）。
func (c *importCtx) inferDatatype(propertyIRI string) (string, *ontology.DatatypeDefinition, bool) {
	ranges := c.rangeByProperty[propertyIRI]
	if len(ranges) == 0 {
		return ontology.IRIXMLSchemaString, nil, false
	}
	first := ranges[0].IRI
	if _, builtin := ontology.IsBuiltinDatatype(first); builtin {
		return first, nil, false
	}
	if def, ok := c.definitionOf(first); ok {
		return first, &def, false
	}
	return first, nil, true
}

// definitionOf datatype 实体的 46 定义（草稿里查首行）。
func (c *importCtx) definitionOf(datatypeIRI string) (ontology.DatatypeDefinition, bool) {
	ref := c.refOf(datatypeIRI)
	if ref < 0 {
		return ontology.DatatypeDefinition{}, false
	}
	for _, d := range c.axioms {
		if d.AxiomType == ontology.AxiomDatatypeDefinition && d.SubjectRef == ref {
			def, err := ontology.ParseDefinition(d.ValueText)
			if err != nil {
				return ontology.DatatypeDefinition{}, false
			}
			return def, true
		}
	}
	return ontology.DatatypeDefinition{}, false
}

func typeOrUnknown(t uint32, ok bool) string {
	if !ok {
		return "未知"
	}
	return entityTypeLabel(t)
}

// finishPlan 上限检查、统计与推断（07 号文档 §4.3 步骤 4/5）。
func (c *importCtx) finishPlan() error {
	if len(c.plan.Entities) > MaxImportEntites {
		return ontology.NewError(ontology.ReasonOWLImportLimitExceeded,
			"文件内容超过导入上限（实体 %d > %d），请拆分后导入", len(c.plan.Entities), MaxImportEntites)
	}
	if len(c.axioms) > MaxImportAxioms {
		return ontology.NewError(ontology.ReasonOWLImportLimitExceeded,
			"文件内容超过导入上限（公理 %d > %d），请拆分后导入", len(c.axioms), MaxImportAxioms)
	}
	if len(c.plan.Entities) == 0 {
		return ontology.NewError(ontology.ReasonOWLImportEmpty,
			"文件中未发现可导入的实体")
	}

	// 公理统计（axiom_type → 行数）
	for _, d := range c.axioms {
		c.plan.Report.AxiomStats[d.AxiomType]++
	}

	// 本体名推断：ontology_iri 尾段，缺省 = 文件名去扩展名
	name := ""
	if c.ontologyIRI != "" {
		name = lastIRISegment(c.ontologyIRI)
	}
	if name == "" {
		name = defaultOntologyName(c.fileName)
	}
	c.plan.Report.SuggestedName = name
	c.plan.Report.SuggestedBaseIRI = majorityNamespace(c.plan.Entities, name)
	c.plan.Report.SuggestedOntologyIRI = c.ontologyIRI
	c.plan.Report.SuggestedDescription = c.plan.Description
	c.plan.Report.SourceFormat = c.plan.SourceFormat.Label()
	return nil
}

// majorityNamespace 实体 IRI 命名空间众数推断（§2.3 #2：按计数取最长、并列取更长；
// 对单个跨命名空间离群实体稳健——源实现修订 #2）。无实体回退 https://owl-import.local/…。
func majorityNamespace(entities []EntityDraft, ontologyName string) string {
	counts := map[string]int{}
	for _, e := range entities {
		ns := namespaceOf(e.IRI)
		if ns == "" {
			continue
		}
		counts[ns]++
	}
	best, bestCount := "", 0
	for ns, n := range counts {
		if n > bestCount || (n == bestCount && len(ns) > len(best)) {
			best, bestCount = ns, n
		}
	}
	if best != "" {
		return best
	}
	return "https://owl-import.local/ontology/" + slugify(ontologyName) + "#"
}

// namespaceOf IRI 的命名空间（最后 '#' 截断优先，其次 '/'）。
func namespaceOf(iri string) string {
	if idx := lastByteIndex(iri, '#'); idx >= 0 {
		return iri[:idx+1]
	}
	if idx := lastByteIndex(iri, '/'); idx >= 0 {
		return iri[:idx+1]
	}
	return ""
}

// defaultOntologyName 文件名去扩展名。
func defaultOntologyName(fileName string) string {
	name := fileName
	if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndexByte(name, '.'); idx > 0 {
		name = name[:idx]
	}
	if strings.TrimSpace(name) == "" {
		return "imported"
	}
	return name
}

// slugify 名称 → URL 安全段（非字母数字替换为连字符；空串回退 imported）。
func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "imported"
	}
	return out
}
