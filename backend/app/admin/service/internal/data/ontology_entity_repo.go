package data

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bLogger "github.com/tx7do/kratos-bootstrap/logger"

	entCrud "github.com/tx7do/go-crud/entgo"
	"github.com/tx7do/go-utils/trans"

	ontologyV1 "go-wind-admin/api/gen/go/ontology/service/v1"
	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/data/ent/ontaxiom"
	"go-wind-admin/app/admin/service/internal/data/ent/ontentity"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// ClassListFilter 类/属性列表过滤条件（铁律 3：contains 模糊；ID 类字段不进模糊搜索）。
// 02 期对象属性列表复用同一过滤结构（无子树范围，属性树不做范围切换）。
type ClassListFilter struct {
	OntologyID   uint32
	LocalName    string   // contains
	DisplayLabel string   // contains
	Deprecated   *bool    // 精确
	SubtreeIDs   []uint32 // 子树范围（服务端算好的后代闭包含自身）；空 = 不过滤
	Page         uint32
	PageSize     uint32
	OrderBy      string // JSON 数组字符串，默认 '["-created_at"]'
}

// ClassAggregate 列表聚合列（一次分页批量计算，避免前端逐行算）
type ClassAggregate struct {
	ParentCount     uint32
	ChildCount      uint32
	DisjointCount   uint32
	Comment         *string
	Notation        *string
	Characteristics []string // 02 期对象属性特征 IRI（按声明序）；类列表恒空
}

// OntEntityRepo 本体实体仓储：读路径 + ontology.EntityReader 实现（工厂查重/引用校验）。
type OntEntityRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *bLogger.Helper
}

func NewOntEntityRepo(
	ctx *bootstrap.Context,
	entClient *entCrud.EntClient[*ent.Client],
) *OntEntityRepo {
	return &OntEntityRepo{
		log:       ctx.NewLoggerHelper("ontology-entity/repo/admin-service"),
		entClient: entClient,
	}
}

// ---- ontology.EntityReader ----

// IRIExists 本体下该 IRI 是否已有未删实体（服务层前置查重；DB 部分唯一索引兜底并发双写）
func (r *OntEntityRepo) IRIExists(ctx context.Context, ontologyID uint32, iri string) (bool, error) {
	return r.entClient.Client().OntEntity.Query().
		Where(
			ontentity.OntologyIDEQ(ontologyID),
			ontentity.IriEQ(iri),
			ontentity.DeletedAtIsNil(),
		).
		Exist(ctx)
}

// GetEntitiesByIDs 批量取未删实体（同本体过滤在此完成，跨本体 id 视为不存在）
func (r *OntEntityRepo) GetEntitiesByIDs(ctx context.Context, ontologyID uint32, ids []uint32) ([]*ent.OntEntity, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return r.entClient.Client().OntEntity.Query().
		Where(
			ontentity.OntologyIDEQ(ontologyID),
			ontentity.IDIn(ids...),
			ontentity.DeletedAtIsNil(),
		).
		All(ctx)
}

// ---- 读路径 ----

// GetEntity 取单个未删实体
func (r *OntEntityRepo) GetEntity(ctx context.Context, id uint32) (*ent.OntEntity, error) {
	return r.entClient.Client().OntEntity.Query().
		Where(ontentity.IDEQ(id), ontentity.DeletedAtIsNil()).
		Only(ctx)
}

// ListEntitiesByType 本体下指定类型的全部未删实体（树构建输入）
// ListAllEntities 本体下全部未删实体（07 期 OWL 导出快照读；type=0 过滤无意义的 EntityTypeEQ(0)）
func (r *OntEntityRepo) ListAllEntities(ctx context.Context, ontologyID uint32) ([]*ent.OntEntity, error) {
	return r.entClient.Client().OntEntity.Query().
		Where(ontentity.OntologyIDEQ(ontologyID), ontentity.DeletedAtIsNil()).
		Order(ent.Asc(ontentity.FieldID)).
		All(ctx)
}

func (r *OntEntityRepo) ListEntitiesByType(ctx context.Context, ontologyID uint32, entityType uint32) ([]*ent.OntEntity, error) {
	return r.entClient.Client().OntEntity.Query().
		Where(
			ontentity.OntologyIDEQ(ontologyID),
			ontentity.EntityTypeEQ(entityType),
			ontentity.DeletedAtIsNil(),
		).
		All(ctx)
}

