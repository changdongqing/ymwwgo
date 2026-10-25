package service

import (
	"context"
	"regexp"
	"strings"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	"github.com/tx7do/go-utils/trans"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"
	"google.golang.org/protobuf/types/known/emptypb"

	onestopV1 "go-wind-admin/onestop/api/gen/go/onestop/service/v1"

	"go-wind-admin/onestop/data"
)

// OperatorReader 从请求上下文提取当前操作人用户 ID。
// 认证中间件与令牌载荷定义都在宿主（admin 服务）侧，本独立 module 不反向依赖宿主，
// 由宿主在装配期注入实现（wiring 时包一层 auth.FromContext）。
type OperatorReader func(ctx context.Context) (uint32, error)

// levelCodePattern 服务编码：2/4/6/8 位数字，位数即层级。
var levelCodePattern = regexp.MustCompile(`^([0-9]{2}|[0-9]{4}|[0-9]{6}|[0-9]{8})$`)

// acceptanceModeSet 验收方式合法值域（存储值与前端/种子约定一致）。
var acceptanceModeSet = map[string]bool{
	"ACCEPT_SIGN":  true, // 签名确认
	"ACCEPT_SCAN":  true, // 扫码核对
	"ACCEPT_PHOTO": true, // 拍照留证
	"ACCEPT_DUAL":  true, // 双人验收
}

// maxTimeLimitMinutes 时限上限：30 天，防分钟/小时单位误填。
const maxTimeLimitMinutes = 30 * 24 * 60

// levelOfCode 由编码位数推导层级。
func levelOfCode(code string) onestopV1.Level {
	switch len(code) {
	case 2:
		return onestopV1.Level_LEVEL_MAJOR
	case 4:
		return onestopV1.Level_LEVEL_MIDDLE
	case 6:
		return onestopV1.Level_LEVEL_MINOR
	case 8:
		return onestopV1.Level_LEVEL_DETAIL
	default:
		return onestopV1.Level_LEVEL_UNSPECIFIED
	}
}

// ResolveTriggerMode 解析类别节点生效的任务触发方式：
// 自身已设置（ON_DEMAND/SCHEDULED）以自身为准；细类未设置回退小类；
// 仍未设置按 TRIGGER_ON_DEMAND（统一模板默认：按需发起）。
// 中类/大类是结构节点不参与回退（触发方式本就不允许配在结构层）。
func ResolveTriggerMode(ctx context.Context, typeRepo *data.ServiceTypeRepo, entity *onestopV1.ServiceType) (onestopV1.TriggerMode, error) {
	if entity.TriggerMode != nil && entity.GetTriggerMode() != onestopV1.TriggerMode_TRIGGER_UNSPECIFIED {
		return entity.GetTriggerMode(), nil
	}
	if entity.GetLevel() == onestopV1.Level_LEVEL_DETAIL && entity.GetParentId() != 0 {
		parent, err := typeRepo.GetByID(ctx, entity.GetParentId())
		if err != nil {
			return onestopV1.TriggerMode_TRIGGER_UNSPECIFIED, err
		}
		if parent.TriggerMode != nil && parent.GetTriggerMode() != onestopV1.TriggerMode_TRIGGER_UNSPECIFIED {
			return parent.GetTriggerMode(), nil
		}
	}
	return onestopV1.TriggerMode_TRIGGER_ON_DEMAND, nil
}

// hasConfigPayload 判断提交数据是否携带任一履约配置（未配置 = NULL/空）。
// repeated 空切片不算（那是显式清空）；requiresApproval 仅 true 算（false 是默认值）。
// 用于结构层（大类/中类）拒绝配置字段：结构节点只承载分类，不承载履约参数。
func hasConfigPayload(d *onestopV1.ServiceType) bool {
	return d.GetExecutionPost() != "" ||
		d.StandardWorkHours != nil ||
		d.ResponseTimeLimit != nil ||
		d.CompletionTimeLimit != nil ||
		d.GetGuidance() != "" ||
		len(d.GetCommonPhrases()) > 0 ||
		len(d.GetStandardTools()) > 0 ||
		len(d.GetSpareParts()) > 0 ||
		len(d.GetStandardItems()) > 0 ||
		len(d.GetCargoTypes()) > 0 ||
		len(d.GetAcceptanceModes()) > 0 ||
		len(d.GetAcceptanceItems()) > 0 ||
		d.DefaultPriority != nil ||
		d.TemperatureRequirement != nil ||
		d.GetRequiresApproval() ||
		d.TriggerMode != nil
}

type ServiceTypeService struct {
	onestopV1.OneStopServiceTypeServiceHTTPServer

	log *bLogger.Helper

	repo     *data.ServiceTypeRepo
	planRepo *data.PeriodicPlanRepo
	operator OperatorReader
}

