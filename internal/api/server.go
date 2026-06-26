// Package api 提供 hostctl 的 HTTP 接口。
package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/yourorg/hostctl/internal/auth"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
	"github.com/yourorg/hostctl/internal/web"
)

// DeployerPort 是 server 调用部署器的最小接口，便于解耦。
type DeployerPort interface {
	Deploy(ctx context.Context, req DeployRequest, ownerTokenID, clientIP string) (*DeployResponse, *APIError)

	ListVersions(ctx context.Context, code string) (*ListVersionsResponse, *APIError)
	LockVersion(ctx context.Context, code string, version int64, locked bool) (*LockResponse, *APIError)
	SwitchCurrent(ctx context.Context, code string, version int64) (*SetCurrentResponse, *APIError)
	SwitchCurrentByUUID(ctx context.Context, code, versionID string) (*SetCurrentResponse, *APIError)
	OverwriteVersion(ctx context.Context, code string, version int64, req OverwriteRequest) (*DeployResponse, *APIError)
	SetVersionStatus(ctx context.Context, code string, version int64, status string) (*LockResponse, *APIError)
	DeleteVersion(ctx context.Context, code string, version int64) (*SetCurrentResponse, *APIError)
	GetContent(ctx context.Context, code string, versionPtr *int64) (*GetContentResponse, *APIError)

	// StreamDownload 写出下载流到 w（zip 多文件 / 单 HTML 直出）。
	StreamDownload(ctx context.Context, code string, versionPtr *int64, w http.ResponseWriter) *APIError

	// OpenAPI primary-strategy
	GetPrimaryStrategy(ctx context.Context, code string) (*PrimaryStrategyResponse, *APIError)
	SetPrimaryStrategy(ctx context.Context, code string, strategy PrimaryVersionStrategy) (*PrimaryStrategyResponse, *APIError)

	// ===== admin UI / 设置 =====

	DeleteSite(ctx context.Context, code string) *APIError
	ListSites(ctx context.Context) ([]store.SiteWithMeta, error)
	SetSitePinned(ctx context.Context, code string, pinned bool) error
	SetSiteVisibility(ctx context.Context, code, visibility string) error
	SetAnonymousDeployLimit(ctx context.Context, n int) error
	SetCooldownSeconds(ctx context.Context, n int) error
	SetUploadLimits(ctx context.Context, singleFileBytes, siteTotalBytes int64, filesPerSite int) error
	SetCORSAllowOrigins(ctx context.Context, origins string) error
	SetEmbedPolicy(ctx context.Context, policy, allowOrigins string) error
	CreateAnonymousSession(ctx context.Context, id string) (store.AnonymousSession, error)
	GetAnonymousSession(ctx context.Context, id string) (store.AnonymousSession, error)
	UpdateAnonymousSessionMeta(ctx context.Context, id, agentID, agentLabel, deviceIP, userAgent string) error
	IncrementAnonymousSessionDeployCount(ctx context.Context, id string) (store.AnonymousSession, error)
	ClaimAnonymousSession(ctx context.Context, id, userID string) (store.AnonymousSessionClaimResult, error)
	ListAnonymousSessions(ctx context.Context, limit int) ([]store.AnonymousSession, error)

	// ===== 应用商城（marketplace） =====

	ListMarketplaceDeploys(ctx context.Context, q, status, sort string, page, pageSize int) ([]store.MarketplaceDeploy, int, error)
	GetMarketplaceDeploy(ctx context.Context, code string) (store.MarketplaceDeploy, error)
	GetMarketplaceDeployByUUID(ctx context.Context, publicID string) (store.MarketplaceDeploy, error)
	IncrementViewCount(ctx context.Context, code string) error
	AddLike(ctx context.Context, code, fingerprint string) (int64, error)
	SiteExists(ctx context.Context, code string) (bool, error)
	GetSite(ctx context.Context, code string) (store.Site, error)
	SetSiteAccessPassword(ctx context.Context, code, password string) error

	CreateScreenPairing(ctx context.Context, pairing store.ScreenPairing) error
	BindScreenPairing(ctx context.Context, code, ownerUserID, name string) (store.Screen, error)
	CompleteScreenPairing(ctx context.Context, pairingID, pairingSecretHash, deviceTokenHash string) error
	GetScreen(ctx context.Context, id string) (store.Screen, error)
	GetScreenByDeviceTokenHash(ctx context.Context, hash string) (store.Screen, error)
	ListScreensByUser(ctx context.Context, ownerUserID string) ([]store.Screen, error)
	ListScreens(ctx context.Context) ([]store.Screen, error)
	PublishScreen(ctx context.Context, screenID, ownerUserID, siteCode string, version *int64) error
	TouchScreenHeartbeat(ctx context.Context, screenID, appVersion, runtime, deviceInfo string) (store.Screen, error)
	RequestScreenScreenshot(ctx context.Context, screenID, requestID string) (store.Screen, error)
	CompleteScreenScreenshot(ctx context.Context, screenID, requestID string, screenshotAt time.Time) (store.Screen, error)
	RequestScreenCommand(ctx context.Context, screenID, requestID, commandType, payload string) (store.Screen, error)
	CompleteScreenCommand(ctx context.Context, screenID, requestID string, completedAt time.Time) (store.Screen, error)
	UnbindScreen(ctx context.Context, screenID, ownerUserID string) error
}

type appURLConfigProvider interface {
	AppURLConfig() AppURLConfig
}

type appURLConfigSetter interface {
	SetAppURLConfig(ctx context.Context, cfg AppURLConfig) error
}

// Server 是 HTTP 服务器。
type Server struct {
	cfg         config.Config
	deployer    DeployerPort
	auth        *auth.Service
	requireAuth bool
	mux         *http.ServeMux
	logger      *log.Logger
	version     string
	captchaMu   sync.Mutex
	captchas    map[string]captchaChallenge
	screenHub   *screenHub
}

type captchaChallenge struct {
	Answer    string
	ExpiresAt time.Time
}

var routeCodeRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{2,30}[a-z0-9])?$`)

// New 构造 Server。
// requireAuth: true 时所有写操作要求 Bearer token；dev 模式下传 false。
func New(cfg config.Config, deployer DeployerPort, authSvc *auth.Service, requireAuth bool, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	s := &Server{
		cfg:         cfg,
		deployer:    deployer,
		auth:        authSvc,
		requireAuth: requireAuth,
		mux:         http.NewServeMux(),
		logger:      logger,
		version:     "dev",
		captchas:    map[string]captchaChallenge{},
		screenHub:   newScreenHub(),
	}
	s.routes()
	return s
}

func (s *Server) requestBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	if base := baseURLFromRequest(r); base != "" {
		return base
	}
	return ""
}

func baseURLFromRequest(r *http.Request) string {
	if base := baseURLFromClientOrigin(r); base != "" {
		return base
	}
	host := forwardedHost(r)
	if host == "" {
		return ""
	}
	if strings.ContainsAny(host, "/?#") {
		return ""
	}
	scheme := requestScheme(r)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + host
}

func baseURLFromClientOrigin(r *http.Request) string {
	origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("X-Hostctl-Current-Origin")), "/")
	if origin == "" || !baseURLLooksValid(origin) {
		return ""
	}
	return origin
}

func requestScheme(r *http.Request) string {
	if proto := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return strings.ToLower(proto)
	}
	if forwarded := r.Header.Get("Forwarded"); forwarded != "" {
		for _, part := range strings.Split(forwarded, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if ok && strings.EqualFold(strings.TrimSpace(key), "proto") {
				return strings.ToLower(strings.Trim(strings.TrimSpace(value), `"`))
			}
		}
	}
	if r.TLS != nil {
		return "https"
	}
	if r.URL != nil && r.URL.Scheme != "" {
		return strings.ToLower(r.URL.Scheme)
	}
	return "http"
}

func firstForwardedValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(value, ",")[0])
}

func (s *Server) appURLConfig() AppURLConfig {
	if p, ok := s.deployer.(appURLConfigProvider); ok {
		return p.AppURLConfig()
	}
	return NewAppURLConfig(s.cfg)
}

func (s *Server) appURLConfigForRequest(r *http.Request) AppURLConfig {
	cfg := s.appURLConfig()
	if base := s.requestBaseURL(r); base != "" {
		cfg = cfg.WithPathBaseURL(base)
	}
	return cfg
}

// WithVersion 设置服务端版本号（用于 /api/config 响应）。
func (s *Server) rewriteDeployResponseURLs(r *http.Request, resp *DeployResponse) {
	if resp == nil || strings.TrimSpace(resp.Code) == "" {
		return
	}
	appURLs := s.appURLConfigForRequest(r)
	version := int64(resp.VersionNumber)
	resp.URL = appURLs.PrimaryAppURL(resp.Code, nil)
	resp.DetailURL = resp.URL
	if version > 0 {
		resp.VersionURL = appURLs.PrimaryAppURL(resp.Code, &version)
	}
	resp.QRCode = generateQRCodeDataURL(resp.URL)
	if resp.AgentGuideURL != "" {
		base := strings.TrimRight(s.requestBaseURL(r), "/")
		if base == "" {
			resp.AgentGuideURL = "/api-docs.html"
		} else {
			resp.AgentGuideURL = base + "/api-docs.html"
		}
	}
}

func (s *Server) versionCreatedResponseForRequest(r *http.Request, resp *DeployResponse) VersionCreatedResponse {
	s.rewriteDeployResponseURLs(r, resp)
	return VersionCreatedResponse{
		Success:                true,
		Code:                   resp.Code,
		VersionID:              resp.VersionID,
		VersionNumber:          resp.VersionNumber,
		URL:                    resp.URL,
		DetailURL:              resp.DetailURL,
		VersionURL:             resp.VersionURL,
		CurrentVersionID:       resp.CurrentVersionID,
		PreserveHint:           resp.PreserveHint,
		PrimaryVersionStrategy: resp.PrimaryVersionStrategy,
	}
}

