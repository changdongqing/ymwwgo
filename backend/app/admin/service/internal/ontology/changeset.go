package ontology

import (
	"strings"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// AxiomOp 公理变更操作
type AxiomOp string

const (
	AxiomOpAdd    AxiomOp = "ADD"
	AxiomOpDelete AxiomOp = "DELETE"
)

// AxiomChange 单条公理级增删变更；由各功能服务（01 类 / 02 属性 ...）组装，ChangeApplier 统一执行。
type AxiomChange struct {
	Op           AxiomOp
	AxiomID      uint32 // DELETE 时必填；ADD 时零值
	AxiomType    uint32 // ADD 时必填（OntAxiomType 常量）
	SubjectID    uint32
	ObjectID     uint32 // 可零值
	PredicateIRI string
	ValueText    string
	Lang         string
	SortOrder    int32

	// 引用本次新建实体：>0 表示该端取 EntityChangeSet.NewEntities[序号-1] 落库后的真实 id，
	// SubjectID/ObjectID 届时忽略（create 路径新实体尚无自增 id 的桥接）；
	// 零值 = 不引用（直接用 SubjectID/ObjectID）——约定零值安全，构造方漏置哨兵不会错绑。
	SubjectNewEntityRef int
	ObjectNewEntityRef  int
}

// EntityChangeSet 一次保存 = 一组公理级增删（对应 OWLModelManager.applyChanges 的本体级变更批）。
type EntityChangeSet struct {
	OntologyID  uint32
	BaseVersion *uint32 // nil = 跳过版本比对（仅限内部初始化路径，与源修订 #7 同语义）
	Operation   string  // 落 ont_change_logs.operation：CREATE_CLASS / UPDATE_CLASS / ...

	// 本次新建实体草稿（create 路径）；Changes 中以 SubjectNewEntityRef/ObjectNewEntityRef 下标引用
	NewEntities []EntityDraft

	Changes []AxiomChange

	// 级联软删的实体（force 删除路径）：实体行与其上全部公理一并消亡
	DeleteEntityIDs []uint32
}

// ApplyResult 公理事务执行结果
type ApplyResult struct {
	NewEntityIDs []uint32 // 本次新建实体 id（返回前端选中新节点，对应 set setSelectedOWLObject）
	NewVersion   uint32   // 变更后本体版本号
}

// ChangeLogEntry 变更日志行（ont_change_logs）
type ChangeLogEntry struct {
	Operation     string
	ChangeDetail  string // JSON 快照 {"added":[公理字段...],"deleted":[公理id...]}
	BeforeVersion uint32
	AfterVersion  uint32
}

// ============================================================================
// diff 算法（frame 全量提交语义，设计文档 §4.4.2）
// ============================================================================

// DiffSubjectSideRelations 单边关系 diff（父类 20 型 / 父属性 30 型 / 定义域 31 / 值域 32）：
// 当前公理集合（以 subject 为主语）vs 目标实体集合，删旧增新；保留条目若位置变化（重排）
// 按"删旧行 + 增新行"重写 sort_order，保证最终排序与提交顺序一致。targetIDs 允许为空
// （清空全部 = 变根/清空域值域）。axiomType 指定公理类型（01 期实现硬编码 20 型，02 期泛化）。
func DiffSubjectSideRelations(subject uint32, axiomType uint32, current []*ent.OntAxiom, targetIDs []uint32) []AxiomChange {
	byObject := make(map[uint32][]*ent.OntAxiom, len(current))
	for _, ax := range current {
		if ax.ObjectID == nil {
			continue
		}
		byObject[*ax.ObjectID] = append(byObject[*ax.ObjectID], ax)
	}

	changes := make([]AxiomChange, 0, len(current)+len(targetIDs))

	// 删：当前不在目标中的；以及重排条目的旧行（用删+增重写 sort_order）
	targetSet := make(map[uint32]int, len(targetIDs))
	for i, id := range targetIDs {
		targetSet[id] = i
	}
	for objectID, rows := range byObject {
		pos, keep := targetSet[objectID]
		if !keep || sortOrderOf(rows[0]) != int32(pos) {
			for _, row := range rows {
				changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: row.ID, SubjectID: subject})
			}
		} else if len(rows) > 1 {
			// 脏数据防御：同目标多行时保留首行，多余行删除
			for _, row := range rows[1:] {
				changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: row.ID, SubjectID: subject})
			}
		}
	}

	// 增：目标中不存在（或因重排被删）的，按提交顺序写入 sort_order
	for i, id := range targetIDs {
		rows, ok := byObject[id]
		needAdd := !ok || sortOrderOf(rows[0]) != int32(i)
		if needAdd {
			changes = append(changes, AxiomChange{
				Op:        AxiomOpAdd,
				AxiomType: axiomType,
				SubjectID: subject,
				ObjectID:  id,
				SortOrder: int32(i),
			})
		}
	}
	return changes
}

