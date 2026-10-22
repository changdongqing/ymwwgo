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

// OntologyClassService 本体类管理服务（BFF：八端点编排，调域内核 + repo）。
//
// create/update/delete 是自定义 RPC（公理事务管线：diff + 环检测 + 乐观锁 + 校验 + 变更日志），
// 收扁平请求体——不是通用 CRUD（铁律 3 的正确适用，勿包 { data }）。
type OntologyClassService struct {
	adminV1.OntologyClassServiceHTTPServer

	log          *bLogger.Helper
	ontologyRepo *data.OntologyRepo
	entityRepo   *data.OntEntityRepo
	axiomRepo    *data.OntAxiomRepo
	factory      *ontology.EntityFactory
	applier      *ontology.ChangeApplier
}

func NewOntologyClassService(
	ctx *bootstrap.Context,
	ontologyRepo *data.OntologyRepo,
	entityRepo *data.OntEntityRepo,
	axiomRepo *data.OntAxiomRepo,
) *OntologyClassService {
	log := ctx.NewLoggerHelper("ontology-class/service/admin-service")
	return &OntologyClassService{
		log:          log,
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		axiomRepo:    axiomRepo,
		factory:      ontology.NewEntityFactory(entityRepo),
		applier:      ontology.NewChangeApplier(axiomRepo, log),
	}
}

// requireOntology 本体存在且启用检查（实体级接口统一闸门，设计文档 §4.7 #9）
func (s *OntologyClassService) requireOntology(ctx context.Context, ontologyID uint32) (*ent.OntOntology, error) {
	if ontologyID == 0 {
		return nil, adminV1.ErrorBadRequest("ontology id is required")
	}
	ont, err := s.ontologyRepo.GetEntityByID(ctx, ontologyID)
	return checkOntologyEnabled(ont, err)
}

// requireClassEntity 实体存在且为类检查
func (s *OntologyClassService) requireClassEntity(ctx context.Context, id uint32) (*ent.OntEntity, error) {
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
	if e.EntityType == nil || *e.EntityType != ontology.EntityTypeClass {
		return nil, ontologyV1.ErrorEntityNotFound("实体 %d 不是本体类", id)
	}
	return e, nil
}

// ---- 树（全量 / 骨架） ----

func (s *OntologyClassService) Tree(ctx context.Context, req *ontologyV1.GetOntologyClassTreeRequest) (*ontologyV1.OntologyClassTreeResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}
	roots, err := s.buildTree(ctx, req.GetOntologyId(), false)
	if err != nil {
		return nil, err
	}
	return &ontologyV1.OntologyClassTreeResponse{Roots: roots}, nil
}

func (s *OntologyClassService) NavTree(ctx context.Context, req *ontologyV1.GetOntologyClassTreeRequest) (*ontologyV1.OntologyClassNavTreeResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}
	roots, err := s.buildTree(ctx, req.GetOntologyId(), true)
	if err != nil {
		return nil, err
	}
	return &ontologyV1.OntologyClassNavTreeResponse{Roots: roots}, nil
}

// buildTree 层级构建（读路径一次查询；owl:Thing 虚拟根由前端固定渲染）
func (s *OntologyClassService) buildTree(ctx context.Context, ontologyID uint32, nav bool) ([]*ontologyV1.OntologyClassTreeNode, error) {
	entities, err := s.entityRepo.ListEntitiesByType(ctx, ontologyID, ontology.EntityTypeClass)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query entities failed")
	}
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}

	roots := ontology.BuildClassTree(ctx, ontology.ClassHierarchyInput{
		Entities: entities,
		Edges:    ontology.CollectHierarchyEdges(axioms, ontology.AxiomSubClassOf),
	}, s.log)
	if nav {
		roots = ontology.PruneNavTree(roots)
	}

	out := make([]*ontologyV1.OntologyClassTreeNode, 0, len(roots))
	for _, r := range roots {
		out = append(out, treeNodeToProto(r))
	}
	return out, nil
}

