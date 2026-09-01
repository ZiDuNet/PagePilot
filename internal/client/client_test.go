package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/api"
)

type immediateMultipartErrorTransport struct {
	mu   sync.Mutex
	body io.ReadCloser
}

func (t *immediateMultipartErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.body = req.Body
	t.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusRequestEntityTooLarge,
		Status:     "413 Request Entity Too Large",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"success":false,"errorCode":"CONTENT_TOO_LARGE","detail":"request rejected early"}`)),
		Request:    req,
	}, nil
}

func (t *immediateMultipartErrorTransport) closeRequestBody() {
	t.mu.Lock()
	body := t.body
	t.mu.Unlock()
	if body != nil {
		_ = body.Close()
	}
}

func TestMultipartEarlyHTTPErrorDoesNotBlockWriter(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "index.html")
	if err := os.WriteFile(source, []byte("<!doctype html><main>early error</main>"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	for _, tc := range []struct {
		name string
		call func(*Client) error
	}{
		{
			name: "deploy",
			call: func(c *Client) error {
				_, err := c.DeployMultipart(context.Background(), MultipartDeployRequest{SourcePath: source})
				return err
			},
		},
		{
			name: "overwrite",
			call: func(c *Client) error {
				_, err := c.OverwriteMultipart(context.Background(), "demo", 1, MultipartOverwriteRequest{SourcePath: source})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &immediateMultipartErrorTransport{}
			c := New("http://example.test", "")
			c.http = &http.Client{Transport: transport}
			done := make(chan error, 1)
			go func() { done <- tc.call(c) }()
			select {
			case err := <-done:
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.Status != http.StatusRequestEntityTooLarge || !apiErr.IsCode(api.CodeContentTooLarge) {
					t.Fatalf("error = %T %v, want CONTENT_TOO_LARGE APIError", err, err)
				}
			case <-time.After(time.Second):
				transport.closeRequestBody()
				t.Fatal("multipart request blocked after early HTTP error")
			}
		})
	}
}

func TestReadinessHelpersSendHeadersAndDecodeResponses(t *testing.T) {
	type requestHeaders struct {
		origin string
		auth   string
	}
	got := map[string]requestHeaders{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got[r.URL.Path] = requestHeaders{
			origin: r.Header.Get(currentOriginHeader),
			auth:   r.Header.Get("Authorization"),
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/health":
			_ = json.NewEncoder(w).Encode(HealthResponse{Success: true, Status: "ok"})
		case "/api/config":
			_ = json.NewEncoder(w).Encode(api.ConfigResponse{
				Success:        true,
				CurrentBaseURL: serverURL(r),
				Mode:           "prod",
				Version:        "test-version",
			})
		case "/api/session":
			_ = json.NewEncoder(w).Encode(api.AnonymousSessionResponse{Success: true, SessionID: "anon-1"})
		case "/api/admin/session":
			_ = json.NewEncoder(w).Encode(api.AdminSessionResponse{
				Success:  true,
				Mode:     "prod",
				TokenID:  "token-1",
				IsAdmin:  true,
				Username: "admin",
			})
		case "/openapi.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"openapi": "3.1.0",
				"info":    map[string]any{"version": "test-version"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := New(server.URL+"/", "admin-token")
	ctx := context.Background()

	health, err := c.Health(ctx)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if !health.Success || health.Status != "ok" {
		t.Fatalf("health = %+v", health)
	}

	config, err := c.Config(ctx)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if !config.Success || config.Mode != "prod" || config.Version != "test-version" {
		t.Fatalf("config = %+v", config)
	}

	session, err := c.AdminSession(ctx)
	if err != nil {
		t.Fatalf("admin session: %v", err)
	}
	if !session.Success || !session.IsAdmin || session.TokenID != "token-1" {
		t.Fatalf("admin session = %+v", session)
	}

	anonymous, err := c.AnonymousSession(ctx)
	if err != nil {
		t.Fatalf("anonymous session: %v", err)
	}
	if !anonymous.Success || anonymous.SessionID != "anon-1" {
		t.Fatalf("anonymous session = %+v", anonymous)
	}

	doc, err := c.OpenAPI(ctx)
	if err != nil {
		t.Fatalf("openapi: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi document = %+v", doc)
	}

	for _, path := range []string{"/api/health", "/api/config", "/api/session", "/api/admin/session", "/openapi.json"} {
		headers, ok := got[path]
		if !ok {
			t.Fatalf("missing request for %s", path)
		}
		if headers.origin != server.URL {
			t.Fatalf("%s current origin = %q, want %q", path, headers.origin, server.URL)
		}
		if headers.auth != "Bearer admin-token" {
			t.Fatalf("%s authorization = %q", path, headers.auth)
		}
	}
}

func TestAdminSessionReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/session" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(api.APIError{
			Success:   false,
			ErrorCode: api.CodeUnauthorized,
			Stage:     "auth",
			Detail:    "token not recognized",
			Hint:      "Use an admin token to sign in.",
		})
	}))
	defer server.Close()

	_, err := New(server.URL, "bad-token").AdminSession(context.Background())
	if err == nil {
		t.Fatal("AdminSession() error = nil, want APIError")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("AdminSession() error = %T %v, want *APIError", err, err)
	}
	if apiErr.Status != http.StatusUnauthorized || !apiErr.IsCode(api.CodeUnauthorized) {
		t.Fatalf("APIError = %+v", apiErr)
	}
	if apiErr.Body == nil || apiErr.Body.Hint != "Use an admin token to sign in." {
		t.Fatalf("APIError body = %+v", apiErr.Body)
	}
}

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

func TestRawDeployPreservesNonJSONHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>upstream unavailable</html>"))
	}))
	defer server.Close()

	out, err := New(server.URL, "").RawDeploy(context.Background(), []byte(`{"description":"demo"}`))
	if out != nil {
		t.Fatalf("raw deploy output = %+v, want nil for non-JSON error", out)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
	if apiErr.Status != http.StatusBadGateway || apiErr.Body == nil || apiErr.Body.ErrorCode != "HTTP_ERROR" {
		t.Fatalf("APIError = %+v", apiErr)
	}
	if !strings.Contains(apiErr.Body.Detail, "upstream unavailable") {
		t.Fatalf("detail = %q", apiErr.Body.Detail)
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

func TestOverwriteMultipartSendsPatchFileAndCurrentOriginHeader(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "index.html")
	if err := os.WriteFile(source, []byte("<!doctype html><title>Overwrite</title>"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	var gotMethod string
	var gotPath string
	var gotOrigin string
	var gotContentType string
	var gotFile string
	var gotUploadName string
	var gotDescription string
	var gotTitle string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotOrigin = r.Header.Get(currentOriginHeader)
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		gotDescription = r.FormValue("description")
		gotTitle = r.FormValue("title")
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
		gotUploadName = header.Filename
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		gotFile = string(data)
		_ = json.NewEncoder(w).Encode(api.DeployResponse{
			Success:       true,
			Code:          "demo-site",
			URL:           serverURL(r) + "/agent/demo-site/",
			DetailURL:     serverURL(r) + "/market/demo-site",
			VersionURL:    serverURL(r) + "/agent/demo-site/versions/7/",
			VersionID:     "version-7",
			VersionNumber: 7,
		})
	}))
	defer server.Close()

	c := New(server.URL+"/", "")
	_, err := c.OverwriteMultipart(context.Background(), "demo-site", 7, MultipartOverwriteRequest{
		SourcePath:  source,
		UploadName:  "overwrite.zip",
		Filename:    "index.html",
		Description: "overwrite with multipart",
		Title:       "Overwrite",
	})
	if err != nil {
		t.Fatalf("multipart overwrite: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/api/deploys/demo-site/versions/7" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotOrigin != server.URL {
		t.Fatalf("current origin header = %q, want %q", gotOrigin, server.URL)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Fatalf("content-type = %q, want multipart/form-data", gotContentType)
	}
	if gotUploadName != "overwrite.zip" {
		t.Fatalf("upload filename = %q, want overwrite.zip", gotUploadName)
	}
	if gotDescription != "overwrite with multipart" || gotTitle != "Overwrite" {
		t.Fatalf("metadata description=%q title=%q", gotDescription, gotTitle)
	}
	if !strings.Contains(gotFile, "Overwrite") {
		t.Fatalf("uploaded file = %q", gotFile)
	}
}

func TestMultipartRequestsPreserveNonJSONErrorDetails(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "index.html")
	if err := os.WriteFile(source, []byte("<!doctype html><main>error test</main>"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><title>Gateway unavailable</title></html>"))
	}))
	defer server.Close()

	tests := []struct {
		name string
		call func(*Client) error
	}{
		{
			name: "deploy",
			call: func(c *Client) error {
				_, err := c.DeployMultipart(context.Background(), MultipartDeployRequest{SourcePath: source, Description: "demo"})
				return err
			},
		},
		{
			name: "overwrite",
			call: func(c *Client) error {
				_, err := c.OverwriteMultipart(context.Background(), "demo", 1, MultipartOverwriteRequest{SourcePath: source, Description: "demo"})
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(New(server.URL, ""))
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %T %v, want *APIError", err, err)
			}
			if apiErr.Status != http.StatusBadGateway || apiErr.Body == nil || apiErr.Body.ErrorCode != "HTTP_ERROR" {
				t.Fatalf("APIError = %+v", apiErr)
			}
			if !strings.Contains(apiErr.Body.Detail, "Gateway unavailable") {
				t.Fatalf("detail = %q", apiErr.Body.Detail)
			}
		})
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
