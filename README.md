# hostctl

hostctl 是 PagePilot 的静态站点控制平面。它让用户和 AI Agent 都能发布单文件 HTML 应用或多文件静态站点，并通过一个小巧的 Go 服务统一管理版本、加密访问、锁定、回滚、令牌、管理员操作和市场浏览。

![PagePilot 首页](docs/screenshots/home.png)

![PagePilot 后台](docs/screenshots/admin.png)

## 包含内容

- 公共首页和市场位于 `/`，展示可搜索、可点赞、可访问密码保护的作品。
- 首页支持全屏弹幕动画。
- 手动部署页面位于 `/deploy.html`，API 文档页面位于 `/api-docs.html`，两者都是内置静态页面。
- 管理员控制台位于 `/admin`，包含登录、仪表盘、部署、站点、令牌、配置和版本控制。
- JSON API，并对外提供 `/openapi.json` 供 Agent 与外部集成使用。
- 版本化静态托管，访问路径为 `/agent/{code}`，并提供应用访问 URL `/agent/{code}/`。
- Go CLI（`hostctl`）、MCP 服务器（`hostctl-mcp`）以及一个独立可用的 Codex/Claude 技能脚本。
- 匿名部署配额、用户所有的 Agent Token，以及按用户的部署上限。
- 硬件屏幕绑定与投放：注册用户可以绑定多个广告屏，屏幕端 APP 通过 X5 WebView 播放 PagePilot 应用。
- 元数据存储使用 SQLite，静态资源托管在文件系统上。
- 提供 Docker、Caddy 和 systemd 的生产环境模板。

## 快速开始

```bash
go build -o bin/hostctl-server ./cmd/hostctl-server
HOSTCTL_DEV=1 ./bin/hostctl-server --addr 127.0.0.1:8787 --public-url http://127.0.0.1:8787
```

打开以下地址：

- 用户应用：`http://127.0.0.1:8787/`
- 管理员控制台：`http://127.0.0.1:8787/admin`
- OpenAPI：`http://127.0.0.1:8787/openapi.json`

在开发模式下，数据会保存在 `./data` 下，部署冷却时间为 1 秒（除非另行覆盖），管理员 API 允许内置的开发会话访问。

Docker 快速启动：

```bash
docker compose up -d --build
```

部署前请把 `docker-compose.yml` 中的 `HOSTCTL_PUBLIC_BASE_URL` 改成用户真实访问地址。完整 Docker 说明请见 [deploy/DOCKER.md](deploy/DOCKER.md)。

Docker 首次启动会在空数据库中自动创建默认管理员：

- 用户名：`admin`
- 密码：`123456`

首次登录后请进入后台的“账号设置”立即修改密码。

## 生产模式

生产环境必须开启认证：

```bash
/usr/local/bin/hostctl-server \
  --addr 127.0.0.1:8787 \
  --hosted-dir /var/www/hosted \
  --db /var/lib/hostctl/hostctl.db \
  --public-url https://host.example.com \
  --require-auth
```

对外可以使用 Caddy、Nginx、宝塔或云厂商负载均衡作为公开 TLS 反向代理。Docker 部署请参见 [deploy/DOCKER.md](deploy/DOCKER.md)；systemd + Caddy 部署、首个管理员、备份与监控说明请参见 [deploy/README.md](deploy/README.md)。

## API 概览

