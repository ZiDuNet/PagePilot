package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestDeployMultipartSendsFileAndCurrentOriginHeader(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "index.html")
	if err := os.WriteFile(source, []byte("<!doctype html><title>Multipart</title>"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	var gotOrigin string
	var gotContentType string
	var gotFile string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrigin = r.Header.Get(currentOriginHeader)
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
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
		SourcePath:  source,
		Filename:    "index.html",
		Description: "demo",
		Title:       "Multipart",
		Visibility:  "unlisted",
		Source:      "cli",
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
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
