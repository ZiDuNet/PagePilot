# Docker 部署

本文档说明如何使用仓库内置的 `Dockerfile` 与 `docker-compose.yml` 部署 PagePilot。

## 适用场景

- 单机部署或先在服务器上验证生产配置。
- 希望把 SQLite 数据库、托管文件和维护 SQL 固定挂载到宿主机目录。
- 已经有外层 Nginx、Caddy、宝塔或云厂商负载均衡负责 HTTPS，只需要 PagePilot 容器监听内网端口。

如果你希望直接使用 systemd + Caddy，请参考 [deploy/README.md](README.md)。

## 快速启动

PagePilot 不需要配置入口域名。浏览器访问时，首页、后台、`/agents/`、`/screens/`、Skill/MCP 文案、二维码和 `/agent/{code}/` 路径模式链接都会跟随当前打开的域名或 IP。Skill ZIP 默认使用服务端内置包，管理员也可以在后台上传自定义 ZIP 覆盖；包内默认 server 不会在下载时动态改写。

如果同一套服务通过多个入口访问，PagePilot 会按用户实际打开的入口生成链接。外层反向代理建议透传 `Host`、`X-Forwarded-Host` 和 `X-Forwarded-Proto`。

Skill、MCP 和 CLI 不读取所谓“主站域名配置”。它们使用 `--server`、`HOSTCTL_SERVER` 或客户端保存的服务器地址作为 API 控制面入口，并把这个入口交给后端用于路径模式 URL 生成。发布成功后的 URL 仍以后端响应为准。

然后启动：

```bash
docker compose up -d --build
docker compose logs -f hostctl
```

默认映射端口为 `8787:8787`。如果服务器外层反向代理使用其它端口，例如 `1143`，用户用 `https://pagepilot.example.com:1143` 打开时，页面展示、复制和下载说明会直接使用这个当前地址。

镜像构建会先运行前端 `npm ci && npm run build`，再把用户端和后台产物打进 Go 二进制。源码方式直接编译二进制时，请确认以下产物来自最新代码：

- 用户端 React：`frontend/user` 构建到 `internal/web/user/app`。
- 后台 React：`frontend/admin` 构建到 `internal/web/admin/app`。
- 内置 Skill 包：`internal/web/skill/hostctl-deploy.zip`，由 `skill/hostctl-deploy` 重新打包得到，对外主下载地址为 `/skill/pagep.zip`。
- `/admin`、`/deploy`、`/market`、`/agents/`、`/screens/` 都应由 PagePilot 服务自身返回；旧 `/api-docs.html` 仅作为兼容入口重定向到 `/admin?tab=apiDocs`。

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

本轮运行时重构新增了 `site_search_fts`、`audit_logs`、`render_cache` 和 `version_bundles` 等表，并会在服务启动时自动补齐老数据库结构、回填市场搜索索引。只要继续挂载上表中的 `./data/docker/hostctl` 和 `./data/docker/hosted`，执行 `docker compose up -d --build` 不会丢失旧版本数据；不要为了“重新构建”删除 `data/docker`。

## 注册、邮箱验证与 OSS

常用环境变量可以直接写入 `docker-compose.yml` 或 systemd unit：

```yaml
HOSTCTL_ALLOW_REGISTRATION: "true"
HOSTCTL_STORAGE_BACKEND: "local" # local 或 oss
# HOSTCTL_STORAGE_BACKEND: "oss"
# HOSTCTL_OSS_ENDPOINT: "https://oss-cn-hangzhou.aliyuncs.com"
# HOSTCTL_OSS_BUCKET: "pagepilot-assets"
# HOSTCTL_OSS_ACCESS_KEY_ID: "..."
# HOSTCTL_OSS_ACCESS_KEY_SECRET: "..."
# HOSTCTL_OSS_PREFIX: "prod/pagepilot"
```