func sortOrderOf(ax *ent.OntAxiom) int32 {
	if ax.SortOrder == nil {
		return 0
	}
	return int32(*ax.SortOrder)
}

// DiffSubjectSideValues 单边值集合 diff（42 数据属性值域：多行 value_text=内建 datatype IRI；
// 与 DiffSubjectSideRelations 的差异仅在比对键从 object_id 换成 value_text——03 号文档 §4.2，
// 05 期 facet 定义等场景可复用）。语义（源修订 #2 内化，值域列有顺序语义故收紧）：
//   - 存量行 value 命中目标 → 保留不动（含其 sort_order）；
//   - 存量行 value 不在目标 → 删；同 value 多行（脏数据）保留首行、多余行删；
//   - 目标中新增的 value → 按提交序补行，sort_order 从保留行最大值后顺排；
//   - 保留行按提交序展开的存量 sort_order 非严格递增（重排编辑）→ 整段重写
//     （删全部 + 按提交序重建），保证任意混合编辑后回读序 = 提交序。
//
// targetValues 允许为空（清空全部值域）；空串/重复项的归一化由服务层完成，本组件对
// 重复项防御性只补一行。与 DiffCharacteristics 并列（比对键 value_text vs predicate_iri），
// 避免隐式参数开关（转译差异记录 #3）。
func DiffSubjectSideValues(subject uint32, axiomType uint32, current []*ent.OntAxiom, targetValues []string) []AxiomChange {
	firstByValue := make(map[string]*ent.OntAxiom, len(current))
	for _, ax := range current {
		v := nilString(ax.ValueText)
		if _, ok := firstByValue[v]; !ok {
			firstByValue[v] = ax
		}
	}

	// 重排检测：保留行按提交序展开的存量 sort_order 必须严格递增
	prevSort := int32(-1)
	reorder := false
	for _, v := range targetValues {
		row, ok := firstByValue[v]
		if !ok {
			continue
		}
		so := sortOrderOf(row)
		if so <= prevSort {
			reorder = true
			break
		}
		prevSort = so
	}

	changes := make([]AxiomChange, 0, len(current)+len(targetValues))

	if reorder {
		for _, ax := range current {
			changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: ax.ID, SubjectID: subject})
		}
		for i, v := range targetValues {
			changes = append(changes, AxiomChange{
				Op: AxiomOpAdd, AxiomType: axiomType, SubjectID: subject,
				ValueText: v, SortOrder: int32(i),
			})
		}
		return changes
	}

	// 删：value 不在目标中的行；同 value 多行时保留首行
	inTarget := make(map[string]struct{}, len(targetValues))
	for _, v := range targetValues {
		inTarget[v] = struct{}{}
	}
	keptMax := int32(-1)
	for _, ax := range current {
		v := nilString(ax.ValueText)
		if _, keep := inTarget[v]; keep && firstByValue[v].ID == ax.ID {
			if so := sortOrderOf(ax); so > keptMax {
				keptMax = so
			}
			continue
		}
		changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: ax.ID, SubjectID: subject})
	}

	// 增：目标中无存量行的值，按提交序从保留行最大值后顺排（存量行不动）
	added := make(map[string]struct{}, len(targetValues))
	seq := keptMax + 1
	for _, v := range targetValues {
		if _, ok := firstByValue[v]; ok {
			continue
		}
		if _, dup := added[v]; dup {
			continue
		}
		added[v] = struct{}{}
		changes = append(changes, AxiomChange{
			Op: AxiomOpAdd, AxiomType: axiomType, SubjectID: subject,
			ValueText: v, SortOrder: seq,
		})
		seq++
	}
	return changes
}

