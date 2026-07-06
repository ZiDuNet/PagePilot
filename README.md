# PagePilot

PagePilot 是面向 AI Agent 的 HTML / Markdown / ZIP / 多文件静态站点发布平台。你把需求告诉 Agent，Agent 生成页面或应用，PagePilot 负责上线、访问密码、版本回滚、锁定下架、创作市场复用、API/CLI/MCP 接入和广告屏投放。

当前版本：`0.2.0`

![PagePilot 首页](docs/screenshots/home.png)

![PagePilot 后台](docs/screenshots/admin.png)

## 包含内容

- 公共首页和市场位于 `/`，展示可搜索、可点赞、可访问密码保护的作品。
- 首页支持全屏弹幕动画。
- 手动部署页面位于 `/deploy`，创作市场位于 `/market`，屏幕介绍页位于 `/screens/`；API 文档收进后台 `/admin?tab=apiDocs`，机器可读契约位于 `/openapi.json`。
- 用户端页面由 `frontend/user` 的 React + Vite 工程构建，产物输出到 `internal/web/user/app`，并由 Go `embed` 打包进服务端二进制。
- 管理员控制台位于 `/admin`，由 `frontend/admin` 的 React + Vite 工程构建，包含登录、仪表盘、部署、站点、屏幕、令牌、用户、匿名、配置和版本控制。
- JSON API，并对外提供 `/openapi.json` 供 Agent 与外部集成使用。
- 版本化静态托管，访问路径为 `/agent/{code}`，并提供应用访问 URL `/agent/{code}/`。
- Go CLI（兼容旧 `hostctl` 命令，新文档统一称 `pagep`）、MCP 服务器以及一个独立可用的 PagePilot Agent Skill。
- 匿名部署配额、用户所有的 Agent Token，以及按用户的部署上限。
- 硬件屏幕绑定与投放：注册用户可以绑定多个广告屏，屏幕端 APP 通过 X5 WebView 播放 PagePilot 应用。
- 元数据存储使用 SQLite，静态资源托管在文件系统上。
- Skill 下载包默认内置在服务端二进制中，主下载地址为 `/skill/pagep.zip`；旧 `/skill/hostctl-deploy.zip` 保留兼容。后台不再编辑 Skill 源文件，只维护下载包。
- 提供 Docker、Caddy 和 systemd 的生产环境模板。

## 快速开始

```bash
go build -o bin/hostctl-server ./cmd/hostctl-server
HOSTCTL_DEV=1 ./bin/hostctl-server --addr 127.0.0.1:8787
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

主站不需要配置域名。首页、后台、`/agents/`、`/screens/`、二维码、Skill/MCP 文案和 `/agent/{code}/` 路径模式链接都会跟随当前打开 PagePilot 的域名。Skill、MCP 和 CLI 调用接口时，应把 `--server` / `HOSTCTL_SERVER` 设置为希望返回给用户的 PagePilot 入口。完整 Docker 说明请见 [deploy/DOCKER.md](deploy/DOCKER.md)。
应用访问地址默认保持 `/agent/{code}/` 路径模式；如需启用 `https://{code}.example.com/` 泛域名模式，请参考 [deploy/APP_URL_MODE.md](deploy/APP_URL_MODE.md)。

后台“运行设置”只配置应用泛域名、上传限制、CORS、iframe 嵌入和匿名额度。只有启用应用泛域名模式时，才需要填写应用域名后缀；它不会改变主站入口。

发布接口返回的 `url`、`detailUrl` 和 `versionUrl` 是最终权威结果。路径模式下它们按当前访问入口或 `--server` 生成；泛域名模式下它们按后台配置的应用域名后缀、协议和端口生成。Skill、MCP、CLI 不应该自行拼最终应用 URL。

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
| `POST` | `/api/screens/{screenId}/screenshot` | 下发屏幕截图指令 |
| `GET` | `/api/screens/{screenId}/screenshot` | 查看屏幕最近一次截图 |
| `POST` | `/api/screens/{screenId}/command` | 下发刷新、休眠、唤醒、软关机指令 |
| `DELETE` | `/api/screens/{screenId}` | 解绑自己的屏幕 |
| `POST` | `/api/device/pairing/start` | 屏幕 APP 创建短期配对码 |
| `POST` | `/api/device/pairing/complete` | 屏幕 APP 换取 Device Token |
| `GET` | `/api/device/ws` | 屏幕 APP 使用 Device Token 建立 WebSocket 控制通道 |
| `GET` | `/api/device/manifest` | 屏幕 APP 使用 Device Token 拉取播放清单 |
| `POST` | `/api/device/heartbeat` | 屏幕 APP 上报在线状态和设备信息 |
| `POST` | `/api/device/screenshot` | 屏幕 APP 按指令回传截图 |
| `POST` | `/api/device/command/ack` | 屏幕 APP 确认指令已完成 |
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
| `GET` | `/api/admin/session` | 校验后台登录会话 |
| `GET` | `/api/admin/anonymous-sessions` | 查看匿名会话使用情况 |
| `GET` | `/api/admin/sites` | 管理员站点清单 |
| `DELETE` | `/api/admin/sites/{code}` | 删除整个站点 |
| `POST` | `/api/token` | 创建永久或临时令牌 |
| `GET` | `/api/tokens` | 列出令牌 |
| `DELETE` | `/api/tokens/{id}` | 吊销令牌 |
| `GET/PUT` | `/api/config` | 读取 / 更新运行时配置 |

