# PagePilot 当前状态与待办

更新时间：2026-07-07

本文档按当前代码状态整理，区分“底层能力已具备”和“产品化仍待补齐”。它用于避免把计划文档里的目标误读成已经上线的功能。

## 2026-07-07 验证补充

- 已用临时独立 SQLite 数据库和真实 HTTP 服务完成一轮 Playwright 深度 QA：覆盖 `/admin?mode=register`、后台登录后全部 tab、`/market`、`/market/{code}` 详情直达、模板复用弹窗、ZIP 相对资源执行、Markdown KaTeX / Mermaid / 表格 / 代码高亮、匿名输入访问密码查看加密站点、普通访客下载加密站点源码受限。
- 已修复 QA 暴露的后台注册入口问题：`/admin?mode=register` 现在会直接打开注册模式。
- 已修复创作市场详情和模板复用弹窗的视觉问题：右侧详情栏改为可滚动纵向栏，模板复用弹窗改为清晰的两种模式 + 两列步骤布局，长命令独立横向滚动。
- 已补旧库迁移回归测试：模拟旧 `audit_logs` 缺少 `actor_role` / `result` 字段的 SQLite 数据库，验证新版本启动会自动补列、保留旧日志，并避免旧库启动时报 `no such column: result`。同时新增生产近似旧库组合测试，把旧站点、版本、文件、用户、Token、匿名 session、屏幕和审计日志放在同一个旧库里升级，验证新字段补齐、旧业务数据保留、FTS 回填和新增表创建。
- 已补 MCP 工具清单回归测试：锁住 `set_site_reuse_policy`、`set_site_security_mode`、`get_admin_site_detail`、`query_audit_logs`、源码读取和屏幕工具入口，并验证无效复用策略 / 安全模式会在本地参数校验阶段被拒绝。
- 已收紧 CORS 边界：后台配置的 `CORSAllowOrigins` 只作用于 `/api/*` 和 `/openapi.json`，用于外部网站调用 PagePilot API；托管应用内容不再继承 API CORS 白名单。外部 iframe 是否能嵌入应用由嵌入策略 / `frame-ancestors` 控制，二者不混用。
- 已新增可复现运行时 QA：`node scripts/runtime-qa.mjs` / `make runtime-qa` 会自动编译临时服务、启动临时 SQLite 和 hosted 目录，验证注册成功 / 失败、管理员登录、账号改密、登出、Token 创建和吊销、匿名发布显式认领与归属迁移、已认领匿名 session 拒绝继续发布、Markdown 高级渲染、真实 ZIP 站点发布、ZIP Bundle 详情、ZIP 相对资源访问、市场详情、后台 Bundle 文件树、登录用户公开未加密站点源码下载及 `source_download` 成功审计、匿名源码下载登录提示、访问密码 Cookie 不能下载加密站点源码、站点所有者和管理员可下载加密站点源码及 `source_download` 审计、加密访问、访问密码票据绑定版本和切换当前版本后的失效、模板复用来源记录、屏幕绑定 / 投放 / 截图 / 指令 / 解绑、认证 / 账号 / 匿名认领 / 访问密码 / 站点管理 / 版本锁定 / 下架 / 切换当前 / 覆盖 / 删除 / Token 管理 / 运行设置 / 市场分类 / Skill 包上传 / 用户管理 / 屏幕操作审计日志、审计分页与用户 / 站点 / 动作 / 角色 / 时间 / 详情关键字过滤、传统 CSP report 与 Reporting API 审计、CORS 边界、OpenAPI 和 Skill ZIP 下载。
- 已新增真实浏览器视觉 QA：`node scripts/visual-qa.mjs` / `make visual-qa` 会启动临时服务和真实浏览器，覆盖首页、创作市场、市场详情、手动部署、Skill/MCP、屏幕介绍、HTML/Markdown 运行页、加密访问页、后台主要 tab、审计日志筛选和翻页的桌面/移动端横向溢出、空白页和浏览器错误。该脚本会发布多文件站点、加密站点和超过一页的市场批量样本，并检查创作市场加载更多、市场详情 Bundle 信息、折叠文件树、模板复用弹窗、后台站点详情完整文件树和复用参数，以及加密作品的源码下载 / 模板复用受限提示。该脚本本轮暴露并修复了创作市场移动端溢出、Skill/MCP 内容页移动端溢出、后台移动端布局溢出，以及 Markdown nonce 页面缓存导致的 CSP 误拦截。
- 已新增旧库升级演练 QA：`node scripts/legacy-upgrade-qa.mjs` / `make legacy-upgrade-qa` 会构造旧 SQLite + hosted 目录，覆盖公开站点、加密站点、旧管理员、Token、匿名 session、屏幕绑定、审计日志和托管文件；随后启动当前服务触发迁移，并通过后台 API、创作市场搜索、`/agent/{code}/`、访问密码和源码下载权限检查确认升级后仍可用。
- 已新增真实 Docker Compose 旧库升级演练脚本：`node scripts/docker-upgrade-qa.mjs` / `make docker-upgrade-qa` 会在临时目录构造旧库和 hosted，生成临时 compose override，执行 `docker compose up -d --build`，并通过容器 HTTP 接口和直接 SQLite 校验站点、版本、用户、Token、匿名 session、屏幕、访问密码、FTS、Bundle、审计表和 Skill ZIP。该脚本需要服务器具备 Docker Compose 和 Go。
- 已补前后台部署错误面板：手动部署页和后台发布页会展示结构化错误阶段、稳定错误码、服务端 `hint`、本地排查建议和可复制诊断信息；ZIP/Bundle 入口失败不再只显示一行原始错误。真实 `/api/deploy` 链路已由 `runtime-qa` 验证多入口、缺少入口和路径穿越会分别返回 `ZIP_AMBIGUOUS_ENTRY`、`ZIP_ENTRY_MISSING` 和 `ZIP_UNSAFE_PATH`。
- 已补 Markdown sanitizer 安全回归：编码后的 `javascript&#58;` URL、未加引号 / 单引号事件属性、SVG `data:` 图片 URL 都会被剔除，且不会误删正常相对图片和 HTTPS 图片；`runtime-qa` 已把这些危险内容加入真实服务输出检查。
- 已补认证、匿名认领、访问密码验证、源码下载、账号密码修改、管理动作和屏幕操作审计：注册 / 登录 / 登出会记录 `auth.register`、`auth.login`、`auth.logout`，匿名发布显式认领会记录 `anonymous.claim`，加密站点密码输入成功 / 失败会记录 `site.access_login`，源码下载成功 / 失败会记录 `source_download`，当前登录用户修改自己的账号密码成功 / 失败会记录 `account.password`，运行设置、市场分类、Skill 包上传和用户创建 / 更新 / 删除会记录 `config.update`、`config.market_categories`、`skill.package_upload`、`user.create`、`user.update`、`user.delete`，屏幕绑定、投放、截图请求、指令请求和解绑会记录 `screen.bind`、`screen.publish`、`screen.screenshot.request`、`screen.command.request`、`screen.unbind`，均只保留站点、版本、用户、屏幕、请求 ID、结果和失败原因等非敏感信息，不记录密码明文；后台审计动作预设已对齐后端真实落库 action，并用中文标签展示；`runtime-qa` 已验证上述成功链路和关键失败链路都可在后台审计日志查到，并断言不会泄露明文密码或 Token。
- 当前 Windows 本机没有 Docker CLI，未能在本机执行真实 `docker compose up -d --build` 老库升级验证；上线前请在服务器运行 `node scripts/docker-upgrade-qa.mjs`，并额外用真实旧 `hostctl.db` 和 hosted 目录做一次容器升级演练。

