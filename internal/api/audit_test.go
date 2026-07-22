package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

func TestAdminAuditLogsCanBeListedAndFiltered(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	createdAt := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	since := "2026-07-06T00:00:00Z"
	until := "2026-07-07T00:00:00Z"
	stub := &auditLogDeployerStub{
		logs: []store.AuditLog{{
			ID:         7,
			ActorType:  "user",
			ActorID:    "user-1",
			ActorRole:  "admin",
			Action:     "site.pin",
			Result:     "success",
			SiteCode:   "demo",
			TargetType: "site",
			TargetID:   "demo",
			IP:         "127.0.0.1",
			UserAgent:  "agent-test",
			DetailJSON: `{"pinned":true}`,
			CreatedAt:  createdAt,
		}},
	}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodGet,
		"/api/admin/audit-logs?actorType=user&action=site.pin&result=success&siteCode=demo&actorId=user-1&actorRole=admin&targetType=site&targetId=demo&q=pinned&since="+since+"&until="+until+"&page=2&pageSize=25",
		nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if stub.filter.ActorType != "user" || stub.filter.Action != "site.pin" || stub.filter.Result != "success" ||
		stub.filter.SiteCode != "demo" || stub.filter.Query != "pinned" ||
		stub.filter.ActorID != "user-1" || stub.filter.ActorRole != "admin" ||
		stub.filter.TargetType != "site" || stub.filter.TargetID != "demo" ||
		stub.filter.Limit != 25 || stub.filter.Offset != 25 {
		t.Fatalf("filter = %+v; want query filters with page offset", stub.filter)
	}
	if stub.filter.Since == nil || stub.filter.Since.UTC().Format(time.RFC3339) != since ||
		stub.filter.Until == nil || stub.filter.Until.UTC().Format(time.RFC3339) != until {
		t.Fatalf("time filter = since %v until %v; want %s..%s", stub.filter.Since, stub.filter.Until, since, until)
	}
	var out struct {
		Success bool `json:"success"`
		Total   int  `json:"total"`
		Logs    []struct {
			ID         int64           `json:"id"`
			ActorType  string          `json:"actorType"`
			ActorID    string          `json:"actorId"`
			ActorRole  string          `json:"actorRole"`
			Action     string          `json:"action"`
			Result     string          `json:"result"`
			SiteCode   string          `json:"siteCode"`
			TargetType string          `json:"targetType"`
			TargetID   string          `json:"targetId"`
			IP         string          `json:"ip"`
			UserAgent  string          `json:"userAgent"`
			Detail     json.RawMessage `json:"detail"`
			CreatedAt  time.Time       `json:"createdAt"`
		} `json:"logs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !out.Success || out.Total != 1 || len(out.Logs) != 1 {
		t.Fatalf("response = %+v; want one audit log", out)
	}
	got := out.Logs[0]
	if got.Action != "site.pin" || got.Result != "success" || got.ActorRole != "admin" || got.SiteCode != "demo" ||
		!strings.Contains(string(got.Detail), `"pinned":true`) {
		t.Fatalf("log = %+v detail=%s; want site.pin admin detail", got, string(got.Detail))
	}
}

func TestAdminAuditLogsRejectInvalidTimeFilter(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	srv.deployer = &auditLogDeployerStub{}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs?since=not-a-date", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "since must be RFC3339") {
		t.Fatalf("body = %s; want since parse error", rr.Body.String())
	}
}

func TestAdminAuditLogsRequireAdmin(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	user, err := authSvc.CreateUser(t.Context(), "alice", "password123", false, 20)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := authSvc.Generate(t.Context(), "alice-token", false, user.ID, nil)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	srv.deployer = &auditLogDeployerStub{}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+token.Plaintext)
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusForbidden)
	}
}

func TestCSPReportRecordsSecurityAuditLog(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	stub := &auditCSPReportStub{}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPost, "/api/security/csp-report", strings.NewReader(`{
		"csp-report": {
			"document-uri": "https://pagepilot.example.com/agent/demo/",
			"blocked-uri": "https://evil.example.com/x.js",
			"violated-directive": "script-src-elem",
			"effective-directive": "script-src-elem",
			"original-policy": "default-src 'self'; report-uri /api/security/csp-report"
		}
	}`))
	req.Header.Set("Content-Type", "application/csp-report")
	req.Header.Set("User-Agent", "browser-test")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusNoContent)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one security.csp_report log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "security.csp_report" || log.Result != "reported" ||
		log.ActorType != "browser" || log.ActorRole != "public" ||
		log.SiteCode != "demo" || log.TargetType != "csp" || log.TargetID != "script-src-elem" {
		t.Fatalf("audit log = %+v; want CSP report log for demo/script-src-elem", log)
	}
	if log.UserAgent != "browser-test" {
		t.Fatalf("user agent = %q; want browser-test", log.UserAgent)
	}
	if !strings.Contains(log.DetailJSON, `"documentUri":"https://pagepilot.example.com/agent/demo/"`) ||
		!strings.Contains(log.DetailJSON, `"blockedUri":"https://evil.example.com/x.js"`) ||
		!strings.Contains(log.DetailJSON, `"violatedDirective":"script-src-elem"`) {
		t.Fatalf("detail = %s; want normalized CSP report detail", log.DetailJSON)
	}
}

func TestCSPReportingAPIRecordsSecurityAuditLog(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()
	stub := &auditCSPReportStub{}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPost, "/api/security/csp-report", strings.NewReader(`[
		{
			"type": "network-error",
			"url": "https://pagepilot.example.com/agent/ignored/",
			"body": {"blockedURL": "https://ignored.example.com/x.js"}
		},
		{
			"type": "csp-violation",
			"url": "https://pagepilot.example.com/agent/reported/",
			"body": {
				"documentURL": "https://pagepilot.example.com/agent/reported/",
				"blockedURL": "https://cdn.example.com/runtime.js",
				"effectiveDirective": "script-src-elem",
				"violatedDirective": "script-src-elem",
				"originalPolicy": "default-src 'self'; report-uri /api/security/csp-report",
				"sourceFile": "https://pagepilot.example.com/agent/reported/index.html",
				"lineNumber": 17,
				"columnNumber": 9,
				"statusCode": 200,
				"referrer": "https://pagepilot.example.com/market/reported",
				"disposition": "enforce",
				"sample": "runtime reporting api sample"
			}
		}
	]`))
	req.Header.Set("Content-Type", "application/reports+json")
	req.Header.Set("User-Agent", "reporting-api-test")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusNoContent)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one security.csp_report log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "security.csp_report" || log.Result != "reported" ||
		log.ActorType != "browser" || log.ActorRole != "public" ||
		log.SiteCode != "reported" || log.TargetType != "csp" || log.TargetID != "script-src-elem" {
		t.Fatalf("audit log = %+v; want Reporting API CSP log for reported/script-src-elem", log)
	}
	if log.UserAgent != "reporting-api-test" {
		t.Fatalf("user agent = %q; want reporting-api-test", log.UserAgent)
	}
	for _, want := range []string{
		`"documentUri":"https://pagepilot.example.com/agent/reported/"`,
		`"blockedUri":"https://cdn.example.com/runtime.js"`,
		`"violatedDirective":"script-src-elem"`,
		`"effectiveDirective":"script-src-elem"`,
		`"sourceFile":"https://pagepilot.example.com/agent/reported/index.html"`,
		`"lineNumber":17`,
		`"columnNumber":9`,
		`"statusCode":200`,
		`"referrer":"https://pagepilot.example.com/market/reported"`,
		`"sample":"runtime reporting api sample"`,
	} {
		if !strings.Contains(log.DetailJSON, want) {
			t.Fatalf("detail = %s; want %s", log.DetailJSON, want)
		}
	}
}

func TestHostedCSPIncludesReportEndpoint(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	htmlRR := httptest.NewRecorder()
	srv.setHostedContentSecurityHeaders(htmlRR, "strict")
	if csp := htmlRR.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "report-uri /api/security/csp-report") {
		t.Fatalf("hosted HTML CSP = %q; want report-uri endpoint", csp)
	}

	markdownRR := httptest.NewRecorder()
	srv.setHostedMarkdownSecurityHeaders(markdownRR, "nonce-test", false)
	if csp := markdownRR.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "report-uri /api/security/csp-report") {
		t.Fatalf("hosted Markdown CSP = %q; want report-uri endpoint", csp)
	}
}

func TestAdminPinSiteRecordsAuditLog(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &auditSitePinDeployerStub{
		site: store.Site{
			Code:         "demo",
			OwnerTokenID: "user:owner",
			Status:       "active",
			Visibility:   "public",
		},
	}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/sites/demo/pin", strings.NewReader(`{"pinned":true}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "site.pin" || log.ActorType != "user" || log.ActorRole != "admin" ||
		log.Result != "success" || log.SiteCode != "demo" || log.TargetType != "site" || log.TargetID != "demo" {
		t.Fatalf("audit log = %+v; want admin site.pin log for demo", log)
	}
	if !strings.Contains(log.DetailJSON, `"pinned":true`) {
		t.Fatalf("detail = %s; want pinned=true", log.DetailJSON)
	}
}

func TestDeployFailureRecordsAuditLog(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &auditDeployFailureStub{}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPost, "/api/deploy", strings.NewReader(`{
		"filename":"index.html",
		"title":"失败发布",
		"description":"deploy should fail",
		"content":"<h1>fail</h1>"
	}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one failed deploy log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "deploy.create" || log.Result != "failed" || log.ActorRole != "admin" ||
		log.TargetType != "site" {
		t.Fatalf("audit log = %+v; want failed deploy.create admin log", log)
	}
	if !strings.Contains(log.DetailJSON, `"errorCode":"INVALID_INPUT"`) ||
		!strings.Contains(log.DetailJSON, `"stage":"deploy"`) ||
		!strings.Contains(log.DetailJSON, `"title":"失败发布"`) {
		t.Fatalf("detail = %s; want structured deploy failure detail", log.DetailJSON)
	}
}

func TestConfigUpdateFailureRecordsAuditLog(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &auditConfigUpdateStub{}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{
		"corsAllowOrigins":"*"
	}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one failed config.update log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.ActorType != "user" || log.ActorRole != "admin" || log.TargetType != "config" ||
		log.TargetID != "runtime" || log.SiteCode != "" {
		t.Fatalf("audit log = %+v; want admin failed runtime config log", log)
	}
	if !strings.Contains(log.DetailJSON, `"corsAllowOrigins":true`) ||
		!strings.Contains(log.DetailJSON, `"errorCode":"INVALID_INPUT"`) ||
		!strings.Contains(log.DetailJSON, `"stage":"validate"`) {
		t.Fatalf("detail = %s; want failed CORS validation detail", log.DetailJSON)
	}
}

func TestSiteVisibilityFailureRecordsAuditLog(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &auditSiteVisibilityFailureStub{err: store.ErrNotFound}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPatch, "/api/deploys/demo/visibility", strings.NewReader(`{
		"visibility":"public"
	}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusNotFound)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one failed site.visibility log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "site.visibility" || log.Result != "failed" || log.ActorRole != "admin" ||
		log.SiteCode != "demo" || log.TargetType != "site" || log.TargetID != "demo" {
		t.Fatalf("audit log = %+v; want failed visibility log for demo", log)
	}
	if !strings.Contains(log.DetailJSON, `"visibility":"public"`) ||
		!strings.Contains(log.DetailJSON, `"errorCode":"NOT_FOUND"`) ||
		!strings.Contains(log.DetailJSON, `"stage":"site"`) {
		t.Fatalf("detail = %s; want structured visibility failure detail", log.DetailJSON)
	}
}

func TestSiteVisibilityPostAliasRecordsAuditLog(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &auditSiteVisibilityFailureStub{err: store.ErrNotFound}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPost, "/api/deploys/demo/visibility", strings.NewReader(`{
		"visibility":"public"
	}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusNotFound)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one failed site.visibility log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "site.visibility" || log.Result != "failed" || log.SiteCode != "demo" {
		t.Fatalf("audit log = %+v; want failed visibility log for demo", log)
	}
}

func TestVersionAndSiteDeleteFailuresRecordAuditLog(t *testing.T) {
	cases := []struct {
		name           string
		method         string
		path           string
		body           string
		wantStatus     int
		wantAction     string
		wantTargetType string
		wantTargetID   string
		wantDetail     string
	}{
		{
			name:           "lock version",
			method:         http.MethodPost,
			path:           "/api/deploys/demo/versions/7/lock",
			body:           `{"locked":true}`,
			wantStatus:     http.StatusInternalServerError,
			wantAction:     "version.lock",
			wantTargetType: "version",
			wantTargetID:   "7",
			wantDetail:     `"locked":true`,
		},
		{
			name:           "switch current",
			method:         http.MethodPatch,
			path:           "/api/deploys/demo/current",
			body:           `{"versionNumber":9}`,
			wantStatus:     http.StatusInternalServerError,
			wantAction:     "version.current",
			wantTargetType: "version",
			wantTargetID:   "9",
			wantDetail:     `"versionNumber":9`,
		},
		{
			name:           "delete version",
			method:         http.MethodDelete,
			path:           "/api/deploys/demo/versions/3",
			wantStatus:     http.StatusInternalServerError,
			wantAction:     "version.delete",
			wantTargetType: "version",
			wantTargetID:   "3",
		},
		{
			name:           "set version status",
			method:         http.MethodPatch,
			path:           "/api/deploys/demo/versions/5",
			body:           `{"status":"inactive"}`,
			wantStatus:     http.StatusInternalServerError,
			wantAction:     "version.status",
			wantTargetType: "version",
			wantTargetID:   "5",
			wantDetail:     `"status":"inactive"`,
		},
		{
			name:           "overwrite version",
			method:         http.MethodPatch,
			path:           "/api/deploys/demo/versions/6",
			body:           `{"description":"new version","content":"<h1>next</h1>"}`,
			wantStatus:     http.StatusInternalServerError,
			wantAction:     "version.overwrite",
			wantTargetType: "version",
			wantTargetID:   "6",
			wantDetail:     `"description":"new version"`,
		},
		{
			name:           "delete site",
			method:         http.MethodDelete,
			path:           "/api/admin/sites/demo",
			wantStatus:     http.StatusInternalServerError,
			wantAction:     "site.delete",
			wantTargetType: "site",
			wantTargetID:   "demo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, token, cleanup := newDevAuthTestServer(t)
			defer cleanup()
			stub := &auditVersionSiteFailureStub{
				apiErr: NewError(CodeInternal, "simulated", "simulated failure"),
			}
			srv.deployer = stub

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rr := httptest.NewRecorder()

			srv.mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), tc.wantStatus)
			}
			if len(stub.auditLogs) != 1 {
				t.Fatalf("audit logs = %#v; want one failed %s log", stub.auditLogs, tc.wantAction)
			}
			log := stub.auditLogs[0]
			if log.Action != tc.wantAction || log.Result != "failed" || log.ActorRole != "admin" ||
				log.SiteCode != "demo" || log.TargetType != tc.wantTargetType || log.TargetID != tc.wantTargetID {
				t.Fatalf("audit log = %+v; want failed %s log", log, tc.wantAction)
			}
			if !strings.Contains(log.DetailJSON, `"errorCode":"INTERNAL"`) ||
				!strings.Contains(log.DetailJSON, `"stage":"simulated"`) {
				t.Fatalf("detail = %s; want structured API error detail", log.DetailJSON)
			}
			if tc.wantDetail != "" && !strings.Contains(log.DetailJSON, tc.wantDetail) {
				t.Fatalf("detail = %s; want %s", log.DetailJSON, tc.wantDetail)
			}
		})
	}
}

func TestScreenOperationFailuresRecordAuditLog(t *testing.T) {
	version := int64(1)
	cases := []struct {
		name         string
		method       string
		path         string
		body         string
		wantAction   string
		wantSiteCode string
		wantTargetID string
		wantDetail   string
	}{
		{
			name:         "publish",
			method:       http.MethodPost,
			path:         "/api/screens/screen-1/publish",
			body:         `{"code":"demo","versionNumber":1}`,
			wantAction:   "screen.publish",
			wantSiteCode: "demo",
			wantTargetID: "screen-1",
			wantDetail:   `"siteCode":"demo"`,
		},
		{
			name:         "screenshot",
			method:       http.MethodPost,
			path:         "/api/screens/screen-1/screenshot",
			wantAction:   "screen.screenshot.request",
			wantTargetID: "screen-1",
		},
		{
			name:         "command",
			method:       http.MethodPost,
			path:         "/api/screens/screen-1/command",
			body:         `{"type":"refresh"}`,
			wantAction:   "screen.command.request",
			wantTargetID: "screen-1",
			wantDetail:   `"type":"refresh"`,
		},
		{
			name:         "unbind",
			method:       http.MethodDelete,
			path:         "/api/screens/screen-1",
			wantAction:   "screen.unbind",
			wantTargetID: "screen-1",
			wantDetail:   `"ownerUserId":"owner-1"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, token, cleanup := newDevAuthTestServer(t)
			defer cleanup()
			stub := &auditScreenFailureStub{
				site: store.Site{
					Code:           "demo",
					OwnerTokenID:   "user:owner-1",
					CurrentVersion: &version,
					Visibility:     "public",
				},
				screen: store.Screen{
					ID:          "screen-1",
					OwnerUserID: "owner-1",
					Name:        "screen",
				},
				err: errors.New("persist failed"),
			}
			srv.deployer = stub

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rr := httptest.NewRecorder()

			srv.mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusInternalServerError)
			}
			if len(stub.auditLogs) != 1 {
				t.Fatalf("audit logs = %#v; want one failed %s log", stub.auditLogs, tc.wantAction)
			}
			log := stub.auditLogs[0]
			if log.Action != tc.wantAction || log.Result != "failed" || log.ActorRole != "admin" ||
				log.SiteCode != tc.wantSiteCode || log.TargetType != "screen" || log.TargetID != tc.wantTargetID {
				t.Fatalf("audit log = %+v; want failed %s log", log, tc.wantAction)
			}
			if !strings.Contains(log.DetailJSON, `"errorCode":"INTERNAL"`) {
				t.Fatalf("detail = %s; want structured error detail", log.DetailJSON)
			}
			if tc.wantDetail != "" && !strings.Contains(log.DetailJSON, tc.wantDetail) {
				t.Fatalf("detail = %s; want %s", log.DetailJSON, tc.wantDetail)
			}
		})
	}
}

