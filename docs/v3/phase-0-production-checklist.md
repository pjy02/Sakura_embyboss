# 第 0 阶段生产环境核验清单

仓库盘点不能替代生产数据核验。本清单在正式编写 v2 → v3 导入器前执行，不改变生产数据。

## 1. 先创建可恢复备份

使用现有备份中心或 `mysqldump` 创建完整 MySQL 备份，同时单独保留：

- 当前 `.env`；
- 当前 `config.json`；
- `SAKURA_CREDENTIAL_MASTER_KEY`；
- Docker Compose 文件；
- 当前运行镜像的精确 SHA/Tag。

备份文件和主密钥不能存放在同一个公开位置。

## 2. 生成无业务值的数据库画像

更新到包含盘点工具的镜像后，在项目目录执行：

```bash
docker compose --env-file .env exec -T web \
  python3 scripts/profile_v2_database.py \
  --output /app/db_backup/v2-database-profile.json
```

因为 `/app/db_backup` 已映射到宿主机 `./db_backup`，结果应出现在：

```text
./db_backup/v2-database-profile.json
```

该文件包含：

- MySQL 版本；
- 表名、列名、字段类型；
- 索引；
- 表大小和估算行数；
- Schema 指纹。

不包含用户名、密码、Token、Telegram ID、Emby 用户名、订单内容或其他业务字段值。

若需要精确行数，在低峰期增加 `--exact-counts`。大型播放历史表执行精确计数可能较慢。

## 3. 对比仓库模型和生产表

核对生产画像中的表集合与：

```text
docs/v3/generated/v2-inventory.json → database_models
```

生产库额外表必须加入 `v2-migration-decisions.md`；生产缺失表必须记录其最后迁移版本，不能假设为空。

## 4. 生成迁移前统计报告

正式导入器需要只输出聚合统计，不输出用户敏感值：

- 账号总数及各状态数量；
- 本地、Telegram、Emby 身份数量；
- 各 Emby 实例绑定数；
- 钱包总余额和各类交易合计；
- 邀请码总数、已用数、孤立使用记录；
- 充值订单各状态数量和金额合计；
- 设备、风险事件、求片、工单、影评数量；
- 运行中注册和后台任务数量；
- 无法关联统一账号的旧记录数量。

聚合报告将在 v3 导入器第一版中实现，不能通过临时 SQL 手工修改数据。

## 5. 外部服务契约核验

记录但不要公开凭据：

- 每个 Emby 实例版本、Server ID 和可达地址；
- MoviePilot 版本和已使用接口；
- TMDB 语言、地区和图片配置；
- Telegram Bot 用户名及 Bot API/MTProto 必要能力；
- 哪吒或 Komari 是否仍实际使用；
- Caddy/Nginx 的真实转发路径和 Header。

## 6. 完成标志

以下条件全部满足后，生产侧第 0 阶段才算完成：

- 完整备份已验证可读取；
- 数据库画像已保存；
- 生产额外表均有迁移结论；
- 账号、余额、邀请码、Emby 绑定有聚合基线；
- 外部集成清单与真实部署一致；
- 当前镜像和配置可回退；
- 画像及报告中不存在明文敏感数据。