func generateQRCodeDataURL(text string) string {
	png, err := qrcode.Encode(text, qrcode.Medium, 256)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

func (s *Server) WithVersion(v string) *Server { s.version = v; return s }

// routes 注册路由。使用 Go 1.22 {name} 模式做路径参数。
func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/deploy", s.handleDeploy)
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/session", s.handleAnonymousSession)
	s.mux.HandleFunc("POST /api/session/claim", s.handleClaimAnonymousSession)
	s.mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)

	// 应用商城（公开 API：/api/deploys）
	s.mux.HandleFunc("GET /api/deploys", s.handleListMarketplace)
	s.mux.HandleFunc("GET /api/deploys/{publicId}", s.handleGetMarketplaceDeploy)
	s.mux.HandleFunc("POST /api/deploys/{code}/like", s.handleLikeDeploy)
	s.mux.HandleFunc("GET /api/deploys/{code}/qr", s.handleQRCode)
	s.mux.HandleFunc("POST /api/deploys/{code}/access", s.handleSiteAccessLogin)
	s.mux.HandleFunc("PATCH /api/deploys/{code}/access", s.handleSetSiteAccessPassword)
	s.mux.HandleFunc("PATCH /api/deploys/{code}/visibility", s.handleSetSiteVisibility)

	// 屏幕投放：用户侧仅注册用户；设备侧仅 Device Token。
	s.mux.HandleFunc("GET /api/screens", s.handleListScreens)
	s.mux.HandleFunc("POST /api/screens/bind", s.handleBindScreen)
	s.mux.HandleFunc("POST /api/screens/{screenId}/publish", s.handlePublishScreen)
	s.mux.HandleFunc("POST /api/screens/{screenId}/screenshot", s.handleRequestScreenScreenshot)
	s.mux.HandleFunc("GET /api/screens/{screenId}/screenshot", s.handleGetScreenScreenshot)
	s.mux.HandleFunc("POST /api/screens/{screenId}/command", s.handleRequestScreenCommand)
	s.mux.HandleFunc("DELETE /api/screens/{screenId}", s.handleUnbindScreen)
	s.mux.HandleFunc("POST /api/device/pairing/start", s.handleDevicePairingStart)
	s.mux.HandleFunc("POST /api/device/pairing/complete", s.handleDevicePairingComplete)
	s.mux.HandleFunc("GET /api/device/manifest", s.handleDeviceManifest)
	s.mux.HandleFunc("GET /api/device/ws", s.handleDeviceWebSocket)
	s.mux.HandleFunc("POST /api/device/heartbeat", s.handleDeviceHeartbeat)
	s.mux.HandleFunc("POST /api/device/screenshot", s.handleDeviceScreenshot)
	s.mux.HandleFunc("POST /api/device/command/ack", s.handleDeviceCommandAck)

	// 版本管理（OpenAPI）
	s.mux.HandleFunc("GET /api/deploys/{code}/versions", s.handleListVersions)
	s.mux.HandleFunc("POST /api/deploys/{code}/versions/{version}/lock", s.handleLock)
	s.mux.HandleFunc("PATCH /api/deploys/{code}/current", s.handleSetCurrent)
	s.mux.HandleFunc("PATCH /api/deploys/{code}/versions/{version}", s.handlePatchVersion)
	s.mux.HandleFunc("DELETE /api/deploys/{code}/versions/{version}", s.handleDeleteVersion)

	// primary-strategy（OpenAPI）
	s.mux.HandleFunc("GET /api/deploys/{code}/primary-strategy", s.handleGetPrimaryStrategy)
	s.mux.HandleFunc("PATCH /api/deploys/{code}/primary-strategy", s.handleSetPrimaryStrategy)

	// 内容读取（含 download=1 zip 模式）+ 兼容 PATCH 追加版本
	s.mux.HandleFunc("GET /api/deploy/content", s.handleGetContent)
	s.mux.HandleFunc("PATCH /api/deploy/content", s.handlePatchDeployContent)

	// Token 管理
	s.mux.HandleFunc("POST /api/token", s.handleCreateToken)
	s.mux.HandleFunc("GET /api/tokens", s.handleListTokens)
	s.mux.HandleFunc("DELETE /api/tokens/{id}", s.handleRevokeToken)

	// 配置 + 站点列表（admin / 内部）
	s.mux.HandleFunc("GET /api/admin/session", s.handleAdminSession)
	s.mux.HandleFunc("POST /api/admin/login", s.handleAdminLogin)
	s.mux.HandleFunc("POST /api/admin/logout", s.handleAdminLogout)
	s.mux.HandleFunc("POST /api/admin/setup", s.handleAdminSetup)
	s.mux.HandleFunc("GET /api/auth/captcha", s.handleCaptcha)
	s.mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.mux.HandleFunc("PATCH /api/account/password", s.handleAccountPassword)
	s.mux.HandleFunc("GET /api/admin/anonymous-sessions", s.handleAdminAnonymousSessions)
	s.mux.HandleFunc("GET /api/admin/skill", s.handleAdminGetSkill)
	s.mux.HandleFunc("PUT /api/admin/skill", s.handleAdminPutSkill)
	s.mux.HandleFunc("POST /api/admin/skill/package", s.handleAdminUploadSkillPackage)
	s.mux.HandleFunc("GET /api/admin/users", s.handleAdminListUsers)
	s.mux.HandleFunc("POST /api/admin/users", s.handleAdminCreateUser)
	s.mux.HandleFunc("PATCH /api/admin/users/{id}", s.handleAdminUpdateUser)
	s.mux.HandleFunc("DELETE /api/admin/users/{id}", s.handleAdminDeleteUser)
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	s.mux.HandleFunc("GET /api/admin/sites", s.handleAdminListSites)
	s.mux.HandleFunc("PATCH /api/admin/sites/{code}/pin", s.handleAdminSetSitePin)
	s.mux.HandleFunc("DELETE /api/admin/sites/{code}", s.handleAdminDeleteSite)

	// admin 后台单页
	s.mux.HandleFunc("GET /admin", s.handleAdminUI)
	s.mux.HandleFunc("GET /admin/", s.handleAdminUI)
	s.mux.Handle("GET /admin/assets/", http.StripPrefix("/admin/", http.FileServer(http.FS(web.AdminAppFS()))))
	s.mux.HandleFunc("GET /agents/", s.handleAgentGuideUI)
	s.mux.HandleFunc("GET /screens/", s.handleScreenGuideUI)
	s.mux.HandleFunc("GET /skill/hostctl-deploy.zip", s.handleSkillDownload)
	// admin 子资源（admin.css / admin.js 等）从 embed 子树取
	s.mux.Handle("GET /admin/static/", http.StripPrefix("/admin/static/", http.FileServer(http.FS(web.AdminSubFS()))))

	// dev 模式：把已部署站点 serve 起来（生产由 Caddy 做）。
	// 路径模式优先级：/api/*、/admin、/admin/* 已被前面注册占用。
	// /agent/{code}：应用本体 URL（用户部署后访问应用本身用这个前缀）
	// /agent/{code}/{path...}：当前版本的多文件子路径（CSS/JS/图片等）
	// /agent/{code}/versions/{version}：历史版本预览入口
	// /agent/{code}/versions/{version}/{path...}：历史版本的多文件子路径
	// /deploy/{uuid}：详情页（SPA）
	s.mux.HandleFunc("GET /agent/{code}", s.handleAppServe)
	s.mux.HandleFunc("GET /agent/{code}/{path...}", s.handleAppServe)
	s.mux.HandleFunc("GET /agent/{code}/versions/{version}", s.handleAppServe)
	s.mux.HandleFunc("GET /agent/{code}/versions/{version}/{path...}", s.handleAppServe)
	s.mux.HandleFunc("GET /deploy/{uuid}", s.handleDetailUI)

	// 用户端 SPA：根路径托管 user/ 目录（首页 / 部署 / API 文档 / 静态资源）
	s.mux.Handle("GET /", http.FileServer(http.FS(web.UserSubFS())))
}

// ListenAndServe 启动 HTTP 服务。
func (s *Server) ListenAndServe() error {
	s.logger.Printf("hostctl-server listening on %s", s.cfg.HTTPAddr)
	srv := &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           s.withMiddleware(s.mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServe()
}

// withMiddleware 包装请求 ID 注入、日志、CORS（dev 阶段）。
func (s *Server) withMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := newRequestID()
		w.Header().Set("X-Request-Id", reqID)

		// 把 reqID 放进 context，handler 可以取出加到错误响应里
		r = r.WithContext(withRequestID(r.Context(), reqID))

		s.applyCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if s.tryServeDomainApp(w, r) {
			s.logger.Printf("%s %s %s %dms", r.Method, r.URL.Path, reqID, time.Since(start).Milliseconds())
			return
		}

		h.ServeHTTP(w, r)

		s.logger.Printf("%s %s %s %dms", r.Method, r.URL.Path, reqID, time.Since(start).Milliseconds())
	})
}

func (s *Server) tryServeDomainApp(w http.ResponseWriter, r *http.Request) bool {
	appURLs := s.appURLConfig()
	code := appURLs.CodeFromRequestHost(r)
	if code == "" {
		return false
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/deploys/"+code+"/access" {
		r.SetPathValue("code", code)
		s.handleSiteAccessLogin(w, r)
		return true
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return true
	}
	sub := strings.TrimPrefix(r.URL.Path, "/")
	versionText := ""
	if sub == "versions" || strings.HasPrefix(sub, "versions/") {
		parts := strings.SplitN(sub, "/", 3)
		if len(parts) >= 2 {
			versionText = parts[1]
			if len(parts) == 3 {
				sub = parts[2]
			} else {
				sub = ""
			}
		}
	}
	s.serveAppContent(w, r, code, sub, versionText, true)
	return true
}

func (s *Server) applyCORS(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	allowed := config.NormalizeCORSAllowOrigins(s.cfg.CORSAllowOrigins)
	if allowed == "" {
		return
	}
	if origin != "" && originAllowed(origin, allowed) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
	}
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Hostctl-Session, X-Hostctl-Agent-Id, X-Hostctl-Agent-Label")
}

func originAllowed(origin, allowed string) bool {
	for _, item := range strings.FieldsFunc(allowed, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		if strings.TrimSpace(item) == origin {
			return true
		}
	}
	return false
}

func (s *Server) maxRequestBodyBytes() int64 {
	limit := s.cfg.MaxSiteTotalBytes + (1 << 20)
	if limit < 8<<20 {
		return 8 << 20
	}
	if limit > 256<<20 {
		return 256 << 20
	}
	return limit
}

