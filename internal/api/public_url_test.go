package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestSkillDownloadInjectsRequestHostWhenEnabled(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	t.Setenv("HOSTCTL_SKILL_DIR", filepath.Join("..", "..", "skill", "hostctl-deploy"))

	req := httptest.NewRequest(http.MethodGet, "/skill/hostctl-deploy.zip", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pagepilot2.dell.4dbim.cc")
	rr := httptest.NewRecorder()

	srv.handleSkillDownload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body size = %d", rr.Code, rr.Body.Len())
	}
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
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
	if !strings.Contains(script, `DEFAULT_SERVER = os.environ.get("HOSTCTL_SERVER", "https://pagepilot2.dell.4dbim.cc")`) {
		t.Fatalf("script default server was not injected with request host")
	}
}

func TestSkillDownloadInjectsBrowserOriginWhenProxyHostIsStale(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	t.Setenv("HOSTCTL_SKILL_DIR", filepath.Join("..", "..", "skill", "hostctl-deploy"))

	req := httptest.NewRequest(http.MethodGet, "/skill/hostctl-deploy.zip", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pagepilot.dell.4dbim.cc")
	req.Header.Set("X-Hostctl-Current-Origin", "https://pagepilot.chaoxi.live")
	rr := httptest.NewRecorder()

	srv.handleSkillDownload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body size = %d", rr.Code, rr.Body.Len())
	}
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
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
	if !strings.Contains(script, `DEFAULT_SERVER = os.environ.get("HOSTCTL_SERVER", "https://pagepilot.chaoxi.live")`) {
		t.Fatalf("script default server was not injected with browser origin")
	}
}

func TestSkillDownloadInjectsOriginQueryWhenAnchorCannotSendHeader(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	t.Setenv("HOSTCTL_SKILL_DIR", filepath.Join("..", "..", "skill", "hostctl-deploy"))

	req := httptest.NewRequest(http.MethodGet, "/skill/hostctl-deploy.zip?origin=https%3A%2F%2Fpagepilot.chaoxi.live", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pagepilot.dell.4dbim.cc")
	rr := httptest.NewRecorder()

	srv.handleSkillDownload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body size = %d", rr.Code, rr.Body.Len())
	}
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
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
	if !strings.Contains(script, `DEFAULT_SERVER = os.environ.get("HOSTCTL_SERVER", "https://pagepilot.chaoxi.live")`) {
		t.Fatalf("script default server was not injected with origin query")
	}
}
