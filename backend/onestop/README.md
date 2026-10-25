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
│   ├── seeds/                # 内置服务类型字典（README 全量 299 节点）
│   ├── entclient.go          # Ent 客户端（不做 DDL，goose 迁移由 admin 服务承载）
│   ├── service_type_repo.go
│   └── service_type_seed.go  # 空表播种（count == 0 守卫）
└── service/
    └── service_type_service.go   # 业务规则 + HTTP 接口实现（onestopv1）
```

proto 源文件在主仓：`backend/api/protos/onestop/service/v1/`（源领域，HTTP 注解直挂
service，module 自持接口）与 `backend/api/protos/admin/service/v1/i_onestop_service_type.proto`
（admin BFF 门面，只挂路由）。本 module 的生成走主仓模板 `backend/api/buf.onestop.gen.yaml`。

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
