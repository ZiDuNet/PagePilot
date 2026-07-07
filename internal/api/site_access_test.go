package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

	if srv.siteAccessAllowed(req, "demo", nil) {
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
	if !srv.siteAccessAllowed(allowedReq, "demo", nil) {
		t.Fatal("fresh password access cookie was rejected")
	}

	newHash, err := auth.HashPassword("changed123")
	if err != nil {
		t.Fatalf("hash changed password: %v", err)
	}
	deployer.site.AccessPasswordHash = newHash

	if srv.siteAccessAllowed(allowedReq, "demo", nil) {
		t.Fatal("old access cookie was accepted after password change")
	}
}

func TestSiteAccessCookieInvalidatesWhenCurrentVersionChanges(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	versionOne := int64(1)
	versionTwo := int64(2)
	deployer := &siteAccessDeployerStub{
		site: store.Site{
			Code:               "demo",
			AccessPasswordHash: hash,
			CurrentVersion:     &versionOne,
		},
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

	currentReq := httptest.NewRequest(http.MethodGet, "/agent/demo/", nil)
	currentReq.AddCookie(cookies[0])
	if !srv.siteAccessAllowed(currentReq, "demo", nil) {
		t.Fatal("fresh password access cookie was rejected for current version")
	}

	deployer.site.CurrentVersion = &versionTwo
	if srv.siteAccessAllowed(currentReq, "demo", nil) {
		t.Fatal("old access cookie was accepted after current version changed")
	}
	if !srv.siteAccessAllowed(currentReq, "demo", &versionOne) {
		t.Fatal("old access cookie should still allow the explicit version it was issued for")
	}
}

func TestSiteAccessCookieExpiresAfterFiveMinutes(t *testing.T) {
	now := time.Now().UTC()
	version := int64(7)
	value := siteAccessCookieValue("demo", "hash-1", version, now.Add(siteAccessCookieTTL))

	if !validSiteAccessCookie(value, "demo", "hash-1", version, now.Add(siteAccessCookieTTL-time.Second)) {
		t.Fatal("access cookie expired too early")
	}
	if validSiteAccessCookie(value, "demo", "hash-1", version, now.Add(siteAccessCookieTTL+time.Second)) {
		t.Fatal("access cookie was accepted after ttl")
	}
	if validSiteAccessCookie(value, "demo", "hash-1", version+1, now.Add(siteAccessCookieTTL-time.Second)) {
		t.Fatal("access cookie was accepted for a different version")
	}
}

func TestSourceContentRejectsPasswordOnlyAccess(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	deployer := &siteAccessDeployerStub{
		site: store.Site{
			Code:               "demo",
			OwnerTokenID:       "user:owner",
			Status:             "active",
			Visibility:         "public",
			AccessPasswordHash: hash,
		},
	}
	srv.deployer = deployer

	req := httptest.NewRequest(http.MethodGet, "/api/deploy/content?code=demo&download=1", nil)
	req.AddCookie(&http.Cookie{
		Name:  siteAccessCookieName("demo"),
		Value: siteAccessCookieValue("demo", hash, 0, time.Now().UTC().Add(siteAccessCookieTTL)),
	})
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusForbidden)
	}
	if deployer.streamed {
		t.Fatal("source download stream was reached for unlisted password-only access")
	}
	if strings.Contains(rr.Body.String(), "SOURCE") {
		t.Fatalf("body leaked source content: %s", rr.Body.String())
	}
	if len(deployer.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one failed source_download log", deployer.auditLogs)
	}
	log := deployer.auditLogs[0]
	if log.Action != "source_download" || log.Result != "failed" || log.SiteCode != "demo" ||
		log.TargetType != "site" || log.TargetID != "demo" {
		t.Fatalf("audit log = %+v; want failed source_download log for demo", log)
	}
	if !strings.Contains(log.DetailJSON, `"download":true`) ||
		!strings.Contains(log.DetailJSON, `"errorCode":"FORBIDDEN"`) ||
		!strings.Contains(log.DetailJSON, `"stage":"source_download"`) {
		t.Fatalf("detail = %s; want structured source download failure detail", log.DetailJSON)
	}
}

