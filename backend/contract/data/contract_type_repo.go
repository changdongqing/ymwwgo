package data

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/mapper"

	contractV1 "go-wind-admin/contract/api/gen/go/contract/service/v1"

	"go-wind-admin/contract/data/ent"
	"go-wind-admin/contract/data/ent/predicate"
	"go-wind-admin/contract/data/ent/schema"
	"go-wind-admin/contract/data/ent/contracttype"
)

type ContractTypeRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper

	levelConverter     *mapper.EnumTypeConverter[contractV1.ContractLevel, contracttype.Level]
	payMethodConverter *mapper.EnumTypeConverter[contractV1.PayMethod, contracttype.PayMethod]
	directionConverter *mapper.EnumTypeConverter[contractV1.FeeDirection, contracttype.FeeDirection]

	mapper *mapper.CopierMapper[contractV1.ContractType, ent.ContractType]

	repository *entCrud.Repository[
		ent.ContractTypeQuery, ent.ContractTypeSelect,
		ent.ContractTypeCreate, ent.ContractTypeCreateBulk,
		ent.ContractTypeUpdate, ent.ContractTypeUpdateOne,
		ent.ContractTypeDelete,
		predicate.ContractType,
		contractV1.ContractType, ent.ContractType,
	]
}

func NewContractTypeRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *ContractTypeRepo {
	repo := &ContractTypeRepo{
		log:                ctx.NewLoggerHelper("contract-type/repo/contract"),
		entClient:          entClient,
		mapper:             mapper.NewCopierMapper[contractV1.ContractType, ent.ContractType](),
		levelConverter:     mapper.NewEnumTypeConverter[contractV1.ContractLevel, contracttype.Level](contractV1.ContractLevel_name, contractV1.ContractLevel_value),
		payMethodConverter: mapper.NewEnumTypeConverter[contractV1.PayMethod, contracttype.PayMethod](contractV1.PayMethod_name, contractV1.PayMethod_value),
		directionConverter: mapper.NewEnumTypeConverter[contractV1.FeeDirection, contracttype.FeeDirection](contractV1.FeeDirection_name, contractV1.FeeDirection_value),
	}

	repo.init()

	return repo
}

func (r *ContractTypeRepo) init() {
	r.repository = entCrud.NewRepository[
		ent.ContractTypeQuery, ent.ContractTypeSelect,
		ent.ContractTypeCreate, ent.ContractTypeCreateBulk,
		ent.ContractTypeUpdate, ent.ContractTypeUpdateOne,
		ent.ContractTypeDelete,
		predicate.ContractType,
		contractV1.ContractType, ent.ContractType,
	](r.mapper)

	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.levelConverter.NewConverterPair())
	r.mapper.AppendConverters(r.payMethodConverter.NewConverterPair())
	r.mapper.AppendConverters(r.directionConverter.NewConverterPair())
	// 付款节点：proto repeated message ↔ jsonb 结构数组
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
	// 差异字段定义：proto repeated message ↔ jsonb 结构数组（合同 diff_schema 同款复用）
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
}

