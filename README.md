# HostDeck

HostDeck 是一个轻量级 Linux 运维面板。Go 后端单进程负责 API、静态页面托管、SSH 采集、远程命令、告警与清理任务；前端使用 Vue 3 展示控制台界面。

## 功能

- 登录、登出、会话 Cookie、管理员初始化
- 角色权限：`admin`、`operator`、`viewer`
- 服务器资产管理、SSH 连通性测试、主机指纹信任
- 总览仪表盘、资源趋势、服务实例状态、最近活动
- 远程命令执行、批量执行、命令模板、命令历史筛选
- 告警规则、当前告警、告警历史、Webhook 通知
- 用户管理、API Token、通知列表、全局搜索
- 审计事件、认证事件、历史数据定期清理
- PostgreSQL / Neon 持久化，SSH 凭据加密存储

## 技术栈

- Backend: Go 1.25, chi, database/sql
- Frontend: Vue 3, Vite, TypeScript, Naive UI, ECharts
- Database: PostgreSQL / Neon
- Config: YAML，环境变量可覆盖部分配置

## 目录

```text
HostDeck/
  server/              Go 后端、配置示例、数据库迁移
  web/                 Vue 前端
  compose.yaml         可选容器启动
  Dockerfile           可选镜像构建
```

## 本地启动

准备：

- Go 1.25+
- Node.js 20+ 和 npm
- 一个可访问的 PostgreSQL / Neon 数据库

首次启动：

```powershell
cd web
npm install
npm run build

cd ..\server
Copy-Item .\config\config.example.yaml .\config\config.yaml
```

编辑 `server/config/config.yaml`，至少改这几项：

```yaml
db_dsn: "postgresql://user:password@localhost:5432/hostdeck?sslmode=require"
master_key: "replace-with-a-long-random-secret"
bootstrap_admin_username: "admin"
bootstrap_admin_password: "replace-with-a-strong-password"
web_dist_dir: "../../web/dist"
```

启动：

```powershell
go run .\cmd\hostdeck --config .\config\config.yaml
```

访问：

- 页面：http://127.0.0.1:18080/
- 健康检查：http://127.0.0.1:18080/api/healthz

后续如果只改后端，直接重新运行 `go run`。如果改了前端，先在 `web/` 下重新执行 `npm run build`。

## 配置说明

配置文件在 `server/config/config.yaml`，不要提交真实配置。

常用项：

- `http_addr`: 监听地址，默认 `:18080`
- `db_dsn`: PostgreSQL / Neon DSN，必填
- `master_key`: SSH 凭据加密密钥，生产环境必须长期备份
- `bootstrap_admin_username` / `bootstrap_admin_password`: 首次启动时创建管理员
- `bootstrap_admin_token`: 使用初始化接口创建管理员时的令牌
- `session_cookie_secure`: HTTPS 下建议设为 `true`
- `log_file`: 可选文件日志路径，Docker 部署建议 `/app/logs/hostdeck.log`
- `log_max_size_mb` / `log_max_backups` / `log_max_age_days`: 文件日志轮转与清理策略
- `poll_interval_seconds`: SSH 采集周期
- `poll_concurrency`: 并发采集数
- `*_retention_hours`: 历史数据保留时间
- `alert_webhook_url`: 可选告警 Webhook 初始值

## 可选 Docker

默认 Compose 只启动 HostDeck 应用，不内置数据库。先准备外部 PostgreSQL / Neon，并把 `server/config/config.yaml` 中的 `web_dist_dir` 改为：

```yaml
web_dist_dir: "/app/web/dist"
log_file: "/app/logs/hostdeck.log"
```

然后运行：

```powershell
mkdir logs\hostdeck
docker compose up -d --build
```

容器会读取挂载的 `./server/config/config.yaml`，并把应用日志写入 `./logs/hostdeck/hostdeck.log`；日志由 Go 程序按大小、数量和保留天数自动轮转。Linux 服务器上建议先执行 `sudo chown -R 10001:10001 logs/hostdeck`。

默认 Compose 只把 `18080` 绑定到本机 `127.0.0.1`。生产环境建议使用独立的公共反向代理统一占用 `80/443`，再按域名转发到 HostDeck 容器，避免多个项目各自绑定公网端口。

## 验证

```powershell
# 后端测试
cd server
go test ./...

# 前端构建
cd ..\web
npm run build

# Compose 检查
cd ..
docker compose config
```

## 注意事项

- `master_key` 丢失后，已保存的 SSH 密码无法解密。
- 生产环境建议使用 HTTPS，并设置 `session_cookie_secure: true`。
- HostDeck 需要能访问被管理服务器的 SSH 端口。
- 首次 SSH 测试会返回主机指纹，确认信任后后续连接会校验指纹。
- `viewer` 只能查看；`admin` 和 `operator` 可执行运维动作。
