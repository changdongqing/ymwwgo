package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
)

// TransportRoute 一站式运送路线（0106 运送配送类别节点下的常用路线配置）。
//
// 与巡检计划同款「列表型子资源」模式：一个类别可挂多条路线。路线描述
// 「从哪到哪（起点/终点/途经点）、单程预计多久、是否往返、定时班次何时发班」；
// 调度坐席按路线模板派单（ETA 基准），departure_times 非空的定时班次由
// 后续工单对象按时间点生成定时运送任务（本期只做配置管理）。
type TransportRoute struct {
	ent.Schema
}

func (TransportRoute) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "onestop_transport_routes",
		},
		entsql.WithComments(true),
		schema.Comment("一站式运送路线表（0106 运送配送类别的常用路线：起终点/途经点/时长/往返/定时班次）"),
	}
}

// Fields of the TransportRoute.
func (TransportRoute) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Comment("路线名称").
			MaxLen(128).
			NotEmpty().
			Optional().
			Nillable(),

		// 类别节点（0106 子树的小类/细类）；不加 ent Edge/FK（类别是全局字典，
		// 引用完整性由服务层校验），inspection_plan.service_type_id 同款
		field.Uint32("service_type_id").
			Comment("运送类别ID（onestop_service_types.id，0106 子树）").
			Optional().
			Nillable(),

		field.String("origin").
			Comment("起点（取件地，如：静配中心）").
			MaxLen(128).
			NotEmpty().
			Optional().
			Nillable(),

		field.String("destination").
			Comment("终点（送达地，如：内科楼 12 病区）").
			MaxLen(128).
			NotEmpty().
			Optional().
			Nillable(),

		field.Strings("via_points").
			Comment("途经点列表（按序途经的中间点，可空）").
			Optional(),

		field.Uint32("est_duration_minutes").
			Comment("预计单程时长（分钟，1-1440）：调度 ETA 与超时判定基准").
			Optional().
			Nillable(),

		field.Bool("round_trip").
			Comment("是否往返路线（送达后原路返回起点，如餐车回收）").
			Optional().
			Default(false),

		field.Strings("departure_times").
			Comment("定时班次发班时间点列表（HH:mm，每日按点发班）；为空表示按需即时派单").
			Optional(),
	}
}

// Mixin of the TransportRoute.
// 租户运营配置：与周期计划同款租户化（路线是「某租户的运送安排」）。
// 0 = 平台（内置模板）；班次展开调度器按路线的租户生成运送单。
func (TransportRoute) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{},
		mixin.OperatorID{},
		mixin.TenantID[uint32]{},
		mixin.IsEnabled{},
	}
}

// Indexes of the TransportRoute.
func (TransportRoute) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("service_type_id"),
		index.Fields("tenant_id"),
	}
}
