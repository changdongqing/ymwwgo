# 一站式「统一工单底座」详细设计

> 状态：**已实施**（2026-10-26，P2a/P2b/P2c 落地；实施偏差见 §15）｜编写日期：2026-10-25｜
> 范围：`backend/onestop`（工单对象 + 调度器 + 类型层少量新列）+ `frontend/admin/react` 工单管理
>
> 前置阅读：[onestop-unified-service-type-config.md](./onestop-unified-service-type-config.md)
> （统一配置模板，P1，已实施——本文多处沿用其术语与既定口径）、
> `backend/onestop/README.md`（模块接线方式）、
> `docs/task_system.md`（调度器挂载的宿主机制）、
> `docs/tenant_isolation.md`（租户列与 Policy）、
> `docs/database/design/ent-goose-migrations.md`（迁移铁律）。

## 1. 背景与结论

### 1.1 从类型到实例

服务类型字典（299 节点）、统一履约配置模板（15 列）、周期计划、运送路线已在 P1 全部落地。
配置层回答的是「这类服务**怎么约定**」；工单层回答的是「这类服务**怎么干」——
派给谁、按什么时限、执行到哪一步、结果与验收如何、异常怎么流转。

工单是 onestop 模块第一个**租户业务数据**实体（类型/计划/路线目前是平台级配置），
也是 P1 设计文档多处预告的「未来工单对象」（effective-config 接口、任务生成策略、
common_phrases 语义归型语料的消费方）。

### 1.2 上一轮分析结论（本设计的输入）

对 211 个细类逐类核对的结论：**差异真实存在，但差异单位不是「业务类别」而是
「履约过程形状」**。按状态机骨架归并，全部细类落入三种履约形态：

| 履约形态 | 状态机骨架 | 细类数 | 典型子树 |
|---|---|---|---|
| 事件处置 EVENT | 受理 → 派单 → 到场/处理 → 完工 → 验收 →（回访） | 122 | 报修维修全部、应急保障、突发事件、投诉问询、应急保洁 |
| 计划任务 TASK | 计划生成 → 逐项执行记录 → 异常上报（可派生事件单） → 确认 | 77 | 计划维保、巡检巡查、日常保洁/消杀、抄表检测、绿养、盘点 |
| 运送 TRANSPORT | 呼叫/班次 → 取件签收 → 在途 → 送达签收（验收内嵌） | 12 | 运送配送全部（急送/陪检/巡回/押运） |

巡检工单、维保工单、保洁作业单、抄表单**不是多种工单，是同一形态挂不同类型与计划**——
它们的差异已被配置层吃掉（standard_items 项目库、计划点表、验收方式+验收项）。

不进工单层的（防止范围膨胀，P1 已定调 + 本文重申）：

| 出局项 | 去处 |
|---|---|
| 审批（动火/资源申请/押运/备件申领） | 工单状态机的**可选前置段**（`requires_approval` 驱动），不是独立工单形态 |
| 项目型（大修改造/零星工程/节能改造） | 未来「项目对象」；类型字典顶部大类 `02`/`03` 预留其他单据类型 |
| 排班轮替（谁值哪个班） | 排班模块（P1 设计 §13-4 已钉死） |
| 台账登记类（访客登记/设备建档调拨报废/监控调阅） | 登记不是履约，未来台账/资产对象 |

### 1.3 本设计回答的三个问题

**Q1：三种形态是三张工单表吗？**
**不是，一张主表。** `order_kind` 列驱动状态机分支、界面分支与专项字段出现，
与 P1 用 `trigger_mode` + 展示矩阵代替三套配置分节是同一条路线。三张表的代价
（跨形态转单映射、KPI 三表 UNION、移动端三套）远大于收益。

**Q2：类型差异在工单上怎么落地？**
**创建时快照，运行期不回读。** 工单从类型解析 effective-config（细类 → 小类回退，
P1 §8 预留的接口本期兑现），把参与运行期判定的字段快照落库；此后类型改配置
不影响存量单（历史 SLA 判定、验收标准都以开单时刻为准）。

**Q3：计划/路线怎么变成单？**
**调度器展开。** 挂载在宿主 asynq 任务系统上的系统级常驻任务，按频率模型
滚动预生成未来 24h 的 TASK 单与 TRANSPORT 班次单，幂等唯一约束防重。

### 1.4 设计原则

1. **统一底座 + 形态分支**：一套表、一套状态机、一套动作 API；差异由
   `order_kind`（履约维度）与既有 `trigger_mode`（触发维度）两个正交开关表达。
2. **快照不回读**：运行期判定/统计字段创建时快照（SLA/岗位/优先级/验收/点表/路线）；
   纯参考文案（guidance）实时读类型，不落快照。
3. **计划与路线是排期唯一事实源**（P1 原则 3 延续）：工单只引用 + 快照；
   计划后续修改不影响已生成单。
4. **跟进流程 = 事件流**：所有动作落 `onestop_work_order_events`，时间线即跟进记录，
   主表只存当前态与关键时间戳。
5. **工单是租户数据**：挂 `TenantID` mixin 与租户 Policy；作为前置，计划/路线表
   本期补挂租户列（schema 注释里「工单对象落地时一并评估租户态」的预约兑现）。
6. **不吞错、contains 搜索、CRUD 包 `data`、动作 RPC 扁平 body**（全仓铁律 1/3 原样适用）。

## 2. 术语

| 术语 | 含义 |
|---|---|
| 工单 | 一次服务履约的实例：从受理到完结的全生命周期记录（onestop_work_orders） |
| 履约形态 `order_kind` | 工单的履约过程形状：KIND_EVENT 事件处置 / KIND_TASK 计划任务 / KIND_TRANSPORT 运送 |
| 生单来源 `source` | 单子怎么来的：手动（坐席代录）/ 计划展开 / 路线班次 / 转单 / 派生 |
| 发起渠道 `channel` | 手动单从哪进来：坐席台 / 电话（小程序、开放 API 预留枚举位） |
| effective-config | 类型节点解析后的生效履约配置（细类空字段回退小类，P1 §8 机制） |
| 快照 | 创建工单时从类型/计划/路线复制落库的值，此后不再随源变化 |
| 执行项 | TASK 单的逐项作业记录（计划点表展开或类型项目库快照，onestop_work_order_items） |
| 事件 | 工单时间线上的一次动作记录（派单/到场/取件/送达/完工/验收/催办…） |
| 转单 TRANSFER | 关闭当前单，按新类型开新单（source_order_id 关联回来） |
| 派生 DERIVE | 当前单不动，关联生成新单（典型：巡检执行项异常 → 派生报修单） |
| SLA 双时限 | 响应时限（受理 → 到场/取件）与完成时限（受理 → 完工/送达/办结），快照 + deadline 落库 |

## 3. 设计总览

```
配置层（P1 已落地，全部平台级字典 + 本期租户化的计划/路线）
onestop_service_types ── 统一履约配置模板（15 列 + trigger_mode + 本期新增 order_kind）
onestop_periodic_plans ── 周期计划（频率/岗位/点表/生效期）        ← 本期补挂 tenant_id
onestop_transport_routes ─ 运送路线（起终点/时长/往返/班次时刻）    ← 本期补挂 tenant_id
        │
        │ ① effective-config 解析（细类→小类回退；ResolveTriggerMode / ResolveOrderKind）
        ▼
┌─────────────────────────────────────────────────────────────────┐
│ 工单层（本期）                                                     │
│                                                                   │
│  手动链路：坐席台/电话 ──Create──▶ onestop_work_orders             │
│            （快照 + requires_approval ? 先进审批段）                │
│  自动链路：asynq 系统级任务 onestop_plan_expand（每分钟）            │
│            ├─ 计划展开 ──▶ TASK 单（点表展开为执行项）               │
│            └─ 路线班次展开 ──▶ TRANSPORT 班次单                    │
│                                                                   │
│  onestop_work_orders（主表，order_kind 驱动）                      │
│  ├── onestop_work_order_items（执行项：TASK 逐项结果/读数/照片）     │
│  └── onestop_work_order_events（事件流：状态迁移+跟进动作=时间线）    │
│                                                                   │
│  动作 RPC（扁平 body）：dispatch/arrive/pickup/deliver/complete/    │
│  verify/cancel/urge/reassign/transfer/derive/abnormal/approve/     │
│  satisfaction —— 全部落 events，主表只存当前态                      │
└─────────────────────────────────────────────────────────────────┘
        │
        ▼ 明确不做（本期）：SSE/通知推送、KPI 看板、移动端、发起端自助、
          AI 归型、库存联动、计费、实时轨迹 —— 见 §13
```