func TestSourceContentDownloadRecordsAuditLog(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	deployer := &siteAccessDeployerStub{
		site: store.Site{
			Code:                 "demo",
			Status:               "active",
			Visibility:           "public",
			ReusePolicy:          "auto",
			SourceDownloadPolicy: "auto",
		},
	}
	srv.deployer = deployer

	req := httptest.NewRequest(http.MethodGet, "/api/deploy/content?code=demo&version=3&download=1", nil)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if !deployer.streamed {
		t.Fatal("source download stream was not reached")
	}
	if len(deployer.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one successful source_download log", deployer.auditLogs)
	}
	log := deployer.auditLogs[0]
	if log.Action != "source_download" || log.Result != "success" || log.SiteCode != "demo" ||
		log.TargetType != "site" || log.TargetID != "demo" {
		t.Fatalf("audit log = %+v; want successful source_download log for demo", log)
	}
	if !strings.Contains(log.DetailJSON, `"download":true`) ||
		!strings.Contains(log.DetailJSON, `"version":3`) {
		t.Fatalf("detail = %s; want source download audit detail", log.DetailJSON)
	}
}

func TestSourceContentRejectsEncryptedSiteEvenForOwner(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	owner, err := authSvc.CreateUser(t.Context(), "owner", "password123", false, 20)
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	token, err := authSvc.Generate(t.Context(), "owner-token", false, owner.ID, nil)
	if err != nil {
		t.Fatalf("generate owner token: %v", err)
	}
	deployer := &siteAccessDeployerStub{
		site: store.Site{
			Code:               "demo",
			OwnerTokenID:       "user:" + owner.ID,
			Status:             "active",
			Visibility:         "public",
			AccessPasswordHash: hash,
		},
	}
	srv.deployer = deployer

	req := httptest.NewRequest(http.MethodGet, "/api/deploy/content?code=demo&download=1", nil)
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusForbidden)
	}
	if deployer.streamed {
		t.Fatal("source download stream was reached for encrypted owner access")
	}
	if strings.Contains(rr.Body.String(), "SOURCE") {
		t.Fatalf("body leaked source content: %s", rr.Body.String())
	}
}

func TestSourceContentRejectsEncryptedSiteEvenForAdmin(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	admin, err := authSvc.CreateUser(t.Context(), "admin", "password123", true, -1)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	token, err := authSvc.Generate(t.Context(), "admin-token", true, admin.ID, nil)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	deployer := &siteAccessDeployerStub{
		site: store.Site{
			Code:               "demo",
			OwnerTokenID:       "user:owner",
			Status:             "active",
			Visibility:         "public",
			AccessPasswordHash: hash,
		},
	}
	srv.deployer = deployer

	req := httptest.NewRequest(http.MethodGet, "/api/deploy/content?code=demo&download=1", nil)
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusForbidden)
	}
	if deployer.streamed {
		t.Fatal("source download stream was reached for encrypted admin access")
	}
	if strings.Contains(rr.Body.String(), "SOURCE") {
		t.Fatalf("body leaked source content: %s", rr.Body.String())
	}
}

