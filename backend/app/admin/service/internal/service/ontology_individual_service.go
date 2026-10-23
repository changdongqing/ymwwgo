package service

import (
	"context"
	"strings"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"
	"google.golang.org/protobuf/types/known/emptypb"

	adminV1 "go-wind-admin/api/gen/go/admin/service/v1"
	ontologyV1 "go-wind-admin/api/gen/go/ontology/service/v1"

	"go-wind-admin/app/admin/service/internal/data"
	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// OntologyIndividualService 个体管理服务（BFF：八端点编排，调域内核 + repo）。
//
// create/update 是自定义 RPC（取号 + 公理事务管线），收扁平请求体——不是通用 CRUD（铁律 3 的
// 正确适用，勿包 { data }）。创建/改码走 ApplyInTx 事务模板（取号与公理写同事务，冲突回滚号不
// 损耗，06 号文档 §2.3）；类型区走专用全量替换 diff（顺序承载主类目语义，§2.4）。
type OntologyIndividualService struct {
	adminV1.OntologyIndividualServiceHTTPServer

	log          *bLogger.Helper
	ontologyRepo *data.OntologyRepo
	entityRepo   *data.OntEntityRepo
	axiomRepo    *data.OntAxiomRepo
	serialRepo   *data.OntSerialRepo
	factory      *ontology.EntityFactory
	applier      *ontology.ChangeApplier
}

func NewOntologyIndividualService(
	ctx *bootstrap.Context,
	ontologyRepo *data.OntologyRepo,
	entityRepo *data.OntEntityRepo,
	axiomRepo *data.OntAxiomRepo,
	serialRepo *data.OntSerialRepo,
) *OntologyIndividualService {
	log := ctx.NewLoggerHelper("ontology-individual/service/admin-service")
	return &OntologyIndividualService{
		log:          log,
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		axiomRepo:    axiomRepo,
		serialRepo:   serialRepo,
		factory:      ontology.NewEntityFactory(entityRepo),
		applier:      ontology.NewChangeApplier(axiomRepo, log),
	}
}

// codeLocalName 编码数据属性实体的本体级约定局部名（06 号文档 §2.2：IRI = base_iri + "code"，
// 首次赋码时同事务懒创建；多套编码各建独立实体，唯一性按"属性实体 + 值"圈定作用域）
const codeLocalName = "code"

// 编码来源（req.code_source）
const (
	CodeSourceAuto   = "AUTO"
	CodeSourceManual = "MANUAL"
)

// requireOntology 本体存在且启用检查（实体级接口统一闸门）
func (s *OntologyIndividualService) requireOntology(ctx context.Context, ontologyID uint32) (*ent.OntOntology, error) {
	if ontologyID == 0 {
		return nil, adminV1.ErrorBadRequest("ontology id is required")
	}
	ont, err := s.ontologyRepo.GetEntityByID(ctx, ontologyID)
	return checkOntologyEnabled(ont, err)
}

// requireIndividualEntity 实体存在且为个体检查
func (s *OntologyIndividualService) requireIndividualEntity(ctx context.Context, id uint32) (*ent.OntEntity, error) {
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
	if e.EntityType == nil || *e.EntityType != ontology.EntityTypeIndividual {
		return nil, ontologyV1.ErrorEntityNotFound("实体 %d 不是个体", id)
	}
	return e, nil
}

// resolveCodePropertyID 解析编码数据属性实体 id（本体级约定 localName=code；不存在返回 0）
func (s *OntologyIndividualService) resolveCodePropertyID(ctx context.Context, ontologyID uint32) (uint32, error) {
	entities, err := s.entityRepo.ListEntitiesByType(ctx, ontologyID, ontology.EntityTypeDataProperty)
	if err != nil {
		return 0, err
	}
	for _, e := range entities {
		if e.LocalName != nil && *e.LocalName == codeLocalName {
			return e.ID, nil
		}
	}
	return 0, nil
}

// ---- 列表（两段式查询：先行按索引解析过滤 id 集 → 实体分页 IN → 聚合列批量填充） ----

func (s *OntologyIndividualService) List(ctx context.Context, req *ontologyV1.ListIndividualsRequest) (*ontologyV1.ListIndividualsResponse, error) {
	ontologyID := req.GetOntologyId()
	if _, err := s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	// 第一段：编码过滤（按懒创建规则解析编码属性；属性不存在返回空页）
	var ids []uint32
	idsConstrained := false
	if code := strings.TrimSpace(req.GetCode()); code != "" {
		idsConstrained = true
		codePropID, err := s.resolveCodePropertyID(ctx, ontologyID)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("resolve code property failed")
		}
		if codePropID == 0 {
			return &ontologyV1.ListIndividualsResponse{Items: []*ontologyV1.Individual{}}, nil
		}
		ids, err = s.axiomRepo.ListSubjectIDsByCodeValue(ctx, ontologyID, codePropID, code,
			strings.EqualFold(req.GetCodeQueryType(), "PREFIX"))
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("filter individuals by code failed")
		}
	}

	// 第一段：类目过滤（直接实例 / 含子类实例）
	if req.GetClassId() > 0 {
		classIDs := []uint32{req.GetClassId()}
		if req.GetIncludeDescendants() {
			allAxioms, err := s.axiomRepo.ListByOntology(ctx, ontologyID)
			if err != nil {
				return nil, adminV1.ErrorInternalServerError("query axioms failed")
			}
			edges := ontology.CollectHierarchyEdges(allAxioms, ontology.AxiomSubClassOf)
			classIDs = append(classIDs, ontology.SubtreeIDs(req.GetClassId(), edges)...)
		}
		instanceIDs, err := s.axiomRepo.ListSubjectIDsByClassInstances(ctx, ontologyID, classIDs)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("filter individuals by class failed")
		}
		// intersect 用本分支前的约束态：仅 classId 过滤（此前无 keyword/code 约束）时
		// 直接采纳实例 id 集——先置位再相交会把 nil 前集相交成恒空（06 期遗留 bug）
		ids = intersectUint32(ids, idsConstrained, instanceIDs)
		idsConstrained = true
	}

	if idsConstrained && len(ids) == 0 {
		return &ontologyV1.ListIndividualsResponse{Items: []*ontologyV1.Individual{}}, nil
	}

	deprecated := req.Deprecated
	entities, total, err := s.entityRepo.ListIndividuals(ctx, data.IndividualListFilter{
		OntologyID: ontologyID,
		Keyword:    strings.TrimSpace(req.GetKeyword()),
		Deprecated: deprecated,
		IDs:        ids,
		Page:       req.GetPage(),
		PageSize:   req.GetPageSize(),
		OrderBy:    req.GetOrderBy(),
	})
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("list individuals failed")
	}

	// 聚合列批量填充（主类目 brief / 类型数 / 编码 / 注释）
	pageIDs := make([]uint32, 0, len(entities))
	for _, e := range entities {
		pageIDs = append(pageIDs, e.ID)
	}
	codePropID, err := s.resolveCodePropertyID(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("resolve code property failed")
	}
	aggregates, err := s.entityRepo.AggregateIndividualInfo(ctx, ontologyID, pageIDs, codePropID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("aggregate individual info failed")
	}
	mainClassIDs := make([]uint32, 0, len(entities))
	for _, id := range pageIDs {
		if agg := aggregates[id]; agg != nil && agg.MainClassID > 0 {
			mainClassIDs = append(mainClassIDs, agg.MainClassID)
		}
	}
	mainClassBriefs := make(map[uint32]*ontologyV1.EntityBrief, len(mainClassIDs))
	if len(mainClassIDs) > 0 {
		classEntities, err := s.entityRepo.GetEntitiesByIDs(ctx, ontologyID, mainClassIDs)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query main classes failed")
		}
		for _, ce := range classEntities {
			mainClassBriefs[ce.ID] = entityBrief(ce)
		}
	}

	items := make([]*ontologyV1.Individual, 0, len(entities))
	for _, e := range entities {
		item := &ontologyV1.Individual{
			Id:           &e.ID,
			OntologyId:   e.OntologyID,
			Iri:          e.Iri,
			LocalName:    e.LocalName,
			DisplayLabel: e.DisplayLabel,
			Deprecated:   e.Deprecated,
		}
		if agg := aggregates[e.ID]; agg != nil {
			item.Comment = agg.Comment
			if agg.MainClassID > 0 {
				item.MainClass = mainClassBriefs[agg.MainClassID]
			}
			item.TypeCount = &agg.TypeCount
			if codePropID > 0 {
				item.Code = agg.Code
			}
		}
		items = append(items, item)
	}
	return &ontologyV1.ListIndividualsResponse{Items: items, Total: uint64(total)}, nil
}

