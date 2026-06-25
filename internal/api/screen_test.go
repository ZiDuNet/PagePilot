package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/auth"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
)

func TestScreensRejectAnonymousList(t *testing.T) {
	srv, cleanup := newScreenAPITestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/screens", nil)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
}

func TestScreenBindInvalidPairingCodeReturnsActionableError(t *testing.T) {
	srv, cleanup := newScreenAPITestServer(t)
	defer cleanup()

	_, token := createScreenAPIUser(t, srv, "alice")
	bindBody, _ := json.Marshal(ScreenBindRequest{PairingCode: "000000"})
	bindReq := httptest.NewRequest(http.MethodPost, "/api/screens/bind", bytes.NewReader(bindBody))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Authorization", "Bearer "+token)
	bindRR := httptest.NewRecorder()

	srv.mux.ServeHTTP(bindRR, bindReq)

	if bindRR.Code != http.StatusBadRequest {
		t.Fatalf("bind status = %d, body = %s; want %d", bindRR.Code, bindRR.Body.String(), http.StatusBadRequest)
	}
	var apiErr APIError
	if err := json.Unmarshal(bindRR.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode bind error: %v", err)
	}
	if apiErr.ErrorCode != CodeInvalidInput || apiErr.Stage != "pairingCode" {
		t.Fatalf("bind error = %+v, want invalid pairingCode", apiErr)
	}
	if apiErr.Detail == "" || apiErr.Detail == "not found" || apiErr.Hint == "" {
		t.Fatalf("bind error should explain invalid pairing code, got %+v", apiErr)
	}
}

