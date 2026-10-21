# start.md — 本地调试启动指南

> 本机（共享开发机）的 5432 / 6379 / 9000-9001 端口已被 1Panel 等其他项目容器占用，
> 仓库自带的 `backend/docker-compose.yaml` 不能原样使用。本文档描述的是**错端口的本地方案**，
> 配套文件（均未入库）：`backend/docker-compose.local.yaml`、`backend/app/admin/service/configs.local/`、`backend/.air.toml`。

## 0. 前置条件（只装一次）

- Go 1.27+、Node 24 + npm、Docker（含 compose v2）
- gow 与 air（装在 `~/go/bin`，默认不在 PATH，先 `export PATH=$PATH:~/go/bin` 或用全路径）：

```bash
go install github.com/tx7do/go-wind-toolkit/gowind/cmd/gow@latest
go install github.com/air-verse/air@latest
```

## 1. 启动基础设施（PostgreSQL / Redis / MinIO）

```bash
cd backend
docker compose -p ymwwgo-local -f docker-compose.local.yaml up -d
docker ps --filter name=ymwwgo   # 三个容器应均为 healthy
```

| 容器 | 宿主端口 | 凭据 |
|---|---|---|
| ymwwgo-postgres（pgvector 版 PostgreSQL 16） | **25432** | postgres / `*Abcd123456`，库名 `gwa` |
| ymwwgo-redis | **26379** | 密码 `*Abcd123456` |
| ymwwgo-minio | **29000**（API）/ **29001**（控制台） | root / `*Abcd123456` |

首次启动已自动完成：gwa 库、`vector` 扩展、`images` 桶；建表与默认数据播种发生在应用首次启动时。数据存在 named volume（`ymwwgo-local_*`），容器重启不丢。

## 2. 启动后端（热重载）

```bash
cd backend
air
```

- air 读取 `backend/.air.toml`：编译 `app/admin/service/cmd/server` 并以
  `-c app/admin/service/configs.local` 启动（覆盖配置指向 127.0.0.1 的上述端口）。
- 保存任何 `.go` / `.yaml` 文件自动重编译重启；**改了 `.proto` 先 `gow api` 重新生成**，air 会随后自动重启。
- 不想用 air 时等价命令：`go run ./app/admin/service/cmd/server -c "$(pwd)/app/admin/service/configs.local"`
- 启动成功标志：日志出现 `[HTTP] server listening on :7788` 和 `[sse] server listening on :7789`。

## 3. 启动前端（react，唯一维护端）

```bash
cd frontend/admin/react
npm install --legacy-peer-deps                     # 首次 / 依赖变更后
npm i rc-field-form --no-save --legacy-peer-deps   # 补缺失 peer，重装依赖后需重跑
npm run dev                                        # http://localhost:15888
```

> 两个参数的原因：antd 6 与 @ant-design/pro-components 2.8.10 的 peer 声明冲突，必须 `--legacy-peer-deps`；
> 而 `rc-field-form` 是 pro-form 的 peer 依赖，antd 5 由 antd 传递提供、antd 6 不再提供，仓库又未提交 lockfile，故需手动补装。

## 4. 登录

- 打开 http://localhost:15888 （vite 已配置 `/admin → 127.0.0.1:7788` 代理，无 CORS 问题）
- 账号：`admin` / `Abcd@1234`（服务首启自动播种）
- 自动化测试登录取数：`GET /admin/v1/captcha` 得 `captchaId` → Redis 读 `gowind:captcha:<captchaId>` 得答案
  （`docker exec ymwwgo-redis redis-cli -a '*Abcd123456' GET gowind:captcha:<id>`）→ `POST /admin/v1/login`。
  **密码须 AES-128-CBC 加密**（key = iv = `frontend/admin/react/.env` 的 `VITE_AES_KEY`，PKCS7，输出 base64），
  并带请求头 `X-Captcha-Id` / `X-Captcha-Value`。

## 5. 日常启停 / 重置

```bash
docker compose -p ymwwgo-local -f backend/docker-compose.local.yaml stop    # 停基础设施（保数据）
docker compose -p ymwwgo-local -f backend/docker-compose.local.yaml start   # 再启
docker compose -p ymwwgo-local -f backend/docker-compose.local.yaml down    # 删容器（数据卷保留）
docker volume rm ymwwgo-local_pgdata ymwwgo-local_redisdata ymwwgo-local_miniodata   # 彻底重置（慎用，会清库重新播种）
```

## 6. 端口速查

| 用途 | 端口 |
|---|---|
| 后端 HTTP API | 7788 |
| 后端 SSE 网关 | 7789 |
| react dev | 15888 |
| PostgreSQL | 25432 |
| Redis | 26379 |
| MinIO API / 控制台 | 29000 / 29001 |

## 7. 其他

- AI 功能调试需要 mock 模型（OpenAI 兼容 echo 服务，端口 18080）与 pgvector（本栈已带），mock 脚本按根 `AGENTS.md`「本地验证要点」重建即可。
- 换机器部署时按本文档重建三个未入库文件即可；若团队决定固化这套本地方案，可将它们与本文档一并提交。
