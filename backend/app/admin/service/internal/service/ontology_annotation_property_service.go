package service

import (
	"context"

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

// ============================================================================
// 自定义注解区共用辅助（04 期增量，六类实体 frame / Create / Update 共用——04 号文档 §2.5/§4.3）
// ============================================================================

// frameAnnotationsOf 自定义注解区组装：主语侧 100 行中 object_id 非空的行 → AnnotationAssertion
// （注解属性实体名称批量反查，保序 sort_order → id；缺失实体行跳过，不阻塞 frame 展示）。
// 内建谓词行（predicate_iri 路径）不进本区——它们由各 frame 的专属字段承载。
func frameAnnotationsOf(ctx context.Context, entityRepo *data.OntEntityRepo, ontologyID uint32, subjectAxioms []*ent.OntAxiom) ([]*ontologyV1.AnnotationAssertion, error) {
	rows := make([]*ent.OntAxiom, 0, len(subjectAxioms))
	propertyIDs := make([]uint32, 0, len(subjectAxioms))
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomAnnotationAssertion {
			continue
		}
		if ax.ObjectID == nil || *ax.ObjectID == 0 {
			continue
		}
		rows = append(rows, ax)
		propertyIDs = append(propertyIDs, *ax.ObjectID)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	entities, err := entityRepo.GetEntitiesByIDs(ctx, ontologyID, propertyIDs)
	if err != nil {
		return nil, err
	}
	byID := entityMap(entities)

	out := make([]*ontologyV1.AnnotationAssertion, 0, len(rows))
	for _, ax := range rows {
		assertion := &ontologyV1.AnnotationAssertion{
			PropertyId: ax.ObjectID,
			Value:      ax.ValueText,
			Lang:       ax.Lang,
		}
		if p, ok := byID[*ax.ObjectID]; ok {
			assertion.PropertyLocalName = p.LocalName
			assertion.PropertyDisplayLabel = p.DisplayLabel
		}
		out = append(out, assertion)
	}
	return out, nil
}

// normalizeAnnotationInputs 请求载荷 → 域内核归一化输入（值空白拒绝 ANNOTATION_VALUE_INVALID /
// lang 空白归空串 / (property_id, value, lang) 三元组有序去重——04 号文档 §4.3）。
func normalizeAnnotationInputs(inputs []*ontologyV1.AnnotationAssertionInput) ([]ontology.CustomAnnotationInput, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	raw := make([]ontology.CustomAnnotationInput, 0, len(inputs))
	for _, in := range inputs {
		if in == nil {
			continue
		}
		raw = append(raw, ontology.CustomAnnotationInput{PropertyID: in.GetPropertyId(), Value: in.GetValue(), Lang: in.GetLang()})
	}
	out, err := ontology.NormalizeCustomAnnotationInputs(raw)
	if err != nil {
		return nil, mapDomainError(err)
	}
	return out, nil
}

// validateAnnotationPropertyRefs 自定义注解谓词校验：property_id 逐个须为本本体已声明的
// 注解属性实体（服务层前置；校验器 100 分支兜底直改库场景——04 号文档 §7 #4）。
func validateAnnotationPropertyRefs(ctx context.Context, f *ontology.EntityFactory, ontologyID uint32, inputs []ontology.CustomAnnotationInput) error {
	if len(inputs) == 0 {
		return nil
	}
	ids := make([]uint32, 0, len(inputs))
	for _, in := range inputs {
		ids = append(ids, in.PropertyID)
	}
	if err := f.ValidateRefEntities(ctx, ontologyID, ids, ontology.EntityTypeAnnotationProperty, ontology.ReasonAnnotationPropertyTargetInvalid); err != nil {
		return mapDomainError(err)
	}
	return nil
}

// ============================================================================
// OntologyAnnotationPropertyService 注解属性管理服务（BFF：七端点编排，调域内核 + repo）。
//
// create/update/delete 是自定义 RPC（公理事务管线：diff + 环检测 + 乐观锁 + 校验 + 变更日志），
// 收扁平请求体——不是通用 CRUD（铁律 3 的正确适用，勿包 { data }）。
// 与数据属性服务的差异面（04 号文档 §2.7 矩阵）：无定义域/值域/特征/等价/不相交（OWL 2 不给
// 注解属性任何逻辑结构），多一个自定义注解区（frame annotations + DiffCustomAnnotations，递归）；
// usage 反查 50 + 100（object_id 路径）；虚拟根 owl:topAnnotationProperty。
type OntologyAnnotationPropertyService struct {
	adminV1.OntologyAnnotationPropertyServiceHTTPServer

	log          *bLogger.Helper
	ontologyRepo *data.OntologyRepo
	entityRepo   *data.OntEntityRepo
	axiomRepo    *data.OntAxiomRepo
	factory      *ontology.EntityFactory
	applier      *ontology.ChangeApplier
}

func NewOntologyAnnotationPropertyService(
	ctx *bootstrap.Context,
	ontologyRepo *data.OntologyRepo,
	entityRepo *data.OntEntityRepo,
	axiomRepo *data.OntAxiomRepo,
) *OntologyAnnotationPropertyService {
	log := ctx.NewLoggerHelper("ontology-annotation-property/service/admin-service")
	return &OntologyAnnotationPropertyService{
		log:          log,
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		axiomRepo:    axiomRepo,
		factory:      ontology.NewEntityFactory(entityRepo),
		applier:      ontology.NewChangeApplier(axiomRepo, log),
	}
}

// requireOntology 本体存在且启用检查（实体级接口统一闸门）
func (s *OntologyAnnotationPropertyService) requireOntology(ctx context.Context, ontologyID uint32) (*ent.OntOntology, error) {
	if ontologyID == 0 {
		return nil, adminV1.ErrorBadRequest("ontology id is required")
	}
	ont, err := s.ontologyRepo.GetEntityByID(ctx, ontologyID)
	return checkOntologyEnabled(ont, err)
}

// requireAnnotationPropertyEntity 实体存在且为注解属性检查
func (s *OntologyAnnotationPropertyService) requireAnnotationPropertyEntity(ctx context.Context, id uint32) (*ent.OntEntity, error) {
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
	if e.EntityType == nil || *e.EntityType != ontology.EntityTypeAnnotationProperty {
		return nil, ontologyV1.ErrorAnnotationPropertyTargetInvalid("实体 %d 不是注解属性", id)
	}
	return e, nil
}

// ---- 树（全量；owl:topAnnotationProperty 虚拟根由前端固定渲染） ----

func (s *OntologyAnnotationPropertyService) Tree(ctx context.Context, req *ontologyV1.GetAnnotationPropertyTreeRequest) (*ontologyV1.AnnotationPropertyTreeResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}

	entities, err := s.entityRepo.ListEntitiesByType(ctx, req.GetOntologyId(), ontology.EntityTypeAnnotationProperty)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query entities failed")
	}
	axioms, err := s.axiomRepo.ListByOntology(ctx, req.GetOntologyId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}

	roots := ontology.BuildClassTree(ctx, ontology.ClassHierarchyInput{
		Entities: entities,
		Edges:    ontology.CollectHierarchyEdges(axioms, ontology.AxiomSubAnnotationPropertyOf),
	}, s.log)

	out := make([]*ontologyV1.AnnotationPropertyTreeNode, 0, len(roots))
	for _, r := range roots {
		out = append(out, annotationPropertyTreeNodeToProto(r))
	}
	return &ontologyV1.AnnotationPropertyTreeResponse{Roots: out}, nil
}

