# 一站式服务类型「统一配置模板」详细设计

> 状态：**已实施**（2026-10-25，迁移 00012 + P1a/P1b 同期落地；实施偏差见 §17）｜
> 编写日期：2026-10-25｜范围：`backend/onestop` + `frontend/admin/react` 一站式服务类型配置详情
>
> 前置阅读：`backend/onestop/README.md`（统一模板后的模块文档）、
> `docs/database/design/ent-goose-migrations.md`（迁移铁律）。

## 1. 背景与结论

### 1.1 三期演化路径

| 期 | 子树 | 落地物 |
|---|---|---|
| 报修期 | 0101 | 类型表 8 个履约列（岗位/工时/SLA×2/指导/语句/工具/备件） |
| 巡检期 | 0103 | `inspection_items` 标准项目库 + `onestop_inspection_plans` 计划子资源（频率/时间点/岗位/点表/生效期） |
| 运送期 | 0106 | 优先级/验收方式/验收项**提升为两子树通用**；运送专属 4 列（岗位/温控/审批/物品细目）+ `onestop_transport_routes` 路线子资源 |

运送期把优先级/验收从报修专属提升为通用，是统一化的第一次自然收敛。三期后类型表已有 15 个配置列，
其中 9 个（岗位语义、SLA×2、工时、指导、语句、工具、优先级、验收方案）天然跨业态通用，
真正互斥的只有 6 个（报修 2 + 运送 4）。

### 1.2 本设计回答的两个问题

**Q1：所有一站式服务类型能否共用一个配置模板？**
**能。** 统一的是「存储与字段机制 + 通用核心字段」，各中类专属字段（温控/审批/物品细目/标准项目库）
保留在模板内作为「专项属性组」，按中类条件展示。全部字段可空（NULL = 未配置），冗余在存储层零成本，
代价只由展示矩阵控制。

**Q2：所有类型能否周期计划化？**
**不能，也不需要强行。** 按 299 节点字典逐类核对，作业模式分四类：

| 作业模式 | 典型子树 | 占比（细类） | 承载方式 |
|---|---|---|---|
| 周期计划型 | 0102 维保、0103 巡检、0104 日常保洁/消杀、0109 抄表检测、0110 绿养、0112 盘点考核 | 约六成 | **周期计划**（本期泛化） |
| 即时事件型 | 0101 报修、0106 急送、0107 突发、0108 投诉、0111 应急 | 约三成 | 单值履约参数 + 未来工单对象（按需触发） |
| 定时班次型 | 0106 巡回配送、0104 被服收发、0105 值守交接 | 少量 | 计划的 `times`/路线 `departure_times` 表达班次时刻；**轮班排班不在配置层**（排班模块边界） |
| 项目/流程型 | 0112 工程、0109 改造、0108 资源申请、0105 动火审批 | 少量 | `requires_approval` + 未来流程/项目对象；模板字段冗余但无害 |

因此不搞"全部周期化"，改为**统一的触发模式开关（`trigger_mode`）**：每个类型默认「按需发起」，
标记为「计划生成」才开放周期计划入口。开关是配置层的统一机制，计划入口的显隐、
后端挂载校验、未来工单对象的任务生成策略都由它驱动。

### 1.3 设计原则

1. **统一存储 + 条件展示**：一套列、一个分节结构；可见性与文案由中类维度的展示矩阵驱动，
   矩阵未覆盖的新建中类回落为"全组可见 + 通用文案"。
2. **未配置 = NULL**：所有配置列可空；语义回退链（细类 → 小类 → 类型树默认）在读取端解析，不在写入端强制。
3. **计划是唯一排期事实源**：0102 细类名自带的"半月/季度"是行业目录标签（法规档位），
   实际排期以挂在节点上的周期计划为准——双存口径就此钉死，消费侧不得读细类名猜频率。
4. **破坏性变更一次性做完**：列改名、表改名、路由改名集中在迁移 00012 同一 commit；
   项目处于 dev 阶段无外部消费方，晚改成本只增不减。

## 2. 术语

| 术语 | 含义 |
|---|---|
| 大类/中类/小类/细类 | 类型树四级，编码 2/4/6/8 位，位数即层级 |
| 履约配置 | 挂在类型节点上的服务履约参数（岗位/SLA/工时/指导/语句/工具/耗材/优先级/验收） |
| 触发模式 `trigger_mode` | 任务如何产生：按需发起（TRIGGER_ON_DEMAND）/ 计划生成（TRIGGER_SCHEDULED） |
| 周期计划 | 原巡检计划泛化：频率/执行时间点/执行岗位/作业项目点表/生效区间 |
| 展示矩阵 | 中类编码 → 字段组可见性 + 字段文案 的前端常量表 |
| 标准作业项目库 `standard_items` | 原巡检 `inspection_items` 泛化：计划点表的模板来源 |

## 3. 设计总览

