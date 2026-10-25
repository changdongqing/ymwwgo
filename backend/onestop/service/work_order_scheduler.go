package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tx7do/go-crud/viewer"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/timestamppb"

	onestopV1 "go-wind-admin/onestop/api/gen/go/onestop/service/v1"

	"go-wind-admin/onestop/data/ent"
)

// 计划/班次 → 工单展开调度器。
//
// 挂载方式：宿主 asynq 任务系统的系统级常驻任务（tenant_expiry_scan 同款）——
// 任务类型常量在本包定义、宿主 asynq_server.go 注册 handler、startAllTask 末尾
// 注册 cron；不进 sys_tasks 表。cfg.Server.Asynq 未配置时任务子系统整体不启动：
// 计划不自动生单，手动链路不受影响（部署文档写明该依赖）。
//
// 展开窗口 = 当日 + 次日两个日历日全量：正常运行时每张未来单至少提前一天生成，
// 停机恢复当日自动补齐（含已过时刻——迟到单红显催办，好过漏单）；跨日漏单
// 不追补（避免恢复风暴）。幂等靠部分唯一索引 (plan_id/route_id, planned_exec_at)：
// 重复展开 INSERT 冲突即跳过，重跑不重。
//
// 频率口径与 P1 设计 §13 逐条对齐：季度 = FREQ_YEARLY months 多值、
// 半月 = FREQ_MONTHLY month_days=[1,15]、clamp_to_month_end 月底顺延/跳过、
// excluded_dates 全天不生成、生效区间含当日。不解析细类名档位（计划是唯一
// 排期事实源，P1 原则 3）。

// OnestopPlanExpandTaskType 计划展开任务的类型常量（宿主接线引用）。
const OnestopPlanExpandTaskType = "onestop_plan_expand"

// OnestopPlanExpandCronSpec 计划展开的系统级 cron（每分钟：滚动窗口 + 停机补单）。
const OnestopPlanExpandCronSpec = "* * * * *"

// OnestopPlanExpandTaskData 计划展开任务载荷（当前无参数）。
type OnestopPlanExpandTaskData struct{}

// AsyncPlanExpand asynq handler：系统上下文展开窗口内的计划/班次实例。
// 单条计划展开失败：日志带原始错误后继续下一条（单条失败不拖垮整轮），
// 整轮结果留痕。
func (s *WorkOrderService) AsyncPlanExpand(taskType string, taskData *OnestopPlanExpandTaskData) error {
	ctx := viewer.WithSystemContext(context.Background())
	s.log.Infof(ctx, "AsyncPlanExpand [%s] started", taskType)

	created, skipped, failed, err := s.RunPlanExpansion(ctx)
	if err != nil {
		s.log.Errorf(ctx, "AsyncPlanExpand [%s] failed: %v", taskType, err)
		return err
	}
	s.log.Infof(ctx, "AsyncPlanExpand [%s] finished: created=%d skipped=%d failed=%d",
		taskType, created, skipped, failed)
	return nil
}

