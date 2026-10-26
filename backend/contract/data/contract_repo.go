package data

import (
	"context"
	"fmt"
	"time"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/timeutil"
	"github.com/tx7do/go-utils/trans"

	contractV1 "go-wind-admin/contract/api/gen/go/contract/service/v1"

	"go-wind-admin/contract/data/ent"
	"go-wind-admin/contract/data/ent/contract"
	"go-wind-admin/contract/data/ent/contractevent"
	"go-wind-admin/contract/data/ent/contractfee"
	"go-wind-admin/contract/data/ent/predicate"
	"go-wind-admin/contract/data/ent/schema"
)

type ContractRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper

	statusConverter     *mapper.EnumTypeConverter[contractV1.ContractStatus, contract.Status]
	payMethodConverter  *mapper.EnumTypeConverter[contractV1.PayMethod, contract.PayMethod]
	directionConverter  *mapper.EnumTypeConverter[contractV1.FeeDirection, contract.FeeDirection]
	partyBKindConverter *mapper.EnumTypeConverter[contractV1.PartyBKind, contract.PartyBKind]
	importanceConverter *mapper.EnumTypeConverter[contractV1.ContractImportance, contract.Importance]

	feeCategoryConverter *mapper.EnumTypeConverter[contractV1.FeeCategory, contractfee.Category]
	feeStatusConverter   *mapper.EnumTypeConverter[contractV1.FeeStatus, contractfee.Status]
	feeDirectionCNV      *mapper.EnumTypeConverter[contractV1.FeeDirection, contractfee.Direction]

	eventActionConverter *mapper.EnumTypeConverter[contractV1.ContractEventAction, contractevent.Action]

	mapper      *mapper.CopierMapper[contractV1.Contract, ent.Contract]
	feeMapper   *mapper.CopierMapper[contractV1.ContractFee, ent.ContractFee]
	eventMapper *mapper.CopierMapper[contractV1.ContractEvent, ent.ContractEvent]
}

func NewContractRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *ContractRepo {
	repo := &ContractRepo{
		log:         ctx.NewLoggerHelper("contract/repo/contract"),
		entClient:   entClient,
		mapper:      mapper.NewCopierMapper[contractV1.Contract, ent.Contract](),
		feeMapper:   mapper.NewCopierMapper[contractV1.ContractFee, ent.ContractFee](),
		eventMapper: mapper.NewCopierMapper[contractV1.ContractEvent, ent.ContractEvent](),
	}

	repo.statusConverter = mapper.NewEnumTypeConverter[contractV1.ContractStatus, contract.Status](contractV1.ContractStatus_name, contractV1.ContractStatus_value)
	repo.payMethodConverter = mapper.NewEnumTypeConverter[contractV1.PayMethod, contract.PayMethod](contractV1.PayMethod_name, contractV1.PayMethod_value)
	repo.directionConverter = mapper.NewEnumTypeConverter[contractV1.FeeDirection, contract.FeeDirection](contractV1.FeeDirection_name, contractV1.FeeDirection_value)
	repo.partyBKindConverter = mapper.NewEnumTypeConverter[contractV1.PartyBKind, contract.PartyBKind](contractV1.PartyBKind_name, contractV1.PartyBKind_value)
	repo.importanceConverter = mapper.NewEnumTypeConverter[contractV1.ContractImportance, contract.Importance](contractV1.ContractImportance_name, contractV1.ContractImportance_value)
	repo.feeCategoryConverter = mapper.NewEnumTypeConverter[contractV1.FeeCategory, contractfee.Category](contractV1.FeeCategory_name, contractV1.FeeCategory_value)
	repo.feeStatusConverter = mapper.NewEnumTypeConverter[contractV1.FeeStatus, contractfee.Status](contractV1.FeeStatus_name, contractV1.FeeStatus_value)
	repo.feeDirectionCNV = mapper.NewEnumTypeConverter[contractV1.FeeDirection, contractfee.Direction](contractV1.FeeDirection_name, contractV1.FeeDirection_value)
	repo.eventActionConverter = mapper.NewEnumTypeConverter[contractV1.ContractEventAction, contractevent.Action](contractV1.ContractEventAction_name, contractV1.ContractEventAction_value)

	repo.init()

	return repo
}

