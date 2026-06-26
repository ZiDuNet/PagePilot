# 屏幕端 APP 与投屏能力实现记录

本文档记录 PagePilot 屏幕端能力的当前实现口径。它不再作为待执行计划使用；新的开发任务应在新的计划文件中记录。

## 当前目标

PagePilot 支持注册用户绑定多个硬件屏幕，并把自己发布的 HTML 应用或应用商城中可访问的站点投放到屏幕端播放。屏幕端面向广告屏、安卓屏和安卓盒子，采用 Android 壳 + X5 WebView 播放。

## 核心架构

- 后端以 SQLite 保存屏幕、配对码、Device Token、投屏清单、心跳、截图和指令状态。
- 用户侧 API 只接受注册用户 Token 或已登录会话；匿名 session 不能绑定屏幕、投屏、截图或下发指令。
- 设备侧 API 只接受 `Authorization: Device <token>`，Device Token 只允许访问自己的 manifest、WebSocket、心跳、截图回传和指令确认。
- 投屏下发的是 manifest 和服务端生成的 App URL，不是直接下发裸 HTML 字符串。
- 屏幕 APP 的服务器地址保存在本机，官方服务器只是默认选项；切换私有服务器后，绑定、manifest、WebSocket 都使用该服务器。

## 主要文件

- `internal/store/screen.go`：屏幕、配对码、manifest、截图和指令相关存储逻辑。
- `internal/store/store.go`：屏幕实体与 Store 接口定义。
- `internal/store/schema.sql`：屏幕、配对码、投屏清单等表结构。
- `internal/api/screen_types.go`：屏幕 API 请求和响应类型。
- `internal/api/screen_ws.go`：设备 WebSocket 控制通道。
- `internal/api/screen_screenshot.go`：截图请求、上传和读取逻辑。
- `internal/api/server.go`：屏幕用户侧、设备侧 API 路由注册。
- `internal/api/openapi.go`：屏幕接口 OpenAPI 文档。
- `skill/hostctl-deploy/scripts/hostctl_deploy.py`：`screen` 子命令和投屏前横竖屏校验。
- `apps/screen-app/README.md`：Android 屏幕端使用说明。
- `apps/screen-app/app/`：Android X5 播放器项目。

## 已实现能力

- 注册用户可以查看、绑定、投放、截图、刷新、休眠、唤醒、软关机和解绑自己的屏幕。
- 管理后台、Skill 和 MCP 均可操作屏幕；相关操作都要求注册用户身份。
- 屏幕端通过 `/api/device/ws` 保持 WebSocket 长连接，投屏、刷新、截图、休眠、唤醒和软关机指令会实时下发。
- HTTP manifest 仍保留为初始化和兜底读取能力。
- 截图只在后台、Skill 或 MCP 指令触发时回传；APP 不主动上报截图。
- 截图使用 Android 系统截屏能力，APP 端会压缩后再上传。
- 休眠和软关机表现为黑屏待机时钟页，不假设所有硬件都支持真实断电。
- 屏幕端右上角提供网络与 WebSocket 状态点，连续点击 5 次可打开隐藏设置。
- 隐藏设置可查看用户信息、设备型号、Android 版本、分辨率、横竖屏、WebView/X5 状态等信息。
- Skill 投屏前可通过屏幕信息检查横竖屏，不匹配时会提醒并阻止；需要强制投放时使用 `--force-orientation`。

## API 概览

用户侧接口：

```text
GET    /api/screens
POST   /api/screens/bind
POST   /api/screens/{screenId}/publish
POST   /api/screens/{screenId}/screenshot
GET    /api/screens/{screenId}/screenshot
POST   /api/screens/{screenId}/command
DELETE /api/screens/{screenId}
```

设备侧接口：

```text
POST /api/device/pairing/start
POST /api/device/pairing/complete
GET  /api/device/ws
GET  /api/device/manifest
POST /api/device/heartbeat
POST /api/device/screenshot
POST /api/device/command/ack
```

## Skill 命令

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py --server https://pagepilot.example.com screen list
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen bind 123456 --name "大厅屏"
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen publish --screen screen_xxx --app my-landing
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen publish --screen screen_xxx --source ./site --title "大屏展示" --description "Fullscreen display for the lobby."
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen screenshot screen_xxx --output ./screen-shot.jpg
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen refresh screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen sleep screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen wake screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen shutdown screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen unbind screen_xxx
```

`--server` 表示本次 Skill/MCP/CLI 连接 PagePilot 控制面的入口。路径模式下，发布或投屏返回的 App URL 会按该入口生成；泛域名模式下，App URL 由后台配置的应用域名后缀、协议和端口生成。

## 部署注意事项

- 反向代理必须透传 `Host`、`X-Forwarded-Host`、`X-Forwarded-Proto`。
- `/api/device/ws` 必须透传 WebSocket Upgrade 头，否则实时控制会退化或失败。
- Docker 只需要暴露 PagePilot 服务端口，不需要为每个屏幕或每个应用单独暴露端口。
- 真实开机自启、应用守护、定时开关机通常依赖设备厂商、系统权限、Device Owner 或 MDM，不应假设普通 APP 可以覆盖所有硬件。

## 验证建议

- `go test ./internal/api -run Screen -count=1`
- `go test ./internal/store -run Screen -count=1`
- `python skill/hostctl-deploy/scripts/hostctl_deploy.py screen --help`
- Android 端在真机或模拟器上验证配对、WebSocket 状态、投屏、刷新、截图、休眠、唤醒和隐藏设置。
