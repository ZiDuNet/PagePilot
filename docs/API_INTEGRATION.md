# API 与集成

PagePilot 同时提供 HTTP API、Go CLI、Python Skill 和 stdio MCP。所有入口共享同一套服务端权限和 URL 返回契约。

## 基本约定

- API 基础地址是 PagePilot 入口，例如 `https://pagepilot.example.com`。
- 使用 HTTPS 生产环境；反向代理应透传 `Host`、`X-Forwarded-Host`、`X-Forwarded-Proto` 和 `X-Forwarded-For`。
- JSON 请求使用 `Content-Type: application/json`；目录、ZIP 和二进制资源使用 `multipart/form-data`。
- 成功响应通常包含 `success: true`；失败响应统一使用[错误格式](#错误格式)。
- 生成访问 URL 时只使用服务端返回的 `url`、`pathUrl`、`domainUrl`、`versionUrl`，不要在客户端猜测主机名或模式。

基础端点：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/api/health` | 健康检查。 |
| `GET` | `/api/config` | 读取公开能力摘要；管理员会话可读取完整运行设置。 |
| `GET` | `/api/session` | 创建或读取匿名发布 session。 |
| `POST` | `/api/session/claim` | 将匿名 session 认领到当前注册用户。 |
| `POST` | `/api/security/csp-report` | 接收托管页面 CSP 违规报告。 |
| `GET` | `/openapi.json` | 获取机器可读契约。 |

## 认证

### 浏览器会话

用户端和管理后台使用服务端 HttpOnly、SameSite=Lax Cookie：

~~~javascript
fetch("/api/session", { credentials: "same-origin" });
~~~

后台写操作需要管理员登录 Cookie。浏览器前端不会从 localStorage/sessionStorage 自动读取 Bearer Token。

### Token

CLI、Skill、MCP 和外部自动化使用：

~~~http
Authorization: Bearer <pagepilot-token>
~~~

Token 只在创建时返回一次明文；服务端保存哈希。可设置过期时间或 TTL，吊销后立即失效。创建和管理：

~~~bash
pagep token create ci-bot --ttl 24h --save
pagep token list
pagep token revoke <token-id>
~~~

### 匿名 session

无 Token 的 Agent 可以先请求 `GET /api/session`，随后在写请求中发送：

~~~http
X-Hostctl-Session: <session-id>
~~~

Skill 会把它保存在 `~/.pagep/session.json`。注册用户可以通过 `POST /api/session/claim` 认领；认领后该 session 不能继续匿名发布。

## 发布

### 单文件 JSON

`POST /api/deploy` 的最小请求：

~~~bash
curl -X POST "$PAGEPILOT_SERVER/api/deploy" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $PAGEPILOT_TOKEN" \
  --data-binary @- <<'JSON'
{
  "title": "演示站点",
  "description": "可分享的演示页面",
  "filename": "index.html",
  "content": "<!doctype html><html><body><h1>Hello PagePilot</h1></body></html>",
  "visibility": "unlisted"
}
JSON
~~~

`filename` 是可选入口提示。普通单文件、目录和 ZIP 都可以省略，让服务端自动识别；只有入口缺失或多个入口时才显式指定。

### 多文件或 ZIP multipart

~~~bash
curl -X POST "$PAGEPILOT_SERVER/api/deploy" \
  -H "Authorization: Bearer $PAGEPILOT_TOKEN" \
  -F 'title=多文件站点' \
  -F 'description=包含 CSS、JS 和图片的站点' \
  -F 'visibility=unlisted' \
  -F 'files=@./site.zip'
~~~

目录和 ZIP 会执行路径安全检查、入口识别和 Bundle 元数据记录。ZIP 中的绝对路径、`..`、空路径段、符号链接、重复路径、多个独立根目录和大小超限会返回 `stage=zip_bundle`。

### 复用市场作品

发布请求可传 `templateSourceCode` 和 `templateSourceVersion`。服务端会记录来源作品并增加复用次数；源码下载和复用权限由来源站点策略决定，不会因为知道访问密码而绕过策略。

## URL 返回契约

发布成功响应至少包含：

~~~json
{
  "success": true,
  "code": "demo",
  "url": "https://demo.pg.example.com/",
  "pathUrl": "https://pagepilot.example.com/agent/demo/",
  "domainUrl": "https://demo.pg.example.com/",
  "detailUrl": "https://demo.pg.example.com/",
  "versionUrl": "https://demo.pg.example.com/versions/2/",
  "versionPathUrl": "https://pagepilot.example.com/agent/demo/versions/2/",
  "versionDomainUrl": "https://demo.pg.example.com/versions/2/",
  "versionNumber": 2
}
~~~

- `pathUrl` 始终表示 `/agent/{code}/` 形态。
- `domainUrl` 仅在 `domain/dual` 且已配置后缀时出现。
- `url` 是当前模式的主 URL。
- `versionUrl` 是本次版本的直接预览地址。
- `detailUrl` 是站点当前入口；使用 `primaryVersionStrategy` 决定显示 likes 版本还是 latest 版本。
- 历史版本地址在路径模式为 `/agent/{code}/versions/{version}/`，泛域名模式为 `https://{code}.{suffix}/versions/{version}/`。

URL 模式切换后，版本列表和新响应会按当前配置重新生成；站点数据不会保存“创建时的域名模式”。旧路径链接是否仍可用取决于反向代理是否继续转发 `/agent/*`。

## 版本和内容

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/api/deploys/{code}/versions` | 列出版本和所有 URL 变体。 |
| `PATCH`/`POST` | `/api/deploys/{code}/versions/{version}` | 覆盖未锁定版本或修改状态。 |
| `DELETE` | `/api/deploys/{code}/versions/{version}` | 删除未锁定版本。 |
| `POST` | `/api/deploys/{code}/versions/{version}/lock` | 锁定/解锁版本。 |
| `PATCH`/`POST` | `/api/deploys/{code}/current` | 切换当前版本。 |
| `GET` | `/api/deploy/content?code=demo&version=2` | 读取源码元数据；按权限决定内容。 |
| `GET` | `/api/deploy/content?code=demo&version=2&download=1` | 下载源码或 ZIP；需满足复用策略。 |
| `PATCH` | `/api/deploy/content` | 兼容旧客户端的追加版本接口。 |

追加版本优先使用 `POST /api/deploy` 的 `createVersion` 或 CLI `append`。删除单个版本不会恢复站点额度；删除整个站点才会释放一个应用额度。

## 创作市场和访问控制

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/api/deploys` | 搜索和分页浏览公开作品。 |
| `GET` | `/api/deploys/{publicId}` | 通过 UUID 或 code 查看公开详情。 |
| `GET` | `/api/market/categories` | 获取分类 slug。 |
| `POST` | `/api/deploys/{code}/like` | 公开点赞。 |
| `POST` | `/api/deploys/{code}/favorite` | 登录用户收藏。 |
| `GET` | `/api/deploys/{code}/qr` | 获取二维码。 |
| `POST` | `/api/deploys/{code}/access` | 校验访问密码并签发短期查看票据。 |
| `PATCH`/`POST` | `/api/deploys/{code}/access` | owner/admin 设置或清除访问密码。 |
| `PATCH`/`POST` | `/api/deploys/{code}/visibility` | owner/admin 修改公开性。 |

访问密码票据有效 5 分钟并绑定版本；改密码或切换当前版本后旧票据失效。访问密码只控制浏览，不授予源码下载和模板复用。

## 屏幕 API

注册用户使用 `/api/screens` 管理自己绑定的屏幕；屏幕端使用 Device Token 调用 `/api/device/*`：

- `/api/screens/bind`：用户用一次性配对码绑定。
- `/api/screens/{id}/publish`：发布应用到屏幕。
- `/api/screens/{id}/screenshot`：请求或读取截图。
- `/api/screens/{id}/command`：刷新、休眠、唤醒或软关机。
- `/api/device/pairing/start`、`/complete`：设备配对。
- `/api/device/manifest`：设备拉取播放清单。
- `/api/device/ws`：实时控制 WebSocket。
- `/api/device/heartbeat`、`/api/device/screenshot`、`/api/device/command/ack`：设备状态和指令回执。

设备请求使用 `Authorization: Device <device-token>`，不能使用用户 Bearer Token。反向代理必须保留 WebSocket Upgrade 头。

## 管理 API

管理员登录后可使用：

- `/api/admin/session`、`/api/admin/login`、`/api/admin/logout`、`/api/admin/setup`：后台会话。
- `/api/admin/sites`：站点列表、详情、置顶、复用策略、安全模式、分类、标签和删除。
- `/api/admin/users`：用户创建、更新和删除。
- `/api/admin/anonymous-sessions`：匿名会话统计。
- `/api/admin/audit-logs`：按动作、站点、用户、IP、时间和关键字筛选审计。
- `/api/admin/skill`、`/api/admin/skill/package`：查看和上传 Skill ZIP。
- `/api/admin/market/categories`：维护市场分类。
- `/api/config`：读取/更新运行设置。
- `/api/account/password`：修改当前账号密码。

## 错误格式

失败 JSON 示例：

~~~json
{
  "success": false,
  "errorCode": "VERSION_LOCKED",
  "stage": "overwrite",
  "detail": "Version 2 is locked and cannot be modified.",
  "hint": "Append a new version instead.",
  "retryAfterSeconds": 0,
  "requestId": "req-..."
}
~~~

客户端建议：

1. 记录 `requestId`，不要记录 Token、密码或源码。
2. 对 `ZIP_*` 错误先按 `hint` 修复项目，再重试。
3. 对冷却/限流错误等待 `retryAfterSeconds`，不要并发重试。
4. 对 `401/403` 检查 Token、Cookie、站点所有权和管理员权限。
5. 对 `MIXED_CONTENT` 或链接协议问题检查 URL scheme、端口和反向代理头。

## OpenAPI 和跨域

运行中的完整契约位于 `GET /openapi.json`；后台 API 文档入口是 `/admin?tab=apiDocs`，旧 `/api-docs.html` 只做兼容重定向。外部浏览器 fetch/XHR 需要把 origin 加入 CORS 白名单；iframe 是否可嵌入由 Embed Policy 控制，两者互不替代。
