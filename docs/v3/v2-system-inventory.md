# Sakura v2 旧系统完整盘点

本文档是 v3 重构的人工决策基线。精确入口和源码位置以 `generated/v2-inventory.json` 为准。

## 基线摘要

盘点日期：2026-08-01。

| 项目 | 数量 | 说明 |
|---|---:|---|
| Docker Compose 服务 | 5 | mysql、migrate、bot、worker、web |
| 声明式 HTTP 路由 | 183 | 包含正式 API、旧 API、Webhook、健康检查和 SPA 路由 |
| Bot 处理器 | 174 | 命令、回调、群组事件和 Inline Query |
| 独立 Bot 命令 | 58 | 不含 Callback Query 正则入口 |
| ORM 数据模型 | 57 | 不等同于全部物理表，最终迁移仍需核对生产库 |
| Alembic 迁移 | 13 | 从旧表初始化到平台能力中心 |
| Scheduler 注册点 | 8 | 包含通用包装器和面板动态注册点 |
| 持久化任务类型 | 13 | Worker 可消费的 operation task |
| config.json 配置路径 | 107 | 包含分组键和候选敏感键 |
| 环境变量 | 46 | Python、Compose 和示例部署文件合集 |
| Vue 路由声明 | 41 | 管理后台、用户中心、登录和注册入口 |
| Vue 页面组件 | 36 | `web/src/views` 下的页面级组件 |
| 现有行为测试 | 70 | 不含前端 Vitest 测试 |

数量由 `scripts/inventory_v2.py` 生成，任何 v2 入口变化都必须更新快照并解释原因。

仓库静态盘点不能证明生产数据库不存在历史遗留表。正式迁移前需在生产容器环境执行只读画像：

```bash
python scripts/profile_v2_database.py --output db_backup/v2-database-profile.json
```

默认只读取 `information_schema` 的表、列、索引和估算行数，不导出任何业务字段值。需要精确对账时可在维护窗口增加 `--exact-counts`。

## 当前运行拓扑

```text
mysql   -> MySQL 8.4 数据库
migrate -> 通过 import bot.sql_helper 执行 Alembic 后退出
bot     -> python3 main.py，Pyrogram/MTProto 入口
worker  -> python3 -m backend.worker，数据库任务消费者
web     -> python3 -m backend.main，FastAPI + Vue 静态资源
```

当前虽然已经分成 Bot、Worker、Web 三个容器，但代码层仍存在反向依赖：

- `backend` 直接导入 `bot.application`、`bot.domain`、`bot.sql_helper`；
- `backend.settings`、`backend.worker`、`backend.event_relay` 导入 `bot` 获取配置和日志；
- `bot.__init__` 在模块导入阶段读取配置并创建 Pyrogram Client；
- `bot.sql_helper.__init__` 在导入阶段执行数据库迁移；
- Web 登录部分仍直接调用旧 Emby helper 和旧 `emby` 表。

这些依赖是 v3 必须删除的边界问题，不在 v2 内继续大规模整理。

## 业务功能盘点与 v3 结论

