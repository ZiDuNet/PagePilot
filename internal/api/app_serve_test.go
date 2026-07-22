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

	"github.com/yourorg/hostctl/internal/config"
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

func TestUserAppUIInjectsMainSnippets(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.ContentInjection.Main = config.InjectionTargetConfig{
		Enabled:       true,
		HeadCode:      `<meta name="pp-main-injected" content="1">`,
		BodyStartCode: `<div id="pp-main-start"></div>`,
		BodyEndCode:   `<script>window.__ppMainInjected = true;</script>`,
	}

	req := httptest.NewRequest(http.MethodGet, "/market", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	for _, want := range []string{
		`<meta name="pp-main-injected" content="1">`,
		`<div id="pp-main-start"></div>`,
		`<script>window.__ppMainInjected = true;</script>`,
	} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("main UI body missing injected snippet %q", want)
		}
	}
}

func TestAppServeHTMLInjectsAppSnippets(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.ContentInjection.App = config.InjectionTargetConfig{
		Enabled:       true,
		HeadCode:      `<meta name="pp-app-injected" content="1">`,
		BodyStartCode: `<aside id="pp-app-start"></aside>`,
		BodyEndCode:   `<script>window.__ppAppInjected = true;</script>`,
	}
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("<!doctype html><html><head><title>Demo</title></head><body><main>demo</main></body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/demo/", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	for _, want := range []string{
		`<meta name="pp-app-injected" content="1">`,
		`<aside id="pp-app-start"></aside>`,
		`<script>window.__ppAppInjected = true;</script>`,
	} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("hosted HTML body missing injected snippet %q", want)
		}
	}
}

