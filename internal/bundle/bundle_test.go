package bundle

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestAnalyzeZipBundleStripsNestedDistRoot(t *testing.T) {
	zipBytes := makeBundleTestZip(t, map[string]string{
		"project/dist/index.html":      "<!doctype html><html><body><div>Hello</div></body></html>",
		"project/dist/assets/app.css":  "body{color:#075985}",
		"project/README.md":            "# wrapper",
		"project/dist/images/logo.svg": "<svg></svg>",
	})

	result, err := AnalyzeZip(Input{
		Name: "site.zip",
		Data: zipBytes,
		Limits: Limits{
			MaxSingleFileBytes: 1 << 20,
			MaxSiteTotalBytes:  2 << 20,
			MaxFiles:           50,
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeZip returned error: %v", err)
	}
	if result.MainEntry != "index.html" {
		t.Fatalf("MainEntry = %q, want index.html", result.MainEntry)
	}
	if result.Root != "project/dist" {
		t.Fatalf("Root = %q, want project/dist", result.Root)
	}
	if result.Kind != KindHTML {
		t.Fatalf("Kind = %q, want %q", result.Kind, KindHTML)
	}
	if !hasBundlePath(result.Files, "assets/app.css") || !hasBundlePath(result.Files, "images/logo.svg") {
		t.Fatalf("expected assets to be rooted under deploy folder: %#v", result.Files)
	}
	if hasBundlePath(result.Files, "project/dist/index.html") {
		t.Fatalf("expected wrapper directories to be stripped")
	}
	if !strings.Contains(result.TreeJSON, "assets/app.css") {
		t.Fatalf("TreeJSON does not mention asset: %s", result.TreeJSON)
	}
}

func TestAnalyzeZipBundleKeepsMarkdownAssets(t *testing.T) {
	zipBytes := makeBundleTestZip(t, map[string]string{
		"docs/README.md":        "# Guide\n\n![logo](images/logo.png)",
		"docs/images/logo.png":  "not-real-png-but-binary",
		"docs/assets/theme.css": "body{}",
	})

	result, err := AnalyzeZip(Input{
		Name: "docs.zip",
		Data: zipBytes,
		Limits: Limits{
			MaxSingleFileBytes: 1 << 20,
			MaxSiteTotalBytes:  2 << 20,
			MaxFiles:           50,
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeZip returned error: %v", err)
	}
	if result.MainEntry != "README.md" {
		t.Fatalf("MainEntry = %q, want README.md", result.MainEntry)
	}
	if result.Kind != KindMarkdown {
		t.Fatalf("Kind = %q, want %q", result.Kind, KindMarkdown)
	}
	if !hasBundlePath(result.Files, "images/logo.png") {
		t.Fatalf("expected markdown image asset to be kept: %#v", result.Files)
	}
}

func TestAnalyzeZipBundleUsesExplicitEntryHint(t *testing.T) {
	zipBytes := makeBundleTestZip(t, map[string]string{
		"site/index.html": "<!doctype html><html><body><div>default</div></body></html>",
		"site/print.html": "<!doctype html><html><body><div>print</div></body></html>",
		"site/app.css":    "body{}",
	})

	result, err := AnalyzeZip(Input{
		Name:      "site.zip",
		Data:      zipBytes,
		EntryHint: "site/print.html",
		Limits: Limits{
			MaxSingleFileBytes: 1 << 20,
			MaxSiteTotalBytes:  2 << 20,
			MaxFiles:           50,
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeZip returned error: %v", err)
	}
	if result.MainEntry != "print.html" {
		t.Fatalf("MainEntry = %q, want print.html", result.MainEntry)
	}
	if result.Root != "site" {
		t.Fatalf("Root = %q, want site", result.Root)
	}
}

func TestAnalyzeZipBundleRejectsTraversal(t *testing.T) {
	zipBytes := makeBundleTestZip(t, map[string]string{
		"../index.html": "<html><body><div>bad</div></body></html>",
	})

	_, err := AnalyzeZip(Input{
		Name: "bad.zip",
		Data: zipBytes,
		Limits: Limits{
			MaxSingleFileBytes: 1 << 20,
			MaxSiteTotalBytes:  2 << 20,
			MaxFiles:           50,
		},
	})
	if err == nil {
		t.Fatal("expected traversal ZIP entry to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "safe relative path") {
		t.Fatalf("unexpected error: %v", err)
	}
	var bundleErr *Error
	if !errors.As(err, &bundleErr) {
		t.Fatalf("expected structured bundle error, got %T", err)
	}
	if bundleErr.Stage != StageZipBundle {
		t.Fatalf("Stage = %q, want %q", bundleErr.Stage, StageZipBundle)
	}
	if bundleErr.Code != ErrCodeUnsafePath {
		t.Fatalf("Code = %q, want %q", bundleErr.Code, ErrCodeUnsafePath)
	}
	if bundleErr.Hint == "" {
		t.Fatal("expected bundle error to include a hint")
	}
}

func TestAnalyzeZipBundleRejectsBatchWithoutSingleEntryRoot(t *testing.T) {
	zipBytes := makeBundleTestZip(t, map[string]string{
		"one/index.html": "<!doctype html><html><body><div>one</div></body></html>",
		"two/index.html": "<!doctype html><html><body><div>two</div></body></html>",
	})

	_, err := AnalyzeZip(Input{
		Name: "batch.zip",
		Data: zipBytes,
		Limits: Limits{
			MaxSingleFileBytes: 1 << 20,
			MaxSiteTotalBytes:  2 << 20,
			MaxFiles:           50,
		},
	})
	if err == nil {
		t.Fatal("expected batch ZIP to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "multiple") {
		t.Fatalf("unexpected error: %v", err)
	}
	var bundleErr *Error
	if !errors.As(err, &bundleErr) {
		t.Fatalf("expected structured bundle error, got %T", err)
	}
	if bundleErr.Code != ErrCodeAmbiguousEntry {
		t.Fatalf("Code = %q, want %q", bundleErr.Code, ErrCodeAmbiguousEntry)
	}
	if bundleErr.Hint == "" {
		t.Fatal("expected bundle error to include a hint")
	}
}

func makeBundleTestZip(t *testing.T, files map[string]string) []byte {
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

func hasBundlePath(files []File, path string) bool {
	for _, f := range files {
		if f.Path == path {
			return true
		}
	}
	return false
}