// ListClasses 分页查询类列表（contains 搜索在 repo 层显式实现，不走通用 filter DSL）
func (r *OntEntityRepo) ListClasses(ctx context.Context, filter ClassListFilter) ([]*ent.OntEntity, int, error) {
	return r.listEntitiesOfType(ctx, filter, ontology.EntityTypeClass)
}

// ListObjectProperties 分页查询对象属性列表（铁律 3 contains；无子树范围——属性树不做范围切换）
func (r *OntEntityRepo) ListObjectProperties(ctx context.Context, filter ClassListFilter) ([]*ent.OntEntity, int, error) {
	filter.SubtreeIDs = nil
	return r.listEntitiesOfType(ctx, filter, ontology.EntityTypeObjectProperty)
}

// ListDataProperties 分页查询数据属性列表（铁律 3 contains；无子树范围——属性树不做范围切换）
func (r *OntEntityRepo) ListDataProperties(ctx context.Context, filter ClassListFilter) ([]*ent.OntEntity, int, error) {
	filter.SubtreeIDs = nil
	return r.listEntitiesOfType(ctx, filter, ontology.EntityTypeDataProperty)
}

// ListAnnotationProperties 分页查询注解属性列表（铁律 3 contains；无子树范围——属性树不做范围切换）
func (r *OntEntityRepo) ListAnnotationProperties(ctx context.Context, filter ClassListFilter) ([]*ent.OntEntity, int, error) {
	filter.SubtreeIDs = nil
	return r.listEntitiesOfType(ctx, filter, ontology.EntityTypeAnnotationProperty)
}

// ListDatatypes 分页查询数据类型列表（铁律 3 contains；无树——OWL 2 无 subDatatypeOf，05 号文档 §2.5）
func (r *OntEntityRepo) ListDatatypes(ctx context.Context, filter ClassListFilter) ([]*ent.OntEntity, int, error) {
	filter.SubtreeIDs = nil
	return r.listEntitiesOfType(ctx, filter, ontology.EntityTypeDatatype)
}

// ListSimpleDatatypes 全量查询数据类型简况（值域双源下拉数据源；按局部名排序保证下拉稳定）
func (r *OntEntityRepo) ListSimpleDatatypes(ctx context.Context, ontologyID uint32) ([]*ent.OntEntity, error) {
	return r.entClient.Client().OntEntity.Query().
		Where(
			ontentity.OntologyIDEQ(ontologyID),
			ontentity.EntityTypeEQ(ontology.EntityTypeDatatype),
			ontentity.DeletedAtIsNil(),
		).
		Order(ent.Asc(ontentity.FieldLocalName), ent.Asc(ontentity.FieldID)).
		All(ctx)
}

// GetEntityByIRI 全局 IRI 反查（未删；跨本体判定需要全局视野——同一 IRI 可能在多个本体注册，
// 调用方按"同本体命中优先"扫描。值域三分支解析与 42 派生轨校验共用）
func (r *OntEntityRepo) GetEntityByIRI(ctx context.Context, iri string) ([]*ent.OntEntity, error) {
	return r.entClient.Client().OntEntity.Query().
		Where(ontentity.IriEQ(iri), ontentity.DeletedAtIsNil()).
		Order(ent.Asc(ontentity.FieldID)).
		All(ctx)
}

// GetEntitiesByIRIs 批量 IRI 反查（同本体、未删；08 期反查的 61 谓词节点批量填充——
// 单条 WHERE iri IN (...) 避免逐 IRI N+1）
func (r *OntEntityRepo) GetEntitiesByIRIs(ctx context.Context, ontologyID uint32, iris []string) ([]*ent.OntEntity, error) {
	if len(iris) == 0 {
		return nil, nil
	}
	return r.entClient.Client().OntEntity.Query().
		Where(
			ontentity.OntologyIDEQ(ontologyID),
			ontentity.IriIn(iris...),
			ontentity.DeletedAtIsNil(),
		).
		Order(ent.Asc(ontentity.FieldID)).
		All(ctx)
}

