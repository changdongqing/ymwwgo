package data

import (
	"context"
	"time"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	entCrud "github.com/tx7do/go-crud/entgo"
	"github.com/tx7do/go-utils/trans"

	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/data/ent/ontaxiom"
	"go-wind-admin/app/admin/service/internal/data/ent/ontentity"
	"go-wind-admin/app/admin/service/internal/data/ent/ontontology"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// OntAxiomRepo 公理仓储：读路径（usage 反查 / frame 素材）+ ontology.AxiomStore 事务实现
// （公理事务管线 ChangeApplier 的存储端；同一 ent Tx 内保证跨表一致性，表间无 FK）。
type OntAxiomRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper
}

func NewOntAxiomRepo(
	ctx *bootstrap.Context,
	entClient *entCrud.EntClient[*ent.Client],
) *OntAxiomRepo {
	return &OntAxiomRepo{
		log:       ctx.NewLoggerHelper("ontology-axiom/repo/admin-service"),
		entClient: entClient,
	}
}

// ---- 读路径（非事务） ----

// ListByOntology 本体下全部未删公理（树构建输入）
func (r *OntAxiomRepo) ListByOntology(ctx context.Context, ontologyID uint32) ([]*ent.OntAxiom, error) {
	return r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.DeletedAtIsNil()).
		Order(ent.Asc(ontaxiom.FieldID)).
		All(ctx)
}

// ListBySubject 实体主语侧全部未删公理（frame / update diff 现状）
func (r *OntAxiomRepo) ListBySubject(ctx context.Context, ontologyID uint32, subjectID uint32) ([]*ent.OntAxiom, error) {
	return r.entClient.Client().OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.SubjectIDEQ(subjectID),
			ontaxiom.DeletedAtIsNil(),
		).
		Order(ent.Asc(ontaxiom.FieldSortOrder), ent.Asc(ontaxiom.FieldID)).
		All(ctx)
}

// ListByObject 实体宾语侧全部未删公理（usage 反查 / 对称伙伴集）
func (r *OntAxiomRepo) ListByObject(ctx context.Context, ontologyID uint32, objectID uint32) ([]*ent.OntAxiom, error) {
	return r.entClient.Client().OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.ObjectIDEQ(objectID),
			ontaxiom.DeletedAtIsNil(),
		).
		Order(ent.Asc(ontaxiom.FieldID)).
		All(ctx)
}

// CountReferences 实体被引用数（object 侧、类 usage 公理类型集内、未删）——类删除保护计数
func (r *OntAxiomRepo) CountReferences(ctx context.Context, ontologyID uint32, entityID uint32) (int, error) {
	return r.CountReferencesByTypes(ctx, ontologyID, entityID, ontology.ClassUsageAxiomTypes)
}

// CountReferencesByTypes 实体被引用数（object 侧、指定公理类型集内、未删）——删除保护计数
// （类 20/21/22/31/32；对象属性 30/34/35/36；37 链成员与 61 断言谓词不走 object_id 命中，各自单独算）。
func (r *OntAxiomRepo) CountReferencesByTypes(ctx context.Context, ontologyID uint32, entityID uint32, axiomTypes []uint32) (int, error) {
	return r.entClient.Client().OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.ObjectIDEQ(entityID),
			ontaxiom.AxiomTypeIn(axiomTypes...),
			ontaxiom.DeletedAtIsNil(),
		).
		Count(ctx)
}

// ---- 06 期个体查询辅助（两段式 List 的第一段 / usage / 计数徽标） ----

// ListSubjectIDsByCodeValue 编码过滤的第一段（62 行 object_id=编码属性、value_text 精确/前缀命中
// → 去重主语个体 id 集；prefix=true 走 HasPrefix，命中 idx_ont_axioms_type_value 前缀索引）
func (r *OntAxiomRepo) ListSubjectIDsByCodeValue(ctx context.Context, ontologyID, propertyID uint32, value string, prefix bool) ([]uint32, error) {
	valueCond := ontaxiom.ValueTextEQ(value)
	if prefix {
		valueCond = ontaxiom.ValueTextHasPrefix(value)
	}
	rows, err := r.entClient.Client().OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.AxiomTypeEQ(ontology.AxiomDataPropertyAssertion),
			ontaxiom.ObjectIDEQ(propertyID),
			ontaxiom.DeletedAtIsNil(),
			valueCond,
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return distinctSubjectIDs(rows), nil
}

