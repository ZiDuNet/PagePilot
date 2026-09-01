package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/api"
	"github.com/yourorg/hostctl/internal/client"
	appversion "github.com/yourorg/hostctl/internal/version"
)

type doctorClientStub struct {
	health       *client.HealthResponse
	healthErr    error
	config       *api.ConfigResponse
	configErr    error
	adminSession *api.AdminSessionResponse
	adminErr     error
	openAPI      map[string]any
	openAPIErr   error
	tokensErr    error
	tokensCalls  int
}

func (s *doctorClientStub) Health(context.Context) (*client.HealthResponse, error) {
	return s.health, s.healthErr
}

func (s *doctorClientStub) Config(context.Context) (*api.ConfigResponse, error) {
	return s.config, s.configErr
}

func (s *doctorClientStub) AdminSession(context.Context) (*api.AdminSessionResponse, error) {
	return s.adminSession, s.adminErr
}

func (s *doctorClientStub) OpenAPI(context.Context) (map[string]any, error) {
	return s.openAPI, s.openAPIErr
}

func (s *doctorClientStub) ListTokens(context.Context) (*api.TokenListResponse, error) {
	s.tokensCalls++
	if s.tokensErr != nil {
		return nil, s.tokensErr
	}
	return &api.TokenListResponse{Success: true}, nil
}

func readyDoctorStub() *doctorClientStub {
	return &doctorClientStub{
		health:  &client.HealthResponse{Success: true, Status: "ok"},
		config:  &api.ConfigResponse{Success: true, Mode: "prod", Version: "0.3.1", Limits: api.Limits{MaxSingleFileBytes: 1024, MaxSiteTotalBytes: 4096, MaxFilesPerSite: 8}},
		openAPI: map[string]any{"openapi": "3.1.0"},
	}
}

func TestRunDoctorSkipsAnonymousSessionWithoutToken(t *testing.T) {
	stub := readyDoctorStub()
	report := runDoctor(context.Background(), stub, "https://pagepilot.example.com", false, false)

	if !report.Success {
		t.Fatalf("report = %+v; want success", report)
	}
	if report.Mode != "prod" || report.ServerVersion != "0.3.1" {
		t.Fatalf("server metadata = mode %q version %q", report.Mode, report.ServerVersion)
	}
	if report.UploadLimits == nil || report.UploadLimits.MaxFilesPerSite != 8 {
		t.Fatalf("limits = %+v", report.UploadLimits)
	}
	if stub.tokensCalls != 0 {
		t.Fatalf("ListTokens called %d times without a configured token", stub.tokensCalls)
	}
	anonymous := findDoctorCheck(report.Checks, "anonymous_session")
	if anonymous == nil || !anonymous.OK || !anonymous.Skipped {
		t.Fatalf("anonymous session check = %+v; want skipped success", anonymous)
	}
	credential := findDoctorCheck(report.Checks, "credential")
	if credential == nil || !credential.OK || !credential.Skipped {
		t.Fatalf("credential check = %+v; want skipped success", credential)
	}
}

func TestRunDoctorValidatesUserTokenWithoutRequiringAdmin(t *testing.T) {
	stub := readyDoctorStub()
	report := runDoctor(context.Background(), stub, "https://pagepilot.example.com", true, false)

	if !report.Success {
		t.Fatalf("report = %+v; want success", report)
	}
	if stub.tokensCalls != 1 {
		t.Fatalf("ListTokens called %d times, want 1", stub.tokensCalls)
	}
	if findDoctorCheck(report.Checks, "admin_session") != nil {
		t.Fatalf("unexpected admin check: %+v", report.Checks)
	}
}

func TestRunDoctorRequiresAdministratorWhenRequested(t *testing.T) {
	stub := readyDoctorStub()
	stub.adminErr = &client.APIError{
		Status: http.StatusForbidden,
		Body:   &api.APIError{ErrorCode: api.CodeForbidden, Detail: "admin token required"},
	}
	report := runDoctor(context.Background(), stub, "https://pagepilot.example.com", true, true)

	if report.Success {
		t.Fatalf("report = %+v; want failure", report)
	}
	adminCheck := findDoctorCheck(report.Checks, "admin_session")
	if adminCheck == nil || adminCheck.OK || adminCheck.HTTPStatus != http.StatusForbidden || adminCheck.ErrorCode != string(api.CodeForbidden) {
		t.Fatalf("admin check = %+v", adminCheck)
	}
	if len(report.RecommendedSteps) == 0 || !strings.Contains(strings.Join(report.RecommendedSteps, " "), "token") {
		t.Fatalf("recommended steps = %+v", report.RecommendedSteps)
	}
}