func TestTokenManagementFailuresRecordAuditLog(t *testing.T) {
	t.Run("create token invalid ttl", func(t *testing.T) {
		srv, token, cleanup := newDevAuthTestServer(t)
		defer cleanup()
		stub := &auditTokenFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPost, "/api/token", strings.NewReader(`{
			"label":"bad-token",
			"ttlSeconds":-1
		}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed token.create log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "token.create" || log.Result != "failed" || log.ActorRole != "admin" ||
			log.TargetType != "token" {
			t.Fatalf("audit log = %+v; want failed token.create log", log)
		}
		if !strings.Contains(log.DetailJSON, `"label":"bad-token"`) ||
			!strings.Contains(log.DetailJSON, `"errorCode":"INVALID_INPUT"`) ||
			!strings.Contains(log.DetailJSON, `"stage":"expiresAt"`) {
			t.Fatalf("detail = %s; want structured token.create failure detail", log.DetailJSON)
		}
	})

	t.Run("revoke missing token", func(t *testing.T) {
		srv, token, cleanup := newDevAuthTestServer(t)
		defer cleanup()
		stub := &auditTokenFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodDelete, "/api/tokens/missing-token", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusNotFound)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed token.revoke log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "token.revoke" || log.Result != "failed" || log.ActorRole != "admin" ||
			log.TargetType != "token" || log.TargetID != "missing-token" {
			t.Fatalf("audit log = %+v; want failed token.revoke log", log)
		}
		if !strings.Contains(log.DetailJSON, `"errorCode":"NOT_FOUND"`) ||
			!strings.Contains(log.DetailJSON, `"stage":"revoke_token"`) {
			t.Fatalf("detail = %s; want structured token.revoke failure detail", log.DetailJSON)
		}
	})
}

func TestUserManagementFailuresRecordAuditLog(t *testing.T) {
	t.Run("create invalid email", func(t *testing.T) {
		srv, token, cleanup := newDevAuthTestServer(t)
		defer cleanup()
		stub := &auditUserFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(`{
			"username":"bad-email",
			"email":"not-an-email",
			"password":"password123",
			"isAdmin":false,
			"deployLimit":10
		}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed user.create log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "user.create" || log.Result != "failed" || log.ActorRole != "admin" ||
			log.TargetType != "user" {
			t.Fatalf("audit log = %+v; want failed user.create log", log)
		}
		if !strings.Contains(log.DetailJSON, `"username":"bad-email"`) ||
			!strings.Contains(log.DetailJSON, `"errorCode":"INVALID_INPUT"`) ||
			!strings.Contains(log.DetailJSON, `"stage":"users"`) {
			t.Fatalf("detail = %s; want structured user.create failure detail", log.DetailJSON)
		}
	})

	t.Run("update missing user", func(t *testing.T) {
		srv, token, cleanup := newDevAuthTestServer(t)
		defer cleanup()
		stub := &auditUserFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/missing-user", strings.NewReader(`{
			"username":"ghost"
		}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusNotFound)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed user.update log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "user.update" || log.Result != "failed" || log.ActorRole != "admin" ||
			log.TargetType != "user" || log.TargetID != "missing-user" {
			t.Fatalf("audit log = %+v; want failed user.update log", log)
		}
		if !strings.Contains(log.DetailJSON, `"errorCode":"NOT_FOUND"`) ||
			!strings.Contains(log.DetailJSON, `"stage":"users"`) {
			t.Fatalf("detail = %s; want structured user.update failure detail", log.DetailJSON)
		}
	})

	t.Run("delete own account", func(t *testing.T) {
		srv, token, cleanup := newDevAuthTestServer(t)
		defer cleanup()
		stub := &auditUserFailureStub{}
		srv.deployer = stub
		adminID := adminIDFromToken(t, srv, token)

		req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+adminID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed user.delete log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "user.delete" || log.Result != "failed" || log.ActorRole != "admin" ||
			log.TargetType != "user" || log.TargetID != adminID {
			t.Fatalf("audit log = %+v; want failed user.delete log", log)
		}
		if !strings.Contains(log.DetailJSON, `"errorCode":"INVALID_INPUT"`) ||
			!strings.Contains(log.DetailJSON, `"stage":"users"`) {
			t.Fatalf("detail = %s; want structured user.delete failure detail", log.DetailJSON)
		}
	})
}

