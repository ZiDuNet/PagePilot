# Docker 部署

本文档说明如何使用仓库内置的 `Dockerfile` 和 `docker-compose.yml` 部署 PagePilot。上线前请先阅读 [运维手册](../docs/OPERATIONS.md)；应用泛域名配置请阅读 [APP_URL_MODE.md](APP_URL_MODE.md)。

## 适用场景

Docker 适合单机部署、快速验证生产配置和需要固定数据卷的环境。容器只负责运行 PagePilot，公网 TLS、DNS 和域名路由仍由外层 Caddy、Nginx、宝塔或云负载均衡负责。

## 快速启动

~~~bash
cp .env.example .env
openssl rand -base64 32
# 把生成的值写入 .env 的 HOSTCTL_MASTER_KEY，
# 同时设置自己的 HOSTCTL_ADMIN_USERNAME/HOSTCTL_ADMIN_PASSWORD
docker compose up -d --build
docker compose logs -f hostctl
~~~

默认映射 `127.0.0.1:8787:8787`，只允许本机反向代理访问。生产环境不要把容器端口直接暴露到公网；如果必须直连，请额外配置防火墙和 HTTPS。

验证：

~~~bash
curl -fsS http://127.0.0.1:8787/api/health
curl -fsS http://127.0.0.1:8787/openapi.json | jq '.info.title'
~~~

访问页面：

- `http://服务器地址:8787/`（仅适合内网验证）
- `http://服务器地址:8787/admin`（生产应通过 HTTPS 代理访问）

## 首个管理员和主密钥

- `HOSTCTL_MASTER_KEY` 用于加密访问密码、设备授权等敏感数据。已上线环境必须沿用原值，不能因为升级或重建容器而重新生成。
- 空数据库首次启动还必须设置 `HOSTCTL_ADMIN_USERNAME` 和 `HOSTCTL_ADMIN_PASSWORD`。这两个变量只在数据库没有可用管理员时创建首个管理员，不会覆盖已有账号。
- 生产环境必须启用 `REQUIRE_AUTH=true`。开发模式的内置会话不能当作公网认证。
- `.env` 不要提交到 Git；建议使用 Docker Secret、受限环境文件或密码管理器注入。

## 数据卷

Compose 默认把数据写到：

| 宿主机 | 容器 | 用途 |
| --- | --- | --- |
| `./data/docker/hostctl` | `/var/lib/hostctl` | SQLite、运行数据和后台上传的 Skill ZIP。 |
| `./data/docker/sql` | `/var/lib/hostctl/sql` | 维护和迁移辅助 SQL。 |
| `./data/docker/hosted` | `/var/www/hosted` | 已发布站点文件。 |
| `./data/docker/logs` | `/var/log/hostctl` | 日志目录。 |

不要使用 `docker compose down -v`，也不要为了重新构建删除 `data/docker`。数据库和 hosted 必须一起备份。

## 环境变量

Compose 已提供生产安全默认值。常见覆盖项：

~~~yaml
environment:
  HOSTCTL_APP_URL_MODE: "path"
  HOSTCTL_APP_DOMAIN_SUFFIX: ""
  HOSTCTL_APP_URL_SCHEME: "https"
  HOSTCTL_APP_URL_PORT: ""
  REQUIRE_AUTH: "true"
  HOSTCTL_COOLDOWN_SECONDS: "10"
  HOSTCTL_ALLOW_REGISTRATION: "true"
  HOSTCTL_STORAGE_BACKEND: "local"
  HOSTCTL_EMAIL_VERIFICATION_ENABLED: "false"
~~~

完整变量、默认值、旧变量兼容和运行设置见 [配置参考](../docs/CONFIGURATION.md)。不要同时设置 `HOSTCTL_*` 和没有前缀的旧变量；旧变量可能覆盖新变量。

## 泛域名模式前置条件

把 `HOSTCTL_APP_URL_MODE` 改为 `domain` 或 `dual` 之前，必须完成：

1. `*.pg.example.com` DNS A/AAAA/CNAME 指向同一反向代理。
2. TLS 证书覆盖主站 `pagepilot.example.com` 和 `*.pg.example.com`。
3. Nginx/Caddy 同时接收主站和泛域名，并将整个站点转发到容器 `127.0.0.1:8787`。
4. Nginx 转发 `Host`、`X-Forwarded-Host`、`X-Forwarded-Proto`、`X-Forwarded-For`，并为 `/api/device/ws` 保留 WebSocket Upgrade。

