# AGENTS.md — go-wind-admin Monorepo 开发指南（AI Agent 入口）

本文件是 monorepo 总入口：定位布局、指路各端规范、声明全仓铁律。**深入开发前必读对应端的 AGENTS.md**。

## 仓库布局

> **本仓只维护 react 一端（2026-10-20 起强制，见全仓铁律 5）**：`vue-element/` 与 `vue-vben/` 已冻结，仅作历史参照保留——不加功能、不修 bug、不升依赖。
> 对外表述统一按"采用者取一套"口径（见 [docs/adopt-one-frontend.md](./docs/adopt-one-frontend.md)）——
> 不要因为仓库里同时存在三个目录，就在文档 / 报错 / 提示里把"三端"当成使用者的义务。

```
backend/                    Go + Kratos + Ent（DI 手写装配 wiring_*.go，已弃用 Wire；HTTP :7788，SSE 网关 :7789）
frontend/admin/
├── react/                  React 19 + antd 6 + ProComponents + TanStack Query + zustand  ← 唯一维护端
├── vue-element/            Vue 3 + Element Plus + vxe-table + TanStack vue-query + Pinia（已冻结，仅参照）
└── vue-vben/               Vben Admin 5.x monorepo（apps/admin + packages/*）+ Ant Design Vue（已冻结，仅参照）
docs/                       文档体系（总入口 docs/README.md：教程层 docs/tutorial/ + 参考层专题文档）
```

## 前端门禁（仅 react）

> 2026-10-20 起只有 `frontend/admin/react` 继续维护（见全仓铁律 5），门禁只考 react；vue-element / vue-vben 已冻结，不再要求 typecheck 全绿。

| 端 | 命令（在各自目录下） | dev 端口 | 状态 |
|---|---|---|---|
| react | `npm run typecheck` | 15888 | **必须保持全绿** |
| vue-element | `npx vue-tsc --noEmit`（或 `npm run type-check`） | 15777 | 已冻结，仅参照 |
| vue-vben | `pnpm run typecheck` | 15666 | 已冻结，仅参照 |

**react 门禁出现任何新报错，一律当作自己引入的 bug 修复**，不存在"可忽略的既有错误"。改完 react 代码先跑门禁再声称完成。

## 全仓铁律

1. **不吞错**：任何 catch 至少二选一——`console.error/warn` 带出**原始错误对象**，或重新抛出。用户可见的通知/Message ≠ 日志（只有翻译文案）。合法裸 catch 仅限纯本地 best-effort 兜底且注释写明原因。历史教训：认证链路静默吞错曾让 bug 排查耗时数日。
2. **vue-vben 工具链版本已钉死**（catalog 精确版本 + packageManager 匹配本机 pnpm）：禁止改回 `^` 范围、禁止顺手升级 vue/typescript/vue-tsc/pnpm。原因与升级流程见 `frontend/admin/vue-vben/AGENTS.md`「工具链与已知坑」。
3. **搜索条件一律 contains 而非 EQ**、ID 类字段不进模糊搜索；CRUD 请求体必须包 `{ data: {...} }`，但**仅限 CRUD**——`body: "*"` 的自定义 RPC（如 `internal-message/send`）收的是扁平请求体，多包一层 `data` 会被 protojson 当未知字段丢掉，接口照样 200、字段全为空。细节见 `docs/crud_module_guide/`。
4. **关系型数据库只选 PostgreSQL**：本程序一切关系型存储（运行库、新模块、新服务、演示数据）只允许 PostgreSQL（含 pgvector）；禁止新增或启用 MySQL / SQLite 等其他关系型方言。`data.yaml` 的多方言切换项、go.mod 里的 mysql/sqlite 驱动、`*_sqlite_test.go` 内存库测试均属历史遗留，**不得扩展使用**；新代码一律按"只有 PostgreSQL"假设编写（迁移、DSN 示例、部署文档都是 postgres）。
5. **前端只维护 react**：`frontend/admin/react` 是唯一继续维护的前端；`vue-element` 与 `vue-vben` 自 2026-10-20 起冻结——不加功能、不修 bug、不升级依赖、不移植新模块。react 的改动不再向两端移植，"三端镜像 parity"不再是要求；两端代码仅供行为对照时阅读，**不要修改**。
6. **数据库结构变更只走 goose 版本化迁移**：表结构的单一事实源仍是 Ent schema，但落库 DDL 一律通过 `backend/app/admin/service/internal/data/migrations/` 新增 `000NN_<module>_<what>.sql`（**与 schema 改动同一 commit 提交**）；历史的 Ent 自动迁移（`Schema.Create`）已移除，`data.database.migrate` 控制的是启动时执行 goose。存量老库升级先跑 `cmd/migrate baseline`。CI 漂移守卫（db-schema-drift workflow / `make db-drift-check`）校验迁移链与 Ent schema 一致。唯一登记的例外是 AI 向量子系统的运行期动态 DDL（向量列补建/HNSW 门槛索引/`sys_ai_search_index`，见设计文档 §9），除此之外禁止任何运行期 DDL。改任何表结构前先读 `docs/database/design/ent-goose-migrations.md`。