// handleDeploy 处理 POST /api/deploy。
func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())

	if r.Method != http.MethodPost {
		writeError(w, apiErrWithReqID(NewError(CodeMethodNotAllowed, "method",
			fmt.Sprintf("method %s not allowed; use POST", r.Method)), reqID))
		return
	}

	// 强制 application/json，禁用 multipart。
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if !strings.HasPrefix(ct, "application/json") {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "content_type",
			"Content-Type must be application/json; multipart/form-data is not supported"), reqID))
		return
	}

	// 限制 body 大小（防 DoS）
	r.Body = http.MaxBytesReader(w, r.Body, s.maxRequestBodyBytes())

	var req DeployRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "parse_json",
			fmt.Sprintf("invalid JSON body: %v", err)), reqID))
		return
	}

	var ownerTokenID string
	var anonymousSessionID string
	var userID string
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		tok, authErr := s.authenticateToken(r)
		if authErr != nil {
			writeError(w, apiErrWithReqID(authErr, reqID))
			return
		}
		ownerTokenID = ownerForToken(tok)
		if tok.OwnerUserID != "" {
			user, userErr := s.auth.GetUser(r.Context(), tok.OwnerUserID)
			if userErr != nil || !user.IsActive {
				writeError(w, apiErrWithReqID(NewError(CodeForbidden, "user", "token owner is inactive or missing"), reqID))
				return
			}
			if !user.IsAdmin && user.DeployLimit >= 0 && user.DeployCount >= user.DeployLimit {
				writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "user_quota",
					fmt.Sprintf("user deploy limit reached (%d/%d)", user.DeployCount, user.DeployLimit)).
					WithHint("Ask an admin to raise your deploy quota."), reqID))
				return
			}
			userID = user.ID
		}
	} else if user, ok := s.adminUserFromCookie(r); ok {
		if !user.IsAdmin && user.DeployLimit >= 0 && user.DeployCount >= user.DeployLimit {
			writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "user_quota",
				fmt.Sprintf("user deploy limit reached (%d/%d)", user.DeployCount, user.DeployLimit)).
				WithHint("Ask an admin to raise your deploy quota."), reqID))
			return
		}
		userID = user.ID
		ownerTokenID = "user:" + user.ID
	} else {
		sess, sessionErr := s.ensureAnonymousSession(w, r)
		if sessionErr != nil {
			writeError(w, apiErrWithReqID(sessionErr, reqID))
			return
		}
		if s.cfg.AnonymousDeployLimit >= 0 && sess.DeployCount >= s.cfg.AnonymousDeployLimit {
			writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "anonymous_quota",
				fmt.Sprintf("anonymous deploy limit reached (%d/%d)", sess.DeployCount, s.cfg.AnonymousDeployLimit)).
				WithHint("Ask the user to register or sign in, create a user token, then claim this anonymous session."), reqID))
			return
		}
		anonymousSessionID = sess.ID
		ownerTokenID = "anon:" + sess.ID
	}
	if req.EnableCustomCode && strings.TrimSpace(req.CustomCode) != "" {
		if apiErr := s.authorizeDeployCustomCode(r, strings.TrimSpace(req.CustomCode), ownerTokenID); apiErr != nil {
			writeError(w, apiErrWithReqID(apiErr, reqID))
			return
		}
	}
	clientIP := clientIPFromRequest(r)

	resp, apiErr := s.deployer.Deploy(r.Context(), req, ownerTokenID, clientIP)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	if anonymousSessionID != "" {
		_, err := s.deployer.IncrementAnonymousSessionDeployCount(r.Context(), anonymousSessionID)
		if err != nil {
			s.logger.Printf("failed to increment anonymous session %s: %v", anonymousSessionID, err)
		}
	}
	if userID != "" {
		if _, err := s.auth.IncrementUserDeployCount(r.Context(), userID); err != nil {
			s.logger.Printf("failed to increment user deploy count %s: %v", userID, err)
		}
	}
	s.rewriteDeployResponseURLs(r, resp)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleHealth 健康检查。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"status":"ok"}`))
}

func (s *Server) handleAnonymousSession(w http.ResponseWriter, r *http.Request) {
	sess, apiErr := s.ensureAnonymousSession(w, r)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, requestIDFromContext(r.Context())))
		return
	}
	writeJSON(w, http.StatusOK, s.toAnonymousSessionResponse(sess))
}

func (s *Server) handleClaimAnonymousSession(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	userID, authErr := s.claimActorUserID(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req SessionClaimRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = anonymousSessionIDFromRequest(r)
	}
	if sessionID == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "anonymous_session", "sessionId is required"), reqID))
		return
	}
	result, err := s.deployer.ClaimAnonymousSession(r.Context(), sessionID, userID)
	if err != nil {
		code := CodeInvalidInput
		if errors.Is(err, store.ErrNotFound) {
			code = CodeNotFound
		}
		writeError(w, apiErrWithReqID(NewError(code, "anonymous_session", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, SessionClaimResponse{
		Success:        true,
		SessionID:      result.SessionID,
		UserID:         result.UserID,
		SiteCount:      result.SiteCount,
		DeployCount:    result.DeployCount,
		AlreadyClaimed: result.AlreadyClaimed,
	})
}

func (s *Server) claimActorUserID(r *http.Request) (string, *APIError) {
	if user, ok := s.adminUserFromCookie(r); ok {
		if !user.IsActive {
			return "", NewError(CodeForbidden, "auth", "user is inactive")
		}
		return user.ID, nil
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		return "", NewError(CodeUnauthorized, "auth", "login or bearer token required")
	}
	tok, authErr := s.authenticateToken(r)
	if authErr != nil {
		return "", authErr
	}
	user, err := s.auth.GetUser(r.Context(), tok.OwnerUserID)
	if err != nil || !user.IsActive {
		return "", NewError(CodeForbidden, "user", "token owner is inactive or missing")
	}
	return user.ID, nil
}

func (s *Server) ensureAnonymousSession(w http.ResponseWriter, r *http.Request) (store.AnonymousSession, *APIError) {
	id := anonymousSessionIDFromRequest(r)
	if id != "" {
		sess, err := s.deployer.GetAnonymousSession(r.Context(), id)
		if err == nil {
			if strings.TrimSpace(sess.ClaimedByUserID) != "" {
				return store.AnonymousSession{}, NewError(CodeUnauthorized, "anonymous_session", "anonymous session has been claimed")
			}
			s.updateAnonymousSessionMeta(r, sess.ID)
			sess, _ = s.deployer.GetAnonymousSession(r.Context(), sess.ID)
			setAnonymousSessionCookie(w, sess.ID)
			return sess, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return store.AnonymousSession{}, NewError(CodeInternal, "anonymous_session", err.Error())
		}
	}

	id = "anon_" + randomHex(16)
	sess, err := s.deployer.CreateAnonymousSession(r.Context(), id)
	if err != nil {
		return store.AnonymousSession{}, NewError(CodeInternal, "anonymous_session", err.Error())
	}
	s.updateAnonymousSessionMeta(r, sess.ID)
	sess, _ = s.deployer.GetAnonymousSession(r.Context(), sess.ID)
	setAnonymousSessionCookie(w, sess.ID)
	return sess, nil
}

func (s *Server) updateAnonymousSessionMeta(r *http.Request, id string) {
	agentID := strings.TrimSpace(r.Header.Get("X-Hostctl-Agent-Id"))
	agentLabel := strings.TrimSpace(r.Header.Get("X-Hostctl-Agent-Label"))
	userAgent := strings.TrimSpace(r.UserAgent())
	deviceIP := clientIPFromRequest(r)
	if len(agentID) > 128 {
		agentID = agentID[:128]
	}
	if len(agentLabel) > 120 {
		agentLabel = agentLabel[:120]
	}
	if len(userAgent) > 240 {
		userAgent = userAgent[:240]
	}
	if err := s.deployer.UpdateAnonymousSessionMeta(r.Context(), id, agentID, agentLabel, deviceIP, userAgent); err != nil {
		s.logger.Printf("failed to update anonymous session meta %s: %v", id, err)
	}
}

func setAnonymousSessionCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "hostctl_anon_session",
		Value:    id,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})
}

func setAdminSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "hostctl_admin_session",
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})
}

func (s *Server) toAnonymousSessionResponse(sess store.AnonymousSession) AnonymousSessionResponse {
	remaining := s.cfg.AnonymousDeployLimit - sess.DeployCount
	if remaining < 0 {
		remaining = 0
	}
	return AnonymousSessionResponse{
		Success:     true,
		SessionID:   sess.ID,
		AgentID:     sess.AgentID,
		AgentLabel:  sess.AgentLabel,
		DeployCount: sess.DeployCount,
		DeployLimit: s.cfg.AnonymousDeployLimit,
		Remaining:   remaining,
	}
}

// requestID 上下文工具
type reqIDKey struct{}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, reqIDKey{}, id)
}

func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(reqIDKey{}).(string)
	if v == "" {
		return ""
	}
	return v
}

func apiErrWithReqID(e *APIError, reqID string) *APIError {
	return e.WithRequestID(reqID)
}

func newRequestID() string {
	// 简化的 req ID：时间戳 + 短随机
	return fmt.Sprintf("req-%d-%s", time.Now().UnixNano(), randomHex(4))
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	out := make([]byte, n*2)
	const hexChars = "0123456789abcdef"
	for i, v := range b {
		out[i*2] = hexChars[v>>4]
		out[i*2+1] = hexChars[v&0x0f]
	}
	return string(out)
}

// 让 errors 在 import 中可见（Day 3+ 会用 errors.Is）
var _ = errors.Is

// ===== 应用商城（公开 API：/api/deploys） =====

// marketplaceDeployResponse 是 GET /api/deploys 列表里单条记录的 JSON 形态，
// 字段命名保持稳定，便于前端和 Agent 复用。
type marketplaceDeployResponse struct {
	ID                     string  `json:"id"`
	Code                   string  `json:"code"`
	Owned                  bool    `json:"owned,omitempty"`
	CurrentVersionID       *string `json:"currentVersionId,omitempty"`
	PrimaryVersionStrategy string  `json:"primaryVersionStrategy"`
	Title                  string  `json:"title"`
	Description            string  `json:"description"`
	Filename               string  `json:"filename"`
	FilePath               string  `json:"filePath"`
	FileSize               int64   `json:"fileSize"`
	QrCodePath             string  `json:"qrCodePath"`
	PrimaryVersionID       *string `json:"primaryVersionId"`
	CreatedAt              string  `json:"createdAt"`
	UpdatedAt              string  `json:"updatedAt"`
	ViewCount              int64   `json:"viewCount"`
	LikeCount              int64   `json:"likeCount"`
	VersionCount           int     `json:"versionCount"`
	ExpiresAt              *string `json:"expiresAt"`
	Status                 string  `json:"status"`
	Visibility             string  `json:"visibility"`
	AccessProtected        bool    `json:"accessProtected"`
	IsPinned               bool    `json:"isPinned"`
	PinnedAt               *string `json:"pinnedAt"`
}

// handleListMarketplace 处理 GET /api/deploys —— 公开列出所有 deploy（应用商城）。
// 支持 query: q / status / sort / page / pageSize
func (s *Server) handleListMarketplace(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := r.URL.Query().Get("status")
	sort := r.URL.Query().Get("sort")
	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	pageSize := parseIntDefault(r.URL.Query().Get("pageSize"), 24)

	deploys, total, err := s.deployer.ListMarketplaceDeploys(r.Context(), q, status, sort, page, pageSize)
	if err != nil {
		writeError(w, NewError(CodeInternal, "marketplace", "list marketplace: "+err.Error()))
		return
	}

	actor, isAdmin := s.marketplaceActor(r)
	out := make([]marketplaceDeployResponse, 0, len(deploys))
	for _, d := range deploys {
		out = append(out, s.toMarketplaceResponse(r, d, actor, isAdmin))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deploys":  out,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// handleGetMarketplaceDeploy 处理 GET /api/deploys/{publicId} —— 单条详情。
// {publicId} 既可以是 32 字符 UUID，也可以是 site.code（短码）。
func (s *Server) handleGetMarketplaceDeploy(w http.ResponseWriter, r *http.Request) {
	idOrCode := r.PathValue("publicId")
	if idOrCode == "" {
		writeError(w, NewError(CodeInvalidInput, "marketplace", "missing id"))
		return
	}

	var d store.MarketplaceDeploy
	var err error
	if len(idOrCode) == 32 {
		d, err = s.deployer.GetMarketplaceDeployByUUID(r.Context(), idOrCode)
	} else {
		d, err = s.deployer.GetMarketplaceDeploy(r.Context(), idOrCode)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, NewError(CodeNotFound, "marketplace", "deploy not found"))
		return
	}
	if err != nil {
		writeError(w, NewError(CodeInternal, "marketplace", "get deploy: "+err.Error()))
		return
	}

	actor, isAdmin := s.marketplaceActor(r)
	writeJSON(w, http.StatusOK, s.toMarketplaceResponse(r, d, actor, isAdmin))
}

// handleLikeDeploy 处理 POST /api/deploys/{code}/like —— 公开点赞。
// 重复点赞（同一 fingerprint）幂等。点赞只影响市场排序，不授予写权限，不触发锁定。
func (s *Server) handleLikeDeploy(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "like", "missing code"), reqID))
		return
	}

	if _, err := s.deployer.GetSite(r.Context(), code); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "like", "deploy not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "like", "get site: "+err.Error()), reqID))
		return
	}

	fp := likeFingerprint(r)
	newCount, err := s.deployer.AddLike(r.Context(), code, fp)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "like", "add like: "+err.Error()), reqID))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"code":      code,
		"likeCount": newCount,
	})
}

// toMarketplaceResponse 把 store.MarketplaceDeploy 转成对外的 JSON 形态。
// 拼接好 filePath / qrCodePath（绝对 URL）方便前端直接用。
func (s *Server) marketplaceActor(r *http.Request) (string, bool) {
	actor, isAdmin, err := s.optionalActor(r)
	if err != nil {
		return "", false
	}
	return actor, isAdmin
}

func (s *Server) toMarketplaceResponse(r *http.Request, d store.MarketplaceDeploy, actor string, isAdmin bool) marketplaceDeployResponse {
	base := s.requestBaseURL(r)
	appURLs := s.appURLConfigForRequest(r)
	var currentVerID, primaryVerID *string
	if d.CurrentVersionID != "" {
		v := d.CurrentVersionID
		currentVerID = &v
	}
	_ = primaryVerID
	var filePath, qrPath string
	if d.Code != "" {
		filePath = appURLs.PrimaryAppURL(d.Code, nil)
		qrPath = base + "/api/deploys/" + d.Code + "/qr"
	}
	var expiresAt *string
	if d.ExpiresAt != nil {
		t := d.ExpiresAt.UTC().Format(time.RFC3339Nano)
		expiresAt = &t
	}
	var pinnedAt *string
	if d.PinnedAt != nil {
		t := d.PinnedAt.UTC().Format(time.RFC3339Nano)
		pinnedAt = &t
	}
	owned := isAdmin || (actor != "" && d.OwnerTokenID == actor)
	return marketplaceDeployResponse{
		ID:                     d.ID,
		Code:                   d.Code,
		Owned:                  owned,
		CurrentVersionID:       currentVerID,
		PrimaryVersionStrategy: d.PrimaryVersionStrategy,
		Title:                  d.Title,
		Description:            d.Description,
		Filename:               d.Filename,
		FilePath:               filePath,
		FileSize:               d.FileSize,
		QrCodePath:             qrPath,
		PrimaryVersionID:       primaryVerID,
		CreatedAt:              d.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:              d.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ViewCount:              d.ViewCount,
		LikeCount:              d.LikeCount,
		VersionCount:           d.VersionCount,
		ExpiresAt:              expiresAt,
		Status:                 d.Status,
		Visibility:             d.Visibility,
		AccessProtected:        d.AccessProtected,
		IsPinned:               d.IsPinned,
		PinnedAt:               pinnedAt,
	}
}

func (s *Server) handleQRCode(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		http.NotFound(w, r)
		return
	}
	exists, err := s.deployer.SiteExists(r.Context(), code)
	if err != nil || !exists {
		http.NotFound(w, r)
		return
	}
	png, err := qrcode.Encode(s.appURLConfigForRequest(r).PrimaryAppURL(code, nil), qrcode.Medium, 256)
	if err != nil {
		writeError(w, NewError(CodeInternal, "qr", err.Error()))
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(png)
}

type siteAccessRequest struct {
	Password string `json:"password"`
}

type siteAccessUpdateRequest struct {
	Password string `json:"password"`
}

const siteAccessCookieTTL = 5 * time.Minute
const screenAccessCookieTTL = 5 * time.Minute

func (s *Server) handleSiteAccessLogin(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := strings.TrimSpace(r.PathValue("code"))
	site, err := s.deployer.GetSite(r.Context(), code)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeNotFound, "site", "site not found"), reqID))
		return
	}
	var req siteAccessRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if site.AccessPasswordHash == "" {
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	if auth.VerifyPassword(req.Password, site.AccessPasswordHash) {
		setSiteAccessCookie(w, code, site.AccessPasswordHash)
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "access_password", "password is incorrect"), reqID))
}

func (s *Server) handleSetSiteAccessPassword(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := strings.TrimSpace(r.PathValue("code"))
	if apiErr := s.authorizeSiteWrite(r, code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	var req siteAccessUpdateRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	password := strings.TrimSpace(req.Password)
	if password != "" && len(password) < 4 {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "access_password", "password must be at least 4 characters"), reqID))
		return
	}
	if err := s.deployer.SetSiteAccessPassword(r.Context(), code, password); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "access_password", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "code": code, "accessProtected": password != ""})
}

func (s *Server) handleSetSiteVisibility(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := strings.TrimSpace(r.PathValue("code"))
	if apiErr := s.authorizeSiteWrite(r, code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	var req SiteVisibilityRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	visibility := normalizeSiteVisibility(req.Visibility)
	if visibility == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "visibility", "visibility must be public or unlisted"), reqID))
		return
	}
	if err := s.deployer.SetSiteVisibility(r.Context(), code, visibility); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "site", "site not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "visibility", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, SiteVisibilityResponse{Success: true, Code: code, Visibility: visibility})
}

func normalizeSiteVisibility(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "public":
		return "public"
	case "unlisted":
		return "unlisted"
	default:
		return ""
	}
}

func setSiteAccessCookie(w http.ResponseWriter, code, passwordHash string) {
	expiresAt := time.Now().UTC().Add(siteAccessCookieTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     siteAccessCookieName(code),
		Value:    siteAccessCookieValue(code, passwordHash, expiresAt),
		Path:     "/",
		MaxAge:   int(siteAccessCookieTTL.Seconds()),
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})
}

func siteAccessCookieName(code string) string {
	return "pagepilot_access_" + code
}

func siteAccessCookieValue(code, passwordHash string, expiresAt time.Time) string {
	expiresUnix := strconv.FormatInt(expiresAt.Unix(), 10)
	return expiresUnix + "." + siteAccessCookieSignature(code, passwordHash, expiresUnix)
}

func siteAccessCookieSignature(code, passwordHash, expiresUnix string) string {
	mac := hmac.New(sha256.New, []byte(passwordHash))
	_, _ = mac.Write([]byte(code))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(expiresUnix))
	return hex.EncodeToString(mac.Sum(nil))
}

func validSiteAccessCookie(value, code, passwordHash string, now time.Time) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	expiresUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expiresUnix <= now.Unix() {
		return false
	}
	expected := siteAccessCookieSignature(code, passwordHash, parts[0])
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expected)) == 1
}

func screenAccessCookieName(code string) string {
	return "pagepilot_screen_access_" + code
}

func (s *Server) newScreenAccessCookie(screen store.Screen, code string, version *int64) *ScreenAccessCookie {
	code = strings.TrimSpace(code)
	if code == "" || strings.TrimSpace(screen.ID) == "" || strings.TrimSpace(screen.DeviceTokenHash) == "" {
		return nil
	}
	versionNumber := int64(0)
	if version != nil {
		versionNumber = *version
	}
	expiresAt := time.Now().UTC().Add(screenAccessCookieTTL)
	return &ScreenAccessCookie{
		Name:          screenAccessCookieName(code),
		Value:         screenAccessCookieValue(screen.ID, code, versionNumber, expiresAt, screen.DeviceTokenHash),
		Path:          "/",
		MaxAgeSeconds: int(screenAccessCookieTTL.Seconds()),
		ExpiresAt:     expiresAt,
	}
}

func screenAccessCookieValue(screenID, code string, version int64, expiresAt time.Time, deviceTokenHash string) string {
	versionText := strconv.FormatInt(version, 10)
	expiresUnix := strconv.FormatInt(expiresAt.Unix(), 10)
	return strings.Join([]string{
		screenID,
		versionText,
		expiresUnix,
		screenAccessCookieSignature(screenID, code, versionText, expiresUnix, deviceTokenHash),
	}, ".")
}

func screenAccessCookieSignature(screenID, code, version, expiresUnix, deviceTokenHash string) string {
	mac := hmac.New(sha256.New, []byte(deviceTokenHash))
	_, _ = mac.Write([]byte("screen-access-v1|"))
	_, _ = mac.Write([]byte(screenID))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(code))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(version))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(expiresUnix))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) validScreenAccessCookie(r *http.Request, code string, versionPtr *int64, now time.Time) bool {
	c, err := r.Cookie(screenAccessCookieName(code))
	if err != nil || c.Value == "" {
		return false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return false
	}
	cookieVersion, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || cookieVersion < 0 {
		return false
	}
	expiresUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || expiresUnix <= now.Unix() {
		return false
	}
	screen, err := s.deployer.GetScreen(r.Context(), parts[0])
	if err != nil || screen.CurrentSiteCode != code || screen.DeviceTokenHash == "" {
		return false
	}
	if versionPtr != nil && cookieVersion != *versionPtr {
		return false
	}
	if versionPtr == nil && cookieVersion > 0 {
		if screen.CurrentVersion == nil || *screen.CurrentVersion != cookieVersion {
			return false
		}
	}
	expected := screenAccessCookieSignature(parts[0], code, parts[1], parts[2], screen.DeviceTokenHash)
	return subtle.ConstantTimeCompare([]byte(parts[3]), []byte(expected)) == 1
}

// likeFingerprint 用 IP + UA hash 生成点赞指纹（防同一用户重复点赞）。
func likeFingerprint(r *http.Request) string {
	ip := clientIPFromRequest(r)
	ua := r.UserAgent()
	h := sha256.Sum256([]byte(ip + "|" + ua))
	return hex.EncodeToString(h[:16])
}

// parseIntDefault 解析 query 中的整数，失败返回 def。
func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// ===== 版本管理处理程序（Day 3） =====

// handleListVersions 处理 GET /api/deploys/{code}/versions。
func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			"missing code in path"), reqID))
		return
	}
	resp, apiErr := s.deployer.ListVersions(r.Context(), code)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleLock 处理 POST /api/deploys/{code}/versions/{version}/lock。
func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	version, ok := parseVersionPath(w, r, reqID)
	if !ok {
		return
	}
	if apiErr := s.authorizeSiteWrite(r, code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}

	var req LockRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}

	resp, apiErr := s.deployer.LockVersion(r.Context(), code, version, req.Locked)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSetCurrent 处理 PATCH /api/deploys/{code}/current。
// 支持两种模式：按版本号 {versionNumber} 或按 UUID {versionId}。
func (s *Server) handleSetCurrent(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if apiErr := s.authorizeSiteWrite(r, code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}

	var req SetCurrentRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}

	var resp *SetCurrentResponse
	var apiErr *APIError
	switch {
	case req.VersionNumber != nil && *req.VersionNumber > 0:
		resp, apiErr = s.deployer.SwitchCurrent(r.Context(), code, *req.VersionNumber)
	case req.VersionID != "":
		resp, apiErr = s.deployer.SwitchCurrentByUUID(r.Context(), code, req.VersionID)
	default:
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
			"provide either 'versionNumber' or 'versionId'"), reqID))
		return
	}
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePatchVersion 处理 PATCH /api/deploys/{code}/versions/{version}。
// 两种用法：
//   - 状态切换：{"status": "active" | "inactive"}
//   - 覆盖内容：{"description": "...", "content"|"files": ...}
func (s *Server) handlePatchVersion(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	version, ok := parseVersionPath(w, r, reqID)
	if !ok {
		return
	}

	if apiErr := s.authorizeSiteWrite(r, code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}

	if ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))); !strings.HasPrefix(ct, "application/json") {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "content_type",
			"Content-Type must be application/json"), reqID))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.maxRequestBodyBytes())

	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "parse_json",
			fmt.Sprintf("invalid JSON body: %v", err)), reqID))
		return
	}

	if statusVal, hasStatus := raw["status"]; hasStatus {
		status, ok := statusVal.(string)
		if !ok {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"'status' must be a string"), reqID))
			return
		}
		resp, apiErr := s.deployer.SetVersionStatus(r.Context(), code, version, status)
		if apiErr != nil {
			writeError(w, apiErrWithReqID(apiErr, reqID))
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// 覆盖模式：重新解析成结构化对象
	body, _ := json.Marshal(raw)
	var req OverwriteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "parse_json",
			fmt.Sprintf("invalid OverwriteRequest: %v", err)), reqID))
		return
	}
	resp, apiErr := s.deployer.OverwriteVersion(r.Context(), code, version, req)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	s.rewriteDeployResponseURLs(r, resp)
	writeJSON(w, http.StatusOK, resp)
}

// handleDeleteVersion 处理 DELETE /api/deploys/{code}/versions/{version}。
func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	version, ok := parseVersionPath(w, r, reqID)
	if !ok {
		return
	}
	if apiErr := s.authorizeSiteWrite(r, code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	resp, apiErr := s.deployer.DeleteVersion(r.Context(), code, version)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetContent 处理 GET /api/deploy/content?code=&version=&download=1。
// download=1 时直接 stream 内容：
//   - 单文件 site：直接 serve 主入口（HTML 文本）
//   - 多文件 site：打包成 zip 下载
func (s *Server) handleGetContent(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	q := r.URL.Query()
	code := strings.TrimSpace(q.Get("code"))
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
			"query parameter 'code' is required"), reqID))
		return
	}
	var versionPtr *int64
	if vs := strings.TrimSpace(q.Get("version")); vs != "" {
		v, err := parseInt64(vs)
		if err != nil || v <= 0 {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"'version' must be a positive integer"), reqID))
			return
		}
		versionPtr = &v
	}

	if !s.siteAccessAllowed(r, code, versionPtr) {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "access_password",
			"this site is password protected"), reqID))
		return
	}

	// download 模式
	if dl, _ := parseBoolParam(q.Get("download")); dl {
		if apiErr := s.deployer.StreamDownload(r.Context(), code, versionPtr, w); apiErr != nil {
			// 此时响应头可能已写，但出错时一般没写。
			writeError(w, apiErrWithReqID(apiErr, reqID))
		}
		return
	}

	resp, apiErr := s.deployer.GetContent(r.Context(), code, versionPtr)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// parseBoolParam 把 "1"/"true"/"yes"（大小写不敏感）识别为 true。
func parseBoolParam(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "0", "false", "no", "off":
		return false, nil
	case "1", "true", "yes", "on":
		return true, nil
	}
	return false, fmt.Errorf("invalid boolean: %q", s)
}

// ===== 共用工具 =====

// parseVersionPath 把路径中的 {version} 解析为 int64。失败时已写错误响应。
func parseVersionPath(w http.ResponseWriter, r *http.Request, reqID string) (int64, bool) {
	v := r.PathValue("version")
	if v == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			"missing version in path"), reqID))
		return 0, false
	}
	n, err := parseInt64(v)
	if err != nil || n <= 0 {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			fmt.Sprintf("invalid version %q: must be a positive integer", v)), reqID))
		return 0, false
	}
	return n, true
}

// decodeJSONBody 校验 Content-Type + 读取 JSON body 到 dst。失败时已写错误响应。
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, reqID string) error {
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if !strings.HasPrefix(ct, "application/json") {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "content_type",
			"Content-Type must be application/json"), reqID))
		return fmt.Errorf("content-type not json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "parse_json",
			fmt.Sprintf("invalid JSON body: %v", err)), reqID))
		return err
	}
	return nil
}

// parseInt64 解析字符串为 int64。
func parseInt64(s string) (int64, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a digit: %q", c)
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// writeJSON 写状态码 + JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// clientIPFromRequest 从 r.RemoteAddr / X-Forwarded-For 提取客户端 IP。
// 仅取 XFF 第一个（最左边是原始客户端）；RemoteAddr 兜底。
// 这是简化版，生产环境 Caddy 后会传 X-Forwarded-For。
func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// 第一个就是客户端
		if idx := strings.Index(xff, ","); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if rfn := r.Header.Get("X-Real-IP"); rfn != "" {
		return strings.TrimSpace(rfn)
	}
	// RemoteAddr 形如 "1.2.3.4:5678"
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return strings.Trim(host, "[]")
}

// ===== Token 管理处理程序（Day 5） =====

// handleCreateToken 处理 POST /api/token。
// 创建一个新 token，返回明文（仅此一次可见）。
// 必须使用已登录账号或 Bearer Token；普通用户只能给自己创建普通 Token。
func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())

	ownerUserID := ""
	requesterUserID := ""
	isRequesterAdmin := false
	if user, ok := s.adminUserFromCookie(r); ok {
		isRequesterAdmin = user.IsAdmin
		requesterUserID = user.ID
	} else if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		tok, authErr := s.authenticateToken(r)
		if authErr != nil {
			writeError(w, apiErrWithReqID(authErr, reqID))
			return
		}
		isRequesterAdmin = tok.IsAdmin
		requesterUserID = tok.OwnerUserID
	} else {
		writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "auth", "admin account or token required"), reqID))
		return
	}

	var req TokenCreateRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}

	ownerUserID = strings.TrimSpace(req.OwnerUserID)
	if ownerUserID == "" {
		ownerUserID = requesterUserID
	}
	if !isRequesterAdmin {
		if req.IsAdmin {
			writeError(w, apiErrWithReqID(NewError(CodeForbidden, "auth", "admin account required to create admin tokens"), reqID))
			return
		}
		if ownerUserID == "" {
			writeError(w, apiErrWithReqID(NewError(CodeForbidden, "auth", "token must belong to your user account"), reqID))
			return
		}
		if ownerUserID != requesterUserID {
			writeError(w, apiErrWithReqID(NewError(CodeForbidden, "owner", "you can only create tokens for your own user account"), reqID))
			return
		}
	}
	if ownerUserID != "" {
		if user, err := s.auth.GetUser(r.Context(), ownerUserID); err != nil || !user.IsActive {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "owner", "ownerUserId is not an active user"), reqID))
			return
		}
	}
	if ownerUserID == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "owner", "token must belong to a user"), reqID))
		return
	}
	expiresAt, parseErr := parseTokenExpiresAt(req)
	if parseErr != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "expiresAt", parseErr.Error()), reqID))
		return
	}

	gen, err := s.auth.Generate(r.Context(), strings.TrimSpace(req.Label), req.IsAdmin, ownerUserID, expiresAt)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "create_token",
			fmt.Sprintf("failed to create token: %v", err)), reqID))
		return
	}
	writeJSON(w, http.StatusOK, TokenCreateResponse{
		Success:     true,
		ID:          gen.ID,
		Token:       gen.Plaintext,
		Label:       gen.Label,
		IsAdmin:     gen.IsAdmin,
		OwnerUserID: gen.OwnerUserID,
		ExpiresAt:   gen.ExpiresAt,
		CreatedAt:   gen.CreatedAt,
	})
}

func parseTokenExpiresAt(req TokenCreateRequest) (*time.Time, error) {
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
		if err != nil {
			return nil, fmt.Errorf("expiresAt must be RFC3339")
		}
		if !t.After(time.Now()) {
			return nil, fmt.Errorf("expiresAt must be in the future")
		}
		return &t, nil
	}
	if req.TTLSeconds != nil {
		if *req.TTLSeconds <= 0 {
			return nil, fmt.Errorf("ttlSeconds must be positive")
		}
		t := time.Now().UTC().Add(time.Duration(*req.TTLSeconds) * time.Second)
		return &t, nil
	}
	return nil, nil
}

// handleListTokens 处理 GET /api/tokens。
func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())

	actor, isAdmin, authErr := s.optionalActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	actorUserID := strings.TrimPrefix(actor, "user:")
	if actor == "" {
		writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "auth", "login or bearer token required"), reqID))
		return
	}
	if !isAdmin && actorUserID == "" {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "auth", "user account required"), reqID))
		return
	}

	toks, err := s.auth.List(r.Context(), false)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "list_tokens",
			fmt.Sprintf("failed to list tokens: %v", err)), reqID))
		return
	}
	items := make([]TokenListItem, 0, len(toks))
	usernames := map[string]string{}
	if users, err := s.auth.ListUsers(r.Context()); err == nil {
		for _, user := range users {
			usernames[user.ID] = user.Username
		}
	}
	for _, t := range toks {
		if strings.TrimSpace(t.OwnerUserID) == "" {
			continue
		}
		if !isAdmin && t.OwnerUserID != actorUserID {
			continue
		}
		items = append(items, TokenListItem{
			ID:            t.ID,
			Label:         t.Label,
			IsAdmin:       t.IsAdmin,
			IsRevoked:     t.IsRevoked,
			OwnerUserID:   t.OwnerUserID,
			OwnerUsername: usernames[t.OwnerUserID],
			ExpiresAt:     t.ExpiresAt,
			CreatedAt:     t.CreatedAt,
			LastUsedAt:    t.LastUsedAt,
		})
	}
	writeJSON(w, http.StatusOK, TokenListResponse{Success: true, Tokens: items})
}

// handleRevokeToken 处理 DELETE /api/tokens/{id}。
func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	actor, isAdmin, authErr := s.optionalActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	actorUserID := strings.TrimPrefix(actor, "user:")
	if actor == "" {
		writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "auth", "login or bearer token required"), reqID))
		return
	}
	if !isAdmin && actorUserID == "" {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "auth", "user account required"), reqID))
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			"missing token id in path"), reqID))
		return
	}
	if !isAdmin {
		tok, err := s.auth.GetToken(r.Context(), id)
		if err != nil || tok.OwnerUserID != actorUserID {
			writeError(w, apiErrWithReqID(NewError(CodeForbidden, "auth", "you can only revoke your own tokens"), reqID))
			return
		}
	}
	if err := s.auth.Revoke(r.Context(), id); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeNotFound, "revoke_token",
			fmt.Sprintf("token %q not found or already revoked", id)), reqID))
		return
	}
	writeJSON(w, http.StatusOK, TokenRevokeResponse{Success: true, ID: id})
}

func (s *Server) authenticate(r *http.Request) (string, *APIError) {
	tok, authErr := s.authenticateToken(r)
	if authErr != nil {
		return "", authErr
	}
	return ownerForToken(tok), nil
}

func (s *Server) authenticateToken(r *http.Request) (store.Token, *APIError) {
	tok, err := s.auth.VerifyToken(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrMissing):
			return store.Token{}, NewError(CodeUnauthorized, "auth", "Authorization header required").
				WithHint("Use 'Authorization: Bearer <token>'.")
		case errors.Is(err, auth.ErrMalformed):
			return store.Token{}, NewError(CodeUnauthorized, "auth", "Authorization header malformed")
		case errors.Is(err, auth.ErrUnknown):
			return store.Token{}, NewError(CodeUnauthorized, "auth", "token not recognized")
		case errors.Is(err, auth.ErrRevoked):
			return store.Token{}, NewError(CodeForbidden, "auth", "token has been revoked")
		case errors.Is(err, auth.ErrExpired):
			return store.Token{}, NewError(CodeUnauthorized, "auth", "token has expired")
		}
		return store.Token{}, NewError(CodeUnauthorized, "auth", err.Error())
	}
	return tok, nil
}

func anonymousSessionIDFromRequest(r *http.Request) string {
	id := strings.TrimSpace(r.Header.Get("X-Hostctl-Session"))
	if id == "" {
		if c, err := r.Cookie("hostctl_anon_session"); err == nil {
			id = strings.TrimSpace(c.Value)
		}
	}
	return id
}

func (s *Server) authenticateAdmin(r *http.Request) (store.Token, *APIError) {
	if c, err := r.Cookie("hostctl_admin_session"); err == nil && strings.TrimSpace(c.Value) != "" {
		user, _, err := s.auth.VerifyAdminSession(r.Context(), c.Value)
		if err == nil {
			if !user.IsAdmin {
				return store.Token{}, NewError(CodeForbidden, "auth", "admin account required")
			}
			return store.Token{ID: "admin:" + user.ID, Label: user.Username, IsAdmin: true, OwnerUserID: user.ID}, nil
		}
		if !errors.Is(err, auth.ErrMissing) && !errors.Is(err, auth.ErrUnknown) && !errors.Is(err, auth.ErrExpired) && !errors.Is(err, auth.ErrRevoked) {
			return store.Token{}, NewError(CodeUnauthorized, "auth", err.Error())
		}
	}
	tok, authErr := s.authenticateToken(r)
	if authErr != nil {
		return store.Token{}, authErr
	}
	if !tok.IsAdmin {
		return store.Token{}, NewError(CodeForbidden, "auth", "admin token required").
			WithHint("Sign in with a token created using isAdmin=true.")
	}
	return tok, nil
}

func (s *Server) adminUserFromCookie(r *http.Request) (store.AdminUser, bool) {
	c, err := r.Cookie("hostctl_admin_session")
	if err != nil || strings.TrimSpace(c.Value) == "" {
		return store.AdminUser{}, false
	}
	user, _, err := s.auth.VerifyAdminSession(r.Context(), c.Value)
	return user, err == nil
}

func (s *Server) siteAccessAllowed(r *http.Request, code string, versionPtr *int64) bool {
	site, err := s.deployer.GetSite(r.Context(), code)
	if err != nil || site.AccessPasswordHash == "" {
		return true
	}
	if c, err := r.Cookie(siteAccessCookieName(code)); err == nil &&
		validSiteAccessCookie(c.Value, code, site.AccessPasswordHash, time.Now()) {
		return true
	}
	if s.validScreenAccessCookie(r, code, versionPtr, time.Now()) {
		return true
	}
	if authErr := s.authorizeSiteWrite(r, code); authErr == nil {
		return true
	}
	return false
}

func (s *Server) renderAccessPasswordPage(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>需要访问密码 - PagePilot</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f6fbff;color:#0f172a;font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.card{width:min(420px,calc(100%% - 32px));border:1px solid #dbeafe;border-radius:16px;background:#fff;box-shadow:0 22px 60px rgba(14,116,144,.12);padding:24px}h1{margin:0;font-size:22px}.muted{color:#64748b;line-height:1.7}input{width:100%%;height:42px;border:1px solid #cbd5e1;border-radius:10px;padding:0 12px;font:inherit}button{width:100%%;height:42px;margin-top:12px;border:0;border-radius:10px;background:#0284c7;color:white;font-weight:800;cursor:pointer}.err{min-height:20px;color:#be123c;font-size:13px}</style></head><body><form class="card" id="f"><h1>这个网页已加密</h1><p class="muted">请输入访问密码后继续查看。</p><input id="p" type="password" autocomplete="current-password" placeholder="访问密码" autofocus><button type="submit">进入网页</button><p class="err" id="e"></p></form><script>document.getElementById("f").addEventListener("submit",async e=>{e.preventDefault();const r=await fetch("/api/deploys/%s/access",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({password:document.getElementById("p").value})});if(r.ok) location.reload(); else document.getElementById("e").textContent="密码不正确";});</script></body></html>`, code)
}

