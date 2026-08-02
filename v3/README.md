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
| `cmd/reconcile-v2` | v2/v3 账号、余额、账本和 Emby 对账门禁 | 是 | 否 | 否 | 否 |

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

## 钱包、交易与批量运营

钱包余额不是数据库中的可编辑字段，而是从不可变双重记账分录实时计算。每笔充值、退款、管理员调整和会员购买都必须同时产生金额相等的借方与贷方；数据库触发器拒绝不平衡交易，并拒绝修改或删除已过账交易。

典型配置顺序：

1. 在凭据中心创建 `payment.webhook.<渠道名>`，类型填写 `payment_webhook`，密文是该支付渠道调用回调接口时使用的 Bearer Token。
2. 管理员创建充值商品和会员商品。
3. 用户创建充值订单，支付渠道调用 `POST /api/v3/internal/payments/{provider}/callback`。
4. 回调的 `event_id` 与支付渠道外部订单号共同参与幂等控制；重复回调只会记录事件，不会重复增加余额。
5. 退款以相反分录冲销原充值；退款比例必须与商品的支付金额和钱包额度一致，余额不足时拒绝退款。
6. 管理员增减余额也只能提交带原因和幂等键的平衡调整交易，不能直接编辑余额。

批量运营支持以下操作：

- 批量添加或移除账号标签；
- 批量延长或分配会员；
- 批量写入站内通知，或通过 Bot 的可靠通知队列发送 Telegram 消息；
- 按明确账号集合、账号状态或已有标签固定目标集合。

每个批量任务和每个目标账号都有独立状态。任务支持暂停、恢复、失败项重试和取消；执行效果、目标项状态与审计记录采用事务提交，管理端可查看失败账号和错误原因。Worker 停止不会丢失进度，重新启动后会接管未完成或租约过期的任务。

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

第 8 阶段的生产副本演练、加密备份恢复、压测、故障演练、蓝绿发布和正式切换见
[`../docs/v3/phase-8-cutover-runbook.md`](../docs/v3/phase-8-cutover-runbook.md)。所有生产部署都应使用 Docker Hub 工作流生成的 digest 固定镜像。
对账命令会扫描生产 MySQL 的全部真实表；未知或尚未实现活动域迁移且非空的表会返回退出码 2，阻止维护窗口继续。

随后先添加对应 Emby 实例，再调用 `POST /api/v3/admin/emby/instances/{id}/adopt-legacy`。正式切换前仍应同时备份旧 MySQL 与新 PostgreSQL。

## 开发检查

```bash
go fmt ./...
go test ./...
go vet ./...
go build ./...
```

设置 `SAKURA_V3_TEST_DATABASE_URL` 后，测试会在独立 PostgreSQL schema 中真实运行迁移，并验收幂等建号、失败重试、多实例绑定、远端导入、认领与快照，以及影片匹配、重复求片、MoviePilot 去重、通知重试和工单备注隔离。

完整 HTTP 合同见运行时 `/openapi.yaml` 或 [api/openapi.yaml](api/openapi.yaml)。

## 播放、设备与风险中心

Worker 会按 `playback.sync_interval_seconds` 为每个已启用的 Emby 实例分别创建 `emby.playback_sync` 任务。在线播放是可更新快照，播放历史会在换片或会话结束时收尾；设备画像按实例、远端用户和设备键聚合，不把单次会话当成永久设备。

- 设备规则支持全局或指定实例、允许名单、拒绝名单、精确/包含/前缀/正则匹配。内置常见客户端允许名单可以禁用或调整优先级，自定义规则使用同一业务层。
- 允许名单优先于拒绝名单；已允许设备仍可产生观察事件，但不会触发自动封禁或停止播放。
- 风险规则支持并发播放、转码、高码率、远端地址和自定义字段条件。`observation_mode=true` 时只保存证据、规则快照和告警，不创建远端处置任务。
- 自动处置通过独立 `risk.action` 任务执行。每个事件都保存命中原因、证据、当时的规则版本、处置前后状态和时间线。
- 管理员标记误判时，未执行动作会被取消；已经执行的账号禁用会创建 `risk.revert` 补偿任务并恢复处置前状态。正在执行的动作会拒绝并发撤销，待动作完成后可再次提交。
- 风险 Telegram 接收账号通过动态设置 `risk.telegram_alert_account_ids` 配置；`risk.notify_affected_account` 控制是否同时通知已绑定 Telegram 的受影响账号。消息复用可靠通知队列。
- 每个 Emby 实例独立累计失败并开启熔断，阈值和冷却时间由 `risk.max_instance_failures`、`risk.circuit_cooldown_seconds` 控制。一个实例请求失败只会重试该实例的任务，其他实例仍会继续采集。

