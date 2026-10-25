package data

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/mapper"

	onestopV1 "go-wind-admin/onestop/api/gen/go/onestop/service/v1"

	"go-wind-admin/onestop/data/ent"
	"go-wind-admin/onestop/data/ent/predicate"
	"go-wind-admin/onestop/data/ent/servicetype"
)

type ServiceTypeRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper

	levelConverter *mapper.EnumTypeConverter[onestopV1.Level, servicetype.Level]

	mapper *mapper.CopierMapper[onestopV1.ServiceType, ent.ServiceType]

	repository *entCrud.Repository[
		ent.ServiceTypeQuery, ent.ServiceTypeSelect,
		ent.ServiceTypeCreate, ent.ServiceTypeCreateBulk,
		ent.ServiceTypeUpdate, ent.ServiceTypeUpdateOne,
		ent.ServiceTypeDelete,
		predicate.ServiceType,
		onestopV1.ServiceType, ent.ServiceType,
	]
}

func NewServiceTypeRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *ServiceTypeRepo {
	repo := &ServiceTypeRepo{
		log:            ctx.NewLoggerHelper("service-type/repo/onestop"),
		entClient:      entClient,
		mapper:         mapper.NewCopierMapper[onestopV1.ServiceType, ent.ServiceType](),
		levelConverter: mapper.NewEnumTypeConverter[onestopV1.Level, servicetype.Level](onestopV1.Level_name, onestopV1.Level_value),
	}

	repo.init()

	return repo
}

func (r *ServiceTypeRepo) init() {
	r.repository = entCrud.NewRepository[
		ent.ServiceTypeQuery, ent.ServiceTypeSelect,
		ent.ServiceTypeCreate, ent.ServiceTypeCreateBulk,
		ent.ServiceTypeUpdate, ent.ServiceTypeUpdateOne,
		ent.ServiceTypeDelete,
		predicate.ServiceType,
		onestopV1.ServiceType, ent.ServiceType,
	](r.mapper)

	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.levelConverter.NewConverterPair())
}

func (r *ServiceTypeRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.entClient.Client().ServiceType.Query()
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query service type count failed: %s", err.Error())
		return 0, onestopV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

func (r *ServiceTypeRepo) List(ctx context.Context, req *paginationV1.PagingRequest) (*onestopV1.ListServiceTypeResponse, error) {
	if req == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().ServiceType.Query()

	// 默认业务序 = 服务编码升序。编码是唯一键且天然按「大类 → 中类 → 小类 → 细类」
	// 前缀递增，单独维护 sort_order 只会制造第二套顺序来源（本期已移除该字段）。
	// 调用方显式传 sorting/order_by 时尊重显式排序。
	if len(req.GetSorting()) == 0 && (req.GetOrderBy() == "" || req.GetOrderBy() == "[]") {
		builder.Order(ent.Asc(servicetype.FieldCode))
	}

	ret, err := r.repository.ListWithPaging(ctx, builder, builder.Clone(), req)
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return &onestopV1.ListServiceTypeResponse{Total: 0, Items: nil}, nil
	}

	return &onestopV1.ListServiceTypeResponse{
		Total: ret.Total,
		Items: ret.Items,
	}, nil
}

