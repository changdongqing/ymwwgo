package ontology

import (
	"context"
	"encoding/json"
	"fmt"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// AxiomTx 公理事务存储端接口：管线七步所需的全部数据访问。
// postgres 实现在 data 层（ontology_axiom_repo.go 等），单测以桩替换。
type AxiomTx interface {
	// LockOntology 锁定本体行（SELECT ... FOR UPDATE）；未找到返回 ent NotFound
	LockOntology(ctx context.Context, ontologyID uint32) (*ent.OntOntology, error)
	// InsertEntity 落库新实体草稿，返回自增 id
	InsertEntity(ctx context.Context, ontologyID uint32, draft *EntityDraft, createdBy uint32) (uint32, error)
	// InsertAxiom 插入公理行，返回自增 id
	InsertAxiom(ctx context.Context, ontologyID uint32, c AxiomChange, createdBy uint32) (uint32, error)
	// SoftDeleteAxiom 软删公理行（置 deleted_at / deleted_by）
	SoftDeleteAxiom(ctx context.Context, axiomID uint32, deletedBy uint32) error
	// SoftDeleteEntity 软删实体行（置 deleted_at / deleted_by）
	SoftDeleteEntity(ctx context.Context, entityID uint32, deletedBy uint32) error
	// ListAnnotationAxiomsBySubjects 取一组主语的未删注解断言公理（渲染缓存重算）
	ListAnnotationAxiomsBySubjects(ctx context.Context, ontologyID uint32, subjectIDs []uint32) ([]*ent.OntAxiom, error)
	// ListEntities 本体下全部未删实体（校验器输入 / 渲染缓存存活判定）
	ListEntities(ctx context.Context, ontologyID uint32) ([]*ent.OntEntity, error)
	// ListAxioms 本体下全部未删公理（校验器输入）
	ListAxioms(ctx context.Context, ontologyID uint32) ([]*ent.OntAxiom, error)
	// UpdateRenderCache 更新实体渲染缓存列（display_label 可为 nil = 置 NULL）
	UpdateRenderCache(ctx context.Context, entityID uint32, displayLabel *string, deprecated bool) error
	// BumpVersion 写入自增后的本体版本号
	BumpVersion(ctx context.Context, ontologyID uint32, nextVersion uint32) error
	// InsertChangeLog 写变更日志行
	InsertChangeLog(ctx context.Context, entry *ChangeLogEntry, ontologyID uint32, createdBy uint32) error
}

// AxiomStore 事务工厂：WithTx 内任何一步出错即回滚。
type AxiomStore interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, tx AxiomTx) error) error
}

// ChangeApplier 公理变更事务管线（对应 OWLModelManager.applyChanges / 源 AxiomChangeService.apply）。
//
// 管线七步（设计文档 §3.4）：锁本体 → 乐观锁比对 → 逐条应用变更 → 注解缓存同步 →
// 结构校验 → 版本自增 → 写变更日志；任何一步出错整体回滚。
type ChangeApplier struct {
	store AxiomStore
	log   Logger
}

func NewChangeApplier(store AxiomStore, log Logger) *ChangeApplier {
	return &ChangeApplier{store: store, log: log}
}