// ---- frame ----

func (s *OntologyIndividualService) Frame(ctx context.Context, req *ontologyV1.GetIndividualFrameRequest) (*ontologyV1.IndividualFrame, error) {
	e, err := s.requireIndividualEntity(ctx, req.GetId())
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

	frame := &ontologyV1.IndividualFrame{
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

	// 类型区（60 行 (sort_order,id) 升序，首位 = 主类目）
	typeRows := filterByType(subjectAxioms, ontology.AxiomTypeAssertion)
	ontology.StableSortAxioms(typeRows)
	typeIDs := make([]uint32, 0, len(typeRows))
	for _, ax := range typeRows {
		if ax.ObjectID != nil && *ax.ObjectID != 0 {
			typeIDs = append(typeIDs, *ax.ObjectID)
		}
	}
	if len(typeIDs) > 0 {
		briefs, err := s.briefsByIDs(ctx, ontologyID, typeIDs)
		if err != nil {
			return nil, err
		}
		frame.Types = briefs
	}

	// 编码区（62 行、编码属性谓词；约定 localName=code）
	codePropID, err := s.resolveCodePropertyID(ctx, ontologyID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("resolve code property failed")
	}
	if codePropID > 0 {
		for _, ax := range filterByType(subjectAxioms, ontology.AxiomDataPropertyAssertion) {
			if ax.ObjectID != nil && *ax.ObjectID == codePropID && ax.ValueText != nil {
				frame.Code = &ontologyV1.IndividualCode{PropertyId: &codePropID, Value: ax.ValueText}
				break
			}
		}
	}

	// 对象属性断言区（61 行）
	objAssertionRows := filterByType(subjectAxioms, ontology.AxiomObjectPropertyAssertion)
	if len(objAssertionRows) > 0 {
		targetIDs := make([]uint32, 0, len(objAssertionRows))
		for _, ax := range objAssertionRows {
			if ax.ObjectID != nil && *ax.ObjectID != 0 {
				targetIDs = append(targetIDs, *ax.ObjectID)
			}
		}
		targetBriefs, err := s.briefsByIDs(ctx, ontologyID, targetIDs)
		if err != nil {
			return nil, err
		}
		byID := entityBriefMap(targetBriefs)
		predicateBriefs, err := s.predicateBriefsByIRIs(ctx, objAssertionRows)
		if err != nil {
			return nil, err
		}
		frame.ObjectAssertions = make([]*ontologyV1.ObjectAssertion, 0, len(objAssertionRows))
		for _, ax := range objAssertionRows {
			row := &ontologyV1.ObjectAssertion{PredicateIri: ax.PredicateIri}
			if b, ok := predicateBriefs[derefStr(ax.PredicateIri)]; ok {
				row.Predicate = b
			}
			if ax.ObjectID != nil {
				row.Target = byID[*ax.ObjectID]
			}
			frame.ObjectAssertions = append(frame.ObjectAssertions, row)
		}
	}

	// 数据属性断言区（62 行，编码行除外——编码在 code 字段单独回显）
	dataAssertionRows := make([]*ent.OntAxiom, 0, 4)
	for _, ax := range filterByType(subjectAxioms, ontology.AxiomDataPropertyAssertion) {
		if codePropID > 0 && ax.ObjectID != nil && *ax.ObjectID == codePropID {
			continue
		}
		dataAssertionRows = append(dataAssertionRows, ax)
	}
	if len(dataAssertionRows) > 0 {
		propertyIDs := make([]uint32, 0, len(dataAssertionRows))
		for _, ax := range dataAssertionRows {
			if ax.ObjectID != nil && *ax.ObjectID != 0 {
				propertyIDs = append(propertyIDs, *ax.ObjectID)
			}
		}
		propertyBriefs, err := s.briefsByIDs(ctx, ontologyID, propertyIDs)
		if err != nil {
			return nil, err
		}
		byID := entityBriefMap(propertyBriefs)
		frame.DataAssertions = make([]*ontologyV1.DataAssertion, 0, len(dataAssertionRows))
		for _, ax := range dataAssertionRows {
			row := &ontologyV1.DataAssertion{Value: ax.ValueText, Lang: ax.Lang}
			if ax.ObjectID != nil {
				row.PropertyId = ax.ObjectID
				row.Property = byID[*ax.ObjectID]
			}
			frame.DataAssertions = append(frame.DataAssertions, row)
		}
	}

	// 自定义注解区（内建字段之外，保序；04 期共享）
	annotations, err := frameAnnotationsOf(ctx, s.entityRepo, ontologyID, subjectAxioms)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query annotations failed")
	}
	frame.Annotations = annotations

	return frame, nil
}

