package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// WorkOrderEvent 工单事件（时间线）：所有动作的不可变记录，按时间倒序即详情页
// 时间线，也是「跟进流程」的数据载体。主表只存当前态与关键时间戳，动作明细
//（改派目标、签收人、催办附言、转单去向等）由 note/photos 承载；不改状态的动作
//（催办/异常上报/派生）status_from == status_to。只增不改不删：写错不修，
// 补一条更正事件（本期无此需求，仅原则）。
type WorkOrderEvent struct {
	ent.Schema
}

func (WorkOrderEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "onestop_work_order_events",
		},
		entsql.WithComments(true),
		schema.Comment("一站式工单事件表（时间线：派单/到场/取件/送达/完工/验收/催办等动作记录）"),
	}
}

// Fields of the WorkOrderEvent.
func (WorkOrderEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("work_order_id").
			Comment("工单ID（onestop_work_orders.id）").
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（ACTION_CREATE...），
		// 供 mapper.EnumTypeConverter 按名称直转。
		// ACTION_CLAIM 为移动端接单预留枚举位，本期不实现。
		field.Enum("action").
			Comment("动作 ACTION_CREATE:创建 ACTION_DISPATCH:派单 ACTION_REASSIGN:改派 ACTION_ARRIVE:到场 ACTION_PICKUP:取件签收 ACTION_DELIVER:送达签收 ACTION_COMPLETE:完工提交 ACTION_VERIFY_PASS:验收通过 ACTION_VERIFY_REJECT:验收驳回 ACTION_CANCEL:撤销 ACTION_URGE:催办 ACTION_ABNORMAL:异常上报 ACTION_TRANSFER:转单 ACTION_DERIVE:派生 ACTION_APPROVE_PASS:审批通过 ACTION_APPROVE_REJECT:审批驳回 ACTION_CLAIM:接单(预留) ACTION_SATISFACTION:满意度回访补录").
			NamedValues(
				"Create", "ACTION_CREATE",
				"Dispatch", "ACTION_DISPATCH",
				"Reassign", "ACTION_REASSIGN",
				"Arrive", "ACTION_ARRIVE",
				"Pickup", "ACTION_PICKUP",
				"Deliver", "ACTION_DELIVER",
				"Complete", "ACTION_COMPLETE",
				"VerifyPass", "ACTION_VERIFY_PASS",
				"VerifyReject", "ACTION_VERIFY_REJECT",
				"Cancel", "ACTION_CANCEL",
				"Urge", "ACTION_URGE",
				"Abnormal", "ACTION_ABNORMAL",
				"Transfer", "ACTION_TRANSFER",
				"Derive", "ACTION_DERIVE",
				"ApprovePass", "ACTION_APPROVE_PASS",
				"ApproveReject", "ACTION_APPROVE_REJECT",
				"Claim", "ACTION_CLAIM",
				"Satisfaction", "ACTION_SATISFACTION",
			).
			Optional().
			Nillable(),

		field.Enum("status_from").
			Comment("动作前状态（不改状态的动作与动作后相同或为空）").
			NamedValues(
				"Pending", "STATUS_PENDING",
				"Approving", "STATUS_APPROVING",
				"Dispatched", "STATUS_DISPATCHED",
				"Processing", "STATUS_PROCESSING",
				"Completed", "STATUS_COMPLETED",
				"Closed", "STATUS_CLOSED",
				"Cancelled", "STATUS_CANCELLED",
			).
			Optional().
			Nillable(),

		field.Enum("status_to").
			Comment("动作后状态").
			NamedValues(
				"Pending", "STATUS_PENDING",
				"Approving", "STATUS_APPROVING",
				"Dispatched", "STATUS_DISPATCHED",
				"Processing", "STATUS_PROCESSING",
				"Completed", "STATUS_COMPLETED",
				"Closed", "STATUS_CLOSED",
				"Cancelled", "STATUS_CANCELLED",
			).
			Optional().
			Nillable(),

		field.String("note").
			Comment("动作附言（改派目标/签收人/转单去向等摘要在附言承载）").
			MaxLen(512).
			Optional().
			Nillable(),

		field.Strings("photos").
			Comment("动作留证照片").
			Optional(),
	}
}

// Mixin of the WorkOrderEvent.
// 随主表挂租户；操作人由 OperatorID 的 created_by 兼任（系统动作为系统上下文）。
func (WorkOrderEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the WorkOrderEvent.
func (WorkOrderEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("work_order_id"),
	}
}
