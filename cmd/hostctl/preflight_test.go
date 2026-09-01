package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/bundle"
)

func TestAnalyzePreflightZIPUsesServerBundleRules(t *testing.T) {
	source := filepath.Join(t.TempDir(), "site.zip")
	writePreflightZIP(t, source, map[string]string{
		"project/dist/index.html":     "<!doctype html><html><body><main>ready</main></body></html>",
		"project/dist/assets/app.css": "body{}",
		"project/README.md":           "# Wrapper",
	})

	report := analyzePreflight(source, "")
	if !report.Success {
		t.Fatalf("report = %+v", report)
	}
	if report.SourceType != "zip" || report.Kind != "zip_site" || report.Root != "project/dist" || report.MainEntry != "index.html" {
		t.Fatalf("unexpected report = %+v", report)
	}
	if report.Count != 2 || report.Files[0].Path != "assets/app.css" || report.Files[1].Path != "index.html" {
		t.Fatalf("files = %+v", report.Files)
	}
}

func TestAnalyzePreflightReportsStructuredBundleError(t *testing.T) {
	source := filepath.Join(t.TempDir(), "unsafe.zip")
	writePreflightZIP(t, source, map[string]string{
		"../index.html": "<html></html>",
	})

	report := analyzePreflight(source, "")
	if report.Success {
		t.Fatalf("report = %+v; want failure", report)
	}
	if len(report.Errors) != 1 || report.Errors[0].Code != "ZIP_UNSAFE_PATH" || report.Errors[0].Stage != "zip_bundle" {
		t.Fatalf("errors = %+v", report.Errors)
	}
}

func TestAnalyzePreflightAcceptsDeployableHTML(t *testing.T) {
	source := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(source, []byte("<!doctype html><html><body><main>ready</main></body></html>"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	report := analyzePreflight(source, "")
	if !report.Success || report.Kind != "single_html" || report.MainEntry != "index.html" {
		t.Fatalf("report = %+v", report)
	}
}

func TestAnalyzePreflightUsesMultipartFilenameNormalization(t *testing.T) {
	source := filepath.Join(t.TempDir(), "my page.html")
	if err := os.WriteFile(source, []byte("<!doctype html><html><body><main>ready</main></body></html>"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	report := analyzePreflight(source, "")
	if !report.Success || report.MainEntry != "my-page.html" || report.Files[0].Path != "my-page.html" {
		t.Fatalf("report = %+v", report)
	}
}

func TestAnalyzePreflightMatchesServerEntrypointValidation(t *testing.T) {
	tests := []struct {
		name   string
		source string
		files  map[string]string
	}{
		{
			name:   "short single HTML",
			source: "index.html",
		},
		{
			name:   "non-page ZIP entry",
			source: "site.zip",
			files:  map[string]string{"index.html": "this is not an HTML page but is long enough to bypass a length-only check"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), tc.source)
			if tc.files != nil {
				writePreflightZIP(t, source, tc.files)
			} else if err := os.WriteFile(source, []byte("<html></html>"), 0o644); err != nil {
				t.Fatalf("write source: %v", err)
			}

			report := analyzePreflight(source, "")
			if report.Success {
				t.Fatalf("report = %+v; want failure", report)
			}
			if len(report.Errors) != 1 || report.Errors[0].Code != "INVALID_INPUT" || report.Errors[0].Stage != "validate" {
				t.Fatalf("errors = %+v", report.Errors)
			}
		})
	}
}

func TestAnalyzePreflightRejectsUnsafeEntryHint(t *testing.T) {
	source := filepath.Join(t.TempDir(), "site.zip")
	writePreflightZIP(t, source, map[string]string{"index.html": "<html></html>"})

	report := analyzePreflight(source, "../index.html")
	if report.Success {
		t.Fatalf("report = %+v; want failure", report)
	}
	if len(report.Errors) != 1 || report.Errors[0].Code != "UNSAFE_ENTRY_PATH" {
		t.Fatalf("errors = %+v", report.Errors)
	}
}

func TestNormalizeFilenameHintUsesServerPathSeparators(t *testing.T) {
	got, err := normalizeFilenameHint(`dist\index.html`)
	if err != nil {
		t.Fatalf("normalizeFilenameHint() error = %v", err)
	}
	if got != "dist/index.html" {
		t.Fatalf("normalized filename = %q, want dist/index.html", got)
	}
}

func TestSafePreflightPathMatchesServerRules(t *testing.T) {
	deep := strings.Repeat("a/", 16) + "index.html"
	long := strings.Repeat("a", 256) + ".html"
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "invalid character", path: "assets/app?.js"},
		{name: "reserved name", path: "CON.txt"},
		{name: "too deep", path: deep},
		{name: "too long", path: long},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if safePreflightPath(tc.path) {
				t.Fatalf("safePreflightPath(%q) = true, want false", tc.path)
			}
		})
	}
	for _, path := range []string{"index.html", "assets/王关飞.css", "a-b_c.d/e.js", "assets/logo@2x.png", "fonts/Inter (1).woff2", "js/app+polyfills.js"} {
		if !safePreflightPath(path) {
			t.Fatalf("safePreflightPath(%q) = false, want true", path)
		}
	}
}

func TestPrepareMultipartSourceRejectsBackslashTraversal(t *testing.T) {
	source := filepath.Join(t.TempDir(), "site")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "index.html"), []byte("<!doctype html><html><body><main>ready</main></body></html>"), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, `evil\..\asset.js`), []byte("alert(1)"), 0o644); err != nil {
		t.Fatalf("write unsafe filename: %v", err)
	}

	if _, err := prepareMultipartSource(source); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("prepareMultipartSource() error = %v, want unsafe path error", err)
	}
}

func TestAnalyzePreflightRejectsOversizedOuterZIP(t *testing.T) {
	source := filepath.Join(t.TempDir(), "large.zip")
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for index := 0; index < 99; index++ {
		header := &zip.FileHeader{Name: fmt.Sprintf("assets/%03d.bin", index), Method: zip.Store}
		part, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create large entry: %v", err)
		}
		if _, err := part.Write(bytes.Repeat([]byte{byte(index)}, 11*1024)); err != nil {
			t.Fatalf("write large entry: %v", err)
		}
	}
	part, err := writer.Create("index.html")
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := part.Write([]byte("<html></html>")); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if int64(buf.Len()) <= bundle.DefaultLimits().MaxSingleFileBytes {
		t.Fatalf("test archive is only %d bytes", buf.Len())
	}
	if err := os.WriteFile(source, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	report := analyzePreflight(source, "")
	if report.Success {
		t.Fatalf("report = %+v; want failure", report)
	}
	if len(report.Errors) != 1 || report.Errors[0].Code != "ZIP_FILE_TOO_LARGE" {
		t.Fatalf("errors = %+v", report.Errors)
	}
}

func TestAnalyzePreflightRejectsDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "site")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(source, "linked.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	report := analyzePreflight(source, "")
	if report.Success {
		t.Fatalf("report = %+v; want failure", report)
	}
	if len(report.Errors) == 0 || report.Errors[0].Code != "SOURCE_PREPARE_FAILED" {
		t.Fatalf("errors = %+v", report.Errors)
	}
}

func writePreflightZIP(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
}
