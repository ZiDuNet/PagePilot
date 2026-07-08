// Package client 是 hostctl HTTP API 的 Go 客户端。
// CLI 和 MCP 都基于此封装。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/hostctl/internal/api"
)

const currentOriginHeader = "X-Hostctl-Current-Origin"

// Client 是 hostctl API 客户端。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New 构造客户端。token 可空（用于 dev 模式）。
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		// P15：30s 整体超时，避免服务器无响应时进程永久挂起
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError 是服务端返回的标准化错误。
type APIError struct {
	Status int
	Body   *api.APIError
}

func (e *APIError) Error() string {
	if e.Body != nil {
		return fmt.Sprintf("[%d] %s: %s", e.Status, e.Body.ErrorCode, e.Body.Detail)
	}
	return fmt.Sprintf("http %d", e.Status)
}

// IsCode 判断错误码。
func (e *APIError) IsCode(code api.ErrorCode) bool {
	return e.Body != nil && e.Body.ErrorCode == code
}

// do 发请求；如果响应 4xx/5xx，解析 JSON 错误体并返回 *APIError。
func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.baseURL != "" {
		req.Header.Set(currentOriginHeader, c.baseURL)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		// P17：先尝试解析为 JSON，失败时把响应体原文存入 Detail（最多 500 字节）
		var apiErr api.APIError
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr != nil {
			apiErr = api.APIError{
				ErrorCode: "HTTP_ERROR",
				Detail:     fmt.Sprintf("non-JSON response (status %d): %s", resp.StatusCode, truncateString(string(body), 500)),
			}
		}
		return resp, &APIError{Status: resp.StatusCode, Body: &apiErr}
	}
	return resp, nil
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// doGet 发 GET 请求并把响应解码到 v。
func (c *Client) doGet(ctx context.Context, path string, v any) error {
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// doPost 发 POST 请求并把响应解码到 v。
func (c *Client) doPost(ctx context.Context, path string, body, v any) error {
	resp, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// doPatch / doDelete 类似。
func (c *Client) doPatch(ctx context.Context, path string, body, v any) error {
	resp, err := c.do(ctx, http.MethodPatch, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *Client) doDelete(ctx context.Context, path string, v any) error {
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// ===== API 方法 =====

// Deploy 调用 POST /api/deploy。
func (c *Client) Deploy(ctx context.Context, req api.DeployRequest) (*api.DeployResponse, error) {
	var resp api.DeployResponse
	if err := c.doPost(ctx, "/api/deploy", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type MultipartDeployRequest struct {
	SourcePath            string
	UploadName            string
	Filename              string
	Description           string
	Title                 string
	CustomCode            string
	CreateVersion         bool
	Visibility            string
	Category              string
	Tags                  []string
	AccessPassword        string
	Source                string
	TemplateSourceCode    string
	TemplateSourceVersion int64
	EnableCustomCode      bool
}

func (c *Client) DeployMultipart(ctx context.Context, req MultipartDeployRequest) (*api.DeployResponse, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	errCh := make(chan error, 1)
	go func() {
		errCh <- writeMultipartDeploy(writer, pw, req)
	}()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/deploy", pr)
	if err != nil {
		_ = pr.Close()
		_ = <-errCh
		return nil, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if c.baseURL != "" {
		httpReq.Header.Set(currentOriginHeader, c.baseURL)
	}
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		_ = pr.Close()
		if writeErr := <-errCh; writeErr != nil {
			return nil, writeErr
		}
		return nil, err
	}
	defer resp.Body.Close()
	if writeErr := <-errCh; writeErr != nil {
		return nil, writeErr
	}
	if resp.StatusCode >= 400 {
		var apiErr api.APIError
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return nil, &APIError{Status: resp.StatusCode, Body: &apiErr}
	}
	var out api.DeployResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

type MultipartOverwriteRequest struct {
	SourcePath  string
	UploadName  string
	Filename    string
	Description string
	Title       string
}

func (c *Client) OverwriteMultipart(ctx context.Context, code string, version int64, req MultipartOverwriteRequest) (*api.DeployResponse, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	errCh := make(chan error, 1)
	go func() {
		errCh <- writeMultipartDeploy(writer, pw, MultipartDeployRequest{
			SourcePath:  req.SourcePath,
			UploadName:  req.UploadName,
			Filename:    req.Filename,
			Description: req.Description,
			Title:       req.Title,
			Source:      "cli",
		})
	}()

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		fmt.Sprintf("%s/api/deploys/%s/versions/%d", c.baseURL, url.PathEscape(code), version),
		pr,
	)
	if err != nil {
		_ = pr.Close()
		_ = <-errCh
		return nil, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if c.baseURL != "" {
		httpReq.Header.Set(currentOriginHeader, c.baseURL)
	}
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		_ = pr.Close()
		if writeErr := <-errCh; writeErr != nil {
			return nil, writeErr
		}
		return nil, err
	}
	defer resp.Body.Close()
	if writeErr := <-errCh; writeErr != nil {
		return nil, writeErr
	}
	if resp.StatusCode >= 400 {
		var apiErr api.APIError
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return nil, &APIError{Status: resp.StatusCode, Body: &apiErr}
	}
	var out api.DeployResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

func writeMultipartDeploy(writer *multipart.Writer, pw *io.PipeWriter, req MultipartDeployRequest) error {
	fail := func(format string, args ...any) error {
		err := fmt.Errorf(format, args...)
		_ = pw.CloseWithError(err)
		return err
	}
	fields := map[string]string{
		"description":    req.Description,
		"title":          req.Title,
		"filename":       req.Filename,
		"visibility":     req.Visibility,
		"category":       req.Category,
		"accessPassword": req.AccessPassword,
		"source":         req.Source,
	}
	if req.CustomCode != "" {
		fields["customCode"] = req.CustomCode
		fields["enableCustomCode"] = "true"
	}
	if req.EnableCustomCode {
		fields["enableCustomCode"] = "true"
	}
	if req.CreateVersion {
		fields["createVersion"] = "true"
	}
	if len(req.Tags) > 0 {
		fields["tags"] = strings.Join(req.Tags, ",")
	}
	if strings.TrimSpace(req.TemplateSourceCode) != "" {
		fields["templateSourceCode"] = strings.TrimSpace(req.TemplateSourceCode)
	}
	if req.TemplateSourceVersion > 0 {
		fields["templateSourceVersion"] = strconv.FormatInt(req.TemplateSourceVersion, 10)
	}
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return fail("write multipart field %s: %w", key, err)
		}
	}
	uploadName := req.UploadName
	if uploadName == "" {
		uploadName = req.Filename
	}
	if err := addMultipartFile(writer, req.SourcePath, uploadName); err != nil {
		_ = pw.CloseWithError(err)
		return err
	}
	if err := writer.Close(); err != nil {
		return fail("close multipart writer: %w", err)
	}
	return pw.Close()
}

func addMultipartFile(writer *multipart.Writer, sourcePath, filename string) error {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return fmt.Errorf("source path is required")
	}
	if filename == "" {
		filename = filepath.Base(sourcePath)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("create multipart file: %w", err)
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", sourcePath, err)
	}
	defer file.Close()
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copy %s: %w", sourcePath, err)
	}
	return nil
}

// RawDeploy 是 Deploy 的等价物，但接受原始 JSON 字节，
// 便于调用方按 map 构造请求体（用于 MCP server 等场景）。
func (c *Client) RawDeploy(ctx context.Context, body []byte) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/deploy", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.baseURL != "" {
		req.Header.Set(currentOriginHeader, c.baseURL)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return out, &APIError{Status: resp.StatusCode, Body: parseAPIError(out)}
	}
	return out, nil
}

// parseAPIError 从 RawDeploy 等返回的 map 里抽出 api.APIError。
func parseAPIError(m map[string]any) *api.APIError {
	if m == nil {
		return nil
	}
	out := &api.APIError{}
	if v, ok := m["errorCode"].(string); ok {
		out.ErrorCode = api.ErrorCode(v)
	}
	if v, ok := m["stage"].(string); ok {
		out.Stage = v
	}
	if v, ok := m["detail"].(string); ok {
		out.Detail = v
	}
	if v, ok := m["hint"].(string); ok {
		out.Hint = v
	}
	return out
}

// SearchMarketplace 调用 GET /api/deploys —— 公开创作市场搜索。
// q 可空；sort 取 newest/oldest/likes_desc/views_desc；page / pageSize 默认值由服务端兜底。
func (c *Client) SearchMarketplace(ctx context.Context, q, sort string, page, pageSize int) (map[string]any, error) {
	return c.SearchMarketplaceWithFilters(ctx, q, sort, "", "", page, pageSize)
}

func (c *Client) SearchMarketplaceWithFilters(ctx context.Context, q, sort, category, kind string, page, pageSize int) (map[string]any, error) {
	qs := url.Values{}
	if q != "" {
		qs.Set("q", q)
	}
	if sort != "" {
		qs.Set("sort", sort)
	}
	if category != "" {
		qs.Set("category", category)
	}
	if kind != "" {
		qs.Set("kind", kind)
	}
	if page > 0 {
		qs.Set("page", fmt.Sprintf("%d", page))
	}
	if pageSize > 0 {
		qs.Set("pageSize", fmt.Sprintf("%d", pageSize))
	}
	path := "/api/deploys"
	if enc := qs.Encode(); enc != "" {
		path += "?" + enc
	}
	var out map[string]any
	if err := c.doGet(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) MarketCategories(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.doGet(ctx, "/api/market/categories", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetDeployDetail 调用 GET /api/deploys/{publicId} —— 单条应用详情。
// publicId 既支持 32 字符 UUID，也支持短码 code。
func (c *Client) GetDeployDetail(ctx context.Context, publicID string) (map[string]any, error) {
	var out map[string]any
	if err := c.doGet(ctx, "/api/deploys/"+url.PathEscape(publicID), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LikeDeploy 调用 POST /api/deploys/{code}/like。
// 点赞按用户 / token / 指纹去重，只影响市场排序，不授予写权限。
func (c *Client) LikeDeploy(ctx context.Context, code string) (map[string]any, error) {
	var out map[string]any
	if err := c.doPost(ctx, "/api/deploys/"+url.PathEscape(code)+"/like", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetPrimaryStrategy 调用 PATCH /api/deploys/{code}/primary-strategy。
// strategy: "likes" 或 "latest"。决定 /agent/{code} 对外暴露的版本。
func (c *Client) SetPrimaryStrategy(ctx context.Context, code, strategy string) (*api.PrimaryStrategyResponse, error) {
	var resp api.PrimaryStrategyResponse
	if err := c.doPatch(ctx,
		"/api/deploys/"+url.PathEscape(code)+"/primary-strategy",
		api.PrimaryStrategyRequest{PrimaryVersionStrategy: api.PrimaryVersionStrategy(strategy)},
		&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SetSiteAccessPassword sets or clears a site's visit password.
func (c *Client) SetSiteAccessPassword(ctx context.Context, code, password string) (map[string]any, error) {
	var resp map[string]any
	if err := c.doPatch(ctx,
		"/api/deploys/"+url.PathEscape(code)+"/access",
		map[string]any{"password": password}, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// SetSitePin 设置或取消管理员创作市场置顶。
func (c *Client) SetSitePin(ctx context.Context, code string, pinned bool) (map[string]any, error) {
	var resp map[string]any
	if err := c.doPatch(ctx,
		"/api/admin/sites/"+url.PathEscape(code)+"/pin",
		map[string]any{"pinned": pinned}, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// SetSiteReusePolicy 设置源码下载和模板复用策略。
func (c *Client) SetSiteReusePolicy(ctx context.Context, code, reusePolicy, sourceDownloadPolicy string) (map[string]any, error) {
	var resp map[string]any
	if err := c.doPatch(ctx,
		"/api/admin/sites/"+url.PathEscape(code)+"/reuse-policy",
		map[string]any{
			"reusePolicy":          reusePolicy,
			"sourceDownloadPolicy": sourceDownloadPolicy,
		}, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// SetSiteSecurityMode 设置站点级安全模式。
func (c *Client) SetSiteSecurityMode(ctx context.Context, code, securityMode string) (map[string]any, error) {
	var resp map[string]any
	if err := c.doPatch(ctx,
		"/api/admin/sites/"+url.PathEscape(code)+"/security-mode",
		map[string]any{"securityMode": securityMode}, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// AdminSiteDetail 读取后台站点详情，包含 Bundle、文件树、版本和复用参数。
func (c *Client) AdminSiteDetail(ctx context.Context, code string) (map[string]any, error) {
	var out map[string]any
	if err := c.doGet(ctx, "/api/admin/sites/"+url.PathEscape(code), &out); err != nil {
		return nil, err
	}
	return out, nil
}

type AuditLogQuery struct {
	ActorType  string
	ActorID    string
	ActorRole  string
	Action     string
	Result     string
	SiteCode   string
	TargetType string
	TargetID   string
	Query      string
	Since      string
	Until      string
	Page       int
	PageSize   int
}

// ListAuditLogs 查询管理员审计日志。
func (c *Client) ListAuditLogs(ctx context.Context, query AuditLogQuery) (map[string]any, error) {
	qs := url.Values{}
	setIfNotEmpty := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			qs.Set(key, strings.TrimSpace(value))
		}
	}
	setIfNotEmpty("actorType", query.ActorType)
	setIfNotEmpty("actorId", query.ActorID)
	setIfNotEmpty("actorRole", query.ActorRole)
	setIfNotEmpty("action", query.Action)
	setIfNotEmpty("result", query.Result)
	setIfNotEmpty("siteCode", query.SiteCode)
	setIfNotEmpty("targetType", query.TargetType)
	setIfNotEmpty("targetId", query.TargetID)
	setIfNotEmpty("q", query.Query)
	setIfNotEmpty("since", query.Since)
	setIfNotEmpty("until", query.Until)
	if query.Page > 0 {
		qs.Set("page", fmt.Sprintf("%d", query.Page))
	}
	if query.PageSize > 0 {
		qs.Set("pageSize", fmt.Sprintf("%d", query.PageSize))
	}
	path := "/api/admin/audit-logs"
	if enc := qs.Encode(); enc != "" {
		path += "?" + enc
	}
	var out map[string]any
	if err := c.doGet(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListVersions 调用 GET /api/deploys/{code}/versions。
func (c *Client) ListVersions(ctx context.Context, code string) (*api.ListVersionsResponse, error) {
	var resp api.ListVersionsResponse
	if err := c.doGet(ctx, "/api/deploys/"+url.PathEscape(code)+"/versions", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Lock 调用 POST /api/deploys/{code}/versions/{version}/lock。
func (c *Client) Lock(ctx context.Context, code string, version int64, locked bool) (*api.LockResponse, error) {
	var resp api.LockResponse
	if err := c.doPost(ctx,
		fmt.Sprintf("/api/deploys/%s/versions/%d/lock", url.PathEscape(code), version),
		api.LockRequest{Locked: locked}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SetCurrent 调用 PATCH /api/deploys/{code}/current。
func (c *Client) SetCurrent(ctx context.Context, code string, version int64) (*api.SetCurrentResponse, error) {
	var resp api.SetCurrentResponse
	body := api.SetCurrentRequest{VersionNumber: &version}
	if err := c.doPatch(ctx, "/api/deploys/"+url.PathEscape(code)+"/current", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Overwrite 调用 PATCH /api/deploys/{code}/versions/{version}（覆盖模式）。
func (c *Client) Overwrite(ctx context.Context, code string, version int64, req api.OverwriteRequest) (*api.DeployResponse, error) {
	var resp api.DeployResponse
	if err := c.doPatch(ctx,
		fmt.Sprintf("/api/deploys/%s/versions/%d", url.PathEscape(code), version),
		req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SetStatus 调用 PATCH /api/deploys/{code}/versions/{version}（状态模式）。
func (c *Client) SetStatus(ctx context.Context, code string, version int64, status string) (*api.LockResponse, error) {
	var resp api.LockResponse
	body := map[string]any{"status": status}
	if err := c.doPatch(ctx,
		fmt.Sprintf("/api/deploys/%s/versions/%d", url.PathEscape(code), version),
		body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteVersion 调用 DELETE /api/deploys/{code}/versions/{version}。
func (c *Client) DeleteVersion(ctx context.Context, code string, version int64) (*api.SetCurrentResponse, error) {
	var resp api.SetCurrentResponse
	if err := c.doDelete(ctx,
		fmt.Sprintf("/api/deploys/%s/versions/%d", url.PathEscape(code), version),
		&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetContent 调用 GET /api/deploy/content（JSON 模式）。
func (c *Client) GetContent(ctx context.Context, code string, version *int64) (*api.GetContentResponse, error) {
	q := url.Values{}
	q.Set("code", code)
	if version != nil {
		q.Set("version", fmt.Sprintf("%d", *version))
	}
	var resp api.GetContentResponse
	if err := c.doGet(ctx, "/api/deploy/content?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Download 调用 GET /api/deploy/content?download=1，返回 stream。
// 调用方负责关闭返回的 io.ReadCloser。
func (c *Client) Download(ctx context.Context, code string, version *int64) (io.ReadCloser, string, error) {
	q := url.Values{}
	q.Set("code", code)
	q.Set("download", "1")
	if version != nil {
		q.Set("version", fmt.Sprintf("%d", *version))
	}
	resp, err := c.do(ctx, http.MethodGet, "/api/deploy/content?"+q.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	return resp.Body, contentType, nil
}

// CreateToken 调用 POST /api/token。
func (c *Client) CreateToken(ctx context.Context, label string, isAdmin bool, expiresAt string, ttlSeconds *int64) (*api.TokenCreateResponse, error) {
	var resp api.TokenCreateResponse
	if err := c.doPost(ctx, "/api/token",
		api.TokenCreateRequest{Label: label, IsAdmin: isAdmin, ExpiresAt: expiresAt, TTLSeconds: ttlSeconds}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ClaimAnonymousSession(ctx context.Context, sessionID string) (*api.SessionClaimResponse, error) {
	var resp api.SessionClaimResponse
	if err := c.doPost(ctx, "/api/session/claim",
		api.SessionClaimRequest{SessionID: sessionID}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListTokens 调用 GET /api/tokens。
func (c *Client) ListTokens(ctx context.Context) (*api.TokenListResponse, error) {
	var resp api.TokenListResponse
	if err := c.doGet(ctx, "/api/tokens", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RevokeToken 调用 DELETE /api/tokens/{id}。
func (c *Client) RevokeToken(ctx context.Context, id string) (*api.TokenRevokeResponse, error) {
	var resp api.TokenRevokeResponse
	if err := c.doDelete(ctx, "/api/tokens/"+url.PathEscape(id), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListScreens 调用 GET /api/screens。
func (c *Client) ListScreens(ctx context.Context) (*api.ScreenListResponse, error) {
	var resp api.ScreenListResponse
	if err := c.doGet(ctx, "/api/screens", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BindScreen 调用 POST /api/screens/bind。
func (c *Client) BindScreen(ctx context.Context, pairingCode, name string) (*api.ScreenBindResponse, error) {
	var resp api.ScreenBindResponse
	if err := c.doPost(ctx,
		"/api/screens/bind",
		api.ScreenBindRequest{PairingCode: pairingCode, Name: name},
		&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PublishScreen 调用 POST /api/screens/{screenId}/publish。
func (c *Client) PublishScreen(ctx context.Context, screenID, code string, versionNumber *int64) (*api.ScreenPublishResponse, error) {
	var resp api.ScreenPublishResponse
	if err := c.doPost(ctx,
		"/api/screens/"+url.PathEscape(screenID)+"/publish",
		api.ScreenPublishRequest{Code: code, VersionNumber: versionNumber},
		&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RequestScreenScreenshot 调用 POST /api/screens/{screenId}/screenshot。
func (c *Client) RequestScreenScreenshot(ctx context.Context, screenID string) (*api.ScreenScreenshotResponse, error) {
	var resp api.ScreenScreenshotResponse
	if err := c.doPost(ctx,
		"/api/screens/"+url.PathEscape(screenID)+"/screenshot",
		nil,
		&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RequestScreenCommand 调用 POST /api/screens/{screenId}/command。
func (c *Client) RequestScreenCommand(ctx context.Context, screenID, commandType string, payload json.RawMessage) (*api.ScreenCommandResponse, error) {
	var resp api.ScreenCommandResponse
	req := api.ScreenCommandRequest{Type: commandType, Payload: payload}
	if err := c.doPost(ctx,
		"/api/screens/"+url.PathEscape(screenID)+"/command",
		req,
		&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UnbindScreen 调用 DELETE /api/screens/{screenId}。
func (c *Client) UnbindScreen(ctx context.Context, screenID string) (*api.ScreenDeleteResponse, error) {
	var resp api.ScreenDeleteResponse
	if err := c.doDelete(ctx, "/api/screens/"+url.PathEscape(screenID), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
