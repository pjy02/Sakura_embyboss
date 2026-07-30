# FastAPI 登录与权限体系

## 启动方式

推荐让 Bot 和 Web API 使用同一个镜像、不同进程：

```bash
python main.py
python -m backend.main
```

兼容旧部署时，`main.py` 仍会根据 `config.json` 中的 `api.status` 在 Bot 进程内启动 Web API。生产环境建议关闭内嵌模式并独立运行，以免同步数据库或外部请求阻塞 Bot。

Bot 与 API 两个进程必须获得同一个 `SAKURA_WEB_SESSION_SECRET`，否则 Bot 无法确认 API 创建的一次性登录请求。启用安全 Cookie 时缺少该变量会拒绝启动，而不会静默使用 Bot Token。

## 必要环境变量

```dotenv
SAKURA_WEB_SESSION_SECRET=请替换为至少32字节的独立随机字符串
WEB_ADMIN_PATH=sakura-console-k7fd92
SAKURA_PUBLIC_BASE_URL=https://emby.example.com
SAKURA_COOKIE_SECURE=true
```

`WEB_ADMIN_PATH` 只能包含字母、数字、下划线和连字符，长度为 3-64。系统拒绝 `admin`、`manage`、`dashboard` 等常见路径。用户中心默认位于 `/app`，管理中心只在自定义路径提供。

如果必须继续使用旧 Emby webhook/API：

```dotenv
SAKURA_LEGACY_API_ENABLED=true
SAKURA_LEGACY_API_TOKEN=请使用独立随机令牌
```

旧接口不会再默认接受 Bot Token。推荐通过 `X-Sakura-Legacy-Token` 请求头传递专用令牌。

## Telegram 登录流程

1. 浏览器调用 `POST /api/v1/auth/telegram/start`。
2. API 返回一次性请求令牌和 Telegram Deep Link。
3. 用户在 Bot 中打开链接并点击“确认登录”。
4. 浏览器轮询 `POST /api/v1/auth/telegram/status`。
5. 确认后调用 `POST /api/v1/auth/telegram/exchange`。
6. 服务端写入 `HttpOnly + Secure + SameSite=Lax` 会话 Cookie，并下发单独的 CSRF Cookie。

请求令牌和会话令牌都只以 HMAC 摘要形式存入数据库。一次性登录请求默认 5 分钟失效，成功换取会话后不能再次使用。

Emby 用户名和密码也可通过 `POST /api/v1/auth/emby` 登录用户中心。即使该 Telegram 用户拥有管理员角色，Emby 登录得到的会话也不能调用管理 API；管理操作必须使用 Telegram 确认登录。

## 权限模型

- `owner`：所有权限，只能由配置中的 owner Telegram ID 获得。
- `admin`：用户、兑换码、分区、任务、审计和安全数据管理。
- `operator`：日常用户、兑换码、分区和任务操作。
- `auditor`：用户、任务、审计和安全数据只读。
- `user`：自己的资料和流水。

权限支持精确值和命名空间通配，例如 `users:read`、`users:update`、`users:*`。角色成员保存在 `web_role_members`，只有 owner 能增删角色。

管理写操作同时要求：

1. 有效会话；
2. 登录方式是 Telegram；
3. 对应 RBAC 权限；
4. `X-CSRF-Token` 请求头正确。

会产生余额或状态变化的接口还要求 `Idempotency-Key` 请求头。同一登录用户使用相同键重试时只返回第一次结果，不会重复扣款或加款。

## 当前 API

```text
POST /api/v1/auth/telegram/start
POST /api/v1/auth/telegram/status
POST /api/v1/auth/telegram/exchange
POST /api/v1/auth/emby
GET  /api/v1/auth/session
POST /api/v1/auth/logout
POST /api/v1/auth/logout-all

GET  /api/v1/me
GET  /api/v1/me/point-transactions

GET  /api/v1/admin/users
GET  /api/v1/admin/users/{tg}
POST /api/v1/admin/users/{tg}/points
GET  /api/v1/admin/audit
GET  /api/v1/admin/roles
PUT  /api/v1/admin/users/{tg}/role
```

健康检查：

```text
GET /healthz
GET /readyz
```

API 文档默认关闭。只应在受控环境通过 `SAKURA_DOCS_ENABLED=true` 临时开启。
