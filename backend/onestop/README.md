# onestop — 一站式服务（独立 Go module）

一站式服务业务域的独立 Go module（`go-wind-admin/onestop`），由 admin 服务
（`go-wind-admin` 主 module）以 `replace` 本地引入装配，**不是**独立运行的微服务：
HTTP 路由挂在 admin 服务 `:7788` 的 Kratos REST server 上，认证/鉴权/审计/租户闸门
全部复用宿主中间件链。未来服务对象、工单对象及其跟进流程都在本 module 内扩展。

## 模块布局

```
onestop/
├── go.mod                    # module go-wind-admin/onestop（主 module replace 本地引入）
├── Makefile                  # ent / buf 代码生成
├── api/gen/go/               # 本 module 的生成产物（包 onestopv1）
├── data/
│   ├── ent/schema/           # ent schema（结构单一事实源）
│   ├── ent/                  # ent 生成产物
│   ├── seeds/                # 内置服务类型字典（README 全量 299 节点）+ 周期计划/运送路线模板
│   ├── entclient.go          # Ent 客户端（不做 DDL，goose 迁移由 admin 服务承载）
│   ├── service_type_repo.go / periodic_plan_repo.go / transport_route_repo.go
│   ├── service_type_seed.go  # 空表播种（count == 0 守卫）
│   └── periodic_plan_seed_test.go / service_type_seed_test.go / transport_route_seed_test.go
└── service/
    ├── service_type_service.go       # 业务规则 + HTTP 接口实现（onestopv1）
    ├── periodic_plan_service.go
    └── transport_route_service.go
```

proto 源文件在主仓：`backend/api/protos/onestop/service/v1/`（源领域，HTTP 注解直挂
service，module 自持接口）与 `backend/api/protos/admin/service/v1/i_onestop_service_type.proto`
等 BFF 门面（只挂路由）。本 module 的生成走主仓模板 `backend/api/buf.onestop.gen.yaml`。

## 与主仓的接线方式（为什么这样设计）

| 主题 | 说明 |
|---|---|
| 依赖方向 | 只允许 宿主 → 本 module。宿主主 module 的包（`pkg/middleware/auth` 等）不可被本 module 引用，操作人提取经 `service.OperatorReader` 函数在装配期注入 |
| 类型共享 | 主仓 `buf.gen.yaml` 把 `onestop/service/v1` 的 go_package 覆盖为 **本 module 的 gen 包**（`go-wind-admin/onestop/api/gen/go/onestop/service/v1;onestopv1`），因此 admin 门面生成的 HTTP 接口与本 module service 的方法签名完全一致——rest_server 直接注册，零适配层；主 gen 不生成 onestop 副本（paths 枚举排除），无描述符重复注册 |
| 分页类型 | 与 go-crud 仓储、宿主共用远程包 `github.com/tx7do/go-crud/api/gen/go/pagination/v1`，无转换层 |
| ent 代码生成 | `make ent`（`go run entgo.io/ent/cmd/ent`——全局 `ent` 二进制与当前工具链组合报 "package context without types"） |
| 数据库迁移 | 不做 DDL；表结构变更写 `app/admin/service/internal/data/migrations/000NN_onestop_<what>.sql`，与 ent schema 同 commit（全仓铁律 6）；CI 漂移守卫的 automigrate-ent 已扩展为同时建本 module 的 ent schema |
| 租户 | 服务类型是全局字典表：不挂 `mixin.TenantID`、不配租户 Policy（参照 `sys_languages`/`sys_configs`） |
| OpenAPI/Api 表 | BFF 门面随 `protos/admin/service/v1` 进入标准 openapi 管线；服务 tag `OneStopServiceTypeService` 已在 `pkg/constants/module_mapping.go` 登记（暂挂 SYSTEM，字典为平台级全局数据） |

## 服务类型（当前唯一实体）

四级分类字典（大类 → 中类 → 小类 → 细类），编码规则见
[docs/leixing/biaodan/README.md](../../docs/leixing/biaodan/README.md)：

