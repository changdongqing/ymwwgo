package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// OntChangeLog holds the schema definition for the OntChangeLog entity.
//
// 本体变更日志（对应 Protégé HistoryManager），V1 只记不回滚。
// 一次公理事务（ChangeApplier.Apply）写一行，change_detail 是公理级增删 JSON 快照。
type OntChangeLog struct {
	ent.Schema
}

func (OntChangeLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "ont_change_logs",
		},
		entsql.WithComments(true),
		schema.Comment("本体变更日志表（对应 Protégé HistoryManager，V1 只记不回滚）"),
	}
}

// Mixin of the OntChangeLog.
func (OntChangeLog) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
	}
}

// Fields of the OntChangeLog.
func (OntChangeLog) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("ontology_id").
			Comment("所属本体 id").
			Optional().
			Nillable(),

		field.String("operation").
			Comment("操作名 CREATE_CLASS / UPDATE_CLASS / DELETE_CLASS / CREATE_ONTOLOGY ...").
			MaxLen(32).
			NotEmpty().
			Optional().
			Nillable(),

		field.Text("change_detail").
			Comment("公理级增删 JSON 快照（added=新增公理字段数组，deleted=被删公理 id 数组）").
			Optional().
			Nillable(),

		field.Uint32("before_version").
			Comment("变更前本体版本").
			Optional().
			Nillable(),

		field.Uint32("after_version").
			Comment("变更后本体版本").
			Optional().
			Nillable(),
	}
}

// Indexes of the OntChangeLog.
func (OntChangeLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ontology_id", "created_at").
			StorageKey("idx_ont_change_logs_ontology_time"),
	}
}