// briefsByIDs 按 id 序取实体简况（frame 各 section 共用；缺失实体跳过——不阻塞 frame 展示）
func (s *OntologyIndividualService) briefsByIDs(ctx context.Context, ontologyID uint32, ids []uint32) ([]*ontologyV1.EntityBrief, error) {
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

// predicateBriefsByIRIs 61 断言谓词 IRI → 对象属性实体简况（同本体命中优先；缺失跳过）
func (s *OntologyIndividualService) predicateBriefsByIRIs(ctx context.Context, rows []*ent.OntAxiom) (map[string]*ontologyV1.EntityBrief, error) {
	out := make(map[string]*ontologyV1.EntityBrief, len(rows))
	for _, ax := range rows {
		iri := derefStr(ax.PredicateIri)
		if iri == "" {
			continue
		}
		if _, ok := out[iri]; ok {
			continue
		}
		entities, err := s.entityRepo.GetEntityByIRI(ctx, iri)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query predicate entities failed")
		}
		for _, e := range entities { // 同本体命中优先
			if derefU32(e.OntologyID) == derefU32(ax.OntologyID) {
				out[iri] = entityBrief(e)
				break
			}
		}
	}
	return out, nil
}

// entityBriefMap 简况列表 → id 索引
func entityBriefMap(briefs []*ontologyV1.EntityBrief) map[uint32]*ontologyV1.EntityBrief {
	out := make(map[uint32]*ontologyV1.EntityBrief, len(briefs))
	for _, b := range briefs {
		if b != nil && b.Id != nil {
			out[*b.Id] = b
		}
	}
	return out
}

// ---- code-preview / count-by-class ----

// CodePreview 预计编码（预览不加锁，并发下实际号可能 +N；文案必须写"预计"——§5）
func (s *OntologyIndividualService) CodePreview(ctx context.Context, req *ontologyV1.GetIndividualCodePreviewRequest) (*ontologyV1.IndividualCodePreview, error) {
	ontologyID := req.GetOntologyId()
	if _, err := s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}
	mainClassID := req.GetMainClassId()
	if mainClassID == 0 {
		return nil, ontologyV1.ErrorIndividualMainClassRequired("个体必须指定主类目")
	}
	if err := s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{mainClassID}, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
		return nil, mapDomainError(err)
	}

	subjectAxioms, err := s.axiomRepo.ListBySubject(ctx, ontologyID, mainClassID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	prefix := ontology.PickAnnotationPreferred(
		filterByType(subjectAxioms, ontology.AxiomAnnotationAssertion), ontology.PredicateSKOSNotation)
	if prefix == nil || *prefix == "" {
		return nil, ontologyV1.ErrorIndividualCodeSourceInvalid("主类目无编码，无法自动赋码，请手输编码或改选手动模式")
	}
	if err = ontology.ValidateCodeCharset(*prefix); err != nil {
		return nil, mapDomainError(err)
	}

	nextNo, err := s.serialRepo.PeekNo(ctx, ontologyID, ontology.SerialPurposeCode, *prefix)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("peek serial failed")
	}
	preview := ontology.FormatCode(*prefix, nextNo)
	return &ontologyV1.IndividualCodePreview{Prefix: prefix, PreviewCode: &preview}, nil
}

// CountByClass 类树实例数徽标（60 行 group by object_id → 直接实例计数）
func (s *OntologyIndividualService) CountByClass(ctx context.Context, req *ontologyV1.GetIndividualCountByClassRequest) (*ontologyV1.IndividualCountByClassResponse, error) {
	if _, err := s.requireOntology(ctx, req.GetOntologyId()); err != nil {
		return nil, err
	}
	counts, err := s.axiomRepo.CountDirectInstancesByClass(ctx, req.GetOntologyId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("count instances failed")
	}
	items := make([]*ontologyV1.IndividualCountByClassResponse_ClassInstanceCount, 0, len(counts))
	for classID, count := range counts {
		c := uint32(count)
		items = append(items, &ontologyV1.IndividualCountByClassResponse_ClassInstanceCount{
			ClassId: &classID, DirectCount: &c,
		})
	}
	return &ontologyV1.IndividualCountByClassResponse{Items: items}, nil
}

// ---- usage ----

