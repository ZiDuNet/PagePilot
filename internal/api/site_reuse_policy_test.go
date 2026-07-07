package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

func TestAdminSetSiteReusePolicy(t *testing.T) {
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
	srv.deployer = &siteReusePolicyDeployerStub{
		site: store.Site{
			Code:                 "demo",
			PublicID:             "public-demo",
			OwnerTokenID:         "user:owner",
			Status:               "active",
			Visibility:           "public",
			ReusePolicy:          "auto",
			SourceDownloadPolicy: "auto",
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		},
	}

	body := strings.NewReader(`{"reusePolicy":"deny","sourceDownloadPolicy":"allow"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/sites/demo/reuse-policy", body)
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var out struct {
		Success bool `json:"success"`
		Site    struct {
			Code                 string `json:"code"`
			ReusePolicy          string `json:"reusePolicy"`
			SourceDownloadPolicy string `json:"sourceDownloadPolicy"`
		} `json:"site"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !out.Success || out.Site.Code != "demo" || out.Site.ReusePolicy != "deny" || out.Site.SourceDownloadPolicy != "allow" {
		t.Fatalf("response = %+v; want updated policies", out)
	}
}

func TestAdminSetSiteSecurityMode(t *testing.T) {
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
	srv.deployer = &siteReusePolicyDeployerStub{
		site: store.Site{
			Code:         "demo",
			PublicID:     "public-demo",
			OwnerTokenID: "user:owner",
			Status:       "active",
			Visibility:   "public",
			SecurityMode: "auto",
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		},
	}

	body := strings.NewReader(`{"securityMode":"compatible"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/sites/demo/security-mode", body)
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var out struct {
		Success bool `json:"success"`
		Site    struct {
			Code         string `json:"code"`
			SecurityMode string `json:"securityMode"`
		} `json:"site"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !out.Success || out.Site.Code != "demo" || out.Site.SecurityMode != "compatible" {
		t.Fatalf("response = %+v; want compatible security mode", out)
	}
}

type siteReusePolicyDeployerStub struct {
	DeployerPort
	site store.Site
}

func (s *siteReusePolicyDeployerStub) GetSite(_ context.Context, code string) (store.Site, error) {
	if s.site.Code != code {
		return store.Site{}, store.ErrNotFound
	}
	return s.site, nil
}

func (s *siteReusePolicyDeployerStub) SetSiteReusePolicy(_ context.Context, code, reusePolicy, sourceDownloadPolicy string) error {
	if s.site.Code != code {
		return store.ErrNotFound
	}
	s.site.ReusePolicy = reusePolicy
	s.site.SourceDownloadPolicy = sourceDownloadPolicy
	return nil
}

func (s *siteReusePolicyDeployerStub) SetSiteSecurityMode(_ context.Context, code, securityMode string) error {
	if s.site.Code != code {
		return store.ErrNotFound
	}
	s.site.SecurityMode = securityMode
	return nil
}