// listEntitiesOfType 分页查询指定实体类型列表（ListClasses 的泛化底座）
func (r *OntEntityRepo) listEntitiesOfType(ctx context.Context, filter ClassListFilter, entityType uint32) ([]*ent.OntEntity, int, error) {
	q := r.entClient.Client().OntEntity.Query().
		Where(
			ontentity.OntologyIDEQ(filter.OntologyID),
			ontentity.EntityTypeEQ(entityType),
			ontentity.DeletedAtIsNil(),
		)
	if filter.LocalName != "" {
		q = q.Where(ontentity.LocalNameContains(filter.LocalName))
	}
	if filter.DisplayLabel != "" {
		q = q.Where(ontentity.DisplayLabelContains(filter.DisplayLabel))
	}
	if filter.Deprecated != nil {
		q = q.Where(ontentity.DeprecatedEQ(*filter.Deprecated))
	}
	if len(filter.SubtreeIDs) > 0 {
		q = q.Where(ontentity.IDIn(filter.SubtreeIDs...))
	}

	total, err := q.Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "count classes failed: %s", err.Error())
		return nil, 0, err
	}

	q = applyClassOrder(q, filter.OrderBy)

	if filter.PageSize > 0 {
		q = q.Limit(int(filter.PageSize))
		if filter.Page > 1 {
			q = q.Offset(int((filter.Page - 1) * filter.PageSize))
		}
	}

	entities, err := q.All(ctx)
	if err != nil {
		r.log.Errorf(ctx, "list classes failed: %s", err.Error())
		return nil, 0, err
	}
	return entities, total, nil
}

// classOrderWhitelist 排序白名单（防注入；仅允许业务列）
var classOrderWhitelist = map[string]string{
	"created_at":    ontentity.FieldCreatedAt,
	"updated_at":    ontentity.FieldUpdatedAt,
	"local_name":    ontentity.FieldLocalName,
	"display_label": ontentity.FieldDisplayLabel,
	"iri":           ontentity.FieldIri,
	"id":            ontentity.FieldID,
}

// applyClassOrder 解析 order_by（JSON 数组字符串，"-" 前缀 = 倒序）；白名单外字段忽略。
func applyClassOrder(q *ent.OntEntityQuery, orderBy string) *ent.OntEntityQuery {
	fields := parseOrderBy(orderBy)
	if len(fields) == 0 {
		return q.Order(ent.Desc(ontentity.FieldCreatedAt))
	}
	order := make([]ontentity.OrderOption, 0, len(fields))
	for _, f := range fields {
		name := strings.TrimPrefix(f, "-")
		column, ok := classOrderWhitelist[name]
		if !ok {
			continue
		}
		if strings.HasPrefix(f, "-") {
			order = append(order, ent.Desc(column))
		} else {
			order = append(order, ent.Asc(column))
		}
	}
	if len(order) == 0 {
		return q.Order(ent.Desc(ontentity.FieldCreatedAt))
	}
	return q.Order(order...)
}

// parseOrderBy 解析 '["-created_at","local_name"]' 形态的 JSON 数组字符串
func parseOrderBy(orderBy string) []string {
	trimmed := strings.TrimSpace(orderBy)
	if trimmed == "" {
		return nil
	}
	var fields []string
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return nil
	}
	return fields
}

// AggregateClassInfo 批量计算聚合列：父类数/直接子类数/不相交数 + 注释/编码首选值。
func (r *OntEntityRepo) AggregateClassInfo(ctx context.Context, ontologyID uint32, entityIDs []uint32) (map[uint32]*ClassAggregate, error) {
	out := make(map[uint32]*ClassAggregate, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}
	for _, id := range entityIDs {
		out[id] = &ClassAggregate{}
	}

	type countRow struct {
		SubjectID uint32 `json:"subject_id"`
		ObjectID  uint32 `json:"object_id"`
		Count     int    `json:"count"`
	}

	// 父类数：20 型公理以我为主语
	var parentRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomSubClassOf),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldSubjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &parentRows); err != nil {
		return nil, err
	}
	for _, row := range parentRows {
		if agg, ok := out[row.SubjectID]; ok {
			agg.ParentCount = uint32(row.Count)
		}
	}

	// 直接子类数：20 型公理以我为宾语
	var childRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomSubClassOf),
			ontaxiom.ObjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldObjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &childRows); err != nil {
		return nil, err
	}
	for _, row := range childRows {
		if agg, ok := out[row.ObjectID]; ok {
			agg.ChildCount = uint32(row.Count)
		}
	}

	// 不相交数：21 型公理以我为主语计数（成对存储只数主语侧，防双倍）
	var disjointRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomDisjointWith),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldSubjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &disjointRows); err != nil {
		return nil, err
	}
	for _, row := range disjointRows {
		if agg, ok := out[row.SubjectID]; ok {
			agg.DisjointCount = uint32(row.Count)
		}
	}

	// 注释/编码首选值：100 型注解公理，lang 优先级与渲染缓存同规则
	axioms, err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomAnnotationAssertion),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	bySubject := make(map[uint32][]*ent.OntAxiom, len(entityIDs))
	for _, ax := range axioms {
		if ax.SubjectID == nil {
			continue
		}
		bySubject[*ax.SubjectID] = append(bySubject[*ax.SubjectID], ax)
	}
	for id, agg := range out {
		agg.Comment = ontology.PickAnnotationPreferred(bySubject[id], ontology.PredicateRDFSComment)
		agg.Notation = ontology.PickAnnotationPreferred(bySubject[id], ontology.PredicateSKOSNotation)
	}
	return out, nil
}