两个正交维度（P1 §4.2 的延续）：

| | 按需发起 ON_DEMAND | 计划生成 SCHEDULED |
|---|---|---|
| **事件 EVENT** | 报修/投诉/应急（坐席建单，PENDING 起步） | —— |
| **计划任务 TASK** | 计划漏单手工补（允许，无计划关联） | 维保/巡检/保洁/抄表（生成即派岗） |
| **运送 TRANSPORT** | 急送/陪检/押运（坐席建单） | 巡回配送/班次（路线 departure_times 或计划 times 展开） |

`trigger_mode` 管「怎么生单」（P1 已有），`order_kind` 管「单子怎么履约」（本期新增），
二者独立解析、互不推导——除了默认推导规则（§4.2）共享对类型树的一次读取。

## 4. 履约形态与类型解析

### 4.1 类型表新增 `order_kind` 列

`onestop_service_types` 增 `order_kind`（与 `trigger_mode` 同款：varchar 存储
`KIND_EVENT / KIND_TASK / KIND_TRANSPORT`，NULL = 未设置，小类/细类两级可填，
细类回退小类；大类/中类纯结构节点提交该字段 422，沿用 P1 门控）。

**解析链与默认推导**（`ResolveOrderKind`，与 `ResolveTriggerMode` 同款实现，
前后端同口径双实现）：

1. 细类自身值；NULL 则取小类值；
2. 两级都 NULL 时按推导规则给默认值：
   - 编码 `0106` 子树（运送配送）→ `KIND_TRANSPORT`；
   - 否则解析后 `trigger_mode = SCHEDULED` → `KIND_TASK`；
   - 否则 → `KIND_EVENT`；
3. 显式设置永远优先于推导（这就是该列存在的意义——处理推导覆盖不了的特例）。

**种子与迁移回填的显式覆写只有一处**：`010405` 被服收发（小类 + 2 细类）标
`KIND_TRANSPORT`——「脏被服收集/净被服发放」本质是巡回收送交接（双地点、有签收），
按业务形状归运送，尽管它挂在 0104 保洁环卫下。守卫测试三处同步钉死
（种子 / 迁移回填 / 推导规则文档），与 P1 trigger_mode 矩阵同一维护纪律。

其余 208 个细类零配置：0102/0103 等计划型推导为 TASK，0106 推导为 TRANSPORT，
其余推导为 EVENT，与 §1.2 的 122/77/12 分布一致。

### 4.2 三形态定义

| | KIND_EVENT 事件处置 | KIND_TASK 计划任务 | KIND_TRANSPORT 运送 |
|---|---|---|---|
| 状态机起点 | PENDING（或 APPROVING） | DISPATCHED（生成即派岗） | PENDING（手动）/ DISPATCHED（班次） |
| 响应事件 | 到场 arrive | 到场 arrive | 取件签收 pickup |
| 完成事件 | 完工 complete | 逐项结果提交 complete | 送达签收 deliver（=完工+验收合一） |
| 验收 | 完工后独立验收 verify | 作业记录确认（验收项走类型配置） | 双签收内嵌，无独立验收步 |
| 专项面板 | 备件/耗材、结果照片 | 执行项清单（读数/逐项结果） | 起终点/路线/温控/物品/两段签收 |
| 执行项子表 | 不用 | 用（计划点表或项目库展开） | 不用 |

**边界案例口径（上一轮分析遗留三问，此处钉死）**：

1. **被服收发（010405）** = TRANSPORT + SCHEDULED：生单来源是挂在 010405 细类上的
   周期计划（times 表达巡回时刻），单据形态走取件/送达双签收；
2. **培训演练（010506）、满意度调查（01080402）** = TASK 默认推导：执行项可为空
   （计划点表/项目库为空时生成为空清单，complete 时无逐项约束），单据即活动留痕；
3. **回访（01080401 工单完结回访）** = 不预生成：任意 CLOSED 单可补录满意度
   （`satisfaction` 动作）；01080402 满意度调查作为 TASK 计划单批量生成回访任务，
   结果回填到被回访单的 satisfaction 字段（payload 关联），自身走 TASK 生命周期。

### 4.3 effective-config 接口（P1 §8 预留，本期兑现）

```
GET /admin/v1/onestop/service-types/{id}/effective-config
```

挂在 `OneStopServiceTypeService`（资源属主），入参 id 须为小类/细类（其余层级 422）。
响应 `EffectiveServiceConfig`：解析后的 trigger_mode、order_kind + 15 个配置字段的
生效值。实现即一个纯函数 + 两次查库（细类、小类），工单创建服务内部复用同一函数，
前端发起表单也用它预填（岗位/优先级/验收提示）。

## 5. 数据模型

### 5.1 主表 `onestop_work_orders`

按组列出全部业务列（mixin 列见 §5.9；jsonb 数组语义沿用 go-crud：NULL = 未填）：

**标识与归属**

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 工单号 | `order_no` | varchar(32) UNIQUE NOT NULL | `WO-` + 8 位序列（§5.6） |
| 服务类型 | `service_type_id` | uint32 NOT NULL | 细类（LEVEL_DETAIL），服务层校验；无 ent 外键（字典引用完整性服务层把关，plan 同款） |
| 类型编码快照 | `service_type_code` / `service_type_name` | varchar(8) / varchar(128) | 字典只停用不删除，但快照使列表/导出不依赖 join 且免受改名影响 |
| 履约形态 | `order_kind` | enum KIND_* NOT NULL | 创建时 `ResolveOrderKind` 解析落库，不接受请求指定（转类型时按新类型重解析） |
| 生单来源 | `source` | enum SOURCE_* NOT NULL | SOURCE_MANUAL / SOURCE_PLAN / SOURCE_ROUTE / SOURCE_TRANSFER / SOURCE_DERIVED |
| 发起渠道 | `channel` | enum（可空） | CHANNEL_DESK 坐席台 / CHANNEL_PHONE 电话；CHANNEL_MINI_APP、CHANNEL_OPEN_API 预留枚举位 |
| 来源单 | `source_order_id` | uint32（可空） | 转单/派生时的旧单 id |

**发起信息（手动单填写；计划单由计划名/类别生成摘要）**

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 发起人 | `requester_name` / `requester_phone` | varchar(64) / varchar(32) | 自由文本（电话呼叫方常无账号） |
| 发起用户 | `requester_user_id` | uint32（可空） | 登录用户发起时记录；坐席代录时发起人是最终用户，坐席身份在 created_by |
| 位置 | `location` | varchar(256) | 楼/层/房间自由描述；`space_ref` 预留（空间对象模块落地后启用，对齐计划表口径） |
| 服务对象 | `equipment_desc` | varchar(256)（可空） | 报修的故障设备/设施描述（与现象分开）；设备 ref 同上预留 |
| 描述 | `description` | varchar(1024) NOT NULL | 需求/现象描述（一句话也算） |
| 现场照片 | `images` | jsonb string[] | 发起时照片 |
| 命中语句 | `common_phrase` | varchar(128)（可空） | 发起时选中的快捷语句（AI 归型语料回流预留） |