func annotationPropertyTreeNodeToProto(n *ontology.TreeNode) *ontologyV1.AnnotationPropertyTreeNode {
	node := &ontologyV1.AnnotationPropertyTreeNode{
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
		node.Children = make([]*ontologyV1.AnnotationPropertyTreeNode, 0, len(n.Children))
		for _, c := range n.Children {
			node.Children = append(node.Children, annotationPropertyTreeNodeToProto(c))
		}
	}
	return node
}

// ---- 列表 ----

func (s *OntologyAnnotationPropertyService) List(ctx context.Context, req *ontologyV1.ListAnnotationPropertiesRequest) (*ontologyV1.ListAnnotationPropertiesResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}

	entities, total, err := s.entityRepo.ListAnnotationProperties(ctx, data.ClassListFilter{
		OntologyID:   req.GetOntologyId(),
		LocalName:    req.GetLocalName(),
		DisplayLabel: req.GetDisplayLabel(),
		Deprecated:   req.Deprecated,
		Page:         req.GetPage(),
		PageSize:     req.GetPageSize(),
		OrderBy:      req.GetOrderBy(),
	})
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("list annotation properties failed")
	}

	ids := make([]uint32, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.ID)
	}
	aggregates, err := s.entityRepo.AggregateAnnotationPropertyInfo(ctx, req.GetOntologyId(), ids)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("aggregate annotation property info failed")
	}

	items := make([]*ontologyV1.AnnotationProperty, 0, len(entities))
	for _, e := range entities {
		items = append(items, data.EntEntityToAnnotationPropertyDTO(e, aggregates[e.ID]))
	}
	return &ontologyV1.ListAnnotationPropertiesResponse{Items: items, Total: uint64(total)}, nil
}

