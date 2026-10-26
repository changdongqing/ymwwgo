package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// Contract 合同（统一合同底座：一份合同从草稿到终结的全生命周期实例）。
//
// 与一站式工单同构（docs/connn/contract-type-and-contract-design.md §4）：
// 类型差异全部由「类型模板 + 差异字段快照」吃掉，不分外包/施工/租赁……多表。
// 快照原则：创建时从类型 effective-config（细类→大类回退）解析并落库运行期判定
// 字段（requires_approval/pay_method/diff_fields schema/类型编码名称）；生效时按
// 快照拆解费用。此后类型改模板不影响存量合同。纯参考文案（guidance/乙方称谓/
// 必备附件清单）实时读类型，不落快照。
//
// 版本链：变更 = 旧版关为 CHANGED + 复制开新版（version+1，source_contract_id
// 回链上一版）；contract_no 全链不变。所有动作落 contract_events 时间线。
// 租户业务数据：挂 TenantID mixin 与租户 Policy（类型字典是平台级）。
type Contract struct {
	ent.Schema
}

func (Contract) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "contract_contracts",
		},
		entsql.WithComments(true),
		schema.Comment("合同表（统一合同底座：生命周期状态机+类型快照+版本链+事件流）"),
	}
}

// moneyType 金额列统一口径：numeric(14,2)（单位元，2 位小数），
// 拆解计算在服务层以分（int64）为单位消除浮点误差。
func moneyType() map[string]string {
	return map[string]string{
		dialect.Postgres: "numeric(14,2)",
	}
}