// Usage 61 引用清单（"谁把我当宾语"；主语侧断言属于自身 frame，不计入被引用——§4.4）
func (s *OntologyIndividualService) Usage(ctx context.Context, req *ontologyV1.GetIndividualUsageRequest) (*ontologyV1.EntityUsage, error) {
	e, err := s.requireIndividualEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	objectAxioms, err := s.axiomRepo.ListByObject(ctx, ontologyID, e.ID)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query references failed")
	}

	seen := make(map[uint32]struct{})
	refIDs := make([]uint32, 0)
	for _, ax := range objectAxioms {
		if ax.SubjectID == nil || ax.AxiomType == nil {
			continue
		}
		if *ax.AxiomType != ontology.AxiomObjectPropertyAssertion || *ax.SubjectID == e.ID {
			continue
		}
		if _, ok := seen[*ax.SubjectID]; ok {
			continue
		}
		seen[*ax.SubjectID] = struct{}{}
		refIDs = append(refIDs, *ax.SubjectID)
	}

	usage := &ontologyV1.EntityUsage{EntityId: &e.ID}
	if len(refIDs) > 0 {
		entities, err := s.entityRepo.GetEntitiesByIDs(ctx, ontologyID, refIDs)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query referencing entities failed")
		}
		byID := entityMap(entities)
		usage.Refs = make([]*ontologyV1.EntityUsageRef, 0, len(refIDs))
		for _, id := range refIDs {
			axiomType := ontology.AxiomObjectPropertyAssertion
			ref := &ontologyV1.EntityUsageRef{EntityId: &id, AxiomType: &axiomType}
			if re, ok := byID[id]; ok {
				ref.LocalName = re.LocalName
				ref.DisplayLabel = re.DisplayLabel
			}
			usage.Refs = append(usage.Refs, ref)
		}
	}
	return usage, nil
}

// ---- create（§4.2 管线：取号与组装在 ApplyInTx 回调内整体完成） ----

func (s *OntologyIndividualService) Create(ctx context.Context, req *ontologyV1.CreateIndividualRequest) (*ontologyV1.CreateIndividualResponse, error) {
	if req == nil || req.GetOntologyId() == 0 {
		return nil, adminV1.ErrorBadRequest("ontology id is required")
	}
	ont, err := s.requireOntology(ctx, req.GetOntologyId())
	if err != nil {
		return nil, err
	}
	ontologyID := req.GetOntologyId()
	baseIRI := derefStr(ont.BaseIri)

	// 主类目校验（必填，赋码依据——§2.4）
	mainClassID := req.GetMainClassId()
	if mainClassID == 0 {
		return nil, ontologyV1.ErrorIndividualMainClassRequired("个体必须指定主类目")
	}
	if err = s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{mainClassID}, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
		return nil, mapDomainError(err)
	}

	// 其余类型（去重并剔除与主类目重复项，防 60 行双落）
	typeIDs := dedupeUint32(req.GetTypeIds())
	typeIDs = removeUint32(typeIDs, mainClassID)
	if err = s.factory.ValidateRefEntities(ctx, ontologyID, typeIDs, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
		return nil, mapDomainError(err)
	}

	// 高级模式：编码属性显式指定校验
	explicitCodePropID := uint32(0)
	if req.CodePropertyId != nil && *req.CodePropertyId > 0 {
		if err = s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{*req.CodePropertyId}, ontology.EntityTypeDataProperty, ontology.ReasonDataPropertyTargetInvalid); err != nil {
			return nil, mapDomainError(err)
		}
		explicitCodePropID = *req.CodePropertyId
	}

	// 编码来源校验（AUTO/MANUAL 之外拒绝；MANUAL 空码/非法字符快速失败，事务内复核唯一性）
	codeSource := strings.ToUpper(strings.TrimSpace(req.GetCodeSource()))
	if codeSource == "" {
		codeSource = CodeSourceAuto
	}
	if codeSource != CodeSourceAuto && codeSource != CodeSourceManual {
		return nil, adminV1.ErrorBadRequest("invalid code source: %s", req.GetCodeSource())
	}
	if codeSource == CodeSourceManual {
		code := strings.TrimSpace(req.GetCode())
		if code == "" {
			return nil, ontologyV1.ErrorIndividualCodeRequired("个体编码不能为空")
		}
		if err = ontology.ValidateCodeCharset(code); err != nil {
			return nil, mapDomainError(err)
		}
	}

	// 自定义注解（04 双轨）
	annotationInputs, err := normalizeAnnotationInputs(req.GetAnnotations())
	if err != nil {
		return nil, err
	}
	if err = validateAnnotationPropertyRefs(ctx, s.factory, ontologyID, annotationInputs); err != nil {
		return nil, err
	}

	// 手输 localName（高级模式）：局部名校验 + IRI 前置查重（自动生成为 Tx 内取号后组装）
	var manualDraft *ontology.EntityDraft
	var manualDeclaration ontology.AxiomChange
	if req.LocalName != nil && strings.TrimSpace(*req.LocalName) != "" {
		manualDraft, manualDeclaration, err = s.factory.CreateEntityDraft(ctx, ontologyID, ontology.EntityTypeIndividual, baseIRI, strings.TrimSpace(*req.LocalName))
		if err != nil {
			return nil, mapDomainError(err)
		}
	}

	var finalCode string
	result, err := s.applier.ApplyInTx(ctx, operatorUserID(ctx), ontologyID, nil, ontology.OpCreateIndividual,
		func(tx ontology.AxiomTx) (*ontology.EntityChangeSet, error) {
			set, code, err := s.assembleCreateChangeset(ctx, tx, ontologyID, baseIRI, req, mainClassID, typeIDs,
				explicitCodePropID, codeSource, annotationInputs, manualDraft, manualDeclaration)
			if err != nil {
				return nil, err
			}
			finalCode = code
			return set, nil
		})
	if err != nil {
		return nil, mapDomainError(err)
	}

	resp := &ontologyV1.CreateIndividualResponse{BaseVersion: &result.NewVersion, Code: &finalCode}
	if len(result.NewEntityIDs) > 0 {
		resp.Id = &result.NewEntityIDs[0]
	}
	return resp, nil
}