## 已落地

- 发布链路支持单 HTML、Markdown、ZIP 和多文件静态站点。
- `POST /api/deploy` 同时支持 JSON 和 `multipart/form-data`；`PATCH /api/deploys/{code}/versions/{version}` 覆盖版本也支持 `multipart/form-data`。CLI 和 Skill 发布、追加、覆盖本地文件、目录或 ZIP 时优先走 multipart，MCP 发布/追加同样走 multipart。
- ZIP 入口识别已拆到 `internal/bundle`，支持剥离单一外层目录、识别 HTML/Markdown 入口、拒绝路径穿越和疑似批量包；失败会返回 `stage=zip_bundle`、稳定 Bundle 错误码和面向用户的 `hint`，前后台部署页已把这些结构化信息展示为可操作的错误面板，`runtime-qa` 已锁定真实部署接口的 `ZIP_AMBIGUOUS_ENTRY` / `ZIP_ENTRY_MISSING` / `ZIP_UNSAFE_PATH` 返回。
- 后台“发布应用”已与手动部署页对齐：单文件上传接受 HTML / Markdown / ZIP，单 ZIP 会自动进入多文件发布并交由服务端 Bundle 识别；多文件本地校验接受 HTML、Markdown 或单 ZIP，避免在前端误拦截合法 ZIP 包。
- 新建发布和覆盖版本都会写入 `version_bundles`，Bundle `kind` 稳定为 `single_html`、`markdown`、`zip_site`、`static_site`，详情接口会返回中文类型、入口、ZIP 根目录、文件树、入口说明和安全模式；旧数据缺少 Bundle 元数据时会按入口和文件数兜底推断。
- Markdown 托管渲染已拆到 `internal/render`，支持相对图片、表格、任务列表、删除线、自动链接、代码块高亮、行内数学公式、单行 / 多行块级数学公式、Mermaid 图表、同源 KaTeX / Mermaid runtime、内置 KaTeX 字体、内置样式和初始化脚本 nonce、严格脚本 CSP 和渲染缓存。
- SQLite schema 已增加 `site_search_fts`、`audit_logs`、`render_cache`、`version_bundles`。
- 创作市场搜索已接入 SQLite FTS5，并保留中文 `LIKE` 回退和老数据启动回填。
- 访问密码查看票据已收紧为 5 分钟，并绑定站点密码哈希和目标版本；匿名访客也可以输入访问密码查看加密站点，站点改密码或切换当前版本后旧票据会失效。
- 后台运行设置已区分主站跟随当前访问域名和应用泛域名规则；主站不再需要配置固定 Public Base URL。
- Skill 下载包改为后台上传维护固定 ZIP，默认 `/skill/pagep.zip`，旧 `/skill/hostctl-deploy.zip` 保留兼容；本地 `make build` / `make docker` 和直接 `docker compose up -d --build` 都会在编译前重新生成内置 Skill ZIP，避免源码改了但 embed 包仍是旧版本。
- `pagep` Skill 发布和追加版本成功后会输出中文摘要，明确展示服务端返回的访问 URL、详情 URL、版本 URL，并继续保留 JSON 供自动化解析；Agent 不需要也不应该自行拼接应用链接。
- 屏幕端控制链路已使用 Device Token 和 WebSocket，支持投放、刷新、截图指令、休眠、唤醒和软关机指令。

