# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

**PagePilot (hostctl)** 是一个 Agent 优先的静态站点发布平台 (v0.2.0)，让普通用户和 AI Agent 都能发布单文件 HTML、Markdown、ZIP 或多文件静态站点，并通过 Go 服务统一管理版本、加密访问、锁定、回滚、令牌、管理员操作和市场浏览。同时支持硬件屏幕绑定与投放（Android 屏幕 APP 通过 X5 WebView 播放 PagePilot 应用）。

## 技术栈

| 层次 | 技术 |
|---|---|
| 后端服务 | Go 1.22，`modernc.org/sqlite`（纯 Go SQLite，无 CGO） |
| 前端用户端 | React 18 + TypeScript + Vite 6 + Ant Design 6 + Lucide 图标 |
| 前端管理端 | React 18 + TypeScript + Vite 6 + Lucide 图标 |
| CLI | Go，`spf13/cobra`（二进制名 `pagep`，兼容旧名 `hostctl`） |
| MCP 服务器 | Go，`cmd/hostctl-mcp`（JSON-RPC stdio 2.0，无外部依赖） |
| Agent 技能 | Python 脚本，仅依赖标准库 |
| 存储 | SQLite（WAL 模式）；文件系统或 Aliyun OSS |
| 容器化 | Docker 三阶段构建（Node → Go → Alpine） |
| Android 屏幕端 | Kotlin + X5 WebView（`apps/screen-app/`） |

## 常用命令

### 构建

```bash
make build                # 完整构建: skill-zip + 前端(user+admin) + Go 二进制
make frontend             # 仅构建前端（user + admin）
make build-linux          # 交叉编译 Linux amd64 二进制（CGO_ENABLED=0）
make tidy                 # go mod tidy
```

### 开发

```bash
make run                  # 构建并启动本地开发服务器 (127.0.0.1:8787)
                          # 等价于: HOSTCTL_DEV=1 ./bin/hostctl-server --addr 127.0.0.1:8787
```

访问地址: 用户应用 `http://localhost:8787/` | 管理后台 `/admin` | OpenAPI `/openapi.json`

### 测试

```bash
# Go 单元/集成测试（提交前必跑）
go test -count=1 ./cmd/... ./internal/... ./apps/...

# 指定包或函数
go test -count=1 ./internal/api/...
go test -run TestScreen ./internal/api/...

# 前端单元测试（Node.js node:test）
node frontend/user/scripts/previewSandbox.test.mjs
node frontend/user/scripts/templateReuse.test.mjs
node frontend/admin/scripts/auditFilters.test.mjs
node frontend/admin/scripts/deviceInfo.test.mjs
node frontend/admin/scripts/deployUpload.test.mjs
node frontend/admin/scripts/previewSandbox.test.mjs

# 前端构建验证
npm run build --prefix frontend/user
npm run build --prefix frontend/admin

# Skill Python 测试
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py skill/hostctl-deploy/scripts/pagep.py
python skill/hostctl-deploy/scripts/hostctl_deploy_test.py

# Skill 冒烟测试（需要本地 dev 服务器在 127.0.0.1:8787 运行）
python test_skill.py
```

> **注意**: 不要使用 `go test ./...`，因为本地 `竞品/` 参考目录包含嵌套 Go 文件，会导致无效导入路径错误。始终使用 `go test -count=1 ./cmd/... ./internal/... ./apps/...`。

### 代码质量

```bash
make fmt                  # gofmt -w .
make vet                  # go vet ./...
```

### QA

```bash
make runtime-qa           # Node.js 运行时冒烟测试（临时服务器+数据库）
make visual-qa            # Playwright 浏览器可视化 QA
make legacy-upgrade-qa    # 旧 SQLite 数据库升级演练
make docker-upgrade-qa    # Docker Compose 升级演练
make docker               # docker build -t hostctl:latest .
make clean                # rm -rf bin/ data/
```

## 架构概要

### 服务器启动流程

`cmd/hostctl-server/main.go` → `config.Default()` → `store.NewSQLiteStore()`（自动执行 `schema.sql` + 增量迁移） → `auth.New()` + `EnsureBootstrapAdmin()` → `deploy.New()` → `deployer.LoadPersistedSettings()` → `api.New()` → 启动 HTTP 监听。

### HTTP API 分层 (`internal/api/`)

`server.go` 是核心路由文件（~212KB），包含所有路由 handler，使用 Go 1.22 增强路由语法（`"POST /api/deploy"`）。通过 `DeployerPort` 接口解耦 HTTP 层与部署逻辑。

| 包文件 | 职责 |
|---|---|
| `server.go` | 路由注册、中间件、CORS、请求日志、所有 handler |
| `types.go` | 请求/响应 Go struct |
| `openapi.go` | 自动生成 OpenAPI JSON（`/openapi.json`） |
| `errors.go` | 结构化错误响应（`errorCode`、`stage`、`hint`） |
| `screen.go` / `screen_types.go` | 屏幕绑定/投放/截图/指令 |
| `admin_types.go` / `token_types.go` / `version_types.go` | 管理员/令牌/版本 API 类型 |
| `app_url.go` | 应用 URL 生成（路径模式 vs 泛域名模式） |

### 数据访问层 (`internal/store/`)

`Store` 接口 + `SQLiteStore` 实现。SQLite WAL 模式，busy timeout，外键约束，单连接池。Schema 通过嵌入的 `schema.sql` + 增量迁移函数管理。FTS5 虚拟表支持市场全文搜索。

### 部署引擎 (`internal/deploy/`)

