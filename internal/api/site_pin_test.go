package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

func TestAdminCanPinAndUnpinSite(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()

	admin, err := authSvc.CreateUser(ctx, "admin", "password123", true, -1)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	adminToken, err := authSvc.Generate(ctx, "admin-token", true, admin.ID, nil)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	pinStub := newSitePinDeployerStub("demo", "user:owner")
	srv.deployer = pinStub

	resp := patchSitePin(t, srv, "demo", adminToken.Plaintext, true)
	if resp.Code != http.StatusOK {
		t.Fatalf("pin status = %d, body = %s; want %d", resp.Code, resp.Body.String(), http.StatusOK)
	}

	var pinned struct {
		Success  bool    `json:"success"`
		Code     string  `json:"code"`
		IsPinned bool    `json:"isPinned"`
		PinnedAt *string `json:"pinnedAt"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &pinned); err != nil {
		t.Fatalf("decode pin response: %v", err)
	}
	if !pinned.Success || pinned.Code != "demo" || !pinned.IsPinned || pinned.PinnedAt == nil {
		t.Fatalf("pin response = %+v, want pinned site", pinned)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/sites", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken.Plaintext)
	listRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list sites status = %d, body = %s", listRR.Code, listRR.Body.String())
	}
	var list struct {
		Sites []struct {
			Code     string  `json:"code"`
			IsPinned bool    `json:"isPinned"`
			PinnedAt *string `json:"pinnedAt"`
		} `json:"sites"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list.Sites) != 1 || !list.Sites[0].IsPinned || list.Sites[0].PinnedAt == nil {
		t.Fatalf("site list pin fields = %+v, want pinned", list.Sites)
	}

	resp = patchSitePin(t, srv, "demo", adminToken.Plaintext, false)
	if resp.Code != http.StatusOK {
		t.Fatalf("unpin status = %d, body = %s; want %d", resp.Code, resp.Body.String(), http.StatusOK)
	}
	var unpinned struct {
		IsPinned bool    `json:"isPinned"`
		PinnedAt *string `json:"pinnedAt"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &unpinned); err != nil {
		t.Fatalf("decode unpin response: %v", err)
	}
	if unpinned.IsPinned || unpinned.PinnedAt != nil {
		t.Fatalf("unpin response = %+v, want not pinned", unpinned)
	}
}

func TestNonAdminCannotPinOwnedSite(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()

	user, err := authSvc.CreateUser(ctx, "alice", "password123", false, 20)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := authSvc.Generate(ctx, "alice-token", false, user.ID, nil)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}
	srv.deployer = newSitePinDeployerStub("demo", "user:"+user.ID)

	resp := patchSitePin(t, srv, "demo", token.Plaintext, true)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("pin status = %d, body = %s; want %d", resp.Code, resp.Body.String(), http.StatusForbidden)
	}
}

func TestAdminPinRequiresPinnedField(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	ctx := context.Background()

	admin, err := authSvc.CreateUser(ctx, "admin", "password123", true, -1)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	adminToken, err := authSvc.Generate(ctx, "admin-token", true, admin.ID, nil)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	srv.deployer = newSitePinDeployerStub("demo", "user:owner")

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/sites/demo/pin", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken.Plaintext)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("pin status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
}

func patchSitePin(t *testing.T, srv *Server, code, token string, pinned bool) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]bool{"pinned": pinned})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/sites/"+code+"/pin", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	return rr
}

func newSitePinDeployerStub(code, owner string) *sitePinDeployerStub {
	now := time.Now().UTC()
	return &sitePinDeployerStub{
		site: store.Site{
			Code:         code,
			PublicID:     code + "-public-id",
			OwnerTokenID: owner,
			Status:       "active",
			CreatedAt:    now,
			UpdatedAt:    now,
			Source:       "api",
		},
	}
}

type sitePinDeployerStub struct {
	DeployerPort
	site store.Site
}

func (s *sitePinDeployerStub) ListSites(context.Context) ([]store.SiteWithMeta, error) {
	return []store.SiteWithMeta{{
		Code:            s.site.Code,
		PublicID:        s.site.PublicID,
		OwnerTokenID:    s.site.OwnerTokenID,
		CreatedAt:       s.site.CreatedAt,
		UpdatedAt:       s.site.UpdatedAt,
		Source:          s.site.Source,
		Status:          s.site.Status,
		IsPinned:        s.site.IsPinned,
		PinnedAt:        s.site.PinnedAt,
		AccessProtected: s.site.AccessPasswordHash != "",
	}}, nil
}

func (s *sitePinDeployerStub) GetSite(context.Context, string) (store.Site, error) {
	if s.site.Code == "" {
		return store.Site{}, store.ErrNotFound
	}
	return s.site, nil
}

func (s *sitePinDeployerStub) SetSitePinned(_ context.Context, code string, pinned bool) error {
	if code != s.site.Code {
		return store.ErrNotFound
	}
	if pinned {
		now := time.Now().UTC()
		s.site.IsPinned = true
		s.site.PinnedAt = &now
		return nil
	}
	s.site.IsPinned = false
	s.site.PinnedAt = nil
	return nil
}
