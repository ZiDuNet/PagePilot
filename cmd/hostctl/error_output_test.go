package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/client"
)

func TestPrintErrJSONIncludesStableGenericErrorShape(t *testing.T) {
	oldJSON := flagJSON
	flagJSON = true
	defer func() { flagJSON = oldJSON }()

	output := captureStdout(t, func() {
		printErr(errors.New("--description is required"))
	})
	var payload cliErrorPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("JSON error output = %q: %v", output, err)
	}
	if payload.Success {
		t.Fatal("generic error success = true, want false")
	}
	if payload.ErrorCode != "CLI_ERROR" {
		t.Fatalf("generic error code = %q, want CLI_ERROR", payload.ErrorCode)
	}
	if payload.Detail != "--description is required" {
		t.Fatalf("generic detail = %q", payload.Detail)
	}
}

func TestPrintErrJSONPreservesAPIErrorContext(t *testing.T) {
	oldJSON := flagJSON
	flagJSON = true
	defer func() { flagJSON = oldJSON }()

	retryAfter := 12
	err := fmtWrappedAPIError(&client.APIError{
		Status: http.StatusTooManyRequests,
		Body: &api.APIError{
			ErrorCode:         api.CodeRateLimited,
			Stage:             "cooldown",
			Detail:            "global deploy cooldown active",
			Hint:              "Wait before retrying.",
			RetryAfterSeconds: &retryAfter,
			RequestID:         "req-json-1",
		},
	})

	output := captureStdout(t, func() { printErr(err) })
	var payload cliErrorPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("JSON API error output = %q: %v", output, err)
	}
	if payload.Success || payload.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("API error status = %+v", payload)
	}
	if payload.ErrorCode != string(api.CodeRateLimited) || payload.Stage != "cooldown" {
		t.Fatalf("API error identity = %+v", payload)
	}
	if payload.Detail != "global deploy cooldown active" || payload.Hint != "Wait before retrying." {
		t.Fatalf("API error context = %+v", payload)
	}
	if payload.RetryAfterSeconds == nil || *payload.RetryAfterSeconds != retryAfter || payload.RequestID != "req-json-1" {
		t.Fatalf("API error retry/request = %+v", payload)
	}
}

func TestPrintErrHumanOutputIncludesRequestID(t *testing.T) {
	oldJSON := flagJSON
	flagJSON = false
	defer func() { flagJSON = oldJSON }()

	output := captureStderr(t, func() {
		printErr(&client.APIError{
			Status: http.StatusBadRequest,
			Body: &api.APIError{
				ErrorCode: api.CodeInvalidInput,
				Detail:    "invalid input",
				RequestID: "req-human-1",
			},
		})
	})
	if !strings.Contains(output, "Error: INVALID_INPUT") || !strings.Contains(output, "request: req-human-1") {
		t.Fatalf("human error output = %q", output)
	}
}

func TestGetDownloadJSONReportsWrittenFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/deploy/content" || r.URL.Query().Get("code") != "demo" || r.URL.Query().Get("download") != "1" {
			t.Fatalf("request = %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("zip-data"))
	}))
	defer server.Close()

	oldServer, oldToken, oldJSON := flagServer, flagToken, flagJSON
	flagServer, flagToken, flagJSON = server.URL, "", true
	defer func() {
		flagServer, flagToken, flagJSON = oldServer, oldToken, oldJSON
	}()

	outputDir := t.TempDir()
	command := cmdGet()
	command.SetArgs([]string{"demo", "--download", "--output", outputDir})
	output := captureStdout(t, func() {
		if err := command.Execute(); err != nil {
			t.Fatalf("get download: %v", err)
		}
	})
	var payload struct {
		Success     bool   `json:"success"`
		Code        string `json:"code"`
		Output      string `json:"output"`
		ContentType string `json:"contentType"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("JSON output = %q: %v", output, err)
	}
	if !payload.Success || payload.Code != "demo" || !strings.Contains(payload.ContentType, "zip") {
		t.Fatalf("payload = %+v", payload)
	}
	if want := filepath.Join(outputDir, "demo.zip"); payload.Output != want {
		t.Fatalf("output = %q, want %q", payload.Output, want)
	}
	data, err := os.ReadFile(payload.Output)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != "zip-data" {
		t.Fatalf("downloaded data = %q", data)
	}
}

func fmtWrappedAPIError(err *client.APIError) error {
	return fmt.Errorf("request failed: %w", err)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = writer
	defer func() { os.Stderr = old }()

	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return buf.String()
}
