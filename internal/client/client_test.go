package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/api"
)

func TestDeploySendsCurrentOriginHeader(t *testing.T) {
	var gotOrigin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/deploy" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotOrigin = r.Header.Get(currentOriginHeader)
		_ = json.NewEncoder(w).Encode(api.DeployResponse{
			Success:       true,
			Code:          "demo-site",
			URL:           serverURL(r) + "/agent/demo-site/",
			DetailURL:     serverURL(r) + "/agent/demo-site/",
			VersionURL:    serverURL(r) + "/agent/demo-site/versions/1/",
			VersionID:     "version-1",
			VersionNumber: 1,
		})
	}))
	defer server.Close()

	c := New(server.URL+"/", "")
	_, err := c.Deploy(context.Background(), api.DeployRequest{
		Filename:    "index.html",
		Description: "demo",
		Content:     "<!doctype html><html><body>demo</body></html>",
	})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if gotOrigin != server.URL {
		t.Fatalf("current origin header = %q, want %q", gotOrigin, server.URL)
	}
}

func TestRawDeploySendsCurrentOriginHeader(t *testing.T) {
	var gotOrigin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/deploy" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotOrigin = r.Header.Get(currentOriginHeader)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"code":    "demo-site",
			"url":     serverURL(r) + "/agent/demo-site/",
		})
	}))
	defer server.Close()

	c := New(server.URL+"/", "")
	_, err := c.RawDeploy(context.Background(), []byte(`{"description":"demo"}`))
	if err != nil {
		t.Fatalf("raw deploy: %v", err)
	}
	if gotOrigin != server.URL {
		t.Fatalf("current origin header = %q, want %q", gotOrigin, server.URL)
	}
}

func TestDeployMultipartSendsFileAndCurrentOriginHeader(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "index.html")
	if err := os.WriteFile(source, []byte("<!doctype html><title>Multipart</title>"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	var gotOrigin string
	var gotContentType string
	var gotFile string
	var gotUploadName string
	var gotFilenameField string
	var gotTemplateSourceCode string
	var gotTemplateSourceVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrigin = r.Header.Get(currentOriginHeader)
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
		gotUploadName = header.Filename
		gotFilenameField = r.FormValue("filename")
		gotTemplateSourceCode = r.FormValue("templateSourceCode")
		gotTemplateSourceVersion = r.FormValue("templateSourceVersion")
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		gotFile = string(data)
		_ = json.NewEncoder(w).Encode(api.DeployResponse{
			Success:       true,
			Code:          "demo-site",
			URL:           serverURL(r) + "/agent/demo-site/",
			DetailURL:     serverURL(r) + "/agent/demo-site/",
			VersionURL:    serverURL(r) + "/agent/demo-site/versions/1/",
			VersionID:     "version-1",
			VersionNumber: 1,
		})
	}))
	defer server.Close()

	c := New(server.URL+"/", "")
	_, err := c.DeployMultipart(context.Background(), MultipartDeployRequest{
		SourcePath:            source,
		UploadName:            "site.zip",
		Filename:              "index.html",
		Description:           "demo",
		Title:                 "Multipart",
		Visibility:            "unlisted",
		Source:                "cli",
		TemplateSourceCode:    "source-demo",
		TemplateSourceVersion: 3,
	})
	if err != nil {
		t.Fatalf("multipart deploy: %v", err)
	}
	if gotOrigin != server.URL {
		t.Fatalf("current origin header = %q, want %q", gotOrigin, server.URL)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Fatalf("content-type = %q, want multipart/form-data", gotContentType)
	}
	if !strings.Contains(gotFile, "Multipart") {
		t.Fatalf("uploaded file = %q", gotFile)
	}
	if gotUploadName != "site.zip" {
		t.Fatalf("upload filename = %q, want site.zip", gotUploadName)
	}
	if gotFilenameField != "index.html" {
		t.Fatalf("filename field = %q, want index.html", gotFilenameField)
	}
	if gotTemplateSourceCode != "source-demo" {
		t.Fatalf("templateSourceCode = %q, want source-demo", gotTemplateSourceCode)
	}
	if gotTemplateSourceVersion != "3" {
		t.Fatalf("templateSourceVersion = %q, want 3", gotTemplateSourceVersion)
	}
}

func TestSetSiteReusePolicySendsAdminPatch(t *testing.T) {
	var gotOrigin string
	var gotAuth string
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/admin/sites/demo/reuse-policy" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotOrigin = r.Header.Get(currentOriginHeader)
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"site": map[string]any{
				"code":                 "demo",
				"reusePolicy":          "deny",
				"sourceDownloadPolicy": "allow",
			},
		})
	}))
	defer server.Close()

	c := New(server.URL+"/", "admin-token")
	resp, err := c.SetSiteReusePolicy(context.Background(), "demo", "deny", "allow")
	if err != nil {
		t.Fatalf("set reuse policy: %v", err)
	}
	if gotOrigin != server.URL {
		t.Fatalf("current origin header = %q, want %q", gotOrigin, server.URL)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotBody["reusePolicy"] != "deny" || gotBody["sourceDownloadPolicy"] != "allow" {
		t.Fatalf("body = %+v", gotBody)
	}
	if resp["success"] != true {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAdminSiteDetailUsesAdminEndpoint(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/sites/demo-site" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"site":    map[string]any{"code": "demo-site"},
			"files":   []map[string]any{{"path": "index.html", "size": 42}},
			"reuse":   map[string]any{"allowDownload": true},
		})
	}))
	defer server.Close()

	c := New(server.URL, "admin-token")
	resp, err := c.AdminSiteDetail(context.Background(), "demo-site")
	if err != nil {
		t.Fatalf("admin site detail: %v", err)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if resp["success"] != true {
		t.Fatalf("response = %+v", resp)
	}
}

func TestListAuditLogsBuildsQuery(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/audit-logs" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"logs":     []map[string]any{},
			"total":    0,
			"page":     2,
			"pageSize": 25,
		})
	}))
	defer server.Close()

	c := New(server.URL, "admin-token")
	resp, err := c.ListAuditLogs(context.Background(), AuditLogQuery{
		ActorType:  "user",
		Action:     "site.pin",
		Result:     "success",
		SiteCode:   "demo",
		ActorID:    "user-1",
		ActorRole:  "admin",
		TargetType: "site",
		TargetID:   "demo",
		Query:      "pinned",
		Since:      "2026-07-06T00:00:00Z",
		Until:      "2026-07-07T00:00:00Z",
		Page:       2,
		PageSize:   25,
	})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	for _, want := range []string{
		"actorType=user",
		"action=site.pin",
		"result=success",
		"siteCode=demo",
		"actorId=user-1",
		"actorRole=admin",
		"targetType=site",
		"targetId=demo",
		"q=pinned",
		"since=2026-07-06T00%3A00%3A00Z",
		"until=2026-07-07T00%3A00%3A00Z",
		"page=2",
		"pageSize=25",
	} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query = %q, want contains %q", gotQuery, want)
		}
	}
	if resp["success"] != true {
		t.Fatalf("response = %+v", resp)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
