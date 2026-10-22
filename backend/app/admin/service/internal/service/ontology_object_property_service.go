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

// OntologyObjectPropertyService 对象属性管理服务（BFF：七端点编排，调域内核 + repo）。
//
// create/update/delete 是自定义 RPC（公理事务管线：diff + 环检测 + 乐观锁 + 校验 + 变更日志），
// 收扁平请求体——不是通用 CRUD（铁律 3 的正确适用，勿包 { data }）。
// 与类服务的差异面：七特征（33 型 predicate_iri）、属性链（37 型 value_text JSON）、
// 域/值域指向类（31/32）、无 nav 树与子树范围（属性量级小，全量树 + 定位高亮）。
type OntologyObjectPropertyService struct {
	adminV1.OntologyObjectPropertyServiceHTTPServer

	log          *bLogger.Helper
	ontologyRepo *data.OntologyRepo
	entityRepo   *data.OntEntityRepo
	axiomRepo    *data.OntAxiomRepo
	factory      *ontology.EntityFactory
	applier      *ontology.ChangeApplier
}

func NewOntologyObjectPropertyService(
	ctx *bootstrap.Context,
	ontologyRepo *data.OntologyRepo,
	entityRepo *data.OntEntityRepo,
	axiomRepo *data.OntAxiomRepo,
) *OntologyObjectPropertyService {
	log := ctx.NewLoggerHelper("ontology-object-property/service/admin-service")
	return &OntologyObjectPropertyService{
		log:          log,
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		axiomRepo:    axiomRepo,
		factory:      ontology.NewEntityFactory(entityRepo),
		applier:      ontology.NewChangeApplier(axiomRepo, log),
	}
}

// requireOntology 本体存在且启用检查（实体级接口统一闸门，设计文档 §4.7 #9）
func (s *OntologyObjectPropertyService) requireOntology(ctx context.Context, ontologyID uint32) (*ent.OntOntology, error) {
	if ontologyID == 0 {
		return nil, adminV1.ErrorBadRequest("ontology id is required")
	}
	ont, err := s.ontologyRepo.GetEntityByID(ctx, ontologyID)
	return checkOntologyEnabled(ont, err)
}

// requireObjectPropertyEntity 实体存在且为对象属性检查
func (s *OntologyObjectPropertyService) requireObjectPropertyEntity(ctx context.Context, id uint32) (*ent.OntEntity, error) {
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
	if e.EntityType == nil || *e.EntityType != ontology.EntityTypeObjectProperty {
		return nil, ontologyV1.ErrorEntityNotFound("实体 %d 不是对象属性", id)
	}
	return e, nil
}

// ---- 树（全量；无 nav 剪枝） ----

func (s *OntologyObjectPropertyService) Tree(ctx context.Context, req *ontologyV1.GetObjectPropertyTreeRequest) (*ontologyV1.ObjectPropertyTreeResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}

	entities, err := s.entityRepo.ListEntitiesByType(ctx, req.GetOntologyId(), ontology.EntityTypeObjectProperty)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query entities failed")
	}
	axioms, err := s.axiomRepo.ListByOntology(ctx, req.GetOntologyId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}

	roots := ontology.BuildClassTree(ctx, ontology.ClassHierarchyInput{
		Entities: entities,
		Edges:    ontology.CollectHierarchyEdges(axioms, ontology.AxiomSubObjectPropertyOf),
	}, s.log)

	out := make([]*ontologyV1.ObjectPropertyTreeNode, 0, len(roots))
	for _, r := range roots {
		out = append(out, propertyTreeNodeToProto(r))
	}
	return &ontologyV1.ObjectPropertyTreeResponse{Roots: out}, nil
}

func propertyTreeNodeToProto(n *ontology.TreeNode) *ontologyV1.ObjectPropertyTreeNode {
	node := &ontologyV1.ObjectPropertyTreeNode{
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
		node.Children = make([]*ontologyV1.ObjectPropertyTreeNode, 0, len(n.Children))
		for _, c := range n.Children {
			node.Children = append(node.Children, propertyTreeNodeToProto(c))
		}
	}
	return node
}

// ---- 列表 ----

