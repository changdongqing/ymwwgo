package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-crud/entgo/mixin"
)

// ContractType 合同类型（二级分类字典：大类 → 细类）。
//
// 设计口径见 docs/connn/contract-type-and-contract-design.md §3：code 为 2/4 位
// 数字字符串，位数即层级，细类前 2 位 = 父大类编码；层级与编码一致性由服务层
// 校验。统一合同配置模板两级均可填，细类空字段回退大类（effective-config）。
type ContractType struct {
	ent.Schema
}

// ContractPayNode 付款节点模板（jsonb 存储，contract_types.pay_nodes 的元素）。
// PAY_NODE/PAY_PROGRESS 付款方式的自动拆解比例来源，ratioPercent 之和以 100 为准
//（≠100 时拆解器按余数进末节点容错）。
type ContractPayNode struct {
	Name         string `json:"name"`
	RatioPercent int32  `json:"ratioPercent"`
}

// ContractDiffField 差异化字段定义（jsonb 存储，contract_types.diff_fields 的元素）。
// 类型驱动的动态表单 schema：合同创建表单按此渲染差异信息区，diff_values 按填值。
type ContractDiffField struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Widget  string   `json:"widget"` // WIDGET_TEXT/WIDGET_TEXTAREA/WIDGET_SELECT/WIDGET_DATE
	Options []string `json:"options,omitempty"`
}

func (ContractType) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "contract_types",
		},
		entsql.WithComments(true),
		schema.Comment("合同类型表（二级分类字典：大类/细类，编码位数即层级，统一合同配置模板）"),
	}
}

// Fields of the ContractType.
func (ContractType) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").
			Comment("类型编码：2/4 位数字字符串，位数即层级，细类前2位=父大类").
			MaxLen(4).
			NotEmpty().
			Unique().
			Optional().
			Nillable(),

		field.String("name").
			Comment("类型名称").
			MaxLen(64).
			NotEmpty().
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（LEVEL_MAJOR...），
		// 供 mapper.EnumTypeConverter 按名称直转
		field.Enum("level").
			Comment("类型层级 LEVEL_MAJOR:大类 LEVEL_DETAIL:细类").
			NamedValues(
				"Major", "LEVEL_MAJOR",
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

		// —— 统一合同配置模板（两级均可填，全部可空，未配置 = NULL；细类空字段回退大类）——
		// 消费方为合同创建（快照运行期判定字段）与费用拆解器。
		// 列 COMMENT 是迁移 SQL 的逐字契约（漂移守卫比对注释文本），改文案须同步迁移。
		field.String("party_b_label").
			Comment("乙方称谓模板（供应商/施工单位/服务单位/维保单位/三方公司等，按类型展示）").
			MaxLen(32).
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（PAY_ONCE...），
		// 供 mapper.EnumTypeConverter 按名称直转
		field.Enum("pay_method").
			Comment("默认付款方式 PAY_ONCE:一次性 PAY_MONTHLY:按月 PAY_QUARTERLY:按季 PAY_NODE:按节点 PAY_PROGRESS:按进度").
			NamedValues(
				"Once", "PAY_ONCE",
				"Monthly", "PAY_MONTHLY",
				"Quarterly", "PAY_QUARTERLY",
				"Node", "PAY_NODE",
				"Progress", "PAY_PROGRESS",
			).
			Optional().
			Nillable(),

		field.JSON("pay_nodes", []ContractPayNode{}).
			Comment("付款节点模板 [{name,ratioPercent}]：按节点/进度付款的拆解比例（如 基础30/主体40/竣工25/质保5）").
			Optional(),

		// 枚举存储值与 proto 枚举值名保持一致（DIRECTION_PAYABLE...）
		field.Enum("fee_direction").
			Comment("默认费用方向 DIRECTION_PAYABLE:应付（我方付款） DIRECTION_RECEIVABLE:应收（我方收款，如出租）").
			NamedValues(
				"Payable", "DIRECTION_PAYABLE",
				"Receivable", "DIRECTION_RECEIVABLE",
			).
			Optional().
			Nillable(),

		field.Bool("requires_approval").
			Comment("是否需审批（true=提交后进待审核，false=提交即生效；默认 true）").
			Optional().
			Nillable(),

		field.Bool("requires_deposit").
			Comment("是否需履约保证金（true=拆解时追加押金费用行）").
			Optional().
			Nillable(),

		field.JSON("diff_fields", []ContractDiffField{}).
			Comment("差异字段定义 [{key,label,widget,options}]：类型驱动的动态表单 schema，创建时快照").
			Optional(),

		field.Strings("required_attachments").
			Comment("必备附件名清单（提示性校验，如 中标通知书/施工图/合同正本）").
			Optional(),

		field.String("guidance").
			Comment("合同录入提示（纯参考文案，实时读类型，不快照）").
			MaxLen(2048).
			Optional().
			Nillable(),
	}
}

// Mixin of the ContractType.
// 平台级全局字典表：不挂 TenantID、不配租户 Policy（与 onestop_service_types 同口径）。
func (ContractType) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.Tree[ContractType]{},
		mixin.IsEnabled{},
	}
}

// Indexes of the ContractType.
func (ContractType) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("parent_id"),
		index.Fields("level"),
	}
}