func (r *ContractRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.statusConverter.NewConverterPair())
	r.mapper.AppendConverters(r.payMethodConverter.NewConverterPair())
	r.mapper.AppendConverters(r.directionConverter.NewConverterPair())
	r.mapper.AppendConverters(r.partyBKindConverter.NewConverterPair())
	r.mapper.AppendConverters(r.importanceConverter.NewConverterPair())
	// 差异字段快照/填值：proto ↔ jsonb（数组同类型仓储同款，map 为同构拷贝）
	r.mapper.AppendConverters(copierutil.NewGenericTypeConverterPair(
		[]*contractV1.ContractDiffField{},
		[]schema.ContractDiffField{},
		func(items []*contractV1.ContractDiffField) []schema.ContractDiffField {
			return diffFieldsToEntity(items)
		},
		func(items []schema.ContractDiffField) []*contractV1.ContractDiffField {
			return diffFieldsToDTO(items)
		},
	))
	// 付款节点快照：proto repeated message ↔ jsonb 结构数组
	r.mapper.AppendConverters(copierutil.NewGenericTypeConverterPair(
		[]*contractV1.ContractPayNode{},
		[]schema.ContractPayNode{},
		func(items []*contractV1.ContractPayNode) []schema.ContractPayNode {
			return payNodesToEntity(items)
		},
		func(items []schema.ContractPayNode) []*contractV1.ContractPayNode {
			return payNodesToDTO(items)
		},
	))

	r.feeMapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.feeMapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.feeMapper.AppendConverters(r.feeCategoryConverter.NewConverterPair())
	r.feeMapper.AppendConverters(r.feeStatusConverter.NewConverterPair())
	r.feeMapper.AppendConverters(r.feeDirectionCNV.NewConverterPair())

	r.eventMapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.eventMapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.eventMapper.AppendConverters(r.eventActionConverter.NewConverterPair())
}

// DeriveContractNo 合同号 = CT- + 8 位零填自增 id（单调性与全局序列等价，
// 不引入 ent 之外的结构对象——漂移守卫是严格 diff，独立业务序列会永远红）。
func DeriveContractNo(id uint32) string {
	return fmt.Sprintf("CT-%08d", id)
}

func (r *ContractRepo) List(ctx context.Context, req *paginationV1.PagingRequest) (*contractV1.ListContractsResponse, error) {
	if req == nil {
		return nil, contractV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().Contract.Query()

	// 默认按创建序倒序（最新合同在前）；调用方显式排序时尊重显式值
	if len(req.GetSorting()) == 0 && (req.GetOrderBy() == "" || req.GetOrderBy() == "[]") {
		builder.Order(ent.Desc(contract.FieldCreatedAt))
	}

	repository := entCrud.NewRepository[
		ent.ContractQuery, ent.ContractSelect,
		ent.ContractCreate, ent.ContractCreateBulk,
		ent.ContractUpdate, ent.ContractUpdateOne,
		ent.ContractDelete,
		predicate.Contract,
		contractV1.Contract, ent.Contract,
	](r.mapper)

	ret, err := repository.ListWithPaging(ctx, builder, builder.Clone(), req)
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return &contractV1.ListContractsResponse{Total: 0, Items: nil}, nil
	}

	return &contractV1.ListContractsResponse{
		Total: ret.Total,
		Items: ret.Items,
	}, nil
}

