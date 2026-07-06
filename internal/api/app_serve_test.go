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

func TestHostedMarkdownRenderUsesCache(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	cache := newMarkdownCacheStub()
	srv.deployer = &cachedMarkdownDeployerStub{markdownCacheStub: cache}

	body := []byte("# Cached")
	first := srv.renderHostedMarkdown(context.Background(), "docs", nil, "README.md", body)
	second := srv.renderHostedMarkdown(context.Background(), "docs", nil, "README.md", body)

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

type appServeDeployerStub struct {
	DeployerPort
	site     store.Site
	siteRoot string
}

type cachedMarkdownDeployerStub struct {
	DeployerPort
	*markdownCacheStub
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
