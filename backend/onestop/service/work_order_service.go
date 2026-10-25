package service

import (
	"context"
	"strings"
	"time"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	"github.com/tx7do/go-utils/trans"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	onestopV1 "go-wind-admin/onestop/api/gen/go/onestop/service/v1"

	"go-wind-admin/onestop/data"
	"go-wind-admin/onestop/data/ent"
	"go-wind-admin/onestop/data/ent/workorder"
)

// 统一工单底座服务：三形态（事件/计划任务/运送）共用一套状态机与动作 RPC，
// 差异由 order_kind 驱动（形态约束 + 专项字段）。快照原则：创建时从类型
// effective-config 解析运行期判定字段落库，此后类型修改不影响存量单。
// 所有改状态的动作经 repo.Transition（状态守卫 + 同事务事件）落库，
// 不改状态的动作（催办/异常/派生）走 Patch + AddEvent。

// isTerminalStatus 终态：已完结/已撤销。
func isTerminalStatus(status onestopV1.WorkOrderStatus) bool {
	return status == onestopV1.WorkOrderStatus_STATUS_CLOSED ||
		status == onestopV1.WorkOrderStatus_STATUS_CANCELLED
}

type WorkOrderService struct {
	onestopV1.OneStopWorkOrderServiceHTTPServer

	log *bLogger.Helper

	repo      *data.WorkOrderRepo
	typeRepo  *data.ServiceTypeRepo
	planRepo  *data.PeriodicPlanRepo
	routeRepo *data.TransportRouteRepo
	operator  OperatorReader
}

func NewWorkOrderService(
	ctx *bootstrap.Context,
	repo *data.WorkOrderRepo,
	typeRepo *data.ServiceTypeRepo,
	planRepo *data.PeriodicPlanRepo,
	routeRepo *data.TransportRouteRepo,
	operator OperatorReader,
) *WorkOrderService {
	return &WorkOrderService{
		log:       ctx.NewLoggerHelper("work-order/service/onestop"),
		repo:      repo,
		typeRepo:  typeRepo,
		planRepo:  planRepo,
		routeRepo: routeRepo,
		operator:  operator,
	}
}

// —— 建单快照管线 ——

