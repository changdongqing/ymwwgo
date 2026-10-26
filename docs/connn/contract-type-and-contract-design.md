# 合同管理「类型 + 合同底座」详细设计

> 状态：**已实施**（2026-10-26）｜编写日期：2026-10-26｜
> 范围：`backend/contract`（合同类型字典 + 合同底座 + 费用明细 + 到期调度）+ `frontend/admin/react` 合同管理
>
> 前置阅读：[docs/yizhanshi/onestop-unified-service-type-config.md](../yizhanshi/onestop-unified-service-type-config.md)、
> [docs/yizhanshi/onestop-unified-work-order.md](../yizhanshi/onestop-unified-work-order.md)
> （一站式「服务类型 + 工单」两模块——本文整体沿用其既定术语与机制，多处直接映射）、
> `backend/onestop/README.md`（独立 Go module 接线方式）、
> `docs/task_system.md`（调度器挂载）、`docs/tenant_isolation.md`（租户列）、
> `docs/database/design/ent-goose-migrations.md`（迁移铁律）。
>
> 原型参照：`/home/chdq/src/ymwwgit/youmingwanwu/web/src/views/prototype/contract/`
> （医院后勤合同管理原型，纯前端 mock）。原型**没有先定义合同类型**——6 个类型、
> 付款方式推断、差异化字段全部硬编码在前端 `data.ts`；本设计的核心优化即把它们
> 升级为**平台级类型字典 + 类型驱动配置模板**，见 §1.2。

## 1. 背景与结论

### 1.1 从一站式到合同：同一条路线

一站式服务模块的骨架是两个功能模块的组合：

| 一站式 | 回答的问题 | 表 |
|---|---|---|
| 服务类型 | 这类服务**怎么约定**（履约配置模板：岗位/SLA/验收/触发方式） | `onestop_service_types`（平台级字典） |
| 工单 | 这类服务**怎么干**（状态机/快照/调度展开/事件流） | `onestop_work_orders` + items + events（租户业务数据） |

合同管理套用同一条路线：

| 合同 | 回答的问题 | 表 |
|---|---|---|
| 合同类型 | 这类合同**怎么约定**（称谓/付款方式/拆解节点/差异字段/审批开关） | `contract_types`（平台级字典） |
| 合同 | 这份合同**怎么走**（生命周期状态机/快照/费用拆解/到期调度/事件流） | `contract_contracts` + fees + events（租户业务数据） |

类型差异在创建时快照、运行期不回读；跨类型没有转单语义（合同不做 transfer），
但有**版本链**（变更 = 旧版 CHANGED + 新版 v+1 DRAFT，`source_contract_id` 回链）。

### 1.2 原型缺什么，本设计补什么

原型（医院后勤合同）的功能盘点与去向：

| 原型要素 | 原型实现方式 | 本设计 |
|---|---|---|
| 6 个合同类型（外包/施工/租赁/服务/维保/采购） | `data.ts` 硬编码常量 | **二级类型字典**（大类 2 位 / 细类 4 位编码），可管理可扩展 |
| 类型驱动差异化字段（diffFields） | 每类型硬编码字段数组 | 类型模板 `diff_fields`（key/label/widget/options），创建时**快照 schema** |
| 付款方式推断（inferPayMethod） | 按 6 类型 if-else | 类型模板 `pay_method` + `pay_nodes`（节点/进度拆解比例） |
| 乙方称谓随类型（供应商/施工单位/…） | 每类型硬编码 label | 类型模板 `party_b_label` |
| 合同状态（8 个中文字符串） | 无状态机 | 7 态枚举 + 显式转移 + 事件流（§4） |
| 版本/变更记录 | mock 的 version 字段 | 版本链 + CHANGE 事件 + 变更即开新版 |
| 费用拆解（按月/季/节点/进度/一次性） | 前端 generateFeeDetails | 后端**纯函数拆解器**（§6），审批生效时自动落 `contract_fees` |
| 押金（收/退两条腿） | mock DEPOSIT_FEES | 费用明细 category=DEPOSIT，PENDING→DONE→REFUNDED |
| 到期预警（30/90/180 天） | 独立预警页 | 列表 `expiry_type` 过滤 + 每日到期扫描任务（§7） |
| 附件（中标通知书/施工图…） | 硬编码必传规则 | 类型模板 `required_attachments`（提示性）+ 附件 URL 列表 |
| 统计分析页（4 图 + 汇总卡） | mock | **本期不做**，列表足够后单独立项（§12） |
| 5 个预警独立页 | mock | **本期不做**，统一收进列表过滤（§12） |

