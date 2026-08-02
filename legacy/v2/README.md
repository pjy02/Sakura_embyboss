# Sakura EmbyBoss v2 archive

这里保存正式切换前的 Python/MySQL v2 源码、配置样例、运维脚本和文档，仅用于迁移审计与紧急只读回退。

- 此目录不参与根仓库 CI、Docker 镜像或生产启动。
- 不要把这里的 Compose、Dockerfile 或 Actions 当作 v3 部署入口。
- v3 最终导入使用 `v3/cmd/import-v2`，对账使用 `v3/cmd/reconcile-v2`。
- 生产回退应使用切换前保留的 v2 镜像、只读数据库和原部署目录，不应从此归档重新构建。

活动项目入口位于仓库根目录的 `v3`。
