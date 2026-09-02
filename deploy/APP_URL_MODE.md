# 应用访问地址模式

PagePilot 的“应用链接规则”只决定发布接口、版本列表、二维码和前端展示哪个 URL 作为主链接。它不会自动创建 DNS 记录、申请证书或修改 Nginx；启用泛域名模式前，必须先完成网络层配置。

## 先确认三件事

以主站 `pagepilot.example.com`、应用后缀 `pg.example.com`、应用 code `demo` 为例：

~~~text
主站入口：      https://pagepilot.example.com
应用入口：      https://demo.pg.example.com
泛解析记录：    *.pg.example.com -> 反向代理公网 IP
TLS 证书：       pagepilot.example.com + *.pg.example.com
PagePilot：      反向代理 -> 127.0.0.1:8787
~~~

以下三项缺一不可：

1. DNS：添加 `*.pg.example.com` 的 A/AAAA 或 CNAME，指向接收主站流量的同一台 Nginx/Caddy。无需为每个 code 添加记录。
2. TLS：证书必须覆盖主站和 `*.pg.example.com`。泛域名只覆盖一层子域名，不覆盖 `pg.example.com` 本身，也不覆盖 `a.demo.pg.example.com`；Let's Encrypt 泛域名通常需要 DNS-01 验证。
3. 反向代理：主站和泛域名由同一个站点转发到 PagePilot，并保留外部 Host、协议和端口。`/api/device/ws` 还需要 WebSocket Upgrade。

如果只配置了 PagePilot 后台而没有完成这些网络条件，域名模式会生成看似正确但打不开的链接；HTTPS 市场页面还可能因为应用 URL 变成 HTTP 而触发 Mixed Content。

## 三种模式

| 模式 | 主 URL | 历史版本 | 说明 |
| --- | --- | --- | --- |
| `path` | `https://pagepilot.example.com/agent/demo/` | `https://pagepilot.example.com/agent/demo/versions/2/` | 默认，不需要泛域名。 |
| `domain` | `https://demo.pg.example.com/` | `https://demo.pg.example.com/versions/2/` | 主链接使用独立子域名。 |
| `dual` | 同时返回 path 和 domain | 同时返回两种历史链接 | 迁移期间同时保留两套地址。 |

说明：

- 环境变量 `HOSTCTL_APP_DOMAIN_SUFFIX`（后台字段 `appDomainSuffix`）只填 `pg.example.com`，不要填写 `*.pg.example.com`；服务端会规范化前后点号和 `*.`，但配置文件仍应保持清晰。
- `appURLScheme` 必须与浏览器看到的外部协议一致。TLS 在代理终止时，PagePilot 仍应设置为 `https`。
- `appURLPort` 只填写外部非标准端口，例如 `1143`；不要填写容器内部监听端口 `8787`。
- `url` 是当前模式的主链接；`pathUrl` 和 `domainUrl` 是显式变体。调用方应使用服务端返回值，不要自行拼接。
- 版本列表和新发布响应按当前配置重新生成 URL；版本数据不会记住“创建时的 URL 模式”。

## DNS 和证书

DNS 示例：

~~~text
pagepilot.example.com.  A     203.0.113.10
*.pg.example.com.       A     203.0.113.10
~~~

也可以使用指向主站的 CNAME（取决于 DNS 服务商是否允许在该位置使用 CNAME）。确认泛解析已经生效：

~~~bash
dig +short pagepilot.example.com
dig +short demo.pg.example.com
# 或
nslookup demo.pg.example.com
~~~

证书至少要包含：

~~~text
pagepilot.example.com
*.pg.example.com
~~~

只给主站申请证书，或只给 `pg.example.com` 申请证书，都不能覆盖 `demo.pg.example.com`。

## Nginx 反向代理

下面的配置把主站和所有应用子域名转到同一个 PagePilot。`$http_host` 会保留客户端请求中的端口，比 `$host` 更适合非标准端口；公网 Nginx 必须覆盖客户端可能伪造的转发头。

在 `http {}` 级别添加一次：

~~~nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}
~~~

在 `server {}` 中：

