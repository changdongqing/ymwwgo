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
