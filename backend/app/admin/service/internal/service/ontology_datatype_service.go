package service

import (
	"context"
	"strings"

	"github.com/tx7do/go-utils/trans"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"
	"google.golang.org/protobuf/types/known/emptypb"

	adminV1 "go-wind-admin/api/gen/go/admin/service/v1"
	ontologyV1 "go-wind-admin/api/gen/go/ontology/service/v1"

	"go-wind-admin/app/admin/service/internal/data"
	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// OntologyDatatypeService 数据类型管理服务（BFF：七端点编排，调域内核 + repo）。
//
// 系列最简实体（05 号文档 §1.2）：OWL 2 不给 datatype 层级/特征/成对关系——无 tree 端点、
// 无父级/等价/不相交/特征分区（Protégé OWLDatatypeDescriptionFrame 仅"类型定义"一节同构），
// 多一个 SimpleList 端点（数据属性值域双源下拉数据源，对应 getKnownDatatypes 自定义半边）。
// create/update/delete 是自定义 RPC（公理事务管线：diff + 乐观锁 + 校验 + 变更日志），
// 收扁平请求体——不是通用 CRUD（铁律 3 的正确适用，勿包 { data }）。
type OntologyDatatypeService struct {
	adminV1.OntologyDatatypeServiceHTTPServer

	log          *bLogger.Helper
	ontologyRepo *data.OntologyRepo
	entityRepo   *data.OntEntityRepo
	axiomRepo    *data.OntAxiomRepo
	factory      *ontology.EntityFactory
	applier      *ontology.ChangeApplier
}

func NewOntologyDatatypeService(
	ctx *bootstrap.Context,
	ontologyRepo *data.OntologyRepo,
	entityRepo *data.OntEntityRepo,
	axiomRepo *data.OntAxiomRepo,
) *OntologyDatatypeService {
	log := ctx.NewLoggerHelper("ontology-datatype/service/admin-service")
	return &OntologyDatatypeService{
		log:          log,
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		axiomRepo:    axiomRepo,
		factory:      ontology.NewEntityFactory(entityRepo),
		applier:      ontology.NewChangeApplier(axiomRepo, log),
	}
}

// requireOntology 本体存在且启用检查（实体级接口统一闸门）
func (s *OntologyDatatypeService) requireOntology(ctx context.Context, ontologyID uint32) (*ent.OntOntology, error) {
	if ontologyID == 0 {
		return nil, adminV1.ErrorBadRequest("ontology id is required")
	}
	ont, err := s.ontologyRepo.GetEntityByID(ctx, ontologyID)
	return checkOntologyEnabled(ont, err)
}

// requireDatatypeEntity 实体存在且为数据类型检查
func (s *OntologyDatatypeService) requireDatatypeEntity(ctx context.Context, id uint32) (*ent.OntEntity, error) {
	if id == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.entityRepo.GetEntity(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ontologyV1.ErrorEntityNotFound("本体实体不存在：%d", id)
		}
		return nil, adminV1.ErrorInternalServerError("query entity failed")
	}
	if e.EntityType == nil || *e.EntityType != ontology.EntityTypeDatatype {
		return nil, ontologyV1.ErrorEntityNotFound("实体 %d 不是数据类型", id)
	}
	return e, nil
}

// ---- 列表 ----

func (s *OntologyDatatypeService) List(ctx context.Context, req *ontologyV1.ListDatatypesRequest) (*ontologyV1.ListDatatypesResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}

	entities, total, err := s.entityRepo.ListDatatypes(ctx, data.ClassListFilter{
		OntologyID:   req.GetOntologyId(),
		LocalName:    req.GetLocalName(),
		DisplayLabel: req.GetDisplayLabel(),
		Deprecated:   req.Deprecated,
		Page:         req.GetPage(),
		PageSize:     req.GetPageSize(),
		OrderBy:      req.GetOrderBy(),
	})
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("list datatypes failed")
	}

	ids := make([]uint32, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.ID)
	}
	aggregates, err := s.entityRepo.AggregateDatatypeInfo(ctx, req.GetOntologyId(), ids)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("aggregate datatype info failed")
	}

	items := make([]*ontologyV1.Datatype, 0, len(entities))
	for _, e := range entities {
		items = append(items, data.EntEntityToDatatypeDTO(e, aggregates[e.ID]))
	}
	return &ontologyV1.ListDatatypesResponse{Items: items, Total: uint64(total)}, nil
}