// Fields of the Contract.
func (Contract) Fields() []ent.Field {
	return []ent.Field{
		// —— 标识与版本链 ——
		field.String("contract_no").
			Comment("合同号：CT- + 8 位全局序列，事务内生成；版本链全链不变").
			MaxLen(32).
			NotEmpty().
			Unique().
			Optional().
			Nillable(),

		field.Uint32("version").
			Comment("版本号（首版 1，变更开新版 +1）").
			Default(1).
			Optional().
			Nillable(),

		field.Uint32("source_contract_id").
			Comment("上一版合同ID（变更链回链，contract_contracts.id）").
			Optional().
			Nillable(),

		// —— 类型与快照 ——
		// 类型：必须为启用的叶子节点（无子级，大类/细类均可）；无 ent 外键
		//（字典引用完整性由服务层校验），与 work_order.service_type_id 同款
		field.Uint32("contract_type_id").
			Comment("合同类型ID（contract_types.id，创建时必须为启用叶子节点）").
			Optional().
			Nillable(),

		field.String("contract_type_code").
			Comment("合同类型编码快照（创建时落库，历史合同不受字典调整影响）").
			MaxLen(4).
			Optional().
			Nillable(),

		field.String("contract_type_name").
			Comment("合同类型名称快照（创建时落库）").
			MaxLen(64).
			Optional().
			Nillable(),

		// 审批开关快照：submit 走向（进待审核 or 直接生效）以开立时刻为准
		field.Bool("requires_approval").
			Comment("是否需审批快照（true=提交进待审核，false=提交即生效）").
			Optional().
			Nillable(),

		// —— 状态 ——
		// 枚举存储值与 proto 枚举值名保持一致（STATUS_DRAFT...），
		// 供 mapper.EnumTypeConverter 按名称直转
		field.Enum("status").
			Comment("状态 STATUS_DRAFT:草稿 STATUS_REVIEWING:待审核 STATUS_EFFECTIVE:执行中 STATUS_CHANGED:变更终止 STATUS_EXPIRED:过期终止 STATUS_TERMINATED:已终止 STATUS_INVALIDATED:已作废").
			NamedValues(
				"Draft", "STATUS_DRAFT",
				"Reviewing", "STATUS_REVIEWING",
				"Effective", "STATUS_EFFECTIVE",
				"Changed", "STATUS_CHANGED",
				"Expired", "STATUS_EXPIRED",
				"Terminated", "STATUS_TERMINATED",
				"Invalidated", "STATUS_INVALIDATED",
			).
			Optional().
			Nillable(),

		// —— 甲方（我方）信息 ——
		field.String("region").
			Comment("单位/机构（甲方主体，如 长治院区/长治西院区）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("dept").
			Comment("经办部门（如 后勤保障部/基建办公室/设备科）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("party_a_contact").
			Comment("甲方联系人").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("party_a_phone").
			Comment("甲方联系电话").
			MaxLen(32).
			Optional().
			Nillable(),

		// —— 乙方信息 ——
		field.String("party_b_name").
			Comment("乙方名称（自由输入；供应商档案联动未落地，预留）").
			MaxLen(128).
			NotEmpty().
			Optional().
			Nillable(),

		field.String("party_b_credit").
			Comment("乙方统一社会信用代码").
			MaxLen(32).
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（PARTY_B_ENTERPRISE...）
		field.Enum("party_b_kind").
			Comment("乙方类型 PARTY_B_ENTERPRISE:企业法人 PARTY_B_ORG:非法人组织 PARTY_B_NATURAL:自然人").
			NamedValues(
				"Enterprise", "PARTY_B_ENTERPRISE",
				"Org", "PARTY_B_ORG",
				"Natural", "PARTY_B_NATURAL",
			).
			Optional().
			Nillable(),

		field.String("party_b_legal").
			Comment("乙方法定代表人").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("party_b_contact").
			Comment("乙方联系人").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("party_b_phone").
			Comment("乙方联系电话").
			MaxLen(32).
			Optional().
			Nillable(),

		field.String("party_b_bank").
			Comment("乙方开户行").
			MaxLen(128).
			Optional().
			Nillable(),

		field.String("party_b_account").
			Comment("乙方银行账号").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("party_b_rating").
			Comment("乙方信用评级（A/B/C/D 级）").
			MaxLen(8).
			Optional().
			Nillable(),

		// —— 基本信息 ——
		// 枚举存储值与 proto 枚举值名保持一致（IMPORTANCE_NORMAL...）
		field.Enum("importance").
			Comment("合同重要性 IMPORTANCE_NORMAL:一般 IMPORTANCE_IMPORTANT:重要").
			NamedValues(
				"Normal", "IMPORTANCE_NORMAL",
				"Important", "IMPORTANCE_IMPORTANT",
			).
			Optional().
			Nillable(),

		field.Time("start_date").
			Comment("合同开始日期（日期语义，零点存储）").
			Optional().
			Nillable(),

		field.Time("end_date").
			Comment("合同结束日期（日期语义；到期扫描依据：早于当日零点自动过期）").
			Optional().
			Nillable(),

		field.Float("amount").
			Comment("合同金额（元，2位小数；拆解按付款方式分解到费用明细）").
			SchemaType(moneyType()).
			Optional().
			Nillable(),

		field.Uint32("tax_rate").
			Comment("税率（%，0/1/3/5/6/9/13）").
			Optional().
			Nillable(),

		// 付款方式快照：类型模板默认值，创建时可改；拆解依据
		field.Enum("pay_method").
			Comment("付款方式快照（类型默认可覆盖） PAY_ONCE:一次性 PAY_MONTHLY:按月 PAY_QUARTERLY:按季 PAY_NODE:按节点 PAY_PROGRESS:按进度").
			NamedValues(
				"Once", "PAY_ONCE",
				"Monthly", "PAY_MONTHLY",
				"Quarterly", "PAY_QUARTERLY",
				"Node", "PAY_NODE",
				"Progress", "PAY_PROGRESS",
			).
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（DIRECTION_PAYABLE...）
		field.Enum("fee_direction").
			Comment("费用方向 DIRECTION_PAYABLE:应付（我方付款） DIRECTION_RECEIVABLE:应收（我方收款）").
			NamedValues(
				"Payable", "DIRECTION_PAYABLE",
				"Receivable", "DIRECTION_RECEIVABLE",
			).
			Optional().
			Nillable(),

		field.Float("deposit").
			Comment("履约保证金金额（元；类型 requires_deposit 时拆解追加押金费用行）").
			SchemaType(moneyType()).
			Optional().
			Nillable(),

		// —— 差异字段（schema 快照定型，values 用户填写）——
		field.JSON("diff_schema", []ContractDiffField{}).
			Comment("差异字段定义快照 [{key,label,widget,options}]（创建时从类型解析落库，表单按此定型）").
			Optional(),

		field.JSON("diff_values", map[string]string{}).
			Comment("差异字段填值 {key:value}（按 diff_schema 键值对）").
			Optional(),

		// 付款节点快照：按节点/进度付款的拆解比例（PAY_NODE/PAY_PROGRESS 用，
		// 创建时从类型解析落库；拆解以本快照为准，类型后续改模板不影响存量合同）
		field.JSON("pay_nodes", []ContractPayNode{}).
			Comment("付款节点模板快照 [{name,ratioPercent}]（按节点/进度付款的拆解比例，拆解以快照为准）").
			Optional(),

		// —— 附件 ——
		field.Strings("attachments").
			Comment("附件URL列表（上传设施落 URL；必备清单由类型 required_attachments 提示）").
			Optional(),

		// —— 审核与关闭 ——
		field.Uint32("reviewer_id").
			Comment("审核人用户ID（通过/驳回动作人）").
			Optional().
			Nillable(),

		field.Time("reviewed_at").
			Comment("审核时间（通过或驳回）").
			Optional().
			Nillable(),

		field.String("review_note").
			Comment("审核意见（驳回原因）").
			MaxLen(255).
			Optional().
			Nillable(),

		field.String("terminate_reason").
			Comment("终止/作废原因（terminate/invalidate 必填；变更关闭自动生成「变更至 vN」）").
			MaxLen(255).
			Optional().
			Nillable(),

		field.String("remark").
			Comment("备注").
			MaxLen(1024).
			Optional().
			Nillable(),
	}
}

// Mixin of the Contract.
// 租户业务数据：挂 TenantID 与其自带的租户 Privacy（查询/写入按 viewer 隔离，
// 平台/系统上下文放行）。无启停概念故不挂 IsEnabled。
func (Contract) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the Contract.
func (Contract) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "status").
			StorageKey("contract_tenant_status"),
		index.Fields("contract_type_id"),
		index.Fields("end_date"),
		index.Fields("source_contract_id"),
		index.Fields("created_at"),
	}
}
