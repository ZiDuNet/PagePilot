# 运维手册

本页覆盖生产上线、反向代理、备份、升级、监控和常见故障。Docker 细节见 [../deploy/DOCKER.md](../deploy/DOCKER.md)，systemd + Caddy 模板见 [../deploy/README.md](../deploy/README.md)。

## 上线前检查

- 固定保存一个随机生成的 `HOSTCTL_MASTER_KEY`；已有环境升级时绝不能更换。
- 生产启用 `REQUIRE_AUTH=true`，并为全新空库提供首个管理员凭据。
- 服务只监听内网地址，公网入口由 Caddy、Nginx、宝塔或负载均衡提供 HTTPS。
- 持久化 SQLite 和 hosted 文件，确认磁盘有足够空间和备份。
- 若启用域名/双模式，先完成 wildcard DNS、TLS 证书和代理，见 [APP_URL_MODE.md](../deploy/APP_URL_MODE.md)。
- 若启用 OSS，先用测试 Bucket 验证发布、预览、下载、覆盖、删除和恢复流程。
- 若启用邮箱验证，先验证 SMTP 发送和验证码过期行为。
- 发布前运行 Go、前端、Skill 和运行时 QA；真实 Docker 升级要在目标环境演练。

## Docker

~~~bash
cp .env.example .env
# 编辑 .env：HOSTCTL_MASTER_KEY、HOSTCTL_ADMIN_USERNAME、HOSTCTL_ADMIN_PASSWORD
docker compose up -d --build
docker compose logs -f hostctl
curl -fsS http://127.0.0.1:8787/api/health
~~~

默认数据卷：

| 宿主机 | 容器 | 内容 |
| --- | --- | --- |
| `./data/docker/hostctl` | `/var/lib/hostctl` | SQLite、运行数据和后台上传的 Skill ZIP。 |
| `./data/docker/hosted` | `/var/www/hosted` | 已发布站点文件。 |
| `./data/docker/sql` | `/var/lib/hostctl/sql` | 维护 SQL 和迁移辅助文件。 |
| `./data/docker/logs` | `/var/log/hostctl` | 日志目录。 |

不要用 `docker compose down -v` 清理生产卷，也不要为“重新构建”删除 `data/docker`。

## systemd + Caddy

仓库提供 `deploy/hostctl-server.service` 和 `deploy/Caddyfile`。基本流程：

~~~bash
make build-linux
scp bin/hostctl-server-linux-amd64 root@server:/usr/local/bin/hostctl-server
sudo cp deploy/hostctl-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now hostctl-server
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
~~~

systemd unit 默认使用 `127.0.0.1:8787`、`/var/lib/hostctl/hostctl.db` 和 `/var/www/hosted`；按服务器实际目录检查 unit 和环境文件后再启动。

## 反向代理

路径模式只需要一个主站；域名/双模式需要主站和泛域名共同指向同一代理：

~~~text
DNS: *.pg.example.com -> 反向代理公网 IP
TLS: 证书覆盖 pagepilot.example.com 与 *.pg.example.com
Proxy: pagepilot.example.com、*.pg.example.com -> PagePilot:8787
~~~

Nginx 最小示例：

~~~nginx
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

如果没有全局 `map`，可在 `http {}` 中添加：

~~~nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}
~~~

非标准外部端口（例如 `1143`）时，监听和证书配置要与实际入口一致，并让 `Host`/`X-Forwarded-Host` 保留端口；`X-Forwarded-Port` 可以作为补充，不能替代带端口的 `X-Forwarded-Host`。服务端配置使用 `HOSTCTL_APP_URL_PORT=1143`，不要把内部 8787 写进应用链接。

Caddy 通常不需要手写 Upgrade 头：

~~~caddyfile
pagepilot.example.com, *.pg.example.com {
    reverse_proxy 127.0.0.1:8787
}
~~~

通配符证书、历史版本 URL 和验证命令见 [APP_URL_MODE.md](../deploy/APP_URL_MODE.md)。

## 备份和恢复

必须成对备份 SQLite 和 hosted 文件：