// AggregateObjectPropertyInfo 批量计算对象属性聚合列：父属性数/直接子属性数（30 型）+
// 特征 IRI（33 型，按声明序输出）+ 注释首选值。
func (r *OntEntityRepo) AggregateObjectPropertyInfo(ctx context.Context, ontologyID uint32, entityIDs []uint32) (map[uint32]*ClassAggregate, error) {
	out := make(map[uint32]*ClassAggregate, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}
	for _, id := range entityIDs {
		out[id] = &ClassAggregate{}
	}

	type countRow struct {
		SubjectID uint32 `json:"subject_id"`
		ObjectID  uint32 `json:"object_id"`
		Count     int    `json:"count"`
	}

	// 父属性数：30 型公理以我为主语
	var parentRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomSubObjectPropertyOf),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldSubjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &parentRows); err != nil {
		return nil, err
	}
	for _, row := range parentRows {
		if agg, ok := out[row.SubjectID]; ok {
			agg.ParentCount = uint32(row.Count)
		}
	}

	// 直接子属性数：30 型公理以我为宾语
	var childRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomSubObjectPropertyOf),
			ontaxiom.ObjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldObjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &childRows); err != nil {
		return nil, err
	}
	for _, row := range childRows {
		if agg, ok := out[row.ObjectID]; ok {
			agg.ChildCount = uint32(row.Count)
		}
	}

	// 特征 + 注释：33/100 型公理
	axioms, err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.AxiomTypeIn(ontology.AxiomObjectPropertyCharacteristic, ontology.AxiomAnnotationAssertion),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	bySubject := make(map[uint32][]*ent.OntAxiom, len(entityIDs))
	for _, ax := range axioms {
		if ax.SubjectID == nil {
			continue
		}
		bySubject[*ax.SubjectID] = append(bySubject[*ax.SubjectID], ax)
	}
	for id, agg := range out {
		agg.Comment = ontology.PickAnnotationPreferred(bySubject[id], ontology.PredicateRDFSComment)
		agg.Characteristics = orderCharacteristicIRIs(bySubject[id])
	}
	return out, nil
}

// orderCharacteristicIRIs 特征 IRI 按 PropertyCharacteristicIRIs 声明序输出（源修订 #5）；
// 白名单外 IRI（校验器已拦，防御兼容）排声明序尾部、保持行序。
func orderCharacteristicIRIs(axioms []*ent.OntAxiom) []string {
	rank := make(map[string]int, len(ontology.PropertyCharacteristicIRIs))
	for i, iri := range ontology.PropertyCharacteristicIRIs {
		rank[iri] = i
	}
	type row struct {
		iri  string
		rank int
		seq  uint32
	}
	rows := make([]row, 0, len(axioms))
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomObjectPropertyCharacteristic || ax.PredicateIri == nil {
			continue
		}
		r, ok := rank[*ax.PredicateIri]
		if !ok {
			r = len(ontology.PropertyCharacteristicIRIs)
		}
		rows = append(rows, row{iri: *ax.PredicateIri, rank: r, seq: ax.ID})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].rank != rows[j].rank {
			return rows[i].rank < rows[j].rank
		}
		return rows[i].seq < rows[j].seq
	})
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.iri)
	}
	return out
}

