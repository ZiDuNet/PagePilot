package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourorg/hostctl/internal/store"
)

func TestAnonymousVisitorCanLikeDeploy(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	srv.deployer = &siteLikeDeployerStub{
		site: store.Site{Code: "demo"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/deploys/demo/like", nil)
	req.Header.Set("User-Agent", "anon-browser")
	resp := httptest.NewRecorder()
	srv.mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("like status = %d, body = %s; want %d", resp.Code, resp.Body.String(), http.StatusOK)
	}

	var out struct {
		Success   bool   `json:"success"`
		Code      string `json:"code"`
		LikeCount int64  `json:"likeCount"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode like response: %v", err)
	}
	if !out.Success || out.Code != "demo" || out.LikeCount != 1 {
		t.Fatalf("like response = %+v, want anonymous like counted", out)
	}
}

type siteLikeDeployerStub struct {
	DeployerPort
	site      store.Site
	likeCount int64
}

func (s *siteLikeDeployerStub) GetSite(_ context.Context, code string) (store.Site, error) {
	if s.site.Code != code {
		return store.Site{}, store.ErrNotFound
	}
	return s.site, nil
}

func (s *siteLikeDeployerStub) AddLike(_ context.Context, code, fingerprint string) (int64, error) {
	if s.site.Code != code {
		return 0, store.ErrNotFound
	}
	if fingerprint == "" {
		return 0, nil
	}
	s.likeCount++
	return s.likeCount, nil
}
