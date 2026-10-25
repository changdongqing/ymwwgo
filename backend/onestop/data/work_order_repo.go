package data

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/tx7do/go-utils/timeutil"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/mapper"
	"google.golang.org/protobuf/proto"

	onestopV1 "go-wind-admin/onestop/api/gen/go/onestop/service/v1"

	"go-wind-admin/onestop/data/ent"
	"go-wind-admin/onestop/data/ent/predicate"
	"go-wind-admin/onestop/data/ent/schema"
	"go-wind-admin/onestop/data/ent/workorder"
	"go-wind-admin/onestop/data/ent/workorderevent"
	"go-wind-admin/onestop/data/ent/workorderitem"
)

type WorkOrderRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper

	kindConverter        *mapper.EnumTypeConverter[onestopV1.OrderKind, workorder.OrderKind]
	sourceConverter      *mapper.EnumTypeConverter[onestopV1.WorkOrderSource, workorder.Source]
	channelConverter     *mapper.EnumTypeConverter[onestopV1.WorkOrderChannel, workorder.Channel]
	statusConverter      *mapper.EnumTypeConverter[onestopV1.WorkOrderStatus, workorder.Status]
	priorityConverter    *mapper.EnumTypeConverter[onestopV1.ServicePriority, workorder.Priority]
	temperatureConverter *mapper.EnumTypeConverter[onestopV1.TemperatureRequirement, workorder.TemperatureRequirement]
	itemScopeConverter   *mapper.EnumTypeConverter[onestopV1.WorkOrderItemScope, workorderitem.Scope]
	itemResultConverter  *mapper.EnumTypeConverter[onestopV1.WorkOrderItemResult, workorderitem.Result]
	eventActionConverter *mapper.EnumTypeConverter[onestopV1.WorkOrderEventAction, workorderevent.Action]
	eventFromConverter   *mapper.EnumTypeConverter[onestopV1.WorkOrderStatus, workorderevent.StatusFrom]
	eventToConverter     *mapper.EnumTypeConverter[onestopV1.WorkOrderStatus, workorderevent.StatusTo]

	mapper *mapper.CopierMapper[onestopV1.WorkOrder, ent.WorkOrder]
}

func NewWorkOrderRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *WorkOrderRepo {
	repo := &WorkOrderRepo{
		log:                  ctx.NewLoggerHelper("work-order/repo/onestop"),
		entClient:            entClient,
		mapper:               mapper.NewCopierMapper[onestopV1.WorkOrder, ent.WorkOrder](),
		kindConverter:        mapper.NewEnumTypeConverter[onestopV1.OrderKind, workorder.OrderKind](onestopV1.OrderKind_name, onestopV1.OrderKind_value),
		sourceConverter:      mapper.NewEnumTypeConverter[onestopV1.WorkOrderSource, workorder.Source](onestopV1.WorkOrderSource_name, onestopV1.WorkOrderSource_value),
		channelConverter:     mapper.NewEnumTypeConverter[onestopV1.WorkOrderChannel, workorder.Channel](onestopV1.WorkOrderChannel_name, onestopV1.WorkOrderChannel_value),
		statusConverter:      mapper.NewEnumTypeConverter[onestopV1.WorkOrderStatus, workorder.Status](onestopV1.WorkOrderStatus_name, onestopV1.WorkOrderStatus_value),
		priorityConverter:    mapper.NewEnumTypeConverter[onestopV1.ServicePriority, workorder.Priority](onestopV1.ServicePriority_name, onestopV1.ServicePriority_value),
		temperatureConverter: mapper.NewEnumTypeConverter[onestopV1.TemperatureRequirement, workorder.TemperatureRequirement](onestopV1.TemperatureRequirement_name, onestopV1.TemperatureRequirement_value),
		itemScopeConverter:   mapper.NewEnumTypeConverter[onestopV1.WorkOrderItemScope, workorderitem.Scope](onestopV1.WorkOrderItemScope_name, onestopV1.WorkOrderItemScope_value),
		itemResultConverter:  mapper.NewEnumTypeConverter[onestopV1.WorkOrderItemResult, workorderitem.Result](onestopV1.WorkOrderItemResult_name, onestopV1.WorkOrderItemResult_value),
		eventActionConverter: mapper.NewEnumTypeConverter[onestopV1.WorkOrderEventAction, workorderevent.Action](onestopV1.WorkOrderEventAction_name, onestopV1.WorkOrderEventAction_value),
		eventFromConverter:   mapper.NewEnumTypeConverter[onestopV1.WorkOrderStatus, workorderevent.StatusFrom](onestopV1.WorkOrderStatus_name, onestopV1.WorkOrderStatus_value),
		eventToConverter:     mapper.NewEnumTypeConverter[onestopV1.WorkOrderStatus, workorderevent.StatusTo](onestopV1.WorkOrderStatus_name, onestopV1.WorkOrderStatus_value),
	}

	repo.init()

	return repo
}

