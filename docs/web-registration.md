# Web 注册中心与共享注册队列

用户中心默认在 `/{WEB_USER_PATH}/register` 提供注册入口，例如 `/app/register`。管理后台不会提供公开注册页。

## 注册流程

1. 网页创建用途为 `registration` 的一次性 Telegram 验证请求。
2. 用户打开 Bot 深链；Bot 先检查指定群组和频道资格，再让用户确认本次注册。
3. 验证成功后，Web 获得仅属于当前 Telegram 用户的安全会话。
4. 用户填写 Emby 用户名、4–6 位数字安全码；关闭开放注册时，还可以同时核销注册码。
5. API 在一个事务中校验名额、资格、用户名和重复任务，随后写入 `registration.account` 持久化任务。
6. Bot 进程中的 Task Worker 创建 Emby 账号，并通过 `UserService` 写回账号、有效期和安全信息。

Bot 的“创建账户”按钮也调用同一个 `RegistrationService`，不再依赖只存在于 Bot 内存中的注册队列。因此 Bot 与 Web 的名额、资格、排队状态和最终账号数据一致，容器重启不会丢失待处理任务。

## 可靠性与安全边界

- `web_login_requests.purpose` 隔离普通登录令牌和注册验证令牌。
- 注册会话带有独立用途标记，普通登录会话不能提交注册；注册授权最多保留 15 分钟。
- `registration_state` 单行互斥记录串行化名额预留，避免多个用户并发提交时超卖注册名额。
- 同一 Telegram 用户的活动注册任务唯一；相同 `Idempotency-Key` 重试返回原任务。
- 注册码在资格和任务创建成功写入同一事务后才算核销；任务失败或取消时，资格仍保留。
- 通用后台任务 API 不允许管理员直接构造 `registration.account`，避免绕过注册服务的资格校验。
- 安全码、注册码和生成的 Emby 密码不会出现在通用任务列表、审计详情或管理事件中。生成密码仅向任务所属用户和 Telegram 通知展示。
- Emby 远端账号创建成功、但本地写入失败时，会尝试删除远端账号后让任务失败，避免留下孤立账号。

## 运行要求

数据库迁移必须升级到最新版本。生产 Compose 中的 `migrate` 一次性容器会自动执行迁移；显示 `Exited (0)` 表示迁移成功。

注册任务由 Bot 容器中的任务 Worker 执行，需要保持：

```env
SAKURA_TASK_WORKER_ENABLED=1
```

Web 和 Bot 必须连接同一个 MySQL，并使用相同的 `SAKURA_WEB_SESSION_SECRET`。修改 `WEB_USER_PATH` 后，注册页会自动跟随新的用户中心路径，无需重新编译前端。
