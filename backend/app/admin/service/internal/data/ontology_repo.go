package data

import (
	"context"
	"net/url"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	entCrud "github.com/tx7do/go-crud/entgo"
	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/trans"

	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/data/ent/ontaxiom"
	"go-wind-admin/app/admin/service/internal/data/ent/ontchangelog"
	"go-wind-admin/app/admin/service/internal/data/ent/ontentity"
	"go-wind-admin/app/admin/service/internal/data/ent/ontontology"
	"go-wind-admin/app/admin/service/internal/data/ent/predicate"

	adminV1 "go-wind-admin/api/gen/go/admin/service/v1"
	ontologyV1 "go-wind-admin/api/gen/go/ontology/service/v1"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// OntologyRepo 本体文档仓储（标准通用 CRUD，镜像 dict_type_repo；软删为显式 UPDATE 实现——
// 本仓 TimeAt mixin 只加列不带软删拦截器，所有读路径显式过滤 deleted_at IS NULL）。
type OntologyRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper

	mapper *mapper.CopierMapper[ontologyV1.Ontology, ent.OntOntology]

	repository *entCrud.Repository[
		ent.OntOntologyQuery, ent.OntOntologySelect,
		ent.OntOntologyCreate, ent.OntOntologyCreateBulk,
		ent.OntOntologyUpdate, ent.OntOntologyUpdateOne,
		ent.OntOntologyDelete,
		predicate.OntOntology,
		ontologyV1.Ontology, ent.OntOntology,
	]
}

func NewOntologyRepo(
	ctx *bootstrap.Context,
	entClient *entCrud.EntClient[*ent.Client],
) *OntologyRepo {
	repo := &OntologyRepo{
		log:       ctx.NewLoggerHelper("ontology/repo/admin-service"),
		entClient: entClient,
		mapper:    mapper.NewCopierMapper[ontologyV1.Ontology, ent.OntOntology](),
	}

	repo.init()

	return repo
}

func (r *OntologyRepo) init() {
	r.repository = entCrud.NewRepository[
		ent.OntOntologyQuery, ent.OntOntologySelect,
		ent.OntOntologyCreate, ent.OntOntologyCreateBulk,
		ent.OntOntologyUpdate, ent.OntOntologyUpdateOne,
		ent.OntOntologyDelete,
		predicate.OntOntology,
		ontologyV1.Ontology, ent.OntOntology,
	](r.mapper)

	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

// notDeleted 软删过滤（显式实现，无拦截器兜底）
func notDeleted(s *sql.Selector) {
	s.Where(sql.IsNull(s.C(ontontology.FieldDeletedAt)))
}

// mapDuplicateError 把 PG 唯一冲突（23505）映射为领域错误（partial unique index 兜底并发双写，
// 设计文档 §3.1.3）；其余错误原样返回。
func mapDuplicateError(err error, reason, message string) error {
	if ent.IsConstraintError(err) {
		return ontology.NewError(reason, "%s", message)
	}
	return err
}

// NormalizeBaseIRI base_iri 校验与规范化：必须为合法绝对 IRI，尾部无 # 时统一补 #。
func NormalizeBaseIRI(baseIRI string) (string, error) {
	trimmed := strings.TrimSpace(baseIRI)
	if trimmed == "" {
		return "", ontology.NewError(ontology.ReasonOntologyBaseIRIInvalid, "实体命名空间不合法：%s", baseIRI)
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" {
		return "", ontology.NewError(ontology.ReasonOntologyBaseIRIInvalid, "实体命名空间不合法：%s", baseIRI)
	}
	if !strings.HasSuffix(trimmed, "#") {
		trimmed += "#"
	}
	return trimmed, nil
}

// Count 未删本体行数（启动期 count==0 播种守卫用）
func (r *OntologyRepo) Count(ctx context.Context) (int, error) {
	return r.entClient.Client().OntOntology.Query().
		Where(ontontology.DeletedAtIsNil()).
		Count(ctx)
}

// List 分页查询（通用 filter DSL + 软删过滤）
func (r *OntologyRepo) List(ctx context.Context, req *paginationV1.PagingRequest) (*ontologyV1.ListOntologiesResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().OntOntology.Query().Modify(notDeleted)

	ret, err := r.repository.ListWithPaging(ctx, builder, builder.Clone(), req)
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return &ontologyV1.ListOntologiesResponse{Total: 0, Items: nil}, nil
	}

	return &ontologyV1.ListOntologiesResponse{
		Total: ret.Total,
		Items: ret.Items,
	}, nil
}

