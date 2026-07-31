# 后台任务、实时同步与可靠性

前端实时通道现在由共享连接中心管理：同一用户端或管理端只建立一条 SSE 连接，页面按事件类型订阅；断线时使用最后事件 ID 续传，并在恢复后重新同步页面数据。后台事件还会按当前角色的模块读取权限过滤，避免自定义角色接收到无权查看的业务事件。

第四阶段把第一阶段预留的 `operation_tasks`、`system_events`、`job_runs` 正式投入使用，并新增 `worker_heartbeats`。

## 执行模型

Web 管理员发起耗时操作时，API 只校验权限、确认风险并写入任务表，随后立即返回 `202`。Bot 主进程中的 Task Worker 负责执行：

1. 使用带条件的数据库更新领取一个任务。
2. 写入 Worker ID 和租约到期时间，避免多个实例重复执行。
3. 执行期间每 12 秒续租并写入心跳。
4. 成功后保存结构化结果；失败后按 5、10、20 秒等指数退避重试。
5. Worker 意外退出时，其他 Worker 在租约过期后自动恢复任务。

默认支持：

- `sync.favorites`：同步 Emby 收藏
- `sync.moviepilot`：同步 MoviePilot 下载/入库状态
- `maintenance.partition_access`：检查并回收过期分区授权
- `maintenance.expired_accounts`：执行到期账户维护
- `maintenance.backup_database`：创建数据库备份

有数据修改风险的任务要求前端和 API 双重确认。任务创建、取消和重跑均进入审计日志。

Task Worker 默认随 `main.py` 启动。若 Bot 与 FastAPI 分进程运行，Worker 仍应由 Bot 进程承载，因为部分任务需要已经连接的 Telegram Client。可以使用 `SAKURA_TASK_WORKER_ENABLED=0` 禁用某个 Bot 实例的 Worker。

## 实时同步

共享业务服务在同一数据库事务中写入 `system_events`。FastAPI 的 Event Relay 轮询未发布事件、唤醒本进程的 SSE 客户端并填写 `published_at`。

实时接口：

```text
GET /api/v1/events/stream
GET /api/v1/admin/events/stream
```

用户流只返回 `aggregate_type=user` 且属于当前 Telegram ID 的事件；管理流需要 Telegram 管理员身份和 `tasks:read` 权限。

SSE 客户端使用事件 ID 续传。即使反向代理断线或事件由另一个 API 实例处理，重新连接后仍会从数据库游标补齐。服务每 15 秒发送注释心跳，并返回 `X-Accel-Buffering: no`。

Nginx 反向代理 Web API 时建议为 SSE 路径配置：

```nginx
proxy_http_version 1.1;
proxy_buffering off;
proxy_cache off;
proxy_read_timeout 1h;
```

## 管理 API

```text
GET  /api/v1/admin/task-definitions
GET  /api/v1/admin/tasks
GET  /api/v1/admin/tasks/{task_id}
POST /api/v1/admin/tasks
POST /api/v1/admin/tasks/{task_id}/cancel
POST /api/v1/admin/tasks/{task_id}/retry
GET  /api/v1/admin/jobs
GET  /api/v1/admin/system/status
```

写操作要求 Telegram 管理员会话、`tasks:update` 权限、CSRF Token；任务创建额外要求 `Idempotency-Key`。

## 可观测性与清理

- APScheduler 每次成功、失败或错过执行都会写入 `job_runs`。
- `/readyz` 分别报告数据库、Task Worker 和 Event Relay 状态。
- 管理后台任务中心展示队列计数、Worker 状态、重试次数和定时任务历史。
- Event Relay 每小时清理已发布 7 天以上的事件、已完成 30 天以上的任务、90 天以上的定时任务记录以及 7 天以上的失联 Worker 心跳。

## 配置

```dotenv
SAKURA_TASK_WORKER_ENABLED=1
SAKURA_TASK_POLL_SECONDS=1
SAKURA_TASK_LEASE_SECONDS=45
SAKURA_TASK_HEARTBEAT_SECONDS=12
```

租约必须明显长于心跳间隔。生产环境不建议把租约设置低于 30 秒。

数据库升级版本为 `20260730_03`，应用启动时会自动执行 Alembic 迁移。
