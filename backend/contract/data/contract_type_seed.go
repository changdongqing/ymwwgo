package data

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"go-wind-admin/contract/data/ent/schema"
	"go-wind-admin/contract/data/ent/contracttype"
)

// payMethodOfSeed / directionOfSeed 种子枚举串 → ent 枚举（空串返回 nil = 不写）。
// 非法值静默忽略会制造"看似播种成功"的假象，由种子守卫测试钉死值域。
func payMethodOfSeed(v *string) *contracttype.PayMethod {
	if v == nil || *v == "" {
		return nil
	}
	p := contracttype.PayMethod(*v)
	return &p
}

func directionOfSeed(v *string) *contracttype.FeeDirection {
	if v == nil || *v == "" {
		return nil
	}
	d := contracttype.FeeDirection(*v)
	return &d
}

// 合同类型内置字典：docs/connn/contract-type-and-contract-design.md §3.4 的
// 6 大类 + 16 细类 = 22 节点（大类携带默认合同配置模板，细类整组回退大类）。
// 数据源自医院后勤合同原型（youmingwanwu prototype/contract）的 6 类型硬编码
// 升级：称谓/付款方式/拆解节点/差异字段/必备附件进模板列。
// 播种采用空表守卫（count == 0），与宿主 DefaultMenus 的启动期播种口径一致。
//
//go:embed seeds/contract_types.json
var contractTypeSeedData []byte

type contractTypeSeed struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Level      string `json:"level"`
	ParentCode string `json:"parentCode"`
	// 统一合同配置模板（主要配在大类，细类可覆写；全部可空）
	PartyBLabel       *string                 `json:"partyBLabel,omitempty"`
	PayMethod         *string                 `json:"payMethod,omitempty"`
	PayNodes          []contractPayNodeSeed   `json:"payNodes,omitempty"`
	FeeDirection      *string                 `json:"feeDirection,omitempty"`
	RequiresApproval  *bool                   `json:"requiresApproval,omitempty"`
	RequiresDeposit   *bool                   `json:"requiresDeposit,omitempty"`
	DiffFields        []contractDiffFieldSeed `json:"diffFields,omitempty"`
	RequiredAttach    []string                `json:"requiredAttachments,omitempty"`
	Guidance          *string                 `json:"guidance,omitempty"`
}

type contractPayNodeSeed struct {
	Name         string `json:"name"`
	RatioPercent int32  `json:"ratioPercent"`
}

type contractDiffFieldSeed struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Widget  string   `json:"widget"`
	Options []string `json:"options,omitempty"`
}

// SeedDefaults 在合同类型表为空时播种内置二级字典；非空表不做任何写入，
// 避免覆盖运维在管理页调整过的启停状态或名称。
func (r *ContractTypeRepo) SeedDefaults(ctx context.Context) error {
	count, err := r.entClient.Client().ContractType.Query().Count(ctx)
	if err != nil {
		r.log.Errorf(ctx, "count contract types before seeding failed: %v", err)
		return err
	}
	if count > 0 {
		return nil
	}

	var seeds []contractTypeSeed
	if err = json.Unmarshal(contractTypeSeedData, &seeds); err != nil {
		r.log.Errorf(ctx, "unmarshal contract type seed data failed: %v", err)
		return err
	}

	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf(ctx, "begin contract type seed transaction failed: %v", err)
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			// 回滚失败必须留痕，否则半截字典会静默留在库里。
			r.log.Errorf(ctx, "rollback contract type seed transaction failed: %v", rollbackErr)
		}
	}()

	// 数据按层级（2 → 4 位）排序，父节点一定先于子节点落库。
	now := time.Now()
	idByCode := make(map[string]uint32, len(seeds))
	for _, seed := range seeds {
		level := contracttype.Level(seed.Level)
		if err = contracttype.LevelValidator(level); err != nil {
			r.log.Errorf(ctx, "invalid contract type seed %q: %v", seed.Code, err)
			return err
		}

		builder := tx.ContractType.Create().
			SetCode(seed.Code).
			SetName(seed.Name).
			SetLevel(level).
			SetIsEnabled(true).
			SetNillablePartyBLabel(seed.PartyBLabel).
			SetNillablePayMethod(payMethodOfSeed(seed.PayMethod)).
			SetNillableFeeDirection(directionOfSeed(seed.FeeDirection)).
			SetNillableRequiresApproval(seed.RequiresApproval).
			SetNillableRequiresDeposit(seed.RequiresDeposit).
			SetNillableGuidance(seed.Guidance).
			SetCreatedAt(now).
			SetUpdatedAt(now)

		// jsonb 数组：非空才写
		if len(seed.PayNodes) > 0 {
			nodes := make([]schema.ContractPayNode, 0, len(seed.PayNodes))
			for _, n := range seed.PayNodes {
				nodes = append(nodes, schema.ContractPayNode{Name: n.Name, RatioPercent: n.RatioPercent})
			}
			builder.SetPayNodes(nodes)
		}
		if len(seed.DiffFields) > 0 {
			fields := make([]schema.ContractDiffField, 0, len(seed.DiffFields))
			for _, f := range seed.DiffFields {
				df := schema.ContractDiffField{Key: f.Key, Label: f.Label, Widget: f.Widget}
				if len(f.Options) > 0 {
					df.Options = f.Options
				}
				fields = append(fields, df)
			}
			builder.SetDiffFields(fields)
		}
		if len(seed.RequiredAttach) > 0 {
			builder.SetRequiredAttachments(seed.RequiredAttach)
		}

		if seed.ParentCode != "" {
			parentID, ok := idByCode[seed.ParentCode]
			if !ok {
				err = fmt.Errorf("contract type seed %q references missing parent %q", seed.Code, seed.ParentCode)
				r.log.Errorf(ctx, "%v", err)
				return err
			}
			builder.SetParentID(parentID)
		}

		entity, createErr := builder.Save(ctx)
		if createErr != nil {
			r.log.Errorf(ctx, "insert contract type seed %q failed: %v", seed.Code, createErr)
			return createErr
		}
		idByCode[seed.Code] = entity.ID
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf(ctx, "commit contract type seed transaction failed: %v", err)
		return err
	}
	committed = true

	r.log.Infof(ctx, "seeded %d built-in contract types", len(seeds))
	return nil
}