// SymmetricAxiomPair 对称关系现状：伙伴 + 双向两行公理 id（成对存储）。
type SymmetricAxiomPair struct {
	OtherID         uint32
	ForwardAxiomID  uint32 // me→other
	BackwardAxiomID uint32 // other→me
}

// DiffSymmetricPairs 对称关系 diff（不相交 21 型 / 等价 22 型，成对存储）：
// 删/增均成对两行（(me→other) 与 (other→me)），usage 反查与差异计算均为 O(1) 行操作。
func DiffSymmetricPairs(me uint32, axiomType uint32, current []SymmetricAxiomPair, targetIDs []uint32) []AxiomChange {
	currentByOther := make(map[uint32]SymmetricAxiomPair, len(current))
	for _, p := range current {
		currentByOther[p.OtherID] = p
	}

	targetSet := make(map[uint32]struct{}, len(targetIDs))
	for _, id := range targetIDs {
		targetSet[id] = struct{}{}
	}

	changes := make([]AxiomChange, 0, (len(current)+len(targetIDs))*2)

	// 删：成对两行（各自带主语，供管线收集受影响实体做缓存同步）
	for _, p := range current {
		if _, keep := targetSet[p.OtherID]; !keep {
			changes = append(changes,
				AxiomChange{Op: AxiomOpDelete, AxiomID: p.ForwardAxiomID, SubjectID: me},
				AxiomChange{Op: AxiomOpDelete, AxiomID: p.BackwardAxiomID, SubjectID: p.OtherID},
			)
		}
	}

	// 增：成对两行（sort_order 取提交顺序，双向同行号）
	for i, id := range targetIDs {
		if _, exists := currentByOther[id]; exists {
			continue
		}
		changes = append(changes,
			AxiomChange{Op: AxiomOpAdd, AxiomType: axiomType, SubjectID: me, ObjectID: id, SortOrder: int32(i)},
			AxiomChange{Op: AxiomOpAdd, AxiomType: axiomType, SubjectID: id, ObjectID: me, SortOrder: int32(i)},
		)
	}
	return changes
}

// DiffAnnotations 内建注解 replace diff（label / comment / notation 三谓词逐个）：
// 目标键存在才参与 diff（键不存在 = 本字段不变）；值相等（与该谓词现存首选行比较）跳过；
// 否则删该谓词全部现存行 + 加一行新值（空串 = 清空，即删后不增）。
//
// 与设计文档"提交 null = 清空"的适配说明：protojson 把 JSON null 归并为字段缺省
// （实测 protobuf-go 对 optional 字段的 null 不置 presence，"缺省"与"null"不可区分），
// 故本仓定案为——字段缺省 = 不变、提交空串 = 清空、提交值 = 全量替换；
// 前端 hooks 层按此封装（源修订 #8 全量替换语义保持不变）。
func DiffAnnotations(subject uint32, current []*ent.OntAxiom, targets map[string]*string) []AxiomChange {
	changes := make([]AxiomChange, 0, len(targets))

	for predicate, target := range targets {
		if target == nil {
			continue
		}

		// 该谓词的现存行（含多语言）
		rows := make([]*ent.OntAxiom, 0, 2)
		for _, ax := range current {
			if ax.PredicateIri != nil && *ax.PredicateIri == predicate {
				rows = append(rows, ax)
			}
		}

		// 值相等且恰一行 → 跳过（全量提交的高频路径）
		if len(rows) == 1 && rows[0].ValueText != nil && *rows[0].ValueText == *target {
			continue
		}

		for _, row := range rows {
			changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: row.ID, SubjectID: subject})
		}
		if *target != "" {
			changes = append(changes, AxiomChange{
				Op:           AxiomOpAdd,
				AxiomType:    AxiomAnnotationAssertion,
				SubjectID:    subject,
				PredicateIRI: predicate,
				ValueText:    *target,
				Lang:         builtinPredicateLang(predicate),
			})
		}
	}
	return changes
}

