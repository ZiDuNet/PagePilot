package api

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresForwardedHeadersFromPublicPeer(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.20:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("X-Real-IP", "203.0.113.8")

	if got := clientIPFromRequest(req); got != "198.51.100.20" {
		t.Fatalf("clientIPFromRequest() = %q; want remote peer", got)
	}
}

func TestClientIPAcceptsForwardedHeadersFromPrivateProxy(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 127.0.0.1")

	if got := clientIPFromRequest(req); got != "203.0.113.7" {
		t.Fatalf("clientIPFromRequest() = %q; want first forwarded address", got)
	}
}

func TestClientIPRejectsMalformedForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.4:1234"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	req.Header.Set("X-Real-IP", "also-not-an-ip")

	if got := clientIPFromRequest(req); got != "10.0.0.4" {
		t.Fatalf("clientIPFromRequest() = %q; want remote peer", got)
	}
}
