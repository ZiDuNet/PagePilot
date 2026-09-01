package deploy

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
)

func TestAnonymousDeployForcesUnlistedEvenWhenPublicRequested(t *testing.T) {
	tmp := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(tmp, "hostctl.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}()

	cfg := config.Default()
	cfg.HostedDir = filepath.Join(tmp, "hosted")
	cfg.MaxSingleFileBytes = 1 << 20
	cfg.MaxSiteTotalBytes = 2 << 20
	cfg.MaxFilesPerSite = 20
	cfg.CooldownSeconds = 0

	d := New(cfg, st)
	resp, apiErr := d.Deploy(context.Background(), api.DeployRequest{
		Filename:    "index.html",
		Title:       "Anonymous public request",
		Description: "Anonymous deploys must stay out of the marketplace unless claimed by a user.",
		Content:     "<!doctype html><html><body><h1>Hello PagePilot</h1></body></html>",
		Visibility:  "public",
	}, "anon:test-session", "127.0.0.1")
	if apiErr != nil {
		t.Fatalf("Deploy returned API error: %s", apiErr.Detail)
	}

	site, err := st.GetSite(context.Background(), resp.Code)
	if err != nil {
		t.Fatalf("get site: %v", err)
	}
	if site.OwnerTokenID != "anon:test-session" {
		t.Fatalf("OwnerTokenID = %q; want anon:test-session", site.OwnerTokenID)
	}
	if site.Visibility != "unlisted" {
		t.Fatalf("Visibility = %q; want unlisted", site.Visibility)
	}
	version, err := st.GetVersion(context.Background(), resp.Code, 1)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if version.StorageBackend != "local" || version.StoragePrefix != filepath.ToSlash(filepath.Join(resp.Code, "versions", "1")) {
		t.Fatalf("version storage = %s %q; want local code/versions/1",
			version.StorageBackend, version.StoragePrefix)
	}

	_, total, err := st.ListMarketplaceDeploys(context.Background(), "", "", "likes_desc", "", "", "", "", 1, 10)
	if err != nil {
		t.Fatalf("list marketplace: %v", err)
	}
	if total != 0 {
		t.Fatalf("marketplace total = %d; want 0 for anonymous unlisted deploy", total)
	}
}

func TestDeployPersistsBundleMetadataForContentModes(t *testing.T) {
	ctx := context.Background()
	d, st := newDeployTestHarness(t)

	cases := []struct {
		name         string
		code         string
		req          api.DeployRequest
		wantKind     string
		wantRoot     string
		wantEntry    string
		wantSecurity string
	}{
		{
			name: "single html",
			code: "single-html",
			req: api.DeployRequest{
				Filename: "index.html",
				Content:  "<!doctype html><html><body><h1>Single HTML</h1></body></html>",
			},
			wantKind:     "single_html",
			wantEntry:    "index.html",
			wantSecurity: "standard",
		},
		{
			name: "single html without filename",
			code: "single-html-no-filename",
			req: api.DeployRequest{
				Content: "<!doctype html><html><body><h1>Single HTML</h1></body></html>",
			},
			wantKind:     "single_html",
			wantEntry:    "index.html",
			wantSecurity: "standard",
		},
		{
			name: "single markdown",
			code: "single-md",
			req: api.DeployRequest{
				Filename: "README.md",
				Content:  "# 单文件 Markdown\n\n这是一个文档入口。",
			},
			wantKind:     "markdown",
			wantEntry:    "README.md",
			wantSecurity: "strict",
		},
		{
			name: "single markdown without filename",
			code: "single-md-no-filename",
			req: api.DeployRequest{
				Content: "# 单文件 Markdown\n\n这是一个文档入口。",
			},
			wantKind:     "markdown",
			wantEntry:    "README.md",
			wantSecurity: "strict",
		},
		{
			name: "single markdown inferred from default html filename",
			code: "single-md-inferred",
			req: api.DeployRequest{
				Filename: "index.html",
				Content:  "# Inferred Markdown\n\n- item one\n- item two",
			},
			wantKind:     "markdown",
			wantEntry:    "index.md",
			wantSecurity: "strict",
		},
		{
			name: "single markdown appends missing extension",
			code: "single-md-no-ext",
			req: api.DeployRequest{
				Filename: "demo",
				Content:  "# Demo Markdown\n\nThis document has no filename suffix.",
			},
			wantKind:     "markdown",
			wantEntry:    "demo.md",
			wantSecurity: "strict",
		},
		{
			name: "single html inferred from markdown filename",
			code: "single-html-inferred",
			req: api.DeployRequest{
				Filename: "README.md",
				Content:  "<!doctype html><html><body><main>HTML wins by content</main></body></html>",
			},
			wantKind:     "single_html",
			wantEntry:    "README.html",
			wantSecurity: "standard",
		},
		{
			name: "single html appends missing extension",
			code: "single-html-no-ext",
			req: api.DeployRequest{
				Filename: "demo",
				Content:  "<!doctype html><html><body><main>HTML document without suffix</main></body></html>",
			},
			wantKind:     "single_html",
			wantEntry:    "demo.html",
			wantSecurity: "standard",
		},
		{
			name: "multi file static site",
			code: "static-site",
			req: api.DeployRequest{
				Files: []api.DeployFile{
					{Path: "index.html", Content: "<!doctype html><html><body><main>Static</main></body></html>"},
					{Path: "assets/app.css", Content: "body{color:#0f172a}"},
				},
			},
			wantKind:     "static_site",
			wantEntry:    "index.html",
			wantSecurity: "standard",
		},
		{
			name: "multi file site keeps empty asset",
			code: "empty-asset",
			req: api.DeployRequest{
				Files: []api.DeployFile{
					{Path: "index.html", Content: "<!doctype html><html><body><main>Static</main></body></html>"},
					{Path: "assets/empty.css"},
				},
			},
			wantKind:     "static_site",
			wantEntry:    "index.html",
			wantSecurity: "standard",
		},
		{
			name: "zip site",
			code: "zip-site",
			req: api.DeployRequest{
				Files: []api.DeployFile{{
					Path: "site.zip",
					ContentBase64: base64.StdEncoding.EncodeToString(makeTestZip(t, map[string]string{
						"project/dist/index.html":     "<!doctype html><html><body><main>Zip</main></body></html>",
						"project/dist/assets/app.css": "body{color:#0f172a}",
					})),
				}},
			},
			wantKind:     "zip_site",
			wantRoot:     "project/dist",
			wantEntry:    "index.html",
			wantSecurity: "standard",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			req.EnableCustomCode = true
			req.CustomCode = tc.code
			req.Title = "Bundle 元数据"
			req.Description = "发布后应该记录可产品化展示的 Bundle 元数据。"
			_, apiErr := d.Deploy(ctx, req, "user:owner", "127.0.0.1")
			if apiErr != nil {
				t.Fatalf("Deploy returned API error: %s", apiErr.Detail)
			}

			meta, err := st.GetVersionBundle(ctx, tc.code, 1)
			if err != nil {
				t.Fatalf("GetVersionBundle returned error: %v", err)
			}
			if meta.Kind != tc.wantKind || meta.Root != tc.wantRoot ||
				meta.MainEntry != tc.wantEntry || meta.SecurityMode != tc.wantSecurity {
				t.Fatalf("bundle metadata = %+v; want kind=%s root=%q entry=%s security=%s",
					meta, tc.wantKind, tc.wantRoot, tc.wantEntry, tc.wantSecurity)
			}
			if meta.TreeJSON == "" || meta.TreeJSON == "[]" {
				t.Fatalf("TreeJSON = %q; want file tree", meta.TreeJSON)
			}
		})
	}
}