// EntEntityToObjectPropertyDTO 实体行 + 聚合 → ObjectProperty DTO（服务层组装用）
func EntEntityToObjectPropertyDTO(e *ent.OntEntity, agg *ClassAggregate) *ontologyV1.ObjectProperty {
	dto := &ontologyV1.ObjectProperty{
		Id:           &e.ID,
		OntologyId:   e.OntologyID,
		Iri:          e.Iri,
		LocalName:    e.LocalName,
		DisplayLabel: e.DisplayLabel,
		Deprecated:   e.Deprecated,
	}
	if agg != nil {
		dto.ParentCount = trans.Ptr(agg.ParentCount)
		dto.ChildCount = trans.Ptr(agg.ChildCount)
		dto.Comment = agg.Comment
		dto.Characteristics = agg.Characteristics
	}
	return dto
}

// DataPropertyAggregate 数据属性列表聚合列（一次分页批量计算）
type DataPropertyAggregate struct {
	ParentCount     uint32
	ChildCount      uint32
	Comment         *string
	Characteristics []string // 43 型特征 IRI（V1 至多函数性）
	RangeIRIs       []string // 42 型值域 datatype IRI（保序 sort_order → id）
}

// AggregateDataPropertyInfo 批量计算数据属性聚合列：父属性数/直接子属性数（40 型）+
// 特征 IRI（43 型）+ 值域 datatype IRI（42 型 value_text，保序 sort_order → id）+ 注释首选值。
func (r *OntEntityRepo) AggregateDataPropertyInfo(ctx context.Context, ontologyID uint32, entityIDs []uint32) (map[uint32]*DataPropertyAggregate, error) {
	out := make(map[uint32]*DataPropertyAggregate, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}
	for _, id := range entityIDs {
		out[id] = &DataPropertyAggregate{}
	}

	type countRow struct {
		SubjectID uint32 `json:"subject_id"`
		ObjectID  uint32 `json:"object_id"`
		Count     int    `json:"count"`
	}

	// 父属性数：40 型公理以我为主语
	var parentRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomSubDataPropertyOf),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldSubjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &parentRows); err != nil {
		return nil, err
	}
	for _, row := range parentRows {
		if agg, ok := out[row.SubjectID]; ok {
			agg.ParentCount = uint32(row.Count)
		}
	}

	// 直接子属性数：40 型公理以我为宾语
	var childRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomSubDataPropertyOf),
			ontaxiom.ObjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldObjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &childRows); err != nil {
		return nil, err
	}
	for _, row := range childRows {
		if agg, ok := out[row.ObjectID]; ok {
			agg.ChildCount = uint32(row.Count)
		}
	}

	// 特征 / 值域 / 注释：43 / 42 / 100 型公理
	axioms, err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.AxiomTypeIn(ontology.AxiomDataPropertyCharacteristic, ontology.AxiomDataPropertyRange, ontology.AxiomAnnotationAssertion),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		Order(ent.Asc(ontaxiom.FieldSortOrder), ent.Asc(ontaxiom.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	bySubject := make(map[uint32][]*ent.OntAxiom, len(entityIDs))
	rangeObjectIDs := make([]uint32, 0)
	for _, ax := range axioms {
		if ax.SubjectID == nil {
			continue
		}
		bySubject[*ax.SubjectID] = append(bySubject[*ax.SubjectID], ax)
	}
	for id := range bySubject {
		rangeObjectIDs = append(rangeObjectIDs, RangeObjectIDsOf(bySubject[id])...)
	}
	// 42 派生轨反查 datatype 实体 IRI（跨 id 批量；悬空行投影时防御跳过）
	datatypeIRIs := make(map[uint32]string, len(rangeObjectIDs))
	if len(rangeObjectIDs) > 0 {
		datatypeEntities, err := r.GetEntitiesByIDs(ctx, ontologyID, rangeObjectIDs)
		if err != nil {
			return nil, err
		}
		for _, e := range datatypeEntities {
			if e.Iri != nil {
				datatypeIRIs[e.ID] = *e.Iri
			}
		}
	}
	for id, agg := range out {
		agg.Comment = ontology.PickAnnotationPreferred(bySubject[id], ontology.PredicateRDFSComment)
		agg.Characteristics = orderDataPropertyCharacteristics(bySubject[id])
		agg.RangeIRIs = RangeIRIsOf(bySubject[id], datatypeIRIs)
	}
	return out, nil
}

// orderDataPropertyCharacteristics 特征 IRI 按声明序（V1 数据属性仅函数性；白名单外防御性补尾）
func orderDataPropertyCharacteristics(axioms []*ent.OntAxiom) []string {
	out := make([]string, 0, 1)
	present := make(map[string]struct{}, 1)
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyCharacteristic || ax.PredicateIri == nil {
			continue
		}
		present[*ax.PredicateIri] = struct{}{}
	}
	if _, ok := present[ontology.IRIOWLFunctionalProperty]; ok {
		out = append(out, ontology.IRIOWLFunctionalProperty)
		delete(present, ontology.IRIOWLFunctionalProperty)
	}
	for _, ax := range axioms { // 白名单外（校验器已拦，防御兼容）按行序补尾
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

// RangeObjectIDsOf 收集 42 行派生轨 object_id（保序去重；批量反查 datatype 实体 IRI 的输入）。
func RangeObjectIDsOf(axioms []*ent.OntAxiom) []uint32 {
	out := make([]uint32, 0, 1)
	seen := make(map[uint32]struct{}, 1)
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyRange || ax.ObjectID == nil {
			continue
		}
		if *ax.ObjectID == 0 {
			continue
		}
		if _, ok := seen[*ax.ObjectID]; ok {
			continue
		}
		seen[*ax.ObjectID] = struct{}{}
		out = append(out, *ax.ObjectID)
	}
	return out
}

// RangeIRIsOf 42 行双轨投影为 IRI 序列（05 期增量，frame 与 page 聚合共用投影——05 号文档 §4.4）：
// 内建轨 value_text 原样、派生轨 object_id 经 entityIRIByID 反查实体 IRI；悬空 object_id 行
// （实体已删，直改库场景）防御跳过。axioms 须按 sort_order → id 保序（ListBySubject/聚合查询序）。
func RangeIRIsOf(axioms []*ent.OntAxiom, entityIRIByID map[uint32]string) []string {
	out := make([]string, 0, 1)
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyRange {
			continue
		}
		if ax.ObjectID != nil && *ax.ObjectID != 0 {
			if iri, ok := entityIRIByID[*ax.ObjectID]; ok {
				out = append(out, iri)
			}
			continue
		}
		if ax.ValueText != nil && *ax.ValueText != "" {
			out = append(out, *ax.ValueText)
		}
	}
	return out
}