// SimpleList 活动本体切换器简况（仅启用本体，按名称排序）
func (r *OntologyRepo) SimpleList(ctx context.Context) (*ontologyV1.ListOntologiesSimpleResponse, error) {
	entities, err := r.entClient.Client().OntOntology.Query().
		Where(
			ontontology.DeletedAtIsNil(),
			ontontology.IsEnabledEQ(true),
		).
		Order(ent.Asc(ontontology.FieldOntologyName)).
		All(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query simple ontologies failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query ontologies failed")
	}

	items := make([]*ontologyV1.OntologyBrief, 0, len(entities))
	for _, e := range entities {
		items = append(items, &ontologyV1.OntologyBrief{
			Id:           &e.ID,
			OntologyName: e.OntologyName,
		})
	}
	return &ontologyV1.ListOntologiesSimpleResponse{Items: items}, nil
}

// Get 详情
func (r *OntologyRepo) Get(ctx context.Context, req *ontologyV1.GetOntologyRequest) (*ontologyV1.Ontology, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().OntOntology.Query().Modify(notDeleted)

	dto, err := r.repository.Get(ctx, builder, req.GetViewMask(), func(s *sql.Selector) {
		s.Where(sql.EQ(ontontology.FieldID, req.GetId()))
	})
	if err != nil {
		return nil, err
	}
	return dto, nil
}

// GetEntityByID 取本体 ent 行（服务层校验存在/启用用）
func (r *OntologyRepo) GetEntityByID(ctx context.Context, id uint32) (*ent.OntOntology, error) {
	return r.entClient.Client().OntOntology.Query().
		Where(ontontology.IDEQ(id), ontontology.DeletedAtIsNil()).
		Only(ctx)
}

// ExistsName 同名本体查重（仅未删行；名称唯一性不建索引，允许删除后重建同名）
func (r *OntologyRepo) ExistsName(ctx context.Context, name string) (bool, error) {
	return r.entClient.Client().OntOntology.Query().
		Where(ontontology.OntologyNameEQ(name), ontontology.DeletedAtIsNil()).
		Exist(ctx)
}

// ExistsIRI 本体 IRI 查重（仅未删行；DB 部分唯一索引兜底并发双写）
func (r *OntologyRepo) ExistsIRI(ctx context.Context, iri string) (bool, error) {
	return r.entClient.Client().OntOntology.Query().
		Where(ontontology.OntologyIriEQ(iri), ontontology.DeletedAtIsNil()).
		Exist(ctx)
}

// Create 创建本体文档
func (r *OntologyRepo) Create(ctx context.Context, data *ontologyV1.Ontology, createdBy uint32) error {
	builder := r.entClient.Client().OntOntology.Create().
		SetNillableOntologyName(data.OntologyName).
		SetNillableOntologyIri(data.OntologyIri).
		SetNillableBaseIri(data.BaseIri).
		SetNillableDescription(data.Description).
		SetNillableIsEnabled(data.IsEnabled).
		SetVersion(0).
		SetNillableCreatedBy(data.CreatedBy).
		SetCreatedAt(time.Now())

	if _, err := builder.Save(ctx); err != nil {
		r.log.Errorf(ctx, "insert ontology failed: %s", err.Error())
		return mapDuplicateError(err, ontology.ReasonOntologyIRIDuplicate, "已存在相同 IRI 的本体")
	}
	return nil
}

