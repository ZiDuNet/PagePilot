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