func (s *OntologyObjectPropertyService) List(ctx context.Context, req *ontologyV1.ListObjectPropertiesRequest) (*ontologyV1.ListObjectPropertiesResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}

	entities, total, err := s.entityRepo.ListObjectProperties(ctx, data.ClassListFilter{
		OntologyID:   req.GetOntologyId(),
		LocalName:    req.GetLocalName(),
		DisplayLabel: req.GetDisplayLabel(),
		Deprecated:   req.Deprecated,
		Page:         req.GetPage(),
		PageSize:     req.GetPageSize(),
		OrderBy:      req.GetOrderBy(),
	})
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("list object properties failed")
	}

	ids := make([]uint32, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.ID)
	}
	aggregates, err := s.entityRepo.AggregateObjectPropertyInfo(ctx, req.GetOntologyId(), ids)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("aggregate object property info failed")
	}

	items := make([]*ontologyV1.ObjectProperty, 0, len(entities))
	for _, e := range entities {
		items = append(items, data.EntEntityToObjectPropertyDTO(e, aggregates[e.ID]))
	}
	return &ontologyV1.ListObjectPropertiesResponse{Items: items, Total: uint64(total)}, nil
}

// ---- frame ----

func (s *OntologyObjectPropertyService) Frame(ctx context.Context, req *ontologyV1.GetObjectPropertyFrameRequest) (*ontologyV1.ObjectPropertyFrame, error) {
	e, err := s.requireObjectPropertyEntity(ctx, req.GetId())
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

	frame := &ontologyV1.ObjectPropertyFrame{
		Id:              &e.ID,
		OntologyId:      &ontologyID,
		Iri:             e.Iri,
		LocalName:       e.LocalName,
		DisplayLabel:    e.DisplayLabel,
		Deprecated:      e.Deprecated,
		Comment:         ontology.PickAnnotationPreferred(subjectAxioms, ontology.PredicateRDFSComment),
		Characteristics: orderPropertyCharacteristics(filterByType(subjectAxioms, ontology.AxiomObjectPropertyCharacteristic)),
		BaseVersion:     ont.Version,
		UpdatedAt:       timePtrToTimestamp(e.UpdatedAt),
	}

	// 父属性（保序 sort_order）
	superIDs := make([]uint32, 0)
	for _, ax := range filterByType(subjectAxioms, ontology.AxiomSubObjectPropertyOf) {
		if ax.ObjectID != nil {
			superIDs = append(superIDs, *ax.ObjectID)
		}
	}
	if len(superIDs) > 0 {
		briefs, err := s.briefsByIDs(ctx, ontologyID, superIDs)
		if err != nil {
			return nil, err
		}
		frame.SuperPropertyIds = superIDs
		frame.SuperProperties = briefs
	}

	// 定义域 / 值域（多值保序）
	domainIDs := objectIDsOf(subjectAxioms, ontology.AxiomObjectPropertyDomain)
	if len(domainIDs) > 0 {
		briefs, err := s.briefsByIDs(ctx, ontologyID, domainIDs)
		if err != nil {
			return nil, err
		}
		frame.DomainIds = domainIDs
		frame.Domains = briefs
	}
	rangeIDs := objectIDsOf(subjectAxioms, ontology.AxiomObjectPropertyRange)
	if len(rangeIDs) > 0 {
		briefs, err := s.briefsByIDs(ctx, ontologyID, rangeIDs)
		if err != nil {
			return nil, err
		}
		frame.RangeIds = rangeIDs
		frame.Ranges = briefs
	}

	// 逆属性 / 不相交（对称伙伴去重：双向并集）
	frame.InverseIds = symmetricPartners(e.ID, subjectAxioms, objectAxioms, ontology.AxiomInverseOf)
	frame.DisjointIds = symmetricPartners(e.ID, subjectAxioms, objectAxioms, ontology.AxiomDisjointObjectProperties)

	// 等价（V1 单值，成对存储取主语侧）
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomEquivalentObjectProperties || ax.ObjectID == nil {
			continue
		}
		frame.EquivalentId = ax.ObjectID
		break
	}

	// 属性链（挂本属性为超属性；成员简况批量补齐，保序）
	chains := ontology.PropertyChainsOf(ctx, subjectAxioms, e.ID, s.log)
	parseable := make([]ontology.PropertyChain, 0, len(chains))
	memberIDs := make([]uint32, 0)
	for _, c := range chains {
		if len(c.MemberIDs) > 0 { // 解析失败行不进 frame（脏数据由校验器暴露，diff 自愈）
			parseable = append(parseable, c)
			memberIDs = append(memberIDs, c.MemberIDs...)
		}
	}
	if len(parseable) > 0 {
		memberEntities, err := s.entityRepo.GetEntitiesByIDs(ctx, ontologyID, dedupeUint32(memberIDs))
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query chain members failed")
		}
		byID := entityMap(memberEntities)
		frame.Chains = make([]*ontologyV1.ObjectPropertyChain, 0, len(parseable))
		for _, c := range parseable {
			pc := &ontologyV1.ObjectPropertyChain{Id: &c.AxiomID, MemberIds: c.MemberIDs}
			for _, m := range c.MemberIDs {
				if me, ok := byID[m]; ok {
					pc.Members = append(pc.Members, entityBrief(me))
				}
			}
			frame.Chains = append(frame.Chains, pc)
		}
	}

	return frame, nil
}