func TestRunDoctorReportsConfiguredCredentialFailure(t *testing.T) {
	stub := readyDoctorStub()
	stub.tokensErr = errors.New("connection reset")
	report := runDoctor(context.Background(), stub, "https://pagepilot.example.com", true, false)

	if report.Success {
		t.Fatalf("report = %+v; want failure", report)
	}
	credential := findDoctorCheck(report.Checks, "credential")
	if credential == nil || credential.OK || credential.Detail != "connection reset" {
		t.Fatalf("credential check = %+v", credential)
	}
}

func TestRunDoctorReportsEmptyHealthResponse(t *testing.T) {
	stub := readyDoctorStub()
	stub.health = nil

	report := runDoctor(context.Background(), stub, "https://pagepilot.example.com", false, false)
	if report.Success {
		t.Fatalf("report = %+v; want failure", report)
	}
	health := findDoctorCheck(report.Checks, "health")
	if health == nil || health.OK || health.Detail != "health endpoint returned an empty response" {
		t.Fatalf("health check = %+v", health)
	}
}

func TestRunDoctorReportsEmptyAdminSessionResponse(t *testing.T) {
	stub := readyDoctorStub()
	stub.adminSession = nil
	report := runDoctor(context.Background(), stub, "https://pagepilot.example.com", true, true)
	if report.Success {
		t.Fatalf("report = %+v; want failure", report)
	}
	admin := findDoctorCheck(report.Checks, "admin_session")
	if admin == nil || admin.OK || admin.Detail != "admin session endpoint returned an empty response" {
		t.Fatalf("admin check = %+v", admin)
	}
}

func TestVersionCommandPrintsCurrentRelease(t *testing.T) {
	oldJSON := flagJSON
	flagJSON = false
	defer func() { flagJSON = oldJSON }()

	output := captureStdout(t, func() {
		if err := cmdVersion().RunE(cmdVersion(), nil); err != nil {
			t.Fatalf("version command: %v", err)
		}
	})
	if got, want := strings.TrimSpace(output), "pagep "+appversion.Current; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestSavedTokenIsBoundToConfiguredServer(t *testing.T) {
	cfg := map[string]string{
		"server": "https://one.example/",
		"token":  "secret-token",
	}
	if got := savedTokenForServer(cfg, "https://two.example"); got != "" {
		t.Fatalf("savedTokenForServer() = %q for a different server; want empty", got)
	}
	if got := savedTokenForServer(cfg, "https://one.example"); got != "secret-token" {
		t.Fatalf("savedTokenForServer() = %q for the configured server; want token", got)
	}
	if got := savedTokenForServer(map[string]string{"token": "legacy-token"}, "https://any.example"); got != "legacy-token" {
		t.Fatalf("savedTokenForServer() = %q without a saved server; want legacy token", got)
	}
}

func TestResolvedTokenPrefersExplicitAndEnvironmentValues(t *testing.T) {
	t.Setenv("PAGEPILOT_TOKEN", "environment-token")
	t.Setenv("HOSTCTL_TOKEN", "legacy-environment-token")
	cfg := map[string]string{"server": "https://one.example", "token": "saved-token"}
	if got := resolvedToken(cfg, "https://one.example", "explicit-token"); got != "explicit-token" {
		t.Fatalf("resolvedToken() explicit = %q, want explicit-token", got)
	}
	if got := resolvedToken(cfg, "https://one.example", ""); got != "environment-token" {
		t.Fatalf("resolvedToken() environment = %q, want environment-token", got)
	}
	t.Setenv("PAGEPILOT_TOKEN", "")
	t.Setenv("HOSTCTL_TOKEN", "")
	if got := resolvedToken(cfg, "https://one.example", ""); got != "saved-token" {
		t.Fatalf("resolvedToken() saved = %q, want saved-token", got)
	}
}

func findDoctorCheck(checks []doctorCheck, name string) *doctorCheck {
	for index := range checks {
		if checks[index].Name == name {
			return &checks[index]
		}
	}
	return nil
}