func TestSiteAccessLoginRecordsAuditLogWithoutPassword(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	version := int64(7)
	deployer := &siteAccessDeployerStub{
		site: store.Site{
			Code:               "demo",
			AccessPasswordHash: hash,
			CurrentVersion:     &version,
		},
	}
	srv.deployer = deployer

	req := httptest.NewRequest(http.MethodPost, "/api/deploys/demo/access?version=7", strings.NewReader(`{
		"password":"wrong-pass"
	}`))
	req.SetPathValue("code", "demo")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "password-audit-test")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
	if len(deployer.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one site.access_login log", deployer.auditLogs)
	}
	log := deployer.auditLogs[0]
	if log.Action != "site.access_login" || log.Result != "failed" ||
		log.ActorType != "browser" || log.ActorRole != "public" ||
		log.SiteCode != "demo" || log.TargetType != "site" || log.TargetID != "demo" ||
		log.UserAgent != "password-audit-test" {
		t.Fatalf("audit log = %+v; want failed public access login log for demo", log)
	}
	if !strings.Contains(log.DetailJSON, `"versionNumber":7`) ||
		!strings.Contains(log.DetailJSON, `"reason":"incorrect_password"`) ||
		strings.Contains(log.DetailJSON, "wrong-pass") ||
		strings.Contains(log.DetailJSON, "secret123") {
		t.Fatalf("detail = %s; want version/reason without password", log.DetailJSON)
	}
}

func TestSiteAccessLoginSuccessRecordsAuditLog(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	version := int64(8)
	deployer := &siteAccessDeployerStub{
		site: store.Site{
			Code:               "demo",
			AccessPasswordHash: hash,
			CurrentVersion:     &version,
		},
	}
	srv.deployer = deployer

	req := httptest.NewRequest(http.MethodPost, "/api/deploys/demo/access", strings.NewReader(`{
		"password":"secret123"
	}`))
	req.SetPathValue("code", "demo")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if len(deployer.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one successful site.access_login log", deployer.auditLogs)
	}
	log := deployer.auditLogs[0]
	if log.Action != "site.access_login" || log.Result != "success" ||
		log.ActorType != "browser" || log.ActorRole != "public" ||
		log.SiteCode != "demo" || log.TargetType != "site" || log.TargetID != "demo" {
		t.Fatalf("audit log = %+v; want successful public access login log for demo", log)
	}
	if !strings.Contains(log.DetailJSON, `"versionNumber":8`) ||
		strings.Contains(log.DetailJSON, "secret123") {
		t.Fatalf("detail = %s; want version without password", log.DetailJSON)
	}
}

func TestSetSiteAccessPasswordFailureRecordsAuditLog(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &siteAccessAuditFailureStub{err: errors.New("persist failed")}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPatch, "/api/deploys/demo/access", strings.NewReader(`{
		"password":"secret123"
	}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusInternalServerError)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one failed site.access_password log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "site.access_password" || log.Result != "failed" || log.ActorRole != "admin" ||
		log.SiteCode != "demo" || log.TargetType != "site" || log.TargetID != "demo" {
		t.Fatalf("audit log = %+v; want failed access password log for demo", log)
	}
	if !strings.Contains(log.DetailJSON, `"accessProtected":true`) ||
		!strings.Contains(log.DetailJSON, `"errorCode":"INTERNAL"`) ||
		!strings.Contains(log.DetailJSON, `"stage":"access_password"`) {
		t.Fatalf("detail = %s; want structured access password failure detail", log.DetailJSON)
	}
}

type siteAccessDeployerStub struct {
	DeployerPort
	site      store.Site
	streamed  bool
	auditLogs []store.AuditLog
}

func (s *siteAccessDeployerStub) GetSite(context.Context, string) (store.Site, error) {
	if s.site.Code == "" {
		return store.Site{}, store.ErrNotFound
	}
	return s.site, nil
}

func (s *siteAccessDeployerStub) StreamDownload(_ context.Context, _ string, _ *int64, w http.ResponseWriter) *APIError {
	s.streamed = true
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("SOURCE"))
	return nil
}

func (s *siteAccessDeployerStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type siteAccessAuditFailureStub struct {
	DeployerPort
	err       error
	auditLogs []store.AuditLog
}

func (s *siteAccessAuditFailureStub) SetSiteAccessPassword(context.Context, string, string) error {
	return s.err
}

func (s *siteAccessAuditFailureStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}