// ---- 简况列表（值域双源下拉数据源） ----

func (s *OntologyDatatypeService) SimpleList(ctx context.Context, req *ontologyV1.ListSimpleDatatypesRequest) (*ontologyV1.SimpleDatatypeListResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}

	entities, err := s.entityRepo.ListSimpleDatatypes(ctx, req.GetOntologyId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("list simple datatypes failed")
	}

	ids := make([]uint32, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.ID)
	}
	aggregates, err := s.entityRepo.AggregateDatatypeInfo(ctx, req.GetOntologyId(), ids)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("aggregate datatype info failed")
	}

	items := make([]*ontologyV1.SimpleDatatype, 0, len(entities))
	for _, e := range entities {
		item := &ontologyV1.SimpleDatatype{
			Id:           &e.ID,
			Iri:          e.Iri,
			LocalName:    e.LocalName,
			DisplayLabel: e.DisplayLabel,
			Deprecated:   e.Deprecated,
		}
		if agg := aggregates[e.ID]; agg != nil && agg.DefinitionJSON != nil {
			if def, err := ontology.ParseDefinition(*agg.DefinitionJSON); err == nil && !def.IsEmpty() {
				item.BaseIri = trans.Ptr(definitionBaseIRIOf(def))
			}
		}
		items = append(items, item)
	}
	return &ontologyV1.SimpleDatatypeListResponse{Items: items}, nil
}

// ---- frame ----

func (s *OntologyDatatypeService) Frame(ctx context.Context, req *ontologyV1.GetDatatypeFrameRequest) (*ontologyV1.DatatypeFrame, error) {
	e, err := s.requireDatatypeEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	ont, err := s.requireOntology(ctx, ontologyID)
	if err != nil {
		return nil, err
	}

	subjectAxioms, err := s.axiomRepo.ListBySubject(ctx, ontologyID, e.ID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}

	frame := &ontologyV1.DatatypeFrame{
		Id:           &e.ID,
		OntologyId:   &ontologyID,
		Iri:          e.Iri,
		LocalName:    e.LocalName,
		DisplayLabel: e.DisplayLabel,
		Deprecated:   e.Deprecated,
		Comment:      ontology.PickAnnotationPreferred(subjectAxioms, ontology.PredicateRDFSComment),
		BaseVersion:  ont.Version,
		UpdatedAt:    timePtrToTimestamp(e.UpdatedAt),
	}

	// 类型定义（46 型首行；无行 = 仅声明未定义，definition 置空。多行脏数据取首行——源修订 #12；
	// 行存在而 JSON 损坏按 §7 #11 报 DATATYPE_DEFINITION_JSON_INVALID，列表/usage 不受影响）
	if raw := definitionJSONOf(subjectAxioms); strings.TrimSpace(raw) != "" {
		def, err := ontology.ParseDefinition(raw)
		if err != nil {
			return nil, ontologyV1.ErrorDatatypeDefinitionJsonInvalid("数据类型定义解析失败：%s", err.Error())
		}
		if !def.IsEmpty() {
			frame.Definition = datatypeDefinitionToProto(def)
		}
	}

	// 自定义注解区（04 期通用注解区，datatype 也可挂注解）
	annotations, err := frameAnnotationsOf(ctx, s.entityRepo, ontologyID, subjectAxioms)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query annotations failed")
	}
	frame.Annotations = annotations

	return frame, nil
}

// ---- usage ----