// briefsByIDs 按 id 序取实体简况（frame 各 section 共用；缺失实体跳过——不阻塞 frame 展示）
func (s *OntologyObjectPropertyService) briefsByIDs(ctx context.Context, ontologyID uint32, ids []uint32) ([]*ontologyV1.EntityBrief, error) {
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

// objectIDsOf 指定类型的主语侧公理宾语序列（保序 sort_order）
func objectIDsOf(subjectAxioms []*ent.OntAxiom, axiomType uint32) []uint32 {
	out := make([]uint32, 0)
	for _, ax := range subjectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != axiomType || ax.ObjectID == nil {
			continue
		}
		out = append(out, *ax.ObjectID)
	}
	return out
}

// orderPropertyCharacteristics frame 特征 IRI 按声明序（复用 repo 侧同一排序规则）
func orderPropertyCharacteristics(axioms []*ent.OntAxiom) []string {
	// 聚合排序逻辑与 data.orderCharacteristicIRIs 同源；此处小集合直接按声明序过滤输出
	out := make([]string, 0, len(axioms))
	present := make(map[string]struct{}, len(axioms))
	for _, ax := range axioms {
		if ax.PredicateIri != nil {
			present[*ax.PredicateIri] = struct{}{}
		}
	}
	for _, iri := range ontology.PropertyCharacteristicIRIs {
		if _, ok := present[iri]; ok {
			out = append(out, iri)
			delete(present, iri)
		}
	}
	// 白名单外（校验器已拦，防御兼容）按行序补尾
	for _, ax := range axioms {
		if ax.PredicateIri != nil {
			if _, ok := present[*ax.PredicateIri]; ok {
				out = append(out, *ax.PredicateIri)
				delete(present, *ax.PredicateIri)
			}
		}
	}
	return out
}

// ---- usage ----

