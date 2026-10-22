package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-crud/entgo/mixin"
)

// ServiceType 一站式服务类型（四级分类字典：大类 → 中类 → 小类 → 细类）。
//
// 编码规则见 docs/leixing/biaodan/README.md：code 为 2/4/6/8 位数字字符串，
// 位数即层级，上级编码 = 本编码去掉末两位；层级与编码的一致性由服务层校验。
type ServiceType struct {
	ent.Schema
}

func (ServiceType) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "onestop_service_types",
		},
		entsql.WithComments(true),
		schema.Comment("一站式服务类型表（四级分类字典：大/中/小/细，编码位数即层级）"),
	}
}

// Fields of the ServiceType.
func (ServiceType) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").
			Comment("服务编码：2/4/6/8 位数字字符串，位数即层级").
			MaxLen(8).
			NotEmpty().
			Unique().
			Optional().
			Nillable(),

		field.String("name").
			Comment("服务名称").
			MaxLen(128).
			NotEmpty().
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（LEVEL_MAJOR...），
		// 供 mapper.EnumTypeConverter 按名称直转
		field.Enum("level").
			Comment("服务层级 LEVEL_MAJOR:大类 LEVEL_MIDDLE:中类 LEVEL_MINOR:小类 LEVEL_DETAIL:细类").
			NamedValues(
				"Major", "LEVEL_MAJOR",
				"Middle", "LEVEL_MIDDLE",
				"Minor", "LEVEL_MINOR",
				"Detail", "LEVEL_DETAIL",
			).
			Optional().
			Nillable(),

		field.String("description").
			Comment("描述").
			MaxLen(512).
			Default("").
			Optional().
			Nillable(),
	}
}

// Mixin of the ServiceType.
// 全局字典表：不挂 TenantID、不配租户 Policy（参照 sys_languages / sys_configs）。
func (ServiceType) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.Tree[ServiceType]{},
		mixin.IsEnabled{},
		mixin.SortOrder{},
	}
}

// Indexes of the ServiceType.
func (ServiceType) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("parent_id"),
		index.Fields("level"),
	}
}