**派单与执行**

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 执行岗位 | `execution_post` | varchar(64) | effective-config 快照（计划单优先计划级 post） |
| 执行人 | `assignee_id` / `assignee_name` | uint32 / varchar(64)（均可空） | 岗位派单时无人，接单（CLAIM，预留）后补；name 快照防改名影响历史 |
| 派单时间 | `assigned_at` | timestamp（可空） | dispatch 动作时间；计划/班次单 = 生成时刻 |
| 计划执行时刻 | `planned_exec_at` | timestamp（可空） | TASK/班次单必填（计划时刻或发班时刻）；EVENT 单为空 |
| 来源计划/路线 | `plan_id` / `route_id` | uint32（均可空） | 无 ent 外键，服务层校验；调度幂等键组成部分 |

**SLA（快照 + deadline 落库）**

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 优先级 | `priority` | enum PRIORITY_* | 快照自 default_priority，Create/派单可显式覆盖 |
| 双时限 | `response_time_limit` / `completion_time_limit` | uint32 分钟（可空） | effective-config 快照；空 = 该单不限时 |
| 双 deadline | `response_deadline` / `completion_deadline` | timestamp（可空） | 创建时计算落库（起算口径 §5.2），列表超时过滤/红黄标不用现算 |
| 关键时间戳 | `intake_at` / `responded_at` / `completed_at` / `verified_at` | timestamp | 受理（手动单=建单；计划单=生成）/ 到场·取件 / 完工·送达 / 验收通过 |

**状态与跟进**

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 状态 | `status` | enum STATUS_* NOT NULL | 七状态统一状态机（§5.2） |
| 撤销原因 | `cancel_reason` | varchar(256)（可空） | cancel/审批驳回/转单关闭时必填（转单=「转单至 WO-xxx」自动生成） |
| 异常标记 | `has_abnormality` | bool default false | 事件 ABNORMAL 或执行项 ITEM_ABNORMAL 置位，列表过滤用 |
| 催办 | `urge_count` | uint32 default 0 | events 有明细，主表冗余计数便于列表 |

**运送专项（KIND_TRANSPORT 才填；存储层不校验 kind，可见性由前端形态面板控制——与 P1 专项属性组同思路）**

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 起终点 | `origin` / `destination` | varchar(128) | 手动单必填；班次单快照自路线；via_points 途经点 jsonb |
| 往返 | `round_trip` | bool default false | 路线快照 |
| 预计时长 | `est_duration_minutes` | uint32（可空） | 路线快照，ETA 基准 |
| 温控 | `temperature_requirement` | enum TEMP_*（可空） | effective-config 快照 |
| 物品 | `cargo_desc` | varchar(256) | 本单实际承运物品（类型级 cargo_types 是承接范围，单级是实例） |
| 取件签收 | `picked_up_at` / `picked_signer` / `picked_photos` | timestamp / varchar(64) / jsonb | pickup 动作落 |
| 送达签收 | `delivered_at` / `delivered_signer` / `delivered_photos` | 同上 | deliver 动作落 |

**完工与验收（EVENT/TASK）**

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 完工描述 | `result_desc` | varchar(1024) | complete 动作提交 |
| 完工照片 | `result_photos` | jsonb string[] | |
| 备件耗材 | `used_materials` | jsonb `[{name,qty,unit,remark}]` | 维修领用记录，本期手工填（库存联动出局）；jsonb 而非子表：单均 1-5 项，无独立查询需求 |
| 实际工时 | `actual_work_hours` | double（可空） | complete 时手填，绩效核算预留 |
| 验收快照 | `acceptance_modes` / `acceptance_items` | jsonb | effective-config 快照（方式多选 + `[{name,required}]` 清单） |
| 验收结果 | `acceptance_result` | jsonb `[{name,passed,remark}]` | verify 动作逐项勾选结果 |
| 验收附件 | `acceptance_photos` / `acceptance_signer` | jsonb / varchar(64) | |

**回访**

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 满意度 | `satisfaction_score` / `satisfaction_comment` / `revisited_at` | uint32(1-5) / varchar(512) / timestamp | satisfaction 动作补录，仅 CLOSED 单 |

### 5.2 状态机与 SLA 起算口径

**七状态**（枚举存储值与 proto 值名一致，供 EnumTypeConverter 直转，全文同此约定）：

| 状态 | 语义 | 谁在动 |
|---|---|---|
| `STATUS_PENDING` 待派单 | 已受理未派（手动单起步态） | 坐席 |
| `STATUS_APPROVING` 审批中 | requires_approval 的单在建单后先走审批段 | 审批人 |
| `STATUS_DISPATCHED` 已派单 | 已派到岗位/人（或计划单生成即派岗），待到场/取件 | 执行人 |
| `STATUS_PROCESSING` 执行中 | 已到场/已取件，作业进行中 | 执行人 |
| `STATUS_COMPLETED` 已完工 | EVENT/TASK 完工提交，待验收 | 坐席/发起人 |
| `STATUS_CLOSED` 已完结 | 验收通过；TRANSPORT 送达签收直接进入 | ——（终态） |
| `STATUS_CANCELLED` 已撤销 | 撤销/审批驳回/转单关闭 | ——（终态） |

**SLA 起算口径（deadline 计算的唯一事实源，写死防实现期再议）**：

- 手动单（EVENT/TRANSPORT 按 ATT）：`intake_at = created_at`（坐席建单即受理）；
  `response_deadline = intake_at + response_time_limit`，
  `completion_deadline = intake_at + completion_time_limit`；
- 计划/班次单（TASK/TRANSPORT 按 PLAN/ROUTE）：无受理动作，
  `response_deadline = planned_exec_at + response_time_limit`
  （对应矩阵文案「计划内催办时限」），completion 同理以 planned_exec_at 起算；
- 时限快照为空的：对应 deadline 落 NULL，不参与超时判定；
- 验收打回返工（COMPLETED → PROCESSING）：deadline **不重算**，仍以原 deadline 判超
  （返工时间消耗是事实，不因打回而豁免）；
- 超时判定 = `now > deadline` 且相应节点未达成（responded_at / completed_at 为空），
  由列表查询表达式实现（`overdue` 过滤参数），不做定时扫描。

### 5.3 状态迁移矩阵（服务层唯一合法性判据）

| 动作 | 允许的当前状态 | 目标状态 | 主表落点 |
|---|---|---|---|
| Create（无需审批） | — | PENDING | intake_at |
| Create（requires_approval） | — | APPROVING | |
| approve（通过） | APPROVING | PENDING | |
| approve（驳回） | APPROVING | CANCELLED | cancel_reason=审批驳回 |
| dispatch 派单 | PENDING | DISPATCHED | assigned_at、岗位/执行人 |
| reassign 改派 | DISPATCHED / PROCESSING | DISPATCHED | 岗位/执行人更新（PROCESSING 改派视同撤回重派） |
| arrive 到场 | DISPATCHED（EVENT/TASK） | PROCESSING | responded_at |
| pickup 取件签收 | DISPATCHED（TRANSPORT） | PROCESSING | responded_at=picked_up_at、签收人/照片 |
| deliver 送达签收 | PROCESSING（TRANSPORT） | **CLOSED** | delivered_at=completed_at=verified_at、签收人/照片 |
| complete 完工提交 | PROCESSING（EVENT/TASK） | COMPLETED | result_*、completed_at；TASK 要求全部执行项非 ITEM_PENDING |
| verify 验收通过 | COMPLETED | CLOSED | verified_at、验收结果/签收人 |
| verify 驳回 | COMPLETED | PROCESSING | 返工（deadline 不重算） |
| cancel 撤销 | PENDING / APPROVING / DISPATCHED / PROCESSING | CANCELLED | cancel_reason 必填 |
| urge 催办 | 任意非终态 | 不变 | urge_count+1 |
| abnormal 异常上报 | 任意非终态 | 不变 | has_abnormality=true |
| transfer 转单 | 任意非终态 | CANCELLED | 旧单关闭（reason 自动），新单 source=TRANSFER |
| derive 派生 | 任意（含 CLOSED） | 不变 | 新单 source=DERIVED |
| satisfaction 回访补录 | CLOSED | 不变 | score/comment/revisited_at |

