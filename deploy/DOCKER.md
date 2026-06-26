# Docker 部署

本文档说明如何使用仓库内置的 `Dockerfile` 与 `docker-compose.yml` 部署 PagePilot。

## 适用场景

- 单机部署或先在服务器上验证生产配置。
- 希望把 SQLite 数据库、托管文件和维护 SQL 固定挂载到宿主机目录。
- 已经有外层 Nginx、Caddy、宝塔或云厂商负载均衡负责 HTTPS，只需要 PagePilot 容器监听内网端口。

如果你希望直接使用 systemd + Caddy，请参考 [deploy/README.md](README.md)。

## 快速启动

浏览器访问时，首页、后台、`/agents/`、`/screens/`、Skill/MCP 文案、下载包默认 server、二维码和 `/agent/{code}/` 路径模式链接都会跟随当前打开域名。`HOSTCTL_PUBLIC_BASE_URL` 只作为无浏览器请求上下文时的兜底地址，建议仍填一个可访问的默认地址：

```yaml
HOSTCTL_PUBLIC_BASE_URL: "https://pagepilot.example.com"
```

如果希望同一套服务能通过多个主站域名访问，默认即可按当前访问域名生成。也可以在后台“运行设置”确认“主站链接来源”为“按当前访问域名生成”，或通过环境变量设置：

```yaml
HOSTCTL_PUBLIC_URL_MODE: "request_host"
```

这种模式会让浏览器页面和新版本 Skill 请求优先使用当前访问域名；没有浏览器上下文时再使用反向代理请求头或 `HOSTCTL_PUBLIC_BASE_URL` 兜底。应用泛域名配置仍然独立，不受这个开关影响。

然后启动：

```bash
docker compose up -d --build
docker compose logs -f hostctl
```

默认映射端口为 `8787:8787`。如果服务器外层反向代理使用其它端口，例如 `1143`，请把 `HOSTCTL_PUBLIC_BASE_URL` 写成用户真实访问的完整地址：

```yaml
HOSTCTL_PUBLIC_BASE_URL: "https://pagepilot.example.com:1143"
```

镜像构建会把内置用户端和后台产物打进 Go 二进制。源码部署时请确认以下产物来自最新代码：

- 用户端 React：`frontend/user` 构建到 `internal/web/user/app`。
- 后台 React：`frontend/admin` 构建到 `internal/web/admin/app`。
- `/admin`、`/deploy.html`、`/api-docs.html`、`/agents/`、`/screens/` 都应由 PagePilot 服务自身返回。

## 首次管理员

空数据库首次启动时，容器会自动创建默认管理员：

```yaml
HOSTCTL_ADMIN_USERNAME: "admin"
HOSTCTL_ADMIN_PASSWORD: "123456"
```

打开 `/admin` 登录后，请立即在后台“账号设置”修改密码。已有用户时，这两个变量不会覆盖现有账号。

生产环境建议把默认密码改成一次性强密码，或在首次登录后从 compose 中移除默认密码。

## 数据卷

`docker-compose.yml` 默认使用宿主机 bind mount：

| 宿主机路径 | 容器路径 | 用途 |
|---|---|---|
| `./data/docker/hostctl` | `/var/lib/hostctl` | SQLite 数据库与运行数据 |
| `./data/docker/sql` | `/var/lib/hostctl/sql` | 人工维护、备份或迁移用 SQL |
| `./data/docker/hosted` | `/var/www/hosted` | 已发布的静态站点文件 |
| `./data/docker/logs` | `/var/log/hostctl` | 服务日志目录 |

升级容器前请保留这些目录。删除这些目录会删除数据库和已发布站点。

## 反向代理

PagePilot 容器内监听 `0.0.0.0:8787`。外层反向代理只需要把整个站点转发到容器端口，不需要维护路径白名单。

Caddy 示例：

```caddyfile
pagepilot.example.com {
    reverse_proxy 127.0.0.1:8787
}
```

Nginx 示例：

```nginx
server {
    listen 443 ssl http2;
    server_name pagepilot.example.com;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

如果你使用的是 `https://pagepilot.example.com:1143` 这类带端口地址，浏览器页面会直接使用当前打开地址生成链接；但 `HOSTCTL_PUBLIC_BASE_URL` 作为兜底地址时也建议带相同端口，避免旧客户端或后台任务生成错误地址。

