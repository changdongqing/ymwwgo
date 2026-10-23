package service

import (
	"context"
	"net/url"
	"strings"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminV1 "go-wind-admin/api/gen/go/admin/service/v1"
	ontologyV1 "go-wind-admin/api/gen/go/ontology/service/v1"

	"go-wind-admin/app/admin/service/internal/data"
	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// OntologyReferenceService 通用反查（引用边查询）服务（08 期）。
//
// 只读投影：一次请求答案"哪些边引用了目标实体"，边单位而非去重实体集合（与六个 Usage RPC
// 的本质差异）。单端点 + axiomTypes/positions 参数化覆盖六类实体；不为每类实体复制端点。
// 不写变更日志、不动本体版本（§2.3）。
type OntologyReferenceService struct {
	adminV1.OntologyReferenceServiceHTTPServer

	log          *bLogger.Helper
	ontologyRepo *data.OntologyRepo
	entityRepo   *data.OntEntityRepo
	axiomRepo    *data.OntAxiomRepo
}

func NewOntologyReferenceService(
	ctx *bootstrap.Context,
	ontologyRepo *data.OntologyRepo,
	entityRepo *data.OntEntityRepo,
	axiomRepo *data.OntAxiomRepo,
) *OntologyReferenceService {
	return &OntologyReferenceService{
		log:          ctx.NewLoggerHelper("ontology-reference/service/admin-service"),
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		axiomRepo:    axiomRepo,
	}
}

// 分页钳制（对齐 docs/list_query_rule.md 与全局 TABLE.DEFAULT_PAGE_SIZE）
const (
	referenceDefaultPageSize = 20
	referenceMaxPageSize     = 200
)

// referenceQuery 反查的规范化入参（校验/装配完成后进入取数管线）
type referenceQuery struct {
	ontologyID   uint32
	target       *ent.OntEntity
	targetID     uint32
	targetIRI    string
	targetType   uint32
	types        []uint32
	positions    []ontology.ReferencePosition
	predicateIRI string // 谓词收敛 IRI（61 文本匹配 / 62 实体匹配）
	predicateID  uint32 // 62 的谓词实体（IRI 反解失败为 0）
	includeDesc  bool
	excludeDep   bool
	page         int
	pageSize     int
	order        []ontology.RefOrderField
}

// ListReferences 反查引用目标实体的边（分页）。
func (s *OntologyReferenceService) ListReferences(ctx context.Context, req *ontologyV1.ListReferencesRequest) (*ontologyV1.ListReferencesResponse, error) {
	if req == nil || req.GetOntologyId() == 0 {
		return nil, adminV1.ErrorBadRequest("ontology id is required")
	}
	if req.GetTargetId() == 0 {
		return nil, adminV1.ErrorBadRequest("target id is required")
	}

	ontologyID := req.GetOntologyId()
	if _, err := s.requireOntology(ctx, ontologyID); err != nil {
		return nil, err
	}

	target, err := s.requireTargetEntity(ctx, ontologyID, req.GetTargetId())
	if err != nil {
		return nil, err
	}

	types, err := resolveReferenceAxiomTypes(req.GetAxiomTypes())
	if err != nil {
		return nil, err
	}
	positions, err := resolveReferencePositions(req.GetPositions())
	if err != nil {
		return nil, err
	}
	predicateIRI := strings.TrimSpace(req.GetPredicateIri())
	predicateID, err := s.resolveReferencePredicate(ctx, ontologyID, predicateIRI, types)
	if err != nil {
		return nil, err
	}

	page, pageSize := clampReferencePage(req.GetPage(), req.GetPageSize())
	q := &referenceQuery{
		ontologyID:   ontologyID,
		target:       target,
		targetID:     target.ID,
		targetIRI:    derefStr(target.Iri),
		targetType:   derefU32(target.EntityType),
		types:        types,
		positions:    positions,
		predicateIRI: predicateIRI,
		predicateID:  predicateID,
		includeDesc:  req.GetIncludeDescendants(),
		excludeDep:   req.GetExcludeDeprecated(),
		page:         page,
		pageSize:     pageSize,
		order:        ontology.ParseRefOrder(req.GetOrderBy()),
	}

	rows, total, err := s.collectReferenceRows(ctx, q)
	if err != nil {
		return nil, err
	}

	index, err := s.buildReferenceNodeIndex(ctx, ontologyID, rows)
	if err != nil {
		return nil, err
	}
	if q.excludeDep {
		rows = filterDeprecatedReferenceRows(rows, index)
		total = int64(len(rows))
	}
	if !q.directPageable() {
		ontology.SortRefRows(rows, q.order)
		total = int64(len(rows))
		rows = ontology.PageRefRows(rows, q.page, q.pageSize)
	}

	edges, err := assembleReferenceEdges(rows, index)
	if err != nil {
		return nil, err
	}
	return &ontologyV1.ListReferencesResponse{Edges: edges, Total: &total}, nil
}

// requireOntology 本体存在且启用（实体级接口统一闸门）
func (s *OntologyReferenceService) requireOntology(ctx context.Context, ontologyID uint32) (*ent.OntOntology, error) {
	ont, err := s.ontologyRepo.GetEntityByID(ctx, ontologyID)
	return checkOntologyEnabled(ont, err)
}

// requireTargetEntity 目标实体存在、未软删且属于该本体（跨本体一律 404，不泄漏他本体存在性）
func (s *OntologyReferenceService) requireTargetEntity(ctx context.Context, ontologyID, targetID uint32) (*ent.OntEntity, error) {
	e, err := s.entityRepo.GetEntity(ctx, targetID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ontologyV1.ErrorEntityNotFound("本体实体不存在：%d", targetID)
		}
		return nil, adminV1.ErrorInternalServerError("query target entity failed")
	}
	if derefU32(e.OntologyID) != ontologyID {
		return nil, ontologyV1.ErrorEntityNotFound("本体实体不存在：%d", targetID)
	}
	return e, nil
}