func (r *ContractRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.entClient.Client().Contract.Query().
		Where(contract.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query contract exist failed: %s", err.Error())
		return false, contractV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *ContractRepo) Get(ctx context.Context, req *contractV1.GetContractRequest) (*contractV1.Contract, error) {
	if req == nil {
		return nil, contractV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.entClient.Client().Contract.Query().
		Where(contract.IDEQ(req.GetId())).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, contractV1.ErrorNotFound("contract not found")
		}
		r.log.Errorf(ctx, "query contract failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("query contract failed")
	}

	return r.mapper.ToDTO(entity), nil
}

// GetByID 按主键取单条；不存在返回 ErrorNotFound。
func (r *ContractRepo) GetByID(ctx context.Context, id uint32) (*contractV1.Contract, error) {
	entity, err := r.entClient.Client().Contract.Query().
		Where(contract.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, contractV1.ErrorNotFound("contract not found")
		}
		r.log.Errorf(ctx, "query contract by id failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("query contract failed")
	}

	return r.mapper.ToDTO(entity), nil
}

// GetEntityByID 按主键取实体（到期扫描直接读原始列）。
func (r *ContractRepo) GetEntityByID(ctx context.Context, id uint32) (*ent.Contract, error) {
	entity, err := r.entClient.Client().Contract.Query().
		Where(contract.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, contractV1.ErrorNotFound("contract not found")
		}
		r.log.Errorf(ctx, "query contract entity by id failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("query contract failed")
	}
	return entity, nil
}

// Create 创建合同（单事务）：插入主表 → 派生合同号 → 落创建事件。
// data 为服务层已解析快照的草稿；租户由 TenantID Privacy 按登录上下文写入。
func (r *ContractRepo) Create(ctx context.Context, data *contractV1.Contract) (*contractV1.Contract, error) {
	if data == nil {
		return nil, contractV1.ErrorBadRequest("invalid parameter")
	}

	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin contract create transaction failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback contract create transaction failed: %v", rollbackErr)
		}
	}()

	d := data
	builder := tx.Contract.Create().
		SetNillableVersion(d.Version).
		SetNillableSourceContractID(d.SourceContractId).
		SetNillableContractTypeID(d.ContractTypeId).
		SetNillableContractTypeCode(d.ContractTypeCode).
		SetNillableContractTypeName(d.ContractTypeName).
		SetNillableRequiresApproval(d.RequiresApproval).
		SetNillableStatus(r.statusConverter.ToEntity(d.Status)).
		SetNillableRegion(d.Region).
		SetNillableDept(d.Dept).
		SetNillablePartyAContact(d.PartyAContact).
		SetNillablePartyAPhone(d.PartyAPhone).
		SetNillablePartyBName(d.PartyBName).
		SetNillablePartyBCredit(d.PartyBCredit).
		SetNillablePartyBKind(r.partyBKindConverter.ToEntity(d.PartyBKind)).
		SetNillablePartyBLegal(d.PartyBLegal).
		SetNillablePartyBContact(d.PartyBContact).
		SetNillablePartyBPhone(d.PartyBPhone).
		SetNillablePartyBBank(d.PartyBBank).
		SetNillablePartyBAccount(d.PartyBAccount).
		SetNillablePartyBRating(d.PartyBRating).
		SetNillableImportance(r.importanceConverter.ToEntity(d.Importance)).
		SetNillableStartDate(timeutil.TimestamppbToTime(d.StartDate)).
		SetNillableEndDate(timeutil.TimestamppbToTime(d.EndDate)).
		SetNillableAmount(d.Amount).
		SetNillableTaxRate(d.TaxRate).
		SetNillablePayMethod(r.payMethodConverter.ToEntity(d.PayMethod)).
		SetNillableFeeDirection(r.directionConverter.ToEntity(d.FeeDirection)).
		SetNillableDeposit(d.Deposit).
		SetNillableRemark(d.Remark).
		SetNillableCreatedBy(d.CreatedBy).
		SetCreatedAt(time.Now())

	if len(d.GetAttachments()) > 0 {
		builder.SetAttachments(d.GetAttachments())
	}
	if fields := diffFieldsToEntity(d.GetDiffSchema()); len(fields) > 0 {
		builder.SetDiffSchema(fields)
	}
	if len(d.GetDiffValues()) > 0 {
		builder.SetDiffValues(d.GetDiffValues())
	}
	if nodes := payNodesToEntity(d.GetPayNodes()); len(nodes) > 0 {
		builder.SetPayNodes(nodes)
	}

	entity, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf(ctx, "insert contract failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("insert contract failed")
	}

	// 合同号 = CT- + 零填 id（同事务派生，见 DeriveContractNo）
	if err = tx.Contract.UpdateOneID(entity.ID).
		SetContractNo(DeriveContractNo(entity.ID)).
		SetUpdatedAt(time.Now()).
		Exec(ctx); err != nil {
		r.log.Errorf(ctx, "derive contract no failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("derive contract no failed")
	}

	// 创建事件（status_to 补齐为落库后状态）
	event := &contractV1.ContractEvent{
		Action:    contractV1.ContractEventAction_ACTION_CREATE.Enum(),
		CreatedBy: d.CreatedBy,
	}
	if v := r.statusConverter.ToDTO(entity.Status); v != nil {
		event.StatusTo = trans.Ptr(v.String())
	}
	if err = r.insertEventTx(ctx, tx, entity.ID, event); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit contract create transaction failed: %v", err)
		return nil, contractV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true

	return r.GetByID(ctx, entity.ID)
}