func (r *WorkOrderRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.kindConverter.NewConverterPair())
	r.mapper.AppendConverters(r.sourceConverter.NewConverterPair())
	r.mapper.AppendConverters(r.channelConverter.NewConverterPair())
	r.mapper.AppendConverters(r.statusConverter.NewConverterPair())
	r.mapper.AppendConverters(r.priorityConverter.NewConverterPair())
	r.mapper.AppendConverters(r.temperatureConverter.NewConverterPair())
	// jsonb 结构列：proto repeated message ↔ jsonb 结构数组
	r.mapper.AppendConverters(copierutil.NewGenericTypeConverterPair(
		[]*onestopV1.WorkOrderMaterial{},
		[]schema.WorkOrderMaterial{},
		func(items []*onestopV1.WorkOrderMaterial) []schema.WorkOrderMaterial {
			return MaterialsToEntity(items)
		},
		func(items []schema.WorkOrderMaterial) []*onestopV1.WorkOrderMaterial {
			return materialsToDTO(items)
		},
	))
	r.mapper.AppendConverters(copierutil.NewGenericTypeConverterPair(
		[]*onestopV1.WorkOrderAcceptanceResult{},
		[]schema.WorkOrderAcceptanceResultItem{},
		func(items []*onestopV1.WorkOrderAcceptanceResult) []schema.WorkOrderAcceptanceResultItem {
			return AcceptanceResultsToEntity(items)
		},
		func(items []schema.WorkOrderAcceptanceResultItem) []*onestopV1.WorkOrderAcceptanceResult {
			return acceptanceResultsToDTO(items)
		},
	))
	// 验收项清单（与类型表同结构）
	r.mapper.AppendConverters(copierutil.NewGenericTypeConverterPair(
		[]*onestopV1.ServiceAcceptanceItem{},
		[]schema.ServiceAcceptanceItem{},
		func(items []*onestopV1.ServiceAcceptanceItem) []schema.ServiceAcceptanceItem {
			return acceptanceItemsToEntity(items)
		},
		func(items []schema.ServiceAcceptanceItem) []*onestopV1.ServiceAcceptanceItem {
			return acceptanceItemsToDTO(items)
		},
	))
}