### 1.3 设计原则（沿用 onestop 六原则的合同版）

1. **统一底座**：一张合同表、一套状态机；类型差异全部由类型模板 + 差异字段快照吃掉，
   不为外包/施工/租赁……分表。
2. **快照不回读**：参与运行期判定的字段（`requires_approval`、`pay_method`、`pay_nodes`、
   `diff_fields` schema、类型编码/名称）创建时快照落库；类型后续改模板不影响存量合同。
   纯参考文案（`guidance`）实时读类型，不落快照。
3. **跟进流程 = 事件流**：所有动作落 `contract_events`，时间线即跟进记录；
   主表只存当前态与关键时间戳（审核人/终止原因）。
4. **费用是拆解唯一事实源**：合同生效即拆解生成费用明细；金额变更走「重新拆解」
   （删自动行再生成，手工行保留）。
5. **合同是租户数据、类型是平台数据**：类型字典不挂租户（全平台共享），
   合同/费用/事件挂 `TenantID`。
6. **不吞错、contains 搜索、CRUD 包 `data`、动作 RPC 扁平 body**（全仓铁律 1/3 原样适用）。

## 2. 术语

| 术语 | 含义 |
|---|---|
| 合同类型 | 平台级二级字典：大类（2 位编码）/ 细类（4 位编码），携带合同配置模板（contract_types） |
| 合同配置模板 | 类型节点上的可空配置列集合：乙方称谓、付款方式、付款节点、费用方向、审批开关、保证金开关、差异字段定义、必备附件、提示 |
| effective-config | 类型节点解析后的生效模板（细类空字段回退大类，§3.3） |
| 快照 | 创建合同时从类型复制落库的值（模板列 + 类型编码/名称），此后不随源变化 |
| 差异字段 `diff_fields` | 类型模板定义的动态字段 schema `[{key,label,widget,options[]}]`，合同按 schema 填 `diff_values` |
| 费用明细 | 合同金额按付款方式拆解出的应收/应付/押金行（contract_fees） |
| 费用方向 `fee_direction` | 应收（我方收钱，如出租）/ 应付（我方付钱） |
| 变更 CHANGE | 生效合同关闭为 CHANGED，按同类型复制开新版（v+1）再走审核 |
| 版本链 | `source_contract_id` 逐版回链；`contract_no` 全链不变，`version` 递增 |

## 3. 合同类型字典（contract_types）

### 3.1 结构：二级编码

| 级别 | 编码 | 说明 |
|---|---|---|
| LEVEL_MAJOR 大类 | 2 位数字 | 01 外包 / 02 施工 / 03 租赁 / 04 服务 / 05 维保 / 06 采购（种子） |
| LEVEL_DETAIL 细类 | 4 位数字 | 前 2 位必须等于父大类编码，如 0501 电梯维保 |

与服务类型四级（2/4/6/8）相比合同域更扁平：两级足够（原型仅 6 个类型）。
**合同创建挂载在叶子节点**（无子节点的启用节点，大类或细类均可）——大类不拆细类
即可直接立合同，避免强迫先建细类。

### 3.2 表结构

表 `contract_types`，平台级（**不挂 TenantID**），mixins：
`AutoIncrementId + TimeAt + OperatorID + Tree + IsEnabled`（与服务类型完全一致）。

