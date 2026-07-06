# PagePilot 运行时大重构实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框语法来跟进进度。

**目标：** 把 PagePilot 的 HTML/Markdown/ZIP 发布链路重构为可维护、可审计、可搜索、CLI/MCP/Skill 对齐的应用运行时。

**架构：** 后端新增 `internal/bundle`、`internal/render`、`internal/hosted`、`internal/audit` 四个边界清晰的模块，原有 `internal/deploy`、`internal/api`、`internal/store` 只保留编排职责。前端用户端和后台继续使用现有 React/Vite 工程，在市场复用、文件树、审计日志和运行设置页上补齐能力。

**技术栈：** Go 1.22、SQLite/FTS5、React 18、Vite、PagePilot CLI、MCP stdio server、PagePilot Skill。

---

## 文件结构

- 创建 `internal/bundle/bundle.go`：解析目录/ZIP、识别真实根目录、入口文件和 bundle 类型。
- 创建 `internal/bundle/bundle_test.go`：覆盖嵌套 ZIP、Markdown 包、批量包拒绝、路径穿越和友好错误。
- 修改 `internal/deploy/deployer.go`：调用 `bundle` 模块，保留 Deploy 编排、权限、写文件、元数据写入。
- 修改 `internal/deploy/zip_test.go`：迁移到新 bundle 行为和 deploy 兼容测试。
- 创建 `internal/render/markdown.go`：Markdown 渲染、HTML sanitizer、代码块/公式/图表占位和资源注入。
- 创建 `internal/render/cache.go`：按版本 hash 计算渲染缓存 key。
- 创建 `internal/render/markdown_test.go`：覆盖图片、链接、代码块、Mermaid、KaTeX、XSS。
- 创建 `internal/hosted/csp.go`：生成 admin/user/hosted-html/hosted-markdown/preview profile。
- 创建 `internal/hosted/html.go`：HTML 兼容脚本、base 注入、托管响应辅助。
- 创建 `internal/hosted/csp_test.go`：覆盖 sandbox、frame-ancestors、Markdown nonce 和预览 profile。
- 创建 `internal/audit/audit.go`：审计事件类型和记录辅助。
- 修改 `internal/store/schema.sql`：增加 `site_search_fts`、`audit_logs`、`render_cache`、`version_bundles`。
- 修改 `internal/store/store.go`：增加 FTS、审计、渲染缓存、bundle 元数据接口。
- 修改 `internal/store/sqlite.go`：增加迁移、FTS 同步、审计写入、缓存读写、文件树查询。
- 修改 `internal/api/types.go`、`internal/api/admin_types.go`：增加 bundle、搜索、审计、multipart 返回字段类型。
- 修改 `internal/api/server.go`：接入新渲染/CSP/审计模块，新增 multipart 上传接口和审计日志 API。
- 修改 `internal/api/openapi.go`：补齐新接口和 schema。
- 修改 `cmd/hostctl/main.go`：目录/ZIP 优先走 multipart，保留 JSON/base64 fallback。
- 修改 `cmd/hostctl-mcp/main.go`：工具描述与参数同步，补模板复用、文件树、审计可见能力。
- 修改 `skill/hostctl-deploy/SKILL.md`：重写 Agent 操作规约。
- 修改 `skill/hostctl-deploy/scripts/hostctl_deploy.py`：优先 multipart，保留 JSON 兼容。
- 修改 `frontend/user/src/App.tsx`、`frontend/user/src/types.ts`、`frontend/user/src/styles.css`：市场搜索、复用抽屉、详情页展示 bundle/渲染信息。
- 修改 `frontend/admin/src/App.tsx`、`frontend/admin/src/styles.css`：站点文件树、安全模式、审计日志、运行设置和 Skill/MCP 文案。
- 修改 `README.md`、`deploy/DOCKER.md`、`deploy/APP_URL_MODE.md`、`docs/PAGEPILOT_REMEDIATION_PLAN.md`、`docs/CODEX_HANDOFF.md`：同步新能力和部署注意事项。
- 重新生成 `internal/web/skill/hostctl-deploy.zip`。

## 任务 1：Bundle/ZIP 解析模块

