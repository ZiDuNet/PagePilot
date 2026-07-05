package deploy

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/config"
)

func TestResolveContentExpandsNestedZipRoot(t *testing.T) {
	d := New(config.Default(), nil)
	d.cfg.MaxSingleFileBytes = 1 << 20
	d.cfg.MaxSiteTotalBytes = 2 << 20
	d.cfg.MaxFilesPerSite = 50

	zipBytes := makeTestZip(t, map[string]string{
		"project/dist/index.html":      "<!doctype html><html><body><div>Hello</div></body></html>",
		"project/dist/assets/app.css":  "body{color:#075985}",
		"project/README.md":            "# wrapper",
		"project/dist/images/logo.svg": "<svg></svg>",
	})

	files, mainEntry, apiErr := d.resolveContent(api.DeployRequest{
		Files: []api.DeployFile{{
			Path:          "site.zip",
			ContentBase64: base64.StdEncoding.EncodeToString(zipBytes),
		}},
	}, "")
	if apiErr != nil {
		t.Fatalf("resolveContent returned error: %v", apiErr.Detail)
	}
	if mainEntry != "index.html" {
		t.Fatalf("mainEntry = %q, want index.html", mainEntry)
	}
	if !hasResolvedPath(files, "assets/app.css") || !hasResolvedPath(files, "images/logo.svg") {
		t.Fatalf("expected assets to be rooted under deploy folder: %#v", files)
	}
	if hasResolvedPath(files, "project/dist/index.html") {
		t.Fatalf("expected wrapper directories to be stripped")
	}
}

func TestResolveContentMarkdownZipWithImages(t *testing.T) {
	d := New(config.Default(), nil)
	d.cfg.MaxSingleFileBytes = 1 << 20
	d.cfg.MaxSiteTotalBytes = 2 << 20
	d.cfg.MaxFilesPerSite = 50

	zipBytes := makeTestZip(t, map[string]string{
		"docs/README.md":        "# Guide\n\n![logo](images/logo.png)",
		"docs/images/logo.png":  "not-real-png-but-binary",
		"docs/assets/theme.css": "body{}",
	})

	files, mainEntry, apiErr := d.resolveContent(api.DeployRequest{
		Files: []api.DeployFile{{
			Path:          "docs.zip",
			ContentBase64: base64.StdEncoding.EncodeToString(zipBytes),
		}},
	}, "")
	if apiErr != nil {
		t.Fatalf("resolveContent returned error: %v", apiErr.Detail)
	}
	if mainEntry != "README.md" {
		t.Fatalf("mainEntry = %q, want README.md", mainEntry)
	}
	if !hasResolvedPath(files, "images/logo.png") {
		t.Fatalf("expected markdown image asset to be kept: %#v", files)
	}
	if apiErr := validateEntrypoint(files, mainEntry); apiErr != nil {
		t.Fatalf("validateEntrypoint returned error: %v", apiErr.Detail)
	}
}

func TestResolveContentRejectsZipTraversal(t *testing.T) {
	d := New(config.Default(), nil)
	d.cfg.MaxSingleFileBytes = 1 << 20
	d.cfg.MaxSiteTotalBytes = 2 << 20
	d.cfg.MaxFilesPerSite = 50

	zipBytes := makeTestZip(t, map[string]string{
		"../index.html": "<html><body><div>bad</div></body></html>",
	})

	_, _, apiErr := d.resolveContent(api.DeployRequest{
		Files: []api.DeployFile{{
			Path:          "bad.zip",
			ContentBase64: base64.StdEncoding.EncodeToString(zipBytes),
		}},
	}, "")
	if apiErr == nil {
		t.Fatal("expected traversal ZIP entry to be rejected")
	}
	if !strings.Contains(strings.ToLower(apiErr.Detail), "safe relative path") {
		t.Fatalf("unexpected error detail: %s", apiErr.Detail)
	}
}

func makeTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for path, content := range files {
		w, err := zw.Create(path)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func hasResolvedPath(files []resolvedFile, path string) bool {
	for _, f := range files {
		if f.Path == path {
			return true
		}
	}
	return false
}
