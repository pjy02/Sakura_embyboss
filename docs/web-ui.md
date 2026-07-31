# Sakura Web 用户中心与管理后台

第三阶段提供两套 Vue 3 + TypeScript 单页应用，共用 FastAPI 会话、权限和业务数据：

- 用户中心：默认 `/app`
- 管理后台：由 `WEB_ADMIN_PATH` 指定，默认示例为 `/sakura-console`

管理后台入口不会出现在用户中心页面，也不会在常见的 `/admin`、`/manage`、`/dashboard` 路径响应。

## 已实现功能

用户中心：

- Web 注册中心（默认 `/app/register`）
- Telegram Bot 扫码/跳转确认登录
- Emby 用户名和密码登录
- 账户、等级、积分、注册额度和有效期概览
- 积分与注册天数流水
- 当前会话退出、全部设备退出
- 桌面端与移动端自适应布局

管理后台：

- Telegram 管理员强制登录
- 运营指标和用户等级分布
- 用户搜索、等级筛选、分页和详情
- 积分/注册天数调整（幂等请求、CSRF、权限与审计保护）
- Owner 分配或移除后台角色
- 角色权限说明
- 审计日志时间线
- 后台任务中心、Worker 健康状态和定时任务运行记录
- SSE 实时任务进度与用户数据自动刷新
- 根据当前权限隐藏不可用的导航与页面

后台任务、租约重试和实时事件的详细设计见 [task-reliability.md](task-reliability.md)。

## 本地开发

```bash
cd web
npm install
npm run dev:portal
```

管理后台开发服务：

```bash
npm run dev:admin
```

开发服务器会将 `/api` 转发到 `http://127.0.0.1:8838`。FastAPI 服务需要使用与 Bot 相同的数据库和 `SAKURA_WEB_SESSION_SECRET`。

## 生产构建

```bash
cd web
npm run typecheck
npm run build
```

构建结果分别位于：

- `web/dist/portal`
- `web/dist/admin`

FastAPI 会在运行时把它们挂载到配置的用户路径和管理路径。`runtime-config.js` 动态注入路径、API 前缀、Bot 用户名和 CSRF Cookie 名称，因此修改 `WEB_ADMIN_PATH` 或 `WEB_USER_PATH` 后不需要重新构建前端。

Dockerfile 已加入独立 Node 构建阶段。现有 GitHub Actions 使用该 Dockerfile，因此推送到 Docker Hub 的镜像会自动包含两套 Web 界面。

## 安全边界

- 浏览器只保存服务端签发的 Cookie，不包含 Bot Token、Emby API Key 或数据库密码。
- 会话 Cookie 为 HttpOnly；所有写操作必须携带 CSRF Token。
- 管理接口仍以服务端 RBAC 为最终授权依据，隐藏菜单不代替权限检查。
- 管理员通过 Emby 密码登录时不能调用管理接口，必须使用 Telegram 完成管理员身份确认。
- 静态资源使用严格 CSP、安全响应头和长期哈希缓存；API 响应禁止缓存。
