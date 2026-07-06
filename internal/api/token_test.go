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

func TestAnonymousDeployWithoutExistingSessionIsTracked(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
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

func TestDeployAcceptsMultipartFileUpload(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
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
}

func newTrackingAnonymousDeployStub() *trackingAnonymousDeployStub {
	return &trackingAnonymousDeployStub{
		sessions: map[string]store.AnonymousSession{},
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

func (s *trackingAnonymousDeployStub) Deploy(
	_ context.Context,
	req DeployRequest,
	ownerTokenID string,
	_ string,
) (*DeployResponse, *APIError) {
	s.deployOwner = ownerTokenID
	s.lastReq = req
	return &DeployResponse{
		Success:                true,
		Code:                   "anon-track-test",
		URL:                    "http://example.test/agent/anon-track-test/",
		DetailURL:              "http://example.test/agent/anon-track-test/",
		VersionURL:             "http://example.test/agent/anon-track-test/versions/1/",
		VersionID:              "version-1",
		CurrentVersionID:       "version-1",
		VersionNumber:          1,
		PrimaryVersionStrategy: StrategyLikes,
		Visibility:             "unlisted",
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
	srv := New(cfg, nil, authSvc, true, log.New(bytes.NewBuffer(nil), "", 0)).
		WithVersion("test")
	return srv, authSvc, func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}
}