应用访问地址默认使用 `/agent/{code}/`。如需让应用使用 `https://{code}.example.com/` 泛域名访问，或在路径模式和泛域名之间切换，请参考 [APP_URL_MODE.md](APP_URL_MODE.md)。外部 Nginx 只需要一条泛域名 `server_name`，不需要为每个应用单独配置。

浏览器页面会把当前 `origin` 传给后端；没有浏览器上下文时，后端会依赖 `Host`、`X-Forwarded-Host` 和 `X-Forwarded-Proto` 判断真实公网域名和协议。反向代理仍建议保留这些头，不要把内网地址写进去。

## 常用命令

```bash
# 构建并启动
docker compose up -d --build

# 查看日志
docker compose logs -f hostctl

# 健康检查
curl -fsS http://127.0.0.1:8787/api/health

# 进入容器执行 CLI
docker compose exec hostctl hostctl --help

# 停止
docker compose down
```

## 升级

```bash
git pull
docker compose build --pull hostctl
docker compose up -d hostctl
docker compose logs -f hostctl
```

升级后建议检查：

```bash
curl -fsS http://127.0.0.1:8787/api/health
curl -fsS http://127.0.0.1:8787/deploy.html >/dev/null
curl -fsS http://127.0.0.1:8787/api-docs.html >/dev/null
curl -fsS http://127.0.0.1:8787/screens/ >/dev/null
curl -fsS http://127.0.0.1:8787/admin >/dev/null
```

`/admin`、`/deploy.html`、`/api-docs.html`、`/agents/` 和 `/screens/` 是内置页面，应该由 PagePilot 直接返回，不能被反向代理拦截成 404。

## 备份与恢复

备份：

```bash
mkdir -p backup
docker compose stop hostctl
tar -czf backup/pagepilot-$(date +%F).tar.gz data/docker
docker compose start hostctl
```

恢复：

```bash
docker compose down
tar -xzf backup/pagepilot-YYYY-MM-DD.tar.gz
docker compose up -d
```

如果数据量较大，可以使用 SQLite `.backup` 命令导出数据库，同时归档 `./data/docker/hosted`。

## 安全注意

- 生产环境保持 `REQUIRE_AUTH=true`。
- 首次登录后立即修改默认管理员密码。
- Token 明文只返回一次，请使用密码管理器或 CI Secret 保存。
- 访问密码仅保护前台查看入口。匿名用户也可以输入访问密码查看加密站点；输入正确后浏览器获得 5 分钟访问票据，改密码后旧票据立即失效。
- 用户上传的 HTML/JS 会以托管应用形式运行。路径模式默认加 CSP sandbox，建议生产环境使用泛域名模式隔离用户上传脚本，详见 [APP_URL_MODE.md](APP_URL_MODE.md)。
- CORS 只控制外部网页跨域调用 API，不控制 iframe 嵌入。是否允许其它网站嵌入应用 URL，请在后台“运行设置 -> 跨域与嵌入”配置 iframe 嵌入策略；可选任意、仅本站、白名单或禁止嵌入。白名单来源必须写完整 `http(s)://域名[:端口]`，不要带路径。
- 不要把 `./data/docker/hostctl/hostctl.db`、`./data/docker/hosted` 或备份包提交到 Git。

## 排障

| 现象 | 检查项 |
|---|---|
| 首页可访问但 `/deploy.html` 或 `/screens/` 404 | 确认容器已重新构建并启动最新镜像；反向代理应把所有路径转发到 PagePilot。 |
| 二维码或分享链接域名错误 | 新版浏览器页面会把当前 `location.origin` 传给后端；请先确认访问页面本身的域名正确。若来自旧客户端或后台任务，再检查 `HOSTCTL_PUBLIC_BASE_URL`，以及反向代理是否传递 `Host`、`X-Forwarded-Host` 和 `X-Forwarded-Proto`。 |
| 登录默认管理员失败 | 如果数据库已有用户，默认管理员不会再次创建；请用已有管理员或备份恢复。 |
| 发布后静态文件丢失 | 检查 `./data/docker/hosted` 是否正确挂载且未被清空。 |