// ListSubjectIDsByClassInstances 类目过滤的第一段（60 行 object_id ∈ 类目集合 → 去重主语个体 id 集）
func (r *OntAxiomRepo) ListSubjectIDsByClassInstances(ctx context.Context, ontologyID uint32, classIDs []uint32) ([]uint32, error) {
	if len(classIDs) == 0 {
		return nil, nil
	}
	rows, err := r.entClient.Client().OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.AxiomTypeEQ(ontology.AxiomTypeAssertion),
			ontaxiom.ObjectIDIn(classIDs...),
			ontaxiom.DeletedAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return distinctSubjectIDs(rows), nil
}

// CountDirectInstancesByClass 类树实例数徽标（60 行 group by object_id → 直接实例计数，不含子类实例）
func (r *OntAxiomRepo) CountDirectInstancesByClass(ctx context.Context, ontologyID uint32) (map[uint32]int, error) {
	rows, err := r.entClient.Client().OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.AxiomTypeEQ(ontology.AxiomTypeAssertion),
			ontaxiom.DeletedAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[uint32]int, len(rows))
	for _, ax := range rows {
		if ax.ObjectID == nil || *ax.ObjectID == 0 {
			continue
		}
		counts[*ax.ObjectID]++
	}
	return counts, nil
}

// ListByPredicateIRI 谓词 IRI 命中的公理行（61 断言谓词走 predicate_iri，usage 反查/级联删除专用；
// 不走 object_id 索引，量级 = 该属性的断言数，可接受）
func (r *OntAxiomRepo) ListByPredicateIRI(ctx context.Context, ontologyID uint32, axiomType uint32, predicateIRI string) ([]*ent.OntAxiom, error) {
	return r.entClient.Client().OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.AxiomTypeEQ(axiomType),
			ontaxiom.PredicateIriEQ(predicateIRI),
			ontaxiom.DeletedAtIsNil(),
		).
		Order(ent.Asc(ontaxiom.FieldSortOrder), ent.Asc(ontaxiom.FieldID)).
		All(ctx)
}

// ListBySubjects 一组主语的全部未删公理（个体列表聚合列批量填充）
func (r *OntAxiomRepo) ListBySubjects(ctx context.Context, ontologyID uint32, subjectIDs []uint32) ([]*ent.OntAxiom, error) {
	if len(subjectIDs) == 0 {
		return nil, nil
	}
	return r.entClient.Client().OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.SubjectIDIn(subjectIDs...),
			ontaxiom.DeletedAtIsNil(),
		).
		Order(ent.Asc(ontaxiom.FieldSortOrder), ent.Asc(ontaxiom.FieldID)).
		All(ctx)
}

// distinctSubjectIDs 公理行 → 去重主语 id 集（保序）
func distinctSubjectIDs(rows []*ent.OntAxiom) []uint32 {
	seen := make(map[uint32]struct{}, len(rows))
	out := make([]uint32, 0, len(rows))
	for _, ax := range rows {
		if ax.SubjectID == nil || *ax.SubjectID == 0 {
			continue
		}
		if _, ok := seen[*ax.SubjectID]; ok {
			continue
		}
		seen[*ax.SubjectID] = struct{}{}
		out = append(out, *ax.SubjectID)
	}
	return out
}

// ---- ontology.AxiomStore：事务工厂 ----