```
onestop_service_types（类型字典 = 统一配置模板宿主）
│
├── 结构字段：code / name / level / parent_id / description / is_enabled
│
├── 触发模式：trigger_mode（NULL=未设置；解析链：细类 → 小类 → 默认 ON_DEMAND）
│
├── 履约配置（统一模板，全可空，仅小类/细类两级可填）
│   ├── A 派单与时效：execution_post / default_priority / response_time_limit
│   │                  / completion_time_limit / standard_work_hours
│   ├── B 作业标准：  guidance / common_phrases / standard_tools / spare_parts
│   ├── C 验收方案：  acceptance_modes / acceptance_items
│   └── D 专项属性：  temperature_requirement / requires_approval / cargo_types
│                      / standard_items（按展示矩阵条件出现）
│
├── onestop_periodic_plans（原 onestop_inspection_plans 泛化，列表型子资源）
│   └── 挂载条件：类别节点（或其小类）解析后 trigger_mode = SCHEDULED
│
└── onestop_transport_routes（维持运送专属，本期不动）
    └── 挂载条件：维持 0106 子树小类/细类不变（与触发方式无关——按需押运同样
        需要路线模板，仅定时班次路线参与后续任务生成）
```

三处门控函数 `isRepairConfigurable / isInspectionConfigurable / isTransportConfigurable`
收敛为一个 `isConfigurable`（6/8 位编码即可配置），差异全部转入展示矩阵与 `trigger_mode`。

## 4. 数据模型设计

### 4.1 `onestop_service_types` 列收敛映射

| 动作 | 旧 | 新 | 说明 |
|---|---|---|---|
| 合并新增 | `repair_post`、`transport_post` | `execution_post` varchar(64) | 执行岗位（派单目标岗位）。两旧列属不同子树、同行不可能双非空，`COALESCE` 搬迁后删旧列 |
| 改名 | `inspection_items` | `standard_items` jsonb | 标准作业项目库（计划点表模板来源），语义从巡检放宽到一切计划型类别 |
| 新增 | — | `trigger_mode` varchar | TRIGGER_ON_DEMAND / TRIGGER_SCHEDULED；NULL = 未设置（走继承） |
| 文案放宽 | `spare_parts` | 同名 | label 改「耗材/备件清单」（保洁耗材、维保耗材、维修备件同列承载），列名不动 |
| 不动 | 其余 10 列 | — | 工时/SLA×2/指导/语句/工具/优先级/验收×2/温控/审批/物品细目原样沿用 |

`trigger_mode` 采用 **NULLable 而非 Default**：细类默认值 ON_DEMAND 无法区分"未设置"与
"显式按需"，继承链要求 NULL 表达"未设置，向上回退"。

**专项属性列（D 组）的存储约束取消**：温控/审批/物品细目不再限定 0106 子树（后端不校验前缀），
`requires_approval` 由此自然覆盖动火审批（01050401）、资源申请（010803）、搬入搬出（01060501）
等审批类服务。可见性交给展示矩阵，存储层全放开——统一模板"允许冗余"的落点。

### 4.2 `trigger_mode` 语义与解析链

| 值 | 语义 | 配置层表现 | 工单层表现（P2） |
|---|---|---|---|
| TRIGGER_ON_DEMAND | 按需发起 | 无周期计划入口 | 工单由用户/呼叫触发，走 SLA + 优先级派单 |
| TRIGGER_SCHEDULED | 计划生成 | 编辑态出现「周期计划」面板 | 调度器按计划频率生成任务（班次时刻即计划的 times） |
| NULL（未设置） | 继承 | 细类回退小类；小类也未设置按 ON_DEMAND | 同左解析结果 |

约束：

- 小类/细类两级可填（与履约配置同层）；大类/中类纯结构节点不接受任何配置字段。
- **由 SCHEDULED 切回 ON_DEMAND 时，若节点已挂周期计划 → 422 拒绝**，提示先删计划。
  计划挂载校验同步改为：类别解析后（细类空则取小类）`trigger_mode = SCHEDULED` 才允许，
  **替代现有 0103 编码前缀校验**。

### 4.3 周期计划表泛化（改名不改结构）

`onestop_inspection_plans` → **`onestop_periodic_plans`**，全部字段沿用——
频率模型（freq_type/times/weekdays/month_days/months/clamp_to_month_end/excluded_dates）、
`post`（计划级执行岗位，覆盖类型级 `execution_post`）、
`space_items` / `equipment_items`（语义放宽：空间作业项/设备作业项，`{name,enabled}` 结构
对巡检点表/维保项目单/保洁作业标准同构）、`space_refs` / `equipment_refs`（预留）、生效区间。

命名变更清单（一次到位）：

