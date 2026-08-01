# Sakura v3 重构工作区

v3 与可部署的 v2 并行开发，v2 进入功能冻结状态。Web、Bot 和开放 API 是平等入口，共享统一账号和业务层；Bot 不访问数据库，Worker 独立运行。

## 阶段状态

| 阶段 | 状态 | 已完成内容 |
|---|---|---|
| 第 0 阶段：冻结和盘点 | 已完成 | v2 入口、模型、任务、配置、测试和迁移决策基线 |
| 第 1 阶段：v3 基础骨架 | 已完成 | Go、PostgreSQL、Redis、迁移、独立入口、健康检查、OpenAPI、Compose、CI |
| 第 2 阶段：账号和权限 | 已完成 | 统一账号、本地注册登录、Telegram 绑定、Session、RBAC、API Scope、动态设置、凭据、审计、生命周期、v2 导入器 |

## 不可违反的边界

- Web 不调用 Bot；
- Bot 不直接访问 PostgreSQL 或 Redis；
- API 启动不会启动 Bot，Bot 启动不会执行迁移；
- Worker 停止不影响 API 基础查询；
- 管理操作必须经过服务端权限校验和审计；
- 设置变更必须有版本记录，凭据不得明文存储或出现在日志中。

## 相关文档

- [v2 系统完整盘点](./v2-system-inventory.md)
- [逐表迁移决策](./v2-migration-decisions.md)
- [生产环境核验清单](./phase-0-production-checklist.md)
- [v3 运行与第 2 阶段说明](../../v3/README.md)

v2 确需修复时，必须同步更新行为测试、运行 `python scripts/inventory_v2.py`，并审核盘点快照差异。
