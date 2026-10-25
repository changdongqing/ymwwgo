package data

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"go-wind-admin/onestop/data/ent/schema"
	"go-wind-admin/onestop/data/ent/servicetype"
)

// temperatureOfSeed / priorityOfSeed / triggerOfSeed 种子枚举串 → ent 枚举
// （空串返回零值 = 不写）。非法值在 Level 校验同款位置兜不住（这里静默忽略会制造
// "看似播种成功"的假象），因此直接透传给 ent 枚举构造，非法值由种子守卫测试钉死。
func temperatureOfSeed(v *string) *servicetype.TemperatureRequirement {
	if v == nil || *v == "" {
		return nil
	}
	t := servicetype.TemperatureRequirement(*v)
	return &t
}

func priorityOfSeed(v *string) *servicetype.DefaultPriority {
	if v == nil || *v == "" {
		return nil
	}
	p := servicetype.DefaultPriority(*v)
	return &p
}

func triggerOfSeed(v *string) *servicetype.TriggerMode {
	if v == nil || *v == "" {
		return nil
	}
	t := servicetype.TriggerMode(*v)
	return &t
}

func orderKindOfSeed(v *string) *servicetype.OrderKind {
	if v == nil || *v == "" {
		return nil
	}
	k := servicetype.OrderKind(*v)
	return &k
}

// 服务类型内置字典：docs/leixing/biaodan/README.md 的 211 行细类平铺清单，
// 按编码前缀拆成 1 大类 + 12 中类 + 75 小类 + 211 细类 = 299 个节点。
//
// 数据与文档同源（2026-10 全量清单），由脚本从 README 表格生成；
// 文档新增条目时同步更新本文件即可。播种采用空表守卫（count == 0），
// 与宿主 DefaultMenus/DefaultConfigs 的启动期播种口径一致。
//
//go:embed seeds/service_types.json
var serviceTypeSeedData []byte

type serviceTypeSeed struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Level      string `json:"level"`
	ParentCode string `json:"parentCode"`
	// 统一履约配置模板（仅小类/细类携带，全部可空；可见性由前端展示矩阵按中类驱动）
	ExecutionPost          *string                     `json:"executionPost,omitempty"`
	StandardWorkHours      *float64                    `json:"standardWorkHours,omitempty"`
	ResponseTimeLimit      *uint32                     `json:"responseTimeLimit,omitempty"`
	CompletionTimeLimit    *uint32                     `json:"completionTimeLimit,omitempty"`
	Guidance               *string                     `json:"guidance,omitempty"`
	CommonPhrases          []string                    `json:"commonPhrases,omitempty"`
	StandardTools          []string                    `json:"standardTools,omitempty"`
	SpareParts             []string                    `json:"spareParts,omitempty"`
	DefaultPriority        *string                     `json:"defaultPriority,omitempty"`
	AcceptanceModes        []string                    `json:"acceptanceModes,omitempty"`
	AcceptanceItems        []serviceTypeAcceptanceSeed `json:"acceptanceItems,omitempty"`
	TemperatureRequirement *string                     `json:"temperatureRequirement,omitempty"`
	RequiresApproval       *bool                       `json:"requiresApproval,omitempty"`
	CargoTypes             []string                    `json:"cargoTypes,omitempty"`
	// 标准作业项目库：周期计划点表模板（计划型子树的小类/细类携带）
	StandardItems []string `json:"standardItems,omitempty"`
	// 任务触发方式：仅 SCHEDULED 节点显式标记，其余留空（NULL = 回退上级，默认按需）
	TriggerMode *string `json:"triggerMode,omitempty"`
	// 履约形态：仅推导覆盖不了的特例显式标记（当前只有 010405 被服收发按
	// 业务形状归运送），其余留空走推导（0106 子树/计划生成/事件顺序）
	OrderKind *string `json:"orderKind,omitempty"`
}

