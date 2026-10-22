package ontology

import (
	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 06 期个体断言 diff（06 号文档 §4.3：类型 / 编码 / 属性断言三区，frame 全量提交语义）。
// 与既有 diff 同族：本文件只做纯函数 diff，编码 62 行的隔离（不参与断言区比对）由服务层
// 在传入 current 时完成过滤。

// ObjectAssertionInput 61 对象属性断言提交载荷（全量替换；与 proto 同名消息逐字对应）。
type ObjectAssertionInput struct {
	PredicateIRI string // 对象属性 IRI（须为本本体已声明对象属性）
	TargetID     uint32 // 宾语个体
}

// DataAssertionInput 62 数据属性断言提交载荷（全量替换；编码行由服务层单独承载，不进本区）。
type DataAssertionInput struct {
	PropertyID uint32 // 数据属性实体 id（谓词）
	Value      string // 字面量词形
	Lang       string // 可选语言标签（空串 = 无语言）
}

// DiffTypeAssertions 类型断言专用全量替换 diff（60 型，06 号文档 §2.4）：
// 类型顺序承载主类目语义（sort_order=0 即主类目），必须用全量替换——存量行按 (sort_order,id)
// 升序展开与目标序列逐一比对，序列完全一致（id 与位置都相同）才零操作；否则删全部旧 60 行 +
// 按新序增全部（sort_order 重建）。不能复用 DiffSubjectSideRelations：它只删缺增新、不重排
// 既有行，主类目变更（次序类提升为主）会失真。targetIDs 允许为空（调用方保证个体 ≥1 类型，
// 清空场景由服务层前置拒绝，本函数不做业务拦截）。
func DiffTypeAssertions(subject uint32, current []*ent.OntAxiom, typeIDs []uint32) []AxiomChange {
	ordered := make([]*ent.OntAxiom, 0, len(current))
	for _, ax := range current {
		ordered = append(ordered, ax)
	}
	// current 已由 ListBySubject 按 (sort_order,id) 升序返回；防御性再排一次，避免调用方换数据源后失真
	stableSortAxioms(ordered)

	unchanged := len(ordered) == len(typeIDs)
	if unchanged {
		for i, ax := range ordered {
			if ax.ObjectID == nil || *ax.ObjectID != typeIDs[i] {
				unchanged = false
				break
			}
		}
	}
	if unchanged {
		return nil
	}

	changes := make([]AxiomChange, 0, len(ordered)+len(typeIDs))
	for _, ax := range ordered {
		changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: ax.ID, SubjectID: subject})
	}
	for i, id := range typeIDs {
		changes = append(changes, AxiomChange{
			Op:        AxiomOpAdd,
			AxiomType: AxiomTypeAssertion,
			SubjectID: subject,
			ObjectID:  id,
			SortOrder: int32(i),
		})
	}
	return changes
}

// objectAssertionKey 61 diff 键：(predicate_iri, target_id)。
type objectAssertionKey struct {
	predicate string
	targetID  uint32
}

// DiffObjectAssertions 对象属性断言 replace diff（61 型，06 号文档 §4.3）：
//   - 现状集 = current 中 predicate_iri 非空的 61 行；
//   - diff 键 = (predicate_iri, object_id)：键在目标中 → 保留不动，否则删；
//   - 目标中无存量行的键 → 增（sort_order 从存量行最大值后顺排）；
//   - 全量替换语义（06 号文档 §12 #5）：inputs 为空 = 清空全部对象属性断言。
//
// 谓词声明性/目标个体性校验在服务层与校验器两道拦截，本函数不做业务校验。
func DiffObjectAssertions(subject uint32, current []*ent.OntAxiom, inputs []ObjectAssertionInput) []AxiomChange {
	currentByKey := make(map[objectAssertionKey][]*ent.OntAxiom, len(current))
	maxSort := int32(-1)
	for _, ax := range current {
		if ax.PredicateIri == nil || *ax.PredicateIri == "" || ax.ObjectID == nil {
			continue
		}
		key := objectAssertionKey{predicate: *ax.PredicateIri, targetID: *ax.ObjectID}
		currentByKey[key] = append(currentByKey[key], ax)
		if so := sortOrderOf(ax); so > maxSort {
			maxSort = so
		}
	}

	changes := make([]AxiomChange, 0, len(current)+len(inputs))

	// 删：存量行中键不在目标集合的（同键多行脏数据全部删除，增侧会补一行规范行）
	targetKeys := make(map[objectAssertionKey]struct{}, len(inputs))
	for _, in := range inputs {
		targetKeys[objectAssertionKey{predicate: in.PredicateIRI, targetID: in.TargetID}] = struct{}{}
	}
	for key, rows := range currentByKey {
		if _, keep := targetKeys[key]; keep {
			continue
		}
		for _, row := range rows {
			changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: row.ID, SubjectID: subject})
		}
	}

	// 增：目标中无存量行的键，按提交序从存量最大 sort_order 后顺排
	seq := maxSort + 1
	for _, in := range inputs {
		key := objectAssertionKey{predicate: in.PredicateIRI, targetID: in.TargetID}
		if _, ok := currentByKey[key]; ok {
			continue
		}
		changes = append(changes, AxiomChange{
			Op:           AxiomOpAdd,
			AxiomType:    AxiomObjectPropertyAssertion,
			SubjectID:    subject,
			PredicateIRI: in.PredicateIRI,
			ObjectID:     in.TargetID,
			SortOrder:    seq,
		})
		seq++
	}
	return changes
}