## 开发策略：只做 react 端

新功能/新模块**只在 `frontend/admin/react` 实现**。此前"react 先行、验证通过后移植 vue-element / vue-vben"的工序随铁律 5（2026-10-20）废止：两端已冻结，移植环节取消。

> 对外要说的是：采用者直接选 react（[docs/adopt-one-frontend.md](./docs/adopt-one-frontend.md)）；偶尔需要对照历史行为时，只读冻结两端的代码，不要改。

**CRUD 模块**：全流程手册在 `docs/crud_module_guide/`（后端 + 前端端到端流程）。

**代码生成器**：配套工具 [go-wind-toolkit/gowind-uiapp](https://github.com/tx7do/go-wind-toolkit/tree/main/gowind-uiapp)（桌面 GUI + CLI，从数据库表/SQL 生成前后端代码，含简单表单）。CLI（`gowind-cli`）非交互、JSON 输出，适合 Agent 调用。工具产物仍须按本仓铁律与约定验收补齐（`{ data: {...} }` 包裹、contains 搜索、已部署实例的新端点走管理页「接口同步」登记进 Api 表、`make ts` 生成三端 TS（仅 react 端产物为必查项，vue 两端已冻结）等）。

> 系统默认数据（admin 用户、角色、菜单、权限、语言等）由服务启动时在 Go 侧自动播种（`pkg/constants/default_data.go` + 各 service 的 count==0 守卫；Api 表仅在空表时于启动期自动同步，**已部署实例新增端点须在管理页「接口同步」手动触发全量重建**，否则租户闸门 fail-closed 403；「接口同步」重建读的是**打进二进制的** `backend/app/admin/service/cmd/server/assets/openapi.yaml`，所以新 proto 必须先 `make openapi` 再重启进程，否则同步"成功"而新端点依旧不在表里。新菜单同理：`count==0` 守卫意味着老库要靠各端「菜单同步」(MERGE) 才会多出这一行），**不要**找 SQL 种子脚本，`backend/sql/` 下只剩演示数据。

## 后端任务：gow 优先，make 兜底

后端的运行与代码生成统一走 [gow CLI](https://github.com/tx7do/go-wind-toolkit/tree/main/gowind)（安装：`go install github.com/tx7do/go-wind-toolkit/gowind/cmd/gow@latest`），在 `backend/` 下执行。**接手后不要上来就用 Makefile 的 make 命令**：Windows 无原生 make、嵌套 Makefile 需要跨目录 cd，而 gow 自动发现 `app/*/service`，行为一致：

| 任务 | gow 命令 |
|---|---|
| 运行服务 | `gow run admin` |
| Ent 生成 | `gow ent`（全部服务）/ `gow ent admin` |
| Proto / API Go 代码 | `gow api` |
| 从数据库表生成 CRUD | `gow generate`（DSN 驱动；`--proto-only` 仅出 proto） |

项目已弃用 Wire（DI 为手写 `wiring_*.go`），不要运行 `gow wire`，也不要在新代码里引入 wire 依赖。

gow 未覆盖的任务（三端 TS 生成 `make ts`、OpenAPI `make openapi`、`make test/lint` 等）才退回 Makefile（`backend/` 根目录执行）。

## 本地验证要点

- 后端起在 `:7788`（`gow run admin`；启动方式见 `docs/windows-startup-guide.md` / `docs/backend_deploy.md`）；前端 dev 端口见上表，代理已配置好 API 转发。
- 登录账号：全新环境播种为 `admin / Abcd@1234`（`pkg/constants/default_data.go` 的 `DefaultUserPassword`）；本机 `gwa` 库实测（2026-09-20）即为此值，历史备注的 `admin / admin` 已失效（登录返回 `INVALID_PASSWORD`）。图形验证码的答案可在 Redis 中按 `gowind:captcha:<captchaId>` 直接读取，便于自动化验证。
- vue-element 若见 router-view 塌空/白屏：**别再往 vite 依赖优化/HMR 上归因**（那是早先的误判）。真因是 `<transition mode="out-in">` 叠 vue-router 5 懒加载路由的竞态，已通过去掉 `out-in` 改同帧交叉淡入淡出修掉；回归判定与保留的防御见 `frontend/admin/vue-element/AGENTS.md`「白屏（router-view 塌空）真因与处置」。
- AI 功能本地调试：`.zcode/tmp/mock_llm.py`（OpenAI 兼容 echo + embeddings，端口 18080）可替代真实模型打通全部链路；生产需配置真实 provider（管理页）与 `pgvector` 扩展（见 `docs/ai_module.md`）。
- `go test ./...` 偶发 `fork/exec %TEMP%\go-build...\x.test.exe: Access is denied.`（Windows 上对刚链接好的测试二进制执行被拦，疑似安全策略/杀软实时扫描）：属环境问题、**不是代码失败**。复验办法是绕开 Temp 执行——`go test -c -o <工作区内路径>/x.test.exe ./pkg/x` 后直接跑该 exe；判成"测试挂了"之前先这样确认一次。

## 文档索引

- **文档总入口（两层索引：教程层 + 参考层）**：`docs/README.md`；渐进教程系列（面向采用者的 10 章学习路径）在 `docs/tutorial/`
- 各端规范：`frontend/admin/{react,vue-element,vue-vben}/AGENTS.md`（react 为唯一维护端；vue-element / vue-vben 已冻结，其 AGENTS.md 仅作参照）
- 后端：`docs/backend_project_struct.md`、`docs/backend_deploy.md`、`docs/audit-log-producer-design.md`
- 前端权限模型：`docs/frontend_authority.md`
- 查询/分页规则：`docs/list_query_rule.md`
- **改一个字段速查**：`docs/field_change_guide.md`（给**已有**资源加字段的四档判定 + 命令数 + 文件清单，含"哪些事明确不用做"）——本仓最高频的任务，别套用 04 章的新模块流程；接到"加个字段/页面少一列"这类需求先读它
- **数据库结构版本化迁移**：`docs/database/design/ent-goose-migrations.md`（goose 版本化迁移：改表结构标准五步流程、危险变更 expand–migrate–contract、存量库纳管、CI 漂移守卫、`cmd/migrate` CLI；改任何表结构前必读，排"迁移失败/结构漂移/老库升级"时读它）
- **新增 CRUD 模块全流程**：`docs/crud_module_guide/`（新增完整业务模块的端到端手册：README 跨端编排与通用陷阱、backend.md 后端全流程与样例镜像索引、三端各自的页面实现参考；正文为英文原稿）——接到「新增模块/新增资源/新建管理页」这类需求、动工新增整个模块前先读它；只给已有资源加字段读上面的速查，别套新模块流程
- 脚本系统：`docs/script_system.md`（Lua/JS 脚本级插件：钩子点/定时任务/HTTP 出站/安全模型；改钩子点或模块先读它）
- 认证与令牌链路：`docs/authentication.md`（登录全流程/令牌与刷新轮换/MFA/限流策略/会话吊销/已知问题；改登录、令牌、刷新、MFA、限流或登录策略前先读它）
- 多租户隔离：`docs/tenant_isolation.md`（上下文链路/HTTP 闸门/数据层读写隔离/套餐联动/覆盖边界与排障；改隔离层、Api 表、套餐门禁或给新表接租户前先读它）
- 套餐与计费管控：`docs/plan_billing.md`（三档到期策略全链路/模块白名单/配额与用量计量/租户数据清理；改套餐、配额、到期处置或排租户 403 前先读它）
- 任务调度系统：`docs/task_system.md`（配置与启动链/任务数据模型/调度生命周期/系统级常驻任务/脚本任务桥/多租户语义与排障；加任务类型、排"任务没跑"或接新调度需求前先读它）
- SSE 推送架构：`docs/sse_architecture.md`（服务端配置与生命周期/流鉴权与 streamID 语义/事件生产/三端消费/部署与排障；改推送链路、加事件类型或排"收不到通知"前先读它）
- 数据权限范围：`docs/data_scope_design.md`（角色级行数据范围：语义/聚合/接入步骤/运维边界；新表接入数据范围或改聚合规则先读它）
- 设计语言规范：`docs/design-language.md`（三端视觉唯一权威值表，改颜色/圆角/布局尺寸先改这里再同步三端）
- AI 模块：`docs/ai_module.md`（模型提供商/流式对话/用量配额/知识库 RAG/脚本 ai 模块/安全洞察/智能问数的架构与部署要求；改 AI 相关代码、接新模型、部署 pgvector 或排租户 AI 403 前先读它）
- 教程第 10 章：`docs/tutorial/10-ai-module.md`（AI 功能的采用者上手路径：配置三步/使用/集成）
