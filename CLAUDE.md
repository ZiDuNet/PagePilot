# CLAUDE.md

This file is the contributor guide for PagePilot. Product usage and deployment steps belong in [README.md](README.md) and [docs/](docs/README.md).

## 项目概述

PagePilot（兼容旧名 hostctl）是 Agent 优先的静态站点发布平台，当前版本为 `0.3.1`。它支持 HTML、Markdown、ZIP、多文件站点、版本管理、访问密码、创作市场、源码复用、CLI、Skill、MCP 和 Android 屏幕投放。

## 技术栈

| 层次 | 技术 |
|---|---|
| 后端 | Go 1.22、Go 1.22 路由、modernc.org/sqlite（纯 Go SQLite） |
| 用户端 | React 18 + TypeScript + Vite + Ant Design + Lucide |
| 管理端 | React 18 + TypeScript + Vite + Lucide |
| CLI | Go + Cobra，命令名 `pagep`，`hostctl` 兼容 |
| MCP | Go stdio JSON-RPC，命令名 `pagep-mcp`，`hostctl-mcp` 兼容 |
| Skill | Python 标准库脚本，源码位于 `skill/hostctl-deploy` |
| 存储 | SQLite 元数据；本地文件或 Aliyun OSS 资源 |
| 屏幕端 | Android Kotlin + X5 WebView，位于 `apps/screen-app` |

## 目录和边界

| 目录 | 责任 |
|---|---|
| `cmd/hostctl-server` | HTTP 服务入口、生产认证和启动配置 |
| `cmd/hostctl` | Go CLI |
| `cmd/hostctl-mcp` | MCP 服务器 |
| `internal/api` | 路由、请求/响应类型、错误、OpenAPI、URL 规则 |
| `internal/auth` | Bearer Token 和会话认证 |
| `internal/client` | CLI 使用的 Go API 客户端 |
| `internal/config` | 环境变量、默认值和校验 |
| `internal/deploy` | 发布、Bundle、版本、配额和存储 |
| `internal/store` | SQLite schema、迁移和数据访问 |
| `internal/web` | Go embed 的用户端、后台和 Skill ZIP |
| `frontend/user` | 用户端源码；构建到 `internal/web/user/app` |
| `frontend/admin` | 后台源码；构建到 `internal/web/admin/app` |
| `scripts` | 运行时、视觉和升级 QA |
| `deploy` | Docker、systemd、Caddy 和 URL 模式文档 |

`demo/` 是用户的演示素材，除非任务明确要求，不要删除、格式化或提交。不要提交本地竞品目录、数据库、`data/`、Token 或密钥。

## 常用命令

~~~bash
# 完整构建：Skill ZIP + 两个前端 + Go server/CLI/MCP
make build

# 只构建前端或 Linux amd64 产物
make frontend
make build-linux

# 开发服务（写入 data/）
HOSTCTL_DEV=1 go run ./cmd/hostctl-server --addr 127.0.0.1:8787
# 或
make run

# Go 测试
make test
go test -count=1 ./cmd/... ./internal/...

# 前端类型检查和构建
(cd frontend/user && npm run typecheck && npm run build)
(cd frontend/admin && npm run typecheck && npm run build)

