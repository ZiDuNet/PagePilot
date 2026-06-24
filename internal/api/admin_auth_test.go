package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevAdminSessionRequiresLogin(t *testing.T) {
	srv, _, cleanup := newDevAuthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/session", nil)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
}

func TestDevAdminSitesRequiresLogin(t *testing.T) {
	srv, _, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	srv.deployer = newSitePinDeployerStub("demo", "user:owner")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/sites", nil)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
}

func TestDevAdminDeleteSiteRequiresLogin(t *testing.T) {
	srv, _, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	srv.deployer = newSitePinDeployerStub("demo", "user:owner")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sites/demo", nil)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
}

func TestDevCreateTokenRequiresLogin(t *testing.T) {
	srv, _, cleanup := newDevAuthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/token", nil)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
}

func TestAdminSessionAllowsRegisteredUserToken(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()

	user, err := authSvc.CreateUser(t.Context(), "alice", "password123", false, 20)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := authSvc.Generate(t.Context(), "alice-token", false, user.ID, nil)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/session", nil)
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), `"username":"alice"`) {
		t.Fatalf("body = %s; want registered user session", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"isAdmin":true`) {
		t.Fatalf("body = %s; regular user token must not become admin", rr.Body.String())
	}
}

func newDevAuthTestServer(t *testing.T) (*Server, string, func()) {
	t.Helper()
	srv, authSvc, cleanup := newTokenTestServer(t)
	srv.requireAuth = false

	admin, err := authSvc.CreateUser(t.Context(), "admin", "password123", true, -1)
	if err != nil {
		cleanup()
		t.Fatalf("create admin: %v", err)
	}
	token, err := authSvc.Generate(t.Context(), "admin-token", true, admin.ID, nil)
	if err != nil {
		cleanup()
		t.Fatalf("generate admin token: %v", err)
	}
	return srv, token.Plaintext, cleanup
}
