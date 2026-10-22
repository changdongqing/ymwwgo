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

// notationPattern skos:notation 字符集（06 号文档 §3.3 定案回填——允许字母/数字/点/下划线/连字符，
// 首字符为字母或数字，允许纯数字前缀（行政区划码）；与个体编码字符集同一正则 serial.go codeCharsetPattern）。
var notationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateAxioms 纯 Go 结构校验器（替代源 OntAxiomCompiler 的 OWLAPI 投影，决策五）。
//
// V1 校验项（设计文档 §3.6）：
//  1. IRI 语法：每实体 iri 可被 url.Parse 解析为绝对 IRI 且以本体 base_iri 为前缀；
//  2. 引用存在性与类型匹配：20/21/22 两端均须为本体下未删的类；
//  3. 声明完备性：每实体恰有一条未删 DECLARATION(10)，反之每条 10 型公理主体存在（防御校验）；
//  4. 注解断言双轨（04 期激活）：object_id 非空 → 宾语须为本本体已声明的注解属性实体；
//     否则 predicate_iri ∈ 内建白名单；二者皆空拒绝。lang ≤ 16 字符、value_text 非空白；
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
	// 61 断言谓词反查（predicate_iri → 本本体实体；内建谓词行不走此路径）
	byIRI := make(map[string]*ent.OntEntity, len(entities))
	for _, e := range entities {
		if e.Iri != nil {
			byIRI[*e.Iri] = e
		}
	}

	// 1. IRI 语法（07 期 IRI 保真定案放宽：不再要求以本体 base_iri 为前缀——导入的实体
	//    IRI 原样落库，base_iri 只服务"本系统新建实体时的 IRI 生成"；01 号文档"实体 IRI =
	//    base_iri + local_name"是新建路径的生成规则，非结构校验项）
	for _, e := range entities {
		iri := nilString(e.Iri)
		if u, err := url.Parse(iri); err != nil || !u.IsAbs() {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"实体 %s 的 IRI %s 不是绝对 IRI", entityLabel(e), iri))
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

		// 2d. 04 期注解属性公理（50）：两端实体均存在、未删、entity_type = ANNOTATION_PROPERTY
		// （04 号文档 §4.5；环检测在写路径前置 AssertNoCycle，校验器不重复）
		case AxiomSubAnnotationPropertyOf:
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
				} else if et != EntityTypeAnnotationProperty {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的%s实体 %d 类型不符（期望注解属性）", ax.ID, end.role, end.id))
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
			// 42 值域双轨（05 期激活，05 号文档 §4.7；服务层三分支已拦，校验器兜底直改库脏数据）：
			//   派生轨：object_id 非空 → 实体须为本本体已声明 DATATYPE（二选一不双写，object_id 优先）；
			//   内建轨：object_id 空 → value_text ∈ 内建白名单（03 期存量行为）
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
			if objectID := nilUint(ax.ObjectID); objectID != 0 {
				if et, ok := entityTypeOf(objectID); !ok {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的值域引用实体 %d 不存在或已删除", ax.ID, objectID))
				} else if et != EntityTypeDatatype {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的值域引用必须是数据类型实体：%d", ax.ID, objectID))
				}
			} else {
				value := nilString(ax.ValueText)
				if _, ok := IsBuiltinDatatype(value); !ok {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的值域 %s 不是内建数据类型", ax.ID, value))
				}
			}
		case AxiomDatatypeDefinition:
			// 46 定义公理（05 期新增）：subject 须为已声明 datatype 实体；value_text 须可解析且
			// 结构合法（base 白名单 / facet 白名单 / 适用族 / 值词法 / 静态冲突——服务层已拦，
			// 兜底直改库脏数据）。多行 46 属脏数据：读取侧取首行、服务层 diff 自愈删多余行，
			// 校验器对每行独立校验、不走错误（源修订 #12 内化）
			if ax.SubjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语", ax.ID))
				continue
			}
			if et, ok := entityTypeOf(*ax.SubjectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 不存在或已删除", ax.ID, *ax.SubjectID))
			} else if et != EntityTypeDatatype {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的主语实体 %d 类型不符（期望数据类型）", ax.ID, *ax.SubjectID))
			}
			def, err := ParseDefinition(nilString(ax.ValueText))
			if err == nil {
				err = ValidateDefinition(def)
			}
			if err != nil {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的数据类型定义不合法：%s", ax.ID, err.Error()))
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
		case AxiomTypeAssertion:
			// 60 类型断言（06 期新增，06 号文档 §4.5）：subject 为已声明个体、object 为已声明类；
			// sort_order=0 即主类目（语义在服务层与 frame 装载，校验器只做结构校验）
			if ax.SubjectID == nil || ax.ObjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语或宾语实体", ax.ID))
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
					"公理 %d 的宾语实体 %d 不存在或已删除", ax.ID, *ax.ObjectID))
			} else if et != EntityTypeClass {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的宾语实体 %d 类型不符（期望类）", ax.ID, *ax.ObjectID))
			}
		case AxiomObjectPropertyAssertion:
			// 61 对象属性断言（06 期新增，02 号文档 §2.6 的本期校验义务）：subject/object 为
			// 已声明个体；predicate_iri 须命中本本体已声明对象属性（entity_type=20）
			if ax.SubjectID == nil || ax.ObjectID == nil {
				problems = append(problems, NewError(ReasonAxiomInvalid, "公理 %d 缺少主语或宾语实体", ax.ID))
				continue
			}
			for _, end := range []struct {
				role string
				id   uint32
			}{{"主语", *ax.SubjectID}, {"宾语", *ax.ObjectID}} {
				if et, ok := entityTypeOf(end.id); !ok {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的%s实体 %d 不存在或已删除", ax.ID, end.role, end.id))
				} else if et != EntityTypeIndividual {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的%s实体 %d 类型不符（期望个体）", ax.ID, end.role, end.id))
				}
			}
			predicate := nilString(ax.PredicateIri)
			if predicate == "" {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 缺少对象属性谓词 IRI", ax.ID))
			} else {
				if predicateEntity, ok := byIRI[predicate]; !ok {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的谓词 %s 不是本本体已声明的对象属性", ax.ID, predicate))
				} else if predicateEntity.EntityType == nil || *predicateEntity.EntityType != EntityTypeObjectProperty {
					problems = append(problems, NewError(ReasonAxiomInvalid,
						"公理 %d 的谓词 %s 类型不符（期望对象属性）", ax.ID, predicate))
				}
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
			// 词法链三段分派（05 号文档 §2.6，03 号文档 §4.4 预告的兑现）
			if !validateAssertionLexical(value, axioms, *ax.ObjectID) {
				datatypeLabel := assertionDatatypeLabel(axioms, *ax.ObjectID, byID)
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的字面量 %q 不符合 %s 的词法", ax.ID, value, datatypeLabel))
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

	// 3b. 个体必须有型（06 号文档 §4.5 防御项）：每个未删 INDIVIDUAL 实体存在至少一条未删 60 行。
	//     创建不变量（服务层必选主类目）+ 删除保护（类删除双态拒绝）保证；此处兜底直改库场景。
	typeAssertionCount := make(map[uint32]int, len(entities))
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != AxiomTypeAssertion {
			continue
		}
		if ax.SubjectID == nil {
			continue
		}
		typeAssertionCount[*ax.SubjectID]++
	}
	for _, e := range entities {
		if e.EntityType != nil && *e.EntityType == EntityTypeIndividual && typeAssertionCount[e.ID] == 0 {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"个体 %s 缺少类型断言（个体必须至少挂一个类型）", entityLabel(e)))
		}
	}

	// 4/5. 注解断言谓词双轨（04 号文档 §4.5）、lang 长度与值词法
	whitelist := make(map[string]struct{}, len(BuiltinAnnotationPredicates))
	for _, p := range BuiltinAnnotationPredicates {
		whitelist[p] = struct{}{}
	}
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != AxiomAnnotationAssertion {
			continue
		}

		// 谓词双轨二选一（服务层已拦，校验器兜底直改库场景）：
		//   自定义轨：object_id 非空 → 实体必须是本本体已声明的注解属性；
		//   内建轨：object_id 空 → predicate_iri ∈ 内建白名单（01~03 期存量行为，不变）；
		//   二者皆空 → 拒绝。object_id 优先，predicate_iri 与 object_id 并存按自定义轨判。
		predicate := nilString(ax.PredicateIri)
		if objectID := nilUint(ax.ObjectID); objectID != 0 {
			if et, ok := entityTypeOf(objectID); !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的注解谓词实体 %d 不存在或已删除", ax.ID, objectID))
			} else if et != EntityTypeAnnotationProperty {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的注解谓词必须是注解属性实体：%d", ax.ID, objectID))
			}
		} else {
			if _, ok := whitelist[predicate]; !ok {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的注解谓词 %s 不在白名单内（自定义注解谓词须指向注解属性实体）", ax.ID, predicate))
			}
		}

		if ax.Lang != nil && utf8.RuneCountInString(*ax.Lang) > 16 {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"公理 %d 的语言标签超长（≤16 字符）", ax.ID))
		}

		value := nilString(ax.ValueText)
		if strings.TrimSpace(value) == "" {
			problems = append(problems, NewError(ReasonAxiomInvalid,
				"公理 %d 的注解值为空白", ax.ID))
		}
		switch predicate {
		case PredicateOWLDeprecated:
			if value != "true" && value != "false" {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的 owl:deprecated 值 %q 非法（仅允许 true/false）", ax.ID, value))
			}
		case PredicateSKOSNotation:
			if !notationPattern.MatchString(value) {
				problems = append(problems, NewError(ReasonAxiomInvalid,
					"公理 %d 的编码 %q 含非法字符（允许字母/数字/点/下划线/连字符，首字符为字母或数字）", ax.ID, value))
			}
		}
	}

	// 未知 axiom_type：WARN 跳过（06 期号段尚未启用，兼容增量；62 分支 03 期已定稿、46 分支 05 期启用）
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
		AxiomDatatypeDefinition:           {},
		AxiomSubAnnotationPropertyOf:      {},
		AxiomTypeAssertion:                {},
		AxiomObjectPropertyAssertion:      {},
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