// Update 草稿编辑（状态守卫由服务层校验后在此落地，仅更新业务字段组）。
func (r *ContractRepo) Update(ctx context.Context, req *contractV1.UpdateContractRequest) error {
	if req == nil || req.Data == nil {
		return contractV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return contractV1.ErrorBadRequest("id is required")
	}

	d := req.Data
	return r.Patch(ctx, req.GetId(), func(u *ent.ContractUpdateOne) {
		u.SetUpdatedAt(time.Now())
		if d.Region != nil {
			u.SetNillableRegion(d.Region)
		}
		if d.Dept != nil {
			u.SetNillableDept(d.Dept)
		}
		if d.PartyAContact != nil {
			u.SetNillablePartyAContact(d.PartyAContact)
		}
		if d.PartyAPhone != nil {
			u.SetNillablePartyAPhone(d.PartyAPhone)
		}
		if d.PartyBName != nil {
			u.SetNillablePartyBName(d.PartyBName)
		}
		if d.PartyBCredit != nil {
			u.SetNillablePartyBCredit(d.PartyBCredit)
		}
		if d.PartyBKind != nil {
			u.SetNillablePartyBKind(r.partyBKindConverter.ToEntity(d.PartyBKind))
		}
		if d.PartyBLegal != nil {
			u.SetNillablePartyBLegal(d.PartyBLegal)
		}
		if d.PartyBContact != nil {
			u.SetNillablePartyBContact(d.PartyBContact)
		}
		if d.PartyBPhone != nil {
			u.SetNillablePartyBPhone(d.PartyBPhone)
		}
		if d.PartyBBank != nil {
			u.SetNillablePartyBBank(d.PartyBBank)
		}
		if d.PartyBAccount != nil {
			u.SetNillablePartyBAccount(d.PartyBAccount)
		}
		if d.PartyBRating != nil {
			u.SetNillablePartyBRating(d.PartyBRating)
		}
		if d.Importance != nil {
			u.SetNillableImportance(r.importanceConverter.ToEntity(d.Importance))
		}
		if d.StartDate != nil {
			u.SetNillableStartDate(timeutil.TimestamppbToTime(d.StartDate))
		}
		if d.EndDate != nil {
			u.SetNillableEndDate(timeutil.TimestamppbToTime(d.EndDate))
		}
		if d.Amount != nil {
			u.SetNillableAmount(d.Amount)
		}
		if d.TaxRate != nil {
			u.SetNillableTaxRate(d.TaxRate)
		}
		if d.PayMethod != nil {
			u.SetNillablePayMethod(r.payMethodConverter.ToEntity(d.PayMethod))
		}
		if d.FeeDirection != nil {
			u.SetNillableFeeDirection(r.directionConverter.ToEntity(d.FeeDirection))
		}
		if d.Deposit != nil {
			u.SetNillableDeposit(d.Deposit)
		}
		if d.Remark != nil {
			u.SetNillableRemark(d.Remark)
		}
		if len(d.GetAttachments()) > 0 {
			u.SetAttachments(d.GetAttachments())
		}
		if len(d.GetDiffValues()) > 0 {
			u.SetDiffValues(d.GetDiffValues())
		}
		if d.UpdatedBy != nil {
			u.SetNillableUpdatedBy(d.UpdatedBy)
		}
	})
}

// Patch 无状态变更的字段更新（审核人落库等）：不带状态守卫，但限定单据存在。
func (r *ContractRepo) Patch(ctx context.Context, id uint32, setFunc func(u *ent.ContractUpdateOne)) error {
	builder := r.entClient.Client().Contract.UpdateOneID(id)
	setFunc(builder)
	if _, err := builder.Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return contractV1.ErrorNotFound("contract not found")
		}
		r.log.Errorf(ctx, "patch contract %d failed: %s", id, err.Error())
		return contractV1.ErrorInternalServerError("update contract failed")
	}
	return nil
}

