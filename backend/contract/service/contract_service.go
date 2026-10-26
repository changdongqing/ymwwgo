package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	"github.com/tx7do/go-utils/trans"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	contractV1 "go-wind-admin/contract/api/gen/go/contract/service/v1"

	"go-wind-admin/contract/data"
	"go-wind-admin/contract/data/ent"
	"go-wind-admin/contract/data/ent/contract"
	"go-wind-admin/contract/data/ent/contractfee"
)

// 统一合同底座服务：一套生命周期状态机（草稿→待审核→执行中→四路关闭），
// 类型差异由「类型模板 + 差异字段快照」吃掉。快照原则：创建时从类型
// effective-config 解析运行期判定字段（审批开关/付款方式/差异字段 schema/
// 付款节点），生效时按快照拆解费用明细，此后类型修改不影响存量合同。
// 所有改状态的动作经 repo.Transition（状态守卫 + 同事务事件）落库；
// 费用收付经 repo.SettleFee；不改状态的动作（重新拆解）走 ReplaceAutoFees + AddEvent。

// isTerminalStatus 终态：变更终止/过期终止/已终止/已作废。
func isTerminalStatus(status contractV1.ContractStatus) bool {
	switch status {
	case contractV1.ContractStatus_STATUS_CHANGED,
		contractV1.ContractStatus_STATUS_EXPIRED,
		contractV1.ContractStatus_STATUS_TERMINATED,
		contractV1.ContractStatus_STATUS_INVALIDATED:
		return true
	}
	return false
}

// maxTaxRate 税率上限（%）。
const maxTaxRate = 13

type ContractService struct {
	contractV1.ContractServiceHTTPServer

	log *bLogger.Helper

	repo     *data.ContractRepo
	typeRepo *data.ContractTypeRepo
	operator OperatorReader
}

func NewContractService(
	ctx *bootstrap.Context,
	repo *data.ContractRepo,
	typeRepo *data.ContractTypeRepo,
	operator OperatorReader,
) *ContractService {
	return &ContractService{
		log:      ctx.NewLoggerHelper("contract/service/contract"),
		repo:     repo,
		typeRepo: typeRepo,
		operator: operator,
	}
}

// —— 建单快照管线 ——

// buildContractDraft 按类型解析快照并组装草稿；创建/变更开新版共用。
// src 携带调用方的业务字段（甲乙方/金额/差异字段值等），配置类字段
//（类型编码/名称/审批开关/差异字段 schema/付款节点）全部快照自
// effective-config；pay_method/fee_direction 以类型默认预填、允许调用方覆写。
func (s *ContractService) buildContractDraft(ctx context.Context, src *contractV1.Contract) (*contractV1.Contract, error) {
	if src.GetContractTypeId() == 0 {
		return nil, contractV1.ErrorBadRequest("contract type is required")
	}
	typeEntity, err := s.typeRepo.GetByID(ctx, src.GetContractTypeId())
	if err != nil {
		return nil, err
	}
	if !typeEntity.GetIsEnabled() {
		return nil, contractV1.ErrorUnprocessableEntity("contract type is disabled")
	}
	// 叶子判定：有子级的类型不允许直接立合同（先建/选细类）
	hasChildren, err := s.typeRepo.HasChildren(ctx, src.GetContractTypeId())
	if err != nil {
		return nil, err
	}
	if hasChildren {
		return nil, contractV1.ErrorUnprocessableEntity("contract type must be a leaf node (no children)")
	}

	eff, err := ResolveEffectiveContractConfig(ctx, s.typeRepo, typeEntity)
	if err != nil {
		return nil, err
	}

	draft := &contractV1.Contract{
		ContractTypeId:     src.ContractTypeId,
		ContractTypeCode:   typeEntity.Code,
		ContractTypeName:   typeEntity.Name,
		RequiresApproval:   eff.RequiresApproval,
		PayMethod:          eff.PayMethod,
		FeeDirection:       eff.FeeDirection,
		DiffSchema:         eff.GetDiffFields(),
		PayNodes:           eff.GetPayNodes(),
		Version:            src.Version,
		SourceContractId:   src.SourceContractId,
		Region:             src.Region,
		Dept:               src.Dept,
		PartyAContact:      src.PartyAContact,
		PartyAPhone:        src.PartyAPhone,
		PartyBName:         src.PartyBName,
		PartyBCredit:       src.PartyBCredit,
		PartyBKind:         src.PartyBKind,
		PartyBLegal:        src.PartyBLegal,
		PartyBContact:      src.PartyBContact,
		PartyBPhone:        src.PartyBPhone,
		PartyBBank:         src.PartyBBank,
		PartyBAccount:      src.PartyBAccount,
		PartyBRating:       src.PartyBRating,
		Importance:         src.Importance,
		StartDate:          src.StartDate,
		EndDate:            src.EndDate,
		Amount:             src.Amount,
		TaxRate:            src.TaxRate,
		Deposit:            src.Deposit,
		DiffValues:         src.DiffValues,
		Attachments:        src.GetAttachments(),
		Remark:             src.Remark,
		Status:             contractV1.ContractStatus_STATUS_DRAFT.Enum(),
	}

	// 付款方式/费用方向允许调用方覆写类型默认（出租改承租等场景）
	if src.PayMethod != nil && src.GetPayMethod() != contractV1.PayMethod_PAY_METHOD_UNSPECIFIED {
		draft.PayMethod = src.PayMethod
	}
	if src.FeeDirection != nil && src.GetFeeDirection() != contractV1.FeeDirection_DIRECTION_UNSPECIFIED {
		draft.FeeDirection = src.FeeDirection
	}
	if draft.PayMethod == nil {
		draft.PayMethod = contractV1.PayMethod_PAY_ONCE.Enum()
	}
	if draft.FeeDirection == nil {
		draft.FeeDirection = contractV1.FeeDirection_DIRECTION_PAYABLE.Enum()
	}
	if draft.Importance == nil {
		draft.Importance = contractV1.ContractImportance_IMPORTANCE_NORMAL.Enum()
	}
	if draft.Version == nil {
		draft.Version = trans.Ptr(uint32(1))
	}

	return draft, nil
}