// validateAssertionLexical 62 断言词法链三段分派（05 号文档 §2.6，03 号文档 §4.4 预告的兑现）：
// 首条 42 行（按 sort_order/id 最小，03 期规则）→
//
//	内建轨（value_text 非空）→ 词法校验；派生轨（object_id 非空）→ 有定义 base+facet 校验、
//	未定义放行（无词法空间可校验）；无 42 行或脏空行 → 缺省 xsd:string。
func validateAssertionLexical(value string, axioms []*ent.OntAxiom, propertyID uint32) bool {
	row := firstDataPropertyRangeRow(axioms, propertyID)
	if row == nil {
		return ValidateLexical(IRIXMLSchemaString, value)
	}
	if v := nilString(row.ValueText); v != "" {
		return ValidateLexical(v, value)
	}
	if datatypeID := nilUint(row.ObjectID); datatypeID != 0 {
		def, defined := datatypeDefinitionOf(axioms, datatypeID)
		if !defined {
			return true
		}
		return ValidateLexicalDerived(value, def)
	}
	return ValidateLexical(IRIXMLSchemaString, value)
}

// assertionDatatypeLabel 62 词法错误的 datatype 展示标签：内建轨/缺省取 IRI，派生轨取
// datatype 实体 IRI（实体缺失回落 #id——与 42 分支的实体错误并行报出）。
func assertionDatatypeLabel(axioms []*ent.OntAxiom, propertyID uint32, byID map[uint32]*ent.OntEntity) string {
	row := firstDataPropertyRangeRow(axioms, propertyID)
	if row == nil {
		return IRIXMLSchemaString
	}
	if v := nilString(row.ValueText); v != "" {
		return v
	}
	if datatypeID := nilUint(row.ObjectID); datatypeID != 0 {
		if e, ok := byID[datatypeID]; ok && e.Iri != nil {
			return *e.Iri
		}
		return "#" + strconvFormatUint(datatypeID)
	}
	return IRIXMLSchemaString
}

