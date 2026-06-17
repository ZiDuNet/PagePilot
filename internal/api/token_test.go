package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
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

type marketplaceDeploysStub struct {
	DeployerPort
	deploys []store.MarketplaceDeploy
}

func (s marketplaceDeploysStub) ListMarketplaceDeploys(
	context.Context,
	string,
	string,
	string,
	int,
	int,
) ([]store.MarketplaceDeploy, int, error) {
	return s.deploys, len(s.deploys), nil
}

func (s marketplaceDeploysStub) PublicBaseURL() string {
	return "http://example.test"
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
		URL:                    "http://example.test/demo",
		DetailURL:              "http://example.test/agent/demo/",
		VersionURL:             "http://example.test/demo?v=1",
		VersionID:              "version-1",
		CurrentVersionID:       "version-1",
		PrimaryVersionStrategy: StrategyLatest,
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
	cfg.PublicBaseURL = "http://example.test"
	cfg.CooldownSeconds = 0
	authSvc := auth.New(st)
	srv := New(cfg, nil, authSvc, true, log.New(bytes.NewBuffer(nil), "", 0)).
		WithVersion("test")
	return srv, authSvc, func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}
}
