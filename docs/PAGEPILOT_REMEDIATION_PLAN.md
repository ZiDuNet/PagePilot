# PagePilot 全盘整改技术计划

本文档用于约束 PagePilot 后续整改顺序、产品取舍、接口默认值、Skill/CLI/MCP 行为和验收标准。后续开发按阶段推进，每阶段完成后必须同步测试、构建和文档。

## 2026-07-07 当前验证状态

- 已完成一轮真实服务 + Playwright 深度 QA：后台注册入口、后台登录后全部 tab、创作市场列表/详情、模板复用弹窗、ZIP 多文件相对资源、Markdown KaTeX / Mermaid / 表格 / 代码高亮、匿名访问密码、普通访客加密站点源码下载受限均通过。
- 已修复本轮 QA 暴露的问题：`/admin?mode=register` 不进入注册模式、创作市场详情右侧操作区拥挤、模板复用弹窗模式切换和长命令布局不清晰。
- 已补旧库迁移回归测试：旧 `audit_logs` 缺少 `actor_role` / `result` 时可以自动迁移并保留旧日志，避免旧库启动出现 `no such column: result`；同时新增生产近似旧库组合测试，覆盖旧站点、版本、文件、用户、Token、匿名 session、屏幕、审计日志、FTS 回填和新增表创建。
- 已新增可复现运行时 QA：`scripts/runtime-qa.mjs` / `make runtime-qa` 会自动编译临时服务、启动临时 SQLite 和 hosted 目录，验证注册成功 / 失败、管理员登录、账号改密、登出、Token 创建和吊销、匿名发布显式认领与归属迁移、已认领匿名 session 拒绝继续发布、Markdown 高级渲染、真实 ZIP 站点发布、ZIP Bundle 详情、ZIP 相对资源访问、市场详情、后台 Bundle 文件树、登录用户公开未加密站点源码下载及 `source_download` 成功审计、匿名源码下载登录提示、加密站点对发布者 Token / 管理员 Cookie / 匿名访问密码 Cookie 的源码下载禁用及 `source_download` 失败审计、加密访问、访问密码票据绑定版本和切换当前版本后的失效、版本锁定 / 下架 / 切换当前 / 覆盖 / 删除审计、Token 管理审计、匿名认领审计、传统 CSP report 与 Reporting API 审计、运行设置 / 市场分类 / Skill 包上传 / 用户管理审计、模板复用来源记录、屏幕绑定 / 投放 / 截图 / 指令 / 解绑、认证 / 账号 / 访问密码 / 站点管理 / 屏幕操作审计日志、审计分页与用户 / 站点 / 动作 / 角色 / 时间 / 详情关键字过滤、CORS 边界、OpenAPI 和 Skill ZIP 下载。
- 已新增真实浏览器视觉 QA：`scripts/visual-qa.mjs` / `make visual-qa` 会启动临时服务并用真实浏览器检查前台主要页面、HTML/Markdown 运行页、加密访问页、后台主要 tab、审计日志筛选和翻页、市场详情的 Bundle 信息 / 折叠文件树 / 模板复用弹窗，以及后台站点详情中的 Bundle 信息、完整文件树、复用参数和加密作品的源码下载 / 模板复用受限提示的桌面/移动端溢出、空白页与浏览器错误。本轮已据此修复创作市场移动端、Skill/MCP 内容页、后台移动端布局，以及 Markdown nonce 页面缓存导致的 CSP 误拦截。
- 已新增旧库升级演练 QA：`scripts/legacy-upgrade-qa.mjs` / `make legacy-upgrade-qa` 会构造旧 SQLite + hosted 目录，覆盖公开站点、加密站点、旧管理员、Token、匿名 session、屏幕绑定、审计日志和托管文件，并通过当前服务启动迁移、后台 API、市场搜索、访问密码、源码下载权限和直接 SQLite 校验确认升级后仍可用。
- 已新增真实容器升级演练脚本：`scripts/docker-upgrade-qa.mjs` / `make docker-upgrade-qa` 会构造临时旧库和 hosted 目录，执行真实 `docker compose up -d --build`，再通过容器 HTTP 接口和 SQLite 校验站点、版本、用户、Token、匿名 session、屏幕、访问密码、FTS、Bundle、审计表和 Skill ZIP。
- 已补前后台部署错误面板：手动部署页和后台发布页会把 `stage`、稳定错误码、服务端 `hint` 和本地排查建议展示出来，并支持复制诊断信息。
- 本机缺少 Docker CLI，真实 Docker 老库升级演练仍未执行；上线前需要在服务器运行 `node scripts/docker-upgrade-qa.mjs`，并用真实旧数据目录执行一次 `docker compose up -d --build` 核对站点、版本、用户、Token、访问密码、屏幕绑定和 hosted 文件。