// firstDataPropertyRangeRow 属性首条未删 42 行（sort_order → id 最小；03 号文档 §2.6 规则）。
func firstDataPropertyRangeRow(axioms []*ent.OntAxiom, propertyID uint32) *ent.OntAxiom {
	var best *ent.OntAxiom
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != AxiomDataPropertyRange {
			continue
		}
		if ax.SubjectID == nil || *ax.SubjectID != propertyID {
			continue
		}
		if best == nil || axiomLess(ax, best) {
			best = ax
		}
	}
	return best
}

// datatypeDefinitionOf 反查 datatype 实体的 46 定义行（首行；多行脏数据取首行——源修订 #12）。
// 无定义行或 JSON 损坏返回 defined=false（62 词法放行；损坏由 46 分支另行报错，不重复计数）。
func datatypeDefinitionOf(axioms []*ent.OntAxiom, datatypeID uint32) (DatatypeDefinition, bool) {
	var best *ent.OntAxiom
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != AxiomDatatypeDefinition {
			continue
		}
		if ax.SubjectID == nil || *ax.SubjectID != datatypeID {
			continue
		}
		if best == nil || axiomLess(ax, best) {
			best = ax
		}
	}
	if best == nil {
		return DatatypeDefinition{}, false
	}
	def, err := ParseDefinition(nilString(best.ValueText))
	if err != nil {
		return DatatypeDefinition{}, false
	}
	return def, true
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
