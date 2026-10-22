package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// OntOntology holds the schema definition for the OntOntology entity.
//
// 本体文档表（owl:Ontology，平台级：不挂 TenantID、不配租户 Policy，参照 sys_languages）。
// 建模用户是平台管理员，四张 ont_* 表均为平台级共享知识资产；设计文档
// docs/ontology/design/01-阶段0前置与本体类详细设计.md §2.3 / §3.1。
type OntOntology struct {
	ent.Schema
}

func (OntOntology) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "ont_ontologies",
		},
		entsql.WithComments(true),
		schema.Comment("本体文档表（owl:Ontology，平台级：不挂 TenantID、不配租户 Policy，参照 sys_languages）"),
	}
}

// Mixin of the OntOntology.
func (OntOntology) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.IsEnabled{},
	}
}

// Fields of the OntOntology.
func (OntOntology) Fields() []ent.Field {
	return []ent.Field{
		field.String("ontology_name").
			Comment("显示名").
			MaxLen(128).
			NotEmpty().
			Optional().
			Nillable(),

		field.String("ontology_iri").
			Comment("本体 IRI（owl:Ontology IRI，全局唯一）").
			MaxLen(512).
			NotEmpty().
			Immutable().
			Optional().
			Nillable(),

		field.String("base_iri").
			Comment("实体命名空间，以 # 结尾；实体 IRI = base_iri || local_name").
			MaxLen(512).
			NotEmpty().
			Immutable(). // 创建后不可变（源修订 #1：变更命名空间会使既有实体 IRI 语义漂移），服务层 Update 忽略
			Optional().
			Nillable(),

		field.String("description").
			Comment("描述").
			MaxLen(512).
			Default("").
			Optional().
			Nillable(),

		// 乐观锁版本号：任何公理级变更（创建/修改/删除实体）在同一事务内 +1
		// （本仓首个乐观锁先例，见设计文档 §2.4；本仓现存其他 version 字段均为业务语义）
		field.Uint32("version").
			Comment("乐观锁版本号").
			Default(0).
			Optional().
			Nillable(),
	}
}

// Indexes of the OntOntology.
func (OntOntology) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ontology_name").
			StorageKey("idx_ont_ontologies_ontology_name"),

		// 部分唯一索引：软删后 IRI 可复用；并发双写由 DB 兜底（23505 → ENTITY_IRI_DUPLICATE）
		index.Fields("ontology_iri").
			Unique().
			StorageKey("uix_ont_ontologies_iri").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
	}
}
