# hostctl

hostctl 是 PagePilot 的静态站点控制平面。它让用户和 AI Agent 都能发布单文件 HTML 应用或多文件静态站点，并通过一个小巧的 Go 服务统一管理版本、加密访问、锁定、回滚、令牌、管理员操作和市场浏览。

![PagePilot 首页](docs/screenshots/home.png)

![PagePilot 后台](docs/screenshots/admin.png)

## 包含内容

- 公共首页和市场位于 `/`，展示可搜索、可点赞、可访问密码保护的作品。
- 首页支持全屏弹幕动画。
- 手动部署页面，支持粘贴 / 上传流程。
- 管理员控制台位于 `/admin`，包含登录、仪表盘、部署、站点、令牌、配置和版本控制。
- JSON API，并对外提供 `/openapi.json` 供 Agent 与外部集成使用。
- 版本化静态托管，访问路径为 `/agent/{code}`，并提供短链接 `/{code}`。
- Go CLI（`hostctl`）、MCP 服务器（`hostctl-mcp`）以及一个独立可用的 Codex/Claude 技能脚本。
- 匿名部署配额、用户所有的 Agent 绑定令牌，以及按用户的部署上限。
- 元数据存储使用 SQLite，静态资源托管在文件系统上。
- 提供 Caddy 和 systemd 的生产环境模板。

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

对外使用 Caddy 作为公开 TLS 反向代理。完整的 VPS 部署、首个管理员令牌引导、备份与监控说明，请参见 [deploy/README.md](deploy/README.md)。

## API 概览

核心端点：

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/api/health` | 健康检查 |
| `GET` | `/openapi.json` | 机器可读的 API 契约 |
| `GET` | `/api/session` | 创建 / 读取匿名部署会话 |
| `POST` | `/api/deploy` | 部署新站点或追加版本 |
| `GET` | `/api/deploy/content?code=&version=&download=1` | 读取元数据或下载 HTML / zip |
| `GET` | `/api/deploys` | 公共市场搜索 |
| `GET` | `/api/deploys/{publicId}` | 通过 UUID 或 code 获取公共部署详情 |
| `POST` | `/api/deploys/{code}/like` | 公开点赞 |
| `GET` | `/api/deploys/{code}/versions` | 列出所有版本 |
| `PATCH` | `/api/deploys/{code}/versions/{version}` | 覆盖未锁定的版本或修改状态 |
| `DELETE` | `/api/deploys/{code}/versions/{version}` | 删除未锁定的版本 |
| `PATCH` | `/api/deploys/{code}/current` | 回滚或切换当前版本 |
| `POST` | `/api/deploys/{code}/versions/{version}/lock` | 锁定 / 解锁某个版本 |
| `GET/PATCH` | `/api/deploys/{code}/primary-strategy` | 读取或设置 `likes` / `latest` 策略 |
| `GET` | `/api/admin/session` | 校验管理员登录 |
| `GET` | `/api/admin/anonymous-sessions` | 查看匿名会话使用情况 |
| `GET` | `/api/admin/sites` | 管理员站点清单 |
| `DELETE` | `/api/admin/sites/{code}` | 删除整个站点 |
| `POST` | `/api/token` | 创建令牌 |
| `GET` | `/api/tokens` | 列出令牌 |
| `DELETE` | `/api/tokens/{id}` | 吊销令牌 |
| `POST` | `/api/agent-binding-codes` | 创建一次性 Agent 绑定码 |
| `GET` | `/api/agent-binding-codes` | 列出当前登录用户最近的绑定码 |
| `POST` | `/api/agent/bind` | 用绑定码换取用户所有的 Agent 令牌 |
| `GET/PUT` | `/api/config` | 读取 / 更新运行时配置 |

生产环境认证规则：

- 匿名部署允许在配置的配额内进行，默认每个会话可部署 5 次。Agent 先调用 `/api/session`，再在写请求中携带 `X-Hostctl-Session`。
- 匿名部署 7 天后过期；用户令牌的部署则是永久的。
- 匿名配额用尽后，Agent 应提示用户注册 / 登录，生成 Agent 绑定码，再到 `/api/agent/bind` 换取令牌。
- 一个用户可绑定多个 Agent。每个 Agent 拥有独立的用户所有令牌，并且只能修改该用户的站点。
- 管理员控制台、令牌管理、配置写入以及整站删除都需要管理员令牌（`isAdmin=true`）。
- 公共市场、点赞、静态页面以及内容读取保持公开。

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
bin/hostctl token create admin --admin
```

## Agent 技能

内置技能位于 [skill/hostctl-deploy/SKILL.md](skill/hostctl-deploy/SKILL.md)。它的 Python 包装器仅依赖标准库，可以脱离 Go CLI 单独运行：

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py doctor --server http://127.0.0.1:8787
python skill/hostctl-deploy/scripts/hostctl_deploy.py bind <binding-code> --agent-label codex-laptop
python skill/hostctl-deploy/scripts/hostctl_deploy.py deploy ./site --code demo --description "Shareable demo site."
python skill/hostctl-deploy/scripts/hostctl_deploy.py deploy ./site --update --description "Revised demo site."
python skill/hostctl-deploy/scripts/hostctl_deploy.py token create --label alice --admin
python skill/hostctl-deploy/scripts/hostctl_deploy.py admin sites
```

本项目还在 `cmd/hostctl-mcp` 提供了 MCP 服务器，供偏好通过 stdio 走 JSON-RPC 的工具使用。

对已有项目，Agent 应在原 code 上追加版本，而不是创建新的短链接。技能会把 `source -> code` 记在 `~/.hostctl/projects.json`；如果没有记录的 code，Agent 在部署更新前应向用户索要原始 code 或 URL。

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
- 令牌明文只返回一次；服务器只保存哈希。
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
skill/              hostctl-deploy agent 技能
```