- `code`：2/4/6/8 位数字字符串，**位数即层级**（`level` 由服务层按位数推导，不可自定）；
- `parent_id`：上级服务类型（Tree mixin 自关联）；大类为根，其余层级必须挂在恰好上一级之下；
- 全局唯一索引兜底 `code` 唯一；有下级时禁止删除；上级不得指向自身子树；
- **不设 `sort_order`**：列表固定按 `code` 升序（编码本身即层级业务序），
  后端在调用方未显式排序时也以 `code ASC` 兜底；
- **前端平铺普通表**：不展开树表，上级编码由编码前缀直接推导展示；
- 空表启动时按 README 全量清单播种 299 个节点（1 大类 + 12 中类 + 75 小类 + 211 细类），
  种子见 `data/seeds/service_types.json`，采用 `count == 0` 守卫，非空表不覆盖。

接口路由（挂 admin 服务）：

```
GET    /admin/v1/onestop/service-types
GET    /admin/v1/onestop/service-types/{id}
POST   /admin/v1/onestop/service-types
PUT    /admin/v1/onestop/service-types/{id}
DELETE /admin/v1/onestop/service-types/{id}
```

## 统一履约配置模板（本期）

**定位**：服务类型的「配置详情」= 挂在类型节点上的服务履约配置。统一模板期
（报修 0101 → 巡检 0103 → 运送 0106 三期演化后收敛）不再按子树设独立配置分节：
**所有小类/细类共用一个「履约配置」分节**，四组字段全部可空（未配置 = NULL），
专项属性按中类维度的展示矩阵条件出现（存储层不限子树）。消费方是后续的工单对象
（派单、SLA 预警、绩效工时核算）与发起端（快捷语句、语义归型语料）。
医院后勤 / 商场 / 园区三业态同构适用。

设计文档：[docs/yizhanshi/onestop-unified-service-type-config.md](../../docs/yizhanshi/onestop-unified-service-type-config.md)。

### 字段分组（15 列）

| 组 | 字段 | 列 | 语义 |
|---|---|---|---|
| A 派单与时效 | 执行岗位 | `execution_post` | 派单目标岗位（电工/运送员/保洁员…），合并原 repair_post/transport_post |
| | 默认优先级 | `default_priority` | 派单排序与超时预警加权；细类覆盖小类 |
| | 响应/完成时限 | `response_time_limit` / `completion_time_limit` | 受理 → 到场/取件/处置、受理 → 完工/送达/办结（文案按业态） |
| | 标准工时 | `standard_work_hours` | 单次作业标准人时 |
| B 作业标准 | 指导说明 | `guidance` | 作业 SOP 与安全注意事项 |
| | 常见语句 | `common_phrases` | 发起端快捷语句 + 语义归型语料 |
| | 工具/装备 | `standard_tools` | 出工标配（工具/轮椅/标本箱/清洁机具…） |
| | 耗材/备件 | `spare_parts` | 出工常备，配合库存领用 |
| C 验收方案 | 验收方式/验收项 | `acceptance_modes` / `acceptance_items` | 报修=完工验收、运送=交接签收、客服=办结回访；可组合 |
| D 专项属性 | 温控要求 | `temperature_requirement` | 冷链 2-8℃ / 冷冻 / 保温（推荐 0106） |
| | 需事前审批 | `requires_approval` | 搬入搬出押运、动火作业、资源申请等管控类 |
| | 物品细目 | `cargo_types` | 承接物品范围（推荐 0106） |
| | 标准作业项目库 | `standard_items` | 周期计划点表模板（原巡检项目库泛化） |
| 触发方式 | 任务触发方式 | `trigger_mode` | 周期计划统一开关，见下 |

数值域由服务层校验：工时须为正；时限 1–43200 分钟；**大类/中类是纯结构节点，
提交任一配置字段（含 trigger_mode）→ 422**（后端补挡，与前端门控同口径）。

### 任务触发方式（trigger_mode，周期计划的统一开关）

- `TRIGGER_ON_DEMAND` 按需发起：工单由用户/呼叫触发（报修、突发、投诉、应急）；
- `TRIGGER_SCHEDULED` 计划生成：可挂周期计划，任务由调度器按频率生成；
- NULL = 未设置：**细类回退小类，仍未设置按 ON_DEMAND**（后端 `ResolveTriggerMode`
  与前端 `resolveTriggerMode` 同口径实现）。

约束：由 SCHEDULED 切回 ON_DEMAND（或清空后经回退链解析不再计划化）时，
节点自身或其直接子级仍挂周期计划 → 409 拒绝（先删计划）。