// buildOrderDraft 按目标类型解析快照并组装草稿；手动建单/转单/派生共用。
// src 携带调用方的发起信息（描述/位置/照片/发起人/起终点等），配置类字段
//（岗位/优先级/时限/验收/温控/项目库）全部快照自 effective-config；
// plannedAt 非空时按计划执行时刻起算 SLA（调度生单路径）。
func (s *WorkOrderService) buildOrderDraft(ctx context.Context, toTypeID uint32, src *onestopV1.WorkOrder, plannedAt *time.Time, planPost string) (*onestopV1.WorkOrder, []*onestopV1.WorkOrderItem, error) {
	if toTypeID == 0 {
		return nil, nil, onestopV1.ErrorBadRequest("service type is required")
	}
	typeEntity, err := s.typeRepo.GetByID(ctx, toTypeID)
	if err != nil {
		return nil, nil, err
	}
	if len(typeEntity.GetCode()) != 8 {
		return nil, nil, onestopV1.ErrorUnprocessableEntity(
			"work order service type must be a detail level (8-digit code) service type")
	}
	if !typeEntity.GetIsEnabled() {
		return nil, nil, onestopV1.ErrorUnprocessableEntity("service type is disabled")
	}

	eff, err := ResolveEffectiveConfig(ctx, s.typeRepo, typeEntity)
	if err != nil {
		return nil, nil, err
	}
	kind := eff.GetOrderKind()

	now := time.Now()
	order := &onestopV1.WorkOrder{
		ServiceTypeId:     trans.Ptr(toTypeID),
		ServiceTypeCode:   typeEntity.Code,
		ServiceTypeName:   typeEntity.Name,
		OrderKind:         kind.Enum(),
		Priority:          eff.DefaultPriority,
		SourceOrderId:     src.SourceOrderId,
		RequesterName:     src.RequesterName,
		RequesterPhone:    src.RequesterPhone,
		RequesterUserId:   src.RequesterUserId,
		Location:          src.Location,
		EquipmentDesc:     src.EquipmentDesc,
		Description:       src.Description,
		CommonPhrase:      src.CommonPhrase,
		Images:            src.GetImages(),
		Origin:            src.Origin,
		Destination:       src.Destination,
		CargoDesc:         src.CargoDesc,
		AcceptanceModes:   eff.GetAcceptanceModes(),
		AcceptanceItems:   eff.GetAcceptanceItems(),
		Channel:           src.Channel,
		// 岗位快照：计划级岗位覆盖类别岗位（P1 计划 post 的既定口径）
		ExecutionPost: eff.ExecutionPost,
	}
	if planPost != "" {
		order.ExecutionPost = trans.Ptr(planPost)
	}

	// 优先级可被发起信息显式覆盖（急送呼叫单建单即特急等场景）
	if src.Priority != nil && src.GetPriority() != onestopV1.ServicePriority_PRIORITY_UNSPECIFIED {
		order.Priority = src.Priority
	}
	if eff.TemperatureRequirement != nil {
		order.TemperatureRequirement = eff.TemperatureRequirement
	}

	if strings.TrimSpace(order.GetDescription()) == "" {
		return nil, nil, onestopV1.ErrorBadRequest("description is required")
	}

	// 运送形态：起终点必填
	if kind == onestopV1.OrderKind_KIND_TRANSPORT {
		if strings.TrimSpace(order.GetOrigin()) == "" || strings.TrimSpace(order.GetDestination()) == "" {
			return nil, nil, onestopV1.ErrorUnprocessableEntity("transport work order requires origin and destination")
		}
	}

	// 计划任务形态（手动补单）：执行项 = 类别标准作业项目库快照（scope 空）
	var items []*onestopV1.WorkOrderItem
	if kind == onestopV1.OrderKind_KIND_TASK {
		for _, name := range eff.GetStandardItems() {
			if strings.TrimSpace(name) == "" {
				continue
			}
			items = append(items, &onestopV1.WorkOrderItem{Name: trans.Ptr(name)})
		}
	}

	// SLA 起算口径：手动单从受理（=建单）起算；计划/班次单从计划执行时刻起算
	order.IntakeAt = timestamppb.New(now)
	base := now
	if plannedAt != nil {
		base = *plannedAt
		order.PlannedExecAt = timestamppb.New(*plannedAt)
	}
	if eff.GetResponseTimeLimit() > 0 {
		order.ResponseTimeLimit = eff.ResponseTimeLimit
		order.ResponseDeadline = timestamppb.New(base.Add(time.Duration(eff.GetResponseTimeLimit()) * time.Minute))
	}
	if eff.GetCompletionTimeLimit() > 0 {
		order.CompletionTimeLimit = eff.CompletionTimeLimit
		order.CompletionDeadline = timestamppb.New(base.Add(time.Duration(eff.GetCompletionTimeLimit()) * time.Minute))
	}

	// 审批前置段：类型要求事前审批 → 建单先进审批中
	if eff.GetRequiresApproval() {
		order.Status = onestopV1.WorkOrderStatus_STATUS_APPROVING.Enum()
	} else {
		order.Status = onestopV1.WorkOrderStatus_STATUS_PENDING.Enum()
	}

	return order, items, nil
}

// —— CRUD ——

func (s *WorkOrderService) List(ctx context.Context, req *paginationV1.PagingRequest) (*onestopV1.ListWorkOrdersResponse, error) {
	return s.repo.List(ctx, req)
}

func (s *WorkOrderService) Get(ctx context.Context, req *onestopV1.GetWorkOrderRequest) (*onestopV1.WorkOrder, error) {
	return s.repo.Get(ctx, req)
}

func (s *WorkOrderService) ListEvents(ctx context.Context, req *onestopV1.ListWorkOrderEventsRequest) (*onestopV1.ListWorkOrderEventsResponse, error) {
	if req.GetId() == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}
	events, err := s.repo.ListEvents(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &onestopV1.ListWorkOrderEventsResponse{Items: events, Total: uint64(len(events))}, nil
}

func (s *WorkOrderService) ListItems(ctx context.Context, req *onestopV1.ListWorkOrderItemsRequest) (*onestopV1.ListWorkOrderItemsResponse, error) {
	if req.GetId() == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}
	items, err := s.repo.ListItems(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &onestopV1.ListWorkOrderItemsResponse{Items: items, Total: uint64(len(items))}, nil
}

