# 交易、工单与求片

第三阶段在共享业务层上增加充值交易、服务工单和求片订阅。Bot 与 Web 继续共用同一套 MySQL 数据、积分流水、审计日志和实时事件。

## 充值交易

当前实现为人工核验模式，不包含第三方支付 SDK，也不会由网页自动扣款：

1. 管理员在“充值中心”维护商品、金额、基础积分和赠送积分。
2. 用户在个人中心选择商品并提交订单，可填写转账备注。
3. 管理员核验实际收款后确认入账；订单、积分余额、积分流水和账单流水在同一事务内写入。
4. 待确认订单可以由用户取消；已入账订单不会重复记账。

创建订单必须携带 `Idempotency-Key`。管理员重复确认已入账订单时只返回原结果，不会再次增加积分。账单流水记录订单创建、确认入账、拒绝和用户取消事件。

## 服务工单

用户可以创建账户、播放、充值、求片、技术或其他类型工单，并在网页中与管理员连续对话。管理员可以：

- 搜索和筛选工单；
- 分派负责人、调整优先级和状态；
- 回复用户；
- 添加仅管理员可见的内部备注。

所有用户端查询都按当前登录 Telegram ID 隔离。普通用户不能读取其他用户工单，也看不到内部备注。已关闭工单不能继续回复。

## 求片订阅

Web 用户可以提交作品名称、年份、类型和补充要求，随后查看审核、搜索、下载、入库或拒绝状态。管理员可维护进度、MoviePilot 下载 ID、积分成本和处理备注。

Telegram Bot 原有 MoviePilot 点播流程保持可用。Bot 成功创建下载任务后会同时写入新的 `media_requests` 表，定时同步任务会把下载和入库状态更新到 Web。升级迁移也会将现有 `request_records` 数据导入新表。

管理员也可以给 Web 求片填写 MoviePilot 下载 ID，并将状态设为“已批准”“搜索中”或“下载中”；定时任务会同时扫描新旧记录，自动同步下载进度、入库结果和 Telegram 通知。

## 权限

新增权限域：

- `billing:read`、`billing:update`、`billing:*`
- `tickets:read`、`tickets:update`、`tickets:*`
- `requests:read`、`requests:update`、`requests:*`

系统角色在迁移时自动获得相应权限。用户自助接口不使用管理权限，但必须通过登录、CSRF 校验和 Telegram ID 数据隔离。

## 主要接口

用户接口位于 `/api/v1/me`：

- `/recharge/products`、`/recharge/orders`
- `/billing/ledger`
- `/tickets`、`/tickets/{id}/messages`
- `/requests`、`/requests/{id}/cancel`

管理接口位于 `/api/v1/admin`：

- `/recharge/products`、`/recharge/orders/{id}/decision`
- `/billing/ledger`
- `/tickets`、`/tickets/{id}`
- `/requests`、`/requests/{id}`

部署新版本时，现有 `migrate` 容器会执行 `20260731_05` 数据库迁移，无需新增环境变量。