| 层 | 旧 | 新 |
|---|---|---|
| 表 | `onestop_inspection_plans` | `onestop_periodic_plans` |
| ent 实体/包 | `InspectionPlan` / `inspectionplan` | `PeriodicPlan` / `periodicplan` |
| proto 文件 | `onestop/service/v1/inspection_plan.proto` | `onestop/service/v1/periodic_plan.proto`（消息 `PeriodicPlan`，错误沿用 onestop_error） |
| 服务 tag | `OneStopInspectionPlanService` | `OneStopPeriodicPlanService`（`module_mapping.go` 同步） |
| 路由 | `/admin/v1/onestop/inspection-plans` | `/admin/v1/onestop/periodic-plans` |
| BFF 门面 | `i_onestop_inspection_plan.proto` | `i_onestop_periodic_plan.proto` |
| repo/service | `inspection_plan_repo.go` / `inspection_plan_service.go` | `periodic_plan_repo.go` / `periodic_plan_service.go` |

### 4.4 迁移草案（`00012_onestop_unified_type_config.sql`）

沿用仓内风格（character varying + COMMENT 逐字对齐 ent Comment、IF NOT EXISTS、成对 Down）：

```sql
-- +goose Up
-- 1) 岗位列合并：新增 execution_post，搬迁两旧列值后删除
ALTER TABLE public.onestop_service_types
    ADD COLUMN IF NOT EXISTS execution_post character varying(64);
UPDATE public.onestop_service_types
   SET execution_post = COALESCE(repair_post, transport_post)
 WHERE execution_post IS NULL;
ALTER TABLE public.onestop_service_types
    DROP COLUMN IF EXISTS repair_post,
    DROP COLUMN IF EXISTS transport_post;

-- 2) 标准项目库改名
ALTER TABLE public.onestop_service_types
    RENAME COLUMN inspection_items TO standard_items;

-- 3) 触发模式列
ALTER TABLE public.onestop_service_types
    ADD COLUMN IF NOT EXISTS trigger_mode character varying;

-- 4) 存量库按矩阵回填 SCHEDULED（种子守卫只在空表播种，已部署库靠这里；
--    前缀清单与 §7 播种矩阵一致，须同步维护）。只回填 SCHEDULED：
--    其余节点保持 NULL，由解析链默认按需（§4.2），不显式写 ON_DEMAND，
--    以免污染"NULL = 未设置，向上继承"的语义。
UPDATE public.onestop_service_types SET trigger_mode = 'TRIGGER_SCHEDULED'
 WHERE code LIKE '0102%' OR code LIKE '0103%'
    OR code LIKE '010401%' OR code LIKE '01040301%' OR code LIKE '01040303%'
    OR code LIKE '01040401%' OR code LIKE '010405%'
    OR code LIKE '010503%' OR code LIKE '010505%' OR code LIKE '010506%'
    OR code LIKE '01060201%' OR code LIKE '01060301%' OR code LIKE '01060502%'
    OR code LIKE '01080402%'
    OR code LIKE '01090102%' OR code LIKE '010903%'
    OR code LIKE '011001%' OR code LIKE '01100201%'
    OR code LIKE '01120402%' OR code LIKE '01120403%';

-- 5) 计划表改名（pkey 约束/索引/identity 序列一并改名，与 ent 全新建表命名对齐；
--    索引名是 ent 风格 <实体蛇形>_<字段>，序列名随表名——漂移守卫全量 dump diff，
--    任何一个名字不一致都会红）
ALTER TABLE public.onestop_inspection_plans RENAME TO onestop_periodic_plans;
ALTER TABLE public.onestop_periodic_plans
    RENAME CONSTRAINT onestop_inspection_plans_pkey TO onestop_periodic_plans_pkey;
ALTER INDEX IF EXISTS inspectionplan_service_type_id RENAME TO periodicplan_service_type_id;
ALTER INDEX IF EXISTS inspectionplan_freq_type RENAME TO periodicplan_freq_type;
ALTER SEQUENCE IF EXISTS public.onestop_inspection_plans_id_seq RENAME TO onestop_periodic_plans_id_seq;

-- 6) COMMENT（与 ent schema Comment 逐字一致，此处略去逐条）

-- +goose Down
ALTER TABLE public.onestop_periodic_plans RENAME TO onestop_inspection_plans;
ALTER SEQUENCE IF EXISTS public.onestop_periodic_plans_id_seq RENAME TO onestop_inspection_plans_id_seq;
ALTER INDEX IF EXISTS periodicplan_service_type_id RENAME TO inspectionplan_service_type_id;
ALTER INDEX IF EXISTS periodicplan_freq_type RENAME TO inspectionplan_freq_type;
ALTER TABLE public.onestop_periodic_plans
    RENAME CONSTRAINT onestop_periodic_plans_pkey TO onestop_inspection_plans_pkey;
ALTER TABLE public.onestop_service_types
    DROP COLUMN IF EXISTS trigger_mode;
ALTER TABLE public.onestop_service_types
    RENAME COLUMN standard_items TO inspection_items;
ALTER TABLE public.onestop_service_types
    ADD COLUMN IF NOT EXISTS repair_post character varying(64),
    ADD COLUMN IF NOT EXISTS transport_post character varying(64);
UPDATE public.onestop_service_types
   SET repair_post  = execution_post WHERE code LIKE '0101%';
UPDATE public.onestop_service_types
   SET transport_post = execution_post WHERE code LIKE '0106%';
ALTER TABLE public.onestop_service_types
    DROP COLUMN IF EXISTS execution_post;
```