// resolveReferenceAxiomTypes 公理类型集合校验：缺省取默认集；未知号段 400；重复值静默去重。
func resolveReferenceAxiomTypes(raw []uint32) ([]uint32, error) {
	if len(raw) == 0 {
		return append([]uint32(nil), ontology.DefaultReferenceAxiomTypes...), nil
	}
	out := make([]uint32, 0, len(raw))
	for _, t := range raw {
		if !ontology.IsKnownAxiomType(t) {
			return nil, adminV1.ErrorBadRequest("unknown axiom type: %d", t)
		}
		if !ontology.HasAxiomType(out, t) {
			out = append(out, t)
		}
	}
	return out, nil
}

// resolveReferencePositions 位置集合规范化：缺省 [OBJECT]；UNSPECIFIED 按 OBJECT；保序去重。
func resolveReferencePositions(raw []ontologyV1.ReferencePosition) ([]ontology.ReferencePosition, error) {
	if len(raw) == 0 {
		return []ontology.ReferencePosition{ontology.ReferencePositionObject}, nil
	}
	out := make([]ontology.ReferencePosition, 0, len(raw))
	push := func(p ontology.ReferencePosition) {
		for _, v := range out {
			if v == p {
				return
			}
		}
		out = append(out, p)
	}
	for _, p := range raw {
		switch p {
		case ontologyV1.ReferencePosition_REFERENCE_POSITION_UNSPECIFIED,
			ontologyV1.ReferencePosition_REFERENCE_POSITION_OBJECT:
			push(ontology.ReferencePositionObject)
		case ontologyV1.ReferencePosition_REFERENCE_POSITION_PREDICATE:
			push(ontology.ReferencePositionPredicate)
		case ontologyV1.ReferencePosition_REFERENCE_POSITION_CHAIN_MEMBER:
			push(ontology.ReferencePositionChainMember)
		default:
			return nil, adminV1.ErrorBadRequest("unknown reference position: %d", int32(p))
		}
	}
	return out, nil
}