## 1. 总体目标

PagePilot 的定位是 **Agent 生成应用的发布控制台**。它不只是 HTML 文件托管，而是把 Agent 生成的 HTML、Markdown、ZIP、多文件站点、演示型应用发布为可访问、可治理、可复用、可投放的应用资产。

核心交付链路：

1. Agent 或用户产出 HTML / Markdown / ZIP / 多文件站点。
2. PagePilot 发布为 `/agent/{code}/` 应用。
3. 系统记录版本、访问权限、分类、标签、来源和归属。
4. 登录用户可选择公开进入创作市场，匿名发布默认仅链接访问。
5. 创作市场提供预览、收藏、点赞、下载源码、CLI/MCP/Agent 二次创作。
6. 管理后台负责用户、匿名 session、应用、分类、标签、Token、运行设置、Skill/MCP/CLI、API 文档和屏幕投放管理。

## 2. 全局产品原则

- 默认不公开：所有发布入口默认 `visibility=unlisted`，公开进创作市场必须显式选择。
- 默认不加密：访问密码默认关闭，但首次发布需要询问是否加密。
- 匿名可试用：匿名可以发布和更新，但只能操作当前匿名 session 自己的内容。
- 认领即转移：匿名 session 认领到用户后，更新权转移到登录用户，原匿名 session 不再允许继续更新。
- 不允许猜 code 更新：更新已有发布必须从当前用户或当前匿名 session 可更新列表选择，不能自由填写别人的 code。
- code 不作为市场卡片主信息：市场卡片隐藏 code，详情页展示 code 和复制入口。
- Skill/CLI/MCP/API 能力对齐：同一套默认值、同一套权限边界、同一套文案。
- 兼容旧 API 但不宣传旧入口：兼容仅用于旧客户端迁移，新 UI、新文档、新 Skill 只宣传 PagePilot/pagep 入口。

## 3. jpage 对照后的取舍

### 需要吸收

- Markdown 高级渲染：代码高亮、KaTeX、Mermaid、明暗主题。
- ZIP Bundle 识别：自动找到真正入口层，避免多层目录包访问 404。
- CSP/sandbox 分级：后台严格 CSP，用户 HTML 隔离预览，Markdown 更严格。
- 市场卡片：预览为主，hover 显示操作，信息更克制。
- 标签/分类/收藏/公开状态：用于市场发现和后台运营。
- 邮箱验证：通过 env 开启注册和 SMTP 验证。
- API/CLI/MCP 对称：网页能做的核心能力，CLI/MCP 也要能做。

### 暂不照搬

- 短链 `/s/:key`：PagePilot 先继续使用 `/agent/{code}/` 和预留泛域名模式。
- Token 可再次查看明文：安全上不建议，PagePilot 仍采用一次性显示。
- Reveal.js 作为核心代码渲染链路：暂不在服务端单独内置 Reveal 引擎。

### Reveal.js 处理方式

Reveal.js Bundle 模式作为 **Skill/CLI 的可选创作能力**：

- Skill 可以建议 Agent 生成 Reveal.js 演示站点，并以 ZIP/多文件站点发布。
- PagePilot 服务端只需要按普通 ZIP/多文件站点托管，不新增专门 Reveal 后端。
- 创作市场可增加“演示文稿/路演汇报/大屏展示”分类或标签。
- 后续若 Markdown Slides 需求强，再作为 Markdown 渲染增强独立立项。

