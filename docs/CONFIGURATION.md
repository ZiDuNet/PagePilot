# 配置参考

PagePilot 的启动环境变量、后台运行设置和 URL 规则共同决定服务行为。本页列出当前代码支持的配置；修改环境变量后需要重启服务。

## 配置来源和优先级

推荐只使用 `HOSTCTL_` 前缀变量。当前实现仍兼容没有前缀的旧变量（例如 `APP_URL_MODE`），并且旧变量读取阶段在 `HOSTCTL_*` 之后执行；同名变量同时存在时，旧变量可能覆盖新变量。迁移时应删除旧变量，避免来源不明。

- 启动环境变量：服务启动时读取，适合部署、存储、密钥和默认限制。
- 后台“运行设置”：管理员通过 `GET/PUT /api/config` 管理应用 URL、CORS、iframe、限额和内容注入等可运行配置，并查看邮箱/存储状态；保存后立即用于新请求，具体可编辑项以后台页面和接口为准。
- CLI/Skill 的 `pagep config`：只保存客户端的 PagePilot server 和 Token，不修改服务端配置。

## 服务和文件

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HOSTCTL_HTTP_ADDR` | `127.0.0.1:8787` | 服务监听地址；Docker Compose 覆盖为 `0.0.0.0:8787`。 |
| `HOSTCTL_HOSTED_DIR` | `/var/www/hosted` | 本地静态文件根目录。开发模式默认 `data/hosted`。 |
| `HOSTCTL_DB_PATH` | `/var/lib/hostctl/hostctl.db` | SQLite 路径。开发模式默认 `data/hostctl.db`。 |
| `REQUIRE_AUTH` | 生产入口应为 `true` | 生产环境启用认证；开发会话不应当作为公网安全边界。 |
| `HOSTCTL_MASTER_KEY` | 无 | 加密访问密码、设备授权等敏感数据的固定主密钥。生产必填，必须解码为 32 字节；升级和重启必须沿用原值。 |
| `HOSTCTL_ADMIN_USERNAME` | 无 | 空数据库首次启动时创建的管理员用户名，不覆盖已有账号。 |
| `HOSTCTL_ADMIN_PASSWORD` | 无 | 空数据库首次启动时创建的管理员密码，不覆盖已有账号。 |

## 应用 URL

| 变量 | 可选值 | 说明 |
| --- | --- | --- |
| `HOSTCTL_APP_URL_MODE` | `path`、`domain`、`dual` | 路径、泛域名或同时返回两种链接；默认 `path`。 |
| `HOSTCTL_APP_DOMAIN_SUFFIX` | `pg.example.com` | 泛域名后缀，不要写 `*.`；仅 `domain/dual` 使用。 |
| `HOSTCTL_APP_URL_SCHEME` | `http`、`https` | 对外协议；生产反向代理通常为 `https`。 |
| `HOSTCTL_APP_URL_PORT` | 空或 `1-65535` | 对外非标准端口，例如 `1143`；不要填容器内部 8787。 |

示例：

~~~bash
HOSTCTL_APP_URL_MODE=domain
HOSTCTL_APP_DOMAIN_SUFFIX=pg.example.com
HOSTCTL_APP_URL_SCHEME=https
HOSTCTL_APP_URL_PORT=
~~~

如果外部入口是 `https://pagepilot.example.com:1143`，则设置 `HOSTCTL_APP_URL_PORT=1143`，并让代理的 `Host`/`X-Forwarded-Host` 保留 `:1143`。启用泛域名前，必须先完成 `*.pg.example.com` DNS、覆盖主站和泛域名的 TLS 证书，以及接收主站和泛域名的同一反向代理。完整示例见 [APP_URL_MODE.md](../deploy/APP_URL_MODE.md)。

## CORS、iframe 和内容注入

| 变量/设置 | 说明 |
| --- | --- |
| `HOSTCTL_CORS_ALLOW_ORIGINS` | 逗号分隔的完整 `http(s)://host[:port]` 白名单；留空关闭浏览器跨域 API。不要使用 `*`。 |
| `HOSTCTL_EMBED_POLICY` | `any`、`self`、`allowlist`、`deny`；控制托管应用的 CSP `frame-ancestors`。 |
| `HOSTCTL_EMBED_ALLOW_ORIGINS` | `allowlist` 模式下的完整 origin 列表，不带路径。 |
| 后台“内容注入” | 分别对主站和托管应用设置 head、body 开始、body 结束代码；服务端会清理 NUL 字符并按站点安全策略输出。 |

