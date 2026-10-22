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

// OntologyDataPropertyService 数据属性管理服务（BFF：七端点编排，调域内核 + repo）。
//
// create/update/delete 是自定义 RPC（公理事务管线：diff + 环检测 + 乐观锁 + 校验 + 变更日志），
// 收扁平请求体——不是通用 CRUD（铁律 3 的正确适用，勿包 { data }）。
// 与对象属性服务的差异面（03 号文档 §2.4 矩阵）：值域是内建 datatype IRI（42 型 value_text +
// 白名单，非实体引用）、特征仅函数性（43 型一种）、无逆属性/属性链（OWL 2 语言约束）、
// 虚拟根 owl:topDataProperty；usage 反查 40/44/45（无链扫描）。
type OntologyDataPropertyService struct {
	adminV1.OntologyDataPropertyServiceHTTPServer

	log          *bLogger.Helper
	ontologyRepo *data.OntologyRepo
	entityRepo   *data.OntEntityRepo
	axiomRepo    *data.OntAxiomRepo
	factory      *ontology.EntityFactory
	applier      *ontology.ChangeApplier
}

func NewOntologyDataPropertyService(
	ctx *bootstrap.Context,
	ontologyRepo *data.OntologyRepo,
	entityRepo *data.OntEntityRepo,
	axiomRepo *data.OntAxiomRepo,
) *OntologyDataPropertyService {
	log := ctx.NewLoggerHelper("ontology-data-property/service/admin-service")
	return &OntologyDataPropertyService{
		log:          log,
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		axiomRepo:    axiomRepo,
		factory:      ontology.NewEntityFactory(entityRepo),
		applier:      ontology.NewChangeApplier(axiomRepo, log),
	}
}

// requireOntology 本体存在且启用检查（实体级接口统一闸门）
func (s *OntologyDataPropertyService) requireOntology(ctx context.Context, ontologyID uint32) (*ent.OntOntology, error) {
	if ontologyID == 0 {
		return nil, adminV1.ErrorBadRequest("ontology id is required")
	}
	ont, err := s.ontologyRepo.GetEntityByID(ctx, ontologyID)
	return checkOntologyEnabled(ont, err)
}

// requireDataPropertyEntity 实体存在且为数据属性检查
func (s *OntologyDataPropertyService) requireDataPropertyEntity(ctx context.Context, id uint32) (*ent.OntEntity, error) {
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
	if e.EntityType == nil || *e.EntityType != ontology.EntityTypeDataProperty {
		return nil, ontologyV1.ErrorEntityNotFound("实体 %d 不是数据属性", id)
	}
	return e, nil
}

// ---- 树（全量；无 nav 剪枝） ----

func (s *OntologyDataPropertyService) Tree(ctx context.Context, req *ontologyV1.GetDataPropertyTreeRequest) (*ontologyV1.DataPropertyTreeResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}

	entities, err := s.entityRepo.ListEntitiesByType(ctx, req.GetOntologyId(), ontology.EntityTypeDataProperty)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query entities failed")
	}
	axioms, err := s.axiomRepo.ListByOntology(ctx, req.GetOntologyId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}

	roots := ontology.BuildClassTree(ctx, ontology.ClassHierarchyInput{
		Entities: entities,
		Edges:    ontology.CollectHierarchyEdges(axioms, ontology.AxiomSubDataPropertyOf),
	}, s.log)

	out := make([]*ontologyV1.DataPropertyTreeNode, 0, len(roots))
	for _, r := range roots {
		out = append(out, dataPropertyTreeNodeToProto(r))
	}
	return &ontologyV1.DataPropertyTreeResponse{Roots: out}, nil
}

func dataPropertyTreeNodeToProto(n *ontology.TreeNode) *ontologyV1.DataPropertyTreeNode {
	node := &ontologyV1.DataPropertyTreeNode{
		Id:           &n.ID,
		Iri:          trans.Ptr(n.IRI),
		LocalName:    trans.Ptr(n.LocalName),
		DisplayLabel: trans.Ptr(n.DisplayLabel),
		Deprecated:   &n.Deprecated,
		ChildCount:   &n.ChildCount,
	}
	if n.DisplayLabel == "" {
		node.DisplayLabel = nil
	}
	if n.Children != nil {
		node.Children = make([]*ontologyV1.DataPropertyTreeNode, 0, len(n.Children))
		for _, c := range n.Children {
			node.Children = append(node.Children, dataPropertyTreeNodeToProto(c))
		}
	}
	return node
}

