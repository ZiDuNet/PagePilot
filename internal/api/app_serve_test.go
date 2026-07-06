package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

func TestAppServeServesCurrentAndHistoricalVersion(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	stub := &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}
	srv.deployer = stub

	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	versionDir := filepath.Join(srv.cfg.HostedDir, "demo", "versions", "1")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatalf("mkdir version: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("<!doctype html><title>current</title><body>CURRENT</body>"), 0o644); err != nil {
		t.Fatalf("write current index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "index.html"), []byte("<!doctype html><title>version</title><body>VERSION-1</body>"), 0o644); err != nil {
		t.Fatalf("write version index: %v", err)
	}

	currentReq := httptest.NewRequest(http.MethodGet, "/agent/demo/", nil)
	currentRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(currentRR, currentReq)
	if currentRR.Code != http.StatusOK {
		t.Fatalf("current status = %d, body = %s", currentRR.Code, currentRR.Body.String())
	}
	if !strings.Contains(currentRR.Body.String(), "CURRENT") {
		t.Fatalf("current body = %q, want CURRENT", currentRR.Body.String())
	}

	versionReq := httptest.NewRequest(http.MethodGet, "/agent/demo/versions/1/", nil)
	versionRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(versionRR, versionReq)
	if versionRR.Code != http.StatusOK {
		t.Fatalf("version status = %d, body = %s", versionRR.Code, versionRR.Body.String())
	}
	if !strings.Contains(versionRR.Body.String(), "VERSION-1") {
		t.Fatalf("version body = %q, want VERSION-1", versionRR.Body.String())
	}
}

func TestAppServeAllowsSandboxedSameAppFetch(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "chapter.html"), []byte("<p>chapter</p>"), 0o644); err != nil {
		t.Fatalf("write chapter: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/demo/chapter.html", nil)
	req.Header.Set("Origin", "null")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "null" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want null", got)
	}
	if got := rr.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q, want Origin", got)
	}
}

func TestAppServeEmbedPolicyAddsFrameAncestors(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.EmbedPolicy = "allowlist"
	srv.cfg.EmbedAllowOrigins = "https://portal.example.com"
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("<p>demo</p>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/demo/", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'self' https://portal.example.com") {
		t.Fatalf("CSP = %q, want configured frame-ancestors", csp)
	}
}

func TestAppServeEmbedPolicyDenyAddsFrameAncestorsNone(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.EmbedPolicy = "deny"
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("<p>demo</p>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/demo/", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP = %q, want frame-ancestors none", csp)
	}
}

func TestAppServeRedirectsLegacyVersionQuery(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/demo/?v=1", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/agent/demo/versions/1/" {
		t.Fatalf("location = %q, want /agent/demo/versions/1/", loc)
	}
}

func TestAppServeRejectsPathTraversal(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("SAFE"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/demo/..", nil)
	rr := httptest.NewRecorder()

	srv.serveAppContent(rr, req, "demo", "..", "", false)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s; want 404", rr.Code, rr.Body.String())
	}
}

func TestRenderHostedMarkdownAdvancedBlocksAreSafe(t *testing.T) {
	body := []byte(`# Guide

![logo](images/logo.png)
[bad](javascript:alert(1))

| Name | Value |
| --- | --- |
| HTML | <script>alert(1)</script> |

- [x] publish
- [ ] review

` + "```go" + `
fmt.Println("<safe>")
` + "```" + `

` + "```mermaid" + `
flowchart LR
  A-->B
` + "```" + `

` + "```katex" + `
E = mc^2
` + "```" + `
`)

	rendered := renderHostedMarkdown(body)
	mustContain := []string{
		`<img src="images/logo.png" alt="logo">`,
		`<table>`,
		`&lt;script&gt;alert(1)&lt;/script&gt;`,
		`class="task-list"`,
		`type="checkbox" disabled checked`,
		`class="language-go"`,
		`fmt.Println(&#34;&lt;safe&gt;&#34;)`,
		`class="markdown-diagram"`,
		`class="language-mermaid"`,
		`class="markdown-math"`,
		`class="language-math"`,
	}
	for _, want := range mustContain {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered markdown missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `href="javascript:alert(1)"`) || strings.Contains(rendered, "<script>alert(1)</script>") {
		t.Fatalf("rendered markdown contains unsafe active content:\n%s", rendered)
	}
}

func TestAppServeMarkdownUsesHTMLContentTypeAndSandbox(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "docs"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "docs"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "docs", "current")
	if err := os.MkdirAll(filepath.Join(currentDir, "images"), 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("<!doctype html><html><body><main>fallback</main></body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "README.md"), []byte("# Docs\n\n![logo](images/logo.png)"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/docs/README.md", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	if csp := rr.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Fatalf("CSP = %q, want sandbox", csp)
	} else if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "script-src") {
		t.Fatalf("Markdown CSP = %q, should not allow scripts", csp)
	}
	if !strings.Contains(rr.Body.String(), `<img src="images/logo.png" alt="logo">`) {
		t.Fatalf("markdown relative image was not rendered: %s", rr.Body.String())
	}
}

type appServeDeployerStub struct {
	DeployerPort
	site     store.Site
	siteRoot string
}

func (s *appServeDeployerStub) GetSite(context.Context, string) (store.Site, error) {
	if s.site.Code == "" {
		return store.Site{}, store.ErrNotFound
	}
	return s.site, nil
}

func (s *appServeDeployerStub) GetContent(
	context.Context,
	string,
	*int64,
) (*GetContentResponse, *APIError) {
	return &GetContentResponse{
		Success:   true,
		Code:      s.site.Code,
		Version:   1,
		MainEntry: "index.html",
	}, nil
}

func (s *appServeDeployerStub) ReadAppFile(_ context.Context, code string, versionPtr *int64, path string) ([]byte, time.Time, *APIError) {
	if s.site.Code != code {
		return nil, time.Time{}, NewError(CodeNotFound, "site", "site not found")
	}
	root := filepath.Join(s.siteRoot, "current")
	if versionPtr != nil {
		root = filepath.Join(s.siteRoot, "versions", strconv.FormatInt(*versionPtr, 10))
	}
	full := filepath.Join(root, path)
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, time.Time{}, NewError(CodeNotFound, "file", "file not found")
	}
	modTime := time.Time{}
	if st, statErr := os.Stat(full); statErr == nil {
		modTime = st.ModTime()
	}
	return data, modTime, nil
}

func (s *appServeDeployerStub) IncrementViewCount(context.Context, string) error {
	return nil
}
