# PagePilot 当前状态与待办

更新时间：2026-07-06

本文档按当前代码状态整理，区分“底层能力已具备”和“产品化仍待补齐”。它用于避免把计划文档里的目标误读成已经上线的功能。

## 已落地

- 发布链路支持单 HTML、Markdown、ZIP 和多文件静态站点。
- `POST /api/deploy` 同时支持 JSON 和 `multipart/form-data`，CLI、MCP、Skill 发布本地文件、目录或 ZIP 时优先走 multipart。
- ZIP 入口识别已拆到 `internal/bundle`，支持剥离单一外层目录、识别 HTML/Markdown 入口、拒绝路径穿越和疑似批量包。
- Markdown 托管渲染已拆到 `internal/render`，支持相对图片、表格、任务列表、代码块、Mermaid/数学公式语义块和渲染缓存。
- SQLite schema 已增加 `site_search_fts`、`audit_logs`、`render_cache`、`version_bundles`。
- 创作市场搜索已接入 SQLite FTS5，并保留中文 `LIKE` 回退和老数据启动回填。
- 访问密码查看票据已收紧为 5 分钟，匿名访客也可以输入访问密码查看加密站点。
- 后台运行设置已区分主站跟随当前访问域名和应用泛域名规则；主站不再需要配置固定 Public Base URL。
- Skill 下载包改为后台上传维护固定 ZIP，默认 `/skill/pagep.zip`，旧 `/skill/hostctl-deploy.zip` 保留兼容。
- 屏幕端控制链路已使用 Device Token 和 WebSocket，支持投放、刷新、截图指令、休眠、唤醒和软关机指令。

## jpage 对照结论

本地竞品目录 `竞品/jpage` 的实现可借鉴，但不能照搬：

- Skill 规约：jpage 的 `skills/jpage/SKILL.md` 强约束 CLI 优先、MCP 兜底、大文件/ZIP 不走模型 base64 流、内容生成后必须上传并返回链接。PagePilot 的 `pagep` Skill 已按这个思路中文化，但保持 PagePilot 自己的工具名、权限和 URL 规则。
- Skill 分发：jpage 会扫描 `skills/*/SKILL.md`、解析 frontmatter，并支持实时 ZIP 下载；PagePilot 现在选择固定 `/skill/pagep.zip`，后台只上传维护 ZIP，不在后台编辑源文件。这个产品取舍目前保持不变。
- Markdown 渲染：jpage 服务端用 `marked`、`marked-highlight`、`highlight.js` 和 `katex.renderToString` 处理 Markdown/代码/公式；Mermaid 不是服务端 SVG 预渲染，而是服务端输出 `<pre class="mermaid">`，模板加载同源 Mermaid 运行时并用 CSP nonce 在浏览器端绘图。
- ZIP Bundle：jpage 会区分网站包和批量文件，识别 `index.html`、嵌套入口和 Markdown 入口，Bundle 渲染时注入 `<base>`。PagePilot 已支持多文件/ZIP 和入口识别，但市场详情、后台详情和错误提示还需要继续产品化。
- MCP/CLI 对称：jpage 的 CLI 与 MCP 基本围绕同一套 REST API，但部分 MCP URL 仍按 `protocol/host/port` 拼接。PagePilot 不应照搬这一点，发布和投屏链接必须以服务端返回为准。

## 待产品化

1. 后台审计日志 API/UI

   现状：`audit_logs` 表、`store.AuditLog`、`RecordAuditLog`、`ListAuditLogs` 和 store 测试已存在。

   待办：补齐 `/api/admin/audit-logs` 路由、OpenAPI 描述、权限校验、后台审计日志页面或 tab，并把发布、下架、删除、加密、投屏、Token、配置修改等关键动作实际写入审计日志。

2. 文件树、Bundle 类型、安全模式和复用参数展示

   现状：`version_bundles` 表和 `VersionBundle` store 接口已存在，ZIP 分析可生成入口、根目录、文件树和安全模式元数据。

   待办：在创作市场详情和后台站点详情中完整展示文件树、Bundle 类型、主入口、安全模式、入口识别提示和复用参数；后台站点列表当前仍主要是站点摘要字段，没有完整详情抽屉/API。

3. Markdown 高级渲染链路

   现状：当前是安全语义渲染和缓存：代码块有 language class，Mermaid/数学公式保留为语义容器，Markdown CSP 更严格，不执行脚本。

   待办：如果要达到 jpage 那种体验，需要引入可维护的 Markdown 高级链路：服务端完成 Markdown 解析、highlight.js 代码高亮和 KaTeX `renderToString` 公式渲染；Mermaid 则由平台内置前端运行时和 nonce 初始化脚本在浏览器端绘图，而不是让用户自己上传 Mermaid 脚本。实现时还要重新审查 CSP、缓存键和 XSS 边界。

4. 创作市场“模板复用”体验

   现状：前台已有基础“使用/复用”抽屉，可给出源码下载、CLI 命令和 Agent/MCP 提示词。

   待办：按 jpage 思路补齐模板市场级体验：下载源文件结构说明、文件树预览、Agent 复用指南、CLI/MCP 参数生成、复用后的新建/更新边界提示、权限/加密状态下的可下载策略。

5. 运行时视觉 QA

   现状：已运行前后台构建和自动化测试，但没有完成一次覆盖所有页面、桌面/移动视口、实际数据状态的视觉 QA。

   待办：用真实服务检查首页、创作市场列表、详情、手动部署、Skill & MCP、Screens、登录注册、加密访问、后台各 tab；重点看中文截断、横向滚动、按钮溢出、预览卡片加载、空状态和错误状态。

6. Docker 老数据库升级验证

   现状：迁移代码和测试覆盖了部分老库字段补齐，文档说明只要保留挂载目录不应丢数据。

   待办：拿一份旧版本真实 SQLite 数据库和 hosted 文件目录，在 Docker 环境执行 `docker compose up -d --build` 升级，确认站点、版本、用户、Token、访问密码、分类、屏幕绑定、文件资源、FTS 回填和新增表迁移都正常。

7. OSS、SMTP 和安全专项验证

   现状：代码和单元测试覆盖了 OSS 适配、邮箱验证基础流程、访问密码票据、ZIP 路径防护等核心逻辑。

   待办：用真实阿里云 OSS、真实 SMTP、生产反向代理、泛域名/路径模式、CORS/iframe 嵌入策略做端到端验证；同时复查用户上传 JS、HTML sandbox/CSP、源码下载权限和匿名认领边界。

## 建议优先级

1. 先做后台审计日志 API/UI，因为这会影响发布平台的可追踪性和上线信心。
2. 再做文件树、Bundle 类型、安全模式和复用参数展示，让多文件/ZIP 站点具备可解释性。
3. 然后完善创作市场模板复用体验，打通“发现 -> 下载/复用 -> Agent 二次创作 -> 新发布”的闭环。
4. Markdown 高级渲染单独作为安全专项推进，避免为了效果引入新的 XSS/CSP 风险。
5. 最后做完整视觉 QA 和旧库 Docker 升级验证，作为发布前准入项。
