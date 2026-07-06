---
name: pagep
description: 当 Agent 需要生成 HTML、Markdown、Reveal.js 幻灯片、多文件静态站点，发布或更新 PagePilot 应用，复用创作市场作品，管理访问密码、版本、Token、屏幕投放和截图时使用。
---

# PagePilot pagep Skill

## 核心规则

当用户要求生成网页、报告、仪表板、简历、可视化、Markdown 文档、Reveal.js 幻灯片，或要求「发布到 PagePilot」「生成访问链接」「投放到屏幕」时，统一走本 Skill。

内容生成后必须发布到 PagePilot，并把服务端返回的 `url`、`detailUrl` 或 `versionUrl` 交给用户。不要只输出代码块让用户自己复制，也不要自行拼接最终公网链接。

## 安装和入口

下载地址固定为：

```text
/skill/pagep.zip
```

安装后推荐使用命令名：

```bash
pagep doctor --server https://pagepilot.example.com
```

如果环境没有独立 `pagep` 命令，但已经解压 Skill 包，则在 Skill 目录内运行：

```bash
python scripts/pagep.py doctor --server https://pagepilot.example.com
```

目标服务器必须显式指定：使用 `--server` 或 `PAGEPILOT_SERVER`。用户用哪个 PagePilot 入口访问，就用哪个入口调用 API；路径模式下返回的应用链接会跟随这个入口。泛域名模式下，应用链接由后台的应用域名规则决定。

## 入口优先级

1. 能执行本地命令时，优先使用 `pagep` 或 `python scripts/pagep.py`。
2. 上传目录、ZIP、图片、字体、Reveal.js 演示或大文件时，优先走命令行 multipart 上传，避免把大段 base64 放进模型上下文。
3. 不能执行本地命令、只能使用 MCP 时，再调用 `pagep-mcp` 工具。
4. Skill、CLI、MCP 必须使用同一个 PagePilot 服务器地址和同一个用户 Token。
5. 所有入口都只展示服务端返回的链接，不按本地 host、端口或域名规则自行拼接。

## 身份和权限

- 匿名 Agent 会在本地创建或复用 `~/.pagep/agent.json` 和 `~/.pagep/session.json`。
- 匿名 session 决定未登录发布的所有权；Agent 标识、IP 和 User-Agent 只用于后台展示和排查。
- 所有未登录发布都会记录为匿名会话。只创建 session 但从未发布的空会话，不展示在后台匿名列表。
- 匿名发布受额度限制，但可以发布、更新自己拥有的站点、删除自己的站点、设置或清除访问密码。
- 用户注册或提供 Token 后，应把当前匿名 session 认领到用户：

```bash
pagep claim-session
```

- Token 归属于注册用户。`token create` 默认创建长期 Token；临时 Token 使用 `--expires-at` 或 `--ttl-seconds`。
- 屏幕绑定、投放、截图、刷新、休眠、唤醒和关机指令只允许注册用户 Token 使用。

## 发布前必须确认

- 先确认用户要「新建发布」还是「更新已有发布」。
- 如果用户要更新但不知道原 code 或 URL，先列出当前身份拥有的站点让用户选择，不要猜 code。
- 新建发布必须提供有意义的中文 `--title`，禁止使用 `index.html`、`demo`、`test`、`未命名` 这类名字。
- 必须提供 `--description`，控制在 240 字以内。
- 新建稳定项目建议使用可读的 `--code`。code 只能使用小写字母、数字和连字符。
- 首次发布前分别确认：是否进入创作市场、分类和标签、是否设置访问密码。
- `--visibility public` 表示进入 PagePilot 创作市场，可搜索、可点赞、可复用；`--visibility unlisted` 表示不进市场，只能通过链接访问。
- 匿名发布默认且只能使用 `unlisted`。
- 新公开作品发布前，先调用 `market categories` 获取服务端分类 slug，不要按文件后缀臆造分类。
- 访问密码只保护浏览器查看。匿名访客也可以输入密码访问；校验成功后获得 5 分钟访问授权。
- 追加版本或 `--update` 沿用原站点公开性和访问密码，除非用户明确要求修改。

