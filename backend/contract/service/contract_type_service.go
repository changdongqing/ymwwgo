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

	contractV1 "go-wind-admin/contract/api/gen/go/contract/service/v1"

	"go-wind-admin/contract/data"
)

// OperatorReader 从请求上下文提取当前操作人用户 ID。
// 认证中间件与令牌载荷定义都在宿主（admin 服务）侧，本独立 module 不反向依赖宿主，
// 由宿主在装配期注入实现（wiring 时包一层 auth.FromContext）。
type OperatorReader func(ctx context.Context) (uint32, error)

// levelCodePattern 类型编码：2/4 位数字，位数即层级。
var levelCodePattern = regexp.MustCompile(`^([0-9]{2}|[0-9]{4})$`)

// diffWidgetSet 差异字段控件合法值域（存储值与前端/种子约定一致）。
var diffWidgetSet = map[string]bool{
	"WIDGET_TEXT":     true, // 单行文本
	"WIDGET_TEXTAREA": true, // 多行文本
	"WIDGET_SELECT":   true, // 下拉选择
	"WIDGET_DATE":     true, // 日期
}

// levelOfCode 由编码位数推导层级。
func levelOfCode(code string) contractV1.ContractLevel {
	if len(code) == 2 {
		return contractV1.ContractLevel_LEVEL_MAJOR
	}
	return contractV1.ContractLevel_LEVEL_DETAIL
}

// ResolvePayMethod 解析节点生效的默认付款方式：自身已设置以自身为准；
// 细类未设置回退大类；仍未设置按 PAY_ONCE（合同默认一次性）。
func ResolvePayMethod(entity *contractV1.ContractType) contractV1.PayMethod {
	if entity.PayMethod != nil && entity.GetPayMethod() != contractV1.PayMethod_PAY_METHOD_UNSPECIFIED {
		return entity.GetPayMethod()
	}
	return contractV1.PayMethod_PAY_ONCE
}

// ResolveFeeDirection 解析节点生效的默认费用方向：自身已设置以自身为准；
// 细类未设置回退大类；仍未设置按 DIRECTION_PAYABLE（合同绝大多数是我方付款）。
func ResolveFeeDirection(entity *contractV1.ContractType) contractV1.FeeDirection {
	if entity.FeeDirection != nil && entity.GetFeeDirection() != contractV1.FeeDirection_DIRECTION_UNSPECIFIED {
		return entity.GetFeeDirection()
	}
	return contractV1.FeeDirection_DIRECTION_PAYABLE
}

// ResolveEffectiveContractConfig 解析节点的生效合同配置（合并视图）：
// 单值列细类为空回退大类；数组列整组覆盖或整组继承（不做数组内 merge）；
// 开关与方式/方向为解析后的兜底生效值（requires_approval 默认 true、
// requires_deposit 默认 false）。创建表单预填与合同创建快照共用本口径。
func ResolveEffectiveContractConfig(ctx context.Context, typeRepo *data.ContractTypeRepo, entity *contractV1.ContractType) (*contractV1.ContractType, error) {
	view := proto.Clone(entity).(*contractV1.ContractType)

	if entity.GetLevel() == contractV1.ContractLevel_LEVEL_DETAIL && entity.GetParentId() != 0 {
		parent, err := typeRepo.GetByID(ctx, entity.GetParentId())
		if err != nil {
			return nil, err
		}

		if view.PartyBLabel == nil {
			view.PartyBLabel = parent.PartyBLabel
		}
		if view.Guidance == nil {
			view.Guidance = parent.Guidance
		}
		if view.PayMethod == nil {
			view.PayMethod = parent.PayMethod
		}
		if view.FeeDirection == nil {
			view.FeeDirection = parent.FeeDirection
		}
		if view.RequiresApproval == nil {
			view.RequiresApproval = parent.RequiresApproval
		}
		if view.RequiresDeposit == nil {
			view.RequiresDeposit = parent.RequiresDeposit
		}
		// 数组列：整组继承（自身为空才回退，不合并）
		if len(view.GetPayNodes()) == 0 {
			view.PayNodes = parent.PayNodes
		}
		if len(view.GetDiffFields()) == 0 {
			view.DiffFields = parent.DiffFields
		}
		if len(view.GetRequiredAttachments()) == 0 {
			view.RequiredAttachments = parent.RequiredAttachments
		}
	}

	// 兜底默认值（两级都未设置时给出生效值）
	if view.RequiresApproval == nil {
		view.RequiresApproval = trans.Ptr(true)
	}
	if view.RequiresDeposit == nil {
		view.RequiresDeposit = trans.Ptr(false)
	}
	view.PayMethod = ResolvePayMethod(view).Enum()
	view.FeeDirection = ResolveFeeDirection(view).Enum()

	return view, nil
}

