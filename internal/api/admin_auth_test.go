package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
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

func TestRegistrationRateLimitsByIP(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.cfg.AllowRegistration = true
	for attempt := 0; attempt < registrationPerIP+1; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"alice","password":"password123"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.12:1234"
		rr := httptest.NewRecorder()
		srv.mux.ServeHTTP(rr, req)
		want := http.StatusBadRequest
		if attempt == registrationPerIP {
			want = http.StatusTooManyRequests
		}
		if rr.Code != want {
			t.Fatalf("attempt %d status = %d, body = %s; want %d", attempt+1, rr.Code, rr.Body.String(), want)
		}
	}
}

func TestValidatePasswordStrengthRejectsBlank(t *testing.T) {
	for _, password := range []string{"", "   ", "\t\n"} {
		if err := validatePasswordStrength(password); err == nil || err.ErrorCode != CodeInvalidInput {
			t.Fatalf("validatePasswordStrength(%q) = %#v; want INVALID_INPUT", password, err)
		}
	}
}

func TestAdminSetupRateLimitsCaptchaFailures(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	for i := 0; i < loginFailThreshold+1; i++ {
		captchaID := fmt.Sprintf("setup-%d", i)
		srv.captchaMu.Lock()
		srv.captchas[captchaID] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}
		srv.captchaMu.Unlock()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/setup", strings.NewReader(fmt.Sprintf(`{"username":"admin","password":"password123","captchaId":%q,"captcha":"0000"}`, captchaID)))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.10:1234"
		rr := httptest.NewRecorder()
		srv.mux.ServeHTTP(rr, req)
		want := http.StatusBadRequest
		if i == loginFailThreshold {
			want = http.StatusTooManyRequests
		}
		if rr.Code != want {
			t.Fatalf("attempt %d status = %d, body = %s; want %d", i+1, rr.Code, rr.Body.String(), want)
		}
	}
	if has, err := authSvc.HasAdminUser(context.Background()); err != nil || has {
		t.Fatalf("setup failures created an admin: has=%v err=%v", has, err)
	}
}

func TestCaptchaUsesRasterPNG(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/captcha", nil)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var response CaptchaResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(response.Image, prefix) {
		t.Fatalf("image = %q; want PNG data URL", response.Image)
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(response.Image, prefix))
	if err != nil {
		t.Fatalf("decode captcha image: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	if got := img.Bounds().Dx(); got != 144 {
		t.Fatalf("captcha image width = %d, want 144", got)
	}
}

func TestCaptchaRateLimitsByIP(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	for i := 0; i < captchaStartPerIP+1; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/captcha", nil)
		req.RemoteAddr = "198.51.100.22:1234"
		rr := httptest.NewRecorder()
		srv.mux.ServeHTTP(rr, req)
		want := http.StatusOK
		if i == captchaStartPerIP {
			want = http.StatusTooManyRequests
		}
		if rr.Code != want {
			t.Fatalf("attempt %d status = %d, body = %s; want %d", i+1, rr.Code, rr.Body.String(), want)
		}
	}
}

func TestAdminSetupIsDisabledWhenAuthenticationIsRequired(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.requireAuth = true
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup", strings.NewReader(`{"username":"admin","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusForbidden)
	}
}

func TestAdminLoginRateLimitsByIPAcrossUsernames(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	if _, err := authSvc.CreateUser(context.Background(), "admin", "password123", true, -1); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	for i := 0; i < loginFailThreshold+1; i++ {
		captchaID := fmt.Sprintf("login-%d", i)
		srv.captchaMu.Lock()
		srv.captchas[captchaID] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}
		srv.captchaMu.Unlock()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(fmt.Sprintf(`{"username":"admin-%d","password":"wrong1234","captchaId":%q,"captcha":"1234"}`, i, captchaID)))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.11:1234"
		rr := httptest.NewRecorder()
		srv.mux.ServeHTTP(rr, req)
		want := http.StatusUnauthorized
		if i == loginFailThreshold {
			want = http.StatusTooManyRequests
		}
		if rr.Code != want {
			t.Fatalf("attempt %d status = %d, body = %s; want %d", i+1, rr.Code, rr.Body.String(), want)
		}
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
	if _, err := authSvc.GetUser(context.Background(), "alice"); err == nil {
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
	user, err := authSvc.GetUser(context.Background(), out.UserID)
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

	user, err := authSvc.CreateUser(context.Background(), "alice", "password123", false, 20)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := authSvc.Generate(context.Background(), "alice-token", false, user.ID, nil)
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

func TestDisablingUserRevokesExistingBearerToken(t *testing.T) {
	srv, adminToken, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	user, err := srv.auth.CreateUser(context.Background(), "disabled-user", "password123", false, 20)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userToken, err := srv.auth.Generate(context.Background(), "disabled-user-token", false, user.ID, nil)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+user.ID, strings.NewReader(`{"isActive":false}`))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(updateRR, updateReq)
	if updateRR.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s; want 200", updateRR.Code, updateRR.Body.String())
	}

	verifyReq := httptest.NewRequest(http.MethodGet, "/api/admin/session", nil)
	verifyReq.Header.Set("Authorization", "Bearer "+userToken.Plaintext)
	verifyRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(verifyRR, verifyReq)
	if verifyRR.Code != http.StatusForbidden {
		t.Fatalf("disabled token status = %d, body = %s; want 403", verifyRR.Code, verifyRR.Body.String())
	}
	stored, err := srv.auth.GetToken(context.Background(), userToken.ID)
	if err != nil {
		t.Fatalf("load revoked token: %v", err)
	}
	if !stored.IsRevoked {
		t.Fatal("disabling a user did not revoke their existing bearer token")
	}
}

