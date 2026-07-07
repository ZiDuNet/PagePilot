package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/store"
)

func TestPatchVersionAcceptsMultipartOverwrite(t *testing.T) {
	srv, token, cleanup := newDevAuthTestServer(t)
	defer cleanup()
	stub := &versionMultipartOverwriteStub{}
	srv.deployer = stub

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("description", "multipart overwrite"); err != nil {
		t.Fatalf("write description: %v", err)
	}
	if err := writer.WriteField("title", "Multipart 覆盖"); err != nil {
		t.Fatalf("write title: %v", err)
	}
	if err := writer.WriteField("filename", "index.html"); err != nil {
		t.Fatalf("write filename: %v", err)
	}
	part, err := writer.CreateFormFile("file", "index.html")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("<!doctype html><title>multipart overwrite</title>")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/deploys/demo/versions/3", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()

	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s; want %d", rr.Code, rr.Body.String(), http.StatusOK)
	}
	if !stub.called {
		t.Fatal("OverwriteVersion was not called")
	}
	if stub.code != "demo" || stub.version != 3 {
		t.Fatalf("target = %s v%d; want demo v3", stub.code, stub.version)
	}
	if stub.req.Description != "multipart overwrite" || stub.req.Title != "Multipart 覆盖" || stub.req.Filename != "index.html" {
		t.Fatalf("overwrite request metadata = %+v", stub.req)
	}
	if len(stub.req.Files) != 1 {
		t.Fatalf("files = %+v; want one multipart file", stub.req.Files)
	}
	if stub.req.Files[0].Path != "index.html" || !strings.Contains(stub.req.Files[0].Content, "multipart overwrite") {
		t.Fatalf("uploaded file = %+v", stub.req.Files[0])
	}
}


type versionMultipartOverwriteStub struct {
	DeployerPort
	called  bool
	code    string
	version int64
	req     OverwriteRequest
}

func (s *versionMultipartOverwriteStub) OverwriteVersion(
	_ context.Context,
	code string,
	version int64,
	req OverwriteRequest,
) (*DeployResponse, *APIError) {
	s.called = true
	s.code = code
	s.version = version
	s.req = req
	return &DeployResponse{
		Success:       true,
		Code:          code,
		VersionNumber: int(version),
		VersionID:     "version-3",
		URL:           "/agent/demo/",
		DetailURL:     "/market/demo",
		VersionURL:    "/agent/demo/versions/3/",
	}, nil
}

func (s *versionMultipartOverwriteStub) RecordAuditLog(_ context.Context, _ store.AuditLog) error {
	return nil
}
