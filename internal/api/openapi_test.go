package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAPIIncludesAuditAndSiteDetailContracts(t *testing.T) {
	srv, _, cleanup := newDevAuthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	var doc struct {
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}

	auditPath := doc.Paths["/api/admin/audit-logs"]
	if auditPath == nil || auditPath["get"] == nil {
		t.Fatalf("openapi missing GET /api/admin/audit-logs")
	}
	auditGet := auditPath["get"].(map[string]any)
	params, _ := auditGet["parameters"].([]any)
	if len(params) == 0 {
		if raw, ok := auditGet["parameters"].([]map[string]any); ok {
			for _, item := range raw {
				params = append(params, item)
			}
		}
	}
	paramNames := map[string]bool{}
	for _, raw := range params {
		if item, ok := raw.(map[string]any); ok {
			if name, _ := item["name"].(string); name != "" {
				paramNames[name] = true
			}
		}
	}
	if !paramNames["since"] || !paramNames["until"] || paramNames["from"] || paramNames["to"] {
		t.Fatalf("audit log time params = %+v; want since/until only", paramNames)
	}
	if !paramNames["targetId"] {
		t.Fatalf("audit log params = %+v; want targetId filter", paramNames)
	}
	if !paramNames["actorType"] || !paramNames["q"] {
		t.Fatalf("audit log params = %+v; want actorType and q filters", paramNames)
	}
	cspPath := doc.Paths["/api/security/csp-report"]
	if cspPath == nil || cspPath["post"] == nil {
		t.Fatalf("openapi missing POST /api/security/csp-report")
	}
	cspPost := cspPath["post"].(map[string]any)
	if _, ok := cspPost["security"].([]any); !ok {
		t.Fatalf("CSP report endpoint should be public with explicit empty security: %+v", cspPost)
	}
	accessPath := doc.Paths["/api/deploys/{code}/access"]
	if accessPath == nil || accessPath["post"] == nil {
		t.Fatalf("openapi missing POST /api/deploys/{code}/access")
	}
	accessPost := accessPath["post"].(map[string]any)
	accessParams, _ := accessPost["parameters"].([]any)
	accessParamNames := map[string]bool{}
	for _, raw := range accessParams {
		if item, ok := raw.(map[string]any); ok {
			if name, _ := item["name"].(string); name != "" {
				accessParamNames[name] = true
			}
		}
	}
	if !accessParamNames["code"] || !accessParamNames["version"] {
		t.Fatalf("site access params = %+v; want code and version", accessParamNames)
	}
	versionPath := doc.Paths["/api/deploys/{code}/versions/{version}"]
	if versionPath == nil || versionPath["patch"] == nil {
		t.Fatalf("openapi missing PATCH /api/deploys/{code}/versions/{version}")
	}
	versionPatch := versionPath["patch"].(map[string]any)
	versionBody, _ := versionPatch["requestBody"].(map[string]any)
	versionContent, _ := versionBody["content"].(map[string]any)
	if versionContent["application/json"] == nil || versionContent["multipart/form-data"] == nil {
		t.Fatalf("version patch content = %+v; want json and multipart", versionContent)
	}
	sitePath := doc.Paths["/api/admin/sites/{code}"]
	if sitePath == nil || sitePath["get"] == nil || sitePath["delete"] == nil {
		t.Fatalf("openapi /api/admin/sites/{code} = %+v; want both GET and DELETE", sitePath)
	}
	reusePolicyPath := doc.Paths["/api/admin/sites/{code}/reuse-policy"]
	if reusePolicyPath == nil || reusePolicyPath["patch"] == nil {
		t.Fatalf("openapi missing PATCH /api/admin/sites/{code}/reuse-policy")
	}
	securityModePath := doc.Paths["/api/admin/sites/{code}/security-mode"]
	if securityModePath == nil || securityModePath["patch"] == nil {
		t.Fatalf("openapi missing PATCH /api/admin/sites/{code}/security-mode")
	}
	for _, name := range []string{
		"AuditLogListResponse",
		"AuditLogListItem",
		"SiteDetailResponse",
		"BundleDetail",
		"ReuseDetail",
		"ContentFile",
		"SiteReusePolicyRequest",
		"SiteReusePolicyResponse",
		"SiteSecurityModeRequest",
		"SiteSecurityModeResponse",
	} {
		if doc.Components.Schemas[name] == nil {
			t.Fatalf("openapi missing schema %s", name)
		}
	}
	deploySchema, _ := doc.Components.Schemas["DeployRequest"].(map[string]any)
	deployRequired, _ := deploySchema["required"].([]any)
	for _, raw := range deployRequired {
		if raw == "filename" {
			t.Fatalf("DeployRequest required = %+v; filename must be optional", deployRequired)
		}
	}
	if len(deployRequired) != 1 || deployRequired[0] != "description" {
		t.Fatalf("DeployRequest required = %+v; want only description", deployRequired)
	}
	reuseSchema, _ := doc.Components.Schemas["ReuseDetail"].(map[string]any)
	reuseProps, _ := reuseSchema["properties"].(map[string]any)
	if reuseProps["policyNote"] == nil {
		t.Fatalf("ReuseDetail schema missing policyNote: %+v", reuseProps)
	}
	bundleSchema, _ := doc.Components.Schemas["BundleDetail"].(map[string]any)
	bundleProps, _ := bundleSchema["properties"].(map[string]any)
	if bundleProps["siteSecurityMode"] == nil || bundleProps["effectiveSecurityMode"] == nil {
		t.Fatalf("BundleDetail schema missing security mode fields: %+v", bundleProps)
	}
	kindSchema, _ := bundleProps["kind"].(map[string]any)
	kindValues, _ := kindSchema["enum"].([]any)
	wantKinds := map[string]bool{"single_html": true, "markdown": true, "zip_site": true, "static_site": true}
	for _, raw := range kindValues {
		if value, _ := raw.(string); value != "" {
			delete(wantKinds, value)
		}
	}
	if len(wantKinds) != 0 {
		t.Fatalf("BundleDetail kind enum missing values: %+v in %+v", wantKinds, kindSchema)
	}
}

func TestSanitizeMultipartDeployPath(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		fallback string
		want     string
	}{
		{name: "keeps chinese letters", in: "2026-07-03_王关飞.zip", fallback: "upload", want: "2026-07-03_王关飞.zip"},
		{name: "removes special characters", in: "my file(1).html", fallback: "upload", want: "my-file-1.html"},
		{name: "cleans nested path", in: "../assets/app demo.css", fallback: "asset", want: "assets/app-demo.css"},
		{name: "fallback keeps extension", in: "!!!.md", fallback: "upload", want: "upload.md"},
		{name: "empty fallback", in: "", fallback: "upload", want: "upload"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeMultipartDeployPath(tc.in, tc.fallback); got != tc.want {
				t.Fatalf("sanitizeMultipartDeployPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
