package data

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"go-wind-admin/onestop/data/ent/servicetype"
)

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
	// 报修维修类型配置（仅 0101 子树的小类/细类携带，其余条目留空）
	RepairPost          *string  `json:"repairPost,omitempty"`
	StandardWorkHours   *float64 `json:"standardWorkHours,omitempty"`
	ResponseTimeLimit   *uint32  `json:"responseTimeLimit,omitempty"`
	CompletionTimeLimit *uint32  `json:"completionTimeLimit,omitempty"`
	Guidance            *string  `json:"guidance,omitempty"`
	CommonPhrases       []string `json:"commonPhrases,omitempty"`
	StandardTools       []string `json:"standardTools,omitempty"`
	SpareParts          []string `json:"spareParts,omitempty"`
	// 巡检巡查类型配置（仅 0103 子树携带）
	InspectionItems []string `json:"inspectionItems,omitempty"`
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
			SetNillableRepairPost(seed.RepairPost).
			SetNillableStandardWorkHours(seed.StandardWorkHours).
			SetNillableResponseTimeLimit(seed.ResponseTimeLimit).
			SetNillableCompletionTimeLimit(seed.CompletionTimeLimit).
			SetNillableGuidance(seed.Guidance).
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
		if len(seed.InspectionItems) > 0 {
			builder.SetInspectionItems(seed.InspectionItems)
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
