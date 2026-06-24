# 屏幕端 APP 与投屏能力实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为 PagePilot 增加注册用户可绑定多个硬件屏幕并发布 HTML 应用到屏幕的能力。

**架构：** 后端新增屏幕、配对码、设备 Token、投屏清单和心跳数据模型；用户侧 API 只接受注册用户 Token 或已登录用户会话；设备侧 API 只接受 Device Token。屏幕 APP 采用 Android 壳 + X5 WebView，服务器地址由用户首次配置并保存。

**技术栈：** Go HTTP API、SQLite、Python Skill CLI、Android Kotlin/X5 WebView 骨架。

---

## 文件结构

- `internal/store/screen.go`：屏幕相关 SQLite 存储实现。
- `internal/store/store.go`：新增屏幕实体和 Store 接口。
- `internal/store/schema.sql`：新增屏幕、配对码、投屏清单表。
- `internal/api/screen_types.go`：新增屏幕 API 请求和响应类型。
- `internal/api/server.go`：注册并实现屏幕用户侧和设备侧 API。
- `internal/api/screen_test.go`：覆盖注册用户投屏、匿名拒绝、设备 manifest 权限。
- `skill/hostctl-deploy/scripts/hostctl_deploy.py`：新增 `screen` 子命令，复用已保存服务器地址。
- `apps/screen-app/README.md`：更新 X5 和服务器地址可配置说明。
- `apps/screen-app/app/`：放置 Android X5 播放器骨架。

## 任务 1：存储模型

- [ ] 编写失败测试：创建配对码、注册用户绑定屏幕、发布 manifest、设备 Token 拉取 manifest。
- [ ] 运行 `go test ./internal/store -run Screen -count=1`，确认因接口不存在失败。
- [ ] 新增 SQLite 表和 Store 方法。
- [ ] 运行 `go test ./internal/store -run Screen -count=1`，确认通过。

## 任务 2：HTTP API

- [ ] 编写失败测试：匿名访问 `/api/screens` 返回 401。
- [ ] 编写失败测试：注册用户绑定配对码后可以在 `/api/screens` 看到屏幕。
- [ ] 编写失败测试：注册用户只能向自己屏幕发布站点。
- [ ] 编写失败测试：设备 Token 可以获取自己的 `/api/device/manifest`。
- [ ] 实现最小 handler 和鉴权。
- [ ] 运行 `go test ./internal/api -run Screen -count=1`。

## 任务 3：Skill 命令

- [ ] 新增 `screen list`、`screen bind`、`screen publish`、`screen status`、`screen unbind` 命令。
- [ ] Skill 投屏命令必须要求 `HOSTCTL_TOKEN` 或保存的注册用户 Token。
- [ ] 匿名 session 不参与投屏。
- [ ] 用 `python skill/hostctl-deploy/scripts/hostctl_deploy.py screen --help` 验证命令存在。

## 任务 4：屏幕 APP 骨架

- [ ] 添加 Android 项目骨架，主路线为 X5 WebView。
- [ ] 保存服务器地址到本地配置。
- [ ] 首次启动无服务器地址时显示配置页。
- [ ] 有服务器地址但未绑定时显示配对页。
- [ ] 绑定后拉取 manifest 并交给 X5 WebView 播放。

## 任务 5：整体验证

- [ ] 运行 `go test ./...`。
- [ ] 运行 Skill CLI help 验证。
- [ ] 检查 `git diff --check`。
- [ ] 分阶段提交。