// validateDraft 草稿业务校验（创建与更新共用）。
func (s *ContractService) validateDraft(d *contractV1.Contract) error {
	if strings.TrimSpace(d.GetPartyBName()) == "" {
		return contractV1.ErrorBadRequest("party B name is required")
	}
	if d.Amount == nil || d.GetAmount() <= 0 {
		return contractV1.ErrorBadRequest("contract amount must be positive")
	}
	if d.TaxRate != nil && d.GetTaxRate() > maxTaxRate {
		return contractV1.ErrorUnprocessableEntity("tax rate must not exceed %d%%", maxTaxRate)
	}
	if d.StartDate == nil || d.EndDate == nil {
		return contractV1.ErrorBadRequest("start date and end date are required")
	}
	if d.GetEndDate().AsTime().Before(d.GetStartDate().AsTime()) {
		return contractV1.ErrorUnprocessableEntity("end date must not be earlier than start date")
	}
	if d.Deposit != nil && d.GetDeposit() < 0 {
		return contractV1.ErrorUnprocessableEntity("deposit must not be negative")
	}
	if d.GetPayMethod() == contractV1.PayMethod_PAY_METHOD_UNSPECIFIED {
		return contractV1.ErrorUnprocessableEntity("pay method is required")
	}
	if d.GetFeeDirection() == contractV1.FeeDirection_DIRECTION_UNSPECIFIED {
		return contractV1.ErrorUnprocessableEntity("fee direction is required")
	}
	return nil
}

// —— CRUD ——

func (s *ContractService) List(ctx context.Context, req *paginationV1.PagingRequest) (*contractV1.ListContractsResponse, error) {
	return s.repo.List(ctx, req)
}

func (s *ContractService) Get(ctx context.Context, req *contractV1.GetContractRequest) (*contractV1.Contract, error) {
	return s.repo.Get(ctx, req)
}