// Transition 状态迁移 + 事件落库（单事务）：WHERE id AND status IN(from)，
// 命中 0 行 = 状态已被并发动作改变 → 409 回滚。setFunc 内做该动作的字段落库。
func (r *ContractRepo) Transition(ctx context.Context, id uint32, from []contract.Status, event *contractV1.ContractEvent, setFunc func(u *ent.ContractUpdate)) error {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin contract action transaction failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback contract action transaction failed: %v", rollbackErr)
		}
	}()

	builder := tx.Contract.Update().
		Where(contract.IDEQ(id), contract.StatusIn(from...))
	setFunc(builder)
	affected, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf(ctx, "transition contract %d failed: %s", id, err.Error())
		return contractV1.ErrorInternalServerError("update contract failed")
	}
	if affected == 0 {
		// 未命中：并发动作已改状态。事务无写入但必须回滚释放连接
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback unmatched contract transition failed: %v", rollbackErr)
		}
		committed = true
		return contractV1.ErrorConflict("contract status changed by a concurrent action, reload and retry")
	}

	if event != nil {
		if err = r.insertEventTx(ctx, tx, id, event); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit contract action transaction failed: %v", err)
		return contractV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true
	return nil
}

// AddEvent 追加不改状态的事件（费用收付/重新拆解记录）。
func (r *ContractRepo) AddEvent(ctx context.Context, contractID uint32, event *contractV1.ContractEvent) error {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin contract event transaction failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback contract event transaction failed: %v", rollbackErr)
		}
	}()
	if err = r.insertEventTx(ctx, tx, contractID, event); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit contract event transaction failed: %v", err)
		return contractV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true
	return nil
}

func (r *ContractRepo) insertEventTx(ctx context.Context, tx *ent.Tx, contractID uint32, event *contractV1.ContractEvent) error {
	eb := tx.ContractEvent.Create().
		SetContractID(contractID).
		SetAction(*r.eventActionConverter.ToEntity(event.Action)).
		SetCreatedAt(time.Now())
	if event.GetCreatedBy() != 0 {
		eb.SetCreatedBy(event.GetCreatedBy())
	}
	if event.GetStatusFrom() != "" {
		eb.SetStatusFrom(event.GetStatusFrom())
	}
	if event.GetStatusTo() != "" {
		eb.SetStatusTo(event.GetStatusTo())
	}
	if event.GetNote() != "" {
		eb.SetNote(event.GetNote())
	}
	// 事件随主表租户
	if c, err := tx.Contract.Get(ctx, contractID); err == nil && c.TenantID != nil {
		eb.SetTenantID(*c.TenantID)
	}
	if err := eb.Exec(ctx); err != nil {
		r.log.Errorf(ctx, "insert contract event failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("insert contract event failed")
	}
	return nil
}

// Delete 软删（仅草稿，服务层守卫）：主表 + 费用明细 + 事件同事务标记删除。
func (r *ContractRepo) Delete(ctx context.Context, id uint32) error {
	if id == 0 {
		return contractV1.ErrorBadRequest("id is required")
	}

	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin contract delete transaction failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback contract delete transaction failed: %v", rollbackErr)
		}
	}()

	now := time.Now()
	if _, err = tx.Contract.Update().
		Where(contract.IDEQ(id)).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "delete contract failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("delete contract failed")
	}
	if _, err = tx.ContractFee.Update().
		Where(contractfee.ContractIDEQ(id)).
		SetDeletedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "delete contract fees failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("delete contract fees failed")
	}
	if _, err = tx.ContractEvent.Update().
		Where(contractevent.ContractIDEQ(id)).
		SetDeletedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "delete contract events failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("delete contract events failed")
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit contract delete transaction failed: %v", err)
		return contractV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true
	return nil
}

// —— 子资源：事件与费用 ——

