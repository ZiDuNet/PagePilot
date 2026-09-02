package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/auth"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
)

func TestAppURLConfigBuildsPathAndDomainURLs(t *testing.T) {
	cfg := config.Default()
	cfg.AppURLMode = AppURLModePath
	cfg.AppDomainSuffix = "pagepilot.example.com"
	cfg.AppURLScheme = "https"
	cfg.AppURLPort = "1143"

	appURLs := NewAppURLConfig(cfg).WithPathBaseURL("https://pagepilot.example.com:1143")
	version := int64(7)
	if got := appURLs.PrimaryAppURL("demo", nil); got != "https://pagepilot.example.com:1143/agent/demo/" {
		t.Fatalf("path primary url = %q", got)
	}
	if got := appURLs.PrimaryAppURL("demo", &version); got != "https://pagepilot.example.com:1143/agent/demo/versions/7/" {
		t.Fatalf("path version url = %q", got)
	}

	cfg.AppURLMode = AppURLModeDomain
	appURLs = NewAppURLConfig(cfg).WithPathBaseURL("https://pagepilot.example.com:1143")
	if got := appURLs.PrimaryAppURL("demo", nil); got != "https://demo.pagepilot.example.com:1143/" {
		t.Fatalf("domain primary url = %q", got)
	}
	if got := appURLs.PrimaryAppURL("demo", &version); got != "https://demo.pagepilot.example.com:1143/versions/7/" {
		t.Fatalf("domain version url = %q", got)
	}
}

func TestAppURLMethodsNormalizeDirectConfigValues(t *testing.T) {
	cfg := AppURLConfig{
		AppURLMode:      " DOMAIN ",
		AppDomainSuffix: "*.Apps.Example.com.",
		AppURLScheme:    "HTTP",
		AppURLPort:      "80",
	}.WithPathBaseURL("http://control.example")
	version := int64(3)
	set := cfg.URLSet("demo", &version)
	if set.URL != "http://demo.apps.example.com/versions/3/" ||
		set.PathURL != "http://control.example/agent/demo/versions/3/" ||
		set.DomainURL != set.URL {
		t.Fatalf("normalized URL set = %+v", set)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "DEMO.APPS.EXAMPLE.COM:80"
	if got := cfg.CodeFromRequestHost(req); got != "demo" {
		t.Fatalf("CodeFromRequestHost = %q, want demo", got)
	}
}

func TestAppURLSetKeepsCanonicalAndVariantURLsConsistent(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantURL    string
		wantPath   string
		wantDomain string
	}{
		{
			name:     "path mode",
			mode:     AppURLModePath,
			wantURL:  "https://control.example/publish/demo/versions/7/",
			wantPath: "https://control.example/publish/demo/versions/7/",
		},
		{
			name:       "domain mode",
			mode:       AppURLModeDomain,
			wantURL:    "https://demo.apps.example.com:8443/versions/7/",
			wantPath:   "https://control.example/publish/demo/versions/7/",
			wantDomain: "https://demo.apps.example.com:8443/versions/7/",
		},
		{
			name:       "dual mode keeps path canonical",
			mode:       AppURLModeDual,
			wantURL:    "https://control.example/publish/demo/versions/7/",
			wantPath:   "https://control.example/publish/demo/versions/7/",
			wantDomain: "https://demo.apps.example.com:8443/versions/7/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AppURLConfig{
				AppURLMode:      tt.mode,
				AppDomainSuffix: "apps.example.com",
				AppURLScheme:    "https",
				AppURLPort:      "8443",
				AppPathBase:     "/publish",
			}.WithPathBaseURL("https://control.example")
			version := int64(7)
			got := cfg.URLSet("demo", &version)
			if got.URL != tt.wantURL || got.PathURL != tt.wantPath || got.DomainURL != tt.wantDomain {
				t.Fatalf("URLSet = %+v, want url=%q path=%q domain=%q", got, tt.wantURL, tt.wantPath, tt.wantDomain)
			}
		})
	}
}