type ContractTypeService struct {
	contractV1.ContractTypeServiceHTTPServer

	log *bLogger.Helper

	repo    *data.ContractTypeRepo
	contRepo *data.ContractRepo
	operator OperatorReader
}

func NewContractTypeService(ctx *bootstrap.Context, repo *data.ContractTypeRepo, contRepo *data.ContractRepo, operator OperatorReader) *ContractTypeService {
	s := &ContractTypeService{
		log:      ctx.NewLoggerHelper("contract-type/service/contract"),
		repo:     repo,
		contRepo: contRepo,
		operator: operator,
	}

	// 空表时播种内置二级类型字典（count == 0 守卫，非空表不覆盖）。
	// 播种失败不阻断启动：类型表为空时管理页仍可手工录入，
	// 但错误必须带原始对象落日志，便于排查迁移/权限问题。
	if err := repo.SeedDefaults(ctx.Context()); err != nil {
		s.log.Errorf(ctx.Context(), "seed built-in contract types failed: %v", err)
	}

	return s
}

// validate 校验类型编码/层级/上级链一致性 + 模板列值域（selfID 为 0 表示新建）。
func (s *ContractTypeService) validate(ctx context.Context, data *contractV1.ContractType, selfID uint32) error {
	if data.GetCode() == "" {
		return contractV1.ErrorBadRequest("type code is required")
	}
	if data.GetName() == "" {
		return contractV1.ErrorBadRequest("type name is required")
	}
	if !levelCodePattern.MatchString(data.GetCode()) {
		return contractV1.ErrorUnprocessableEntity("type code must be 2 or 4 digits")
	}

	// 模板值域校验：字段可空，但给了就必须合法
	if data.PayMethod != nil {
		switch data.GetPayMethod() {
		case contractV1.PayMethod_PAY_ONCE, contractV1.PayMethod_PAY_MONTHLY,
			contractV1.PayMethod_PAY_QUARTERLY, contractV1.PayMethod_PAY_NODE,
			contractV1.PayMethod_PAY_PROGRESS:
		default:
			return contractV1.ErrorUnprocessableEntity("pay method is invalid")
		}
	}
	if data.FeeDirection != nil {
		switch data.GetFeeDirection() {
		case contractV1.FeeDirection_DIRECTION_PAYABLE, contractV1.FeeDirection_DIRECTION_RECEIVABLE:
		default:
			return contractV1.ErrorUnprocessableEntity("fee direction is invalid")
		}
	}

	// 付款节点：名称非空不重复、比例 1-100；PAY_NODE/PAY_PROGRESS 时建议和为 100
	//（≠100 拆解器按余数进末节点容错，这里只挡非法单值）
	nodeSeen := make(map[string]bool, len(data.GetPayNodes()))
	ratioSum := int32(0)
	for _, node := range data.GetPayNodes() {
		if node == nil {
			continue
		}
		if strings.TrimSpace(node.GetName()) == "" {
			return contractV1.ErrorUnprocessableEntity("pay node name is required")
		}
		if nodeSeen[node.GetName()] {
			return contractV1.ErrorUnprocessableEntity("duplicate pay node: %s", node.GetName())
		}
		nodeSeen[node.GetName()] = true
		if node.GetRatioPercent() < 1 || node.GetRatioPercent() > 100 {
			return contractV1.ErrorUnprocessableEntity("pay node ratio must be 1-100 percent")
		}
		ratioSum += node.GetRatioPercent()
	}
	if len(nodeSeen) > 0 && ratioSum != 100 {
		return contractV1.ErrorUnprocessableEntity("pay node ratios must sum to 100")
	}

	// 差异字段：key/label 非空不重复、控件值域、下拉须有选项
	fieldSeen := make(map[string]bool, len(data.GetDiffFields()))
	for _, field := range data.GetDiffFields() {
		if field == nil {
			continue
		}
		if strings.TrimSpace(field.GetKey()) == "" {
			return contractV1.ErrorUnprocessableEntity("diff field key is required")
		}
		if strings.TrimSpace(field.GetLabel()) == "" {
			return contractV1.ErrorUnprocessableEntity("diff field label is required")
		}
		if fieldSeen[field.GetKey()] {
			return contractV1.ErrorUnprocessableEntity("duplicate diff field key: %s", field.GetKey())
		}
		fieldSeen[field.GetKey()] = true
		if !diffWidgetSet[field.GetWidget()] {
			return contractV1.ErrorUnprocessableEntity(
				"diff field widget must be one of WIDGET_TEXT/WIDGET_TEXTAREA/WIDGET_SELECT/WIDGET_DATE")
		}
		if field.GetWidget() == "WIDGET_SELECT" && len(field.GetOptions()) == 0 {
			return contractV1.ErrorUnprocessableEntity("diff field %s is a select but has no options", field.GetKey())
		}
	}

	// 必备附件：非空串
	for _, name := range data.GetRequiredAttachments() {
		if strings.TrimSpace(name) == "" {
			return contractV1.ErrorUnprocessableEntity("required attachment name must not be empty")
		}
	}

	level := levelOfCode(data.GetCode())
	if data.Level != nil && data.GetLevel() != level {
		return contractV1.ErrorUnprocessableEntity("type level does not match code length")
	}

	if level == contractV1.ContractLevel_LEVEL_MAJOR {
		// 大类是根节点，不允许挂上级
		if data.GetParentId() != 0 {
			return contractV1.ErrorUnprocessableEntity("major-level contract type must not have a parent")
		}
		return nil
	}

	// 细类：必须挂大类，且编码前 2 位 = 父大类编码
	if data.GetParentId() == 0 {
		return contractV1.ErrorUnprocessableEntity("parent contract type is required")
	}
	if data.GetParentId() == selfID {
		return contractV1.ErrorConflict("parent contract type must not be itself")
	}

	parent, err := s.repo.GetByID(ctx, data.GetParentId())
	if err != nil {
		return err
	}
	if parent.GetLevel() != contractV1.ContractLevel_LEVEL_MAJOR {
		return contractV1.ErrorUnprocessableEntity("parent contract type must be major level")
	}
	if !strings.HasPrefix(data.GetCode(), parent.GetCode()) {
		return contractV1.ErrorUnprocessableEntity("detail code must extend parent major code (first 2 digits)")
	}

	return nil
}