// MaterialsToEntity / materialsToDTO 领用记录 proto ↔ jsonb 互转。
func MaterialsToEntity(items []*onestopV1.WorkOrderMaterial) []schema.WorkOrderMaterial {
	if len(items) == 0 {
		return nil
	}
	out := make([]schema.WorkOrderMaterial, 0, len(items))
	for _, item := range items {
		if item == nil || item.GetName() == "" {
			continue
		}
		out = append(out, schema.WorkOrderMaterial{
			Name:   item.GetName(),
			Qty:    item.GetQty(),
			Unit:   item.GetUnit(),
			Remark: item.GetRemark(),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func materialsToDTO(items []schema.WorkOrderMaterial) []*onestopV1.WorkOrderMaterial {
	if len(items) == 0 {
		return nil
	}
	out := make([]*onestopV1.WorkOrderMaterial, 0, len(items))
	for _, item := range items {
		out = append(out, &onestopV1.WorkOrderMaterial{
			Name:   item.Name,
			Qty:    item.Qty,
			Unit:   item.Unit,
			Remark: item.Remark,
		})
	}
	return out
}

// AcceptanceResultsToEntity / acceptanceResultsToDTO 验收项结果 proto ↔ jsonb 互转。
func AcceptanceResultsToEntity(items []*onestopV1.WorkOrderAcceptanceResult) []schema.WorkOrderAcceptanceResultItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]schema.WorkOrderAcceptanceResultItem, 0, len(items))
	for _, item := range items {
		if item == nil || item.GetName() == "" {
			continue
		}
		out = append(out, schema.WorkOrderAcceptanceResultItem{
			Name:   item.GetName(),
			Passed: item.GetPassed(),
			Remark: item.GetRemark(),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func acceptanceResultsToDTO(items []schema.WorkOrderAcceptanceResultItem) []*onestopV1.WorkOrderAcceptanceResult {
	if len(items) == 0 {
		return nil
	}
	out := make([]*onestopV1.WorkOrderAcceptanceResult, 0, len(items))
	for _, item := range items {
		out = append(out, &onestopV1.WorkOrderAcceptanceResult{
			Name:   item.Name,
			Passed: item.Passed,
			Remark: item.Remark,
		})
	}
	return out
}

// DeriveOrderNo 工单号 = WO- + 8 位零填自增 id（单调性与全局序列等价，
// 不引入 ent 之外的结构对象——漂移守卫是严格 diff，独立业务序列会永远红）。
func DeriveOrderNo(id uint32) string {
	return fmt.Sprintf("WO-%08d", id)
}

func (r *WorkOrderRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.entClient.Client().WorkOrder.Query()
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query work order count failed: %s", err.Error())
		return 0, onestopV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

func (r *WorkOrderRepo) List(ctx context.Context, req *paginationV1.PagingRequest) (*onestopV1.ListWorkOrdersResponse, error) {
	if req == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().WorkOrder.Query()

	// 默认按创建序倒序（最新单在前）；调用方显式排序时尊重显式值
	if len(req.GetSorting()) == 0 && (req.GetOrderBy() == "" || req.GetOrderBy() == "[]") {
		builder.Order(ent.Desc(workorder.FieldCreatedAt))
	}

	var repository = entCrud.NewRepository[
		ent.WorkOrderQuery, ent.WorkOrderSelect,
		ent.WorkOrderCreate, ent.WorkOrderCreateBulk,
		ent.WorkOrderUpdate, ent.WorkOrderUpdateOne,
		ent.WorkOrderDelete,
		predicate.WorkOrder,
		onestopV1.WorkOrder, ent.WorkOrder,
	](r.mapper)

	ret, err := repository.ListWithPaging(ctx, builder, builder.Clone(), req)
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return &onestopV1.ListWorkOrdersResponse{Total: 0, Items: nil}, nil
	}

	return &onestopV1.ListWorkOrdersResponse{
		Total: ret.Total,
		Items: ret.Items,
	}, nil
}

func (r *WorkOrderRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.entClient.Client().WorkOrder.Query().
		Where(workorder.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query work order exist failed: %s", err.Error())
		return false, onestopV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *WorkOrderRepo) Get(ctx context.Context, req *onestopV1.GetWorkOrderRequest) (*onestopV1.WorkOrder, error) {
	if req == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.entClient.Client().WorkOrder.Query().
		Where(workorder.IDEQ(req.GetId())).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, onestopV1.ErrorNotFound("work order not found")
		}
		r.log.Errorf(ctx, "query work order failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("query work order failed")
	}

	return r.mapper.ToDTO(entity), nil
}

// GetByID 按主键取单条；不存在返回 ErrorNotFound。
func (r *WorkOrderRepo) GetByID(ctx context.Context, id uint32) (*onestopV1.WorkOrder, error) {
	entity, err := r.entClient.Client().WorkOrder.Query().
		Where(workorder.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, onestopV1.ErrorNotFound("work order not found")
		}
		r.log.Errorf(ctx, "query work order by id failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("query work order failed")
	}

	return r.mapper.ToDTO(entity), nil
}

// createOrderInput 建单统一入参：主表字段 + 首个事件（CREATE）+ 执行项
// （计划任务形态：计划点表展开或类别项目库快照）。
// 建单走单事务：插入主表 → 派生工单号 → 落事件与执行项，任一步失败整体回滚。
type createOrderInput struct {
	Data      *onestopV1.WorkOrder
	Event     *onestopV1.WorkOrderEvent
	Items     []*onestopV1.WorkOrderItem
	SkipOnDup bool   // 调度生单：幂等唯一索引冲突时静默跳过（该时刻已生成过）
	TenantID  uint32 // 调度生单显式租户（计划/路线归属；0 = 平台）
}

// errOrderDuplicated 调度幂等冲突的内部哨兵（调用方据此区分“跳过”与“失败”）。
var errOrderDuplicated = fmt.Errorf("work order instance already generated")

func (r *WorkOrderRepo) createOrder(ctx context.Context, in *createOrderInput) (*ent.WorkOrder, error) {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin work order create transaction failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback work order create transaction failed: %v", rollbackErr)
		}
	}()

	d := in.Data
	builder := tx.WorkOrder.Create().
		SetNillableServiceTypeID(d.ServiceTypeId).
		SetNillableServiceTypeCode(d.ServiceTypeCode).
		SetNillableServiceTypeName(d.ServiceTypeName).
		SetNillableOrderKind(r.kindConverter.ToEntity(d.OrderKind)).
		SetNillableSource(r.sourceConverter.ToEntity(d.Source)).
		SetNillableChannel(r.channelConverter.ToEntity(d.Channel)).
		SetNillableSourceOrderID(d.SourceOrderId).
		SetNillableRequesterName(d.RequesterName).
		SetNillableRequesterPhone(d.RequesterPhone).
		SetNillableRequesterUserID(d.RequesterUserId).
		SetNillableLocation(d.Location).
		SetNillableEquipmentDesc(d.EquipmentDesc).
		SetNillableDescription(d.Description).
		SetNillableCommonPhrase(d.CommonPhrase).
		SetNillableExecutionPost(d.ExecutionPost).
		SetNillableAssigneeID(d.AssigneeId).
		SetNillableAssigneeName(d.AssigneeName).
		SetNillablePlanID(d.PlanId).
		SetNillableRouteID(d.RouteId).
		SetNillablePriority(r.priorityConverter.ToEntity(d.Priority)).
		SetNillableResponseTimeLimit(d.ResponseTimeLimit).
		SetNillableCompletionTimeLimit(d.CompletionTimeLimit).
		SetNillableResponseDeadline(timeutil.TimestamppbToTime(d.ResponseDeadline)).
		SetNillableCompletionDeadline(timeutil.TimestamppbToTime(d.CompletionDeadline)).
		SetNillableStatus(r.statusConverter.ToEntity(d.Status)).
		SetNillableIntakeAt(timeutil.TimestamppbToTime(d.IntakeAt)).
		SetNillableAssignedAt(timeutil.TimestamppbToTime(d.AssignedAt)).
		SetNillablePlannedExecAt(timeutil.TimestamppbToTime(d.PlannedExecAt)).
		SetNillableOrigin(d.Origin).
		SetNillableDestination(d.Destination).
		SetNillableRoundTrip(d.RoundTrip).
		SetNillableEstDurationMinutes(d.EstDurationMinutes).
		SetNillableTemperatureRequirement(r.temperatureConverter.ToEntity(d.TemperatureRequirement)).
		SetNillableCargoDesc(d.CargoDesc).
		SetNillableCreatedBy(d.CreatedBy).
		SetCreatedAt(time.Now())

	// 调度生单显式按计划/路线归属租户落库（系统上下文 = 平台视图，
	// TenantPrivacy 尊重显式值）；手动建单不设，由 Privacy 按登录上下文强制写入
	if in.SkipOnDup {
		builder.SetTenantID(in.TenantID)
	}

	if len(d.GetImages()) > 0 {
		builder.SetImages(d.GetImages())
	}
	if len(d.GetViaPoints()) > 0 {
		builder.SetViaPoints(d.GetViaPoints())
	}
	if len(d.GetAcceptanceModes()) > 0 {
		builder.SetAcceptanceModes(d.GetAcceptanceModes())
	}
	if items := acceptanceItemsToEntity(d.GetAcceptanceItems()); len(items) > 0 {
		builder.SetAcceptanceItems(items)
	}

	entity, err := builder.Save(ctx)
	if err != nil {
		// 调度幂等：部分唯一索引 (plan_id/route_id, planned_exec_at) 冲突 =
		// 该时刻已生成过，静默跳过（重跑展开不重）。事务已被该错误中止，
		// 必须显式回滚释放连接——置 committed 跳过回滚会让 aborted 事务
		// 挂在连接上，调度器每分钟一批冲突就能耗尽连接池（本机实测教训）
		if in.SkipOnDup && ent.IsConstraintError(err) {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				r.log.Errorf(ctx, "rollback duplicated work order insert failed: %v", rollbackErr)
			}
			committed = true
			return nil, errOrderDuplicated
		}
		r.log.Errorf(ctx, "insert work order failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("insert work order failed")
	}

	// 工单号 = WO- + 零填 id（同事务派生，见 DeriveOrderNo）
	if err = tx.WorkOrder.UpdateOneID(entity.ID).
		SetOrderNo(DeriveOrderNo(entity.ID)).
		SetUpdatedAt(time.Now()).
		Exec(ctx); err != nil {
		r.log.Errorf(ctx, "derive work order no failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("derive work order no failed")
	}

	if in.Event != nil {
		// 创建事件的 status_to 补齐为落库后状态（时间线首条完整展示状态迁移）
		if in.Event.GetAction() == onestopV1.WorkOrderEventAction_ACTION_CREATE && in.Event.StatusTo == nil {
			if v := r.statusConverter.ToDTO(entity.Status); v != nil {
				in.Event.StatusTo = v.Enum()
			}
		}
		if err = r.insertEventTx(ctx, tx, entity.ID, in.Event); err != nil {
			return nil, err
		}
	}

	for i, item := range in.Items {
		if item == nil {
			continue
		}
		itemBuilder := tx.WorkOrderItem.Create().
			SetWorkOrderID(entity.ID).
			SetName(item.GetName()).
			SetSort(uint32(i)).
			SetCreatedAt(time.Now())
		if entity.TenantID != nil {
			itemBuilder.SetTenantID(*entity.TenantID)
		}
		if item.GetId() != 0 {
			itemBuilder.SetID(item.GetId())
		}
		if s := r.itemScopeConverter.ToEntity(item.Scope); s != nil {
			itemBuilder.SetScope(*s)
		}
		if err = itemBuilder.Exec(ctx); err != nil {
			r.log.Errorf(ctx, "insert work order item failed: %s", err.Error())
			return nil, onestopV1.ErrorInternalServerError("insert work order item failed")
		}
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit work order create transaction failed: %v", err)
		return nil, onestopV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true

	created, err := r.entClient.Client().WorkOrder.Query().
		Where(workorder.IDEQ(entity.ID)).
		Only(ctx)
	if err != nil {
		r.log.Errorf(ctx, "reload created work order failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("reload work order failed")
	}
	return created, nil
}

func (r *WorkOrderRepo) insertEventTx(ctx context.Context, tx *ent.Tx, orderID uint32, event *onestopV1.WorkOrderEvent) error {
	eb := tx.WorkOrderEvent.Create().
		SetWorkOrderID(orderID).
		SetAction(*r.eventActionConverter.ToEntity(event.Action)).
		SetCreatedAt(time.Now()).
		SetCreatedBy(event.GetCreatedBy())
	if event.StatusFrom != nil {
		eb.SetStatusFrom(*r.eventFromConverter.ToEntity(event.StatusFrom))
	}
	if event.StatusTo != nil {
		eb.SetStatusTo(*r.eventToConverter.ToEntity(event.StatusTo))
	}
	if event.GetNote() != "" {
		eb.SetNote(event.GetNote())
	}
	if len(event.GetPhotos()) > 0 {
		eb.SetPhotos(event.GetPhotos())
	}
	// 事件随主表租户（创建路径外的事件由调用方保证同单）
	if order, err := tx.WorkOrder.Get(ctx, orderID); err == nil && order.TenantID != nil {
		eb.SetTenantID(*order.TenantID)
	}
	if err := eb.Exec(ctx); err != nil {
		r.log.Errorf(ctx, "insert work order event failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("insert work order event failed")
	}
	return nil
}

// Create 手动建单（坐席代发起/转单/派生）：data 为服务层已解析快照的单据，
// items 为计划任务形态的执行项快照；返回完整单（含工单号）。
// 租户由 TenantID Privacy 按登录上下文强制写入（平台上下文显式值优先）。
func (r *WorkOrderRepo) Create(ctx context.Context, data *onestopV1.WorkOrder, items []*onestopV1.WorkOrderItem) (*onestopV1.WorkOrder, error) {
	if data == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}
	event := &onestopV1.WorkOrderEvent{Action: onestopV1.WorkOrderEventAction_ACTION_CREATE.Enum(), CreatedBy: data.CreatedBy}
	entity, err := r.createOrder(ctx, &createOrderInput{Data: data, Event: event, Items: items})
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDTO(entity), nil
}

// CreateScheduled 调度生单（计划/班次展开）：幂等冲突时返回 (nil, nil)。
// 调度器以系统上下文调用（平台视图放行），tenantID 为计划/路线的租户，
// 显式落库（平台上下文下 TenantPrivacy 尊重显式值）。
func (r *WorkOrderRepo) CreateScheduled(ctx context.Context, order *onestopV1.WorkOrder, tenantID uint32, items []*onestopV1.WorkOrderItem) (*onestopV1.WorkOrder, error) {
	order = proto.Clone(order).(*onestopV1.WorkOrder)
	entity, err := r.createOrder(ctx, &createOrderInput{
		Data:      order,
		Event:     &onestopV1.WorkOrderEvent{Action: onestopV1.WorkOrderEventAction_ACTION_CREATE.Enum()},
		Items:     items,
		SkipOnDup: true,
		TenantID:  tenantID,
	})
	if err != nil {
		if err == errOrderDuplicated {
			return nil, nil
		}
		return nil, err
	}
	return r.mapper.ToDTO(entity), nil
}

// Transition 状态迁移 + 事件落库（单事务）：WHERE id AND status IN(from)，
// 命中 0 行 = 状态已被并发动作改变 → 409 回滚。setFunc 内做该动作的字段落库；
// event 可为 nil（纯字段更新场景）。事件随主表租户写入。
func (r *WorkOrderRepo) Transition(ctx context.Context, id uint32, from []workorder.Status, event *onestopV1.WorkOrderEvent, setFunc func(u *ent.WorkOrderUpdate)) error {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin work order action transaction failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback work order action transaction failed: %v", rollbackErr)
		}
	}()

	builder := tx.WorkOrder.Update().
		Where(workorder.IDEQ(id), workorder.StatusIn(from...))
	setFunc(builder)
	affected, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf(ctx, "transition work order %d failed: %s", id, err.Error())
		return onestopV1.ErrorInternalServerError("update work order failed")
	}
	if affected == 0 {
		// 未命中：并发动作已改状态。事务无写入但必须回滚释放连接（同上）
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback unmatched work order transition failed: %v", rollbackErr)
		}
		committed = true
		return onestopV1.ErrorConflict("work order status changed by a concurrent action, reload and retry")
	}

	if event != nil {
		if err = r.insertEventTx(ctx, tx, id, event); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit work order action transaction failed: %v", err)
		return onestopV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true
	return nil
}

// AddEvent 追加不改状态的事件（催办/异常上报/派生记录）。
func (r *WorkOrderRepo) AddEvent(ctx context.Context, orderID uint32, event *onestopV1.WorkOrderEvent) error {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin work order event transaction failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback work order event transaction failed: %v", rollbackErr)
		}
	}()
	if err = r.insertEventTx(ctx, tx, orderID, event); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit work order event transaction failed: %v", err)
		return onestopV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true
	return nil
}

// Patch 无状态变更的字段更新（催办计数/异常标记/回访补录等）：
// 不带状态守卫，但仍限定单据存在。
func (r *WorkOrderRepo) Patch(ctx context.Context, id uint32, setFunc func(u *ent.WorkOrderUpdateOne)) error {
	builder := r.entClient.Client().WorkOrder.UpdateOneID(id)
	setFunc(builder)
	if _, err := builder.Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return onestopV1.ErrorNotFound("work order not found")
		}
		r.log.Errorf(ctx, "patch work order %d failed: %s", id, err.Error())
		return onestopV1.ErrorInternalServerError("update work order failed")
	}
	return nil
}