// RunPlanExpansion 展开一轮：计划实例 + 路线班次实例（幂等，重复执行不重）。
func (s *WorkOrderService) RunPlanExpansion(ctx context.Context) (created, skipped, failed int, err error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dates := []time.Time{today, today.AddDate(0, 0, 1)}

	// —— 计划展开（TASK / 计划型 TRANSPORT）——
	plans, err := s.planRepo.ListEnabledEntities(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, plan := range plans {
		instances := expandPlanInstances(plan, dates)
		if len(instances) == 0 {
			continue
		}
		for _, plannedAt := range instances {
			c, serr := s.expandPlanInstance(ctx, plan, plannedAt)
			if serr != nil {
				s.log.Errorf(ctx, "expand plan %q instance %s failed: %v",
					derefStr(plan.Name), plannedAt.Format(time.RFC3339), serr)
				failed++
				continue
			}
			if c != nil {
				created++
			} else {
				skipped++
			}
		}
	}

	// —— 路线班次展开（TRANSPORT：departure_times 非空才参与）——
	routes, err := s.routeRepo.ListTimetableEntities(ctx)
	if err != nil {
		return created, skipped, failed, err
	}
	for _, route := range routes {
		for _, date := range dates {
			for _, dep := range route.DepartureTimes {
				hhmm, perr := parseHHMM(dep)
				if perr != nil {
					s.log.Errorf(ctx, "expand route %q departure time %q invalid: %v",
						derefStr(route.Name), dep, perr)
					failed++
					continue
				}
				plannedAt := date.Add(hhmm)
				c, serr := s.expandRouteInstance(ctx, route, plannedAt)
				if serr != nil {
					s.log.Errorf(ctx, "expand route %q instance %s failed: %v",
						derefStr(route.Name), plannedAt.Format(time.RFC3339), serr)
					failed++
					continue
				}
				if c != nil {
					created++
				} else {
					skipped++
				}
			}
		}
	}

	return created, skipped, failed, nil
}

// expandPlanInstance 生成计划单：status=DISPATCHED（生成即派岗），
// 岗位 = 计划级 post 优先，回退类别 effective 岗位；点表展开为执行项。
func (s *WorkOrderService) expandPlanInstance(ctx context.Context, plan *ent.PeriodicPlan, plannedAt time.Time) (*onestopV1.WorkOrder, error) {
	if plan.ServiceTypeID == nil {
		return nil, fmt.Errorf("plan %q has no service type", derefStr(plan.Name))
	}
	typeEntity, err := s.typeRepo.GetByID(ctx, *plan.ServiceTypeID)
	if err != nil {
		return nil, err
	}
	eff, err := ResolveEffectiveConfig(ctx, s.typeRepo, typeEntity)
	if err != nil {
		return nil, err
	}
	kind, err := ResolveOrderKind(ctx, s.typeRepo, typeEntity)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	order := &onestopV1.WorkOrder{
		ServiceTypeId:          trans.Ptr(*plan.ServiceTypeID),
		ServiceTypeCode:        typeEntity.Code,
		ServiceTypeName:        typeEntity.Name,
		OrderKind:              kind.Enum(),
		Source:                 onestopV1.WorkOrderSource_SOURCE_PLAN.Enum(),
		Status:                 onestopV1.WorkOrderStatus_STATUS_DISPATCHED.Enum(),
		Description:            plan.Name,
		ExecutionPost:          eff.ExecutionPost,
		Priority:               eff.DefaultPriority,
		AcceptanceModes:        eff.GetAcceptanceModes(),
		AcceptanceItems:        eff.GetAcceptanceItems(),
		TemperatureRequirement: eff.TemperatureRequirement,
		PlanId:                 trans.Ptr(plan.ID),
		IntakeAt:               timestamppb.New(now),
		AssignedAt:             timestamppb.New(now),
		PlannedExecAt:          timestamppb.New(plannedAt),
	}
	if plan.Post != nil && *plan.Post != "" {
		order.ExecutionPost = plan.Post
	}
	if eff.GetResponseTimeLimit() > 0 {
		order.ResponseTimeLimit = eff.ResponseTimeLimit
		order.ResponseDeadline = timestamppb.New(plannedAt.Add(time.Duration(eff.GetResponseTimeLimit()) * time.Minute))
	}
	if eff.GetCompletionTimeLimit() > 0 {
		order.CompletionTimeLimit = eff.CompletionTimeLimit
		order.CompletionDeadline = timestamppb.New(plannedAt.Add(time.Duration(eff.GetCompletionTimeLimit()) * time.Minute))
	}

	// 点表展开为执行项（停用项保留在计划里但不执行）；计划型运送单（如被服
	// 收发巡回）无固定起终点，origin/destination 留空由执行时按点表走
	var items []*onestopV1.WorkOrderItem
	for _, item := range plan.SpaceItems {
		if !item.Enabled {
			continue
		}
		items = append(items, &onestopV1.WorkOrderItem{
			Name:  trans.Ptr(item.Name),
			Scope: onestopV1.WorkOrderItemScope_SCOPE_SPACE.Enum(),
		})
	}
	for _, item := range plan.EquipmentItems {
		if !item.Enabled {
			continue
		}
		items = append(items, &onestopV1.WorkOrderItem{
			Name:  trans.Ptr(item.Name),
			Scope: onestopV1.WorkOrderItemScope_SCOPE_EQUIPMENT.Enum(),
		})
	}

	tenantID := uint32(0)
	if plan.TenantID != nil {
		tenantID = *plan.TenantID
	}
	return s.repo.CreateScheduled(ctx, order, tenantID, items)
}

// expandRouteInstance 生成班次运送单：路线参数全量快照，送达走双签收链路。
func (s *WorkOrderService) expandRouteInstance(ctx context.Context, route *ent.TransportRoute, plannedAt time.Time) (*onestopV1.WorkOrder, error) {
	if route.ServiceTypeID == nil {
		return nil, fmt.Errorf("route %q has no service type", derefStr(route.Name))
	}
	typeEntity, err := s.typeRepo.GetByID(ctx, *route.ServiceTypeID)
	if err != nil {
		return nil, err
	}
	eff, err := ResolveEffectiveConfig(ctx, s.typeRepo, typeEntity)
	if err != nil {
		return nil, err
	}
	kind, err := ResolveOrderKind(ctx, s.typeRepo, typeEntity)
	if err != nil {
		return nil, err
	}
	if kind != onestopV1.OrderKind_KIND_TRANSPORT {
		// 路线只挂 0106 子树（挂载门控），形态推导必然为运送；防御性兜底
		return nil, fmt.Errorf("route %q attaches to non-transport type %q", derefStr(route.Name), typeEntity.GetCode())
	}

	now := time.Now()
	order := &onestopV1.WorkOrder{
		ServiceTypeId:          trans.Ptr(*route.ServiceTypeID),
		ServiceTypeCode:        typeEntity.Code,
		ServiceTypeName:        typeEntity.Name,
		OrderKind:              kind.Enum(),
		Source:                 onestopV1.WorkOrderSource_SOURCE_ROUTE.Enum(),
		Status:                 onestopV1.WorkOrderStatus_STATUS_DISPATCHED.Enum(),
		Description:            trans.Ptr(derefStr(route.Name) + " 定时班次"),
		ExecutionPost:          eff.ExecutionPost,
		Priority:               eff.DefaultPriority,
		AcceptanceModes:        eff.GetAcceptanceModes(),
		AcceptanceItems:        eff.GetAcceptanceItems(),
		TemperatureRequirement: eff.TemperatureRequirement,
		RouteId:                trans.Ptr(route.ID),
		Origin:                 route.Origin,
		Destination:            route.Destination,
		ViaPoints:              route.ViaPoints,
		RoundTrip:              trans.Ptr(route.RoundTrip),
		IntakeAt:               timestamppb.New(now),
		AssignedAt:             timestamppb.New(now),
		PlannedExecAt:          timestamppb.New(plannedAt),
	}
	if route.EstDurationMinutes != nil {
		order.EstDurationMinutes = route.EstDurationMinutes
	}
	if eff.GetResponseTimeLimit() > 0 {
		order.ResponseTimeLimit = eff.ResponseTimeLimit
		order.ResponseDeadline = timestamppb.New(plannedAt.Add(time.Duration(eff.GetResponseTimeLimit()) * time.Minute))
	}
	if eff.GetCompletionTimeLimit() > 0 {
		order.CompletionTimeLimit = eff.CompletionTimeLimit
		order.CompletionDeadline = timestamppb.New(plannedAt.Add(time.Duration(eff.GetCompletionTimeLimit()) * time.Minute))
	}

	tenantID := uint32(0)
	if route.TenantID != nil {
		tenantID = *route.TenantID
	}
	return s.repo.CreateScheduled(ctx, order, tenantID, nil)
}

// —— 展开纯函数（单测钉死 P1 §13 口径）——

// expandPlanInstances 计划在给定日历日内的全部执行时刻（dates 为 00:00 起点
// 的日历日列表；返回值含具体时分）。
func expandPlanInstances(plan *ent.PeriodicPlan, dates []time.Time) []time.Time {
	if plan == nil || plan.FreqType == nil {
		return nil
	}
	var out []time.Time
	for _, date := range dates {
		if !planCoversDate(plan, date) {
			continue
		}
		if !isExecutionDay(plan, date) {
			continue
		}
		for _, tm := range plan.Times {
			d, err := parseHHMM(tm)
			if err != nil {
				continue
			}
			out = append(out, date.Add(d))
		}
	}
	return out
}

// planCoversDate 生效区间判定（含当日；时间为空表示不设限）。
func planCoversDate(plan *ent.PeriodicPlan, date time.Time) bool {
	dayStart := date
	if plan.EffectiveFrom != nil {
		from := *plan.EffectiveFrom
		fromDay := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
		if dayStart.Before(fromDay) {
			return false
		}
	}
	if plan.EffectiveTo != nil {
		to := *plan.EffectiveTo
		toDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
		if dayStart.After(toDay) {
			return false
		}
	}
	return true
}

// isExecutionDay 频率执行日判定：每日恒真；每周按 weekdays；每月/每年按
// month_days（-1=月末，clamp_to_month_end 月底顺延或跳过）× months（每年）。
func isExecutionDay(plan *ent.PeriodicPlan, date time.Time) bool {
	for _, ex := range plan.ExcludedDates {
		if strings.TrimSpace(ex) == "" {
			continue
		}
		if ex == date.Format("2006-01-02") {
			return false
		}
	}

	switch *plan.FreqType {
	case "FREQ_DAILY":
		return true
	case "FREQ_WEEKLY":
		// weekdays 语义：1=周一…7=周日（ISO）
		wd := int(date.Weekday())
		if wd == 0 {
			wd = 7
		}
		for _, v := range plan.Weekdays {
			if int(v) == wd {
				return true
			}
		}
		return false
	case "FREQ_MONTHLY", "FREQ_YEARLY":
		if *plan.FreqType == "FREQ_YEARLY" {
			monthHit := false
			for _, m := range plan.Months {
				if int(m) == int(date.Month()) {
					monthHit = true
					break
				}
			}
			if !monthHit {
				return false
			}
		}
		lastDay := time.Date(date.Year(), date.Month()+1, 0, 0, 0, 0, 0, date.Location()).Day()
		clamp := plan.ClampToMonthEnd
		for _, md := range plan.MonthDays {
			if md == -1 {
				if date.Day() == lastDay {
					return true
				}
				continue
			}
			day := int(md)
			if day > lastDay {
				if clamp {
					// 号数超出当月天数：顺延至当月最后一天执行
					if date.Day() == lastDay {
						return true
					}
				}
				// 不顺延则当月跳过
				continue
			}
			if date.Day() == day {
				return true
			}
		}
		return false
	}
	return false
}

// derefStr 解引用可空字符串（空安全）。
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseHHMM "HH:mm" → 当日时长偏移。
func parseHHMM(s string) (time.Duration, error) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("invalid HH:mm time %q: %w", s, err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid HH:mm time %q", s)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}
