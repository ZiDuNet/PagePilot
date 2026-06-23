# PagePilot Screen App

本目录用于承载 PagePilot 屏幕端 APP 的代码与说明。屏幕端 APP 面向广告屏、安卓屏、安卓盒子或其他可安装播放器的硬件环境，用于接收 PagePilot 后台发布的内容并全屏播放。

## 核心目标

- 一个注册用户可以绑定多个硬件屏幕。
- 屏幕 APP 只作为播放终端，不持有用户 Token。
- Skill 可以发布内容到屏幕，但仅限注册用户 Token。
- 匿名用户不能调用投屏接口；匿名内容需要先归属到注册用户后才能投屏。
- 设备绑定、内容发布、设备播放三件事在权限上彻底分离。

## 身份与权限

### User Token

注册用户身份。用于后台管理、Skill 发布、绑定屏幕、解绑屏幕、查看屏幕状态。

允许：

- 查看自己绑定的屏幕列表。
- 使用短期配对码绑定屏幕。
- 向自己名下的屏幕发布内容。
- 解绑或重命名自己的屏幕。

不允许：

- 访问其他用户的屏幕。
- 冒充设备拉取播放清单。

### Device Token

屏幕设备身份。由服务器在配对成功后下发给屏幕 APP，长期有效但必须可吊销、可轮换。

允许：

- 拉取自己的播放清单。
- 上报心跳、版本、播放状态。
- 上报错误信息或截图。

不允许：

- 发布内容。
- 修改用户数据。
- 查看其他屏幕或用户信息。

### Pairing Code

短期一次性配对码。只用于首次绑定屏幕，不是 Token，也不是长期权限。

建议规则：

- 5 分钟有效。
- 一次性使用。
- 只显示在屏幕 APP 首次启动或重新配对页面。
- 用户必须登录后才能使用配对码绑定屏幕。

## MVP 范围

第一阶段建议只做最小可用闭环：

1. 屏幕 APP 首次启动显示配对码和二维码。
2. 注册用户在后台或 Skill 中输入配对码完成绑定。
3. 用户可以向指定屏幕发布一个 PagePilot 应用。
4. 屏幕 APP 通过 Device Token 拉取播放清单并全屏展示。
5. 屏幕 APP 定时上报心跳，后台显示在线状态。
6. 屏幕 APP 缓存最后一次成功内容，断网时继续播放。

暂不做复杂排期、多屏分组、远程截图审核、远程系统控制等高级能力。

## 后端接口草案

用户侧接口，必须是注册用户 Token：

```text
GET    /api/screens
POST   /api/screens/bind
POST   /api/screens/{screen_id}/publish
PATCH  /api/screens/{screen_id}
DELETE /api/screens/{screen_id}
```

设备侧接口，必须是 Device Token：

```text
POST /api/device/pairing/start
GET  /api/device/manifest
POST /api/device/heartbeat
POST /api/device/playback-events
```

Skill 侧命令草案：

```bash
pagepilot screen list
pagepilot screen bind <pairing_code>
pagepilot screen publish --screen <screen_id> --app <slug>
pagepilot screen publish --screen <screen_id> --path ./dist --name "屏幕展示"
pagepilot screen status <screen_id>
pagepilot screen unbind <screen_id>
```

## 目录规划

后续实现时建议按以下结构展开：

```text
apps/screen-app/
  README.md
  app/            # 屏幕端 APP 源码
  docs/           # 设备协议、部署说明、调试说明
  scripts/        # 打包、签名、调试脚本
```

技术路线可以先按 Android WebView 壳 + PagePilot 播放清单实现。这样可以复用现有 HTML 应用能力，同时保留后续做原生缓存、开机自启、远程日志和设备守护的空间。
