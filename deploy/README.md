# 生产环境部署

本目录包含在 VPS 上运行 hostctl 所需的 Caddy 模板和 systemd unit 文件。

## 1. 准备服务器

Ubuntu 22.04 / Debian 12：

```bash
sudo apt update
sudo apt install -y caddy sqlite3 ca-certificates

sudo useradd -r -s /usr/sbin/nologin -d /var/lib/hostctl -M hostctl
sudo mkdir -p /var/www/hosted /var/lib/hostctl /var/log/hostctl /backup
sudo chown -R hostctl:hostctl /var/www/hosted /var/lib/hostctl /var/log/hostctl
```

把你的域名 A / AAAA 记录指向该 VPS。Caddy 会自动申请并续签 TLS 证书。

## 2. 构建并上传二进制

在你的开发机上：

```bash
make build-linux
scp bin/hostctl-server-linux-amd64 root@vps:/usr/local/bin/hostctl-server
scp bin/hostctl-linux-amd64 root@vps:/usr/local/bin/hostctl
ssh root@vps 'chmod +x /usr/local/bin/hostctl-server /usr/local/bin/hostctl'
```

## 3. 安装 systemd

编辑 `deploy/hostctl-server.service`，把 `https://host.example.com` 替换为真实的对外 URL。

```bash
sudo cp deploy/hostctl-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hostctl-server
sudo systemctl status hostctl-server
```

该 unit 把 API 跑在 `127.0.0.1:8787`，SQLite 存放在 `/var/lib/hostctl/hostctl.db`，静态字节从 `/var/www/hosted` 对外提供，并开启 `--require-auth`。

## 4. 安装 Caddy

编辑 `deploy/Caddyfile`，替换其中的 `host.example.com`。

```bash
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Caddy 必须把以下路由反向代理到 hostctl：

- `/api/*`
- `/admin*`
- `/agent/*`
- `/deploy/*`
- `/openapi.json`

其余路径可以由静态文件服务器处理短代码路径。

## 5. 创建首个管理员令牌

生产服务启动时已开启认证，所以需要在 VPS 上本地做一次短暂维护运行，以创建首个管理员令牌：

```bash
sudo systemctl stop hostctl-server

sudo -u hostctl /usr/local/bin/hostctl-server \
  --addr 127.0.0.1:8787 \
  --hosted-dir /var/www/hosted \
  --db /var/lib/hostctl/hostctl.db \
  --public-url https://host.example.com &

curl -s -X POST http://127.0.0.1:8787/api/token \
  -H 'Content-Type: application/json' \
  -d '{"label":"bootstrap-admin","isAdmin":true}'

sudo pkill -u hostctl hostctl-server
sudo systemctl start hostctl-server
```

请立即保存返回的 `token`。它只显示一次。然后打开 `https://host.example.com/admin`，用它登录。

之后可从管理员 UI、CLI 或技能中创建用户或管理员令牌：

```bash
hostctl --server https://host.example.com --token <admin-token> token create ci-bot
hostctl --server https://host.example.com --token <admin-token> token create ops-admin --admin
```

## 6. 验证

```bash
curl -s https://host.example.com/api/health
curl -s https://host.example.com/openapi.json | jq '.info.title'
curl -s https://host.example.com/api/admin/session \
  -H "Authorization: Bearer <admin-token>"
```

也可以运行：

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py \
  --server https://host.example.com \
  --token <admin-token> \
  doctor --require-admin
```

## 备份

需要同时备份 SQLite 数据库和托管文件：

```bash
sudo systemctl stop hostctl-server
sudo tar -czf /backup/hostctl-$(date +%F).tar.gz /var/lib/hostctl /var/www/hosted
sudo systemctl start hostctl-server
```

若要降低停机时间，可使用 SQLite 备份工具，再归档 `/var/www/hosted`。

## 监控

```bash
journalctl -u hostctl-server -f
journalctl -u caddy -f
df -h /var/www/hosted /var/lib/hostctl
```

推荐的外部检查项：

- `GET /api/health`
- `GET /openapi.json`
- 用一个专用的监控管理员令牌做管理员会话校验（如果你的安全策略允许）。
