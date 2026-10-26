package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// ContractFee 合同费用明细（合同金额按付款方式拆解出的应收/应付/押金行）。
//
// 生成时机：合同生效（Approve，或 requires_approval=false 的 Submit）由拆解器
// 自动生成（auto_generated=true）；「重新拆解」删自动行重生成，手工行保留。
// 状态机：PENDING（待付款/待收取）→ DONE（已付款/已收取）；押金行 DONE 语义为
// 已收/已付（待退款），可再 Refund → REFUNDED（已退款）——普通行 DONE 即终态。
type ContractFee struct {
	ent.Schema
}

func (ContractFee) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "contract_fees",
		},
		entsql.WithComments(true),
		schema.Comment("合同费用明细表（拆解生成的应收/应付/押金行，收付退款状态机）"),
	}
}

// Fields of the ContractFee.
func (ContractFee) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("contract_id").
			Comment("所属合同ID（contract_contracts.id）").
			Optional().
			Nillable(),

		field.String("name").
			Comment("费用名称（合同款/首期/基础节点/履约保证金…）").
			MaxLen(64).
			NotEmpty().
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（CATEGORY_INCOME...）
		field.Enum("category").
			Comment("费用类别 CATEGORY_INCOME:收入 CATEGORY_EXPENSE:费用 CATEGORY_DEPOSIT:押金").
			NamedValues(
				"Income", "CATEGORY_INCOME",
				"Expense", "CATEGORY_EXPENSE",
				"Deposit", "CATEGORY_DEPOSIT",
			).
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（DIRECTION_PAYABLE...）
		field.Enum("direction").
			Comment("费用方向 DIRECTION_PAYABLE:应付（我方付款） DIRECTION_RECEIVABLE:应收（我方收款）").
			NamedValues(
				"Payable", "DIRECTION_PAYABLE",
				"Receivable", "DIRECTION_RECEIVABLE",
			).
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（FEE_STATUS_PENDING...）
		field.Enum("status").
			Comment("状态 FEE_STATUS_PENDING:待收付 FEE_STATUS_DONE:已收付（押金=待退款） FEE_STATUS_REFUNDED:已退款（押金终态）").
			NamedValues(
				"Pending", "FEE_STATUS_PENDING",
				"DONE", "FEE_STATUS_DONE",
				"Refunded", "FEE_STATUS_REFUNDED",
			).
			Optional().
			Nillable(),

		field.Float("amount").
			Comment("金额（元，2位小数；拆解等分与余数在服务层按分计算）").
			SchemaType(moneyType()).
			Optional().
			Nillable(),

		field.Time("plan_date").
			Comment("计划收付日期（节点付款行空，履约时补）").
			Optional().
			Nillable(),

		field.Time("actual_date").
			Comment("实际收付日期（付款/收款动作时间）").
			Optional().
			Nillable(),

		field.Bool("auto_generated").
			Comment("是否拆解自动生成（重新拆解只删自动行，手工行保留）").
			Optional().
			Default(false),

		field.String("remark").
			Comment("备注").
			MaxLen(255).
			Optional().
			Nillable(),
	}
}

// Mixin of the ContractFee.
// 租户业务数据：挂 TenantID（随合同租户），无启停概念。
func (ContractFee) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the ContractFee.
func (ContractFee) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("contract_id"),
		index.Fields("tenant_id", "status").
			StorageKey("contract_fee_tenant_status"),
		index.Fields("plan_date"),
	}
}