## 4. 阶段一：安全默认值与发布链路

### 目标

先修最危险、最影响信任的默认值和更新逻辑。

### 改动

- 前台、后台、Skill、CLI、MCP、API 文档统一默认 `visibility=unlisted`。
- 匿名发布强制默认不公开，不能直接进入创作市场。
- 手动部署页面路径统一为 `/deploy`，不再保留旧的 `.html` 前台入口。
- 首次发布时 UI 显示“市场可见性”“访问密码”“分类”“标签”的明确含义。
- 更新已有发布时：
  - 登录用户只能从“我的发布”选择。
  - 匿名用户只能从当前匿名 session 的发布选择。
  - 不允许自由填写 code 更新。
  - 更新版本默认继承原可见性、访问密码、分类、标签。

### 后端约束

- `POST /api/deploy` 如果没有认证上下文或为匿名上下文，最终 visibility 必须落为 `unlisted`。
- append/update 必须校验 owner：`user:{id}`、`token:{id}` 或 `anon:{session}`。
- 匿名 session 已认领后，原匿名 session 再发布/更新应返回中文友好错误。

### 验收

- 未登录手动发布后不出现在创作市场。
- 匿名 API 明传 `visibility=public` 也不能进入创作市场。
- 登录用户公开发布才出现在市场。
- 不能用 A 用户更新 B 用户 code。
- 被认领的匿名 session 不能继续匿名更新。

## 5. 阶段二：创作市场与标签产品化

### 目标

把创作市场从“数据列表”改为“作品发现与复用中心”。

### 改动

- 市场卡片改成预览主导：
  - 默认展示预览图、分类、精选/热度、类型、标题、描述、作者、标签、更新时间。
  - hover 展示点赞、收藏、打开、详情、使用模板。
  - 删除按钮仅登录后且有权限时出现。
  - code 从卡片隐藏，详情页展示。
- 增加标签能力：
  - 发布时可填 tag。
  - 后台应用管理可查看、编辑、筛选 tag。
  - 市场可按 tag 搜索。
- 分类筛选：
  - 内容筛选是一层：全部、HTML、Markdown、加密、精选、我的发布、我的收藏。
  - 应用分类是二层：由后台维护，可联合筛选。
- 详情页：
  - 返回列表按钮在预览工具条左侧。
  - 返回保留市场筛选、排序、滚动位置。
  - 右侧信息卡压缩统计布局。
  - 默认只显示 2 个版本，多余版本在右侧卡片内展开，不拉长整个页面。

### 验收

- 宽屏卡片自适应但不拥挤。
- 未登录点击收藏显示友好提示，不暴露原始鉴权错误。
- “我的发布”数量与实际归属一致。
- 标签在前台详情、市场卡片、后台列表/渲染视图中均可见。

## 6. 阶段三：后台管理台产品化

### 目标

后台按管理系统重构，不再用大块卡片堆内容。

### 菜单

- 发布应用
- 应用管理
- 应用分类
- 屏幕管理
- API 令牌管理
- 账号设置
- 用户管理
- 匿名管理
- 运行设置
- Skill / MCP / CLI
- API 文档
- 返回首页

### 改动

- 应用管理支持列表视图和渲染视图。
- 增加筛选：公开状态、加密状态、分类、标签、匿名/注册用户、来源、更新时间。
- 分类管理改为紧凑表格 + 搜索 + 弹窗/抽屉编辑。
- 匿名管理显示 agent 名称、设备 IP、UA、发布数、认领状态、最后活跃时间。
- Token 管理按用户区分，Token 创建只显示一次明文。
- API 文档采用左侧目录 + 右侧端点详情。
- 全局错误提示中文化、可关闭、自动消失。

### 验收

- 管理员看到全量管理功能。
- 普通用户只看到自己的应用、Token、收藏、账号设置。
- 后台所有页面无横向滚动、无文字遮挡、无过度卡片化。

## 7. 阶段四：邮箱注册验证

### 目标

支持可配置的邮箱注册验证闭环。

### Env