type serviceTypeAcceptanceSeed struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// SeedDefaults 在服务类型表为空时播种内置四级字典；非空表不做任何写入，
// 避免覆盖运维在管理页调整过的启停状态或名称。
func (r *ServiceTypeRepo) SeedDefaults(ctx context.Context) error {
	count, err := r.entClient.Client().ServiceType.Query().Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "count service types before seeding failed: %v", err)
		return err
	}
	if count > 0 {
		return nil
	}

	var seeds []serviceTypeSeed
	if err = json.Unmarshal(serviceTypeSeedData, &seeds); err != nil {
		r.log.Errorf(ctx, "unmarshal service type seed data failed: %v", err)
		return err
	}

	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin service type seed transaction failed: %v", err)
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			// 回滚失败必须留痕，否则半截字典会静默留在库里。
			r.log.Errorf(ctx, "rollback service type seed transaction failed: %v", rollbackErr)
		}
	}()

	// 数据按层级（2 → 4 → 6 → 8 位）排序，父节点一定先于子节点落库。
	now := time.Now()
	idByCode := make(map[string]uint32, len(seeds))
	for _, seed := range seeds {
		level := servicetype.Level(seed.Level)
		if err = servicetype.LevelValidator(level); err != nil {
			r.log.Errorf(ctx, "invalid service type seed %q: %v", seed.Code, err)
			return err
		}

		builder := tx.ServiceType.Create().
			SetCode(seed.Code).
			SetName(seed.Name).
			SetLevel(level).
			SetIsEnabled(true).
			SetNillableExecutionPost(seed.ExecutionPost).
			SetNillableStandardWorkHours(seed.StandardWorkHours).
			SetNillableResponseTimeLimit(seed.ResponseTimeLimit).
			SetNillableCompletionTimeLimit(seed.CompletionTimeLimit).
			SetNillableGuidance(seed.Guidance).
			SetNillableTemperatureRequirement(temperatureOfSeed(seed.TemperatureRequirement)).
			SetNillableRequiresApproval(seed.RequiresApproval).
			SetNillableDefaultPriority(priorityOfSeed(seed.DefaultPriority)).
			SetNillableTriggerMode(triggerOfSeed(seed.TriggerMode)).
			SetNillableOrderKind(orderKindOfSeed(seed.OrderKind)).
			SetCreatedAt(now).
			SetUpdatedAt(now)

		// jsonb 数组：非空才写
		if len(seed.CommonPhrases) > 0 {
			builder.SetCommonPhrases(seed.CommonPhrases)
		}
		if len(seed.StandardTools) > 0 {
			builder.SetStandardTools(seed.StandardTools)
		}
		if len(seed.SpareParts) > 0 {
			builder.SetSpareParts(seed.SpareParts)
		}
		if len(seed.StandardItems) > 0 {
			builder.SetStandardItems(seed.StandardItems)
		}
		if len(seed.CargoTypes) > 0 {
			builder.SetCargoTypes(seed.CargoTypes)
		}
		if len(seed.AcceptanceModes) > 0 {
			builder.SetAcceptanceModes(seed.AcceptanceModes)
		}
		if len(seed.AcceptanceItems) > 0 {
			items := make([]schema.ServiceAcceptanceItem, 0, len(seed.AcceptanceItems))
			for _, item := range seed.AcceptanceItems {
				items = append(items, schema.ServiceAcceptanceItem{Name: item.Name, Required: item.Required})
			}
			builder.SetAcceptanceItems(items)
		}

		if seed.ParentCode != "" {
			parentID, ok := idByCode[seed.ParentCode]
			if !ok {
				err = fmt.Errorf("service type seed %q references missing parent %q", seed.Code, seed.ParentCode)
				r.log.Errorf(ctx, "%v", err)
				return err
			}
			builder.SetParentID(parentID)
		}

		entity, createErr := builder.Save(ctx)
		if createErr != nil {
			r.log.Errorf(ctx, "insert service type seed %q failed: %v", seed.Code, createErr)
			return createErr
		}
		idByCode[seed.Code] = entity.ID
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit service type seed transaction failed: %v", err)
		return err
	}
	committed = true

	r.log.Infof(ctx, "seeded %d built-in service types", len(seeds))
	return nil
}