核心端点：

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/api/health` | 健康检查 |
| `GET` | `/openapi.json` | 机器可读的 API 契约 |
| `GET` | `/api/session` | 创建 / 读取匿名部署会话 |
| `POST` | `/api/session/claim` | 将匿名会话发布内容认领到当前用户 |
| `POST` | `/api/deploy` | 部署新站点或追加版本 |
| `GET` | `/api/deploy/content?code=&version=&download=1` | 读取元数据或下载 HTML / zip |
| `POST` | `/api/deploys/{code}/access` | 匿名或公开访客输入访问密码，获取 5 分钟查看票据 |
| `PATCH` | `/api/deploys/{code}/access` | 站点 owner 或管理员设置 / 清除访问密码 |
| `GET` | `/api/screens` | 列出当前注册用户绑定的硬件屏幕 |
| `POST` | `/api/screens/bind` | 使用短期配对码绑定硬件屏幕 |
| `POST` | `/api/screens/{screenId}/publish` | 将自己的应用投放到自己的屏幕 |
| `POST` | `/api/device/pairing/start` | 屏幕 APP 创建短期配对码 |
| `POST` | `/api/device/pairing/complete` | 屏幕 APP 换取 Device Token |
| `GET` | `/api/device/manifest` | 屏幕 APP 使用 Device Token 拉取播放清单 |
| `GET` | `/api/deploys` | 公共市场搜索 |
| `GET` | `/api/deploys/{publicId}` | 通过 UUID 或 code 获取公共部署详情 |
| `POST` | `/api/deploys/{code}/like` | 公开点赞 |
| `GET` | `/api/deploys/{code}/versions` | 列出所有版本 |
| `PATCH` | `/api/deploys/{code}/versions/{version}` | 覆盖未锁定的版本或修改状态 |
| `DELETE` | `/api/deploys/{code}/versions/{version}` | 删除未锁定的版本 |
| `PATCH` | `/api/deploys/{code}/current` | 回滚或切换当前版本 |
| `POST` | `/api/deploys/{code}/versions/{version}/lock` | 锁定 / 解锁某个版本 |
| `GET/PATCH` | `/api/deploys/{code}/primary-strategy` | 读取或设置 `likes` / `latest` 策略 |
| `PATCH` | `/api/admin/sites/{code}/pin` | 管理员置顶 / 取消置顶首页应用 |
| `GET` | `/api/admin/session` | 校验管理员登录 |
| `GET` | `/api/admin/anonymous-sessions` | 查看匿名会话使用情况 |
| `GET` | `/api/admin/sites` | 管理员站点清单 |
| `DELETE` | `/api/admin/sites/{code}` | 删除整个站点 |
| `POST` | `/api/token` | 创建永久或临时令牌 |
| `GET` | `/api/tokens` | 列出令牌 |
| `DELETE` | `/api/tokens/{id}` | 吊销令牌 |
| `GET/PUT` | `/api/config` | 读取 / 更新运行时配置 |

生产环境认证规则：

- 匿名部署允许在配置的配额内进行，默认每个会话可部署 5 次。Agent 先调用 `/api/session`，再在写请求中携带 `X-Hostctl-Session`。
- 匿名会话可以设置访问密码、删除和修改自己发布的站点；匿名统计按 session 记录，未发布的空 session 不计入后台列表。
- 用户注册 / 登录或使用 Bearer Token 后，可以调用 `/api/session/claim` 认领当前匿名 session。认领后该 session 已发布的站点会迁移到 `user:{userId}`，一个用户可以认领多个匿名 session。
- Token 必须归属到用户。创建 Token 时默认永久有效，也可传 `expiresAt` 或 `ttlSeconds` 创建临时 Token。
- 管理员控制台、令牌管理、配置写入以及整站删除都需要管理员权限（`isAdmin=true`）。
- 公共市场、点赞、静态页面以及内容读取保持公开。
- 首页应用商城保留点赞排行；管理员置顶会优先于所有排序，置顶分组内部仍按当前选择的排序（如 `likes_desc`）排列。
- 访问密码输入入口保持公开，匿名访客也可以输入密码查看加密站点；验证通过后浏览器获得 5 分钟签名访问票据，站点改密码后旧票据立即失效。
- 屏幕投放只允许注册用户 Token 或登录用户会话调用；匿名 session 不能绑定屏幕或投屏。
- 屏幕 APP 不持有用户 Token，只持有可吊销的 Device Token；Device Token 只能拉取自己的 manifest 和上报心跳。
- 屏幕配对码是 5 分钟一次性短码，只用于首次绑定，不是长期权限。
- 内置页面 `/deploy.html`、`/api-docs.html`、`/agents/` 由 Go 服务内嵌返回；反向代理应把这些路径原样转发给 PagePilot。

结构化错误格式如下：

```json
{
  "success": false,
  "errorCode": "VERSION_LOCKED",
  "stage": "overwrite",
  "detail": "Version 2 is locked and cannot be modified.",
  "hint": "Append a new version instead of overwriting.",
  "retryAfterSeconds": null,
  "requestId": "req-..."
}
```

## CLI

```bash
go build -o bin/hostctl ./cmd/hostctl

bin/hostctl config set server https://host.example.com
bin/hostctl config set token <hostctl-token>