// assembleCreateChangeset 创建变更集组装（ApplyInTx 回调；全部读取都在传入事务上——
// 取号/前缀快照与公理写同事务，冲突整体回滚号不损耗）
func (s *OntologyIndividualService) assembleCreateChangeset(
	ctx context.Context, tx ontology.AxiomTx, ontologyID uint32, baseIRI string,
	req *ontologyV1.CreateIndividualRequest, mainClassID uint32, typeIDs []uint32,
	explicitCodePropID uint32, codeSource string, annotationInputs []ontology.CustomAnnotationInput,
	manualDraft *ontology.EntityDraft, manualDeclaration ontology.AxiomChange,
) (*ontology.EntityChangeSet, string, error) {
	allocator, ok := tx.(ontology.SerialTx)
	if !ok {
		return nil, "", adminV1.ErrorInternalServerError("serial allocator unavailable")
	}

	// a. 编码属性解析（懒创建，§2.2）：约定 localName=code；不存在且本次需落编码 → 同一 ChangeSet
	codePropID := explicitCodePropID
	var lazyDraft *ontology.EntityDraft
	var lazyDeclaration ontology.AxiomChange
	if codePropID == 0 {
		entities, err := tx.ListEntities(ctx, ontologyID)
		if err != nil {
			return nil, "", err
		}
		for _, e := range entities {
			if e.EntityType != nil && *e.EntityType == ontology.EntityTypeDataProperty &&
				e.LocalName != nil && *e.LocalName == codeLocalName {
				codePropID = e.ID
				break
			}
		}
		if codePropID == 0 {
			// 懒创建：实体行 + DECLARATION(10) + 42 值域 xsd:string 行同批落库，变更日志同批留痕
			lazyDraft, lazyDeclaration, err = s.factory.CreateEntityDraft(ctx, ontologyID, ontology.EntityTypeDataProperty, baseIRI, codeLocalName)
			if err != nil {
				return nil, "", err
			}
		}
	}

	// b. 编码来源（AUTO 前缀快照取号 / MANUAL 原样）
	code := ""
	switch codeSource {
	case CodeSourceAuto:
		annAxioms, err := tx.ListAnnotationAxiomsBySubjects(ctx, ontologyID, []uint32{mainClassID})
		if err != nil {
			return nil, "", err
		}
		prefix := ontology.PickAnnotationPreferred(annAxioms, ontology.PredicateSKOSNotation)
		if prefix == nil || *prefix == "" {
			return nil, "", ontology.NewError(ontology.ReasonIndividualCodeSourceInvalid,
				"主类目无编码，无法自动赋码，请手输编码或改选手动模式")
		}
		if err = ontology.ValidateCodeCharset(*prefix); err != nil {
			return nil, "", err
		}
		seq, err := allocator.NextNo(ctx, ontologyID, ontology.SerialPurposeCode, *prefix)
		if err != nil {
			return nil, "", err
		}
		code = ontology.FormatCode(*prefix, seq)
	default: // MANUAL（服务层已校验非空与字符集；此处仅取值）
		code = strings.TrimSpace(req.GetCode())
	}

	// c. 编码唯一性（同本体 + 同编码属性 + 值 → INDIVIDUAL_CODE_DUPLICATE；
	// 懒创建场景新属性尚无任何 62 行，天然唯一）
	if codePropID != 0 {
		if err := s.checkCodeUniqueInTx(ctx, tx, ontologyID, codePropID, code, 0); err != nil {
			return nil, "", err
		}
	}

	// d. 组装 EntityChangeSet（个体草稿固定 NewEntities[0]；懒创建编码属性追加为 [1]）
	set := &ontology.EntityChangeSet{OntologyID: ontologyID, Operation: ontology.OpCreateIndividual}
	if manualDraft != nil {
		set.NewEntities = append(set.NewEntities, *manualDraft)
		set.Changes = append(set.Changes, manualDeclaration)
	} else {
		// 自动 localName：ind 伪前缀号池取号 → FormatLocalName（§2.1：身份与编码分离）。
		// 手输 localName 抢占了即将发放的名（IRI 撞号）时取下一号重试（有界），避免个别
		// 占位名永久卡死 AUTO 模式；被跳的号不回收（流水号不承诺连续——§4.1 语义③）。
		const maxLocalNameRetry = 5
		draft, declaration, err := func() (*ontology.EntityDraft, ontology.AxiomChange, error) {
			var lastErr error
			for attempt := 0; attempt < maxLocalNameRetry; attempt++ {
				seq, err := allocator.NextNo(ctx, ontologyID, ontology.SerialPurposeInd, ontology.SerialIndPrefix)
				if err != nil {
					return nil, ontology.AxiomChange{}, err
				}
				d, decl, err := s.factory.CreateEntityDraft(ctx, ontologyID, ontology.EntityTypeIndividual, baseIRI, ontology.FormatLocalName(seq))
				if err == nil {
					return d, decl, nil
				}
				if de, ok := err.(*ontology.DomainError); ok && de.Reason == ontology.ReasonEntityIRIDuplicate {
					lastErr = err
					continue
				}
				return nil, ontology.AxiomChange{}, err
			}
			return nil, ontology.AxiomChange{}, lastErr
		}()
		if err != nil {
			return nil, "", err
		}
		set.NewEntities = append(set.NewEntities, *draft)
		set.Changes = append(set.Changes, declaration)
	}
	if lazyDraft != nil {
		set.NewEntities = append(set.NewEntities, *lazyDraft)
		// CreateEntityDraft 硬编码声明 ref=1（NewEntities[0] 惯例）；懒创建草稿挂在个体之后，
		// 须改写为实际下标（index+1），否则声明错绑到个体（校验器以"声明数量应为 1"拦截）
		lazyDeclaration.SubjectNewEntityRef = len(set.NewEntities)
		set.Changes = append(set.Changes, lazyDeclaration)
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op:                  ontology.AxiomOpAdd,
			AxiomType:           ontology.AxiomDataPropertyRange,
			SubjectNewEntityRef: len(set.NewEntities), // 值域 xsd:string 行挂在懒创建的编码属性上
			ValueText:           ontology.IRIXMLSchemaString,
		})
	}

	// 类型断言×N（主类目 sort_order=0，其余按提交序 1..n——§2.4）
	set.Changes = append(set.Changes, ontology.AxiomChange{
		Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomTypeAssertion,
		SubjectNewEntityRef: 1, ObjectID: mainClassID, SortOrder: 0,
	})
	for i, id := range typeIDs {
		set.Changes = append(set.Changes, ontology.AxiomChange{
			Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomTypeAssertion,
			SubjectNewEntityRef: 1, ObjectID: id, SortOrder: int32(i + 1),
		})
	}

	// 编码断言（62 行；object 端按懒创建形态走 NewEntityRef 或既有 id）
	codeChange := ontology.AxiomChange{
		Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDataPropertyAssertion,
		SubjectNewEntityRef: 1, ValueText: code,
	}
	if lazyDraft != nil {
		codeChange.ObjectNewEntityRef = len(set.NewEntities) // 懒创建编码属性（NewEntities 末位）
	} else {
		codeChange.ObjectID = codePropID
	}
	set.Changes = append(set.Changes, codeChange)

	// 注解（label/comment/deprecated 可选）+ 自定义注解（04 双轨）
	appendAnnotationAdds(set, req.DisplayLabel, req.Comment, nil, req.Deprecated)
	ontology.AppendCustomAnnotationAxioms(set, 1, 0, annotationInputs)

	return set, code, nil
}