只配置容器环境变量不会自动创建泛解析或证书。`HOSTCTL_APP_DOMAIN_SUFFIX` 只填写 `pg.example.com`，不要填写 `*.pg.example.com`。完整配置见 [APP_URL_MODE.md](APP_URL_MODE.md)。

## 反向代理

Caddy：

~~~caddyfile
pagepilot.example.com, *.pg.example.com {
    reverse_proxy 127.0.0.1:8787
}
~~~

Nginx：

~~~nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}

server {
    listen 443 ssl;
    server_name pagepilot.example.com *.pg.example.com;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Forwarded-Host $http_host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
~~~

使用 `https://host:1143` 等非标准端口时，让 `Host`/`X-Forwarded-Host` 保留端口，并设置 `HOSTCTL_APP_URL_PORT=1143`；`X-Forwarded-Port` 只能作为补充。不要使用 `$host` 生成带端口的链接。代理应使用统一 `location /`，不要只放行 `/api` 或 `/agent`。

## 构建产物

Docker builder 会在编译 Go 二进制前：

1. 安装前端依赖并构建 `frontend/user`、`frontend/admin`。
2. 运行 `python scripts/build_skill_zip.py`。
3. 将前端和 Skill ZIP embed 到服务端。

源码方式构建时也要刷新这些产物：

~~~bash
(cd frontend/user && npm install && npm run build)
(cd frontend/admin && npm install && npm run build)
python scripts/build_skill_zip.py
go build -o bin/hostctl-server ./cmd/hostctl-server
~~~

内置 Skill 对外地址为 `/skill/pagep.zip`；旧 `/skill/hostctl-deploy.zip` 兼容保留。后台上传的包优先于内置包。

## 常用命令

~~~bash
docker compose up -d --build
docker compose ps
docker compose logs -f hostctl
docker compose exec hostctl pagep --help
curl -fsS http://127.0.0.1:8787/api/health
docker compose stop hostctl
docker compose start hostctl
docker compose down
~~~

## 升级

~~~bash
# 先备份 data/docker，再更新代码
git pull
docker compose up -d --build
docker compose logs -f hostctl
curl -fsS http://127.0.0.1:8787/api/health
~~~

升级不会自动清空数据库或 hosted。建议先执行：

~~~bash
node scripts/legacy-upgrade-qa.mjs
node scripts/docker-upgrade-qa.mjs
~~~

第二个脚本需要 Docker Compose 和 Go；它使用临时目录，不应指向生产数据。升级后检查首页、部署页、市场、后台、当前/历史应用、Skill ZIP 和 WebSocket。

## 备份和恢复

备份：

~~~bash
mkdir -p backup
docker compose stop hostctl
tar -czf backup/pagepilot-$(date +%F).tar.gz data/docker
docker compose start hostctl
~~~

恢复：

~~~bash
docker compose down
tar -xzf backup/pagepilot-YYYY-MM-DD.tar.gz
docker compose up -d
~~~

恢复后检查宿主机目录属主、SQLite 文件权限、`hosted` 软链接和 `/api/health`。不要只恢复数据库或只恢复 hosted。

## 安全和排障

- Token 明文只返回一次；用密码管理器或 CI Secret 保存。
- 访问密码只授权浏览，源码下载和模板复用仍由站点策略控制。
- 用户上传 HTML/JS 会运行在托管应用上下文；生产建议使用泛域名模式隔离，并配置合理 CSP/iframe 策略。
- CORS 只控制 API fetch/XHR，不控制 iframe；两者分别在运行设置中配置。
- 不要在日志、工单或提交中暴露 Token、主密钥、管理员密码、数据库和用户源码。

| 现象 | 检查 |
| --- | --- |
| `/deploy`、`/market` 或 `/screens/` 404 | 代理是否统一转发整个站点；容器是否已重建。 |
| 应用链接指向内网 | `Host`/`X-Forwarded-Host` 和 CLI/Skill/MCP 的 server 是否使用公网入口。 |
| HTTPS 页面出现 Mixed Content | scheme、外部端口、证书和反代头是否一致。 |
| 泛域名打不开 | `dig`、证书 SAN、`server_name` 和 `curl --resolve`。 |
| `/api/device/ws` 失败 | Nginx 是否启用 HTTP/1.1、Upgrade 和 Connection。 |
| Skill ZIP 404 | 重打包并重建服务端，检查 `/skill/pagep.zip`。 |