// payNodesToEntity / payNodesToDTO 付款节点 proto ↔ jsonb 结构互转。
func payNodesToEntity(items []*contractV1.ContractPayNode) []schema.ContractPayNode {
	if len(items) == 0 {
		return nil
	}
	out := make([]schema.ContractPayNode, 0, len(items))
	for _, item := range items {
		if item == nil || item.GetName() == "" {
			continue
		}
		out = append(out, schema.ContractPayNode{Name: item.GetName(), RatioPercent: item.GetRatioPercent()})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func payNodesToDTO(items []schema.ContractPayNode) []*contractV1.ContractPayNode {
	if len(items) == 0 {
		return nil
	}
	out := make([]*contractV1.ContractPayNode, 0, len(items))
	for _, item := range items {
		out = append(out, &contractV1.ContractPayNode{Name: item.Name, RatioPercent: item.RatioPercent})
	}
	return out
}

// diffFieldsToEntity / diffFieldsToDTO 差异字段定义 proto ↔ jsonb 结构互转。
func diffFieldsToEntity(items []*contractV1.ContractDiffField) []schema.ContractDiffField {
	if len(items) == 0 {
		return nil
	}
	out := make([]schema.ContractDiffField, 0, len(items))
	for _, item := range items {
		if item == nil || item.GetKey() == "" {
			continue
		}
		f := schema.ContractDiffField{Key: item.GetKey(), Label: item.GetLabel(), Widget: item.GetWidget()}
		if len(item.GetOptions()) > 0 {
			f.Options = item.GetOptions()
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func diffFieldsToDTO(items []schema.ContractDiffField) []*contractV1.ContractDiffField {
	if len(items) == 0 {
		return nil
	}
	out := make([]*contractV1.ContractDiffField, 0, len(items))
	for _, item := range items {
		f := &contractV1.ContractDiffField{Key: item.Key, Label: item.Label, Widget: item.Widget}
		if len(item.Options) > 0 {
			f.Options = item.Options
		}
		out = append(out, f)
	}
	return out
}

func (r *ContractTypeRepo) List(ctx context.Context, req *paginationV1.PagingRequest) (*contractV1.ListContractTypesResponse, error) {
	if req == nil {
		return nil, contractV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().ContractType.Query()

	// 默认业务序 = 类型编码升序（编码是唯一键，前缀序即 大类 → 细类）。
	// 调用方显式传 sorting/order_by 时尊重显式排序。
	if len(req.GetSorting()) == 0 && (req.GetOrderBy() == "" || req.GetOrderBy() == "[]") {
		builder.Order(ent.Asc(contracttype.FieldCode))
	}

	ret, err := r.repository.ListWithPaging(ctx, builder, builder.Clone(), req)
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return &contractV1.ListContractTypesResponse{Total: 0, Items: nil}, nil
	}

	return &contractV1.ListContractTypesResponse{
		Total: ret.Total,
		Items: ret.Items,
	}, nil
}

func (r *ContractTypeRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.entClient.Client().ContractType.Query().
		Where(contracttype.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query contract type exist failed: %s", err.Error())
		return false, contractV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *ContractTypeRepo) Get(ctx context.Context, req *contractV1.GetContractTypeRequest) (*contractV1.ContractType, error) {
	if req == nil {
		return nil, contractV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().ContractType.Query()

	var whereCond []func(s *sql.Selector)
	switch req.QueryBy.(type) {
	default:
	case *contractV1.GetContractTypeRequest_Id:
		whereCond = append(whereCond, contracttype.IDEQ(req.GetId()))
	}

	dto, err := r.repository.Get(ctx, builder, req.GetViewMask(), whereCond...)
	if err != nil {
		return nil, err
	}

	return dto, err
}

// GetByID 按主键取单条；不存在返回 ErrorNotFound。
func (r *ContractTypeRepo) GetByID(ctx context.Context, id uint32) (*contractV1.ContractType, error) {
	entity, err := r.entClient.Client().ContractType.Query().
		Where(contracttype.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, contractV1.ErrorNotFound("contract type not found")
		}
		r.log.Errorf(ctx, "query contract type by id failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("query contract type failed")
	}

	return r.mapper.ToDTO(entity), nil
}

// GetEntityByID 按主键取实体（快照解析要读 parent_id/is_enabled 等原始列）。
func (r *ContractTypeRepo) GetEntityByID(ctx context.Context, id uint32) (*ent.ContractType, error) {
	entity, err := r.entClient.Client().ContractType.Query().
		Where(contracttype.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, contractV1.ErrorNotFound("contract type not found")
		}
		r.log.Errorf(ctx, "query contract type entity by id failed: %s", err.Error())
		return nil, contractV1.ErrorInternalServerError("query contract type failed")
	}
	return entity, nil
}

// IsExistCode 类型编码是否已存在（excludeID 用于更新场景排除自身）。
func (r *ContractTypeRepo) IsExistCode(ctx context.Context, code string, excludeID uint32) (bool, error) {
	query := r.entClient.Client().ContractType.Query().
		Where(contracttype.CodeEQ(code))
	if excludeID != 0 {
		query = query.Where(contracttype.IDNEQ(excludeID))
	}

	exist, err := query.Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query contract type code exist failed: %s", err.Error())
		return false, contractV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

// HasChildren 是否存在下级类型（删除前守卫 / 合同创建的叶子判定）。
func (r *ContractTypeRepo) HasChildren(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.entClient.Client().ContractType.Query().
		Where(contracttype.ParentIDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf(ctx, "query contract type children failed: %s", err.Error())
		return false, contractV1.ErrorInternalServerError("query children failed")
	}
	return exist, nil
}

func (r *ContractTypeRepo) Create(ctx context.Context, req *contractV1.CreateContractTypeRequest) error {
	if req == nil || req.Data == nil {
		return contractV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().ContractType.Create().
		SetNillableCode(req.Data.Code).
		SetNillableName(req.Data.Name).
		SetNillableLevel(r.levelConverter.ToEntity(req.Data.Level)).
		SetNillableParentID(req.Data.ParentId).
		SetNillableDescription(req.Data.Description).
		SetNillableIsEnabled(req.Data.IsEnabled).
		SetNillablePartyBLabel(req.Data.PartyBLabel).
		SetNillablePayMethod(r.payMethodConverter.ToEntity(req.Data.PayMethod)).
		SetNillableFeeDirection(r.directionConverter.ToEntity(req.Data.FeeDirection)).
		SetNillableRequiresApproval(req.Data.RequiresApproval).
		SetNillableRequiresDeposit(req.Data.RequiresDeposit).
		SetNillableGuidance(req.Data.Guidance).
		SetNillableCreatedBy(req.Data.CreatedBy).
		SetCreatedAt(time.Now())

	// jsonb 数组列：非空才写，空 = 未配置（NULL）
	if nodes := payNodesToEntity(req.Data.GetPayNodes()); len(nodes) > 0 {
		builder.SetPayNodes(nodes)
	}
	if fields := diffFieldsToEntity(req.Data.GetDiffFields()); len(fields) > 0 {
		builder.SetDiffFields(fields)
	}
	if len(req.Data.GetRequiredAttachments()) > 0 {
		builder.SetRequiredAttachments(req.Data.GetRequiredAttachments())
	}

	if req.Data.Id != nil {
		builder.SetID(req.GetData().GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf(ctx, "insert contract type failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("insert contract type failed")
	}

	return nil
}

func (r *ContractTypeRepo) Update(ctx context.Context, req *contractV1.UpdateContractTypeRequest) error {
	if req == nil || req.Data == nil {
		return contractV1.ErrorBadRequest("invalid parameter")
	}
	if req.GetId() == 0 {
		return contractV1.ErrorBadRequest("id is required")
	}

	// 如果不存在则创建
	if req.GetAllowMissing() {
		exist, err := r.IsExist(ctx, req.GetId())
		if err != nil {
			return err
		}
		if !exist {
			createReq := &contractV1.CreateContractTypeRequest{Data: req.Data}
			createReq.Data.CreatedBy = createReq.Data.UpdatedBy
			createReq.Data.UpdatedBy = nil
			return r.Create(ctx, createReq)
		}
	}

	builder := r.entClient.Client().ContractType.Update()
	err := r.repository.UpdateX(ctx, builder, req.Data, req.GetUpdateMask(),
		func(dto *contractV1.ContractType) {
			builder.
				SetNillableCode(req.Data.Code).
				SetNillableName(req.Data.Name).
				SetNillableLevel(r.levelConverter.ToEntity(req.Data.Level)).
				SetNillableParentID(req.Data.ParentId).
				SetNillableDescription(req.Data.Description).
				SetNillableIsEnabled(req.Data.IsEnabled).
				SetNillablePartyBLabel(req.Data.PartyBLabel).
				SetNillablePayMethod(r.payMethodConverter.ToEntity(req.Data.PayMethod)).
				SetNillableFeeDirection(r.directionConverter.ToEntity(req.Data.FeeDirection)).
				SetNillableRequiresApproval(req.Data.RequiresApproval).
				SetNillableRequiresDeposit(req.Data.RequiresDeposit).
				SetNillableGuidance(req.Data.Guidance).
				SetNillableUpdatedBy(req.Data.UpdatedBy).
				SetUpdatedAt(time.Now())

			// jsonb 数组列：UpdateX 已先按 mask 过滤 req.Data（未列入 mask 的数组被清空），
			// 非空才 Set；mask 内且为空切片时由 go-crud 的 applyUpdateNilFieldMask 统一置
			// NULL——这里不能再 Clear，同一列双赋值会触发 PG 42601（参照 org_unit.subtype_path）。
			if nodes := payNodesToEntity(req.Data.GetPayNodes()); len(nodes) > 0 {
				builder.SetPayNodes(nodes)
			}
			if fields := diffFieldsToEntity(req.Data.GetDiffFields()); len(fields) > 0 {
				builder.SetDiffFields(fields)
			}
			if len(req.Data.GetRequiredAttachments()) > 0 {
				builder.SetRequiredAttachments(req.Data.GetRequiredAttachments())
			}
		},
		func(s *sql.Selector) {
			s.Where(sql.EQ(contracttype.FieldID, req.GetId()))
		},
	)
	if err != nil {
		r.log.Errorf(ctx, "update contract type failed: %s", err.Error())
		return err
	}

	return err
}

func (r *ContractTypeRepo) Delete(ctx context.Context, req *contractV1.DeleteContractTypeRequest) error {
	if req == nil {
		return contractV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().ContractType.Delete()

	_, err := r.repository.Delete(ctx, builder, func(s *sql.Selector) {
		s.Where(sql.EQ(contracttype.FieldID, req.GetId()))
	})
	if err != nil {
		r.log.Errorf(ctx, "delete contract type failed: %s", err.Error())
		return contractV1.ErrorInternalServerError("delete contract type failed")
	}

	return nil
}