func (s *OntologyObjectPropertyService) Usage(ctx context.Context, req *ontologyV1.GetObjectPropertyUsageRequest) (*ontologyV1.EntityUsage, error) {
	e, err := s.requireObjectPropertyEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	// 引用集 = ① object 侧、ObjectPropertyUsageAxiomTypes（30/34/35/36）命中；
	// ② 37 链成员包含（全量 37 行 value_text 判定）
	objectAxioms, err := s.axiomRepo.ListByObject(ctx, ontologyID, e.ID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query references failed")
	}
	allAxioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	chainRefs := ontology.ChainReferrers(ctx, allAxioms, e.ID, s.log)

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
		if !isUsageAxiomType(*ax.AxiomType, ontology.ObjectPropertyUsageAxiomTypes) {
			continue
		}
		key := refKey{entityID: *ax.SubjectID, axiomType: *ax.AxiomType}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		refKeys = append(refKeys, key)
	}
	for _, cr := range chainRefs {
		key := refKey{entityID: cr.SuperPropertyID, axiomType: ontology.AxiomSubPropertyChainOf}
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

func isUsageAxiomType(t uint32, types []uint32) bool {
	for _, v := range types {
		if v == t {
			return true
		}
	}
	return false
}

// ---- create / update / delete ----

func (s *OntologyObjectPropertyService) Create(ctx context.Context, req *ontologyV1.CreateObjectPropertyRequest) (*ontologyV1.CreateObjectPropertyResponse, error) {
	if req == nil || req.GetOntologyId() == 0 || req.GetLocalName() == "" {
		return nil, adminV1.ErrorBadRequest("ontology id and local name are required")
	}
	ont, err := s.requireOntology(ctx, req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	baseIRI := derefStr(ont.BaseIri)

	// 实体草稿：局部名校验 + IRI 前置查重
	draft, declaration, err := s.factory.CreateEntityDraft(ctx, req.GetOntologyId(), ontology.EntityTypeObjectProperty, baseIRI, req.GetLocalName())
	if err != nil {
		return nil, mapDomainError(err)
	}

	// 八步校验管线（02 号文档 §4.2）：引用目标校验 + 自引用三态 + 环 + 链 + 特征
	superIDs := dedupeUint32(req.GetSuperPropertyIds())
	domainIDs := dedupeUint32(req.GetDomainIds())
	rangeIDs := dedupeUint32(req.GetRangeIds())
	inverseIDs := dedupeUint32(req.GetInverseIds())
	disjointIDs := dedupeUint32(req.GetDisjointIds())
	var equivalentID uint32
	if req.EquivalentId != nil && *req.EquivalentId > 0 {
		equivalentID = *req.EquivalentId
	}
	if err = s.validatePropertyRefs(ctx, req.GetOntologyId(), superIDs, domainIDs, rangeIDs, inverseIDs, disjointIDs, equivalentID, req.GetChains()); err != nil {
		return nil, err
	}

	// 特征白名单
	if err = validateCharacteristics(req.GetCharacteristics()); err != nil {
		return nil, err
	}

	// 环检测（create 时新实体无任何入边，理论上恒通过；防御性保留同一管线）
	if err = s.assertNoCycleForProperty(ctx, req.GetOntologyId(), 0, superIDs); err != nil {
		return nil, err
	}

	// 组装变更集：新实体放 NewEntities[0]
	set := &ontology.EntityChangeSet{
		OntologyID:  req.GetOntologyId(),
		Operation:   ontology.OpCreateObjectProperty,
		NewEntities: []ontology.EntityDraft{*draft},
		Changes:     []ontology.AxiomChange{declaration},
	}
	appendPropertyRelationAdds(set, superIDs, domainIDs, rangeIDs, inverseIDs, disjointIDs, equivalentID, req.GetChains())
	appendCharacteristicsAdds(set, req.GetCharacteristics())
	appendAnnotationAdds(set, req.DisplayLabel, req.Comment, nil, req.Deprecated)

	result, err := s.applier.Apply(ctx, set, operatorUserID(ctx))
	if err != nil {
		return nil, mapDomainError(err)
	}

	resp := &ontologyV1.CreateObjectPropertyResponse{BaseVersion: &result.NewVersion}
	if len(result.NewEntityIDs) > 0 {
		resp.Id = &result.NewEntityIDs[0]
	}
	return resp, nil
}

func (s *OntologyObjectPropertyService) Update(ctx context.Context, req *ontologyV1.UpdateObjectPropertyRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireObjectPropertyEntity(ctx, req.GetId())
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
	rangeIDs := dedupeUint32(req.GetRangeIds())
	inverseIDs := dedupeUint32(req.GetInverseIds())
	disjointIDs := dedupeUint32(req.GetDisjointIds())

	// 自引用三态：逆属性/不相交/等价自引用拒绝（父属性自引用交由环检测捕获）
	for _, id := range inverseIDs {
		if id == req.GetId() {
			return nil, ontologyV1.ErrorPropertyInverseSelfInvalid("属性不能与自身互为逆属性：%d", id)
		}
	}
	for _, id := range disjointIDs {
		if id == req.GetId() {
			return nil, ontologyV1.ErrorPropertySelfReferenceInvalid("属性不能与自身不相交：%d", id)
		}
	}
	var equivalentTarget []uint32
	equivalentSet := req.EquivalentId != nil
	if equivalentSet && *req.EquivalentId > 0 {
		if *req.EquivalentId == req.GetId() {
			return nil, ontologyV1.ErrorPropertySelfReferenceInvalid("属性不能与自身等价：%d", *req.EquivalentId)
		}
		equivalentTarget = []uint32{*req.EquivalentId}
	}

	if err = s.validatePropertyRefs(ctx, ontologyID, superIDs, domainIDs, rangeIDs, inverseIDs, disjointIDs, uint32(0), req.GetChains()); err != nil {
		return nil, err
	}
	if err = validateCharacteristics(req.GetCharacteristics()); err != nil {
		return nil, err
	}
	// 等价目标单独校验（equivalentID=0 表示清空，不参与引用校验）
	if len(equivalentTarget) > 0 {
		if err = s.factory.ValidateRefEntities(ctx, ontologyID, equivalentTarget, ontology.EntityTypeObjectProperty, ontology.ReasonPropertyTargetInvalid); err != nil {
			return nil, mapDomainError(err)
		}
	}

	// 环检测（全量新父集；含"把子孙设为父属性"与自环）
	if err = s.assertNoCycleForProperty(ctx, ontologyID, req.GetId(), superIDs); err != nil {
		return nil, err
	}

	// frame 全量提交语义 diff（逐 section）
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	subjectAxioms := filterBySubject(axioms, req.GetId())
	annotationAxioms := filterByType(subjectAxioms, ontology.AxiomAnnotationAssertion)

	changes := ontology.DiffSubjectSideRelations(req.GetId(), ontology.AxiomSubObjectPropertyOf,
		filterByType(subjectAxioms, ontology.AxiomSubObjectPropertyOf), superIDs)
	changes = append(changes, ontology.DiffSubjectSideRelations(req.GetId(), ontology.AxiomObjectPropertyDomain,
		filterByType(subjectAxioms, ontology.AxiomObjectPropertyDomain), domainIDs)...)
	changes = append(changes, ontology.DiffSubjectSideRelations(req.GetId(), ontology.AxiomObjectPropertyRange,
		filterByType(subjectAxioms, ontology.AxiomObjectPropertyRange), rangeIDs)...)
	changes = append(changes, ontology.DiffCharacteristics(req.GetId(),
		filterByType(subjectAxioms, ontology.AxiomObjectPropertyCharacteristic), req.GetCharacteristics())...)
	changes = append(changes, ontology.DiffSymmetricPairs(req.GetId(), ontology.AxiomInverseOf,
		buildSymmetricPairs(req.GetId(), axioms, ontology.AxiomInverseOf), inverseIDs)...)
	changes = append(changes, ontology.DiffSymmetricPairs(req.GetId(), ontology.AxiomDisjointObjectProperties,
		buildSymmetricPairs(req.GetId(), axioms, ontology.AxiomDisjointObjectProperties), disjointIDs)...)
	if equivalentSet {
		changes = append(changes, ontology.DiffSymmetricPairs(req.GetId(), ontology.AxiomEquivalentObjectProperties,
			buildSymmetricPairs(req.GetId(), axioms, ontology.AxiomEquivalentObjectProperties), equivalentTarget)...)
	}
	changes = append(changes, ontology.DiffChains(req.GetId(),
		ontology.PropertyChainsOf(ctx, subjectAxioms, req.GetId(), s.log),
		propertyChainTargets(req.GetChains()))...)
	changes = append(changes, ontology.DiffAnnotations(req.GetId(), annotationAxioms, map[string]*string{
		ontology.PredicateRDFSLabel:   req.DisplayLabel, // nil = 不变；空串 = 清空
		ontology.PredicateRDFSComment: req.Comment,
	})...)
	if req.Deprecated != nil {
		changes = append(changes, ontology.DiffDeprecated(req.GetId(), annotationAxioms, *req.Deprecated)...)
	}

	set := &ontology.EntityChangeSet{
		OntologyID:  ontologyID,
		BaseVersion: req.BaseVersion,
		Operation:   ontology.OpUpdateObjectProperty,
		Changes:     changes,
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OntologyObjectPropertyService) Delete(ctx context.Context, req *ontologyV1.DeleteObjectPropertyRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireObjectPropertyEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	force := req.Force != nil && *req.Force

	// 删除保护：object 侧命中（30/34/35/36）+ 链成员包含（37）计数
	refsCount, err := s.axiomRepo.CountReferencesByTypes(ctx, ontologyID, req.GetId(), ontology.ObjectPropertyUsageAxiomTypes)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("count references failed")
	}
	allAxioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	chainRefs := ontology.ChainReferrers(ctx, allAxioms, req.GetId(), s.log)
	if refsCount+len(chainRefs) > 0 && !force {
		return nil, ontologyV1.ErrorEntityInUse("实体被 %d 处引用，无法删除", refsCount+len(chainRefs))
	}

	// 收集删除变更：主语侧全部公理（声明/子属性/域值域/特征/对称/链/注解）
	subjectAxioms, err := s.axiomRepo.ListBySubject(ctx, ontologyID, req.GetId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	deleted := make(map[uint32]struct{}, len(subjectAxioms)+len(chainRefs))
	changes := make([]ontology.AxiomChange, 0, len(subjectAxioms)+len(chainRefs))
	for _, ax := range subjectAxioms {
		if _, ok := deleted[ax.ID]; ok {
			continue
		}
		deleted[ax.ID] = struct{}{}
		changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: ax.ID, SubjectID: derefU32(ax.SubjectID)})
	}
	// 宾语侧（34/35/36 镜像对、30 子属性边——子属性自动变根属性）
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
	// 含我的链行（超属性是他人）
	for _, cr := range chainRefs {
		if _, ok := deleted[cr.AxiomID]; ok {
			continue
		}
		deleted[cr.AxiomID] = struct{}{}
		changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: cr.AxiomID, SubjectID: cr.SuperPropertyID})
	}

	// force 级联：实体行一并软删；子属性自动变根属性
	set := &ontology.EntityChangeSet{
		OntologyID:      ontologyID,
		BaseVersion:     req.BaseVersion,
		Operation:       ontology.OpDeleteObjectProperty,
		Changes:         changes,
		DeleteEntityIDs: []uint32{req.GetId()},
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- 校验与组装辅助 ----

// validatePropertyRefs 引用目标类型与存在性校验（八步管线第 4/6 步）：
// 属性侧引用（父/逆/不相交/等价/链成员）→ PROPERTY_TARGET_INVALID；域/值域 → 专属码。
// 自引用校验不在本方法（调用方按三态分流；create 不做——新实体尚无 id）。
func (s *OntologyObjectPropertyService) validatePropertyRefs(
	ctx context.Context, ontologyID uint32,
	superIDs, domainIDs, rangeIDs, inverseIDs, disjointIDs []uint32,
	equivalentID uint32,
	chains []*ontologyV1.ObjectPropertyChain,
) error {
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, superIDs, ontology.EntityTypeObjectProperty, ontology.ReasonPropertyTargetInvalid); err != nil {
		return mapDomainError(err)
	}
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, inverseIDs, ontology.EntityTypeObjectProperty, ontology.ReasonPropertyTargetInvalid); err != nil {
		return mapDomainError(err)
	}
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, disjointIDs, ontology.EntityTypeObjectProperty, ontology.ReasonPropertyTargetInvalid); err != nil {
		return mapDomainError(err)
	}
	if equivalentID > 0 {
		if err := s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{equivalentID}, ontology.EntityTypeObjectProperty, ontology.ReasonPropertyTargetInvalid); err != nil {
			return mapDomainError(err)
		}
	}

	// 域/值域指向非类不复用 ENTITY_TYPE_MISMATCH：专属文案码（02 号文档 §3.2）
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, domainIDs, ontology.EntityTypeClass, ontology.ReasonPropertyDomainInvalid); err != nil {
		return mapDomainError(err)
	}
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, rangeIDs, ontology.EntityTypeClass, ontology.ReasonPropertyRangeInvalid); err != nil {
		return mapDomainError(err)
	}

	// 链成员：逐个命中已声明对象属性（合并去重校验）
	chainMembers := make([]uint32, 0)
	for _, c := range chains {
		chainMembers = append(chainMembers, c.GetMemberIds()...)
	}
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, dedupeUint32(chainMembers), ontology.EntityTypeObjectProperty, ontology.ReasonPropertyTargetInvalid); err != nil {
		return mapDomainError(err)
	}
	// 链长度 ≥ 2（成员保序不去重，允许 P∘P 与含超属性自身）
	for _, c := range chains {
		if len(c.GetMemberIds()) > 0 && len(c.GetMemberIds()) < 2 {
			return ontologyV1.ErrorPropertyChainInvalid("属性链不合法：%v（至少两个属性成员）", c.GetMemberIds())
		}
	}
	return nil
}