**文件：**
- 创建：`internal/bundle/bundle.go`
- 创建：`internal/bundle/bundle_test.go`
- 修改：`internal/deploy/deployer.go`
- 修改：`internal/deploy/zip_test.go`

- [ ] **步骤 1：编写失败测试**

```go
func TestAnalyzeZipBundleStripsNestedDistRoot(t *testing.T) {
    zipBytes := makeBundleTestZip(t, map[string]string{
        "project/dist/index.html":     "<!doctype html><html><body><div>Hello</div></body></html>",
        "project/dist/assets/app.css": "body{color:#075985}",
        "project/README.md":           "# wrapper",
    })

    result, err := bundle.AnalyzeZip(bundle.Input{
        Name: "site.zip",
        Data: zipBytes,
        Limits: bundle.Limits{
            MaxSingleFileBytes: 1 << 20,
            MaxSiteTotalBytes:  2 << 20,
            MaxFiles:           50,
        },
    })

    if err != nil {
        t.Fatalf("AnalyzeZip returned error: %v", err)
    }
    if result.MainEntry != "index.html" {
        t.Fatalf("MainEntry = %q, want index.html", result.MainEntry)
    }
    if !hasBundlePath(result.Files, "assets/app.css") {
        t.Fatalf("expected asset path to be stripped into deploy root: %#v", result.Files)
    }
    if result.Root != "project/dist" {
        t.Fatalf("Root = %q, want project/dist", result.Root)
    }
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test -count=1 ./internal/bundle ./internal/deploy
```

预期：`internal/bundle` 包不存在或 `AnalyzeZip` 未定义。

- [ ] **步骤 3：实现最小 bundle 模块**

```go
package bundle

type Limits struct {
    MaxSingleFileBytes int64
    MaxSiteTotalBytes  int64
    MaxFiles           int
}

type Input struct {
    Name   string
    Data   []byte
    Limits Limits
}

type File struct {
    Path     string
    Bytes    []byte
    IsBinary bool
    SHA256   string
}

type Result struct {
    Files     []File
    MainEntry string
    Root      string
    Kind      string
    TreeJSON  string
}

func AnalyzeZip(input Input) (Result, error) {
    // 使用 archive/zip 读取，拒绝绝对路径、..、盘符、UNC、空段。
    // 收集文件后选择真实根目录，再剥离 root。
    // 入口优先级：index.html > index.htm > README.md > README.markdown > 唯一 HTML/Markdown。
    return Result{}, nil
}
```

- [ ] **步骤 4：接入 Deployer**

把 `internal/deploy/deployer.go` 中 `expandZipContent`、`chooseArchiveRoot`、ZIP 路径校验逻辑迁移到 `internal/bundle`，`resolveContent` 只负责把 `bundle.Result` 转换成 `resolvedFile`。

- [ ] **步骤 5：运行验证**

```bash
go test -count=1 ./internal/bundle ./internal/deploy
```

- [ ] **步骤 6：提交**

```bash
git add internal/bundle internal/deploy/deployer.go internal/deploy/zip_test.go docs/superpowers/plans/2026-07-06-pagepilot-runtime-refactor.md
git commit -m "refactor: 拆分 Bundle 入口识别"
```

## 任务 2：数据库迁移、FTS、审计、缓存和 Bundle 元数据

**文件：**
- 修改：`internal/store/schema.sql`
- 修改：`internal/store/store.go`
- 修改：`internal/store/sqlite.go`
- 创建：`internal/store/search_audit_test.go`

- [ ] **步骤 1：编写失败测试**

```go
func TestMarketplaceSearchUsesFTSAndBackfills(t *testing.T) {
    st := newSQLiteStoreForTest(t)
    now := time.Now().UTC()
    mustCreateSiteAndVersion(t, st, "ai-report", "PagePilot 渗透测试报告", "发现 CORS 和源码泄露风险", "security,report", now)

    got, total, err := st.ListMarketplaceDeploys(context.Background(), "源码泄露", "active", "newest", "", "", "", "", 1, 10)
    if err != nil {
        t.Fatalf("ListMarketplaceDeploys returned error: %v", err)
    }
    if total != 1 || got[0].Code != "ai-report" {
        t.Fatalf("search result = total %d %#v, want ai-report", total, got)
    }
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test -count=1 ./internal/store
```

