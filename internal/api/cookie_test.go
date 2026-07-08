package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourorg/hostctl/internal/config"
)

func TestCookieSecureForRequestHonorsForwardedProto(t *testing.T) {
	srv := &Server{cfg: config.Config{AppURLScheme: "https"}}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", nil)
	req.Header.Set("X-Forwarded-Proto", "http")

	if srv.cookieSecureForRequest(req) {
		t.Fatalf("cookieSecureForRequest() = true; want false for forwarded http")
	}

	req.Header.Set("X-Forwarded-Proto", "https")
	if !srv.cookieSecureForRequest(req) {
		t.Fatalf("cookieSecureForRequest() = false; want true for forwarded https")
	}
}

func TestCookieSecureForRequestHonorsTLS(t *testing.T) {
	srv := &Server{cfg: config.Config{AppURLScheme: "http"}}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", nil)
	req.TLS = &tls.ConnectionState{}

	if !srv.cookieSecureForRequest(req) {
		t.Fatalf("cookieSecureForRequest() = false; want true for TLS request")
	}
}

func TestCookieSecureForRequestUsesDevFallback(t *testing.T) {
	t.Setenv("HOSTCTL_DEV", "1")

	srv := &Server{cfg: config.Config{AppURLScheme: "https"}}

	if srv.cookieSecureForRequest(nil) {
		t.Fatalf("cookieSecureForRequest(nil) = true; want false in dev mode")
	}
}
