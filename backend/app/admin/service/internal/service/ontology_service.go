package service

import (
	"context"
	"errors"
	"time"

	"github.com/tx7do/go-utils/timeutil"
	"github.com/tx7do/go-utils/trans"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"

	adminV1 "go-wind-admin/api/gen/go/admin/service/v1"
	ontologyV1 "go-wind-admin/api/gen/go/ontology/service/v1"

	"github.com/tx7do/go-crud/viewer"

	"go-wind-admin/app/admin/service/internal/data"
	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/ontology"
	"go-wind-admin/pkg/constants"
	"go-wind-admin/pkg/middleware/auth"
)

// OntologyService 本体文档管理服务（BFF：通用 CRUD 编排 + simple-list + 级联删除）。
//
// 本体文档是标准通用 CRUD：create/update 收 { data: {...} } 包裹（铁律 3 的"仅限 CRUD"分支）；
// 实体级操作（本体类等）走公理事务管线，见 OntologyClassService。
type OntologyService struct {
	adminV1.OntologyServiceHTTPServer

	log          *bLogger.Helper
	ontologyRepo *data.OntologyRepo
}

func NewOntologyService(
	ctx *bootstrap.Context,
	ontologyRepo *data.OntologyRepo,
) *OntologyService {
	s := &OntologyService{
		log:          ctx.NewLoggerHelper("ontology/service/admin-service"),
		ontologyRepo: ontologyRepo,
	}

	s.init()

	return s
}

// init 启动期播种守卫：本体为空表时落 constants.DefaultOntologies（与 language/menu 同机制）。
func (s *OntologyService) init() {
	ctx := viewer.WithSystemContext(context.Background())
	if count, _ := s.ontologyRepo.Count(ctx); count == 0 {
		_ = s.createDefaultOntologies(ctx)
	}
}

// createDefaultOntologies 播种默认本体（有名万物；createdBy=0 系统上下文）
func (s *OntologyService) createDefaultOntologies(ctx context.Context) error {
	for _, ont := range constants.DefaultOntologies {
		if err := s.ontologyRepo.Create(ctx, ont, 0); err != nil {
			s.log.Errorf(ctx, "create default ontology err: %v", err)
			return err
		}
	}
	return nil
}

// operatorUserID 操作人提取（auth.FromContext；系统上下文取 0，设计文档 §3.4 第 3 步）
func operatorUserID(ctx context.Context) uint32 {
	operator, err := auth.FromContext(ctx)
	if err != nil || operator == nil {
		return 0
	}
	return operator.UserId
}

// mapDomainError 域内核错误 → Kratos 错误（reason 字符串映射；未知错误原样透传为 500）
func mapDomainError(err error) error {
	if err == nil {
		return nil
	}
	var de *ontology.DomainError
	if !errors.As(err, &de) {
		return err
	}
	switch de.Reason {
	case ontology.ReasonOntologyNotFound:
		return ontologyV1.ErrorOntologyNotFound("%s", de.Message)
	case ontology.ReasonOntologyNameDuplicate:
		return ontologyV1.ErrorOntologyNameDuplicate("%s", de.Message)
	case ontology.ReasonOntologyBaseIRIInvalid:
		return ontologyV1.ErrorOntologyBaseIriInvalid("%s", de.Message)
	case ontology.ReasonOntologyVersionConflict:
		return ontologyV1.ErrorOntologyVersionConflict("%s", de.Message)
	case ontology.ReasonOntologyIRIDuplicate:
		return ontologyV1.ErrorOntologyIriDuplicate("%s", de.Message)
	case ontology.ReasonEntityNotFound:
		return ontologyV1.ErrorEntityNotFound("%s", de.Message)
	case ontology.ReasonEntityIRIDuplicate:
		return ontologyV1.ErrorEntityIriDuplicate("%s", de.Message)
	case ontology.ReasonLocalNameInvalid:
		return ontologyV1.ErrorLocalNameInvalid("%s", de.Message)
	case ontology.ReasonEntityTypeMismatch:
		return ontologyV1.ErrorEntityTypeMismatch("%s", de.Message)
	case ontology.ReasonAxiomInvalid:
		return ontologyV1.ErrorAxiomInvalid("%s", de.Message)
	case ontology.ReasonHierarchyCyclic:
		return ontologyV1.ErrorHierarchyCyclic("%s", de.Message)
	case ontology.ReasonEntityInUse:
		return ontologyV1.ErrorEntityInUse("%s", de.Message)

	// 02 期对象属性（源 1-071-003 段）
	case ontology.ReasonPropertyDomainInvalid:
		return ontologyV1.ErrorPropertyDomainInvalid("%s", de.Message)
	case ontology.ReasonPropertyRangeInvalid:
		return ontologyV1.ErrorPropertyRangeInvalid("%s", de.Message)
	case ontology.ReasonPropertyChainInvalid:
		return ontologyV1.ErrorPropertyChainInvalid("%s", de.Message)
	case ontology.ReasonPropertyInverseSelfInvalid:
		return ontologyV1.ErrorPropertyInverseSelfInvalid("%s", de.Message)
	case ontology.ReasonPropertySelfReferenceInvalid:
		return ontologyV1.ErrorPropertySelfReferenceInvalid("%s", de.Message)
	case ontology.ReasonPropertyCharacteristicInvalid:
		return ontologyV1.ErrorPropertyCharacteristicInvalid("%s", de.Message)
	case ontology.ReasonPropertyTargetInvalid:
		return ontologyV1.ErrorPropertyTargetInvalid("%s", de.Message)

	// 03 期数据属性（源 1-071-004 段）
	case ontology.ReasonDataPropertyDomainInvalid:
		return ontologyV1.ErrorDataPropertyDomainInvalid("%s", de.Message)
	case ontology.ReasonDataPropertyRangeInvalid:
		return ontologyV1.ErrorDataPropertyRangeInvalid("%s", de.Message)
	case ontology.ReasonDataPropertySelfReferenceInvalid:
		return ontologyV1.ErrorDataPropertySelfReferenceInvalid("%s", de.Message)
	case ontology.ReasonDataPropertyTargetInvalid:
		return ontologyV1.ErrorDataPropertyTargetInvalid("%s", de.Message)
	case ontology.ReasonDataPropertyCharacteristicInvalid:
		return ontologyV1.ErrorDataPropertyCharacteristicInvalid("%s", de.Message)

	// 04 期注解属性（源 1-071-005 段）
	case ontology.ReasonAnnotationPropertyTargetInvalid:
		return ontologyV1.ErrorAnnotationPropertyTargetInvalid("%s", de.Message)
	case ontology.ReasonAnnotationValueInvalid:
		return ontologyV1.ErrorAnnotationValueInvalid("%s", de.Message)
	default:
		return adminV1.ErrorInternalServerError("%s", de.Message)
	}
}