// ---- 列表 ----

func (s *OntologyDataPropertyService) List(ctx context.Context, req *ontologyV1.ListDataPropertiesRequest) (*ontologyV1.ListDataPropertiesResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}

	entities, total, err := s.entityRepo.ListDataProperties(ctx, data.ClassListFilter{
		OntologyID:   req.GetOntologyId(),
		LocalName:    req.GetLocalName(),
		DisplayLabel: req.GetDisplayLabel(),
		Deprecated:   req.Deprecated,
		Page:         req.GetPage(),
		PageSize:     req.GetPageSize(),
		OrderBy:      req.GetOrderBy(),
	})
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("list data properties failed")
	}

	ids := make([]uint32, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.ID)
	}
	aggregates, err := s.entityRepo.AggregateDataPropertyInfo(ctx, req.GetOntologyId(), ids)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("aggregate data property info failed")
	}

	items := make([]*ontologyV1.DataProperty, 0, len(entities))
	for _, e := range entities {
		items = append(items, data.EntEntityToDataPropertyDTO(e, aggregates[e.ID]))
	}
	return &ontologyV1.ListDataPropertiesResponse{Items: items, Total: uint64(total)}, nil
}

// ---- frame ----

func (s *OntologyDataPropertyService) Frame(ctx context.Context, req *ontologyV1.GetDataPropertyFrameRequest) (*ontologyV1.DataPropertyFrame, error) {
	e, err := s.requireDataPropertyEntity(ctx, req.GetId())
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
	objectAxioms, err := s.axiomRepo.ListByObject(ctx, ontologyID, e.ID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}

	frame := &ontologyV1.DataPropertyFrame{
		Id:              &e.ID,
		OntologyId:      &ontologyID,
		Iri:             e.Iri,
		LocalName:       e.LocalName,
		DisplayLabel:    e.DisplayLabel,
		Deprecated:      e.Deprecated,
		Comment:         ontology.PickAnnotationPreferred(subjectAxioms, ontology.PredicateRDFSComment),
		Characteristics: dataPropertyCharacteristicsOf(subjectAxioms),
		BaseVersion:     ont.Version,
		UpdatedAt:       timePtrToTimestamp(e.UpdatedAt),
	}

	// 父属性（保序 sort_order）
	superIDs := objectIDsOf(subjectAxioms, ontology.AxiomSubDataPropertyOf)
	if len(superIDs) > 0 {
		briefs, err := s.briefsByIDs(ctx, ontologyID, superIDs)
		if err != nil {
			return nil, err
		}
		frame.SuperPropertyIds = superIDs
		frame.SuperProperties = briefs
	}

	// 定义域（多值保序）
	domainIDs := objectIDsOf(subjectAxioms, ontology.AxiomDataPropertyDomain)
	if len(domainIDs) > 0 {
		briefs, err := s.briefsByIDs(ctx, ontologyID, domainIDs)
		if err != nil {
			return nil, err
		}
		frame.DomainIds = domainIDs
		frame.Domains = briefs
	}

	// 值域（42 型 value_text = datatype IRI，多值保序）
	rangeIRIs := rangeIRIsOfSubject(subjectAxioms)
	if len(rangeIRIs) > 0 {
		frame.RangeIris = rangeIRIs
	}

	// 不相交（对称伙伴去重：双向并集）
	frame.DisjointIds = symmetricPartners(e.ID, subjectAxioms, objectAxioms, ontology.AxiomDisjointDataProperties)

	// 等价（V1 单值，成对存储取主语侧）
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomEquivalentDataProperties || ax.ObjectID == nil {
			continue
		}
		frame.EquivalentId = ax.ObjectID
		break
	}

	// 自定义注解区（内建字段之外，保序；04 期增量——04 号文档 §4.3）
	annotations, err := frameAnnotationsOf(ctx, s.entityRepo, ontologyID, subjectAxioms)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query annotations failed")
	}
	frame.Annotations = annotations

	return frame, nil
}

// briefsByIDs 按 id 序取实体简况（frame 各 section 共用；缺失实体跳过——不阻塞 frame 展示）
func (s *OntologyDataPropertyService) briefsByIDs(ctx context.Context, ontologyID uint32, ids []uint32) ([]*ontologyV1.EntityBrief, error) {
	entities, err := s.entityRepo.GetEntitiesByIDs(ctx, ontologyID, ids)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query referenced entities failed")
	}
	byID := entityMap(entities)
	out := make([]*ontologyV1.EntityBrief, 0, len(ids))
	for _, id := range ids {
		if e, ok := byID[id]; ok {
			out = append(out, entityBrief(e))
		}
	}
	return out, nil
}

