package service

import (
	"context"
	"regexp"
	"strings"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	"github.com/tx7do/go-utils/trans"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	onestopV1 "go-wind-admin/onestop/api/gen/go/onestop/service/v1"

	"go-wind-admin/onestop/data"
)

// 周期计划挂载门控：类别须为小类/细类，且解析后的任务触发方式为
// TRIGGER_SCHEDULED（细类未设置回退小类）——统一模板的「周期计划开关」。
// 解析逻辑见 service_type_service.go 的 ResolveTriggerMode。

var (
	timePattern     = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	excludedPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

type PeriodicPlanService struct {
	onestopV1.OneStopPeriodicPlanServiceHTTPServer

	log *bLogger.Helper

	repo     *data.PeriodicPlanRepo
	typeRepo *data.ServiceTypeRepo
	operator OperatorReader
}

func NewPeriodicPlanService(ctx *bootstrap.Context, repo *data.PeriodicPlanRepo, typeRepo *data.ServiceTypeRepo, operator OperatorReader) *PeriodicPlanService {
	s := &PeriodicPlanService{
		log:      ctx.NewLoggerHelper("periodic-plan/service/onestop"),
		repo:     repo,
		typeRepo: typeRepo,
		operator: operator,
	}

	// 空表时播种内置周期计划模板（count == 0 守卫）。播种失败不阻断启动，
	// 错误必须带原始对象落日志。依赖 wiring 中 ServiceTypeService 先行构造
	//（其构造函数播种类别字典），此处按编码解析类别才能命中。
	if err := repo.SeedDefaults(ctx.Context()); err != nil {
		s.log.Errorf(ctx.Context(), "seed built-in periodic plans failed: %v", err)
	}

	return s
}

// validate 周期计划业务校验：类别归属、频率规则、日期区间、点表。
//
// view 是"合并视图"（Update 场景未携带的字段已用库内值补齐），本函数可就地
// 整理 view（去重、补默认值）；repo 收到的仍是调用方原始提交值，mask 语义
// （mask 内空数组 = 显式清空）不受影响。
func (s *PeriodicPlanService) validate(ctx context.Context, view *onestopV1.PeriodicPlan) error {
	if view.GetName() == "" {
		return onestopV1.ErrorBadRequest("plan name is required")
	}

	// 类别：必须真实存在，为小类/细类，且解析后触发方式为「计划生成」
	//（细类未设置回退小类——挂在小类 SCHEDULED 下的新细类无须再显式开启）
	if view.ServiceTypeId == nil || view.GetServiceTypeId() == 0 {
		return onestopV1.ErrorBadRequest("service type is required")
	}
	typeEntity, err := s.typeRepo.GetByID(ctx, view.GetServiceTypeId())
	if err != nil {
		return err
	}
	if len(typeEntity.GetCode()) < 6 {
		return onestopV1.ErrorUnprocessableEntity("periodic plan must attach to a minor/detail level service type")
	}
	trigger, err := ResolveTriggerMode(ctx, s.typeRepo, typeEntity)
	if err != nil {
		return err
	}
	if trigger != onestopV1.TriggerMode_TRIGGER_SCHEDULED {
		return onestopV1.ErrorUnprocessableEntity(
			"periodic plan must attach to a TRIGGER_SCHEDULED service type (enable scheduled trigger mode first)")
	}

	dedupe := func(values []int32) []int32 {
		seen := make(map[int32]bool, len(values))
		out := make([]int32, 0, len(values))
		for _, v := range values {
			if seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
		return out
	}
	dedupeStr := func(values []string) []string {
		seen := make(map[string]bool, len(values))
		out := make([]string, 0, len(values))
		for _, v := range values {
			if seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
		return out
	}

	// 周期类型与各周期的执行日规则（执行日字段域校验对所有计划生效，
	// 非空要求数按周期判定）
	switch view.GetFreqType() {
	case onestopV1.FreqType_FREQ_UNSPECIFIED:
		return onestopV1.ErrorUnprocessableEntity("frequency type is required")
	case onestopV1.FreqType_FREQ_WEEKLY:
		if len(view.GetWeekdays()) == 0 {
			return onestopV1.ErrorUnprocessableEntity("weekly plan requires at least one weekday")
		}
	case onestopV1.FreqType_FREQ_MONTHLY, onestopV1.FreqType_FREQ_YEARLY:
		if len(view.GetMonthDays()) == 0 {
			return onestopV1.ErrorUnprocessableEntity("monthly/yearly plan requires at least one day of month")
		}
		if view.GetFreqType() == onestopV1.FreqType_FREQ_YEARLY && len(view.GetMonths()) == 0 {
			return onestopV1.ErrorUnprocessableEntity("yearly plan requires at least one month")
		}
	case onestopV1.FreqType_FREQ_DAILY:
	default:
		return onestopV1.ErrorUnprocessableEntity("unknown frequency type")
	}

	for _, wd := range dedupe(view.GetWeekdays()) {
		if wd < 1 || wd > 7 {
			return onestopV1.ErrorUnprocessableEntity("weekdays must be 1(Mon)-7(Sun)")
		}
	}
	view.Weekdays = dedupe(view.GetWeekdays())

	for _, md := range dedupe(view.GetMonthDays()) {
		if (md < 1 || md > 31) && md != -1 {
			return onestopV1.ErrorUnprocessableEntity("month days must be 1-31 or -1 (last day)")
		}
	}
	view.MonthDays = dedupe(view.GetMonthDays())

	for _, m := range dedupe(view.GetMonths()) {
		if m < 1 || m > 12 {
			return onestopV1.ErrorUnprocessableEntity("months must be 1-12")
		}
	}
	view.Months = dedupe(view.GetMonths())

	if view.ClampToMonthEnd == nil {
		view.ClampToMonthEnd = trans.Ptr(true)
	}

	// 执行时间点：1-10 个，HH:mm，去重
	times := view.GetTimes()
	if len(times) == 0 {
		return onestopV1.ErrorUnprocessableEntity("plan requires at least one execution time")
	}
	if len(times) > 10 {
		return onestopV1.ErrorUnprocessableEntity("too many execution times (max 10)")
	}
	view.Times = dedupeStr(times)
	for _, tm := range view.Times {
		if !timePattern.MatchString(tm) {
			return onestopV1.ErrorUnprocessableEntity("execution time must be HH:mm (e.g. 08:30)")
		}
	}

	// 排除日期：YYYY-MM-DD
	view.ExcludedDates = dedupeStr(view.GetExcludedDates())
	for _, d := range view.ExcludedDates {
		if !excludedPattern.MatchString(d) {
			return onestopV1.ErrorUnprocessableEntity("excluded date must be YYYY-MM-DD")
		}
	}

	// 生效区间：from <= to
	if view.EffectiveFrom != nil && view.EffectiveTo != nil &&
		view.GetEffectiveFrom().AsTime().After(view.GetEffectiveTo().AsTime()) {
		return onestopV1.ErrorUnprocessableEntity("effective_from must not be after effective_to")
	}

	// 点表：两组合计至少 1 项，名称非空且组内不重复（停用项保留在点表中）
	itemCount := 0
	checkItems := func(items []*onestopV1.PeriodicPlanItem) error {
		nameSeen := make(map[string]bool, len(items))
		for _, item := range items {
			if item == nil {
				continue
			}
			if strings.TrimSpace(item.GetName()) == "" {
				return onestopV1.ErrorUnprocessableEntity("plan item name is required")
			}
			if nameSeen[item.GetName()] {
				return onestopV1.ErrorUnprocessableEntity("duplicate plan item: %s", item.GetName())
			}
			nameSeen[item.GetName()] = true
			itemCount++
		}
		return nil
	}
	if err = checkItems(view.GetSpaceItems()); err != nil {
		return err
	}
	if err = checkItems(view.GetEquipmentItems()); err != nil {
		return err
	}
	if itemCount == 0 {
		return onestopV1.ErrorUnprocessableEntity("plan requires at least one plan item")
	}

	return nil
}

// maskHasField 判断 updateMask 是否含该字段（兼容 camelCase / snake_case 路径）。
func maskHasField(req *onestopV1.UpdatePeriodicPlanRequest, fieldSnake, fieldCamel string) bool {
	for _, p := range req.GetUpdateMask().GetPaths() {
		if p == fieldSnake || p == fieldCamel {
			return true
		}
	}
	return false
}

func (s *PeriodicPlanService) List(ctx context.Context, req *paginationV1.PagingRequest) (*onestopV1.ListPeriodicPlansResponse, error) {
	return s.repo.List(ctx, req)
}

func (s *PeriodicPlanService) Get(ctx context.Context, req *onestopV1.GetPeriodicPlanRequest) (*onestopV1.PeriodicPlan, error) {
	return s.repo.Get(ctx, req)
}

func (s *PeriodicPlanService) Create(ctx context.Context, req *onestopV1.CreatePeriodicPlanRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	if err := s.validate(ctx, req.Data); err != nil {
		return nil, err
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	req.Data.CreatedBy = trans.Ptr(operator)

	if err = s.repo.Create(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *PeriodicPlanService) Update(ctx context.Context, req *onestopV1.UpdatePeriodicPlanRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}

	// 存量记录：用于构造校验合并视图（未携带的字段以库内值为准，
	// 部分更新如只改名称/启停时频率校验不至于误判缺执行日）
	existing, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	// 当前周期类型（以提交值优先，未携带时取库内值）下，必填执行日字段
	// 不允许被 mask 显式清空（提交空数组且 mask 含该字段 = 破坏计划完整性）
	freq := req.Data.GetFreqType()
	if freq == onestopV1.FreqType_FREQ_UNSPECIFIED {
		freq = existing.GetFreqType()
	}
	clearsRequired := func(fieldSnake, fieldCamel string, submittedLen int) bool {
		if submittedLen > 0 || !maskHasField(req, fieldSnake, fieldCamel) {
			return false
		}
		switch fieldSnake {
		case "times":
			return true
		case "weekdays":
			return freq == onestopV1.FreqType_FREQ_WEEKLY
		case "month_days":
			return freq == onestopV1.FreqType_FREQ_MONTHLY || freq == onestopV1.FreqType_FREQ_YEARLY
		case "months":
			return freq == onestopV1.FreqType_FREQ_YEARLY
		}
		return false
	}
	if clearsRequired("times", "times", len(req.Data.GetTimes())) {
		return nil, onestopV1.ErrorUnprocessableEntity("plan requires at least one execution time")
	}
	if clearsRequired("weekdays", "weekdays", len(req.Data.GetWeekdays())) {
		return nil, onestopV1.ErrorUnprocessableEntity("weekly plan requires at least one weekday")
	}
	if clearsRequired("month_days", "monthDays", len(req.Data.GetMonthDays())) {
		return nil, onestopV1.ErrorUnprocessableEntity("monthly/yearly plan requires at least one day of month")
	}
	if clearsRequired("months", "months", len(req.Data.GetMonths())) {
		return nil, onestopV1.ErrorUnprocessableEntity("yearly plan requires at least one month")
	}

	// 校验走合并视图副本：repo 收到的 req.Data 保持原样，
	// jsonb 数组"mask 内空值 = 显式清空"的语义不被破坏
	view := proto.Clone(req.Data).(*onestopV1.PeriodicPlan)
	if view.Name == nil {
		view.Name = existing.Name
	}
	if view.ServiceTypeId == nil {
		view.ServiceTypeId = existing.ServiceTypeId
	}
	if view.FreqType == nil {
		view.FreqType = existing.FreqType
	}
	if len(view.GetTimes()) == 0 {
		view.Times = existing.Times
	}
	if len(view.GetWeekdays()) == 0 {
		view.Weekdays = existing.Weekdays
	}
	if len(view.GetMonthDays()) == 0 {
		view.MonthDays = existing.MonthDays
	}
	if len(view.GetMonths()) == 0 {
		view.Months = existing.Months
	}
	if len(view.GetExcludedDates()) == 0 {
		view.ExcludedDates = existing.ExcludedDates
	}
	if len(view.GetSpaceItems()) == 0 {
		view.SpaceItems = existing.SpaceItems
	}
	if len(view.GetEquipmentItems()) == 0 {
		view.EquipmentItems = existing.EquipmentItems
	}
	if view.ClampToMonthEnd == nil {
		view.ClampToMonthEnd = existing.ClampToMonthEnd
	}
	if view.EffectiveFrom == nil {
		view.EffectiveFrom = existing.EffectiveFrom
	}
	if view.EffectiveTo == nil {
		view.EffectiveTo = existing.EffectiveTo
	}

	if err = s.validate(ctx, view); err != nil {
		return nil, err
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	req.Data.Id = trans.Ptr(req.GetId())
	req.Data.UpdatedBy = trans.Ptr(operator)
	if req.UpdateMask != nil && !maskHasField(req, "updated_by", "updatedBy") {
		req.UpdateMask.Paths = append(req.UpdateMask.Paths, "updated_by")
	}

	if err = s.repo.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *PeriodicPlanService) Delete(ctx context.Context, req *onestopV1.DeletePeriodicPlanRequest) (*emptypb.Empty, error) {
	if req.GetId() == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}

	if err := s.repo.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
