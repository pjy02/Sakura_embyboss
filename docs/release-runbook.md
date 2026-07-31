# 实时同步、测试与上线手册

第五阶段将页面实时更新收敛为每个登录区域一条 SSE 连接。多个页面共同复用连接；断线后按 1、2、4、8、16、30 秒退避重连，携带最后事件游标续传，并在恢复连接后主动刷新当前页面数据。

后台事件流会根据当前登录者的实际权限过滤：

- `reviews:read` 只能订阅影评事件；
- `notifications:read` 只能订阅通知事件；
- 交易、工单、求片、任务、播放、设备、线路和用户事件分别使用各自的读取权限；
- 普通用户始终只能收到以本人 Telegram ID 为聚合对象的事件。

页面顶部会显示“正在连接”“实时同步”或“正在重连”。这只是实时通道状态；业务数据仍以数据库查询结果为准。

## 上线前检查

在服务器项目目录执行：

```bash
python3 scripts/preflight.py --env-file .env --config config.json
```

检查会拒绝示例密钥、HTTP 公网地址、不安全 Cookie、通配可信域名、保留管理路径、重复数据库密码和无效 Bot/Emby 配置。使用 `latest` 只会给出警告；生产环境推荐固定版本，例如：

```dotenv
SAKURA_IMAGE=233bit/sakura_embyboss:2.3.0
```

## 一键升级与失败回退

```bash
bash scripts/deploy.sh
```

脚本依次完成：

1. 配置预检与 Compose 结构检查；
2. 如果 MySQL 已运行，使用一致性快照备份数据库，并备份 `config.json`；
3. 记录当前应用镜像作为临时回滚镜像；
4. 拉取新镜像，等待 MySQL、Bot 和 Web 健康；
5. 新版本未在 180 秒内就绪时，自动切回上线前应用镜像。

备份保存在 `db_backup/releases/`。自动回退只回退应用镜像，不会自动执行数据库降级；当前版本使用新增、扩展型迁移，但上线前仍应保留数据库备份。

使用其他环境文件时：

```bash
bash scripts/deploy.sh /absolute/path/to/production.env
```

## 反向代理与 SSE

Caddy 示例位于 `caddy/web.Caddyfile.example`，Nginx 示例位于 `nginx/web.conf.example`。Nginx 必须为两个事件流路径关闭响应缓冲并提高读取超时，否则页面可能长时间停留在“正在重连”：

- `/api/v1/events/stream`
- `/api/v1/admin/events/stream`

公网只开放 80/443，Docker Web 端口继续绑定 `127.0.0.1:8838`。

## 测试与发布门禁

CI 现在依次运行：

1. 后端业务、认证、并发、实时事件和部署契约测试；
2. Vue 类型检查；
3. 实时连接单元测试；
4. 用户端与管理端生产构建；
5. Compose 配置验证；
6. 生产 Docker 镜像构建。

只有全部通过后，Docker Hub 工作流才会发布镜像。推荐正式上线使用 `Publish Release Docker image` 生成精确版本标签；`latest` 适合测试或持续更新环境。

上线后检查：

```bash
docker compose --env-file .env ps -a
curl -fsS http://127.0.0.1:8838/healthz
curl -fsS http://127.0.0.1:8838/readyz
docker compose --env-file .env logs --tail=200 bot web migrate
```

再从能够访问公网域名的设备运行完整检查：

```bash
python3 scripts/verify_deployment.py \
  --base-url https://emby.example.com \
  --admin-path sakura-console-k7fd92 \
  --user-path app
```

该检查会验证健康状态、两个前端入口、运行配置和关键安全响应头。

`migrate` 显示 `Exited (0)` 是正常完成。`readyz` 会同时验证数据库与 Web 实时事件转发器；任务 Worker 状态会在响应组件中单独显示。
