# 共享业务层

## 目标

Telegram Bot、Web API、后台任务和定时任务共同使用 `bot/application` 中的业务服务。数据库写入不应在新的入口层里直接实现，入口只负责校验输入、调用服务并把结果转换成用户界面响应。

调用方向固定为：

```text
Telegram / Web API / Worker / Scheduler
                  ↓
            bot/application
                  ↓
            bot/repositories
                  ↓
     MySQL models / Emby / MoviePilot
```

## 当前服务

- `UserService`：创建用户、受控字段更新、完成 Emby 注册绑定。
- `PointService`：积分和注册资格调整、签到、积分购买等级。余额行锁、流水、审计、事件和幂等结果在同一事务提交。
- `CodeService`：注册、续期、白名单码核销，以及积分购买注册码。
- `PartitionService`：先预留分区码，Emby 授权成功后再写入授权并核销；Emby 失败时释放预留。

调用者身份统一使用 `Actor`。Telegram 使用 `Actor.telegram(...)`，未来 Web 使用 `Actor.web(...)`，系统任务使用 `Actor.system(...)`。

## 事务和幂等约定

一个业务动作只开启一个 `SqlAlchemyUnitOfWork`。需要修改的用户、注册码或分区码必须先通过仓储的 `get_for_update` 获取行锁。

会被客户端重试的动作必须传入稳定的 `idempotency_key`：

- Telegram：建议使用 `telegram:<chat_id>:<message_or_callback_id>:<action>`。
- Web：使用请求头 `Idempotency-Key`，服务端再拼接已登录用户和动作域。
- Worker：使用任务 ID 和步骤名。

幂等键只能保存业务结果，不保存令牌、密码、注册码全文等敏感响应。

## 新数据表

- 可追溯性：`audit_logs`、`point_transactions`、`security_events`。
- 幂等与事件：`idempotency_records`、`system_events`。
- 异步执行：`operation_tasks`、`job_runs`。
- 动态配置：`dynamic_settings`、`config_revisions`。
- Web 登录与权限：`web_sessions`、`web_login_requests`、`web_roles`、`web_role_members`。

`system_events` 是事务内事件箱：业务提交时只写数据库，后续 worker 再发布到 Redis/SSE，并填写 `published_at`。这样即使进程在提交后重启，Web 页面仍可补发更新。

## 兼容策略

已有 `bot/sql_helper/sql_*.py` 函数暂时保留，避免一次重写全部 Bot 功能。新增功能不得继续扩散直接写表；旧入口按风险逐批迁移到共享服务。

本阶段已经接入共享层的入口包括：

- `/start` 首次录入；
- 注册码、续期码和白名单码核销；
- 每日签到；
- 管理员积分/注册资格调整；
- 积分兑换白名单和注册码；
- 分区码兑换。

数据库升级由 Alembic 自动执行，也可在部署前显式运行：

```bash
alembic upgrade head
```

快速测试：

```bash
python scripts/test_application_services.py
python scripts/test_register_queue.py
python scripts/test_emby_policy.py
```