// WithTx 在单一 ent 事务内执行管线（任何一步出错回滚）
func (r *OntAxiomRepo) WithTx(ctx context.Context, fn func(ctx context.Context, tx ontology.AxiomTx) error) error {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "start transaction failed: %s", err.Error())
		return err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				r.log.Errorf(ctx, "transaction rollback failed: %s", rollbackErr.Error())
			}
		}
	}()

	if err = fn(ctx, &ontologyTx{tx: tx, log: r.log}); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "transaction commit failed: %s", err.Error())
		return err
	}
	committed = true
	return nil
}

// ontologyTx AxiomTx 的事务实现（方法全部跑在 *ent.Tx 上）
type ontologyTx struct {
	tx  *ent.Tx
	log *bLogger.Helper
}

// LockOntology 本体行 SELECT ... FOR UPDATE（未删过滤；软删/不存在 → ent NotFound）
func (t *ontologyTx) LockOntology(ctx context.Context, ontologyID uint32) (*ent.OntOntology, error) {
	return t.tx.OntOntology.Query().
		Where(ontontology.IDEQ(ontologyID), ontontology.DeletedAtIsNil()).
		ForUpdate().
		Only(ctx)
}

// InsertEntity 落库新实体草稿
func (t *ontologyTx) InsertEntity(ctx context.Context, ontologyID uint32, draft *ontology.EntityDraft, createdBy uint32) (uint32, error) {
	now := time.Now()
	e, err := t.tx.OntEntity.Create().
		SetOntologyID(ontologyID).
		SetEntityType(draft.EntityType).
		SetIri(draft.IRI).
		SetLocalName(draft.LocalName).
		SetDeprecated(false).
		SetNillableCreatedBy(trans.Ptr(createdBy)).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		t.log.Errorf(ctx, "insert ontology entity failed: %s", err.Error())
		return 0, mapDuplicateError(err, ontology.ReasonEntityIRIDuplicate, "已存在相同 IRI 的实体："+draft.IRI)
	}
	return e.ID, nil
}

// InsertAxiom 插入公理行
func (t *ontologyTx) InsertAxiom(ctx context.Context, ontologyID uint32, c ontology.AxiomChange, createdBy uint32) (uint32, error) {
	now := time.Now()
	builder := t.tx.OntAxiom.Create().
		SetOntologyID(ontologyID).
		SetAxiomType(c.AxiomType).
		SetSubjectID(c.SubjectID).
		SetSortOrder(sortOrderToUint32(c.SortOrder)).
		SetNillableCreatedBy(trans.Ptr(createdBy)).
		SetCreatedAt(now)
	if c.ObjectID != 0 {
		builder.SetObjectID(c.ObjectID)
	}
	if c.PredicateIRI != "" {
		builder.SetPredicateIri(c.PredicateIRI)
	}
	if c.ValueText != "" {
		builder.SetValueText(c.ValueText)
	}
	if c.Lang != "" {
		builder.SetLang(c.Lang)
	}
	ax, err := builder.Save(ctx)
	if err != nil {
		t.log.Errorf(ctx, "insert ontology axiom failed: %s", err.Error())
		return 0, err
	}
	return ax.ID, nil
}

// SoftDeleteAxiom 软删公理行
func (t *ontologyTx) SoftDeleteAxiom(ctx context.Context, axiomID uint32, deletedBy uint32) error {
	now := time.Now()
	if err := t.tx.OntAxiom.UpdateOneID(axiomID).
		SetDeletedAt(now).
		SetNillableDeletedBy(trans.Ptr(deletedBy)).
		SetUpdatedAt(now).
		Exec(ctx); err != nil {
		t.log.Errorf(ctx, "soft delete ontology axiom %d failed: %s", axiomID, err.Error())
		return err
	}
	return nil
}

// SoftDeleteEntity 软删实体行
func (t *ontologyTx) SoftDeleteEntity(ctx context.Context, entityID uint32, deletedBy uint32) error {
	now := time.Now()
	if err := t.tx.OntEntity.UpdateOneID(entityID).
		SetDeletedAt(now).
		SetNillableDeletedBy(trans.Ptr(deletedBy)).
		SetUpdatedAt(now).
		Exec(ctx); err != nil {
		t.log.Errorf(ctx, "soft delete ontology entity %d failed: %s", entityID, err.Error())
		return err
	}
	return nil
}