生产环境认证规则：

- 匿名部署允许在配置的配额内进行，默认每个会话可部署 5 次。Agent 可以先调用 `/api/session` 并在写请求中携带 `X-Hostctl-Session`；如果未登录发布没有携带 session，服务端也会自动创建并记录匿名 session。
- 匿名身份分两类入口但底层统一：网页匿名使用浏览器 `hostctl_anon_session` HttpOnly cookie；Agent 匿名使用本地 `~/.hostctl/session.json` 中的 `sessionId` 并发送 `X-Hostctl-Session`。两者在服务端都映射为 `anon:{sessionId}` owner，`X-Hostctl-Agent-Id`、`X-Hostctl-Agent-Label`、IP 和 UA 只用于后台展示和排查。
- 匿名会话可以设置访问密码、删除和修改自己发布的站点；匿名统计只按实际未登录发布记录，未发布的空 session 不计入后台列表。
- 用户注册 / 登录或使用 Bearer Token 后，可以调用 `/api/session/claim` 认领当前匿名 session。认领后该 session 已发布的站点会迁移到 `user:{userId}`，一个用户可以认领多个匿名 session。
- Token 必须归属到用户。创建 Token 时默认永久有效，也可传 `expiresAt` 或 `ttlSeconds` 创建临时 Token。
- 管理员控制台、令牌管理、配置写入以及整站删除都需要管理员权限（`isAdmin=true`）。
- 后台“Skill & MCP”只维护固定下载包。默认内置包会保证 `/skill/pagep.zip` 不返回 404；旧的 `/skill/hostctl-deploy.zip` 保留兼容。管理员上传 ZIP 后会覆盖内置包。源码修改应在仓库或本地完成并重新打包，不能在后台直接编辑。
- 公共市场、点赞、静态页面以及内容读取保持公开。
- 创作市场保留点赞排行；管理员置顶会优先于所有排序，置顶分组内部仍按当前选择的排序（如 `likes_desc`）排列。
- 访问密码输入入口保持公开，匿名访客也可以输入密码查看加密站点；验证通过后浏览器获得 5 分钟签名访问票据，站点改密码后旧票据立即失效。
- 屏幕投放只允许注册用户 Token 或登录用户会话调用；匿名 session 不能绑定屏幕或投屏。
- 屏幕 APP 不持有用户 Token，只持有可吊销的 Device Token；Device Token 只能拉取自己的 manifest、建立自己的 WebSocket 控制通道、上报心跳和按指令回传截图。
- 屏幕配对码是 5 分钟一次性短码，只用于首次绑定，不是长期权限。
- 内置页面 `/deploy`、`/market`、`/agents/`、`/screens/` 由 Go 服务内嵌返回；后台 API 文档位于 `/admin?tab=apiDocs`。反向代理应把这些路径原样转发给 PagePilot。
- 如果前面有 Nginx、宝塔或负载均衡，必须为 `/api/device/ws` 转发 WebSocket Upgrade 头，否则后台刷新、截图、休眠等指令会退化为不可实时或连接失败。
- CORS 白名单只控制外部网页用 `fetch` / XHR 调用 PagePilot API，不控制 iframe。应用是否允许被外部网站嵌入由后台“运行设置 -> 跨域与嵌入 -> iframe 嵌入”控制，支持任意、仅本站、白名单和禁止嵌入，底层会写入托管应用响应的 CSP `frame-ancestors`。

## 手动部署与多文件站点

手动部署页面和后台“发布应用”都支持两种模式：

- 单 HTML：粘贴或上传一个 HTML 文件，入口默认为 `index.html`。
- 多文件项目：上传多个文件或目录，PagePilot 按相对路径保存 `HTML/CSS/JS/图片/字体/JSON` 等资源，并优先选择 `index.html` 作为入口；如果没有 `index.html`，会选择第一个 HTML 文件。

`POST /api/deploy` 同时支持 `application/json` 和 `multipart/form-data`。CLI、MCP 和 Skill 发布本地文件、目录或 ZIP 时优先走 multipart：目录会临时打成 ZIP，ZIP 文件作为单个文件上传，服务端负责识别真实站点根目录、入口文件和文件树。旧 JSON/base64 请求仍保留兼容，但不再作为大包、多文件项目的首选链路。

