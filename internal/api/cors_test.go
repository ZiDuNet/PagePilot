package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
)

func TestApplyCORSDisabledByDefault(t *testing.T) {
	srv := New(config.Default(), nil, nil, true, logTestLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()

	srv.applyCORS(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestApplyCORSAllowsExplicitOriginsOnly(t *testing.T) {
	cfg := config.Default()
	cfg.CORSAllowOrigins = "https://admin.example.com, https://studio.example.com"
	srv := New(cfg, nil, nil, true, logTestLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Origin", "https://studio.example.com")
	rr := httptest.NewRecorder()

	srv.applyCORS(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://studio.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://studio.example.com", got)
	}
	if got := rr.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q, want Origin", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want Authorization", got)
	}
}

func TestMiddlewareAppliesConfiguredCORSToAPIOnly(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.CORSAllowOrigins = "https://studio.example.com"

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Origin", "https://studio.example.com")
	rr := httptest.NewRecorder()

	srv.withMiddleware(srv.mux).ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://studio.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want configured API CORS origin", got)
	}
}

func TestMiddlewareDoesNotApplyConfiguredCORSToHostedApps(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.CORSAllowOrigins = "https://studio.example.com"
	srv.deployer = &appServeDeployerStub{
		site:     store.Site{Code: "demo"},
		siteRoot: filepath.Join(srv.cfg.HostedDir, "demo"),
	}
	currentDir := filepath.Join(srv.cfg.HostedDir, "demo", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "index.html"), []byte("<!doctype html><p>demo</p>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/agent/demo/", nil)
	req.Header.Set("Origin", "https://studio.example.com")
	rr := httptest.NewRecorder()

	srv.withMiddleware(srv.mux).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for hosted app content", got)
	}
}

func TestCorsOriginsLookValidRejectsWildcard(t *testing.T) {
	if corsOriginsLookValid("*") {
		t.Fatal("wildcard * must not be accepted")
	}
	if !corsOriginsLookValid("https://admin.example.com, https://studio.example.com") {
		t.Fatal("explicit origin list should be accepted")
	}
}

func logTestLogger() *log.Logger {
	return log.New(bytes.NewBuffer(nil), "", 0)
}