func TestAccountPasswordChangeRecordsAuditLog(t *testing.T) {
	t.Run("success without plaintext", func(t *testing.T) {
		srv, authSvc, cleanup := newTokenTestServer(t)
		defer cleanup()
		user, err := authSvc.CreateUser(t.Context(), "alice", "old-password123", false, 20)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		session, err := authSvc.LoginAdmin(t.Context(), "alice", "old-password123", time.Hour)
		if err != nil {
			t.Fatalf("login user: %v", err)
		}
		stub := &auditUserFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPatch, "/api/account/password", strings.NewReader(`{
			"oldPassword":"old-password123",
			"newPassword":"new-password456"
		}`))
		req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one account.password log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "account.password" || log.Result != "success" ||
			log.ActorType != "user" || log.ActorID != user.ID || log.ActorRole != "user" ||
			log.TargetType != "user" || log.TargetID != user.ID {
			t.Fatalf("audit log = %+v; want successful account.password log", log)
		}
		if !strings.Contains(log.DetailJSON, `"username":"alice"`) {
			t.Fatalf("detail = %s; want username", log.DetailJSON)
		}
		if strings.Contains(log.DetailJSON, "old-password123") || strings.Contains(log.DetailJSON, "new-password456") {
			t.Fatalf("detail = %s; must not include plaintext passwords", log.DetailJSON)
		}
	})

	t.Run("wrong old password", func(t *testing.T) {
		srv, authSvc, cleanup := newTokenTestServer(t)
		defer cleanup()
		user, err := authSvc.CreateUser(t.Context(), "alice", "old-password123", false, 20)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		session, err := authSvc.LoginAdmin(t.Context(), "alice", "old-password123", time.Hour)
		if err != nil {
			t.Fatalf("login user: %v", err)
		}
		stub := &auditUserFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPatch, "/api/account/password", strings.NewReader(`{
			"oldPassword":"wrong-password",
			"newPassword":"new-password456"
		}`))
		req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed account.password log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "account.password" || log.Result != "failed" ||
			log.ActorType != "user" || log.ActorID != user.ID || log.ActorRole != "user" ||
			log.TargetType != "user" || log.TargetID != user.ID {
			t.Fatalf("audit log = %+v; want failed account.password log", log)
		}
		if !strings.Contains(log.DetailJSON, `"errorCode":"UNAUTHORIZED"`) ||
			!strings.Contains(log.DetailJSON, `"stage":"password"`) {
			t.Fatalf("detail = %s; want structured password failure detail", log.DetailJSON)
		}
		if strings.Contains(log.DetailJSON, "wrong-password") || strings.Contains(log.DetailJSON, "new-password456") {
			t.Fatalf("detail = %s; must not include plaintext passwords", log.DetailJSON)
		}
	})

	t.Run("short new password", func(t *testing.T) {
		srv, authSvc, cleanup := newTokenTestServer(t)
		defer cleanup()
		user, err := authSvc.CreateUser(t.Context(), "alice", "old-password123", false, 20)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		session, err := authSvc.LoginAdmin(t.Context(), "alice", "old-password123", time.Hour)
		if err != nil {
			t.Fatalf("login user: %v", err)
		}
		stub := &auditUserFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPatch, "/api/account/password", strings.NewReader(`{
			"oldPassword":"old-password123",
			"newPassword":"short"
		}`))
		req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed account.password log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "account.password" || log.Result != "failed" ||
			log.ActorType != "user" || log.ActorID != user.ID || log.ActorRole != "user" ||
			log.TargetType != "user" || log.TargetID != user.ID {
			t.Fatalf("audit log = %+v; want failed account.password log", log)
		}
		if !strings.Contains(log.DetailJSON, `"errorCode":"INVALID_INPUT"`) ||
			!strings.Contains(log.DetailJSON, `"stage":"password"`) {
			t.Fatalf("detail = %s; want structured password validation detail", log.DetailJSON)
		}
		if strings.Contains(log.DetailJSON, "old-password123") || strings.Contains(log.DetailJSON, "short") {
			t.Fatalf("detail = %s; must not include plaintext passwords", log.DetailJSON)
		}
	})
}