// RangeRowsOf 42 行投影为双轨 diff 行（DiffRangeRefs 输入；axioms 按 sort_order → id 保序）：
// 内建轨 IRI=value_text；派生轨 IRI=实体 IRI 反查——悬空行 IRI 置空，由 diff 删除侧清理自愈。
func RangeRowsOf(axioms []*ent.OntAxiom, entityIRIByID map[uint32]string) []ontology.RangeRow {
	out := make([]ontology.RangeRow, 0, len(axioms))
	for _, ax := range axioms {
		if ax.AxiomType == nil || *ax.AxiomType != ontology.AxiomDataPropertyRange {
			continue
		}
		row := ontology.RangeRow{
			AxiomID:   ax.ID,
			ValueText: nilStringAx(ax.ValueText),
			ObjectID:  nilUintAx(ax.ObjectID),
			SortOrder: int32(nilUintAx(ax.SortOrder)),
		}
		if row.ObjectID != 0 {
			row.IRI = entityIRIByID[row.ObjectID] // 悬空行取零值空串
		} else {
			row.IRI = row.ValueText
		}
		out = append(out, row)
	}
	return out
}

// nilStringAx / nilUintAx / nilInt32Ax 公理可空列读取（data 包本地版，避免循环依赖服务层辅助）
func nilStringAx(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nilUintAx(p *uint32) uint32 {
	if p == nil {
		return 0
	}
	return *p
}

// EntEntityToDataPropertyDTO 实体行 + 聚合 → DataProperty DTO（服务层组装用）
func EntEntityToDataPropertyDTO(e *ent.OntEntity, agg *DataPropertyAggregate) *ontologyV1.DataProperty {
	dto := &ontologyV1.DataProperty{
		Id:           &e.ID,
		OntologyId:   e.OntologyID,
		Iri:          e.Iri,
		LocalName:    e.LocalName,
		DisplayLabel: e.DisplayLabel,
		Deprecated:   e.Deprecated,
	}
	if agg != nil {
		dto.ParentCount = trans.Ptr(agg.ParentCount)
		dto.ChildCount = trans.Ptr(agg.ChildCount)
		dto.Comment = agg.Comment
		dto.Characteristics = agg.Characteristics
		dto.RangeIris = agg.RangeIRIs
	}
	return dto
}

// AnnotationPropertyAggregate 注解属性列表聚合列（一次分页批量计算）
type AnnotationPropertyAggregate struct {
	ParentCount uint32
	ChildCount  uint32
	Comment     *string
}

// AggregateAnnotationPropertyInfo 批量计算注解属性聚合列：父属性数/直接子属性数（50 型）+
// 注释首选值。注解属性无特征/域值域/等价/不相交（OWL 2 语言约束，04 号文档 §2.7）。
func (r *OntEntityRepo) AggregateAnnotationPropertyInfo(ctx context.Context, ontologyID uint32, entityIDs []uint32) (map[uint32]*AnnotationPropertyAggregate, error) {
	out := make(map[uint32]*AnnotationPropertyAggregate, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}
	for _, id := range entityIDs {
		out[id] = &AnnotationPropertyAggregate{}
	}

	type countRow struct {
		SubjectID uint32 `json:"subject_id"`
		ObjectID  uint32 `json:"object_id"`
		Count     int    `json:"count"`
	}

	// 父属性数：50 型公理以我为主语
	var parentRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomSubAnnotationPropertyOf),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldSubjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &parentRows); err != nil {
		return nil, err
	}
	for _, row := range parentRows {
		if agg, ok := out[row.SubjectID]; ok {
			agg.ParentCount = uint32(row.Count)
		}
	}

	// 直接子属性数：50 型公理以我为宾语
	var childRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomSubAnnotationPropertyOf),
			ontaxiom.ObjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldObjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &childRows); err != nil {
		return nil, err
	}
	for _, row := range childRows {
		if agg, ok := out[row.ObjectID]; ok {
			agg.ChildCount = uint32(row.Count)
		}
	}

	// 注释首选值：100 型注解公理（内建谓词行；自定义行不参与聚合列）
	axioms, err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomAnnotationAssertion),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	bySubject := make(map[uint32][]*ent.OntAxiom, len(entityIDs))
	for _, ax := range axioms {
		if ax.SubjectID == nil {
			continue
		}
		bySubject[*ax.SubjectID] = append(bySubject[*ax.SubjectID], ax)
	}
	for id, agg := range out {
		agg.Comment = ontology.PickAnnotationPreferred(bySubject[id], ontology.PredicateRDFSComment)
	}
	return out, nil
}

