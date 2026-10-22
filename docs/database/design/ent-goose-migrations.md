# 数据库结构版本化迁移设计（Ent + goose）

> 状态：已实施（2026-10-21 起）。本文是数据库结构变更（DDL）管理的唯一权威文档。
> 改登录/令牌链路看 `docs/authentication.md`，改数据访问层模式看 `docs/backend_project_struct.md`——改**表结构**前必读本文。

## 1. 背景与问题

历史方案：Ent 内置自动迁移。服务启动时若 `data.yaml` 的 `data.database.migrate: true`，
`ent_client.go` 执行 `client.Schema.Create(ctx, migrate.WithForeignKeys(true))`，把当前库与最新
Ent schema 求差并应用差量 DDL。

它的能力边界（也是本次改造的动因）：

- **无版本记录**：库处于"哪个版本"无从知晓，跨版本升级没有审计依据；
- **只加不减**：删字段/索引/表不会跟随 schema 删除，库里留下孤儿结构；
- **危险变更无落点**：改列名被当作"加新列"、改列类型不做校验、数据回填完全没有位置；
- **无回滚**：出问题只能手工修；
- **并发隐患**：`Schema.Create` 在服务启动路径里执行，多副本同时启动会并发抢 DDL。

## 2. 决策

**DDL 的唯一执行通道 = goose 版本化迁移；Ent schema 保持"结构与代码生成的单一事实源"，不再直接落库。**

选 goose 而非 Atlas / golang-migrate / 手自研的理由：

| 考量 | goose 胜出的点 |
|---|---|
| 部署体验 | 迁移 SQL `embed` 进二进制，采用者 `docker compose up` 即完成升级，零新组件 |
| 回滚 | Up/Down 写在同一个文件里，回滚路径显式可见 |
| 并发 | 内置 PostgreSQL 会话级 advisory lock（`lock.NewPostgresSessionLocker`），多副本天然安全 |
| 生态契合 | 纯手写 SQL、方言无关——这也是为将来可能的信创数据库适配预留的逃生舱（见 §10） |

代价（接受的取舍）：失去 Ent 的 schema diff 能力后，"改了 schema 忘写迁移 SQL"成为主要风险，
由 §7 的 CI 漂移守卫机器化兜底；goose 版本表无内容校验和，靠 review 纪律 + CI 补偿。

## 3. 组件与落点

```
backend/app/admin/service/
├── cmd/migrate/main.go                        # 运维 CLI：up / status / automigrate-ent
└── internal/data/
    ├── migrations/                            # 迁移通道（唯一 DDL 入口）
    │   ├── migrations.go                      # embed FS + goose Provider 装配 + Up/Status
    │   └── 00001_init.sql                     # 基线：当前全量结构（2026-10-21 从参考库导出）
    ├── ent_client.go                          # 启动链：migrate=true 时先跑 goose，Schema.Create 已移除
    └── ...
```

- **migrations 包**：`go:embed *.sql` + `goose.NewProvider(DialectPostgres, db, fsys,
  WithSessionLocker(...))`。运行库连接用的驱动与 Ent 相同（配置 `driver: "postgres"` →
  `lib/pq`），且**只接受 postgres**（全仓铁律 4）。
- **版本表**：`goose_db_version`（goose 默认，自动建）。查当前版本：`goose status` 或直接查表。
- **cmd/migrate CLI**：服务二进制之外的运维入口，CI 与手工操作共用一套代码路径：
  - `go run ./app/admin/service/cmd/migrate up -dsn <dsn>` —— 应用全部未执行迁移；
  - `... status -dsn <dsn>` —— 查看迁移状态；
  - `... automigrate-ent -dsn <dsn>` —— 仅限**空库**：按 Ent schema 全量建表（漂移守卫用，见 §7）。

## 4. `migrate` 配置开关（语义变更）

`data.yaml` 的 `data.database.migrate` 保留但语义变更：

- `true`（默认）：服务启动时自动执行 goose 迁移（嵌入 SQL，无外部依赖）；
- `false`：启动**不做**任何 DDL——用于多副本部署（迁移交给部署链单独一步）或 DBA 管控变更的场景。

> GORM 构建标签（`gorm_backend`，骨架演示用途）未接入本机制，其自身 AutoMigrate 行为不变，
> 见 §9 边界。

## 5. 标准工作流：改一个表结构

以"给已有实体加一个字段"为例，全流程五步：

1. **改 Ent schema**：`internal/data/ent/schema/*.go` 加字段，`gow ent` 重新生成（类型安全、
   代码生成不受影响）；
2. **写迁移 SQL**：`internal/data/migrations/` 下新建 `000NN_<module>_<what>.sql`，
   Up 必写、Down 尽量写：
   ```sql
   -- +goose Up
   ALTER TABLE sys_user ADD COLUMN nickname VARCHAR(64) NULL;

   -- +goose Down
   ALTER TABLE sys_user DROP COLUMN nickname;
   ```
3. **本地验证**：起本地栈，重启服务（`air` 或 `go run`），观察 goose 应用日志；
4. **CI 漂移守卫**（§7）在 push 时自动验证迁移与 Ent schema 一致；
5. **提交**：schema 与迁移 SQL 必须**同一个 commit**——CI 会拦，但纪律在前。

**编号规则**：新迁移取当前最大版本号 +1，一个逻辑变更一个文件，禁止改已合入的迁移文件
（goose 无校验和，改历史文件等于制造隐形漂移）。

## 6. 危险变更纪律（改名 / 删列 / 改类型 / 回填）