// validateCharacteristics 特征 IRI 白名单校验（八步管线第 7 步）
func validateCharacteristics(iris []string) error {
	for _, iri := range iris {
		if iri == "" {
			continue
		}
		if !propertyCharacteristicKnown(iri) {
			return ontologyV1.ErrorPropertyCharacteristicInvalid("未知的对象属性特征：%s", iri)
		}
	}
	return nil
}

func propertyCharacteristicKnown(iri string) bool {
	for _, v := range ontology.PropertyCharacteristicIRIs {
		if v == iri {
			return true
		}
	}
	return false
}

// propertyChainTargets proto 链 → diff 目标投影（id 透传：已存在链的公理行 id）
func propertyChainTargets(chains []*ontologyV1.ObjectPropertyChain) []ontology.PropertyChain {
	out := make([]ontology.PropertyChain, 0, len(chains))
	for _, c := range chains {
		out = append(out, ontology.PropertyChain{AxiomID: c.GetId(), MemberIDs: c.GetMemberIds()})
	}
	return out
}

// assertNoCycleForProperty 环检测（写路径前置，读全量 30 型边）
func (s *OntologyObjectPropertyService) assertNoCycleForProperty(ctx context.Context, ontologyID, targetID uint32, superIDs []uint32) error {
	axioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
	if err != nil {
		return adminV1.ErrorInternalServerError("query axioms failed")
	}
	edges := ontology.CollectHierarchyEdges(axioms, ontology.AxiomSubObjectPropertyOf)
	var names map[uint32]string
	if entities, listErr := s.entityRepo.ListEntitiesByType(ctx, ontologyID, ontology.EntityTypeObjectProperty); listErr == nil {
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

// appendPropertyRelationAdds 创建时的关系 ADD 变更（父属性/域/值域/逆/不相交/等价/链；
// 逆/不相交/等价成对存储；链 value_text 保序编码）
func appendPropertyRelationAdds(
	set *ontology.EntityChangeSet,
	superIDs, domainIDs, rangeIDs, inverseIDs, disjointIDs []uint32,
	equivalentID uint32,
	chains []*ontologyV1.ObjectPropertyChain,
) {
	const selfRef = 1 // 主语 = 新实体（NewEntities[0]）
	for i, pid := range superIDs {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomSubObjectPropertyOf,
			SubjectNewEntityRef: selfRef, ObjectID: pid, SortOrder: int32(i),
		})
	}
	for i, did := range domainIDs {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomObjectPropertyDomain,
			SubjectNewEntityRef: selfRef, ObjectID: did, SortOrder: int32(i),
		})
	}
	for i, rid := range rangeIDs {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomObjectPropertyRange,
			SubjectNewEntityRef: selfRef, ObjectID: rid, SortOrder: int32(i),
		})
	}
	for i, iid := range inverseIDs {
		set.Changes = append(set.Changes,
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomInverseOf,
				SubjectNewEntityRef: selfRef, ObjectID: iid, SortOrder: int32(i),
			},
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomInverseOf,
				SubjectID: iid, ObjectNewEntityRef: selfRef, SortOrder: int32(i),
			},
		)
	}
	for i, did := range disjointIDs {
		set.Changes = append(set.Changes,
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDisjointObjectProperties,
				SubjectNewEntityRef: selfRef, ObjectID: did, SortOrder: int32(i),
			},
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDisjointObjectProperties,
				SubjectID: did, ObjectNewEntityRef: selfRef, SortOrder: int32(i),
			},
		)
	}
	if equivalentID > 0 {
		set.Changes = append(set.Changes,
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomEquivalentObjectProperties,
				SubjectNewEntityRef: selfRef, ObjectID: equivalentID, SortOrder: 0,
			},
			ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomEquivalentObjectProperties,
				SubjectID: equivalentID, ObjectNewEntityRef: selfRef, SortOrder: 0,
			},
		)
	}
	for i, c := range chains {
		if len(c.GetMemberIds()) == 0 {
			continue
		}
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op:                  ontology.AxiomOpAdd,
			AxiomType:           ontology.AxiomSubPropertyChainOf,
			SubjectNewEntityRef: selfRef,
			ValueText:           ontology.EncodeChainMemberIDs(c.GetMemberIds()),
			SortOrder:           int32(i),
		})
	}
}

// appendCharacteristicsAdds 创建时的特征 ADD 变更（每特征一行，object_id/value_text 空）
func appendCharacteristicsAdds(set *ontology.EntityChangeSet, iris []string) {
	for _, iri := range iris {
		if iri == "" {
			continue
		}
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op:                  ontology.AxiomOpAdd,
			AxiomType:           ontology.AxiomObjectPropertyCharacteristic,
			SubjectNewEntityRef: 1,
			PredicateIRI:        iri,
		})
	}
}