## 内容生成规范

### 默认使用单 HTML

普通单页、报告、简历、名片、仪表板、简单可视化、活动页和工具页，优先生成单个自包含 HTML：

- CSS 放在 `<style>`。
- JS 放在 `<script>`。
- 少量图片可用 data URI 或在线 URL。
- 不要为了「看起来工程化」拆成 `index.html + style.css + app.js`。
- 不要把大型图片、视频、字体全部塞进 base64。

HTML 必须包含：

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>中文标题</title>
</head>
<body>
</body>
</html>
```

中文字体使用系统字体栈：

```css
font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
  "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif;
```

### 使用多文件或 ZIP

以下情况使用目录或 ZIP 网站包：

- 多页面应用。
- CSS、JS、图片、字体、视频资源较多。
- 用户已经提供项目目录或 ZIP。
- Reveal.js 幻灯片。
- 需要离线稳定展示。
- Markdown 引用相对图片或附件。

多文件路径必须使用干净相对路径和 `/`。拒绝绝对路径、盘符、反斜杠、`..`、空路径片段、符号链接和源目录外文件。

目录或 ZIP 推荐结构：

```text
site/
├── index.html
├── assets/
│   ├── app.css
│   └── app.js
└── images/
    └── cover.webp
```

多页面 HTML 使用相对链接，例如 `settings.html` 或 `./settings.html`。不要在路径模式下写 `/settings.html` 这类根路径。

### Markdown 边界

PagePilot 支持 Markdown 发布和安全渲染缓存，适合普通文档、说明书、报告正文和 README。

如果用户明确需要公式、Mermaid 图表、复杂代码高亮或交互组件，优先生成 HTML 或多文件静态站点，并把 KaTeX、Mermaid、highlight.js 等运行时资源随站点一起打包。不要假设 PagePilot 会自动完整注入这些库。

## Reveal.js 幻灯片规范

用户要求 PPT、幻灯片、演示文稿、deck、路演、答辩 slides 时，必须使用多文件 Bundle，不要生成单个超大 HTML。

### 规划结构

常见结构：

- 简单式：封面 → N 张内容 → 总结。
- 章节式：封面 → 章节分隔页 → 内容页 → 下一章节 → 总结。

结构语法：

- `1` 表示一张水平页。
- `N` 表示 N 张垂直堆叠页。
- `d` 表示居中大字分隔页。

示例：`1,d,3,d,2,d,1` 表示封面、分隔、3 页内容、分隔、2 页内容、分隔、总结。

### 选择主题

| 用户表达 | 主题 | 风格 |
|---|---|---|
| 商务、汇报、正式、提案、季度、年终 | `business` | 深蓝、白底、清晰信息层级 |
| 学术、论文、答辩、研究 | `academic` | 深灰、米白、克制排版 |
| 创意、产品、发布、活泼、设计 | `creative` | 高饱和、强视觉记忆点 |
| 极简、简约、Keynote、苹果风 | `minimal` | 黑白、留白、一个强调色 |

用户没有指定时默认 `business`。

### 目录结构

```text
deck/
├── index.html
└── assets/
    ├── reveal.js
    ├── reveal-base.css
    ├── theme.css
    └── plugin/
        ├── highlight/
        │   ├── plugin.js
        │   └── monokai.css
        └── notes/
            └── notes.js
```

### index.html 要求

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>幻灯片标题</title>
  <link rel="stylesheet" href="assets/reveal-base.css">
  <link rel="stylesheet" href="assets/theme.css">
  <link rel="stylesheet" href="assets/plugin/highlight/monokai.css">
</head>
<body>
  <div class="reveal">
    <div class="slides">
      <section><h1>标题</h1><p>副标题</p></section>
      <section>水平页</section>
      <section>
        <section>垂直堆叠页 1</section>
        <section>垂直堆叠页 2</section>
      </section>
    </div>
  </div>
  <script src="assets/reveal.js"></script>
  <script src="assets/plugin/highlight/plugin.js"></script>
  <script>
    Reveal.initialize({
      embedded: true,
      hash: true,
      controls: true,
      progress: true,
      slideNumber: true,
      transition: "slide",
      plugins: [RevealHighlight]
    });
  </script>
</body>
</html>
```

