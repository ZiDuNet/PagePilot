package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/hostctl/internal/client"
	appversion "github.com/yourorg/hostctl/internal/version"
)

func TestInitializeReportsSharedReleaseVersion(t *testing.T) {
	response := dispatch(context.Background(), nil, &rpcReq{Method: "initialize"})
	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("initialize result = %#v; want object", response.Result)
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("serverInfo = %#v; want object", result["serverInfo"])
	}
	if got, want := serverInfo["version"], appversion.Current; got != want {
		t.Fatalf("serverInfo.version = %#v; want %q", got, want)
	}
}

func TestIsNotificationOnlyTreatsMissingIDAsNotification(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "missing", want: true},
		{name: "null", id: "null", want: false},
		{name: "zero", id: "0", want: false},
		{name: "false", id: "false", want: false},
		{name: "empty string", id: `""`, want: false},
		{name: "string", id: `"request-1"`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := rpcReq{}
			if tt.id != "" {
				req.ID = json.RawMessage(tt.id)
			}
			if got := isNotification(req); got != tt.want {
				t.Fatalf("isNotification(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestToolListIncludesAdminRuntimeTools(t *testing.T) {
	tools := map[string]toolDef{}
	for _, tool := range toolList() {
		tools[tool.Name] = tool
	}
	for _, name := range []string{
		"deploy_site",
		"set_site_reuse_policy",
		"set_site_security_mode",
		"get_admin_site_detail",
		"query_audit_logs",
		"get_site_content",
		"list_screens",
		"publish_screen",
		"request_screen_screenshot",
		"send_screen_command",
	} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("tool %s is missing from MCP tool list", name)
		}
	}

	reuseTool := tools["set_site_reuse_policy"]
	if !strings.Contains(reuseTool.Description, "加密站点") {
		t.Fatalf("reuse policy description = %q; want encrypted-site source restriction", reuseTool.Description)
	}
	assertRequired(t, reuseTool, "code", "reuse_policy", "source_download_policy")
	assertEnum(t, reuseTool, "reuse_policy", "auto", "allow", "deny")
	assertEnum(t, reuseTool, "source_download_policy", "auto", "allow", "deny")

	securityTool := tools["set_site_security_mode"]
	assertRequired(t, securityTool, "code", "security_mode")
	assertEnum(t, securityTool, "security_mode", "auto", "strict", "compatible", "trusted")

	detailTool := tools["get_admin_site_detail"]
	if !strings.Contains(detailTool.Description, "Bundle") ||
		!strings.Contains(detailTool.Description, "文件树") ||
		!strings.Contains(detailTool.Description, "复用参数") {
		t.Fatalf("admin detail description = %q; want bundle/file/reuse wording", detailTool.Description)
	}

	auditTool := tools["query_audit_logs"]
	for _, prop := range []string{"actor_type", "actor_id", "actor_role", "action", "result", "site_code", "target_type", "target_id", "q", "since", "until", "page", "page_size"} {
		if auditTool.InputSchema.Properties[prop] == nil {
			t.Fatalf("query_audit_logs property %s is missing", prop)
		}
	}
}

func TestToolCallRejectsInvalidPolicyArgumentsBeforeNetwork(t *testing.T) {
	c := client.New("http://127.0.0.1:1", "token")

	reuseResp := handleToolCall(context.Background(), c, rawCallParams(t, callParams{
		Name: "set_site_reuse_policy",
		Arguments: map[string]any{
			"code":                   "demo",
			"reuse_policy":           "public",
			"source_download_policy": "allow",
		},
	}))
	assertToolError(t, reuseResp, "reuse_policy must be auto, allow, or deny")

	securityResp := handleToolCall(context.Background(), c, rawCallParams(t, callParams{
		Name: "set_site_security_mode",
		Arguments: map[string]any{
			"code":          "demo",
			"security_mode": "unsafe",
		},
	}))
	assertToolError(t, securityResp, "security_mode must be auto, strict, compatible, or trusted")
}

func TestLocalSourceRejectsSymlinksAndUnsafePaths(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "site")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "index.html"), []byte("<html><body><main>ok</main></body></html>"), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	linked := filepath.Join(source, "linked.txt")
	if err := os.Symlink(outside, linked); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := readSourceDir(source); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("readSourceDir() error = %v, want symbolic-link rejection", err)
	}
	if _, err := prepareMultipartSource(source); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("prepareMultipartSource() error = %v, want symbolic-link rejection", err)
	}

	unsafeDir := filepath.Join(root, "unsafe")
	if err := os.Mkdir(unsafeDir, 0o755); err != nil {
		t.Fatalf("mkdir unsafe source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unsafeDir, "bad?.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write unsafe filename: %v", err)
	}
	if _, err := prepareMultipartSource(unsafeDir); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("prepareMultipartSource() error = %v, want unsafe-path rejection", err)
	}
}

func assertRequired(t *testing.T, tool toolDef, names ...string) {
	t.Helper()
	got := map[string]bool{}
	for _, name := range tool.InputSchema.Required {
		got[name] = true
	}
	for _, name := range names {
		if !got[name] {
			t.Fatalf("%s required fields = %v; want %s", tool.Name, tool.InputSchema.Required, name)
		}
	}
}

func assertEnum(t *testing.T, tool toolDef, prop string, values ...string) {
	t.Helper()
	schema := tool.InputSchema.Properties[prop]
	if schema == nil {
		t.Fatalf("%s property %s is missing", tool.Name, prop)
	}
	got := strings.Join(schema.Enum, ",")
	want := strings.Join(values, ",")
	if got != want {
		t.Fatalf("%s.%s enum = %q; want %q", tool.Name, prop, got, want)
	}
}

func rawCallParams(t *testing.T, params callParams) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal call params: %v", err)
	}
	return raw
}

func assertToolError(t *testing.T, resp rpcResp, want string) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}
	result, ok := resp.Result.(callResult)
	if !ok {
		t.Fatalf("result = %#v; want callResult", resp.Result)
	}
	if !result.IsError {
		t.Fatalf("IsError = false; want true")
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, want) {
		t.Fatalf("content = %+v; want error containing %q", result.Content, want)
	}
}