// DiffDeprecated owl:deprecated 断言存在性 diff（源修订 #8：deprecated=false = 移除断言）：
// target=true 补断言（值 "true"，无 lang）；target=false 删全部 owl:deprecated 断言行。
func DiffDeprecated(subject uint32, current []*ent.OntAxiom, target bool) []AxiomChange {
	rows := make([]*ent.OntAxiom, 0, 1)
	for _, ax := range current {
		if ax.PredicateIri != nil && *ax.PredicateIri == PredicateOWLDeprecated {
			rows = append(rows, ax)
		}
	}

	exists := false
	for _, row := range rows {
		if row.ValueText != nil && *row.ValueText == "true" {
			exists = true
			break
		}
	}
	if exists == target {
		return nil
	}

	changes := make([]AxiomChange, 0, len(rows)+1)
	for _, row := range rows {
		changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: row.ID, SubjectID: subject})
	}
	if target {
		changes = append(changes, AxiomChange{
			Op:           AxiomOpAdd,
			AxiomType:    AxiomAnnotationAssertion,
			SubjectID:    subject,
			PredicateIRI: PredicateOWLDeprecated,
			ValueText:    "true",
		})
	}
	return changes
}

// builtinPredicateLang 内建谓词的写入语言标签：label/comment 带 zh（多语言预留），
// deprecated/notation 无 lang（与源存储约定一致）。
func builtinPredicateLang(predicate string) string {
	switch predicate {
	case PredicateRDFSLabel, PredicateRDFSComment:
		return "zh"
	default:
		return ""
	}
}

// PropertyChain 属性链投影（37 型公理行）与链 diff 见 object_property.go（02 期增量）。

// DiffChains 属性链 diff（37 型，按链内容比对，frame 全量替换语义——02 号文档 §2.4）：
//   - 提交含 id 且成员序列未变 → 不动；
//   - 提交含 id 且变更 → 删旧行 + 增新行；
//   - 提交无 id → 增；
//   - 现存行未被任何提交引用（缺 id）→ 删。
//
// 目标侧 memberIDs 为空且无 id 的行不产出变更（前端空行防御）。成员序列保序比对，不做 distinct。
func DiffChains(me uint32, current []PropertyChain, targets []PropertyChain) []AxiomChange {
	currentByID := make(map[uint32]PropertyChain, len(current))
	for _, c := range current {
		if c.AxiomID != 0 {
			currentByID[c.AxiomID] = c
		}
	}

	changes := make([]AxiomChange, 0, len(current)+len(targets))

	// 删：现存行未被提交引用
	kept := make(map[uint32]struct{}, len(targets))
	for _, t := range targets {
		if t.AxiomID != 0 {
			kept[t.AxiomID] = struct{}{}
		}
	}
	for _, c := range current {
		if c.AxiomID == 0 {
			continue
		}
		if _, ok := kept[c.AxiomID]; !ok {
			changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: c.AxiomID, SubjectID: me})
		}
	}

	// 增/换行：按提交顺序写 sort_order（同一属性多条链之间用既有 sort_order 排序）
	for i, t := range targets {
		if len(t.MemberIDs) == 0 {
			continue
		}
		if t.AxiomID != 0 {
			cur, ok := currentByID[t.AxiomID]
			if ok && chainMembersEqual(cur.MemberIDs, t.MemberIDs) {
				continue // 未变
			}
			if ok {
				// 变更：删旧行 + 增新行
				changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: t.AxiomID, SubjectID: me})
			}
			// id 不在现状（防御：提交了未知行 id）按新增处理
		}
		changes = append(changes, AxiomChange{
			Op:        AxiomOpAdd,
			AxiomType: AxiomSubPropertyChainOf,
			SubjectID: me,
			ValueText: EncodeChainMemberIDs(t.MemberIDs),
			SortOrder: int32(i),
		})
	}
	return changes
}