// Usage 引用明细（对应 OWLDataTypeUsageViewComponent）：42 值域行派生轨（P 是数据属性的
// 派生值域，object_id 路径命中）。datatype 无其余入边公理——定义 46 行以 P 为主语、
// base 是 IRI 常量，均不构成"被引用"（05 号文档 §4.5）。
func (s *OntologyDatatypeService) Usage(ctx context.Context, req *ontologyV1.GetDatatypeUsageRequest) (*ontologyV1.EntityUsage, error) {
	e, err := s.requireDatatypeEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	objectAxioms, err := s.axiomRepo.ListByObjectFiltered(ctx, data.ObjectReferenceFilter{
		OntologyID: ontologyID,
		ObjectIDs:  []uint32{e.ID},
		AxiomTypes: ontology.DatatypeUsageAxiomTypes,
	})
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query references failed")
	}

	type refKey struct {
		entityID  uint32
		axiomType uint32
	}
	seen := make(map[refKey]struct{})
	refKeys := make([]refKey, 0)
	for _, ax := range objectAxioms {
		if ax.SubjectID == nil || ax.AxiomType == nil {
			continue
		}
		key := refKey{entityID: *ax.SubjectID, axiomType: *ax.AxiomType}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		refKeys = append(refKeys, key)
	}

	usage := &ontologyV1.EntityUsage{EntityId: &e.ID}
	if len(refKeys) > 0 {
		ids := make([]uint32, 0, len(refKeys))
		for _, k := range refKeys {
			ids = append(ids, k.entityID)
		}
		entities, err := s.entityRepo.GetEntitiesByIDs(ctx, ontologyID, ids)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query referencing entities failed")
		}
		byID := entityMap(entities)
		usage.Refs = make([]*ontologyV1.EntityUsageRef, 0, len(refKeys))
		for _, k := range refKeys {
			ref := &ontologyV1.EntityUsageRef{
				EntityId:  &k.entityID,
				AxiomType: &k.axiomType,
			}
			if re, ok := byID[k.entityID]; ok {
				ref.LocalName = re.LocalName
				ref.DisplayLabel = re.DisplayLabel
			}
			usage.Refs = append(usage.Refs, ref)
		}
	}
	return usage, nil
}

// ---- create / update / delete ----

func (s *OntologyDatatypeService) Create(ctx context.Context, req *ontologyV1.CreateDatatypeRequest) (*ontologyV1.CreateDatatypeResponse, error) {
	if req == nil || req.GetOntologyId() == 0 || req.GetLocalName() == "" {
		return nil, adminV1.ErrorBadRequest("ontology id and local name are required")
	}
	ont, err := s.requireOntology(ctx, req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	baseIRI := derefStr(ont.BaseIri)

	// 实体草稿：局部名校验 + IRI 前置查重
	draft, declaration, err := s.factory.CreateEntityDraft(ctx, req.GetOntologyId(), ontology.EntityTypeDatatype, baseIRI, req.GetLocalName())
	if err != nil {
		return nil, mapDomainError(err)
	}

	// 校验管线（05 号文档 §4.2）：定义五规则 + 自定义注解归一化与谓词校验。
	// 无环检测、无自引用校验、无成对 diff——datatype 无任何实体间关系，管线最短。
	definition := datatypeDefinitionFromProto(req.Definition)
	if !definition.IsEmpty() {
		if err = ontology.ValidateDefinition(definition); err != nil {
			return nil, mapDomainError(err)
		}
	}
	annotationInputs, err := normalizeAnnotationInputs(req.GetAnnotations())
	if err != nil {
		return nil, err
	}
	if err = validateAnnotationPropertyRefs(ctx, s.factory, req.GetOntologyId(), annotationInputs); err != nil {
		return nil, err
	}

	// 组装变更集：新实体放 NewEntities[0]（DECLARATION + 46 定义[可选] + 100 注解）
	set := &ontology.EntityChangeSet{
		OntologyID:  req.GetOntologyId(),
		Operation:   ontology.OpCreateDatatype,
		NewEntities: []ontology.EntityDraft{*draft},
		Changes:     []ontology.AxiomChange{declaration},
	}
	if !definition.IsEmpty() {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op:                  ontology.AxiomOpAdd,
			AxiomType:           ontology.AxiomDatatypeDefinition,
			SubjectNewEntityRef: 1, // 主语 = 新实体（NewEntities[0]）
			ValueText:           ontology.NormalizeDefinition(definition),
			SortOrder:           0, // 至多一行，固定 0
		})
	}
	appendAnnotationAdds(set, req.DisplayLabel, req.Comment, nil, req.Deprecated)
	ontology.AppendCustomAnnotationAxioms(set, 1, 0, annotationInputs)

	result, err := s.applier.Apply(ctx, set, operatorUserID(ctx))
	if err != nil {
		return nil, mapDomainError(err)
	}

	resp := &ontologyV1.CreateDatatypeResponse{BaseVersion: &result.NewVersion}
	if len(result.NewEntityIDs) > 0 {
		resp.Id = &result.NewEntityIDs[0]
	}
	return resp, nil
}

