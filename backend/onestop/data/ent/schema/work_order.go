package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// WorkOrderMaterial 维修/保养领用的备件耗材记录（jsonb 存储，used_materials 的元素）。
// 本期手工填写（库存联动未落地），单均 1-5 项，无独立查询需求故不设子表。
type WorkOrderMaterial struct {
	Name   string  `json:"name"`
	Qty    float64 `json:"qty"`
	Unit   string  `json:"unit"`
	Remark string  `json:"remark"`
}

// WorkOrderAcceptanceResultItem 验收项结果（jsonb 存储，acceptance_result 的元素）。
// name 对应创建时快照的 acceptance_items 清单项，passed 为逐项勾选结论。
type WorkOrderAcceptanceResultItem struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Remark string `json:"remark"`
}

// WorkOrder 一站式工单（统一工单底座：一次服务履约的全生命周期实例）。
//
// 三种履约形态（order_kind）共用一张主表，形态只驱动状态机分支、界面面板与
// 专项字段出现，不裂表——与配置层「统一模板 + 展示矩阵」同一条路线：
//   - KIND_EVENT 事件处置：受理→派单→到场→完工→验收（报修/投诉/应急等按需单）；
//   - KIND_TASK 计划任务：计划生成→逐项执行（items 子表）→确认（维保/巡检/保洁等）；
//   - KIND_TRANSPORT 运送：取件签收→送达签收（双签收内嵌验收，送达即完结）。
//
// 快照原则：创建时从类型 effective-config（细类→小类回退）解析并落库运行期判定
// 字段（岗位/优先级/SLA 双时限/验收方案/温控）；计划/路线生单时点表与路线参数
// 快照为执行项与运送字段。此后类型/计划修改不影响存量单。纯参考文案（指导
// 说明/常见语句/工具/耗材清单）实时读类型，不落快照。
//
// 租户业务数据：挂 TenantID mixin 与租户 Policy（与类型/计划/路线不同——它们是
// 平台级字典与模板，0 = 平台）。跨类流转经 source_order_id 关联（转单关旧开新 /
// 巡检异常派生报修），所有动作落 onestop_work_order_events 时间线。
type WorkOrder struct {
	ent.Schema
}

func (WorkOrder) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "onestop_work_orders",
		},
		entsql.WithComments(true),
		schema.Comment("一站式工单表（统一工单底座：事件处置/计划任务/运送三形态，快照+事件流）"),
	}
}

