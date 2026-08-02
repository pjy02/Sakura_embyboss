# v3 权益、线路、社区与外部联调

## 运行时闭环

- 权益码只在生成响应中返回一次明文，数据库仅保存 SHA-256 摘要和提示。兑换或管理员授权后，会创建 `entitlement.sync` 任务，由 Worker 根据有效权益统一写入 Emby 用户策略的 `EnabledFolders`。权益到期或撤销也由定时任务重新下发。
- 线路管理保存独立修订号、维护状态、排序和权重。实时探测访问 `<线路>/emby/System/Info/Public`，探测样本和管理操作都会留档。
- 影评点赞按账号幂等保存；每个账号对同一影评只能保留一份举报。举报只有管理员可以处理，解决与驳回均写审计日志。
- Emby 收藏以账号绑定为边界。Web 只写期望状态和任务，Worker 负责调用 Emby；定时同步会把远端收藏重新导入，不会让 API 进程依赖 Emby 可用性。

## 凭据和动态设置

请先在“系统与审计 → 凭据元数据”对应的凭据中心保存以下凭据：

| 名称 | 用途 |
| --- | --- |
| Emby 实例配置中的 `credential_name` | Emby API Token |
| `tmdb.api_token` | TMDB Read Access Token |
| `moviepilot.api_token` | MoviePilot API Token |
| `telegram.bot_token` | Telegram Bot Token |

可在动态设置中调整：

- `tmdb.api_base_url`、`tmdb.credential_name`
- `moviepilot.api_base_url`、`moviepilot.credential_name`、`moviepilot.health_path`
- `telegram.api_base_url`
- `lines.probe_timeout_seconds`
- `entitlements.sync_interval_seconds`、`favorites.sync_interval_seconds`

## 真实联调

管理员进入“外部联调”页面，分别执行 Emby、TMDB、MoviePilot、Telegram 探测。服务端会执行真实 HTTPS/HTTP 请求，并保存目标主机、状态、延迟、非敏感版本信息和错误；Token、URL 用户信息及查询参数不会进入探测记录。

也可以在与 v3 相同网络的服务器上复制 `v3/ops/external-smoke.example.sh`，通过只读 secret 文件提供四个 Token，然后运行脚本。它不会把 Token 输出到终端。

真实联调的通过条件：

1. Emby `/emby/System/Info` 返回服务 ID 和版本。
2. TMDB `/3/configuration` 返回配置。
3. MoviePilot 配置的只读健康地址返回 2xx。
4. Telegram `getMe` 返回当前 Bot 身份。

联调成功只证明连接和凭据有效；Emby 权益及收藏还应创建测试账号各执行一次写入与撤销，确认 Worker 任务最终为 `succeeded`。
