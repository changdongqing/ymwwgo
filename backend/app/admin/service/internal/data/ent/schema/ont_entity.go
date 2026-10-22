package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// OntEntity holds the schema definition for the OntEntity entity.
//
// 六类本体实体统一注册表（类/对象属性/数据属性/注解属性/数据类型/个体）。
// 与 ont_axioms 之间不建 FK 约束（源同款）：跨表一致性由公理事务管线
// （internal/ontology.ChangeApplier）在同一 ent Tx 内保证。
type OntEntity struct {
	ent.Schema
}

func (OntEntity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "ont_entities",
		},
		entsql.WithComments(true),
		schema.Comment("本体实体注册表（六类实体统一注册：类/对象属性/数据属性/注解属性/数据类型/个体）"),
	}
}

// Mixin of the OntEntity.
func (OntEntity) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
	}
}

// Fields of the OntEntity.
func (OntEntity) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("ontology_id").
			Comment("所属本体 id（ont_ontologies.id，无 FK，事务保证一致性）").
			Optional().
			Nillable(),

		// 实体类型用号段数字（不用 field.Enum）：02~06 期逐期启用预留号段时
		// 无需改 schema 重跑 ent 生成，可读性由 Go 常量与 proto 枚举承载
		field.Uint32("entity_type").
			Comment("实体类型 10:类 20:对象属性 30:数据属性 40:注解属性 50:数据类型 60:个体").
			Optional().
			Nillable(),

		field.String("iri").
			Comment("完整 IRI，本体文档内唯一").
			MaxLen(512).
			NotEmpty().
			Immutable().
			Optional().
			Nillable(),

		field.String("local_name").
			Comment("局部名，创建后不可变（V1 不提供 rename）").
			MaxLen(255).
			NotEmpty().
			Immutable().
			Optional().
			Nillable(),

		// 渲染缓存列：非事实源，rdfs:label / owl:deprecated 注解公理变更时同事务刷新（设计文档 §3.3）
		field.String("display_label").
			Comment("渲染缓存：rdfs:label 首选值；事实源在注解公理，同事务同步").
			MaxLen(255).
			Optional().
			Nillable(),

		field.Bool("deprecated").
			Comment("渲染缓存：owl:deprecated").
			Default(false).
			Optional().
			Nillable(),
	}
}

// Indexes of the OntEntity.
func (OntEntity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ontology_id", "entity_type").
			StorageKey("idx_ont_entities_ontology_type"),

		index.Fields("local_name").
			StorageKey("idx_ont_entities_local_name"),

		// 部分唯一索引：软删后 IRI 可复用；并发双写由 DB 兜底（23505 → ENTITY_IRI_DUPLICATE）
		index.Fields("ontology_id", "iri").
			Unique().
			StorageKey("uix_ont_entities_ontology_iri").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
	}
}