// Fields of the WorkOrder.
func (WorkOrder) Fields() []ent.Field {
	return []ent.Field{
		// —— 标识与归属 ——
		field.String("order_no").
			Comment("工单号：WO- + 8 位全局序列，排序友好").
			MaxLen(32).
			NotEmpty().
			Unique().
			Optional().
			Nillable(),

		// 类别：必须为启用细类（LEVEL_DETAIL）；无 ent 外键（字典引用完整性由
		// 服务层校验），periodic_plan.service_type_id 同款
		field.Uint32("service_type_id").
			Comment("服务类型ID（onestop_service_types.id，创建时必须为启用细类）").
			Optional().
			Nillable(),

		// 编码/名称快照：字典只停用不删除，快照使列表/导出不依赖 join 且免受改名影响
		field.String("service_type_code").
			Comment("服务类型编码快照（创建时落库，历史单不受字典调整影响）").
			MaxLen(8).
			Optional().
			Nillable(),

		field.String("service_type_name").
			Comment("服务类型名称快照（创建时落库）").
			MaxLen(128).
			Optional().
			Nillable(),

		// 枚举存储值与 proto 枚举值名保持一致（KIND_EVENT...），
		// 供 mapper.EnumTypeConverter 按名称直转
		field.Enum("order_kind").
			Comment("履约形态 KIND_EVENT:事件处置 KIND_TASK:计划任务 KIND_TRANSPORT:运送").
			NamedValues(
				"Event", "KIND_EVENT",
				"Task", "KIND_TASK",
				"Transport", "KIND_TRANSPORT",
			).
			Optional().
			Nillable(),

		field.Enum("source").
			Comment("生单来源 SOURCE_MANUAL:手动创建 SOURCE_PLAN:计划展开 SOURCE_ROUTE:路线班次 SOURCE_TRANSFER:转单 SOURCE_DERIVED:派生").
			NamedValues(
				"Manual", "SOURCE_MANUAL",
				"Plan", "SOURCE_PLAN",
				"Route", "SOURCE_ROUTE",
				"Transfer", "SOURCE_TRANSFER",
				"Derived", "SOURCE_DERIVED",
			).
			Optional().
			Nillable(),

		field.Enum("channel").
			Comment("发起渠道（手动单）CHANNEL_DESK:坐席台 CHANNEL_PHONE:电话，小程序/开放API预留").
			NamedValues(
				"Desk", "CHANNEL_DESK",
				"Phone", "CHANNEL_PHONE",
			).
			Optional().
			Nillable(),

		field.Uint32("source_order_id").
			Comment("来源工单ID（转单/派生时的旧单，onestop_work_orders.id）").
			Optional().
			Nillable(),

		// —— 发起信息（手动单填写；计划/班次单由计划名/类别生成摘要）——
		field.String("requester_name").
			Comment("发起人称呼（电话呼叫方常无账号，自由文本）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("requester_phone").
			Comment("发起人联系电话").
			MaxLen(32).
			Optional().
			Nillable(),

		field.Uint32("requester_user_id").
			Comment("发起用户ID（登录用户发起时记录；坐席代录时记录最终用户）").
			Optional().
			Nillable(),

		field.String("location").
			Comment("位置描述（楼/层/房间自由描述；空间对象引用待空间模块落地预留）").
			MaxLen(256).
			Optional().
			Nillable(),

		field.String("equipment_desc").
			Comment("服务对象描述（报修的故障设备/设施，与现象描述分开；设备引用预留）").
			MaxLen(256).
			Optional().
			Nillable(),

		field.String("description").
			Comment("需求/现象描述（一句话也算）").
			MaxLen(1024).
			NotEmpty().
			Optional().
			Nillable(),

		field.Strings("images").
			Comment("发起现场照片").
			Optional(),

		field.String("common_phrase").
			Comment("命中的快捷语句（发起端语义归型语料回流预留）").
			MaxLen(128).
			Optional().
			Nillable(),

		// —— 派单与执行 ——
		// 岗位快照：手动单取类型 effective 岗位；计划单优先计划级 post（覆盖类别岗位）
		field.String("execution_post").
			Comment("执行岗位快照（派单目标岗位；计划单优先计划级岗位覆盖类别岗位）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.Uint32("assignee_id").
			Comment("执行人用户ID（岗位派单时可空，接单后补）").
			Optional().
			Nillable(),

		field.String("assignee_name").
			Comment("执行人姓名快照（防改名影响历史展示）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.Time("assigned_at").
			Comment("派单时间（手动单=派单动作；计划/班次单=生成时刻即视为已派岗）").
			Optional().
			Nillable(),

		field.Time("planned_exec_at").
			Comment("计划执行时刻（计划/班次单必填：计划时刻或发班时刻；事件单为空）").
			Optional().
			Nillable(),

		field.Uint32("plan_id").
			Comment("来源周期计划ID（onestop_periodic_plans.id；调度幂等键组成部分）").
			Optional().
			Nillable(),

		field.Uint32("route_id").
			Comment("来源运送路线ID（onestop_transport_routes.id；班次单幂等键组成部分）").
			Optional().
			Nillable(),

		// —— SLA（快照 + deadline 落库，超时过滤/红黄标不现算）——
		field.Enum("priority").
			Comment("优先级快照 PRIORITY_NORMAL:常规 PRIORITY_URGENT:加急 PRIORITY_EMERGENCY:特急（默认取类别，可覆盖）").
			NamedValues(
				"Normal", "PRIORITY_NORMAL",
				"Urgent", "PRIORITY_URGENT",
				"Emergency", "PRIORITY_EMERGENCY",
			).
			Optional().
			Nillable(),

		field.Uint32("response_time_limit").
			Comment("响应时限快照（分钟，受理→到场/取件；空=不限时）").
			Optional().
			Nillable(),

		field.Uint32("completion_time_limit").
			Comment("完成时限快照（分钟，受理→完工/送达/办结；空=不限时）").
			Optional().
			Nillable(),

		field.Time("response_deadline").
			Comment("响应截止时刻（手动单=建单+时限；计划/班次单=计划执行时刻+时限）").
			Optional().
			Nillable(),

		field.Time("completion_deadline").
			Comment("完成截止时刻（同响应截止的起算口径）").
			Optional().
			Nillable(),

		field.Time("intake_at").
			Comment("受理时间（手动单=建单即受理；计划/班次单=生成时刻）").
			Optional().
			Nillable(),

		field.Time("responded_at").
			Comment("响应达成时间（事件/任务=到场；运送=取件）").
			Optional().
			Nillable(),

		field.Time("completed_at").
			Comment("完成时间（事件/任务=完工提交；运送=送达签收）").
			Optional().
			Nillable(),

		field.Time("verified_at").
			Comment("验收通过时间（事件/任务=验收动作；运送=送达即验收）").
			Optional().
			Nillable(),

		// —— 状态与跟进 ——
		field.Enum("status").
			Comment("状态 STATUS_PENDING:待派单 STATUS_APPROVING:审批中 STATUS_DISPATCHED:已派单 STATUS_PROCESSING:执行中 STATUS_COMPLETED:已完工待验收 STATUS_CLOSED:已完结 STATUS_CANCELLED:已撤销").
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

		field.String("cancel_reason").
			Comment("撤销/驳回/转单关闭原因（cancel 必填；转单自动生成「转单至 WO-xxx」）").
			MaxLen(256).
			Optional().
			Nillable(),

		field.Bool("has_abnormality").
			Comment("是否存在异常（异常上报事件或执行项异常置位，列表过滤用）").
			Optional().
			Default(false),

		field.Uint32("urge_count").
			Comment("催办次数（明细在事件表，主表冗余计数便于列表）").
			Optional().
			Default(0),

		// —— 运送专项（KIND_TRANSPORT；存储层不校验形态，可见性由前端形态面板控制）——
		field.String("origin").
			Comment("起点（取件地；手动单必填，班次单快照自路线）").
			MaxLen(128).
			Optional().
			Nillable(),

		field.String("destination").
			Comment("终点（送达地）").
			MaxLen(128).
			Optional().
			Nillable(),

		field.Strings("via_points").
			Comment("途经点列表（快照自路线，按序途经）").
			Optional(),

		field.Bool("round_trip").
			Comment("是否往返（送达后原路返回，路线快照）").
			Optional().
			Default(false),

		field.Uint32("est_duration_minutes").
			Comment("预计单程时长（分钟，路线快照，ETA 基准）").
			Optional().
			Nillable(),

		field.Enum("temperature_requirement").
			Comment("温控要求快照 TEMP_NORMAL:常温 TEMP_COLD:冷藏2-8℃ TEMP_FROZEN:冷冻 TEMP_WARM:保温").
			NamedValues(
				"Normal", "TEMP_NORMAL",
				"Cold", "TEMP_COLD",
				"Frozen", "TEMP_FROZEN",
				"Warm", "TEMP_WARM",
			).
			Optional().
			Nillable(),

		field.String("cargo_desc").
			Comment("本单承运物品描述（类型级 cargo_types 是承接范围，单级是实例）").
			MaxLen(256).
			Optional().
			Nillable(),

		field.Time("picked_up_at").
			Comment("取件签收时间（运送）").
			Optional().
			Nillable(),

		field.String("picked_signer").
			Comment("取件签收人").
			MaxLen(64).
			Optional().
			Nillable(),

		field.Strings("picked_photos").
			Comment("取件签收照片").
			Optional(),

		field.Time("delivered_at").
			Comment("送达签收时间（运送，送达即完结）").
			Optional().
			Nillable(),

		field.String("delivered_signer").
			Comment("送达签收人").
			MaxLen(64).
			Optional().
			Nillable(),

		field.Strings("delivered_photos").
			Comment("送达签收照片").
			Optional(),

		// —— 完工与验收（事件/任务形态）——
		field.String("result_desc").
			Comment("完工结果描述（完工提交）").
			MaxLen(1024).
			Optional().
			Nillable(),

		field.Strings("result_photos").
			Comment("完工结果照片").
			Optional(),

		field.JSON("used_materials", []WorkOrderMaterial{}).
			Comment("备件/耗材领用记录 [{name,qty,unit,remark}]（本期手工填，库存联动未落地）").
			Optional(),

		field.Float("actual_work_hours").
			Comment("实际工时（小时，完工时手填，绩效核算预留）").
			Optional().
			Nillable(),

		field.Strings("acceptance_modes").
			Comment("验收方式快照（多选 ACCEPT_SIGN:签名确认 ACCEPT_SCAN:扫码核对 ACCEPT_PHOTO:拍照留证 ACCEPT_DUAL:双人验收）").
			Optional(),

		field.JSON("acceptance_items", []ServiceAcceptanceItem{}).
			Comment("验收项清单快照 [{name,required}]（创建时从类别快照，验收逐项勾选）").
			Optional(),

		field.JSON("acceptance_result", []WorkOrderAcceptanceResultItem{}).
			Comment("验收项结果 [{name,passed,remark}]（验收动作提交）").
			Optional(),

		field.Strings("acceptance_photos").
			Comment("验收照片").
			Optional(),

		field.String("acceptance_signer").
			Comment("验收签收人").
			MaxLen(64).
			Optional().
			Nillable(),

		// —— 回访 ——
		field.Uint32("satisfaction_score").
			Comment("满意度评分（1-5，仅已完结单可补录）").
			Optional().
			Nillable(),

		field.String("satisfaction_comment").
			Comment("满意度回访意见").
			MaxLen(512).
			Optional().
			Nillable(),

		field.Time("revisited_at").
			Comment("回访补录时间").
			Optional().
			Nillable(),
	}
}

// Mixin of the WorkOrder.
// 租户业务数据：挂 TenantID 与其自带的租户 Privacy（查询/写入按 viewer 隔离，
// 平台/系统上下文放行）。无启停概念故不挂 IsEnabled。
func (WorkOrder) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the WorkOrder.
func (WorkOrder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "status").
			StorageKey("workorder_tenant_status"),
		index.Fields("service_type_id"),
		index.Fields("assignee_id"),
		index.Fields("planned_exec_at"),
		index.Fields("response_deadline"),
		index.Fields("completion_deadline"),
		index.Fields("created_at"),
		// 调度幂等：一个计划/路线在同一时刻最多生成一单（软删行不占坑——
		// 误删的调度单会因窗口内时刻未过而被重新生成，防漏是有意行为）
		index.Fields("plan_id", "planned_exec_at").
			Unique().
			Annotations(entsql.IndexWhere("plan_id IS NOT NULL AND deleted_at IS NULL")).
			StorageKey("workorder_plan_instance"),
		index.Fields("route_id", "planned_exec_at").
			Unique().
			Annotations(entsql.IndexWhere("route_id IS NOT NULL AND deleted_at IS NULL")).
			StorageKey("workorder_route_instance"),
	}
}