func (r *ServiceTypeRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.entClient.Client().ServiceType.Query().
		Where(servicetype.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query service type exist failed: %s", err.Error())
		return false, onestopV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *ServiceTypeRepo) Get(ctx context.Context, req *onestopV1.GetServiceTypeRequest) (*onestopV1.ServiceType, error) {
	if req == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().ServiceType.Query()

	var whereCond []func(s *sql.Selector)
	switch req.QueryBy.(type) {
	default:
	case *onestopV1.GetServiceTypeRequest_Id:
		whereCond = append(whereCond, servicetype.IDEQ(req.GetId()))
	}

	dto, err := r.repository.Get(ctx, builder, req.GetViewMask(), whereCond...)
	if err != nil {
		return nil, err
	}

	return dto, err
}

// GetByID 按主键取单条；不存在返回 ErrorNotFound。
func (r *ServiceTypeRepo) GetByID(ctx context.Context, id uint32) (*onestopV1.ServiceType, error) {
	entity, err := r.entClient.Client().ServiceType.Query().
		Where(servicetype.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, onestopV1.ErrorNotFound("service type not found")
		}
		r.log.Errorf(ctx, "query service type by id failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("query service type failed")
	}

	return r.mapper.ToDTO(entity), nil
}

// IsExistCode 服务编码是否已存在（excludeID 用于更新场景排除自身）。
func (r *ServiceTypeRepo) IsExistCode(ctx context.Context, code string, excludeID uint32) (bool, error) {
	query := r.entClient.Client().ServiceType.Query().
		Where(servicetype.CodeEQ(code))
	if excludeID != 0 {
		query = query.Where(servicetype.IDNEQ(excludeID))
	}

	exist, err := query.Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query service type code exist failed: %s", err.Error())
		return false, onestopV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

// HasChildren 是否存在下级服务类型（删除前守卫）。
func (r *ServiceTypeRepo) HasChildren(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.entClient.Client().ServiceType.Query().
		Where(servicetype.ParentIDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query service type children failed: %s", err.Error())
		return false, onestopV1.ErrorInternalServerError("query children failed")
	}
	return exist, nil
}

func (r *ServiceTypeRepo) Create(ctx context.Context, req *onestopV1.CreateServiceTypeRequest) error {
	if req == nil || req.Data == nil {
		return onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().ServiceType.Create().
		SetNillableCode(req.Data.Code).
		SetNillableName(req.Data.Name).
		SetNillableLevel(r.levelConverter.ToEntity(req.Data.Level)).
		SetNillableParentID(req.Data.ParentId).
		SetNillableDescription(req.Data.Description).
		SetNillableIsEnabled(req.Data.IsEnabled).
		SetNillableRepairPost(req.Data.RepairPost).
		SetNillableStandardWorkHours(req.Data.StandardWorkHours).
		SetNillableResponseTimeLimit(req.Data.ResponseTimeLimit).
		SetNillableCompletionTimeLimit(req.Data.CompletionTimeLimit).
		SetNillableGuidance(req.Data.Guidance).
		SetNillableCreatedBy(req.Data.CreatedBy).
		SetCreatedAt(time.Now())

	// jsonb 数组列：非空才写，空 = 未配置（NULL）
	if len(req.Data.GetCommonPhrases()) > 0 {
		builder.SetCommonPhrases(req.Data.GetCommonPhrases())
	}
	if len(req.Data.GetStandardTools()) > 0 {
		builder.SetStandardTools(req.Data.GetStandardTools())
	}
	if len(req.Data.GetSpareParts()) > 0 {
		builder.SetSpareParts(req.Data.GetSpareParts())
	}

	if req.Data.Id != nil {
		builder.SetID(req.GetData().GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf(ctx, "insert service type failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("insert service type failed")
	}

	return nil
}

func (r *ServiceTypeRepo) Update(ctx context.Context, req *onestopV1.UpdateServiceTypeRequest) error {
	if req == nil || req.Data == nil {
		return onestopV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return onestopV1.ErrorBadRequest("id is required")
	}

	// 如果不存在则创建
	if req.GetAllowMissing() {
		exist, err := r.IsExist(ctx, req.GetId())
		if err != nil {
			return err
		}
		if !exist {
			createReq := &onestopV1.CreateServiceTypeRequest{Data: req.Data}
			createReq.Data.CreatedBy = createReq.Data.UpdatedBy
			createReq.Data.UpdatedBy = nil
			return r.Create(ctx, createReq)
		}
	}

	builder := r.entClient.Client().ServiceType.Update()
	err := r.repository.UpdateX(ctx, builder, req.Data, req.GetUpdateMask(),
		func(dto *onestopV1.ServiceType) {
			builder.
				SetNillableCode(req.Data.Code).
				SetNillableName(req.Data.Name).
				SetNillableLevel(r.levelConverter.ToEntity(req.Data.Level)).
				SetNillableParentID(req.Data.ParentId).
				SetNillableDescription(req.Data.Description).
				SetNillableIsEnabled(req.Data.IsEnabled).
				SetNillableRepairPost(req.Data.RepairPost).
				SetNillableStandardWorkHours(req.Data.StandardWorkHours).
				SetNillableResponseTimeLimit(req.Data.ResponseTimeLimit).
				SetNillableCompletionTimeLimit(req.Data.CompletionTimeLimit).
				SetNillableGuidance(req.Data.Guidance).
				SetNillableUpdatedBy(req.Data.UpdatedBy).
				SetUpdatedAt(time.Now())

			// jsonb 数组列：UpdateX 已先按 mask 过滤 req.Data（未列入 mask 的数组被清空），
			// 非空才 Set；mask 内且为空切片时由 go-crud 的 applyUpdateNilFieldMask 统一置
			// NULL——这里不能再 Clear，同一列双赋值会触发 PG 42601（参照 org_unit.subtype_path）。
			if len(req.Data.GetCommonPhrases()) > 0 {
				builder.SetCommonPhrases(req.Data.GetCommonPhrases())
			}
			if len(req.Data.GetStandardTools()) > 0 {
				builder.SetStandardTools(req.Data.GetStandardTools())
			}
			if len(req.Data.GetSpareParts()) > 0 {
				builder.SetSpareParts(req.Data.GetSpareParts())
			}
		},
		func(s *sql.Selector) {
			s.Where(sql.EQ(servicetype.FieldID, req.GetId()))
		},
	)
	if err != nil {
		r.log.Errorf(ctx, "update service type failed: %s", err.Error())
		return err
	}

	return err
}

func (r *ServiceTypeRepo) Delete(ctx context.Context, req *onestopV1.DeleteServiceTypeRequest) error {
	if req == nil {
		return onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().ServiceType.Delete()

	_, err := r.repository.Delete(ctx, builder, func(s *sql.Selector) {
		s.Where(sql.EQ(servicetype.FieldID, req.GetId()))
	})
	if err != nil {
		r.log.Errorf(ctx, "delete service type failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("delete service type failed")
	}

	return nil
}