func (s *ContractTypeService) List(ctx context.Context, req *paginationV1.PagingRequest) (*contractV1.ListContractTypesResponse, error) {
	return s.repo.List(ctx, req)
}

func (s *ContractTypeService) Get(ctx context.Context, req *contractV1.GetContractTypeRequest) (*contractV1.ContractType, error) {
	return s.repo.Get(ctx, req)
}

func (s *ContractTypeService) Create(ctx context.Context, req *contractV1.CreateContractTypeRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, contractV1.ErrorBadRequest("invalid parameter")
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
		return nil, contractV1.ErrorConflict("type code already exists")
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

func (s *ContractTypeService) Update(ctx context.Context, req *contractV1.UpdateContractTypeRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, contractV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return nil, contractV1.ErrorBadRequest("id is required")
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

	// 编码全局唯一（排除自身）
	exist, err := s.repo.IsExistCode(ctx, req.Data.GetCode(), req.GetId())
	if err != nil {
		return nil, err
	}
	if exist {
		return nil, contractV1.ErrorConflict("type code already exists")
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

func (s *ContractTypeService) Delete(ctx context.Context, req *contractV1.DeleteContractTypeRequest) (*emptypb.Empty, error) {
	if req.GetId() == 0 {
		return nil, contractV1.ErrorBadRequest("id is required")
	}

	// 有下级禁止删除，避免悬空子树
	hasChildren, err := s.repo.HasChildren(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if hasChildren {
		return nil, contractV1.ErrorConflict("contract type has children, delete children first")
	}

	// 有合同引用禁止删除（快照只存编码/名称，但删除会让新增无类型可选）
	count, err := s.contRepo.CountByContractTypeIDs(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, contractV1.ErrorConflict("contract type is referenced by %d contract(s), stop it instead of deleting", count)
	}

	if err = s.repo.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// GetEffectiveConfig 查询类型的生效合同配置（细类→大类回退解析后的完整视图）：
// 创建表单预填与合同创建快照共用同一解析口径。任意层级可查
//（大类无回退链，直接给兜底默认值）。
func (s *ContractTypeService) GetEffectiveConfig(ctx context.Context, req *contractV1.GetEffectiveContractConfigRequest) (*contractV1.EffectiveContractConfig, error) {
	if req.GetId() == 0 {
		return nil, contractV1.ErrorBadRequest("id is required")
	}

	entity, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	view, err := ResolveEffectiveContractConfig(ctx, s.repo, entity)
	if err != nil {
		return nil, err
	}

	return &contractV1.EffectiveContractConfig{Effective: view}, nil
}