### 展示矩阵与配置继承

- 前端 `CATEGORY_PROFILE`（按中类编码）驱动专项属性可见性与时限文案；
  未登记的新建中类回落全组可见 + 通用文案（统一模板兜底）；
- `standard_items` 不进矩阵：解析后为 SCHEDULED 即出现（计划点表模板与
  是否计划型强相关）；
- 配置继承（细类为空回退小类）由读取端解析：本期前端详情页展示继承值，
  运行期消费留给工单对象（P2）。

### 种子行业模板

`data/seeds/service_types.json`：299 节点 + 配置模板按「小类=工种层（岗位/SLA/
指导/项目库）、细类=作业层（工时/语句/工具/耗材）」分层预置——报修 11 小类 +
运送 5 小类（既有）+ 统一期新增 17 个计划型小类（维保 10 / 保洁 2 / 消防 3 /
检测 1 / 绿养 1），执行岗位共 33、细类工时 52；**105 个节点标记 SCHEDULED**
（80 细类 + 25 小类，矩阵见设计文档 §7）。守卫测试 `data/service_type_seed_test.go`
钉死以上口径（与迁移 00012 的回填前缀清单三处同步维护）。

### 本期明确不做

- 工单对象与调度器（计划 → 任务生成、运行期继承、ETA、逐级上报）：P2；
- 计费/有偿、默认费用字段：等计费模型落地；
- trigger_mode 多选化（同一类型既按需又跑计划）：工单期按需扩展。

## 周期计划（onestop_periodic_plans）

**定位**：与履约配置（类型上的单值参数）不同，周期计划是**类别节点下的列表型
子资源**——一个类别可挂多套周期不同的计划。由巡检计划（0103）泛化而来，
覆盖巡检、维保、日常保洁、消杀、抄表、绿化养护、库存盘点等周期型服务。
计划描述「谁（岗位）按什么频率在什么范围内做哪些项（点表）」，生效区间约束
执行窗口；运行期任务生成由后续工单对象实现，本期只做配置管理。

**挂载门控**：类别为小类/细类，且解析后的任务触发方式 = TRIGGER_SCHEDULED
（替代原 0103 编码前缀校验；细类未设置回退小类，挂在 SCHEDULED 小类下的
新细类无须再显式开启）。

### 频率模型

`freq_type` 选周期 + 各周期填各自的执行日字段 + `times` 执行日内时间点列表
（每日多次 = 多个时间点）+ `excluded_dates` 排除日期（节假日不生成任务）：

| 周期 | 必填执行日字段 | 月底兼容 |
|---|---|---|
| FREQ_DAILY | —（仅 times） | — |
| FREQ_WEEKLY | `weekdays`（1=周一…7=周日，可多选） | — |
| FREQ_MONTHLY | `month_days`（1-31，`-1`=当月最后一天） | `clamp_to_month_end` |
| FREQ_YEARLY | `months`（1-12）+ `month_days` | 同上 |

既定口径（防实现期再议）：**季度** = FREQ_YEARLY 的 months 多值（如 [1,4,7,10]）；
**半月** = FREQ_MONTHLY 的 month_days=[1,15]（weekdays 表达不了双周）。

### 点表与范围

- 类别节点挂**标准作业项目库**（`service_type.standard_items` jsonb），
  是计划点表的模板来源（巡检点/维保项目单/保洁作业标准同构）；
- 计划带**空间点表**（`space_items`）与**设备点表**（`equipment_items`）两组
  jsonb（`[{name,enabled}]`），计划内可增（含自定义）/删/逐项停用启用，
  停用项保留在点表中但不执行；
- `space_refs` / `equipment_refs` 为**预留字段**：空间/设备对象模块落地后启用。

### 其他约定

- Update 走「合并视图校验」：未携带字段以库内值补齐后校验，repo 收原始提交值，
  jsonb 数组保留「mask 内空数组 = 显式清空」语义；当前周期必填执行日字段不允许
  被 mask 显式清空；
