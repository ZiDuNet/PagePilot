# 入门指南

本页从零完成一次本地运行、发布、更新和回滚。生产部署请切换到 [运维手册](OPERATIONS.md)。

## 1. 准备依赖

- Go 1.22+：服务端和 Go CLI。
- Node.js 22+、npm：重新构建用户端和管理后台时使用。
- Python 3.10+：运行 `pagep` Skill 时使用。
- Docker Compose：使用 Docker 或执行容器升级演练时使用。

## 2. 启动开发服务

开发模式会把数据放到仓库的 `data/`，冷却时间默认为 1 秒：

~~~bash
HOSTCTL_DEV=1 go run ./cmd/hostctl-server --addr 127.0.0.1:8787
~~~

访问：

- 用户端：<http://127.0.0.1:8787/>
- 发布页：<http://127.0.0.1:8787/deploy>
- 创作市场：<http://127.0.0.1:8787/market>
- 管理后台：<http://127.0.0.1:8787/admin>
- OpenAPI：<http://127.0.0.1:8787/openapi.json>

开发服务不应直接暴露到公网。生产环境必须设置固定的 `HOSTCTL_MASTER_KEY` 和认证策略，见 [配置参考](CONFIGURATION.md)。

## 3. 通过网页发布

在 `/deploy`：

1. 选择单文件或多文件。
2. 粘贴 HTML/Markdown，或选择目录、ZIP。
3. 填写有意义的标题和不超过 240 字的描述。
4. 选择 `public`（进入市场）或 `unlisted`（仅链接访问）。
5. 按需设置访问密码、分类和标签。
6. 发布后保存服务端返回的访问 URL、详情 URL 和版本 URL。

ZIP/目录入口会优先识别 `index.html`、`index.htm`、`README.md`、`README.markdown`。存在多个独立站点根目录时，先拆分项目或显式指定入口，不要反复上传同一个包。

## 4. 通过 CLI 发布

构建 CLI 并配置控制面：

~~~bash
go build -o bin/pagep ./cmd/hostctl
bin/pagep config set server https://pagepilot.example.com
bin/pagep token create local-dev --save
bin/pagep doctor
~~~

发布前先本地检查，`preflight` 不会创建 session、不上传文件，也不占用额度：

~~~bash
bin/pagep preflight ./site
bin/pagep deploy ./site \
  --code demo \
  --title "演示站点" \
  --description "可分享的演示页面"
~~~

更新已有站点应追加版本：

~~~bash
bin/pagep append demo ./site-v2 --description "第二个版本"
bin/pagep versions demo
bin/pagep current demo 2
~~~

`overwrite` 只用于明确替换一个未锁定版本；锁定后的版本不能覆盖或删除。需要给脚本使用时，添加 `--json`，并按 `errorCode`、`hint` 处理失败。

## 5. 匿名发布和认领

没有 Token 时，服务端会按需创建匿名 session。Python Skill 会将身份保存在本地；自动化场景建议使用注册用户 Token，或显式保存并发送 session ID：

~~~text
~/.pagep/session.json
~~~

匿名发布默认只能是 `unlisted`，默认每个 session 最多保有 5 个应用。注册或登录后可以认领：

~~~bash
bin/pagep claim-session <session-id>
~~~

或者调用 `POST /api/session/claim`。认领会把仍存在的站点迁移到注册用户；已认领 session 不能继续作为匿名身份发布。删除整个站点会立即释放一个应用额度，删除单个版本不会恢复站点额度。

## 6. 查看历史版本

版本列表返回每个版本的 `url`、`pathUrl`、`domainUrl`：

~~~bash
bin/pagep versions demo
~~~

在路径模式中，历史版本是：

~~~text
https://pagepilot.example.com/agent/demo/versions/2/
~~~

在泛域名模式中，历史版本是：

~~~text
https://demo.pg.example.com/versions/2/
~~~

切换当前版本只影响站点主 URL；显式历史 URL 仍指向指定版本。历史链接使用当前运行时的 URL 配置生成，因此切换 URL 模式时，列表中的新链接会随新配置变化；原先已经分享出去的路径链接仍然可以作为兼容入口，前提是反向代理仍转发 `/agent/*`。泛域名 DNS、证书和 Nginx 配置见 [应用链接模式](../deploy/APP_URL_MODE.md)。

## 7. 使用创作市场

公开作品可在 `/market` 搜索、点赞、收藏和查看详情。详情页会返回 Bundle 类型、入口文件、文件树、源码下载策略和复用参数。复用时：

- 新建二创：不传已有 code，服务端生成新的站点。
- 更新自己的作品：明确填写已有 code，作为新版本追加。
- 加密、下架或策略禁止的作品，普通用户不能下载源码或复用；访问密码只授权浏览。

## 8. 常见结果

每次发布或版本操作都应优先使用服务端返回的链接，不要按本地端口、代理域名或 `code` 自行拼接。典型响应字段：

~~~json
{
  "success": true,
  "code": "demo",
  "url": "https://demo.pg.example.com/",
  "pathUrl": "https://pagepilot.example.com/agent/demo/",
  "domainUrl": "https://demo.pg.example.com/",
  "detailUrl": "https://demo.pg.example.com/",
  "versionUrl": "https://demo.pg.example.com/versions/2/"
}
~~~

接口失败时保存 `requestId`，把 `hint` 和脱敏后的请求信息一起交给管理员。错误结构见 [API 与集成](API_INTEGRATION.md)。

## 9. 下一步

- 需要改服务器参数：看 [配置参考](CONFIGURATION.md)。
- 需要接入自己的 Agent：看 [API 与集成](API_INTEGRATION.md) 和 Skill [SKILL.md](../skill/hostctl-deploy/SKILL.md)。
- 需要上公网：先看 [运维手册](OPERATIONS.md)，再按需配置 [泛域名模式](../deploy/APP_URL_MODE.md)。