func (s *OntologyDatatypeService) Update(ctx context.Context, req *ontologyV1.UpdateDatatypeRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireDatatypeEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	// local_name 忽略（不可变）：不参与 diff、不校验

	annotationInputs, err := normalizeAnnotationInputs(req.GetAnnotations())
	if err != nil {
		return nil, err
	}
	if err = validateAnnotationPropertyRefs(ctx, s.factory, ontologyID, annotationInputs); err != nil {
		return nil, err
	}

	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	subjectAxioms := filterBySubject(axioms, req.GetId())
	annotationAxioms := filterByType(subjectAxioms, ontology.AxiomAnnotationAssertion)

	changes := make([]ontology.AxiomChange, 0)
	// 定义区 diff（05 号文档 §4.3 两段语义）：definition nil = 跳过；显式空定义 = 清空；
	// DiffKey 等值（facet 调序不敏感）→ 无操作（行 id 不变，version 仍 +1）。
	// 存量多行 46 属脏数据：首行参与比对、多余行随 diff 删除自愈（源修订 #12）。
	if req.Definition != nil {
		definition := datatypeDefinitionFromProto(req.Definition)
		if !definition.IsEmpty() {
			if err = ontology.ValidateDefinition(definition); err != nil {
				return nil, mapDomainError(err)
			}
		}
		changes = append(changes, s.diffDatatypeDefinition(req.GetId(), definitionAxiomsOf(subjectAxioms), definition)...)
	}
	changes = append(changes, ontology.DiffAnnotations(req.GetId(), annotationAxioms, map[string]*string{
		ontology.PredicateRDFSLabel:   req.DisplayLabel, // nil = 不变；空串 = 清空
		ontology.PredicateRDFSComment: req.Comment,
	})...)
	if req.Deprecated != nil {
		changes = append(changes, ontology.DiffDeprecated(req.GetId(), annotationAxioms, *req.Deprecated)...)
	}
	// 自定义注解区（全量替换：nil/缺失/空数组统一 = 清空——04 号文档 §4.3/§12 #1）
	changes = append(changes, ontology.DiffCustomAnnotations(req.GetId(), annotationAxioms, annotationInputs)...)

	set := &ontology.EntityChangeSet{
		OntologyID:  ontologyID,
		BaseVersion: req.BaseVersion,
		Operation:   ontology.OpUpdateDatatype,
		Changes:     changes,
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OntologyDatatypeService) Delete(ctx context.Context, req *ontologyV1.DeleteDatatypeRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireDatatypeEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	force := req.Force != nil && *req.Force

	// 删除保护：object 侧命中（42 派生轨值域行）计数（05 号文档 §4.5）
	refsCount, err := s.axiomRepo.CountReferencesByTypes(ctx, ontologyID, req.GetId(), ontology.DatatypeUsageAxiomTypes)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("count references failed")
	}
	if refsCount > 0 && !force {
		return nil, ontologyV1.ErrorEntityInUse("实体被 %d 处引用，无法删除", refsCount)
	}

	// 收集删除变更：主语侧全部公理（声明/定义/自身注解）
	subjectAxioms, err := s.axiomRepo.ListBySubject(ctx, ontologyID, req.GetId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	deleted := make(map[uint32]struct{}, len(subjectAxioms))
	changes := make([]ontology.AxiomChange, 0, len(subjectAxioms))
	for _, ax := range subjectAxioms {
		if _, ok := deleted[ax.ID]; ok {
			continue
		}
		deleted[ax.ID] = struct{}{}
		changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: ax.ID, SubjectID: derefU32(ax.SubjectID)})
	}
	// 宾语侧（42 派生轨值域行——引用方数据属性的值域随之少一项，其余值域与属性本体完好；
	// 对齐 OWLEntityDeleter "删提及实体的公理"语义）
	objectAxioms, err := s.axiomRepo.ListByObject(ctx, ontologyID, req.GetId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	for _, ax := range objectAxioms {
		if _, ok := deleted[ax.ID]; ok {
			continue
		}
		deleted[ax.ID] = struct{}{}
		changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: ax.ID, SubjectID: derefU32(ax.SubjectID)})
	}

	// force 级联：实体行一并软删
	set := &ontology.EntityChangeSet{
		OntologyID:      ontologyID,
		BaseVersion:     req.BaseVersion,
		Operation:       ontology.OpDeleteDatatype,
		Changes:         changes,
		DeleteEntityIDs: []uint32{req.GetId()},
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- 定义区 diff 辅助 ----

// diffDatatypeDefinition 46 单行整体替换 diff（05 号文档 §4.3）：
//   - 目标空定义 → 删除全部存量行（清空）；
//   - DiffKey 相同（facet 名+值排序后等值，调序不敏感）→ 无操作（行 id 不变），
//     多余行（脏数据多行 46）仍删除自愈；
//   - 不同 → 删全部存量行 + 增一行（sort_order=0，落库形态保提交序）。
func (s *OntologyDatatypeService) diffDatatypeDefinition(subject uint32, current []*ent.OntAxiom, target ontology.DatatypeDefinition) []ontology.AxiomChange {
	if len(current) == 0 {
		if target.IsEmpty() {
			return nil
		}
		return []ontology.AxiomChange{{
			Op:        ontology.AxiomOpAdd,
			AxiomType: ontology.AxiomDatatypeDefinition,
			SubjectID: subject,
			ValueText: ontology.NormalizeDefinition(target),
			SortOrder: 0,
		}}
	}

	existingKey := ""
	if def, err := ontology.ParseDefinition(derefStr(current[0].ValueText)); err == nil {
		existingKey = ontology.DiffKeyDefinition(def)
	} // 损坏 JSON 视为不同 → 走替换自愈

	targetKey := ""
	if !target.IsEmpty() {
		targetKey = ontology.DiffKeyDefinition(target)
	}

	changes := make([]ontology.AxiomChange, 0, len(current)+1)
	if target.IsEmpty() || existingKey != targetKey {
		for _, row := range current {
			changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: row.ID, SubjectID: subject})
		}
	}
	if !target.IsEmpty() && existingKey != targetKey {
		changes = append(changes, ontology.AxiomChange{
			Op:        ontology.AxiomOpAdd,
			AxiomType: ontology.AxiomDatatypeDefinition,
			SubjectID: subject,
			ValueText: ontology.NormalizeDefinition(target),
			SortOrder: 0,
		})
	}
	return changes
}