| 列 | 类型 | 说明 |
|---|---|---|
| code | varchar(8) unique | 2/4 位数字编码，长度即级别 |
| name | varchar(64) | 类型名 |
| level | enum(MAJOR, DETAIL) | 级别 |
| description | varchar(255) | 描述 |
| party_b_label | varchar(32) null | 乙方称谓模板（供应商/施工单位/服务单位/维保单位/三方公司/合同乙方…） |
| pay_method | enum(PAY_ONCE, PAY_MONTHLY, PAY_QUARTERLY, PAY_NODE, PAY_PROGRESS) null | 默认付款方式 |
| pay_nodes | jsonb null | 付款节点模板 `[{name, ratioPercent}]`，按节点/进度付款的拆解比例 |
| fee_direction | enum(DIRECTION_PAYABLE, DIRECTION_RECEIVABLE) null | 默认费用方向 |
| requires_approval | bool null | 是否需审批（决定提交走向） |
| requires_deposit | bool null | 是否需履约保证金 |
| diff_fields | jsonb null | 差异字段定义 `[{key,label,widget,options[]}]`，widget: text/select/date/textarea |
| required_attachments | jsonb null | 必备附件名清单（提示性校验） |
| guidance | varchar(1024) null | 合同录入提示（纯参考，实时读，不快照） |
| parent_id / is_enabled / … | mixin 列 | 树 + 启停 + 审计 |

索引：`parent_id`、`level`。枚举存储值与 proto 枚举值名一致（`mapper.EnumTypeConverter` 口径）。

### 3.3 模板解析（effective-config）

与 onestop `ResolveEffectiveConfig` 同构，回退单位是「细类 → 大类」：

- 标量列：自身为 NULL → 回退父级；
- 数组列（pay_nodes / diff_fields / required_attachments）：自身为空 → **整组**回退父级（不合并）；
- 解析兜底默认：`requires_approval=true`（合同默认要审）、`requires_deposit=false`、
  `pay_method=PAY_ONCE`、`fee_direction=DIRECTION_PAYABLE`、`party_b_label="合同乙方"`。

`GET /admin/v1/contract/types/{id}/effective-config` 供创建表单预填；
创建合同与拆解器共用同一解析路径（唯一解释口径）。

### 3.4 种子（空表守卫）

构造函数内 `count==0` 时播种（与 onestop 相同机制）：

- 6 大类 + 16 细类（0101 保安服务外包 / 0102 保洁服务外包 / 0103 餐饮服务外包 / 0104 信息运维外包 /
  0201 零星维修施工 / 0202 改造工程施工 / 0301 房屋场地租赁 / 0302 设备租赁 /
  0401 运送服务 / 0402 消杀服务 / 0403 绿化养护服务 / 0501 电梯维保 / 0502 消防维保 /
  0503 空调维保 / 0504 医疗设备维保 / 0601 设施设备采购）；
- 大类携带原型推断出的默认模板：

| 大类 | party_b_label | pay_method | pay_nodes | diff_fields（要点） | required_attachments |
|---|---|---|---|---|---|
| 01 外包 | 三方公司 | PAY_MONTHLY | — | 岗位人数、服务开始/结束日期、服务内容 | 合同正本 |
| 02 施工 | 施工单位 | PAY_NODE | 基础30/主体40/竣工25/质保5 | 施工形式、是否包含检测费、施工内容 | 中标通知书、施工图、合同正本 |
| 03 租赁 | 合同乙方 | PAY_QUARTERLY | — | 租赁形式、租赁面积(㎡)、免租期、合同内容 | 合同正本 |
| 04 服务 | 服务单位 | PAY_MONTHLY | — | 服务频次、服务标准/SLA、服务内容 | 合同正本 |
| 05 维保 | 维保单位 | PAY_MONTHLY | — | 维保形式、维保系统、驻场人数、是否包含检测费、合同内容 | 合同正本 |
| 06 采购 | 供应商 | PAY_PROGRESS | 预付30/到货60/验收10 | 采购方式、供货周期(天)、质保期(月)、安装调试责任、合同内容 | 中标通知书、采购明细清单、合同正本 |

