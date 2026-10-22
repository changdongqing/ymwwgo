package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// OntAxiom holds the schema definition for the OntAxiom entity.
//
// OWL 公理行化存储，六项本体建模功能共用的事实源（设计文档 §3.1.2）。
// 层级不做 parent_id 冗余——全部由 SUBCLASS_OF(20) 公理派生（设计文档 §2.2 决策二）；
// 不相交/等价成对存储（(A,B) 与 (B,A) 各一行），usage 反查与差异计算均为 O(1) 行操作。
type OntAxiom struct {
	ent.Schema
}

func (OntAxiom) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "ont_axioms",
		},
		entsql.WithComments(true),
		schema.Comment("OWL 公理行化存储表（六项本体建模功能共用；层级/不相交/等价/注解断言均落此处）"),
	}
}

// Mixin of the OntAxiom.
func (OntAxiom) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.SortOrder{}, // 多父类展示顺序（SUBCLASS_OF 多条按提交顺序保序）
	}
}

// Fields of the OntAxiom.
func (OntAxiom) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("ontology_id").
			Comment("所属本体 id").
			Optional().
			Nillable(),

		// 公理类型用号段数字（不用 field.Enum），理由同 OntEntity.entity_type
		field.Uint32("axiom_type").
			Comment("公理类型号段，见 OntologyAxiomType（10 声明/20 子类/21 不相交/22 等价/100 注解断言；30~62 预留 02~06 期）").
			Optional().
			Nillable(),

		field.Uint32("subject_id").
			Comment("主实体 id（ont_entities.id）").
			Optional().
			Nillable(),

		field.Uint32("object_id").
			Comment("对象实体 id（层级/不相交/等价另一端；04 期自定义注解谓词时为注解属性实体）").
			Optional().
			Nillable(),

		field.String("predicate_iri").
			Comment("注解断言谓词 IRI（rdfs:label/rdfs:comment/owl:deprecated/skos:notation）").
			MaxLen(512).
			Optional().
			Nillable(),

		// 注解值 / 字面量（02 期起复用存属性链 JSON、内建值域 IRI 等）
		field.Text("value_text").
			Comment("注解值 / 字面量（02 期起复用存属性链 JSON、内建值域 IRI 等）").
			Optional().
			Nillable(),

		field.String("lang").
			Comment("语言标签（多语言 label 预留，BCP47 简化校验 ≤16 字符）").
			MaxLen(16).
			Optional().
			Nillable(),
	}
}

// Indexes of the OntAxiom.
func (OntAxiom) Indexes() []ent.Index {
	return []ent.Index{
		// 主语侧查询路径：按实体取全部公理（frame/渲染缓存/删除级联）
		index.Fields("ontology_id", "subject_id", "axiom_type").
			StorageKey("idx_ont_axioms_subject"),

		// 宾语侧查询路径：usage 反查（谁引用了我）
		index.Fields("ontology_id", "object_id").
			StorageKey("idx_ont_axioms_object"),

		// 06 期个体赋码需要 (ontology_id, axiom_type, value_text text_pattern_ops) 前缀检索索引，
		// 届时随 000NN_ontology_serial.sql 一并新增，本期不建。
	}
}