// ---- frame ----

func (s *OntologyAnnotationPropertyService) Frame(ctx context.Context, req *ontologyV1.GetAnnotationPropertyFrameRequest) (*ontologyV1.AnnotationPropertyFrame, error) {
	e, err := s.requireAnnotationPropertyEntity(ctx, req.GetId())
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

	frame := &ontologyV1.AnnotationPropertyFrame{
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

	// 父属性（保序 sort_order）
	superIDs := objectIDsOf(subjectAxioms, ontology.AxiomSubAnnotationPropertyOf)
	if len(superIDs) > 0 {
		briefs, err := s.briefsByIDs(ctx, ontologyID, superIDs)
		if err != nil {
			return nil, err
		}
		frame.SuperPropertyIds = superIDs
		frame.SuperProperties = briefs
	}

	// 自定义注解区（内建字段之外；注解属性自身也可挂注解，递归——04 号文档 §2.5）
	annotations, err := frameAnnotationsOf(ctx, s.entityRepo, ontologyID, subjectAxioms)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query annotations failed")
	}
	frame.Annotations = annotations

	return frame, nil
}

// briefsByIDs 按 id 序取实体简况（frame 各 section 共用；缺失实体跳过——不阻塞 frame 展示）
func (s *OntologyAnnotationPropertyService) briefsByIDs(ctx context.Context, ontologyID uint32, ids []uint32) ([]*ontologyV1.EntityBrief, error) {
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

// ---- usage ----

// Usage 引用明细（对应 OWLEntityUsageViewComponent）：50 子注解属性（P 是父属性）+
// 100 被注解（P 是自定义谓词，object_id 路径命中）。无域值域/伙伴扫描——注解属性没有这些关系。
func (s *OntologyAnnotationPropertyService) Usage(ctx context.Context, req *ontologyV1.GetAnnotationPropertyUsageRequest) (*ontologyV1.EntityUsage, error) {
	e, err := s.requireAnnotationPropertyEntity(ctx, req.GetId())
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
		AxiomTypes: ontology.AnnotationPropertyUsageAxiomTypes,
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

func (s *OntologyAnnotationPropertyService) Create(ctx context.Context, req *ontologyV1.CreateAnnotationPropertyRequest) (*ontologyV1.CreateAnnotationPropertyResponse, error) {
	if req == nil || req.GetOntologyId() == 0 || req.GetLocalName() == "" {
		return nil, adminV1.ErrorBadRequest("ontology id and local name are required")
	}
	ont, err := s.requireOntology(ctx, req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	baseIRI := derefStr(ont.BaseIri)

	// 实体草稿：局部名校验 + IRI 前置查重
	draft, declaration, err := s.factory.CreateEntityDraft(ctx, req.GetOntologyId(), ontology.EntityTypeAnnotationProperty, baseIRI, req.GetLocalName())
	if err != nil {
		return nil, mapDomainError(err)
	}

	// 校验管线（04 号文档 §4.2）：父属性引用 + 自定义注解归一化与谓词校验
	superIDs := dedupeUint32(req.GetSuperPropertyIds())
	if err = s.factory.ValidateRefEntities(ctx, req.GetOntologyId(), superIDs, ontology.EntityTypeAnnotationProperty, ontology.ReasonAnnotationPropertyTargetInvalid); err != nil {
		return nil, mapDomainError(err)
	}
	annotationInputs, err := normalizeAnnotationInputs(req.GetAnnotations())
	if err != nil {
		return nil, err
	}
	if err = validateAnnotationPropertyRefs(ctx, s.factory, req.GetOntologyId(), annotationInputs); err != nil {
		return nil, err
	}

	// 环检测（create 时新实体无任何入边，理论上恒通过；防御性保留同一管线）
	if err = s.assertNoCycleForAnnotationProperty(ctx, req.GetOntologyId(), 0, superIDs); err != nil {
		return nil, err
	}

	// 组装变更集：新实体放 NewEntities[0]
	set := &ontology.EntityChangeSet{
		OntologyID:  req.GetOntologyId(),
		Operation:   ontology.OpCreateAnnotationProperty,
		NewEntities: []ontology.EntityDraft{*draft},
		Changes:     []ontology.AxiomChange{declaration},
	}
	for i, pid := range superIDs {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomSubAnnotationPropertyOf,
			SubjectNewEntityRef: 1, ObjectID: pid, SortOrder: int32(i),
		})
	}
	appendAnnotationAdds(set, req.DisplayLabel, req.Comment, nil, req.Deprecated)
	ontology.AppendCustomAnnotationAxioms(set, 1, 0, annotationInputs) // 主语 = 新实体（NewEntities[0]）

	result, err := s.applier.Apply(ctx, set, operatorUserID(ctx))
	if err != nil {
		return nil, mapDomainError(err)
	}

	resp := &ontologyV1.CreateAnnotationPropertyResponse{BaseVersion: &result.NewVersion}
	if len(result.NewEntityIDs) > 0 {
		resp.Id = &result.NewEntityIDs[0]
	}
	return resp, nil
}

func (s *OntologyAnnotationPropertyService) Update(ctx context.Context, req *ontologyV1.UpdateAnnotationPropertyRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireAnnotationPropertyEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	// local_name 忽略（不可变）：不参与 diff、不校验

	superIDs := dedupeUint32(req.GetSuperPropertyIds())
	if err = s.factory.ValidateRefEntities(ctx, ontologyID, superIDs, ontology.EntityTypeAnnotationProperty, ontology.ReasonAnnotationPropertyTargetInvalid); err != nil {
		return nil, mapDomainError(err)
	}
	// 自引用语义：superPropertyIds 含自身 → 交由环检测（HIERARCHY_CYCLIC；04 号文档 §4.2 第 4 步）
	annotationInputs, err := normalizeAnnotationInputs(req.GetAnnotations())
	if err != nil {
		return nil, err
	}
	if err = validateAnnotationPropertyRefs(ctx, s.factory, ontologyID, annotationInputs); err != nil {
		return nil, err
	}

	// 环检测（全量新父集；含"把子孙设为父属性"与自环）
	if err = s.assertNoCycleForAnnotationProperty(ctx, ontologyID, req.GetId(), superIDs); err != nil {
		return nil, err
	}

	// frame 全量提交语义 diff（逐 section）
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	subjectAxioms := filterBySubject(axioms, req.GetId())
	annotationAxioms := filterByType(subjectAxioms, ontology.AxiomAnnotationAssertion)

	changes := ontology.DiffSubjectSideRelations(req.GetId(), ontology.AxiomSubAnnotationPropertyOf,
		filterByType(subjectAxioms, ontology.AxiomSubAnnotationPropertyOf), superIDs)
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
		Operation:   ontology.OpUpdateAnnotationProperty,
		Changes:     changes,
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OntologyAnnotationPropertyService) Delete(ctx context.Context, req *ontologyV1.DeleteAnnotationPropertyRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireAnnotationPropertyEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	force := req.Force != nil && *req.Force

	// 删除保护：object 侧命中（50 子属性边 + 100 被注解）计数（04 号文档 §4.4）
	refsCount, err := s.axiomRepo.CountReferencesByTypes(ctx, ontologyID, req.GetId(), ontology.AnnotationPropertyUsageAxiomTypes)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("count references failed")
	}
	if refsCount > 0 && !force {
		return nil, ontologyV1.ErrorEntityInUse("实体被 %d 处引用，无法删除", refsCount)
	}

	// 收集删除变更：主语侧全部公理（声明/父边/自身注解）
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
	// 宾语侧（50 子属性边——子属性自动变根属性；他人挂的自定义注解断言随删）
	// 被注解实体本身及其内建字段注解完好（对齐 OWLEntityDeleter "删提及实体的公理"语义）
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
		Operation:       ontology.OpDeleteAnnotationProperty,
		Changes:         changes,
		DeleteEntityIDs: []uint32{req.GetId()},
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// assertNoCycleForAnnotationProperty 环检测（写路径前置，读全量 50 型边）
func (s *OntologyAnnotationPropertyService) assertNoCycleForAnnotationProperty(ctx context.Context, ontologyID, targetID uint32, superIDs []uint32) error {
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return adminV1.ErrorInternalServerError("query axioms failed")
	}
	edges := ontology.CollectHierarchyEdges(axioms, ontology.AxiomSubAnnotationPropertyOf)
	var names map[uint32]string
	if entities, listErr := s.entityRepo.ListEntitiesByType(ctx, ontologyID, ontology.EntityTypeAnnotationProperty); listErr == nil {
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