// dataAssertionKey 62 diff 键：(property_id, value, lang)——同属性多值天然支持。
type dataAssertionKey struct {
	propertyID uint32
	value      string
	lang       string
}

// DiffDataAssertions 数据属性断言 replace diff（62 型，06 号文档 §4.3；键形态与
// DiffCustomAnnotations 同构，比对键从注解属性换数据属性）：
//   - 现状集 = current 中 object_id 非空的 62 行（编码 62 行由服务层过滤后传入，本函数不特判）；
//   - diff 键 = (object_id, value_text, lang)：键在目标中 → 保留不动，否则删；
//   - 目标中无存量行的键 → 增（sort_order 从存量行最大值后顺排）；
//   - 全量替换语义（06 号文档 §12 #5）：inputs 为空 = 清空全部数据属性断言（编码行除外——
//     由调用方在 current 侧隔离）。
//
// 词法校验走校验器既有 62 分支（值域推断 + facet 闸门），本函数不做词法校验。
func DiffDataAssertions(subject uint32, current []*ent.OntAxiom, inputs []DataAssertionInput) []AxiomChange {
	currentByKey := make(map[dataAssertionKey][]*ent.OntAxiom, len(current))
	maxSort := int32(-1)
	for _, ax := range current {
		if ax.ObjectID == nil || *ax.ObjectID == 0 {
			continue
		}
		key := dataAssertionKey{propertyID: *ax.ObjectID, value: nilString(ax.ValueText), lang: nilString(ax.Lang)}
		currentByKey[key] = append(currentByKey[key], ax)
		if so := sortOrderOf(ax); so > maxSort {
			maxSort = so
		}
	}

	changes := make([]AxiomChange, 0, len(current)+len(inputs))

	targetKeys := make(map[dataAssertionKey]struct{}, len(inputs))
	for _, in := range inputs {
		targetKeys[dataAssertionKey{propertyID: in.PropertyID, value: in.Value, lang: in.Lang}] = struct{}{}
	}
	for key, rows := range currentByKey {
		if _, keep := targetKeys[key]; keep {
			continue
		}
		for _, row := range rows {
			changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: row.ID, SubjectID: subject})
		}
	}

	seq := maxSort + 1
	for _, in := range inputs {
		key := dataAssertionKey{propertyID: in.PropertyID, value: in.Value, lang: in.Lang}
		if _, ok := currentByKey[key]; ok {
			continue
		}
		changes = append(changes, AxiomChange{
			Op:        AxiomOpAdd,
			AxiomType: AxiomDataPropertyAssertion,
			SubjectID: subject,
			ObjectID:  in.PropertyID,
			ValueText: in.Value,
			Lang:      in.Lang,
			SortOrder: seq,
		})
		seq++
	}
	return changes
}

// stableSortAxioms (sort_order, id) 升序稳定排序（frame 装载与 diff 顺序语义的统一保证）。
// 导出别名供服务层 frame 装载复用（ListBySubject 已按此排序，防御性调用方仍可显式排序）。
func stableSortAxioms(rows []*ent.OntAxiom) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && axiomLess(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// StableSortAxioms stableSortAxioms 的导出形态（服务层 frame 装载 60 行保序复用）。
func StableSortAxioms(rows []*ent.OntAxiom) { stableSortAxioms(rows) }