细类不携带模板（整组回退大类）；平台管理员可按细类覆写。
`fee_direction` 种子仅 03 租赁为 **DIRECTION_RECEIVABLE**（原型口径：出租方为我方时收租），
其余大类 PAYABLE；具体每份合同创建时仍可改。

## 4. 合同底座（contract_contracts）

### 4.1 状态机

```
                 ┌──────────────────────────── 变更 CHANGE ──────────┐
                 │                                                   ▼
DRAFT ──submit──▶ REVIEWING ──approve──▶ EFFECTIVE ──expire(调度)──▶ EXPIRED
  ▲                  │reject                │  └──terminate──▶ TERMINATED
  └──────────────────┘                      └───change──────▶ CHANGED（新版 v+1 回 DRAFT）
DRAFT/REVIEWING/EFFECTIVE ──invalidate（作废，需原因）──▶ INVALIDATED
```

| 状态 | 含义 | 终态 |
|---|---|---|
| STATUS_DRAFT 草稿 | 待提交，可编辑/删除 | 否 |
| STATUS_REVIEWING 待审核 | 已提交，锁定不可改 | 否 |
| STATUS_EFFECTIVE 执行中 | 审核通过生效，费用已拆解 | 否 |
| STATUS_CHANGED 变更终止 | 被新版替代关闭 | 是 |
| STATUS_EXPIRED 过期终止 | 到期扫描自动关闭 | 是 |
| STATUS_TERMINATED 已终止 | 提前终止（需原因） | 是 |
| STATUS_INVALIDATED 已作废 | 任意非终态可作废（需原因） | 是 |

动作与守卫：

| 动作 | 转移 | 守卫/附注 |
|---|---|---|
| Submit 提交 | DRAFT→REVIEWING；快照 `requires_approval=false` 时 DRAFT→EFFECTIVE 并直接拆解 | 平铺 body |
| Approve 通过 | REVIEWING→EFFECTIVE | 记 reviewer/reviewed_at；生效即拆解费用 |
| Reject 驳回 | REVIEWING→DRAFT | 原因必填 |
| Change 变更 | EFFECTIVE→CHANGED + 开新版（v+1，DRAFT） | 新版复制全部业务字段 + `source_contract_id`；备注进事件 |
| Terminate 终止 | EFFECTIVE→TERMINATED | 原因必填 |
| Invalidate 作废 | DRAFT/REVIEWING/EFFECTIVE→INVALIDATED | 原因必填 |
| Expire 到期 | EFFECTIVE→EXPIRED | 仅调度器触发（§7），人不可调 |
| Update/Delete | 仅 DRAFT | 其余状态一律 409 |

并发守卫与服务类型一致：先载状态校验 + `UPDATE ... WHERE id AND status IN(from)`
乐观转移，0 行命中即 409；每次转移同事务写 `contract_events`。

### 4.2 表结构

表 `contract_contracts`，租户业务数据（`AutoIncrementId + TimeAt + OperatorID + TenantID`）。
金额列一律 `numeric(14,2)`（单位：元，2 位小数；拆解计算内部走分）。

