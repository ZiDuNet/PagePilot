package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/auth"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
)

func TestCreateTokenRejectsNonAdminOwnerOverride(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()

	userA, err := authSvc.CreateUser(ctx, "alice", "password123", false, 20)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	userB, err := authSvc.CreateUser(ctx, "bob", "password123", false, 20)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	tokenA, err := authSvc.Generate(ctx, "alice-token", false, userA.ID, nil)
	if err != nil {
		t.Fatalf("generate alice token: %v", err)
	}

	body, _ := json.Marshal(TokenCreateRequest{
		Label:       "bob-token",
		OwnerUserID: userB.ID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenA.Plaintext)
	rr := httptest.NewRecorder()

	srv.handleCreateToken(rr, req.WithContext(withRequestID(req.Context(), "test-req")))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusForbidden)
	}
}

func TestCreateTokenDefaultsToPermanent(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()

	user, err := authSvc.CreateUser(ctx, "alice", "password123", false, 20)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	parentToken, err := authSvc.Generate(ctx, "alice-token", false, user.ID, nil)
	if err != nil {
		t.Fatalf("generate parent token: %v", err)
	}

	body, _ := json.Marshal(TokenCreateRequest{Label: "permanent-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+parentToken.Plaintext)
	rr := httptest.NewRecorder()

	srv.handleCreateToken(rr, req.WithContext(withRequestID(req.Context(), "test-req")))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var resp TokenCreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ExpiresAt != nil {
		t.Fatalf("response expiresAt = %v, want permanent nil", resp.ExpiresAt)
	}
	stored, err := authSvc.GetToken(ctx, resp.ID)
	if err != nil {
		t.Fatalf("get stored token: %v", err)
	}
	if stored.ExpiresAt != nil {
		t.Fatalf("stored expiresAt = %v, want permanent nil", stored.ExpiresAt)
	}
}

func TestListMarketplaceMarksOwnedWithoutLeakingOwnerTokenID(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()

	user, err := authSvc.CreateUser(ctx, "alice", "password123", false, 20)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := authSvc.Generate(ctx, "alice-token", false, user.ID, nil)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	now := time.Now().UTC()
	srv.deployer = marketplaceDeploysStub{
		deploys: []store.MarketplaceDeploy{
			{
				ID:                     "public-1",
				Code:                   "demo-app",
				OwnerTokenID:           "user:" + user.ID,
				PrimaryVersionStrategy: "latest",
				Title:                  "Demo App",
				Filename:               "index.html",
				CreatedAt:              now,
				UpdatedAt:              now,
				Status:                 "active",
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/deploys", nil)
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	rr := httptest.NewRecorder()

	srv.handleListMarketplace(rr, req.WithContext(withRequestID(req.Context(), "test-req")))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if strings.Contains(rr.Body.String(), "ownerTokenId") || strings.Contains(rr.Body.String(), "user:"+user.ID) {
		t.Fatalf("marketplace response leaked owner token id: %s", rr.Body.String())
	}
	var got struct {
		Deploys []struct {
			Owned bool `json:"owned"`
		} `json:"deploys"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Deploys) != 1 || !got.Deploys[0].Owned {
		t.Fatalf("owned flag = %#v, want one owned deploy", got.Deploys)
	}
}

func TestDeployRejectsClaimedAnonymousSession(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.requireAuth = false
	stub := &claimedAnonymousDeployStub{}
	srv.deployer = stub

	body, _ := json.Marshal(DeployRequest{
		Filename:    "index.html",
		Description: "demo",
		Content:     "<!doctype html><title>demo</title>",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hostctl-Session", "anon-claimed")
	rr := httptest.NewRecorder()

	srv.handleDeploy(rr, req.WithContext(withRequestID(req.Context(), "test-req")))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
	if stub.deployCalled {
		t.Fatal("deploy was called for a claimed anonymous session")
	}
}

func TestRequireAuthRejectsAnonymousDeploy(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.requireAuth = true
	stub := newTrackingAnonymousDeployStub()
	srv.deployer = stub

	body, _ := json.Marshal(DeployRequest{
		Filename:    "index.html",
		Description: "anonymous production deploy must be rejected",
		Content:     "<!doctype html><title>blocked</title>",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
	if stub.deployCalled || len(stub.sessions) != 0 {
		t.Fatalf("anonymous request reached deploy/session creation: called=%v sessions=%#v", stub.deployCalled, stub.sessions)
	}
	if !strings.Contains(rr.Body.String(), "registered user or admin session required") {
		t.Fatalf("body = %s; want require-auth diagnostic", rr.Body.String())
	}
}

func TestAnonymousDeployWithoutExistingSessionIsTracked(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.requireAuth = false
	stub := newTrackingAnonymousDeployStub()
	srv.deployer = stub
	admin, err := authSvc.CreateUser(context.Background(), "admin", "password123", true, 20)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	token, err := authSvc.Generate(context.Background(), "admin-token", true, admin.ID, nil)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	body, _ := json.Marshal(DeployRequest{
		Filename:       "index.html",
		Title:          "匿名发布记录测试",
		Description:    "匿名发布记录测试页面",
		Content:        "<!doctype html><html><head><title>匿名发布记录测试</title></head><body><h1>ok</h1></body></html>",
		Visibility:     "unlisted",
		AccessPassword: "secret123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hostctl-Agent-Id", "agent-abc")
	req.Header.Set("X-Hostctl-Agent-Label", "匿名测试 Agent")
	req.Header.Set("User-Agent", "PagePilotTest/1.0")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("deploy status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if stub.deployOwner != "anon:"+stub.createdSessionID {
		t.Fatalf("deploy owner = %q, want anon owner for generated session %q", stub.deployOwner, stub.createdSessionID)
	}
	cookies := rr.Result().Cookies()
	var sessionID string
	for _, c := range cookies {
		if c.Name == "hostctl_anon_session" {
			sessionID = c.Value
			break
		}
	}
	if !strings.HasPrefix(sessionID, "anon_") {
		t.Fatalf("anonymous session cookie = %q, want generated anon_ id", sessionID)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/anonymous-sessions", nil)
	listReq.Header.Set("Authorization", "Bearer "+token.Plaintext)
	listRR := httptest.NewRecorder()

	srv.mux.ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s; want %d", listRR.Code, listRR.Body.String(), http.StatusOK)
	}
	var got AnonymousSessionListResponse
	if err := json.Unmarshal(listRR.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("anonymous sessions = %#v, want one tracked deploy", got.Sessions)
	}
	session := got.Sessions[0]
	if session.ID != sessionID {
		t.Fatalf("session id = %q, want %q", session.ID, sessionID)
	}
	if session.AgentID != "agent-abc" || session.AgentLabel != "匿名测试 Agent" {
		t.Fatalf("agent meta = %#v, want request headers recorded", session)
	}
	if session.DeployCount != 1 {
		t.Fatalf("deploy count = %d, want 1", session.DeployCount)
	}
	if session.Remaining != 49 {
		t.Fatalf("remaining = %d, want 49", session.Remaining)
	}
}

func TestAnonymousVersionDeployDoesNotConsumeSiteQuota(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.requireAuth = false
	srv.cfg.AnonymousDeployLimit = 1
	stub := newTrackingAnonymousDeployStub()
	stub.siteExists = true
	stub.created = false
	stub.siteOwnerTokenID = "anon:anon-existing"
	now := time.Now()
	stub.sessions["anon-existing"] = store.AnonymousSession{
		ID:          "anon-existing",
		DeployCount: 1,
		CreatedAt:   now,
		LastUsedAt:  now,
	}
	srv.deployer = stub

	body, _ := json.Marshal(DeployRequest{
		Filename:         "index.html",
		Title:            "匿名版本更新",
		Description:      "匿名用户在额度满额后追加版本",
		Content:          "<!doctype html><title>update</title>",
		EnableCustomCode: true,
		CustomCode:       "anon-track-test",
		CreateVersion:    true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hostctl-Session", "anon-existing")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("deploy status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if !stub.deployCalled {
		t.Fatal("deploy was not called for existing-site version update")
	}
	if got := stub.sessions["anon-existing"].DeployCount; got != 1 {
		t.Fatalf("deploy count = %d, want unchanged 1", got)
	}
}

func TestAnonymousSessionAuthorizesOwnedSiteWrites(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.requireAuth = false
	stub := newTrackingAnonymousDeployStub()
	stub.siteExists = true
	stub.siteOwnerTokenID = "anon:anon-existing"
	stub.sessions["anon-existing"] = store.AnonymousSession{ID: "anon-existing"}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPatch, "/api/deploys/anon-owned/current", bytes.NewBufferString(`{"versionNumber":1}`))
	req.Header.Set("X-Hostctl-Session", "anon-existing")

	if apiErr := srv.authorizeSiteWrite(req, "anon-owned"); apiErr != nil {
		t.Fatalf("authorizeSiteWrite returned %v for the owning anonymous session", apiErr)
	}
	actor, isAdmin, apiErr := srv.authenticateActor(req)
	if apiErr != nil {
		t.Fatalf("authenticateActor returned %v", apiErr)
	}
	if actor != "anon:anon-existing" || isAdmin {
		t.Fatalf("actor = %q, isAdmin = %v; want anonymous owner and non-admin", actor, isAdmin)
	}
}

func TestAnonymousSessionRoutesAuthorizeOwnedSiteWrites(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.requireAuth = false
	stub := newTrackingAnonymousDeployStub()
	stub.siteExists = true
	stub.siteOwnerTokenID = "anon:anon-existing"
	stub.sessions["anon-existing"] = store.AnonymousSession{ID: "anon-existing"}
	srv.deployer = stub

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		action string
	}{
		{name: "set current", method: http.MethodPatch, path: "/api/deploys/anon-owned/current", body: `{"versionNumber":1}`, action: "current"},
		{name: "lock version", method: http.MethodPost, path: "/api/deploys/anon-owned/versions/1/lock", body: `{"locked":true}`, action: "lock"},
		{name: "delete version", method: http.MethodDelete, path: "/api/deploys/anon-owned/versions/1", action: "delete"},
		{name: "set access password", method: http.MethodPost, path: "/api/deploys/anon-owned/access/set", body: `{"password":"secret123"}`, action: "access"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("X-Hostctl-Session", "anon-existing")
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rr := httptest.NewRecorder()
			srv.mux.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
			}
		})
	}
	if got := strings.Join(stub.siteWriteActions, ","); got != "current,lock,delete,access" {
		t.Fatalf("site write actions = %q", got)
	}
}

func TestAnonymousSessionCannotManageRegisteredTokens(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.requireAuth = false
	stub := newTrackingAnonymousDeployStub()
	stub.sessions["anon-token-test"] = store.AnonymousSession{ID: "anon-token-test"}
	srv.deployer = stub

	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "list", method: http.MethodGet, path: "/api/tokens"},
		{name: "revoke", method: http.MethodDelete, path: "/api/tokens/token-id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("X-Hostctl-Session", "anon-token-test")
			rr := httptest.NewRecorder()
			srv.mux.ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusForbidden)
			}
			if !strings.Contains(rr.Body.String(), "registered user account required") {
				t.Fatalf("body = %s; want registered-user restriction", rr.Body.String())
			}
		})
	}
}

func TestUserVersionDeployDoesNotConsumeSiteQuota(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()
	user, err := authSvc.CreateUser(ctx, "alice", "password123", false, 1)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := authSvc.IncrementUserDeployCount(ctx, user.ID); err != nil {
		t.Fatalf("seed deploy count: %v", err)
	}
	session, err := authSvc.LoginAdmin(ctx, "alice", "password123", time.Hour)
	if err != nil {
		t.Fatalf("login user: %v", err)
	}
	stub := newTrackingAnonymousDeployStub()
	stub.siteExists = true
	stub.created = false
	stub.siteOwnerTokenID = "user:" + user.ID
	srv.deployer = stub

	body, _ := json.Marshal(DeployRequest{
		Filename:         "index.html",
		Title:            "注册用户版本更新",
		Description:      "注册用户在额度满额后追加版本",
		Content:          "<!doctype html><title>update</title>",
		EnableCustomCode: true,
		CustomCode:       "user-update-test",
		CreateVersion:    true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("deploy status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if !stub.deployCalled {
		t.Fatal("deploy was not called for existing-site version update")
	}
	got, err := authSvc.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.DeployCount != 1 {
		t.Fatalf("user deploy count = %d, want unchanged 1", got.DeployCount)
	}
}

func TestCreateVersionForMissingSiteStillConsumesSiteQuota(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()
	user, err := authSvc.CreateUser(ctx, "alice", "password123", false, 1)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := authSvc.IncrementUserDeployCount(ctx, user.ID); err != nil {
		t.Fatalf("seed deploy count: %v", err)
	}
	session, err := authSvc.LoginAdmin(ctx, "alice", "password123", time.Hour)
	if err != nil {
		t.Fatalf("login user: %v", err)
	}
	stub := newTrackingAnonymousDeployStub()
	srv.deployer = stub

	body, _ := json.Marshal(DeployRequest{
		Filename:         "index.html",
		Title:            "满额新站点",
		Description:      "目标 code 不存在时仍按新站点额度处理",
		Content:          "<!doctype html><title>new</title>",
		EnableCustomCode: true,
		CustomCode:       "missing-site-test",
		CreateVersion:    true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("deploy status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
	if stub.deployCalled {
		t.Fatal("deploy was called even though missing-site createVersion should consume quota")
	}
}

func TestUserDeployQuotaRollsBackWhenDeploymentFails(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()
	user, err := authSvc.CreateUser(ctx, "quota-failure", "password123", false, 1)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	session, err := authSvc.LoginAdmin(ctx, "quota-failure", "password123", time.Hour)
	if err != nil {
		t.Fatalf("login user: %v", err)
	}
	srv.deployer = quotaFailureDeployStub{}

	body, _ := json.Marshal(DeployRequest{
		Filename: "index.html",
		Title:    "quota rollback",
		Content:  "<!doctype html><title>quota rollback</title>",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("deploy status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusInternalServerError)
	}
	got, err := authSvc.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.DeployCount != 0 {
		t.Fatalf("user deploy count = %d after failed deployment, want 0", got.DeployCount)
	}
}

func TestUserDeployQuotaRollsBackWhenConcurrentCreateBecomesVersion(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()
	user, err := authSvc.CreateUser(ctx, "quota-version", "password123", false, 1)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	session, err := authSvc.LoginAdmin(ctx, "quota-version", "password123", time.Hour)
	if err != nil {
		t.Fatalf("login user: %v", err)
	}
	// SiteExists reports false during the preflight check, but Deploy reports
	// an update. This is the shape of a concurrent creator winning the race.
	stub := newTrackingAnonymousDeployStub()
	stub.created = false
	srv.deployer = stub

	body, _ := json.Marshal(DeployRequest{
		Filename: "index.html",
		Title:    "quota version rollback",
		Content:  "<!doctype html><title>quota version rollback</title>",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("deploy status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	got, err := authSvc.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.DeployCount != 0 {
		t.Fatalf("user deploy count = %d after version response, want 0", got.DeployCount)
	}
}

func TestUserDeployQuotaAllowsOnlyConfiguredConcurrentCreates(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()
	user, err := authSvc.CreateUser(ctx, "quota-concurrent", "password123", false, 3)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	session, err := authSvc.LoginAdmin(ctx, "quota-concurrent", "password123", time.Hour)
	if err != nil {
		t.Fatalf("login user: %v", err)
	}
	srv.deployer = quotaSuccessDeployStub{}

	const attempts = 12
	statuses := make(chan int, attempts)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			body, _ := json.Marshal(DeployRequest{
				Filename: "index.html",
				Title:    "concurrent quota",
				Content:  "<!doctype html><title>concurrent quota</title>",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/deploy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
			rr := httptest.NewRecorder()
			srv.mux.ServeHTTP(rr, req)
			statuses <- rr.Code
		}()
	}
	close(start)
	wg.Wait()
	close(statuses)

	var success, limited int
	for status := range statuses {
		switch status {
		case http.StatusOK:
			success++
		case http.StatusUnauthorized:
			limited++
		default:
			t.Fatalf("concurrent deploy status = %d; want 200 or 401", status)
		}
	}
	if success != 3 || limited != attempts-success {
		t.Fatalf("concurrent deploy results: success=%d limited=%d; want 3/%d", success, limited, attempts-3)
	}
	got, err := authSvc.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.DeployCount != 3 {
		t.Fatalf("user deploy count = %d, want exactly 3", got.DeployCount)
	}
}

func TestLegacyContentPatchDoesNotConsumeSiteQuota(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()
	user, err := authSvc.CreateUser(ctx, "alice", "password123", false, 1)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := authSvc.IncrementUserDeployCount(ctx, user.ID); err != nil {
		t.Fatalf("seed deploy count: %v", err)
	}
	session, err := authSvc.LoginAdmin(ctx, "alice", "password123", time.Hour)
	if err != nil {
		t.Fatalf("login user: %v", err)
	}
	stub := newTrackingAnonymousDeployStub()
	stub.siteExists = true
	stub.created = false
	stub.siteOwnerTokenID = "user:" + user.ID
	srv.deployer = stub

	body, _ := json.Marshal(ContentPatchRequest{
		Code:        "legacy-update-test",
		Title:       "旧接口版本更新",
		Description: "旧更新接口在额度满额后追加版本",
		Content:     "<!doctype html><title>legacy update</title>",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/deploy/content", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if !stub.deployCalled {
		t.Fatal("deploy was not called for legacy content patch")
	}
	got, err := authSvc.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.DeployCount != 1 {
		t.Fatalf("user deploy count = %d, want unchanged 1", got.DeployCount)
	}
}

func TestLegacyContentPatchPassesAdminToDeployAs(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()
	admin, err := authSvc.CreateUser(ctx, "patch-admin", "password123", true, -1)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	session, err := authSvc.LoginAdmin(ctx, "patch-admin", "password123", time.Hour)
	if err != nil {
		t.Fatalf("login admin: %v", err)
	}
	if !admin.IsAdmin {
		t.Fatal("created patch user is not an admin")
	}
	base := newTrackingAnonymousDeployStub()
	base.siteExists = true
	base.created = false
	base.siteOwnerTokenID = "user:someone-else"
	stub := &adminAwareDeployStub{trackingAnonymousDeployStub: base}
	srv.deployer = stub

	body, _ := json.Marshal(ContentPatchRequest{
		Code:        "admin-patch-test",
		Title:       "管理员追加版本",
		Description: "管理员可以更新其他用户的站点",
		Content:     "<!doctype html><title>admin</title>",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/deploy/content", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if !stub.deployAsCalled || !stub.deployAsAdmin {
		t.Fatalf("DeployAs called=%v admin=%v; want admin-aware dispatch", stub.deployAsCalled, stub.deployAsAdmin)
	}
}

func TestDeployAcceptsMultipartFileUpload(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.requireAuth = false
	stub := newTrackingAnonymousDeployStub()
	srv.deployer = stub

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fields := map[string]string{
		"description": "multipart deploy",
		"title":       "Multipart Demo",
		"filename":    "index.html",
		"visibility":  "unlisted",
	}
	for key, value := range fields {
		if err := mw.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	part, err := mw.CreateFormFile("file", "index.html")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write([]byte("<!doctype html><title>Multipart Demo</title><h1>ok</h1>")); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/deploy", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("deploy status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if stub.lastReq.Filename != "index.html" || len(stub.lastReq.Files) != 1 {
		t.Fatalf("multipart request = %+v, want one uploaded index.html", stub.lastReq)
	}
	if stub.lastReq.Files[0].Path != "index.html" || !strings.Contains(stub.lastReq.Files[0].Content, "Multipart Demo") {
		t.Fatalf("uploaded file = %+v", stub.lastReq.Files[0])
	}
}

type marketplaceDeploysStub struct {
	DeployerPort
	deploys []store.MarketplaceDeploy
}

func (s marketplaceDeploysStub) ListMarketplaceDeploys(
	context.Context,
	string,
	string,
	string,
	string,
	string,
	string,
	string,
	int,
	int,
) ([]store.MarketplaceDeploy, int, error) {
	return s.deploys, len(s.deploys), nil
}

type claimedAnonymousDeployStub struct {
	DeployerPort
	deployCalled bool
}

func (s *claimedAnonymousDeployStub) GetAnonymousSession(context.Context, string) (store.AnonymousSession, error) {
	return store.AnonymousSession{
		ID:              "anon-claimed",
		ClaimedByUserID: "user-1",
	}, nil
}

func (s *claimedAnonymousDeployStub) UpdateAnonymousSessionMeta(
	context.Context,
	string,
	string,
	string,
	string,
	string,
) error {
	return nil
}

func (s *claimedAnonymousDeployStub) IncrementAnonymousSessionDeployCount(
	context.Context,
	string,
) (store.AnonymousSession, error) {
	return store.AnonymousSession{}, store.ErrNotFound
}

func (s *claimedAnonymousDeployStub) Deploy(
	context.Context,
	DeployRequest,
	string,
	string,
) (*DeployResponse, *APIError) {
	s.deployCalled = true
	return &DeployResponse{
		Success:                true,
		Code:                   "demo",
		URL:                    "http://example.test/agent/demo/",
		DetailURL:              "http://example.test/agent/demo/",
		VersionURL:             "http://example.test/agent/demo/versions/1/",
		VersionID:              "version-1",
		CurrentVersionID:       "version-1",
		PrimaryVersionStrategy: StrategyLatest,
	}, nil
}

type trackingAnonymousDeployStub struct {
	DeployerPort
	sessions         map[string]store.AnonymousSession
	createdSessionID string
	deployOwner      string
	lastReq          DeployRequest
	siteExists       bool
	siteOwnerTokenID string
	created          bool
	deployCalled     bool
	siteWriteActions []string
}

type quotaFailureDeployStub struct {
	DeployerPort
}

func (quotaFailureDeployStub) Deploy(context.Context, DeployRequest, string, string) (*DeployResponse, *APIError) {
	return nil, NewError(CodeInternal, "deploy", "forced deployment failure")
}

type quotaSuccessDeployStub struct {
	DeployerPort
}

func (quotaSuccessDeployStub) Deploy(context.Context, DeployRequest, string, string) (*DeployResponse, *APIError) {
	return &DeployResponse{
		Success:                true,
		Code:                   "quota-concurrent-test",
		URL:                    "http://example.test/agent/quota-concurrent-test/",
		DetailURL:              "http://example.test/agent/quota-concurrent-test/",
		VersionURL:             "http://example.test/agent/quota-concurrent-test/versions/1/",
		VersionID:              "quota-version-1",
		CurrentVersionID:       "quota-version-1",
		VersionNumber:          1,
		PrimaryVersionStrategy: StrategyLatest,
		Visibility:             "unlisted",
		Created:                true,
	}, nil
}

type adminAwareDeployStub struct {
	*trackingAnonymousDeployStub
	deployAsCalled bool
	deployAsAdmin  bool
}

func (s *adminAwareDeployStub) DeployAs(
	ctx context.Context,
	req DeployRequest,
	ownerTokenID, clientIP string,
	isAdmin bool,
) (*DeployResponse, *APIError) {
	s.deployAsCalled = true
	s.deployAsAdmin = isAdmin
	return s.trackingAnonymousDeployStub.Deploy(ctx, req, ownerTokenID, clientIP)
}

func newTrackingAnonymousDeployStub() *trackingAnonymousDeployStub {
	return &trackingAnonymousDeployStub{
		sessions: map[string]store.AnonymousSession{},
		created:  true,
	}
}

func (s *trackingAnonymousDeployStub) CreateAnonymousSession(
	_ context.Context,
	id string,
) (store.AnonymousSession, error) {
	now := time.Now()
	sess := store.AnonymousSession{ID: id, CreatedAt: now, LastUsedAt: now}
	s.sessions[id] = sess
	s.createdSessionID = id
	return sess, nil
}

func (s *trackingAnonymousDeployStub) GetAnonymousSession(
	_ context.Context,
	id string,
) (store.AnonymousSession, error) {
	sess, ok := s.sessions[id]
	if !ok {
		return store.AnonymousSession{}, store.ErrNotFound
	}
	return sess, nil
}

func (s *trackingAnonymousDeployStub) UpdateAnonymousSessionMeta(
	_ context.Context,
	id string,
	agentID string,
	agentLabel string,
	deviceIP string,
	userAgent string,
) error {
	sess, ok := s.sessions[id]
	if !ok {
		return store.ErrNotFound
	}
	if agentID != "" {
		sess.AgentID = agentID
	}
	if agentLabel != "" {
		sess.AgentLabel = agentLabel
	}
	if deviceIP != "" {
		sess.DeviceIP = deviceIP
	}
	if userAgent != "" {
		sess.UserAgent = userAgent
	}
	sess.LastUsedAt = time.Now()
	s.sessions[id] = sess
	return nil
}

func (s *trackingAnonymousDeployStub) IncrementAnonymousSessionDeployCount(
	_ context.Context,
	id string,
) (store.AnonymousSession, error) {
	sess, ok := s.sessions[id]
	if !ok {
		return store.AnonymousSession{}, store.ErrNotFound
	}
	sess.DeployCount++
	sess.LastUsedAt = time.Now()
	s.sessions[id] = sess
	return sess, nil
}

func (s *trackingAnonymousDeployStub) ListAnonymousSessions(
	context.Context,
	int,
) ([]store.AnonymousSession, error) {
	out := make([]store.AnonymousSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		if sess.DeployCount > 0 {
			out = append(out, sess)
		}
	}
	return out, nil
}

func (s *trackingAnonymousDeployStub) SiteExists(
	context.Context,
	string,
) (bool, error) {
	return s.siteExists, nil
}

func (s *trackingAnonymousDeployStub) GetSite(
	_ context.Context,
	code string,
) (store.Site, error) {
	if !s.siteExists {
		return store.Site{}, store.ErrNotFound
	}
	return store.Site{
		Code:         code,
		OwnerTokenID: s.siteOwnerTokenID,
		Status:       "active",
	}, nil
}

func (s *trackingAnonymousDeployStub) SwitchCurrent(_ context.Context, code string, version int64) (*SetCurrentResponse, *APIError) {
	s.siteWriteActions = append(s.siteWriteActions, "current")
	return &SetCurrentResponse{Success: true, Code: code, CurrentVersion: version}, nil
}

func (s *trackingAnonymousDeployStub) LockVersion(_ context.Context, code string, version int64, locked bool) (*LockResponse, *APIError) {
	s.siteWriteActions = append(s.siteWriteActions, "lock")
	return &LockResponse{Success: true, Code: code, VersionNumber: version, IsLocked: locked}, nil
}

func (s *trackingAnonymousDeployStub) DeleteVersion(_ context.Context, code string, version int64) (*SetCurrentResponse, *APIError) {
	s.siteWriteActions = append(s.siteWriteActions, "delete")
	return &SetCurrentResponse{Success: true, Code: code, CurrentVersion: version}, nil
}

func (s *trackingAnonymousDeployStub) SetSiteAccessPassword(context.Context, string, string) error {
	s.siteWriteActions = append(s.siteWriteActions, "access")
	return nil
}

func (s *trackingAnonymousDeployStub) Deploy(
	_ context.Context,
	req DeployRequest,
	ownerTokenID string,
	_ string,
) (*DeployResponse, *APIError) {
	s.deployCalled = true
	s.deployOwner = ownerTokenID
	s.lastReq = req
	versionNumber := 1
	if !s.created {
		versionNumber = 2
	}
	code := strings.TrimSpace(req.CustomCode)
	if code == "" {
		code = "anon-track-test"
	}
	return &DeployResponse{
		Success:                true,
		Code:                   code,
		URL:                    "http://example.test/agent/anon-track-test/",
		DetailURL:              "http://example.test/agent/anon-track-test/",
		VersionURL:             "http://example.test/agent/anon-track-test/versions/1/",
		VersionID:              "version-1",
		CurrentVersionID:       "version-1",
		VersionNumber:          versionNumber,
		PrimaryVersionStrategy: StrategyLikes,
		Visibility:             "unlisted",
		Created:                s.created,
	}, nil
}

func newTokenTestServer(t *testing.T) (*Server, *auth.Service, func()) {
	t.Helper()
	tmp := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(tmp, "hostctl.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	cfg := config.Default()
	cfg.HostedDir = filepath.Join(tmp, "hosted")
	cfg.DBPath = filepath.Join(tmp, "hostctl.db")
	cfg.CooldownSeconds = 0
	cfg.AnonymousDeployLimit = 50
	authSvc := auth.New(st)
	// Most token tests exercise the development-mode anonymous workflow; the
	// production require-auth boundary is covered separately.
	srv := New(cfg, nil, authSvc, false, log.New(bytes.NewBuffer(nil), "", 0)).
		WithVersion("test")
	return srv, authSvc, func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}
}
