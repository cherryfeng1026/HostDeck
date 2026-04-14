# HostDeck

面向个人或小团队场景的轻量级 Linux 运维面板。后端使用 Go 单进程同时提供 API、定时采集与静态页面托管；前端使用 Vue 3 + Naive UI 展示总览、服务器列表、详情、命令执行和告警页面。

## 当前能力

- 基于会话 Cookie 的登录、登出、当前用户查询、用户列表查询与修改密码
- 三种角色权限：`admin`、`operator`、`viewer`
- 新增独立用户管理页，支持个人改密与管理员只读查看用户列表
- 服务器资产增删改查
- SSH 连通性测试与即时采集
- 状态总览、单机详情、历史趋势
- 单条命令远程执行与命令日志
- 告警规则管理与当前告警视图
- 通知列表、活动流与全局搜索
- PostgreSQL / Neon 持久化
- SSH 密码加密存储，页面只写不回显
- 后端直接托管 `web/dist`，单进程运行整套系统

## 技术栈

- Backend: Go 1.25 + chi + database/sql
- Frontend: Vue 3 + Vite + TypeScript + Naive UI + ECharts
- Database: PostgreSQL / Neon
- SSH: `golang.org/x/crypto/ssh`
- Config: YAML + 环境变量覆盖

## 目录结构

```text
HostDeck/
  docs/
  server/
    cmd/
    config/
    internal/
    scripts/
  web/
```

## 环境要求

- Go 1.25+
- Node.js 20+
- npm 10+
- PostgreSQL / Neon 数据库

如果本机已经把 Go 安装到 `D:\dve\go` 并配置了 `GOROOT` / `Path`，重新打开终端后执行 `go version` 应可直接使用。

## 快速启动

1. 首次启动或前端改动后，先构建前端：

```powershell
Set-Location .\web
npm install
npm run build
```

2. 准备配置文件：

```powershell
Set-Location ..\server
Copy-Item .\config\config.example.yaml .\config\config.yaml
```

3. 编辑 `server/config/config.yaml`，至少确认以下配置：

- `db_driver: "postgres"`
- `db_dsn`: 你的 PostgreSQL / Neon 连接串
- `master_key`: 必须替换 `change-me`，用于加密服务器 SSH 密码
- `session_cookie_name`: 会话 Cookie 名称，默认 `hostdeck_session`
- `session_ttl_hours`: 会话有效期，默认 24 小时
- `bootstrap_admin_username` / `bootstrap_admin_password`: 可选，若同时配置则启动时优先创建指定管理员
- `bootstrap_admin_token`: 可选，用于启用 `POST /api/auth/bootstrap-admin` 初始化接口
- 若数据库内没有任何账号且未配置 bootstrap 管理员，系统会自动创建默认管理员 `admin / admin123`，首次登录后请立即修改密码

4. 启动后端：

```powershell
go run .\cmd\hostdeck --config .\config\config.yaml
```

5. 验证启动：

```powershell
Invoke-WebRequest http://127.0.0.1:18080/api/healthz
```

预期返回 `200 OK`，响应体包含：

```json
{"ok":true,"version":"0.1.0"}
```

默认访问地址：

- 页面：`http://127.0.0.1:18080/`
- 健康检查：`http://127.0.0.1:18080/api/healthz`

## 关键说明

- 这个项目不是前后端分离部署模式。日常运行时只需要启动 Go 后端，前端会以静态文件形式由后端直接托管。
- 只有在首次启动，或前端代码发生改动后，才需要重新执行一次 `npm run build`。
- 前端正式入口为 `/login`，未登录访问业务页面会被重定向到登录页。
- 服务器 SSH 密码只在创建或编辑时提交，后端会加密存储，列表和详情接口都不会返回明文密码。
- `viewer` 仅可查看，`admin` 与 `operator` 可执行采集、命令、规则修改等运维动作。
- 如果 `master_key` 未配置，服务器密码无法安全保存，相关操作会直接报错。
- 如需首次初始化管理员，可配置 `bootstrap_admin_username` 与 `bootstrap_admin_password`，或使用带 `X-HostDeck-Bootstrap-Token` 的 `/api/auth/bootstrap-admin` 接口。

## 相关文档

- [项目文档](./docs/项目文档.md)

## 常用验证命令

```powershell
# 前端类型检查与构建
Set-Location .\web
npx vue-tsc --noEmit
npm run build

# 后端测试
Set-Location ..\server
go test ./...
```
