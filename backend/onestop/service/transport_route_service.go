package service

import (
	"context"
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

// 运送路线只允许挂在 0106 运送配送子树的小类/细类上。
const transportCategoryCode = "0106"

// maxRouteDurationMinutes 单程时长上限：24 小时（防把往返总时长误填进单程）。
const maxRouteDurationMinutes = 24 * 60

type TransportRouteService struct {
	onestopV1.OneStopTransportRouteServiceHTTPServer

	log *bLogger.Helper

	repo     *data.TransportRouteRepo
	typeRepo *data.ServiceTypeRepo
	operator OperatorReader
}

func NewTransportRouteService(ctx *bootstrap.Context, repo *data.TransportRouteRepo, typeRepo *data.ServiceTypeRepo, operator OperatorReader) *TransportRouteService {
	s := &TransportRouteService{
		log:      ctx.NewLoggerHelper("transport-route/service/onestop"),
		repo:     repo,
		typeRepo: typeRepo,
		operator: operator,
	}

	// 空表时播种内置行业模板路线（count == 0 守卫）。播种失败不阻断启动，
	// 错误必须带原始对象落日志。依赖 wiring 中 ServiceTypeService 先行构造
	//（其构造函数播种类别字典），此处按编码解析类别才能命中。
	if err := repo.SeedDefaults(ctx.Context()); err != nil {
		s.log.Errorf(ctx.Context(), "seed built-in transport routes failed: %v", err)
	}

	return s
}

// validate 运送路线业务校验：类别归属、起终点、时长域、班次时间点。
//
// view 是"合并视图"（Update 场景未携带的字段已用库内值补齐），本函数可就地
// 整理 view（去重）；repo 收到的仍是调用方原始提交值，mask 语义
// （mask 内空数组 = 显式清空）不受影响。
func (s *TransportRouteService) validate(ctx context.Context, view *onestopV1.TransportRoute) error {
	if view.GetName() == "" {
		return onestopV1.ErrorBadRequest("route name is required")
	}
	if strings.TrimSpace(view.GetOrigin()) == "" {
		return onestopV1.ErrorBadRequest("route origin is required")
	}
	if strings.TrimSpace(view.GetDestination()) == "" {
		return onestopV1.ErrorBadRequest("route destination is required")
	}

	// 类别：必须真实存在且属于 0106 运送配送子树的小类/细类
	if view.ServiceTypeId == nil || view.GetServiceTypeId() == 0 {
		return onestopV1.ErrorBadRequest("service type is required")
	}
	typeEntity, err := s.typeRepo.GetByID(ctx, view.GetServiceTypeId())
	if err != nil {
		return err
	}
	code := typeEntity.GetCode()
	if len(code) < 6 || code[:4] != transportCategoryCode {
		return onestopV1.ErrorUnprocessableEntity("transport route must attach to a 0106 subtree service type")
	}

	// 单程时长：给了就必须落在 1 分钟 - 24 小时
	if view.EstDurationMinutes != nil &&
		(view.GetEstDurationMinutes() == 0 || view.GetEstDurationMinutes() > maxRouteDurationMinutes) {
		return onestopV1.ErrorUnprocessableEntity("estimated duration must be 1-1440 minutes")
	}

	// 途经点：非空串、去重
	viaSeen := make(map[string]bool, len(view.GetViaPoints()))
	via := make([]string, 0, len(view.GetViaPoints()))
	for _, p := range view.GetViaPoints() {
		if strings.TrimSpace(p) == "" {
			return onestopV1.ErrorUnprocessableEntity("via point must not be empty")
		}
		if viaSeen[p] {
			continue
		}
		viaSeen[p] = true
		via = append(via, p)
	}
	view.ViaPoints = via

	// 定时班次：可空（按需即时）；给了则 HH:mm、去重、至多 12 班
	times := view.GetDepartureTimes()
	if len(times) > 12 {
		return onestopV1.ErrorUnprocessableEntity("too many departure times (max 12)")
	}
	kept := make([]string, 0, len(times))
	seen := make(map[string]bool, len(times))
	for _, tm := range times {
		if seen[tm] {
			continue
		}
		seen[tm] = true
		if !timePattern.MatchString(tm) {
			return onestopV1.ErrorUnprocessableEntity("departure time must be HH:mm (e.g. 08:30)")
		}
		kept = append(kept, tm)
	}
	view.DepartureTimes = kept

	if view.RoundTrip == nil {
		view.RoundTrip = trans.Ptr(false)
	}

	return nil
}

// maskHasField 判断 updateMask 是否含该字段（兼容 camelCase / snake_case 路径）。
func routeMaskHasField(req *onestopV1.UpdateTransportRouteRequest, fieldSnake, fieldCamel string) bool {
	for _, p := range req.GetUpdateMask().GetPaths() {
		if p == fieldSnake || p == fieldCamel {
			return true
		}
	}
	return false
}

func (s *TransportRouteService) List(ctx context.Context, req *paginationV1.PagingRequest) (*onestopV1.ListTransportRoutesResponse, error) {
	return s.repo.List(ctx, req)
}

func (s *TransportRouteService) Get(ctx context.Context, req *onestopV1.GetTransportRouteRequest) (*onestopV1.TransportRoute, error) {
	return s.repo.Get(ctx, req)
}

func (s *TransportRouteService) Create(ctx context.Context, req *onestopV1.CreateTransportRouteRequest) (*emptypb.Empty, error) {
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

func (s *TransportRouteService) Update(ctx context.Context, req *onestopV1.UpdateTransportRouteRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}

	// 存量记录：用于构造校验合并视图（未携带的字段以库内值为准，
	// 部分更新如只改名称/启停时类别与起终点校验不至于误判缺失）
	existing, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	// 校验走合并视图副本：repo 收到的 req.Data 保持原样，
	// jsonb 数组"mask 内空值 = 显式清空"的语义不被破坏
	view := proto.Clone(req.Data).(*onestopV1.TransportRoute)
	if view.Name == nil {
		view.Name = existing.Name
	}
	if view.ServiceTypeId == nil {
		view.ServiceTypeId = existing.ServiceTypeId
	}
	if view.Origin == nil {
		view.Origin = existing.Origin
	}
	if view.Destination == nil {
		view.Destination = existing.Destination
	}
	if view.EstDurationMinutes == nil {
		view.EstDurationMinutes = existing.EstDurationMinutes
	}
	if view.RoundTrip == nil {
		view.RoundTrip = existing.RoundTrip
	}
	if len(view.GetViaPoints()) == 0 {
		view.ViaPoints = existing.ViaPoints
	}
	if len(view.GetDepartureTimes()) == 0 {
		view.DepartureTimes = existing.DepartureTimes
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
	if req.UpdateMask != nil && !routeMaskHasField(req, "updated_by", "updatedBy") {
		req.UpdateMask.Paths = append(req.UpdateMask.Paths, "updated_by")
	}

	if err = s.repo.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *TransportRouteService) Delete(ctx context.Context, req *onestopV1.DeleteTransportRouteRequest) (*emptypb.Empty, error) {
	if req.GetId() == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}

	if err := s.repo.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