## jpage 对照结论

本地竞品目录 `竞品/jpage` 的实现可借鉴，但不能照搬：

- Skill 规约：jpage 的 `skills/jpage/SKILL.md` 强约束 CLI 优先、MCP 兜底、大文件/ZIP 不走模型 base64 流、内容生成后必须上传并返回链接。PagePilot 的 `pagep` Skill 已按这个思路中文化，但保持 PagePilot 自己的工具名、权限和 URL 规则。
- Skill 分发：jpage 会扫描 `skills/*/SKILL.md`、解析 frontmatter，并支持实时 ZIP 下载；PagePilot 现在选择固定 `/skill/pagep.zip`，后台只上传维护 ZIP，不在后台编辑源文件。这个产品取舍目前保持不变。
- Markdown 渲染：jpage 服务端用 `marked`、`marked-highlight`、`highlight.js` 和 `katex.renderToString` 处理 Markdown/代码/公式；Mermaid 不是服务端 SVG 预渲染，而是服务端输出 `<pre class="mermaid">`，模板加载同源 Mermaid 运行时并用 CSP nonce 在浏览器端绘图。
- ZIP Bundle：jpage 会区分网站包和批量文件，识别 `index.html`、嵌套入口和 Markdown 入口，Bundle 渲染时注入 `<base>`。PagePilot 已支持多文件/ZIP、入口识别、稳定 Bundle 类型和详情展示；前台市场详情和后台站点详情的文件树已支持搜索、目录/文件统计、复制路径和复制 SHA，前后台发布失败会展示 ZIP/Bundle 阶段、错误码和修复建议。后续重点是更多真实 ZIP 包兼容 QA。
- MCP/CLI 对称：jpage 的 CLI 与 MCP 基本围绕同一套 REST API，但部分 MCP URL 仍按 `protocol/host/port` 拼接。PagePilot 不应照搬这一点，发布和投屏链接必须以服务端返回为准。

## 待产品化