// rangeIRIsOfSubject 主语侧 42 行的 value_text 序列（保序 sort_order → id）
func rangeIRIsOfSubject(subjectAxioms []*ent.OntAxiom) []string {
	out := make([]string, 0)
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyRange {
			continue
		}
		if ax.ValueText != nil && *ax.ValueText != "" {
			out = append(out, *ax.ValueText)
		}
	}
	return out
}

// dataPropertyCharacteristicsOf 主语侧 43 行特征（V1 至多函数性；白名单外防御性补尾）
func dataPropertyCharacteristicsOf(subjectAxioms []*ent.OntAxiom) []string {
	out := make([]string, 0, 1)
	present := make(map[string]struct{}, 1)
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyCharacteristic || ax.PredicateIri == nil {
			continue
		}
		present[*ax.PredicateIri] = struct{}{}
	}
	if _, ok := present[ontology.IRIOWLFunctionalProperty]; ok {
		out = append(out, ontology.IRIOWLFunctionalProperty)
		delete(present, ontology.IRIOWLFunctionalProperty)
	}
	for _, ax := range subjectAxioms { // 白名单外（校验器已拦，防御兼容）按行序补尾
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyCharacteristic || ax.PredicateIri == nil {
			continue
		}
		if _, ok := present[*ax.PredicateIri]; ok {
			out = append(out, *ax.PredicateIri)
			delete(present, *ax.PredicateIri)
		}
	}
	return out
}

// ---- usage ----

