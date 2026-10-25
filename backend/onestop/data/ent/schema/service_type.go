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

// ServiceAcceptanceItem 验收项（jsonb 存储，service_type.acceptance_items 的元素）。
//
// 报修场景 = 完工验收项（故障已修复/现场已清理/旧件处置确认），
// 运送场景 = 交接签收项（数量准确/包装完好/温度达标/单据回收）；
// required=false 的提示项在确认页可跳过。
type ServiceAcceptanceItem struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
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

		// —— 统一履约配置模板（所有小类/细类两级可填，全部可空，未配置 = NULL）——
		// 四组：派单与时效 / 作业标准 / 验收方案 / 专项属性。存储层不限子树，
		// 可见性与文案由前端展示矩阵（CATEGORY_PROFILE）按中类驱动；消费方为
		// 后续工单对象（派单/SLA 预警/绩效工时核算）与发起端（快捷语句/语义归型）。
		// 列 COMMENT 是迁移 SQL 的逐字契约（漂移守卫比对注释文本），改文案须同步迁移。
		field.String("execution_post").
			Comment("执行岗位（派单目标岗位，如：电工/运送员/保洁员）").
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

		// 履约形态：工单对象的「单子怎么履约」开关（与 trigger_mode 正交——后者管
		// 怎么生单）。NULL = 未设置：细类回退小类；两级都空按推导规则给默认值
		//（0106 子树→运送；解析后 SCHEDULED→计划任务；否则→事件处置），
		// 显式设置只用于处理推导覆盖不了的特例（如 010405 被服收发按业务形状归运送）。
		// 枚举存储值与 proto 枚举值名保持一致（KIND_EVENT...），
		// 供 mapper.EnumTypeConverter 按名称直转
		field.Enum("order_kind").
			Comment("履约形态 KIND_EVENT:事件处置 KIND_TASK:计划任务 KIND_TRANSPORT:运送（未设置回退上级；两级都空按 0106 子树/计划生成/事件顺序推导）").
			NamedValues(
				"Event", "KIND_EVENT",
				"Task", "KIND_TASK",
				"Transport", "KIND_TRANSPORT",
			).
			Optional().
			Nillable(),

		// jsonb 数组列，语义与 sys_org_units.subtype_path 一致；
		// 不加 Nillable：空/NULL = 未配置
		field.Strings("common_phrases").
			Comment("常见语句清单：发起端快捷语句，兼作语义归型语料").
			Optional(),

		field.Strings("standard_tools").
			Comment("服务标准工具清单：出工标配工具").
			Optional(),

		field.Strings("spare_parts").
			Comment("常用备件/耗材清单：出工常备，配合库存领用").
			Optional(),

		// 任务触发方式：控制周期计划入口的统一开关。按需发起 = 工单由用户/呼叫
		// 触发；计划生成 = 可挂周期计划（onestop_periodic_plans），任务由调度器
		// 按频率生成。NULL = 未设置：细类回退小类，小类也未设置按 TRIGGER_ON_DEMAND。
		// 枚举存储值与 proto 枚举值名保持一致（TRIGGER_ON_DEMAND...），
		// 供 mapper.EnumTypeConverter 按名称直转
		field.Enum("trigger_mode").
			Comment("任务触发方式 TRIGGER_ON_DEMAND:按需发起 TRIGGER_SCHEDULED:计划生成（未设置回退上级，默认按需）").
			NamedValues(
				"OnDemand", "TRIGGER_ON_DEMAND",
				"Scheduled", "TRIGGER_SCHEDULED",
			).
			Optional().
			Nillable(),

		// —— 验收方案（通用履约）——
		// 报修场景 = 完工验收，运送场景 = 交接签收，客服场景 = 办结回访；可组合。
		field.Enum("default_priority").
			Comment("默认优先级 PRIORITY_NORMAL:常规 PRIORITY_URGENT:加急 PRIORITY_EMERGENCY:特急（派单排序与超时预警加权）").
			NamedValues(
				"Normal", "PRIORITY_NORMAL",
				"Urgent", "PRIORITY_URGENT",
				"Emergency", "PRIORITY_EMERGENCY",
			).
			Optional().
			Nillable(),

		field.Strings("acceptance_modes").
			Comment("验收方式（多选 ACCEPT_SIGN:签名确认 ACCEPT_SCAN:扫码核对 ACCEPT_PHOTO:拍照留证 ACCEPT_DUAL:双人验收）").
			Optional(),

		field.JSON("acceptance_items", []ServiceAcceptanceItem{}).
			Comment("验收项清单 [{name,required}]：完工/送达确认页逐项核对").
			Optional(),

		// —— 专项属性（推荐填写子树见展示矩阵；存储层不限子树）——
		// 温控/审批/物品细目主要为运送配送语义；审批同时覆盖动火作业、资源申请等。
		field.Enum("temperature_requirement").
			Comment("温控要求 TEMP_NORMAL:常温 TEMP_COLD:冷藏2-8℃ TEMP_FROZEN:冷冻 TEMP_WARM:保温").
			NamedValues(
				"Normal", "TEMP_NORMAL",
				"Cold", "TEMP_COLD",
				"Frozen", "TEMP_FROZEN",
				"Warm", "TEMP_WARM",
			).
			Optional().
			Nillable(),

		field.Bool("requires_approval").
			Comment("是否需事前审批（搬入搬出押运、动火作业、资源申请等管控类服务）").
			Optional().
			Default(false),

		field.Strings("cargo_types").
			Comment("运送物品细目清单：该类型承接的物品范围（推荐 0106 运送配送子树填写）").
			Optional(),

		field.Strings("standard_items").
			Comment("标准作业项目库：周期计划点表模板").
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