func TestAuthLoginRecordsAuditLog(t *testing.T) {
	t.Run("success without plaintext", func(t *testing.T) {
		srv, authSvc, cleanup := newTokenTestServer(t)
		defer cleanup()
		user, err := authSvc.CreateUser(t.Context(), "alice", "password123", false, 20)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		srv.captchas["captcha-login"] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}
		stub := &auditUserFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{
			"username":"alice",
			"password":"password123",
			"captchaId":"captcha-login",
			"captcha":"1234"
		}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one auth.login log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "auth.login" || log.Result != "success" ||
			log.ActorType != "user" || log.ActorID != user.ID || log.ActorRole != "user" ||
			log.TargetType != "user" || log.TargetID != user.ID {
			t.Fatalf("audit log = %+v; want successful auth.login log", log)
		}
		if !strings.Contains(log.DetailJSON, `"username":"alice"`) {
			t.Fatalf("detail = %s; want username", log.DetailJSON)
		}
		if strings.Contains(log.DetailJSON, "password123") {
			t.Fatalf("detail = %s; must not include plaintext password", log.DetailJSON)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		srv, authSvc, cleanup := newTokenTestServer(t)
		defer cleanup()
		if _, err := authSvc.CreateUser(t.Context(), "alice", "password123", false, 20); err != nil {
			t.Fatalf("create user: %v", err)
		}
		srv.captchas["captcha-login"] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}
		stub := &auditUserFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{
			"username":"alice",
			"password":"wrong-password",
			"captchaId":"captcha-login",
			"captcha":"1234"
		}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed auth.login log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "auth.login" || log.Result != "failed" ||
			log.ActorType != "unknown" || log.ActorRole != "public" ||
			log.TargetType != "user" || log.TargetID != "alice" {
			t.Fatalf("audit log = %+v; want failed auth.login log", log)
		}
		if !strings.Contains(log.DetailJSON, `"errorCode":"UNAUTHORIZED"`) ||
			!strings.Contains(log.DetailJSON, `"stage":"auth"`) {
			t.Fatalf("detail = %s; want structured auth failure detail", log.DetailJSON)
		}
		if strings.Contains(log.DetailJSON, "wrong-password") {
			t.Fatalf("detail = %s; must not include plaintext password", log.DetailJSON)
		}
	})
}