| 列组 | 列 | 说明 |
|---|---|---|
| 标识 | contract_no unique | `CT-%08d(id)`，事务内生成（WO- 同法） |
| | version int default 1 | 版本号，变更 +1 |
| | source_contract_id null | 上一版合同 id（版本链） |
| 类型 | contract_type_id + contract_type_code/name | 快照 |
| | requires_approval bool | **快照**（submit 走向依据） |
| 状态 | status enum(7 态) | 当前态 |
| 甲方（我方） | region varchar(64) | 单位/机构 |
| | dept varchar(64) | 经办部门 |
| | party_a_contact / party_a_phone | 甲方联系人/电话 |
| 乙方 | party_b_name varchar(128) | 乙方名称（自由输入，供应商档案联动留后续） |
| | party_b_credit varchar(32) | 统一社会信用代码 |
| | party_b_kind enum(ENTERPRISE/ORG/NATURAL) | 企业法人/非法人组织/自然人 |
| | party_b_legal varchar(64) | 法定代表人 |
| | party_b_contact / party_b_phone | 联系人/电话 |
| | party_b_bank varchar(128) / party_b_account varchar(64) | 开户行/账号 |
| | party_b_rating varchar(8) | 信用评级 A/B/C/D |
| 基本 | importance enum(NORMAL/IMPORTANT) | 一般/重要 |
| | start_date / end_date timestamp | 起止（日期语义，end_date 驱动到期扫描） |
| | amount numeric(14,2) | 合同金额 |
| | tax_rate int | 税率 %（0/1/3/5/6/9/13） |
| | pay_method enum | **快照**（可创建时改） |
| | fee_direction enum | 应收/应付（类型默认，可改） |
| | deposit numeric(14,2) | 履约保证金金额 |
| 差异字段 | diff_schema jsonb | **快照**：解析后的 diff_fields 定义 |
| | diff_values jsonb | `{key: value}` 用户填写值 |
| 附件 | attachments jsonb | URL 字符串数组（走既有上传设施） |
| 审核 | reviewer_id / reviewed_at / review_note | 通过/驳回记录 |
| 关闭 | terminate_reason varchar(255) | 终止/作废原因 |
| | remark varchar(1024) | 备注 |

索引：`(tenant_id,status)`、`contract_type_id`、`end_date`（到期扫描）、`source_contract_id`、
`contract_no` 唯一。

### 4.3 子表：费用明细 contract_fees

| 列 | 说明 |
|---|---|
| contract_id | 所属合同 |
| name varchar(64) | 费用名称（合同款/首期/基础节点…/履约保证金） |
| category enum(INCOME/EXPENSE/DEPOSIT) | 收入/费用/押金 |
| direction enum(RECEIVABLE/PAYABLE) | 应收/应付（生成时按合同 fee_direction 定） |
| status enum(PENDING/DONE/REFUNDED) | 待收付 → 已收付 → （押金）已退款 |
| amount numeric(14,2) | 金额 |
| plan_date timestamp null | 计划收付日期 |
| actual_date timestamp null | 实际收付日期 |
| auto_generated bool | 拆解生成（重新拆解只删自动行） |
| remark varchar(255) | 备注 |

状态口径（对齐原型六态文案）：PENDING 按方向显示「待付款/待收取」；DONE 显示
「已付款/已收取」；押金行 DONE 语义为「已收取(待退款)/已支付(待退款)」，可再 Refund →
REFUNDED「已退款」。普通行 DONE 即终态。
索引：`contract_id`、`(tenant_id,status)`、`plan_date`。

### 4.4 子表：事件流 contract_events

`contract_id + action + status_from/status_to + note`，追加不变（append-only）。
action 枚举：CREATE / UPDATE / SUBMIT / APPROVE / REJECT / CHANGE / TERMINATE /
INVALIDATE / EXPIRE / DECOMPOSE / FEE_PAY / FEE_RECEIVE / FEE_REFUND。

## 5. 快照原则（哪些落、哪些不落）

| 字段 | 快照? | 理由 |
|---|---|---|
| contract_type_code/name | 落 | 列表/详情直读，类型改名不追溯 |
| requires_approval | 落 | submit 走向判定以开立时刻为准 |
| pay_method | 落 | 拆解依据；改付款方式 = 改合同（重新拆解） |
| pay_nodes | 落（生效时经 effective-config） | 拆解依据 |
| diff_fields → diff_schema | 落 | 表单 schema 定型；类型改字段不影响存量合同填写值 |
| party_b_label | **不落** | 纯展示称谓，实时读类型 |
| guidance / required_attachments | **不落** | 纯参考提示，实时读 |