func (s *ContractService) ListEvents(ctx context.Context, req *contractV1.ListContractEventsRequest) (*contractV1.ListContractEventsResponse, error) {
	if req.GetId() == 0 {
		return nil, contractV1.ErrorBadRequest("id is required")
	}
	events, err := s.repo.ListEvents(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &contractV1.ListContractEventsResponse{Items: events, Total: uint64(len(events))}, nil
}

func (s *ContractService) ListFees(ctx context.Context, req *contractV1.ListContractFeesRequest) (*contractV1.ListContractFeesResponse, error) {
	if req.GetId() == 0 {
		return nil, contractV1.ErrorBadRequest("id is required")
	}
	fees, err := s.repo.ListFees(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &contractV1.ListContractFeesResponse{Items: fees, Total: uint64(len(fees))}, nil
}

// Create 创建合同（草稿起步）：快照解析后落库，合同号由仓储派生。
func (s *ContractService) Create(ctx context.Context, req *contractV1.CreateContractRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, contractV1.ErrorBadRequest("invalid parameter")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	draft, err := s.buildContractDraft(ctx, req.Data)
	if err != nil {
		return nil, err
	}
	if err = s.validateDraft(draft); err != nil {
		return nil, err
	}
	draft.CreatedBy = trans.Ptr(operator)

	if _, err = s.repo.Create(ctx, draft); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Update 草稿编辑：仅草稿可改（其余状态走动作接口）。
func (s *ContractService) Update(ctx context.Context, req *contractV1.UpdateContractRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, contractV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return nil, contractV1.ErrorBadRequest("id is required")
	}

	existing, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if existing.GetStatus() != contractV1.ContractStatus_STATUS_DRAFT {
		return nil, contractV1.ErrorUnprocessableEntity(
			"only DRAFT contracts can be edited, use action endpoints otherwise")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	req.Data.UpdatedBy = trans.Ptr(operator)

	if err = s.repo.Update(ctx, req); err != nil {
		return nil, err
	}
	s.addUpdateEvent(ctx, req.GetId(), operator)
	return &emptypb.Empty{}, nil
}

// addUpdateEvent 草稿编辑留痕（失败只落日志不阻断：主数据已改，事件缺失可容忍）。
func (s *ContractService) addUpdateEvent(ctx context.Context, id, operator uint32) {
	if err := s.repo.AddEvent(ctx, id, &contractV1.ContractEvent{
		Action:    contractV1.ContractEventAction_ACTION_UPDATE.Enum(),
		CreatedBy: trans.Ptr(operator),
	}); err != nil {
		s.log.Errorf(ctx, "record contract update event failed: %v", err)
	}
}

// Delete 删除合同（仅草稿；软删，同事务清理费用与事件）。
func (s *ContractService) Delete(ctx context.Context, req *contractV1.DeleteContractRequest) (*emptypb.Empty, error) {
	if req.GetId() == 0 {
		return nil, contractV1.ErrorBadRequest("id is required")
	}

	existing, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if existing.GetStatus() != contractV1.ContractStatus_STATUS_DRAFT {
		return nil, contractV1.ErrorUnprocessableEntity("only DRAFT contracts can be deleted")
	}

	if err = s.repo.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// —— 动作（状态机，docs/connn/contract-type-and-contract-design.md §4.1）——

func (s *ContractService) entStatuses(in ...contractV1.ContractStatus) []contract.Status {
	out := make([]contract.Status, 0, len(in))
	for _, v := range in {
		switch v {
		case contractV1.ContractStatus_STATUS_DRAFT:
			out = append(out, contract.StatusDraft)
		case contractV1.ContractStatus_STATUS_REVIEWING:
			out = append(out, contract.StatusReviewing)
		case contractV1.ContractStatus_STATUS_EFFECTIVE:
			out = append(out, contract.StatusEffective)
		case contractV1.ContractStatus_STATUS_CHANGED:
			out = append(out, contract.StatusChanged)
		case contractV1.ContractStatus_STATUS_EXPIRED:
			out = append(out, contract.StatusExpired)
		case contractV1.ContractStatus_STATUS_TERMINATED:
			out = append(out, contract.StatusTerminated)
		case contractV1.ContractStatus_STATUS_INVALIDATED:
			out = append(out, contract.StatusInvalidated)
		}
	}
	return out
}

// eventFor 构造动作事件（from/to + 备注 + 操作人）。
func eventFor(action contractV1.ContractEventAction, from, to contractV1.ContractStatus, note string, operator uint32) *contractV1.ContractEvent {
	e := &contractV1.ContractEvent{
		Action:    action.Enum(),
		CreatedBy: trans.Ptr(operator),
	}
	if from != contractV1.ContractStatus_CONTRACT_STATUS_UNSPECIFIED {
		e.StatusFrom = trans.Ptr(from.String())
	}
	if to != contractV1.ContractStatus_CONTRACT_STATUS_UNSPECIFIED {
		e.StatusTo = trans.Ptr(to.String())
	}
	if note != "" {
		e.Note = trans.Ptr(note)
	}
	return e
}

func (s *ContractService) loadContract(ctx context.Context, id uint32) (*contractV1.Contract, error) {
	if id == 0 {
		return nil, contractV1.ErrorBadRequest("id is required")
	}
	return s.repo.GetByID(ctx, id)
}

// decomposeInput 合同快照 → 拆解输入。
func decomposeInput(c *contractV1.Contract) DecomposeInput {
	in := DecomposeInput{
		Amount:       c.GetAmount(),
		Deposit:      c.GetDeposit(),
		PayMethod:    c.GetPayMethod(),
		FeeDirection: c.GetFeeDirection(),
		StartDate:    c.GetStartDate().AsTime(),
		EndDate:      c.GetEndDate().AsTime(),
		PayNodes:     c.GetPayNodes(),
	}
	return in
}

// Submit 提交：DRAFT → REVIEWING；审批开关快照为 false 时直接生效并拆解费用。
func (s *ContractService) Submit(ctx context.Context, req *contractV1.SubmitContractRequest) (*emptypb.Empty, error) {
	c, err := s.loadContract(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if c.GetStatus() != contractV1.ContractStatus_STATUS_DRAFT {
		return nil, contractV1.ErrorConflict("submit is only allowed on DRAFT contracts")
	}
	if err = s.validateDraft(c); err != nil {
		return nil, err
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	if c.GetRequiresApproval() {
		err = s.repo.Transition(ctx, req.GetId(),
			s.entStatuses(contractV1.ContractStatus_STATUS_DRAFT),
			eventFor(contractV1.ContractEventAction_ACTION_SUBMIT,
				contractV1.ContractStatus_STATUS_DRAFT, contractV1.ContractStatus_STATUS_REVIEWING, req.GetNote(), operator),
			func(u *ent.ContractUpdate) {
				u.SetStatus(contract.StatusReviewing).SetUpdatedAt(now)
			})
		if err != nil {
			return nil, err
		}
		return &emptypb.Empty{}, nil
	}

	// 审批开关快照为 false：提交即生效（同工单 requires_approval 直进派单的口径）
	if err = s.transitionEffective(ctx, c,
		contractV1.ContractEventAction_ACTION_SUBMIT,
		contractV1.ContractStatus_STATUS_DRAFT, req.GetNote(), operator, 0); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// transitionEffective 生效落地：REVIEWING/DRAFT → EFFECTIVE + 拆解费用。
// 审核人仅 Approve 动作落（Submit 直生效时 reviewer=操作人）。
func (s *ContractService) transitionEffective(ctx context.Context, c *contractV1.Contract,
	action contractV1.ContractEventAction, from contractV1.ContractStatus, note string, operator, reviewer uint32) error {
	now := time.Now()
	err := s.repo.Transition(ctx, c.GetId(),
		s.entStatuses(from),
		eventFor(action, from, contractV1.ContractStatus_STATUS_EFFECTIVE, note, operator),
		func(u *ent.ContractUpdate) {
			u.SetStatus(contract.StatusEffective).
				SetUpdatedAt(now)
			if reviewer != 0 {
				u.SetReviewerID(reviewer).
					SetReviewedAt(now)
			}
		})
	if err != nil {
		return err
	}

	// 生效即拆解费用（快照驱动；失败留日志，可走「重新拆解」补齐）
	fees := DecomposeContractFees(decomposeInput(c))
	if len(fees) > 0 {
		if insertErr := s.repo.InsertFees(ctx, c.GetId(), fees); insertErr != nil {
			s.log.Errorf(ctx, "decompose fees for contract %d failed: %v", c.GetId(), insertErr)
			return nil //nolint:nilerr // 主流程已生效，费用拆解失败不回滚生效（重新拆解可补）
		}
	}
	return nil
}

// Approve 审核：通过 → EFFECTIVE（拆解费用）；驳回 → DRAFT（原因必填）。
func (s *ContractService) Approve(ctx context.Context, req *contractV1.ApproveContractRequest) (*emptypb.Empty, error) {
	c, err := s.loadContract(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if c.GetStatus() != contractV1.ContractStatus_STATUS_REVIEWING {
		return nil, contractV1.ErrorConflict("approve is only allowed on REVIEWING contracts")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	if req.GetPassed() {
		if err = s.transitionEffective(ctx, c,
			contractV1.ContractEventAction_ACTION_APPROVE,
			contractV1.ContractStatus_STATUS_REVIEWING, req.GetNote(), operator, operator); err != nil {
			return nil, err
		}
		return &emptypb.Empty{}, nil
	}

	// 驳回：回草稿可改再提，审核意见必填
	reason := req.GetNote()
	if strings.TrimSpace(reason) == "" {
		return nil, contractV1.ErrorBadRequest("reject reason is required")
	}
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(contractV1.ContractStatus_STATUS_REVIEWING),
		eventFor(contractV1.ContractEventAction_ACTION_REJECT,
			contractV1.ContractStatus_STATUS_REVIEWING, contractV1.ContractStatus_STATUS_DRAFT, reason, operator),
		func(u *ent.ContractUpdate) {
			u.SetStatus(contract.StatusDraft).
				SetReviewerID(operator).
				SetReviewedAt(now).
				SetReviewNote(reason).
				SetUpdatedAt(now)
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Change 变更：EFFECTIVE → CHANGED + 复制开新版草稿（v+1，回链旧版）；
// 新版沿用同类型重新解析快照（类型模板期间改过则以新版为准），返回新版。
func (s *ContractService) Change(ctx context.Context, req *contractV1.ChangeContractRequest) (*contractV1.Contract, error) {
	c, err := s.loadContract(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if c.GetStatus() != contractV1.ContractStatus_STATUS_EFFECTIVE {
		return nil, contractV1.ErrorConflict("change is only allowed on EFFECTIVE contracts")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	// 新版草稿：复制全部业务字段 + 版本 +1 + 回链；同类型重新走快照管线
	newVersion := c.GetVersion() + 1
	closeReason := "变更至 v" + strconv.Itoa(int(newVersion))
	src := proto2Draft(c)
	src.Version = trans.Ptr(newVersion)
	src.SourceContractId = trans.Ptr(c.GetId())
	// 变更说明带进新版备注（留痕于旧版关闭事件 + 新版备注）
	note := req.GetNote()
	if note != "" {
		if src.Remark == nil {
			src.Remark = trans.Ptr("")
		}
		src.Remark = trans.Ptr(src.GetRemark() + "【变更自 v" + strconv.Itoa(int(c.GetVersion())) + "：" + note + "】")
	}

	draft, err := s.buildContractDraft(ctx, src)
	if err != nil {
		return nil, err
	}
	draft.CreatedBy = trans.Ptr(operator)

	// 旧版关闭（CHANGED + 自动原因）→ 新版落库；关闭失败先回（并发守卫）
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(contractV1.ContractStatus_STATUS_EFFECTIVE),
		eventFor(contractV1.ContractEventAction_ACTION_CHANGE,
			contractV1.ContractStatus_STATUS_EFFECTIVE, contractV1.ContractStatus_STATUS_CHANGED,
			strings.TrimSpace(note+" "+closeReason), operator),
		func(u *ent.ContractUpdate) {
			u.SetStatus(contract.StatusChanged).
				SetTerminateReason(closeReason).
				SetUpdatedAt(now)
		})
	if err != nil {
		return nil, err
	}

	created, err := s.repo.Create(ctx, draft)
	if err != nil {
		return nil, err
	}
	// 新版首事件补一条 CHANGE 关联说明（时间线上能看出"从哪份变更来"）
	if err = s.repo.AddEvent(ctx, created.GetId(), &contractV1.ContractEvent{
		Action:    contractV1.ContractEventAction_ACTION_CHANGE.Enum(),
		Note:      trans.Ptr("由 " + c.GetContractNo() + " v" + strconv.Itoa(int(c.GetVersion())) + " 变更生成"),
		CreatedBy: trans.Ptr(operator),
	}); err != nil {
		s.log.Errorf(ctx, "record change event on new contract %d failed: %v", created.GetId(), err)
	}

	return created, nil
}

// proto2Draft 从存量合同复制业务字段组（快照组不复制——新版重新解析）。
func proto2Draft(c *contractV1.Contract) *contractV1.Contract {
	return &contractV1.Contract{
		ContractTypeId: c.ContractTypeId,
		Region:         c.Region,
		Dept:           c.Dept,
		PartyAContact:  c.PartyAContact,
		PartyAPhone:    c.PartyAPhone,
		PartyBName:     c.PartyBName,
		PartyBCredit:   c.PartyBCredit,
		PartyBKind:     c.PartyBKind,
		PartyBLegal:    c.PartyBLegal,
		PartyBContact:  c.PartyBContact,
		PartyBPhone:    c.PartyBPhone,
		PartyBBank:     c.PartyBBank,
		PartyBAccount:  c.PartyBAccount,
		PartyBRating:   c.PartyBRating,
		Importance:     c.Importance,
		StartDate:      c.StartDate,
		EndDate:        c.EndDate,
		Amount:         c.Amount,
		TaxRate:        c.TaxRate,
		PayMethod:      c.PayMethod,
		FeeDirection:   c.FeeDirection,
		Deposit:        c.Deposit,
		DiffValues:     c.DiffValues,
		Attachments:    c.GetAttachments(),
		Remark:         c.Remark,
	}
}

// Terminate 终止：EFFECTIVE → TERMINATED，原因必填。
func (s *ContractService) Terminate(ctx context.Context, req *contractV1.TerminateContractRequest) (*emptypb.Empty, error) {
	c, err := s.loadContract(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if c.GetStatus() != contractV1.ContractStatus_STATUS_EFFECTIVE {
		return nil, contractV1.ErrorConflict("terminate is only allowed on EFFECTIVE contracts")
	}
	if strings.TrimSpace(req.GetReason()) == "" {
		return nil, contractV1.ErrorBadRequest("terminate reason is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(contractV1.ContractStatus_STATUS_EFFECTIVE),
		eventFor(contractV1.ContractEventAction_ACTION_TERMINATE,
			contractV1.ContractStatus_STATUS_EFFECTIVE, contractV1.ContractStatus_STATUS_TERMINATED, req.GetReason(), operator),
		func(u *ent.ContractUpdate) {
			u.SetStatus(contract.StatusTerminated).
				SetTerminateReason(req.GetReason()).
				SetUpdatedAt(now)
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Invalidate 作废：DRAFT/REVIEWING/EFFECTIVE → INVALIDATED，原因必填。
func (s *ContractService) Invalidate(ctx context.Context, req *contractV1.InvalidateContractRequest) (*emptypb.Empty, error) {
	c, err := s.loadContract(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if isTerminalStatus(c.GetStatus()) {
		return nil, contractV1.ErrorConflict("invalidate is not allowed on terminal contracts")
	}
	if strings.TrimSpace(req.GetReason()) == "" {
		return nil, contractV1.ErrorBadRequest("invalidate reason is required")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	err = s.repo.Transition(ctx, req.GetId(),
		s.entStatuses(
			contractV1.ContractStatus_STATUS_DRAFT,
			contractV1.ContractStatus_STATUS_REVIEWING,
			contractV1.ContractStatus_STATUS_EFFECTIVE),
		eventFor(contractV1.ContractEventAction_ACTION_INVALIDATE,
			c.GetStatus(), contractV1.ContractStatus_STATUS_INVALIDATED, req.GetReason(), operator),
		func(u *ent.ContractUpdate) {
			u.SetStatus(contract.StatusInvalidated).
				SetTerminateReason(req.GetReason()).
				SetUpdatedAt(now)
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DecomposeFees 重新拆解（EFFECTIVE 可调）：删自动行后按当前快照重拆，手工行保留。
func (s *ContractService) DecomposeFees(ctx context.Context, req *contractV1.DecomposeFeesRequest) (*emptypb.Empty, error) {
	c, err := s.loadContract(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if c.GetStatus() != contractV1.ContractStatus_STATUS_EFFECTIVE {
		return nil, contractV1.ErrorConflict("decompose is only allowed on EFFECTIVE contracts")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	fees := DecomposeContractFees(decomposeInput(c))
	if err = s.repo.ReplaceAutoFees(ctx, c.GetId(), fees,
		eventFor(contractV1.ContractEventAction_ACTION_DECOMPOSE, 0, 0, req.GetNote(), operator)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// —— 费用收付 ——

// actualDateOf 实际收付日期：请求缺省 = 当天，带时刻的只取日期语义。
func actualDateOf(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Now()
	}
	return ts.AsTime()
}

// settleFee 费用行收付第一腿（PENDING → DONE）通用管线：方向/状态守卫 +
// SettleFee（并发 409）+ 主表事件。押金行同样按方向走付款/收款收第一腿；
// 第二腿（DONE → REFUNDED）只有 RefundFee。
func (s *ContractService) settleFee(ctx context.Context, feeID uint32,
	expectDirection contractV1.FeeDirection,
	action contractV1.ContractEventAction, actionLabel string,
	actual time.Time, from, to contractV1.FeeStatus) (*emptypb.Empty, error) {
	fee, err := s.repo.GetFeeByID(ctx, feeID)
	if err != nil {
		return nil, err
	}
	if fee.GetDirection() != expectDirection {
		return nil, contractV1.ErrorConflict("%s is only allowed on %s fees", actionLabel, expectDirection)
	}
	if fee.GetStatus() != from {
		return nil, contractV1.ErrorConflict("fee is in %s status, %s expects %s", fee.GetStatus(), actionLabel, from)
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	event := &contractV1.ContractEvent{
		Action:    action.Enum(),
		Note:      trans.Ptr(actionLabel + "：" + fee.GetName()),
		CreatedBy: trans.Ptr(operator),
	}
	err = s.repo.SettleFee(ctx, feeID, []contractfee.Status{contractfee.Status(from.String())}, event,
		func(u *ent.ContractFeeUpdate) {
			u.SetStatus(contractfee.Status(to.String())).
				SetActualDate(actual).
				SetUpdatedAt(time.Now())
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// PayFee 付款（应付行：PENDING → DONE）。
func (s *ContractService) PayFee(ctx context.Context, req *contractV1.PayFeeRequest) (*emptypb.Empty, error) {
	return s.settleFee(ctx, req.GetId(), contractV1.FeeDirection_DIRECTION_PAYABLE,
		contractV1.ContractEventAction_ACTION_FEE_PAY, "付款",
		actualDateOf(req.GetActualDate()),
		contractV1.FeeStatus_FEE_STATUS_PENDING, contractV1.FeeStatus_FEE_STATUS_DONE)
}

// ReceiveFee 收款（应收行：PENDING → DONE）。
func (s *ContractService) ReceiveFee(ctx context.Context, req *contractV1.ReceiveFeeRequest) (*emptypb.Empty, error) {
	return s.settleFee(ctx, req.GetId(), contractV1.FeeDirection_DIRECTION_RECEIVABLE,
		contractV1.ContractEventAction_ACTION_FEE_RECEIVE, "收款",
		actualDateOf(req.GetActualDate()),
		contractV1.FeeStatus_FEE_STATUS_PENDING, contractV1.FeeStatus_FEE_STATUS_DONE)
}

// RefundFee 押金退款（押金行：DONE → REFUNDED，收付两向均可退）。
func (s *ContractService) RefundFee(ctx context.Context, req *contractV1.RefundFeeRequest) (*emptypb.Empty, error) {
	fee, err := s.repo.GetFeeByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if fee.GetCategory() != contractV1.FeeCategory_CATEGORY_DEPOSIT {
		return nil, contractV1.ErrorConflict("refund is only allowed on deposit fees")
	}
	if fee.GetStatus() != contractV1.FeeStatus_FEE_STATUS_DONE {
		return nil, contractV1.ErrorConflict("refund expects fee in DONE (待退款) status")
	}

	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}

	event := &contractV1.ContractEvent{
		Action:    contractV1.ContractEventAction_ACTION_FEE_REFUND.Enum(),
		Note:      trans.Ptr("押金退款：" + fee.GetName()),
		CreatedBy: trans.Ptr(operator),
	}
	err = s.repo.SettleFee(ctx, req.GetId(), []contractfee.Status{contractfee.StatusDONE}, event,
		func(u *ent.ContractFeeUpdate) {
			u.SetStatus(contractfee.StatusRefunded).
				SetActualDate(actualDateOf(req.GetActualDate())).
				SetUpdatedAt(time.Now())
		})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