// checkCodeUniqueInTx 编码唯一性校验（同本体 + 同编码属性 + 同值即拒绝；excludeSelf 供改码路径
// 排除自身现存行）。在 ApplyInTx 回调内执行（读公理走传入事务），并发双写由本体乐观锁串行化。
func (s *OntologyIndividualService) checkCodeUniqueInTx(ctx context.Context, tx ontology.AxiomTx,
	ontologyID, codePropID uint32, code string, excludeSelf uint32) error {
	axioms, err := tx.ListAxioms(ctx, ontologyID)
	if err != nil {
		return err
	}
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyAssertion {
			continue
		}
		if ax.ObjectID == nil || *ax.ObjectID != codePropID {
			continue
		}
		if ax.ValueText == nil || *ax.ValueText != code {
			continue
		}
		if excludeSelf != 0 && ax.SubjectID != nil && *ax.SubjectID == excludeSelf {
			continue
		}
		return ontology.NewError(ontology.ReasonIndividualCodeDuplicate, "个体编码已存在：%s", code)
	}
	return nil
}

// ---- update（§4.3 三区 diff：类型专用全量替换 / 编码删旧增新 / 断言区键比对） ----

func (s *OntologyIndividualService) Update(ctx context.Context, req *ontologyV1.UpdateIndividualRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireIndividualEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	ont, err := s.requireOntology(ctx, ontologyID)
	if err != nil {
		return nil, err
	}
	baseIRI := derefStr(ont.BaseIri)

	// 类型区校验（主类目必填；变更不触发改码——§2.4）
	mainClassID := req.GetMainClassId()
	if mainClassID == 0 {
		return nil, ontologyV1.ErrorIndividualMainClassRequired("个体必须指定主类目")
	}
	if err = s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{mainClassID}, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
		return nil, mapDomainError(err)
	}
	typeIDs := dedupeUint32(req.GetTypeIds())
	typeIDs = removeUint32(typeIDs, mainClassID)
	if err = s.factory.ValidateRefEntities(ctx, ontologyID, typeIDs, ontology.EntityTypeClass, ontology.ReasonEntityTypeMismatch); err != nil {
		return nil, mapDomainError(err)
	}

	// 对象属性断言前置校验：谓词须命中本本体已声明对象属性 + 目标为已声明个体（§7 #16）
	objectInputs := make([]ontology.ObjectAssertionInput, 0, len(req.GetObjectAssertions()))
	for _, in := range req.GetObjectAssertions() {
		if in == nil {
			continue
		}
		predicate := strings.TrimSpace(in.GetPredicateIri())
		if predicate == "" || in.GetTargetId() == 0 {
			continue // 前端空行防御
		}
		if err = s.validateObjectPropertyPredicate(ctx, ontologyID, predicate); err != nil {
			return nil, err
		}
		if err = s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{in.GetTargetId()}, ontology.EntityTypeIndividual, ontology.ReasonIndividualTargetInvalid); err != nil {
			return nil, mapDomainError(err)
		}
		objectInputs = append(objectInputs, ontology.ObjectAssertionInput{PredicateIRI: predicate, TargetID: in.GetTargetId()})
	}

	// 数据属性断言前置校验：属性须为已声明数据属性（词法按值域校验由校验器统一兜底）
	dataInputs := make([]ontology.DataAssertionInput, 0, len(req.GetDataAssertions()))
	for _, in := range req.GetDataAssertions() {
		if in == nil || in.GetPropertyId() == 0 {
			continue // 前端空行防御
		}
		if err = s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{in.GetPropertyId()}, ontology.EntityTypeDataProperty, ontology.ReasonDataPropertyTargetInvalid); err != nil {
			return nil, mapDomainError(err)
		}
		dataInputs = append(dataInputs, ontology.DataAssertionInput{
			PropertyID: in.GetPropertyId(), Value: in.GetValue(), Lang: strings.TrimSpace(in.GetLang()),
		})
	}

	// 改码（显式动作；编码不可清空——空串拒绝）
	codeChange := false
	newCode := ""
	if req.Code != nil {
		code := strings.TrimSpace(*req.Code)
		if code == "" {
			return nil, ontologyV1.ErrorIndividualCodeRequired("个体编码不能为空（编码不可清空，如需变更请输入新编码）")
		}
		if err = ontology.ValidateCodeCharset(code); err != nil {
			return nil, mapDomainError(err)
		}
		codeChange = true
		newCode = code
	}
	explicitCodePropID := uint32(0)
	if req.CodePropertyId != nil && *req.CodePropertyId > 0 {
		if err = s.factory.ValidateRefEntities(ctx, ontologyID, []uint32{*req.CodePropertyId}, ontology.EntityTypeDataProperty, ontology.ReasonDataPropertyTargetInvalid); err != nil {
			return nil, mapDomainError(err)
		}
		explicitCodePropID = *req.CodePropertyId
	}

	// 注解区（04 双轨）
	annotationInputs, err := normalizeAnnotationInputs(req.GetAnnotations())
	if err != nil {
		return nil, err
	}
	if err = validateAnnotationPropertyRefs(ctx, s.factory, ontologyID, annotationInputs); err != nil {
		return nil, err
	}

	if _, err = s.applier.ApplyInTx(ctx, operatorUserID(ctx), ontologyID, req.BaseVersion, ontology.OpUpdateIndividual,
		func(tx ontology.AxiomTx) (*ontology.EntityChangeSet, error) {
			return s.assembleUpdateChangeset(ctx, tx, ontologyID, baseIRI, req.GetId(), mainClassID, typeIDs,
				codeChange, newCode, explicitCodePropID, objectInputs, dataInputs, annotationInputs, req)
		}); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// assembleUpdateChangeset 更新变更集组装（ApplyInTx 回调；个体已存在，主语侧 diff 全部走实 id）
