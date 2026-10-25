package data

import (
	"context"
	_ "embed"
	"encoding/json"
	"time"

	"github.com/tx7do/go-utils/timeutil"

	"entgo.io/ent/dialect/sql"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/mapper"

	onestopV1 "go-wind-admin/onestop/api/gen/go/onestop/service/v1"

	"go-wind-admin/onestop/data/ent"
	"go-wind-admin/onestop/data/ent/inspectionplan"
	"go-wind-admin/onestop/data/ent/predicate"
	"go-wind-admin/onestop/data/ent/schema"
	"go-wind-admin/onestop/data/ent/servicetype"
)

type InspectionPlanRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper

	freqConverter *mapper.EnumTypeConverter[onestopV1.FreqType, inspectionplan.FreqType]

	mapper *mapper.CopierMapper[onestopV1.InspectionPlan, ent.InspectionPlan]

	repository *entCrud.Repository[
		ent.InspectionPlanQuery, ent.InspectionPlanSelect,
		ent.InspectionPlanCreate, ent.InspectionPlanCreateBulk,
		ent.InspectionPlanUpdate, ent.InspectionPlanUpdateOne,
		ent.InspectionPlanDelete,
		predicate.InspectionPlan,
		onestopV1.InspectionPlan, ent.InspectionPlan,
	]
}

func NewInspectionPlanRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *InspectionPlanRepo {
	repo := &InspectionPlanRepo{
		log:           ctx.NewLoggerHelper("inspection-plan/repo/onestop"),
		entClient:     entClient,
		mapper:        mapper.NewCopierMapper[onestopV1.InspectionPlan, ent.InspectionPlan](),
		freqConverter: mapper.NewEnumTypeConverter[onestopV1.FreqType, inspectionplan.FreqType](onestopV1.FreqType_name, onestopV1.FreqType_value),
	}

	repo.init()

	return repo
}

// planItemsToEntity / planItemsToDTO 计划点表 proto ↔ jsonb 结构互转。
func planItemsToEntity(items []*onestopV1.InspectionPlanItem) []schema.InspectionPlanItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]schema.InspectionPlanItem, 0, len(items))
	for _, item := range items {
		if item == nil || item.GetName() == "" {
			continue
		}
		out = append(out, schema.InspectionPlanItem{Name: item.GetName(), Enabled: item.GetEnabled()})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func planItemsToDTO(items []schema.InspectionPlanItem) []*onestopV1.InspectionPlanItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]*onestopV1.InspectionPlanItem, 0, len(items))
	for _, item := range items {
		out = append(out, &onestopV1.InspectionPlanItem{Name: item.Name, Enabled: item.Enabled})
	}
	return out
}

func (r *InspectionPlanRepo) init() {
	r.repository = entCrud.NewRepository[
		ent.InspectionPlanQuery, ent.InspectionPlanSelect,
		ent.InspectionPlanCreate, ent.InspectionPlanCreateBulk,
		ent.InspectionPlanUpdate, ent.InspectionPlanUpdateOne,
		ent.InspectionPlanDelete,
		predicate.InspectionPlan,
		onestopV1.InspectionPlan, ent.InspectionPlan,
	](r.mapper)

	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.freqConverter.NewConverterPair())
	// 计划点表：proto repeated message ↔ jsonb 结构数组（planItemsToEntity 的过滤
	// 在写侧兜底空名项；读侧保序还原）
	r.mapper.AppendConverters(copierutil.NewGenericTypeConverterPair(
		[]*onestopV1.InspectionPlanItem{},
		[]schema.InspectionPlanItem{},
		func(items []*onestopV1.InspectionPlanItem) []schema.InspectionPlanItem {
			return planItemsToEntity(items)
		},
		func(items []schema.InspectionPlanItem) []*onestopV1.InspectionPlanItem {
			return planItemsToDTO(items)
		},
	))
}