func NewServiceTypeService(ctx *bootstrap.Context, repo *data.ServiceTypeRepo, planRepo *data.PeriodicPlanRepo, operator OperatorReader) *ServiceTypeService {
	s := &ServiceTypeService{
		log:      ctx.NewLoggerHelper("service-type/service/onestop"),
		repo:     repo,
		planRepo: planRepo,
		operator: operator,
	}

	// 空表时播种 README 全量类型字典（count == 0 守卫，非空表不覆盖）。
	// 播种失败不阻断启动：类型表为空时管理页仍可手工录入，
	// 但错误必须带原始对象落日志，便于排查迁移/权限问题。
	if err := repo.SeedDefaults(ctx.Context()); err != nil {
		s.log.Errorf(ctx.Context(), "seed built-in service types failed: %v", err)
	}

	return s
}

// validate 校验服务类型的编码/层级/上级链一致性（selfID 为 0 表示新建）。
func (s *ServiceTypeService) validate(ctx context.Context, data *onestopV1.ServiceType, selfID uint32) error {
	if data.GetCode() == "" {
		return onestopV1.ErrorBadRequest("service code is required")
	}
	if data.GetName() == "" {
		return onestopV1.ErrorBadRequest("service name is required")
	}
	if !levelCodePattern.MatchString(data.GetCode()) {
		return onestopV1.ErrorUnprocessableEntity("service code must be 2/4/6/8 digits")
	}

	// 履约配置数值校验：字段可空，但给了就必须落在合理区间
	if data.StandardWorkHours != nil && data.GetStandardWorkHours() <= 0 {
		return onestopV1.ErrorUnprocessableEntity("standard work hours must be positive")
	}
	if data.ResponseTimeLimit != nil &&
		(data.GetResponseTimeLimit() == 0 || data.GetResponseTimeLimit() > maxTimeLimitMinutes) {
		return onestopV1.ErrorUnprocessableEntity("response time limit must be 1-43200 minutes")
	}
	if data.CompletionTimeLimit != nil &&
		(data.GetCompletionTimeLimit() == 0 || data.GetCompletionTimeLimit() > maxTimeLimitMinutes) {
		return onestopV1.ErrorUnprocessableEntity("completion time limit must be 1-43200 minutes")
	}

	// 任务触发方式：值域校验（proto3 枚举可传任意整数值，这里收口）
	if data.TriggerMode != nil {
		switch data.GetTriggerMode() {
		case onestopV1.TriggerMode_TRIGGER_ON_DEMAND, onestopV1.TriggerMode_TRIGGER_SCHEDULED:
		default:
			return onestopV1.ErrorUnprocessableEntity("trigger mode must be TRIGGER_ON_DEMAND or TRIGGER_SCHEDULED")
		}
	}

	// 通用履约配置：验收方式值域 + 验收项（名称非空不重复）
	for _, mode := range data.GetAcceptanceModes() {
		if !acceptanceModeSet[mode] {
			return onestopV1.ErrorUnprocessableEntity(
				"acceptance mode must be one of ACCEPT_SIGN/ACCEPT_SCAN/ACCEPT_PHOTO/ACCEPT_DUAL")
		}
	}
	itemSeen := make(map[string]bool, len(data.GetAcceptanceItems()))
	for _, item := range data.GetAcceptanceItems() {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.GetName()) == "" {
			return onestopV1.ErrorUnprocessableEntity("acceptance item name is required")
		}
		if itemSeen[item.GetName()] {
			return onestopV1.ErrorUnprocessableEntity("duplicate acceptance item: %s", item.GetName())
		}
		itemSeen[item.GetName()] = true
	}

	// 运送配送配置：物品细目非空串
	for _, cargo := range data.GetCargoTypes() {
		if strings.TrimSpace(cargo) == "" {
			return onestopV1.ErrorUnprocessableEntity("cargo type must not be empty")
		}
	}

	level := levelOfCode(data.GetCode())
	if data.Level != nil && data.GetLevel() != level {
		return onestopV1.ErrorUnprocessableEntity("service level does not match code length")
	}

	// 统一模板门控：履约配置（含触发方式）只允许挂在小类/细类上，
	// 大类/中类是纯结构节点（此前仅前端门控，统一后后端补挡）
	if level == onestopV1.Level_LEVEL_MAJOR || level == onestopV1.Level_LEVEL_MIDDLE {
		if hasConfigPayload(data) {
			return onestopV1.ErrorUnprocessableEntity(
				"fulfillment config is only allowed on minor/detail level service types")
		}
	}

	if level == onestopV1.Level_LEVEL_MAJOR {
		// 大类是根节点，不允许挂上级
		if data.GetParentId() != 0 {
			return onestopV1.ErrorUnprocessableEntity("major-level service type must not have a parent")
		}
		return nil
	}

	if data.GetParentId() == 0 {
		return onestopV1.ErrorUnprocessableEntity("parent service type is required")
	}
	if data.GetParentId() == selfID {
		return onestopV1.ErrorConflict("parent service type must not be itself")
	}

	parent, err := s.repo.GetByID(ctx, data.GetParentId())
	if err != nil {
		return err
	}
	if parent.GetLevel() != level-1 {
		return onestopV1.ErrorUnprocessableEntity("parent service type level must be exactly one level up")
	}

	// 编辑场景：上级不能落在自身节点的子树里（沿上级链向上走，四级树最多四步）
	ancestor := parent
	for i := 0; i < 4 && ancestor != nil; i++ {
		if ancestor.GetId() == selfID {
			return onestopV1.ErrorConflict("parent service type must not be a descendant of itself")
		}
		if ancestor.GetParentId() == 0 {
			break
		}
		if ancestor, err = s.repo.GetByID(ctx, ancestor.GetParentId()); err != nil {
			return err
		}
	}

	return nil
}

