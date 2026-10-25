package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// PeriodicPlanItem 计划点表项（jsonb 存储，space_items / equipment_items 的元素）。
//
// 计划点表 = 从类别「标准作业项目库」挑选 + 计划内自定义的项目集合，
// 每项可独立停用/启用（停用项保留在点表里但不生成执行项）。
type PeriodicPlanItem struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// PeriodicPlan 一站式周期计划（任务触发方式为「计划生成」的类别节点下的计划配置）。
//
// 由巡检计划（0103）泛化而来：一个类别可挂多套周期不同的计划；计划描述
// 「谁（岗位）按什么频率在什么范围内做哪些项（点表）」，生效区间约束执行窗口。
// 运行期任务生成由后续工单对象实现，本期只做配置管理。
//
// 挂载门控：类别（小类/细类）解析后的任务触发方式须为 TRIGGER_SCHEDULED
// （细类未设置回退小类），由服务层校验。
//
// 频率模型：freq_type 选周期，各周期填各自的执行日字段，times 是执行日内的
// 时间点列表（每日多次 = 多个时间点）；excluded_dates 命中的日期不生成任务。
// month_days 支持 -1（月末）；clamp_to_month_end=true 时号数超出当月天数
// （如 2 月的 30/31 号）顺延至当月最后一天，false 则当月跳过。
// 「季度」以 FREQ_YEARLY 的 months 多值表达（如 [1,4,7,10]），
// 「半月」以 FREQ_MONTHLY 的 month_days=[1,15] 表达。
type PeriodicPlan struct {
	ent.Schema
}

func (PeriodicPlan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "onestop_periodic_plans",
		},
		entsql.WithComments(true),
		schema.Comment("一站式周期计划表（计划生成类类别的计划配置：频率/岗位/点表/生效期）"),
	}
}

// Fields of the PeriodicPlan.
func (PeriodicPlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Comment("计划名称").
			MaxLen(128).
			NotEmpty().
			Optional().
			Nillable(),

		// 类别节点（触发模式为计划生成的小类/细类）；不加 ent Edge/FK（类别是
		// 全局字典，引用完整性由服务层校验），org_unit.leader_id 同款
		field.Uint32("service_type_id").
			Comment("计划类别ID（onestop_service_types.id，触发模式为计划生成的小类/细类）").
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（FREQ_DAILY...），
		// 供 mapper.EnumTypeConverter 按名称直转
		field.Enum("freq_type").
			Comment("周期类型 FREQ_DAILY:每日 FREQ_WEEKLY:每周 FREQ_MONTHLY:每月 FREQ_YEARLY:每年").
			NamedValues(
				"Daily", "FREQ_DAILY",
				"Weekly", "FREQ_WEEKLY",
				"Monthly", "FREQ_MONTHLY",
				"Yearly", "FREQ_YEARLY",
			).
			Optional().
			Nillable(),

		field.Strings("times").
			Comment("执行时间点列表（HH:mm），每个执行日按列表各执行一次").
			Optional(),

		field.JSON("weekdays", []int32{}).
			Comment("周几执行（1=周一…7=周日），每周计划使用").
			Optional(),

		field.JSON("month_days", []int32{}).
			Comment("几号执行（1-31；-1=当月最后一天），每月/每年计划使用").
			Optional(),

		field.Bool("clamp_to_month_end").
			Comment("月底日期兼容：号数超出当月天数时顺延至当月最后一天").
			Optional().
			Default(true),

		field.JSON("months", []int32{}).
			Comment("几月执行（1-12），每年计划使用").
			Optional(),

		field.Strings("excluded_dates").
			Comment("排除日期列表（YYYY-MM-DD，如法定节假日）").
			Optional(),

		field.String("post").
			Comment("执行岗位（派单目标岗位）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.JSON("space_items", []PeriodicPlanItem{}).
			Comment("空间作业点表 [{name,enabled}]").
			Optional(),

		field.JSON("equipment_items", []PeriodicPlanItem{}).
			Comment("设备作业点表 [{name,enabled}]").
			Optional(),

		// 预留：空间/设备对象模块落地后启用（当前无业务表，仅保留契约）
		field.Strings("space_refs").
			Comment("空间范围（预留：空间对象标识列表）").
			Optional(),

		field.Strings("equipment_refs").
			Comment("设备范围（预留：设备对象标识列表）").
			Optional(),

		field.Time("effective_from").
			Comment("生效日期（含当日；为空表示立即生效）").
			Optional().
			Nillable(),

		field.Time("effective_to").
			Comment("结束日期（含当日；为空表示长期有效）").
			Optional().
			Nillable(),
	}
}

// Mixin of the PeriodicPlan.
// 租户运营配置：计划是「某租户的运营安排」（本院每日巡查两次），挂 TenantID
// 与其自带租户 Privacy（类型字典保持平台级全局）。0 = 平台（内置模板）；
// 租户用户仅见本租户计划，平台/系统上下文放行（调度器按计划的租户生单）。
func (PeriodicPlan) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.TenantID[uint32]{},
		mixin.IsEnabled{},
	}
}

// Indexes of the PeriodicPlan.
func (PeriodicPlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("service_type_id"),
		index.Fields("freq_type"),
		index.Fields("tenant_id"),
	}
}