// Update 发起信息修正（仅待派单/审批中，状态守卫由服务层校验后在此落地）。
func (r *WorkOrderRepo) Update(ctx context.Context, req *onestopV1.UpdateWorkOrderRequest) error {
	if req == nil || req.Data == nil {
		return onestopV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return onestopV1.ErrorBadRequest("id is required")
	}

	d := req.Data
	return r.Patch(ctx, req.GetId(), func(u *ent.WorkOrderUpdateOne) {
		u.SetUpdatedAt(time.Now())
		if d.Description != nil {
			u.SetNillableDescription(d.Description)
		}
		if d.Location != nil {
			u.SetNillableLocation(d.Location)
		}
		if d.EquipmentDesc != nil {
			u.SetNillableEquipmentDesc(d.EquipmentDesc)
		}
		if d.RequesterName != nil {
			u.SetNillableRequesterName(d.RequesterName)
		}
		if d.RequesterPhone != nil {
			u.SetNillableRequesterPhone(d.RequesterPhone)
		}
		if d.CommonPhrase != nil {
			u.SetNillableCommonPhrase(d.CommonPhrase)
		}
		if d.Priority != nil {
			u.SetNillablePriority(r.priorityConverter.ToEntity(d.Priority))
		}
		if d.Origin != nil {
			u.SetNillableOrigin(d.Origin)
		}
		if d.Destination != nil {
			u.SetNillableDestination(d.Destination)
		}
		if d.CargoDesc != nil {
			u.SetNillableCargoDesc(d.CargoDesc)
		}
		if len(d.GetImages()) > 0 {
			u.SetImages(d.GetImages())
		}
		if d.UpdatedBy != nil {
			u.SetNillableUpdatedBy(d.UpdatedBy)
		}
	})
}

