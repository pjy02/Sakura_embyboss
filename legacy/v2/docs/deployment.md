# Docker、GitHub Actions 与生产上线

第五阶段把 Bot、Web API、数据库迁移和 MySQL 拆成可独立检查的容器，并让 Docker Hub 发布复用同一套后端测试、前端类型检查和镜像构建验证。

## 1. 首次准备

服务器需要 Docker Engine 和 Docker Compose v2。先在项目目录准备配置文件：

```bash
cp config_example.json config.json
cp deploy.env.example .env
```

必须完成以下修改：

1. 在 `config.json` 中填写 Telegram Bot、Emby、owner/admin 等业务配置。Compose 会通过环境变量覆盖数据库地址和密码，因此不需要把真实数据库密码再写进 JSON。
2. 在 `.env` 中设置镜像名、两个 MySQL 密码、`SAKURA_WEB_SESSION_SECRET`、`SAKURA_CREDENTIAL_MASTER_KEY`、公网 HTTPS 地址和可信域名。凭据主密钥至少 32 字节，必须独立于会话密钥；保存过 TMDB、MoviePilot 或 Emby 凭据后不得更换，否则历史密文将无法解密。
3. 把 `WEB_ADMIN_PATH` 改成 3-64 位、难以猜测且不是 `admin` 的路径。`WEB_USER_PATH` 是用户中心路径。
4. 推荐用 `openssl rand -hex 32` 分别生成 Web 会话密钥、凭据主密钥和数据库密码，不要相互复用，也不要复用 Bot Token。

如果首次部署不准备使用 Telegram 登录，可在 `.env` 临时设置 `SAKURA_BOOTSTRAP_ADMIN_USERNAME` 和 `SAKURA_BOOTSTRAP_ADMIN_PASSWORD`。Web 首次启动会把该登录身份绑定到配置中的 Owner，数据库只保存 scrypt 摘要。确认本地管理员能够登录后删除这两个环境变量；后续启动不会覆盖已经存在的 Owner 本地密码。

生产 Compose 默认只把 Web 绑定到 `127.0.0.1:8838`，MySQL 不对宿主机开放端口。请让 Caddy 或 Nginx 把 HTTPS 站点反向代理到该地址。

## 2. 启动与检查

```bash
docker compose pull
docker compose up -d
docker compose ps
curl -fsS http://127.0.0.1:8838/healthz
curl -fsS http://127.0.0.1:8838/readyz
```

启动顺序由 Compose 保证：

```text
MySQL 健康 -> migrate 完成数据库迁移 -> Bot / Worker / Web 独立启动
```

`migrate` 是一次性容器，完成后显示 `Exited (0)` 属于正常状态。Bot 只承载 Telegram 交互，Worker 独立执行注册和后台任务，Web 提供 FastAPI、用户中心和管理后台。停止 Bot 不会中断 Web 本地账号登录和注册队列。

完成首次配置后，也可以使用带配置预检、数据库备份、健康等待和失败回退的一键上线脚本：

```bash
bash scripts/deploy.sh
```

完整流程见 [实时同步、测试与上线手册](release-runbook.md)。

Compose 也会禁止多个启动进程同时回写 `config.json`；后续通过 Bot 配置面板产生的显式保存仍然有效。

查看日志：

```bash
docker compose logs -f --tail=200 bot worker web migrate
```

升级或切换镜像：

```bash
docker compose pull
docker compose up -d --remove-orphans
```

回滚时把 `.env` 的 `SAKURA_IMAGE` 改成上一个明确版本，例如 `your-user/sakura_embyboss:2.1.0`，然后再次执行上面的 `pull` 和 `up`。生产环境推荐固定版本标签；`latest` 更适合持续更新环境。

如果旧部署使用 MySQL 5.7 或宿主机目录保存数据库，请先生成逻辑备份，再导入新的 MySQL 8.4 实例。不要直接把未经备份的 5.7 数据目录挂到 8.4。

## 3. 测试与镜像验证

本地完整入口：

```bash
python -m pip install -r requirements-test.txt
python scripts/run_test_suite.py
cd web
npm install
npm run typecheck
npm run build
```

有 Docker 的机器再执行：

```bash
docker build -t sakura-embyboss:local .
docker compose config
```

`scripts/run_test_suite.py` 只在缺少 `config.json` 时临时复制示例配置，结束后会清理，不覆盖现有生产配置。真实 Emby 注册压力测试仍需显式设置 `REGISTER_QUEUE_REAL=1`，不会被 CI 自动执行。

## 4. GitHub Actions 与 Docker Hub

在 GitHub 仓库的 `Settings -> Secrets and variables -> Actions` 添加：

- `DOCKER_USERNAME`：Docker Hub 用户名。
- `DOCKER_TOKEN`：Docker Hub Personal Access Token，推荐只授予目标仓库写权限。

为兼容旧仓库，工作流也接受原来的 `DOCKER_PASSWORD`，但优先使用 `DOCKER_TOKEN`。

三个工作流职责如下：

- `CI`：分支推送、Pull Request 或手动运行；执行 Python 测试、Vue 类型检查与双前端构建，并验证生产 Dockerfile。
- `Publish Latest Docker image`：`master` 推送或手动运行；完整测试通过后发布 `latest` 和 `sha-*`，平台为 `linux/amd64`、`linux/arm64`。
- `Publish Release Docker image`：GitHub Release 发布，或手动输入 `v2.1.0`；测试通过后发布精确版本、SemVer 别名、提交标签，并按选择更新 `latest`。

发布镜像带 OCI 元数据、SBOM 和 provenance，构建缓存保存在 GitHub Actions。任何后端测试、前端构建或 Docker 构建失败都会阻止推送。

## 5. 上线安全清单

- 公网只开放 80/443，Web 容器保持回环绑定，MySQL 不暴露。
- 只有在 Web 保持回环绑定并由受信任反向代理访问时，才使用 `SAKURA_FORWARDED_ALLOW_IPS=*`。
- `SAKURA_COOKIE_SECURE=true`，公网地址必须是 HTTPS。
- `SAKURA_TRUSTED_HOSTS` 只填写实际域名；多个值用逗号分隔。
- 管理路径使用随机值，但仍以登录、权限、CSRF 和审计为真正安全边界。
- API 文档和旧兼容 API 默认关闭；需要时临时开启并设置独立令牌。
- 上线前备份数据库和 `config.json`，升级后检查 `/readyz`、Bot Worker 心跳和任务中心。