矩阵之外的（from, action）组合一律 409（非法状态迁移），错误信息带当前状态与
合法动作清单。**TRANSPORT 无独立验收步**（送达签收即完结）是唯一按形态收窄的
转移，理由：双签收即验收，多一步坐席确认无业务价值；验收项如需留证，
deliver 动作的 photos 承载。

### 5.4 子表 `onestop_work_order_events`（时间线）

所有动作的不可变记录，按时间倒序即详情页时间线，也是跟进流程的数据载体
（P1 README 预告的「工单对象及其跟进流程」落点）：

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 工单 | `work_order_id` | uint32 NOT NULL | 索引；无 ent 外键（同表删除策略：工单硬删级联服务层处理） |
| 动作 | `action` | enum ACTION_* NOT NULL | ACTION_CREATE / DISPATCH / REASSIGN / ARRIVE / PICKUP / DELIVER / COMPLETE / VERIFY_PASS / VERIFY_REJECT / CANCEL / URGE / ABNORMAL / TRANSFER / DERIVE / APPROVE_PASS / APPROVE_REJECT / CLAIM（枚举位预留，本期不实现） |
| 状态迁移 | `status_from` / `status_to` | enum（可空） | 不改状态的动作（urge/abnormal/derive）两值相等或为空 |
| 备注 | `note` | varchar(512)（可空） | 动作附言 |
| 照片 | `photos` | jsonb string[] | 动作留证 |
| 动作附件 | `payload` | jsonb（可空） | 动作扩展数据（签收人、改派目标、转单新单号、派生新单号等），结构由各动作服务层约定 |
| 操作人 | OperatorID mixin 的 `created_by` | | 兼任操作人；系统动作（调度器生成）= 系统上下文 |

事件表只增不改不删（写错不修，补一条更正事件——本期无此需求，仅原则）。

### 5.5 子表 `onestop_work_order_items`（执行项）

TASK 单的逐项作业记录，创建时快照生成，执行中逐项回填：

| 字段 | 列 | 类型 | 语义 |
|---|---|---|---|
| 工单 | `work_order_id` | uint32 NOT NULL | 索引 |
| 分组 | `scope` | enum（可空） | SCOPE_SPACE / SCOPE_EQUIPMENT（计划单两组点表各自展开）；手动 TASK 单从类型 standard_items 展开，scope 为空 |
| 名称 | `name` | varchar(128) NOT NULL | 点表项名称（快照） |
| 序 | `sort` | uint32 | 展开时按点表顺序落库 |
| 结果 | `result` | enum default ITEM_PENDING | ITEM_PENDING 未执行 / ITEM_NORMAL 正常 / ITEM_ABNORMAL 异常 / ITEM_SKIPPED 跳过（须 explicit） |
| 读数/值 | `value` | varchar(128)（可空） | 抄表读数、测量值等 |
| 照片 | `photos` | jsonb string[] | 该项留证 |
| 备注 | `remark` | varchar(512)（可空） | |
| 对象 ref | `space_ref` / `equipment_ref` | varchar(64)（可空，预留） | 空间/设备对象模块落地后启用 |

**生成规则**：计划展开单 = 计划 `space_items`（scope=SPACE）+ `equipment_items`
（scope=EQUIPMENT）中 `enabled=true` 的项；手动 TASK 单 = 类型 `standard_items`
全部项（scope 空）。`ITEM_ABNORMAL` 提交时联动主表 `has_abnormality=true`，并可
一键 derive 报修单（前端把执行项名称/备注带入新单 description）。

**complete 的前置约束**：TASK 单存在 `result=ITEM_PENDING` 的执行项时 422
（要么逐项给结果、要么显式 SKIPPED——不允许漏项完工）。

### 5.6 工单号

`order_no = 'WO-' || lpad(nextval('onestop_work_order_no_seq')::text, 8, '0')`
（序列 START 100000，PG 全局序列，服务层取号后填入）。取舍写明：**全局序列跨租户
共享**，单号单调递增利于排序，但理论上可从单号推测平台总量——工单号非敏感凭据，
可接受；租户隔离靠数据层闸门，不靠单号。若未来要求租户不可推测，再改
「租户前缀 + 租户内序列」，order_no 列宽 32 已预留。

### 5.7 快照清单（快什么、不快什么）

| 快照 | 来源 | 理由 |
|---|---|---|
| execution_post / priority / 双时限 / acceptance_modes / acceptance_items / temperature_requirement | 类型 effective-config | 运行期判定与验收标准以开单时刻为准；类型配置后续修订不追溯存量单 |
| service_type_code / service_type_name / assignee_name / picked_signer 等人名 | 类型/人员 | 展示免 join，防改名/停用影响历史 |
| 计划单：post、点表展开为执行项行 | 计划 | 计划是排期事实源，但已生成单是事实快照（P1 原则 3 的工单侧体现） |
| 班次单：origin/destination/via_points/round_trip/est_duration_minutes | 路线 | 同上 |
| **不快照**：guidance / common_phrases / standard_tools / spare_parts | 类型实时读 | 纯参考文案，类型修订后新旧单都展示最新 SOP 反而正确；发起端表单用 effective-config 现取 |

### 5.8 迁移草案（`00013_onestop_work_orders.sql`）

沿用仓内风格（character varying + COMMENT 与 ent Comment 逐字一致、IF NOT EXISTS、
成对 Down、一个迁移承载同一主题的全部结构变更）：