// Delete 软删（管理清理）：主表 + 执行项 + 事件同事务标记删除。
func (r *WorkOrderRepo) Delete(ctx context.Context, req *onestopV1.DeleteWorkOrderRequest) error {
	if req == nil || req.GetId() == 0 {
		return onestopV1.ErrorBadRequest("invalid parameter")
	}

	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin work order delete transaction failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback work order delete transaction failed: %v", rollbackErr)
		}
	}()

	now := time.Now()
	if _, err = tx.WorkOrder.Update().
		Where(workorder.IDEQ(req.GetId())).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "delete work order failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("delete work order failed")
	}
	if _, err = tx.WorkOrderItem.Update().
		Where(workorderitem.WorkOrderIDEQ(req.GetId())).
		SetDeletedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "delete work order items failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("delete work order items failed")
	}
	if _, err = tx.WorkOrderEvent.Update().
		Where(workorderevent.WorkOrderIDEQ(req.GetId())).
		SetDeletedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "delete work order events failed: %s", err.Error())
		return onestopV1.ErrorInternalServerError("delete work order events failed")
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit work order delete transaction failed: %v", err)
		return onestopV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true
	return nil
}

// —— 事件与执行项子资源 ——

func (r *WorkOrderRepo) ListEvents(ctx context.Context, orderID uint32) ([]*onestopV1.WorkOrderEvent, error) {
	entities, err := r.entClient.Client().WorkOrderEvent.Query().
		Where(workorderevent.WorkOrderIDEQ(orderID)).
		Order(ent.Desc(workorderevent.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf(ctx, "list work order events failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("query work order events failed")
	}

	out := make([]*onestopV1.WorkOrderEvent, 0, len(entities))
	for _, e := range entities {
		dto := &onestopV1.WorkOrderEvent{
			Id:     &e.ID,
			Note:   e.Note,
			Photos: e.Photos,
		}
		if e.WorkOrderID != nil {
			dto.WorkOrderId = e.WorkOrderID
		}
		if e.Action != nil {
			if v := r.eventActionConverter.ToDTO(e.Action); v != nil {
				dto.Action = v.Enum()
			}
		}
		if e.StatusFrom != nil {
			if v := r.eventFromConverter.ToDTO(e.StatusFrom); v != nil {
				dto.StatusFrom = v.Enum()
			}
		}
		if e.StatusTo != nil {
			if v := r.eventToConverter.ToDTO(e.StatusTo); v != nil {
				dto.StatusTo = v.Enum()
			}
		}
		if e.CreatedAt != nil {
			dto.CreatedAt = timeutil.TimeToTimestamppb(e.CreatedAt)
		}
		out = append(out, dto)
	}
	return out, nil
}

func (r *WorkOrderRepo) ListItems(ctx context.Context, orderID uint32) ([]*onestopV1.WorkOrderItem, error) {
	entities, err := r.entClient.Client().WorkOrderItem.Query().
		Where(workorderitem.WorkOrderIDEQ(orderID), workorderitem.DeletedAtIsNil()).
		Order(ent.Asc(workorderitem.FieldSort), ent.Asc(workorderitem.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf(ctx, "list work order items failed: %s", err.Error())
		return nil, onestopV1.ErrorInternalServerError("query work order items failed")
	}

	out := make([]*onestopV1.WorkOrderItem, 0, len(entities))
	for _, it := range entities {
		dto := &onestopV1.WorkOrderItem{
			Id:     &it.ID,
			Sort:   &it.Sort,
			Photos: it.Photos,
		}
		if it.WorkOrderID != nil {
			dto.WorkOrderId = it.WorkOrderID
		}
		if it.Name != nil {
			dto.Name = it.Name
		}
		if it.Value != nil {
			dto.Value = it.Value
		}
		if it.Remark != nil {
			dto.Remark = it.Remark
		}
		if it.Scope != nil {
			if v := r.itemScopeConverter.ToDTO(it.Scope); v != nil {
				dto.Scope = v.Enum()
			}
		}
		if v := r.itemResultConverter.ToDTO(&it.Result); v != nil {
			dto.Result = v.Enum()
		}
		out = append(out, dto)
	}
	return out, nil
}

// SubmitItems 完工提交时逐项回填结果（限定本单范围内的执行项 id）。
func (r *WorkOrderRepo) SubmitItems(ctx context.Context, orderID uint32, submissions []*onestopV1.WorkOrderItemSubmission) error {
	for _, sub := range submissions {
		if sub == nil || sub.GetId() == 0 {
			continue
		}
		builder := r.entClient.Client().WorkOrderItem.Update().
			Where(workorderitem.IDEQ(sub.GetId()), workorderitem.WorkOrderIDEQ(orderID))
		if v := r.itemResultConverter.ToEntity(&sub.Result); v != nil {
			builder.SetResult(*v)
		}
		if sub.GetValue() != "" {
			builder.SetValue(sub.GetValue())
		}
		if sub.GetRemark() != "" {
			builder.SetRemark(sub.GetRemark())
		}
		if len(sub.GetPhotos()) > 0 {
			builder.SetPhotos(sub.GetPhotos())
		}
		if _, err := builder.Save(ctx); err != nil {
			r.log.Errorf(ctx, "submit work order item %d failed: %s", sub.GetId(), err.Error())
			return onestopV1.ErrorInternalServerError("update work order item failed")
		}
	}
	return nil
}

// CountPendingItems 未执行（ITEM_PENDING）执行项数：完工提交守卫。
func (r *WorkOrderRepo) CountPendingItems(ctx context.Context, orderID uint32) (int, error) {
	count, err := r.entClient.Client().WorkOrderItem.Query().
		Where(
			workorderitem.WorkOrderIDEQ(orderID),
			workorderitem.DeletedAtIsNil(),
			workorderitem.ResultEQ(workorderitem.ResultPending),
		).
		Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "count pending work order items failed: %s", err.Error())
		return 0, onestopV1.ErrorInternalServerError("query count failed")
	}
	return count, nil
}

// HasAbnormalItem 是否存在异常执行项（完工提交后联动主表异常标记）。
func (r *WorkOrderRepo) HasAbnormalItem(ctx context.Context, orderID uint32) (bool, error) {
	exist, err := r.entClient.Client().WorkOrderItem.Query().
		Where(
			workorderitem.WorkOrderIDEQ(orderID),
			workorderitem.DeletedAtIsNil(),
			workorderitem.ResultEQ(workorderitem.ResultAbnormal),
		).
		Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query abnormal work order items failed: %s", err.Error())
		return false, onestopV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}