| 领域 | v2 能力 | v3 决策 |
|---|---|---|
| 账号 | 旧 Emby 用户、统一账号、本地身份、Telegram 身份 | 保留能力，统一到 Account + Identity + Emby Binding |
| 登录 | Telegram 确认、本地密码、Emby 密码、Session | 保留；Web 本地登录为独立入口，Emby 登录变成可选身份验证方式 |
| 注册 | Bot 注册、Web 本地注册、TG 验证、共享注册队列 | 保留；合并为同一 RegisterAccount 用例 |
| 邀请码 | Rcode、注册码、续期码、白名单码、分区码 | 保留语义，迁移到统一邀请码/权益码模型 |
| 会员 | 会员方案、有效期、标签、生命周期操作 | 保留并以显式状态机重建 |
| 积分 | point transaction、wallet ledger、签到、兑换 | 保留；全部进入不可变账本，迁移时对账 |
| 充值 | 商品、订单、人工审核、退款、账单 | 保留；订单与复式/平衡账本统一 |
| Emby | 单实例旧配置、多实例、用户创建、策略、媒体库 | 保留；多实例为唯一模型，旧配置只作为导入源 |
| 设备 | 设备画像、信任、封禁、客户端规则 | 保留；规则判断与处置动作分离 |
| 播放 | 在线播放、历史、终止会话、排行榜 | 保留；排行视为播放数据的查询投影 |
| 线路 | 线路、权重、维护、健康探测、上报 | 保留；健康样本设置保留策略 |
| 风险 | 安全事件、规则、处置、告警 | 保留；统一 RiskEvent/RiskDecision/RiskAction |
| 求片 | Bot 求片、Web 求片、MoviePilot 同步、记录 | 保留；TMDB 媒体实体作为请求主引用 |
| 影片 | TMDB 搜索与缓存、Emby 匹配 | 保留；缓存可重建，不作为关键迁移数据 |
| 工单 | 工单、消息、内部备注、状态 | 保留；内部备注继续保持权限隔离 |
| 影评 | 影评、反应、举报、审核 | 保留 |
| 通知 | 站内通知、偏好、广播、Telegram 投递 | 保留；投递渠道插件化 |
| 自动化 | 事件触发、规则、运行记录 | 保留；使用 Outbox 事件作为稳定输入 |
| 任务 | 租约、心跳、重试、取消、事件流 | 保留语义；实现迁移到 Redis 队列 + PostgreSQL 任务记录 |
| 设置 | config.json、环境变量、动态设置、版本回滚 | 重建；启动配置、凭据、动态业务设置三分离 |
| 权限 | Web 角色、成员、权限目录 | 保留；重建 RBAC 和开放 API Scope |
| 审计 | 操作审计、CSV 导出 | 保留；关键写操作必须强制审计 |
| 备份 | mysqldump、下载、摘要 | 重建为 PostgreSQL 备份、恢复任务和恢复演练 |
| 诊断 | 服务探测、风险联动、Telegram 告警 | 保留；标准化 readiness、指标和告警状态 |
| 开放 API | API Client、Scope、Webhook | 保留；单独使用 `/open/v1` 版本空间 |

## HTTP 入口

主要正式 API 模块：

- `backend/api/auth.py`：Telegram、本地和 Emby 登录、Session；
- `backend/api/registration.py`：注册状态、验证、提交和任务查询；
- `backend/api/accounts.py`：统一账号、会员、标签和邀请码；
- `backend/api/admin.py`：用户、积分、角色和审计；
- `backend/api/operations.py`：播放、设备和线路；
- `backend/api/commerce.py`：充值、账单、工单和求片；
- `backend/api/community.py`：影评和通知；
- `backend/api/governance.py`：风险事件和动态设置；
- `backend/api/operations_center.py`：风险规则、诊断和批量运营；
- `backend/api/platform.py`：多 Emby、凭据、TMDB、自动化、备份和开放 API；
- `backend/api/tasks.py`：任务管理和实时事件。

兼容入口位于 `bot/web/api`，生产默认应关闭 `SAKURA_LEGACY_API_ENABLED`。v3 不迁移旧接口实现，只在有真实调用方证据时提供有限期兼容适配器。

## Bot 入口

机器快照识别到 58 个独立命令和 174 个处理器。功能分布：

- `panel`：104 个，承担大量状态面板和业务回调；
- `commands`：44 个，包含用户管理、同步、审核、续期等操作；
- `extra`：19 个，包含红包、建号、反频道等能力；
- `callback`：7 个，包含登录确认、签到、群组事件等。

v3 不逐文件照搬这些 Handler。迁移方式是先将命令映射到业务用例，再由新 Bot 适配器负责参数解析和消息渲染。

需要保留的命令类别：

- 用户自助：`start`、`myinfo`、`score`、`playing`、`watching`、`srank`；
- 注册和账号：建号、删号、续期、用户查询、设备/IP 查询；
- 运维：备份、过期检查、同步、排行榜、配置面板；
- 安全：IP/设备/客户端审核、封禁和白名单；
- 内容：求片、收藏、MoviePilot 下载状态；
- 群组：管理员、用户同步、退出处理、频道规则。