```sql
-- +goose Up

-- 1) 类型表新增履约形态列（NULL=未设置，细类回退小类；大类/中类提交该字段 422 由服务层挡）
ALTER TABLE public.onestop_service_types
    ADD COLUMN IF NOT EXISTS order_kind character varying;
COMMENT ON COLUMN public.onestop_service_types.order_kind IS '履约形态 KIND_EVENT:事件处置 KIND_TASK:计划任务 KIND_TRANSPORT:运送（未设置回退上级；两级都空按 0106 子树/计划生成/事件顺序推导）';

-- 2) 存量库覆写回填：被服收发按业务形状归运送（与种子同口径，守卫测试三处同步）
UPDATE public.onestop_service_types SET order_kind = 'KIND_TRANSPORT'
 WHERE code LIKE '010405%';

-- 3) 计划/路线租户化：工单是租户业务数据，其生单来源必须先归属租户
--    （P1 schema 注释「工单对象落地时一并评估租户态」的兑现；默认租户 id 实施时按现网确认）
ALTER TABLE public.onestop_periodic_plans
    ADD COLUMN IF NOT EXISTS tenant_id bigint;
UPDATE public.onestop_periodic_plans SET tenant_id = 1 WHERE tenant_id IS NULL;
ALTER TABLE public.onestop_periodic_plans ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS periodicplan_tenant_id ON public.onestop_periodic_plans (tenant_id);

ALTER TABLE public.onestop_transport_routes
    ADD COLUMN IF NOT EXISTS tenant_id bigint;
UPDATE public.onestop_transport_routes SET tenant_id = 1 WHERE tenant_id IS NULL;
ALTER TABLE public.onestop_transport_routes ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS transportroute_tenant_id ON public.onestop_transport_routes (tenant_id);

-- 4) 工单号序列
CREATE SEQUENCE IF NOT EXISTS public.onestop_work_order_no_seq START 100000;

-- 5) 主表（COMMENT 全量逐字对齐 ent Comment，此处列结构；时间/操作人列随 mixin）
CREATE TABLE IF NOT EXISTS public.onestop_work_orders (
    id                   bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    order_no             character varying(32)  NOT NULL,
    service_type_id      bigint                 NOT NULL,
    service_type_code    character varying(8),
    service_type_name    character varying(128),
    order_kind           character varying      NOT NULL,
    source               character varying      NOT NULL,
    channel              character varying,
    source_order_id      bigint,
    requester_name       character varying(64),
    requester_phone      character varying(32),
    requester_user_id    bigint,
    location             character varying(256),
    equipment_desc       character varying(256),
    description          character varying(1024) NOT NULL,
    images               jsonb,
    common_phrase        character varying(128),
    priority             character varying,
    execution_post       character varying(64),
    assignee_id          bigint,
    assignee_name        character varying(64),
    assigned_at          timestamptz,
    planned_exec_at      timestamptz,
    plan_id              bigint,
    route_id             bigint,
    response_time_limit  integer,
    completion_time_limit integer,
    response_deadline    timestamptz,
    completion_deadline  timestamptz,
    intake_at            timestamptz,
    responded_at         timestamptz,
    completed_at         timestamptz,
    verified_at          timestamptz,
    status               character varying      NOT NULL,
    cancel_reason        character varying(256),
    has_abnormality      boolean                NOT NULL DEFAULT false,
    urge_count           integer                NOT NULL DEFAULT 0,
    origin               character varying(128),
    destination          character varying(128),
    via_points           jsonb,
    round_trip           boolean                NOT NULL DEFAULT false,
    est_duration_minutes integer,
    temperature_requirement character varying,
    cargo_desc           character varying(256),
    picked_up_at         timestamptz,
    picked_signer        character varying(64),
    picked_photos        jsonb,
    delivered_at         timestamptz,
    delivered_signer     character varying(64),
    delivered_photos     jsonb,
    result_desc          character varying(1024),
    result_photos        jsonb,
    used_materials       jsonb,
    actual_work_hours    double precision,
    acceptance_modes     jsonb,
    acceptance_items     jsonb,
    acceptance_result    jsonb,
    acceptance_photos    jsonb,
    acceptance_signer    character varying(64),
    satisfaction_score   integer,
    satisfaction_comment character varying(512),
    revisited_at         timestamptz,
    tenant_id            bigint                 NOT NULL,
    created_by           bigint,
    updated_by           bigint,
    deleted_by           bigint,
    created_at           timestamptz,
    updated_at           timestamptz,
    deleted_at           timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS workorder_order_no
    ON public.onestop_work_orders (order_no);
CREATE INDEX IF NOT EXISTS workorder_tenant_status
    ON public.onestop_work_orders (tenant_id, status);
CREATE INDEX IF NOT EXISTS workorder_service_type_id
    ON public.onestop_work_orders (service_type_id);
CREATE INDEX IF NOT EXISTS workorder_assignee_id
    ON public.onestop_work_orders (assignee_id);
CREATE INDEX IF NOT EXISTS workorder_planned_exec_at
    ON public.onestop_work_orders (planned_exec_at);
CREATE INDEX IF NOT EXISTS workorder_response_deadline
    ON public.onestop_work_orders (response_deadline);
CREATE INDEX IF NOT EXISTS workorder_completion_deadline
    ON public.onestop_work_orders (completion_deadline);
CREATE INDEX IF NOT EXISTS workorder_created_at
    ON public.onestop_work_orders (created_at);
-- 调度幂等：一个计划/路线在同一时刻最多生成一单（软删行不占坑，删除重生成口径见 §8.4）
CREATE UNIQUE INDEX IF NOT EXISTS workorder_plan_instance
    ON public.onestop_work_orders (plan_id, planned_exec_at)
    WHERE plan_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS workorder_route_instance
    ON public.onestop_work_orders (route_id, planned_exec_at)
    WHERE route_id IS NOT NULL AND deleted_at IS NULL;

-- 6) 执行项子表
CREATE TABLE IF NOT EXISTS public.onestop_work_order_items (
    id            bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    work_order_id bigint        NOT NULL,
    scope         character varying,
    name          character varying(128) NOT NULL,
    sort          integer       NOT NULL DEFAULT 0,
    result        character varying NOT NULL DEFAULT 'ITEM_PENDING',
    value         character varying(128),
    photos        jsonb,
    remark        character varying(512),
    space_ref     character varying(64),
    equipment_ref character varying(64),
    tenant_id     bigint        NOT NULL,
    created_by    bigint, updated_by bigint, deleted_by bigint,
    created_at    timestamptz, updated_at timestamptz, deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS workorderitem_work_order_id
    ON public.onestop_work_order_items (work_order_id);

-- 7) 事件子表
CREATE TABLE IF NOT EXISTS public.onestop_work_order_events (
    id            bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    work_order_id bigint        NOT NULL,
    action        character varying NOT NULL,
    status_from   character varying,
    status_to     character varying,
    note          character varying(512),
    photos        jsonb,
    payload       jsonb,
    tenant_id     bigint        NOT NULL,
    created_by    bigint, updated_by bigint, deleted_by bigint,
    created_at    timestamptz, updated_at timestamptz, deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS workorderevent_work_order_id
    ON public.onestop_work_order_events (work_order_id);

-- +goose Down（成对回退；工单为业务数据表，Down 仅保结构）
DROP TABLE IF EXISTS public.onestop_work_order_events;
DROP TABLE IF EXISTS public.onestop_work_order_items;
DROP TABLE IF EXISTS public.onestop_work_orders;
DROP SEQUENCE IF EXISTS public.onestop_work_order_no_seq;
ALTER TABLE public.onestop_transport_routes DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE public.onestop_periodic_plans DROP COLUMN IF EXISTS tenant_id;
UPDATE public.onestop_service_types SET order_kind = NULL WHERE code LIKE '010405%';
ALTER TABLE public.onestop_service_types DROP COLUMN IF EXISTS order_kind;
```

> COMMENT 全量以 ent schema 为准逐字写入迁移（漂移守卫全量 dump diff，逐字一致），
> 正文略。计划/路线租户列的默认租户回填值（上文以 1 示意）实施时按现网
> `sys_tenants` 默认租户确认后写死；dev 库 gwa 以实际为准。

### 5.9 ent schema 要点

- 三个实体：`WorkOrder` / `WorkOrderItem` / `WorkOrderEvent`，表名与注释按 §5.8；
- Mixin：`AutoIncrementId + TimeAt + OperatorID + TenantID[uint32]{}`（三个表都挂租户）；
  **不挂 IsEnabled**（工单无启停概念）、不挂 Tree；
- 枚举全部 `NamedValues` 对齐 proto 值名（`KIND_EVENT` / `STATUS_PENDING` /
  `SOURCE_MANUAL` / `CHANNEL_DESK` / `ACTION_CREATE` / `ITEM_PENDING` /
  `SCOPE_SPACE` / `PRIORITY_*` / `TEMP_*` 复用既有）；
- `PeriodicPlan` / `TransportRoute` schema 增 `TenantID` mixin 与 Policy，注释同步改
  「租户运营配置」；`ServiceType` 增 `order_kind` 列（Comment 见迁移）；
- 租户 Policy：mixin.TenantID 自带的查询/创建隐私拦截为底线；对齐
  `internal_message` 的「跨租户变更防护」做法，在 **onestop module 内**实现同款
  轻量 Policy（不引主仓包——依赖方向铁律：宿主 → module 单向）；
- 生成：`make ent`（gow 未覆盖本 module 时按 README 用 `go run entgo.io/ent/cmd/ent`）。

## 6. API / proto 设计

### 6.1 文件与路由