func TestExtractCodeFromURLWithConfig(t *testing.T) {
	domainCfg := AppURLConfig{
		AppURLMode:      AppURLModeDomain,
		AppDomainSuffix: "*.apps.example.com.",
		AppPathBase:     "/publish",
	}.WithPathBaseURL("https://control.example")
	tests := []struct {
		name string
		cfg  AppURLConfig
		raw  string
		want string
	}{
		{
			name: "domain root",
			cfg:  domainCfg,
			raw:  "https://demo.apps.example.com/",
			want: "demo",
		},
		{
			name: "domain history with query and fragment",
			cfg:  domainCfg,
			raw:  "https://DEMO.apps.example.com.:8443/versions/7/?preview=1#top",
			want: "demo",
		},
		{
			name: "path history with query",
			cfg:  domainCfg,
			raw:  "https://control.example/publish/demo/versions/7/?preview=1",
			want: "demo",
		},
		{
			name: "bare path compatibility",
			cfg:  domainCfg,
			raw:  "/demo/?preview=1",
			want: "demo",
		},
		{
			name: "custom path base",
			cfg:  domainCfg,
			raw:  "/agent/demo/versions/7/",
			want: "",
		},
		{
			name: "wrong suffix",
			cfg:  domainCfg,
			raw:  "https://demo.other.example.com/",
			want: "",
		},
		{
			name: "external host path is rejected",
			cfg:  domainCfg,
			raw:  "https://evil.example/publish/demo/",
			want: "",
		},
		{
			name: "nested subdomain is not a code",
			cfg:  domainCfg,
			raw:  "https://preview.demo.apps.example.com/",
			want: "",
		},
		{
			name: "path mode rejects domain host",
			cfg:  AppURLConfig{AppURLMode: AppURLModePath, AppDomainSuffix: "apps.example.com", AppPathBase: "/agent"},
			raw:  "https://demo.apps.example.com/",
			want: "",
		},
		{
			name: "empty and malformed values",
			cfg:  domainCfg,
			raw:  "https://",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractCodeFromURLWithConfig(tt.raw, tt.cfg); got != tt.want {
				t.Fatalf("extractCodeFromURLWithConfig(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
	if got := extractCodeFromURL("https://legacy.example/agent/demo/versions/2/"); got != "demo" {
		t.Fatalf("legacy extractCodeFromURL = %q, want demo", got)
	}
}

func TestListVersionsDecoratesCurrentURLModeVariants(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AppURLMode = AppURLModeDomain
	srv.cfg.AppDomainSuffix = "apps.example.com"
	srv.cfg.AppURLScheme = "https"
	srv.cfg.AppURLPort = "8443"
	versionNumber := int64(7)
	srv.deployer = &siteDetailDeployerStub{
		versions: &ListVersionsResponse{
			Success:        true,
			Code:           "demo",
			CurrentVersion: &versionNumber,
			Versions:       []VersionItem{{VersionNumber: versionNumber}},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/deploys/demo/versions?preview=1", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "control.example")
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var out ListVersionsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(out.Versions) != 1 {
		t.Fatalf("versions = %+v, want one version", out.Versions)
	}
	got := out.Versions[0]
	wantPath := "https://control.example/agent/demo/versions/7/"
	wantDomain := "https://demo.apps.example.com:8443/versions/7/"
	if got.URL != wantDomain || got.PathURL != wantPath || got.DomainURL != wantDomain {
		t.Fatalf("version URL contract = %+v, want url=%q path=%q domain=%q", got, wantDomain, wantPath, wantDomain)
	}
}

func TestAdminSiteListDecoratesCurrentURLModeVariants(t *testing.T) {
	srv, adminToken, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	srv.cfg.AppURLMode = AppURLModeDomain
	srv.cfg.AppDomainSuffix = "apps.example.com"
	srv.cfg.AppURLScheme = "https"
	srv.cfg.AppURLPort = "8443"
	srv.deployer = newSitePinDeployerStub("demo", "user:owner")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/sites", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "control.example")
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var out SiteListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(out.Sites) != 1 {
		t.Fatalf("sites = %+v, want one site", out.Sites)
	}
	got := out.Sites[0]
	wantPath := "https://control.example/agent/demo/"
	wantDomain := "https://demo.apps.example.com:8443/"
	if got.URL != wantDomain || got.PathURL != wantPath || got.DomainURL != wantDomain {
		t.Fatalf("site URL contract = %+v, want url=%q path=%q domain=%q", got, wantDomain, wantPath, wantDomain)
	}
}

func TestDomainHostServesAppContent(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AppURLMode = AppURLModeDomain
	srv.cfg.AppDomainSuffix = "pagepilot.example.com"
	srv.cfg.AppURLScheme = "https"
	srv.cfg.AppURLPort = "1143"
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}

	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("<!doctype html><body>DOMAIN</body>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	historyDir := filepath.Join(srv.cfg.HostedDir, "demo", "versions", "7")
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		t.Fatalf("mkdir history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(historyDir, "index.html"), []byte("<!doctype html><body>DOMAIN HISTORY</body>"), 0o644); err != nil {
		t.Fatalf("write history index: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://demo.pagepilot.example.com:1143/", nil)
	req.Host = "demo.pagepilot.example.com:1143"
	rr := httptest.NewRecorder()
	srv.withMiddleware(srv.mux).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "DOMAIN") {
		t.Fatalf("body = %q, want DOMAIN", rr.Body.String())
	}
	if csp := rr.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Fatalf("missing hosted content sandbox CSP: %q", csp)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "https://demo.pagepilot.example.com:1143/versions/7/", nil)
	historyReq.Host = "demo.pagepilot.example.com:1143"
	historyRR := httptest.NewRecorder()
	srv.withMiddleware(srv.mux).ServeHTTP(historyRR, historyReq)
	if historyRR.Code != http.StatusOK || !strings.Contains(historyRR.Body.String(), "DOMAIN HISTORY") {
		t.Fatalf("history status = %d, body = %q; want domain historical version", historyRR.Code, historyRR.Body.String())
	}
}

func TestDomainHostBlocksAPIOnAppOrigin(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AppURLMode = AppURLModeDomain
	srv.cfg.AppDomainSuffix = "pagepilot.example.com"

	req := httptest.NewRequest(http.MethodPost, "https://demo.pagepilot.example.com/api/deploy", strings.NewReader("{}"))
	req.Host = "demo.pagepilot.example.com"
	rr := httptest.NewRecorder()
	srv.withMiddleware(srv.mux).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestDomainHostAllowsOwnPasswordLogin(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AppURLMode = AppURLModeDomain
	srv.cfg.AppDomainSuffix = "pagepilot.example.com"

	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo", AccessPasswordHash: hash},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}

	body, _ := json.Marshal(siteAccessRequest{Password: "secret123"})
	req := httptest.NewRequest(
		http.MethodPost,
		"https://demo.pagepilot.example.com/api/deploys/demo/access",
		bytes.NewReader(body),
	)
	req.Host = "demo.pagepilot.example.com"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.withMiddleware(srv.mux).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(rr.Result().Cookies()) == 0 {
		t.Fatal("expected access cookie")
	}
}