// ---- proto ↔ 域形态转换 ----

// datatypeDefinitionFromProto proto 定义 → 域形态（nil = 未提交，返回空定义由调用方按 presence 分流）
func datatypeDefinitionFromProto(m *ontologyV1.DatatypeDefinition) ontology.DatatypeDefinition {
	if m == nil {
		return ontology.DatatypeDefinition{}
	}
	def := ontology.DatatypeDefinition{BaseIRI: m.GetBaseIri()}
	for _, f := range m.GetFacets() {
		if f == nil {
			continue
		}
		def.Facets = append(def.Facets, ontology.DatatypeFacet{Facet: f.GetFacet(), Value: f.GetValue()})
	}
	return def
}

// datatypeDefinitionToProto 域形态 → proto 定义（frame 回读）
func datatypeDefinitionToProto(def ontology.DatatypeDefinition) *ontologyV1.DatatypeDefinition {
	out := &ontologyV1.DatatypeDefinition{
		BaseIri: trans.Ptr(def.BaseIRI),
	}
	for _, f := range def.Facets {
		out.Facets = append(out.Facets, &ontologyV1.DatatypeFacet{Facet: f.Facet, Value: f.Value})
	}
	return out
}

// definitionAxiomsOf 主语侧 46 行（保序 sort_order → id——ListBySubject 查询序）
func definitionAxiomsOf(subjectAxioms []*ent.OntAxiom) []*ent.OntAxiom {
	out := make([]*ent.OntAxiom, 0, 1)
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDatatypeDefinition {
			continue
		}
		out = append(out, ax)
	}
	return out
}

// definitionJSONOf 主语侧 46 行的 value_text（首行；多行脏数据取首行——源修订 #12；
// 空串 = 无定义行，调用方按"仅声明未定义"处理）
func definitionJSONOf(subjectAxioms []*ent.OntAxiom) string {
	rows := definitionAxiomsOf(subjectAxioms)
	if len(rows) == 0 {
		return ""
	}
	return derefStr(rows[0].ValueText)
}

// definitionBaseIRIOf 定义基类型 IRI（simple-list 聚合列）
func definitionBaseIRIOf(def ontology.DatatypeDefinition) string {
	return def.BaseIRI
}