func (s *OntologyIndividualService) assembleUpdateChangeset(
	ctx context.Context, tx ontology.AxiomTx, ontologyID uint32, baseIRI string,
	selfID, mainClassID uint32, typeIDs []uint32,
	codeChange bool, newCode string, explicitCodePropID uint32,
	objectInputs []ontology.ObjectAssertionInput, dataInputs []ontology.DataAssertionInput,
	annotationInputs []ontology.CustomAnnotationInput, req *ontologyV1.UpdateIndividualRequest,
) (*ontology.EntityChangeSet, error) {
	set := &ontology.EntityChangeSet{OntologyID: ontologyID, BaseVersion: req.BaseVersion, Operation: ontology.OpUpdateIndividual}

	// 编码属性解析：约定解析始终执行——即使本次不改码，编码 62 行也要从断言区比对中隔离
	//（漏解析会让编码行被断言区全量替换误删）；改码且无既有属性 → 同事务懒创建
	codePropID := explicitCodePropID
	lazyRef := 0
	{
		entities, err := tx.ListEntities(ctx, ontologyID)
		if err != nil {
			return nil, err
		}
		for _, e := range entities {
			if codePropID == 0 && e.EntityType != nil && *e.EntityType == ontology.EntityTypeDataProperty &&
				e.LocalName != nil && *e.LocalName == codeLocalName {
				codePropID = e.ID
			}
		}
	}
	if codeChange && codePropID == 0 {
		{
			draft, declaration, err := s.factory.CreateEntityDraft(ctx, ontologyID, ontology.EntityTypeDataProperty, baseIRI, codeLocalName)
			if err != nil {
				return nil, err
			}
			set.NewEntities = append(set.NewEntities, *draft)
			lazyRef = len(set.NewEntities)            // NewEntities 下标 + 1（resolveRef 约定）
			declaration.SubjectNewEntityRef = lazyRef // CreateEntityDraft 硬编码 ref=1，显式对齐防漂移
			set.Changes = append(set.Changes, declaration)
			set.Changes = append(set.Changes, ontology.AxiomChange{
				Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDataPropertyRange,
				SubjectNewEntityRef: lazyRef, ValueText: ontology.IRIXMLSchemaString,
			})
		}
	}
	if codeChange && codePropID != 0 {
		if err := s.checkCodeUniqueInTx(ctx, tx, ontologyID, codePropID, newCode, selfID); err != nil {
			return nil, err
		}
	}

	// 现状（Tx 内读全量公理；frame 全量提交语义逐区 diff）
	axioms, err := tx.ListAxioms(ctx, ontologyID)
	if err != nil {
		return nil, err
	}
	subjectAxioms := filterBySubject(axioms, selfID)
	typeRows := filterByType(subjectAxioms, ontology.AxiomTypeAssertion)
	objectAssertionRows := filterByType(subjectAxioms, ontology.AxiomObjectPropertyAssertion)
	dataAssertionRows := filterByType(subjectAxioms, ontology.AxiomDataPropertyAssertion)
	annotationRows := filterByType(subjectAxioms, ontology.AxiomAnnotationAssertion)

	changes := make([]ontology.AxiomChange, 0, len(subjectAxioms)+len(typeIDs))

	// 类型区：专用全量替换 diff（顺序变化即删全部增全部、sort_order 重建——§2.4）
	targetTypeIDs := make([]uint32, 0, len(typeIDs)+1)
	targetTypeIDs = append(targetTypeIDs, mainClassID)
	targetTypeIDs = append(targetTypeIDs, typeIDs...)
	changes = append(changes, ontology.DiffTypeAssertions(selfID, typeRows, targetTypeIDs)...)

	// 编码区：改码时删旧 62 行（编码属性名下全部行）+ 增新行（纠错/重排同事务留痕——§4.3；
	// 主类目变更不改码——编码终身不变）
	if codeChange {
		for _, ax := range dataAssertionRows {
			if ax.ObjectID != nil && *ax.ObjectID == codePropID {
				changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: ax.ID, SubjectID: selfID})
			}
		}
	}
	if codeChange {
		c := ontology.AxiomChange{
			Op: ontology.AxiomOpAdd, AxiomType: ontology.AxiomDataPropertyAssertion,
			SubjectID: selfID, ValueText: newCode,
		}
		if lazyRef > 0 {
			c.ObjectNewEntityRef = lazyRef
		} else {
			c.ObjectID = codePropID
		}
		changes = append(changes, c)
	}

	// 属性断言区（61 按 (predicate_iri, target_id)、62 按 (property_id, value, lang) 键全量替换；
	// 编码 62 行归编码区管，不参与本区比对——§4.3）
	nonCodeAssertionRows := make([]*ent.OntAxiom, 0, len(dataAssertionRows))
	for _, ax := range dataAssertionRows {
		if ax.ObjectID != nil && *ax.ObjectID == codePropID {
			continue
		}
		nonCodeAssertionRows = append(nonCodeAssertionRows, ax)
	}
	changes = append(changes, ontology.DiffObjectAssertions(selfID, objectAssertionRows, objectInputs)...)
	changes = append(changes, ontology.DiffDataAssertions(selfID, nonCodeAssertionRows, dataInputs)...)

	// 注解区（内建 + 自定义双轨，全量替换语义）
	changes = append(changes, ontology.DiffAnnotations(selfID, annotationRows, map[string]*string{
		ontology.PredicateRDFSLabel:   req.DisplayLabel, // nil = 不变；空串 = 清空
		ontology.PredicateRDFSComment: req.Comment,
	})...)
	if req.Deprecated != nil {
		changes = append(changes, ontology.DiffDeprecated(selfID, annotationRows, *req.Deprecated)...)
	}
	changes = append(changes, ontology.DiffCustomAnnotations(selfID, annotationRows, annotationInputs)...)

	set.Changes = changes
	return set, nil
}

