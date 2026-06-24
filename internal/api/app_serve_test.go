package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/store"
)

func TestAppServeServesCurrentAndHistoricalVersion(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	stub := &appServeDeployerStub{
		site: store.Site{Code: "demo"},
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

func TestAppServeRedirectsLegacyVersionQuery(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	srv.deployer = &appServeDeployerStub{
		site: store.Site{Code: "demo"},
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
		site: store.Site{Code: "demo"},
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

type appServeDeployerStub struct {
	DeployerPort
	site store.Site
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

func (s *appServeDeployerStub) IncrementViewCount(context.Context, string) error {
	return nil
}