func treeNodeToProto(n *ontology.TreeNode) *ontologyV1.OntologyClassTreeNode {
	node := &ontologyV1.OntologyClassTreeNode{
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
		node.Children = make([]*ontologyV1.OntologyClassTreeNode, 0, len(n.Children))
		for _, c := range n.Children {
			node.Children = append(node.Children, treeNodeToProto(c))
		}
	}
	return node
}

// ---- 列表 ----

func (s *OntologyClassService) List(ctx context.Context, req *ontologyV1.ListOntologyClassesRequest) (*ontologyV1.ListOntologyClassesResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}

	// 子树范围：服务端算后代闭包（含自身），前端只传 rootId 一个参数
	var subtreeIDs []uint32
	if req.GetRootId() > 0 {
		axioms, err := s.axiomRepo.ListByOntology(ctx, req.GetOntologyId())
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query axioms failed")
		}
		subtreeIDs = ontology.SubtreeIDs(req.GetRootId(), ontology.CollectHierarchyEdges(axioms, ontology.AxiomSubClassOf))
	}

	entities, total, err := s.entityRepo.ListClasses(ctx, data.ClassListFilter{
		OntologyID:   req.GetOntologyId(),
		LocalName:    req.GetLocalName(),
		DisplayLabel: req.GetDisplayLabel(),
		Deprecated:   req.Deprecated,
		SubtreeIDs:   subtreeIDs,
		Page:         req.GetPage(),
		PageSize:     req.GetPageSize(),
		OrderBy:      req.GetOrderBy(),
	})
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("list classes failed")
	}

	ids := make([]uint32, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.ID)
	}
	aggregates, err := s.entityRepo.AggregateClassInfo(ctx, req.GetOntologyId(), ids)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("aggregate class info failed")
	}

	items := make([]*ontologyV1.OntologyClass, 0, len(entities))
	for _, e := range entities {
		items = append(items, data.EntEntityToClassDTO(e, aggregates[e.ID]))
	}
	return &ontologyV1.ListOntologyClassesResponse{Items: items, Total: uint64(total)}, nil
}

// ---- frame ----

func (s *OntologyClassService) Frame(ctx context.Context, req *ontologyV1.GetOntologyClassFrameRequest) (*ontologyV1.OntologyClassFrame, error) {
	e, err := s.requireClassEntity(ctx, req.GetId())
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

	frame := &ontologyV1.OntologyClassFrame{
		Id:           &e.ID,
		OntologyId:   &ontologyID,
		Iri:          e.Iri,
		LocalName:    e.LocalName,
		DisplayLabel: e.DisplayLabel,
		Deprecated:   e.Deprecated,
		Comment:      ontology.PickAnnotationPreferred(subjectAxioms, ontology.PredicateRDFSComment),
		Notation:     ontology.PickAnnotationPreferred(subjectAxioms, ontology.PredicateSKOSNotation),
		BaseVersion:  ont.Version,
		UpdatedAt:    timePtrToTimestamp(e.UpdatedAt),
	}

	// 父类（保序 sort_order）
	parentIDs := make([]uint32, 0)
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomSubClassOf || ax.ObjectID == nil {
			continue
		}
		parentIDs = append(parentIDs, *ax.ObjectID)
	}
	if len(parentIDs) > 0 {
		parents, err := s.entityRepo.GetEntitiesByIDs(ctx, ontologyID, parentIDs)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query parent entities failed")
		}
		byID := entityMap(parents)
		frame.ParentIds = parentIDs
		frame.Parents = make([]*ontologyV1.EntityBrief, 0, len(parentIDs))
		for _, pid := range parentIDs {
			if p, ok := byID[pid]; ok {
				frame.Parents = append(frame.Parents, entityBrief(p))
			}
		}
	}

	// 不相交（对称伙伴去重）：以我为主语的宾语 ∪ 以我为宾语的主语（frame 只带 id，briefs 由前端按需取）
	frame.DisjointIds = symmetricPartners(e.ID, subjectAxioms, objectAxioms, ontology.AxiomDisjointWith)

	// 等价（V1 单值，成对存储取主语侧）
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomEquivalentClasses || ax.ObjectID == nil {
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

// symmetricPartners 对称关系的伙伴集合（双向并集、保序去重）
func symmetricPartners(me uint32, subjectAxioms, objectAxioms []*ent.OntAxiom, axiomType uint32) []uint32 {
	seen := make(map[uint32]struct{})
	out := make([]uint32, 0)
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != axiomType || ax.ObjectID == nil {
			continue
		}
		if _, ok := seen[*ax.ObjectID]; !ok {
			seen[*ax.ObjectID] = struct{}{}
			out = append(out, *ax.ObjectID)
		}
	}
	for _, ax := range objectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != axiomType || ax.SubjectID == nil {
			continue
		}
		if _, ok := seen[*ax.SubjectID]; !ok {
			seen[*ax.SubjectID] = struct{}{}
			out = append(out, *ax.SubjectID)
		}
	}
	return out
}