func TestOverwriteVersionRefreshesBundleMetadata(t *testing.T) {
	ctx := context.Background()
	d, st := newDeployTestHarness(t)

	_, apiErr := d.Deploy(ctx, api.DeployRequest{
		EnableCustomCode: true,
		CustomCode:       "bundle-refresh",
		Filename:         "index.html",
		Title:            "Bundle 初始版本",
		Description:      "初始是单 HTML，覆盖后应该变成多文件站点。",
		Content:          "<!doctype html><html><body><main>Initial</main></body></html>",
	}, "user:owner", "127.0.0.1")
	if apiErr != nil {
		t.Fatalf("Deploy returned API error: %s", apiErr.Detail)
	}

	_, apiErr = d.OverwriteVersion(ctx, "bundle-refresh", 1, api.OverwriteRequest{
		Filename:    "index.html",
		Title:       "Bundle 覆盖版本",
		Description: "覆盖为多文件站点后应该刷新 Bundle 元数据。",
		Files: []api.DeployFile{
			{Path: "index.html", Content: "<!doctype html><html><body><main>Updated</main></body></html>"},
			{Path: "assets/app.css", Content: "body{color:#0f172a}"},
		},
	})
	if apiErr != nil {
		t.Fatalf("OverwriteVersion returned API error: %s", apiErr.Detail)
	}

	meta, err := st.GetVersionBundle(ctx, "bundle-refresh", 1)
	if err != nil {
		t.Fatalf("GetVersionBundle returned error: %v", err)
	}
	if meta.Kind != "static_site" || meta.MainEntry != "index.html" ||
		!strings.Contains(meta.TreeJSON, "assets/app.css") {
		t.Fatalf("bundle metadata = %+v; want refreshed static site metadata", meta)
	}
}