> Down 只保证结构可回退（岗位值按子树前缀拆回两列），`trigger_mode` 的存量值不保留——
> dev 阶段可接受，Down 仅用于结构对齐。列 COMMENT 文本以 ent schema 为准同步写全，
> 漂移守卫（`make db-drift-check`）比对逐字一致。

### 4.5 ent schema 变更

- `ServiceType`：删 `repair_post` / `transport_post` / `inspection_items`，
  增 `execution_post`（MaxLen 64）、`trigger_mode`（Enum NamedValues：
  `OnDemand, "TRIGGER_ON_DEMAND"` / `Scheduled, "TRIGGER_SCHEDULED"`，Optional + Nillable）、
  `standard_items`（field.Strings，Comment 语义放宽）；`spare_parts` 仅改 Comment 文案。
- `InspectionPlan` → `PeriodicPlan`：实体改名、表名注解改 `onestop_periodic_plans`、
  字段全沿用（`post` 的 Comment 由「执行岗位（派单目标岗位）」保持，语义即通用）。
- 生成：`cd backend/onestop && make ent`（失败用 `go run entgo.io/ent/cmd/ent`，见运维备忘）。

## 5. 统一模板字段全表（配置详情的完整清单）

### A 派单与时效（所有小类/细类）

| 字段 | 列 | 类型 | 语义 | 校验 |
|---|---|---|---|---|
| 执行岗位 | `execution_post` | varchar(64) | 派单目标岗位（电工/运送员/保洁员/消控员…），工单派发路由依据 | 自由文本 + 建议值 |
| 默认优先级 | `default_priority` | 枚举 NORMAL/URGENT/EMERGENCY | 派单排序与超时预警加权 | 枚举 |
| 响应时限 | `response_time_limit` | uint32 分钟 | 受理 → 到场/取件/处置（文案按业态） | 1–43200 |
| 完成时限 | `completion_time_limit` | uint32 分钟 | 受理 → 完工/送达/办结（文案按业态） | 1–43200 |
| 标准工时 | `standard_work_hours` | double 小时 | 单次作业标准人时，负载评估与绩效核算 | > 0 |

### B 作业标准（所有小类/细类）

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 标准指导说明 | `guidance` | varchar(2048) | 作业 SOP 与安全注意事项（停电挂牌/双人作业/动火监护…） |
| 常见语句 | `common_phrases` | jsonb 数组 | 发起端快捷语句 + 语义归型语料（报修语句/呼叫语句/投诉选项） |
| 标准工具/装备 | `standard_tools` | jsonb 数组 | 出工标配（工具箱/轮椅/标本箱/清洁机具…） |
| 耗材/备件清单 | `spare_parts` | jsonb 数组 | 出工常装备件与耗材，配合库存领用 |

### C 验收方案（所有小类/细类）

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 验收方式 | `acceptance_modes` | jsonb 多选 SIGN/SCAN/PHOTO/DUAL | 报修=完工验收、运送=交接签收、客服=办结回访，可组合 |
| 验收项清单 | `acceptance_items` | jsonb `[{name,required}]` | 确认页逐项核对 checklist，required=false 为提示项 |

### D 专项属性（展示矩阵控制出现；存储层不限子树）

| 字段 | 列 | 类型 | 推荐子树 | 语义 |
|---|---|---|---|---|
| 温控要求 | `temperature_requirement` | 枚举 NORMAL/COLD/FROZEN/WARM | 0106 | 冷链 2-8℃ / 冷冻 / 保温 ≥60℃ |
| 需事前审批 | `requires_approval` | bool | 0106 / 0105 动火 / 0108 资源申请 | 发起前须审批 |
| 物品细目 | `cargo_types` | jsonb 数组 | 0106 | 承接物品范围，超范围另选类型 |
| 标准作业项目库 | `standard_items` | jsonb 数组 | 一切 SCHEDULED 类别 | 计划点表模板来源（巡检点/维保项目单/保洁作业标准/消杀范围） |

jsonb 数组列的 mask 行为不变：Create/Update 非空才 Set；mask 内空数组 = 显式清空（置 NULL）；
mask 不含该字段 = 不触碰。

## 6. 展示矩阵（CATEGORY_PROFILE）