func (s *Server) renderAccessPasswordPageV2(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>需要访问密码 - PagePilot</title>
<style>
*{box-sizing:border-box}body{margin:0;min-height:100vh;color:#0f172a;font-family:Inter,"Noto Sans SC",system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:radial-gradient(circle at 18%% 14%%,rgba(34,211,238,.28),transparent 30%%),radial-gradient(circle at 82%% 12%%,rgba(129,140,248,.24),transparent 30%%),linear-gradient(180deg,#e8f7ff 0%%,#f8fbff 54%%,#fff 100%%);display:grid;place-items:center;padding:24px}.card{width:min(460px,100%%);border:1px solid rgba(255,255,255,.86);border-radius:24px;background:rgba(255,255,255,.84);box-shadow:0 24px 70px rgba(14,116,144,.16);backdrop-filter:blur(18px);padding:30px}.brand{display:flex;align-items:center;gap:12px;margin-bottom:26px;font-weight:900;font-size:22px}.logo{width:44px;height:44px;border-radius:14px;display:grid;place-items:center;background:linear-gradient(145deg,#101348,#155e75);color:#fff;box-shadow:0 12px 26px rgba(15,23,42,.18)}.lock{width:58px;height:58px;border-radius:18px;display:grid;place-items:center;background:#ecfeff;color:#0369a1;border:1px solid #bae6fd;margin-bottom:18px}h1{margin:0;font-size:26px;line-height:1.18;letter-spacing:0;font-weight:900}p{margin:10px 0 22px;color:#64748b;line-height:1.7}.field{display:grid;gap:8px}label{font-size:13px;font-weight:800;color:#334155}input{width:100%%;height:48px;border:1px solid #cbd5e1;border-radius:14px;background:#fff;padding:0 14px;font:inherit;outline:none;transition:border-color .16s,box-shadow .16s}input:focus{border-color:#38bdf8;box-shadow:0 0 0 4px rgba(56,189,248,.16)}button{width:100%%;height:48px;margin-top:14px;border:0;border-radius:14px;background:#0f172a;color:#fff;font-weight:900;cursor:pointer;transition:transform .16s,background .16s}button:hover{background:#1e293b;transform:translateY(-1px)}button:disabled{opacity:.62;cursor:not-allowed;transform:none}.err{min-height:20px;margin:10px 0 0;color:#be123c;font-size:13px}.foot{margin-top:18px;text-align:center;font-size:12px;color:#94a3b8}.home{color:#0369a1;font-weight:800;text-decoration:none}
</style>
</head>
<body>
<form class="card" id="f">
<div class="brand"><div class="logo">P</div><span>PagePilot</span></div>
<div class="lock"><svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg></div>
<h1>这个网页已加密</h1>
<p>请输入访问密码后继续查看。验证通过后，本浏览器会在一段时间内记住访问权限。</p>
<div class="field">
<label for="p">访问密码</label>
<input id="p" type="password" autocomplete="current-password" placeholder="输入访问密码" autofocus>
</div>
<button id="submit" type="submit">进入网页</button>
<p class="err" id="e"></p>
<div class="foot"><a class="home" href="/">返回 PagePilot 首页</a></div>
</form>
<script>
const form=document.getElementById("f");
const btn=document.getElementById("submit");
const err=document.getElementById("e");
form.addEventListener("submit",async e=>{
  e.preventDefault();
  err.textContent="";
  btn.disabled=true;
  btn.textContent="验证中...";
  try{
    const r=await fetch("/api/deploys/%s/access",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({password:document.getElementById("p").value})});
    if(r.ok){location.reload();return}
    err.textContent="密码不正确，请重新输入。";
  }catch(_){
    err.textContent="验证失败，请稍后再试。";
  }finally{
    btn.disabled=false;
    btn.textContent="进入网页";
  }
});
</script>
</body>
</html>`, code)
}

func (s *Server) authenticateActor(r *http.Request) (string, bool, *APIError) {
	if user, ok := s.adminUserFromCookie(r); ok {
		return "user:" + user.ID, user.IsAdmin, nil
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		actor, ok, err := s.anonymousActor(r)
		if err != nil {
			return "", false, err
		}
		if ok {
			return actor, false, nil
		}
		return "", false, NewError(CodeUnauthorized, "auth", "Authorization header, admin session, or anonymous session required")
	}
	tok, authErr := s.authenticateToken(r)
	if authErr != nil {
		return "", false, authErr
	}
	return ownerForToken(tok), tok.IsAdmin, nil
}

func (s *Server) optionalActor(r *http.Request) (string, bool, *APIError) {
	if user, ok := s.adminUserFromCookie(r); ok {
		return "user:" + user.ID, user.IsAdmin, nil
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		actor, ok, err := s.anonymousActor(r)
		if err != nil {
			return "", false, err
		}
		if ok {
			return actor, false, nil
		}
		return "", false, nil
	}
	tok, authErr := s.authenticateToken(r)
	if authErr != nil {
		return "", false, authErr
	}
	return ownerForToken(tok), tok.IsAdmin, nil
}

func ownerForToken(tok store.Token) string {
	return "user:" + strings.TrimSpace(tok.OwnerUserID)
}

func (s *Server) anonymousActor(r *http.Request) (string, bool, *APIError) {
	id := anonymousSessionIDFromRequest(r)
	if id == "" {
		return "", false, nil
	}
	sess, err := s.deployer.GetAnonymousSession(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", false, NewError(CodeUnauthorized, "anonymous_session", "anonymous session not found")
		}
		return "", false, NewError(CodeInternal, "anonymous_session", err.Error())
	}
	if strings.TrimSpace(sess.ClaimedByUserID) != "" {
		return "", false, NewError(CodeUnauthorized, "anonymous_session", "anonymous session has been claimed")
	}
	return "anon:" + id, true, nil
}

func (s *Server) authorizeSiteWrite(r *http.Request, code string) *APIError {
	actor, isAdmin, authErr := s.authenticateActor(r)
	if authErr != nil {
		return authErr
	}
	if isAdmin {
		return nil
	}
	site, err := s.deployer.GetSite(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		return NewError(CodeNotFound, "site", "site not found")
	}
	if err != nil {
		return NewError(CodeInternal, "site", err.Error())
	}
	if site.OwnerTokenID != actor {
		return NewError(CodeForbidden, "owner", "you can only modify sites you own").
			WithHint("Use the token or account that created this site, or ask an admin.")
	}
	return nil
}

func (s *Server) authorizeDeployCustomCode(r *http.Request, code, owner string) *APIError {
	site, err := s.deployer.GetSite(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return NewError(CodeInternal, "site", err.Error())
	}
	_, isAdmin, authErr := s.authenticateActor(r)
	if authErr == nil && isAdmin {
		return nil
	}
	if site.OwnerTokenID != owner {
		return NewError(CodeForbidden, "owner", "you can only append versions to sites you own").
			WithHint("Use the same account, token, or anonymous session that created this site.")
	}
	return nil
}

// ===== Day 7：管理 UI + 配置 =====

// handleAdminUI 返回单页 admin HTML（embed 进二进制）。
// 不强制鉴权——UI 内部用 token 调 API；token 错就显示提示。
func (s *Server) handleAdminUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(web.AdminHTML())
}

func (s *Server) handleAgentGuideUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	bytes, err := fs.ReadFile(web.UserSubFS(), "agents.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write(bytes)
}

func (s *Server) handleScreenGuideUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	bytes, err := fs.ReadFile(web.UserSubFS(), "screens.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write(bytes)
}

func (s *Server) handleSkillDownload(w http.ResponseWriter, r *http.Request) {
	path := s.managedSkillZipPath()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="hostctl-deploy-skill.zip"`)
	http.ServeFile(w, r, path)
}