func TestAppServeHTMLPreviewSkipsAppSnippets(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.ContentInjection.App = config.InjectionTargetConfig{
		Enabled:       true,
		HeadCode:      `<script src="https://analytics.example.com/sdk.js"></script>`,
		BodyStartCode: `<aside id="pp-app-start"></aside>`,
		BodyEndCode:   `<script>window.__ppAppInjected = true;</script>`,
	}
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("<!doctype html><html><head><title>Demo</title></head><body><main>demo</main></body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/demo/?preview=1", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	for _, unwanted := range []string{
		`https://analytics.example.com/sdk.js`,
		`pp-app-start`,
		`window.__ppAppInjected`,
	} {
		if strings.Contains(rr.Body.String(), unwanted) {
			t.Fatalf("preview response unexpectedly contains injected snippet %q", unwanted)
		}
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

func TestAppServeStrictSecurityModeTightensCSP(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo", SecurityMode: "strict"},
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
	if !strings.Contains(csp, "default-src 'self'") || strings.Contains(csp, "'unsafe-eval'") {
		t.Fatalf("strict CSP = %q, want self-only default and no unsafe-eval", csp)
	}
	if strings.Contains(csp, "allow-top-navigation-by-user-activation") {
		t.Fatalf("strict CSP = %q, want no top-navigation sandbox allowance", csp)
	}
}

func TestAppServeTrustedSecurityModeRemovesSandbox(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo", SecurityMode: "trusted"},
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
	if strings.Contains(csp, "sandbox") {
		t.Fatalf("trusted CSP = %q, want no sandbox directive", csp)
	}
	if !strings.Contains(csp, "default-src *") {
		t.Fatalf("trusted CSP = %q, want compatibility default-src", csp)
	}
}

func TestHostedContentSandboxModesNeverAllowSameOrigin(t *testing.T) {
	for _, mode := range []string{"", "standard", "compatible", "strict"} {
		t.Run(mode, func(t *testing.T) {
			csp := hostedContentCSP(mode)
			if !strings.Contains(csp, "sandbox") {
				t.Fatalf("%s CSP = %q, want sandbox directive", mode, csp)
			}
			if strings.Contains(csp, "allow-same-origin") {
				t.Fatalf("%s CSP = %q, must not combine sandbox scripts with allow-same-origin", mode, csp)
			}
		})
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

func TestAppServeMarkdownUsesHTMLContentTypeAndStrictRuntimeCSP(t *testing.T) {
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
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store for nonce-bearing markdown HTML", got)
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") ||
		!strings.Contains(csp, "script-src 'nonce-") ||
		!strings.Contains(csp, "style-src 'self' 'nonce-") ||
		!strings.Contains(csp, "style-src-elem 'self' 'unsafe-inline'") ||
		!strings.Contains(csp, "style-src-attr 'unsafe-inline'") ||
		!strings.Contains(csp, "base-uri 'none'") ||
		!strings.Contains(csp, "form-action 'none'") {
		t.Fatalf("Markdown CSP = %q, want nonce-only script policy", csp)
	}
	if strings.Contains(csp, "unsafe-eval") ||
		strings.Contains(csp, "script-src 'self'") ||
		strings.Contains(csp, "script-src 'unsafe-inline'") ||
		strings.Contains(csp, "script-src *") ||
		strings.Contains(csp, "style-src *") {
		t.Fatalf("Markdown CSP = %q, should not allow broad script/style execution", csp)
	}
	if !strings.Contains(rr.Body.String(), `<img src="images/logo.png" alt="logo">`) {
		t.Fatalf("markdown relative image was not rendered: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `<style nonce="`) ||
		!strings.Contains(rr.Body.String(), `<script defer src="/markdown-assets/katex/katex.min.js" nonce="`) ||
		!strings.Contains(rr.Body.String(), `<script defer src="/markdown-assets/katex/contrib/auto-render.min.js" nonce="`) ||
		!strings.Contains(rr.Body.String(), `<script defer src="/markdown-assets/mermaid/mermaid.min.js" nonce="`) ||
		!strings.Contains(rr.Body.String(), `<script defer nonce="`) ||
		!strings.Contains(rr.Body.String(), `/markdown-assets/katex/katex.min.js`) ||
		!strings.Contains(rr.Body.String(), `/markdown-assets/mermaid/mermaid.min.js`) {
		t.Fatalf("markdown runtime style/scripts were not injected with nonce: %s", rr.Body.String())
	}

	cachedReq := httptest.NewRequest(http.MethodGet, "/agent/docs/README.md", nil)
	cachedReq.Header.Set("If-Modified-Since", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	cachedRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(cachedRR, cachedReq)
	if cachedRR.Code != http.StatusOK {
		t.Fatalf("conditional markdown status = %d, body = %s; want 200 to avoid CSP nonce/cache mismatch", cachedRR.Code, cachedRR.Body.String())
	}
	if !strings.Contains(cachedRR.Body.String(), `<script defer nonce="`) {
		t.Fatalf("conditional markdown response missing fresh nonce body: %s", cachedRR.Body.String())
	}
}

func TestAppServeMarkdownInjectsAppSnippetsWithNonce(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.ContentInjection.App = config.InjectionTargetConfig{
		Enabled:     true,
		HeadCode:    `<script src="https://analytics.example.com/sdk.js"></script><style>.pp-injected{display:block}</style>`,
		BodyEndCode: `<script>window.__ppMarkdownInjected = true;</script>`,
	}
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "docs"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "docs"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "docs", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "README.md"), []byte("# Docs"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/docs/README.md", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`<script nonce="`,
		`src="https://analytics.example.com/sdk.js"`,
		`<style nonce="`,
		`window.__ppMarkdownInjected = true;`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("markdown body missing injected nonce snippet %q:\n%s", want, body)
		}
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'nonce-") ||
		!strings.Contains(csp, "https: http:") ||
		!strings.Contains(csp, "connect-src 'self' https: http: wss: ws:") {
		t.Fatalf("Markdown CSP with app injection = %q, want nonce and external reporting support", csp)
	}
}

func TestAppServeMarkdownPreviewSkipsAppSnippets(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.ContentInjection.App = config.InjectionTargetConfig{
		Enabled:     true,
		HeadCode:    `<script src="https://analytics.example.com/sdk.js"></script><style>.pp-injected{display:block}</style>`,
		BodyEndCode: `<script>window.__ppMarkdownInjected = true;</script>`,
	}
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "docs"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "docs"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "docs", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "README.md"), []byte("# Docs"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/docs/README.md?preview=1", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, unwanted := range []string{
		`https://analytics.example.com/sdk.js`,
		`pp-injected`,
		`window.__ppMarkdownInjected`,
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("preview markdown unexpectedly contains injected snippet %q:\n%s", unwanted, body)
		}
	}
	csp := rr.Header().Get("Content-Security-Policy")
	scriptSrc := cspDirective(csp, "script-src")
	connectSrc := cspDirective(csp, "connect-src")
	if strings.Contains(scriptSrc, "https:") || strings.Contains(scriptSrc, "http:") {
		t.Fatalf("preview markdown CSP unexpectedly allows external injection targets: %q", csp)
	}
	if strings.Contains(connectSrc, "https:") || strings.Contains(connectSrc, "http:") ||
		strings.Contains(connectSrc, "wss:") || strings.Contains(connectSrc, "ws:") {
		t.Fatalf("preview markdown CSP unexpectedly allows external injection connections: %q", csp)
	}
}

func cspDirective(csp, name string) string {
	for _, part := range strings.Split(csp, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+" ") || part == name {
			return part
		}
	}
	return ""
}

func TestMarkdownAssetsAreServedFromSameOrigin(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	for _, path := range []string{
		"/markdown-assets/katex/katex.min.css",
		"/markdown-assets/katex/katex.min.js",
		"/markdown-assets/katex/contrib/auto-render.min.js",
		"/markdown-assets/katex/fonts/KaTeX_Main-Regular.woff2",
		"/markdown-assets/katex/fonts/KaTeX_Math-Italic.woff2",
		"/markdown-assets/mermaid/mermaid.min.js",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Origin", "null")
		rr := httptest.NewRecorder()
		srv.mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s; want 200", path, rr.Code, rr.Body.String())
		}
		if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("%s X-Content-Type-Options = %q, want nosniff", path, got)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "null" {
			t.Fatalf("%s Access-Control-Allow-Origin = %q, want null for sandboxed markdown runtime", path, got)
		}
	}
}

func TestHostedMarkdownRenderUsesCache(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	cache := newMarkdownCacheStub()
	srv.deployer = &cachedMarkdownDeployerStub{markdownCacheStub: cache}

	body := []byte("# Cached")
	first := srv.renderHostedMarkdown(context.Background(), "docs", nil, "README.md", body, "auto")
	second := srv.renderHostedMarkdown(context.Background(), "docs", nil, "README.md", body, "auto")

	if first != second {
		t.Fatalf("cached render mismatch")
	}
	if cache.puts != 1 {
		t.Fatalf("cache puts = %d, want 1", cache.puts)
	}
	if cache.hits != 1 {
		t.Fatalf("cache hits = %d, want 1", cache.hits)
	}
}

func TestHostedMarkdownRenderCacheSeparatesTheme(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	cache := newMarkdownCacheStub()
	srv.deployer = &cachedMarkdownDeployerStub{markdownCacheStub: cache}

	body := []byte("# Cached")
	auto := srv.renderHostedMarkdown(context.Background(), "docs", nil, "README.md", body, "auto")
	dark := srv.renderHostedMarkdown(context.Background(), "docs", nil, "README.md", body, "dark")
	darkAgain := srv.renderHostedMarkdown(context.Background(), "docs", nil, "README.md", body, "dark")

	if auto == dark || !strings.Contains(dark, `data-theme="dark"`) {
		t.Fatalf("theme render did not produce theme-specific HTML: auto=%q dark=%q", auto, dark)
	}
	if dark != darkAgain {
		t.Fatalf("cached dark render mismatch")
	}
	if cache.puts != 2 {
		t.Fatalf("cache puts = %d, want 2 for auto and dark themes", cache.puts)
	}
	if cache.hits != 1 {
		t.Fatalf("cache hits = %d, want 1 for repeated dark theme", cache.hits)
	}
	for key, entry := range cache.entries {
		if !strings.Contains(key, ":"+entry.Theme+":") {
			t.Fatalf("cache key %q does not include theme %q", key, entry.Theme)
		}
	}
}

func TestAppServeMarkdownCurrentVersionCacheUsesResolvedVersion(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	currentVersion := int64(7)
	cache := newMarkdownCacheStub()
	siteRoot := filepath.Join(srv.cfg.HostedDir, "docs")
	srv.deployer = &cachedAppServeDeployerStub{
		appServeDeployerStub: appServeDeployerStub{
			site: store.Site{
				Code:           "docs",
				CurrentVersion: &currentVersion,
			},
			siteRoot: siteRoot,
		},
		markdownCacheStub: cache,
	}
	currentDir := filepath.Join(siteRoot, "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "README.md"), []byte("# Current Version"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/docs/README.md", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(cache.entries) != 1 {
		t.Fatalf("cache entries = %d, want 1", len(cache.entries))
	}
	for key, entry := range cache.entries {
		if entry.VersionNumber != currentVersion {
			t.Fatalf("cache entry version = %d, want %d; key=%q", entry.VersionNumber, currentVersion, key)
		}
		if !strings.Contains(key, ":7:README.md:") || strings.Contains(key, ":0:README.md:") {
			t.Fatalf("cache key = %q, want resolved current version and entry path", key)
		}
	}
}

type appServeDeployerStub struct {
	DeployerPort
	site     store.Site
	siteRoot string
}

type cachedAppServeDeployerStub struct {
	appServeDeployerStub
	*markdownCacheStub
}

type cachedMarkdownDeployerStub struct {
	DeployerPort
	*markdownCacheStub
}

func (s *cachedMarkdownDeployerStub) GetSite(context.Context, string) (store.Site, error) {
	return store.Site{}, store.ErrNotFound
}

type markdownCacheStub struct {
	entries map[string]store.RenderCacheEntry
	puts    int
	hits    int
}

func newMarkdownCacheStub() *markdownCacheStub {
	return &markdownCacheStub{entries: map[string]store.RenderCacheEntry{}}
}

func (s *markdownCacheStub) GetRenderCache(_ context.Context, cacheKey string) (store.RenderCacheEntry, bool, error) {
	entry, ok := s.entries[cacheKey]
	if ok {
		s.hits++
	}
	return entry, ok, nil
}

func (s *markdownCacheStub) PutRenderCache(_ context.Context, entry store.RenderCacheEntry) error {
	s.entries[entry.CacheKey] = entry
	s.puts++
	return nil
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
