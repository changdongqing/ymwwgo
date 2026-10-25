package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// WorkOrderItem 工单执行项（计划任务形态的逐项作业记录）。
//
// 创建时快照生成：计划展开单 = 计划 space_items（scope=空间）+ equipment_items
//（scope=设备）中 enabled 的项；手动创建的计划任务单 = 类别 standard_items 全部项
//（scope 为空）。执行中逐项回填结果/读数/照片；存在未执行项时不可完工提交
//（要么逐项给结果、要么显式跳过——不允许漏项完工）。结果为异常时联动主表
// has_abnormality，并可一键派生报修单。
type WorkOrderItem struct {
	ent.Schema
}

func (WorkOrderItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "onestop_work_order_items",
		},
		entsql.WithComments(true),
		schema.Comment("一站式工单执行项表（计划任务形态的逐项作业记录：点表快照+结果回填）"),
	}
}

// Fields of the WorkOrderItem.
func (WorkOrderItem) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("work_order_id").
			Comment("工单ID（onestop_work_orders.id，随工单同事务创建/清理）").
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（SCOPE_SPACE...），
		// 供 mapper.EnumTypeConverter 按名称直转
		field.Enum("scope").
			Comment("点表分组 SCOPE_SPACE:空间点表 SCOPE_EQUIPMENT:设备点表（手动创建的计划任务单 scope 为空）").
			NamedValues(
				"Space", "SCOPE_SPACE",
				"Equipment", "SCOPE_EQUIPMENT",
			).
			Optional().
			Nillable(),

		field.String("name").
			Comment("执行项名称（点表快照）").
			MaxLen(128).
			NotEmpty().
			Optional().
			Nillable(),

		field.Uint32("sort").
			Comment("展开顺序（按点表顺序落库）").
			Optional().
			Default(0),

		field.Enum("result").
			Comment("执行结果 ITEM_PENDING:未执行 ITEM_NORMAL:正常 ITEM_ABNORMAL:异常 ITEM_SKIPPED:跳过（须显式）").
			NamedValues(
				"Pending", "ITEM_PENDING",
				"Normal", "ITEM_NORMAL",
				"Abnormal", "ITEM_ABNORMAL",
				"Skipped", "ITEM_SKIPPED",
			).
			Optional().
			Default("ITEM_PENDING"),

		field.String("value").
			Comment("读数/测量值（抄表、检测类）").
			MaxLen(128).
			Optional().
			Nillable(),

		field.Strings("photos").
			Comment("该项留证照片").
			Optional(),

		field.String("remark").
			Comment("该项备注").
			MaxLen(512).
			Optional().
			Nillable(),

		// —— 预留：空间/设备对象模块落地后启用（当前无业务表，仅保留契约）——
		field.String("space_ref").
			Comment("空间对象引用（预留）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("equipment_ref").
			Comment("设备对象引用（预留）").
			MaxLen(64).
			Optional().
			Nillable(),
	}
}

// Mixin of the WorkOrderItem.
// 随主表挂租户（同租户隔离）；无启停概念。
func (WorkOrderItem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the WorkOrderItem.
func (WorkOrderItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_order_id"),
	}
}
