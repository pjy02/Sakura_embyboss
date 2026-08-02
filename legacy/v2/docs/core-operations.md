# 核心运营模块

第二阶段把仪表盘、站点账号、在线播放、播放历史、设备管理和线路管理接入同一套业务与权限体系。

## 数据流

- Bot 进程每隔 `SAKURA_OPERATIONS_SYNC_SECONDS` 秒读取 Emby `/emby/Sessions`。
- 当前会话写入 `playback_sessions`，离线会话自动补记结束时间。
- 会话中的客户端、设备、用户和 IP 汇总到 `known_devices`。
- Web 的在线播放页面也会主动同步，默认每 15 秒刷新一次。
- 线路配置保存在 `line_endpoints`，每次探测在 `line_health_samples` 留下样本。
- Bot 的服务器面板、注册结果和账号创建消息优先读取这里的启用线路；可分别配置普通线路和白名单专属线路。
- 停止播放、设备信任/封禁、线路新增/修改/探测都写入审计日志并产生系统事件。

数据库迁移 `20260731_04` 会自动建表，并为内置角色补充以下权限：

- `dashboard:read`
- `playback:read`、`playback:stop`
- `devices:read`、`devices:update`
- `lines:read`、`lines:update`

所有核心运营管理接口仍要求 Telegram 确认的后台会话。写操作同时要求 CSRF 校验。

## 管理接口

- `GET /api/v1/admin/dashboard/core`
- `GET /api/v1/admin/playback/live`
- `GET /api/v1/admin/playback/history`
- `POST /api/v1/admin/playback/{session_id}/stop`
- `GET /api/v1/admin/devices`
- `PATCH /api/v1/admin/devices/{device_key}`
- `GET|POST /api/v1/admin/lines`
- `PATCH /api/v1/admin/lines/{line_id}`
- `POST /api/v1/admin/lines/{line_id}/probe`
- `GET /api/v1/admin/lines/{line_id}/health`

## 上线

镜像启动时会自动执行 Alembic 迁移。更新镜像后按原方式重新创建容器即可：

```bash
docker compose --env-file .env up -d --pull always --force-recreate
```

可在 `.env` 中设置同步周期，最小值为 10 秒：

```dotenv
SAKURA_OPERATIONS_SYNC_SECONDS=30
```

线路管理当前只维护线路目录、启停/维护状态、权重和健康探测，不会直接修改 Caddy、Nginx 或 DNS。这样可以避免一次后台误操作影响生产入口；自动发布和回滚将在线路发布阶段单独接入。

升级后如果数据库中还没有线路，Bot 会继续使用 `config.json` 的 `emby_line` 与 `emby_whitelist_line`。创建第一条后台线路后，数据库线路成为 Bot 和 Web 的共同数据源；被停用或进入维护的线路不会再发给用户。