func TestAuthRegisterRecordsAuditLog(t *testing.T) {
	t.Run("success without plaintext", func(t *testing.T) {
		srv, _, cleanup := newTokenTestServer(t)
		defer cleanup()
		srv.cfg.AllowRegistration = true
		srv.captchas["captcha-register"] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}
		stub := &auditUserFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{
			"username":"alice",
			"email":"Alice@Example.COM",
			"password":"password123",
			"captchaId":"captcha-register",
			"captcha":"1234"
		}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one auth.register log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "auth.register" || log.Result != "success" ||
			log.ActorType != "user" || log.ActorRole != "user" ||
			log.TargetType != "user" || log.TargetID == "" {
			t.Fatalf("audit log = %+v; want successful auth.register log", log)
		}
		if !strings.Contains(log.DetailJSON, `"username":"alice"`) ||
			!strings.Contains(log.DetailJSON, `"email":"alice@example.com"`) {
			t.Fatalf("detail = %s; want normalized registration detail", log.DetailJSON)
		}
		if strings.Contains(log.DetailJSON, "password123") {
			t.Fatalf("detail = %s; must not include plaintext password", log.DetailJSON)
		}
	})

	t.Run("duplicate username", func(t *testing.T) {
		srv, authSvc, cleanup := newTokenTestServer(t)
		defer cleanup()
		srv.cfg.AllowRegistration = true
		if _, err := authSvc.CreateUser(t.Context(), "alice", "password123", false, 20); err != nil {
			t.Fatalf("create user: %v", err)
		}
		srv.captchas["captcha-register"] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}
		stub := &auditUserFailureStub{}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{
			"username":"alice",
			"password":"password456",
			"captchaId":"captcha-register",
			"captcha":"1234"
		}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed auth.register log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "auth.register" || log.Result != "failed" ||
			log.ActorType != "unknown" || log.ActorRole != "public" ||
			log.TargetType != "user" || log.TargetID != "alice" {
			t.Fatalf("audit log = %+v; want failed auth.register log", log)
		}
		if !strings.Contains(log.DetailJSON, `"errorCode":"INVALID_INPUT"`) ||
			!strings.Contains(log.DetailJSON, `"stage":"register"`) {
			t.Fatalf("detail = %s; want structured register failure detail", log.DetailJSON)
		}
		if strings.Contains(log.DetailJSON, "password456") {
			t.Fatalf("detail = %s; must not include plaintext password", log.DetailJSON)
		}
	})
}

func TestAuthLogoutRecordsAuditLog(t *testing.T) {
	srv, authSvc, cleanup := newTokenTestServer(t)
	defer cleanup()
	user, err := authSvc.CreateUser(t.Context(), "alice", "password123", false, 20)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	session, err := authSvc.LoginAdmin(t.Context(), "alice", "password123", time.Hour)
	if err != nil {
		t.Fatalf("login user: %v", err)
	}
	stub := &auditUserFailureStub{}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
	req.AddCookie(&http.Cookie{Name: "hostctl_admin_session", Value: session.Plaintext})
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one auth.logout log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "auth.logout" || log.Result != "success" ||
		log.ActorType != "user" || log.ActorID != user.ID || log.ActorRole != "user" ||
		log.TargetType != "user" || log.TargetID != user.ID {
		t.Fatalf("audit log = %+v; want successful auth.logout log", log)
	}
	if !strings.Contains(log.DetailJSON, `"username":"alice"`) {
		t.Fatalf("detail = %s; want username", log.DetailJSON)
	}
}

