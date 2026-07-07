package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintAuditLogListIncludesUserAgentAndDetailJSON(t *testing.T) {
	resp := map[string]any{
		"total":    float64(1),
		"page":     float64(1),
		"pageSize": float64(20),
		"logs": []any{
			map[string]any{
				"id":         float64(7),
				"createdAt":  "2026-07-07T08:30:00Z",
				"action":     "site.access_login",
				"result":     "failed",
				"actorType":  "browser",
				"actorId":    "browser",
				"actorRole":  "public",
				"siteCode":   "secret-site",
				"targetType": "site",
				"targetId":   "secret-site",
				"ip":         "203.0.113.10",
				"userAgent":  "Mozilla/5.0 PagePilot-QA",
				"detail": map[string]any{
					"versionNumber": float64(3),
					"reason":        "invalid_password",
				},
			},
		},
	}

	output := captureStdout(t, func() {
		printAuditLogList(resp)
	})

	for _, want := range []string{
		"User-Agent",
		"Mozilla/5.0 PagePilot-QA",
		"Detail",
		"versionNumber",
		"invalid_password",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("audit output missing %q:\n%s", want, output)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = old }()

	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return buf.String()
}