// EntEntityToAnnotationPropertyDTO 实体行 + 聚合 → AnnotationProperty DTO（服务层组装用）
func EntEntityToAnnotationPropertyDTO(e *ent.OntEntity, agg *AnnotationPropertyAggregate) *ontologyV1.AnnotationProperty {
	dto := &ontologyV1.AnnotationProperty{
		Id:           &e.ID,
		OntologyId:   e.OntologyID,
		Iri:          e.Iri,
		LocalName:    e.LocalName,
		DisplayLabel: e.DisplayLabel,
		Deprecated:   e.Deprecated,
	}
	if agg != nil {
		dto.ParentCount = trans.Ptr(agg.ParentCount)
		dto.ChildCount = trans.Ptr(agg.ChildCount)
		dto.Comment = agg.Comment
	}
	return dto
}

// EntEntityToClassDTO 实体行 + 聚合 → OntologyClass DTO（服务层组装用）
func EntEntityToClassDTO(e *ent.OntEntity, agg *ClassAggregate) *ontologyV1.OntologyClass {
	dto := &ontologyV1.OntologyClass{
		Id:           &e.ID,
		OntologyId:   e.OntologyID,
		Iri:          e.Iri,
		LocalName:    e.LocalName,
		DisplayLabel: e.DisplayLabel,
		Deprecated:   e.Deprecated,
	}
	if agg != nil {
		dto.ParentCount = trans.Ptr(agg.ParentCount)
		dto.ChildCount = trans.Ptr(agg.ChildCount)
		dto.DisjointCount = trans.Ptr(agg.DisjointCount)
		dto.Comment = agg.Comment
		dto.Notation = agg.Notation
	}
	return dto
}

// DatatypeAggregate 数据类型列表聚合列（一次分页批量计算；05 期）
type DatatypeAggregate struct {
	Comment        *string
	DefinitionJSON *string // 46 行 value_text（首行；多行脏数据由服务层 diff 自愈，聚合防御取首行）
	ReferenceCount uint32  // 被 42 行派生轨引用的数据属性数
}