func TestSiteManagementPolicyFailuresRecordAuditLog(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantAction string
		wantDetail string
	}{
		{
			name:       "pin",
			method:     http.MethodPatch,
			path:       "/api/admin/sites/demo/pin",
			body:       `{"pinned":true}`,
			wantStatus: http.StatusInternalServerError,
			wantAction: "site.pin",
			wantDetail: `"pinned":true`,
		},
		{
			name:       "reuse policy",
			method:     http.MethodPatch,
			path:       "/api/admin/sites/demo/reuse-policy",
			body:       `{"reusePolicy":"allow","sourceDownloadPolicy":"deny"}`,
			wantStatus: http.StatusInternalServerError,
			wantAction: "site.reuse_policy",
			wantDetail: `"sourceDownloadPolicy":"deny"`,
		},
		{
			name:       "security mode",
			method:     http.MethodPatch,
			path:       "/api/admin/sites/demo/security-mode",
			body:       `{"securityMode":"strict"}`,
			wantStatus: http.StatusInternalServerError,
			wantAction: "site.security_mode",
			wantDetail: `"securityMode":"strict"`,
		},
		{
			name:       "category",
			method:     http.MethodPatch,
			path:       "/api/admin/sites/demo/category",
			body:       `{"category":"tool"}`,
			wantStatus: http.StatusInternalServerError,
			wantAction: "site.category",
			wantDetail: `"category":"tool"`,
		},
		{
			name:       "tags",
			method:     http.MethodPatch,
			path:       "/api/admin/sites/demo/tags",
			body:       `{"tags":["alpha","beta"]}`,
			wantStatus: http.StatusInternalServerError,
			wantAction: "site.tags",
			wantDetail: `"tags":["alpha","beta"]`,
		},
		{
			name:       "primary strategy",
			method:     http.MethodPatch,
			path:       "/api/deploys/demo/primary-strategy",
			body:       `{"primaryVersionStrategy":"latest"}`,
			wantStatus: http.StatusInternalServerError,
			wantAction: "site.primary_strategy",
			wantDetail: `"strategy":"latest"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, token, cleanup := newDevAuthTestServer(t)
			defer cleanup()
			stub := &auditSiteManagementFailureStub{
				site: store.Site{
					Code:         "demo",
					OwnerTokenID: "user:owner",
					Status:       "active",
					Visibility:   "public",
				},
				err:    errors.New("persist failed"),
				apiErr: NewError(CodeInternal, "simulated", "simulated failure"),
			}
			srv.deployer = stub

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			srv.mux.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), tc.wantStatus)
			}
			if len(stub.auditLogs) != 1 {
				t.Fatalf("audit logs = %#v; want one failed %s log", stub.auditLogs, tc.wantAction)
			}
			log := stub.auditLogs[0]
			if log.Action != tc.wantAction || log.Result != "failed" || log.ActorRole != "admin" ||
				log.SiteCode != "demo" || log.TargetType != "site" || log.TargetID != "demo" {
				t.Fatalf("audit log = %+v; want failed %s log for demo", log, tc.wantAction)
			}
			if !strings.Contains(log.DetailJSON, `"errorCode":"INTERNAL"`) ||
				!strings.Contains(log.DetailJSON, tc.wantDetail) {
				t.Fatalf("detail = %s; want error detail and %s", log.DetailJSON, tc.wantDetail)
			}
		})
	}
}

func TestAnonymousClaimRecordsAuditLog(t *testing.T) {
	t.Run("explicit claim success", func(t *testing.T) {
		srv, token, cleanup := newDevAuthTestServer(t)
		defer cleanup()
		stub := &auditAnonymousClaimStub{
			result: store.AnonymousSessionClaimResult{
				SessionID:      "anon-1",
				UserID:         "user-1",
				SiteCount:      2,
				DeployCount:    3,
				AlreadyClaimed: false,
			},
		}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPost, "/api/session/claim", strings.NewReader(`{"sessionId":"anon-1"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one anonymous.claim log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "anonymous.claim" || log.Result != "success" || log.TargetType != "anonymous_session" ||
			log.TargetID != "anon-1" || log.ActorRole != "user" {
			t.Fatalf("audit log = %+v; want explicit anonymous claim success", log)
		}
		if !strings.Contains(log.DetailJSON, `"siteCount":2`) ||
			!strings.Contains(log.DetailJSON, `"deployCount":3`) ||
			!strings.Contains(log.DetailJSON, `"source":"explicit"`) {
			t.Fatalf("detail = %s; want explicit claim detail", log.DetailJSON)
		}
	})

	t.Run("explicit claim failure", func(t *testing.T) {
		srv, token, cleanup := newDevAuthTestServer(t)
		defer cleanup()
		stub := &auditAnonymousClaimStub{err: store.ErrNotFound}
		srv.deployer = stub

		req := httptest.NewRequest(http.MethodPost, "/api/session/claim", strings.NewReader(`{"sessionId":"missing"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusNotFound)
		}
		if len(stub.auditLogs) != 1 {
			t.Fatalf("audit logs = %#v; want one failed anonymous.claim log", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "anonymous.claim" || log.Result != "failed" || log.TargetType != "anonymous_session" ||
			log.TargetID != "missing" || log.ActorRole != "user" {
			t.Fatalf("audit log = %+v; want explicit anonymous claim failure", log)
		}
		if !strings.Contains(log.DetailJSON, `"sessionId":"missing"`) ||
			!strings.Contains(log.DetailJSON, `"source":"explicit"`) ||
			!strings.Contains(log.DetailJSON, `"errorCode":"NOT_FOUND"`) {
			t.Fatalf("detail = %s; want failed explicit claim detail", log.DetailJSON)
		}
	})
}

func TestAutoAnonymousClaimOnLoginRecordsAuditLog(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv, authSvc, cleanup := newTokenTestServer(t)
		defer cleanup()
		user, err := authSvc.CreateUser(t.Context(), "alice", "password123", false, 20)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		stub := &auditAnonymousClaimStub{
			result: store.AnonymousSessionClaimResult{
				SessionID:   "anon-login",
				UserID:      user.ID,
				SiteCount:   1,
				DeployCount: 1,
			},
		}
		srv.deployer = stub
		srv.captchas["captcha-login"] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}

		req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{
			"username":"alice",
			"password":"password123",
			"captchaId":"captcha-login",
			"captcha":"1234"
		}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hostctl-Session", "anon-login")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
		}
		if len(stub.auditLogs) != 2 {
			t.Fatalf("audit logs = %#v; want anonymous.claim and auth.login logs", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "anonymous.claim" || log.Result != "success" || log.TargetID != "anon-login" ||
			log.ActorID != user.ID {
			t.Fatalf("audit log = %+v; want login auto anonymous claim success", log)
		}
		if !strings.Contains(log.DetailJSON, `"source":"login"`) ||
			!strings.Contains(log.DetailJSON, `"auto":true`) {
			t.Fatalf("detail = %s; want login auto claim detail", log.DetailJSON)
		}
		loginLog := stub.auditLogs[1]
		if loginLog.Action != "auth.login" || loginLog.Result != "success" ||
			loginLog.TargetType != "user" || loginLog.TargetID != user.ID ||
			loginLog.ActorID != user.ID {
			t.Fatalf("audit log = %+v; want login success after auto claim", loginLog)
		}
		if !strings.Contains(loginLog.DetailJSON, `"anonymousClaimed":true`) {
			t.Fatalf("detail = %s; want auth.login to note anonymous claim", loginLog.DetailJSON)
		}
	})

	t.Run("failure does not block login", func(t *testing.T) {
		srv, authSvc, cleanup := newTokenTestServer(t)
		defer cleanup()
		user, err := authSvc.CreateUser(t.Context(), "alice", "password123", false, 20)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		stub := &auditAnonymousClaimStub{err: errors.New("already claimed by another user")}
		srv.deployer = stub
		srv.captchas["captcha-login"] = captchaChallenge{Answer: "1234", ExpiresAt: time.Now().Add(time.Minute)}

		req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{
			"username":"alice",
			"password":"password123",
			"captchaId":"captcha-login",
			"captcha":"1234"
		}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hostctl-Session", "anon-login")
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s; want login to continue", rr.Code, rr.Body.String())
		}
		if len(stub.auditLogs) != 2 {
			t.Fatalf("audit logs = %#v; want failed anonymous.claim and auth.login logs", stub.auditLogs)
		}
		log := stub.auditLogs[0]
		if log.Action != "anonymous.claim" || log.Result != "failed" || log.TargetID != "anon-login" ||
			log.ActorID != user.ID {
			t.Fatalf("audit log = %+v; want login auto anonymous claim failure", log)
		}
		if !strings.Contains(log.DetailJSON, `"source":"login"`) ||
			!strings.Contains(log.DetailJSON, `"auto":true`) ||
			!strings.Contains(log.DetailJSON, `"errorCode":"INVALID_INPUT"`) {
			t.Fatalf("detail = %s; want failed login auto claim detail", log.DetailJSON)
		}
		loginLog := stub.auditLogs[1]
		if loginLog.Action != "auth.login" || loginLog.Result != "success" ||
			loginLog.TargetType != "user" || loginLog.TargetID != user.ID ||
			loginLog.ActorID != user.ID {
			t.Fatalf("audit log = %+v; want login success even after auto claim failure", loginLog)
		}
		if !strings.Contains(loginLog.DetailJSON, `"anonymousClaimed":false`) {
			t.Fatalf("detail = %s; want auth.login to note claim failure", loginLog.DetailJSON)
		}
	})
}

func TestMarketCategoriesFailureRecordsAuditLog(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &auditMarketCategoriesFailureStub{err: errors.New("persist failed")}
	srv.deployer = stub

	req := httptest.NewRequest(http.MethodPut, "/api/admin/market/categories", strings.NewReader(`{
		"categories":[
			{"slug":"tool","label":"效率工具"},
			{"slug":"docs","label":"文档报告"}
		]
	}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusInternalServerError)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one failed config.market_categories log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "config.market_categories" || log.Result != "failed" || log.ActorRole != "admin" ||
		log.TargetType != "config" || log.TargetID != "market_categories" {
		t.Fatalf("audit log = %+v; want failed market categories config log", log)
	}
	if !strings.Contains(log.DetailJSON, `"count":2`) ||
		!strings.Contains(log.DetailJSON, `"errorCode":"INTERNAL"`) ||
		!strings.Contains(log.DetailJSON, `"stage":"market_categories"`) {
		t.Fatalf("detail = %s; want structured market categories failure detail", log.DetailJSON)
	}
}

func TestSkillPackageUploadFailureRecordsAuditLog(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &auditSkillPackageStub{}
	srv.deployer = stub

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "bad.zip")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write([]byte("not a zip")); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/skill/package", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one failed skill.package_upload log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "skill.package_upload" || log.Result != "failed" || log.ActorRole != "admin" ||
		log.TargetType != "skill_package" || log.TargetID != "hostctl-deploy.zip" {
		t.Fatalf("audit log = %+v; want failed skill package upload log", log)
	}
	if !strings.Contains(log.DetailJSON, `"filename":"bad.zip"`) ||
		!strings.Contains(log.DetailJSON, `"size":9`) ||
		!strings.Contains(log.DetailJSON, `"errorCode":"INVALID_INPUT"`) {
		t.Fatalf("detail = %s; want failed skill package detail", log.DetailJSON)
	}
}

func TestSkillPackageUploadSuccessRecordsAuditLog(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &auditSkillPackageStub{}
	srv.deployer = stub

	zipData := newTestSkillZip(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "pagep.zip")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(zipData); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/skill/package", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if len(stub.auditLogs) != 1 {
		t.Fatalf("audit logs = %#v; want one success skill.package_upload log", stub.auditLogs)
	}
	log := stub.auditLogs[0]
	if log.Action != "skill.package_upload" || log.Result != "success" || log.ActorRole != "admin" ||
		log.TargetType != "skill_package" || log.TargetID != "hostctl-deploy.zip" {
		t.Fatalf("audit log = %+v; want success skill package upload log", log)
	}
	if !strings.Contains(log.DetailJSON, `"filename":"pagep.zip"`) ||
		!strings.Contains(log.DetailJSON, `"sha256"`) {
		t.Fatalf("detail = %s; want success skill package detail", log.DetailJSON)
	}
}

func newTestSkillZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("SKILL.md")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("# PagePilot Skill\n")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func adminIDFromToken(t *testing.T, srv *Server, token string) string {
	t.Helper()
	tok, apiErr := srv.authenticateToken(httptest.NewRequest(http.MethodGet, "/", nil))
	if apiErr == nil && tok.OwnerUserID != "" {
		return tok.OwnerUserID
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	tok, apiErr = srv.authenticateToken(req)
	if apiErr != nil {
		t.Fatalf("authenticate token: %v", apiErr)
	}
	return tok.OwnerUserID
}

type auditConfigUpdateStub struct {
	DeployerPort
	auditLogs []store.AuditLog
}

func (s *auditConfigUpdateStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditSiteVisibilityFailureStub struct {
	DeployerPort
	err       error
	auditLogs []store.AuditLog
}

func (s *auditSiteVisibilityFailureStub) SetSiteVisibility(context.Context, string, string) error {
	return s.err
}

func (s *auditSiteVisibilityFailureStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditVersionSiteFailureStub struct {
	DeployerPort
	apiErr    *APIError
	auditLogs []store.AuditLog
}

func (s *auditVersionSiteFailureStub) LockVersion(context.Context, string, int64, bool) (*LockResponse, *APIError) {
	return nil, s.apiErr
}

func (s *auditVersionSiteFailureStub) SwitchCurrent(context.Context, string, int64) (*SetCurrentResponse, *APIError) {
	return nil, s.apiErr
}

func (s *auditVersionSiteFailureStub) DeleteVersion(context.Context, string, int64) (*SetCurrentResponse, *APIError) {
	return nil, s.apiErr
}

func (s *auditVersionSiteFailureStub) SetVersionStatus(context.Context, string, int64, string) (*LockResponse, *APIError) {
	return nil, s.apiErr
}

func (s *auditVersionSiteFailureStub) OverwriteVersion(context.Context, string, int64, OverwriteRequest) (*DeployResponse, *APIError) {
	return nil, s.apiErr
}

func (s *auditVersionSiteFailureStub) DeleteSite(context.Context, string) *APIError {
	return s.apiErr
}

func (s *auditVersionSiteFailureStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditScreenFailureStub struct {
	DeployerPort
	site      store.Site
	screen    store.Screen
	err       error
	auditLogs []store.AuditLog
}

func (s *auditScreenFailureStub) GetSite(context.Context, string) (store.Site, error) {
	return s.site, nil
}

func (s *auditScreenFailureStub) GetScreen(context.Context, string) (store.Screen, error) {
	return s.screen, nil
}

func (s *auditScreenFailureStub) PublishScreen(context.Context, string, string, string, *int64) error {
	return s.err
}

func (s *auditScreenFailureStub) RequestScreenScreenshot(context.Context, string, string) (store.Screen, error) {
	return store.Screen{}, s.err
}

func (s *auditScreenFailureStub) RequestScreenCommand(context.Context, string, string, string, string) (store.Screen, error) {
	return store.Screen{}, s.err
}

func (s *auditScreenFailureStub) UnbindScreen(context.Context, string, string) error {
	return s.err
}

func (s *auditScreenFailureStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditTokenFailureStub struct {
	DeployerPort
	auditLogs []store.AuditLog
}

func (s *auditTokenFailureStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditUserFailureStub struct {
	DeployerPort
	auditLogs []store.AuditLog
}

func (s *auditUserFailureStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditSiteManagementFailureStub struct {
	DeployerPort
	site      store.Site
	err       error
	apiErr    *APIError
	auditLogs []store.AuditLog
}

func (s *auditSiteManagementFailureStub) GetSite(_ context.Context, code string) (store.Site, error) {
	if s.site.Code != code {
		return store.Site{}, store.ErrNotFound
	}
	return s.site, nil
}

func (s *auditSiteManagementFailureStub) SetSitePinned(context.Context, string, bool) error {
	return s.err
}

func (s *auditSiteManagementFailureStub) SetSiteReusePolicy(context.Context, string, string, string) error {
	return s.err
}

func (s *auditSiteManagementFailureStub) SetSiteSecurityMode(context.Context, string, string) error {
	return s.err
}

func (s *auditSiteManagementFailureStub) SetSiteCategory(context.Context, string, string) error {
	return s.err
}

func (s *auditSiteManagementFailureStub) SetSiteTags(context.Context, string, []string) error {
	return s.err
}

func (s *auditSiteManagementFailureStub) SetPrimaryStrategy(context.Context, string, PrimaryVersionStrategy) (*PrimaryStrategyResponse, *APIError) {
	return nil, s.apiErr
}

func (s *auditSiteManagementFailureStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditAnonymousClaimStub struct {
	DeployerPort
	result    store.AnonymousSessionClaimResult
	err       error
	auditLogs []store.AuditLog
}

func (s *auditAnonymousClaimStub) ClaimAnonymousSession(_ context.Context, id, userID string) (store.AnonymousSessionClaimResult, error) {
	if s.err != nil {
		return store.AnonymousSessionClaimResult{}, s.err
	}
	result := s.result
	if result.SessionID == "" {
		result.SessionID = id
	}
	if result.UserID == "" {
		result.UserID = userID
	}
	return result, nil
}

func (s *auditAnonymousClaimStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditMarketCategoriesFailureStub struct {
	DeployerPort
	err       error
	auditLogs []store.AuditLog
}

func (s *auditMarketCategoriesFailureStub) SetMarketCategories(context.Context, []MarketCategory) error {
	return s.err
}

func (s *auditMarketCategoriesFailureStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditSkillPackageStub struct {
	DeployerPort
	auditLogs []store.AuditLog
}

func (s *auditSkillPackageStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditLogDeployerStub struct {
	DeployerPort
	logs   []store.AuditLog
	filter store.AuditLogFilter
}

func (s *auditLogDeployerStub) ListAuditLogs(_ context.Context, filter store.AuditLogFilter) ([]store.AuditLog, int, error) {
	s.filter = filter
	return s.logs, len(s.logs), nil
}

type auditCSPReportStub struct {
	DeployerPort
	auditLogs []store.AuditLog
}

func (s *auditCSPReportStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditSitePinDeployerStub struct {
	DeployerPort
	site      store.Site
	auditLogs []store.AuditLog
}

func (s *auditSitePinDeployerStub) SetSitePinned(_ context.Context, code string, pinned bool) error {
	if s.site.Code != code {
		return store.ErrNotFound
	}
	s.site.IsPinned = pinned
	now := time.Now().UTC()
	s.site.PinnedAt = &now
	return nil
}

func (s *auditSitePinDeployerStub) GetSite(_ context.Context, code string) (store.Site, error) {
	if s.site.Code != code {
		return store.Site{}, store.ErrNotFound
	}
	return s.site, nil
}

func (s *auditSitePinDeployerStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}

type auditDeployFailureStub struct {
	DeployerPort
	auditLogs []store.AuditLog
}

func (s *auditDeployFailureStub) Deploy(context.Context, DeployRequest, string, string) (*DeployResponse, *APIError) {
	return nil, NewError(CodeInvalidInput, "deploy", "simulated deploy failure")
}

func (s *auditDeployFailureStub) RecordAuditLog(_ context.Context, log store.AuditLog) error {
	s.auditLogs = append(s.auditLogs, log)
	return nil
}
