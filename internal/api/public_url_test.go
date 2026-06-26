package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigAlwaysUsesRequestHostForMainSiteLinks(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pagepilot2.dell.4dbim.cc")
	rr := httptest.NewRecorder()

	srv.handleGetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp ConfigResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.CurrentBaseURL != "https://pagepilot2.dell.4dbim.cc" {
		t.Fatalf("currentBaseURL = %q, want request host", resp.CurrentBaseURL)
	}
	if got := srv.appURLConfigForRequest(req).PathAppURL("demo", nil); got != "https://pagepilot2.dell.4dbim.cc/agent/demo/" {
		t.Fatalf("path app url = %q, want request host", got)
	}
}

func TestConfigUsesBrowserOriginWhenProxyHostIsStale(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pagepilot.dell.4dbim.cc")
	req.Header.Set("X-Hostctl-Current-Origin", "https://pagepilot.chaoxi.live")
	rr := httptest.NewRecorder()

	srv.handleGetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp ConfigResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.CurrentBaseURL != "https://pagepilot.chaoxi.live" {
		t.Fatalf("currentBaseURL = %q, want browser origin", resp.CurrentBaseURL)
	}
	if got := srv.appURLConfigForRequest(req).PathAppURL("demo", nil); got != "https://pagepilot.chaoxi.live/agent/demo/" {
		t.Fatalf("path app url = %q, want browser origin", got)
	}
}

func TestConfigIgnoresOriginQueryForNormalAPIs(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/config?origin=https%3A%2F%2Ffake.example.com", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pagepilot.chaoxi.live")
	rr := httptest.NewRecorder()

	srv.handleGetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp ConfigResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.CurrentBaseURL != "https://pagepilot.chaoxi.live" {
		t.Fatalf("currentBaseURL = %q, want request host", resp.CurrentBaseURL)
	}
}

func TestRequestHostDoesNotOverrideDomainAppURL(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AppURLMode = AppURLModeDomain
	srv.cfg.AppDomainSuffix = "apps.example.com"
	srv.cfg.AppURLScheme = "https"

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pagepilot2.dell.4dbim.cc")

	appURLs := srv.appURLConfigForRequest(req)
	if got := appURLs.PrimaryAppURL("demo", nil); got != "https://demo.apps.example.com/" {
		t.Fatalf("domain primary url = %q", got)
	}
	if got := appURLs.PathAppURL("demo", nil); got != "https://pagepilot2.dell.4dbim.cc/agent/demo/" {
		t.Fatalf("path fallback url = %q", got)
	}
}

func TestSkillDownloadReturnsManagedZipWithoutRuntimeInjection(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	want := makeTestSkillZip(t, map[string]string{
		"hostctl-deploy/scripts/hostctl_deploy.py": `DEFAULT_SERVER = os.environ.get("HOSTCTL_SERVER", "http://localhost:8787")`,
	})
	writeManagedSkillZip(t, srv, want)

	req := httptest.NewRequest(http.MethodGet, "/skill/hostctl-deploy.zip", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pagepilot2.dell.4dbim.cc")
	rr := httptest.NewRecorder()

	srv.handleSkillDownload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body size = %d", rr.Code, rr.Body.Len())
	}
	if !bytes.Equal(rr.Body.Bytes(), want) {
		t.Fatalf("downloaded zip was modified at runtime")
	}
	zr, err := zip.NewReader(bytes.NewReader(want), int64(len(want)))
	if err != nil {
		t.Fatalf("open skill zip: %v", err)
	}
	var script string
	for _, f := range zr.File {
		if f.Name != "hostctl-deploy/scripts/hostctl_deploy.py" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open script in zip: %v", err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read script in zip: %v", err)
		}
		script = string(data)
		break
	}
	if strings.Contains(script, "pagepilot.chaoxi.live") || strings.Contains(script, "pagepilot2.dell.4dbim.cc") {
		t.Fatalf("managed skill zip should not receive runtime origin injection: %s", script)
	}
}

func TestSkillDownloadMissingManagedZipReturnsNotFound(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/skill/hostctl-deploy.zip", nil)
	rr := httptest.NewRecorder()

	srv.handleSkillDownload(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusNotFound)
	}
}

func TestAdminUploadSkillZipPersistsManagedPackage(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	admin, err := authSvc.CreateUser(t.Context(), "admin", "password123", true, -1)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	token, err := authSvc.Generate(t.Context(), "admin-token", true, admin.ID, nil)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	want := makeTestSkillZip(t, map[string]string{
		"hostctl-deploy/SKILL.md": "managed package",
	})
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "hostctl-deploy.zip")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := fw.Write(want); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/skill/package", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	req = httptest.NewRequest(http.MethodGet, "/skill/hostctl-deploy.zip", nil)
	rr = httptest.NewRecorder()
	srv.handleSkillDownload(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("download status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if !bytes.Equal(rr.Body.Bytes(), want) {
		t.Fatalf("downloaded package differs from uploaded package")
	}
}

func makeTestSkillZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func writeManagedSkillZip(t *testing.T, srv *Server, data []byte) {
	t.Helper()
	path := srv.managedSkillZipPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write skill zip: %v", err)
	}
}