goose 把这些操作从"不可能"变成"显式可控"，遵循 expand–migrate–contract：

1. **expand**：迁移加新列（或新表），发布并跑稳；代码双写新旧两列；
2. **migrate**：数据回填——纯 SQL 写进迁移文件；需要业务逻辑的沿用 Go 侧启动播种守卫
   （`pkg/constants/default_data.go` 的 count==0 模式），不要塞进 SQL；
3. **contract**：确认读路径切换后，出删旧列的迁移（这是唯一允许"删"的时机）。

改列名 = expand（加新列）→ migrate（`UPDATE ... SET new = old`）→ contract（删旧列），三步三个版本。

## 7. CI 漂移守卫

**原理**：同一台 postgres 起两个空库——

- 库 A 走"真实通道"：`cmd/migrate up` 应用全部 goose 迁移；
- 库 B 走"期望结构"：先 `CREATE EXTENSION IF NOT EXISTS vector`，再 `cmd/migrate automigrate-ent`
  按 Ent schema 全量建表；
- 两库 `pg_dump --schema-only` 后 diff（排除 `goose_db_version` 及其序列），不一致即失败。

任何"改了 schema 忘写迁移"或"迁移 SQL 与 schema 打架"都会在这里被拦下。
入口：`make db-drift-check`（本地，需可达的 postgres）与
`.github/workflows/db-schema-drift.yml`（push 到涉及路径时自动跑）。

## 8. 部署与升级

- **全新环境**：`docker compose up` → 服务启动自动应用 00001 全量结构（需 pgvector 镜像，
  基线含 `CREATE EXTENSION IF NOT EXISTS vector`）；
- **已部署实例升级（存量库纳管）**：`pg_dump` 备份 → 换新版本二进制 → **先跑一次
  `cmd/migrate baseline`**（把基线版本标记为"已应用"而不执行，幂等）→ 再启动服务，
  之后走增量迁移。顺序不能反：存量库上直接 `up` 会对已存在的表重新 CREATE 而失败；
- **多副本**：所有副本 `migrate: false`，部署链先跑一次 `cmd/migrate up` 再起服务
  （advisory lock 仍然兜底并发）；单实例直接 `migrate: true` 即可；
- **种子数据**（admin 用户/菜单/Api 表等）照旧由 Go 侧启动播种（`pkg/constants/default_data.go`），
  与结构迁移无关，**不要**写进迁移 SQL。

## 9. 边界与已知限制

- **goose 无迁移内容校验和**：已应用文件被篡改不会被发现。防线 = 禁改历史文件（§5）+ code review；
- **AI 向量子系统的运行期动态 DDL 是已登记的唯一例外**（铁律 6 的边界）：`sys_ai_chunks.embedding`
  向量列既不在 ent schema 也不在基线里——ent 无法建模 pgvector 自定义列型（`ent/schema/ai_chunk.go`
  头注释），该列由 `data.EnsureVectorColumnDim` 在知识库首次使用时按需补建/定维重建，HNSW 索引按行数
  门槛由 `EnsureHnswIndexIfLarge` 自动落，`sys_ai_search_index` 全文检索表由 `ai_content_service`
  首用即建。这些是幂等的运行期自适应 DDL（表名是参数、时机取决于数据量），无法表达为静态版本文件；
  漂移守卫因此天然不受它们干扰（基线不含、ent 侧也不建，两侧一致）。**除此之外不允许任何运行期 DDL**；
- **`gorm_backend` 构建标签未接入**：GORM 后端是骨架演示（大量写路径 not implemented），
  其 AutoMigrate 保持原样；未来若真切换 GORM 后端（备案方案：GORM + goose），本迁移通道原样复用；
- **只支持 postgres**：runner 显式拒绝其他 driver（铁律 4）；`data.yaml` 里 mysql 示例属历史遗留；
- **基线不可回滚**：00001 的 Down 是占位注释。全量重建 = 重新 `migrate up`，不做"回滚到空库"；
- **enttest 内存库测试不受影响**：`*_sqlite_test.go` 走 enttest 自建 schema，与本机制正交
  （历史遗留，铁律 4 不得扩展）。

## 10. 信创备案（不出本方案的范围）

本方案下若未来部署目标变为国产库（达梦/金仓/openGauss）：Ent 数据层整体不支持，届时切换
GORM 后端 + 保留 goose 是备案路径——goose 手写 SQL 天然方言无关，`migrations/` 目录与
`cmd/migrate` 原样保留，只换每份 SQL 的方言。该备案不构成本方案的额外成本。

## FAQ

**Q：本地开发第一次起服务，库里啥都没有？**
直接起。`migrate: true` 会自动应用全部迁移建好结构，Go 侧播种会灌入系统默认数据。

**Q：怎么知道库现在是什么版本？**
`go run ./app/admin/service/cmd/migrate status -dsn <dsn>`，或查 `goose_db_version` 表。

**Q：迁移写错了还没合入？**
本地 `go run ./app/admin/service/cmd/migrate` 无法直接撤销未发布的文件——直接删掉该文件后
把库 drop 重建（或 down 再删），重新 up。已合入/已部署的迁移文件严禁修改（§5）。

**Q：老库起服务报 `CREATE TABLE` 已存在 / 迁移失败？**
存量库没做纳管：先 `cmd/migrate baseline`（幂等）再启动服务，见 §8。

**Q：漂移守卫红了怎么办？**
说明迁移 SQL 与 Ent schema 不一致：大概率是忘了写迁移、或 SQL 里写了 schema 里没有的东西。
以 Ent schema 为准补齐/修正迁移文件。