// Update 更新本体文档（base_iri 创建后不可变：DTO 只携带允许字段，服务层已忽略其余）
func (r *OntologyRepo) Update(ctx context.Context, req *ontologyV1.UpdateOntologyRequest, updatedBy uint32) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return adminV1.ErrorBadRequest("id is required")
	}

	// 仅投影允许更新的字段（base_iri/ontology_iri 不可变，version 由公理事务管线维护）
	allowed := &ontologyV1.Ontology{
		Id:           req.Data.Id,
		OntologyName: req.Data.OntologyName,
		Description:  req.Data.Description,
		IsEnabled:    req.Data.IsEnabled,
	}

	builder := r.entClient.Client().OntOntology.UpdateOneID(req.GetId())
	_, err := r.repository.UpdateOne(ctx, builder, allowed, req.GetUpdateMask(),
		func(dto *ontologyV1.Ontology) {
			builder.
				SetNillableOntologyName(dto.OntologyName).
				SetNillableDescription(dto.Description).
				SetNillableIsEnabled(dto.IsEnabled).
				SetNillableUpdatedBy(trans.Ptr(updatedBy)).
				SetUpdatedAt(time.Now())
		},
		func(s *sql.Selector) {
			s.Where(sql.EQ(ontontology.FieldID, req.GetId()))
		},
	)
	if err != nil {
		r.log.Errorf(ctx, "update ontology failed: %s", err.Error())
		return err
	}
	return nil
}

// Delete 删除本体文档（同一事务级联软删该本体下实体/公理/变更日志全部行；
// 变更日志随本体消亡，跨本体操作审计由宿主审计日志中间件承载）
func (r *OntologyRepo) Delete(ctx context.Context, ids []uint32, deletedBy uint32) error {
	if len(ids) == 0 {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "start transaction failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("start transaction failed")
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				r.log.Errorf(ctx, "transaction rollback failed: %s", rollbackErr.Error())
			}
		}
	}()

	now := time.Now()
	deletedByPtr := trans.Ptr(deletedBy)
	// 级联顺序：日志 → 公理 → 实体 → 号池 → 本体行
	if _, err = tx.OntChangeLog.Update().
		Where(ontchangelog.OntologyIDIn(ids...), ontchangelog.DeletedAtIsNil()).
		SetDeletedAt(now).SetNillableDeletedBy(deletedByPtr).SetUpdatedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "cascade delete change logs failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete ontology failed")
	}
	if _, err = tx.OntAxiom.Update().
		Where(ontaxiom.OntologyIDIn(ids...), ontaxiom.DeletedAtIsNil()).
		SetDeletedAt(now).SetNillableDeletedBy(deletedByPtr).SetUpdatedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "cascade delete axioms failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete ontology failed")
	}
	if _, err = tx.OntEntity.Update().
		Where(ontentity.OntologyIDIn(ids...), ontentity.DeletedAtIsNil()).
		SetDeletedAt(now).SetNillableDeletedBy(deletedByPtr).SetUpdatedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "cascade delete entities failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete ontology failed")
	}
	// 06 期增量：级联软删号池行（号池行是本体下无实体时的唯一残留；重建同名本体 = 全新编号空间）
	if err = DeleteByOntologies(ctx, tx, ids, deletedBy, now); err != nil {
		r.log.Errorf(ctx, "cascade delete serials failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete ontology failed")
	}
	if _, err = tx.OntOntology.Update().
		Where(ontontology.IDIn(ids...), ontontology.DeletedAtIsNil()).
		SetDeletedAt(now).SetNillableDeletedBy(deletedByPtr).SetUpdatedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "delete ontologies failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete ontology failed")
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "transaction commit failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("transaction commit failed")
	}
	committed = true
	return nil
}

// ToProto 本体 ent 行 → DTO（服务层复用）
func (r *OntologyRepo) ToProto(e *ent.OntOntology) *ontologyV1.Ontology {
	return r.mapper.ToDTO(e)
}