type adminSkillFile struct {
	Path      string `json:"path"`
	Label     string `json:"label"`
	Size      int64  `json:"size"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type adminSkillResponse struct {
	Success bool              `json:"success"`
	Path    string            `json:"path"`
	Content string            `json:"content"`
	Files   []adminSkillFile  `json:"files"`
	Package adminSkillPackage `json:"package"`
}

type adminSkillPackage struct {
	Exists    bool   `json:"exists"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Sha256    string `json:"sha256,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type adminSkillUpdateRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (s *Server) handleAdminGetSkill(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		rel = "SKILL.md"
	}
	full, label, apiErr := skillEditablePath(rel)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "skill", err.Error()), reqID))
		return
	}
	files := skillEditableFiles()
	for i := range files {
		if files[i].Path == rel {
			files[i].Label = label
		}
	}
	writeJSON(w, http.StatusOK, adminSkillResponse{
		Success: true,
		Path:    rel,
		Content: string(data),
		Files:   files,
		Package: s.skillPackageInfo(),
	})
}

func (s *Server) handleAdminPutSkill(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req adminSkillUpdateRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	full, _, apiErr := skillEditablePath(req.Path)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	if len(req.Content) > 512*1024 {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "skill", "skill file is too large"), reqID))
		return
	}
	if err := os.WriteFile(full, []byte(req.Content), 0o644); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "skill", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "path": req.Path})
}

func (s *Server) handleAdminUploadSkillPackage(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	if err := r.ParseMultipartForm(21 << 20); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "skill_package", "multipart file is required"), reqID))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "skill_package", "file field is required"), reqID))
		return
	}
	defer file.Close()
	name := strings.ToLower(strings.TrimSpace(header.Filename))
	if !strings.HasSuffix(name, ".zip") {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "skill_package", "skill package must be a .zip file"), reqID))
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "skill_package", err.Error()), reqID))
		return
	}
	if len(data) == 0 || len(data) > 20<<20 {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "skill_package", "skill package size must be between 1 byte and 20 MB"), reqID))
		return
	}
	if err := validateSkillZip(data); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "skill_package", err.Error()), reqID))
		return
	}
	path := s.managedSkillZipPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "skill_package", err.Error()), reqID))
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hostctl-deploy-*.zip")
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "skill_package", err.Error()), reqID))
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "skill_package", err.Error()), reqID))
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "skill_package", err.Error()), reqID))
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "skill_package", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"package": s.skillPackageInfo(),
	})
}

func validateSkillZip(data []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("invalid zip package")
	}
	if len(zr.File) == 0 {
		return fmt.Errorf("zip package is empty")
	}
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if strings.HasPrefix(name, "/") || strings.Contains(name, "../") || strings.Contains(name, `\`) {
			return fmt.Errorf("zip package contains unsafe path %q", f.Name)
		}
	}
	return nil
}

func skillEditablePath(rel string) (string, string, *APIError) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	allowed := map[string]string{
		"SKILL.md":                  "Skill 说明",
		"scripts/hostctl_deploy.py": "Python 脚本",
	}
	label, ok := allowed[rel]
	if !ok {
		return "", "", NewError(CodeInvalidInput, "skill", "skill file is not editable")
	}
	return filepath.Join(skillRoot(), filepath.FromSlash(rel)), label, nil
}

func skillEditableFiles() []adminSkillFile {
	paths := []struct {
		rel   string
		label string
	}{
		{"SKILL.md", "Skill 说明"},
		{"scripts/hostctl_deploy.py", "Python 脚本"},
	}
	files := make([]adminSkillFile, 0, len(paths))
	for _, p := range paths {
		item := adminSkillFile{Path: p.rel, Label: p.label}
		if st, err := os.Stat(filepath.Join(skillRoot(), filepath.FromSlash(p.rel))); err == nil {
			item.Size = st.Size()
			item.UpdatedAt = st.ModTime().Format(time.RFC3339)
		}
		files = append(files, item)
	}
	return files
}

func (s *Server) managedSkillZipPath() string {
	if v := strings.TrimSpace(os.Getenv("HOSTCTL_SKILL_ZIP")); v != "" {
		return v
	}
	base := filepath.Dir(s.cfg.DBPath)
	if base == "." || base == "" {
		base = filepath.Join("data")
	}
	return filepath.Join(base, "skill", "hostctl-deploy.zip")
}

func (s *Server) skillPackageInfo() adminSkillPackage {
	path := s.managedSkillZipPath()
	info := adminSkillPackage{Path: path}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return info
	}
	info.Exists = true
	info.Size = st.Size()
	info.UpdatedAt = st.ModTime().UTC().Format(time.RFC3339)
	if data, err := os.ReadFile(path); err == nil {
		sum := sha256.Sum256(data)
		info.Sha256 = hex.EncodeToString(sum[:])
	}
	return info
}

func skillRoot() string {
	if v := strings.TrimSpace(os.Getenv("HOSTCTL_SKILL_DIR")); v != "" {
		return v
	}
	return filepath.Join("skill", "hostctl-deploy")
}

// handleAdminSession validates the admin UI bearer token or admin cookie.
func (s *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	mode := "prod"
	if !s.requireAuth {
		mode = "dev"
	}
	hasAdmin, err := s.auth.HasAdminUser(r.Context())
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "auth", err.Error()), requestIDFromContext(r.Context())))
		return
	}
	if c, err := r.Cookie("hostctl_admin_session"); err == nil && strings.TrimSpace(c.Value) != "" {
		user, _, err := s.auth.VerifyAdminSession(r.Context(), c.Value)
		if err == nil {
			writeJSON(w, http.StatusOK, AdminSessionResponse{
				Success:     true,
				Mode:        mode,
				UserID:      user.ID,
				Username:    user.Username,
				Label:       user.Username,
				IsAdmin:     user.IsAdmin,
				LoginMethod: "password",
			})
			return
		}
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		writeJSON(w, http.StatusUnauthorized, AdminSessionResponse{
			Success:    false,
			Mode:       mode,
			NeedsSetup: !hasAdmin,
		})
		return
	}

	tok, err := s.auth.VerifyToken(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		apiErr := NewError(CodeUnauthorized, "auth", "Authorization header required").
			WithHint("Use an admin token to sign in.")
		switch {
		case errors.Is(err, auth.ErrMalformed):
			apiErr = NewError(CodeUnauthorized, "auth", "Authorization header malformed")
		case errors.Is(err, auth.ErrUnknown):
			apiErr = NewError(CodeUnauthorized, "auth", "token not recognized")
		case errors.Is(err, auth.ErrRevoked):
			apiErr = NewError(CodeForbidden, "auth", "token has been revoked")
		case errors.Is(err, auth.ErrExpired):
			apiErr = NewError(CodeUnauthorized, "auth", "token has expired")
		}
		writeError(w, apiErrWithReqID(apiErr, requestIDFromContext(r.Context())))
		return
	}
	var username string
	isAdmin := tok.IsAdmin
	if strings.TrimSpace(tok.OwnerUserID) != "" {
		user, userErr := s.auth.GetUser(r.Context(), tok.OwnerUserID)
		if userErr != nil || !user.IsActive {
			writeError(w, apiErrWithReqID(NewError(CodeForbidden, "auth", "token owner is inactive or missing"), requestIDFromContext(r.Context())))
			return
		}
		username = user.Username
		isAdmin = isAdmin || user.IsAdmin
	}

	writeJSON(w, http.StatusOK, AdminSessionResponse{
		Success:     true,
		Mode:        mode,
		TokenID:     tok.ID,
		UserID:      tok.OwnerUserID,
		Username:    username,
		Label:       tok.Label,
		IsAdmin:     isAdmin,
		LoginMethod: "token",
	})
}

func (s *Server) handleCaptcha(w http.ResponseWriter, r *http.Request) {
	a := randomIntRange(2, 18)
	b := randomIntRange(2, 18)
	id := "cap_" + randomHex(12)
	s.captchaMu.Lock()
	now := time.Now()
	for key, item := range s.captchas {
		if now.After(item.ExpiresAt) {
			delete(s.captchas, key)
		}
	}
	s.captchas[id] = captchaChallenge{
		Answer:    strconv.Itoa(a + b),
		ExpiresAt: now.Add(5 * time.Minute),
	}
	s.captchaMu.Unlock()
	writeJSON(w, http.StatusOK, CaptchaResponse{
		Success: true,
		ID:      id,
		Prompt:  fmt.Sprintf("%d + %d = ?", a, b),
	})
}

func randomIntRange(min, max int) int {
	if max <= min {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}

func (s *Server) verifyCaptcha(id, answer string) bool {
	id = strings.TrimSpace(id)
	answer = strings.TrimSpace(answer)
	if id == "" || answer == "" {
		return false
	}
	s.captchaMu.Lock()
	defer s.captchaMu.Unlock()
	item, ok := s.captchas[id]
	delete(s.captchas, id)
	if !ok || time.Now().After(item.ExpiresAt) {
		return false
	}
	return subtleConstantTimeString(item.Answer, answer)
}

func subtleConstantTimeString(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	var req RegisterRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if !s.verifyCaptcha(req.CaptchaID, req.Captcha) {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "captcha", "captcha is incorrect or expired"), reqID))
		return
	}
	user, err := s.auth.CreateUser(r.Context(), req.Username, req.Password, false, 20)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "register", "username already exists or password is invalid"), reqID))
		return
	}
	s.claimCurrentAnonymousSession(r, user.ID)
	writeJSON(w, http.StatusOK, RegisterResponse{Success: true, UserID: user.ID, Username: user.Username})
}

func (s *Server) handleAccountPassword(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	user, ok := s.adminUserFromCookie(r)
	if !ok {
		writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "auth", "login required"), reqID))
		return
	}
	var req AccountPasswordRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "password", "new password must be at least 8 characters"), reqID))
		return
	}
	if err := s.auth.ChangeUserPassword(r.Context(), user.ID, req.OldPassword, req.NewPassword); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "password", "old password is incorrect"), reqID))
		return
	}
	writeJSON(w, http.StatusOK, AccountPasswordResponse{Success: true})
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	var req AdminLoginRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if !s.verifyCaptcha(req.CaptchaID, req.Captcha) {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "captcha", "captcha is incorrect or expired"), reqID))
		return
	}
	res, err := s.auth.LoginAdmin(r.Context(), req.Username, req.Password, 7*24*time.Hour)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "auth", "username or password is incorrect"), reqID))
		return
	}
	s.claimCurrentAnonymousSession(r, res.User.ID)
	setAdminSessionCookie(w, res.Plaintext, int((7 * 24 * time.Hour).Seconds()))
	mode := "prod"
	if !s.requireAuth {
		mode = "dev"
	}
	writeJSON(w, http.StatusOK, AdminLoginResponse{
		Success:  true,
		Mode:     mode,
		UserID:   res.User.ID,
		Username: res.User.Username,
		IsAdmin:  res.User.IsAdmin,
	})
}

func (s *Server) claimCurrentAnonymousSession(r *http.Request, userID string) {
	sessionID := anonymousSessionIDFromRequest(r)
	if sessionID == "" {
		return
	}
	if _, err := s.deployer.ClaimAnonymousSession(r.Context(), sessionID, userID); err != nil {
		s.logger.Printf("failed to claim anonymous session %s for user %s: %v", sessionID, userID, err)
	}
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("hostctl_admin_session"); err == nil {
		_ = s.auth.RevokeAdminSession(r.Context(), c.Value)
	}
	setAdminSessionCookie(w, "", -1)
	writeJSON(w, http.StatusOK, AdminLogoutResponse{Success: true})
}

func (s *Server) handleAdminSetup(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	hasAdmin, err := s.auth.HasAdminUser(r.Context())
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "auth", err.Error()), reqID))
		return
	}
	if hasAdmin {
		if _, authErr := s.authenticateAdmin(r); authErr != nil {
			writeError(w, apiErrWithReqID(authErr, reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "auth", "admin account already exists"), reqID))
		return
	}
	var req AdminSetupRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if !s.verifyCaptcha(req.CaptchaID, req.Captcha) {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "captcha", "captcha is incorrect or expired"), reqID))
		return
	}
	user, err := s.auth.CreateFirstAdmin(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "auth", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, AdminSetupResponse{Success: true, UserID: user.ID, Username: user.Username})
}

// handleGetConfig 返回当前生效的配置。
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	mode := "prod"
	if !s.requireAuth {
		mode = "dev"
	}
	currentBase := s.requestBaseURL(r)
	writeJSON(w, http.StatusOK, ConfigResponse{
		Success:           true,
		CurrentBaseURL:    currentBase,
		AppURL:            s.appURLConfigForRequest(r),
		Mode:              mode,
		CORSAllowOrigins:  s.cfg.CORSAllowOrigins,
		EmbedPolicy:       config.NormalizeEmbedPolicy(s.cfg.EmbedPolicy),
		EmbedAllowOrigins: s.cfg.EmbedAllowOrigins,
		CooldownSeconds:   s.cfg.CooldownSeconds,
		Limits: Limits{
			MaxSingleFileBytes: s.cfg.MaxSingleFileBytes,
			MaxSiteTotalBytes:  s.cfg.MaxSiteTotalBytes,
			MaxFilesPerSite:    s.cfg.MaxFilesPerSite,
		},
		AnonymousPolicy: AnonymousPolicy{
			DeployLimit: s.cfg.AnonymousDeployLimit,
		},
		Version: s.version,
	})
}

func (s *Server) handleAdminAnonymousSessions(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
	sessions, err := s.deployer.ListAnonymousSessions(r.Context(), limit)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "anonymous_sessions", err.Error()), reqID))
		return
	}
	items := make([]AnonymousSessionListItem, 0, len(sessions))
	for _, sess := range sessions {
		remaining := s.cfg.AnonymousDeployLimit - sess.DeployCount
		if remaining < 0 {
			remaining = 0
		}
		items = append(items, AnonymousSessionListItem{
			ID:              sess.ID,
			AgentID:         sess.AgentID,
			AgentLabel:      sess.AgentLabel,
			DeviceIP:        sess.DeviceIP,
			UserAgent:       sess.UserAgent,
			DeployCount:     sess.DeployCount,
			Remaining:       remaining,
			ClaimedByUserID: sess.ClaimedByUserID,
			ClaimedAt:       sess.ClaimedAt,
			CreatedAt:       sess.CreatedAt,
			LastUsedAt:      sess.LastUsedAt,
		})
	}
	writeJSON(w, http.StatusOK, AnonymousSessionListResponse{
		Success:     true,
		DeployLimit: s.cfg.AnonymousDeployLimit,
		Sessions:    items,
	})
}

// handlePutConfig 更新可热修改的运行时配置。
func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())

	// 鉴权：运行配置属于后台管理能力，dev 模式也必须登录管理员。
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}

	var req ConfigUpdateRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}

	if req.AppURLMode != nil || req.AppDomainSuffix != nil || req.AppURLScheme != nil || req.AppURLPort != nil {
		next := s.appURLConfig()
		if req.AppURLMode != nil {
			next.AppURLMode = normalizeAppURLMode(*req.AppURLMode)
		}
		if req.AppDomainSuffix != nil {
			next.AppDomainSuffix = normalizeAppDomainSuffix(*req.AppDomainSuffix)
		}
		if req.AppURLScheme != nil {
			next.AppURLScheme = normalizeAppURLScheme(*req.AppURLScheme)
		}
		if req.AppURLPort != nil {
			rawPort := strings.TrimSpace(*req.AppURLPort)
			next.AppURLPort = normalizeAppURLPort(rawPort)
			if rawPort != "" && next.AppURLPort == "" {
				writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
					"appURLPort must be a number between 1 and 65535"), reqID))
				return
			}
		}
		if (next.AppURLMode == AppURLModeDomain || next.AppURLMode == AppURLModeDual) && next.AppDomainSuffix == "" {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"appDomainSuffix is required when appURLMode is domain or dual"), reqID))
			return
		}
		setter, ok := s.deployer.(appURLConfigSetter)
		if !ok {
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "set_config",
				"deployer does not support app URL config"), reqID))
			return
		}
		if err := setter.SetAppURLConfig(r.Context(), next); err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist app URL config: %v", err)), reqID))
			return
		}
		s.cfg.AppURLMode = next.AppURLMode
		s.cfg.AppDomainSuffix = next.AppDomainSuffix
		s.cfg.AppURLScheme = next.AppURLScheme
		s.cfg.AppURLPort = next.AppURLPort
	}
	if req.AnonymousDeployLimit != nil {
		if *req.AnonymousDeployLimit < -1 {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"anonymousDeployLimit must be -1 or greater"), reqID))
			return
		}
		if err := s.deployer.SetAnonymousDeployLimit(r.Context(), *req.AnonymousDeployLimit); err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist anonymous limit: %v", err)), reqID))
			return
		}
		s.cfg.AnonymousDeployLimit = *req.AnonymousDeployLimit
	}
	if req.CooldownSeconds != nil {
		if *req.CooldownSeconds < 0 || *req.CooldownSeconds > 3600 {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"cooldownSeconds must be between 0 and 3600"), reqID))
			return
		}
		if err := s.deployer.SetCooldownSeconds(r.Context(), *req.CooldownSeconds); err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist cooldown: %v", err)), reqID))
			return
		}
		s.cfg.CooldownSeconds = *req.CooldownSeconds
	}
	if req.MaxSingleFileBytes != nil || req.MaxSiteTotalBytes != nil || req.MaxFilesPerSite != nil {
		single := s.cfg.MaxSingleFileBytes
		total := s.cfg.MaxSiteTotalBytes
		files := s.cfg.MaxFilesPerSite
		if req.MaxSingleFileBytes != nil {
			single = *req.MaxSingleFileBytes
		}
		if req.MaxSiteTotalBytes != nil {
			total = *req.MaxSiteTotalBytes
		}
		if req.MaxFilesPerSite != nil {
			files = *req.MaxFilesPerSite
		}
		if single <= 0 || total <= 0 || files <= 0 {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"upload limits must be positive"), reqID))
			return
		}
		if single > total {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"maxSingleFileBytes cannot exceed maxSiteTotalBytes"), reqID))
			return
		}
		if err := s.deployer.SetUploadLimits(r.Context(), single, total, files); err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist upload limits: %v", err)), reqID))
			return
		}
		s.cfg.MaxSingleFileBytes = single
		s.cfg.MaxSiteTotalBytes = total
		s.cfg.MaxFilesPerSite = files
	}
	if req.CORSAllowOrigins != nil {
		origins := strings.TrimSpace(*req.CORSAllowOrigins)
		if origins == "*" {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"corsAllowOrigins no longer supports wildcard *; leave it blank to disable CORS"), reqID))
			return
		}
		origins = config.NormalizeCORSAllowOrigins(origins)
		if origins != "" && !corsOriginsLookValid(origins) {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"corsAllowOrigins must be empty or a comma/newline separated list of http(s) origins"), reqID))
			return
		}
		if err := s.deployer.SetCORSAllowOrigins(r.Context(), origins); err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist CORS origins: %v", err)), reqID))
			return
		}
		s.cfg.CORSAllowOrigins = origins
	}
	if req.EmbedPolicy != nil || req.EmbedAllowOrigins != nil {
		policy := config.NormalizeEmbedPolicy(s.cfg.EmbedPolicy)
		origins := s.cfg.EmbedAllowOrigins
		if req.EmbedPolicy != nil {
			policy = config.NormalizeEmbedPolicy(*req.EmbedPolicy)
		}
		if req.EmbedAllowOrigins != nil {
			origins = config.NormalizeOriginList(*req.EmbedAllowOrigins)
		}
		if policy == "allowlist" && strings.TrimSpace(origins) == "" {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"embedAllowOrigins is required when embedPolicy is allowlist"), reqID))
			return
		}
		if origins != "" && !corsOriginsLookValid(origins) {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
				"embedAllowOrigins must be empty or a comma/newline separated list of http(s) origins"), reqID))
			return
		}
		if err := s.deployer.SetEmbedPolicy(r.Context(), policy, origins); err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist embed policy: %v", err)), reqID))
			return
		}
		s.cfg.EmbedPolicy = policy
		s.cfg.EmbedAllowOrigins = origins
	}
	currentBase := s.requestBaseURL(r)
	writeJSON(w, http.StatusOK, ConfigUpdateResponse{
		Success:           true,
		CurrentBaseURL:    currentBase,
		AppURL:            s.appURLConfigForRequest(r),
		CORSAllowOrigins:  s.cfg.CORSAllowOrigins,
		EmbedPolicy:       config.NormalizeEmbedPolicy(s.cfg.EmbedPolicy),
		EmbedAllowOrigins: s.cfg.EmbedAllowOrigins,
		CooldownSeconds:   s.cfg.CooldownSeconds,
		Limits: Limits{
			MaxSingleFileBytes: s.cfg.MaxSingleFileBytes,
			MaxSiteTotalBytes:  s.cfg.MaxSiteTotalBytes,
			MaxFilesPerSite:    s.cfg.MaxFilesPerSite,
		},
		AnonymousPolicy: AnonymousPolicy{
			DeployLimit: s.cfg.AnonymousDeployLimit,
		},
	})
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	users, err := s.auth.ListUsers(r.Context())
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "users", err.Error()), reqID))
		return
	}
	items := make([]UserListItem, 0, len(users))
	for _, user := range users {
		items = append(items, toUserListItem(user))
	}
	writeJSON(w, http.StatusOK, UserListResponse{Success: true, Users: items})
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req UserCreateRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	user, err := s.auth.CreateUser(r.Context(), req.Username, req.Password, req.IsAdmin, req.DeployLimit)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "users", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, UserCreateResponse{Success: true, User: toUserListItem(user)})
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	id := r.PathValue("id")
	user, err := s.auth.GetUser(r.Context(), id)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeNotFound, "users", "user not found"), reqID))
		return
	}
	var req UserUpdateRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if req.Username != nil {
		user.Username = strings.TrimSpace(*req.Username)
	}
	if req.IsAdmin != nil {
		user.IsAdmin = *req.IsAdmin
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	if req.DeployLimit != nil {
		user.DeployLimit = *req.DeployLimit
	}
	if err := s.auth.UpdateUser(r.Context(), user); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "users", err.Error()), reqID))
		return
	}
	user, _ = s.auth.GetUser(r.Context(), id)
	writeJSON(w, http.StatusOK, UserUpdateResponse{Success: true, User: toUserListItem(user)})
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	admin, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "users", "missing user id"), reqID))
		return
	}
	if id == admin.ID {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "users", "cannot delete your own account"), reqID))
		return
	}
	user, err := s.auth.GetUser(r.Context(), id)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeNotFound, "users", "user not found"), reqID))
		return
	}
	if user.IsAdmin {
		users, err := s.auth.ListUsers(r.Context())
		if err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "users", err.Error()), reqID))
			return
		}
		activeAdmins := 0
		for _, u := range users {
			if u.IsAdmin && u.IsActive {
				activeAdmins++
			}
		}
		if activeAdmins <= 1 {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "users", "cannot delete the last active admin"), reqID))
			return
		}
	}
	if err := s.auth.DeleteUser(r.Context(), id); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "users", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, UserDeleteResponse{Success: true, ID: id})
}

func toUserListItem(user store.AdminUser) UserListItem {
	remaining := user.DeployLimit - user.DeployCount
	if user.DeployLimit < 0 {
		remaining = -1
	} else if remaining < 0 {
		remaining = 0
	}
	return UserListItem{
		ID:          user.ID,
		Username:    user.Username,
		IsAdmin:     user.IsAdmin,
		IsActive:    user.IsActive,
		DeployLimit: user.DeployLimit,
		DeployCount: user.DeployCount,
		Remaining:   remaining,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
	}
}

// baseURLLooksValid 校验 baseURL 格式：必须 http(s)://host[:port]，不含路径。
func baseURLLooksValid(s string) bool {
	if !strings.HasPrefix(strings.ToLower(s), "http://") &&
		!strings.HasPrefix(strings.ToLower(s), "https://") {
		return false
	}
	// 去掉 scheme 后剩下应该形如 host[:port]，不能再有 /
	rest := ""
	if i := strings.Index(s, "://"); i >= 0 {
		rest = s[i+3:]
	}
	if rest == "" || strings.ContainsAny(rest, "/?#") {
		return false
	}
	return true
}

func corsOriginsLookValid(s string) bool {
	for _, item := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !baseURLLooksValid(item) {
			return false
		}
	}
	return true
}

// handleAdminListSites 列出所有站点（含统计聚合）。
func (s *Server) handleAdminListSites(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	actor, isAdmin, authErr := s.optionalActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	if actor == "" {
		writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "auth", "login or bearer token required"), reqID))
		return
	}
	sites, err := s.deployer.ListSites(r.Context())
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "list_sites",
			fmt.Sprintf("failed to list sites: %v", err)), reqID))
		return
	}
	usernames := map[string]string{}
	if s.auth != nil {
		if users, err := s.auth.ListUsers(r.Context()); err == nil {
			for _, u := range users {
				usernames[u.ID] = u.Username
			}
		}
	}
	items := make([]SiteListItem, 0, len(sites))
	for _, site := range sites {
		if !isAdmin && site.OwnerTokenID != actor {
			continue
		}
		ownerUsername := ""
		if strings.HasPrefix(site.OwnerTokenID, "user:") {
			ownerUsername = usernames[strings.TrimPrefix(site.OwnerTokenID, "user:")]
		}
		items = append(items, SiteListItem{
			Code:            site.Code,
			PublicID:        site.PublicID,
			OwnerTokenID:    site.OwnerTokenID,
			OwnerUsername:   ownerUsername,
			CurrentVersion:  site.CurrentVersion,
			VersionCount:    site.VersionCount,
			TotalSize:       site.TotalSize,
			ViewCount:       site.ViewCount,
			LikeCount:       site.LikeCount,
			Status:          site.Status,
			Visibility:      site.Visibility,
			AccessProtected: site.AccessProtected,
			IsPinned:        site.IsPinned,
			PinnedAt:        site.PinnedAt,
			CreatedAt:       site.CreatedAt,
			Source:          site.Source,
			LastVersionAt:   site.LastVersionAt,
		})
	}
	writeJSON(w, http.StatusOK, SiteListResponse{Success: true, Sites: items})
}

// handleAdminSetSitePin 允许管理员置顶或取消置顶首页应用商城站点。
func (s *Server) handleAdminSetSitePin(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			"missing code in path"), reqID))
		return
	}
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req SitePinRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if req.Pinned == nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "pinned",
			"pinned is required"), reqID))
		return
	}
	if err := s.deployer.SetSitePinned(r.Context(), code, *req.Pinned); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "site", "site not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "site_pin", err.Error()), reqID))
		return
	}
	site, err := s.deployer.GetSite(r.Context(), code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "site", "site not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "site_pin", err.Error()), reqID))
		return
	}
	resp := SitePinResponse{Success: true, Code: code, IsPinned: site.IsPinned}
	if site.PinnedAt != nil {
		t := site.PinnedAt.UTC().Format(time.RFC3339Nano)
		resp.PinnedAt = &t
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAdminDeleteSite 删除整个 site。
func (s *Server) handleAdminDeleteSite(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			"missing code in path"), reqID))
		return
	}
	if apiErr := s.authorizeSiteWrite(r, code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	if apiErr := s.deployer.DeleteSite(r.Context(), code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, SiteDeleteResponse{Success: true, Code: code})
}

// ===== OpenAPI 兼容端点 =====

// handleGetPrimaryStrategy 处理 GET /api/deploys/{code}/primary-strategy。
func (s *Server) handleGetPrimaryStrategy(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			"missing code in path"), reqID))
		return
	}
	resp, apiErr := s.deployer.GetPrimaryStrategy(r.Context(), code)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSetPrimaryStrategy 处理 PATCH /api/deploys/{code}/primary-strategy。
func (s *Server) handleSetPrimaryStrategy(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			"missing code in path"), reqID))
		return
	}
	if apiErr := s.authorizeSiteWrite(r, code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	var req PrimaryStrategyRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	resp, apiErr := s.deployer.SetPrimaryStrategy(r.Context(), code, req.PrimaryVersionStrategy)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePatchDeployContent 处理 PATCH /api/deploy/content（兼容追加版本）。
// 等价于 POST /api/deploy with createVersion=true，但用 code/url 标识 site。
func (s *Server) handlePatchDeployContent(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())

	if ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))); !strings.HasPrefix(ct, "application/json") {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "content_type",
			"Content-Type must be application/json"), reqID))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.maxRequestBodyBytes())

	var req ContentPatchRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "parse_json",
			fmt.Sprintf("invalid JSON body: %v", err)), reqID))
		return
	}

	// 从 code 或 url 提取 code
	code := strings.TrimSpace(req.Code)
	if code == "" && req.URL != "" {
		code = extractCodeFromURL(req.URL)
	}
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "validate",
			"either 'code' or 'url' is required"), reqID))
		return
	}
	actor, _, actorErr := s.authenticateActor(r)
	if actorErr != nil {
		writeError(w, apiErrWithReqID(actorErr, reqID))
		return
	}
	if apiErr := s.authorizeSiteWrite(r, code); apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}

	// 鉴权
	var ownerTokenID string
	var userID string
	if user, ok := s.adminUserFromCookie(r); ok {
		if !user.IsAdmin && user.DeployLimit >= 0 && user.DeployCount >= user.DeployLimit {
			writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "user_quota",
				fmt.Sprintf("user deploy limit reached (%d/%d)", user.DeployCount, user.DeployLimit)).
				WithHint("Ask an admin to raise your deploy quota."), reqID))
			return
		}
		ownerTokenID = "user:" + user.ID
		userID = user.ID
	} else if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		tok, authErr := s.authenticateToken(r)
		if authErr != nil {
			writeError(w, apiErrWithReqID(authErr, reqID))
			return
		}
		ownerTokenID = ownerForToken(tok)
		if tok.OwnerUserID != "" {
			user, userErr := s.auth.GetUser(r.Context(), tok.OwnerUserID)
			if userErr != nil || !user.IsActive {
				writeError(w, apiErrWithReqID(NewError(CodeForbidden, "user", "token owner is inactive or missing"), reqID))
				return
			}
			if !user.IsAdmin && user.DeployLimit >= 0 && user.DeployCount >= user.DeployLimit {
				writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "user_quota",
					fmt.Sprintf("user deploy limit reached (%d/%d)", user.DeployCount, user.DeployLimit)).
					WithHint("Ask an admin to raise your deploy quota."), reqID))
				return
			}
			userID = user.ID
		}
	} else {
		ownerTokenID = actor
	}
	clientIP := clientIPFromRequest(r)

	// 转换成 DeployRequest，强制 createVersion=true
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "index.html"
	}
	deployReq := DeployRequest{
		Filename:         filename,
		Description:      req.Description,
		Title:            req.Title,
		Content:          req.Content,
		EnableCustomCode: true,
		CustomCode:       code,
		CreateVersion:    true,
	}

	resp, apiErr := s.deployer.Deploy(r.Context(), deployReq, ownerTokenID, clientIP)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	if userID != "" {
		if _, err := s.auth.IncrementUserDeployCount(r.Context(), userID); err != nil {
			s.logger.Printf("failed to increment user deploy count %s: %v", userID, err)
		}
	}

	// 响应用 VersionCreatedResponse 结构（OpenAPI 对齐）
	writeJSON(w, http.StatusOK, s.versionCreatedResponseForRequest(r, resp))
}

// extractCodeFromURL 从类似 https://host/foo 或 https://host/agent/foo 的 URL 提取 code。
func extractCodeFromURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	// 去掉 scheme
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	// 去掉 host
	if i := strings.Index(u, "/"); i >= 0 {
		u = u[i+1:]
	} else {
		return ""
	}
	// 去掉 query
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	u = strings.Trim(u, "/")
	// 处理 /agent/{code} 前缀
	if strings.HasPrefix(u, "agent/") {
		u = strings.TrimPrefix(u, "agent/")
	}
	// 取第一段
	if i := strings.Index(u, "/"); i >= 0 {
		u = u[:i]
	}
	return u
}

// handleAppServe 处理 GET /agent/{code}、GET /agent/{code}/{path...}
// 以及 GET /agent/{code}/versions/{version} 相关入口。
// 当前版本走 HostedDir/{code}/current，历史版本走 HostedDir/{code}/versions/{version}。
func (s *Server) handleAppServe(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	sub := r.PathValue("path")
	versionValue := strings.TrimSpace(r.PathValue("version"))
	s.serveAppContent(w, r, code, sub, versionValue, false)
}

func (s *Server) serveAppContent(w http.ResponseWriter, r *http.Request, code, sub, versionValue string, domainMode bool) {
	if !routeCodeRe.MatchString(code) {
		http.NotFound(w, r)
		return
	}

	var versionPtr *int64
	if versionValue != "" {
		versionNumber, err := parseInt64(versionValue)
		if err != nil || versionNumber <= 0 {
			http.NotFound(w, r)
			return
		}
		versionPtr = &versionNumber
	} else if versionQuery := strings.TrimSpace(r.URL.Query().Get("v")); versionQuery != "" {
		versionNumber, err := parseInt64(versionQuery)
		if err != nil || versionNumber <= 0 {
			http.NotFound(w, r)
			return
		}
		target := *r.URL
		if domainMode {
			target.Path = buildDomainVersionServePath(versionNumber, sub)
		} else {
			target.Path = buildVersionServePath(code, versionNumber, sub)
		}
		query := target.Query()
		query.Del("v")
		target.RawQuery = query.Encode()
		http.Redirect(w, r, target.String(), http.StatusPermanentRedirect)
		return
	}

	if sub == "" && !strings.HasSuffix(r.URL.Path, "/") {
		u := *r.URL
		if versionPtr != nil {
			if domainMode {
				u.Path = buildDomainVersionServePath(*versionPtr, "")
			} else {
				u.Path = buildVersionServePath(code, *versionPtr, "")
			}
		} else {
			u.Path = r.URL.Path + "/"
		}
		http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
		return
	}
	if !s.siteAccessAllowed(r, code, versionPtr) {
		s.renderAccessPasswordPageV2(w, code)
		return
	}

	base := filepath.Join(s.cfg.HostedDir, code, "current")
	var mainEntry string
	if versionPtr != nil {
		base = filepath.Join(s.cfg.HostedDir, code, "versions", strconv.FormatInt(*versionPtr, 10))
	}
	if sub == "" {
		content, apiErr := s.deployer.GetContent(r.Context(), code, versionPtr)
		if apiErr != nil {
			http.NotFound(w, r)
			return
		}
		mainEntry = content.MainEntry
	}
	if sub == "" {
		sub = mainEntry
	}
	full := filepath.Join(base, sub)

	absBase, err := filepath.Abs(base)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel, err := filepath.Rel(absBase, absFull)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}

	// 仅当前版本入口统计浏览量；历史版本预览不计数。
	if versionPtr == nil && sub == mainEntry {
		go func(c string) {
			_ = s.deployer.IncrementViewCount(context.Background(), c)
		}(code)
	}

	routePrefix := buildAppRoutePrefix(code)
	if domainMode {
		routePrefix = "/"
	}
	if versionPtr != nil {
		if domainMode {
			routePrefix = buildDomainVersionServePath(*versionPtr, "")
		} else {
			routePrefix = buildVersionServePath(code, *versionPtr, "")
		}
	}
	s.serveHostedFile(w, r, sub, absFull, routePrefix)
}

func (s *Server) serveHostedFile(w http.ResponseWriter, r *http.Request, sub, absFull, routePrefix string) {
	s.setHostedContentSecurityHeaders(w)
	setHostedContentCORSHeaders(w, r)
	lowerSub := strings.ToLower(sub)
	if !strings.HasSuffix(lowerSub, ".html") && !strings.HasSuffix(lowerSub, ".htm") {
		http.ServeFile(w, r, absFull)
		return
	}
	body, err := os.ReadFile(absFull)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, filepath.Base(absFull), fileModTime(absFull), strings.NewReader(injectHostedHTMLCompat(body, routePrefix)))
}

func (s *Server) setHostedContentSecurityHeaders(w http.ResponseWriter) {
	csp := "sandbox allow-scripts allow-forms allow-popups allow-downloads; " +
		"default-src * data: blob: 'unsafe-inline' 'unsafe-eval'; " +
		"img-src * data: blob:; media-src * data: blob:; font-src * data:; " +
		"connect-src *; frame-src *; child-src *"
	if frameAncestors := s.frameAncestorsDirective(); frameAncestors != "" {
		csp += "; " + frameAncestors
	}
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func (s *Server) frameAncestorsDirective() string {
	switch config.NormalizeEmbedPolicy(s.cfg.EmbedPolicy) {
	case "deny":
		return "frame-ancestors 'none'"
	case "self":
		return "frame-ancestors 'self'"
	case "allowlist":
		origins := strings.FieldsFunc(s.cfg.EmbedAllowOrigins, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		})
		items := []string{"'self'"}
		for _, origin := range origins {
			origin = strings.TrimRight(strings.TrimSpace(origin), "/")
			if origin != "" && baseURLLooksValid(origin) {
				items = append(items, origin)
			}
		}
		return "frame-ancestors " + strings.Join(items, " ")
	default:
		return ""
	}
}

func setHostedContentCORSHeaders(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("Origin")) != "null" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "null")
	w.Header().Add("Vary", "Origin")
}

func buildAppRoutePrefix(code string) string {
	return "/agent/" + code + "/"
}

func buildVersionServePath(code string, version int64, sub string) string {
	base := "/agent/" + code + "/versions/" + strconv.FormatInt(version, 10)
	if sub == "" {
		return base + "/"
	}
	return base + "/" + strings.TrimPrefix(sub, "/")
}

func buildDomainVersionServePath(version int64, sub string) string {
	base := "/versions/" + strconv.FormatInt(version, 10)
	if sub == "" {
		return base + "/"
	}
	return base + "/" + strings.TrimPrefix(sub, "/")
}

func fileModTime(path string) time.Time {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

func injectHostedHTMLCompat(body []byte, routePrefix string) string {
	htmlText := string(body)
	script := hostedHTMLCompatScript(routePrefix)
	lower := strings.ToLower(htmlText)
	if i := strings.LastIndex(lower, "</body>"); i >= 0 {
		return htmlText[:i] + script + htmlText[i:]
	}
	return htmlText + script
}

func hostedHTMLCompatScript(routePrefix string) string {
	prefixJSON, _ := json.Marshal(routePrefix)
	return "\n<script data-pagepilot-compat>\n(function(){\n" +
		"var prefix=" + string(prefixJSON) + ";\n" +
		"function isPlainClick(e){return e.button===0&&!e.metaKey&&!e.ctrlKey&&!e.shiftKey&&!e.altKey;}\n" +
		"function shouldHandle(e,a){\n" +
		" if(!a||!isPlainClick(e)) return false;\n" +
		" var href=a.getAttribute('href')||'';\n" +
		" if(!href||href==='#'||href.indexOf('javascript:')===0||href.indexOf('mailto:')===0||href.indexOf('tel:')===0) return false;\n" +
		" try{var u=new URL(href,location.href); if(u.origin!==location.origin) return false; return /\\.html?$/i.test(u.pathname) && (u.pathname.indexOf(prefix)===0 || (u.pathname.indexOf('/')===0 && u.pathname.indexOf(prefix)!==0));}catch(_){return false;}\n" +
		"}\n" +
		"function targetURL(a){\n" +
		" var u=new URL(a.getAttribute('href'),location.href);\n" +
		" if(u.pathname.indexOf(prefix)!==0) u.pathname=prefix+u.pathname.replace(/^\\/+/, '');\n" +
		" return u.href;\n" +
		"}\n" +
		"document.addEventListener('click',function(e){\n" +
		" var a=e.target&&e.target.closest&&e.target.closest('a[href]');\n" +
		" if(!shouldHandle(e,a)) return;\n" +
		" setTimeout(function(){\n" +
		"  if(e.defaultPrevented) location.href=targetURL(a);\n" +
		" },0);\n" +
		"},true);\n" +
		"})();\n</script>\n"
}

// handleDetailUI 处理 GET /deploy/{uuid}（详情页 SPA）。
// dev 模式：返回用户端 SPA 详情页（user/detail.html），SPA 自己根据 URL 解析参数。
// 生产环境由 Caddy 把 /deploy/{uuid} 指向 detail.html。
func (s *Server) handleDetailUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	bytes, err := fs.ReadFile(web.UserSubFS(), "detail.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write(bytes)
}