func TestDemotingAdminRevokesExistingAdminToken(t *testing.T) {
	srv, rootToken, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	secondAdmin, err := srv.auth.CreateUser(context.Background(), "second-admin", "password123", true, -1)
	if err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	adminToken, err := srv.auth.Generate(context.Background(), "second-admin-token", true, secondAdmin.ID, nil)
	if err != nil {
		t.Fatalf("generate second admin token: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+secondAdmin.ID, strings.NewReader(`{"isAdmin":false}`))
	updateReq.Header.Set("Authorization", "Bearer "+rootToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(updateRR, updateReq)
	if updateRR.Code != http.StatusOK {
		t.Fatalf("demote status = %d, body = %s; want 200", updateRR.Code, updateRR.Body.String())
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	adminReq.Header.Set("Authorization", "Bearer "+adminToken.Plaintext)
	adminRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(adminRR, adminReq)
	if adminRR.Code != http.StatusForbidden {
		t.Fatalf("demoted admin token status = %d, body = %s; want 403", adminRR.Code, adminRR.Body.String())
	}
}

func TestPrivilegeChangeRevokesExistingAdminSession(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "disable", body: `{"isActive":false}`},
		{name: "demote", body: `{"isAdmin":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, rootToken, cleanup := newDevAuthTestServer(t)
			defer cleanup()
			target, err := srv.auth.CreateUser(context.Background(), "session-"+tc.name, "password123", true, -1)
			if err != nil {
				t.Fatalf("create target admin: %v", err)
			}
			session, err := srv.auth.LoginAdmin(context.Background(), target.Username, "password123", time.Hour)
			if err != nil {
				t.Fatalf("login target admin: %v", err)
			}

			updateReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+target.ID, strings.NewReader(tc.body))
			updateReq.Header.Set("Authorization", "Bearer "+rootToken)
			updateReq.Header.Set("Content-Type", "application/json")
			updateRR := httptest.NewRecorder()
			srv.mux.ServeHTTP(updateRR, updateReq)
			if updateRR.Code != http.StatusOK {
				t.Fatalf("update status = %d, body = %s; want 200", updateRR.Code, updateRR.Body.String())
			}

			if _, _, err := srv.auth.VerifyAdminSession(context.Background(), session.Plaintext); err == nil {
				t.Fatal("privilege change did not revoke the existing admin session")
			}
		})
	}
}

func TestCannotDisableLastActiveAdmin(t *testing.T) {
	srv, adminToken, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	admin, err := srv.auth.VerifyToken(context.Background(), "Bearer "+adminToken)
	if err != nil {
		t.Fatalf("verify bootstrap admin token: %v", err)
	}
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+admin.OwnerUserID, strings.NewReader(`{"isActive":false}`))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(updateRR, updateReq)
	if updateRR.Code != http.StatusBadRequest {
		t.Fatalf("disable last admin status = %d, body = %s; want 400", updateRR.Code, updateRR.Body.String())
	}
}

func newDevAuthTestServer(t *testing.T) (*Server, string, func()) {
	t.Helper()
	srv, authSvc, cleanup := newTokenTestServer(t)
	srv.requireAuth = false

	admin, err := authSvc.CreateUser(context.Background(), "admin", "password123", true, -1)
	if err != nil {
		cleanup()
		t.Fatalf("create admin: %v", err)
	}
	token, err := authSvc.Generate(context.Background(), "admin-token", true, admin.ID, nil)
	if err != nil {
		cleanup()
		t.Fatalf("generate admin token: %v", err)
	}
	return srv, token.Plaintext, cleanup
}