// ---- usage ----

func (s *OntologyClassService) Usage(ctx context.Context, req *ontologyV1.GetOntologyClassUsageRequest) (*ontologyV1.EntityUsage, error) {
	e, err := s.requireClassEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	// 引用集 = object 侧、ClassUsageAxiomTypes 内的未删公理（02/03/06 期扩号段时改常量切片）
	axioms, err := s.axiomRepo.ListByObject(ctx, ontologyID, e.ID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query references failed")
	}

	type refKey struct {
		entityID  uint32
		axiomType uint32
	}
	seen := make(map[refKey]struct{})
	refKeys := make([]refKey, 0)
	for _, ax := range axioms {
		if ax.SubjectID == nil || ax.AxiomType == nil {
			continue
		}
		if !isClassUsageAxiomType(*ax.AxiomType) {
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

func isClassUsageAxiomType(t uint32) bool {
	for _, v := range ontology.ClassUsageAxiomTypes {
		if v == t {
			return true
		}
	}
	return false
}

// ---- create / update / delete ----

func (s *OntologyClassService) Create(ctx context.Context, req *ontologyV1.CreateOntologyClassRequest) (*ontologyV1.CreateOntologyClassResponse, error) {
	if req == nil || req.GetOntologyId() == 0 || req.GetLocalName() == "" {
		return nil, adminV1.ErrorBadRequest("ontology id and local name are required")
	}
	ont, err := s.requireOntology(ctx, req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	baseIRI := derefStr(ont.BaseIri)

	// 实体草稿：局部名校验 + IRI 前置查重（友好报错带 IRI 明文）
	draft, declaration, err := s.factory.CreateEntityDraft(ctx, req.GetOntologyId(), ontology.EntityTypeClass, baseIRI, req.GetLocalName())
	if err != nil {
		return nil, mapDomainError(err)
	}

	// 引用校验：均为本体下已声明的类（create 不做自引用校验——新实体尚无 id）
	parentIDs := dedupeUint32(req.GetParentIds())
	disjointIDs := dedupeUint32(req.GetDisjointIds())
	if err = s.factory.ValidateRefEntities(ctx, req.GetOntologyId(), parentIDs, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
		return nil, mapDomainError(err)
	}
	if err = s.factory.ValidateRefEntities(ctx, req.GetOntologyId(), disjointIDs, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
		return nil, mapDomainError(err)
	}
	var equivalentID uint32
	if req.EquivalentId != nil && *req.EquivalentId > 0 {
		equivalentID = *req.EquivalentId
		if err = s.factory.ValidateRefEntities(ctx, req.GetOntologyId(), []uint32{equivalentID}, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
			return nil, mapDomainError(err)
		}
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
	if err = s.assertNoCycleForClass(ctx, req.GetOntologyId(), 0, parentIDs); err != nil {
		return nil, err
	}

	// 组装变更集：新实体放 NewEntities[0]，公理以 SubjectNewEntityRef=0 引用
	set := &ontology.EntityChangeSet{
		OntologyID:  req.GetOntologyId(),
		Operation:   ontology.OpCreateClass,
		NewEntities: []ontology.EntityDraft{*draft},
		Changes:     []ontology.AxiomChange{declaration},
	}
	appendRelationAdds(set, parentIDs, disjointIDs, equivalentID)
	appendAnnotationAdds(set, req.DisplayLabel, req.Comment, req.Notation, req.Deprecated)
	ontology.AppendCustomAnnotationAxioms(set, 1, 0, annotationInputs) // 主语 = 新实体（NewEntities[0]）

	result, err := s.applier.Apply(ctx, set, operatorUserID(ctx))
	if err != nil {
		return nil, mapDomainError(err)
	}

	resp := &ontologyV1.CreateOntologyClassResponse{BaseVersion: &result.NewVersion}
	if len(result.NewEntityIDs) > 0 {
		resp.Id = &result.NewEntityIDs[0]
	}
	return resp, nil
}

func (s *OntologyClassService) Update(ctx context.Context, req *ontologyV1.UpdateOntologyClassRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireClassEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	// local_name 忽略（不可变）：不参与 diff、不校验

	// 引用校验（update 时不得自引用：父类自引用走环检测，不相交/等价自引用直接拒绝）
	parentIDs := dedupeUint32(req.GetParentIds())
	disjointIDs := dedupeUint32(req.GetDisjointIds())
	if err = s.factory.ValidateRefEntities(ctx, ontologyID, parentIDs, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
		return nil, mapDomainError(err)
	}
	if err = s.factory.ValidateRefEntities(ctx, ontologyID, disjointIDs, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
		return nil, mapDomainError(err)
	}
	for _, did := range disjointIDs {
		if did == req.GetId() {
			return nil, ontologyV1.ErrorEntityTypeMismatch("不能与自身不相交：%d", did)
		}
	}
	var equivalentTarget []uint32
	equivalentSet := req.EquivalentId != nil
	if equivalentSet && *req.EquivalentId > 0 {
		if *req.EquivalentId == req.GetId() {
			return nil, ontologyV1.ErrorEntityTypeMismatch("不能与自身等价：%d", *req.EquivalentId)
		}
		if err = s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{*req.EquivalentId}, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
			return nil, mapDomainError(err)
		}
		equivalentTarget = []uint32{*req.EquivalentId}
	}

	// 环检测（全量新父集；含"把子孙设为父类"与自环）
	if err = s.assertNoCycleForClass(ctx, ontologyID, req.GetId(), parentIDs); err != nil {
		return nil, err
	}

	// frame 全量提交语义 diff
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	subjectAxioms := filterBySubject(axioms, req.GetId())
	annotationAxioms := filterByType(subjectAxioms, ontology.AxiomAnnotationAssertion)

	changes := ontology.DiffSubjectSideRelations(req.GetId(), ontology.AxiomSubClassOf, filterByType(subjectAxioms, ontology.AxiomSubClassOf), parentIDs)
	changes = append(changes, ontology.DiffSymmetricPairs(req.GetId(), ontology.AxiomDisjointWith,
		buildSymmetricPairs(req.GetId(), axioms, ontology.AxiomDisjointWith), disjointIDs)...)
	if equivalentSet {
		changes = append(changes, ontology.DiffSymmetricPairs(req.GetId(), ontology.AxiomEquivalentClasses,
			buildSymmetricPairs(req.GetId(), axioms, ontology.AxiomEquivalentClasses), equivalentTarget)...)
	}
	changes = append(changes, ontology.DiffAnnotations(req.GetId(), annotationAxioms, map[string]*string{
		ontology.PredicateRDFSLabel:    req.DisplayLabel, // nil = 不变；空串 = 清空（protojson null 归并缺省）
		ontology.PredicateRDFSComment:  req.Comment,
		ontology.PredicateSKOSNotation: req.Notation,
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
		Operation:   ontology.OpUpdateClass,
		Changes:     changes,
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OntologyClassService) Delete(ctx context.Context, req *ontologyV1.DeleteOntologyClassRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireClassEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	force := req.Force != nil && *req.Force

	// 删除保护：存在引用且未 force → ENTITY_IN_USE（409，前端拉 usage 明细弹窗）。
	// 06 期增量：CountReferences 的类型集已并入 60（个体断型）。
	refsCount, err := s.axiomRepo.CountReferences(ctx, ontologyID, req.GetId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("count references failed")
	}
	// 06 期增量（双态拒绝）：有实例断言（60）的类 force=true 亦拒绝——级联删 60 行产出
	// "无型个体"破坏创建不变量、级联删个体则把单实体删除放大为批量删除，均不可接受；
	// 非 60 引用（20/21/22/31/32/41）保持 force=true 级联既有语义（06 号文档 §4.4）。
	instanceCount, err := s.axiomRepo.CountReferencesByTypes(ctx, ontologyID, req.GetId(), []uint32{ontology.AxiomTypeAssertion})
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("count instances failed")
	}
	if instanceCount > 0 {
		return nil, ontologyV1.ErrorEntityInUse("该类存在 %d 个实例个体，请先删除或改挂他类", instanceCount)
	}
	if refsCount > 0 && !force {
		return nil, ontologyV1.ErrorEntityInUse("实体被 %d 处引用，无法删除", refsCount)
	}

	// 收集删除变更：主语侧全部公理（声明/层级/对称/注解）+ 宾语侧引用公理
	subjectAxioms, err := s.axiomRepo.ListBySubject(ctx, ontologyID, req.GetId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	objectAxioms, err := s.axiomRepo.ListByObject(ctx, ontologyID, req.GetId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}

	changes := make([]ontology.AxiomChange, 0, len(subjectAxioms)+len(objectAxioms))
	for _, ax := range subjectAxioms {
		changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: ax.ID, SubjectID: derefU32(ax.SubjectID)})
	}
	for _, ax := range objectAxioms {
		if ax.SubjectID != nil && *ax.SubjectID == req.GetId() {
			continue // 已在主语侧收集
		}
		changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: ax.ID, SubjectID: derefU32(ax.SubjectID)})
	}

	// force 级联：实体行一并软删；子类自动变根类（其 20 型边已随主语侧/宾语侧公理删除）
	set := &ontology.EntityChangeSet{
		OntologyID:      ontologyID,
		BaseVersion:     req.BaseVersion,
		Operation:       ontology.OpDeleteClass,
		Changes:         changes,
		DeleteEntityIDs: []uint32{req.GetId()},
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- 组装辅助 ----

// appendRelationAdds 父类/不相交/等价的 ADD 变更（不相交与等价成对存储）
func appendRelationAdds(set *ontology.EntityChangeSet, parentIDs, disjointIDs []uint32, equivalentID uint32) {
	for i, pid := range parentIDs {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op:                  ontology.AxiomOpAdd,
			AxiomType:           ontology.AxiomSubClassOf,
			SubjectNewEntityRef: 1, // 主语 = 新实体（NewEntities[0]）
			ObjectID:            pid,
			SortOrder:           int32(i),
		})
	}
	for i, did := range disjointIDs {
		set.Changes = append(set.Changes,
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDisjointWith,
				SubjectNewEntityRef: 1, ObjectID: did, SortOrder: int32(i),
			},
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDisjointWith,
				SubjectID: did, ObjectNewEntityRef: 1, SortOrder: int32(i),
			},
		)
	}
	if equivalentID > 0 {
		set.Changes = append(set.Changes,
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomEquivalentClasses,
				SubjectNewEntityRef: 1, ObjectID: equivalentID, SortOrder: 0,
			},
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomEquivalentClasses,
				SubjectID: equivalentID, ObjectNewEntityRef: 1, SortOrder: 0,
			},
		)
	}
}