func (r *ContractRepo) ListEvents(ctx context.Context, contractID uint32) ([]*contractV1.ContractEvent, error) {
	entities, err := r.entClient.Client().ContractEvent.Query().
		Where(contractevent.ContractIDEQ(contractID)).
		Order(ent.Desc(contractevent.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf(ctx, "list contract events failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("query contract events failed")
	}
	if len(entities) == 0 {
		return nil, nil
	}

	out := make([]*contractV1.ContractEvent, 0, len(entities))
	for _, entity := range entities {
		out = append(out, r.eventMapper.ToDTO(entity))
	}
	return out, nil
}

func (r *ContractRepo) ListFees(ctx context.Context, contractID uint32) ([]*contractV1.ContractFee, error) {
	entities, err := r.entClient.Client().ContractFee.Query().
		Where(contractfee.ContractIDEQ(contractID)).
		Order(ent.Asc(contractfee.FieldPlanDate), ent.Asc(contractfee.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf(ctx, "list contract fees failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("query contract fees failed")
	}
	if len(entities) == 0 {
		return nil, nil
	}

	out := make([]*contractV1.ContractFee, 0, len(entities))
	for _, entity := range entities {
		out = append(out, r.feeMapper.ToDTO(entity))
	}
	return out, nil
}

// GetFeeByID 按主键取费用行（收付动作前校验）。
func (r *ContractRepo) GetFeeByID(ctx context.Context, id uint32) (*contractV1.ContractFee, error) {
	entity, err := r.entClient.Client().ContractFee.Query().
		Where(contractfee.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, contractV1.ErrorNotFound("contract fee not found")
		}
		r.log.Errorf(ctx, "query contract fee failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("query contract fee failed")
	}
	return r.feeMapper.ToDTO(entity), nil
}

// SettleFee 费用行收付/退款（状态守卫 + 主表事件，单事务）：
// WHERE id AND status IN(from)，命中 0 行 = 并发收付 → 409。
func (r *ContractRepo) SettleFee(ctx context.Context, feeID uint32, from []contractfee.Status, event *contractV1.ContractEvent, setFunc func(u *ent.ContractFeeUpdate)) error {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin fee settle transaction failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback fee settle transaction failed: %v", rollbackErr)
		}
	}()

	builder := tx.ContractFee.Update().
		Where(contractfee.IDEQ(feeID), contractfee.StatusIn(from...))
	setFunc(builder)
	affected, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf(ctx, "settle contract fee %d failed: %s", feeID, err.Error())
		return contractV1.ErrorInternalServerError("update contract fee failed")
	}
	if affected == 0 {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback unmatched fee settle failed: %v", rollbackErr)
		}
		committed = true
		return contractV1.ErrorConflict("contract fee status changed by a concurrent action, reload and retry")
	}

	feeEntity, err := tx.ContractFee.Get(ctx, feeID)
	if err != nil {
		r.log.Errorf(ctx, "reload contract fee %d failed: %s", feeID, err.Error())
		return contractV1.ErrorInternalServerError("reload contract fee failed")
	}
	if feeEntity.ContractID == nil {
		return contractV1.ErrorInternalServerError("contract fee has no contract")
	}

	if event != nil {
		if err = r.insertEventTx(ctx, tx, *feeEntity.ContractID, event); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit fee settle transaction failed: %v", err)
		return contractV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true
	return nil
}

// ReplaceAutoFees 重新拆解（单事务）：软删 auto_generated=true 的费用行，
// 插入新费用行集，落 DECOMPOSE 事件。手工行（auto_generated=false）保留。
func (r *ContractRepo) ReplaceAutoFees(ctx context.Context, contractID uint32, fees []*contractV1.ContractFee, event *contractV1.ContractEvent) error {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin fee decompose transaction failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback fee decompose transaction failed: %v", rollbackErr)
		}
	}()

	main, err := tx.Contract.Get(ctx, contractID)
	if err != nil {
		if ent.IsNotFound(err) {
			return contractV1.ErrorNotFound("contract not found")
		}
		r.log.Errorf(ctx, "load contract for decompose failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("load contract failed")
	}

	now := time.Now()
	if _, err = tx.ContractFee.Update().
		Where(contractfee.ContractIDEQ(contractID), contractfee.AutoGeneratedEQ(true)).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		r.log.Errorf(ctx, "delete auto-generated fees failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("delete auto fees failed")
	}

	for _, fee := range fees {
		if fee == nil {
			continue
		}
		fb := tx.ContractFee.Create().
			SetContractID(contractID).
			SetName(fee.GetName()).
			SetCategory(*r.feeCategoryConverter.ToEntity(fee.Category)).
			SetDirection(*r.feeDirectionCNV.ToEntity(fee.Direction)).
			SetStatus(*r.feeStatusConverter.ToEntity(fee.Status)).
			SetNillableAmount(fee.Amount).
			SetNillablePlanDate(timeutil.TimestamppbToTime(fee.PlanDate)).
			SetAutoGenerated(fee.GetAutoGenerated()).
			SetCreatedAt(now)
		if fee.GetRemark() != "" {
			fb.SetRemark(fee.GetRemark())
		}
		if main.TenantID != nil {
			fb.SetTenantID(*main.TenantID)
		}
		if err = fb.Exec(ctx); err != nil {
			r.log.Errorf(ctx, "insert decomposed fee failed: %s", err.Error())
			return contractV1.ErrorInternalServerError("insert decomposed fee failed")
		}
	}

	if event != nil {
		if err = r.insertEventTx(ctx, tx, contractID, event); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit fee decompose transaction failed: %v", err)
		return contractV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true
	return nil
}

// InsertFees 生效时首拆费用（单事务批量插入；不落事件——生效事件已在转移里落）。
func (r *ContractRepo) InsertFees(ctx context.Context, contractID uint32, fees []*contractV1.ContractFee) error {
	if len(fees) == 0 {
		return nil
	}

	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin fee insert transaction failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("begin transaction failed")
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf(ctx, "rollback fee insert transaction failed: %v", rollbackErr)
		}
	}()

	main, err := tx.Contract.Get(ctx, contractID)
	if err != nil {
		if ent.IsNotFound(err) {
			return contractV1.ErrorNotFound("contract not found")
		}
		r.log.Errorf(ctx, "load contract for fee insert failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("load contract failed")
	}

	now := time.Now()
	for _, fee := range fees {
		if fee == nil {
			continue
		}
		fb := tx.ContractFee.Create().
			SetContractID(contractID).
			SetName(fee.GetName()).
			SetCategory(*r.feeCategoryConverter.ToEntity(fee.Category)).
			SetDirection(*r.feeDirectionCNV.ToEntity(fee.Direction)).
			SetStatus(*r.feeStatusConverter.ToEntity(fee.Status)).
			SetNillableAmount(fee.Amount).
			SetNillablePlanDate(timeutil.TimestamppbToTime(fee.PlanDate)).
			SetAutoGenerated(fee.GetAutoGenerated()).
			SetCreatedAt(now)
		if fee.GetRemark() != "" {
			fb.SetRemark(fee.GetRemark())
		}
		if main.TenantID != nil {
			fb.SetTenantID(*main.TenantID)
		}
		if err = fb.Exec(ctx); err != nil {
			r.log.Errorf(ctx, "insert contract fee failed: %s", err.Error())
			return contractV1.ErrorInternalServerError("insert contract fee failed")
		}
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit fee insert transaction failed: %v", err)
		return contractV1.ErrorInternalServerError("commit transaction failed")
	}
	committed = true
	return nil
}

// ListExpiredEffective 到期扫描：执行中且结束日期早于给定时刻的合同（逐条转移）。
func (r *ContractRepo) ListExpiredEffective(ctx context.Context, before time.Time) ([]*ent.Contract, error) {
	entities, err := r.entClient.Client().Contract.Query().
		Where(
			contract.StatusEQ(contract.StatusEffective),
			contract.EndDateLT(before),
		).
		All(ctx)
	if err != nil {
		r.log.Errorf(ctx, "list expired effective contracts failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("query expired contracts failed")
	}
	return entities, nil
}

// CountByContractTypeIDs 引用给定类型的合同数（类型删除守卫）。
func (r *ContractRepo) CountByContractTypeIDs(ctx context.Context, typeIDs ...uint32) (int, error) {
	if len(typeIDs) == 0 {
		return 0, nil
	}
	count, err := r.entClient.Client().Contract.Query().
		Where(contract.ContractTypeIDIn(typeIDs...)).
		Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "count contracts by type ids failed: %s", err.Error())
		return 0, contractV1.ErrorInternalServerError("query count failed")
	}
	return count, nil
}