前端常量表，键为中类编码（4 位）。**矩阵只管展示（可见性 + 文案），不管存储**；
新建中类（用户自定义 0113+）不在表内 → 回落全组可见 + 通用文案，统一模板兜底。

| 中类 | 履约组 | 响应时限文案 | 完成时限文案 | 验收语义 | 专项属性出现 | 默认触发模式 |
|---|---|---|---|---|---|---|
| 0101 报修维修 | ABC | 受理 → 到场（分钟） | 受理 → 完工 | 完工验收 | — | 按需 |
| 0102 计划维保 | ABC + standard_items | 计划内催办时限 | 计划内完成时限 | 维保验收单 | standard_items | **计划** |
| 0103 巡检巡查 | ABC + standard_items | 异常上报时限 | 异常处置时限 | 巡查记录确认 | standard_items | **计划** |
| 0104 保洁环卫 | ABC | 呼叫 → 到场 | 作业完成 | 保洁查验 | standard_items（计划型子树） | 混合（子树级，见 §7） |
| 0105 消防安全 | ABC | 警情 → 到场 | 处置完成 | 处置确认 | requires_approval（动火）、standard_items（检测/值守） | 混合 |
| 0106 运送配送 | ABCD | 呼叫 → 取件 | 取件 → 送达 | 交接签收 | 温控/审批/物品细目全组 | 混合 |
| 0107 秩序安保 | ABC | 接报 → 到位 | 处置完成 | 处置确认 | — | 按需 |
| 0108 客服服务 | ABC | 接单 → 首次响应 | 受理 → 办结 | 办结回访 | requires_approval（资源申请） | 按需 |
| 0109 能源环境 | ABC | 接报 → 排查到场 | 排查/整改完成 | 检测报告归档 | standard_items（检测/抄表） | 混合 |
| 0110 绿化美化 | ABC | 呼叫 → 到场 | 作业完成 | 养护查验 | standard_items | 混合 |
| 0111 应急保障 | ABC | 接警 → 到场 | 应急处置完成 | 处置确认 | — | 按需 |
| 0112 资产与工程 | ABC | —（少用） | —（少用） | 验收移交 | requires_approval、standard_items（盘点） | 混合 |

矩阵驱动项（每条 profile）：

```ts
interface CategoryProfile {
  groups: { dispatch: boolean; standard: boolean; acceptance: boolean; special: boolean };
  specialFields?: ('temperatureRequirement' | 'requiresApproval' | 'cargoTypes' | 'standardItems')[];
  labels?: Partial<Record<'responseTimeLimit' | 'completionTimeLimit' | ..., string>>;
  defaultTriggerMode?: 'TRIGGER_ON_DEMAND' | 'TRIGGER_SCHEDULED'; // 仅新建时的表单初值建议
}
```

## 7. `trigger_mode` 播种矩阵（种子 + 迁移回填共用的口径）

小类与其下全部细类都标；混合中类只标子树；其余节点留 NULL（解析默认按需）。
共 **105 个节点** SCHEDULED（80 个细类 + 25 个小类，细类占 211 的约 38%——
计划型里真正需要预置计划入口的子集，其余计划型类别可后续手动开启）。

| 子树 | SCHEDULED 原因 | 计划模板示例（种子预置） |
|---|---|---|
| 0102 全部（10 小类 29 细类） | 维保天然按法定周期 | 电梯半月维保（FREQ_MONTHLY month_days=[1,15]，口径见 §13-2）、消防设施月保、滤网季度清洗（FREQ_YEARLY months=[3,6,9,12]）、水箱清洗半年（months=[6]）、发电机月度试机 |
| 0103 全部（8 小类 20 细类） | 既有 9 套计划种子沿用 | 已有（每日防火巡查/月末检查/周一三五抄表/年度专项） |
| 010401 日常保洁、01040301/01040303 消杀预防、01040401 日常清运、010405 被服收发 | 定时巡回/定时清运 | 病区保洁每日两次（08:00/14:00）、四害消杀每月、被服收集每日巡回（times 多点） |
| 010503 检测、010505 值守、010506 演练 | 年检/值班交接/年度 | 消防年度检测（FREQ_YEARLY）、消控室交接班每日三次、疏散演练年度 |
| 01060201 / 01060301 / 01060502 | 定时班次（配合既有路线 departure_times） | 静配巡回配送计划（按路线班次） |
| 01080402 满意度调查 | 季度回访 | 季度满意度调查（FREQ_YEARLY months=[1,4,7,10]，口径见 §13-1） |
| 01090102 抄表、010903 检测 | 月度抄表/定期检测 | 能源抄表每月 1 号、水质检测季度 |
| 011001 绿养、01100201 绿植更换 | 季节性周期 | 浇灌每日、修剪每月、施肥季节性（FREQ_YEARLY months=[3,6,9,12]）、绿植更换每月 |
| 01120402 盘点、01120403 外包考核 | 月度/季度 | 库存月盘、外包履约月度考核 |