- `HOSTCTL_ALLOW_REGISTRATION=false` 会关闭公开注册；登录页只保留登录入口，管理员仍可在后台维护用户。
- `HOSTCTL_STORAGE_BACKEND=oss` 时，发布写入、预览读取、源码下载、覆盖版本、删除版本和删除站点都会走阿里云 OSS；SQLite 仍然保存在 `/var/lib/hostctl`。
- 开启邮箱验证后，注册页会先通过图片验证码请求邮箱验证码，再用 6 位邮箱验证码完成注册；后台运行设置会展示 SMTP 是否配置完整。
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

如果你使用的是 `https://pagepilot.example.com:1143` 这类带端口地址，浏览器页面会直接使用当前打开地址生成链接。请确保反向代理传给后端的 `Host` 或 `X-Forwarded-Host` 也包含外部端口。

应用访问地址默认使用 `/agent/{code}/`。如需让应用使用 `https://{code}.example.com/` 泛域名访问，或在路径模式和泛域名之间切换，请参考 [APP_URL_MODE.md](APP_URL_MODE.md)。外部 Nginx 只需要一条泛域名 `server_name`，不需要为每个应用单独配置。

浏览器页面会把当前 `origin` 传给后端；普通 HTTP 请求会依赖 `Host`、`X-Forwarded-Host` 和 `X-Forwarded-Proto` 判断真实公网域名和协议。反向代理仍建议保留这些头，不要把内网地址写进去。

后台“Skill & MCP”页只维护下载包，不提供源码编辑。需要调整 Skill 时，请在仓库或本地修改 `skill/hostctl-deploy`，打包成 `pagep.zip` 后在后台上传。上传包保存在数据目录下，优先级高于服务端内置包；删除或缺失上传包时会回退到内置包。

## 常用命令

```bash
# 构建并启动
docker compose up -d --build

# 查看日志
docker compose logs -f hostctl

# 健康检查
curl -fsS http://127.0.0.1:8787/api/health

# 进入容器执行 CLI
docker compose exec hostctl pagep --help

# 停止
docker compose down
```

## 升级

```bash
git pull
docker compose up -d --build
docker compose logs -f hostctl
```

如果需要强制拉取基础镜像再构建，可以使用：

```bash
git pull
docker compose build --pull hostctl
docker compose up -d hostctl
docker compose logs -f hostctl
```

升级后建议检查：

```bash
curl -fsS http://127.0.0.1:8787/api/health
curl -fsS http://127.0.0.1:8787/deploy >/dev/null
curl -fsSI http://127.0.0.1:8787/api-docs.html | grep -i 'location: /admin?tab=apiDocs'
curl -fsS http://127.0.0.1:8787/screens/ >/dev/null
curl -fsS http://127.0.0.1:8787/admin >/dev/null
```

`/admin`、`/deploy`、`/market`、`/agents/` 和 `/screens/` 是内置页面，应该由 PagePilot 直接返回，不能被反向代理拦截成 404。旧 `/api-docs.html` 只保留为 302 兼容重定向，真正的 API 文档在 `/admin?tab=apiDocs`，机器可读契约在 `/openapi.json`。

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
| 首页可访问但 `/deploy` 或 `/screens/` 404 | 确认容器已重新构建并启动最新镜像；反向代理应把所有路径转发到 PagePilot。 |
| `/skill/pagep.zip` 404 | 确认镜像包含 `internal/web/skill/hostctl-deploy.zip`，并已重新构建；正常情况下没有后台上传包也会返回内置默认包。旧 `/skill/hostctl-deploy.zip` 也保留兼容。 |
| 二维码或分享链接域名错误 | 请先确认浏览器当前打开的域名正确；再检查反向代理是否传递 `Host`、`X-Forwarded-Host` 和 `X-Forwarded-Proto`，不要把内网地址透传给后端。 |
| Skill/MCP 发布后返回内网链接 | 检查 `--server` 或 `HOSTCTL_SERVER` 是否使用了内网地址。路径模式下要返回公网链接，就让 Skill/MCP 用公网入口调用 PagePilot。 |
| 登录默认管理员失败 | 如果数据库已有用户，默认管理员不会再次创建；请用已有管理员或备份恢复。 |
| 发布后静态文件丢失 | 检查 `./data/docker/hosted` 是否正确挂载且未被清空。 |