- 种子：空表守卫播种 **28 套**行业模板计划（巡检期 9 套沿用 + 统一期 19 套：
  电梯半月维保、消防月保、红外测温季度、发电机月度试机、水箱清洗半年、滤网季度、
  病区保洁每日、四害消杀每月、被服收发、消控交接班、静配巡回配送、快递分发、
  季度满意度、能源抄表、水质检测、绿化修剪、绿植更换、库存月盘、外包考核——
  四种周期全覆盖），类别编码解析类别 id，查不到类别跳过留痕；
  守卫测试 `data/periodic_plan_seed_test.go`（计划只能挂 SCHEDULED 类别）；
- 管理入口在服务类型页「计划生成」类别的编辑页签（履约配置分节 + 「周期计划」
  面板 + 抽屉表单），独立「周期计划管理」菜单页留给工单模块一起做。

接口路由（挂 admin 服务，`OneStopPeriodicPlanService`，SYSTEM 模块）：

```
GET    /admin/v1/onestop/periodic-plans
GET    /admin/v1/onestop/periodic-plans/{id}
POST   /admin/v1/onestop/periodic-plans
PUT    /admin/v1/onestop/periodic-plans/{id}
DELETE /admin/v1/onestop/periodic-plans/{id}
```

## 常用运送路线（0106 运送配送，维持运送专属）

**定位**：`onestop_transport_routes`，与周期计划同款「列表型子资源」模式：一个
0106 类别（小类/细类，服务层校验）可挂多条起终点/班次不同的路线。路线描述
「从哪到哪（起点/终点/途经点）、单程预计多久、是否往返、定时班次何时发班」；
调度坐席按路线模板派单（ETA 基准），departure_times 非空的定时班次由后续工单
对象按时间点生成定时运送任务。**挂载门控维持 0106 子树不变**（与触发方式无关：
按需押运同样需要路线模板，仅定时班次路线参与后续任务生成）。

| 字段 | 语义 |
|---|---|
| `name` / `service_type_id` | 路线名称 / 所属类别（0106 小类/细类，服务层校验，无 ent 外键） |
| `origin` / `destination` / `via_points` | 起点（取件地）→ 途经点（有序可空）→ 终点（送达地） |
| `est_duration_minutes` | 单程预计时长（1-1440 分钟）：调度 ETA 与超时判定基准 |
| `round_trip` | 往返路线（送达后原路返回，如餐车回收/巡回收件） |
| `departure_times` | 定时班次发班时间点（HH:mm 列表）；空 = 按需即时派单 |

Update 语义与周期计划同款「合并视图校验」；种子：空表守卫播种 **11 条**模板
路线，守卫测试 `data/transport_route_seed_test.go`。

> 已部署实例需 `make openapi` 后重启进程，并在管理页触发「接口同步」登记新端点
>（租户闸门 fail-closed，未同步前新端点 403）。服务类型字段级变更新增列
> 不需要「接口同步」（无新端点）。


## 新增实体的步骤（本 module 内扩展时）

1. `backend/api/protos/onestop/service/v1/<entity>.proto`（消息 + HTTP 注解直挂 service；
   错误沿用 `onestop_error.proto`）；
2. `backend/api/protos/admin/service/v1/i_<entity>.proto` BFF 门面（照抄
   `i_onestop_service_type.proto`：消息类型引用 onestop 源领域，go_package 覆盖已全局生效）；
3. 主仓 `backend/api` 下 `buf generate`（门面进 admin gen）+ `buf generate --template buf.onestop.gen.yaml`
   （源领域进本 module gen）；
4. `data/ent/schema/<entity>.go` → `make ent`；
5. admin 服务的 `internal/data/migrations/` 新增 `000NN_onestop_<what>.sql`（与 schema 同 commit）；
6. `data/<entity>_repo.go` + `service/<entity>_service.go`（操作人走 `OperatorReader`）；
7. 宿主侧接线：`cmd/server/wiring_ent.go` 构造 repo/service、`internal/server/rest_server.go`
   `adminV1.Register<Entity>ServiceHTTPServer`、`pkg/constants/module_mapping.go` 登记服务 tag；
8. `make openapi` + `make ts`，管理页「接口同步」。

## 开发命令（在本目录执行）

```bash
# 源领域 proto 生成（在 backend/api 执行；本 module 无独立 buf 工作区）
cd ../api && buf generate --template buf.onestop.gen.yaml
make ent   # ent 生成（在本目录执行）
go build ./... && go vet ./...
```
