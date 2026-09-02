# PagePilot 当前状态与待办

更新时间：2026-09-03
版本：`0.3.1`

本文档是当前实现的简要状态，不是产品规划书。功能是否可用以代码、自动化测试和运行中的 `/openapi.json` 为准；历史决策和旧阶段记录请看 [CODEX_HANDOFF.md](CODEX_HANDOFF.md) 与 [PAGEPILOT_REMEDIATION_PLAN.md](PAGEPILOT_REMEDIATION_PLAN.md)。

## 已落地能力

### 发布和内容

- 支持单 HTML、Markdown、ZIP 和多文件静态站点。
- `POST /api/deploy` 支持 JSON 和 `multipart/form-data`；覆盖未锁定版本也支持 multipart。
- ZIP 会剥离单一外层目录、识别入口并拒绝路径穿越、符号链接、重复路径、多个独立根目录和大小超限。
- Bundle 元数据类型为 `single_html`、`markdown`、`zip_site`、`static_site`，详情接口和后台展示入口、根目录、文件树、大小、文件数和安全模式。
- Markdown 支持 GFM、相对图片、代码高亮、KaTeX、Mermaid、渲染缓存和严格 CSP nonce。
- 上传、覆盖、删除、下载和预览按版本记录的存储归属执行；本地文件和阿里云 OSS 均有适配入口。

### 版本、市场和额度

- 每个站点有稳定 `code`，版本可追加、覆盖、锁定、解锁、下线、切换当前或删除。
- 创作市场支持搜索、分类、点赞、收藏、详情、二维码、文件树和模板复用。
- `public` 进入市场，`unlisted` 仅链接访问；匿名发布只能使用 `unlisted`。
- 新建站点占用一个应用额度；追加版本、隐藏、下线和锁定不额外占用。
- 删除整个站点立即恢复一个应用额度；删除单个版本不会恢复站点额度。
- 访问密码票据有效 5 分钟并绑定版本；访问密码不等于源码下载或模板复用权限。

### Agent 和设备

- Go CLI 对外命令为 `pagep`，旧 `hostctl` 为兼容别名。
- Python `pagep` Skill 支持本地 preflight、发布、追加、覆盖、Token、市场、管理员和屏幕命令。
- `pagep-mcp` 提供部署、版本、市场、访问策略、站点详情、审计和屏幕工具。
- Android 屏幕端使用 Device Token + WebSocket，支持配对、manifest、投放、刷新、截图、休眠、唤醒和软关机。
- Skill 固定下载地址为 `/skill/pagep.zip`，后台上传包优先，旧下载路径兼容。

### 安全和可运维性

- 浏览器前台/后台使用 HttpOnly、SameSite=Lax Cookie；CLI/Skill/MCP 使用 Bearer Token。
- 生产环境要求固定 `HOSTCTL_MASTER_KEY` 和管理员认证；关键认证、发布、版本、站点、Token、审计、屏幕和安全策略动作会写入审计日志。
- CORS 只作用于 API/OpenAPI，iframe 嵌入由 CSP `frame-ancestors` 策略单独控制。
- 路径、泛域名、双模式均返回显式 URL 变体；域名历史版本地址为 `https://{code}.{suffix}/versions/{version}/`。
- 提供 Docker、systemd、Caddy、备份、运行时 QA、视觉 QA 和旧库升级演练脚本。

## 验证范围

本地稳定检查：

~~~bash
go test -count=1 ./cmd/... ./internal/...
(cd frontend/user && npm run typecheck && npm run build)
(cd frontend/admin && npm run typecheck && npm run build)
node --test frontend/user/scripts/*.test.mjs
node --test frontend/admin/scripts/*.test.mjs
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py skill/hostctl-deploy/scripts/pagep.py
python skill/hostctl-deploy/scripts/hostctl_deploy_test.py
~~~

生产近似检查：

~~~bash
node scripts/runtime-qa.mjs
node scripts/visual-qa.mjs
node scripts/legacy-upgrade-qa.mjs
node scripts/docker-upgrade-qa.mjs
~~~

`runtime-qa.mjs` 覆盖真实 HTTP + SQLite 的认证、匿名认领、发布、ZIP、Markdown、市场、访问密码、源码权限、版本、Token、配置、审计、CORS、屏幕和 Skill ZIP。`visual-qa.mjs` 覆盖前台、后台、市场详情、Bundle 文件树、复用弹窗、加密作品提示和桌面/移动端布局。`legacy-upgrade-qa.mjs` 不依赖 Docker；`docker-upgrade-qa.mjs` 需要 Docker Compose 和 Go，必须在目标服务器再跑一次。

## 当前剩余风险

这些不是“未实现的菜单”，而是上线前应继续验证的外部条件：

1. 使用真实 DNS provider、泛域名 TLS 证书、Nginx/Caddy 和非标准端口验证 path/domain/dual 三种 URL。
2. 使用真实阿里云 OSS 验证新旧版本混合存储、对象丢失回退、备份恢复和权限策略。
3. 使用真实 SMTP 验证邮箱验证码发送、过期、重发和失败提示。
4. 使用生产级数据量复核市场、审计、文件树和长文本布局。
5. 继续扩充复杂 ZIP、Markdown/KaTeX/Mermaid、安全模式和上传内容的 XSS/CSP 回归样例。
6. 在目标 Docker 主机用真实旧数据库和 hosted 目录做一次升级与回滚演练。

## 文档和实现对齐规则

- API 变化同时更新类型、OpenAPI、[API_INTEGRATION.md](API_INTEGRATION.md) 和客户端示例。
- 环境变量变化同时更新 [CONFIGURATION.md](CONFIGURATION.md)、[.env.example](../.env.example) 和部署模板。
- URL/代理行为变化同时更新 [../deploy/APP_URL_MODE.md](../deploy/APP_URL_MODE.md)，并验证当前和历史版本。
- 重新构建服务端前先刷新前端产物和内嵌 Skill ZIP，避免源码与生产 bundle 不一致。