// Apply 执行一次变更集；operatorID 取 auth.FromContext（系统上下文取 0）。
func (a *ChangeApplier) Apply(ctx context.Context, set *EntityChangeSet, operatorID uint32) (*ApplyResult, error) {
	if set == nil || set.OntologyID == 0 {
		return nil, NewError(ReasonOntologyNotFound, "本体不存在")
	}

	var result *ApplyResult
	err := a.store.WithTx(ctx, func(ctx context.Context, tx AxiomTx) error {
		r, err := a.applyInTx(ctx, tx, set, operatorID)
		if err != nil {
			return err
		}
		result = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (a *ChangeApplier) applyInTx(ctx context.Context, tx AxiomTx, set *EntityChangeSet, operatorID uint32) (*ApplyResult, error) {
	// 1. 本体行 SELECT ... FOR UPDATE（本仓首个乐观锁先例，设计文档 §2.4）
	ont, err := tx.LockOntology(ctx, set.OntologyID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, NewError(ReasonOntologyNotFound, "本体不存在")
		}
		return nil, err
	}
	if ont.BaseIri == nil || ont.Version == nil {
		return nil, NewError(ReasonOntologyNotFound, "本体数据不完整：%d", set.OntologyID)
	}
	beforeVersion := *ont.Version

	// 2. 乐观锁比对（FOR UPDATE 行锁下比对再自增，把源实现的应用层竞态窗口关死）
	if set.BaseVersion != nil && *set.BaseVersion != beforeVersion {
		return nil, NewError(ReasonOntologyVersionConflict,
			"本体已被他人修改（当前版本 %d），请刷新后重试", beforeVersion)
	}

	// 3a. 新实体落库，登记草稿下标 → 自增 id 映射
	draftIDs := make([]uint32, len(set.NewEntities))
	newEntityIDs := make([]uint32, 0, len(set.NewEntities))
	for i := range set.NewEntities {
		id, err := tx.InsertEntity(ctx, set.OntologyID, &set.NewEntities[i], operatorID)
		if err != nil {
			return nil, err
		}
		draftIDs[i] = id
		newEntityIDs = append(newEntityIDs, id)
	}
	resolveRef := func(ref int, fallback uint32) uint32 {
		if ref > 0 && ref <= len(draftIDs) {
			return draftIDs[ref-1]
		}
		return fallback
	}

	// 3b. 逐条应用公理变更：ADD 插入 / DELETE 软删；收集变更主语（缓存同步）与日志快照
	addedSnapshots := make([]axiomSnapshot, 0, len(set.Changes))
	deletedIDs := make([]uint32, 0, len(set.Changes))
	subjects := make(map[uint32]struct{}, len(set.Changes))
	for _, c := range set.Changes {
		switch c.Op {
		case AxiomOpAdd:
			change := c
			change.SubjectID = resolveRef(c.SubjectNewEntityRef, c.SubjectID)
			change.ObjectID = resolveRef(c.ObjectNewEntityRef, c.ObjectID)
			if change.SubjectID == 0 {
				return nil, NewError(ReasonAxiomInvalid, "公理缺少主语（类型 %d）", c.AxiomType)
			}
			if _, err := tx.InsertAxiom(ctx, set.OntologyID, change, operatorID); err != nil {
				return nil, err
			}
			addedSnapshots = append(addedSnapshots, axiomSnapshotOf(change))
			subjects[change.SubjectID] = struct{}{}
		case AxiomOpDelete:
			if c.AxiomID == 0 {
				return nil, NewError(ReasonAxiomInvalid, "公理删除缺少公理 id")
			}
			if err := tx.SoftDeleteAxiom(ctx, c.AxiomID, operatorID); err != nil {
				return nil, err
			}
			deletedIDs = append(deletedIDs, c.AxiomID)
			if c.SubjectID != 0 {
				subjects[c.SubjectID] = struct{}{}
			}
		default:
			return nil, NewError(ReasonAxiomInvalid, "未知公理操作：%s", c.Op)
		}
	}

	// 3c. force 删除路径：实体行随公理一并软删（引用公理已在上面的 Changes 中删除）
	for _, id := range set.DeleteEntityIDs {
		if err := tx.SoftDeleteEntity(ctx, id, operatorID); err != nil {
			return nil, err
		}
		delete(subjects, id) // 实体已消亡，无需缓存同步
	}

	// 4. 注解缓存同步（对应源 syncAnnotationCache）：受影响主语的 display_label / deprecated 重算。
	//    仅服务层写路径会改公理，缓存与公理漂移时以下一步读到的公理为准（设计文档 §4.7 #10）。
	if len(subjects) > 0 {
		if err := a.syncRenderCache(ctx, tx, set.OntologyID, subjects); err != nil {
			return nil, err
		}
	}

	// 5. 结构校验（事务内读全量未删实体 + 公理；失败整体回滚）
	entities, err := tx.ListEntities(ctx, set.OntologyID)
	if err != nil {
		return nil, err
	}
	axioms, err := tx.ListAxioms(ctx, set.OntologyID)
	if err != nil {
		return nil, err
	}
	if problems := ValidateAxioms(ctx, *ont.BaseIri, entities, axioms, a.log); len(problems) > 0 {
		msgs := make([]string, 0, len(problems))
		for _, p := range problems {
			msgs = append(msgs, p.Error())
		}
		return nil, NewError(ReasonAxiomInvalid, "公理校验失败：%s", joinProblems(msgs))
	}

	// 6. 版本自增
	afterVersion := beforeVersion + 1
	if err := tx.BumpVersion(ctx, set.OntologyID, afterVersion); err != nil {
		return nil, err
	}

	// 7. 变更日志（operation + JSON 快照 + before/after_version）
	detail, err := json.Marshal(map[string]any{
		"added":   addedSnapshots,
		"deleted": deletedIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("serialize change detail: %w", err)
	}
	if err := tx.InsertChangeLog(ctx, &ChangeLogEntry{
		Operation:     set.Operation,
		ChangeDetail:  string(detail),
		BeforeVersion: beforeVersion,
		AfterVersion:  afterVersion,
	}, set.OntologyID, operatorID); err != nil {
		return nil, err
	}

	return &ApplyResult{NewEntityIDs: newEntityIDs, NewVersion: afterVersion}, nil
}

// syncRenderCache 重算受影响主语的渲染缓存列（设计文档 §3.3 同步规则）：
// display_label 取 rdfs:label 候选中按 lang 优先级 zh > 无 lang > 其它、再按 sort_order、id
// 稳定排序的首条，全空则显式置 NULL；deprecated 取 owl:deprecated 值为 "true" 的断言存在性。
func (a *ChangeApplier) syncRenderCache(ctx context.Context, tx AxiomTx, ontologyID uint32, subjects map[uint32]struct{}) error {
	ids := make([]uint32, 0, len(subjects))
	for id := range subjects {
		ids = append(ids, id)
	}

	// 已软删实体（本次 force 删除等）无需同步
	alive := make(map[uint32]struct{}, len(ids))
	entities, err := tx.ListEntities(ctx, ontologyID)
	if err != nil {
		return err
	}
	for _, e := range entities {
		alive[e.ID] = struct{}{}
	}

	targets := make([]uint32, 0, len(ids))
	for _, id := range ids {
		if _, ok := alive[id]; ok {
			targets = append(targets, id)
		}
	}
	if len(targets) == 0 {
		return nil
	}

	axioms, err := tx.ListAnnotationAxiomsBySubjects(ctx, ontologyID, targets)
	if err != nil {
		return err
	}
	bySubject := make(map[uint32][]*ent.OntAxiom, len(targets))
	for _, ax := range axioms {
		if ax.SubjectID == nil {
			continue
		}
		bySubject[*ax.SubjectID] = append(bySubject[*ax.SubjectID], ax)
	}

	for _, id := range targets {
		label := PickDisplayLabel(bySubject[id])
		deprecated := HasDeprecatedAssertion(bySubject[id])
		if err := tx.UpdateRenderCache(ctx, id, label, deprecated); err != nil {
			return err
		}
	}
	return nil
}

// PickDisplayLabel rdfs:label 首选值（lang 优先级 zh > 无 lang > 其它，再按 sort_order、id）。
func PickDisplayLabel(annotationAxioms []*ent.OntAxiom) *string {
	return PickAnnotationPreferred(annotationAxioms, PredicateRDFSLabel)
}

// PickAnnotationPreferred 指定谓词的注解首选值（渲染缓存/列表聚合共用同一条回退规则）。
func PickAnnotationPreferred(annotationAxioms []*ent.OntAxiom, predicate string) *string {
	var best *ent.OntAxiom
	for _, ax := range annotationAxioms {
		if ax.PredicateIri == nil || *ax.PredicateIri != predicate || ax.ValueText == nil {
			continue
		}
		if best == nil || labelLangRank(ax) < labelLangRank(best) || axiomLess(ax, best) {
			best = ax
		}
	}
	if best == nil {
		return nil
	}
	v := *best.ValueText
	return &v
}

// HasDeprecatedAssertion owl:deprecated 值为 "true" 的断言是否存在。
func HasDeprecatedAssertion(annotationAxioms []*ent.OntAxiom) bool {
	for _, ax := range annotationAxioms {
		if ax.PredicateIri == nil || ax.ValueText == nil {
			continue
		}
		if *ax.PredicateIri == PredicateOWLDeprecated && *ax.ValueText == "true" {
			return true
		}
	}
	return false
}

// labelLangRank lang 优先级：zh=0、无 lang=1、其它=2。
func labelLangRank(ax *ent.OntAxiom) int {
	lang := ""
	if ax.Lang != nil {
		lang = *ax.Lang
	}
	switch lang {
	case "zh":
		return 0
	case "":
		return 1
	default:
		return 2
	}
}

// axiomLess sort_order、id 稳定排序（同为首选 lang 时）。
func axiomLess(a, b *ent.OntAxiom) bool {
	if a.ID == b.ID {
		return false
	}
	soA, soB := int32(0), int32(0)
	if a.SortOrder != nil {
		soA = int32(*a.SortOrder)
	}
	if b.SortOrder != nil {
		soB = int32(*b.SortOrder)
	}
	if soA != soB {
		return soA < soB
	}
	return a.ID < b.ID
}

// axiomSnapshot 变更日志的公理字段快照（{"added":[...]} 的元素）。
type axiomSnapshot struct {
	AxiomType    uint32 `json:"axiom_type"`
	SubjectID    uint32 `json:"subject_id"`
	ObjectID     uint32 `json:"object_id,omitempty"`
	PredicateIRI string `json:"predicate_iri,omitempty"`
	ValueText    string `json:"value_text,omitempty"`
	Lang         string `json:"lang,omitempty"`
	SortOrder    int32  `json:"sort_order"`
}

func axiomSnapshotOf(c AxiomChange) axiomSnapshot {
	return axiomSnapshot{
		AxiomType:    c.AxiomType,
		SubjectID:    c.SubjectID,
		ObjectID:     c.ObjectID,
		PredicateIRI: c.PredicateIRI,
		ValueText:    c.ValueText,
		Lang:         c.Lang,
		SortOrder:    c.SortOrder,
	}
}

// joinProblems 校验失败明细拼接（截断防超长）。
func joinProblems(msgs []string) string {
	const max = 8
	if len(msgs) > max {
		return fmt.Sprintf("%s 等 %d 处", msgs[:max], len(msgs))
	}
	out := ""
	for i, m := range msgs {
		if i > 0 {
			out += "；"
		}
		out += m
	}
	return out
}
