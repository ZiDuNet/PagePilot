package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

func TestMarketplaceDetailIncludesBundleFilesAndReuse(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	user, err := authSvc.CreateUser(t.Context(), "reader", "password123", false, 20)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := authSvc.Generate(t.Context(), "reader-token", false, user.ID, nil)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	currentVersion := int64(3)
	srv.deployer = &siteDetailDeployerStub{
		market: store.MarketplaceDeploy{
			ID:              "public-demo",
			Code:            "demo",
			CurrentVersion:  &currentVersion,
			Title:           "演示文档",
			Description:     "Markdown bundle",
			Filename:        "README.md",
			FileSize:        2048,
			VersionCount:    2,
			Status:          "active",
			Visibility:      "public",
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
			AccessProtected: false,
		},
		content: bundleTestContent("demo", currentVersion),
		bundle: store.VersionBundle{
			SiteCode:      "demo",
			VersionNumber: currentVersion,
			Kind:          "markdown",
			Root:          "docs",
			MainEntry:     "README.md",
			TreeJSON:      `[{"path":"README.md","size":1024,"isBinary":false}]`,
			SecurityMode:  "strict",
			CreatedAt:     time.Now().UTC(),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/deploys/demo", nil)
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var out struct {
		Code   string `json:"code"`
		Bundle struct {
			Kind         string          `json:"kind"`
			MainEntry    string          `json:"mainEntry"`
			SecurityMode string          `json:"securityMode"`
			FileCount    int             `json:"fileCount"`
			TotalSize    int64           `json:"totalSize"`
			Tree         json.RawMessage `json:"tree"`
			EntryNote    string          `json:"entryNote"`
		} `json:"bundle"`
		Files []ContentFile `json:"files"`
		Reuse struct {
			CLI         string         `json:"cli"`
			AgentPrompt string         `json:"agentPrompt"`
			MCP         map[string]any `json:"mcp"`
		} `json:"reuse"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != "demo" || out.Bundle.Kind != "markdown" || out.Bundle.MainEntry != "README.md" ||
		out.Bundle.SecurityMode != "strict" || out.Bundle.FileCount != 2 || out.Bundle.TotalSize != 2048 ||
		!json.Valid(out.Bundle.Tree) || !strings.Contains(out.Bundle.EntryNote, "Markdown") {
		t.Fatalf("bundle detail = %+v tree=%s; want markdown bundle metadata", out.Bundle, string(out.Bundle.Tree))
	}
	if len(out.Files) != 2 || out.Files[0].Path != "README.md" || out.Files[1].Path != "assets/app.css" {
		t.Fatalf("files = %+v; want detail file list", out.Files)
	}
	if !strings.Contains(out.Reuse.CLI, "--template-source-code demo") ||
		!strings.Contains(out.Reuse.AgentPrompt, "演示文档") ||
		out.Reuse.MCP["template_source_code"] != "demo" ||
		out.Reuse.MCP["template_source_version"] == nil {
		t.Fatalf("reuse = %+v; want cli, prompt, and mcp params", out.Reuse)
	}
}

func TestMarketplaceDetailRequiresLoginForSourceReuse(t *testing.T) {
	srv, _, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	currentVersion := int64(1)
	srv.deployer = &siteDetailDeployerStub{
		market: store.MarketplaceDeploy{
			ID:                   "public-demo",
			Code:                 "demo",
			CurrentVersion:       &currentVersion,
			Title:                "公开作品",
			Filename:             "index.html",
			FileSize:             1024,
			VersionCount:         1,
			Status:               "active",
			Visibility:           "public",
			ReusePolicy:          "auto",
			SourceDownloadPolicy: "auto",
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		},
		content: bundleTestContent("demo", currentVersion),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/deploys/demo", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var out struct {
		Reuse struct {
			AllowReuse    bool   `json:"allowReuse"`
			AllowDownload bool   `json:"allowDownload"`
			DownloadURL   string `json:"downloadUrl"`
			CLI           string `json:"cli"`
			PolicyNote    string `json:"policyNote"`
		} `json:"reuse"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Reuse.AllowReuse || out.Reuse.AllowDownload || out.Reuse.DownloadURL != "" || out.Reuse.CLI != "" {
		t.Fatalf("reuse = %+v; anonymous viewer must not get source download/reuse enabled", out.Reuse)
	}
	if !strings.Contains(out.Reuse.PolicyNote, "需要先登录") {
		t.Fatalf("policyNote = %q; want login restriction explanation", out.Reuse.PolicyNote)
	}
}

func TestMarketplaceProtectedDetailMarksSourceDownloadRestricted(t *testing.T) {
	srv, _, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	currentVersion := int64(1)
	srv.deployer = &siteDetailDeployerStub{
		market: store.MarketplaceDeploy{
			ID:              "public-secret",
			Code:            "secret",
			CurrentVersion:  &currentVersion,
			Title:           "加密作品",
			Description:     "需要密码访问，但源码不公开",
			Filename:        "index.html",
			FileSize:        1024,
			VersionCount:    1,
			Status:          "active",
			Visibility:      "public",
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
			AccessProtected: true,
		},
		site: store.Site{
			Code:               "secret",
			PublicID:           "public-secret",
			OwnerTokenID:       "user:owner",
			CurrentVersion:     &currentVersion,
			Status:             "active",
			Visibility:         "public",
			AccessPasswordHash: "hash",
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		},
		content: bundleTestContent("secret", currentVersion),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/deploys/secret", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var out struct {
		AccessProtected bool `json:"accessProtected"`
		Reuse           struct {
			AllowReuse    bool   `json:"allowReuse"`
			AllowDownload bool   `json:"allowDownload"`
			PolicyNote    string `json:"policyNote"`
			DownloadURL   string `json:"downloadUrl"`
		} `json:"reuse"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !out.AccessProtected {
		t.Fatal("accessProtected = false, want true")
	}
	if out.Reuse.AllowReuse || out.Reuse.AllowDownload || out.Reuse.DownloadURL != "" {
		t.Fatalf("reuse = %+v; protected public viewer must not get source download/reuse enabled", out.Reuse)
	}
	if out.Reuse.PolicyNote == "" {
		t.Fatalf("policyNote is empty; want source download restriction explanation")
	}
}

func TestMarketplaceProtectedDetailBlocksExplicitReusePolicyAllow(t *testing.T) {
	srv, _, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	currentVersion := int64(1)
	srv.deployer = &siteDetailDeployerStub{
		market: store.MarketplaceDeploy{
			ID:                   "public-secret",
			Code:                 "secret",
			CurrentVersion:       &currentVersion,
			Title:                "加密作品",
			Description:          "需要密码访问，即使策略允许也不公开源码",
			Filename:             "index.html",
			FileSize:             1024,
			VersionCount:         1,
			Status:               "active",
			Visibility:           "public",
			ReusePolicy:          "allow",
			SourceDownloadPolicy: "allow",
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
			AccessProtected:      true,
		},
		site: store.Site{
			Code:                 "secret",
			PublicID:             "public-secret",
			OwnerTokenID:         "user:owner",
			CurrentVersion:       &currentVersion,
			Status:               "active",
			Visibility:           "public",
			ReusePolicy:          "allow",
			SourceDownloadPolicy: "allow",
			AccessPasswordHash:   "hash",
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		},
		content: bundleTestContent("secret", currentVersion),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/deploys/secret", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var out struct {
		Reuse struct {
			AllowReuse    bool   `json:"allowReuse"`
			AllowDownload bool   `json:"allowDownload"`
			DownloadURL   string `json:"downloadUrl"`
			PolicyNote    string `json:"policyNote"`
		} `json:"reuse"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Reuse.AllowReuse || out.Reuse.AllowDownload || out.Reuse.DownloadURL != "" {
		t.Fatalf("reuse = %+v; encrypted site must not expose source download/reuse", out.Reuse)
	}
	if !strings.Contains(out.Reuse.PolicyNote, "加密作品") {
		t.Fatalf("policyNote = %q; want encrypted source restriction explanation", out.Reuse.PolicyNote)
	}
}

func TestReusePolicyBlocksEncryptedSiteEvenForManager(t *testing.T) {
	allowDownload, allowReuse, note := reusePolicy(detailReusePolicy{
		CanManage:            true,
		AccessProtected:      true,
		Visibility:           "public",
		Status:               "active",
		ReusePolicy:          "allow",
		SourceDownloadPolicy: "allow",
	})

	if allowDownload || allowReuse {
		t.Fatalf("allowDownload=%v allowReuse=%v; encrypted site must block source delivery", allowDownload, allowReuse)
	}
	if !strings.Contains(note, "加密作品不提供源码下载") {
		t.Fatalf("note = %q; want encrypted source delivery restriction", note)
	}
}

func TestAdminSiteDetailIncludesVersionsBundleAndFiles(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	admin, err := authSvc.CreateUser(t.Context(), "admin", "password123", true, -1)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	token, err := authSvc.Generate(t.Context(), "admin-token", true, admin.ID, nil)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	currentVersion := int64(3)
	srv.deployer = &siteDetailDeployerStub{
		site: store.Site{
			Code:               "demo",
			PublicID:           "public-demo",
			OwnerTokenID:       "user:owner",
			CurrentVersion:     &currentVersion,
			Status:             "active",
			Visibility:         "public",
			AccessPasswordHash: "hash",
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
			Source:             "api",
		},
		content: bundleTestContent("demo", currentVersion),
		bundle: store.VersionBundle{
			SiteCode:      "demo",
			VersionNumber: currentVersion,
			Kind:          "html",
			MainEntry:     "index.html",
			TreeJSON:      `[{"path":"index.html","size":1024,"isBinary":false}]`,
			SecurityMode:  "standard",
			CreatedAt:     time.Now().UTC(),
		},
		versions: &ListVersionsResponse{
			Success:        true,
			Code:           "demo",
			CurrentVersion: &currentVersion,
			Versions: []VersionItem{{
				VersionNumber: currentVersion,
				ID:            "v3",
				Title:         "演示文档",
				Size:          2048,
				FileCount:     2,
				IsCurrent:     true,
				Status:        "active",
				CreatedAt:     time.Now().UTC(),
			}},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/sites/demo", nil)
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var out struct {
		Success bool `json:"success"`
		Site    struct {
			Code            string `json:"code"`
			AccessProtected bool   `json:"accessProtected"`
		} `json:"site"`
		Versions []VersionItem `json:"versions"`
		Bundle   struct {
			Kind         string `json:"kind"`
			MainEntry    string `json:"mainEntry"`
			SecurityMode string `json:"securityMode"`
		} `json:"bundle"`
		Files []ContentFile `json:"files"`
		Reuse struct {
			AllowReuse    bool           `json:"allowReuse"`
			AllowDownload bool           `json:"allowDownload"`
			PolicyNote    string         `json:"policyNote"`
			MCP           map[string]any `json:"mcp"`
		} `json:"reuse"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !out.Success || out.Site.Code != "demo" || !out.Site.AccessProtected {
		t.Fatalf("site detail = %+v; want protected demo", out.Site)
	}
	if len(out.Versions) != 1 || out.Versions[0].VersionNumber != currentVersion {
		t.Fatalf("versions = %+v; want current version", out.Versions)
	}
	if out.Bundle.Kind != "static_site" || out.Bundle.MainEntry != "index.html" || out.Bundle.SecurityMode != "standard" {
		t.Fatalf("bundle = %+v; want static site bundle", out.Bundle)
	}
	if len(out.Files) != 2 {
		t.Fatalf("files = %+v; want detail files", out.Files)
	}
	if out.Reuse.AllowReuse || out.Reuse.AllowDownload || out.Reuse.MCP["template_source_code"] != nil ||
		!strings.Contains(out.Reuse.PolicyNote, "加密作品不提供源码下载") {
		t.Fatalf("reuse = %+v; encrypted admin detail must block source delivery", out.Reuse)
	}
}

func bundleTestContent(code string, version int64) *GetContentResponse {
	return &GetContentResponse{
		Success:     true,
		Code:        code,
		Version:     version,
		Title:       "演示文档",
		Description: "bundle detail",
		MainEntry:   "README.md",
		TotalSize:   2048,
		Files: []ContentFile{
			{Path: "README.md", Size: 1024, Sha256: "sha-readme"},
			{Path: "assets/app.css", Size: 1024, Sha256: "sha-css"},
		},
		CreatedAt: time.Now().UTC(),
	}
}

type siteDetailDeployerStub struct {
	DeployerPort
	site     store.Site
	market   store.MarketplaceDeploy
	content  *GetContentResponse
	bundle   store.VersionBundle
	versions *ListVersionsResponse
}

func (s *siteDetailDeployerStub) GetMarketplaceDeploy(_ context.Context, code string) (store.MarketplaceDeploy, error) {
	if s.market.Code != code {
		return store.MarketplaceDeploy{}, store.ErrNotFound
	}
	return s.market, nil
}

func (s *siteDetailDeployerStub) GetMarketplaceDeployByUUID(_ context.Context, publicID string) (store.MarketplaceDeploy, error) {
	if s.market.ID != publicID {
		return store.MarketplaceDeploy{}, store.ErrNotFound
	}
	return s.market, nil
}

func (s *siteDetailDeployerStub) GetSite(_ context.Context, code string) (store.Site, error) {
	if s.site.Code != code {
		return store.Site{}, store.ErrNotFound
	}
	return s.site, nil
}

func (s *siteDetailDeployerStub) GetContent(_ context.Context, code string, versionPtr *int64) (*GetContentResponse, *APIError) {
	if s.content == nil || s.content.Code != code {
		return nil, NewError(CodeNotFound, "content", "content not found")
	}
	if versionPtr != nil && *versionPtr != s.content.Version {
		return nil, NewError(CodeNotFound, "content", "version not found")
	}
	return s.content, nil
}

func (s *siteDetailDeployerStub) GetVersionBundle(_ context.Context, code string, version int64) (store.VersionBundle, error) {
	if s.bundle.SiteCode != code || s.bundle.VersionNumber != version {
		return store.VersionBundle{}, store.ErrNotFound
	}
	return s.bundle, nil
}

func (s *siteDetailDeployerStub) ListVersions(_ context.Context, code string) (*ListVersionsResponse, *APIError) {
	if s.versions == nil || s.versions.Code != code {
		return nil, NewError(CodeNotFound, "versions", "versions not found")
	}
	return s.versions, nil
}
