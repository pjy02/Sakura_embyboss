# Sakura v2 数据迁移决策

本文给出当前 57 个 ORM 模型的逐表迁移结论。正式上线前还必须连接生产 MySQL，对比 `information_schema`，补充仅存在于生产库的历史表和列。

决策类型：

- **转换**：清洗并导入新的领域模型；
- **合并**：与其他旧表合并后导入；
- **归档**：保留审计或历史，但不进入实时业务表；
- **重建**：不迁移行数据，由 v3 根据当前状态重新生成；
- **失效**：切换时主动终止，不导入。

| v2 表 | 决策 | v3 目标/说明 | 校验要求 |
|---|---|---|---|
| `accounts` | 转换 | `accounts` | 数量、状态、创建时间 |
| `account_identities` | 转换 | `account_identities` | 身份类型和外部 ID 唯一性 |
| `account_memberships` | 转换 | `memberships` | 有效期、方案、账号归属 |
| `membership_plans` | 转换 | `membership_plans` | 权益配置人工复核 |
| `account_tags` | 转换 | `tags` | 名称唯一性 |
| `account_tag_assignments` | 转换 | `account_tag_assignments` | 不允许孤立账号或标签 |
| `account_wallets` | 合并 | `wallets` | 与全部积分/账单汇总对账 |
| `account_ledger_entries` | 合并 | `ledger_transactions`、`ledger_entries` | 每个账户余额一致 |
| `account_lifecycle_events` | 归档+转换 | `account_lifecycle_events`、`audit_logs` | 账号映射和事件顺序 |
| `emby` | 合并 | `accounts`、`account_identities`、`emby_bindings` | TG、用户名、Emby ID、有效期冲突报告 |
| `emby2` | 合并 | `accounts`、`emby_bindings` | 与主实例用户去重 |
| `emby_instances` | 转换 | `emby_instances` | URL、实例状态、凭据引用 |
| `account_emby_bindings` | 转换 | `emby_bindings` | 账号、实例、远端用户三方唯一性 |
| `emby_favorites` | 转换或重建 | `media_favorites` | 默认重建；需要保留用户时间线时转换 |
| `Rcode` | 合并 | `invitation_codes` | 代码、类型、使用人、使用时间 |
| `partition_codes` | 合并 | `invitation_codes`、`entitlement_codes` | 分区权益不能丢失 |
| `partition_grants` | 转换 | `account_entitlements` | 到期时间和 Emby 媒体库一致 |
| `point_transactions` | 合并 | `ledger_transactions`、`ledger_entries` | 幂等键、数额、最终余额 |
| `billing_entries` | 合并 | `ledger_transactions`、`ledger_entries` | 与订单、退款、钱包交叉核对 |
| `recharge_products` | 转换 | `recharge_products` | 商品价格和状态 |
| `recharge_orders` | 转换 | `recharge_orders`、`payment_attempts` | 状态、金额、入账次数、退款 |
| `idempotency_records` | 归档 | `migration_archive.idempotency_records` | 不作为 v3 活跃幂等键 |
| `line_endpoints` | 转换 | `line_endpoints` | 地址、权重、维护状态 |
| `line_health_samples` | 归档+抽样 | `line_health_samples` | 按保留期迁移，旧数据可降采样 |
| `playback_sessions` | 归档+转换 | `playback_history` | 已结束会话转历史，在线状态重建 |
| `known_devices` | 转换 | `devices`、`account_devices` | 设备指纹、账号映射、信任状态 |
| `device_client_rules` | 转换 | `device_rules` | 优先级、动作、启用状态 |
| `security_events` | 转换 | `risk_events` | 严重度、证据、处置状态 |
| `risk_rules` | 转换 | `risk_rules` | 规则类型、阈值、冷却时间 |
| `service_probes` | 归档+抽样 | `service_probe_history` | 近期明细保留，旧数据聚合 |
| `alert_deliveries` | 归档+转换 | `notification_deliveries` | 渠道、状态、尝试次数 |
| `media_catalog_items` | 重建 | `media_catalog` | TMDB 可重取，仅保留被业务引用的条目 |
| `media_requests` | 转换 | `media_requests` | 用户、影片、状态、MoviePilot 引用 |
| `request_records` | 合并 | `media_requests`、`migration_archive.request_records` | 与新求片去重，保留原始记录 |
| `media_reviews` | 转换 | `media_reviews` | 作者、影片、审核状态 |
| `review_reactions` | 转换 | `review_reactions` | 用户与影评唯一性 |
| `review_reports` | 转换 | `review_reports` | 举报人、原因、处理结果 |
| `support_tickets` | 转换 | `support_tickets` | 用户、状态、负责人、时间 |
| `ticket_messages` | 转换 | `ticket_messages` | 顺序、作者、内部备注隔离 |
| `user_notifications` | 转换 | `notifications` | 接收者、已读状态、业务引用 |
| `notification_preferences` | 转换 | `notification_preferences` | 用户和渠道配置 |
| `automation_rules` | 转换后禁用 | `automation_rules` | 导入后管理员复核再启用 |
| `automation_runs` | 归档 | `automation_run_archive` | 保留结果，不恢复执行状态 |
| `operation_tasks` | 归档 | `job_archive` | 运行中任务切换前停止，不继续消费 |
| `job_runs` | 归档 | `job_run_archive` | 只保留诊断历史 |
| `worker_heartbeats` | 失效 | 无 | v3 Worker 启动后重建 |
| `system_events` | 归档 | `event_archive` | 不重新投递旧事件 |
| `registration_state` | 失效+人工处理 | 无或迁移异常清单 | 切换前清空队列，未完成注册进入异常报告 |
| `web_sessions` | 失效 | 无 | 切换后所有用户重新登录 |
| `web_login_requests` | 失效 | 无 | Telegram 登录请求不迁移 |
| `web_roles` | 转换 | `roles` | 权限名映射到 v3 权限目录 |
| `web_role_members` | 转换 | `role_assignments` | 账号映射和 Owner 安全检查 |
| `audit_logs` | 转换 | `audit_logs` | 操作者、资源、时间、详情完整 |
| `dynamic_settings` | 转换后校验 | `settings` | 按 v3 Schema 校验，未知项进入报告 |
| `config_revisions` | 归档+转换 | `setting_revisions` | 仅转换仍存在的设置键 |
| `managed_credentials` | 重新加密 | `credentials` | 必须提供旧主密钥；逐项连接测试 |
| `api_clients` | 转换但轮换密钥 | `api_clients` | Scope 映射；旧密钥默认失效并重新签发 |