```env
ALLOW_REGISTRATION=false
EMAIL_VERIFICATION_ENABLED=false
SMTP_HOST=
SMTP_PORT=465
SMTP_SECURE=true
SMTP_USER=
SMTP_PASS=
SMTP_FROM=
APP_PUBLIC_URL=
```

### 改动

- 注册功能由 `ALLOW_REGISTRATION` 控制。
- 邮箱验证由 `EMAIL_VERIFICATION_ENABLED` 控制。
- 登录页提供注册入口，不在首页放注册按钮。
- 注册流程：图形验证码 -> 邮箱验证码 -> 创建用户 -> 自动登录。
- 后台运行设置显示 SMTP 状态。
- 管理员仍可直接创建用户。

### 验收

- 关闭注册时普通用户无法自助注册。
- 开启注册但未配置 SMTP 时给出明确提示。
- 邮箱验证码过期、重复、错误都有中文错误。
- 注册成功后 email_verified 正确记录。

## 8. 阶段五：OSS 存储适配

### 目标

文件存储支持本地和阿里云 OSS，可由 env 切换。

### Env

```env
STORAGE_DRIVER=local
OSS_ENDPOINT=
OSS_BUCKET=
OSS_ACCESS_KEY_ID=
OSS_ACCESS_KEY_SECRET=
OSS_PREFIX=pagepilot/
OSS_PUBLIC_BASE_URL=
```

### 改动

- 抽象 Storage 接口：Put、Get、Delete、Exists、ListPrefix、Open。
- 默认 local，不影响现有部署。
- 阿里云 OSS 作为可选 driver。
- 部署文件、源码 ZIP、预览资源统一走 Storage。
- SQLite 仍保存元数据。
- ZIP 解压必须防路径穿透，OSS key 必须规范化。

### 验收

- local 模式现有测试不回退。
- OSS 模式可以发布 HTML、Markdown、ZIP、多文件站点。
- 删除应用会清理对应对象。
- 路径穿透 ZIP 被拒绝。

## 9. 阶段六：Markdown / Bundle 能力

### 目标

让 Markdown 文档页成为一等应用，同时支持 Reveal.js Bundle 作为可选发布形态。

详细当前状态和跨阶段待办见 [CURRENT_STATUS_AND_TODO.md](CURRENT_STATUS_AND_TODO.md)。

### 2026-07-06 已落地状态

- 已新增 `internal/bundle`，ZIP 会识别真实站点根目录、入口文件、Markdown 包、嵌套目录和批量包误传，并拒绝路径穿越；ZIP/Bundle 失败会返回 `stage=zip_bundle`、稳定错误码和可直接展示的 `hint`。
- 已新增 `internal/render`，Markdown 托管页支持相对图片、表格、任务列表、代码块、Mermaid/数学公式语义块和渲染缓存。
- 已新增 SQLite FTS5 市场搜索、中文 `LIKE` 回退、渲染缓存、Bundle 元数据表和审计日志表；老数据库启动时自动补齐并回填索引。审计日志已具备后台列表 API、OpenAPI、全局审计 UI、站点详情最近审计摘要、分页筛选、用户 / 站点下拉、动作预设、操作者类型、对象 ID 过滤、RFC3339 时间过滤、IP/UA/详情关键字搜索、CLI/MCP/Skill 查询和发布失败日志。
- `POST /api/deploy` 已支持 multipart，`PATCH /api/deploys/{code}/versions/{version}` 覆盖版本也支持 multipart；Go CLI 和 Python Skill 发布、追加、覆盖文件/目录/ZIP 时优先走 multipart，MCP 发布/追加同样走 multipart，旧 JSON/base64 仍保留兼容。Python Skill 发布/追加/覆盖成功后会输出服务端返回 URL 摘要，并保留 JSON 供自动化解析。

### 仍需补齐