> 0102 计划模板注意：细类名"半月维保"是目录档位，计划频率按实际（FREQ_WEEKLY 双周需两条
> weekdays？——**不需要**：FREQ_WEEKLY 的 weekdays 语义是"周内哪几天"，双周表达不了；
> 半月维保的排期本期以 FREQ_MONTHLY month_days=[1,15] 表达，文档钉死该口径，工单调度器同口径实现）。

## 8. 配置继承（细类 → 小类回退）

统一模板铺开后，大量细类依赖小类层默认值，继承从"可选优化"升级为"模板可用性前提"，本期纳入。

**算法（读取端解析，写入端不聚合）：**

1. 取细类自身字段值；单值列 NULL 或数组列为空 → 取小类对应字段；
2. 数组列拼接语义不合并（细类整组覆盖或整组继承，不做数组内 merge）；
3. `trigger_mode` 同链解析：细类 → 小类 → TRIGGER_ON_DEMAND；
4. 两级都空 = 未配置，由工单对象按业态默认处理（P2 定义）。

**本期落地范围：**

- 前端配置详情页：细类为空的字段以占位样式显示小类继承值 + 「继承自上级」标记（列表数据
  已全量在前端，父链即 `parentId`，无新增接口）；
- 后端：不新增"生效配置"接口（`GET .../effective-config` 留给工单期，届时一次定义全局复用）；
- 种子：继续按"小类配工种层（岗位/SLA/指导），细类配作业层（工时/语句/工具/耗材）"分层预置。

## 9. 服务层校验规则（`service_type_service.go` / `periodic_plan_service.go`）

| 规则 | 层 | 行为 |
|---|---|---|
| 配置字段仅小类/细类可填 | 类型服务 | 大类/中类提交任一配置列（含 trigger_mode）→ 422（此前仅前端门控，统一后后端补挡） |
| 数值域沿用 | 类型服务 | 工时 > 0；时限 1–43200 分钟；HH:mm 格式与去重（计划侧） |
| trigger_mode 值域 | 类型服务 | 仅 ON_DEMAND / SCHEDULED |
| SCHEDULED 切回 ON_DEMAND | 类型服务 | 节点（含子树内）已挂周期计划 → 422「先删除计划」 |
| 计划挂载 | 计划服务 | 类别存在 + 为小类/细类 + 解析后 trigger_mode = SCHEDULED（替代 0103 前缀校验） |
| 路线挂载 | 路线服务 | 维持 0106 子树小类/细类不变（与触发方式无关：按需押运也需要路线模板） |
| jsonb mask 语义 | 仓储 | 不变（非空才 Set / mask 空数组置 NULL / repo 闭包不 Clear） |
| 计划 Update 合并视图校验 | 计划服务 | 沿用（未携带字段以库内值补齐后校验；当前周期必填执行日字段不可被 mask 显式清空） |

## 10. API / proto 变更

| 文件 | 变更 |
|---|---|
| `backend/api/protos/onestop/service/v1/service_type.proto` | `repair_post`/`transport_post` → `execution_post`；`inspection_items` → `standard_items`；新增 `TriggerMode` 枚举 + `trigger_mode` 字段；注释同步 |
| `backend/api/protos/onestop/service/v1/inspection_plan.proto` | 改名 `periodic_plan.proto`：消息 `InspectionPlan` → `PeriodicPlan`、服务 `OneStopInspectionPlanService` → `OneStopPeriodicPlanService`、路由 `/admin/v1/onestop/periodic-plans`；字段不变 |
| `backend/api/protos/admin/service/v1/i_onestop_service_type.proto` | 字段随源领域 |
| `backend/api/protos/admin/service/v1/i_onestop_inspection_plan.proto` | 改名 `i_onestop_periodic_plan.proto` |
| `pkg/constants/module_mapping.go` | 服务 tag 换名（仍挂 SYSTEM） |
| 生成 | `backend/api` 下 `buf generate` + `buf generate --template buf.onestop.gen.yaml`；`make ent`、`make openapi`、`make ts` |
| 接口同步 | 路由改名 = 新端点：已部署实例须 `make openapi` 重启后在管理页触发「接口同步」（fail-closed，未同步 403）。类型字段级改名不涉新端点 |

## 11. 前端设计（`frontend/admin/react`）

### 11.1 门控收敛

- `isRepairConfigurable / isInspectionConfigurable / isTransportConfigurable` →
  **`isConfigurable(code)`**：`code.length === 6 || code.length === 8`；
- 三处分节（报修/巡检/运送）→ **一个「履约配置」分节**，内部按 §5 四组渲染；
- 岗位建议值合并：`REPAIR_POST_SUGGESTIONS + TRANSPORT_POST_SUGGESTIONS` → `EXECUTION_POST_SUGGESTIONS`（去重并补保洁员/消控员/绿化工等）。