// validateObjectPropertyPredicate 61 谓词校验：IRI 命中本本体已声明对象属性 → PROPERTY_TARGET_INVALID
func (s *OntologyIndividualService) validateObjectPropertyPredicate(ctx context.Context, ontologyID uint32, predicateIRI string) error {
	entities, err := s.entityRepo.GetEntityByIRI(ctx, predicateIRI)
	if err != nil {
		return adminV1.ErrorInternalServerError("query predicate entities failed")
	}
	for _, e := range entities {
		if derefU32(e.OntologyID) != ontologyID {
			continue
		}
		if e.EntityType != nil && *e.EntityType == ontology.EntityTypeObjectProperty {
			return nil
		}
		return ontologyV1.ErrorPropertyTargetInvalid("引用目标必须是已声明的对象属性：%s", predicateIRI)
	}
	return ontologyV1.ErrorPropertyTargetInvalid("引用目标必须是已声明的对象属性：%s", predicateIRI)
}

// ---- delete（usage 保护 + 号不回收——§4.4） ----

func (s *OntologyIndividualService) Delete(ctx context.Context, req *ontologyV1.DeleteIndividualRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("id is required")
	}
	e, err := s.requireIndividualEntity(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	ontologyID := derefU32(e.OntologyID)
	if _, err = s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	force := req.Force != nil && *req.Force

	// usage 保护：61 宾语命中（主语侧断言属自身 frame 不计入；自引用行随主语侧消亡亦不计）
	objectAxioms, err := s.axiomRepo.ListByObject(ctx, ontologyID, req.GetId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("count references failed")
	}
	refRows := make([]*ent.OntAxiom, 0, len(objectAxioms))
	for _, ax := range objectAxioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomObjectPropertyAssertion {
			continue
		}
		if ax.SubjectID != nil && *ax.SubjectID == req.GetId() {
			continue
		}
		refRows = append(refRows, ax)
	}
	if len(refRows) > 0 && !force {
		return nil, ontologyV1.ErrorEntityInUse("个体被 %d 处对象属性断言引用，无法删除", len(refRows))
	}

	// 收集删除变更：主语侧全部公理（60/61/62/声明/注解）+ force 级联"以我为宾语"的 61 行
	subjectAxioms, err := s.axiomRepo.ListBySubject(ctx, ontologyID, req.GetId())
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query axioms failed")
	}
	deleted := make(map[uint32]struct{}, len(subjectAxioms)+len(refRows))
	changes := make([]ontology.AxiomChange, 0, len(subjectAxioms)+len(refRows))
	for _, ax := range subjectAxioms {
		if _, ok := deleted[ax.ID]; ok {
			continue
		}
		deleted[ax.ID] = struct{}{}
		changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: ax.ID, SubjectID: derefU32(ax.SubjectID)})
	}
	for _, ax := range refRows { // force=true：引用方个体的 frame 内断言随之消失（语义等价 OWLEntityDeleter）
		if _, ok := deleted[ax.ID]; ok {
			continue
		}
		deleted[ax.ID] = struct{}{}
		changes = append(changes, ontology.AxiomChange{Op: ontology.AxiomOpDelete, AxiomID: ax.ID, SubjectID: derefU32(ax.SubjectID)})
	}

	// 号不回收：ont_serials.next_no 不回退，删除后取号续发（编码管理基本原则）
	set := &ontology.EntityChangeSet{
		OntologyID:      ontologyID,
		BaseVersion:     req.BaseVersion,
		Operation:       ontology.OpDeleteIndividual,
		Changes:         changes,
		DeleteEntityIDs: []uint32{req.GetId()},
	}
	if _, err = s.applier.Apply(ctx, set, operatorUserID(ctx)); err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- 小工具 ----

// removeUint32 从切片中移除指定 id（保序）
func removeUint32(ids []uint32, target uint32) []uint32 {
	out := ids[:0:0]
	for _, id := range ids {
		if id != target {
			out = append(out, id)
		}
	}
	return out
}

// intersectUint32 前置过滤段交集：prevConstrained=false 时 prev 未生效（直接取 next），
// 否则取二者交集（保序）。
func intersectUint32(prev []uint32, prevConstrained bool, next []uint32) []uint32 {
	if !prevConstrained {
		return next
	}
	seen := make(map[uint32]struct{}, len(prev))
	for _, id := range prev {
		seen[id] = struct{}{}
	}
	out := make([]uint32, 0, len(next))
	for _, id := range next {
		if _, ok := seen[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