- Markdown 已接入 GFM、Chroma 高亮、同源 KaTeX / Mermaid runtime、内置 KaTeX 字体、内置样式和初始化脚本 nonce、严格脚本 CSP；行内公式、单行 / 多行块级公式和图表在浏览器端由平台内置运行时渲染。Markdown 脚本策略使用 nonce-only `script-src`，不依赖 `script-src 'self'`，也不允许 `script-src 'unsafe-inline'` / `unsafe-eval`，KaTeX / Mermaid 需要的运行时样式通过受控的 `style-src-elem` / `style-src-attr` 放行。Markdown 页面支持 `?theme=auto|light|dark`，渲染缓存 key 包含 `code / version / entry / contentSHA / theme / rendererVersion`。本轮已补公式提取边界和 sanitizer 边界：代码 span / 普通代码块内的美元符号不会误渲染成公式，带参数 info string 的 `mermaid title=...` 和 `katex display` 也会识别；编码 active URL、事件属性、SVG `data:` 图片、不安全 `srcset` 候选和 `xlink:href` 等命名空间 URL 属性会被剔除。后续仍需补充更多公式 / Mermaid 回归和安全专项复查。
- 后台 `/admin` 入口和 `/admin/assets/*` 静态资源已接入独立严格 CSP、`nosniff`、`X-Frame-Options: DENY` 和 `frame-ancestors 'none'`，不允许 `unsafe-inline` / `unsafe-eval`，并通过 `/api/security/csp-report` 上报违规。
- 前台应用卡片、市场详情、手动部署实时预览和后台渲染视图已统一预览 iframe sandbox：允许脚本、表单、下载、弹窗逃逸和用户触发顶层导航，但不授予 `allow-same-origin`；真实 `/agent/{code}` 运行时仍由站点级安全模式决定。
- 审计日志基础产品化已完成，发布失败、注册 / 登录失败、运行配置更新失败、市场分类配置失败、Skill 包上传失败、访问密码设置失败、访问密码验证失败、账号密码修改失败、站点可见性设置失败、置顶 / 分类 / 标签 / 源码下载策略 / 模板复用策略 / 安全模式 / 主版本策略失败、版本管理失败、站点删除失败、匿名认领失败、屏幕操作失败、Token 管理失败和用户管理失败已经写入结构化 failed 日志；发布、版本、配置、站点、注册 / 登录 / 登出、访问密码验证成功、账号密码修改成功、匿名认领、Skill 包、屏幕、Token 和用户管理等关键成功动作也会写入日志。认证、访问密码验证和账号密码修改日志只记录站点、版本、用户、结果和失败原因等非敏感信息，不记录密码明文。托管 HTML / Markdown CSP 已接入 `/api/security/csp-report`，浏览器通过传统 `application/csp-report` 或 Reporting API 上报的违规报告都会归一化为 `security.csp_report` 安全审计日志。后台全局审计页已提供用户 / 站点下拉、动作预设、操作者类型、角色、结果、对象类型和时间范围筛选，站点详情会展示当前 code 的最近审计摘要；`runtime-qa` 已在真实 SQLite 服务上验证分页、站点、动作、操作者、角色、时间窗口、详情 JSON 关键字过滤，`version.lock` / `version.status` / `version.current` / `version.overwrite` / `version.delete` 五类版本管理成功日志，`token.create` / `token.revoke` 成功日志，`anonymous.claim` 匿名认领日志，`security.csp_report` 安全日志，`config.update` / `config.market_categories` 配置日志，`skill.package_upload` Skill 包日志，以及 `user.create` / `user.update` / `user.delete` 用户管理日志，且不泄露明文 Token 或用户密码。后续需要继续补齐更多低频管理动作失败日志，并做真实生产数据量下的后台查询 QA。
- 文件树、Bundle 类型、安全模式、入口识别说明、模板复用策略和最近审计摘要已在市场详情或后台站点详情中展示；前台支持 `/market/{code-or-publicId}` 直达详情，刷新或分享链接后仍会重新拉取文件树与复用参数。前台市场详情和后台站点详情的文件树已支持路径 / 文件名 / SHA 搜索、目录数 / 文件数 / 总大小统计、复制路径和复制 SHA。新建发布和覆盖版本会写入 `single_html`、`markdown`、`zip_site`、`static_site` 四类稳定 Bundle kind，旧数据会按入口和文件数兜底推断；ZIP/Bundle 入口识别失败会返回 `ZIP_UNSAFE_PATH`、`ZIP_AMBIGUOUS_ENTRY` 等稳定错误码和 `hint`，前后台发布页已展示错误阶段、错误码、修复建议和可复制诊断信息；CLI/MCP/Skill 可查询后台站点详情和文件树；后台站点详情已提供源码下载 / 模板复用策略和站点级安全模式开关，`/agent/{code}` HTML 运行时会按 `auto / strict / compatible / trusted` 生效模式设置 CSP/sandbox，且加密作品对普通用户不提供源码下载，站点所有者和管理员可直接下载；CSP 违规会进入审计日志辅助排查安全模式兼容性。后续继续打磨安全模式真实兼容性 QA 和复杂 ZIP 样例。
- 前台“模板复用”已接入抽屉和服务端复用策略，详情接口会返回 `allowDownload`、`allowReuse`、`policyNote`、源码下载、Agent 提示词、CLI 命令和 MCP 参数；复用抽屉已区分“新建二创”和“更新已有发布”，更新模式自动带入当前作品 code，只有所有者或管理员可复制追加版本命令，同时展示源文件结构摘要并提供独立 MCP 参数复制。发布 API、CLI、MCP 和 Skill 已支持 `templateSourceCode` / `templateSourceVersion`，复用后会记录来源并增加来源作品复用次数。访问密码浏览权限不再等同于源码下载权限；浏览票据已绑定目标版本，切换当前版本后需要重新验证；后续继续打磨真实生产数据视觉 QA 和批量策略。
- 已完成一轮基础运行时视觉 smoke：临时干净库启动真实服务，完成管理员登录、Token 创建、公开样例发布、市场 API 查询，并用 Playwright 覆盖 `/`、`/market`、`/deploy`、`/agents/`、`/screens/`、`/agent/{code}/`、`/admin`、`/openapi.json`、`/skill/pagep.zip`；桌面 1440px 和手机 390px 首页无横向溢出。后续又用临时库验证了 `/market/{code}` 详情直达、ZIP 文件树展示、ZIP 运行页相对资源执行、Markdown KaTeX / Mermaid / 表格 / 代码高亮渲染、加密站点访问密码查看、访问密码 Cookie 不能下载加密站点源码、站点所有者和管理员可下载加密站点源码，以及后台登录态和总览页渲染。当前已把非浏览器运行时链路沉淀为 `scripts/runtime-qa.mjs`，用于复现 Markdown、真实 ZIP 站点发布、ZIP Bundle 详情、ZIP 相对资源访问、加密访问、源码下载隔离、访问密码票据绑定版本和切换当前版本后的失效、模板来源、屏幕绑定 / 投放 / 截图 / 指令 / 解绑、注册成功 / 失败、登录、账号改密、登出、访问密码、站点管理、传统 CSP report 与 Reporting API 审计、版本锁定 / 下架 / 切换当前 / 覆盖 / 删除、Token 管理、运行设置、市场分类、Skill 包上传、用户管理和屏幕操作审计日志、审计分页和筛选、CORS、OpenAPI 和 Skill ZIP 验证；同时新增 `scripts/visual-qa.mjs`，用真实浏览器覆盖前台主要页面、HTML/Markdown 运行页、加密访问页、后台主要 tab、审计日志筛选和翻页、市场详情 Bundle 信息 / 折叠文件树 / 模板复用弹窗、后台站点详情完整文件树 / 复用参数，以及加密作品的普通用户源码下载 / 模板复用受限提示的桌面/移动端溢出、空白页和浏览器错误。本轮已修复移动端市场、Skill/MCP 内容页、后台布局和 Markdown nonce 缓存导致的 CSP 问题。
- 尚未在当前 Windows 本机用旧版本真实 SQLite 数据库和 hosted 文件目录跑 Docker 升级验证；当前只能说明迁移设计为增量、不主动清空挂载数据。单元测试已覆盖老 `admin_users.email`、`audit_logs.result` 和 `sites.public_id` 缺列时先补列再建索引的启动迁移，并补充生产近似旧库组合场景，验证旧站点、版本、文件、用户、Token、匿名 session、屏幕和审计日志升级后仍可读取。`scripts/legacy-upgrade-qa.mjs` 已补充本地旧库 + hosted 升级演练，覆盖迁移启动后的后台站点详情、市场搜索、托管文件读取、访问密码、加密站点源码下载权限、屏幕、Token、匿名 session、审计日志、FTS 回填和新增表检查；`scripts/docker-upgrade-qa.mjs` 已提供真实 Docker Compose 演练入口，仍需在具备 Docker 的服务器上执行。