支持单 HTML、单 Markdown、多文件上传、ZIP 解压。流水线：验证 → SHA256 → 创建版本 → 更新 `current` 软链 → 写磁盘。包含冷却时间控制、版本管理（锁定/回滚/切换/覆盖/删除）、屏幕部署、路径验证、二维码生成、内容下载。存储后端支持本地文件或 Aliyun OSS。

### 前端构建管线

1. `frontend/user/` 和 `frontend/admin/` 是独立 Vite + React 工程
2. 构建产物输出到 `internal/web/user/app/` 和 `internal/web/admin/app/`
3. Go 服务通过 `//go:embed` 将前端产物嵌入二进制 → 单二进制部署
4. 两个 SPA 都是大型单组件应用，使用 `useState` 页面切换（无路由库）

### 身份认证模型

| 身份 | 认证方式 | 权限范围 |
|---|---|---|
| 注册用户 | `Authorization: Bearer <token>`（SHA-256 哈希存储） | 部署、令牌管理、屏幕绑定 |
| 匿名会话 | `X-Hostctl-Session` header 或 `hostctl_anon_session` cookie | 匿名配额内发布（可认领） |
| 管理员 | 管理会话 cookie（`/admin` 登录） | 全站管理 |

### 应用 URL 模式

- **路径模式** (默认): `https://host/agent/{code}/`
- **泛域名模式**: `https://{code}.pagepilot.example.com/`（需 DNS 泛解析 + SSL）
- **双模式**: 同时支持，主链接按路径模式生成

### 屏幕投放子系统

硬件屏幕设备绑定是核心差异化功能：

```
屏幕 APP (Android/X5 WebView)
    ↕ Device Token (可吊销)
PagePilot 服务器
    ↕ Bearer Token (注册用户)
用户后台 / CLI / MCP / Skill
```

流程: 屏幕创建配对码(5min) → 用户绑定 → 部署应用 → 投放到屏幕 → 屏幕通过 manifest 加载 → 后台下发指令（截图/刷新/休眠/唤醒/关机）

## 兼容性规则

- `/skill/pagep.zip` 是主要 Skill 下载路径，`/skill/hostctl-deploy.zip` 保持兼容别名
- 保持旧部署 API 兼容（现有 Skill/CLI/MCP 依赖）
- 新 UI/文档使用 PagePilot 和 `pagep` 命名
- 不要重新引入公开 `/api-docs.html` 导航；文档在管理后台内
- 不要实现短链接分享功能（除非明确要求）
- 默认发布可见性为 `unlisted`

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `HOSTCTL_HTTP_ADDR` | `0.0.0.0:8787` | 监听地址 |
| `HOSTCTL_HOSTED_DIR` | `/var/www/hosted` | 静态文件目录 |
| `HOSTCTL_DB_PATH` | `hostctl.db` | SQLite 数据库路径 |
| `HOSTCTL_PUBLIC_BASE_URL` | `http://localhost:8787` | 公网基础 URL |
| `HOSTCTL_APP_URL_MODE` | `path` | URL 模式 (`path`/`domain`/`dual`) |
| `HOSTCTL_APP_DOMAIN_SUFFIX` | — | 泛域名后缀 |
| `HOSTCTL_APP_URL_SCHEME` | `http` | URL 协议 |
| `HOSTCTL_APP_URL_PORT` | — | URL 端口 |
| `HOSTCTL_STORAGE_BACKEND` | `local` | 存储后端 (`local`/`oss`) |
| `HOSTCTL_COOLDOWN_SECONDS` | `10` | 部署冷却时间(秒) |
| `REQUIRE_AUTH` | `false` | 生产模式：写操作要求认证 |
| `HOSTCTL_ADMIN_USERNAME` | `admin` | 首个管理员用户名 |
| `HOSTCTL_ADMIN_PASSWORD` | `123456` | 首个管理员密码(首次登录后修改) |

OSS 存储相关: `HOSTCTL_OSS_ENDPOINT`, `HOSTCTL_OSS_BUCKET`, `HOSTCTL_OSS_ACCESS_KEY_ID`, `HOSTCTL_OSS_ACCESS_KEY_SECRET`, `HOSTCTL_OSS_PREFIX`, `HOSTCTL_OSS_PUBLIC_BASE_URL`

| `HOSTCTL_MASTER_KEY` | (dev 模式自动生成) | AES-256 主密钥，用于加密站点访问密码明文。生产环境必须 base64 编码 32 字节 |

## 限制与安全

- 单文件上限 1 MiB，整站上限 10 MiB，单站点文件数上限 100
- 描述必填，上限 240 字符
- 路径拒绝绝对路径和 `..`
- 版本锁定后不可覆盖/删除
- Token 只保存 bcrypt 哈希，过期自动拒绝
- **密码强度**：用户密码至少 8 字符、含字母和数字；站点访问密码至少 8 字符
- **登录限速**：5 次/5 分钟内失败锁定 15 分钟
- **Cookie Secure**：HTTPS 模式下所有 cookie 自动设置 `Secure` 标志
- 访问密码票据仅存 HttpOnly Cookie，5 分钟有效期
- 管理员可查看站点访问密码明文（通过 `POST /api/admin/sites/{code}/access/reveal`），审计日志记录 reveal 操作
- 托管内容响应自带 CSP `sandbox`（不含 `allow-same-origin`）
- 泛域名模式提供最强的应用/平台隔离
- **生产部署安全**：必须配置 `HOSTCTL_MASTER_KEY` 作为 AES 主密钥，否则拒绝启动

## 存储布局

```
/var/www/hosted/
  {code}/
    current → versions/3          # 软链指向当前版本
    versions/
      1/ 2/ 3/                    # 每个版本包含完整文件
```

SQLite 存储所有元数据（站点、版本、文件、令牌、点赞、屏幕、设置），静态文件直存磁盘或 OSS。
