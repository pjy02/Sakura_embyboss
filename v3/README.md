# Sakura v3

Sakura v3 是与 v2 并行开发的 Go 重构版本。API、Worker、Bot、迁移器和旧数据导入器是相互独立的进程；Web 与 Telegram Bot 只作为不同入口，共用同一套账号、会员、邀请码和 Emby 业务服务。

## 进程边界

| 入口 | 职责 | PostgreSQL | Redis | 凭据主密钥 | 自动迁移 |
|---|---|---:|---:|---:|---:|
| `cmd/api` | Web/API、账号、权限、会员、邀请码、多 Emby 管理 | 是 | 是 | 是 | 否 |
| `cmd/worker` | Emby 建号、导入、同步、快照和自动对账 | 是 | 是 | 是 | 否 |
| `cmd/bot` | Telegram 适配器，只调用内部 API | 否 | 否 | 否 | 否 |
| `cmd/migrate` | 串行、校验并执行数据库迁移 | 是 | 否 | 否 | 是 |
| `cmd/import-v2` | v2 MySQL 账号导入，默认只预检 | 按模式 | 否 | 否 | 否 |

停止 Worker 不影响 API 的账号查询与后台浏览；API 或 Bot 启动也不会隐式执行迁移。

## Docker 启动

```bash
cd v3
cp .env.example .env
```

至少替换 PostgreSQL 密码、Redis 密码、64 位十六进制凭据主密钥和独立内部 Bot Token。随机值可使用：

```bash
openssl rand -hex 32
```

首次启动可临时设置 `SAKURA_V3_BOOTSTRAP_ADMIN_USERNAME` 和 `SAKURA_V3_BOOTSTRAP_ADMIN_PASSWORD`，Owner 创建成功后从 `.env` 删除。

```bash
docker compose --env-file .env up -d --build
docker compose --env-file .env ps -a
curl http://127.0.0.1:8080/health/ready
```

生产环境应通过 HTTPS 反向代理 API，并保持 `SAKURA_V3_COOKIE_SECURE=true`。

## 第 3 阶段使用顺序

1. 在凭据中心创建 Emby API Token，例如凭据名 `emby.primary`、类型 `emby_api_token`。
2. 调用 `POST /api/v3/admin/emby/instances` 添加实例。`base_url` 可填写 `http://emby:8096`、`https://example.com` 或带反向代理前缀的地址；末尾 `/emby` 会被自动规范化。
3. 在会员方案中配置有效期和 `max_emby_accounts`，然后给账号分配会员，或生成 TG 兼容格式的邀请码。
4. Web 用户调用 `POST /api/v3/me/emby/provision-requests`。返回的是持久化任务，使用任务 ID 查询状态；建号成功后，生成密码可在 24 小时内读取。
5. Telegram 用户先用 `/bind` 绑定统一账号，再发送 `/register <邀请码或-> <Emby用户名> [实例ID]`。Bot 调用的就是同一共享建号服务，不维护第二套注册逻辑。

所有写请求需要 Session、`X-CSRF-Token`，并建议提供 `Idempotency-Key`。Web 与 Bot 对同一业务请求都会得到相同的任务、绑定和会员结果。

## 多 Emby、导入与认领

- 同一统一账号可在不同 Emby 实例各绑定一个远端账号，上限由当前会员方案的 `max_emby_accounts` 控制。
- 管理员可为实例创建 `import`、`sync`、`reconcile` 任务。导入会把现有 Emby 用户写入远端用户目录，不会自动抢占账号归属。
- 未认领用户可由管理员直接绑定，或生成一次性认领码交给用户在 Web 端认领。
- v2 导入器产生的旧 `emby` 身份，可通过实例的 `adopt-legacy` 接口批量转成新绑定。
- 每次导入、同步和对账均产生不可变状态快照；远端缺失用户会标为 `missing`。

## 可靠性模型

- Worker 使用数据库队列、租约续期、指数退避和最大重试次数，多个 Worker 可安全竞争任务。
- 建号先进行远端用户名预检并持久化创建边界。即使 Emby 已创建用户后网络中断，重试也会认领同一个远端用户，不会再次创建。
- 会员到期、账号停用或封禁后，对账任务会禁用对应 Emby 账号；恢复后会按期望状态恢复。
- Emby Token 由 AES-256-GCM 加密保存，只交给 API 和 Worker；Bot 不持有数据库地址、主密钥或 Emby Token。

## Telegram 命令

```text
/bind <一次性绑定码>
/register <邀请码或-> <Emby用户名> [实例ID]
/register-status <任务ID>
```

Telegram Bot Token 推荐存入凭据中心，名称固定为 `telegram.bot_token`。首次启动也可临时使用 `SAKURA_V3_TELEGRAM_BOT_TOKEN`。

## v2 账号与旧 Emby 身份导入

导入器读取旧 MySQL、写入新 PostgreSQL。默认 dry-run，不会写目标库：

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

随后先添加对应 Emby 实例，再调用 `POST /api/v3/admin/emby/instances/{id}/adopt-legacy`。正式切换前仍应同时备份旧 MySQL 与新 PostgreSQL。

## 开发检查

```bash
go fmt ./...
go test ./...
go vet ./...
go build ./...
```

设置 `SAKURA_V3_TEST_DATABASE_URL` 后，测试会在独立 PostgreSQL schema 中真实运行迁移，并验收幂等建号、失败重试、多实例绑定、远端导入、认领与快照。

完整 HTTP 合同见运行时 `/openapi.yaml` 或 [api/openapi.yaml](api/openapi.yaml)。