### 改动

- Markdown 渲染支持代码高亮、KaTeX、Mermaid、明暗主题。
- Markdown ZIP 支持图片和相对资源。
- ZIP 自动识别包含 HTML/Markdown 入口的真实层级。
- Skill 增加 Reveal.js Bundle 指南：
  - 适合路演、汇报、课程、广告屏展示。
  - 建议输出 `index.html`、`dist/`、`assets/` 或 `slides.md` + reveal runtime。
  - 通过普通 ZIP/多文件站点发布。

### 验收

- Markdown 图片相对路径可访问。
- Mermaid/KaTeX 不破坏 CSP。
- Reveal Bundle 按普通站点访问正常。
- Skill 不承诺服务端内置 Reveal 引擎。

## 10. 阶段七：Skill / CLI / MCP / API / 文档对齐

### 目标

统一所有外部入口的能力和说法。

### Skill

- 全部改为 PagePilot / pagep，不再宣传 hostctl。
- 默认不公开、默认不加密，但必须询问。
- 发布前先拉分类。
- 更新前先列当前可更新项目，不允许猜 code。
- 匿名更新只限当前匿名 session。
- 认领后更新权转给用户。
- Reveal.js Bundle 作为可选创作建议。

### CLI / MCP

- 增加/确认命令：
  - `pagep deploy`
  - `pagep append`
  - `pagep list --mine`
  - `pagep market categories`
  - `pagep tags`
  - `pagep access`
  - `pagep download`
  - `pagep screen publish`
  - `pagep admin site-detail`
  - `pagep admin audit-logs`
  - `pagep admin reuse-policy`
  - `pagep admin security-mode`
