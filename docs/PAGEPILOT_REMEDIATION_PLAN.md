# PagePilot 全盘整改技术计划

本文档用于约束 PagePilot 后续整改顺序、产品取舍、接口默认值、Skill/CLI/MCP 行为和验收标准。后续开发按阶段推进，每阶段完成后必须同步测试、构建和文档。

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

- 已新增 `internal/bundle`，ZIP 会识别真实站点根目录、入口文件、Markdown 包、嵌套目录和批量包误传，并拒绝路径穿越。
- 已新增 `internal/render`，Markdown 托管页支持相对图片、表格、任务列表、代码块、Mermaid/数学公式语义块和渲染缓存。
- 已新增 SQLite FTS5 市场搜索、中文 `LIKE` 回退、渲染缓存、Bundle 元数据表和审计日志表；老数据库启动时自动补齐并回填索引。
- `POST /api/deploy` 已支持 multipart；Go CLI、MCP 和 Python Skill 发布文件/目录/ZIP 时优先走 multipart，旧 JSON/base64 仍保留兼容。

### 仍需补齐

- Markdown 当前是安全语义渲染，不是 jpage 那种服务端 Markdown / highlight.js / KaTeX 渲染加内置 Mermaid 前端运行时的完整链路。
- 审计日志已有存储接口，但后台审计列表 API 和 UI 仍需产品化补齐。
- 文件树、Bundle 类型、安全模式和模板复用信息仍需在市场详情和后台站点详情中进一步展示。
- 前台“模板复用”还只是基础抽屉，没有达到 jpage 那种下载源文件、文件树、Agent 提示词、CLI/MCP 参数统一生成的完整体验。
- 尚未做一次覆盖首页、创作市场、详情、手动部署、Skill & MCP、Screens、登录注册、加密访问和后台各 tab 的运行时视觉 QA。
- 尚未用旧版本真实 SQLite 数据库和 hosted 文件目录跑 Docker 升级验证；当前只能说明迁移设计为增量、不主动清空挂载数据。

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