bin/hostctl deploy ./site --code my-landing --description "Landing page for Project X."
bin/hostctl append my-landing ./site-v2 --description "Second version with updated copy."
bin/hostctl versions my-landing
bin/hostctl current my-landing 1
bin/hostctl lock my-landing 2
bin/hostctl token create ci-bot
bin/hostctl token create temp-runner --ttl 24h
bin/hostctl token create admin --admin
bin/hostctl claim-session <anonymous-session-id>
bin/hostctl admin pin-site my-landing
bin/hostctl admin pin-site my-landing --unpin
```

## Agent 技能

内置技能位于 [skill/hostctl-deploy/SKILL.md](skill/hostctl-deploy/SKILL.md)。它的 Python 包装器仅依赖标准库，可以脱离 Go CLI 单独运行：

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py doctor --server http://127.0.0.1:8787
python skill/hostctl-deploy/scripts/hostctl_deploy.py deploy ./site --code demo --description "Shareable demo site."
python skill/hostctl-deploy/scripts/hostctl_deploy.py deploy ./site --update --description "Revised demo site."
python skill/hostctl-deploy/scripts/hostctl_deploy.py token create --label ci-bot
python skill/hostctl-deploy/scripts/hostctl_deploy.py token create --label temp-runner --ttl-seconds 86400
python skill/hostctl-deploy/scripts/hostctl_deploy.py claim-session
python skill/hostctl-deploy/scripts/hostctl_deploy.py admin sites
python skill/hostctl-deploy/scripts/hostctl_deploy.py admin pin-site my-landing
```

屏幕投放命令仅支持注册用户 Token：

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py --server https://host.example.com screen list
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen bind 123456 --name "大厅屏"
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen publish --screen screen_xxx --app my-landing
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen publish --screen screen_xxx --source ./site --title "大屏展示" --description "Fullscreen display for the lobby."
```

本项目还在 `cmd/hostctl-mcp` 提供了 MCP 服务器，供偏好通过 stdio 走 JSON-RPC 的工具使用；管理员置顶对应工具为 `set_site_pin`。

对已有项目，Agent 应在原 code 上追加版本，而不是创建新的访问地址。技能会把 `source -> code` 记在 `~/.hostctl/projects.json`；如果没有记录的 code，Agent 在部署更新前应向用户索要原始 code 或 URL。

## 硬件屏幕 APP

屏幕端代码位于 [apps/screen-app](apps/screen-app)。当前路线是 Android Kotlin 壳 + X5 WebView：

- 首次启动由现场人员输入 PagePilot 服务器地址，地址保存在设备本地。
- 屏幕 APP 创建配对码，用户在后台“屏幕”页或 Skill 中输入配对码绑定。
- 一个注册用户可以绑定多个屏幕。
- 投屏发布的是 manifest 播放清单，不是直接下发裸 HTML 字符串。
- 第一版在线加载 `entryUrl`，manifest 已包含资源 hash，后续可实现离线缓存。

## 存储布局

```text
/var/www/hosted/
  {code}/
    current -> versions/3
    versions/
      1/
      2/
      3/
        index.html
        styles.css
        assets/logo.png
```

SQLite 中保存令牌、站点、版本、文件、点赞与可变设置。静态字节直接落在磁盘上，让对外服务保持简单，也方便备份。

## 限制与安全

- 单文件默认上限：1 MiB。
- 整站默认上限：10 MiB。
- 单站点文件数默认上限：100。
- 描述（description）为必填项，长度上限 240 字符。
- 路径会拒绝绝对路径以及 `..`。
- 版本锁定后无法覆盖或删除。
- 令牌明文只返回一次；服务器只保存哈希，并按 `expires_at` 自动拒绝过期 Token。
- 访问密码票据只保存于 HttpOnly Cookie 中，有效期为 5 分钟，且与当前站点密码哈希绑定。
- 生产服务模板使用受限的 systemd 沙箱。

## 测试

```bash
go test ./...
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py
python test_skill.py
```

`test_skill.py` 需要在 `http://127.0.0.1:8787` 上有本地开发服务器。

## 项目结构

```text
cmd/
  hostctl/          CLI
  hostctl-server/   HTTP 服务器
  hostctl-mcp/      MCP 服务器
internal/
  api/              HTTP 路由、类型、错误、OpenAPI
  auth/             bearer token 服务
  client/           Go API 客户端
  config/           运行时配置
  deploy/           部署 / 版本逻辑
  store/            SQLite 存储
  web/              内嵌的用户与管理界面
deploy/             Caddy / systemd 生产模板
  DOCKER.md         Docker 部署、升级、备份与排障
skill/              hostctl-deploy agent 技能
```