// resolveReferencePredicate 谓词收敛参数校验与 62 谓词实体反解。
// 约束：必须是绝对 IRI。谓词收敛只对 61/62 有意义——实现修订（08 号文档 §12 #10）：
// 设计原定"与 61/62 以外类型同用 → 400"，但面板默认集含 100/30 等号段，400 会让
// "按谓词过滤"在最常见入口直接不可用；改为有效号段收敛到 {61,62}（其余号段无谓词语义，
// 自然 0 条），不报错。62 的反解失败同样不是错误（该号段随后自然无命中）。
func (s *OntologyReferenceService) resolveReferencePredicate(ctx context.Context, ontologyID uint32, iri string, types []uint32) (uint32, error) {
	if iri == "" {
		return 0, nil
	}
	if u, err := url.Parse(iri); err != nil || !u.IsAbs() {
		return 0, adminV1.ErrorBadRequest("predicate iri is not an absolute IRI: %s", iri)
	}
	if !ontology.HasAxiomType(types, ontology.AxiomDataPropertyAssertion) {
		return 0, nil
	}
	entities, err := s.entityRepo.GetEntityByIRI(ctx, iri)
	if err != nil {
		return 0, adminV1.ErrorInternalServerError("resolve predicate entity failed")
	}
	for _, e := range entities {
		if derefU32(e.OntologyID) == ontologyID && derefU32(e.EntityType) == ontology.EntityTypeDataProperty {
			return e.ID, nil
		}
	}
	return 0, nil
}

// clampReferencePage 页码/页大小钳制：page ≥ 1、1 ≤ pageSize ≤ 200，缺省 1/20。
func clampReferencePage(page, pageSize int32) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = referenceDefaultPageSize
	}
	if pageSize > referenceMaxPageSize {
		pageSize = referenceMaxPageSize
	}
	return int(page), int(pageSize)
}

// directPageable 单位置直查快路径：单位置 + 无成对去重 + 无 deprecated 过滤 + id 排序。
// 快路径把分页/计数交给 SQL（谓词位十万级断言的主要性能路径，§4.6）。
func (q *referenceQuery) directPageable() bool {
	if q.excludeDep || len(q.positions) != 1 {
		return false
	}
	if len(q.order) != 1 || q.order[0].Field != "id" {
		return false
	}
	switch q.positions[0] {
	case ontology.ReferencePositionObject:
		for _, t := range q.types {
			if ontology.IsPairedAxiomType(t) {
				return false
			}
		}
		return true
	case ontology.ReferencePositionPredicate:
		return q.targetType == ontology.EntityTypeObjectProperty &&
			len(ontology.IntersectAxiomTypes(q.types, ontology.PredicateReferenceAxiomTypes)) > 0
	default:
		return false
	}
}

// collectReferenceRows 取数：快路径 SQL 分页；否则全量合并（成对去重 + 37 链 + 多位置并集）。
func (s *OntologyReferenceService) collectReferenceRows(ctx context.Context, q *referenceQuery) ([]ontology.RefRow, int64, error) {
	if q.directPageable() {
		return s.collectDirectRows(ctx, q)
	}
	return s.collectMergedRows(ctx, q)
}

// collectDirectRows 单位置直查（无去重/无合并）
func (s *OntologyReferenceService) collectDirectRows(ctx context.Context, q *referenceQuery) ([]ontology.RefRow, int64, error) {
	offset := (q.page - 1) * q.pageSize
	desc := q.order[0].Desc

	if q.positions[0] == ontology.ReferencePositionPredicate {
		if q.predicateIRI != "" && q.predicateIRI != q.targetIRI {
			return nil, 0, nil // 谓词收敛与目标 IRI 不一致：谓词位不可能命中
		}
		total, err := s.axiomRepo.CountByPredicateIRI(ctx, q.ontologyID, ontology.AxiomObjectPropertyAssertion, q.targetIRI)
		if err != nil {
			return nil, 0, adminV1.ErrorInternalServerError("count references failed")
		}
		if total == 0 {
			return nil, 0, nil
		}
		axioms, err := s.axiomRepo.ListByPredicateIRIPaged(ctx, q.ontologyID,
			ontology.AxiomObjectPropertyAssertion, q.targetIRI, offset, q.pageSize, desc)
		if err != nil {
			return nil, 0, adminV1.ErrorInternalServerError("query references failed")
		}
		return ontology.RefRowsOf(axioms, ontology.ReferencePositionPredicate), int64(total), nil
	}

	filter := data.ObjectReferenceFilter{
		OntologyID:        q.ontologyID,
		ObjectIDs:         []uint32{q.targetID},
		AxiomTypes:        q.types,
		PredicateIRI:      q.predicateIRI,
		PredicateEntityID: q.predicateID,
	}
	total, err := s.axiomRepo.CountByObjectFiltered(ctx, filter)
	if err != nil {
		return nil, 0, adminV1.ErrorInternalServerError("count references failed")
	}
	if total == 0 {
		return nil, 0, nil
	}
	axioms, err := s.axiomRepo.ListByObjectFilteredPage(ctx, filter, offset, q.pageSize, desc)
	if err != nil {
		return nil, 0, adminV1.ErrorInternalServerError("query references failed")
	}
	return ontology.RefRowsOf(axioms, ontology.ReferencePositionObject), int64(total), nil
}