func chainMembersEqual(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DiffCharacteristics 特征 diff（33 对象属性 / 43 数据属性，按 predicate_iri 集合比对——
// 变体：以谓词列而非 object_id 为集合键，02 号文档 §2.4；axiomType 参数化以供两期共用）：
// 现存谓词不在目标中 → 删行；目标谓词无现存行 → 增行。
// 每特征一行（subject=属性，object_id/value_text 空）。
func DiffCharacteristics(subject uint32, axiomType uint32, current []*ent.OntAxiom, targetIRIs []string) []AxiomChange {
	targetSet := make(map[string]struct{}, len(targetIRIs))
	for _, iri := range targetIRIs {
		if iri == "" {
			continue
		}
		targetSet[iri] = struct{}{}
	}

	changes := make([]AxiomChange, 0, len(current)+len(targetIRIs))
	existing := make(map[string]struct{}, len(current))
	for _, ax := range current {
		p := nilString(ax.PredicateIri)
		if p == "" {
			continue
		}
		if _, keep := targetSet[p]; keep {
			existing[p] = struct{}{}
			continue
		}
		changes = append(changes, AxiomChange{Op: AxiomOpDelete, AxiomID: ax.ID, SubjectID: subject})
	}

	for _, iri := range targetIRIs {
		if iri == "" {
			continue
		}
		if _, ok := existing[iri]; ok {
			continue
		}
		changes = append(changes, AxiomChange{
			Op:           AxiomOpAdd,
			AxiomType:    axiomType,
			SubjectID:    subject,
			PredicateIRI: iri,
		})
	}
	return changes
}

// ============================================================================
// 自定义注解断言 diff（04 期增量，六类实体 frame 共用——04 号文档 §4.3）
// ============================================================================

// CustomAnnotationInput 自定义注解断言提交载荷（100 型 object_id 路径；四面 Create/Update 共用）。
type CustomAnnotationInput struct {
	PropertyID uint32 // 谓词 = 注解属性实体 id
	Value      string // 字面量词形（plain literal 语义；空白非法）
	Lang       string // 可选语言标签（空白归一为空串）
}

// NormalizeCustomAnnotationInputs 提交载荷归一化（服务层在 diff 前统一执行，04 号文档 §4.3）：
//   - value 去首尾空白后为空 → ANNOTATION_VALUE_INVALID；
//   - lang 去首尾空白（空白归一为空串，非法 BCP-47 值 V1 不校验——04 号文档 §7 #16）；
//   - 按 (property_id, value, lang) 三元组有序去重（防前端重复行，重复只落一行）。
//
// propertyID 为 0 的行直接丢弃（前端空行防御）。
func NormalizeCustomAnnotationInputs(inputs []CustomAnnotationInput) ([]CustomAnnotationInput, error) {
	out := make([]CustomAnnotationInput, 0, len(inputs))
	seen := make(map[CustomAnnotationInput]struct{}, len(inputs))
	for _, in := range inputs {
		if in.PropertyID == 0 {
			continue
		}
		value := strings.TrimSpace(in.Value)
		if value == "" {
			return nil, NewError(ReasonAnnotationValueInvalid, "注解值不合法：属性 %d 的注解值为空白", in.PropertyID)
		}
		norm := CustomAnnotationInput{PropertyID: in.PropertyID, Value: value, Lang: strings.TrimSpace(in.Lang)}
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	return out, nil
}

// customAnnotationKey diff 键：(object_id, value_text, lang) 三元组——同属性多值天然支持。
type customAnnotationKey struct {
	propertyID uint32
	value      string
	lang       string
}

func customAnnotationKeyOf(objectID uint32, value, lang string) customAnnotationKey {
	return customAnnotationKey{propertyID: objectID, value: value, lang: lang}
}

// AppendCustomAnnotationAxioms create 侧：新实体的自定义注解断言 ADD 变更（sort_order 从 0 顺排，
// 回读序 = 提交序）。subjectRef > 0 时主语引用 NewEntities[subjectRef-1]（create 路径新实体
// 尚无自增 id 的桥接），否则用 subjectID（内部初始化路径）。inputs 须先过 NormalizeCustomAnnotationInputs。
func AppendCustomAnnotationAxioms(set *EntityChangeSet, subjectRef int, subjectID uint32, inputs []CustomAnnotationInput) {
	for i, in := range inputs {
		set.Changes = append(set.Changes, AxiomChange{
			Op:                  AxiomOpAdd,
			AxiomType:           AxiomAnnotationAssertion,
			SubjectNewEntityRef: subjectRef,
			SubjectID:           subjectID,
			ObjectID:            in.PropertyID,
			ValueText:           in.Value,
			Lang:                in.Lang,
			SortOrder:           int32(i),
		})
	}
}

// DiffCustomAnnotations update 侧：自定义注解断言 replace diff（100 型 object_id 轨，
// 六类实体 frame 共用——类/对象属性/数据属性/注解属性四面 + 06 期个体零成本复用）。
//
//   - 现状集 = current 中 object_id 非空的 100 行（内建谓词行 predicate_iri 路径不参与本 diff）；
//   - diff 键 = (object_id, value_text, lang) 三元组：键在目标中 → 保留不动，否则删；
//   - 目标中无存量行的键 → 增（sort_order 从存量自定义行最大值后顺排，回读序 = 提交序）；
//   - 全量替换语义（04 号文档 §12 #1）：inputs 为空 = 清空全部自定义注解——与 01 期
//     内建注解 frame 全量提交语义对齐；"未提及则不动"的未来升级路径是 optional wrapper message。
//
// inputs 须先过 NormalizeCustomAnnotationInputs（值空白/去重已由服务层前置）。
func DiffCustomAnnotations(subject uint32, current []*ent.OntAxiom, inputs []CustomAnnotationInput) []AxiomChange {
	currentByKey := make(map[customAnnotationKey][]*ent.OntAxiom, len(current))
	maxSort := int32(-1)
	for _, ax := range current {
		if ax.ObjectID == nil || *ax.ObjectID == 0 {
			continue
		}
		key := customAnnotationKeyOf(*ax.ObjectID, nilString(ax.ValueText), nilString(ax.Lang))
		currentByKey[key] = append(currentByKey[key], ax)
		if so := sortOrderOf(ax); so > maxSort {
			maxSort = so
		}
	}

	changes := make([]AxiomChange, 0, len(current)+len(inputs))

	// 删：存量自定义行中键不在目标集合的（同键多行脏数据全部删除，增侧会补一行规范行）
	targetKeys := make(map[customAnnotationKey]struct{}, len(inputs))
	for _, in := range inputs {
		targetKeys[customAnnotationKeyOf(in.PropertyID, in.Value, in.Lang)] = struct{}{}
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
		key := customAnnotationKeyOf(in.PropertyID, in.Value, in.Lang)
		if _, ok := currentByKey[key]; ok {
			continue
		}
		changes = append(changes, AxiomChange{
			Op:        AxiomOpAdd,
			AxiomType: AxiomAnnotationAssertion,
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