1. 后台审计日志

   现状：`audit_logs` 表、`RecordAuditLog`、`ListAuditLogs`、`/api/admin/audit-logs`、OpenAPI 描述、管理员权限校验、后台“审计日志”页面、分页、关键字、用户下拉、站点下拉、动作预设、操作者类型、操作者 ID、角色、结果、对象类型、对象 ID 和时间过滤已具备；后台站点详情也会按当前 code 拉取最近 6 条审计摘要，便于就地追踪单个站点的发布、加密、可见性、复用和安全策略变化。关键字搜索覆盖动作、结果、操作者、角色、站点、对象、IP、UA 和详情 JSON，时间过滤按 RFC3339 严格校验。CLI / MCP / Skill 也能查询审计日志，CLI 普通输出会在摘要表后给出每条日志的 `User-Agent` 和 `Detail` JSON。发布成功和发布失败都会写入结构化审计详情，注册 / 登录 / 登出、运行配置更新、市场分类配置、Skill 包上传、访问密码设置、访问密码验证成功 / 失败、源码下载成功 / 失败、账号密码修改成功 / 失败、站点可见性设置、置顶、分类、标签、源码下载 / 模板复用策略、安全模式、主版本策略、版本锁定、版本状态、切换当前版本、覆盖版本、删除版本、删除站点、匿名认领、投屏、截屏请求、屏幕指令、屏幕解绑、Token 创建、Token 吊销和用户管理的失败路径都会写入对应 failed 日志；配置、用户、Token、站点、版本、匿名认领、Skill 包和屏幕等关键成功动作已有成功日志覆盖。`runtime-qa` 已在真实 HTTP + SQLite 链路里验证 `source_download` 成功和加密拒绝日志、`version.lock`、`version.status`、`version.current`、`version.overwrite`、`version.delete` 成功日志可按站点查到，普通用户删除自己的站点和管理员删除站点都会产生可按站点查到的 `site.delete` 成功日志且删除后不再出现在市场/后台详情，`token.create` / `token.revoke` 成功日志可按 token ID 查到，`anonymous.claim` 可按匿名 session 查到，`config.update` / `config.market_categories` 可按 config 对象查到，`skill.package_upload` 可按 Skill 包对象查到，`user.create` / `user.update` / `user.delete` 可按用户 ID 查到，传统 CSP report 与 Reporting API 产生的 `security.csp_report` 可按站点、动作和详情关键字查到，且不会泄露明文 token 或用户密码。托管 HTML / Markdown 的 CSP 违规报告会通过 `/api/security/csp-report` 归一化为 `security.csp_report` 审计日志，管理员可按动作、站点、IP、UA 和详情关键字排查被浏览器拦截的资源。

   待办：继续扩大失败日志覆盖面到更多低频管理动作；用生产级数据量复核分页、筛选和站点详情最近审计体验。当前 `runtime-qa` 已在真实 SQLite 服务上覆盖分页、站点、动作、操作者、角色、时间窗口和详情 JSON 关键字过滤。

2. 文件树、Bundle 类型、安全模式和复用参数展示

   现状：`version_bundles` 表和 `VersionBundle` store 接口已存在，新建发布和覆盖版本会写入稳定 Bundle 类型、入口、根目录、文件树和安全模式元数据；创作市场详情和后台站点详情已经展示 Bundle 类型、主入口、根目录、完整文件树、安全模式、入口识别说明和复用策略。文件树已提供路径 / 文件名 / SHA 搜索、目录数 / 文件数 / 总大小统计、复制路径和复制 SHA。前台已支持 `/market/{code-or-publicId}` 直达详情，刷新或分享链接后仍会重新拉取文件树与复用参数。站点级 `securityMode` 支持 `auto / strict / compatible / trusted`，后台可设置，CLI / MCP / Skill 可查询和调整，`/agent/{code}` HTML 运行时会按生效模式设置 CSP/sandbox。

   待办：继续打磨不同安全模式下的真实站点兼容性 QA，并补充更多复杂 ZIP 包、空状态和长路径错误状态样例。