## 6. 费用拆解器（纯函数）

输入：合同（amount/deposit/pay_method/fee_direction/start/end）+ 解析后的 pay_nodes。
输出：`[]FeeRow`。规则：

| pay_method | 拆解 |
|---|---|
| PAY_ONCE 一次性 | 1 行「合同款」，金额全量，plan_date=start_date |
| PAY_MONTHLY 按月 | start→end 按月等分（分单位内部计算，余数进最后一行），plan_date=每月同日（月末截断） |
| PAY_QUARTERLY 按季 | 每 3 个月一行，同上 |
| PAY_NODE 按节点 / PAY_PROGRESS 按进度 | 按 pay_nodes 逐节点 `amount×ratio%`（余数进最后一节点），plan_date 空（履约时填） |
| deposit>0 且类型 requires_deposit | 追加 1 行 category=DEPOSIT、direction=合同 fee_direction、plan_date=start_date |

- 拆解时机：**合同生效那一刻**（Approve，或 requires_approval=false 的 Submit）。
- 重新拆解 RPC（EFFECTIVE 可调）：删 `auto_generated=true` 行后重跑拆解（改金额/周期后用），写 DECOMPOSE 事件。
- 拆解器为无副作用纯函数，单测钉死口径（分摊余数、月末截断、节点比例和≠100% 的容错）。

## 7. 到期扫描（调度器）

- 挂宿主 asynq 任务系统，系统级常驻 cron `30 2 * * *`（每日 02:30），
  taskType `contract_expiry_scan`，与 onestop 计划展开同法注册（handler 在
  `asynq_server.go` RegisterSubscriber，cron 在 `task_service.go startAllTask` 尾部）。
- 扫描：租户内 `status=EFFECTIVE AND end_date < 今日零点` → 逐条 Transition 到
  EXPIRED + EXPIRE 事件（复用乐观转移，幂等可重跑）。
- 「即将到期」不做物化：列表 `expiry_type` 过滤（30/90/180 天内/已过期）即时计算。

## 8. API（proto）

独立 Go module `go-wind-admin/contract`（镜像 onestop：独立 go.mod + replace、
独立 ent 客户端不做 DDL、`buf.contract.gen.yaml` 生成到 `contract/api/gen/go` 包 `contractv1`、
BFF 门面 proto `i_contract_*.proto` 放 `protos/admin/service/v1` 零适配注册）。

### 8.1 ContractTypeService（/admin/v1/contract/types）

| RPC | 路由 | 说明 |
|---|---|---|
| List/Get/Create/Update/Delete | go-crud 标准 | CRUD 包 `{data}` |
| GetEffectiveConfig | GET `/{id}/effective-config` | 解析后模板（创建表单预填） |

校验：code 匹配 `^(\d{2}|\d{4})$` 且长度=级别；细类前 2 位=父大类；删除守卫
（有子级 / 有合同引用拒删）。

### 8.2 ContractService（/admin/v1/contract/contracts）

| RPC | 路由 | 说明 |
|---|---|---|
| List/Get/Create/Update/Delete | go-crud 标准 | Update/Delete 仅 DRAFT |
| ListEvents | GET `/{id}/events` | 事件时间线 |
| ListFees | GET `/{id}/fees` | 费用明细 |
| Submit / Approve / Reject / Change / Terminate / Invalidate | POST `/{id}/<action>` | 平铺 body（铁律 3） |
| DecomposeFees | POST `/{id}/decompose-fees` | 重新拆解 |
| PayFee / ReceiveFee / RefundFee | POST `/fees/{id}/pay|receive|refund` | 平铺 body `{actualDate}` |

