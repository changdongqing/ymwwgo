package ontology

import (
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

// DiffCharacteristics 特征 diff（33 型，按 predicate_iri 集合比对——变体：以谓词列而非 object_id
// 为集合键，02 号文档 §2.4）：现存谓词不在目标中 → 删行；目标谓词无现存行 → 增行。
// 每特征一行（subject=属性，object_id/value_text 空）。
func DiffCharacteristics(subject uint32, current []*ent.OntAxiom, targetIRIs []string) []AxiomChange {
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
			AxiomType:    AxiomObjectPropertyCharacteristic,
			SubjectID:    subject,
			PredicateIRI: iri,
		})
	}
	return changes
}
