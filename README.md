# Sakura EmbyBoss v3

Sakura EmbyBoss v3 是面向 Emby 站点的统一运营平台。Web、Telegram Bot、Worker 和开放 API 是彼此独立、地位平等的入口，全部调用同一套 Go 业务服务并共享 PostgreSQL 数据，不再由 Web 依附 Bot，也不再由 Bot 承载后台业务。

## 技术栈

- 后端与任务：Go 1.25
- Web：Vue 3、TypeScript、Vite
- 数据：PostgreSQL 17、Redis 7
- 部署：Docker Compose、独立 API / Worker / Bot / Migrate 进程
- 发布：GitHub Actions 多架构镜像、SBOM、digest 固定部署文件

## 功能范围

统一账号与本地登录、Telegram 身份绑定、Session、RBAC、API Scope、动态设置、凭据中心、审计；会员、邀请码、多 Emby、钱包与不可变账本；批量运营、设备黑白名单、播放同步、风险处置；TMDB、MoviePilot、求片、工单、影评、通知、自动化；旧 v2 数据幂等导入、对账、备份恢复与蓝绿切换。

## Docker 部署

```bash
cd v3
cp .env.example .env
# 编辑 .env 中的密码、主密钥、内部令牌和管理员初始账号
docker compose --env-file .env pull
docker compose --env-file .env up -d
```

默认仅在宿主机回环地址监听：

- Web：`127.0.0.1:8088`
- API：`127.0.0.1:8080`

生产环境应由 Caddy、Nginx 或 1Panel OpenResty 将域名反向代理到 Web 端口。Web 容器会把 `/api/v3`、`/open/v1` 和实时事件转发给 API。

完整变量说明见 [v3/.env.example](v3/.env.example)，迁移和蓝绿切换见 [docs/v3/phase-8-cutover-runbook.md](docs/v3/phase-8-cutover-runbook.md)。

## 镜像发布

仓库只保留 v3 CI。推送到 `master` 后，`Publish Sakura v3 Docker images` 会在质量门通过后发布：

- `233bit/sakura_embyboss:latest`：API、Worker、Bot、Migrate 与迁移工具
- `233bit/sakura_embyboss:web-latest`：Vue Web
- `sha-*` / `web-sha-*`：提交固定标签
- 手动输入版本时额外发布 `v3.0.0`、`web-v3.0.0` 等版本标签

服务器正式切换应优先下载 Actions 生成的 `v3-images.env`，使用镜像 digest 而不是可变标签。

## 开发验证

```bash
cd v3
go test ./...
go vet ./...

cd web
npm ci
npm run typecheck
npm run test
npm run build
```

CI 会额外启动 PostgreSQL、Redis 和 Chromium，验证迁移幂等、业务集成、前端端到端流程与容器构建。

## 旧版归档

旧 Python/MySQL v2 只作为迁移审计材料保存在 `legacy/v2`，不会参与构建、CI、镜像或生产启动。v3 的 `import-v2` 与 `reconcile-v2` 工具仍可读取只读 v2 MySQL 完成最终迁移。

## License

[MIT](LICENSE)