List 过滤：状态/类型/编号（contains）/乙方（contains）/到期窗口 `expiry_type`。

## 9. 前端（frontend/admin/react，唯一维护端）

| 物 | 位置（镜像 onestop） |
|---|---|
| 路由 | `src/router/modules/contract.tsx`（order 2200；类型页 platform_admin，合同页双角色） |
| Hooks | `src/api/hooks/contract.ts`（useList/fetchList/useGet/useCreate `{data}`+updateMask/useDelete/invalidate/useContractAction 映射） |
| 类型页 | `src/pages/app/contract/type/`：Tabs 列表 + `TypeForm`（基础信息 + 模板配置分区：称谓/付款/拆解节点/方向/开关/差异字段编辑器/附件清单/提示） |
| 合同页 | `src/pages/app/contract/list/`：状态页签 + 过滤（类型/编号/乙方/到期窗口）+ `CreateModal`（类型 Cascader→GetEffectiveConfig 预填与动态差异字段）+ `DetailDrawer`（四段信息 + 差异字段 + 附件 + 费用面板（收/付/退款/重新拆解）+ 事件 Timeline + 按状态动作矩阵） |
| i18n | `src/locales/{zh-CN,en-US}/_modules/contract.json` + `_core/routes.json` 增路由与菜单键 |

## 10. 权限、菜单与登记清单

- 菜单种子（`pkg/constants/default_data.go`）：Id 120 目录 `/contract`（BasicLayout）、
  121 类型页（platform_admin）、122 合同页（双角色）；已部署库走「菜单同步」补挂。
- 组件前缀映射：`app/contract/` → `identityV1.Module_SYSTEM`（default_data.go 同函数）。
- Api 表：BFF proto 进 OpenAPI 文档后「接口同步」自动登记；
  **`pkg/constants/module_mapping.go` 必须登记** `ContractTypeService` / `ContractService`
  两个 tag → Module_SYSTEM，否则租户闸门 fail-closed 403。
- 迁移：`00014_contract_types.sql`、`00015_contract_contracts.sql`（三表），
  COMMENT 与 ent `Comment` 逐字一致（漂移守卫）。
- `cmd/migrate/main.go`：contract ent schema create 与 onestop 并列（开发便利）。
- 模块装配：`wiring_ent.go`（类型服务先构造播种 → 合同服务）、`rest_server.go`
  路由注册（register:route 锚点后）、`asynq_server.go` 到期 handler、
  `task_service.go` cron、`backend/go.mod` replace。

## 11. 测试

| 测试 | 钉死什么 |
|---|---|
| `contract/service/decompose_test.go` | 拆解口径：一次性/按月等分与余数/按季/节点比例与容错/押金行/月末截断 |
| `contract/service/expiry_test.go` | 到期判定纯函数：end_date<今日零点、时区口径 |
| `contract/data/contract_type_seed_test.go` | 种子 JSON：编码模式、父子一致、模板结构合法 |
| 前端 `npm run typecheck` | 全绿（门禁） |
| 本地起服冒烟 | 登录→类型列表（种子）→建合同→提交→通过→费用已拆解→变更→新版 |

## 12. 范围外（后续迭代）

| 项 | 去向 |
|---|---|
| 统计分析页（按类型/部门/机构分布、结算进度） | 数据落齐后单独立项 |
| 5 个预警独立页 | 列表 `expiry_type` 过滤已覆盖查；独立待办页等通知域联动的需求明确后做 |
| 供应商档案联动（乙方从档案选择） | 乙方先自由文本；供应商对象立项时回填 |
| BPM 工作流审核（多级会签） | 本期为单步通过/驳回；`reviewer_id` 已留痕 |
| 合同正文 PDF 生成/打印 | 附件承载正本 |
| 回款计划（receivable plan）独立对象 | 费用明细已含计划日期；复杂计划等需求明确 |
| 合同范本（条款库） | 类型 `guidance` 先承载提示 |