| 文件 | 内容 |
|---|---|
| `backend/api/protos/onestop/service/v1/work_order.proto` | 源领域：消息 + 枚举 + HTTP 注解直挂 `OneStopWorkOrderService` |
| `backend/api/protos/admin/service/v1/i_onestop_work_order.proto` | BFF 门面（只挂路由，照抄既有三件套） |
| `service_type.proto` | 增 `order_kind` 字段 + `EffectiveServiceConfig` 消息 + effective-config 路由 |

路由清单（CRUD 包 `data`、动作 RPC 扁平 body——全仓铁律 3）：

```
# CRUD（标准五件套，Create/Update 请求体 { "data": {...} }）
GET    /admin/v1/onestop/work-orders                    列表（过滤参数见 §6.3）
GET    /admin/v1/onestop/work-orders/{id}
POST   /admin/v1/onestop/work-orders
PUT    /admin/v1/onestop/work-orders/{id}               仅 PENDING/APPROVING 可改发起信息
DELETE /admin/v1/onestop/work-orders/{id}               管理清理（软删；有子表行同事务清理）

# effective-config（挂类型服务）
GET    /admin/v1/onestop/service-types/{id}/effective-config

# 子资源
GET    /admin/v1/onestop/work-orders/{id}/events        时间线（时间倒序）
GET    /admin/v1/onestop/work-orders/{id}/items         执行项

# 动作 RPC（body: "*"，收扁平请求体）
POST   /admin/v1/onestop/work-orders/{id}/dispatch      {post?, assigneeId?, note?}
POST   /admin/v1/onestop/work-orders/{id}/approve       {passed, note?}
POST   /admin/v1/onestop/work-orders/{id}/arrive        {note?, photos?}
POST   /admin/v1/onestop/work-orders/{id}/pickup        {signer, photos?, note?}
POST   /admin/v1/onestop/work-orders/{id}/deliver       {signer, photos?, note?}
POST   /admin/v1/onestop/work-orders/{id}/complete      {resultDesc, resultPhotos?,
                                                        usedMaterials?, actualWorkHours?,
                                                        items?: [{id, result, value?, photos?, remark?}]}
POST   /admin/v1/onestop/work-orders/{id}/verify        {passed, signer?, photos?, note?,
                                                        itemsResult?: [{name, passed, remark?}]}
POST   /admin/v1/onestop/work-orders/{id}/cancel        {reason}
POST   /admin/v1/onestop/work-orders/{id}/urge          {note?}
POST   /admin/v1/onestop/work-orders/{id}/reassign      {post?, assigneeId?, note?}
POST   /admin/v1/onestop/work-orders/{id}/transfer      {toTypeId, note?}        → 返回新单
POST   /admin/v1/onestop/work-orders/{id}/derive        {toTypeId, description, note?} → 返回新单
POST   /admin/v1/onestop/work-orders/{id}/abnormal      {desc, photos?, note?}
POST   /admin/v1/onestop/work-orders/{id}/satisfaction  {score, comment?}
```

字段号约定沿用仓内风格：业务字段 1–99，审计 100+，时间戳 200+。
`module_mapping.go` 登记 `OneStopWorkOrderService`（暂挂 SYSTEM，与既有 onestop
三服务一致；套餐模块白名单联动留计费期评估）。新端点须 `make openapi` → 重启 →
管理页「接口同步」（fail-closed，未同步 403）。

### 6.2 动作语义补充

- **transfer 转单**：旧单关闭（cancel_reason 自动「转单至 {新单号}」），新单
  复制发起信息快照、按新类型重新解析 kind/SLA/验收，`source_order_id` 指回；
  toTypeId 必须是启用细类。**derive 派生**：旧单不动，新单同上但 source=DERIVED，
  典型链路巡检异常 → 报修；
- **complete 携带 items**：TASK 单的逐项结果在 complete 请求内一次性提交
  （`items` 数组按 id 回填）；独立 item 级 RPC 不做（无中途保存需求，移动端落地时
  再评估增补）；
- **dispatch 的 assignee 可空**：岗位派单（生成单默认形态），CLAIM 接单动作本期
  只留枚举位不实现；
- **Update 语义**：工单不是自由编辑资源——PUT 仅 PENDING/APPROVING 态允许改
  发起信息组字段（description/location/images/requester/priority 覆盖），其余状态
  提交 422「已进入履约，走动作接口」。

### 6.3 列表过滤（contains 铁律）

| 参数 | 匹配 |
|---|---|
| `orderNo` | EQ（ID 类字段不进模糊） |
| `description` / `requesterName` / `requesterPhone` | contains |
| `status[]` / `orderKind` / `priority` | IN / EQ |
| `serviceTypeId` | 子树：取该节点 code，`service_type_code LIKE '{code}%'` |
| `executionPost` / `assigneeId` | EQ |
| `createdFrom` / `createdTo` | created_at 区间 |
| `overdue` | RESPONSE（responded_at IS NULL AND response_deadline < now）/ COMPLETION / ANY |

## 7. 服务层校验与领域规则

| 规则 | 层 | 行为 |
|---|---|---|
| Create 类型门控 | 工单服务 | service_type_id 必须为**启用细类**（LEVEL_DETAIL）；细类缺位的类别按字典规则建 `99` 兜底细类，不放宽到小类 |
| kind/SLA/验收快照 | 工单服务 | 创建时走 effective-config 解析；TRANSPORT 手动单 origin/destination 必填 |
| requires_approval | 工单服务 | 解析为 true → 初始 APPROVING；审批人范围本期=有审批权限的管理端用户 |
| 状态迁移 | 工单服务 | §5.3 矩阵唯一判据，非法组合 409 带合法动作清单 |
| TASK 完工约束 | 工单服务 | 存在 ITEM_PENDING 执行项时 complete 422 |
| 执行项异常联动 | 工单服务 | items 提交含 ITEM_ABNORMAL → has_abnormality=true |
| 计划挂载 | 计划服务 | 沿用 P1（trigger_mode 解析）；新增**租户内可见**（计划列表随租户闸门隔离） |
| 撤销 | 工单服务 | COMPLETED 后不可撤（验收打回走 verify 驳回）；reason 必填 |
| satisfaction | 工单服务 | 仅 CLOSED；score 1–5 |
| 租户隔离 | 数据层 | 三个新表 + 计划/路线全部经 TenantID Privacy；调度器以 SystemContext + 计划租户注入 |
| 不吞错 | 全部 | 调度器展开单条计划失败：日志带原始错误后继续下一条（单条失败不拖垮整轮），整轮结果留痕 |

## 8. 调度器设计（计划/班次 → 工单展开）

### 8.1 挂载方式

复用宿主 asynq 任务系统的**系统级常驻任务**先例（`tenant_expiry_scan` /
`notification_delivery_sweep` 同款）：

- 任务类型常量 `onestop_plan_expand`（定义在 onestop module 包内，宿主
  `asynq_server.go` 接线时引用——依赖方向合法）；
- 系统级 cron **每分钟**，`RegisterSubscriber(srv, ..., workOrderService.AsyncPlanExpand)`，
  随 `StartAllTask` 的系统级重注册路径自动恢复（照既有系统任务，不进 sys_tasks 表）；
- `cfg.Server.Asynq` 未配置时任务子系统整体不启动（宿主现状）：**计划不自动生单，
  手动链路不受影响**，部署文档写明该依赖（要自动生单必须配 Redis/asynq）。

### 8.2 展开窗口与幂等

- 每轮展开窗口 = **[max(now, 当日 00:00) 所在时刻, 次日 00:00)** 的日历日：
  正常运行时未来单滚动齐全，停机恢复当日自动补单（跨日漏单不追补，人工处理——
  口径写死，避免恢复风暴）；
- 幂等靠部分唯一索引 `workorder_plan_instance` / `workorder_route_instance`
  （plan_id/route_id + planned_exec_at）：重复展开 INSERT 冲突即跳过，天然防重；
