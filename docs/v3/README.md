# Sakura v3 重构工作区

Sakura v3 采用新工程并行开发。v2 保持可部署、可回退，但从本文件建立之日起进入功能冻结状态。

冻结基线父版本：`e877e4a`。后续允许的 v2 修复仍会前进，但必须通过盘点快照审查。

## v2 冻结规则

允许修改 v2 的情况：

- 安全漏洞修复；
- 数据损坏、重复入账、权限绕过等高风险缺陷修复；
- 已有功能无法启动或无法部署的兼容性修复；
- 为数据导出、迁移验证和 v3 切换增加的只读工具；
- 测试、文档和可观测性改进，且不改变业务语义。

默认禁止在 v2 中进行：

- 新增大型业务模块或管理页面；
- 新建只服务于 v2 的业务表；
- 复制 Web、Bot 或 Worker 的业务规则；
- 扩大旧版 API 的使用范围；
- 继续以 Telegram ID、Emby 用户名作为新数据的主身份；
- 无迁移结论地修改已有表和字段含义。

确实需要修改 v2 行为时，必须同时：

1. 增加或更新行为测试；
2. 执行 `python scripts/inventory_v2.py` 更新机器快照；
3. 审查快照差异；
4. 更新迁移决策文档；
5. 在提交说明中标记 `v2-exception` 和修改原因。

CI 中的 `scripts/test_v2_inventory.py` 会阻止未记录的 v2 入口、模型、任务和配置漂移。

## v3 不可违反的边界

- Web 不调用 Bot；
- Bot 不直接查询数据库；
- Bot、Web、开放 API 是平等入口；
- Worker 独立执行异步任务；
- 所有入口共享同一套业务用例和事务规则。

## 第 0 阶段产物

- [旧系统完整盘点](./v2-system-inventory.md)
- [逐表迁移决策](./v2-migration-decisions.md)
- [生产环境核验清单](./phase-0-production-checklist.md)
- [机器可读盘点快照](./generated/v2-inventory.json)
- `scripts/inventory_v2.py`：无导入副作用的 AST 盘点工具
- `scripts/test_v2_inventory.py`：冻结与完整性检查

## 阶段状态

| 阶段 | 状态 | 完成条件 |
|---|---|---|
| 第 0 阶段：冻结和盘点 | 仓库基线已完成 | 入口、模型、任务、配置、测试及逐表迁移结论均有基线；生产画像上线前执行 |
| 第 1 阶段：v3 基础骨架 | 已完成 | API、Bot、Worker、Migrate 独立启动，PostgreSQL、Redis、迁移、OpenAPI、Compose 与 CI 就绪 |
| 第 2 阶段：账号和权限 | 未开始 | Web 本地身份不依赖 Telegram，权限与设置统一 |

## 快照使用方式

生成或主动更新快照：

```bash
python scripts/inventory_v2.py
```

只检查工作树是否与已提交快照一致：

```bash
python scripts/inventory_v2.py --check
```

盘点工具不会导入 `bot` 或 `backend`，因此不会创建 Telegram Client、连接数据库或触发 Alembic。
