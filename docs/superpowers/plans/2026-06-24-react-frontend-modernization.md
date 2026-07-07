# PagePilot React 前端整改计划

## 目标

把用户端和后台从多份原生 HTML 页面逐步迁移到框架化实现，同时保持现有 API、部署、匿名会话、Token、屏幕投放和详情页访问不破坏。

## 已完成的第一阶段

- 用户端新增 `frontend/user` React + Vite 工程。
- 构建产物输出到 `internal/web/user/app`，继续由 Go `embed` 打进服务端二进制。
- `/`、`/deploy.html`、`/agents/`、`/screens/`、`/api-docs.html` 已切换为同一个 React App。
- `/deploy/{uuid}` 详情页暂时保留原实现，避免影响已发布应用详情访问。
- 后台 `Skill` 菜单改为 `Skill & MCP`，并增加 MCP 介绍 tab。
- 匿名 Agent 后台页只展示未登录发布记录，不再为了查看列表主动创建匿名 session。

## 后续阶段

1. 后台 React 化
   - 建议独立 `frontend/admin`，使用 React + Ant Design 或同等成熟控件库。
   - 先迁移概览、站点管理、屏幕管理和 Token 管理。
   - 保留现有 `/admin` API 和权限模型。

2. 用户端功能补齐
   - 首页商城加入分页和源码查看弹窗。
   - 手动部署补齐二进制文件 base64、拖拽目录、上传进度。
   - Screen 页面加入登录后屏幕操作入口。

3. 统一设计系统
   - 抽出 `Logo`、`TopNav`、`Button`、`Badge`、`Panel`、`Table`、`SegmentedControl` 等组件。
   - 把后台和用户端的字体、颜色、间距、按钮状态统一到 token。

4. 验证与发布
   - `npm run build`
   - `go test ./...`
   - Playwright 覆盖首页、部署页、Agent/Screen/API 文档、后台登录态。
   - Docker 构建验证内置页面路径不 404。
