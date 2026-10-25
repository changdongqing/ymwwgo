package data

import (
	"context"
	_ "embed"
	"encoding/json"
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
	"go-wind-admin/onestop/data/ent/transportroute"
)

type TransportRouteRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper

	mapper *mapper.CopierMapper[onestopV1.TransportRoute, ent.TransportRoute]

	repository *entCrud.Repository[
		ent.TransportRouteQuery, ent.TransportRouteSelect,
		ent.TransportRouteCreate, ent.TransportRouteCreateBulk,
		ent.TransportRouteUpdate, ent.TransportRouteUpdateOne,
		ent.TransportRouteDelete,
		predicate.TransportRoute,
		onestopV1.TransportRoute, ent.TransportRoute,
	]
}

func NewTransportRouteRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *TransportRouteRepo {
	repo := &TransportRouteRepo{
		log:       ctx.NewLoggerHelper("transport-route/repo/onestop"),
		entClient: entClient,
		mapper:    mapper.NewCopierMapper[onestopV1.TransportRoute, ent.TransportRoute](),
	}

	repo.init()

	return repo
}

func (r *TransportRouteRepo) init() {
	r.repository = entCrud.NewRepository[
		ent.TransportRouteQuery, ent.TransportRouteSelect,
		ent.TransportRouteCreate, ent.TransportRouteCreateBulk,
		ent.TransportRouteUpdate, ent.TransportRouteUpdateOne,
		ent.TransportRouteDelete,
		predicate.TransportRoute,
		onestopV1.TransportRoute, ent.TransportRoute,
	](r.mapper)

	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

func (r *TransportRouteRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.entClient.Client().TransportRoute.Query()
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query transport route count failed: %s", err.Error())
		return 0, onestopV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

func (r *TransportRouteRepo) List(ctx context.Context, req *paginationV1.PagingRequest) (*onestopV1.ListTransportRoutesResponse, error) {
	if req == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().TransportRoute.Query()

	// 默认按创建序（同类别下按建立先后展示），inspection_plan 同款
	if len(req.GetSorting()) == 0 && (req.GetOrderBy() == "" || req.GetOrderBy() == "[]") {
		builder.Order(ent.Asc(transportroute.FieldID))
	}

	ret, err := r.repository.ListWithPaging(ctx, builder, builder.Clone(), req)
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return &onestopV1.ListTransportRoutesResponse{Total: 0, Items: nil}, nil
	}

	return &onestopV1.ListTransportRoutesResponse{
		Total: ret.Total,
		Items: ret.Items,
	}, nil
}

func (r *TransportRouteRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.entClient.Client().TransportRoute.Query().
		Where(transportroute.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query transport route exist failed: %s", err.Error())
		return false, onestopV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *TransportRouteRepo) Get(ctx context.Context, req *onestopV1.GetTransportRouteRequest) (*onestopV1.TransportRoute, error) {
	if req == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().TransportRoute.Query()

	var whereCond []func(s *sql.Selector)
	switch req.QueryBy.(type) {
	default:
	case *onestopV1.GetTransportRouteRequest_Id:
		whereCond = append(whereCond, transportroute.IDEQ(req.GetId()))
	}

	dto, err := r.repository.Get(ctx, builder, req.GetViewMask(), whereCond...)
	if err != nil {
		return nil, err
	}

	return dto, err
}

// GetByID 按主键取单条；不存在返回 ErrorNotFound。
func (r *TransportRouteRepo) GetByID(ctx context.Context, id uint32) (*onestopV1.TransportRoute, error) {
	entity, err := r.entClient.Client().TransportRoute.Query().
		Where(transportroute.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, onestopV1.ErrorNotFound("transport route not found")
		}
		r.log.Errorf(ctx, "query transport route by id failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("query transport route failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *TransportRouteRepo) Create(ctx context.Context, req *onestopV1.CreateTransportRouteRequest) error {
	if req == nil || req.Data == nil {
		return onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().TransportRoute.Create().
		SetNillableName(req.Data.Name).
		SetNillableServiceTypeID(req.Data.ServiceTypeId).
		SetNillableOrigin(req.Data.Origin).
		SetNillableDestination(req.Data.Destination).
		SetNillableEstDurationMinutes(req.Data.EstDurationMinutes).
		SetNillableRoundTrip(req.Data.RoundTrip).
		SetNillableIsEnabled(req.Data.IsEnabled).
		SetNillableCreatedBy(req.Data.CreatedBy).
		SetCreatedAt(time.Now())

	// jsonb 数组列：非空才写，空 = 未配置（NULL，如按需即时路线无定时班次）
	if len(req.Data.GetViaPoints()) > 0 {
		builder.SetViaPoints(req.Data.GetViaPoints())
	}
	if len(req.Data.GetDepartureTimes()) > 0 {
		builder.SetDepartureTimes(req.Data.GetDepartureTimes())
	}

	if req.Data.Id != nil {
		builder.SetID(req.GetData().GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf(ctx, "insert transport route failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("insert transport route failed")
	}

	return nil
}

func (r *TransportRouteRepo) Update(ctx context.Context, req *onestopV1.UpdateTransportRouteRequest) error {
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
			createReq := &onestopV1.CreateTransportRouteRequest{Data: req.Data}
			createReq.Data.CreatedBy = createReq.Data.UpdatedBy
			createReq.Data.UpdatedBy = nil
			return r.Create(ctx, createReq)
		}
	}

	builder := r.entClient.Client().TransportRoute.Update()
	err := r.repository.UpdateX(ctx, builder, req.Data, req.GetUpdateMask(),
		func(dto *onestopV1.TransportRoute) {
			builder.
				SetNillableName(req.Data.Name).
				SetNillableServiceTypeID(req.Data.ServiceTypeId).
				SetNillableOrigin(req.Data.Origin).
				SetNillableDestination(req.Data.Destination).
				SetNillableEstDurationMinutes(req.Data.EstDurationMinutes).
				SetNillableRoundTrip(req.Data.RoundTrip).
				SetNillableIsEnabled(req.Data.IsEnabled).
				SetNillableUpdatedBy(req.Data.UpdatedBy).
				SetUpdatedAt(time.Now())

			// jsonb 数组列：UpdateX 已先按 mask 过滤 req.Data（未列入 mask 的数组被清空），
			// 非空才 Set；mask 内且为空切片时由 go-crud 的 nil 字段掩码统一置
			// NULL——这里不能再 Clear，同一列双赋值触发 PG 42601（service_type 同款）。
			if len(req.Data.GetViaPoints()) > 0 {
				builder.SetViaPoints(req.Data.GetViaPoints())
			}
			if len(req.Data.GetDepartureTimes()) > 0 {
				builder.SetDepartureTimes(req.Data.GetDepartureTimes())
			}
		},
		func(s *sql.Selector) {
			s.Where(sql.EQ(transportroute.FieldID, req.GetId()))
		},
	)
	if err != nil {
		r.log.Errorf(ctx, "update transport route failed: %s", err.Error())
		return err
	}

	return err
}

func (r *TransportRouteRepo) Delete(ctx context.Context, req *onestopV1.DeleteTransportRouteRequest) error {
	if req == nil {
		return onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().TransportRoute.Delete()

	_, err := r.repository.Delete(ctx, builder, func(s *sql.Selector) {
		s.Where(sql.EQ(transportroute.FieldID, req.GetId()))
	})
	if err != nil {
		r.log.Errorf(ctx, "delete transport route failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("delete transport route failed")
	}

	return nil
}

type transportRouteSeed struct {
	Code               string   `json:"code"` // 运送类别编码（0106 子树）
	Name               string   `json:"name"`
	Origin             string   `json:"origin"`
	Destination        string   `json:"destination"`
	ViaPoints          []string `json:"viaPoints,omitempty"`
	EstDurationMinutes *uint32  `json:"estDurationMinutes,omitempty"`
	RoundTrip          *bool    `json:"roundTrip,omitempty"`
	DepartureTimes     []string `json:"departureTimes,omitempty"`
}

//go:embed seeds/transport_routes.json
var transportRouteSeedData []byte

// SeedDefaults 在运送路线表为空时播种内置行业模板路线；非空表不做任何写入。
// 路线按编码挂到已存在的服务类型上（服务类型种子先行），查不到类别的条目跳过并留痕。
func (r *TransportRouteRepo) SeedDefaults(ctx context.Context) error {
	count, err := r.entClient.Client().TransportRoute.Query().Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "count transport routes before seeding failed: %v", err)
		return err
	}
	if count > 0 {
		return nil
	}

	var seeds []transportRouteSeed
	if err = json.Unmarshal(transportRouteSeedData, &seeds); err != nil {
		r.log.Errorf(ctx, "unmarshal transport route seed data failed: %v", err)
		return err
	}

	// 类别编码 → id（类别由 ServiceTypeRepo.SeedDefaults 先行播种；
	// 老库/缺类别时跳过对应条目，不阻断启动）
	idByCode := make(map[string]uint32)
	for _, seed := range seeds {
		if _, ok := idByCode[seed.Code]; ok {
			continue
		}
		typeEntity, err := r.entClient.Client().ServiceType.Query().
			Where(servicetype.CodeEQ(seed.Code)).
			Only(ctx)
		if err != nil {
			r.log.Warnf(ctx, "transport route seed %q: service type %q not found, skipped", seed.Name, seed.Code)
			continue
		}
		idByCode[seed.Code] = typeEntity.ID
	}

	created := 0
	now := time.Now()
	for _, seed := range seeds {
		typeID, ok := idByCode[seed.Code]
		if !ok {
			continue
		}

		builder := r.entClient.Client().TransportRoute.Create().
			SetName(seed.Name).
			SetServiceTypeID(typeID).
			SetOrigin(seed.Origin).
			SetDestination(seed.Destination).
			SetIsEnabled(true).
			SetCreatedAt(now).
			SetUpdatedAt(now)
		if seed.EstDurationMinutes != nil {
			builder.SetEstDurationMinutes(*seed.EstDurationMinutes)
		}
		if seed.RoundTrip != nil {
			builder.SetRoundTrip(*seed.RoundTrip)
		}
		if len(seed.ViaPoints) > 0 {
			builder.SetViaPoints(seed.ViaPoints)
		}
		if len(seed.DepartureTimes) > 0 {
			builder.SetDepartureTimes(seed.DepartureTimes)
		}

		if err = builder.Exec(ctx); err != nil {
			r.log.Errorf(ctx, "insert transport route seed %q failed: %v", seed.Name, err)
			return err
		}
		created++
	}

	if created > 0 {
		r.log.Infof(ctx, "seeded %d built-in transport routes", created)
	}
	return nil
}