// Create 手动建单（坐席代发起）：快照解析后落库，工单号由仓储派生。
func (s *WorkOrderService) Create(ctx context.Context, req *onestopV1.CreateWorkOrderRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	src := req.Data
	src.Source = onestopV1.WorkOrderSource_SOURCE_MANUAL.Enum()
	if src.Channel == nil {
		src.Channel = onestopV1.WorkOrderChannel_CHANNEL_DESK.Enum()
	}

	order, items, err := s.buildOrderDraft(ctx, src.GetServiceTypeId(), src, nil, "")
	if err != nil {
		return nil, err
	}
	order.Source = src.Source
	order.Channel = src.Channel
	order.CreatedBy = trans.Ptr(operator)

	if _, err = s.repo.Create(ctx, order, items); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Update 发起信息修正：仅待派单/审批中可改（已进入履约的单走动作接口）。
func (s *WorkOrderService) Update(ctx context.Context, req *onestopV1.UpdateWorkOrderRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}

	existing, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if existing.GetStatus() != onestopV1.WorkOrderStatus_STATUS_PENDING &&
		existing.GetStatus() != onestopV1.WorkOrderStatus_STATUS_APPROVING {
		return nil, onestopV1.ErrorUnprocessableEntity(
			"work order is in fulfillment, use action endpoints instead of update")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	req.Data.UpdatedBy = trans.Ptr(operator)

	if err = s.repo.Update(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Delete 管理清理（软删，同事务清理执行项与事件）。调度单软删后不占幂等坑：
// 窗口内时刻未过的单会被重新生成（防漏是有意行为，误删请走撤销）。
func (s *WorkOrderService) Delete(ctx context.Context, req *onestopV1.DeleteWorkOrderRequest) (*emptypb.Empty, error) {
	if err := s.repo.Delete(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// —— 动作（状态机矩阵 §5.3 的服务层判据）——

func (s *WorkOrderService) entStatuses(in ...onestopV1.WorkOrderStatus) []workorder.Status {
	out := make([]workorder.Status, 0, len(in))
	for _, v := range in {
		switch v {
		case onestopV1.WorkOrderStatus_STATUS_PENDING:
			out = append(out, workorder.StatusPending)
		case onestopV1.WorkOrderStatus_STATUS_APPROVING:
			out = append(out, workorder.StatusApproving)
		case onestopV1.WorkOrderStatus_STATUS_DISPATCHED:
			out = append(out, workorder.StatusDispatched)
		case onestopV1.WorkOrderStatus_STATUS_PROCESSING:
			out = append(out, workorder.StatusProcessing)
		case onestopV1.WorkOrderStatus_STATUS_COMPLETED:
			out = append(out, workorder.StatusCompleted)
		case onestopV1.WorkOrderStatus_STATUS_CLOSED:
			out = append(out, workorder.StatusClosed)
		case onestopV1.WorkOrderStatus_STATUS_CANCELLED:
			out = append(out, workorder.StatusCancelled)
		}
	}
	return out
}

// eventFor 构造动作事件（from/to + 附言/照片 + 操作人）。
func eventFor(action onestopV1.WorkOrderEventAction, from, to onestopV1.WorkOrderStatus, note string, photos []string, operator uint32) *onestopV1.WorkOrderEvent {
	e := &onestopV1.WorkOrderEvent{
		Action:    action.Enum(),
		StatusFrom: from.Enum(),
		StatusTo:  to.Enum(),
		Photos:    photos,
		CreatedBy: trans.Ptr(operator),
	}
	if note != "" {
		e.Note = trans.Ptr(note)
	}
	return e
}

func (s *WorkOrderService) loadOrder(ctx context.Context, id uint32) (*onestopV1.WorkOrder, error) {
	if id == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}
	return s.repo.GetByID(ctx, id)
}

// Dispatch 派单：PENDING → DISPATCHED（assignee 可空 = 岗位派单，接单后补）。
func (s *WorkOrderService) Dispatch(ctx context.Context, req *onestopV1.DispatchWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_PENDING {
		return nil, onestopV1.ErrorConflict("dispatch is only allowed on PENDING work orders")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	note := req.GetNote()
	if target := strings.Trim(req.GetPost()+"/"+req.GetAssigneeName(), "/"); target != "" {
		note = strings.TrimSpace("派至 " + target + " " + note)
	}

	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(onestopV1.WorkOrderStatus_STATUS_PENDING),
		eventFor(onestopV1.WorkOrderEventAction_ACTION_DISPATCH,
			onestopV1.WorkOrderStatus_STATUS_PENDING, onestopV1.WorkOrderStatus_STATUS_DISPATCHED, note, nil, operator),
		func(u *ent.WorkOrderUpdate) {
			u.SetStatus(workorder.StatusDispatched).
				SetAssignedAt(now).
				SetUpdatedAt(now)
			if req.GetPost() != "" {
				u.SetExecutionPost(req.GetPost())
			}
			if req.GetAssigneeId() != 0 {
				u.SetAssigneeID(req.GetAssigneeId())
			}
			if req.GetAssigneeName() != "" {
				u.SetAssigneeName(req.GetAssigneeName())
			}
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Approve 审批：通过 → PENDING（进入正常派单链）；驳回 → CANCELLED。
func (s *WorkOrderService) Approve(ctx context.Context, req *onestopV1.ApproveWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_APPROVING {
		return nil, onestopV1.ErrorConflict("approve is only allowed on APPROVING work orders")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if req.GetPassed() {
		err = s.repo.Transition(ctx, req.GetId(),
			s.entStatuses(onestopV1.WorkOrderStatus_STATUS_APPROVING),
			eventFor(onestopV1.WorkOrderEventAction_ACTION_APPROVE_PASS,
				onestopV1.WorkOrderStatus_STATUS_APPROVING, onestopV1.WorkOrderStatus_STATUS_PENDING, req.GetNote(), nil, operator),
			func(u *ent.WorkOrderUpdate) {
				u.SetStatus(workorder.StatusPending).SetUpdatedAt(now)
			})
	} else {
		reason := req.GetNote()
		if reason == "" {
			reason = "审批驳回"
		}
		err = s.repo.Transition(ctx, req.GetId(),
			s.entStatuses(onestopV1.WorkOrderStatus_STATUS_APPROVING),
			eventFor(onestopV1.WorkOrderEventAction_ACTION_APPROVE_REJECT,
				onestopV1.WorkOrderStatus_STATUS_APPROVING, onestopV1.WorkOrderStatus_STATUS_CANCELLED, reason, nil, operator),
			func(u *ent.WorkOrderUpdate) {
				u.SetStatus(workorder.StatusCancelled).
					SetCancelReason(reason).
					SetUpdatedAt(now)
			})
	}
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Arrive 到场（事件/任务形态）：DISPATCHED → PROCESSING，记录响应达成。
func (s *WorkOrderService) Arrive(ctx context.Context, req *onestopV1.ArriveWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if order.GetOrderKind() == onestopV1.OrderKind_KIND_TRANSPORT {
		return nil, onestopV1.ErrorConflict("transport work orders use pickup instead of arrive")
	}
	if order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_DISPATCHED {
		return nil, onestopV1.ErrorConflict("arrive is only allowed on DISPATCHED work orders")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(onestopV1.WorkOrderStatus_STATUS_DISPATCHED),
		eventFor(onestopV1.WorkOrderEventAction_ACTION_ARRIVE,
			onestopV1.WorkOrderStatus_STATUS_DISPATCHED, onestopV1.WorkOrderStatus_STATUS_PROCESSING, req.GetNote(), req.GetPhotos(), operator),
		func(u *ent.WorkOrderUpdate) {
			u.SetStatus(workorder.StatusProcessing).
				SetRespondedAt(now).
				SetUpdatedAt(now)
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Pickup 取件签收（运送形态）：DISPATCHED → PROCESSING，响应达成 = 取件时刻。
func (s *WorkOrderService) Pickup(ctx context.Context, req *onestopV1.PickupWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if order.GetOrderKind() != onestopV1.OrderKind_KIND_TRANSPORT {
		return nil, onestopV1.ErrorConflict("pickup is only allowed on TRANSPORT work orders")
	}
	if order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_DISPATCHED {
		return nil, onestopV1.ErrorConflict("pickup is only allowed on DISPATCHED work orders")
	}
	if strings.TrimSpace(req.GetSigner()) == "" {
		return nil, onestopV1.ErrorBadRequest("pickup signer is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(onestopV1.WorkOrderStatus_STATUS_DISPATCHED),
		eventFor(onestopV1.WorkOrderEventAction_ACTION_PICKUP,
			onestopV1.WorkOrderStatus_STATUS_DISPATCHED, onestopV1.WorkOrderStatus_STATUS_PROCESSING,
			"取件签收人："+req.GetSigner(), req.GetPhotos(), operator),
		func(u *ent.WorkOrderUpdate) {
			u.SetStatus(workorder.StatusProcessing).
				SetRespondedAt(now).
				SetPickedUpAt(now).
				SetPickedSigner(req.GetSigner()).
				SetUpdatedAt(now)
			if len(req.GetPhotos()) > 0 {
				u.SetPickedPhotos(req.GetPhotos())
			}
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Deliver 送达签收（运送形态）：PROCESSING → CLOSED——双签收内嵌验收，
// 送达即完结是运送形态唯一按形态收窄的转移。
func (s *WorkOrderService) Deliver(ctx context.Context, req *onestopV1.DeliverWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if order.GetOrderKind() != onestopV1.OrderKind_KIND_TRANSPORT {
		return nil, onestopV1.ErrorConflict("deliver is only allowed on TRANSPORT work orders")
	}
	if order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_PROCESSING {
		return nil, onestopV1.ErrorConflict("deliver is only allowed on PROCESSING work orders")
	}
	if strings.TrimSpace(req.GetSigner()) == "" {
		return nil, onestopV1.ErrorBadRequest("deliver signer is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(onestopV1.WorkOrderStatus_STATUS_PROCESSING),
		eventFor(onestopV1.WorkOrderEventAction_ACTION_DELIVER,
			onestopV1.WorkOrderStatus_STATUS_PROCESSING, onestopV1.WorkOrderStatus_STATUS_CLOSED,
			"送达签收人："+req.GetSigner(), req.GetPhotos(), operator),
		func(u *ent.WorkOrderUpdate) {
			u.SetStatus(workorder.StatusClosed).
				SetDeliveredAt(now).
				SetDeliveredSigner(req.GetSigner()).
				SetCompletedAt(now).
				SetVerifiedAt(now).
				SetUpdatedAt(now)
			if len(req.GetPhotos()) > 0 {
				u.SetDeliveredPhotos(req.GetPhotos())
			}
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Complete 完工提交（事件/任务形态）：PROCESSING → COMPLETED。
// 计划任务形态存在未执行项时 422（要么逐项给结果、要么显式跳过）；
// 执行项含异常时联动主表异常标记。
func (s *WorkOrderService) Complete(ctx context.Context, req *onestopV1.CompleteWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if order.GetOrderKind() == onestopV1.OrderKind_KIND_TRANSPORT {
		return nil, onestopV1.ErrorConflict("transport work orders are closed by deliver, not complete")
	}
	if order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_PROCESSING {
		return nil, onestopV1.ErrorConflict("complete is only allowed on PROCESSING work orders")
	}
	if strings.TrimSpace(req.GetResultDesc()) == "" {
		return nil, onestopV1.ErrorBadRequest("result description is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	// 执行项结果先落库，再校验完整性（含本次提交后的最终状态）
	if len(req.GetItems()) > 0 {
		if err = s.repo.SubmitItems(ctx, req.GetId(), req.GetItems()); err != nil {
			return nil, err
		}
	}
	if pending, perr := s.repo.CountPendingItems(ctx, req.GetId()); perr != nil {
		return nil, perr
	} else if pending > 0 {
		return nil, onestopV1.ErrorUnprocessableEntity(
			"%d execution item(s) still pending, submit a result or skip them explicitly", pending)
	}
	hasAbnormal, err := s.repo.HasAbnormalItem(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(onestopV1.WorkOrderStatus_STATUS_PROCESSING),
		eventFor(onestopV1.WorkOrderEventAction_ACTION_COMPLETE,
			onestopV1.WorkOrderStatus_STATUS_PROCESSING, onestopV1.WorkOrderStatus_STATUS_COMPLETED, req.GetResultDesc(), req.GetResultPhotos(), operator),
		func(u *ent.WorkOrderUpdate) {
			u.SetStatus(workorder.StatusCompleted).
				SetResultDesc(req.GetResultDesc()).
				SetCompletedAt(now).
				SetUpdatedAt(now)
			if req.ActualWorkHours != nil {
				u.SetNillableActualWorkHours(req.ActualWorkHours)
			}
			if hasAbnormal {
				u.SetHasAbnormality(true)
			}
			if len(req.GetResultPhotos()) > 0 {
				u.SetResultPhotos(req.GetResultPhotos())
			}
			if materials := req.GetUsedMaterials(); len(materials) > 0 {
				u.SetUsedMaterials(data.MaterialsToEntity(materials))
			}
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Verify 验收（事件/任务形态）：COMPLETED → CLOSED（通过）/
// COMPLETED → PROCESSING（驳回返工，deadline 不重算）。
func (s *WorkOrderService) Verify(ctx context.Context, req *onestopV1.VerifyWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if order.GetOrderKind() == onestopV1.OrderKind_KIND_TRANSPORT {
		return nil, onestopV1.ErrorConflict("transport work orders are closed by deliver, no separate verify")
	}
	if order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_COMPLETED {
		return nil, onestopV1.ErrorConflict("verify is only allowed on COMPLETED work orders")
	}
	if !req.GetPassed() && strings.TrimSpace(req.GetNote()) == "" {
		return nil, onestopV1.ErrorBadRequest("reject note is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if req.GetPassed() {
		err = s.repo.Transition(ctx, req.GetId(),
			s.entStatuses(onestopV1.WorkOrderStatus_STATUS_COMPLETED),
			eventFor(onestopV1.WorkOrderEventAction_ACTION_VERIFY_PASS,
				onestopV1.WorkOrderStatus_STATUS_COMPLETED, onestopV1.WorkOrderStatus_STATUS_CLOSED, req.GetNote(), req.GetPhotos(), operator),
			func(u *ent.WorkOrderUpdate) {
				u.SetStatus(workorder.StatusClosed).
					SetVerifiedAt(now).
					SetUpdatedAt(now)
				if req.GetSigner() != "" {
					u.SetAcceptanceSigner(req.GetSigner())
				}
				if len(req.GetPhotos()) > 0 {
					u.SetAcceptancePhotos(req.GetPhotos())
				}
				if results := req.GetItemsResult(); len(results) > 0 {
					u.SetAcceptanceResult(data.AcceptanceResultsToEntity(results))
				}
			})
	} else {
		err = s.repo.Transition(ctx, req.GetId(),
			s.entStatuses(onestopV1.WorkOrderStatus_STATUS_COMPLETED),
			eventFor(onestopV1.WorkOrderEventAction_ACTION_VERIFY_REJECT,
				onestopV1.WorkOrderStatus_STATUS_COMPLETED, onestopV1.WorkOrderStatus_STATUS_PROCESSING, req.GetNote(), req.GetPhotos(), operator),
			func(u *ent.WorkOrderUpdate) {
				// 返工：deadline 不重算（返工时间消耗是事实，不因打回而豁免）
				u.SetStatus(workorder.StatusProcessing).
					SetUpdatedAt(now).
					SetAcceptanceSigner(req.GetSigner())
			})
	}
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Cancel 撤销：非终态未完工可撤（已完工待验收的单走验收驳回）。
func (s *WorkOrderService) Cancel(ctx context.Context, req *onestopV1.CancelWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if isTerminalStatus(order.GetStatus()) || order.GetStatus() == onestopV1.WorkOrderStatus_STATUS_COMPLETED {
		return nil, onestopV1.ErrorConflict("cancel is not allowed on %s work orders", order.GetStatus().String())
	}
	if strings.TrimSpace(req.GetReason()) == "" {
		return nil, onestopV1.ErrorBadRequest("cancel reason is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(
			onestopV1.WorkOrderStatus_STATUS_PENDING,
			onestopV1.WorkOrderStatus_STATUS_APPROVING,
			onestopV1.WorkOrderStatus_STATUS_DISPATCHED,
			onestopV1.WorkOrderStatus_STATUS_PROCESSING,
		),
		eventFor(onestopV1.WorkOrderEventAction_ACTION_CANCEL,
			order.GetStatus(), onestopV1.WorkOrderStatus_STATUS_CANCELLED, req.GetReason(), nil, operator),
		func(u *ent.WorkOrderUpdate) {
			u.SetStatus(workorder.StatusCancelled).
				SetCancelReason(req.GetReason()).
				SetUpdatedAt(now)
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Urge 催办：不改状态，主表冗余计数 +1（明细在事件表）。
func (s *WorkOrderService) Urge(ctx context.Context, req *onestopV1.UrgeWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if isTerminalStatus(order.GetStatus()) {
		return nil, onestopV1.ErrorConflict("urge is not allowed on terminal work orders")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	if err = s.repo.Patch(ctx, req.GetId(), func(u *ent.WorkOrderUpdateOne) {
		u.AddUrgeCount(1).SetUpdatedAt(time.Now())
	}); err != nil {
		return nil, err
	}
	if err = s.repo.AddEvent(ctx, req.GetId(), eventFor(onestopV1.WorkOrderEventAction_ACTION_URGE,
		order.GetStatus(), order.GetStatus(), req.GetNote(), nil, operator)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Reassign 改派：已派单/执行中 → 已派单（同单换岗位/执行人，不换类型；
// 执行中改派视同撤回重派）。
func (s *WorkOrderService) Reassign(ctx context.Context, req *onestopV1.ReassignWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_DISPATCHED &&
		order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_PROCESSING {
		return nil, onestopV1.ErrorConflict("reassign is only allowed on DISPATCHED/PROCESSING work orders")
	}
	if req.GetPost() == "" && req.GetAssigneeId() == 0 && req.GetAssigneeName() == "" {
		return nil, onestopV1.ErrorBadRequest("reassign target (post or assignee) is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	note := req.GetNote()
	if target := strings.Trim(req.GetPost()+"/"+req.GetAssigneeName(), "/"); target != "" {
		note = strings.TrimSpace("改派至 " + target + " " + note)
	}

	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(onestopV1.WorkOrderStatus_STATUS_DISPATCHED, onestopV1.WorkOrderStatus_STATUS_PROCESSING),
		eventFor(onestopV1.WorkOrderEventAction_ACTION_REASSIGN,
			order.GetStatus(), onestopV1.WorkOrderStatus_STATUS_DISPATCHED, note, nil, operator),
		func(u *ent.WorkOrderUpdate) {
			u.SetStatus(workorder.StatusDispatched).SetUpdatedAt(now)
			if req.GetPost() != "" {
				u.SetExecutionPost(req.GetPost())
			}
			if req.GetAssigneeId() != 0 {
				u.SetAssigneeID(req.GetAssigneeId())
			}
			if req.GetAssigneeName() != "" {
				u.SetAssigneeName(req.GetAssigneeName())
			}
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Transfer 转单：旧单关闭（原因自动「转单至 WO-xxx」），按新类型开新单——
// 复制发起信息快照、重新解析 kind/SLA/验收，source_order_id 回链。
func (s *WorkOrderService) Transfer(ctx context.Context, req *onestopV1.TransferWorkOrderRequest) (*onestopV1.WorkOrder, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if isTerminalStatus(order.GetStatus()) {
		return nil, onestopV1.ErrorConflict("transfer is not allowed on terminal work orders")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	src := extractInitiator(order)
	src.SourceOrderId = trans.Ptr(req.GetId())
	src.Source = onestopV1.WorkOrderSource_SOURCE_TRANSFER.Enum()

	newOrder, _, err := s.buildOrderDraft(ctx, req.GetToTypeId(), src, nil, "")
	if err != nil {
		return nil, err
	}
	newOrder.Source = src.Source
	newOrder.SourceOrderId = src.SourceOrderId
	newOrder.CreatedBy = trans.Ptr(operator)

	created, err := s.repo.Create(ctx, newOrder, nil)
	if err != nil {
		return nil, err
	}

	// 旧单关闭 + 双侧事件（旧单关单、新单来源已在创建事件之外补转单附言）
	reason := "转单至 " + created.GetOrderNo()
	if req.GetNote() != "" {
		reason += "：" + req.GetNote()
	}
	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(order.GetStatus()),
		eventFor(onestopV1.WorkOrderEventAction_ACTION_TRANSFER,
			order.GetStatus(), onestopV1.WorkOrderStatus_STATUS_CANCELLED, reason, nil, operator),
		func(u *ent.WorkOrderUpdate) {
			u.SetStatus(workorder.StatusCancelled).
				SetCancelReason(reason).
				SetUpdatedAt(now)
		})
	if err != nil {
		return nil, err
	}

	return created, nil
}

// Derive 派生：旧单不动，按新类型关联生成新单（典型：巡检执行项异常 → 报修）。
func (s *WorkOrderService) Derive(ctx context.Context, req *onestopV1.DeriveWorkOrderRequest) (*onestopV1.WorkOrder, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetDescription()) == "" {
		return nil, onestopV1.ErrorBadRequest("description is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	src := extractInitiator(order)
	src.Description = trans.Ptr(req.GetDescription())
	src.SourceOrderId = trans.Ptr(req.GetId())
	src.Source = onestopV1.WorkOrderSource_SOURCE_DERIVED.Enum()

	newOrder, _, err := s.buildOrderDraft(ctx, req.GetToTypeId(), src, nil, "")
	if err != nil {
		return nil, err
	}
	newOrder.Source = src.Source
	newOrder.SourceOrderId = src.SourceOrderId
	newOrder.CreatedBy = trans.Ptr(operator)

	created, err := s.repo.Create(ctx, newOrder, nil)
	if err != nil {
		return nil, err
	}

	note := "派生 " + created.GetOrderNo()
	if req.GetNote() != "" {
		note += "：" + req.GetNote()
	}
	if err = s.repo.AddEvent(ctx, req.GetId(), eventFor(onestopV1.WorkOrderEventAction_ACTION_DERIVE,
		order.GetStatus(), order.GetStatus(), note, nil, operator)); err != nil {
		return nil, err
	}

	return created, nil
}

// Abnormal 异常上报：不改状态，置异常标记（可配合派生动作生成处置单）。
func (s *WorkOrderService) Abnormal(ctx context.Context, req *onestopV1.AbnormalWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if isTerminalStatus(order.GetStatus()) {
		return nil, onestopV1.ErrorConflict("abnormal report is not allowed on terminal work orders")
	}
	if strings.TrimSpace(req.GetDesc()) == "" {
		return nil, onestopV1.ErrorBadRequest("abnormal description is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	if err = s.repo.Patch(ctx, req.GetId(), func(u *ent.WorkOrderUpdateOne) {
		u.SetHasAbnormality(true).SetUpdatedAt(time.Now())
	}); err != nil {
		return nil, err
	}
	note := req.GetDesc()
	if req.GetNote() != "" {
		note += "：" + req.GetNote()
	}
	if err = s.repo.AddEvent(ctx, req.GetId(), eventFor(onestopV1.WorkOrderEventAction_ACTION_ABNORMAL,
		order.GetStatus(), order.GetStatus(), note, req.GetPhotos(), operator)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Satisfaction 满意度回访补录：仅已完结单，1-5 分。
func (s *WorkOrderService) Satisfaction(ctx context.Context, req *onestopV1.SatisfactionWorkOrderRequest) (*emptypb.Empty, error) {
	order, err := s.loadOrder(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if order.GetStatus() != onestopV1.WorkOrderStatus_STATUS_CLOSED {
		return nil, onestopV1.ErrorConflict("satisfaction is only allowed on CLOSED work orders")
	}
	if req.GetScore() < 1 || req.GetScore() > 5 {
		return nil, onestopV1.ErrorUnprocessableEntity("satisfaction score must be 1-5")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	if err = s.repo.Patch(ctx, req.GetId(), func(u *ent.WorkOrderUpdateOne) {
		u.SetSatisfactionScore(req.GetScore()).SetUpdatedAt(time.Now())
		if req.Comment != nil {
			u.SetNillableSatisfactionComment(req.Comment)
		}
		u.SetRevisitedAt(time.Now())
	}); err != nil {
		return nil, err
	}
	if err = s.repo.AddEvent(ctx, req.GetId(), eventFor(onestopV1.WorkOrderEventAction_ACTION_SATISFACTION,
		order.GetStatus(), order.GetStatus(),
		"满意度回访："+scoreStars(req.GetScore())+" "+req.GetComment(), nil, operator)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// extractInitiator 提取旧单的发起信息组字段（转单/派生复制用）。
func extractInitiator(order *onestopV1.WorkOrder) *onestopV1.WorkOrder {
	return &onestopV1.WorkOrder{
		Description:    order.Description,
		Location:       order.Location,
		EquipmentDesc:  order.EquipmentDesc,
		RequesterName:  order.RequesterName,
		RequesterPhone: order.RequesterPhone,
		RequesterUserId: order.RequesterUserId,
		Images:         order.GetImages(),
		CommonPhrase:   order.CommonPhrase,
		Origin:         order.Origin,
		Destination:    order.Destination,
		CargoDesc:      order.CargoDesc,
	}
}

func scoreStars(score uint32) string {
	stars := make([]rune, 0, 5)
	for i := uint32(0); i < score && i < 5; i++ {
		stars = append(stars, '★')
	}
	return string(stars)
}
