# Sakura v3 第 8 阶段：迁移演练与上线手册

本阶段采用“不可变镜像 + 生产副本演练 + 对账门禁 + 蓝绿切换”。脚本不会把演练冒充生产验证：每次演练都产生独立报告，只有账号、会员、余额和账本检查全部通过，正式切换才会继续。

## 1. 一次性准备

1. 在 GitHub Actions 手动运行 `Publish Sakura v3 Docker images`，输入版本号。正式候选版本勾选 `publish_latest`。下载 `sakura-v3-<version>-deployment`，部署必须使用其中带摘要的两个镜像地址，而不是浮动标签。
2. 把 `v3/ops/phase8.env.example` 复制到服务器仓库外的 root-only 文件并填写。反向代理站点使用：

   ```nginx
   include /opt/sakura/runtime/upstream-target.conf;
   proxy_pass $sakura_upstream;
   ```

3. 安装 `age`，离线保存私钥；服务器只保存 `AGE_RECIPIENT` 公钥。备份包包含 MySQL、`config.json`、v2/v3 环境文件和凭据主密钥，因此任何未加密临时文件都会在脚本退出时清理。
4. v2 MySQL 必须能从 `sakura-v3-shared` 网络访问。可以把 MySQL 容器附加到该网络，或者使用服务器内网地址。不要把 3306 暴露到公网。
5. 复制并配置 `v3/ops/external-smoke.example.sh`，将三个 token 分别放在权限为 `0600` 的文件中，再填写 `SAKURA_EXTERNAL_SMOKE_COMMAND`。它以只读方式验证 Telegram `getMe`、TMDB 配置查询和 MoviePilot 健康接口；任何一项失败都返回非零。正式模式不允许跳过这个检查。

## 2. 至少三轮生产副本演练

每轮使用最新生产备份恢复到隔离环境，使用新的空 v3 PostgreSQL。加载配置后执行：

```bash
set -a
source /root/sakura-phase8.env
set +a
bash v3/ops/cutover.sh rehearse
SAKURA_DRILL_ENVIRONMENT=staging bash v3/ops/fault-drill.sh --confirm
BACKUP_ARCHIVE=/backup/sakura/sakura-cutover-xxx.tar.gz.age \
AGE_IDENTITY_FILE=/root/offline-test-age-key \
bash v3/ops/restore-drill.sh --confirm
```

`cutover.sh rehearse` 默认会对本轮候选颜色执行 k6 压测；可用 `VUS`、`RAMP_UP`、`HOLD` 调整压力。只有在调试流程时才设置 `SAKURA_REHEARSAL_LOAD_TEST=0`，该轮不能计入正式演练次数。

每轮记录代码提交、镜像摘要、生产副本时间、数据量、导入耗时、压测 p95/p99、故障恢复时间、备份恢复时间和对账报告。建议顺序是 T-14 天、T-7 天和维护窗口前 24 小时。任一轮存在阻断项都不得上线。

自动对账报告检查：

- v2 账号与 v3 旧账号映射数量；
- 会员记录数量；
- 每种钱包币种的 v2 总额与 v3 迁移双分录总额，以及逐笔历史流水数量；
- 所有已入账交易借贷平衡；
- v2 Emby ID 导入数量，以及尚未认领为实例绑定的数量；
- v2 托管凭据是否逐项解密后使用 v3 主密钥重新加密，以及 Emby 实例/绑定数量；
- 邀请码、充值商品/订单、工单/消息、通知/偏好、动态设置、审计日志和内置角色成员数量；
- 最后一次导入是否完整完成。

报告还会遍历生产 MySQL 的全部实际表，而不只依赖代码清单。非空表分为 `transform`、`archive`、`rebuild`、`invalidate` 和 `pending_adapter`；未知表或 `pending_adapter` 非空会直接阻止切换。这样生产库里额外出现的历史表、未迁完的工单/订单等数据不会被静默跳过。应根据每轮生产副本报告继续补适配器，直到正式候选报告没有 blocker。

`current` 钱包总额与导入总额的差值是告警，因为演练环境可能已有合法新交易；`imported` 与 v2 源总额的差值是阻断项。

## 3. 正式维护窗口

在 v2 仍正常服务时先部署候选色并完成健康检查。确认客服公告、现场负责人、回退负责人和 Emby/MoviePilot/TMDB/Telegram 凭据均已准备，然后执行：

```bash
set -a
source /root/sakura-phase8.env
set +a
bash v3/ops/cutover.sh execute --confirm
```

自动流程按顺序执行：

1. 拉取摘要锁定的镜像并启动候选 Web/API，Bot 和 Worker 此时不会启动；
2. 执行可重复的 PostgreSQL 迁移；
3. 将反向代理切到 503 维护页，停止 v2 Bot/Web 写入者；
4. 加密备份 MySQL、配置、环境文件、凭据主密钥和可选 v3 PostgreSQL；
5. 最终导入账号、身份、会员方案、会员、标签、钱包与历史流水、邀请码、充值商品/订单、工单/消息、通知偏好、设置、审计和内置角色成员；将 v2 托管凭据解密后以 v3 主密钥重新加密，再迁移多 Emby 实例与账号绑定；
6. 生成并强制通过财务与账号对账；
7. 在候选色启动唯一的一组 Worker/Bot，执行每个 Emby 实例的对账任务；
8. 验证 Web、API、Bot、Worker和站点自定义外部集成探测；
9. 原子写入代理目标、校验 OpenResty 配置并 reload；
10. 记录流量开放时间，保留 v2 容器、旧 MySQL、加密备份及非活动颜色。

运行状态保存在 `v3/.state/runs/<run-id>`，包含检查点、导入输出、对账报告和备份路径。失败后不要跳过检查点强行切代理。

## 4. 回退、观察和下线

如果代理尚未向 v3 开放流量，可以执行 `bash v3/ops/rollback.sh --pre-traffic` 恢复 v2 写入者和代理。

一旦 v3 接受过写入，直接恢复可写 v2 会丢失新注册、余额和订单。此时只能执行 `bash v3/ops/rollback.sh --maintenance-only`，让用户回到维护页并对 v3 向前修复，或先设计经过审核的反向增量迁移。

建议至少观察 7 天。期间：

- v2 Bot/Web 保持停止，旧 MySQL 只作为查询/回退证据，不执行清理；
- 每天检查账本平衡、失败任务、Emby 差异、通知积压和错误率；
- 每天做 PostgreSQL 与凭据主密钥的加密备份，并至少完成一次异机恢复；
- 只有连续稳定且业务负责人签字后，才运行 `blue-green.sh retire <旧颜色>`；
- v2 与旧数据库建议再保留 30 天，随后先导出最终归档，再单独审批下线。脚本不会自动删除旧数据库或备份。

## 5. 验收记录

正式上线记录至少包含：Git SHA、两个镜像 digest、备份 SHA256、导入报告、最终对账报告、Emby 任务结果、压测报告、恢复演练报告、维护起止时间、代理切换时间、操作人与复核人。缺少任何一项都不能把第 8 阶段标记为生产验收完成。
