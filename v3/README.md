# Sakura v3

这是与 v2 并行开发的全新 Go 工程。API、Worker、Bot 和迁移器是四个独立进程；Web、本地账号、Telegram 和开放 API 共用同一套账号与权限模型，Bot 不直接访问数据库。

## 进程边界

| 入口 | 职责 | PostgreSQL | Redis | 自动迁移 |
|---|---|---:|---:|---:|
| `cmd/api` | Web/API、账号、Session、RBAC、设置、凭据与审计 | 是 | 是 | 否 |
| `cmd/worker` | 独立后台任务执行器 | 是 | 是 | 否 |
| `cmd/bot` | Telegram 适配器，只调用内部 API | 否 | 否 | 否 |
| `cmd/migrate` | 串行、校验和数据库迁移 | 是 | 否 | 是 |
| `cmd/import-v2` | v2 MySQL 账号导入工具，默认只预检 | 按模式 | 否 | 否 |

## Docker 启动

```bash
cd v3
cp .env.example .env
```

至少替换 `.env` 中这四类值：PostgreSQL 密码、Redis 密码、64 位十六进制凭据主密钥、独立内部 Bot Token。推荐生成方式：

```bash
openssl rand -hex 32
```

首次启动可临时填写 `SAKURA_V3_BOOTSTRAP_ADMIN_USERNAME` 和 `SAKURA_V3_BOOTSTRAP_ADMIN_PASSWORD`。Owner 创建成功后从 `.env` 删除这两项。

```bash
docker compose --env-file .env up -d --build
docker compose --env-file .env ps -a
curl http://127.0.0.1:8080/health/ready
```

生产环境必须通过 HTTPS 访问，并保持 `SAKURA_V3_COOKIE_SECURE=true`。本地直接使用 HTTP 调试时才改为 `false`。

## Web 注册与登录

Web 不依赖 Telegram：

- `POST /api/v3/auth/register` 创建本地账号；
- `POST /api/v3/auth/login` 设置 HttpOnly Session Cookie，并返回 CSRF Token；
- 所有写操作同时校验 Session、权限和 `X-CSRF-Token`；
- `auth.local_registration_enabled` 可动态关闭公开注册；
- `auth.session_ttl_hours` 的新版本会作用于后续创建的 Session。

完整合同见 `/openapi.yaml` 或 `api/openapi.yaml`。

## Telegram 绑定

1. 已登录用户调用 `POST /api/v3/me/telegram/link-requests` 获取十分钟有效的一次性绑定码。
2. 用户私聊 Bot 发送 `/bind 一次性绑定码`。
3. Bot 使用独立内部 Token 调用 API，API 把 Telegram 身份绑定到现有统一账号。

Bot 不持有数据库地址。Telegram Bot Token 有两种配置方式：

- 推荐：Owner 通过凭据中心写入名为 `telegram.bot_token` 的凭据；Bot 会通过受信内部 API 获取。
- 首次配置备用：在 `.env` 设置 `SAKURA_V3_TELEGRAM_BOT_TOKEN`。

## 权限、设置和凭据

- 内置角色：`owner`、`admin`、`user`；可创建自定义角色并分配权限。
- 系统角色不可被覆盖，最后一个 Owner 不可被移除。
- 所有 `/api/v3/admin/*` 路由都在服务端校验对应权限。
- 动态设置采用乐观版本号，更新和回滚都会生成不可变的新修订。
- 凭据用 AES-256-GCM 加密，普通管理接口只返回掩码；解密仅开放给持有内部服务 Token 的进程，并记录审计。
- 开放 API Token 只在创建时返回一次，数据库只保存哈希，并对每次调用校验 Scope。
- 账号支持 `pending / active / suspended / banned / deleted` 生命周期；离开 active 状态会撤销现有 Session。

## 第一版 v2 账号导入器

导入器读取旧 MySQL，写入新 PostgreSQL。默认是 dry-run，不写目标库，也不会输出密码哈希或其他秘密。

```bash
export SAKURA_V2_DATABASE_DSN='user:password@tcp(mysql:3306)/sakura?parseTime=true'
./sakura-import-v2
```

确认报告后再执行：

```bash
export SAKURA_V3_DATABASE_URL='postgres://sakura:password@postgres:5432/sakura?sslmode=disable'
./sakura-migrate
./sakura-import-v2 --apply
```

导入使用确定性映射和唯一约束，可重复执行；旧版 `scrypt` 本地密码哈希保留兼容验证，用户下次可以直接用原密码登录。正式切换前仍应备份 MySQL 和 PostgreSQL，并先核对 dry-run 数量。

## 开发检查

```bash
go fmt ./...
go test ./...
go vet ./...
go build ./...
```

设置 `SAKURA_V3_TEST_DATABASE_URL` 后，测试会真实执行迁移幂等校验和第 2 阶段账号/权限集成验收。