// collectMergedRows 全量合并管线（§4.3）：OBJECT 直查（含类子树展开与成对去重）+
// PREDICATE 谓词直查 + CHAIN_MEMBER 链成员匹配；排序与内存分页在调用方完成。
func (s *OntologyReferenceService) collectMergedRows(ctx context.Context, q *referenceQuery) ([]ontology.RefRow, int64, error) {
	rows := make([]ontology.RefRow, 0, 32)

	if q.hasPosition(ontology.ReferencePositionObject) {
		objectRows, err := s.collectObjectPositionRows(ctx, q)
		if err != nil {
			return nil, 0, err
		}
		rows = append(rows, objectRows...)
	}

	if q.hasPosition(ontology.ReferencePositionPredicate) &&
		q.targetType == ontology.EntityTypeObjectProperty &&
		len(ontology.IntersectAxiomTypes(q.types, ontology.PredicateReferenceAxiomTypes)) > 0 {
		// 谓词收敛存在时，只有收敛 IRI 恰为目标属性 IRI 才可能命中（61 的谓词就是目标）
		if q.predicateIRI == "" || q.predicateIRI == q.targetIRI {
			axioms, err := s.axiomRepo.ListByPredicateIRI(ctx, q.ontologyID,
				ontology.AxiomObjectPropertyAssertion, q.targetIRI)
			if err != nil {
				return nil, 0, adminV1.ErrorInternalServerError("query predicate references failed")
			}
			rows = append(rows, ontology.RefRowsOf(axioms, ontology.ReferencePositionPredicate)...)
		}
	}

	// 谓词收敛只作用于 61/62；37 链行没有谓词语义，收敛开启时整体跳过（避免"过滤后链行仍在"的
	// 结果不一致——实现修订，见 08 号文档 §12 #11）
	if q.hasPosition(ontology.ReferencePositionChainMember) &&
		q.predicateIRI == "" &&
		q.targetType == ontology.EntityTypeObjectProperty &&
		ontology.HasAxiomType(q.types, ontology.AxiomSubPropertyChainOf) {
		allAxioms, err := s.axiomRepo.ListByOntology(ctx, q.ontologyID)
		if err != nil {
			return nil, 0, adminV1.ErrorInternalServerError("query axioms failed")
		}
		rows = append(rows, ontology.ChainRefRows(ctx, allAxioms, q.targetID, s.log)...)
	}

	return rows, int64(len(rows)), nil
}