- 展开是「按日历日判定执行日」的纯函数遍历：对窗口内每个日期 d，判定每个启用
  计划是否执行日（freq 规则 + 生效区间 + excluded_dates），是则对 times 每个时间点
  生成一单（planned_exec_at = d + t）。

### 8.3 频率展开口径（与 P1 §13 逐条对齐，守卫测试钉死）

- 每日：times 每点一单；每周：weekdays 命中 d 的星期几；每月：month_days 命中
  d 的日号（-1 = 月末）；每年：months × month_days 双命中；
- **季度** = FREQ_YEARLY months 多值（如 [1,4,7,10]）；**半月** = FREQ_MONTHLY
  month_days=[1,15]——调度器同口径展开，不得引入 FREQ_QUARTERLY；
- clamp_to_month_end：2 月的 30/31 号顺延至当月末（true）或当月跳过（false）；
- excluded_dates 命中整天不生成；生效区间（effective_from/to 含当日）窗外不生成。

### 8.4 生成单的形态

- **计划单（TASK/被服 TRANSPORT）**：source=SOURCE_PLAN，status=DISPATCHED
  （生成即派岗），assigned_at=生成时刻，planned_exec_at=计划时刻，execution_post
  = plan.post ?? 类型 effective 岗位，执行项按 §5.5 展开，租户 = plan.tenant_id；
- **班次单（TRANSPORT）**：路线 departure_times 非空才参与展开，source=SOURCE_ROUTE，
  起终点/途经/时长/往返快照自路线，planned_exec_at=发班时刻，description 自动
  「{路线名} 定时班次」；
- **计划停用/删除**：不再生成新单；**已生成单一律不动**（不自动撤销——口径钉死：
  计划变更的存量处理由坐席按列表筛选 planned_exec_at 手工处理，级联撤销留 P3）；
- 生成单的软删（误删后重跑展开）：唯一索引排除软删行，删除的单会因窗口内
  时刻未过而被重新生成——这是有意行为（防漏），误删请走 cancel 而非 delete。

## 9. 前端设计（frontend/admin/react）

### 9.1 页面与路由

```
一站式服务（既有分组）
├── 服务类型（既有，编辑页增「履约形态」字段展示）
└── 工单管理（新增）pages/app/onestop/work-order/
    ├── index.tsx        列表（筛选 + 状态页签 + 表格）
    ├── CreateModal.tsx  坐席代发起（类型级联 + 表单 + effective-config 预填）
    └── DetailDrawer.tsx 详情（状态头 + 动作按钮 + 形态面板 + 时间线）
```

### 9.2 KIND_PROFILE（形态面板常量，对齐 P1 CATEGORY_PROFILE 先例）

```ts
interface KindProfile {
  createRequired: ('origin' | 'destination')[];   // TRANSPORT 必填起终点
  detailPanels: ('route' | 'handover' | 'items' | 'acceptance' | 'materials')[];
  responseEventLabel: string;                     // 到场 / 到场 / 取件签收
  completeLabel: string;                          // 完工提交 / 提交作业结果 / 送达签收
}
```

前端不做 kind 推导（后端解析后返回），只按返回的 order_kind 渲染面板与文案。

### 9.3 列表页

- 页签（状态）：全部 / 待派单 / 已派单 / 执行中 / 已完工 / 已完结 / 已撤销 / 审批中；
- 列：工单号、类型（编码+名称快照）、形态 Tag、状态 Tag、优先级 Tag、执行岗位/人、
  发起人、创建时间、**deadline 红黄标**（响应/完成超时或临期 30 分钟内黄）；
  TRANSPORT 单追加起终点列，计划单显示计划时刻；
- 筛选器对应 §6.3 全量参数；行点击开详情抽屉。

### 9.4 详情抽屉

- 顶部：状态 + 优先级 + SLA 倒计时（超时红）+ 动作按钮组（按 §5.3 矩阵与
  当前状态渲染可见动作；每个动作一个确认弹窗收集动作参数）；
- 基本信息组：发起信息 / 类型快照 / 来源（计划单显示计划名链接、转单显示旧单链接）；
- 形态面板（KIND_PROFILE 驱动）：TRANSPORT 路线 + 双签收卡；TASK 执行项表格
  （逐项结果编辑，complete 弹窗内提交）；EVENT 备件/结果；
- 验收面板：acceptance_items 快照逐项勾选（verify 弹窗）；
- 时间线：events 倒序（动作/操作人/时间/附言/照片/payload 摘要）。

### 9.5 发起弹窗

- 类型选择：平铺字典按四级 Cascader（**必须选到细类**，未到细类禁提交并提示）；
- effective-config 预填：岗位/优先级只读展示（提示来源），优先级可覆盖；
- 表单：描述（必填）、位置、设备描述、发起人/电话、照片上传、快捷语句
  （common_phrases 渲染为快速填充 chips）、命中语句回传；
- TRANSPORT：起终点（必填）+ 温控提示（类型快照）；guidance 以 Alert 展示（实时读）。

### 9.6 服务类型页联动

编辑页「任务触发方式」旁增「履约形态」字段（细类/小类可编辑，继承态显示
「继承上级：运送」或「推导：计划任务」——与 trigger_mode 的继承展示同款交互）。

### 9.7 i18n 与菜单

- `locales/{zh-CN,en-US}/_modules/onestop.json` 增工单分节（状态/形态/动作/字段全量 key）；
- 菜单「一站式服务 → 工单管理」：新库播种，已部署库走「菜单同步」（MERGE），
  遵循仓内既有菜单登记流程。

## 10. 测试策略

| 类别 | 内容 |
|---|---|
| 状态机矩阵单测 | §5.3 全部（from × action）组合：合法迁移断言落库字段、非法断言 409；TRANSPORT 无验收步、COMPLETED 不可撤等收窄点逐一钉 |
| 快照单测 | effective-config 细类→小类回退（单值列 NULL 回退 / 数组列整组覆盖不合并）、order_kind 推导链（显式 > 0106 > SCHEDULED > EVENT）、010405 覆写 |
| 调度展开单测 | 四种频率 + 季度 months 多值 + 半月 [1,15] + clamp（2 月 31 号顺延/跳过）+ excluded_dates + 生效区间 + 幂等（同窗口跑两轮不重）+ 窗口边界（00:00 恰点） |
| 守卫测试 | order_kind 三处同步（种子 010405 == 迁移回填 == 推导规则）；计划种子补租户列后挂载校验不回归 |
| 租户隔离 | 计划/工单跨租户不可见、跨租户动作拒绝 |
| E2E 冒烟 | 手动单全链路（建→派→到→完→验→回访）、计划单（调度生成→逐项→完结）、运送单（建→取→送即完结）、转单/派生链路、审批链路 |

## 11. 实施拆期

| 期 | 内容 | 验收 |
|---|---|---|
| **P2a 后端底座** | 迁移 00013 + 三实体 schema + 计划/路线租户化 + CRUD + 全量动作 RPC + effective-config + 状态机/快照单测 + `make openapi` | `go build/vet/test` 全绿；`make db-drift-check` 通过；E2E：手动单全链路动作走通 |
| **P2b 前端工作台** | 列表/发起/详情 + KIND_PROFILE + i18n + 菜单 + `make ts` | `pnpm run typecheck` 全绿；手动链路 UI 冒烟（三类形态各一条） |
| **P2c 调度器** | `onestop_plan_expand` 接线 + 展开/幂等/补单 + 展开单测 | dev 库跑一轮：28 套种子计划 × 窗口生成数量与时刻逐条核对；重复跑不重 |
| **P2d 转单/审批/回访** | transfer/derive/abnormal/approve/satisfaction + 前端联动（可与 P2b 并行） | 巡检异常→派生报修、动火建单→审批→履约链路 E2E |

每期一个 commit；P2a 与 00012 一样属破坏性前置（计划/路线租户化），先落。