3. Markdown 高级渲染链路

   现状：已接入 Go 侧 Markdown 解析、GFM、Chroma 高亮 HTML、行内 `$...$`、单行块级 `$$E=mc^2$$`、多行块级 `$$ ... $$`、`mermaid` / `katex` 代码块识别、同源 KaTeX / Mermaid runtime、内置 KaTeX 字体、内置样式和初始化脚本 nonce、严格脚本 CSP（nonce-only `script-src`，不依赖 `script-src 'self'`，不允许 `script-src 'unsafe-inline'` / `unsafe-eval`）和包含 `code / version / entry / contentSHA / theme / rendererVersion` 的缓存键。KaTeX / Mermaid 需要的运行时样式通过受控的 `style-src-elem` / `style-src-attr` 放行，不扩大脚本执行面。Markdown 页面支持 `?theme=auto|light|dark`，公式和图表由浏览器端平台 runtime 渲染，不要求 Agent 自行打包这些库。已补充回归：代码 span / 普通代码块中的 `$HOME$` 不会被误转成公式，带参数 info string 的 `mermaid title=...` 和 `katex display` 也会识别为平台渲染块。Markdown HTML sanitizer 会在最终输出前清理编码后的 active URL、事件属性、危险 `data:` 图片、不安全 `srcset` 候选和 `xlink:href` 等命名空间 URL 属性，并保留安全相对资源。

   待办：继续扩充更多公式 / Mermaid 复杂语法、Markdown 相对附件下载策略和真实站点 XSS/CSP 专项复查；真实浏览器 QA 已覆盖 nonce 缓存边界并固定为脚本，后续需要持续扩充样例。

4. 创作市场“模板复用”体验

   现状：前台已有“使用/复用”抽屉，可给出源码下载、CLI 命令、Agent 指令和独立 MCP 参数；抽屉已展示源文件结构摘要，包括 Bundle 类型、入口、根目录、下载形态、文件数量、总大小和前几个文件路径，并区分“新建二创”和“更新已有发布”。更新模式自动带入当前作品 code，只有所有者或管理员可复制追加版本语义的 CLI / Agent 指令。详情 API 已返回 `allowDownload`、`allowReuse`、`policyNote`、`templateSourceCode` 和 `templateSourceVersion`，并区分访问密码浏览权限和源码下载 / 模板复用权限；源码下载需要登录用户或已绑定注册用户的 Token，匿名点击下载只给友好登录提示。发布 API、CLI、MCP 和 Skill 支持 `templateSourceCode` / `templateSourceVersion`，复用后会记录来源站点、来源版本并增加来源作品的 `reuseCount`。后台站点详情已支持管理员设置源码下载和模板复用策略；加密作品对普通用户默认不提供源码下载，但站点所有者和管理员可直接下载用于审计、备份或二次修改。

   待办：继续打磨模板市场级体验的视觉 QA 和批量策略操作；复用后的新建/更新边界已在 Skill、CLI 示例和详情页参数中约束，站点详情可查看最近审计摘要。

5. 运行时视觉 QA

   现状：已用临时干净库启动真实服务并跑过一轮 Playwright smoke。验证链路包括：登录管理员、创建 Token、用 Token 发布公开样例、`/api/deploys` 返回市场作品；页面覆盖 `/`、`/market`、`/deploy`、`/agents/`、`/screens/`、`/agent/{code}/`、`/admin`、`/openapi.json`、`/skill/pagep.zip`，桌面 1440px 和手机 390px 首页均无横向溢出。后续又用临时库验证了 `/market/{code}` 详情直达、ZIP 文件树展示、ZIP 运行页相对资源执行、Markdown KaTeX / Mermaid / 表格 / 代码高亮渲染、加密站点访问密码查看、访问密码 Cookie 不能下载加密站点源码、站点所有者和管理员可下载加密站点源码，以及后台登录态和总览页渲染。现已沉淀为 `scripts/runtime-qa.mjs` 和 `scripts/visual-qa.mjs`：前者复现注册成功 / 失败、管理员登录、账号改密、登出、Token 创建和吊销、Markdown/真实 ZIP/加密/复用/屏幕绑定投放截图指令解绑/审计/CORS/OpenAPI/Skill 包下载链路，并检查 ZIP Bundle 详情、相对资源访问、访问密码票据绑定版本和切换当前版本后的失效、认证审计、账号密码审计、访问密码审计、CSP report 审计、Token 管理审计、站点管理审计、普通用户删除自己的站点、管理员删除站点、版本锁定 / 下架 / 切换当前 / 覆盖 / 删除审计、屏幕操作审计和审计查询分页 / 筛选；后者用真实浏览器覆盖前台主要页面、HTML/Markdown 运行页、加密访问页、后台主要 tab、审计日志筛选和翻页、市场详情 Bundle / 折叠文件树 / 模板复用弹窗、后台站点详情 Bundle / 文件树 / 复用参数，以及加密作品的普通用户源码下载 / 模板复用受限提示的桌面/移动端溢出与浏览器错误。本轮已修复手机首页 hero、创作市场移动端、Skill/MCP 内容页、后台移动端布局以及 Markdown nonce 缓存导致的 CSP 问题。

   待办：继续用真实生产数据量复核视觉 QA，并补充更多空状态、长中文标题、超长命令、预览卡片慢加载和模板复用批量策略场景。