# 前端 node:test
node --test frontend/user/scripts/*.test.mjs
node --test frontend/admin/scripts/*.test.mjs

# Skill 测试和打包
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py skill/hostctl-deploy/scripts/pagep.py
python skill/hostctl-deploy/scripts/hostctl_deploy_test.py
python scripts/build_skill_zip.py
~~~

QA 脚本：

~~~bash
node scripts/runtime-qa.mjs
node scripts/visual-qa.mjs
node scripts/legacy-upgrade-qa.mjs
node scripts/docker-upgrade-qa.mjs
~~~

`runtime-qa`、`visual-qa` 和 `legacy-upgrade-qa` 使用临时服务/数据；`docker-upgrade-qa` 需要 Docker Compose 和 Go，并且不能指向生产数据。修改 Skill 后，必须先刷新内嵌 ZIP 再构建服务端。

## 架构要点

启动顺序：

~~~text
cmd/hostctl-server
  -> config.Default / Validate
  -> SQLite schema + migrations
  -> auth + bootstrap admin
  -> deployer + persisted settings
  -> api routes + middleware
  -> HTTP listener
~~~

`internal/api/server.go` 注册 API、静态页面和应用访问路由。`internal/api/app_url.go` 统一生成路径、泛域名和双模式 URL；调用方必须使用服务端返回的 URL 变体。

身份模型：

- 浏览器用户/管理员：HttpOnly、SameSite=Lax 会话 Cookie。
- CLI/Skill/MCP：`Authorization: Bearer <token>`。
- 匿名 Agent：`X-Hostctl-Session` + 本地 `~/.pagep/session.json`。
- 屏幕设备：`Authorization: Device <device-token>` + WebSocket。

存储模型：

- SQLite 记录账号、Token、站点、版本、文件树、市场、审计和运行设置。
- hosted 目录保存本地站点文件，`current` 指向当前版本。
- OSS 模式按版本记录存储归属；切换配置不会自动迁移历史文件。

## 实现约束

- 保持 `/skill/pagep.zip` 主路径和 `/skill/hostctl-deploy.zip` 兼容路径。
- 新 UI、文档和 CLI 文案使用 PagePilot/`pagep`；旧别名仅用于兼容。
- 保持旧发布 API 和 URL 字段兼容；新增字段同步更新 Go 类型、OpenAPI、前端、CLI/Skill 和 [docs/API_INTEGRATION.md](docs/API_INTEGRATION.md)。
- 发布成功的 `url`、`detailUrl`、`versionUrl` 以后端响应为准；不要在客户端拼接 URL。
- `filename` 是可选入口提示；普通 HTML、目录和 ZIP 让服务端自动识别。
- 路径和 ZIP 必须拒绝绝对路径、`..`、空路径段、符号链接和路径穿越。
- 版本锁定后不能覆盖或删除；删除整个站点恢复一个站点额度，删除单个版本不恢复。
- 访问密码只授权浏览，不授予源码下载或模板复用；Token 明文只显示一次。
- CORS 只作用于 API/OpenAPI，iframe 是否可嵌入由 CSP Embed Policy 单独控制。
- 泛域名模式需要 DNS wildcard、TLS 证书和同一反向代理；反代要转发 WebSocket。细节见 [deploy/APP_URL_MODE.md](deploy/APP_URL_MODE.md)。
- 不要重新引入公开 API 文档导航；后台入口是 `/admin?tab=apiDocs`，机器入口是 `/openapi.json`。
- 不要实现短链接分享，除非用户明确要求。

## 变更后检查

按变更范围运行最小检查：

- Go API/deploy/store：`go test -count=1 ./internal/... ./cmd/...`。
- 前端：对应目录 `npm run typecheck`、`npm run build` 和相关 `node --test`。
- Skill：`py_compile`、Skill 测试和 `build_skill_zip.py`。
- URL/代理：验证当前应用和 `/versions/N/` 历史应用；非标准端口检查 `Host`/`X-Forwarded-Host`。
- 数据迁移：`legacy-upgrade-qa`，目标 Docker 主机再跑 `docker-upgrade-qa`。
- 文档：`git diff --check`，检查相对链接和示例中的环境变量是否仍存在。

## 相关文档

- [README.md](README.md)：项目入口。
- [docs/GETTING_STARTED.md](docs/GETTING_STARTED.md)：第一次运行和发布。
- [docs/CONFIGURATION.md](docs/CONFIGURATION.md)：配置参考。
- [docs/API_INTEGRATION.md](docs/API_INTEGRATION.md)：API/CLI/Skill/MCP。
- [docs/OPERATIONS.md](docs/OPERATIONS.md)：生产运维。
- [docs/CURRENT_STATUS_AND_TODO.md](docs/CURRENT_STATUS_AND_TODO.md)：当前状态和外部验证风险。
