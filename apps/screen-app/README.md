# PagePilot Screen App

本目录承载 PagePilot 屏幕端 Android APP。它面向广告屏、安卓屏、安卓盒子等可安装 APK 的播放终端，用于绑定用户名下的硬件屏幕，并全屏播放后台或 Agent Skill 投放的 PagePilot 应用。

## 设计目标

- 一个注册用户可以绑定多个硬件屏幕。
- 屏幕 APP 只保存服务器地址和 Device Token，不保存用户 Token。
- Device Token 基本长期有效，但后台解绑后立即失效，屏幕端需要重新配对。
- Skill、MCP 和后台都可以操作屏幕，但仅限注册用户 Token 或已登录用户。
- 投放到屏幕的是播放 manifest 和 App URL，不是直接下发裸 HTML 字符串。
- 屏幕端使用 X5 WebView 播放，低版本 Android 兼容到 `minSdk 21`。
- 屏幕端使用 WebSocket 接收实时控制指令，HTTP manifest 和 heartbeat 负责初始化、在线状态和兜底。
- APP 的服务器地址由屏幕端本地保存，官方服务器只是默认选项；切换到私有服务器后，后续绑定、manifest 和 WebSocket 都使用该服务器。

## 身份与权限

### User Token

注册用户身份，用于后台、Skill 和 MCP：

- 查看自己绑定的屏幕。
- 使用短期配对码绑定屏幕。
- 向自己名下屏幕投放自己的站点，或投放可公开访问的应用商城站点。
- 下发刷新、休眠、唤醒、软关机和截图指令。
- 解绑自己的屏幕。

匿名 session 不能绑定屏幕、投屏或请求截图。

### Device Token

屏幕设备身份，由服务器在配对成功后下发给 APP：

- 拉取自己的播放 manifest。
- 建立自己的 `/api/device/ws` WebSocket 控制通道。
- 上报心跳、设备信息、分辨率、横竖屏、系统版本、X5 版本等。
- 按后台指令回传截图或确认命令完成。

Device Token 不能发布内容、管理用户数据，也不能访问其他屏幕。

### Pairing Code

配对码只用于首次绑定或重新配对：

- 5 分钟有效。
- 一次性使用。
- 只显示在屏幕 APP 的绑定页。
- 用户必须登录后台或使用注册用户 Token 才能完成绑定。

## 当前能力

- 首次启动配置服务器地址，地址保存在本机。
- 播放页右上角用两个小圆点显示状态：左侧为网络，右侧为 WebSocket。
- 右上角状态点区域连续点击 5 次打开隐藏设置，设置浮层不会停止播放和 WebSocket。
- 隐藏设置可查看用户信息、屏幕信息、WebSocket 状态、设备型号、Android 版本、分辨率、横竖屏、WebView 控件、X5 版本和实际内核加载状态。
- 隐藏设置可切换服务器、清除本机绑定、返回播放。
- 后台和 Skill/MCP 可通过 WebSocket 实时下发刷新、休眠、唤醒、软关机和截图指令。
- 截图为后台指令触发，APP 不会主动上报截图。截图走 Android 系统截屏能力，首次截图需要屏幕端确认授权，图片会在 APP 端缩放压缩后再上传。
- 休眠和软关机显示为待机时钟页，保留当前日期并显示到秒。
- WebView 使用 `textZoom=100`、`useWideViewPort=true`、`loadWithOverviewMode=true`，用于缓解部分设备文字异常放大的问题。

## 服务端接口

用户侧接口，需要注册用户 Token 或登录会话：

```text
GET    /api/screens
POST   /api/screens/bind
POST   /api/screens/{screenId}/publish
POST   /api/screens/{screenId}/screenshot
GET    /api/screens/{screenId}/screenshot
POST   /api/screens/{screenId}/command
DELETE /api/screens/{screenId}
```

设备侧接口，需要 `Authorization: Device <token>`：

```text
POST /api/device/pairing/start
POST /api/device/pairing/complete
GET  /api/device/ws
GET  /api/device/manifest
POST /api/device/heartbeat
POST /api/device/screenshot
POST /api/device/command/ack
```

`/api/device/ws` 是屏幕控制主通道。连接成功后服务端会先推送一份 manifest，之后后台、Skill 或 MCP 下发投屏、刷新、截图、休眠等操作时会立即推送到该连接。截图图片本身仍通过 `POST /api/device/screenshot` 回传，避免大图占用 WebSocket 控制通道。

manifest 中的应用 URL 由 PagePilot 服务端生成。路径模式会跟随服务端当前入口，泛域名模式会使用后台配置的应用域名后缀。屏幕 APP 只加载 manifest 里的 URL，不自行拼接 `/agent/{code}/` 或泛域名。

如果 PagePilot 部署在 Nginx、宝塔或其他反向代理后面，必须转发 WebSocket Upgrade 头。例如：

```nginx
proxy_http_version 1.1;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
proxy_set_header Host $host;
```

## Skill 命令

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py --server https://pagepilot.example.com screen list
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen bind 123456 --name "大厅屏"
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen publish --screen screen_xxx --app my-landing
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen screenshot screen_xxx --output ./screen-shot.jpg
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen refresh screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen sleep screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen wake screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen shutdown screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen unbind screen_xxx
```

`--server` 表示 Skill/MCP 连接 PagePilot 控制面的入口。生产环境应使用屏幕和用户都能访问的公网地址；如果使用内网地址，路径模式下返回给屏幕的 App URL 也可能是内网地址。

`shutdown` 是软关机或黑屏待机。真实断电、开机自启、定时开关机通常依赖设备厂商能力、系统权限或 Device Owner/MDM 配置，不能假设所有硬件都支持。

## 本地构建

```powershell
cd apps\screen-app\app
$env:ANDROID_HOME="D:\Android\Sdk"
$env:ANDROID_SDK_ROOT="D:\Android\Sdk"
$env:JAVA_HOME="D:\jdk-21.0.2"
D:\Android\gradle\gradle-8.9\bin\gradle.bat :player:assembleDebug
```

生成物：

```text
apps/screen-app/app/player/build/outputs/apk/debug/player-debug.apk
```

## 发布建议

- Debug APK 只用于内测。
- 生产 APK 应使用正式签名，并在后台保留设备版本号。
- 大屏环境建议开启系统层开机自启、常亮、网络守护和应用守护；这些能力更适合放在系统设置、厂商管理平台或 MDM 中统一配置。