## 配置迁移矩阵

| v2 来源 | v3 去向 | 规则 |
|---|---|---|
| 数据库连接、监听地址、主密钥 | 环境变量/Docker Secret | 不进入数据库 |
| Bot Token、Emby Key、TMDB Key、MoviePilot Token | 凭据中心 | 使用 v3 主密钥重新加密 |
| 注册、签到、兑换和会员开关 | 动态设置 | 类型校验并产生初始版本 |
| 设备、线路和媒体库规则 | 对应领域表 | 不继续存为匿名 JSON |
| 管理员和 Owner | Account + Role Assignment | 必须至少保留一个 Owner |
| Web 管理路径、用户路径、品牌 | 部署设置/动态品牌设置 | 路径继续允许自定义 |
| Scheduler 开关 | Job Schedule | 迁移后默认暂停，复核再启用 |
| 自动更新配置 | 不迁移 | 由 Actions 和 Docker 镜像更新替代 |

## 正式迁移前的硬性校验

1. 生产 MySQL 中的真实表集合必须与本清单对比。
2. `accounts`、`emby`、`emby2` 的身份映射不能产生静默覆盖。
3. 每个钱包必须输出旧余额、交易汇总、新余额和差额。
4. 每个已使用邀请码必须能定位使用者或进入异常清单。
5. 每个 Emby Binding 必须与远端 Emby 用户重新核对。
6. 至少保留一个可登录的 Owner，且完成权限验证。
7. 旧 Session、登录请求和运行中任务不得继续执行。
8. 凭据必须逐项解密、重新加密并进行连接测试。
9. 所有迁移异常必须显式导出，禁止静默跳过。
10. 迁移工具必须支持 dry-run、重复运行和确定性输出。

