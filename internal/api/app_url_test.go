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

func TestDomainHostServesAppContent(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AppURLMode = AppURLModeDomain
	srv.cfg.AppDomainSuffix = "pagepilot.example.com"
	srv.cfg.AppURLScheme = "https"
	srv.cfg.AppURLPort = "1143"
	srv.deployer = &appServeDeployerStub{site: store.Site{Code: "demo"}}

	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("<!doctype html><body>DOMAIN</body>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
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
		site: store.Site{Code: "demo", AccessPasswordHash: hash},
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
