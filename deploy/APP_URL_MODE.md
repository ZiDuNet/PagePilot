# PagePilot 应用访问地址模式

PagePilot 默认使用路径模式访问应用，兼容已经上线的地址：

```text
https://pagepilot.example.com/agent/{code}/
```

如果你有泛域名解析和泛域名证书，也可以在后台切换到泛域名模式：

```text
https://{code}.pagepilot.example.com/
```

`/agent/{code}/` 会继续保留，建议把它作为默认兼容入口；泛域名模式更适合隔离用户上传的 HTML/JS。

## 三种应用模式

| 模式 | 含义 | 适合场景 |
|---|---|---|
| `path` | 只生成 `/agent/{code}/` 链接 | 默认模式，最少配置 |
| `domain` | 生成 `https://{code}.{suffix}/` 链接 | 已配置泛解析和证书，希望隔离上传 JS |
| `dual` | 同时支持路径和泛域名，主链接仍按路径模式生成 | 灰度迁移、兼容历史链接 |

后台入口：`/admin` -> 运行设置 -> 应用链接规则。

Skill/CLI 入口：

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py \
  config set-app-url \
  --mode domain \
  --domain-suffix pagepilot.example.com \
  --scheme https
```

如果你的外部 HTTPS 端口不是标准 `443`，例如使用 `1143` 对外提供 HTTPS：

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py \
  config set-app-url \
  --mode domain \
  --domain-suffix pagepilot.dell.4dbim.cc \
  --scheme https \
  --port 1143
```

也可以通过环境变量设置初始值：

```yaml
HOSTCTL_PUBLIC_BASE_URL: "https://pagepilot.example.com"
HOSTCTL_APP_URL_MODE: "domain"
HOSTCTL_APP_DOMAIN_SUFFIX: "pagepilot.example.com"
HOSTCTL_APP_URL_SCHEME: "https"
HOSTCTL_APP_URL_PORT: ""
```

带外部端口的示例：

```yaml
HOSTCTL_PUBLIC_BASE_URL: "https://pagepilot.dell.4dbim.cc:1143"
HOSTCTL_APP_URL_MODE: "domain"
HOSTCTL_APP_DOMAIN_SUFFIX: "pagepilot.dell.4dbim.cc"
HOSTCTL_APP_URL_SCHEME: "https"
HOSTCTL_APP_URL_PORT: "1143"
```

## 主站域名与应用域名

后台“运行设置”里有两类地址，不要混在一起：

- 主站链接来源：决定首页、后台提示、Skill/MCP 下载说明、OpenAPI、二维码，以及路径模式 `/agent/{code}/` 这些链接用哪个主站域名。
- 应用链接规则：决定发布后的应用是使用 `/agent/{code}/`，还是使用 `https://{code}.{suffix}/` 泛域名。

`Public Base URL` 是固定主站地址。默认模式下所有主站外链都按它生成。如果希望同一套 PagePilot 能通过当前访问域名生成链接，可以把“主站链接来源”切到“按当前访问域名生成”，或设置：

```yaml
HOSTCTL_PUBLIC_URL_MODE: "request_host"
```

此时 PagePilot 会读取请求里的 `Host`、`X-Forwarded-Host` 和 `X-Forwarded-Proto` 生成主站链接。这个开关不影响应用泛域名；泛域名仍然只看 `HOSTCTL_APP_URL_MODE`、`HOSTCTL_APP_DOMAIN_SUFFIX`、`HOSTCTL_APP_URL_SCHEME` 和 `HOSTCTL_APP_URL_PORT`。

## Docker 与外部 Nginx

Docker 不需要为每个应用单独暴露端口，也不需要为每个 `code` 单独配置反向代理。外层 DNS 和 Nginx 接收所有泛域名后，统一转发到 PagePilot 容器即可。

DNS 只需要两类记录：

```text
pagepilot.example.com       A/AAAA -> 你的服务器
*.pagepilot.example.com     A/AAAA -> 你的服务器
```

Nginx 标准 443 示例：

```nginx
server {
    listen 443 ssl http2;
    server_name pagepilot.example.com *.pagepilot.example.com;

    ssl_certificate     /etc/nginx/certs/pagepilot.example.com/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/pagepilot.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

如果你像 `https://pagepilot.dell.4dbim.cc:1143` 这样把 `1143` 当作外部 HTTPS 端口使用：

```nginx
server {
    listen 1143 ssl http2;
    server_name pagepilot.dell.4dbim.cc *.pagepilot.dell.4dbim.cc;

    ssl_certificate     /etc/nginx/certs/pagepilot.dell.4dbim.cc/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/pagepilot.dell.4dbim.cc/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Port 1143;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

关键点：

- `server_name` 同时包含主域名和 `*.主域名`。
- `proxy_set_header Host $host` 必须保留，PagePilot 需要用 Host 判断当前访问的是哪个应用。
- `X-Forwarded-Host` 和 `X-Forwarded-Proto` 用于“按当前访问域名生成”模式。
- Docker 容器仍然只监听 `8787`，不需要为每个泛域名单独配置。
- 如果只使用路径模式，不需要泛解析，也不需要泛域名证书。

## CORS 与 iframe 嵌入

CORS 白名单只决定外部网站能否用浏览器 `fetch` / XHR 调用 PagePilot API。它不会控制其它网站是否可以 iframe 嵌入应用页面。

iframe 嵌入由后台“运行设置 -> 跨域与嵌入 -> iframe 嵌入”单独控制：

- 允许任意网站嵌入：不下发 `frame-ancestors` 限制。
- 只允许本站嵌入：下发 `frame-ancestors 'self'`。
- 本站 + 白名单来源：下发 `frame-ancestors 'self' https://portal.example.com ...`。
- 禁止被任何网站嵌入：下发 `frame-ancestors 'none'`。

白名单来源必须是完整 origin，例如 `https://portal.example.com` 或 `https://display.example.com:1143`，不要带路径。

## 安全说明

用户上传的 HTML/JS 是不可信内容。PagePilot 会在托管内容响应上加安全头，尤其是 CSP `sandbox`，并且不授予 `allow-same-origin`。这能降低路径模式下用户 JS 与平台同源带来的风险。

泛域名模式仍然是更清晰的隔离方案：每个应用运行在自己的子域名，平台 API 保持在主域名。应用子域名只允许读取静态内容，以及提交本应用的访问密码校验；其它平台 API 不会在应用子域名开放。