### 11.2 表单结构（ServiceTypeForm）

```
基本信息（编码/层级/名称/上级/描述/启用）     ← 不变
任务触发方式 trigger_mode（radio：按需发起 | 计划生成；继承态显示"继承上级：计划生成"）
「履约配置」分节（isConfigurable 时出现，组与文案由 CATEGORY_PROFILE 驱动）
  A 派单与时效：执行岗位(AutoComplete) / 优先级 / 响应时限 / 完成时限 / 标准工时
  B 作业标准：指导说明 / 常见语句(tags) / 标准工具(tags) / 耗材备件(tags)
  C 验收方案：验收方式(多选) / 验收项清单(编辑器)
  D 专项属性：温控 / 需审批 / 物品细目 / 标准作业项目库 —— 仅矩阵列出的字段渲染
—— 编辑态、解析后 SCHEDULED ——
「周期计划」面板（原 InspectionPlanPanel 改名 PeriodicPlanPanel，逻辑不变）
—— 编辑态、0106 子树 ——
「常用运送路线」面板（不变）
```

交互细节：

- trigger_mode 切到「计划生成」需先保存（面板要类别 id，与现有计划/路线面板同一约束）；
- 已挂计划时 radio 禁切「按需发起」，extra 提示「已挂 N 条计划，先删除」；
- 提交 payload：只带矩阵内字段（后端虽放开存储，前端仍按矩阵收口，避免保洁表单里冒出温控）。

### 11.3 列表页（index.tsx）

- 列收敛：原「维修岗位」/「运送岗位」情境列 → 「执行岗位」一列；
- 新增「触发模式」列：Tag「按需」（default）/「计划」（blue）；
- 优先级、验收摘要列沿用。

### 11.4 i18n

`locales/{zh-CN,en-US}/_modules/onestop.json`：删 repairPost/transportPost 分节 key，
增 executionPost / triggerMode / standardItems（标准作业项目库）与四组标题；
时限文案 key 按矩阵中类覆写（沿用 responseTimeLimitTransport 先例）。

## 12. 种子数据改造

| 文件 | 变更 |
|---|---|
| `service_types.json` | `repairPost`/`transportPost` → `executionPost`（值不变）；`inspectionItems` → `standardItems`；为 §7 矩阵节点补 `triggerMode`（仅 SCHEDULED 显式标记，其余留 NULL 走默认）；0102/0104/0105/0109/0110/0112 计划型小类补 `standardItems` + 履约参数（岗位/SLA/指导）行业模板（医院口径为主，商场/园区同构） |
| `inspection_plans.json` → `periodic_plans.json` | 既有 9 套沿用；新增 19 套跨子树模板（实际落地 28 套总量：电梯半月维保、消防月保、红外测温季度、发电机月度试机、水箱清洗半年、滤网季度、病区保洁每日、四害消杀每月、被服收发、消控交接班、静配巡回配送、快递分发、季度满意度、能源抄表、水质检测、绿化修剪、绿植更换、库存月盘、外包考核），四种周期全覆盖 |
| `transport_routes.json` | 不变 |
| 守卫测试 | `service_type_seed_test.go`：299 节点、execution_post 口径（小类岗位 11+5=16、细类工时 52）、standard_items 只出现在 SCHEDULED 子树、trigger_mode 矩阵钉死；`inspection_plan_seed_test.go` → `periodic_plan_seed_test.go` 计划数与频率类型覆盖 |

## 13. 澄清与既定口径（防实现期再议）

1. **"季度"如何表达**：频率模型无 FREQ_QUARTERLY。口径：季度 = FREQ_YEARLY 的
   months 多值（如 [1,4,7,10]）；月内日随 month_days。工单调度器同口径展开。
2. **"半月维保"如何表达**：FREQ_MONTHLY month_days=[1,15]；不用 FREQ_WEEKLY 表达双周
   （weekdays 语义是周内哪几天，表达不了双周）。
3. **细类名里的周期档位**（半月/季度/半年年度维保）：目录标签，实际排期以计划为准（§1.3 原则 3）。
4. **门岗值守/消控室值班**：交接班检查类（01050502）用每日 times 计划表达；**排班轮替
   （谁值哪个班）不在配置层**，属未来排班模块，本设计不承载。
5. **同一类型既按需又跑计划**（如会议室保洁定时 + 临时呼叫）：本期 trigger_mode 单值，
   该场景用"不同细类"拆分（日常保洁计划型 + 应急保洁按需型已天然拆好）；
   工单期若确需双触发，扩展为 jsonb 多选，存储与解析链已预留（NULL 继承语义不受影响）。
6. **0106 混合子树**：急送标本（01060202）按需、定时巡回（01060201）计划，计划性标在细类层。

## 14. 实施拆期