关键约束：

- `Reveal.initialize` 必须设置 `embedded: true`，避免 iframe 父页面抢键盘事件。
- 不要引用 jsdelivr、unpkg、cdnjs 等 CDN；所有资源放进 `assets/`。
- 每页内容必须精简；Reveal.js 不会自动滚动，溢出会被裁切。
- `reveal-base.css` 在前，`theme.css` 在后。
- 代码高亮不是必需项；没有代码页时不要引入额外高亮资源。

### 复制随包资源

Skill 包内置了 Reveal.js 资源和主题。生成幻灯片时复制：

```bash
mkdir -p deck/assets/plugin/notes deck/assets/plugin/highlight
cp assets/reveal.js deck/assets/
cp assets/reveal-base.css deck/assets/
cp assets/themes/business.css deck/assets/theme.css
cp assets/plugin/notes/notes.js deck/assets/plugin/notes/
cp assets/plugin/highlight/plugin.js deck/assets/plugin/highlight/
cp assets/plugin/highlight/monokai.css deck/assets/plugin/highlight/
```

发布：

```bash
pagep deploy ./deck \
  --title "企业季度经营汇报" \
  --description "适合大屏和会议展示的季度经营汇报幻灯片。" \
  --visibility unlisted
```

发布成功后提示用户：打开幻灯片后先点击页面区域聚焦，再用方向键翻页；如果键盘不响应，使用新窗口打开。

## 创作市场复用

用户要求「参考这个作品」「用这个模板」「按这个风格再做一个」时，先查创作市场或读取作品详情，再下载源文件作为参考。

```bash
pagep market search "报告" --sort hot --page-size 5
pagep market show project-home
pagep get project-home --download --output ./pagepilot-downloads
```

学习维度按优先级：

1. 布局结构：区域划分、信息组织、导航和视觉动线。
2. 色彩方案：主色、辅色、背景、文字色和状态色。
3. 字体排版：字号层级、行高、字重、留白。
4. 组件样式：按钮、卡片、表格、图表容器、标签。
5. 交互效果：动效、筛选、悬停、响应式。

只学习风格和结构，不复制原作品的具体文字、业务数据、密钥或私人内容。复用后默认发布为新 code；只有用户明确说要更新自己已有 code 时才追加版本。

PagePilot 当前是创作市场复用能力，不要调用不存在的内容模板实例化工具。

## 常用命令

检查服务器：

```bash
pagep doctor --server https://pagepilot.example.com
pagep session --server https://pagepilot.example.com
```

新建发布：

```bash
pagep deploy ./site \
  --server https://pagepilot.example.com \
  --code project-home \
  --title "项目官网首页" \
  --category landing \
  --visibility public \
  --description "项目官网的首页展示。"
```

追加版本：

```bash
pagep deploy ./site-v2 \
  --code project-home \
  --update \
  --title "项目官网首页升级版" \
  --description "更新页面结构和文案。"

pagep append project-home ./site-v2 \
  --title "项目官网首页升级版" \
  --description "更新页面结构和文案。"
```

访问密码：

```bash
pagep access project-home --password "change-me"
pagep access project-home --clear
```

Token：

```bash
pagep token create --label ci-bot
pagep token create --label temp-runner --ttl-seconds 86400
pagep token list
```

管理员置顶：

```bash
pagep admin pin-site project-home
pagep admin pin-site project-home --unpin
```

屏幕投放：

