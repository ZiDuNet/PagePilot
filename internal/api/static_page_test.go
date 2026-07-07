package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserStaticPagesWithDotsAreServedBeforeShortCodeRouting(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	for _, path := range []string{"/deploy", "/agents/", "/screens/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %q; want %d", path, rr.Code, rr.Body.String(), http.StatusOK)
		}
		if !strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("%s content type = %q, want text/html", path, rr.Header().Get("Content-Type"))
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api-docs.html", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("/api-docs.html status = %d, body = %q; want %d", rr.Code, rr.Body.String(), http.StatusFound)
	}
	if got := rr.Header().Get("Location"); got != "/admin?tab=apiDocs" {
		t.Fatalf("/api-docs.html redirect = %q, want /admin?tab=apiDocs", got)
	}
}

func TestAdminUIAndAssetsUseStrictCSP(t *testing.T) {
	srv, _, cleanup := newTokenTestServer(t)
	defer cleanup()

	for _, path := range []string{"/admin", "/admin/assets/index.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()

		srv.mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %q; want %d", path, rr.Code, rr.Body.String(), http.StatusOK)
		}
		csp := rr.Header().Get("Content-Security-Policy")
		for _, want := range []string{
			"default-src 'self'",
			"script-src 'self'",
			"style-src 'self'",
			"object-src 'none'",
			"base-uri 'none'",
			"frame-ancestors 'none'",
			"report-uri /api/security/csp-report",
		} {
			if !strings.Contains(csp, want) {
				t.Fatalf("%s CSP = %q, missing %q", path, csp, want)
			}
		}
		if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "*") {
			t.Fatalf("%s CSP = %q, should not allow broad admin execution", path, csp)
		}
		if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s missing nosniff", path)
		}
	}
}
