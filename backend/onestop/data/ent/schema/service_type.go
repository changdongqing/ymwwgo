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

		// —— 报修维修类型配置（本期仅 0101 报修维修子树的小类/细类填写，其余类型留空）——
		// 列 COMMENT 是迁移 SQL 的逐字契约（漂移守卫比对注释文本），改文案须同步 00009 迁移。
		field.String("repair_post").
			Comment("维修岗位（派单目标岗位，如：电工/管工/空调工）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.Float("standard_work_hours").
			Comment("标准工时（小时）：单次作业标准人时，绩效与负载核算用").
			Optional().
			Nillable(),

		field.Uint32("response_time_limit").
			Comment("响应时限（分钟）：受理到到场的 SLA").
			Optional().
			Nillable(),

		field.Uint32("completion_time_limit").
			Comment("完成时限（分钟）：受理到完工的 SLA").
			Optional().
			Nillable(),

		field.String("guidance").
			Comment("标准指导说明：作业 SOP 与安全注意事项").
			MaxLen(2048).
			Optional().
			Nillable(),

		// jsonb 数组列，语义与 sys_org_units.subtype_path 一致；
		// 不加 Nillable：空/NULL = 未配置
		field.Strings("common_phrases").
			Comment("常见报修语句清单：报修端快捷语句，兼作语义归型语料").
			Optional(),

		field.Strings("standard_tools").
			Comment("服务标准工具清单：出工标配工具").
			Optional(),

		field.Strings("spare_parts").
			Comment("常用备件/耗材清单：出工常备，配合库存领用").
			Optional(),
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
		// 不挂 mixin.SortOrder：类型列表固定按 code 升序（编码本身即业务序），
		// 独立排序字段会制造第二套顺序来源。
	}
}

// Indexes of the ServiceType.
func (ServiceType) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("parent_id"),
		index.Fields("level"),
	}
}
