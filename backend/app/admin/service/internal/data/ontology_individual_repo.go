package data

import (
	"context"
	"strings"

	"go-wind-admin/app/admin/service/internal/data/ent"
	"go-wind-admin/app/admin/service/internal/data/ent/ontaxiom"
	"go-wind-admin/app/admin/service/internal/data/ent/ontentity"
	"go-wind-admin/app/admin/service/internal/ontology"
)

// IndividualListFilter 个体列表过滤条件（06 号文档 §5：两段式查询——先行按索引解析过滤 id 集
// → 实体分页 IN → 聚合列批量填充）。keyword 对局部名/显示名 contains（铁律 3；二者 OR）。
type IndividualListFilter struct {
	OntologyID uint32
	Keyword    string   // 局部名/显示名 contains（OR 语义）
	Deprecated *bool    // 精确
	IDs        []uint32 // 前置过滤 id 集（编码/类目两段查询的交集；nil = 不限）
	Page       uint32
	PageSize   uint32
	OrderBy    string // JSON 数组字符串，默认 '["-updated_at"]'
}

// ListIndividuals 分页查询个体列表（两段式查询的第二段：实体分页；
// 能力挂载在 OntEntityRepo 上——个体行同样是 ont_entities 行）
func (r *OntEntityRepo) ListIndividuals(ctx context.Context, filter IndividualListFilter) ([]*ent.OntEntity, int, error) {
	q := r.entClient.Client().OntEntity.Query().
		Where(
			ontentity.OntologyIDEQ(filter.OntologyID),
			ontentity.EntityTypeEQ(ontology.EntityTypeIndividual),
			ontentity.DeletedAtIsNil(),
		)
	if filter.Keyword != "" {
		q = q.Where(ontentity.Or(
			ontentity.LocalNameContains(filter.Keyword),
			ontentity.DisplayLabelContains(filter.Keyword),
		))
	}
	if filter.Deprecated != nil {
		q = q.Where(ontentity.DeprecatedEQ(*filter.Deprecated))
	}
	if filter.IDs != nil {
		if len(filter.IDs) == 0 {
			return nil, 0, nil // 前置过滤段已判空（如编码属性不存在）：直接空页
		}
		q = q.Where(ontentity.IDIn(filter.IDs...))
	}

	total, err := q.Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "count individuals failed: %s", err.Error())
		return nil, 0, err
	}

	q = applyIndividualOrder(q, filter.OrderBy)

	if filter.PageSize > 0 {
		q = q.Limit(int(filter.PageSize))
		if filter.Page > 1 {
			q = q.Offset(int((filter.Page - 1) * filter.PageSize))
		}
	}

	entities, err := q.All(ctx)
	if err != nil {
		r.log.Errorf(ctx, "list individuals failed: %s", err.Error())
		return nil, 0, err
	}
	return entities, total, nil
}

// applyIndividualOrder 排序白名单（防注入，复用 classOrderWhitelist 业务列；
// 个体列表默认 -updated_at——列表主打"最近变更"）
func applyIndividualOrder(q *ent.OntEntityQuery, orderBy string) *ent.OntEntityQuery {
	fields := parseOrderBy(orderBy)
	if len(fields) == 0 {
		return q.Order(ent.Desc(ontentity.FieldUpdatedAt))
	}
	order := make([]ontentity.OrderOption, 0, len(fields))
	for _, f := range fields {
		column, ok := classOrderWhitelist[strings.TrimPrefix(f, "-")]
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
		return q.Order(ent.Desc(ontentity.FieldUpdatedAt))
	}
	return q.Order(order...)
}

// IndividualAggregate 个体列表聚合列（一次分页批量计算，避免前端逐行算）
type IndividualAggregate struct {
	MainClassID   uint32  // 主类目实体 id（60 行 sort_order 最小）
	mainClassSort uint32  // 主类目行的 sort_order（内部比较用）
	TypeCount     uint32  // 类型断言数
	Code          *string // 编码值（62 行、编码属性谓词）
	Comment       *string // rdfs:comment 首选值
}

// AggregateIndividualInfo 批量计算个体聚合列：主类目/类型数/编码/注释。
// codePropertyID 为编码属性实体 id（服务层按"localName=code 约定"解析后传入；0 = 不解析编码列）。
func (r *OntEntityRepo) AggregateIndividualInfo(ctx context.Context, ontologyID uint32, entityIDs []uint32, codePropertyID uint32) (map[uint32]*IndividualAggregate, error) {
	out := make(map[uint32]*IndividualAggregate, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}

	axioms, err := r.entClient.Client().OntAxiom.Query().
		Where(
			ontaxiom.OntologyIDEQ(ontologyID),
			ontaxiom.SubjectIDIn(entityIDs...),
			ontaxiom.DeletedAtIsNil(),
		).
		Order(ent.Asc(ontaxiom.FieldSortOrder), ent.Asc(ontaxiom.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf(ctx, "aggregate individual info failed: %s", err.Error())
		return nil, err
	}

	for _, id := range entityIDs {
		agg := &IndividualAggregate{}
		for _, ax := range axioms {
			if ax.SubjectID == nil || *ax.SubjectID != id || ax.AxiomType == nil {
				continue
			}
			switch *ax.AxiomType {
			case ontology.AxiomTypeAssertion:
				if ax.ObjectID == nil || *ax.ObjectID == 0 {
					continue
				}
				if agg.MainClassID == 0 || sortOrderUint(ax) < agg.mainClassSort {
					agg.MainClassID = *ax.ObjectID
					agg.mainClassSort = sortOrderUint(ax)
				}
				agg.TypeCount++
			case ontology.AxiomDataPropertyAssertion:
				if codePropertyID > 0 && ax.ObjectID != nil && *ax.ObjectID == codePropertyID && ax.ValueText != nil && agg.Code == nil {
					agg.Code = ax.ValueText
				}
			}
		}
		out[id] = agg
	}

	// 注释首选值复用注断言谓词回退规则（与 frame 同一条规则，避免两处漂移）
	for _, id := range entityIDs {
		rows := make([]*ent.OntAxiom, 0, 2)
		for _, ax := range axioms {
			if ax.SubjectID != nil && *ax.SubjectID == id &&
				ax.AxiomType != nil && *ax.AxiomType == ontology.AxiomAnnotationAssertion {
				rows = append(rows, ax)
			}
		}
		out[id].Comment = ontology.PickAnnotationPreferred(rows, ontology.PredicateRDFSComment)
	}
	return out, nil
}

// sortOrderUint nil 安全的 sort_order 读取
func sortOrderUint(ax *ent.OntAxiom) uint32 {
	if ax.SortOrder == nil {
		return 0
	}
	return *ax.SortOrder
}
