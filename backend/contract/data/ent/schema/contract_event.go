package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// ContractEvent 合同事件（时间线上的一次动作记录，追加不变 append-only）。
//
// 主表只存当前态与关键时间戳，全部过程（提交/审核/变更/终止/作废/到期/拆解/
// 收付退款）在这里留痕；详情页时间线与后续审计都读这张表。原则与
// onestop_work_order_events 相同：已落事件不修改不删除。
type ContractEvent struct {
	ent.Schema
}

func (ContractEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "contract_events",
		},
		entsql.WithComments(true),
		schema.Comment("合同事件表（追加不变的时间线：提交/审核/变更/终止/收付退款等动作留痕）"),
	}
}

// Fields of the ContractEvent.
func (ContractEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("contract_id").
			Comment("所属合同ID（contract_contracts.id）").
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（ACTION_CREATE...）
		field.Enum("action").
			Comment("动作 ACTION_CREATE:创建 ACTION_UPDATE:修改 ACTION_SUBMIT:提交 ACTION_APPROVE:审核通过 ACTION_REJECT:审核驳回 ACTION_CHANGE:变更 ACTION_TERMINATE:终止 ACTION_INVALIDATE:作废 ACTION_EXPIRE:到期关闭 ACTION_DECOMPOSE:重新拆解 ACTION_FEE_PAY:费用付款 ACTION_FEE_RECEIVE:费用收款 ACTION_FEE_REFUND:押金退款").
			NamedValues(
				"Create", "ACTION_CREATE",
				"Update", "ACTION_UPDATE",
				"Submit", "ACTION_SUBMIT",
				"Approve", "ACTION_APPROVE",
				"Reject", "ACTION_REJECT",
				"Change", "ACTION_CHANGE",
				"Terminate", "ACTION_TERMINATE",
				"Invalidate", "ACTION_INVALIDATE",
				"Expire", "ACTION_EXPIRE",
				"Decompose", "ACTION_DECOMPOSE",
				"FeePay", "ACTION_FEE_PAY",
				"FeeReceive", "ACTION_FEE_RECEIVE",
				"FeeRefund", "ACTION_FEE_REFUND",
			).
			Optional().
			Nillable(),

		field.String("status_from").
			Comment("转移前状态（创建为空）").
			MaxLen(32).
			Optional().
			Nillable(),

		field.String("status_to").
			Comment("转移后状态（非状态动作为空）").
			MaxLen(32).
			Optional().
			Nillable(),

		field.String("note").
			Comment("动作备注（驳回原因/变更说明/终止原因等）").
			MaxLen(512).
			Optional().
			Nillable(),
	}
}

// Mixin of the ContractEvent.
// 租户业务数据：挂 TenantID（随合同租户），追加不变。
func (ContractEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the ContractEvent.
func (ContractEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("contract_id"),
	}
}