// ListAnnotationAxiomsBySubjects 一组主语的未删注解断言公理（渲染缓存重算）
func (t *ontologyTx) ListAnnotationAxiomsBySubjects(ctx context.Context, ontologyID uint32, subjectIDs []uint32) ([]*ent.OntAxiom, error) {
	if len(subjectIDs) == 0 {
		return nil, nil
	}
	return t.tx.OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.AxiomTypeEQ(ontology.AxiomAnnotationAssertion),
			ontaxiom.SubjectIDIn(subjectIDs...),
			ontaxiom.DeletedAtIsNil(),
		).
		All(ctx)
}

// ListEntities 本体下全部未删实体
func (t *ontologyTx) ListEntities(ctx context.Context, ontologyID uint32) ([]*ent.OntEntity, error) {
	return t.tx.OntEntity.Query().
		Where(ontentity.OntologyIDEQ(ontologyID), ontentity.DeletedAtIsNil()).
		All(ctx)
}

// ListAxioms 本体下全部未删公理
func (t *ontologyTx) ListAxioms(ctx context.Context, ontologyID uint32) ([]*ent.OntAxiom, error) {
	return t.tx.OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.DeletedAtIsNil()).
		All(ctx)
}

// UpdateRenderCache 更新渲染缓存列（display_label nil = 显式置 NULL）
func (t *ontologyTx) UpdateRenderCache(ctx context.Context, entityID uint32, displayLabel *string, deprecated bool) error {
	now := time.Now()
	builder := t.tx.OntEntity.UpdateOneID(entityID).SetDeprecated(deprecated).SetUpdatedAt(now)
	if displayLabel != nil {
		builder.SetDisplayLabel(*displayLabel)
	} else {
		builder.ClearDisplayLabel()
	}
	if err := builder.Exec(ctx); err != nil {
		t.log.Errorf(ctx, "update render cache for entity %d failed: %s", entityID, err.Error())
		return err
	}
	return nil
}

// BumpVersion 写入自增后的本体版本号
func (t *ontologyTx) BumpVersion(ctx context.Context, ontologyID uint32, nextVersion uint32) error {
	if err := t.tx.OntOntology.UpdateOneID(ontologyID).
		SetVersion(nextVersion).
		SetUpdatedAt(time.Now()).
		Exec(ctx); err != nil {
		t.log.Errorf(ctx, "bump ontology version failed: %s", err.Error())
		return err
	}
	return nil
}

// InsertChangeLog 写变更日志行
func (t *ontologyTx) InsertChangeLog(ctx context.Context, entry *ontology.ChangeLogEntry, ontologyID uint32, createdBy uint32) error {
	_, err := t.tx.OntChangeLog.Create().
		SetOntologyID(ontologyID).
		SetOperation(entry.Operation).
		SetChangeDetail(entry.ChangeDetail).
		SetBeforeVersion(entry.BeforeVersion).
		SetAfterVersion(entry.AfterVersion).
		SetNillableCreatedBy(trans.Ptr(createdBy)).
		SetCreatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.log.Errorf(ctx, "insert ontology change log failed: %s", err.Error())
		return err
	}
	return nil
}

// NextNo 号池两段式取号（实现 ontology.SerialTx：ApplyInTx 回调内以接口断言取用——
// 取号与公理写同事务，06 号文档 §2.3/§4.1；实现复用 OntSerialRepo 的 nextSerialNo）
func (t *ontologyTx) NextNo(ctx context.Context, ontologyID uint32, purpose, prefix string) (uint64, error) {
	return nextSerialNo(ctx, t.tx, t.log, ontologyID, purpose, prefix)
}

// sortOrderToUint32 AxiomChange.SortOrder（int32）→ ent uint32 列
func sortOrderToUint32(v int32) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(v)
}