// appendAnnotationAdds 创建时的注解断言（label/comment 带 zh；deprecated/notation 无 lang；
// 空串与缺省都不落公理）
func appendAnnotationAdds(set *ontology.EntityChangeSet, displayLabel, comment, notation *string, deprecated *bool) {
	add := func(predicate, value, lang string) {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op:                  ontology.AxiomOpAdd,
			AxiomType:           ontology.AxiomAnnotationAssertion,
			SubjectNewEntityRef: 1, // 主语 = 新实体（NewEntities[0]）
			PredicateIRI:        predicate,
			ValueText:           value,
			Lang:                lang,
		})
	}
	if displayLabel != nil && *displayLabel != "" {
		add(ontology.PredicateRDFSLabel, *displayLabel, "zh")
	}
	if comment != nil && *comment != "" {
		add(ontology.PredicateRDFSComment, *comment, "zh")
	}
	if notation != nil && *notation != "" {
		add(ontology.PredicateSKOSNotation, *notation, "")
	}
	if deprecated != nil && *deprecated {
		add(ontology.PredicateOWLDeprecated, "true", "")
	}
}

// assertNoCycleForClass 环检测（写路径前置，读全量 20 型边）
func (s *OntologyClassService) assertNoCycleForClass(ctx context.Context, ontologyID, targetID uint32, parentIDs []uint32) error {
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return adminV1.ErrorInternalServerError("query axioms failed")
	}
	edges := ontology.CollectHierarchyEdges(axioms, ontology.AxiomSubClassOf)
	var names map[uint32]string
	if entities, listErr := s.entityRepo.ListEntitiesByType(ctx, ontologyID, ontology.EntityTypeClass); listErr == nil {
		names = make(map[uint32]string, len(entities))
		for _, e := range entities {
			names[e.ID] = derefStr(e.LocalName)
		}
	}
	if err := ontology.AssertNoCycle(targetID, edges, parentIDs, names); err != nil {
		return mapDomainError(err)
	}
	return nil
}

