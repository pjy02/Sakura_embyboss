# 平台能力中心

本版本把设备策略、媒体资料、外部服务、自动化和开放 API 放入 Bot、Web 与独立 Worker 共用的业务层。所有写操作都会记录审计事件，Bot 旧配置只作为未完成迁移时的兼容回退。

## 首次配置顺序

1. 在 `.env` 设置独立且稳定的 `SAKURA_CREDENTIAL_MASTER_KEY`，不得在已经保存凭据后随意更换。
2. 启动服务并完成数据库迁移，在「系统设置」开启 TMDB、多 Emby、MoviePilot 或开放 API。
3. 在「集成与凭据」先添加凭据：
   - `tmdb`：TMDB Read Access Token；
   - `moviepilot`：MoviePilot Bearer/Access Token；
   - `emby`：Emby API Key，可为每个实例单独建立凭据。
4. 添加 Emby 实例并选择默认实例，执行一次探测。之后 Web/Bot 注册队列会优先在默认实例创建账号；没有数据库实例时仍使用 `config.json` 中的单 Emby。
   - 从单 Emby 升级时，在实例列表点击「迁移旧账号」，把现有账号建立为该实例的绑定；此操作只写映射，不会重复创建 Emby 用户。
   - 「启用多 Emby」关闭后，实例和绑定仍保留，但注册与播放聚合会立即回退到 `config.json`，便于故障切换。
5. 在「系统设置」填写非敏感的 MoviePilot 地址并启用联动。用户从影片中心创建的求片会保留 `tmdb:<type>:<id>`，管理员可在求片详情中直接搜索并提交下载。

## 设备和风险

`device_client_rules` 是客户端规则的唯一动态来源。规则支持完全匹配、包含、通配符和正则，动作包括白名单放行、黑名单拦截和仅观察。升级时会导入 21 个官方/常用兼容客户端白名单；自定义黑名单由管理员按站点实际情况建立。

Webhook 命中黑名单时会按设置终止会话或封禁账号，增加规则命中次数，写入 `device.client_blocked` 风险事件，并继续沿用 Telegram 告警与风险规则。

每个托管 Emby 实例都要使用带实例标识的 Webhook 地址：

```text
https://your-domain.example/emby/webhook/client-filter?instance_id=<实例ID>&token=<SAKURA_LEGACY_API_TOKEN>
```

该入口属于受保护的兼容 API，因此需要同时设置 `SAKURA_LEGACY_API_ENABLED=true` 和独立的 `SAKURA_LEGACY_API_TOKEN`。实例 ID 可从管理后台接口或浏览器网络响应中取得；不要在多个实例间复用错误的实例 ID。

## 自动化和独立 Worker

自动化支持系统事件模式（如 `request.*`、`device.*`、`service.*`）与时间间隔，动作限定为创建后台任务、发送 Telegram 告警、创建风险事件。任意命令和脚本不能从后台页面执行。

独立 Worker 负责自动化扫描、外部服务与多 Emby 探测、MoviePilot 同步、每日数据库备份，以及原有注册、通知和账号生命周期任务。Bot 容器只处理 Telegram 更新，停止 Bot 不会停止这些任务。

## 开放 API

开放接口前缀为 `/api/open/v1`，默认关闭。开启后使用后台生成的 Bearer Key，Key 只显示一次，数据库仅保存 SHA-256 摘要。最小权限包括 `health:read`、`media:read`、`requests:read`、`requests:create` 和 `events:write`。

```bash
curl -H 'Authorization: Bearer sk_sakura_xxx' \
  'https://your-domain.example/api/open/v1/media/search?q=沙丘'
```

## 备份与恢复

备份中心可创建、列出、下载 SQL 备份并展示 SHA-256。为了避免网页误操作覆盖生产库，不提供在线恢复按钮。恢复时停止 Bot、Web 与 Worker，人工校验目标文件后使用 MySQL 客户端导入，再启动迁移和服务。

镜像回退不会自动降级数据库。每次上线前应同时保留 SQL 备份、`config.json` 和 `.env` 的安全副本。

生产上线建议先运行 `python3 scripts/preflight.py --env-file .env --config config.json`，再运行 `bash scripts/deploy.sh`。脚本会先创建数据库与配置备份，等待 MySQL、迁移、Bot、Worker 和 Web 的容器状态；失败时使用上线前镜像回退。数据库迁移仍然只向前执行，因此涉及表结构的版本回退必须先恢复对应 SQL 备份。