6. Docker 老数据库升级验证

   现状：迁移代码和测试覆盖了部分老库字段补齐，文档说明只要保留挂载目录不应丢数据；已补充很老的 `sites` 表缺少 `public_id` 时先补列、回填再创建索引的回归测试，避免启动时报 `no such column: public_id`。新增生产近似旧库组合测试，覆盖旧站点、版本、文件、用户、Token、匿名 session、屏幕、审计日志、FTS 回填和新增表创建。新增 `scripts/legacy-upgrade-qa.mjs`，会构造旧 SQLite + hosted 文件目录，启动当前服务触发迁移，并通过后台站点详情、市场搜索、托管文件读取、加密访问、源码下载禁用、屏幕列表、Token 列表、匿名 session 和审计日志 API 做一次端到端升级演练。新增 `scripts/docker-upgrade-qa.mjs`，用于在服务器上通过真实 `docker compose up -d --build` 对同类旧数据做容器升级演练。

   待办：在服务器执行 `node scripts/docker-upgrade-qa.mjs`，并再拿一份真实旧版本 SQLite 数据库和 hosted 文件目录，在 Docker 环境执行升级，确认站点、版本、用户、Token、访问密码、分类、屏幕绑定、文件资源、FTS 回填和新增表迁移都正常。

7. OSS、SMTP 和安全专项验证

   现状：代码和单元测试覆盖了 OSS 适配、邮箱验证基础流程、访问密码票据、ZIP 路径防护等核心逻辑；源码内容接口已与页面访问权限拆开，访问密码 cookie 不能直接下载加密站点源码，只有站点所有者或管理员可下载加密站点源码；访问密码 cookie 已绑定目标版本，站点切换当前版本后旧浏览票据失效；后台 `/admin` 与 `/admin/assets/*` 已使用独立严格 CSP、禁止 iframe 嵌入并接入 CSP report-uri；站点级安全模式已影响 HTML 运行时 CSP/sandbox；前台和后台预览 iframe 已统一使用不含 `allow-same-origin` 的 `PREVIEW_IFRAME_SANDBOX`，只额外允许弹窗逃逸和用户触发顶层导航；托管页 CSP 响应已接入 report-uri，违规报告会落入审计日志；Markdown 脚本 CSP 使用 nonce-only `script-src`，不依赖 `script-src 'self'` 且不允许 `unsafe-inline` / `unsafe-eval`，KaTeX / Mermaid 所需的运行时样式通过受控的 `style-src-elem` / `style-src-attr` 放行；CORS 白名单只作用于 API / OpenAPI，外部嵌入托管应用改由嵌入策略控制，避免把 API 跨域授权误扩散到用户上传内容。

   待办：用真实阿里云 OSS、真实 SMTP、生产反向代理、泛域名/路径模式、CORS/iframe 嵌入策略做端到端验证；同时复查用户上传 JS、HTML sandbox/CSP、源码下载权限和匿名认领边界。

## 建议优先级

1. 继续打磨文件树、Bundle、安全模式和复用参数的真实站点兼容 QA，重点看复杂 ZIP 包和不同安全模式下的兼容性。
2. 完善创作市场模板复用体验，打通“发现 -> 下载/复用 -> Agent 二次创作 -> 新发布”的闭环。
3. 继续扩大审计日志失败路径覆盖，保证出问题时能反推是谁、什么时候、对什么对象做了什么。
4. 对 Markdown 高级渲染做安全专项复查，避免为了效果引入新的 XSS/CSP 风险。
5. 最后在服务器跑 `docker-upgrade-qa` 和真实旧数据 Docker 升级验证，并把 `runtime-qa` / `visual-qa` / `legacy-upgrade-qa` / `docker-upgrade-qa` 作为发布前准入项持续运行。
