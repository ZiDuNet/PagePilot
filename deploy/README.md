# 生产环境部署

本目录包含 PagePilot / hostctl 在生产环境运行所需的 Docker、Caddy 与 systemd 模板。

## Docker 部署

推荐先用 Docker 验证生产配置，或直接用于单机部署：

```bash
docker compose up -d --build
```

完整 Docker 部署、升级、备份、反向代理和排障说明请见 [deploy/DOCKER.md](DOCKER.md)。
应用访问地址默认使用 `/agent/{code}/`，泛域名或双模式配置请参考 [APP_URL_MODE.md](APP_URL_MODE.md)。

主站不需要配置域名。浏览器页面会按当前打开域名生成首页、后台、Skill/MCP、二维码和路径模式应用链接；反向代理只需要透传 `Host`、`X-Forwarded-Host` 和 `X-Forwarded-Proto`。

Skill、MCP 和 CLI 通过 `--server`、`HOSTCTL_SERVER` 或客户端保存的服务器地址连接 PagePilot。这个地址只表示本次 API 控制面入口；路径模式下发布成功返回的应用链接会按该入口生成，泛域名模式下应用链接按后台配置的应用域名生成。

首次启动会在空数据库中自动创建默认管理员：

```yaml
HOSTCTL_ADMIN_USERNAME: "admin"
HOSTCTL_ADMIN_PASSWORD: "123456"
```

打开 `https://pagepilot.example.com/admin`，使用 `admin / 123456` 登录。首次登录后请进入“账号设置”立即修改密码。这个默认账号只会在数据库里没有任何用户时创建；如果已经有用户，不会覆盖现有账号。

Docker 默认把这些目录挂载到宿主机的 `./data/docker/` 下：

| 宿主机路径 | 容器路径 | 用途 |
|---|---|---|
| `./data/docker/hostctl` | `/var/lib/hostctl` | SQLite 数据库与运行数据 |
| `./data/docker/sql` | `/var/lib/hostctl/sql` | 人工维护、备份或迁移用的 SQL 文件 |
| `./data/docker/hosted` | `/var/www/hosted` | 已发布的静态站点文件 |
| `./data/docker/logs` | `/var/log/hostctl` | 服务日志目录 |

如果外层使用 Nginx、Caddy、宝塔或云厂商负载均衡，只需要把整个站点反向代理到容器端口。`/deploy.html`、`/api-docs.html`、`/agents/`、`/screens/`、`/api/*`、`/agent/*` 和应用访问地址都由 PagePilot 自己处理，不要在反向代理里维护路径白名单。

`CORS_ALLOW_ORIGINS` 默认留空，也就是默认关闭浏览器跨域访问。只有在另一个网页前端必须跨域调用 PagePilot API 时，才填写明确的 origin 白名单；不要配置成 `*`。

内置前端说明：

- 用户端 React 工程位于 `frontend/user`，构建产物为 `internal/web/user/app`。
- 后台 React 工程位于 `frontend/admin`，构建产物为 `internal/web/admin/app`。
- 内置 Skill 下载包位于 `internal/web/skill/hostctl-deploy.zip`，用于保证新部署时 `/skill/pagep.zip` 可直接下载；旧 `/skill/hostctl-deploy.zip` 保留兼容。
- Go 服务通过 `embed` 打包这些产物，所以源码方式发布二进制前需要先构建前端，并确认内置 Skill ZIP 是最新的。

## 1. 准备服务器

Ubuntu 22.04 / Debian 12:

```bash
sudo apt update
sudo apt install -y caddy sqlite3 ca-certificates

sudo useradd -r -s /usr/sbin/nologin -d /var/lib/hostctl -M hostctl
sudo mkdir -p /var/www/hosted /var/lib/hostctl /var/log/hostctl /backup
sudo chown -R hostctl:hostctl /var/www/hosted /var/lib/hostctl /var/log/hostctl
```

把域名 A / AAAA 记录指向这台 VPS，Caddy 会自动申请并续签 TLS 证书。

## 2. 构建并上传二进制

在开发机上执行：

```bash
make build-linux
scp bin/hostctl-server-linux-amd64 root@vps:/usr/local/bin/hostctl-server
scp bin/hostctl-linux-amd64 root@vps:/usr/local/bin/hostctl
ssh root@vps 'chmod +x /usr/local/bin/hostctl-server /usr/local/bin/hostctl'
```

如果不使用 `make`，可以手动构建前端和服务端：

```bash
(cd frontend/user && npm install && npm run build)
(cd frontend/admin && npm install && npm run build)
# 如果改过 skill/hostctl-deploy，请重新生成 internal/web/skill/hostctl-deploy.zip
go build -o bin/hostctl-server ./cmd/hostctl-server
go build -o bin/hostctl ./cmd/hostctl
```

## 3. 安装 systemd

检查 `deploy/hostctl-server.service` 里的监听地址、数据目录和运行用户是否符合你的服务器环境。

```bash
sudo cp deploy/hostctl-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hostctl-server
sudo systemctl status hostctl-server
```

该 unit 默认把 API 跑在 `127.0.0.1:8787`，SQLite 存放在 `/var/lib/hostctl/hostctl.db`，静态站点存放在 `/var/www/hosted`，并开启 `--require-auth`。

## 4. 安装 Caddy

编辑 `deploy/Caddyfile`，替换其中的 `host.example.com`。

```bash
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Caddy 直接把整个站点反向代理到 hostctl 即可。hostctl 自己处理首页、后台、API、Skill 下载、应用访问与应用访问路由，不需要在 Caddy 里维护路径白名单。

后台“Skill & MCP”页只维护 `pagep.zip` 下载包，不再直接编辑 Skill 源文件。需要调整 Skill 时，请在仓库或本地修改并打包，再上传 ZIP 覆盖内置包。

## 5. 首次登录

默认管理员账号：

- 用户名：`admin`
- 密码：`123456`

首次登录后请进入后台“账号设置”修改密码。

systemd 部署如果要使用同样的默认账号，请在 unit 中添加：

```ini
Environment=HOSTCTL_ADMIN_USERNAME=admin
Environment=HOSTCTL_ADMIN_PASSWORD=123456
```

这两个变量只会在数据库里没有任何用户时创建首个管理员，不会覆盖已有账号。

## 6. 验证

```bash
curl -s https://host.example.com/api/health
curl -s https://host.example.com/openapi.json | jq '.info.title'
curl -fsS https://host.example.com/deploy.html >/dev/null
curl -fsS https://host.example.com/api-docs.html >/dev/null
curl -fsS https://host.example.com/screens/ >/dev/null
curl -fsS https://host.example.com/admin >/dev/null
curl -fsS https://host.example.com/skill/pagep.zip >/dev/null
```

登录后台后，也可以在 Agent 技能里执行：

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py \
  --server https://host.example.com \
  doctor
```

## 备份

需要同时备份 SQLite 数据库和托管文件：

```bash
sudo systemctl stop hostctl-server
sudo tar -czf /backup/hostctl-$(date +%F).tar.gz /var/lib/hostctl /var/www/hosted
sudo systemctl start hostctl-server
```

如果要降低停机时间，可以使用 SQLite 备份工具导出数据库，再归档 `/var/www/hosted`。

## 监控

```bash
journalctl -u hostctl-server -f
journalctl -u caddy -f
df -h /var/www/hosted /var/lib/hostctl
```

推荐外部检查项：

- `GET /api/health`
- `GET /openapi.json`
- `GET /deploy.html`
- `GET /api-docs.html`
- `GET /screens/`
