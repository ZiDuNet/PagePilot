# 生产环境部署

本目录提供 PagePilot 的 Docker、systemd 和 Caddy 模板。建议先阅读：

- [../docs/OPERATIONS.md](../docs/OPERATIONS.md)：上线检查、备份、升级、监控和排障。
- [APP_URL_MODE.md](APP_URL_MODE.md)：路径、泛域名、双模式和历史版本链接。
- [DOCKER.md](DOCKER.md)：Docker Compose 的完整步骤。
- [../docs/CONFIGURATION.md](../docs/CONFIGURATION.md)：环境变量和运行设置。

## 选择部署方式

| 方式 | 适合 | 入口 |
| --- | --- | --- |
| Docker Compose | 单机、快速上线、已有容器运维 | [DOCKER.md](DOCKER.md) |
| systemd + Caddy | Linux VPS、希望由系统服务管理 | 本页 |
| systemd + Nginx | 已有 Nginx/宝塔/统一网关 | 本页 + [APP_URL_MODE.md](APP_URL_MODE.md) |

无论选择哪种方式，SQLite 数据库和 hosted 文件都必须持久化并一起备份。PagePilot 只监听内网，公网 TLS 和域名由反向代理负责。

## 1. 准备服务器

以下以 Ubuntu 22.04 / Debian 12 为例：

~~~bash
sudo apt update
sudo apt install -y caddy sqlite3 ca-certificates
sudo useradd -r -s /usr/sbin/nologin -d /var/lib/hostctl -M hostctl
sudo mkdir -p /var/www/hosted /var/lib/hostctl /var/log/hostctl /backup
sudo chown -R hostctl:hostctl /var/www/hosted /var/lib/hostctl /var/log/hostctl
~~~

主站只需要一个 A/AAAA 记录。启用泛域名模式时，还必须添加 `*.pg.example.com` 并申请覆盖主站和泛域名的 TLS 证书，详见 [APP_URL_MODE.md](APP_URL_MODE.md)。

## 2. 构建并上传二进制

在开发机执行：

~~~bash
make build-linux
scp bin/hostctl-server-linux-amd64 root@server:/usr/local/bin/hostctl-server
scp bin/pagep-linux-amd64 root@server:/usr/local/bin/pagep
ssh root@server 'chmod +x /usr/local/bin/hostctl-server /usr/local/bin/pagep'
~~~

`make build-linux` 会先构建两个前端并刷新内置 Skill ZIP。手动构建时必须保持相同顺序：

~~~bash
(cd frontend/user && npm install && npm run build)
(cd frontend/admin && npm install && npm run build)
python scripts/build_skill_zip.py
go build -o bin/hostctl-server ./cmd/hostctl-server
go build -o bin/pagep ./cmd/hostctl
~~~

## 3. 配置环境

不要把密码写入 unit 文件。建议创建只允许 root 读取的环境文件：

~~~bash
sudo install -d -m 750 /etc/hostctl
sudo install -m 600 /dev/null /etc/hostctl/hostctl.env
sudoedit /etc/hostctl/hostctl.env
~~~

最小生产配置：

~~~ini
HOSTCTL_MASTER_KEY=<固定的随机主密钥>
HOSTCTL_ADMIN_USERNAME=<首个管理员用户名>
HOSTCTL_ADMIN_PASSWORD=<首个管理员密码>
REQUIRE_AUTH=true

# 路径模式（不需要泛域名）
HOSTCTL_APP_URL_MODE=path
HOSTCTL_APP_URL_SCHEME=https

# 或者泛域名模式（先完成 DNS/证书/反代）
# HOSTCTL_APP_URL_MODE=domain
# HOSTCTL_APP_DOMAIN_SUFFIX=pg.example.com
# HOSTCTL_APP_URL_SCHEME=https
# HOSTCTL_APP_URL_PORT=
~~~

`HOSTCTL_MASTER_KEY` 在升级和重启时必须保持不变；管理员用户名和密码只在数据库没有可用管理员时创建首个账号，不会覆盖已有用户。完整配置见 [../docs/CONFIGURATION.md](../docs/CONFIGURATION.md)。

## 4. 安装 systemd

先检查 [hostctl-server.service](hostctl-server.service) 的用户、目录和二进制路径，再安装：

~~~bash
sudo cp deploy/hostctl-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hostctl-server
sudo systemctl status hostctl-server
~~~

unit 默认：

- 监听 `127.0.0.1:8787`；
- 数据库为 `/var/lib/hostctl/hostctl.db`；
- 静态文件为 `/var/www/hosted`；
- 使用 `--require-auth`；
- 通过 systemd sandbox 限制权限。