- MCP 工具与 CLI 能力对齐。

### 文档

- README
- Docker 部署文档
- `.env.example`
- Skill 下载页文案
- 后台 API 文档
- 运行设置说明
- CODEX_CONTEXT.md

### 清理

- 删除临时测试文件、无关打包产物、旧 hostctl 残留文案。
- 不提交本地数据库、日志、node_modules、构建缓存。

### 验收

- 新 UI、新文档、新 Skill 不应出现旧外部域名；`hostctl` 仅允许作为历史兼容路径、内部模块名或兼容接口说明出现。
- Go 测试通过。
- 前台构建通过。
- 后台构建通过。
- Skill ZIP 重新打包。
- Docker 文档能按新 env 启动。

## 11. 最终验收矩阵

| 场景 | 预期 |
|---|---|
| 匿名发布 | 成功，默认不公开，不进市场 |
| 匿名更新 | 只能更新当前匿名 session 的发布 |
| 匿名认领 | 发布归入用户，原匿名 session 不能继续更新 |
| 登录发布 | 默认不公开，可显式公开进市场 |
| 登录更新 | 只能从我的发布选择 |
| 管理员管理 | 可筛选用户/匿名/分类/tag/状态 |
| 市场卡片 | 信息克制，hover 操作，隐藏 code |
| 详情页 | 显示 code、版本、统计、下载、复用指令 |
| 邮箱注册 | env 开启后可完整验证 |
| OSS | env 切换后发布资源可正常读写 |
| Markdown | 高级渲染、图片、主题正常 |
| 模板复用 | 下载源文件、文件树、Agent 提示词、CLI/MCP 参数都可从详情页一致生成 |
| Reveal Bundle | 作为普通 ZIP/多文件站点可发布 |
| Skill | 默认不公开/不加密但询问，不猜 code 更新 |
| Docker 升级 | 使用旧数据库和旧 hosted 目录升级后数据不丢失 |
| 视觉 QA | 主要页面桌面和移动视口无明显遮挡、截断、横向滚动和空状态缺失 |