CORS 只决定另一个网页能否用 fetch/XHR 调用 API，不决定 iframe 是否能显示。iframe 是否允许嵌入由 Embed Policy 决定。

## 上传、配额和冷却

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HOSTCTL_MAX_SINGLE_FILE_BYTES` | `1048576` | 单文件最大 1 MiB。 |
| `HOSTCTL_MAX_SITE_TOTAL_BYTES` | `10485760` | 整站（解包后）最大 10 MiB。 |
| `HOSTCTL_MAX_FILES_PER_SITE` | `100` | 单站点最大文件数。 |
| `HOSTCTL_COOLDOWN_SECONDS` | `10` | 发布冷却时间，范围 0-3600；开发模式无显式覆盖时为 1。 |
| `HOSTCTL_ANONYMOUS_DEPLOY_LIMIT` | `5` | 每个匿名身份保有的应用数；`-1` 表示不限制。 |
| 后台运行设置 | 以接口返回为准 | 注册用户部署上限、匿名额度等可由管理员调整。 |

额度按“保有的站点数”计算：新建站点占用一个额度，追加版本、下线或隐藏不额外占用；删除整个站点立即释放一个额度，删除单个版本不会恢复额度。

## 注册和邮件

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HOSTCTL_ALLOW_REGISTRATION` | `true` | 是否开放公开注册。 |
| `HOSTCTL_EMAIL_VERIFICATION_ENABLED` | `false` | 是否要求邮箱验证码注册。启用时必须配置 SMTP host 和 from。 |
| `HOSTCTL_SMTP_HOST` | 空 | SMTP 主机。 |
| `HOSTCTL_SMTP_PORT` | 空 | SMTP 端口，例如 587。 |
| `HOSTCTL_SMTP_USERNAME` | 空 | SMTP 用户名。 |
| `HOSTCTL_SMTP_PASSWORD` | 空 | SMTP 密码。 |
| `HOSTCTL_SMTP_FROM` | 空 | 验证邮件发件人。 |
| `HOSTCTL_SMTP_SECURE` | `starttls` | `starttls`、`tls` 或 `none`，按邮件服务商要求设置。 |

## 文件存储

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HOSTCTL_STORAGE_BACKEND` | `local` | `local` 或 `oss`。 |
| `HOSTCTL_OSS_PROVIDER` | `aliyun` | 当前 OSS 适配器标识。 |
| `HOSTCTL_OSS_ENDPOINT` | 空 | OSS endpoint，建议写完整 HTTPS 地址。 |
| `HOSTCTL_OSS_BUCKET` | 空 | Bucket。 |
| `HOSTCTL_OSS_ACCESS_KEY_ID` | 空 | 访问密钥 ID。 |
| `HOSTCTL_OSS_ACCESS_KEY_SECRET` | 空 | 访问密钥 Secret。 |
| `HOSTCTL_OSS_PREFIX` | 空 | 对象前缀，例如 `prod/pagepilot`。 |
| `HOSTCTL_OSS_PUBLIC_BASE_URL` | 空 | 已配置公开对象域名时的可选前缀。 |

选择 `oss` 时 endpoint、bucket、access key ID 和 secret 都必须存在。每个版本记录自己的存储归属，因此切换后历史版本仍按原记录读取；切换不会自动迁移已有文件，OSS 缺失对象时会回退本地历史目录。

## 开发和生产建议

~~~bash
# 本地
HOSTCTL_DEV=1 go run ./cmd/hostctl-server --addr 127.0.0.1:8787

# 生产（示意，密钥应通过受限环境文件或 Secret 注入）
HOSTCTL_MASTER_KEY=<固定且不可轮换的值>
HOSTCTL_APP_URL_MODE=domain
HOSTCTL_APP_DOMAIN_SUFFIX=pg.example.com
HOSTCTL_APP_URL_SCHEME=https
REQUIRE_AUTH=true
~~~

不要把 `.env`、SQLite、`hosted` 目录、Token 或管理员密码提交到 Git。配置完整性可以通过后台“运行设置”、`pagep doctor` 和 `GET /api/config`（管理员会话）检查。