func (s *OntologyService) List(ctx context.Context, req *paginationV1.PagingRequest) (*ontologyV1.ListOntologiesResponse, error) {
	return s.ontologyRepo.List(ctx, req)
}

func (s *OntologyService) SimpleList(ctx context.Context, _ *emptypb.Empty) (*ontologyV1.ListOntologiesSimpleResponse, error) {
	return s.ontologyRepo.SimpleList(ctx)
}

func (s *OntologyService) Get(ctx context.Context, req *ontologyV1.GetOntologyRequest) (*ontologyV1.Ontology, error) {
	return s.ontologyRepo.Get(ctx, req)
}

func (s *OntologyService) Create(ctx context.Context, req *ontologyV1.CreateOntologyRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	payload := req.Data

	if payload.OntologyName == nil || *payload.OntologyName == "" {
		return nil, adminV1.ErrorBadRequest("ontology name is required")
	}
	if payload.OntologyIri == nil || *payload.OntologyIri == "" {
		return nil, adminV1.ErrorBadRequest("ontology iri is required")
	}
	if payload.BaseIri == nil || *payload.BaseIri == "" {
		return nil, adminV1.ErrorBadRequest("base iri is required")
	}

	// base_iri 校验与规范化（尾部统一补 #，设计文档 §3.2）
	baseIRI, err := data.NormalizeBaseIRI(*payload.BaseIri)
	if err != nil {
		return nil, mapDomainError(err)
	}

	// 同名本体查重（服务层友好报错；名称唯一性不建索引，允许删除后重建同名）
	if exists, err := s.ontologyRepo.ExistsName(ctx, *payload.OntologyName); err != nil {
		return nil, adminV1.ErrorInternalServerError("check ontology name failed")
	} else if exists {
		return nil, ontologyV1.ErrorOntologyNameDuplicate("已存在同名本体：%s", *payload.OntologyName)
	}

	// 本体 IRI 查重（DB partial unique index 兜底并发双写）
	if exists, err := s.ontologyRepo.ExistsIRI(ctx, *payload.OntologyIri); err != nil {
		return nil, adminV1.ErrorInternalServerError("check ontology iri failed")
	} else if exists {
		return nil, ontologyV1.ErrorOntologyIriDuplicate("已存在相同 IRI 的本体：%s", *payload.OntologyIri)
	}

	payload.BaseIri = &baseIRI
	payload.CreatedBy = trans.Ptr(operatorUserID(ctx))
	payload.Version = trans.Ptr(uint32(0))

	if err = s.ontologyRepo.Create(ctx, payload, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OntologyService) Update(ctx context.Context, req *ontologyV1.UpdateOntologyRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// base_iri 创建后不可变：repo 仅投影允许字段（源修订 #1），本层不报错、不生效
	if err := s.ontologyRepo.Update(ctx, req, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// Delete 删除本体文档：同一事务级联软删本体下全部实体/公理/变更日志（前端二次确认弹窗固定警示）
func (s *OntologyService) Delete(ctx context.Context, req *ontologyV1.DeleteOntologyRequest) (*emptypb.Empty, error) {
	if req == nil || len(req.GetIds()) == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	if err := s.ontologyRepo.Delete(ctx, req.GetIds(), operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- 共用小工具（ontology 域服务共用） ----

// timePtrToTimestamp *time.Time → *timestamppb.Timestamp
func timePtrToTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timeutil.TimeToTimestamppb(t)
}

// checkOntologyEnabled 本体存在且启用检查（停用不另设 reason，文案区分——设计文档 §4.7 #9）
func checkOntologyEnabled(ont *ent.OntOntology, err error) (*ent.OntOntology, error) {
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ontologyV1.ErrorOntologyNotFound("本体不存在")
		}
		return nil, adminV1.ErrorInternalServerError("query ontology failed")
	}
	if ont.IsEnabled == nil || !*ont.IsEnabled {
		return nil, ontologyV1.ErrorOntologyNotFound("本体已停用，实体级操作不可用")
	}
	return ont, nil
}

// dedupeUint32 保序去重
func dedupeUint32(ids []uint32) []uint32 {
	seen := make(map[uint32]struct{}, len(ids))
	out := make([]uint32, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