// collectObjectPositionRows 宾语位取数：类目标 includeDescendants 时把 60/20 的子类集合展开，
// 其余号段始终按目标精确匹配（§3.3）；成对号段按无序对去重（保留小行号，标记 paired）。
func (s *OntologyReferenceService) collectObjectPositionRows(ctx context.Context, q *referenceQuery) ([]ontology.RefRow, error) {
	baseTypes := make([]uint32, 0, len(q.types))
	for _, t := range q.types {
		if t == ontology.AxiomSubPropertyChainOf {
			continue // 成员在 value_text JSON 内，不走 object_id（CHAIN_MEMBER 位处理）
		}
		baseTypes = append(baseTypes, t)
	}

	var expandedTypes []uint32
	if q.includeDesc && q.targetType == ontology.EntityTypeClass {
		expandedTypes = ontology.IntersectAxiomTypes(q.types, []uint32{
			ontology.AxiomSubClassOf,
			ontology.AxiomTypeAssertion,
		})
		if len(expandedTypes) > 0 {
			baseTypes = subtractAxiomTypes(baseTypes, expandedTypes)
		}
	}

	predicateFilter := data.ObjectReferenceFilter{
		OntologyID:        q.ontologyID,
		ObjectIDs:         []uint32{q.targetID},
		AxiomTypes:        baseTypes,
		PredicateIRI:      q.predicateIRI,
		PredicateEntityID: q.predicateID,
	}

	primary := make([]ontology.RefRow, 0, 16)
	if len(baseTypes) > 0 {
		axioms, err := s.axiomRepo.ListByObjectFiltered(ctx, predicateFilter)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query references failed")
		}
		primary = append(primary, ontology.RefRowsOf(axioms, ontology.ReferencePositionObject)...)
	}
	if len(expandedTypes) > 0 {
		classIDs := []uint32{q.targetID}
		allAxioms, err := s.axiomRepo.ListByOntology(ctx, q.ontologyID)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query class hierarchy failed")
		}
		edges := ontology.CollectHierarchyEdges(allAxioms, ontology.AxiomSubClassOf)
		classIDs = ontology.SubtreeIDs(q.targetID, edges) // 含自身；与 baseTypes 无交集
		axioms, err := s.axiomRepo.ListByObjectFiltered(ctx, data.ObjectReferenceFilter{
			OntologyID:        q.ontologyID,
			ObjectIDs:         classIDs,
			AxiomTypes:        expandedTypes,
			PredicateIRI:      q.predicateIRI,
			PredicateEntityID: q.predicateID,
		})
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query subclass instances failed")
		}
		primary = append(primary, ontology.RefRowsOf(axioms, ontology.ReferencePositionObject)...)
	}

	pairedTypes := ontology.IntersectAxiomTypes(q.types, ontology.PairedAxiomTypes)
	if len(pairedTypes) == 0 || len(primary) == 0 {
		return primary, nil
	}

	pairedRows := make([]ontology.RefRow, 0, len(primary))
	otherRows := make([]ontology.RefRow, 0, len(primary))
	for _, row := range primary {
		if ontology.IsPairedAxiomType(row.AxiomType) {
			pairedRows = append(pairedRows, row)
			continue
		}
		otherRows = append(otherRows, row)
	}
	if len(pairedRows) == 0 {
		return otherRows, nil
	}

	counterparts, err := s.axiomRepo.ListBySubjectFiltered(ctx, q.ontologyID, q.targetID, pairedTypes)
	if err != nil {
		return nil, adminV1.ErrorInternalServerError("query paired references failed")
	}
	merged := ontology.MergePairedRows(pairedRows, ontology.RefRowsOf(counterparts, ontology.ReferencePositionObject))
	return append(otherRows, merged...), nil
}

// hasPosition 位置集中是否含指定位置
func (q *referenceQuery) hasPosition(p ontology.ReferencePosition) bool {
	for _, v := range q.positions {
		if v == p {
			return true
		}
	}
	return false
}

// subtractAxiomTypes a - b（保 a 序）
func subtractAxiomTypes(a, b []uint32) []uint32 {
	if len(a) == 0 || len(b) == 0 {
		return a
	}
	remove := make(map[uint32]struct{}, len(b))
	for _, v := range b {
		remove[v] = struct{}{}
	}
	out := make([]uint32, 0, len(a))
	for _, v := range a {
		if _, ok := remove[v]; ok {
			continue
		}
		out = append(out, v)
	}
	return out
}

// ---- 节点批量填充与边组装（两段式第二段：一次 IN 查询，避免逐行 N+1） ----

// referenceNodeIndex 节点索引：实体 id → 实体行 + IRI → 实体行（61 谓词反解）
type referenceNodeIndex struct {
	byID  map[uint32]*ent.OntEntity
	byIRI map[string]*ent.OntEntity
}