ZIP 入口识别规则会剥离单一外层目录，优先选择 `index.html`、`index.htm`、`README.md` 或 `README.markdown`；如果包里存在多个彼此独立的网站根，服务端会拒绝并返回友好错误，避免误把批量文件包发布成一个坏站点。ZIP 中的绝对路径、盘符、`..`、空路径段和路径穿越都会被拒绝。

多文件站点应使用相对链接，例如 `./assets/app.css`、`settings.html`。默认兼容入口是 `/agent/{code}/`，所以不要在路径模式下把资源写成 `/assets/app.css` 这种根路径，除非已经启用泛域名模式。

更新已有发布时必须填写已有 `code` 并选择“追加为新版本”。`code` 可以从返回链接 `/agent/{code}/`、应用详情页、后台站点列表、Skill `list_sites` 或 MCP `list_sites` 获取。追加版本不会创建新访问地址，也不会改变原站点的公开方式和访问密码。

Markdown 会作为一等入口托管，支持相对图片、表格、任务列表、代码块、Mermaid/数学公式语义块和渲染缓存。当前服务端仍以安全渲染为主：Markdown 响应使用更严格的无脚本 CSP，真实 Mermaid 绘图、KaTeX 排版和完整语法高亮属于后续增强，不应把未验证的外部脚本直接塞进 Markdown 渲染链路。

市场搜索已接入 SQLite FTS5，并保留中文 `LIKE` 回退；老数据库启动时会自动回填索引。新增的渲染缓存、Bundle 元数据和审计日志表都是增量迁移，正常 Docker 升级不会清空已有站点、版本或用户数据。

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
go build -o bin/pagep ./cmd/hostctl

bin/pagep config set server https://host.example.com
bin/pagep config set token <pagepilot-token>

bin/pagep deploy ./site --code my-landing --description "Landing page for Project X."
bin/pagep append my-landing ./site-v2 --description "Second version with updated copy."
bin/pagep versions my-landing
bin/pagep current my-landing 1
bin/pagep lock my-landing 2
bin/pagep token create ci-bot
bin/pagep token create temp-runner --ttl 24h
bin/pagep token create admin --admin
bin/pagep claim-session <anonymous-session-id>
bin/pagep admin pin-site my-landing
bin/pagep admin pin-site my-landing --unpin
```

旧 `hostctl` 二进制名保留为兼容别名；新文档、Docker 镜像和 Agent 文案统一使用 `pagep`。

## Agent 技能

内置 Skill 位于 [skill/hostctl-deploy/SKILL.md](skill/hostctl-deploy/SKILL.md)。它的 Python 包装器仅依赖标准库，可以脱离 Go CLI 单独运行。后台和用户端主下载地址为 `/skill/pagep.zip`，旧地址 `/skill/hostctl-deploy.zip` 保留兼容；服务端优先返回后台上传的 ZIP，没有上传包时返回内置默认包。

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py doctor --server http://127.0.0.1:8787
python skill/hostctl-deploy/scripts/hostctl_deploy.py deploy ./site --code demo --description "Shareable demo site."
python skill/hostctl-deploy/scripts/hostctl_deploy.py deploy ./site --code demo --update --title "演示站点" --description "Revised demo site."
python skill/hostctl-deploy/scripts/hostctl_deploy.py token create --label ci-bot
python skill/hostctl-deploy/scripts/hostctl_deploy.py token create --label temp-runner --ttl-seconds 86400
python skill/hostctl-deploy/scripts/hostctl_deploy.py claim-session
python skill/hostctl-deploy/scripts/hostctl_deploy.py admin sites
python skill/hostctl-deploy/scripts/hostctl_deploy.py admin pin-site my-landing
```

`--server` 或 `HOSTCTL_SERVER` 表示本次 Agent 调用 PagePilot API 的入口地址，不是全局主站配置。路径模式发布成功后，接口返回的应用链接会使用这个入口；如果要把公网链接交给用户，就用公网地址作为 `--server`。泛域名模式的应用链接由后台“运行设置 -> 应用链接规则”决定，和 `--server` 只用于调用控制面入口的职责分开。