```bash
pagep screen list --server https://pagepilot.example.com
pagep screen bind 123456 --name "大厅屏幕"
pagep screen publish --screen screen_xxx --app project-home --expected-orientation landscape
pagep screen publish --screen screen_xxx --source ./site \
  --title "大厅展示页" \
  --visibility unlisted \
  --access-password "change-me" \
  --expected-orientation landscape \
  --description "大厅屏幕全屏展示页面。"
pagep screen screenshot screen_xxx --output ./screen-shot.jpg
pagep screen refresh screen_xxx
pagep screen sleep screen_xxx
pagep screen wake screen_xxx
pagep screen shutdown screen_xxx
pagep screen status screen_xxx
pagep screen unbind screen_xxx
```

## 屏幕投放规则

- 一个注册用户可以绑定多个屏幕。
- 投屏前先用 `screen list` 查看屏幕，多个屏幕时让用户选择。
- 投屏前确认页面预期方向：`portrait`、`landscape` 或 `any`。
- 使用屏幕返回的 `deviceInfo.orientation`、分辨率判断是否匹配。
- 如果页面方向和屏幕方向不一致，提醒用户可能裁切、缩放或留白；只有用户确认后才使用 `--force-orientation`。
- 屏幕投放发送的是播放清单和 PagePilot 应用 URL，不是把 raw HTML 字符串直接塞给硬件。
- 真正断电、定时开关机依赖设备和 OEM 能力，不要承诺所有硬件都支持。

## MCP 工具对照

能用 `pagep` 时优先用命令行；只能使用 MCP 时，按下面工具名调用：

| 场景 | MCP 工具 |
|---|---|
| 发布或追加站点 | `deploy_site` |
| 认领匿名发布 | `claim_anonymous_session` |
| 设置访问密码 | `set_site_access_password` |
| 管理员置顶 | `set_site_pin` |
| 查看文件清单和入口 | `get_site_content` |
| 版本列表、锁定、切换、删除 | `list_versions`、`lock_version`、`set_current_version`、`delete_version` |
| 搜索市场、分类、详情、点赞 | `search_marketplace`、`list_market_categories`、`get_deploy_detail`、`like_deploy` |
| 版本策略 | `set_primary_strategy` |
| 屏幕管理 | `list_screens`、`bind_screen`、`publish_screen`、`request_screen_screenshot`、`send_screen_command`、`unbind_screen` |

MCP 返回里的 URL 同样以服务端 API 返回值为准。

## 常见错误处理

| 问题 | 原因 | 处理方式 |
|---|---|---|
| 发布后 URL 404 | 服务端未部署最新版本、版本入口识别失败或反向代理未转发 | 检查接口返回的 `url`、`versionUrl`、`mainEntry` 和当前版本 |
| 样式或脚本丢失 | 多文件站点使用了根路径资源 | 改成相对路径，例如 `assets/app.css` |
| 更新变成新建 | 没有明确已有 code | 先列出自己的站点，再用 `--update` 或 `append` |
| 幻灯片空白 | Reveal.js 资源未打包或路径错误 | 确保 `assets/reveal.js`、`reveal-base.css`、`theme.css` 都存在 |
| 幻灯片翻页键不响应 | iframe 焦点问题 | 设置 `embedded: true`，提示用户点击页面区域聚焦 |
| 幻灯片文字被裁切 | 单页内容过多 | 拆页，每页只放一个核心观点 |
| 屏幕投放失败 | 未使用注册用户 Token 或屏幕不属于该用户 | 先 `screen list`，确认 Token 和屏幕归属 |
| 加密作品无法预览 | 需要访问密码授权 | 打开应用输入密码；授权有效期为 5 分钟 |
| 匿名额度耗尽 | 当前匿名 session 达到限制 | 注册登录，创建 Token，再执行 `claim-session` |

## 安全红线

- 不要上传 `.env`、API key、Bearer Token、私钥、数据库、本地配置、日志、缓存、`.git`、`node_modules`、`__pycache__`。
- 不要把用户私有内容公开进创作市场，除非用户明确确认。
- 不要绕过访问密码读取加密作品源码。
- 不要把未知第三方脚本注入公开作品；必须使用时说明来源和风险。
- 不要在总结中暴露源码、密码、Token 或敏感配置。