## 12. 风险与对策

| 风险 | 对策 |
|---|---|
| 计划/路线租户化是存量库破坏性变更 | 迁移回填默认租户 + 守卫；dev 库 gwa 先升级验证；P1 已预约该评估点，晚改成本只增 |
| asynq 未配置的部署不知道计划不生单 | 部署文档 + 启动日志显式 warn（「检测到启用计划但任务子系统未启动」） |
| 展开与 P1 §13 频率口径漂移 | 展开纯函数单测逐条引用 P1 §13 条目，两文档口径互为锚 |
| 主表列多（60+） | 列分组固定、jsonb 承载清单类、视图层按 KIND_PROFILE 收口；不做列拆表（跨形态转单要求单表） |
| 快照与源漂移引发「为什么这单岗位不对」 | 详情页明确展示快照值与来源（类型/计划/路线），快照原则写入详情页帮助文案 |
| 单号全局序列跨租户可推测 | 已写明取舍（§5.6）；有诉求再改租户前缀方案，列宽预留 |
| 状态机被移动端需求冲击（接单/暂挂/协作） | 矩阵+事件模型天然可扩：新增动作=新枚举+矩阵行，不破存量；CLAIM 枚举位已留 |

## 13. 本期明确不做

- **移动端执行端 / 用户自助发起端**（小程序/APP）：API 形态已兼容（channel 枚举位、
  CLAIM 接单位、扁平动作 RPC），端本期不做；
- **SSE/消息推送**：状态变更推送接宿主 SSE 网关留 P3（事件表已备好事件源）；
- **KPI 看板与统计接口**（接单量/及时率/完结率/满意度）：数据模型已可支撑，接口另立；
- **实时轨迹/地图、ETA 动态计算**：est_duration_minutes 仅作展示基准；
- **库存联动领用**（01120401 备件申领独立成单，used_materials 手填）；
- **计费/有偿服务费用**（等计费模型）；
- **AI 语义归型**（common_phrases 语料已备，归型入口另立）；
- **排班轮替与自动指派**（派到人为坐席手工动作，抢单仅留位）；
- **批量操作/级联撤销/打印模板**。

## 14. 与既有口径的一致性对账

| 既有口径（P1） | 本设计承接方式 |
|---|---|
| 计划是排期唯一事实源，细类名档位不解读（§1.3 原则 3） | 展开只读计划字段；不解析细类名 |
| 季度 = FREQ_YEARLY months 多值；半月 = FREQ_MONTHLY [1,15]（§13） | 展开口径逐条对齐（§8.3），不新增频率枚举 |
| trigger_mode 驱动任务生成策略（§1.2） | 生单维度完全交给 trigger_mode/路线班次；order_kind 只管履约形状，二者正交 |
| NULL = 未设置、细类回退小类（§4.2/§8） | order_kind 同款解析链；effective-config 兑现预留接口 |
| jsonb「mask 内空数组 = 显式清空」 | 工单 Create/Update 沿用；动作 RPC 为扁平 body 不涉 mask |
| 计划/路线租户态「工单落地时评估」（schema 注释） | §5.8 迁移第 3 节兑现：补挂租户列 |
| 字典编码只停用不复用（字典 README 规则 2） | 快照 code/name 落单，历史单不受字典后续调整影响 |
| CRUD 包 data / 自定义 RPC 扁平 body（全仓铁律 3） | §6.1 路由清单即此约定落地 |
| 迁移与 schema 同 commit（全仓铁律 6） | 00013 与三实体 schema 同 commit，漂移守卫全量校验 |

## 15. 实施偏差记录（2026-10-26 落地时修订）

| 偏差 | 原稿 | 实施口径 | 原因 |
|---|---|---|---|
| 工单号来源 | 独立 PG 序列 `onestop_work_order_no_seq`（WO- + 8 位） | **WO- + 8 位零填自增 id**（同事务派生） | 漂移守卫是严格 diff，ent 之外的业务序列会让守卫永远红；id 单调性与全局序列等价，免序列免重试 |
| 计划/路线租户列 | NOT NULL + 回填默认租户 1 | **可空 `bigint DEFAULT 0`**（0 = 平台） | dev 库实测零租户，无默认租户可指；0 与 `PlatformTenantID` 常量及主仓既有租户列口径一致。租户用户由 TenantPrivacy 只见本租户行，0 行 = 平台模板，租户配置各自建 |
| 事件表 payload 列 | jsonb 动作附件（签收人/转单去向等结构化） | **取消该列**，摘要进 note 文本 | 签收人已有主表字段，转单去向在 cancel_reason；结构化 payload 无消费方，先不落 |
| 调度窗口 | [max(now, 当日00:00), 次日00:00) | **当日 + 次日两个日历日全量**（含已过时刻） | 原稿窗口起点取 now 会漏掉停机期间已过时刻的当日单；全量 + 幂等唯一索引天然补当日、重跑不重（本机实测 52 单两轮不重） |
| 事务回滚语义 | —— | **出错中止的事务必须显式 Rollback 释放连接**（调度幂等冲突路径与 Transition 0 行命中路径同治） | 实测教训：置 committed 跳过回滚会让 aborted 事务挂在连接上，调度器每分钟一批幂等冲突即耗尽连接池（全站查询 context deadline） |
| 种子/调度上下文 | 未提及 | 计划/路线/工单挂 TenantID mixin 后，**种子与调度器必须走 `viewer.WithSystemContext`**（SystemContext=平台视图：查询放行、显式 SetTenantID 受尊重）；onestop 的 viewer 依赖升 v0.0.8（v0.0.7 无 WithSystemContext） | TenantPrivacy 缺 viewer 上下文一律拒绝，启动期播种与 asynq 任务都没有 HTTP viewer |
| 事件枚举 | 17 个动作 | **新增 ACTION_SATISFACTION（满意度回访补录）** | 回访也是时间线动作；proto/ent/迁移三处同步 |
| 前端路由来源 | 菜单表驱动（§9.7 菜单同步） | **react 默认 accessMode=frontend：路由来自静态模块 `router/modules/onestop.tsx`**，菜单表种子与绑定仍补齐（backend 模式与权限管理的消费方） | 实测：只登记菜单表不加静态路由 → 404；两者都要有 |
| 存量缺陷顺带修复 | —— | P1 起菜单绑定种子 MenuIds **漏了 onestop 90/91**（平台管理员导航里整个一站式分组不可见），本次连同 92 一并补入 `default_data.go` | 播种链是「角色→权限→sys_permission_menus」，DefaultMenus 加行不够 |
| dev 库运维动作 | —— | 手工补菜单 92 + 绑定权限 2（`sys_permission_menus`）；正式部署走菜单同步（MERGE）+ 权限管理勾选 | count==0 守卫不补非空库的新菜单 |

验证记录（2026-10-26）：`go build/vet`、onestop 与主仓测试全绿（oss MinIO live 与
org_unit sqlite 为 HEAD 既有失败）；`make db-drift-check` 通过（13 条迁移）；
`make openapi` 含 work-orders 全部端点；react `tsc --noEmit` 全绿。
dev 库 gwa 实测：迁移 00013 应用后调度器自动生成 52 单（16 计划任务 + 36 路线班次，
两轮重跑幂等不重）；API E2E 走通全部链路——手动事件单（含验收驳回返工）、
运送单（起终点校验/温控继承/双签收即完结）、计划任务单（项目库快照/漏项 422/
异常联动）、派生（巡检异常→报修）、转单（关旧开新 + 自动原因）、审批（通过/驳回）、
催办/异常/改派/满意度；工单管理页（列表 61 条真实数据 + 详情抽屉时间线）浏览器渲染验证通过。
已部署实例升级须 `make openapi` 重启后触发「接口同步」（work-orders 全部端点为新端点）。
