package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/auth"
	"github.com/yourorg/hostctl/internal/store"
)

func TestSiteAccessRejectsForgedCookie(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	srv.deployer = &siteAccessDeployerStub{
		site: store.Site{Code: "demo", AccessPasswordHash: hash},
	}

	req := httptest.NewRequest(http.MethodGet, "/demo", nil)
	req.AddCookie(&http.Cookie{Name: "pagepilot_access_demo", Value: "1"})

	if srv.siteAccessAllowed(req, "demo") {
		t.Fatal("forged access cookie was accepted")
	}
}

func TestSiteAccessCookieInvalidatesWhenPasswordChanges(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	deployer := &siteAccessDeployerStub{
		site: store.Site{Code: "demo", AccessPasswordHash: hash},
	}
	srv.deployer = deployer

	body, _ := json.Marshal(siteAccessRequest{Password: "secret123"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/deploys/demo/access", bytes.NewReader(body))
	loginReq.SetPathValue("code", "demo")
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()

	srv.handleSiteAccessLogin(loginRR, loginReq.WithContext(withRequestID(loginReq.Context(), "test-req")))

	if loginRR.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", loginRR.Code, loginRR.Body.String(), http.StatusOK)
	}
	cookies := loginRR.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected access cookie to be set")
	}
	if cookies[0].MaxAge != int(siteAccessCookieTTL.Seconds()) {
		t.Fatalf("cookie max age = %d, want %d", cookies[0].MaxAge, int(siteAccessCookieTTL.Seconds()))
	}

	allowedReq := httptest.NewRequest(http.MethodGet, "/demo", nil)
	allowedReq.AddCookie(cookies[0])
	if !srv.siteAccessAllowed(allowedReq, "demo") {
		t.Fatal("fresh password access cookie was rejected")
	}

	newHash, err := auth.HashPassword("changed123")
	if err != nil {
		t.Fatalf("hash changed password: %v", err)
	}
	deployer.site.AccessPasswordHash = newHash

	if srv.siteAccessAllowed(allowedReq, "demo") {
		t.Fatal("old access cookie was accepted after password change")
	}
}

func TestSiteAccessCookieExpiresAfterFiveMinutes(t *testing.T) {
	now := time.Now().UTC()
	value := siteAccessCookieValue("demo", "hash-1", now.Add(siteAccessCookieTTL))

	if !validSiteAccessCookie(value, "demo", "hash-1", now.Add(siteAccessCookieTTL-time.Second)) {
		t.Fatal("access cookie expired too early")
	}
	if validSiteAccessCookie(value, "demo", "hash-1", now.Add(siteAccessCookieTTL+time.Second)) {
		t.Fatal("access cookie was accepted after ttl")
	}
}

type siteAccessDeployerStub struct {
	DeployerPort
	site store.Site
}

func (s *siteAccessDeployerStub) GetSite(context.Context, string) (store.Site, error) {
	if s.site.Code == "" {
		return store.Site{}, store.ErrNotFound
	}
	return s.site, nil
}
