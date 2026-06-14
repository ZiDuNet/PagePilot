# 生产环境部署

本目录包含 PagePilot / hostctl 在 VPS 上运行所需的 Caddy 与 systemd 模板。

## Docker 部署

推荐先用 Docker 验证生产配置：

```bash
docker compose up -d --build
```

部署前请在 `docker-compose.yml` 中把 `HOSTCTL_PUBLIC_BASE_URL` 改成真实对外地址，例如：

```yaml
HOSTCTL_PUBLIC_BASE_URL: "https://pagepilot.example.com"
```

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

## 3. 安装 systemd

编辑 `deploy/hostctl-server.service`，把 `https://host.example.com` 替换成真实对外 URL。

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

Caddy 需要把这些路径反向代理到 hostctl：

- `/api/*`
- `/admin*`
- `/agent/*`
- `/deploy/*`
- `/openapi.json`

其余短链路径可以由静态文件服务处理。

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