## 影片、求片、工单、影评与通知

用户在 `GET /api/v3/media/search?q=片名` 中按名称搜索 TMDB。API 会把结果缓存为内部影片项，用户随后只提交内部 `media_id`，不需要查找或手工填写 TMDB ID。新求片会为每个已启用 Emby 实例创建独立匹配任务；只要任一实例命中，求片即标记为已入库并通知所有订阅者。

- 同一影片同时只能有一条活动求片。Web 与 Bot 重复提交时会订阅同一条求片并返回 `duplicate=true`，不会创建重复运营单。
- 管理员先调用 `GET /api/v3/admin/media-requests/{id}/moviepilot/resources`，系统自动使用影片标题搜索 MoviePilot；选择资源后调用对应的 `POST .../moviepilot`。同一影片只保留一个活动或已完成下载任务，外部提交由 Worker 重试并携带稳定幂等键。
- 在凭据中心创建 `tmdb.api_token` 和 `moviepilot.api_token`。动态设置 `tmdb.api_base_url`、`tmdb.language`、`moviepilot.api_base_url`、`moviepilot.search_path`、`moviepilot.submit_path` 可适配代理和不同 MoviePilot 版本；密钥本身不会写入动态设置。
- 工单公开回复和内部备注共用一个时间线，但用户查询始终附带账号归属过滤并强制排除 `is_internal=true`；用户接口也不能创建内部备注。
- 影评默认进入待审核状态。管理员审核结果使用乐观版本控制，并按用户通知偏好写入站内信和 Telegram 可靠队列。
- 广播复用可暂停、重试、审计的批量任务。用户可按事件与渠道关闭通知；被偏好过滤的目标会记录为已跳过，不会被当作发送失败。
- 自动化规则订阅持久化业务事件，目前支持通知账号、提交 MoviePilot 和变更求片状态。每个“事件 + 规则”最多成功执行一次，失败会记录原因并按退避策略重试。

Telegram 通知由 Bot 进程租约领取。发送失败后会保存错误、递增尝试次数并延迟重试；Worker、API 或 Bot 重启不会丢失待发送消息。

## 第 7 阶段 Web 与 Bot

最终 Web 位于 `web/`，使用 Vue 3、TypeScript、Pinia 和 Vue Router。它只通过由 `api/openapi.yaml` 生成的客户端访问 API，不连接 PostgreSQL、Redis 或 Bot，也不复制会员、交易、求片、风控等业务判断。登录后 API 返回当前账号的有效权限，前端仅据此展示可访问的管理模块，服务端仍对每一次操作执行 RBAC 与审计。

生产环境执行 `docker compose --env-file .env up -d --build` 后：

- Web 默认监听 `127.0.0.1:8088`，通过 Nginx 将 `/api/` 和 SSE 实时数据流转发给 API；
- API 默认监听 `127.0.0.1:8080`，不启动 Web、Bot、Worker 或迁移；
- Web 只依赖 API，不依赖 Bot；Bot 只依赖内部 API，不依赖 Web；
- 停止 `bot` 后，本地注册、登录、Emby、交易、求片、工单和管理后台仍可使用；
- 停止 `web` 后，Bot 命令、按钮、管理查询和可靠 Telegram 通知仍可使用。

Bot 用户命令包括 `/start`、`/media`、`/request`、`/requests`、`/tickets`、`/ticket`、`/notifications`、`/bind`、`/register` 与 `/register-status`；拥有对应 RBAC 权限的账号还可使用 `/admin`、`/users`、`/tasks`、`/risks` 和 `/broadcast`。命令与内联按钮均调用 `/api/v3/internal/bot/actions` 共享业务门面，Bot 本身不持有业务规则或数据库权限。

前端开发与验收：

```bash
cd v3/web
npm ci
npm run generate:api
npm run typecheck
npm run test
npm exec vite build
npm run test:e2e
```

`generate:api` 直接读取唯一 OpenAPI 合同；CI 会重新生成并检查差异。Playwright 同时覆盖桌面和移动端，验证独立本地登录、用户中心、权限化管理后台和响应式导航。