func (s *OntologyDataPropertyService) Usage(ctx context.Context, req *ontologyV1.GetDataPropertyUsageRequest) (*ontologyV1.EntityUsage, error) {
	e, err := s.requireDataPropertyEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	// 引用集 = object 侧、DataPropertyUsageAxiomTypes（40/44/45）命中。
	// 无链扫描（数据属性没有属性链）；06 期 62 断言（谓词走 object_id）命中同一查询，届时类型集追加 62。
	objectAxioms, err := s.axiomRepo.ListByObject(ctx, ontologyID, e.ID)
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
		if !isUsageAxiomType(*ax.AxiomType, ontology.DataPropertyUsageAxiomTypes) {
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

func (s *OntologyDataPropertyService) Create(ctx context.Context, req *ontologyV1.CreateDataPropertyRequest) (*ontologyV1.CreateDataPropertyResponse, error) {
	if req == nil || req.GetOntologyId() == 0 || req.GetLocalName() == "" {
		return nil, adminV1.ErrorBadRequest("ontology id and local name are required")
	}
	ont, err := s.requireOntology(ctx, req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	baseIRI := derefStr(ont.BaseIri)

	// 实体草稿：局部名校验 + IRI 前置查重
	draft, declaration, err := s.factory.CreateEntityDraft(ctx, req.GetOntologyId(), ontology.EntityTypeDataProperty, baseIRI, req.GetLocalName())
	if err != nil {
		return nil, mapDomainError(err)
	}

	// 八步校验管线（03 号文档 §4.2）：值域白名单 + 引用目标 + 特征 + 环
	superIDs := dedupeUint32(req.GetSuperPropertyIds())
	domainIDs := dedupeUint32(req.GetDomainIds())
	disjointIDs := dedupeUint32(req.GetDisjointIds())
	rangeIRIs := normalizeRangeIRIs(req.GetRangeIris())
	var equivalentID uint32
	if req.EquivalentId != nil && *req.EquivalentId > 0 {
		equivalentID = *req.EquivalentId
	}
	if err = s.validateDataPropertyRefs(ctx, req.GetOntologyId(), superIDs, domainIDs, disjointIDs, equivalentID, rangeIRIs); err != nil {
		return nil, err
	}

	// 特征白名单：数据属性仅函数性（V1）
	if err = validateDataPropertyCharacteristics(req.GetCharacteristics()); err != nil {
		return nil, err
	}

	// 自定义注解（04 期增量）：值空白拒绝 / lang 归一去重 + 谓词须为本本体注解属性实体
	annotationInputs, err := normalizeAnnotationInputs(req.GetAnnotations())
	if err != nil {
		return nil, err
	}
	if err = validateAnnotationPropertyRefs(ctx, s.factory, req.GetOntologyId(), annotationInputs); err != nil {
		return nil, err
	}

	// 环检测（create 时新实体无任何入边，理论上恒通过；防御性保留同一管线）
	if err = s.assertNoCycleForDataProperty(ctx, req.GetOntologyId(), 0, superIDs); err != nil {
		return nil, err
	}

	// 组装变更集：新实体放 NewEntities[0]
	set := &ontology.EntityChangeSet{
		OntologyID:  req.GetOntologyId(),
		Operation:   ontology.OpCreateDataProperty,
		NewEntities: []ontology.EntityDraft{*draft},
		Changes:     []ontology.AxiomChange{declaration},
	}
	appendDataPropertyRelationAdds(set, superIDs, domainIDs, disjointIDs, rangeIRIs, equivalentID)
	appendCharacteristicsAdds(set, ontology.AxiomDataPropertyCharacteristic, req.GetCharacteristics())
	appendAnnotationAdds(set, req.DisplayLabel, req.Comment, nil, req.Deprecated)
	ontology.AppendCustomAnnotationAxioms(set, 1, 0, annotationInputs) // 主语 = 新实体（NewEntities[0]）

	result, err := s.applier.Apply(ctx, set, operatorUserID(ctx))
	if err != nil {
		return nil, mapDomainError(err)
	}

	resp := &ontologyV1.CreateDataPropertyResponse{BaseVersion: &result.NewVersion}
	if len(result.NewEntityIDs) > 0 {
		resp.Id = &result.NewEntityIDs[0]
	}
	return resp, nil
}

func (s *OntologyDataPropertyService) Update(ctx context.Context, req *ontologyV1.UpdateDataPropertyRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireDataPropertyEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	// local_name 忽略（不可变）：不参与 diff、不校验

	superIDs := dedupeUint32(req.GetSuperPropertyIds())
	domainIDs := dedupeUint32(req.GetDomainIds())
	disjointIDs := dedupeUint32(req.GetDisjointIds())
	rangeIRIs := normalizeRangeIRIs(req.GetRangeIris())

	// 自引用：不相交/等价自引用拒绝（父属性自引用交由环检测捕获）
	for _, id := range disjointIDs {
		if id == req.GetId() {
			return nil, ontologyV1.ErrorDataPropertySelfReferenceInvalid("属性不能与自身不相交：%d", id)
		}
	}
	var equivalentTarget []uint32
	equivalentSet := req.EquivalentId != nil
	if equivalentSet && *req.EquivalentId > 0 {
		if *req.EquivalentId == req.GetId() {
			return nil, ontologyV1.ErrorDataPropertySelfReferenceInvalid("属性不能与自身等价：%d", *req.EquivalentId)
		}
		equivalentTarget = []uint32{*req.EquivalentId}
	}

	if err = s.validateDataPropertyRefs(ctx, ontologyID, superIDs, domainIDs, disjointIDs, 0, rangeIRIs); err != nil {
		return nil, err
	}
	if err = validateDataPropertyCharacteristics(req.GetCharacteristics()); err != nil {
		return nil, err
	}
	// 等价目标单独校验（equivalentID=0 表示清空，不参与引用校验）
	if len(equivalentTarget) > 0 {
		if err = s.factory.ValidateRefEntities(ctx, ontologyID, equivalentTarget, ontology.EntityTypeDataProperty, ontology.ReasonDataPropertyTargetInvalid); err != nil {
			return nil, mapDomainError(err)
		}
	}

	// 环检测（全量新父集；含"把子孙设为父属性"与自环）
	if err = s.assertNoCycleForDataProperty(ctx, ontologyID, req.GetId(), superIDs); err != nil {
		return nil, err
	}

	// frame 全量提交语义 diff（逐 section）
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	subjectAxioms := filterBySubject(axioms, req.GetId())
	annotationAxioms := filterByType(subjectAxioms, ontology.AxiomAnnotationAssertion)

	changes := ontology.DiffSubjectSideRelations(req.GetId(), ontology.AxiomSubDataPropertyOf,
		filterByType(subjectAxioms, ontology.AxiomSubDataPropertyOf), superIDs)
	changes = append(changes, ontology.DiffSubjectSideRelations(req.GetId(), ontology.AxiomDataPropertyDomain,
		filterByType(subjectAxioms, ontology.AxiomDataPropertyDomain), domainIDs)...)
	// 值域按 value_text 集合 diff（源修订 #2：新增行 sort_order 存量最大值后顺排，回读序=提交序）
	changes = append(changes, ontology.DiffSubjectSideValues(req.GetId(), ontology.AxiomDataPropertyRange,
		filterByType(subjectAxioms, ontology.AxiomDataPropertyRange), rangeIRIs)...)
	changes = append(changes, ontology.DiffCharacteristics(req.GetId(), ontology.AxiomDataPropertyCharacteristic,
		filterByType(subjectAxioms, ontology.AxiomDataPropertyCharacteristic), req.GetCharacteristics())...)
	changes = append(changes, ontology.DiffSymmetricPairs(req.GetId(), ontology.AxiomDisjointDataProperties,
		buildSymmetricPairs(req.GetId(), axioms, ontology.AxiomDisjointDataProperties), disjointIDs)...)
	if equivalentSet {
		changes = append(changes, ontology.DiffSymmetricPairs(req.GetId(), ontology.AxiomEquivalentDataProperties,
			buildSymmetricPairs(req.GetId(), axioms, ontology.AxiomEquivalentDataProperties), equivalentTarget)...)
	}
	changes = append(changes, ontology.DiffAnnotations(req.GetId(), annotationAxioms, map[string]*string{
		ontology.PredicateRDFSLabel:   req.DisplayLabel, // nil = 不变；空串 = 清空
		ontology.PredicateRDFSComment: req.Comment,
	})...)
	if req.Deprecated != nil {
		changes = append(changes, ontology.DiffDeprecated(req.GetId(), annotationAxioms, *req.Deprecated)...)
	}
	// 自定义注解区（全量替换：nil/缺失/空数组统一 = 清空；04 期增量——04 号文档 §4.3）
	annotationInputs, err := normalizeAnnotationInputs(req.GetAnnotations())
	if err != nil {
		return nil, err
	}
	if err = validateAnnotationPropertyRefs(ctx, s.factory, ontologyID, annotationInputs); err != nil {
		return nil, err
	}
	changes = append(changes, ontology.DiffCustomAnnotations(req.GetId(), annotationAxioms, annotationInputs)...)

	set := &ontology.EntityChangeSet{
		OntologyID:  ontologyID,
		BaseVersion: req.BaseVersion,
		Operation:   ontology.OpUpdateDataProperty,
		Changes:     changes,
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OntologyDataPropertyService) Delete(ctx context.Context, req *ontologyV1.DeleteDataPropertyRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireDataPropertyEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	force := req.Force != nil && *req.Force

	// 删除保护：object 侧命中（40/44/45）计数。无链扫描；06 期 62 断言随类型集扩展纳入。
	refsCount, err := s.axiomRepo.CountReferencesByTypes(ctx, ontologyID, req.GetId(), ontology.DataPropertyUsageAxiomTypes)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("count references failed")
	}
	if refsCount > 0 && !force {
		return nil, ontologyV1.ErrorEntityInUse("实体被 %d 处引用，无法删除", refsCount)
	}

	// 收集删除变更：主语侧全部公理（声明/子属性/域值域/特征/对称/注解）
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
	// 宾语侧（44/45 镜像对、40 子属性边——子属性自动变根属性）
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

	// force 级联：实体行一并软删；子属性自动变根属性
	set := &ontology.EntityChangeSet{
		OntologyID:      ontologyID,
		BaseVersion:     req.BaseVersion,
		Operation:       ontology.OpDeleteDataProperty,
		Changes:         changes,
		DeleteEntityIDs: []uint32{req.GetId()},
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- 校验与组装辅助 ----

// validateDataPropertyRefs 引用目标类型与存在性校验（八步管线第 4/5 步）：
// 属性侧引用（父/不相交/等价）→ DATA_PROPERTY_TARGET_INVALID；定义域 → DATA_PROPERTY_DOMAIN_INVALID；
// 值域逐个命中内建白名单 → DATA_PROPERTY_RANGE_INVALID。
// 自引用校验不在本方法（调用方按三态分流；create 不做——新实体尚无 id）。
func (s *OntologyDataPropertyService) validateDataPropertyRefs(
	ctx context.Context, ontologyID uint32,
	superIDs, domainIDs, disjointIDs []uint32,
	equivalentID uint32,
	rangeIRIs []string,
) error {
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, superIDs, ontology.EntityTypeDataProperty, ontology.ReasonDataPropertyTargetInvalid); err != nil {
		return mapDomainError(err)
	}
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, disjointIDs, ontology.EntityTypeDataProperty, ontology.ReasonDataPropertyTargetInvalid); err != nil {
		return mapDomainError(err)
	}
	if equivalentID > 0 {
		if err := s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{equivalentID}, ontology.EntityTypeDataProperty, ontology.ReasonDataPropertyTargetInvalid); err != nil {
			return mapDomainError(err)
		}
	}

	// 定义域指向非类不复用 ENTITY_TYPE_MISMATCH：专属文案码（03 号文档 §3.3）
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, domainIDs, ontology.EntityTypeClass, ontology.ReasonDataPropertyDomainInvalid); err != nil {
		return mapDomainError(err)
	}

	// 值域白名单（服务层前置拦截；校验器 42 分支兜底直改库脏数据）
	for _, iri := range rangeIRIs {
		if _, ok := ontology.IsBuiltinDatatype(iri); !ok {
			return ontologyV1.ErrorDataPropertyRangeInvalid("数据属性值域必须是内建数据类型：%s", iri)
		}
	}
	return nil
}

// validateDataPropertyCharacteristics 特征白名单校验（八步管线第 6 步）：
// 数据属性仅支持函数性（OWL 2 语言约束，03 号文档 §2.3）
func validateDataPropertyCharacteristics(iris []string) error {
	for _, iri := range iris {
		if iri == "" {
			continue
		}
		if iri != ontology.IRIOWLFunctionalProperty {
			return ontologyV1.ErrorDataPropertyCharacteristicInvalid("数据属性仅支持函数性特征：%s", iri)
		}
	}
	return nil
}

// normalizeRangeIRIs 值域归一化（源修订 #3 内化）：去空白、丢空白项、保序去重
func normalizeRangeIRIs(iris []string) []string {
	out := make([]string, 0, len(iris))
	seen := make(map[string]struct{}, len(iris))
	for _, iri := range iris {
		trimmed := strings.TrimSpace(iri)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// assertNoCycleForDataProperty 环检测（写路径前置，读全量 40 型边）
func (s *OntologyDataPropertyService) assertNoCycleForDataProperty(ctx context.Context, ontologyID, targetID uint32, superIDs []uint32) error {
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return adminV1.ErrorInternalServerError("query axioms failed")
	}
	edges := ontology.CollectHierarchyEdges(axioms, ontology.AxiomSubDataPropertyOf)
	var names map[uint32]string
	if entities, listErr := s.entityRepo.ListEntitiesByType(ctx, ontologyID, ontology.EntityTypeDataProperty); listErr == nil {
		names = make(map[uint32]string, len(entities))
		for _, e := range entities {
			names[e.ID] = derefStr(e.LocalName)
		}
	}
	if err := ontology.AssertNoCycle(targetID, edges, superIDs, names); err != nil {
		return mapDomainError(err)
	}
	return nil
}

// appendDataPropertyRelationAdds 创建时的关系 ADD 变更（父属性 40 / 定义域 41 / 值域 42 /
// 不相交 45 / 等价 44；不相交与等价成对存储；值域 value_text=datatype IRI）
func appendDataPropertyRelationAdds(
	set *ontology.EntityChangeSet,
	superIDs, domainIDs, disjointIDs []uint32,
	rangeIRIs []string,
	equivalentID uint32,
) {
	const selfRef = 1 // 主语 = 新实体（NewEntities[0]）
	for i, pid := range superIDs {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomSubDataPropertyOf,
			SubjectNewEntityRef: selfRef, ObjectID: pid, SortOrder: int32(i),
		})
	}
	for i, did := range domainIDs {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDataPropertyDomain,
			SubjectNewEntityRef: selfRef, ObjectID: did, SortOrder: int32(i),
		})
	}
	for i, iri := range rangeIRIs {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDataPropertyRange,
			SubjectNewEntityRef: selfRef, ValueText: iri, SortOrder: int32(i),
		})
	}
	for i, did := range disjointIDs {
		set.Changes = append(set.Changes,
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDisjointDataProperties,
				SubjectNewEntityRef: selfRef, ObjectID: did, SortOrder: int32(i),
			},
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDisjointDataProperties,
				SubjectID: did, ObjectNewEntityRef: selfRef, SortOrder: int32(i),
			},
		)
	}
	if equivalentID > 0 {
		set.Changes = append(set.Changes,
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomEquivalentDataProperties,
				SubjectNewEntityRef: selfRef, ObjectID: equivalentID, SortOrder: 0,
			},
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomEquivalentDataProperties,
				SubjectID: equivalentID, ObjectNewEntityRef: selfRef, SortOrder: 0,
			},
		)
	}
}
