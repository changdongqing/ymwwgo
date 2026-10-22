package ontology

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// notationPattern skos:notation 字符集（编码语义与取号机制见 06 号文档，本期仅手工录入 + 校验）
var notationPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidateAxioms 纯 Go 结构校验器（替代源 OntAxiomCompiler 的 OWLAPI 投影，决策五）。
//
// V1 校验项（设计文档 §3.6）：
//  1. IRI 语法：每实体 iri 可被 url.Parse 解析为绝对 IRI 且以本体 base_iri 为前缀；
//  2. 引用存在性与类型匹配：20/21/22 两端均须为本体下未删的类；
//  3. 声明完备性：每实体恰有一条未删 DECLARATION(10)，反之每条 10 型公理主体存在（防御校验）；
//  4. 注解断言双轨：100 型谓词必须命中内建白名单（04 期自定义注解属性激活 object_id 双轨后放开），
//     lang ≤ 16 字符；
//  5. 值词法：owl:deprecated ∈ {"true","false"}；skos:notation ∈ [A-Za-z0-9_-]+；
//  6. 层级无环：环检测在写路径前置执行（AssertNoCycle），校验器不重复（职责分工，与源一致）。
//
// 未知 axiom_type（02~06 期号段尚未启用）WARN 跳过、不报错。返回全部问题（空切片 = 通过）。
func ValidateAxioms(ctx context.Context, baseIRI string, entities []*ent.OntEntity, axioms []*ent.OntAxiom, log Logger) []error {
	problems := make([]error, 0)

	byID := make(map[uint32]*ent.OntEntity, len(entities))
	for _, e := range entities {
		byID[e.ID] = e
	}

	// 1. IRI 语法
	for _, e := range entities {
		iri := nilString(e.Iri)
		if u, err := url.Parse(iri); err != nil || !u.IsAbs() {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"实体 %s 的 IRI %s 不是绝对 IRI", entityLabel(e), iri))
			continue
		}
		if !strings.HasPrefix(iri, baseIRI) {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"实体 %s 的 IRI %s 不以本体命名空间 %s 为前缀", entityLabel(e), iri, baseIRI))
		}
	}

	// 2. 引用存在性与类型匹配（20/21/22 两端均须为类）
	entityTypeOf := func(id uint32) (uint32, bool) {
		e, ok := byID[id]
		if !ok || e.EntityType == nil {
			return 0, false
		}
		return *e.EntityType, true
	}
	for _, ax := range axioms {
		if ax.AxiomType == nil {
			continue
		}
		t := *ax.AxiomType
		if t != AxiomSubClassOf && t != AxiomDisjointWith && t != AxiomEquivalentClasses {
			continue
		}
		if ax.SubjectID == nil {
			problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语", ax.ID))
			continue
		}
		for _, end := range []struct {
			role string
			id   uint32
		}{{"主语", *ax.SubjectID}} {
			if et, ok := entityTypeOf(end.id); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的%s实体 %d 不存在或已删除", ax.ID, end.role, end.id))
			} else if et != EntityTypeClass {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的%s实体 %d 类型不符（期望类）", ax.ID, end.role, end.id))
			}
		}
		if ax.ObjectID != nil && *ax.ObjectID != 0 {
			if et, ok := entityTypeOf(*ax.ObjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的宾语实体 %d 不存在或已删除", ax.ID, *ax.ObjectID))
			} else if et != EntityTypeClass {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的宾语实体 %d 类型不符（期望类）", ax.ID, *ax.ObjectID))
			}
		} else {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"公理 %d 缺少宾语实体", ax.ID))
		}
	}

	// 2b. 02 期对象属性公理（30~37）：引用存在性与类型匹配（02 号文档 §4.4）
	for _, ax := range axioms {
		if ax.AxiomType == nil {
			continue
		}
		t := *ax.AxiomType
		switch t {
		case AxiomSubObjectPropertyOf, AxiomInverseOf, AxiomEquivalentObjectProperties, AxiomDisjointObjectProperties:
			// 两端实体均存在、未删、entity_type = OBJECT_PROPERTY
			if ax.SubjectID == nil || ax.ObjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少属性两端实体", ax.ID))
				continue
			}
			for _, end := range []struct {
				role string
				id   uint32
			}{{"主语", *ax.SubjectID}, {"宾语", *ax.ObjectID}} {
				if et, ok := entityTypeOf(end.id); !ok {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的%s实体 %d 不存在或已删除", ax.ID, end.role, end.id))
				} else if et != EntityTypeObjectProperty {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的%s实体 %d 类型不符（期望对象属性）", ax.ID, end.role, end.id))
				}
			}
		case AxiomObjectPropertyDomain, AxiomObjectPropertyRange:
			// subject 为对象属性、object 为已声明类
			if ax.SubjectID == nil || ax.ObjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语或宾语实体", ax.ID))
				continue
			}
			if et, ok := entityTypeOf(*ax.SubjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 不存在或已删除", ax.ID, *ax.SubjectID))
			} else if et != EntityTypeObjectProperty {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 类型不符（期望对象属性）", ax.ID, *ax.SubjectID))
			}
			if et, ok := entityTypeOf(*ax.ObjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的宾语实体 %d 不存在或已删除", ax.ID, *ax.ObjectID))
			} else if et != EntityTypeClass {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的宾语实体 %d 类型不符（期望类）", ax.ID, *ax.ObjectID))
			}
		case AxiomObjectPropertyCharacteristic:
			// predicate_iri ∈ 特征白名单（服务层已拦，校验器兜底直改库脏数据）；subject 须为对象属性
			if ax.SubjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语", ax.ID))
				continue
			}
			if et, ok := entityTypeOf(*ax.SubjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 不存在或已删除", ax.ID, *ax.SubjectID))
			} else if et != EntityTypeObjectProperty {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 类型不符（期望对象属性）", ax.ID, *ax.SubjectID))
			}
			predicate := nilString(ax.PredicateIri)
			if !isPropertyCharacteristicIRI(predicate) {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的对象属性特征 %s 不在白名单内", ax.ID, predicate))
			}
		case AxiomSubPropertyChainOf:
			// subject=超属性（对象属性）；value_text 为合法 JSON 数组、成员 ≥2、逐个命中已声明对象属性
			if ax.SubjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语", ax.ID))
				continue
			}
			if et, ok := entityTypeOf(*ax.SubjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 不存在或已删除", ax.ID, *ax.SubjectID))
			} else if et != EntityTypeObjectProperty {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 类型不符（期望对象属性）", ax.ID, *ax.SubjectID))
			}
			value := nilString(ax.ValueText)
			members, err := ParseChainMemberIDs(value)
			if err != nil {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的属性链成员不合法：%s", ax.ID, value))
				continue
			}
			if len(members) < 2 {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的属性链成员数 %d 不足（至少 2 个）", ax.ID, len(members)))
				continue
			}
			for _, m := range members {
				if et, ok := entityTypeOf(m); !ok || et != EntityTypeObjectProperty {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的属性链成员 %d 不是已声明的对象属性", ax.ID, m))
				}
			}

		// 2c. 03 期数据属性公理（40~45）+ 62 数据断言分支（03 号文档 §4.4）
		case AxiomSubDataPropertyOf, AxiomEquivalentDataProperties, AxiomDisjointDataProperties:
			// 两端实体均存在、未删、entity_type = DATA_PROPERTY
			if ax.SubjectID == nil || ax.ObjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少属性两端实体", ax.ID))
				continue
			}
			for _, end := range []struct {
				role string
				id   uint32
			}{{"主语", *ax.SubjectID}, {"宾语", *ax.ObjectID}} {
				if et, ok := entityTypeOf(end.id); !ok {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的%s实体 %d 不存在或已删除", ax.ID, end.role, end.id))
				} else if et != EntityTypeDataProperty {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的%s实体 %d 类型不符（期望数据属性）", ax.ID, end.role, end.id))
				}
			}
		case AxiomDataPropertyDomain:
			// subject 为数据属性、object 为已声明类
			if ax.SubjectID == nil || ax.ObjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语或宾语实体", ax.ID))
				continue
			}
			if et, ok := entityTypeOf(*ax.SubjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 不存在或已删除", ax.ID, *ax.SubjectID))
			} else if et != EntityTypeDataProperty {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 类型不符（期望数据属性）", ax.ID, *ax.SubjectID))
			}
			if et, ok := entityTypeOf(*ax.ObjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的宾语实体 %d 不存在或已删除", ax.ID, *ax.ObjectID))
			} else if et != EntityTypeClass {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的宾语实体 %d 类型不符（期望类）", ax.ID, *ax.ObjectID))
			}
		case AxiomDataPropertyRange:
			// value_text ∈ 内建 datatype 白名单（服务层已拦，校验器兜底直改库脏数据）；
			// V1 仅 value_text 轨——object_id 槽位留给 05 期自定义 datatype 实体（双轨激活前非空即非法）
			if ax.SubjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语", ax.ID))
				continue
			}
			if et, ok := entityTypeOf(*ax.SubjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 不存在或已删除", ax.ID, *ax.SubjectID))
			} else if et != EntityTypeDataProperty {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 类型不符（期望数据属性）", ax.ID, *ax.SubjectID))
			}
			if ax.ObjectID != nil && *ax.ObjectID != 0 {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的值域行占用 object_id（自定义 datatype 实体 05 期开放）", ax.ID))
			}
			value := nilString(ax.ValueText)
			if _, ok := IsBuiltinDatatype(value); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的值域 %s 不是内建数据类型", ax.ID, value))
			}
		case AxiomDataPropertyCharacteristic:
			// predicate_iri = 函数性常量（数据属性 V1 唯一特征；≠ 则 AXIOM_INVALID）
			if ax.SubjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语", ax.ID))
				continue
			}
			if et, ok := entityTypeOf(*ax.SubjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 不存在或已删除", ax.ID, *ax.SubjectID))
			} else if et != EntityTypeDataProperty {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 类型不符（期望数据属性）", ax.ID, *ax.SubjectID))
			}
			predicate := nilString(ax.PredicateIri)
			if predicate != IRIOWLFunctionalProperty {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的数据属性特征 %s 非法（数据属性仅支持函数性）", ax.ID, predicate))
			}
		case AxiomDataPropertyAssertion:
			// 62 数据断言（03 期定稿行语义，写入方 06 期）：subject=已声明个体、object=已声明数据属性；
			// 字面量词法：lang 有值 → 仅要求 value_text 非空（rdf:langString）；
			// 否则按该属性第一条未删 42 行推断 datatype（缺省 xsd:string）→ 词法校验（03 号文档 §2.6）
			if ax.SubjectID == nil || ax.ObjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语或谓词实体", ax.ID))
				continue
			}
			if et, ok := entityTypeOf(*ax.SubjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 不存在或已删除", ax.ID, *ax.SubjectID))
			} else if et != EntityTypeIndividual {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 类型不符（期望个体）", ax.ID, *ax.SubjectID))
			}
			if et, ok := entityTypeOf(*ax.ObjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的谓词实体 %d 不存在或已删除", ax.ID, *ax.ObjectID))
			} else if et != EntityTypeDataProperty {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的谓词实体 %d 类型不符（期望数据属性）", ax.ID, *ax.ObjectID))
			}
			value := nilString(ax.ValueText)
			if ax.Lang != nil && *ax.Lang != "" {
				if value == "" {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的语言文本字面量为空", ax.ID))
				}
				continue
			}
			datatype := datatypeRangeOf(axioms, *ax.ObjectID)
			if !ValidateLexical(datatype, value) {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的字面量 %q 不符合 %s 的词法", ax.ID, value, datatype))
			}
		}
	}

	// 3. 声明完备性（双向防御）
	declarationCount := make(map[uint32]int, len(entities))
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != AxiomDeclaration {
			continue
		}
		if ax.SubjectID == nil {
			problems = append(problems, NewError(ReasonAxiomInvalid, "声明公理 %d 缺少主语", ax.ID))
			continue
		}
		if _, ok := byID[*ax.SubjectID]; !ok {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"声明公理 %d 的主体实体 %d 不存在（删除流程应保证同步）", ax.ID, *ax.SubjectID))
			continue
		}
		declarationCount[*ax.SubjectID]++
	}
	for _, e := range entities {
		if n := declarationCount[e.ID]; n != 1 {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"实体 %s 的声明公理数量为 %d（应为 1）", entityLabel(e), n))
		}
	}

	// 4/5. 注解断言谓词白名单、lang 长度与值词法
	whitelist := make(map[string]struct{}, len(BuiltinAnnotationPredicates))
	for _, p := range BuiltinAnnotationPredicates {
		whitelist[p] = struct{}{}
	}
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != AxiomAnnotationAssertion {
			continue
		}

		predicate := nilString(ax.PredicateIri)
		if _, ok := whitelist[predicate]; !ok {
			// object_id 双轨（04 期激活）：谓词白名单外时，宾语必须是注解属性实体才合法；
			// 本期注解属性实体尚无法创建，等价于白名单外一律拒绝
			customOK := false
			if ax.ObjectID != nil && *ax.ObjectID != 0 {
				if et, ok := entityTypeOf(*ax.ObjectID); ok && et == EntityTypeAnnotationProperty {
					customOK = true
				}
			}
			if !customOK {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的注解谓词 %s 不在白名单内（自定义注解属性 04 期开放）", ax.ID, predicate))
			}
		}

		if ax.Lang != nil && utf8.RuneCountInString(*ax.Lang) > 16 {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"公理 %d 的语言标签超长（≤16 字符）", ax.ID))
		}

		value := nilString(ax.ValueText)
		switch predicate {
		case PredicateOWLDeprecated:
			if value != "true" && value != "false" {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的 owl:deprecated 值 %q 非法（仅允许 true/false）", ax.ID, value))
			}
		case PredicateSKOSNotation:
			if !notationPattern.MatchString(value) {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的编码 %q 含非法字符（仅允许字母/数字/下划线/连字符）", ax.ID, value))
			}
		}
	}

	// 未知 axiom_type：WARN 跳过（05~06 期号段尚未启用，兼容增量；62 分支 03 期已定稿启用）
	known := map[uint32]struct{}{
		AxiomDeclaration:                  {},
		AxiomSubClassOf:                   {},
		AxiomDisjointWith:                 {},
		AxiomEquivalentClasses:            {},
		AxiomSubObjectPropertyOf:          {},
		AxiomObjectPropertyDomain:         {},
		AxiomObjectPropertyRange:          {},
		AxiomObjectPropertyCharacteristic: {},
		AxiomInverseOf:                    {},
		AxiomEquivalentObjectProperties:   {},
		AxiomDisjointObjectProperties:     {},
		AxiomSubPropertyChainOf:           {},
		AxiomSubDataPropertyOf:            {},
		AxiomDataPropertyDomain:           {},
		AxiomDataPropertyRange:            {},
		AxiomDataPropertyCharacteristic:   {},
		AxiomEquivalentDataProperties:     {},
		AxiomDisjointDataProperties:       {},
		AxiomDataPropertyAssertion:        {},
		AxiomAnnotationAssertion:          {},
	}
	for _, ax := range axioms {
		if ax.AxiomType == nil {
			continue
		}
		if _, ok := known[*ax.AxiomType]; !ok {
			warnf(log, ctx, "ontology validator: 未知公理类型 %d（公理 %d），跳过校验", *ax.AxiomType, ax.ID)
		}
	}

	return problems
}

func entityLabel(e *ent.OntEntity) string {
	name := nilString(e.LocalName)
	if name == "" {
		name = strconv.FormatUint(uint64(e.ID), 10)
	}
	return name
}

// datatypeRangeOf 取数据属性第一条未删 42 行的 value_text 作为断言 datatype 推断依据；
// 无值域行（或值域行 value_text 为空）缺省 xsd:string（03 号文档 §2.6：datatype 不落列，
// 由所在属性的值域行推断，避免双事实源漂移）。
func datatypeRangeOf(axioms []*ent.OntAxiom, propertyID uint32) string {
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != AxiomDataPropertyRange {
			continue
		}
		if ax.SubjectID == nil || *ax.SubjectID != propertyID {
			continue
		}
		if v := nilString(ax.ValueText); v != "" {
			return v
		}
	}
	return IRIXMLSchemaString
}

// isPropertyCharacteristicIRI 特征谓词白名单判定
func isPropertyCharacteristicIRI(iri string) bool {
	for _, v := range PropertyCharacteristicIRIs {
		if v == iri {
			return true
		}
	}
	return false
}
