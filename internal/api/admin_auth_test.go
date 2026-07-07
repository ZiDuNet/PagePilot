package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegisterCanBeDisabledByConfig(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AllowRegistration = false

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"alice","password":"password123","captchaId":"x","captcha":"1234"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusForbidden)
	}
	if !strings.Contains(rr.Body.String(), "registration is disabled") {
		t.Fatalf("body = %s; want disabled registration error", rr.Body.String())
	}
}

func TestRegisterRequiresEmailCodeWhenVerificationEnabled(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AllowRegistration = true
	srv.cfg.EmailVerificationEnabled = true
	srv.cfg.SMTPHost = "smtp.example.test"
	srv.cfg.SMTPFrom = "noreply@example.test"
	srv.captchas["captcha-register"] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"alice","email":"Alice@Example.COM","password":"password123","captchaId":"captcha-register","captcha":"1234"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "email verification code") {
		t.Fatalf("body = %s; want email verification error", rr.Body.String())
	}
	if _, err := authSvc.GetUser(t.Context(), "alice"); err == nil {
		t.Fatal("user was created without verified email code")
	}
}

func TestRegisterConsumesValidEmailCode(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AllowRegistration = true
	srv.cfg.EmailVerificationEnabled = true
	srv.cfg.SMTPHost = "smtp.example.test"
	srv.cfg.SMTPFrom = "noreply@example.test"
	srv.captchas["captcha-register"] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}
	srv.emailCodes["alice@example.com"] = emailVerificationChallenge{Code: "654321", ExpiresAt: time.Now().Add(time.Minute)}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"alice","email":"Alice@Example.COM","password":"password123","captchaId":"captcha-register","captcha":"1234","emailCode":"654321"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var out RegisterResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	user, err := authSvc.GetUser(t.Context(), out.UserID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.Email != "alice@example.com" || !user.EmailVerified {
		t.Fatalf("email fields = %q verified=%v; want verified alice@example.com", user.Email, user.EmailVerified)
	}
	if _, ok := srv.emailCodes["alice@example.com"]; ok {
		t.Fatal("email code was not consumed after successful registration")
	}
}

func TestEmailCodeEndpointRequiresSMTPConfig(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AllowRegistration = true
	srv.cfg.EmailVerificationEnabled = true
	srv.captchas["captcha-email"] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/email-code", strings.NewReader(`{"email":"alice@example.com","captchaId":"captcha-email","captcha":"1234"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "SMTP") {
		t.Fatalf("body = %s; want SMTP config error", rr.Body.String())
	}
}

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

func TestOptionalAdminSessionDoesNotRequireLogin(t *testing.T) {
	srv, _, cleanup := newDevAuthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/session?optional=1", nil)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), `"success":false`) {
		t.Fatalf("body = %s; want anonymous optional session", rr.Body.String())
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

func TestAdminUserManagementIncludesEmailFields(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()

	createBody := `{"username":"bob","email":"Bob@Example.COM","emailVerified":true,"password":"password123","isAdmin":false,"deployLimit":9}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s; want %d", createRR.Code, createRR.Body.String(), http.StatusOK)
	}
	var createOut UserCreateResponse
	if err := json.Unmarshal(createRR.Body.Bytes(), &createOut); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createOut.User.Email != "bob@example.com" || !createOut.User.EmailVerified {
		t.Fatalf("created email = %q verified=%v; want normalized verified email", createOut.User.Email, createOut.User.EmailVerified)
	}

	patchBody := `{"email":"bob2@example.com","emailVerified":false,"deployLimit":11}`
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+createOut.User.ID, strings.NewReader(patchBody))
	patchReq.Header.Set("Authorization", "Bearer "+token)
	patchReq.Header.Set("Content-Type", "application/json")
	patchRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s; want %d", patchRR.Code, patchRR.Body.String(), http.StatusOK)
	}
	var patchOut UserUpdateResponse
	if err := json.Unmarshal(patchRR.Body.Bytes(), &patchOut); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if patchOut.User.Email != "bob2@example.com" || patchOut.User.EmailVerified || patchOut.User.DeployLimit != 11 {
		t.Fatalf("updated user = %+v; want changed unverified email and deploy limit", patchOut.User)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s; want %d", listRR.Code, listRR.Body.String(), http.StatusOK)
	}
	var listOut UserListResponse
	if err := json.Unmarshal(listRR.Body.Bytes(), &listOut); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	found := false
	for _, user := range listOut.Users {
		if user.ID == createOut.User.ID {
			found = true
			if user.Email != "bob2@example.com" || user.EmailVerified {
				t.Fatalf("listed user = %+v; want updated email fields", user)
			}
		}
	}
	if !found {
		t.Fatalf("created user %s missing from list", createOut.User.ID)
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