func (r *InspectionPlanRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.entClient.Client().InspectionPlan.Query()
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query inspection plan count failed: %s", err.Error())
		return 0, onestopV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

func (r *InspectionPlanRepo) List(ctx context.Context, req *paginationV1.PagingRequest) (*onestopV1.ListInspectionPlansResponse, error) {
	if req == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().InspectionPlan.Query()

	// 默认按创建序（同类别下按建立先后展示）
	if len(req.GetSorting()) == 0 && (req.GetOrderBy() == "" || req.GetOrderBy() == "[]") {
		builder.Order(ent.Asc(inspectionplan.FieldID))
	}

	ret, err := r.repository.ListWithPaging(ctx, builder, builder.Clone(), req)
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return &onestopV1.ListInspectionPlansResponse{Total: 0, Items: nil}, nil
	}

	return &onestopV1.ListInspectionPlansResponse{
		Total: ret.Total,
		Items: ret.Items,
	}, nil
}

func (r *InspectionPlanRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.entClient.Client().InspectionPlan.Query().
		Where(inspectionplan.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query inspection plan exist failed: %s", err.Error())
		return false, onestopV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *InspectionPlanRepo) Get(ctx context.Context, req *onestopV1.GetInspectionPlanRequest) (*onestopV1.InspectionPlan, error) {
	if req == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().InspectionPlan.Query()

	var whereCond []func(s *sql.Selector)
	switch req.QueryBy.(type) {
	default:
	case *onestopV1.GetInspectionPlanRequest_Id:
		whereCond = append(whereCond, inspectionplan.IDEQ(req.GetId()))
	}

	dto, err := r.repository.Get(ctx, builder, req.GetViewMask(), whereCond...)
	if err != nil {
		return nil, err
	}

	return dto, err
}

// GetByID 按主键取单条；不存在返回 ErrorNotFound。
func (r *InspectionPlanRepo) GetByID(ctx context.Context, id uint32) (*onestopV1.InspectionPlan, error) {
	entity, err := r.entClient.Client().InspectionPlan.Query().
		Where(inspectionplan.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, onestopV1.ErrorNotFound("inspection plan not found")
		}
		r.log.Errorf(ctx, "query inspection plan by id failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("query inspection plan failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *InspectionPlanRepo) Create(ctx context.Context, req *onestopV1.CreateInspectionPlanRequest) error {
	if req == nil || req.Data == nil {
		return onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().InspectionPlan.Create().
		SetNillableName(req.Data.Name).
		SetNillableServiceTypeID(req.Data.ServiceTypeId).
		SetNillableFreqType(r.freqConverter.ToEntity(req.Data.FreqType)).
		SetNillableClampToMonthEnd(req.Data.ClampToMonthEnd).
		SetNillablePost(req.Data.Post).
		SetNillableEffectiveFrom(timeutil.TimestamppbToTime(req.Data.EffectiveFrom)).
		SetNillableEffectiveTo(timeutil.TimestamppbToTime(req.Data.EffectiveTo)).
		SetNillableIsEnabled(req.Data.IsEnabled).
		SetNillableCreatedBy(req.Data.CreatedBy).
		SetCreatedAt(time.Now())

	// jsonb 数组/结构列：非空才写
	if len(req.Data.GetTimes()) > 0 {
		builder.SetTimes(req.Data.GetTimes())
	}
	if len(req.Data.GetWeekdays()) > 0 {
		builder.SetWeekdays(req.Data.GetWeekdays())
	}
	if len(req.Data.GetMonthDays()) > 0 {
		builder.SetMonthDays(req.Data.GetMonthDays())
	}
	if len(req.Data.GetMonths()) > 0 {
		builder.SetMonths(req.Data.GetMonths())
	}
	if len(req.Data.GetExcludedDates()) > 0 {
		builder.SetExcludedDates(req.Data.GetExcludedDates())
	}
	if items := planItemsToEntity(req.Data.GetSpaceItems()); len(items) > 0 {
		builder.SetSpaceItems(items)
	}
	if items := planItemsToEntity(req.Data.GetEquipmentItems()); len(items) > 0 {
		builder.SetEquipmentItems(items)
	}
	if len(req.Data.GetSpaceRefs()) > 0 {
		builder.SetSpaceRefs(req.Data.GetSpaceRefs())
	}
	if len(req.Data.GetEquipmentRefs()) > 0 {
		builder.SetEquipmentRefs(req.Data.GetEquipmentRefs())
	}

	if req.Data.Id != nil {
		builder.SetID(req.GetData().GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf(ctx, "insert inspection plan failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("insert inspection plan failed")
	}

	return nil
}

func (r *InspectionPlanRepo) Update(ctx context.Context, req *onestopV1.UpdateInspectionPlanRequest) error {
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
			createReq := &onestopV1.CreateInspectionPlanRequest{Data: req.Data}
			createReq.Data.CreatedBy = createReq.Data.UpdatedBy
			createReq.Data.UpdatedBy = nil
			return r.Create(ctx, createReq)
		}
	}

	builder := r.entClient.Client().InspectionPlan.Update()
	err := r.repository.UpdateX(ctx, builder, req.Data, req.GetUpdateMask(),
		func(dto *onestopV1.InspectionPlan) {
			builder.
				SetNillableName(req.Data.Name).
				SetNillableServiceTypeID(req.Data.ServiceTypeId).
				SetNillableFreqType(r.freqConverter.ToEntity(req.Data.FreqType)).
				SetNillableClampToMonthEnd(req.Data.ClampToMonthEnd).
				SetNillablePost(req.Data.Post).
				SetNillableEffectiveFrom(timeutil.TimestamppbToTime(req.Data.EffectiveFrom)).
				SetNillableEffectiveTo(timeutil.TimestamppbToTime(req.Data.EffectiveTo)).
				SetNillableIsEnabled(req.Data.IsEnabled).
				SetNillableUpdatedBy(req.Data.UpdatedBy).
				SetUpdatedAt(time.Now())

			// jsonb 数组/结构列：UpdateX 已先按 mask 过滤 req.Data（未列入 mask 的
			// 数组被清空），非空才 Set；mask 内且为空时由 go-crud 的 nil 字段掩码统一
			// 置 NULL——这里不能再 Clear，同列双赋值触发 PG 42601（service_type 同款）。
			if len(req.Data.GetTimes()) > 0 {
				builder.SetTimes(req.Data.GetTimes())
			}
			if len(req.Data.GetWeekdays()) > 0 {
				builder.SetWeekdays(req.Data.GetWeekdays())
			}
			if len(req.Data.GetMonthDays()) > 0 {
				builder.SetMonthDays(req.Data.GetMonthDays())
			}
			if len(req.Data.GetMonths()) > 0 {
				builder.SetMonths(req.Data.GetMonths())
			}
			if len(req.Data.GetExcludedDates()) > 0 {
				builder.SetExcludedDates(req.Data.GetExcludedDates())
			}
			if items := planItemsToEntity(req.Data.GetSpaceItems()); len(items) > 0 {
				builder.SetSpaceItems(items)
			}
			if items := planItemsToEntity(req.Data.GetEquipmentItems()); len(items) > 0 {
				builder.SetEquipmentItems(items)
			}
			if len(req.Data.GetSpaceRefs()) > 0 {
				builder.SetSpaceRefs(req.Data.GetSpaceRefs())
			}
			if len(req.Data.GetEquipmentRefs()) > 0 {
				builder.SetEquipmentRefs(req.Data.GetEquipmentRefs())
			}
		},
		func(s *sql.Selector) {
			s.Where(sql.EQ(inspectionplan.FieldID, req.GetId()))
		},
	)
	if err != nil {
		r.log.Errorf(ctx, "update inspection plan failed: %s", err.Error())
		return err
	}

	return err
}

func (r *InspectionPlanRepo) Delete(ctx context.Context, req *onestopV1.DeleteInspectionPlanRequest) error {
	if req == nil {
		return onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().InspectionPlan.Delete()

	_, err := r.repository.Delete(ctx, builder, func(s *sql.Selector) {
		s.Where(sql.EQ(inspectionplan.FieldID, req.GetId()))
	})
	if err != nil {
		r.log.Errorf(ctx, "delete inspection plan failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("delete inspection plan failed")
	}

	return nil
}

type inspectionPlanSeed struct {
	Code           string   `json:"code"`  // 巡检类别编码（0103 子树）
	Name           string   `json:"name"`  // 计划名称
	FreqType       string   `json:"freqType"`
	Times          []string `json:"times,omitempty"`
	Weekdays       []int32  `json:"weekdays,omitempty"`
	MonthDays      []int32  `json:"monthDays,omitempty"`
	ClampMonthEnd  *bool    `json:"clampToMonthEnd,omitempty"`
	Months         []int32  `json:"months,omitempty"`
	ExcludedDates  []string `json:"excludedDates,omitempty"`
	Post           string   `json:"post,omitempty"`
	SpaceItems     []string `json:"spaceItems,omitempty"`
	EquipmentItems []string `json:"equipmentItems,omitempty"`
}

//go:embed seeds/inspection_plans.json
var inspectionPlanSeedData []byte

// SeedDefaults 在巡检计划表为空时播种内置行业模板计划；非空表不做任何写入。
// 计划按编码挂到已存在的服务类型上（服务类型种子先行），查不到类别的条目跳过并留痕。
func (r *InspectionPlanRepo) SeedDefaults(ctx context.Context) error {
	count, err := r.entClient.Client().InspectionPlan.Query().Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "count inspection plans before seeding failed: %v", err)
		return err
	}
	if count > 0 {
		return nil
	}

	var seeds []inspectionPlanSeed
	if err = json.Unmarshal(inspectionPlanSeedData, &seeds); err != nil {
		r.log.Errorf(ctx, "unmarshal inspection plan seed data failed: %v", err)
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
			r.log.Warnf(ctx, "inspection plan seed %q: service type %q not found, skipped", seed.Name, seed.Code)
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

		freq := inspectionplan.FreqType(seed.FreqType)
		if err = inspectionplan.FreqTypeValidator(freq); err != nil {
			r.log.Errorf(ctx, "invalid inspection plan seed %q: %v", seed.Name, err)
			return err
		}

		clamp := true
		if seed.ClampMonthEnd != nil {
			clamp = *seed.ClampMonthEnd
		}

		builder := r.entClient.Client().InspectionPlan.Create().
			SetName(seed.Name).
			SetServiceTypeID(typeID).
			SetFreqType(freq).
			SetClampToMonthEnd(clamp).
			SetIsEnabled(true).
			SetCreatedAt(now).
			SetUpdatedAt(now)
		if seed.Post != "" {
			builder.SetPost(seed.Post)
		}
		if len(seed.Times) > 0 {
			builder.SetTimes(seed.Times)
		}
		if len(seed.Weekdays) > 0 {
			builder.SetWeekdays(seed.Weekdays)
		}
		if len(seed.MonthDays) > 0 {
			builder.SetMonthDays(seed.MonthDays)
		}
		if len(seed.Months) > 0 {
			builder.SetMonths(seed.Months)
		}
		if len(seed.ExcludedDates) > 0 {
			builder.SetExcludedDates(seed.ExcludedDates)
		}
		if len(seed.SpaceItems) > 0 {
			items := make([]schema.InspectionPlanItem, 0, len(seed.SpaceItems))
			for _, name := range seed.SpaceItems {
				items = append(items, schema.InspectionPlanItem{Name: name, Enabled: true})
			}
			builder.SetSpaceItems(items)
		}
		if len(seed.EquipmentItems) > 0 {
			items := make([]schema.InspectionPlanItem, 0, len(seed.EquipmentItems))
			for _, name := range seed.EquipmentItems {
				items = append(items, schema.InspectionPlanItem{Name: name, Enabled: true})
			}
			builder.SetEquipmentItems(items)
		}

		if err = builder.Exec(ctx); err != nil {
			r.log.Errorf(ctx, "insert inspection plan seed %q failed: %v", seed.Name, err)
			return err
		}
		created++
	}

	if created > 0 {
		r.log.Infof(ctx, "seeded %d built-in inspection plans", created)
	}
	return nil
}