~~~bash
docker compose stop hostctl
tar -czf backup/pagepilot-$(date +%F).tar.gz data/docker
docker compose start hostctl
~~~

systemd 示例：

~~~bash
sudo systemctl stop hostctl-server
sudo tar -czf /backup/pagepilot-$(date +%F).tar.gz /var/lib/hostctl /var/www/hosted
sudo systemctl start hostctl-server
~~~

恢复前停止服务，恢复后检查文件属主、SQLite 权限和软链接，再启动并访问 `/api/health`。不要只恢复数据库或只恢复 hosted，否则版本记录与静态文件会不一致。

## 升级流程

1. 备份 SQLite、hosted、环境文件和当前二进制。
2. 拉取代码，查看 [CURRENT_STATUS_AND_TODO.md](CURRENT_STATUS_AND_TODO.md) 的兼容性说明。
3. 前端或 Skill 有变更时运行 `make build` 或分别构建并刷新内嵌产物。
4. 先在临时副本运行 `node scripts/legacy-upgrade-qa.mjs`；目标支持 Docker 时再运行 `node scripts/docker-upgrade-qa.mjs`。
5. 替换二进制或执行 `docker compose up -d --build`。
6. 检查健康、首页、发布页、后台、市场、应用当前/历史 URL、Skill ZIP 和 WebSocket。
7. 保留旧版本二进制和备份，确认稳定后再清理。

主密钥不能随升级轮换。数据库迁移会保留历史站点、版本、Token、访问密码、屏幕绑定和审计数据。

## 监控和验收

~~~bash
curl -fsS https://pagepilot.example.com/api/health
curl -fsS https://pagepilot.example.com/openapi.json | jq '.info.title'
curl -fsS https://pagepilot.example.com/deploy >/dev/null
curl -fsS https://pagepilot.example.com/market >/dev/null
curl -fsS https://pagepilot.example.com/admin >/dev/null
curl -fsS https://pagepilot.example.com/skill/pagep.zip >/dev/null
~~~

持续观察：

~~~bash
journalctl -u hostctl-server -f
journalctl -u caddy -f
df -h /var/www/hosted /var/lib/hostctl
~~~

外部监控至少覆盖 `/api/health`、`/deploy`、`/market`、`/admin`、`/openapi.json` 和 `/skill/pagep.zip`。泛域名模式额外检查一个真实 code 的当前 URL 和 `/versions/N/` 历史 URL。

## 排障速查

| 现象 | 优先检查 |
| --- | --- |
| 页面 404 | 代理是否把所有路径转发给 PagePilot；不要只放行 `/api`。 |
| 返回内网链接 | CLI/Skill/MCP 的 server 是否使用内网地址；代理的 `Host` 和 `X-Forwarded-Host` 是否为外部地址。 |
| HTTPS 页面嵌入 HTTP 应用被拦截 | `HOSTCTL_APP_URL_SCHEME`、端口、证书和反代头是否一致；检查 [APP_URL_MODE.md](../deploy/APP_URL_MODE.md)。 |
| 泛域名应用打不开 | `dig code.pg.example.com`、证书 SAN、Nginx `server_name`、代理日志和 `curl --resolve`。 |
| 历史版本 404 | 代理是否保留 `/agent/{code}/versions/*` 或泛域名 `/versions/*`；版本是否被删除或文件存储缺失。 |
| `/api/device/ws` 失败 | Nginx 是否设置 HTTP/1.1、Upgrade 和 Connection；Caddy 通常无需额外配置。 |
| Skill ZIP 404 | 重新运行 `python scripts/build_skill_zip.py` 并重建服务端；检查 `/skill/pagep.zip`。 |
| 首个管理员登录失败 | 数据库是否已有可用管理员；空库是否同时设置了管理员用户名和密码。 |
| 发布后文件丢失 | 数据卷/hosted 目录是否被清理；OSS 配置和版本存储归属是否匹配。 |
| Token 失效 | 是否过期或已吊销；Token 明文是否只在创建时保存了一次。 |

排障时优先保留 `requestId`、时间、HTTP 状态、`errorCode` 和脱敏后的代理配置，不要上传用户源码或密钥。
