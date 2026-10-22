package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// OntSerial holds the schema definition for the OntSerial entity.
//
// 本体建模-取号计数表（06 期，系列唯一新表）：个体 localName 流水 / 业务编码流水。
// 取号 = 两段式（UPDATE 未命中 → INSERT 首行 → 唯一键冲突重试 UPDATE），
// 完整唯一索引 (ontology_id, purpose, prefix) 保证取号路径唯一命中一行（06 号文档 §2.3/§4.1）。
// 行不删除、next_no 只前进（编码/局部名不回收）；仅本体删除时级联软删，
// 而本体 id 永不复用，软删行永不与新行冲突——完整唯一索引与部分索引语义等价。
type OntSerial struct {
	ent.Schema
}

func (OntSerial) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "ont_serials",
		},
		entsql.WithComments(true),
		schema.Comment("本体建模-取号计数表（个体 localName 流水 / 业务编码流水）"),
	}
}

// Mixin of the OntSerial.
func (OntSerial) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
	}
}

// Fields of the OntSerial.
func (OntSerial) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("ontology_id").
			Comment("所属本体 id").
			Optional().
			Nillable(),

		field.String("purpose").
			Comment("用途域：ind=个体 localName 自动生成伪前缀($ind)；code=业务编码流水(prefix=主类目编码)").
			MaxLen(16).
			NotEmpty().
			Optional().
			Nillable(),

		field.String("prefix").
			Comment("code 时=主类目 skos:notation（如 A0111）；ind 时固定 $ind").
			MaxLen(128).
			NotEmpty().
			Optional().
			Nillable(),

		field.Uint64("next_no").
			Comment("下一个流水号；已发放最大号 = next_no - 1；行不删除、只前进（编码/局部名不回收）").
			Default(1).
			Optional().
			Nillable(),
	}
}

// Indexes of the OntSerial.
func (OntSerial) Indexes() []ent.Index {
	return []ent.Index{
		// 完整唯一索引：取号路径唯一命中一行的数据完整性底线（06 号文档 §2.3/§4.1）。
		// 号池行仅在本体删除时软删，而本体 id 永不复用（identity 不复用），
		// 软删行永不与新行冲突——完整唯一索引与部分索引语义等价且实现最简。
		index.Fields("ontology_id", "purpose", "prefix").Unique().StorageKey("uix_ont_serials_key"),
	}
}