// AggregateDatatypeInfo 批量计算数据类型聚合列：定义 JSON（46 型首行）+ 被引用数
// （42 行派生轨 object_id 命中的数据属性数）+ 注释首选值。definition summary / base_iri
// 的 JSON 解析在 DTO 投影完成（损坏 JSON 列表防御降级，不阻塞——05 号文档 §7 #11）。
func (r *OntEntityRepo) AggregateDatatypeInfo(ctx context.Context, ontologyID uint32, entityIDs []uint32) (map[uint32]*DatatypeAggregate, error) {
	out := make(map[uint32]*DatatypeAggregate, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}
	for _, id := range entityIDs {
		out[id] = &DatatypeAggregate{}
	}

	type countRow struct {
		SubjectID uint32 `json:"subject_id"`
		ObjectID  uint32 `json:"object_id"`
		Count     int    `json:"count"`
	}

	// 定义 JSON：46 型公理以我为主语（按 sort_order → id 取首行）
	definitions, err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomDatatypeDefinition),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		Order(ent.Asc(ontaxiom.FieldSortOrder), ent.Asc(ontaxiom.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, ax := range definitions {
		if ax.SubjectID == nil {
			continue
		}
		if agg, ok := out[*ax.SubjectID]; ok && agg.DefinitionJSON == nil && ax.ValueText != nil {
			agg.DefinitionJSON = ax.ValueText
		}
	}

	// 被引用数：42 行派生轨以我为宾语（distinct subject 计数——同属性多值域行不会重复计）
	var refRows []countRow
	if err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomDataPropertyRange),
			ontaxiom.ObjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		GroupBy(ontaxiom.FieldObjectID, ontaxiom.FieldSubjectID).
		Aggregate(ent.As(ent.Count(), "count")).
		Scan(ctx, &refRows); err != nil {
		return nil, err
	}
	refCount := make(map[uint32]int, len(entityIDs))
	seenPair := make(map[[2]uint32]struct{}, len(refRows))
	for _, row := range refRows {
		pair := [2]uint32{row.ObjectID, row.SubjectID}
		if _, ok := seenPair[pair]; ok {
			continue
		}
		seenPair[pair] = struct{}{}
		refCount[row.ObjectID]++
	}
	for id, agg := range out {
		agg.ReferenceCount = uint32(refCount[id])
	}

	// 注释首选值：100 型注解公理（内建谓词行）
	axioms, err := r.entClient.Client().OntAxiom.Query().
		Where(ontaxiom.OntologyIDEQ(ontologyID), ontaxiom.AxiomTypeEQ(ontology.AxiomAnnotationAssertion),
			ontaxiom.SubjectIDIn(entityIDs...), ontaxiom.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	bySubject := make(map[uint32][]*ent.OntAxiom, len(entityIDs))
	for _, ax := range axioms {
		if ax.SubjectID == nil {
			continue
		}
		bySubject[*ax.SubjectID] = append(bySubject[*ax.SubjectID], ax)
	}
	for id, agg := range out {
		agg.Comment = ontology.PickAnnotationPreferred(bySubject[id], ontology.PredicateRDFSComment)
	}
	return out, nil
}

// EntEntityToDatatypeDTO 实体行 + 聚合 → Datatype DTO（服务层组装用；定义摘要由
// ontology.DefinitionSummary 统一投影，损坏 JSON 防御降级为"未定义"形态）
func EntEntityToDatatypeDTO(e *ent.OntEntity, agg *DatatypeAggregate) *ontologyV1.Datatype {
	dto := &ontologyV1.Datatype{
		Id:           &e.ID,
		OntologyId:   e.OntologyID,
		Iri:          e.Iri,
		LocalName:    e.LocalName,
		DisplayLabel: e.DisplayLabel,
		Deprecated:   e.Deprecated,
	}
	if agg == nil {
		return dto
	}
	dto.Comment = agg.Comment
	dto.ReferenceCount = trans.Ptr(agg.ReferenceCount)
	if agg.DefinitionJSON != nil {
		if def, err := ontology.ParseDefinition(*agg.DefinitionJSON); err == nil && !def.IsEmpty() {
			dto.BaseIri = trans.Ptr(strings.TrimSpace(def.BaseIRI))
			dto.DefinitionSummary = trans.Ptr(ontology.DefinitionSummary(def))
		}
	}
	return dto
}