~~~nginx
server {
    listen 443 ssl;
    server_name pagepilot.example.com *.pg.example.com;

    # ssl_certificate /etc/letsencrypt/live/pagepilot.example.com/fullchain.pem;
    # ssl_certificate_key /etc/letsencrypt/live/pagepilot.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_http_version 1.1;

        proxy_set_header Host $http_host;
        proxy_set_header X-Forwarded-Host $http_host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
~~~

不要只配置 `/api` 或 `/agent` 白名单。首页、后台、市场、Skill、屏幕、API、当前应用和历史应用都由 PagePilot 路由，统一 `location /` 最不容易漏路径。

### 外部使用 1143 端口

如果用户访问的是 `https://pagepilot.example.com:1143`，代理需要监听 1143，证书仍然覆盖两个域名，PagePilot 配置使用：

~~~nginx
server {
    listen 1143 ssl;
    server_name pagepilot.example.com *.pg.example.com;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Forwarded-Host $http_host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Port 1143;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
~~~

对应设置：

~~~text
HOSTCTL_APP_URL_MODE=domain
HOSTCTL_APP_DOMAIN_SUFFIX=pg.example.com
HOSTCTL_APP_URL_SCHEME=https
HOSTCTL_APP_URL_PORT=1143
~~~

`X-Forwarded-Port` 是补充信息；当前 URL 生成主要依赖带端口的 `Host`/`X-Forwarded-Host`，所以不要把 `$host` 用在这个场景。

## Caddy

Caddy 会自动处理普通 HTTP/2 和 WebSocket 代理。域名模式示例：

~~~caddyfile
pagepilot.example.com, *.pg.example.com {
    reverse_proxy 127.0.0.1:8787
}
~~~

Caddy 自动申请泛域名证书通常也需要 DNS provider 和 DNS-01；请按实际 DNS 服务商配置，确认最终证书包含主站和应用后缀。

## 修改 PagePilot 配置

### 后台

管理员进入“运行设置 -> 应用链接规则”，选择 `path`、`domain` 或 `dual`，填写应用域名后缀、协议和外部端口。保存后新请求立即使用新规则；切换前先确保 DNS、证书和代理已就绪。

### 环境变量

适合容器或 systemd：

~~~bash
HOSTCTL_APP_URL_MODE=domain
HOSTCTL_APP_DOMAIN_SUFFIX=pg.example.com
HOSTCTL_APP_URL_SCHEME=https
HOSTCTL_APP_URL_PORT=
~~~

修改环境变量后重启服务。完整环境变量说明见 [../docs/CONFIGURATION.md](../docs/CONFIGURATION.md)。

### pagep Skill

Python Skill 提供服务端配置命令，要求当前 Token 有管理员权限：

~~~bash
python scripts/pagep.py config set-app-url \
  --server https://pagepilot.example.com \
  --mode domain \
  --domain-suffix pg.example.com \
  --scheme https
~~~

该命令修改的是服务端运行设置；`pagep config set server` 只保存客户端控制面地址，两者不要混淆。

## 验证当前和历史链接

假设已有站点 `demo`、版本 2：

~~~bash
# 主站和泛域名 DNS
dig +short pagepilot.example.com
dig +short demo.pg.example.com

# 通过域名访问当前应用
curl -fsS https://demo.pg.example.com/ >/dev/null

# 访问历史版本
curl -fsS https://demo.pg.example.com/versions/2/ >/dev/null

# 不改本机 DNS，直接把域名解析到代理 IP
curl --resolve demo.pg.example.com:443:203.0.113.10 \
  -fsS https://demo.pg.example.com/ >/dev/null
curl --resolve demo.pg.example.com:443:203.0.113.10 \
  -fsS https://demo.pg.example.com/versions/2/ >/dev/null
~~~

非标准端口使用 `:1143`：

~~~bash
curl --resolve demo.pg.example.com:1143:203.0.113.10 \
  -fsS https://demo.pg.example.com:1143/ >/dev/null
~~~

同时检查：

- 浏览器地址栏、接口返回和 iframe 的协议是否都是 `https`。
- `url`、`domainUrl` 和 `versionDomainUrl` 是否带正确端口。
- Nginx access log 中 Host 是否分别出现主站和 `demo.pg.example.com`。
- `/api/device/ws` 是否返回 WebSocket 握手，而不是 400/426。
- 未知 code 应返回 404，不应把任意子域名当成有效站点。

## 切换、回滚和历史链接

- 从 `path` 切到 `dual`：先完成 DNS/证书/代理，再保存配置；新响应同时提供两种地址。
- 从 `dual` 切到 `domain`：确认所有消费者已使用 `domainUrl` 或 `versionDomainUrl`，再停止对外宣传路径地址。
- 回滚到 `path`：把模式改回 `path`，保留 DNS 和代理也不会影响路径链接。
- 旧路径链接是否继续可用，取决于代理是否仍转发 `/agent/*`；切换模式不会删除版本。
- 历史版本必须使用带版本号的地址：路径模式 `/agent/{code}/versions/{version}/`，泛域名模式 `https://{code}.{suffix}/versions/{version}/`。删除版本或删除整个站点后，对应地址才会失效。

## 常见错误

| 现象 | 原因和处理 |
| --- | --- |
| `demo.pg.example.com` DNS 不存在 | 没有 `*.pg.example.com` 记录，或 DNS 尚未生效。 |
| 证书报错 | 证书缺少 `*.pg.example.com` 或端口使用了另一套证书。 |
| 返回 PagePilot 首页而不是应用 | 泛域名没有进入同一个 Nginx `server_name`，或 `Host` 被改成内网地址。 |
| 返回链接带 `http` | `appURLScheme`、`X-Forwarded-Proto` 或代理终止 TLS 配置不一致。 |
| HTTPS 市场 iframe 被拦截 | 应用 URL 使用了 HTTP，修正 scheme/port 和反代头后重新请求市场。 |
| 历史版本 404 | 代理漏掉 `/versions/*`，或版本已删除/文件存储丢失。 |
| 屏幕控制断开 | 代理没有转发 `/api/device/ws` 的 HTTP/1.1 Upgrade。 |