// buildSymmetricPairs 从全量公理构建对称关系现状（双向齐备才成对）
func buildSymmetricPairs(me uint32, axioms []*ent.OntAxiom, axiomType uint32) []ontology.SymmetricAxiomPair {
	forward := make(map[uint32]uint32) // other → (me→other) 公理 id
	backward := make(map[uint32]uint32)
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != axiomType || ax.SubjectID == nil {
			continue
		}
		objectID := derefU32(ax.ObjectID)
		if *ax.SubjectID == me && objectID != 0 && objectID != me {
			forward[objectID] = ax.ID
		}
		if objectID == me && *ax.SubjectID != me {
			backward[*ax.SubjectID] = ax.ID
		}
	}
	pairs := make([]ontology.SymmetricAxiomPair, 0, len(forward))
	for other, fid := range forward {
		bid, ok := backward[other]
		if !ok {
			continue // 脏数据防御：单向行不成对，不进 diff（校验器不管成对性）
		}
		pairs = append(pairs, ontology.SymmetricAxiomPair{OtherID: other, ForwardAxiomID: fid, BackwardAxiomID: bid})
	}
	return pairs
}

func filterBySubject(axioms []*ent.OntAxiom, subjectID uint32) []*ent.OntAxiom {
	out := make([]*ent.OntAxiom, 0, len(axioms))
	for _, ax := range axioms {
		if ax.SubjectID != nil && *ax.SubjectID == subjectID {
			out = append(out, ax)
		}
	}
	return out
}

