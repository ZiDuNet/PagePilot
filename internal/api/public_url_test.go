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

	"github.com/yourorg/hostctl/internal/config"
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

func TestConfigOnlyReturnsInjectionCodeToAdmin(t *testing.T) {
	srv, adminToken, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	srv.cfg.ContentInjection = config.ContentInjectionConfig{
		Main: config.InjectionTargetConfig{
			Enabled:  true,
			HeadCode: `<script src="https://analytics.example.com/sdk.js"></script>`,
		},
		App: config.InjectionTargetConfig{
			Enabled:     true,
			BodyEndCode: `<script>window.__appInjected = true;</script>`,
		},
	}

	publicReq := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	publicRR := httptest.NewRecorder()
	srv.handleGetConfig(publicRR, publicReq)
	if publicRR.Code != http.StatusOK {
		t.Fatalf("public status = %d, body = %s", publicRR.Code, publicRR.Body.String())
	}
	var publicResp ConfigResponse
	if err := json.Unmarshal(publicRR.Body.Bytes(), &publicResp); err != nil {
		t.Fatalf("decode public response: %v", err)
	}
	if !publicResp.ContentInjection.Main.Enabled || !publicResp.ContentInjection.App.Enabled {
		t.Fatalf("public content injection flags = %+v, want enabled summary", publicResp.ContentInjection)
	}
	if publicResp.ContentInjection.Main.HeadCode != "" || publicResp.ContentInjection.App.BodyEndCode != "" {
		t.Fatalf("public response leaked injection code: %+v", publicResp.ContentInjection)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	adminReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminRR := httptest.NewRecorder()
	srv.handleGetConfig(adminRR, adminReq)
	if adminRR.Code != http.StatusOK {
		t.Fatalf("admin status = %d, body = %s", adminRR.Code, adminRR.Body.String())
	}
	var adminResp ConfigResponse
	if err := json.Unmarshal(adminRR.Body.Bytes(), &adminResp); err != nil {
		t.Fatalf("decode admin response: %v", err)
	}
	if !strings.Contains(adminResp.ContentInjection.Main.HeadCode, "analytics.example.com") ||
		!strings.Contains(adminResp.ContentInjection.App.BodyEndCode, "__appInjected") {
		t.Fatalf("admin response missing injection code: %+v", adminResp.ContentInjection)
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
		"pagep/scripts/pagep.py": `DEFAULT_SERVER = os.environ.get("PAGEPILOT_SERVER", "https://pagepilot.dell.4dbim.cc:1143/")`,
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
		if f.Name != "pagep/scripts/pagep.py" {
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

func TestSkillDownloadFallsBackToBuiltInZip(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/skill/hostctl-deploy.zip", nil)
	rr := httptest.NewRecorder()

	srv.handleSkillDownload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body size = %d; want %d", rr.Code, rr.Body.Len(), http.StatusOK)
	}
	if rr.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("content-type = %q, want application/zip", rr.Header().Get("Content-Type"))
	}
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatalf("open built-in skill zip: %v", err)
	}
	foundSkill := false
	for _, f := range zr.File {
		if f.Name == "pagep/SKILL.md" {
			foundSkill = true
			break
		}
	}
	if !foundSkill {
		t.Fatalf("built-in zip does not contain pagep/SKILL.md")
	}
	info := srv.skillPackageInfo()
	if !info.Exists || info.Source != "built-in" || info.Size <= 0 {
		t.Fatalf("package info = %+v, want built-in package", info)
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
		"pagep/SKILL.md": "managed package",
	})
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "pagep.zip")
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
	info := srv.skillPackageInfo()
	if info.Source != "uploaded" {
		t.Fatalf("package source = %q, want uploaded", info.Source)
	}
}

func TestAdminSkillSourceFileWriteIsNotAllowed(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPut, "/api/admin/skill", strings.NewReader(`{"path":"SKILL.md","content":"bad"}`))
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("PUT /api/admin/skill unexpectedly succeeded: %s", rr.Body.String())
	}
}

func TestFaviconRoutesServeIcon(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	for _, path := range []string{"/favicon.ico", "/app/favicon.ico", "/admin/favicon.ico"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		srv.mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, rr.Code, rr.Body.String())
		}
		if body := rr.Body.Bytes(); len(body) < 4 || string(body[:4]) != "\x00\x00\x01\x00" {
			t.Fatalf("%s did not return ICO bytes, first bytes = %v", path, body[:min(len(body), 8)])
		}
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
