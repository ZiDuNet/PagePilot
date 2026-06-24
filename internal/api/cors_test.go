package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/config"
)

func TestApplyCORSDisabledByDefault(t *testing.T) {
	srv := New(config.Default(), nil, nil, true, logTestLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()

	srv.applyCORS(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestApplyCORSAllowsExplicitOriginsOnly(t *testing.T) {
	cfg := config.Default()
	cfg.CORSAllowOrigins = "https://admin.example.com, https://studio.example.com"
	srv := New(cfg, nil, nil, true, logTestLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Origin", "https://studio.example.com")
	rr := httptest.NewRecorder()

	srv.applyCORS(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://studio.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://studio.example.com", got)
	}
	if got := rr.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q, want Origin", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want Authorization", got)
	}
}

func TestCorsOriginsLookValidRejectsWildcard(t *testing.T) {
	if corsOriginsLookValid("*") {
		t.Fatal("wildcard * must not be accepted")
	}
	if !corsOriginsLookValid("https://admin.example.com, https://studio.example.com") {
		t.Fatal("explicit origin list should be accepted")
	}
}

func logTestLogger() *log.Logger {
	return log.New(bytes.NewBuffer(nil), "", 0)
}
