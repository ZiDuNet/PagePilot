package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