func filterByObject(axioms []*ent.OntAxiom, objectID uint32) []*ent.OntAxiom {
	out := make([]*ent.OntAxiom, 0)
	for _, ax := range axioms {
		if ax.ObjectID != nil && *ax.ObjectID == objectID {
			out = append(out, ax)
		}
	}
	return out
}

func filterByType(axioms []*ent.OntAxiom, axiomType uint32) []*ent.OntAxiom {
	out := make([]*ent.OntAxiom, 0)
	for _, ax := range axioms {
		if ax.AxiomType != nil && *ax.AxiomType == axiomType {
			out = append(out, ax)
		}
	}
	return out
}

func entityMap(entities []*ent.OntEntity) map[uint32]*ent.OntEntity {
	m := make(map[uint32]*ent.OntEntity, len(entities))
	for _, e := range entities {
		m[e.ID] = e
	}
	return m
}

func entityBrief(e *ent.OntEntity) *ontologyV1.EntityBrief {
	return &ontologyV1.EntityBrief{
		Id:           &e.ID,
		LocalName:    e.LocalName,
		DisplayLabel: e.DisplayLabel,
		Deprecated:   e.Deprecated,
	}
}

// derefU32 / derefStr ent nillable 字段解引用（生成实体不带 getter）
func derefU32(p *uint32) uint32 {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