| 期 | 内容 | 验收 |
|---|---|---|
| **P1a 后端**（一个 commit） | 迁移 00012 + ent schema + proto/BFF 改名 + repo/service 改名 + 校验规则 + 种子改造 + 守卫测试 + `make openapi` | `go build/vet/test` 全绿；`make db-drift-check` 通过；老库（dev gwa）升级后 0103 计划无感可见 |
| **P1b 前端**（同一期） | 门控收敛 + CATEGORY_PROFILE + 表单/面板/列表/i18n + `make ts` | `npm run typecheck` 全绿；管理页「接口同步」后 periodic-plans 路由可用 |
| P2 工单对象（另立设计） | 调度器按计划生成任务、运行期继承与回退、ETA、逐级上报、`effective-config` 接口 | 另行定义 |

## 15. 风险与对策

| 风险 | 对策 |
|---|---|
| proto 字段/路由改名属 breaking | dev 阶段无外部消费方；前后端与 TS 同 commit 收口；「接口同步」重建 Api 表（含删旧 inspection-plans 端点） |
| 迁移 Down 的岗位值拆分丢信息（0101/0106 之外的 execution_post 回不去） | 可接受：Down 仅保结构；真需要时前缀规则可扩 |
| 展示矩阵维护漂移（矩阵与 trigger_mode 播种不一致） | 守卫测试同时钉两边：矩阵 SCHEDULED 集合 == 种子 triggerMode 集合 == 迁移 UPDATE 前缀集合 |
| 统一模板的 UX 噪音回潮（新中类回落全组可见） | 回落是兜底而非默认：新中类入字典时同步补矩阵行（文档化为新增中类的检查项） |
| 计划入口开放后误挂计划到按需类型 | 后端 trigger_mode 校验是唯一闸门（前端显隐只是引导），422 信息写明「先开启计划生成」 |

## 16. 本期明确不做

- 工单对象与调度器（计划 → 任务生成、运行期继承、ETA、上报）：P2；
- FREQ_QUARTERLY / FREQ_BIWEEKLY 枚举扩展：现有频率模型 + §13 口径足够，枚举扩展留给工单期按需；
- 排班轮替（班次人员编排）：排班模块；
- `transport_routes` 泛化为通用班次/路线资源：维持运送专属，二期评估并入计划模型；
- 计费/有偿、默认费用字段：等计费模型；
- `space_refs` / `equipment_refs` 启用：空间/设备对象模块落地后；
- trigger_mode 多选化：见 §13-5 预留，本期单值。

## 17. 实施偏差记录（2026-10-25 落地时修订）

| 偏差 | 原稿 | 实施口径 | 原因 |
|---|---|---|---|
| 计划表改名对象 | 仅表 + 索引（索引名误写为 `inspection_plans_*`） | 表 + pkey 约束 + 索引（ent 风格 `inspectionplan_*` → `periodicplan_*`）+ identity 序列 `onestop_inspection_plans_id_seq` 一并改名 | 漂移守卫是全量 pg_dump diff，约束/索引/序列名任何一个不一致都会红；索引名以 ent 全新建表命名对齐 |
| 路线挂载门控 | 0106 子树 + 解析后 SCHEDULED | 维持 0106 子树小类/细类不变，与触发方式无关 | 按需押运（如搬入搬出）同样需要路线模板；仅定时班次路线参与后续任务生成，属工单期语义 |
| 计划种子规模 | 示例清单约 14 套 | 28 套（沿用 9 + 新增 19），覆盖维保/保洁/消杀/值守/班次运送/回访/抄表/绿养/盘点考核 | 0102 计划维保按小类逐项配模板更贴近行业目录；四种周期全覆盖 |
| 表单 standardItems 提交时机 | 常规随表单提交 | 仅解析后（或显式选择）为 SCHEDULED 时携带该字段 | 避免「按需」类型残留项目库数据造成语义噪音 |
| 计划表 post 注释 | 沿用「执行岗位（派单目标岗位）」 | 同文案，语义注明覆盖类别执行岗位 | 计划级岗位覆盖类型级 execution_post，是运行期任务派发口径 |
| trigger_mode 回填 | 草稿一度含"其余节点显式写 ON_DEMAND" | 只回填 SCHEDULED，其余保持 NULL | 显式 ON_DEMAND 会污染「NULL = 未设置、向上继承」语义（已定稿口径，此处留档） |

验证记录：`make db-drift-check` 通过（迁移链 vs ent 全量建表一致）；dev 库 gwa 应用
00012 后 299 节点、岗位合并/回填矩阵/计划表改名全部生效（该库字典为旧快照，
SCHEDULED = 107 = 矩阵 105 + 两个历史多余节点，属存量差异非迁移缺陷）；
`go build/vet` 与 onestop/主仓测试全绿（oss MinIO live 与 org_unit sqlite 两项为
HEAD 既有失败，干净 worktree 复现确认）；react `pnpm run typecheck` 全绿。