// buildReferenceNodeIndex 批量取边两端涉及的实体（主语、宾语、属性实体、谓词 IRI）
func (s *OntologyReferenceService) buildReferenceNodeIndex(ctx context.Context, ontologyID uint32, rows []ontology.RefRow) (*referenceNodeIndex, error) {
	index := &referenceNodeIndex{
		byID:  make(map[uint32]*ent.OntEntity, len(rows)*2),
		byIRI: make(map[string]*ent.OntEntity),
	}
	ids := make([]uint32, 0, len(rows)*2)
	iris := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = appendReferenceID(ids, row.SubjectID)
		ids = appendReferenceID(ids, row.ObjectID)
		if row.AxiomType == ontology.AxiomObjectPropertyAssertion && row.PredicateIRI != "" {
			iris = append(iris, row.PredicateIRI)
		}
	}
	ids = dedupeUint32(ids)
	if len(ids) > 0 {
		entities, err := s.entityRepo.GetEntitiesByIDs(ctx, ontologyID, ids)
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query referenced entities failed")
		}
		index.byID = entityMap(entities)
	}
	if len(iris) > 0 {
		entities, err := s.entityRepo.GetEntitiesByIRIs(ctx, ontologyID, dedupeStrings(iris))
		if err != nil {
			return nil, adminV1.ErrorInternalServerError("query predicate entities failed")
		}
		for _, e := range entities {
			if e.Iri != nil {
				index.byIRI[*e.Iri] = e
			}
		}
	}
	return index, nil
}

// filterDeprecatedReferenceRows 排除含 deprecated 端的边（excludeDeprecated=true 时；
// 默认只标记不过滤——§4.3）
func filterDeprecatedReferenceRows(rows []ontology.RefRow, index *referenceNodeIndex) []ontology.RefRow {
	out := make([]ontology.RefRow, 0, len(rows))
	for _, row := range rows {
		if referenceRowDeprecated(row, index) {
			continue
		}
		out = append(out, row)
	}
	return out
}

// referenceRowDeprecated 边的任一端（主语/宾语/谓词实体）标记 deprecated 即视为废弃边
func referenceRowDeprecated(row ontology.RefRow, index *referenceNodeIndex) bool {
	if e := index.byID[row.SubjectID]; e != nil && e.Deprecated != nil && *e.Deprecated {
		return true
	}
	if row.ObjectID != 0 {
		if e := index.byID[row.ObjectID]; e != nil && e.Deprecated != nil && *e.Deprecated {
			return true
		}
	}
	if row.AxiomType == ontology.AxiomObjectPropertyAssertion && row.PredicateIRI != "" {
		if e := index.byIRI[row.PredicateIRI]; e != nil && e.Deprecated != nil && *e.Deprecated {
			return true
		}
	}
	return false
}

