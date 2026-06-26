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
| `domain` | 发布接口主返回 `https://{code}.{suffix}/` 链接 | 已配置泛解析和证书，希望隔离上传 JS |
| `dual` | 同时支持路径和泛域名，发布接口主返回仍按路径模式生成 | 灰度迁移、兼容历史链接 |

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
  --domain-suffix pagepilot.example.com \
  --scheme https \
  --port 1143
```

也可以通过环境变量设置初始值：

```yaml
HOSTCTL_APP_URL_MODE: "domain"
HOSTCTL_APP_DOMAIN_SUFFIX: "pagepilot.example.com"
HOSTCTL_APP_URL_SCHEME: "https"
HOSTCTL_APP_URL_PORT: ""
```

带外部端口的示例：

```yaml
HOSTCTL_APP_URL_MODE: "domain"
HOSTCTL_APP_DOMAIN_SUFFIX: "pagepilot.example.com"
HOSTCTL_APP_URL_SCHEME: "https"
HOSTCTL_APP_URL_PORT: "1143"
```

## 访问入口与应用域名

PagePilot 没有入口域名配置项。首页、后台提示、Skill/MCP 下载说明、OpenAPI、二维码，以及路径模式 `/agent/{code}/` 链接都跟随当前访问 PagePilot 的域名或 IP。

应用链接规则只决定发布后的应用是否额外使用 `https://{code}.{suffix}/` 泛域名。也就是说，只有启用 `domain` 或 `dual` 模式时，才需要配置 `HOSTCTL_APP_DOMAIN_SUFFIX`、`HOSTCTL_APP_URL_SCHEME` 和 `HOSTCTL_APP_URL_PORT`。

发布接口返回的 `url`、`detailUrl` 和 `versionUrl` 是权威访问地址：

- `path`：返回当前主站入口下的 `/agent/{code}/`。
- `domain`：返回后台配置的泛域名应用地址，例如 `https://demo.apps.example.com/`。
- `dual`：路径和泛域名都可访问，但主返回值仍是 `/agent/{code}/`，便于兼容历史链接。

浏览器页面会把当前 `origin` 发给后端；Skill、MCP 和 CLI 使用 `--server`、`HOSTCTL_SERVER` 或保存的服务器地址作为当前入口。它们不应该自行拼接最终应用 URL，而应该展示服务端返回的 URL。

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

如果你像 `https://pagepilot.example.com:1143` 这样把 `1143` 当作外部 HTTPS 端口使用：

```nginx
server {
    listen 1143 ssl http2;
    server_name pagepilot.example.com *.pagepilot.example.com;

    ssl_certificate     /etc/nginx/certs/pagepilot.example.com/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/pagepilot.example.com/privkey.pem;

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

- `server_name` 同时包含 PagePilot 入口域名和对应的应用泛域名。
- `proxy_set_header Host $host` 必须保留，PagePilot 需要用 Host 判断当前访问的是哪个应用。
- `X-Forwarded-Host` 和 `X-Forwarded-Proto` 用于“按当前访问域名生成”模式。
- 如果 Skill/MCP 使用内网地址调用 API，路径模式返回的 URL 也可能是内网地址；生产环境应把 `--server` / `HOSTCTL_SERVER` 设置为用户可访问的公网入口。
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

泛域名模式仍然是更清晰的隔离方案：每个应用运行在自己的子域名，平台 API 保持在当前 PagePilot 入口；应用子域名只允许读取静态内容，以及提交本应用的访问密码校验，其它平台 API 不会在应用子域名开放。