func (s *ServiceTypeService) List(ctx context.Context, req *paginationV1.PagingRequest) (*onestopV1.ListServiceTypeResponse, error) {
	return s.repo.List(ctx, req)
}

func (s *ServiceTypeService) Get(ctx context.Context, req *onestopV1.GetServiceTypeRequest) (*onestopV1.ServiceType, error) {
	return s.repo.Get(ctx, req)
}

func (s *ServiceTypeService) Create(ctx context.Context, req *onestopV1.CreateServiceTypeRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}

	if err := s.validate(ctx, req.Data, 0); err != nil {
		return nil, err
	}

	// 编码全局唯一（库层有唯一索引兜底，这里先查给出可读错误）
	exist, err := s.repo.IsExistCode(ctx, req.Data.GetCode(), 0)
	if err != nil {
		return nil, err
	}
	if exist {
		return nil, onestopV1.ErrorConflict("service code already exists")
	}

	// 操作人
	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	req.Data.CreatedBy = trans.Ptr(operator)

	// 层级按编码位数推导，杜绝前后端不一致
	req.Data.Level = trans.Ptr(levelOfCode(req.Data.GetCode()))

	if err = s.repo.Create(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ServiceTypeService) Update(ctx context.Context, req *onestopV1.UpdateServiceTypeRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, onestopV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}

	// 存量记录：合并进本次校验（部分字段更新时未携带的字段以库内值为准）
	existing, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	maskIncludes := func(field string) bool {
		for _, p := range req.GetUpdateMask().GetPaths() {
			if p == field {
				return true
			}
		}
		return false
	}
	if req.Data.GetCode() == "" {
		req.Data.Code = existing.Code
	}
	if req.Data.GetName() == "" {
		req.Data.Name = existing.Name
	}
	if req.Data.GetParentId() == 0 && !maskIncludes("parentId") {
		req.Data.ParentId = existing.ParentId
	}

	if err := s.validate(ctx, req.Data, req.GetId()); err != nil {
		return nil, err
	}

	// 触发方式切换守卫：由「计划生成」切走（显式改按需，或清空后经回退链
	// 解析不再是计划生成），且节点自身或直接子级仍挂周期计划 → 拒绝，先删计划。
	if req.Data.TriggerMode != nil || maskIncludes("triggerMode") || maskIncludes("trigger_mode") {
		var resolved onestopV1.TriggerMode
		if req.Data.TriggerMode != nil && req.Data.GetTriggerMode() != onestopV1.TriggerMode_TRIGGER_UNSPECIFIED {
			resolved = req.Data.GetTriggerMode()
		} else {
			var rerr error
			if resolved, rerr = ResolveTriggerMode(ctx, s.repo, existing); rerr != nil {
				return nil, rerr
			}
		}
		if resolved != onestopV1.TriggerMode_TRIGGER_SCHEDULED {
			ids := []uint32{req.GetId()}
			children, cerr := s.repo.ChildrenIDs(ctx, req.GetId())
			if cerr != nil {
				return nil, cerr
			}
			ids = append(ids, children...)
			planCount, perr := s.planRepo.CountByServiceTypeIDs(ctx, ids...)
			if perr != nil {
				return nil, perr
			}
			if planCount > 0 {
				return nil, onestopV1.ErrorConflict(
					"service type has %d periodic plan(s), delete them before switching trigger mode", planCount)
			}
		}
	}

	// 编码全局唯一（排除自身）
	exist, err := s.repo.IsExistCode(ctx, req.Data.GetCode(), req.GetId())
	if err != nil {
		return nil, err
	}
	if exist {
		return nil, onestopV1.ErrorConflict("service code already exists")
	}

	// 操作人
	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	req.Data.Id = trans.Ptr(req.GetId())
	req.Data.UpdatedBy = trans.Ptr(operator)
	if req.UpdateMask != nil {
		// level 由 code 位数推导，即使前端未提交也必须写入，
		// 否则改编码长度后库内层级会停留在旧值。
		if !maskIncludes("updated_by") {
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "updated_by")
		}
		if !maskIncludes("level") {
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "level")
		}
	}

	// 层级按编码位数推导，杜绝前后端不一致
	req.Data.Level = trans.Ptr(levelOfCode(req.Data.GetCode()))

	if err = s.repo.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ServiceTypeService) Delete(ctx context.Context, req *onestopV1.DeleteServiceTypeRequest) (*emptypb.Empty, error) {
	if req.GetId() == 0 {
		return nil, onestopV1.ErrorBadRequest("id is required")
	}

	// 有下级禁止删除，避免悬空子树
	hasChildren, err := s.repo.HasChildren(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if hasChildren {
		return nil, onestopV1.ErrorConflict("service type has children, delete children first")
	}

	if err = s.repo.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
