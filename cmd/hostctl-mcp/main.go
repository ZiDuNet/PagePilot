// hostctl-mcp 是 hostctl 的 MCP server（stdio JSON-RPC 2.0）。
//
// 实现 MCP 协议核心子集（initialization / tools/list / tools/call），
// 不依赖第三方库，便于单二进制部署。
//
// 使用：
//
//	HOSTCTL_SERVER=https://host.example.com HOSTCTL_TOKEN=xxx hostctl-mcp
//
// 在 Claude Code 里配置：
//
//	{
//	  "mcpServers": {
//	    "hostctl": {
//	      "command": "hostctl-mcp",
//	      "env": {
//	        "HOSTCTL_SERVER": "https://host.example.com",
//	        "HOSTCTL_TOKEN": "..."
//	      }
//	    }
//	  }
//	}
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourorg/hostctl/internal/client"
)

// JSON-RPC 类型
type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// MCP 协议响应类型
type toolDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema jsonSchema `json:"inputSchema"`
}

type jsonSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]*schemaProp `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

type schemaProp struct {
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Items       *schemaProp `json:"items,omitempty"`
}

func main() {
	server := os.Getenv("HOSTCTL_SERVER")
	if server == "" {
		server = "http://localhost:8787"
	}
	token := os.Getenv("HOSTCTL_TOKEN")
	c := client.New(server, token)

	reader := bufio.NewReader(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			return
		}
		line = bytesTrimRightSpace(line)
		if len(line) == 0 {
			continue
		}

		var req rpcReq
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(rpcResp{JSONRPC: "2.0", Error: &rpcErr{
				Code: -32700, Message: "Parse error: " + err.Error(),
			}})
			continue
		}

		resp := dispatch(context.Background(), c, &req)
		resp.JSONRPC = "2.0"
		resp.ID = req.ID
		_ = enc.Encode(resp)
	}
}

func bytesTrimRightSpace(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	return b
}

func dispatch(ctx context.Context, c *client.Client, req *rpcReq) rpcResp {
	switch req.Method {
	case "initialize":
		return rpcResp{Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "hostctl",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}}

	case "notifications/initialized":
		// 通知，无响应
		return rpcResp{}

	case "tools/list":
		return rpcResp{Result: map[string]any{"tools": toolList()}}

	case "tools/call":
		return handleToolCall(ctx, c, req.Params)

	default:
		return rpcResp{Error: &rpcErr{
			Code: -32601, Message: "Method not found: " + req.Method,
		}}
	}
}

func toolList() []toolDef {
	return []toolDef{
		{
			Name:        "deploy_site",
			Description: "把本地路径（文件或目录）部署为可访问的静态网站。修改已有项目时必须传原 custom_code；custom_code 已存在会默认追加版本，保持同一短链。description 必填（≤240 字符）。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"source":          {Type: "string", Description: "本地路径。文件 = 单 HTML；目录 = 多文件 site"},
					"description":     {Type: "string", Description: "必填，≤240 字符，说明本次部署内容"},
					"title":           {Type: "string", Description: "可选站点标题"},
					"custom_code":     {Type: "string", Description: "自定义短码，^[a-z0-9-]{3,32}$；留空自动生成"},
					"create_version":  {Type: "boolean", Description: "custom_code 已存在时，true 追加版本；false/省略 报 CONFLICT"},
					"access_password": {Type: "string", Description: "Optional visit password for a new user-owned site. Empty means public."},
				},
				Required: []string{"source", "description"},
			},
		},
		{
			Name:        "set_site_access_password",
			Description: "Set or clear the visit password for a site owned by the current token/user. Pass an empty password to make it public again.",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"code":     {Type: "string", Description: "Site short code"},
					"password": {Type: "string", Description: "Visit password. Empty string clears protection."},
				},
				Required: []string{"code", "password"},
			},
		},
		{
			Name:        "get_site_content",
			Description: "获取 site 的元数据：文件清单、主入口、大小、是否锁定。可指定 version，默认取当前。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"code":    {Type: "string", Description: "site 短码"},
					"version": {Type: "integer", Description: "可选版本号，省略取当前"},
				},
				Required: []string{"code"},
			},
		},
		{
			Name:        "list_versions",
			Description: "列出某 site 的所有版本及其状态、锁定情况。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"code": {Type: "string"},
				},
				Required: []string{"code"},
			},
		},
		{
			Name:        "lock_version",
			Description: "锁定（locked=true）或解锁（locked=false）某版本。锁定的版本不可被覆盖或删除。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"code":    {Type: "string"},
					"version": {Type: "integer"},
					"locked":  {Type: "boolean"},
				},
				Required: []string{"code", "version", "locked"},
			},
		},
		{
			Name:        "set_current_version",
			Description: "切换 site 对外服务的版本。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"code":    {Type: "string"},
					"version": {Type: "integer"},
				},
				Required: []string{"code", "version"},
			},
		},
		{
			Name:        "delete_version",
			Description: "删除版本。锁定版本拒绝删除。先解锁或换一个版本。删除当前版本会自动切到上一个 active 版本。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"code":    {Type: "string"},
					"version": {Type: "integer"},
				},
				Required: []string{"code", "version"},
			},
		},
		{
			Name:        "search_marketplace",
			Description: "在公开应用市场搜索 / 浏览已上线的应用。支持关键词、排序、分页。无需 token。用于发现现有项目、找热门、避免重复创建。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"q":         {Type: "string", Description: "可选关键词（标题 / 文件名 / code）"},
					"sort":      {Type: "string", Description: "排序：newest(默认) / oldest / likes_desc / views_desc"},
					"page":      {Type: "integer", Description: "页码，默认 1"},
					"page_size": {Type: "integer", Description: "每页数量，默认 12，最大 50"},
				},
			},
		},
		{
			Name:        "get_deploy_detail",
			Description: "读取单个应用的详情（公开市场信息）。public_id 是 UUID（32 字符）或 code 都可以。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"public_id": {Type: "string", Description: "UUID 或短码"},
				},
				Required: []string{"public_id"},
			},
		},
		{
			Name:        "like_deploy",
			Description: "点赞某应用。按用户 / token / 指纹去重，只影响市场排序，不授予写权限。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"code": {Type: "string"},
				},
				Required: []string{"code"},
			},
		},
		{
			Name:        "set_primary_strategy",
			Description: "切换主域名策略：likes（默认，最高赞版本对外）/ latest（最新版本对外，适合日更项目）。更改 /agent/{code} 访问时返回哪个版本。",
			InputSchema: jsonSchema{
				Type: "object",
				Properties: map[string]*schemaProp{
					"code":     {Type: "string"},
					"strategy": {Type: "string", Description: "likes 或 latest"},
				},
				Required: []string{"code", "strategy"},
			},
		},
	}
}

type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

func handleToolCall(ctx context.Context, c *client.Client, params json.RawMessage) rpcResp {
	var p callParams
	if err := json.Unmarshal(params, &p); err != nil {
		return rpcResp{Error: &rpcErr{Code: -32602, Message: "invalid params: " + err.Error()}}
	}

	var (
		resultText string
		callErr    error
	)

	switch p.Name {
	case "deploy_site":
		resultText, callErr = toolDeploySite(ctx, c, p.Arguments)
	case "set_site_access_password":
		resultText, callErr = toolSetSiteAccessPassword(ctx, c, p.Arguments)
	case "get_site_content":
		resultText, callErr = toolGetContent(ctx, c, p.Arguments)
	case "list_versions":
		resultText, callErr = toolListVersions(ctx, c, p.Arguments)
	case "lock_version":
		resultText, callErr = toolLockVersion(ctx, c, p.Arguments)
	case "set_current_version":
		resultText, callErr = toolSetCurrent(ctx, c, p.Arguments)
	case "delete_version":
		resultText, callErr = toolDeleteVersion(ctx, c, p.Arguments)
	case "search_marketplace":
		resultText, callErr = toolSearchMarketplace(ctx, c, p.Arguments)
	case "get_deploy_detail":
		resultText, callErr = toolGetDeployDetail(ctx, c, p.Arguments)
	case "like_deploy":
		resultText, callErr = toolLikeDeploy(ctx, c, p.Arguments)
	case "set_primary_strategy":
		resultText, callErr = toolSetPrimaryStrategy(ctx, c, p.Arguments)
	default:
		return rpcResp{Error: &rpcErr{Code: -32601, Message: "unknown tool: " + p.Name}}
	}

	if callErr != nil {
		return rpcResp{Result: callResult{
			Content: []textContent{{Type: "text", Text: "ERROR: " + callErr.Error()}},
			IsError: true,
		}}
	}
	return rpcResp{Result: callResult{
		Content: []textContent{{Type: "text", Text: resultText}},
	}}
}

func toolDeploySite(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	source, _ := args["source"].(string)
	desc, _ := args["description"].(string)
	title, _ := args["title"].(string)
	customCode, _ := args["custom_code"].(string)
	accessPassword, _ := args["access_password"].(string)
	createVersion, _ := args["create_version"].(bool)

	if source == "" {
		return "", fmt.Errorf("source is required")
	}
	if desc == "" {
		return "", fmt.Errorf("description is required")
	}

	files, err := readSourceDir(source)
	if err != nil {
		return "", fmt.Errorf("read source: %w", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no files found at %s", source)
	}

	// 转成 DeployFile
	df := make([]deployFileT, 0, len(files))
	for _, f := range files {
		if f.IsText {
			df = append(df, deployFileT{Path: f.Path, Content: string(f.Data)})
		} else {
			df = append(df, deployFileT{Path: f.Path, ContentBase64: base64.StdEncoding.EncodeToString(f.Data)})
		}
	}

	// 构造请求 JSON
	reqBody := map[string]any{
		"description": desc,
		"files":       df,
	}
	if title != "" {
		reqBody["title"] = title
	}
	if accessPassword != "" {
		reqBody["accessPassword"] = accessPassword
	}
	if customCode != "" {
		reqBody["enableCustomCode"] = true
		reqBody["customCode"] = customCode
		reqBody["createVersion"] = true
		if createVersion {
			reqBody["createVersion"] = true
		}
	}
	reqJSON, _ := json.Marshal(reqBody)

	resp, err := c.RawDeploy(ctx, reqJSON)
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	url, _ := resp["url"].(string)
	if url != "" {
		return fmt.Sprintf("部署成功！访问 URL: %s\n\n%s", url, string(pretty)), nil
	}
	return string(pretty), nil
}

func toolSetSiteAccessPassword(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	code, _ := args["code"].(string)
	password, ok := args["password"].(string)
	if code == "" {
		return "", fmt.Errorf("code is required")
	}
	if !ok {
		return "", fmt.Errorf("password is required; pass an empty string to clear it")
	}
	resp, err := c.SetSiteAccessPassword(ctx, code, password)
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

// deployFileT 是 DeployFile 的本地等价（避免 import api 包的循环依赖）。
type deployFileT struct {
	Path          string `json:"path"`
	Content       string `json:"content,omitempty"`
	ContentBase64 string `json:"contentBase64,omitempty"`
}

type fileChunk struct {
	Path   string
	Data   []byte
	IsText bool
}

// readSourceDir 把 source 解析成 fileChunk 列表。
//   - source 是单文件：返回单个 chunk
//   - source 是目录：递归读取
func readSourceDir(source string) ([]fileChunk, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, err
		}
		base := filepath.Base(source)
		return []fileChunk{{Path: base, Data: data, IsText: looksText(data)}}, nil
	}

	var out []fileChunk
	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, fileChunk{Path: rel, Data: data, IsText: looksText(data)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func looksText(b []byte) bool {
	n := len(b)
	if n > 1024 {
		n = 1024
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return false
		}
	}
	return true
}

func toolGetContent(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	code, _ := args["code"].(string)
	if code == "" {
		return "", fmt.Errorf("code is required")
	}
	var vPtr *int64
	if vi, ok := args["version"].(float64); ok && vi > 0 {
		v := int64(vi)
		vPtr = &v
	}
	resp, err := c.GetContent(ctx, code, vPtr)
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

func toolListVersions(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	code, _ := args["code"].(string)
	if code == "" {
		return "", fmt.Errorf("code is required")
	}
	resp, err := c.ListVersions(ctx, code)
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

func toolLockVersion(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	code, _ := args["code"].(string)
	vi, _ := args["version"].(float64)
	locked, _ := args["locked"].(bool)
	if code == "" || vi <= 0 {
		return "", fmt.Errorf("code and version are required")
	}
	resp, err := c.Lock(ctx, code, int64(vi), locked)
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

func toolSetCurrent(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	code, _ := args["code"].(string)
	vi, _ := args["version"].(float64)
	if code == "" || vi <= 0 {
		return "", fmt.Errorf("code and version are required")
	}
	resp, err := c.SetCurrent(ctx, code, int64(vi))
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

func toolDeleteVersion(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	code, _ := args["code"].(string)
	vi, _ := args["version"].(float64)
	if code == "" || vi <= 0 {
		return "", fmt.Errorf("code and version are required")
	}
	resp, err := c.DeleteVersion(ctx, code, int64(vi))
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

// toolSearchMarketplace 调用公开应用市场，让 Agent 在部署前先看看是否已有同类应用，
// 避免重复造轮子。无需 token。
func toolSearchMarketplace(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	q, _ := args["q"].(string)
	sort, _ := args["sort"].(string)
	page := toInt64(args["page"])
	pageSize := toInt64(args["page_size"])
	resp, err := c.SearchMarketplace(ctx, q, sort, int(page), int(pageSize))
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

// toolGetDeployDetail 读取单个应用的公开市场信息。
// public_id 可以是 32 字符 UUID 或 site 短码。
func toolGetDeployDetail(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	pid, _ := args["public_id"].(string)
	if pid == "" {
		return "", fmt.Errorf("public_id is required")
	}
	resp, err := c.GetDeployDetail(ctx, pid)
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

// toolLikeDeploy 给公开应用点赞。点赞只影响市场排序。
// 同一指纹（IP+UA）幂等，不会重复计数。
func toolLikeDeploy(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	code, _ := args["code"].(string)
	if code == "" {
		return "", fmt.Errorf("code is required")
	}
	resp, err := c.LikeDeploy(ctx, code)
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

// toolSetPrimaryStrategy 切换某 site 的主版本策略。
// strategy: "likes"（默认，对外暴露最高赞版本） / "latest"（对外暴露最新版本，适合日更项目）。
func toolSetPrimaryStrategy(ctx context.Context, c *client.Client, args map[string]any) (string, error) {
	code, _ := args["code"].(string)
	strategy, _ := args["strategy"].(string)
	if code == "" || strategy == "" {
		return "", fmt.Errorf("code and strategy are required")
	}
	if strategy != "likes" && strategy != "latest" {
		return "", fmt.Errorf("strategy must be 'likes' or 'latest'")
	}
	resp, err := c.SetPrimaryStrategy(ctx, code, strategy)
	if err != nil {
		return "", err
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	return string(pretty), nil
}

// toInt64 兼容 JSON 数字解析为 float64 的常见坑。
func toInt64(v any) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return 0
}

// _unused 让 strings import 不被自动移除（部分预留）。
var _ = strings.TrimSpace