自动更新、直接恢复数据库等高风险命令不会原样迁移，改为后台审批任务。

## 后台任务与调度

持久化任务类型：

- `registration.account`
- `sync.favorites`
- `sync.moviepilot`
- `sync.core_operations`
- `maintenance.partition_access`
- `maintenance.expired_accounts`
- `maintenance.backup_database`
- `monitor.diagnostics`
- `monitor.emby_instances`
- `alert.telegram`
- `notification.telegram`
- `users.batch`
- `automation.evaluate`

调度功能还包括日榜、周榜、过期账号、分区授权、动态设置同步、MoviePilot 同步、Emby 播放同步和诊断。v3 中所有周期任务由独立 Scheduler/Worker 所有，Bot 不再注册系统调度器。

## 外部集成

| 集成 | 当前用途 | v3 处理 |
|---|---|---|
| Telegram MTProto/Pyrogram | Bot、登录确认、通知、群组管理 | 优先切换 Bot API；确需 MTProto 的功能单独列出 |
| Emby | 用户、策略、设备、会话、媒体库、收藏 | 独立多实例 Adapter + 契约测试 |
| TMDB | 影片搜索和元数据 | 独立 Adapter + 有期限缓存 |
| MoviePilot | 搜索、下载、任务状态 | 独立 Adapter + 幂等提交 |
| 哪吒/Komari | 流量或服务信息 | 可选探测 Provider，不进入核心账号模型 |
| MySQL | v2 主数据 | v3 只读迁移来源 |
| Docker | 运行、MySQL 备份 | 改为统一部署与 PostgreSQL 备份 |
| GitHub 自动更新 | Bot 内更新流程 | 不迁移；由 Actions 和镜像发布替代 |
| Caddy/Nginx | HTTPS 和反向代理 | 保留部署模板 |

## 配置来源

当前配置同时存在于：

- `config.json`；
- Docker/系统环境变量；
- `dynamic_settings`；
- `managed_credentials`；
- Compose 默认值。

v3 迁移结论：

- 数据库、Redis、监听地址、主密钥：环境变量或 Docker Secret；
- Telegram、Emby、TMDB、MoviePilot 凭据：凭据中心；
- 注册、会员、设备、风险、通知和品牌规则：动态设置；
- `config.json`：只允许由 v2 导入器读取。

## 已有行为测试基线

现有 70 个 Python 测试覆盖：

- 统一账号、本地身份、会员、标签和钱包；
- 邀请码兼容格式、并发核销和注册队列；
- 登录、Telegram 确认、Session、CSRF 和自定义路径；
- 积分幂等、充值、退款和对账；
- 播放、设备、线路、风险、诊断和批量运营；
- 工单隔离、求片同步、影评审核和通知；
- 角色权限、设置历史与回滚；
- Worker 租约、心跳、失败重试和事件权限；
- 部署、凭据加密、多 Emby、备份和平台路由契约。

这些测试是 v3 用例测试的语义来源，不要求复用测试实现。

仍需在后续阶段补齐：

- 生产数据库脱敏样本的导入对账测试；
- 完整 Bot 命令输入/输出黄金样本；
- Emby、MoviePilot、TMDB HTTP 契约录制；
- 关键 Web 流程 Playwright 测试；
- 备份恢复演练；
- 余额、邀请码、Emby 绑定的全量差异报告。

## 已确认的架构债务

1. 包导入存在运行时副作用。
2. Web/Worker 反向依赖 Bot 包。
3. 旧账号表与统一账号表长期并存。
4. 积分交易与账号钱包账本存在双模型。
5. 配置和凭据来源重叠。
6. Bot Handler 中存在业务逻辑和基础设施调用。
7. 超大服务/helper 文件承担多个职责。
8. 状态和权限大量使用字符串，缺少统一状态机。
9. 外部调用与本地事务边界不清晰。
10. 数据库迁移通过模块导入触发。

以上债务在 v3 新结构中解决，不通过继续扩大 v2 重构范围解决。
