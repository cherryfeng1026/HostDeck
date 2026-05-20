# HostDeck Docker 部署

HostDeck 的容器只运行 Go 后端。前端在镜像构建期打包成静态文件，由后端直接托管。数据库不再由默认 `compose.yaml` 创建，请使用外部 PostgreSQL / Neon。

## 准备配置

复制配置文件：

```bash
cp server/config/config.example.yaml server/config/config.yaml
```

编辑 `server/config/config.yaml`：

```yaml
http_addr: ":18080"
db_dsn: "postgresql://user:password@db.example.com:5432/hostdeck?sslmode=require"
master_key: "replace-with-a-long-random-secret"
bootstrap_admin_username: "admin"
bootstrap_admin_password: "replace-with-a-strong-password"
web_dist_dir: "/app/web/dist"
log_file: "/app/logs/hostdeck.log"
log_max_size_mb: 50
log_max_backups: 14
log_max_age_days: 14
log_compress: true
```

`master_key` 必须备份，后续更换会导致已保存的 SSH 凭据无法解密。

## 准备日志目录

Compose 会把服务器当前目录下的 `./logs/hostdeck` 挂载到容器内的 `/app/logs`，应用会按配置写入并轮转 `logs/hostdeck/hostdeck.log`：

```bash
mkdir -p logs/hostdeck
sudo chown -R 10001:10001 logs/hostdeck
```

镜像内的运行用户固定为 UID/GID `10001`。如果不调整目录属主，容器可能无法写入挂载日志。

默认日志策略是单文件 50MB 后切割，保留 14 个备份，保留 14 天，并压缩旧日志。

## 启动

```bash
docker compose up -d --build
docker compose logs -f hostdeck
curl http://127.0.0.1:18080/api/healthz
```

返回 `{"ok":true,"version":"0.1.0"}` 即启动正常。

查看文件日志：

```bash
tail -f logs/hostdeck/hostdeck.log
```

## 升级

```bash
git pull
docker compose build --pull hostdeck
docker compose up -d
```

应用启动时会自动执行数据库迁移。升级前建议先备份外部数据库和 `server/config/config.yaml`。

## HTTPS

生产环境建议用 Nginx / Caddy / Traefik 反向代理到 `127.0.0.1:18080`。启用 HTTPS 后，把配置中的 `session_cookie_secure` 改为 `true`。

## 检查项

- 不提交 `server/config/config.yaml`。
- 外部数据库允许 HostDeck 容器访问。
- 备份数据库和 `master_key`。
- 只开放必要入口端口；HostDeck 需要能出站访问被管理服务器 SSH 端口。