模板会自动读取可选的 `/etc/hostctl/hostctl.env`；文件不存在时服务仍可启动，但生产环境必须创建并填写主密钥。修改环境文件后执行 `sudo systemctl daemon-reload && sudo systemctl restart hostctl-server`。

## 5. 配置 Caddy

### 路径模式

编辑 [Caddyfile](Caddyfile)，把 `host.example.com` 改为主站：

~~~caddyfile
pagepilot.example.com {
    encode gzip
    reverse_proxy 127.0.0.1:8787
}
~~~

安装并验证：

~~~bash
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
~~~

### 泛域名/双模式

同一个 Caddy site 接收主站和泛域名：

~~~caddyfile
pagepilot.example.com, *.pg.example.com {
    encode gzip
    reverse_proxy 127.0.0.1:8787
}
~~~

Caddy 自动签发泛域名证书通常需要 DNS provider 和 DNS-01；请确认最终证书包含 `pagepilot.example.com` 与 `*.pg.example.com`。Caddy 不需要手动配置 WebSocket Upgrade。

## 6. 使用 Nginx 或宝塔

外层 Nginx/宝塔必须把整个站点转发到 `127.0.0.1:8787`，并保留外部 Host、协议和端口。泛域名还要让 `server_name` 同时包含主站和 `*.pg.example.com`，并转发 `/api/device/ws` 的 WebSocket。

完整 Nginx 配置（含非标准端口和 WebSocket）见 [APP_URL_MODE.md](APP_URL_MODE.md)。不要只代理 `/api` 或 `/agent`，否则后台、市场、Skill、屏幕和历史版本会出现 404。

## 7. 首次登录和验证

首次启动后使用环境文件里的管理员凭据登录 `/admin`，然后进入“账号设置”修改密码。验证：

~~~bash
curl -fsS https://pagepilot.example.com/api/health
curl -fsS https://pagepilot.example.com/openapi.json | jq '.info.title'
curl -fsS https://pagepilot.example.com/deploy >/dev/null
curl -fsS https://pagepilot.example.com/market >/dev/null
curl -fsS https://pagepilot.example.com/admin >/dev/null
curl -fsS https://pagepilot.example.com/skill/pagep.zip >/dev/null
~~~

泛域名模式再验证一个真实站点和历史版本：

~~~bash
curl -fsS https://demo.pg.example.com/ >/dev/null
curl -fsS https://demo.pg.example.com/versions/2/ >/dev/null
~~~

## 8. 注册、邮箱和 OSS

按需在环境文件中配置：

~~~ini
HOSTCTL_ALLOW_REGISTRATION=true
HOSTCTL_EMAIL_VERIFICATION_ENABLED=false
HOSTCTL_STORAGE_BACKEND=local
~~~

启用邮箱验证时配置 SMTP host、from、端口和安全模式；启用 OSS 时配置 endpoint、bucket、access key、secret 和 prefix。切换存储不会自动迁移历史文件，每个版本按数据库中的存储归属读取。完整说明见 [../docs/CONFIGURATION.md](../docs/CONFIGURATION.md)。

## 9. 备份、监控和升级

备份：

~~~bash
sudo systemctl stop hostctl-server
sudo tar -czf /backup/pagepilot-$(date +%F).tar.gz /var/lib/hostctl /var/www/hosted
sudo systemctl start hostctl-server
~~~

监控：

~~~bash
journalctl -u hostctl-server -f
journalctl -u caddy -f
df -h /var/www/hosted /var/lib/hostctl
~~~

升级前备份并保留旧二进制，完成后运行：

~~~bash
sudo systemctl restart hostctl-server
curl -fsS https://pagepilot.example.com/api/health
node scripts/legacy-upgrade-qa.mjs
~~~

`node scripts/docker-upgrade-qa.mjs` 用于真实容器升级演练，需要 Docker Compose 和 Go；它必须使用临时目录。更多升级和恢复步骤见 [../docs/OPERATIONS.md](../docs/OPERATIONS.md)。

## 安全边界

- 不要暴露 `8787` 到公网，生产流量走 HTTPS 反向代理。
- 不要轮换已上线环境的 `HOSTCTL_MASTER_KEY`。
- 不要提交 `/etc/hostctl/hostctl.env`、SQLite、hosted 文件、Token 或备份包。
- CORS 与 iframe 嵌入是两套独立策略，按 [配置参考](../docs/CONFIGURATION.md) 设置。
- 用户上传的 HTML/JS 会运行在托管应用上下文；需要源隔离时启用泛域名模式并保留合理 CSP。