屏幕投放命令仅支持注册用户 Token：

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py --server https://host.example.com screen list
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen bind 123456 --name "大厅屏"
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen publish --screen screen_xxx --app my-landing
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen publish --screen screen_xxx --source ./site --title "大屏展示" --description "Fullscreen display for the lobby."
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen screenshot screen_xxx --output ./screen-shot.jpg
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen refresh screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen sleep screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen wake screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen shutdown screen_xxx
```

本项目还在 `cmd/hostctl-mcp` 提供了 MCP 服务器，构建后的对外命令名为 `pagep-mcp`，供偏好通过 stdio 走 JSON-RPC 的工具使用。MCP 支持部署、访问密码、匿名认领、管理员置顶，以及 `list_screens`、`bind_screen`、`publish_screen`、`request_screen_screenshot`、`send_screen_command`、`unbind_screen` 等屏幕工具。旧 `hostctl-mcp` 可作为兼容别名保留。

MCP 使用 `HOSTCTL_SERVER` 作为控制面入口，并把它发送给后端用于路径模式 URL 生成。反向代理部署时，如果 MCP 使用内网地址连接 PagePilot，路径模式返回值也会偏向内网地址；生产环境建议让 MCP 使用用户可访问的公网入口。

对已有项目，Agent 应在原 code 上追加版本，而不是创建新的访问地址。技能会把 `source -> code` 记在 `~/.hostctl/projects.json`；如果没有记录的 code，Agent 在部署更新前应向用户索要原始 code 或 URL。

## 硬件屏幕 APP

屏幕端代码位于 [apps/screen-app](apps/screen-app)。当前路线是 Android Kotlin 壳 + X5 WebView：

- 首次启动由现场人员输入 PagePilot 服务器地址，地址保存在设备本地。
- 屏幕 APP 创建配对码，用户在后台“屏幕”页或 Skill 中输入配对码绑定。
- 一个注册用户可以绑定多个屏幕。
- 投屏发布的是 manifest 播放清单，不是直接下发裸 HTML 字符串。
- 屏幕 APP 通过 `/api/device/ws` 保持 WebSocket 长连接，投屏、刷新、截图、休眠、唤醒和软关机指令会实时下发；`/api/device/manifest` 保留为初始化和兜底读取。
- 屏幕端右上角连续点击 5 次打开隐藏设置，可查看绑定用户、设备信息、分辨率、横竖屏、WebSocket 状态和 X5/WebView 运行状态，也可切换服务器或清除绑定。
- 后台、Skill 和 MCP 可下发截图、刷新、休眠、唤醒和软关机指令；截图只在后台指令触发时回传，后台点击截图会弹出等待窗口并在新图片返回后立即显示。
- 软关机、开机自启和定时开关机依赖硬件或系统权限，通用 APP 侧只能做黑屏待机和刷新播放。

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

## 存储、邮箱验证与 OSS

当前上传文件默认存储在本地文件系统：

- Docker 部署：`./data/docker/hosted` 挂载到容器内 `/var/www/hosted`。
- 本地开发：`HOSTCTL_DEV=1` 时使用 `./data/hosted`。
- 元数据、账号、Token、分类、版本和运行设置写入 SQLite，Docker 默认挂载到 `./data/docker/hostctl/hostctl.db`。

注册与存储能力通过环境变量控制，后台“运行设置”会展示当前状态：

- `HOSTCTL_ALLOW_REGISTRATION=true|false`：是否允许公开注册。关闭后登录页隐藏注册入口，`POST /api/auth/register` 会返回 403；管理员仍可在后台维护用户。
- `HOSTCTL_EMAIL_VERIFICATION_ENABLED=false`：注册邮箱验证开关。开启后注册页会要求邮箱、图片验证码和 6 位邮箱验证码；服务端通过 SMTP 发送验证码，注册成功后记录 `email_verified=true`。如果开启但 SMTP 未配置，注册页会提示联系管理员。
- `HOSTCTL_STORAGE_BACKEND=local|oss`：文件存储后端。`local` 使用本机 `/var/www/hosted`；`oss` 使用阿里云 OSS，发布写入、预览读取、源码下载、覆盖版本、删除版本和删除站点都会走对象存储。
- 阿里云 OSS 相关：`HOSTCTL_OSS_ENDPOINT`、`HOSTCTL_OSS_BUCKET`、`HOSTCTL_OSS_ACCESS_KEY_ID`、`HOSTCTL_OSS_ACCESS_KEY_SECRET`、`HOSTCTL_OSS_PREFIX`、`HOSTCTL_OSS_PUBLIC_BASE_URL`。`HOSTCTL_OSS_PREFIX` 建议按环境区分，例如 `prod/pagepilot`。

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
  hostctl-mcp/      MCP 服务器源码（对外命令 pagep-mcp）
internal/
  api/              HTTP 路由、类型、错误、OpenAPI
  auth/             bearer token 服务
  client/           Go API 客户端
  config/           运行时配置
  deploy/           部署 / 版本逻辑
  store/            SQLite 存储
  web/              内嵌的用户与管理界面
  web/skill/        内置 Skill ZIP 下载包
deploy/             Caddy / systemd 生产模板
  DOCKER.md         Docker 部署、升级、备份与排障
skill/              hostctl-deploy agent 技能
```