func newDeployTestHarness(t *testing.T) (*Deployer, *store.SQLiteStore) {
	t.Helper()
	tmp := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(tmp, "hostctl.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	cfg := config.Default()
	cfg.HostedDir = filepath.Join(tmp, "hosted")
	cfg.MaxSingleFileBytes = 1 << 20
	cfg.MaxSiteTotalBytes = 2 << 20
	cfg.MaxFilesPerSite = 20
	cfg.CooldownSeconds = 0

	return New(cfg, st), st
}

func TestReadAppFileLimitedRejectsOversizedObject(t *testing.T) {
	d, _ := newDeployTestHarness(t)
	resp, apiErr := d.Deploy(context.Background(), api.DeployRequest{
		Filename:    "index.html",
		Description: "limited file read regression",
		Content:     "<!doctype html><html><body><main>" + strings.Repeat("x", 1024) + "</main></body></html>",
	}, "anon:limited-read", "127.0.0.1")
	if apiErr != nil {
		t.Fatalf("deploy returned API error: %s", apiErr.Detail)
	}
	if _, _, apiErr := d.readAppFileLimited(context.Background(), resp.Code, nil, "index.html", 32); apiErr == nil || apiErr.ErrorCode != api.CodeContentTooLarge {
		t.Fatalf("limited read error = %+v; want CONTENT_TOO_LARGE", apiErr)
	}
	body, _, apiErr := d.ReadAppFile(context.Background(), resp.Code, nil, "index.html")
	if apiErr != nil {
		t.Fatalf("unlimited read returned API error: %s", apiErr.Detail)
	}
	if len(body) <= 32 {
		t.Fatalf("unlimited read returned %d bytes; want full file", len(body))
	}
}

func TestAdminDeployAsCanAppendAnotherUsersSite(t *testing.T) {
	ctx := context.Background()
	d, st := newDeployTestHarness(t)
	first, apiErr := d.Deploy(ctx, api.DeployRequest{
		EnableCustomCode: true,
		CustomCode:       "owned-site",
		CreateVersion:    false,
		Filename:         "index.html",
		Description:      "初始站点内容用于验证管理员追加版本。",
		Content:          "<!doctype html><html><body><main>v1</main></body></html>",
	}, "user:owner", "127.0.0.1")
	if apiErr != nil {
		t.Fatalf("initial deploy returned API error: %s", apiErr.Detail)
	}
	updated, apiErr := d.DeployAs(ctx, api.DeployRequest{
		EnableCustomCode: true,
		CustomCode:       first.Code,
		CreateVersion:    true,
		Filename:         "index.html",
		Description:      "管理员追加版本内容。",
		Content:          "<!doctype html><html><body><main>v2</main></body></html>",
	}, "user:admin", "127.0.0.1", true)
	if apiErr != nil {
		t.Fatalf("admin append returned API error: %s", apiErr.Detail)
	}
	if updated.VersionNumber != 2 || updated.Code != first.Code {
		t.Fatalf("admin append response = %+v; want code %q version 2", updated, first.Code)
	}
	if site, err := st.GetSite(ctx, first.Code); err != nil || site.CurrentVersion == nil || *site.CurrentVersion != 2 {
		t.Fatalf("site current version = %+v, err=%v; want version 2", site.CurrentVersion, err)
	}
}

func TestConcurrentCreateVersionWithDisabledCooldownPublishesDistinctFiles(t *testing.T) {
	ctx := context.Background()
	d, st := newDeployTestHarness(t)
	const code = "concurrent-site"

	_, apiErr := d.Deploy(ctx, api.DeployRequest{
		EnableCustomCode: true,
		CustomCode:       code,
		Filename:         "index.html",
		Title:            "Initial version",
		Description:      "Initial content before concurrent version publishing.",
		Content:          "<!doctype html><html><body><main>initial</main></body></html>",
	}, "user:owner", "127.0.0.1")
	if apiErr != nil {
		t.Fatalf("initial deploy returned API error: %s", apiErr.Detail)
	}

	type result struct {
		marker string
		resp   *api.DeployResponse
		err    *api.APIError
	}
	markers := []string{"concurrent-a", "concurrent-b"}
	start := make(chan struct{})
	results := make(chan result, len(markers))
	var wg sync.WaitGroup
	for _, marker := range markers {
		marker := marker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := d.Deploy(ctx, api.DeployRequest{
				EnableCustomCode: true,
				CustomCode:       code,
				CreateVersion:    true,
				Filename:         "index.html",
				Title:            marker,
				Description:      "Concurrent publishing must keep every version directory intact.",
				Content:          "<!doctype html><html><body><main>" + marker + "</main></body></html>",
			}, "user:owner", "127.0.0.1")
			results <- result{marker: marker, resp: resp, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	versions := map[int64]string{}
	for got := range results {
		if got.err != nil {
			t.Fatalf("concurrent deploy %s returned API error: %s", got.marker, got.err.Detail)
		}
		if got.resp == nil {
			t.Fatalf("concurrent deploy %s returned no response", got.marker)
		}
		version := int64(got.resp.VersionNumber)
		if version != 2 && version != 3 {
			t.Fatalf("concurrent deploy %s version = %d; want 2 or 3", got.marker, version)
		}
		if _, duplicate := versions[version]; duplicate {
			t.Fatalf("two concurrent publishes returned version %d", version)
		}
		versions[version] = got.marker
	}
	if len(versions) != len(markers) {
		t.Fatalf("published versions = %#v; want two distinct versions", versions)
	}

	stored, err := st.ListVersions(ctx, code)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(stored) != 3 {
		t.Fatalf("version count = %d; want 3", len(stored))
	}
	for version, marker := range versions {
		body, _, readErr := d.ReadAppFile(ctx, code, &version, "index.html")
		if readErr != nil {
			t.Fatalf("read version %d: %s", version, readErr.Detail)
		}
		if !strings.Contains(string(body), marker) {
			t.Fatalf("version %d body = %q; want marker %q", version, body, marker)
		}
	}
}

func TestDeployRecordsTemplateSourceAndIncrementsReuseCount(t *testing.T) {
	tmp := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(tmp, "hostctl.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}()

	cfg := config.Default()
	cfg.HostedDir = filepath.Join(tmp, "hosted")
	cfg.MaxSingleFileBytes = 1 << 20
	cfg.MaxSiteTotalBytes = 2 << 20
	cfg.MaxFilesPerSite = 20
	cfg.CooldownSeconds = 0

	ctx := context.Background()
	d := New(cfg, st)
	sourceResp, apiErr := d.Deploy(ctx, api.DeployRequest{
		EnableCustomCode: true,
		CustomCode:       "source-demo",
		Filename:         "index.html",
		Title:            "模板来源",
		Description:      "作为模板被复用的公开作品。",
		Content:          "<!doctype html><html><body><h1>Source</h1></body></html>",
		Visibility:       "public",
	}, "user:owner", "127.0.0.1")
	if apiErr != nil {
		t.Fatalf("source deploy returned API error: %s", apiErr.Detail)
	}

	targetResp, apiErr := d.Deploy(ctx, api.DeployRequest{
		EnableCustomCode:      true,
		CustomCode:            "target-demo",
		Filename:              "index.html",
		Title:                 "复用作品",
		Description:           "基于模板来源二次创作的新作品。",
		Content:               "<!doctype html><html><body><h1>Target</h1></body></html>",
		TemplateSourceCode:    sourceResp.Code,
		TemplateSourceVersion: int64(sourceResp.VersionNumber),
	}, "user:owner", "127.0.0.1")
	if apiErr != nil {
		t.Fatalf("target deploy returned API error: %s", apiErr.Detail)
	}
	if targetResp.TemplateSourceCode != "source-demo" || targetResp.TemplateSourceVersion != 1 {
		t.Fatalf("target response template source = %s v%d, want source-demo v1",
			targetResp.TemplateSourceCode, targetResp.TemplateSourceVersion)
	}

	sourceSite, err := st.GetSite(ctx, "source-demo")
	if err != nil {
		t.Fatalf("get source site: %v", err)
	}
	if sourceSite.ReuseCount != 1 {
		t.Fatalf("source reuse count = %d, want 1", sourceSite.ReuseCount)
	}

	targetSite, err := st.GetSite(ctx, "target-demo")
	if err != nil {
		t.Fatalf("get target site: %v", err)
	}
	if targetSite.TemplateSourceCode != "source-demo" || targetSite.TemplateSourceVersion == nil || *targetSite.TemplateSourceVersion != 1 {
		t.Fatalf("target site template source = %s %v, want source-demo v1",
			targetSite.TemplateSourceCode, targetSite.TemplateSourceVersion)
	}

	version, err := st.GetVersion(ctx, "target-demo", 1)
	if err != nil {
		t.Fatalf("get target version: %v", err)
	}
	if version.TemplateSourceCode != "source-demo" || version.TemplateSourceVersion == nil || *version.TemplateSourceVersion != 1 {
		t.Fatalf("target version template source = %s %v, want source-demo v1",
			version.TemplateSourceCode, version.TemplateSourceVersion)
	}
}

func TestDeployRejectsNegativeTemplateSourceVersion(t *testing.T) {
	d, st := newDeployTestHarness(t)
	_, apiErr := d.Deploy(context.Background(), api.DeployRequest{
		EnableCustomCode:      true,
		CustomCode:            "negative-source-version",
		Filename:              "index.html",
		Description:           "invalid template source version",
		Content:               "<!doctype html><html><body><main>Valid page</main></body></html>",
		TemplateSourceCode:    "missing-source",
		TemplateSourceVersion: -1,
	}, "user:test", "127.0.0.1")
	if apiErr == nil {
		t.Fatal("Deploy accepted negative templateSourceVersion")
	}
	if apiErr.ErrorCode != api.CodeInvalidInput || !strings.Contains(apiErr.Detail, "templateSourceVersion") {
		t.Fatalf("Deploy error = %#v; want invalid templateSourceVersion", apiErr)
	}
	if exists, err := st.SiteExists(context.Background(), "negative-source-version"); err != nil {
		t.Fatalf("check site existence: %v", err)
	} else if exists {
		t.Fatal("Deploy created site despite invalid templateSourceVersion")
	}
}

func TestTemplateSourceReuseAllowedMatchesSitePolicy(t *testing.T) {
	cases := []struct {
		name  string
		site  store.Site
		owner string
		admin bool
		want  bool
	}{
		{
			name:  "public defaults",
			site:  store.Site{Status: "active", Visibility: "public", ReusePolicy: "auto", SourceDownloadPolicy: "auto"},
			owner: "user:other",
			want:  true,
		},
		{
			name:  "unlisted explicit allow",
			site:  store.Site{Status: "active", Visibility: "unlisted", ReusePolicy: "auto", SourceDownloadPolicy: "allow"},
			owner: "user:other",
			want:  true,
		},
		{
			name:  "source deny",
			site:  store.Site{Status: "active", Visibility: "public", ReusePolicy: "allow", SourceDownloadPolicy: "deny"},
			owner: "user:other",
			want:  false,
		},
		{
			name:  "owner bypass",
			site:  store.Site{OwnerTokenID: "user:owner", AccessPasswordHash: "hash", Status: "inactive", Visibility: "unlisted"},
			owner: "user:owner",
			want:  true,
		},
		{
			name:  "admin bypass",
			site:  store.Site{AccessPasswordHash: "hash", Status: "inactive", Visibility: "unlisted"},
			owner: "user:admin",
			admin: true,
			want:  true,
		},
		{
			name:  "anonymous denied",
			site:  store.Site{OwnerTokenID: "anon:s1", Status: "active", Visibility: "public"},
			owner: "anon:s1",
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := templateSourceReuseAllowed(tc.site, tc.owner, tc.admin); got != tc.want {
				t.Fatalf("templateSourceReuseAllowed() = %v, want %v", got, tc.want)
			}
		})
	}
}