func TestRegisteredUserCanBindPublishAndDeviceCanReadManifest(t *testing.T) {
	srv, cleanup := newScreenAPITestServer(t)
	defer cleanup()
	ctx := context.Background()

	user, token := createScreenAPIUser(t, srv, "alice")
	seedScreenPairing(t, srv, "pair-1", "123456", "pair-secret", "screen-1")
	seedScreenSite(t, srv, "demo-app", "user:"+user.ID)

	bindBody, _ := json.Marshal(ScreenBindRequest{PairingCode: "123456", Name: "大厅屏"})
	bindReq := httptest.NewRequest(http.MethodPost, "/api/screens/bind", bytes.NewReader(bindBody))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Authorization", "Bearer "+token)
	bindRR := httptest.NewRecorder()

	srv.mux.ServeHTTP(bindRR, bindReq)

	if bindRR.Code != http.StatusOK {
		t.Fatalf("bind status = %d, body = %s; want %d", bindRR.Code, bindRR.Body.String(), http.StatusOK)
	}
	var bindResp ScreenBindResponse
	if err := json.Unmarshal(bindRR.Body.Bytes(), &bindResp); err != nil {
		t.Fatalf("decode bind response: %v", err)
	}
	if bindResp.Screen.ID != "screen-1" || bindResp.Screen.Name != "大厅屏" {
		t.Fatalf("bind response = %+v", bindResp)
	}

	if err := srv.deployer.CompleteScreenPairing(ctx, "pair-1", auth.HashToken("pair-secret"), auth.HashToken("device-token")); err != nil {
		t.Fatalf("complete pairing: %v", err)
	}

	publishBody, _ := json.Marshal(ScreenPublishRequest{Code: "demo-app"})
	publishReq := httptest.NewRequest(http.MethodPost, "/api/screens/screen-1/publish", bytes.NewReader(publishBody))
	publishReq.Header.Set("Content-Type", "application/json")
	publishReq.Header.Set("Authorization", "Bearer "+token)
	publishRR := httptest.NewRecorder()

	srv.mux.ServeHTTP(publishRR, publishReq)

	if publishRR.Code != http.StatusOK {
		t.Fatalf("publish status = %d, body = %s; want %d", publishRR.Code, publishRR.Body.String(), http.StatusOK)
	}

	manifestReq := httptest.NewRequest(http.MethodGet, "/api/device/manifest", nil)
	manifestReq.Header.Set("Authorization", "Device device-token")
	manifestRR := httptest.NewRecorder()

	srv.mux.ServeHTTP(manifestRR, manifestReq)

	if manifestRR.Code != http.StatusOK {
		t.Fatalf("manifest status = %d, body = %s; want %d", manifestRR.Code, manifestRR.Body.String(), http.StatusOK)
	}
	var manifest ScreenManifestResponse
	if err := json.Unmarshal(manifestRR.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.EntryURL != "http://example.test/agent/demo-app/versions/1/" {
		t.Fatalf("entry URL = %q, want app URL", manifest.EntryURL)
	}
	if manifest.Version != 1 || len(manifest.Assets) != 1 || manifest.Assets[0].Path != "index.html" {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestScreenPublishRejectsOtherUsersSite(t *testing.T) {
	srv, cleanup := newScreenAPITestServer(t)
	defer cleanup()

	user, token := createScreenAPIUser(t, srv, "alice")
	createScreenAPIUser(t, srv, "bob")
	seedScreenPairing(t, srv, "pair-1", "123456", "pair-secret", "screen-1")
	seedScreenSite(t, srv, "bob-app", "user:bob-id")
	if err := srv.deployer.(*screenAPIDeployerStub).st.SetSiteVisibility(context.Background(), "bob-app", "unlisted"); err != nil {
		t.Fatalf("set bob app visibility: %v", err)
	}

	bindBody, _ := json.Marshal(ScreenBindRequest{PairingCode: "123456"})
	bindReq := httptest.NewRequest(http.MethodPost, "/api/screens/bind", bytes.NewReader(bindBody))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Authorization", "Bearer "+token)
	bindRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(bindRR, bindReq)
	if bindRR.Code != http.StatusOK {
		t.Fatalf("bind status = %d, body = %s", bindRR.Code, bindRR.Body.String())
	}

	site, err := srv.deployer.GetSite(context.Background(), "bob-app")
	if err != nil {
		t.Fatalf("get bob app: %v", err)
	}
	if site.OwnerTokenID == "user:"+user.ID {
		t.Fatal("test setup used alice as bob-app owner")
	}

	publishBody, _ := json.Marshal(ScreenPublishRequest{Code: "bob-app"})
	publishReq := httptest.NewRequest(http.MethodPost, "/api/screens/screen-1/publish", bytes.NewReader(publishBody))
	publishReq.Header.Set("Content-Type", "application/json")
	publishReq.Header.Set("Authorization", "Bearer "+token)
	publishRR := httptest.NewRecorder()

	srv.mux.ServeHTTP(publishRR, publishReq)

	if publishRR.Code != http.StatusForbidden {
		t.Fatalf("publish status = %d, body = %s; want %d", publishRR.Code, publishRR.Body.String(), http.StatusForbidden)
	}
}

func TestScreenPublishAllowsPublicMarketplaceSite(t *testing.T) {
	srv, cleanup := newScreenAPITestServer(t)
	defer cleanup()

	_, token := createScreenAPIUser(t, srv, "alice")
	createScreenAPIUser(t, srv, "bob")
	seedScreenPairing(t, srv, "pair-1", "123456", "pair-secret", "screen-1")
	seedScreenSite(t, srv, "bob-app", "user:bob-id")

	bindBody, _ := json.Marshal(ScreenBindRequest{PairingCode: "123456"})
	bindReq := httptest.NewRequest(http.MethodPost, "/api/screens/bind", bytes.NewReader(bindBody))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Authorization", "Bearer "+token)
	bindRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(bindRR, bindReq)
	if bindRR.Code != http.StatusOK {
		t.Fatalf("bind status = %d, body = %s", bindRR.Code, bindRR.Body.String())
	}

	publishBody, _ := json.Marshal(ScreenPublishRequest{Code: "bob-app"})
	publishReq := httptest.NewRequest(http.MethodPost, "/api/screens/screen-1/publish", bytes.NewReader(publishBody))
	publishReq.Header.Set("Content-Type", "application/json")
	publishReq.Header.Set("Authorization", "Bearer "+token)
	publishRR := httptest.NewRecorder()

	srv.mux.ServeHTTP(publishRR, publishReq)

	if publishRR.Code != http.StatusOK {
		t.Fatalf("publish status = %d, body = %s; want %d", publishRR.Code, publishRR.Body.String(), http.StatusOK)
	}
}

func TestScreenScreenshotRequiresBackendCommand(t *testing.T) {
	srv, cleanup := newScreenAPITestServer(t)
	defer cleanup()
	ctx := context.Background()

	_, token := createScreenAPIUser(t, srv, "alice")
	seedScreenPairing(t, srv, "pair-1", "123456", "pair-secret", "screen-1")

	bindBody, _ := json.Marshal(ScreenBindRequest{PairingCode: "123456"})
	bindReq := httptest.NewRequest(http.MethodPost, "/api/screens/bind", bytes.NewReader(bindBody))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Authorization", "Bearer "+token)
	bindRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(bindRR, bindReq)
	if bindRR.Code != http.StatusOK {
		t.Fatalf("bind status = %d, body = %s", bindRR.Code, bindRR.Body.String())
	}
	if err := srv.deployer.CompleteScreenPairing(ctx, "pair-1", auth.HashToken("pair-secret"), auth.HashToken("device-token")); err != nil {
		t.Fatalf("complete pairing: %v", err)
	}

	uploadBody, _ := json.Marshal(DeviceScreenshotRequest{
		RequestID:     "stale",
		ContentBase64: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=",
		MimeType:      "image/png",
		Width:         1,
		Height:        1,
	})
	staleReq := httptest.NewRequest(http.MethodPost, "/api/device/screenshot", bytes.NewReader(uploadBody))
	staleReq.Header.Set("Content-Type", "application/json")
	staleReq.Header.Set("Authorization", "Device device-token")
	staleRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(staleRR, staleReq)
	if staleRR.Code != http.StatusConflict {
		t.Fatalf("stale screenshot status = %d, body = %s; want %d", staleRR.Code, staleRR.Body.String(), http.StatusConflict)
	}

	requestReq := httptest.NewRequest(http.MethodPost, "/api/screens/screen-1/screenshot", nil)
	requestReq.Header.Set("Authorization", "Bearer "+token)
	requestRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(requestRR, requestReq)
	if requestRR.Code != http.StatusOK {
		t.Fatalf("request screenshot status = %d, body = %s; want %d", requestRR.Code, requestRR.Body.String(), http.StatusOK)
	}
	var requestResp ScreenScreenshotResponse
	if err := json.Unmarshal(requestRR.Body.Bytes(), &requestResp); err != nil {
		t.Fatalf("decode screenshot command: %v", err)
	}
	if requestResp.Screenshot == nil || requestResp.Screenshot.RequestID == "" {
		t.Fatalf("screenshot command missing: %+v", requestResp)
	}

	upload := DeviceScreenshotRequest{
		RequestID:     requestResp.Screenshot.RequestID,
		ContentBase64: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=",
		MimeType:      "image/png",
		Width:         1,
		Height:        1,
	}
	okBody, _ := json.Marshal(upload)
	okReq := httptest.NewRequest(http.MethodPost, "/api/device/screenshot", bytes.NewReader(okBody))
	okReq.Header.Set("Content-Type", "application/json")
	okReq.Header.Set("Authorization", "Device device-token")
	okRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(okRR, okReq)
	if okRR.Code != http.StatusOK {
		t.Fatalf("upload screenshot status = %d, body = %s; want %d", okRR.Code, okRR.Body.String(), http.StatusOK)
	}

	viewReq := httptest.NewRequest(http.MethodGet, "/api/screens/screen-1/screenshot", nil)
	viewReq.Header.Set("Authorization", "Bearer "+token)
	viewRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(viewRR, viewReq)
	if viewRR.Code != http.StatusOK {
		t.Fatalf("view screenshot status = %d, body = %s; want %d", viewRR.Code, viewRR.Body.String(), http.StatusOK)
	}
	if got := viewRR.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q, want image/png", got)
	}
}

func TestScreenCommandIsDeliveredAndAcknowledged(t *testing.T) {
	srv, cleanup := newScreenAPITestServer(t)
	defer cleanup()
	ctx := context.Background()

	_, token := createScreenAPIUser(t, srv, "alice")
	seedScreenPairing(t, srv, "pair-1", "123456", "pair-secret", "screen-1")

	bindBody, _ := json.Marshal(ScreenBindRequest{PairingCode: "123456"})
	bindReq := httptest.NewRequest(http.MethodPost, "/api/screens/bind", bytes.NewReader(bindBody))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Authorization", "Bearer "+token)
	bindRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(bindRR, bindReq)
	if bindRR.Code != http.StatusOK {
		t.Fatalf("bind status = %d, body = %s", bindRR.Code, bindRR.Body.String())
	}
	if err := srv.deployer.CompleteScreenPairing(ctx, "pair-1", auth.HashToken("pair-secret"), auth.HashToken("device-token")); err != nil {
		t.Fatalf("complete pairing: %v", err)
	}

	commandBody, _ := json.Marshal(ScreenCommandRequest{Type: "refresh"})
	commandReq := httptest.NewRequest(http.MethodPost, "/api/screens/screen-1/command", bytes.NewReader(commandBody))
	commandReq.Header.Set("Content-Type", "application/json")
	commandReq.Header.Set("Authorization", "Bearer "+token)
	commandRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(commandRR, commandReq)
	if commandRR.Code != http.StatusOK {
		t.Fatalf("command status = %d, body = %s; want %d", commandRR.Code, commandRR.Body.String(), http.StatusOK)
	}
	var commandResp ScreenCommandResponse
	if err := json.Unmarshal(commandRR.Body.Bytes(), &commandResp); err != nil {
		t.Fatalf("decode command response: %v", err)
	}
	if commandResp.Command == nil || commandResp.Command.Type != "refresh" || commandResp.Command.RequestID == "" {
		t.Fatalf("command response missing command: %+v", commandResp)
	}

	manifestReq := httptest.NewRequest(http.MethodGet, "/api/device/manifest", nil)
	manifestReq.Header.Set("Authorization", "Device device-token")
	manifestRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(manifestRR, manifestReq)
	if manifestRR.Code != http.StatusOK {
		t.Fatalf("manifest status = %d, body = %s; want %d", manifestRR.Code, manifestRR.Body.String(), http.StatusOK)
	}
	var manifest ScreenManifestResponse
	if err := json.Unmarshal(manifestRR.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Command == nil || manifest.Command.RequestID != commandResp.Command.RequestID || manifest.Command.Type != "refresh" {
		t.Fatalf("manifest command = %+v, want refresh %s", manifest.Command, commandResp.Command.RequestID)
	}

	ackBody, _ := json.Marshal(DeviceCommandAckRequest{RequestID: commandResp.Command.RequestID, Type: "refresh"})
	ackReq := httptest.NewRequest(http.MethodPost, "/api/device/command/ack", bytes.NewReader(ackBody))
	ackReq.Header.Set("Content-Type", "application/json")
	ackReq.Header.Set("Authorization", "Device device-token")
	ackRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(ackRR, ackReq)
	if ackRR.Code != http.StatusOK {
		t.Fatalf("ack status = %d, body = %s; want %d", ackRR.Code, ackRR.Body.String(), http.StatusOK)
	}

	manifestAfterReq := httptest.NewRequest(http.MethodGet, "/api/device/manifest", nil)
	manifestAfterReq.Header.Set("Authorization", "Device device-token")
	manifestAfterRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(manifestAfterRR, manifestAfterReq)
	if manifestAfterRR.Code != http.StatusOK {
		t.Fatalf("manifest after ack status = %d, body = %s", manifestAfterRR.Code, manifestAfterRR.Body.String())
	}
	var manifestAfter ScreenManifestResponse
	if err := json.Unmarshal(manifestAfterRR.Body.Bytes(), &manifestAfter); err != nil {
		t.Fatalf("decode manifest after ack: %v", err)
	}
	if manifestAfter.Command != nil {
		t.Fatalf("manifest command after ack = %+v, want nil", manifestAfter.Command)
	}
}

func TestAdminCanPublishAnyOwnedSiteToAnyScreen(t *testing.T) {
	srv, cleanup := newScreenAPITestServer(t)
	defer cleanup()

	user, userToken := createScreenAPIUser(t, srv, "alice")
	admin, err := srv.auth.CreateUser(context.Background(), "root", "password123", true, -1)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	adminToken, err := srv.auth.Generate(context.Background(), "admin-token", true, admin.ID, nil)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	seedScreenPairing(t, srv, "pair-1", "123456", "pair-secret", "screen-1")
	seedScreenSite(t, srv, "demo-app", "user:"+user.ID)

	bindBody, _ := json.Marshal(ScreenBindRequest{PairingCode: "123456"})
	bindReq := httptest.NewRequest(http.MethodPost, "/api/screens/bind", bytes.NewReader(bindBody))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("Authorization", "Bearer "+userToken)
	bindRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(bindRR, bindReq)
	if bindRR.Code != http.StatusOK {
		t.Fatalf("bind status = %d, body = %s", bindRR.Code, bindRR.Body.String())
	}

	publishBody, _ := json.Marshal(ScreenPublishRequest{Code: "demo-app"})
	publishReq := httptest.NewRequest(http.MethodPost, "/api/screens/screen-1/publish", bytes.NewReader(publishBody))
	publishReq.Header.Set("Content-Type", "application/json")
	publishReq.Header.Set("Authorization", "Bearer "+adminToken.Plaintext)
	publishRR := httptest.NewRecorder()

	srv.mux.ServeHTTP(publishRR, publishReq)

	if publishRR.Code != http.StatusOK {
		t.Fatalf("admin publish status = %d, body = %s; want %d", publishRR.Code, publishRR.Body.String(), http.StatusOK)
	}
}

func newScreenAPITestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	tmp := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(tmp, "hostctl.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	cfg := config.Default()
	cfg.HostedDir = filepath.Join(tmp, "hosted")
	cfg.DBPath = filepath.Join(tmp, "hostctl.db")
	cfg.PublicBaseURL = "http://example.test"
	cfg.CooldownSeconds = 0
	authSvc := auth.New(st)
	srv := New(cfg, &screenAPIDeployerStub{st: st, baseURL: cfg.PublicBaseURL}, authSvc, true, log.New(bytes.NewBuffer(nil), "", 0)).
		WithVersion("test")
	return srv, func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}
}

func createScreenAPIUser(t *testing.T, srv *Server, username string) (store.AdminUser, string) {
	t.Helper()
	userID := username + "-id"
	user, err := srv.auth.CreateUser(context.Background(), username, "password123", false, 20)
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if user.ID != "" {
		userID = user.ID
	}
	token, err := srv.auth.Generate(context.Background(), username+"-token", false, userID, nil)
	if err != nil {
		t.Fatalf("generate token %s: %v", username, err)
	}
	user.ID = userID
	return user, token.Plaintext
}

func seedScreenPairing(t *testing.T, srv *Server, pairingID, code, secret, screenID string) {
	t.Helper()
	err := srv.deployer.CreateScreenPairing(context.Background(), store.ScreenPairing{
		ID:                pairingID,
		Code:              code,
		PairingSecretHash: auth.HashToken(secret),
		ScreenID:          screenID,
		DeviceName:        "测试屏幕",
		ExpiresAt:         time.Now().UTC().Add(5 * time.Minute),
		CreatedAt:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed screen pairing: %v", err)
	}
}

func seedScreenSite(t *testing.T, srv *Server, code, ownerTokenID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := srv.deployer.(*screenAPIDeployerStub).st.CreateSite(ctx, store.Site{
		Code:         code,
		PublicID:     code + "-public",
		OwnerTokenID: ownerTokenID,
		CreatedAt:    now,
		UpdatedAt:    now,
		Source:       "api",
	}); err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := srv.deployer.(*screenAPIDeployerStub).st.CreateVersion(ctx, store.Version{
		ID:            code + "-version",
		SiteCode:      code,
		VersionNumber: 1,
		Title:         code,
		Description:   "screen test app",
		MainEntry:     "index.html",
		TotalSize:     42,
		FileCount:     1,
		ContentSha256: code + "-sha",
		Status:        "active",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create version: %v", err)
	}
	if err := srv.deployer.(*screenAPIDeployerStub).st.CreateFiles(ctx, []store.FileMeta{{
		SiteCode:      code,
		VersionNumber: 1,
		Path:          "index.html",
		Size:          42,
		Sha256:        code + "-file-sha",
		IsBinary:      false,
	}}); err != nil {
		t.Fatalf("create files: %v", err)
	}
	v := int64(1)
	if err := srv.deployer.(*screenAPIDeployerStub).st.SetCurrentVersion(ctx, code, &v); err != nil {
		t.Fatalf("set current: %v", err)
	}
}

type screenAPIDeployerStub struct {
	DeployerPort
	st      *store.SQLiteStore
	baseURL string
}

func (s *screenAPIDeployerStub) PublicBaseURL() string {
	return s.baseURL
}

func (s *screenAPIDeployerStub) CreateScreenPairing(ctx context.Context, pairing store.ScreenPairing) error {
	return s.st.CreateScreenPairing(ctx, pairing)
}

func (s *screenAPIDeployerStub) BindScreenPairing(ctx context.Context, code, ownerUserID, name string) (store.Screen, error) {
	return s.st.BindScreenPairing(ctx, code, ownerUserID, name)
}

func (s *screenAPIDeployerStub) CompleteScreenPairing(ctx context.Context, pairingID, pairingSecretHash, deviceTokenHash string) error {
	return s.st.CompleteScreenPairing(ctx, pairingID, pairingSecretHash, deviceTokenHash)
}

func (s *screenAPIDeployerStub) GetScreen(ctx context.Context, id string) (store.Screen, error) {
	return s.st.GetScreen(ctx, id)
}

func (s *screenAPIDeployerStub) GetScreenByDeviceTokenHash(ctx context.Context, hash string) (store.Screen, error) {
	return s.st.GetScreenByDeviceTokenHash(ctx, hash)
}

func (s *screenAPIDeployerStub) ListScreensByUser(ctx context.Context, ownerUserID string) ([]store.Screen, error) {
	return s.st.ListScreensByUser(ctx, ownerUserID)
}

func (s *screenAPIDeployerStub) PublishScreen(ctx context.Context, screenID, ownerUserID, siteCode string, version *int64) error {
	return s.st.PublishScreen(ctx, screenID, ownerUserID, siteCode, version)
}

func (s *screenAPIDeployerStub) TouchScreenHeartbeat(ctx context.Context, screenID, appVersion, runtime, deviceInfo string) (store.Screen, error) {
	return s.st.TouchScreenHeartbeat(ctx, screenID, appVersion, runtime, deviceInfo)
}

func (s *screenAPIDeployerStub) RequestScreenScreenshot(ctx context.Context, screenID, requestID string) (store.Screen, error) {
	return s.st.RequestScreenScreenshot(ctx, screenID, requestID)
}

func (s *screenAPIDeployerStub) CompleteScreenScreenshot(ctx context.Context, screenID, requestID string, screenshotAt time.Time) (store.Screen, error) {
	return s.st.CompleteScreenScreenshot(ctx, screenID, requestID, screenshotAt)
}

func (s *screenAPIDeployerStub) RequestScreenCommand(ctx context.Context, screenID, requestID, commandType, payload string) (store.Screen, error) {
	return s.st.RequestScreenCommand(ctx, screenID, requestID, commandType, payload)
}

func (s *screenAPIDeployerStub) CompleteScreenCommand(ctx context.Context, screenID, requestID string, completedAt time.Time) (store.Screen, error) {
	return s.st.CompleteScreenCommand(ctx, screenID, requestID, completedAt)
}

func (s *screenAPIDeployerStub) UnbindScreen(ctx context.Context, screenID, ownerUserID string) error {
	return s.st.UnbindScreen(ctx, screenID, ownerUserID)
}

func (s *screenAPIDeployerStub) ListScreens(ctx context.Context) ([]store.Screen, error) {
	return s.st.ListScreens(ctx)
}

func (s *screenAPIDeployerStub) GetSite(ctx context.Context, code string) (store.Site, error) {
	return s.st.GetSite(ctx, code)
}

func (s *screenAPIDeployerStub) GetContent(ctx context.Context, code string, versionPtr *int64) (*GetContentResponse, *APIError) {
	site, err := s.st.GetSite(ctx, code)
	if err != nil {
		return nil, NewError(CodeNotFound, "site", "site not found")
	}
	version := int64(0)
	if versionPtr != nil {
		version = *versionPtr
	} else if site.CurrentVersion != nil {
		version = *site.CurrentVersion
	}
	v, err := s.st.GetVersion(ctx, code, version)
	if err != nil {
		return nil, NewError(CodeNotFound, "version", "version not found")
	}
	files, err := s.st.ListFiles(ctx, code, version)
	if err != nil {
		return nil, NewError(CodeInternal, "files", err.Error())
	}
	resp := &GetContentResponse{
		Success:     true,
		Code:        code,
		Version:     version,
		Title:       v.Title,
		Description: v.Description,
		MainEntry:   v.MainEntry,
		TotalSize:   v.TotalSize,
		Files:       make([]ContentFile, 0, len(files)),
		CreatedAt:   v.CreatedAt,
	}
	for _, f := range files {
		resp.Files = append(resp.Files, ContentFile{
			Path:     f.Path,
			Size:     f.Size,
			Sha256:   f.Sha256,
			IsBinary: f.IsBinary,
		})
	}
	return resp, nil
}