// assembleReferenceEdges 行 → ReferenceEdge（谓词/宾语/字面量按号段语义投影，§3.2）
func assembleReferenceEdges(rows []ontology.RefRow, index *referenceNodeIndex) ([]*ontologyV1.ReferenceEdge, error) {
	edges := make([]*ontologyV1.ReferenceEdge, 0, len(rows))
	for _, row := range rows {
		edge := &ontologyV1.ReferenceEdge{
			AxiomId:   protoUint64(uint64(row.AxiomID)),
			AxiomType: protoUint32(row.AxiomType),
			SortOrder: protoUint32(row.SortOrder),
			Paired:    protoBool(row.Paired),
			Position:  referencePositionProto(row.Position),
			Subject:   referenceNodeOf(index.byID[row.SubjectID]),
		}
		if row.UpdatedAt != nil {
			edge.UpdatedAt = timestamppb.New(*row.UpdatedAt)
		}

		switch row.AxiomType {
		case ontology.AxiomObjectPropertyAssertion:
			edge.Predicate = referencePredicateNode(row.PredicateIRI, index)
			edge.Object = referenceNodeOf(index.byID[row.ObjectID])
		case ontology.AxiomDataPropertyAssertion, ontology.AxiomAnnotationAssertion:
			// 谓词是属性实体（object_id）；字面量进 literal_value（100 的 IRI 轨 V1 不进默认集）
			edge.Predicate = referenceNodeOf(index.byID[row.ObjectID])
			edge.LiteralValue = protoStringIf(row.ValueText)
			edge.Lang = protoStringIf(row.Lang)
		case ontology.AxiomDataPropertyRange:
			if row.ObjectID != 0 {
				edge.Object = referenceNodeOf(index.byID[row.ObjectID])
			} else {
				edge.LiteralValue = protoStringIf(row.ValueText) // 内建轨：IRI 文本（显式请求时）
			}
		case ontology.AxiomDataPropertyCharacteristic, ontology.AxiomObjectPropertyCharacteristic:
			// 特征谓词是 IRI 文本（无实体）；防御分支——默认集不含 33/43，显式请求也不会命中 object_id
			edge.Predicate = referencePredicateNode(row.PredicateIRI, index)
		case ontology.AxiomSubPropertyChainOf:
			// subject=超属性；成员在 value_text JSON 内，原文回显供 UI 定位
			edge.LiteralValue = protoStringIf(row.ValueText)
		default:
			if row.ObjectID != 0 {
				edge.Object = referenceNodeOf(index.byID[row.ObjectID])
			} else {
				edge.LiteralValue = protoStringIf(row.ValueText)
			}
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

// referenceNodeOf 实体行 → 引用节点（已软删实体不在索引内，节点为空不回显）
func referenceNodeOf(e *ent.OntEntity) *ontologyV1.ReferenceNode {
	if e == nil {
		return nil
	}
	return &ontologyV1.ReferenceNode{
		Id:           protoUint32(e.ID),
		EntityType:   e.EntityType,
		LocalName:    e.LocalName,
		DisplayLabel: e.DisplayLabel,
		Iri:          e.Iri,
		Deprecated:   e.Deprecated,
	}
}

// referencePredicateNode 谓词 IRI → 节点；反解不到时以 IRI 尾段构造并标记 unresolved（§4.3）
func referencePredicateNode(iri string, index *referenceNodeIndex) *ontologyV1.ReferenceNode {
	if iri == "" {
		return nil
	}
	if e := index.byIRI[iri]; e != nil {
		return referenceNodeOf(e)
	}
	return &ontologyV1.ReferenceNode{
		Iri:        protoStringIf(iri),
		LocalName:  protoStringIf(iriLocalName(iri)),
		Unresolved: protoBool(true),
	}
}

// referencePositionProto 域内位置 → proto 枚举
func referencePositionProto(p ontology.ReferencePosition) *ontologyV1.ReferencePosition {
	var v ontologyV1.ReferencePosition
	switch p {
	case ontology.ReferencePositionPredicate:
		v = ontologyV1.ReferencePosition_REFERENCE_POSITION_PREDICATE
	case ontology.ReferencePositionChainMember:
		v = ontologyV1.ReferencePosition_REFERENCE_POSITION_CHAIN_MEMBER
	default:
		v = ontologyV1.ReferencePosition_REFERENCE_POSITION_OBJECT
	}
	return &v
}

// iriLocalName IRI 尾段（# 或 / 之后；取不到时原样返回）
func iriLocalName(iri string) string {
	if idx := strings.LastIndex(iri, "#"); idx >= 0 && idx < len(iri)-1 {
		return iri[idx+1:]
	}
	if idx := strings.LastIndex(iri, "/"); idx >= 0 && idx < len(iri)-1 {
		return iri[idx+1:]
	}
	return iri
}

// dedupeStrings 保序去重（空串剔除）
func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// appendReferenceID 非零 id 入列（重复值由 dedupeUint32 统一收敛）
func appendReferenceID(ids []uint32, id uint32) []uint32 {
	if id == 0 {
		return ids
	}
	return append(ids, id)
}

// proto 标量辅助（保持 nil = 字段缺省；生成代码不含指针辅助）
func protoUint32(v uint32) *uint32 { return &v }

func protoUint64(v uint64) *uint64 { return &v }

func protoBool(v bool) *bool { return &v }

func protoStringIf(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