预期：FTS 表或新接口不存在。

- [ ] **步骤 3：增加 schema 与迁移**

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS site_search_fts USING fts5(
    code UNINDEXED,
    title,
    description,
    category,
    tags,
    content='',
    tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    site_code TEXT NOT NULL DEFAULT '',
    target_type TEXT NOT NULL DEFAULT '',
    target_id TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    detail_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS render_cache (
    cache_key TEXT PRIMARY KEY,
    site_code TEXT NOT NULL,
    version_number INTEGER NOT NULL,
    main_entry TEXT NOT NULL,
    content_sha256 TEXT NOT NULL,
    theme TEXT NOT NULL DEFAULT '',
    html TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    expires_at DATETIME
);

CREATE TABLE IF NOT EXISTS version_bundles (
    site_code TEXT NOT NULL,
    version_number INTEGER NOT NULL,
    kind TEXT NOT NULL,
    root TEXT NOT NULL DEFAULT '',
    main_entry TEXT NOT NULL,
    tree_json TEXT NOT NULL DEFAULT '[]',
    security_mode TEXT NOT NULL DEFAULT 'standard',
    created_at DATETIME NOT NULL,
    PRIMARY KEY (site_code, version_number)
);
```

- [ ] **步骤 4：实现 Store 接口**

```go
type AuditLog struct {
    ID         int64
    ActorType  string
    ActorID    string
    Action     string
    SiteCode   string
    TargetType string
    TargetID   string
    IP         string
    UserAgent  string
    DetailJSON string
    CreatedAt  time.Time
}

type VersionBundle struct {
    SiteCode      string
    VersionNumber int64
    Kind          string
    Root          string
    MainEntry     string
    TreeJSON      string
    SecurityMode  string
    CreatedAt     time.Time
}
```

在创建/更新版本时同步 `site_search_fts` 和 `version_bundles`，删除站点时清理相关表。

- [ ] **步骤 5：运行验证**

```bash
go test -count=1 ./internal/store ./internal/deploy
```

- [ ] **步骤 6：提交**

```bash
git add internal/store internal/deploy
git commit -m "feat: 增加搜索审计和 Bundle 元数据"
```

## 任务 3：Markdown 高级渲染与缓存

**文件：**
- 创建：`internal/render/markdown.go`
- 创建：`internal/render/cache.go`
- 创建：`internal/render/markdown_test.go`
- 修改：`internal/api/server.go`
- 修改：`internal/api/app_serve_test.go`

- [ ] **步骤 1：编写失败测试**

```go
func TestRenderMarkdownSupportsSafeAdvancedBlocks(t *testing.T) {
    html, err := render.Markdown(render.MarkdownInput{
        Source: []byte("# 报告\n\n```mermaid\nflowchart LR\nA-->B\n```\n\n$$E=mc^2$$\n\n```go\nfmt.Println(\"hi\")\n```\n\n<img src=x onerror=alert(1)>"),
        Theme:  "default",
        Nonce:  "abc123",
    })
    if err != nil {
        t.Fatalf("Markdown returned error: %v", err)
    }
    for _, want := range []string{"mermaid", "katex", "language-go", `nonce="abc123"`} {
        if !strings.Contains(html, want) {
            t.Fatalf("rendered HTML missing %q:\n%s", want, html)
        }
    }
    if strings.Contains(html, "onerror") || strings.Contains(html, "<script>alert") {
        t.Fatalf("rendered HTML contains unsafe content:\n%s", html)
    }
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test -count=1 ./internal/render ./internal/api
```

- [ ] **步骤 3：实现 Markdown 渲染**

使用 Go 侧安全渲染和本地资源注入：先实现 CommonMark 常用块、代码块 class、Mermaid/KaTeX 容器与 nonce 初始化脚本，避免 CDN。HTML sanitizer 拒绝 `script`、事件属性、`javascript:` URL。

- [ ] **步骤 4：接入缓存**

在 `serveHostedFile` 读取 Markdown 时，以 `siteCode/version/mainEntry/contentSha256/theme` 取缓存；没有缓存则渲染并写入缓存。缓存失败不阻断访问。

- [ ] **步骤 5：运行验证**

```bash
go test -count=1 ./internal/render ./internal/api
```

- [ ] **步骤 6：提交**

```bash
git add internal/render internal/api internal/store
git commit -m "feat: 增强 Markdown 渲染链路"
```

## 任务 4：托管安全策略和 CSP profile

**文件：**
- 创建：`internal/hosted/csp.go`
- 创建：`internal/hosted/html.go`
- 创建：`internal/hosted/csp_test.go`
- 修改：`internal/api/server.go`
- 修改：`internal/api/app_serve_test.go`

- [ ] **步骤 1：编写失败测试**

```go
func TestCSPProfilesSeparateHTMLMarkdownAndPreview(t *testing.T) {
    html := hosted.CSP(hosted.ProfileHTML, hosted.Options{FrameAncestors: "frame-ancestors 'self'"})
    md := hosted.CSP(hosted.ProfileMarkdown, hosted.Options{Nonce: "n1", FrameAncestors: "frame-ancestors 'self'"})
    preview := hosted.CSP(hosted.ProfilePreviewHTML, hosted.Options{})

    if !strings.Contains(html, "unsafe-eval") || !strings.Contains(html, "sandbox") {
        t.Fatalf("HTML CSP should preserve compatibility: %s", html)
    }
    if strings.Contains(md, "unsafe-eval") || !strings.Contains(md, "'nonce-n1'") {
        t.Fatalf("Markdown CSP should be strict with nonce: %s", md)
    }
    if html == preview {
        t.Fatalf("preview CSP should be distinct from formal hosted HTML")
    }
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test -count=1 ./internal/hosted ./internal/api
```

- [ ] **步骤 3：迁移 CSP 与 HTML 兼容脚本**

把 `setHostedContentSecurityHeaders`、`setHostedMarkdownSecurityHeaders`、`injectHostedHTMLCompat` 从 `server.go` 移入 `internal/hosted`，API 层只选择 profile 和写 header。

- [ ] **步骤 4：运行验证**

```bash
go test -count=1 ./internal/hosted ./internal/api
```

- [ ] **步骤 5：提交**

```bash
git add internal/hosted internal/api
git commit -m "refactor: 抽离托管 CSP 策略"
```

## 任务 5：multipart 上传接口、CLI、MCP、Skill 对齐

**文件：**
- 修改：`internal/api/server.go`
- 修改：`internal/api/types.go`
- 修改：`internal/api/openapi.go`
- 修改：`cmd/hostctl/main.go`
- 修改：`cmd/hostctl-mcp/main.go`
- 修改：`skill/hostctl-deploy/SKILL.md`
- 修改：`skill/hostctl-deploy/scripts/hostctl_deploy.py`
- 修改：`skill/hostctl-deploy/scripts/hostctl_deploy_test.py`
- 修改：`internal/web/skill/hostctl-deploy.zip`

- [ ] **步骤 1：编写失败测试**

```go
func TestDeployUploadAcceptsMultipartZip(t *testing.T) {
    srv, token, cleanup := newTokenTestServer(t)
    defer cleanup()

    body, contentType := makeMultipartDeployBody(t, map[string]string{
        "description": "多文件 ZIP 发布",
        "title":       "多文件演示",
    }, "file", "site.zip", makeTestZip(t, map[string]string{
        "dist/index.html": "<!doctype html><html><body><div>OK</div></body></html>",
    }))

    req := httptest.NewRequest(http.MethodPost, "/api/deploy/upload", body)
    req.Header.Set("Content-Type", contentType)
    req.Header.Set("Authorization", "Bearer "+token)
    rr := httptest.NewRecorder()

    srv.mux.ServeHTTP(rr, req)

    if rr.Code != http.StatusOK {
        t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
    }
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test -count=1 ./internal/api
```

- [ ] **步骤 3：实现 multipart API**

`POST /api/deploy/upload` 接收字段：`description`、`title`、`code`、`filename`、`visibility`、`category`、`tags`、`accessPassword`、`createVersion`、`source`，文件字段支持 `file` 或多段 `files`。服务端复用 `DeployRequest` 进入 `Deployer.Deploy`。

- [ ] **步骤 4：改 CLI 和 Python Skill**

CLI 对文件/目录/ZIP 优先 multipart；遇到旧服务 404/405 时 fallback JSON/base64。Python Skill 同样增加 multipart，MCP 工具描述提示大文件走 CLI。

- [ ] **步骤 5：重写 Skill 规约并打包**

```bash
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py
python test_skill.py
python - <<'PY'
from pathlib import Path
import zipfile
root = Path('skill/hostctl-deploy')
out = Path('internal/web/skill/hostctl-deploy.zip')
with zipfile.ZipFile(out, 'w', zipfile.ZIP_DEFLATED) as z:
    for path in sorted(root.rglob('*')):
        if path.is_file():
            rel = path.relative_to(root)
            if '__pycache__' in rel.parts or path.suffix in {'.pyc', '.pyo'}:
                continue
            z.write(path, rel.as_posix())
PY
```

- [ ] **步骤 6：运行验证**

```bash
go test -count=1 ./cmd/... ./internal/...
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py
python test_skill.py
```

- [ ] **步骤 7：提交**

```bash
git add internal/api cmd/hostctl cmd/hostctl-mcp skill internal/web/skill README.md
git commit -m "feat: 增加 multipart 发布并对齐 Skill MCP"
```

## 任务 6：市场复用、后台文件树和审计日志 UI

**文件：**
- 修改：`frontend/user/src/types.ts`
- 修改：`frontend/user/src/App.tsx`
- 修改：`frontend/user/src/styles.css`
- 修改：`frontend/admin/src/App.tsx`
- 修改：`frontend/admin/src/styles.css`
- 修改：`internal/api/admin_types.go`
- 修改：`internal/api/server.go`

- [ ] **步骤 1：编写前端类型和 API 失败检查**

```bash
npm run typecheck --prefix frontend/user
npm run typecheck --prefix frontend/admin
```

预期：新增字段未定义时失败。

- [ ] **步骤 2：扩展 API 返回**

市场详情和后台站点详情返回 `bundle`、`files`、`securityMode`、`reuse` 信息；新增 `GET /api/admin/audit-logs`。

- [ ] **步骤 3：用户端改造**

市场详情和复用抽屉显示：下载源码、Agent 提示词、CLI 命令、MCP 参数；默认提示“发布为新作品”，更新必须选择自己的 code。

- [ ] **步骤 4：后台改造**

应用管理显示文件树、入口、Bundle 类型、安全模式；新增审计日志 tab，支持按 action、code、actor 搜索。

- [ ] **步骤 5：运行验证**

```bash
npm run build --prefix frontend/user
npm run build --prefix frontend/admin
go test -count=1 ./internal/api ./internal/store
```

- [ ] **步骤 6：提交**

```bash
git add frontend internal/api internal/store
git commit -m "feat: 完善市场复用和审计后台"
```

## 任务 7：文档、完整验证和推送

**文件：**
- 修改：`README.md`
- 修改：`deploy/DOCKER.md`
- 修改：`deploy/APP_URL_MODE.md`
- 修改：`docs/PAGEPILOT_REMEDIATION_PLAN.md`
- 修改：`docs/CODEX_HANDOFF.md`

- [ ] **步骤 1：更新文档**

文档必须覆盖：高级 Markdown、ZIP Bundle、multipart 发布、FTS 搜索、审计日志、CSP profile、Skill/MCP/CLI 行为、Docker 数据不丢失升级说明。

- [ ] **步骤 2：完整验证**

```bash
go test -count=1 ./cmd/... ./internal/... ./apps/...
npm run build --prefix frontend/user
npm run build --prefix frontend/admin
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py
python test_skill.py
```

- [ ] **步骤 3：检查 diff 和无关文件**

```bash
git status --short
git diff --stat
```

- [ ] **步骤 4：提交和推送**

```bash
git add README.md deploy docs
git commit -m "docs: 同步运行时重构文档"
git push -u origin codex/pagepilot-runtime-refactor
```
