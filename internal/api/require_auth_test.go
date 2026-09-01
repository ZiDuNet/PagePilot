package api

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireAuthenticatedWriteBlocksAnonymousMutationsInProduction(t *testing.T) {
	s := &Server{requireAuth: true, logger: log.New(io.Discard, "", 0)}
	h := s.withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		name string
		path string
		want int
	}{
		{name: "deploy", path: "/api/deploy", want: http.StatusUnauthorized},
		{name: "content patch", path: "/api/deploy/content", want: http.StatusUnauthorized},
		{name: "version mutation", path: "/api/deploys/demo/current", want: http.StatusUnauthorized},
		{name: "public like", path: "/api/deploys/demo/like", want: http.StatusNoContent},
		{name: "public access login", path: "/api/deploys/demo/access", want: http.StatusNoContent},
		{name: "registration", path: "/api/auth/register", want: http.StatusNoContent},
		{name: "production setup", path: "/api/admin/setup", want: http.StatusUnauthorized},
		{name: "device pairing start", path: "/api/device/pairing/start", want: http.StatusNoContent},
		{name: "device pairing complete", path: "/api/device/pairing/complete", want: http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestRequireAuthenticatedWriteAllowsBearerRequestsToReachHandler(t *testing.T) {
	s := &Server{requireAuth: true, logger: log.New(io.Discard, "", 0)}
	h := s.withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/deploy", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer invalid")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want handler status %d", rr.Code, http.StatusNoContent)
	}
}
