# Sakura v3 基础骨架

本目录是与 v2 并行开发的全新工程。第 1 阶段只建立运行基础和边界，不包含账号、登录或 Telegram 业务实现。

## 进程边界

| 入口 | 职责 | PostgreSQL | Redis | 运行迁移 |
|---|---|---:|---:|---:|
| `cmd/api` | HTTP API、OpenAPI、查询与命令入口 | 是 | 是 | 否 |
| `cmd/worker` | 后台任务执行器骨架 | 是 | 是 | 否 |
| `cmd/bot` | Telegram 适配器骨架，只调用内部 API | 否 | 否 | 否 |
| `cmd/migrate` | 串行、带校验和的数据库迁移 | 是 | 否 | 是 |

强制约束由 `internal/architecture/boundaries_test.go` 检查。

## 本地启动

准备配置：

```bash
cd v3
cp .env.example .env
```

修改 `.env` 中两个独立密码，然后构建并启动。PostgreSQL 密码会进入连接 URL，请使用 `openssl rand -hex 32` 生成 URL 安全值：

```bash
docker compose --env-file .env up -d --build
```

检查服务：

```bash
docker compose --env-file .env ps -a
curl http://127.0.0.1:8080/health/live
curl http://127.0.0.1:8080/health/ready
curl http://127.0.0.1:8080/api/v3/system/info
curl http://127.0.0.1:8080/openapi.yaml
```

## 独立启动入口

使用本地 Go 工具链时：

```bash
export SAKURA_V3_DATABASE_URL='postgres://sakura:password@127.0.0.1:5432/sakura?sslmode=disable'
export SAKURA_V3_REDIS_ADDRESS='127.0.0.1:6379'
export SAKURA_V3_REDIS_PASSWORD='redis-password'

go run ./cmd/migrate
go run ./cmd/api
go run ./cmd/worker
```

Bot 不读取 PostgreSQL 和 Redis 配置：

```bash
export SAKURA_V3_INTERNAL_API_URL='http://127.0.0.1:8080'
go run ./cmd/bot
```

## 数据库迁移

迁移 SQL 位于 `internal/migrate/sql`，文件名格式为：

```text
000001_foundation.up.sql
```

迁移器保证：

- 使用 PostgreSQL Advisory Lock，避免多个实例并发迁移；
- 每个迁移在独立事务中执行；
- `schema_migrations` 保存版本、名称、SHA-256 和执行时间；
- 已应用迁移不会重复执行；
- 已应用文件被篡改时拒绝启动；
- API、Worker、Bot 不导入迁移包。

重复执行以下命令是安全的：

```bash
go run ./cmd/migrate
go run ./cmd/migrate
```

## 健康检查语义

- `/health/live`：仅表示进程仍在运行；
- `/health/ready`：表示该进程的必要依赖可用；
- API 就绪只检查 PostgreSQL 和 Redis，不检查 Worker 或 Bot；
- Worker 停止不会让 API 的基础查询和健康检查失败；
- Bot 就绪检查内部 API，但不直接检查数据库。

## 配置原则

配置统一使用 `SAKURA_V3_*` 环境变量。日志不会输出数据库 URL 或 Redis 密码。

主要变量：

| 变量 | 使用进程 | 说明 |
|---|---|---|
| `SAKURA_V3_DATABASE_URL` | API、Worker、Migrate | PostgreSQL URL |
| `SAKURA_V3_REDIS_ADDRESS` | API、Worker | Redis 地址 |
| `SAKURA_V3_REDIS_PASSWORD` | API、Worker | Redis 密码 |
| `SAKURA_V3_INTERNAL_API_URL` | Bot | 内部 API 地址 |
| `SAKURA_V3_HTTP_ADDR` | API | API 监听地址 |
| `SAKURA_V3_HEALTH_ADDR` | Bot、Worker | 独立健康监听地址 |
| `SAKURA_V3_LOG_LEVEL` | 全部 | debug、info、warn、error |

## 开发检查

```bash
go fmt ./...
go test ./...
go vet ./...
go build ./...
```

设置 `SAKURA_V3_TEST_DATABASE_URL` 后，测试还会真实执行两次迁移并验证迁移账本没有重复记录。

## 第 1 阶段验收映射

| 验收项 | 实现 |
|---|---|
| 四个入口独立启动 | 四个独立 `cmd/*` main package 和四个镜像内二进制 |
| API 不启动 Bot | API 无 Bot import，Compose 依赖方向为 Bot → API |
| Bot 不自动迁移 | Bot 无数据库、Redis和迁移依赖，架构测试强制检查 |
| Worker 停止不影响查询 | API readiness 不包含 Worker Probe |
| migrate 可重复执行 | 迁移账本、Advisory Lock、事务和集成幂等测试 |
