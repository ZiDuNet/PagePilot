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
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"math/big"
	"mime/multipart"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/yourorg/hostctl/internal/auth"
	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/render"
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
	ReadAppFile(ctx context.Context, code string, versionPtr *int64, path string) ([]byte, time.Time, *APIError)

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
	SetSiteReusePolicy(ctx context.Context, code, reusePolicy, sourceDownloadPolicy string) error
	SetSiteSecurityMode(ctx context.Context, code, securityMode string) error
	SetAnonymousDeployLimit(ctx context.Context, n int) error
	SetCooldownSeconds(ctx context.Context, n int) error
	SetUploadLimits(ctx context.Context, singleFileBytes, siteTotalBytes int64, filesPerSite int) error
	SetCORSAllowOrigins(ctx context.Context, origins string) error
	SetEmbedPolicy(ctx context.Context, policy, allowOrigins string) error
	SetContentInjection(ctx context.Context, cfg config.ContentInjectionConfig) error
	GetMarketCategories(ctx context.Context) ([]MarketCategory, error)
	SetMarketCategories(ctx context.Context, categories []MarketCategory) error
	CreateAnonymousSession(ctx context.Context, id string) (store.AnonymousSession, error)
	GetAnonymousSession(ctx context.Context, id string) (store.AnonymousSession, error)
	UpdateAnonymousSessionMeta(ctx context.Context, id, agentID, agentLabel, deviceIP, userAgent string) error
	IncrementAnonymousSessionDeployCount(ctx context.Context, id string) (store.AnonymousSession, error)
	ClaimAnonymousSession(ctx context.Context, id, userID string) (store.AnonymousSessionClaimResult, error)
	ListAnonymousSessions(ctx context.Context, limit int) ([]store.AnonymousSession, error)

	// ===== 创作市场（marketplace） =====

	ListMarketplaceDeploys(ctx context.Context, q, status, sort, category, kind, ownerTokenID, favoriteOwnerID string, page, pageSize int) ([]store.MarketplaceDeploy, int, error)
	GetMarketplaceDeploy(ctx context.Context, code string) (store.MarketplaceDeploy, error)
	GetMarketplaceDeployByUUID(ctx context.Context, publicID string) (store.MarketplaceDeploy, error)
	IncrementViewCount(ctx context.Context, code string) error
	AddLike(ctx context.Context, code, fingerprint string) (int64, error)
	SetFavorite(ctx context.Context, code, ownerID string, favorited bool) (int64, bool, error)
	SiteExists(ctx context.Context, code string) (bool, error)
	GetSite(ctx context.Context, code string) (store.Site, error)
	SetSiteCategory(ctx context.Context, code, category string) error
	SetSiteTags(ctx context.Context, code string, tags []string) error
	SetSiteAccessPassword(ctx context.Context, code, password string) error
	RevealSiteAccessPassword(ctx context.Context, code string) (string, error)

	CreateScreenPairing(ctx context.Context, pairing store.ScreenPairing) error
	BindScreenPairing(ctx context.Context, code, ownerUserID, name string) (store.Screen, error)
	AssignScreenOwner(ctx context.Context, screenID, ownerUserID, name string) (store.Screen, error)
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
type markdownRenderCache interface {
	GetRenderCache(ctx context.Context, cacheKey string) (store.RenderCacheEntry, bool, error)
	PutRenderCache(ctx context.Context, entry store.RenderCacheEntry) error
}

type auditLogReader interface {
	ListAuditLogs(ctx context.Context, filter store.AuditLogFilter) ([]store.AuditLog, int, error)
}

type auditLogWriter interface {
	RecordAuditLog(ctx context.Context, log store.AuditLog) error
}

type bundleMetadataReader interface {
	GetVersionBundle(ctx context.Context, code string, version int64) (store.VersionBundle, error)
}

type Server struct {
	cfg              config.Config
	deployer         DeployerPort
	auth             *auth.Service
	requireAuth      bool
	mux              *http.ServeMux
	logger           *log.Logger
	version          string
	captchaMu        sync.Mutex
	captchas         map[string]captchaChallenge
	emailMu          sync.Mutex
	emailCodes       map[string]emailVerificationChallenge
	screenHub        *screenHub
	loginMu          sync.Mutex
	loginFails       map[string]*loginFailCounter
	loginCleanupOnce sync.Once
}

// loginFailCounter 记录某主键（用户名/ip）的失败次数。
// 超过阈值后返回 429。
type loginFailCounter struct {
	count       int
	lastFail    time.Time
	lockedUntil time.Time
}

const (
	loginFailWindow      = 5 * time.Minute
	loginFailThreshold   = 5
	loginLockoutDuration = 15 * time.Minute
)

func (s *Server) loginKey(username, ip string) string {
	return strings.ToLower(strings.TrimSpace(username)) + "|" + ip
}

// loginCheckLocked 查询是否在锁定窗口中。若已锁定返回 unlockRemain。
func (s *Server) loginCheckLocked(key string) time.Duration {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	s.loginCleanup()
	c, ok := s.loginFails[key]
	if !ok || c.lockedUntil.IsZero() {
		return 0
	}
	remain := time.Until(c.lockedUntil)
	if remain <= 0 {
		return 0
	}
	return remain
}

// loginRecordFail 增加失败计数，超过阈值锁定。
func (s *Server) loginRecordFail(key string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	s.loginCleanup()
	c, ok := s.loginFails[key]
	if !ok {
		c = &loginFailCounter{}
		s.loginFails[key] = c
	}
	now := time.Now()
	if now.Sub(c.lastFail) > loginFailWindow {
		c.count = 1
	} else {
		c.count++
	}
	c.lastFail = now
	if c.count >= loginFailThreshold {
		c.lockedUntil = now.Add(loginLockoutDuration)
	}
}

// loginReset 成功登录后清除计数。
func (s *Server) loginReset(key string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	delete(s.loginFails, key)
}

func (s *Server) loginCleanup() {
	if s.loginFails == nil {
		s.loginFails = make(map[string]*loginFailCounter)
		return
	}
	now := time.Now()
	for k, c := range s.loginFails {
		if now.Sub(c.lastFail) > loginFailWindow && (c.lockedUntil.IsZero() || now.After(c.lockedUntil)) {
			delete(s.loginFails, k)
		}
	}
}

// startLoginCleanup 启动后台清理协程（幂等）。
func (s *Server) startLoginCleanup() {
	s.loginCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				s.loginMu.Lock()
				s.loginCleanup()
				s.loginMu.Unlock()
			}
		}()
	})
}

type captchaChallenge struct {
	Answer    string
	ExpiresAt time.Time
}

type emailVerificationChallenge struct {
	Code      string
	ExpiresAt time.Time
}

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
var routeCodeRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{2,30}[a-z0-9])?$`)

// validatePasswordStrength 校验用户密码强度：至少 8 字符、含字母和数字。
// 返回 nil 或 *APIError。空密码（用作"清除"语义）不被拦截。
func validatePasswordStrength(password string) *APIError {
	p := strings.TrimSpace(password)
	if p == "" {
		return nil
	}
	if len(p) < 8 {
		return NewError(CodeInvalidInput, "password", "password must be at least 8 characters")
	}
	hasLetter, hasDigit := false, false
	for _, r := range p {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return NewError(CodeInvalidInput, "password", "password must contain both letters and digits")
	}
	return nil
}

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
		emailCodes:  map[string]emailVerificationChallenge{},
		loginFails:  make(map[string]*loginFailCounter),
		screenHub:   newScreenHub(),
	}
	s.routes()
	s.startLoginCleanup()
	s.startCaptchaCleanup()
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
			resp.AgentGuideURL = "/admin?tab=apiDocs"
		} else {
			resp.AgentGuideURL = base + "/admin?tab=apiDocs"
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
	s.mux.HandleFunc("POST /api/security/csp-report", s.handleCSPReport)
	s.mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)

	// 创作市场（公开 API：/api/deploys）
	s.mux.HandleFunc("GET /api/market/categories", s.handleGetMarketCategories)
	s.mux.HandleFunc("GET /api/deploys", s.handleListMarketplace)
	s.mux.HandleFunc("GET /api/deploys/{publicId}", s.handleGetMarketplaceDeploy)
	s.mux.HandleFunc("POST /api/deploys/{code}/like", s.handleLikeDeploy)
	s.mux.HandleFunc("POST /api/deploys/{code}/favorite", s.handleFavoriteDeploy)
	s.mux.HandleFunc("GET /api/deploys/{code}/qr", s.handleQRCode)
	s.mux.HandleFunc("POST /api/deploys/{code}/access", s.handleSiteAccessLogin)
	s.mux.HandleFunc("PATCH /api/deploys/{code}/access", s.handleSetSiteAccessPassword)
	s.mux.HandleFunc("POST /api/deploys/{code}/access/set", s.handleSetSiteAccessPassword)
	s.mux.HandleFunc("PATCH /api/deploys/{code}/visibility", s.handleSetSiteVisibility)
	s.mux.HandleFunc("POST /api/deploys/{code}/visibility", s.handleSetSiteVisibility)

	// 屏幕投放：用户侧仅注册用户；设备侧仅 Device Token。
	s.mux.HandleFunc("GET /api/screens", s.handleListScreens)
	s.mux.HandleFunc("POST /api/screens/bind", s.handleBindScreen)
	s.mux.HandleFunc("POST /api/admin/screens/{screenId}/assign", s.handleAdminAssignScreen)
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
	s.mux.HandleFunc("POST /api/deploys/{code}/current", s.handleSetCurrent)
	s.mux.HandleFunc("PATCH /api/deploys/{code}/versions/{version}", s.handlePatchVersion)
	s.mux.HandleFunc("POST /api/deploys/{code}/versions/{version}", s.handlePatchVersion)
	s.mux.HandleFunc("DELETE /api/deploys/{code}/versions/{version}", s.handleDeleteVersion)

	// primary-strategy（OpenAPI）
	s.mux.HandleFunc("GET /api/deploys/{code}/primary-strategy", s.handleGetPrimaryStrategy)
	s.mux.HandleFunc("PATCH /api/deploys/{code}/primary-strategy", s.handleSetPrimaryStrategy)
	s.mux.HandleFunc("POST /api/deploys/{code}/primary-strategy", s.handleSetPrimaryStrategy)

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
	s.mux.HandleFunc("POST /api/auth/email-code", s.handleEmailVerificationCode)
	s.mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.mux.HandleFunc("PATCH /api/account/password", s.handleAccountPassword)
	s.mux.HandleFunc("POST /api/account/password", s.handleAccountPassword)
	s.mux.HandleFunc("GET /api/admin/audit-logs", s.handleAdminAuditLogs)
	s.mux.HandleFunc("GET /api/admin/anonymous-sessions", s.handleAdminAnonymousSessions)
	s.mux.HandleFunc("GET /api/admin/skill", s.handleAdminGetSkill)
	s.mux.HandleFunc("POST /api/admin/skill/package", s.handleAdminUploadSkillPackage)
	s.mux.HandleFunc("PUT /api/admin/market/categories", s.handleAdminPutMarketCategories)
	s.mux.HandleFunc("GET /api/admin/users", s.handleAdminListUsers)
	s.mux.HandleFunc("POST /api/admin/users", s.handleAdminCreateUser)
	s.mux.HandleFunc("PATCH /api/admin/users/{id}", s.handleAdminUpdateUser)
	s.mux.HandleFunc("POST /api/admin/users/{id}", s.handleAdminUpdateUser)
	s.mux.HandleFunc("DELETE /api/admin/users/{id}", s.handleAdminDeleteUser)
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	s.mux.HandleFunc("GET /api/admin/sites", s.handleAdminListSites)
	s.mux.HandleFunc("GET /api/admin/sites/{code}", s.handleAdminGetSiteDetail)
	s.mux.HandleFunc("PATCH /api/admin/sites/{code}/pin", s.handleAdminSetSitePin)
	s.mux.HandleFunc("POST /api/admin/sites/{code}/pin", s.handleAdminSetSitePin)
	s.mux.HandleFunc("PATCH /api/admin/sites/{code}/reuse-policy", s.handleAdminSetSiteReusePolicy)
	s.mux.HandleFunc("POST /api/admin/sites/{code}/reuse-policy", s.handleAdminSetSiteReusePolicy)
	s.mux.HandleFunc("PATCH /api/admin/sites/{code}/security-mode", s.handleAdminSetSiteSecurityMode)
	s.mux.HandleFunc("POST /api/admin/sites/{code}/security-mode", s.handleAdminSetSiteSecurityMode)
	s.mux.HandleFunc("PATCH /api/admin/sites/{code}/category", s.handleAdminSetSiteCategory)
	s.mux.HandleFunc("POST /api/admin/sites/{code}/category", s.handleAdminSetSiteCategory)
	s.mux.HandleFunc("PATCH /api/admin/sites/{code}/tags", s.handleAdminSetSiteTags)
	s.mux.HandleFunc("POST /api/admin/sites/{code}/tags", s.handleAdminSetSiteTags)
	s.mux.HandleFunc("DELETE /api/admin/sites/{code}", s.handleAdminDeleteSite)
	s.mux.HandleFunc("POST /api/admin/sites/{code}/access/reveal", s.handleAdminRevealSiteAccessPassword)

	// admin 后台单页
	s.mux.HandleFunc("GET /admin", s.handleAdminUI)
	s.mux.HandleFunc("GET /admin/", s.handleAdminUI)
	s.mux.Handle("GET /admin/favicon.ico", http.StripPrefix("/admin/", adminAssetFileServer(web.AdminAppFS())))
	s.mux.Handle("GET /admin/assets/", http.StripPrefix("/admin/", adminAssetFileServer(web.AdminAppFS())))
	s.mux.Handle("GET /app/favicon.ico", http.StripPrefix("/app/", http.FileServer(http.FS(web.UserSubFS()))))
	s.mux.Handle("GET /app/assets/", http.StripPrefix("/app/", http.FileServer(http.FS(web.UserSubFS()))))
	s.mux.HandleFunc("GET /index.html", s.handleUserAppUI)
	s.mux.HandleFunc("GET /deploy", s.handleUserAppUI)
	s.mux.HandleFunc("GET /deploy/", s.handleUserAppUI)
	s.mux.HandleFunc("GET /api-docs.html", s.handleAPIDocsRedirect)
	s.mux.HandleFunc("GET /detail.html", s.handleMarketRedirect)
	s.mux.HandleFunc("GET /agents/", s.handleAgentGuideUI)
	s.mux.HandleFunc("GET /screens/", s.handleScreenGuideUI)
	s.mux.HandleFunc("GET /market", s.handleUserAppUI)
	s.mux.HandleFunc("GET /market/", s.handleUserAppUI)
	s.mux.Handle("GET /markdown-assets/", http.StripPrefix("/markdown-assets/", markdownAssetFileServer(web.MarkdownAssetsFS())))
	s.mux.HandleFunc("GET /skill/hostctl-deploy.zip", s.handleSkillDownload)
	s.mux.HandleFunc("GET /skill/pagep.zip", s.handleSkillDownload)
	// admin 子资源（admin.css / admin.js 等）从 embed 子树取
	s.mux.Handle("GET /admin/static/", http.StripPrefix("/admin/static/", adminAssetFileServer(web.AdminSubFS())))

	// dev 模式：把已部署站点 serve 起来（生产由 Caddy 做）。
	// 路径模式优先级：/api/*、/admin、/admin/* 已被前面注册占用。
	// /agent/{code}：应用本体 URL（用户部署后访问应用本身用这个前缀）
	// /agent/{code}/{path...}：当前版本的多文件子路径（CSS/JS/图片等）
	// /agent/{code}/versions/{version}：历史版本预览入口
	// /agent/{code}/versions/{version}/{path...}：历史版本的多文件子路径
	s.mux.HandleFunc("GET /agent/{code}", s.handleAppServe)
	s.mux.HandleFunc("GET /agent/{code}/{path...}", s.handleAppServe)
	s.mux.HandleFunc("GET /agent/{code}/versions/{version}", s.handleAppServe)
	s.mux.HandleFunc("GET /agent/{code}/versions/{version}/{path...}", s.handleAppServe)

	// 用户端 SPA：根路径托管 user/ 目录（首页 / 部署 / API 文档 / 静态资源）
	s.mux.Handle("GET /", s.userAppFileServer())
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
		s.applyBaseSecurityHeaders(w)

		// 把 reqID 放进 context，handler 可以取出加到错误响应里
		r = r.WithContext(withRequestID(r.Context(), reqID))

		corsManaged := isCORSManagedPath(r.URL.Path)
		if corsManaged {
			s.applyCORS(w, r)
		}
		if corsManaged && r.Method == http.MethodOptions {
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

func isCORSManagedPath(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/openapi.json"
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

func (s *Server) applyBaseSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
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
	var req DeployRequest
	var ownerTokenID string
	var anonymousSessionID string
	var userID string
	actorIsAdmin := false
	writeDeployError := func(apiErr *APIError) {
		actorType, actorID, actorRole := s.auditActorFromRequest(r)
		if ownerTokenID != "" {
			actorType, actorID, actorRole = auditActorFromOwner(ownerTokenID, actorIsAdmin)
		}
		action := "deploy.create"
		if req.CreateVersion {
			action = "deploy.version.create"
		}
		siteCode := strings.TrimSpace(req.CustomCode)
		s.recordAuditLogWithResult(r, actorType, actorID, actorRole, action, siteCode, "site", siteCode, "failed", deployFailureAuditDetail(req, apiErr))
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}

	if r.Method != http.MethodPost {
		writeDeployError(NewError(CodeMethodNotAllowed, "method",
			fmt.Sprintf("method %s not allowed; use POST", r.Method)))
		return
	}

	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	r.Body = http.MaxBytesReader(w, r.Body, s.maxRequestBodyBytes())

	if strings.HasPrefix(ct, "application/json") {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeDeployError(NewError(CodeInvalidInput, "parse_json",
				fmt.Sprintf("invalid JSON body: %v", err)))
			return
		}
	} else if strings.HasPrefix(ct, "multipart/form-data") {
		parsed, apiErr := s.decodeDeployMultipart(r)
		if apiErr != nil {
			writeDeployError(apiErr)
			return
		}
		req = parsed
	} else {
		writeDeployError(NewError(CodeInvalidInput, "content_type",
			"Content-Type must be application/json or multipart/form-data"))
		return
	}

	consumesSiteQuota := s.deployRequestConsumesSiteQuota(r.Context(), req)
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		tok, authErr := s.authenticateToken(r)
		if authErr != nil {
			writeDeployError(authErr)
			return
		}
		actorIsAdmin = tok.IsAdmin
		ownerTokenID = ownerForToken(tok)
		if tok.OwnerUserID != "" {
			user, userErr := s.auth.GetUser(r.Context(), tok.OwnerUserID)
			if userErr != nil || !user.IsActive {
				writeDeployError(NewError(CodeForbidden, "user", "token owner is inactive or missing"))
				return
			}
			if consumesSiteQuota && !user.IsAdmin && user.DeployLimit >= 0 && user.DeployCount >= user.DeployLimit {
				writeDeployError(NewError(CodeUnauthorized, "user_quota",
					fmt.Sprintf("user deploy limit reached (%d/%d)", user.DeployCount, user.DeployLimit)).
					WithHint("Ask an admin to raise your deploy quota."))
				return
			}
			userID = user.ID
		}
	} else if user, ok := s.adminUserFromCookie(r); ok {
		userID = user.ID
		actorIsAdmin = user.IsAdmin
		ownerTokenID = "user:" + user.ID
		if consumesSiteQuota && !user.IsAdmin && user.DeployLimit >= 0 && user.DeployCount >= user.DeployLimit {
			writeDeployError(NewError(CodeUnauthorized, "user_quota",
				fmt.Sprintf("user deploy limit reached (%d/%d)", user.DeployCount, user.DeployLimit)).
				WithHint("Ask an admin to raise your deploy quota."))
			return
		}
	} else {
		sess, sessionErr := s.ensureAnonymousSession(w, r)
		if sessionErr != nil {
			writeDeployError(sessionErr)
			return
		}
		anonymousSessionID = sess.ID
		ownerTokenID = "anon:" + sess.ID
		if consumesSiteQuota && s.cfg.AnonymousDeployLimit >= 0 && sess.DeployCount >= s.cfg.AnonymousDeployLimit {
			writeDeployError(NewError(CodeUnauthorized, "anonymous_quota",
				fmt.Sprintf("anonymous deploy limit reached (%d/%d)", sess.DeployCount, s.cfg.AnonymousDeployLimit)).
				WithHint("Ask the user to register or sign in, create a user token, then claim this anonymous session."))
			return
		}
	}
	if req.EnableCustomCode && strings.TrimSpace(req.CustomCode) != "" {
		if apiErr := s.authorizeDeployCustomCode(r, strings.TrimSpace(req.CustomCode), ownerTokenID); apiErr != nil {
			writeDeployError(apiErr)
			return
		}
	}
	clientIP := clientIPFromRequest(r)

	resp, apiErr := s.deployer.Deploy(r.Context(), req, ownerTokenID, clientIP)
	if apiErr != nil {
		writeDeployError(apiErr)
		return
	}
	if resp.Created && anonymousSessionID != "" {
		_, err := s.deployer.IncrementAnonymousSessionDeployCount(r.Context(), anonymousSessionID)
		if err != nil {
			s.logger.Printf("failed to increment anonymous session %s: %v", anonymousSessionID, err)
		}
	}
	if resp.Created && userID != "" {
		if _, err := s.auth.IncrementUserDeployCount(r.Context(), userID); err != nil {
			s.logger.Printf("failed to increment user deploy count %s: %v", userID, err)
		}
	}
	s.rewriteDeployResponseURLs(r, resp)
	actorType, actorID, actorRole := auditActorFromOwner(ownerTokenID, actorIsAdmin)
	action := "deploy.create"
	if req.CreateVersion {
		action = "deploy.version.create"
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, action, resp.Code, "site", resp.Code, map[string]any{
		"versionNumber":         resp.VersionNumber,
		"versionId":             resp.VersionID,
		"visibility":            resp.Visibility,
		"customCode":            req.EnableCustomCode,
		"createVersion":         req.CreateVersion,
		"accessProtected":       strings.TrimSpace(req.AccessPassword) != "",
		"title":                 req.Title,
		"filename":              req.Filename,
		"templateSourceCode":    req.TemplateSourceCode,
		"templateSourceVersion": req.TemplateSourceVersion,
	})

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) deployRequestConsumesSiteQuota(ctx context.Context, req DeployRequest) bool {
	if !req.CreateVersion {
		return true
	}
	code := strings.TrimSpace(req.CustomCode)
	if !req.EnableCustomCode || code == "" {
		return true
	}
	exists, err := s.deployer.SiteExists(ctx, code)
	if err != nil {
		s.logger.Printf("failed to check site existence for quota code=%s: %v", code, err)
		return true
	}
	return !exists
}

func (s *Server) decodeDeployMultipart(r *http.Request) (DeployRequest, *APIError) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return DeployRequest{}, NewError(CodeInvalidInput, "parse_multipart",
			fmt.Sprintf("invalid multipart body: %v", err))
	}
	req := DeployRequest{
		Filename:       firstMultipartValue(r.MultipartForm, "filename"),
		Description:    firstMultipartValue(r.MultipartForm, "description"),
		Title:          firstMultipartValue(r.MultipartForm, "title"),
		Source:         firstMultipartValue(r.MultipartForm, "source"),
		Visibility:     firstMultipartValue(r.MultipartForm, "visibility"),
		Category:       firstMultipartValue(r.MultipartForm, "category"),
		AccessPassword: firstMultipartValue(r.MultipartForm, "accessPassword"),
	}
	if req.AccessPassword == "" {
		req.AccessPassword = firstMultipartValue(r.MultipartForm, "access_password")
	}
	req.CustomCode = firstMultipartValue(r.MultipartForm, "customCode")
	if req.CustomCode == "" {
		req.CustomCode = firstMultipartValue(r.MultipartForm, "custom_code")
	}
	req.EnableCustomCode = parseBoolForm(firstMultipartValue(r.MultipartForm, "enableCustomCode")) || req.CustomCode != ""
	req.CreateVersion = parseBoolForm(firstMultipartValue(r.MultipartForm, "createVersion"))
	if !req.CreateVersion {
		req.CreateVersion = parseBoolForm(firstMultipartValue(r.MultipartForm, "create_version"))
	}
	req.TemplateSourceCode = firstMultipartValue(r.MultipartForm, "templateSourceCode")
	if req.TemplateSourceCode == "" {
		req.TemplateSourceCode = firstMultipartValue(r.MultipartForm, "template_source_code")
	}
	req.TemplateSourceVersion = parseInt64Form(firstMultipartValue(r.MultipartForm, "templateSourceVersion"))
	if req.TemplateSourceVersion <= 0 {
		req.TemplateSourceVersion = parseInt64Form(firstMultipartValue(r.MultipartForm, "template_source_version"))
	}
	if tags := firstMultipartValue(r.MultipartForm, "tags"); strings.TrimSpace(tags) != "" {
		for _, tag := range strings.Split(tags, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				req.Tags = append(req.Tags, tag)
			}
		}
	}

	files, apiErr := deployFilesFromMultipart(r.MultipartForm)
	if apiErr != nil {
		return DeployRequest{}, apiErr
	}
	req.Files = files
	return req, nil
}

func firstMultipartValue(form *multipart.Form, key string) string {
	if form == nil {
		return ""
	}
	values := form.Value[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func parseBoolForm(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseInt64Form(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func deployFilesFromMultipart(form *multipart.Form) ([]DeployFile, *APIError) {
	if form == nil {
		return nil, NewError(CodeInvalidInput, "multipart_file", "multipart file is required")
	}
	headers := append([]*multipart.FileHeader{}, form.File["file"]...)
	headers = append(headers, form.File["files"]...)
	if len(headers) == 0 {
		return nil, NewError(CodeInvalidInput, "multipart_file", "multipart file is required")
	}
	out := make([]DeployFile, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			return nil, NewError(CodeInvalidInput, "multipart_file",
				fmt.Sprintf("open multipart file %s: %v", header.Filename, err))
		}
		data, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			return nil, NewError(CodeInvalidInput, "multipart_file",
				fmt.Sprintf("read multipart file %s: %v", header.Filename, err))
		}
		path := sanitizeMultipartDeployPath(header.Filename, "upload")
		item := DeployFile{Path: path}
		if looksMultipartBinary(data) {
			item.ContentBase64 = base64.StdEncoding.EncodeToString(data)
		} else {
			item.Content = string(data)
		}
		out = append(out, item)
	}
	return out, nil
}

func sanitizeMultipartDeployPath(name, fallbackStem string) string {
	name = filepath.ToSlash(strings.TrimSpace(name))
	name = strings.TrimLeft(name, "/")
	rawParts := strings.Split(name, "/")
	parts := make([]string, 0, len(rawParts))
	for i, part := range rawParts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		fallback := "folder"
		if i == len(rawParts)-1 {
			fallback = fallbackStem
		}
		parts = append(parts, sanitizeMultipartPathSegment(part, fallback))
	}
	if len(parts) == 0 {
		return fallbackStem
	}
	return strings.Join(parts, "/")
}

func sanitizeMultipartPathSegment(name, fallbackStem string) string {
	name = strings.TrimSpace(name)
	ext := ""
	if dot := strings.LastIndex(name, "."); dot > 0 && dot < len(name)-1 {
		candidate := name[dot:]
		if multipartExtensionSafe(candidate) {
			ext = strings.ToLower(candidate)
			name = name[:dot]
		}
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || unicode.IsSpace(r):
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	base := strings.Trim(b.String(), ".-")
	if base == "" || multipartWindowsReservedName(base) {
		base = fallbackStem
	}
	return base + ext
}

func multipartExtensionSafe(ext string) bool {
	if len(ext) < 2 || len(ext) > 17 || ext[0] != '.' {
		return false
	}
	for _, r := range ext[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func multipartWindowsReservedName(name string) bool {
	name = strings.ToUpper(name)
	switch name {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

func looksMultipartBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	checkLen := len(data)
	if checkLen > 512 {
		checkLen = 512
	}
	nonPrintable := 0
	for i := 0; i < checkLen; i++ {
		c := data[i]
		if c == 0 {
			return true
		}
		if c < 0x09 || (c > 0x0d && c < 0x20) {
			nonPrintable++
		}
	}
	return nonPrintable*8 > checkLen
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
	actorType, actorID, actorRole := auditActorFromOwner("user:"+userID, false)
	claimDetail := map[string]any{
		"userId":    userID,
		"sessionId": sessionID,
		"source":    "explicit",
		"auto":      false,
	}
	writeClaimError := func(apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "anonymous.claim", "", "anonymous_session", sessionID, claimDetail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	if sessionID == "" {
		writeClaimError(NewError(CodeInvalidInput, "anonymous_session", "sessionId is required"))
		return
	}
	result, err := s.deployer.ClaimAnonymousSession(r.Context(), sessionID, userID)
	if err != nil {
		code := CodeInvalidInput
		if errors.Is(err, store.ErrNotFound) {
			code = CodeNotFound
		}
		writeClaimError(NewError(code, "anonymous_session", err.Error()))
		return
	}
	clearAnonymousSessionCookie(w, s.cookieSecureForRequest(r))
	claimDetail["siteCount"] = result.SiteCount
	claimDetail["deployCount"] = result.DeployCount
	claimDetail["alreadyClaimed"] = result.AlreadyClaimed
	s.recordAuditLog(r, actorType, actorID, actorRole, "anonymous.claim", "", "anonymous_session", result.SessionID, map[string]any{
		"userId":         result.UserID,
		"sessionId":      result.SessionID,
		"siteCount":      result.SiteCount,
		"deployCount":    result.DeployCount,
		"alreadyClaimed": result.AlreadyClaimed,
		"source":         "explicit",
		"auto":           false,
	})
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
			setAnonymousSessionCookie(w, sess.ID, s.cookieSecureForRequest(r))
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
	setAnonymousSessionCookie(w, sess.ID, s.cookieSecureForRequest(r))
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

// cookieSecure 根据配置决定 cookie 是否需要 Secure 标志（仅 HTTPS 模式）。
// 开发环境（AppURLScheme == http）下发 Secure 会导致浏览器在 http://localhost
// 下保留 cookie 失败，因此开发模式不设置 Secure。
func (s *Server) cookieSecure() bool {
	return s.cfg.AppURLScheme == "https"
}

func (s *Server) cookieSecureForRequest(r *http.Request) bool {
	if r != nil {
		if r.TLS != nil {
			return true
		}
		if proto := firstForwardedProto(r.Header.Get("X-Forwarded-Proto")); proto != "" {
			return proto == "https"
		}
		switch forwardedProto(r.Header.Get("Forwarded")) {
		case "https":
			return true
		case "http":
			return false
		}
		if r.URL != nil {
			switch strings.ToLower(strings.TrimSpace(r.URL.Scheme)) {
			case "https":
				return true
			case "http":
				return false
			}
		}
	}
	if isTruthyEnv(os.Getenv("HOSTCTL_DEV")) {
		return false
	}
	return s.cookieSecure()
}

func firstForwardedProto(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	first, _, _ := strings.Cut(header, ",")
	return strings.ToLower(strings.TrimSpace(first))
}

func forwardedProto(header string) string {
	for _, part := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "proto") {
			continue
		}
		return strings.ToLower(strings.Trim(strings.TrimSpace(value), `"`))
	}
	return ""
}

func isTruthyEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// secureCookie 是带 HttpOnly + SameSite + 自动 Secure 的辅助函数。
// name/secure/maxAge/path 透传给 http.SetCookie。
func (s *Server) secureCookie(w http.ResponseWriter, name, value string, maxAge int, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		Secure:   s.cookieSecure(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func setAnonymousSessionCookie(w http.ResponseWriter, id string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "hostctl_anon_session",
		Value:    id,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearAnonymousSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "hostctl_anon_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func setAdminSessionCookie(w http.ResponseWriter, value string, maxAge int, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "hostctl_admin_session",
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
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

// ===== 创作市场（公开 API：/api/deploys） =====

// marketplaceDeployResponse 是 GET /api/deploys 列表里单条记录的 JSON 形态，
// 字段命名保持稳定，便于前端和 Agent 复用。
type marketplaceDeployResponse struct {
	ID                     string        `json:"id"`
	Code                   string        `json:"code"`
	Owned                  bool          `json:"owned,omitempty"`
	CanManage              bool          `json:"canManage,omitempty"`
	CurrentVersionID       *string       `json:"currentVersionId,omitempty"`
	PrimaryVersionStrategy string        `json:"primaryVersionStrategy"`
	Title                  string        `json:"title"`
	Description            string        `json:"description"`
	Filename               string        `json:"filename"`
	FilePath               string        `json:"filePath"`
	FileSize               int64         `json:"fileSize"`
	QrCodePath             string        `json:"qrCodePath"`
	PrimaryVersionID       *string       `json:"primaryVersionId"`
	CreatedAt              string        `json:"createdAt"`
	UpdatedAt              string        `json:"updatedAt"`
	ViewCount              int64         `json:"viewCount"`
	LikeCount              int64         `json:"likeCount"`
	ReuseCount             int64         `json:"reuseCount"`
	TemplateSourceCode     string        `json:"templateSourceCode,omitempty"`
	TemplateSourceVersion  *int64        `json:"templateSourceVersion,omitempty"`
	FavoriteCount          int64         `json:"favoriteCount"`
	Favorited              bool          `json:"favorited"`
	VersionCount           int           `json:"versionCount"`
	ExpiresAt              *string       `json:"expiresAt"`
	Status                 string        `json:"status"`
	Visibility             string        `json:"visibility"`
	ReusePolicy            string        `json:"reusePolicy"`
	SourceDownloadPolicy   string        `json:"sourceDownloadPolicy"`
	SecurityMode           string        `json:"securityMode"`
	Category               string        `json:"category,omitempty"`
	Tags                   []string      `json:"tags,omitempty"`
	AccessProtected        bool          `json:"accessProtected"`
	IsPinned               bool          `json:"isPinned"`
	PinnedAt               *string       `json:"pinnedAt"`
	Bundle                 *BundleDetail `json:"bundle,omitempty"`
	Files                  []ContentFile `json:"files,omitempty"`
	Reuse                  *ReuseDetail  `json:"reuse,omitempty"`
}

var marketCategorySlugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?$`)

func DefaultMarketCategories() []MarketCategory {
	return []MarketCategory{
		{Slug: "landing", Label: "活动落地页", Note: "官网、活动、产品介绍"},
		{Slug: "dashboard", Label: "数据看板", Note: "报表、监控、可视化"},
		{Slug: "docs", Label: "文档报告", Note: "Markdown、教程、说明书"},
		{Slug: "tool", Label: "效率工具", Note: "表单、计算器、工作流"},
		{Slug: "game", Label: "互动游戏", Note: "小游戏、课堂互动"},
		{Slug: "screen", Label: "屏幕展示", Note: "门店、展厅、会议屏"},
	}
}

func NormalizeMarketCategories(categories []MarketCategory) ([]MarketCategory, *APIError) {
	out := make([]MarketCategory, 0, len(categories))
	seen := map[string]bool{}
	for _, item := range categories {
		slug := strings.ToLower(strings.TrimSpace(item.Slug))
		label := strings.TrimSpace(item.Label)
		note := strings.TrimSpace(item.Note)
		if slug == "" && label == "" && note == "" {
			continue
		}
		if !marketCategorySlugRe.MatchString(slug) {
			return nil, NewError(CodeInvalidInput, "market_categories", "category slug must use lowercase letters, numbers, and hyphens")
		}
		if label == "" {
			return nil, NewError(CodeInvalidInput, "market_categories", "category label is required")
		}
		if seen[slug] {
			return nil, NewError(CodeInvalidInput, "market_categories", "category slug must be unique")
		}
		seen[slug] = true
		out = append(out, MarketCategory{Slug: slug, Label: label, Note: note})
	}
	if len(out) == 0 {
		return nil, NewError(CodeInvalidInput, "market_categories", "at least one category is required")
	}
	return out, nil
}

func (s *Server) handleGetMarketCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.deployer.GetMarketCategories(r.Context())
	if err != nil {
		writeError(w, NewError(CodeInternal, "market_categories", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, MarketCategoriesResponse{Categories: categories})
}

func (s *Server) handleAdminPutMarketCategories(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	tok, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req MarketCategoriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "market_categories", "invalid JSON body"), reqID))
		return
	}
	actorType, actorID, actorRole := auditActorFromToken(tok)
	writeMarketCategoriesError := func(detail map[string]any, apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "config.market_categories", "", "config", "market_categories", detail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	categories, apiErr := NormalizeMarketCategories(req.Categories)
	if apiErr != nil {
		writeMarketCategoriesError(map[string]any{"count": len(req.Categories)}, apiErr)
		return
	}
	if err := s.deployer.SetMarketCategories(r.Context(), categories); err != nil {
		writeMarketCategoriesError(map[string]any{"count": len(categories)}, NewError(CodeInternal, "market_categories", err.Error()))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "config.market_categories", "", "config", "market_categories", map[string]any{
		"count": len(categories),
	})
	writeJSON(w, http.StatusOK, MarketCategoriesResponse{Success: true, Categories: categories})
}

// handleListMarketplace 处理 GET /api/deploys —— 公开列出所有 deploy（创作市场）。
// 支持 query: q / status / sort / page / pageSize
func (s *Server) handleListMarketplace(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := r.URL.Query().Get("status")
	sort := r.URL.Query().Get("sort")
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	pageSize := parseIntDefault(r.URL.Query().Get("pageSize"), 24)
	if kind == "" {
		switch category {
		case "html", "md", "markdown", "protected", "featured", "mine", "favorites":
			kind = category
			category = ""
		}
	}

	actor, isAdmin := s.marketplaceActor(r)
	ownerTokenID := ""
	if kind == "mine" || kind == "favorites" {
		if actor == "" {
			writeError(w, NewError(CodeUnauthorized, "marketplace", "login required"))
			return
		}
		if kind == "mine" {
			ownerTokenID = actor
		}
	}

	deploys, total, err := s.deployer.ListMarketplaceDeploys(r.Context(), q, status, sort, category, kind, ownerTokenID, actor, page, pageSize)
	if err != nil {
		writeError(w, NewError(CodeInternal, "marketplace", "list marketplace: "+err.Error()))
		return
	}

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
	resp := s.toMarketplaceResponse(r, d, actor, isAdmin)
	s.enrichMarketplaceDetail(r, &resp, d.Title, d.CurrentVersion)
	writeJSON(w, http.StatusOK, resp)
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

func (s *Server) handleFavoriteDeploy(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "favorite", "missing code"), reqID))
		return
	}
	actor, _, authErr := s.authenticateActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req struct {
		Favorited bool `json:"favorited"`
	}
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if _, err := s.deployer.GetSite(r.Context(), code); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "favorite", "deploy not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "favorite", "get site: "+err.Error()), reqID))
		return
	}
	count, favorited, err := s.deployer.SetFavorite(r.Context(), code, actor, req.Favorited)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "favorite", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"code":          code,
		"favorited":     favorited,
		"favoriteCount": count,
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
	owned := actor != "" && d.OwnerTokenID == actor
	return marketplaceDeployResponse{
		ID:                     d.ID,
		Code:                   d.Code,
		Owned:                  owned,
		CanManage:              isAdmin || owned,
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
		ReuseCount:             d.ReuseCount,
		TemplateSourceCode:     d.TemplateSourceCode,
		TemplateSourceVersion:  d.TemplateSourceVersion,
		FavoriteCount:          d.FavoriteCount,
		Favorited:              d.Favorited,
		VersionCount:           d.VersionCount,
		ExpiresAt:              expiresAt,
		Status:                 d.Status,
		Visibility:             d.Visibility,
		ReusePolicy:            normalizeReusePolicy(d.ReusePolicy),
		SourceDownloadPolicy:   normalizeReusePolicy(d.SourceDownloadPolicy),
		SecurityMode:           normalizeSiteSecurityMode(d.SecurityMode),
		Category:               d.Category,
		Tags:                   splitSiteTags(d.Tags),
		AccessProtected:        d.AccessProtected,
		IsPinned:               d.IsPinned,
		PinnedAt:               pinnedAt,
	}
}

type detailReusePolicy struct {
	CanManage            bool
	Authenticated        bool
	AccessProtected      bool
	Visibility           string
	Status               string
	ReusePolicy          string
	SourceDownloadPolicy string
	SecurityMode         string
}

func (s *Server) enrichMarketplaceDetail(r *http.Request, resp *marketplaceDeployResponse, title string, versionPtr *int64) {
	actor, isAdmin := s.marketplaceActor(r)
	policy := detailReusePolicy{
		CanManage:            resp.CanManage,
		Authenticated:        isAdmin || authenticatedUserActor(actor),
		AccessProtected:      resp.AccessProtected,
		Visibility:           resp.Visibility,
		Status:               resp.Status,
		ReusePolicy:          resp.ReusePolicy,
		SourceDownloadPolicy: resp.SourceDownloadPolicy,
		SecurityMode:         resp.SecurityMode,
	}
	bundle, files, reuse, apiErr := s.deployDetailExtras(r, resp.Code, title, versionPtr, policy)
	if apiErr != nil {
		s.logger.Printf("failed to enrich marketplace detail code=%s: %s", resp.Code, apiErr.Detail)
		return
	}
	resp.Bundle = bundle
	resp.Files = files
	resp.Reuse = reuse
}

func (s *Server) deployDetailExtras(r *http.Request, code, title string, versionPtr *int64, policy detailReusePolicy) (*BundleDetail, []ContentFile, *ReuseDetail, *APIError) {
	content, apiErr := s.deployer.GetContent(r.Context(), code, versionPtr)
	if apiErr != nil {
		return nil, nil, nil, apiErr
	}
	files := contentFilesWithoutBody(content.Files)
	bundle := s.bundleDetailForContent(r.Context(), code, content, files, policy.SecurityMode)
	reuse := s.reuseDetailForContent(r, code, title, content, policy)
	return bundle, files, reuse, nil
}

func contentFilesWithoutBody(files []ContentFile) []ContentFile {
	if len(files) == 0 {
		return nil
	}
	out := make([]ContentFile, 0, len(files))
	for _, file := range files {
		file.Content = ""
		out = append(out, file)
	}
	return out
}

func (s *Server) bundleDetailForContent(ctx context.Context, code string, content *GetContentResponse, files []ContentFile, siteSecurityMode string) *BundleDetail {
	if content == nil {
		return nil
	}
	kind := bundleKindFromFiles(files, content.MainEntry, "")
	root := ""
	securityMode := defaultSecurityMode(kind)
	tree := treeJSONFromFiles(files)
	if reader, ok := s.deployer.(bundleMetadataReader); ok {
		if meta, err := reader.GetVersionBundle(ctx, code, content.Version); err == nil {
			if strings.TrimSpace(meta.Kind) != "" {
				kind = strings.TrimSpace(meta.Kind)
			}
			root = strings.TrimSpace(meta.Root)
			if strings.TrimSpace(meta.MainEntry) != "" {
				content.MainEntry = strings.TrimSpace(meta.MainEntry)
			}
			if strings.TrimSpace(meta.SecurityMode) != "" {
				securityMode = strings.TrimSpace(meta.SecurityMode)
			}
			if raw := strings.TrimSpace(meta.TreeJSON); raw != "" && json.Valid([]byte(raw)) {
				tree = json.RawMessage(raw)
			}
		}
	}
	kind = bundleKindFromFiles(files, content.MainEntry, root, kind)
	siteMode := normalizeSiteSecurityMode(siteSecurityMode)
	effectiveMode := effectiveSiteSecurityMode(siteMode, securityMode)
	return &BundleDetail{
		Kind:                  kind,
		KindLabel:             bundleKindLabel(kind, root, len(files)),
		Root:                  root,
		MainEntry:             content.MainEntry,
		SecurityMode:          securityMode,
		SiteSecurityMode:      siteMode,
		EffectiveSecurityMode: effectiveMode,
		FileCount:             len(files),
		TotalSize:             content.TotalSize,
		Tree:                  tree,
		EntryNote:             bundleEntryNote(kind, root, content.MainEntry, len(files)),
	}
}

func treeJSONFromFiles(files []ContentFile) json.RawMessage {
	if len(files) == 0 {
		return json.RawMessage(`[]`)
	}
	type treeFile struct {
		Path     string `json:"path"`
		Size     int64  `json:"size"`
		IsBinary bool   `json:"isBinary"`
		SHA256   string `json:"sha256,omitempty"`
	}
	out := make([]treeFile, 0, len(files))
	for _, file := range files {
		out = append(out, treeFile{Path: file.Path, Size: file.Size, IsBinary: file.IsBinary, SHA256: file.Sha256})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return json.RawMessage(b)
}

func bundleKindFromFiles(files []ContentFile, entry, root string, preferred ...string) string {
	if isMarkdownEntry(entry) {
		return "markdown"
	}
	if len(preferred) > 0 {
		switch strings.ToLower(strings.TrimSpace(preferred[0])) {
		case "markdown":
			return "markdown"
		case "single_html", "static_site", "zip_site":
			return strings.ToLower(strings.TrimSpace(preferred[0]))
		case "html":
			if strings.TrimSpace(root) != "" {
				return "zip_site"
			}
			if len(files) <= 1 {
				return "single_html"
			}
			return "static_site"
		}
	}
	if strings.TrimSpace(root) != "" {
		return "zip_site"
	}
	if len(files) <= 1 {
		return "single_html"
	}
	return "static_site"
}

func isMarkdownEntry(entry string) bool {
	lower := strings.ToLower(strings.TrimSpace(entry))
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

func defaultSecurityMode(kind string) string {
	if kind == "markdown" {
		return "strict"
	}
	return "standard"
}

func bundleKindLabel(kind, root string, fileCount int) string {
	switch kind {
	case "markdown":
		return "Markdown 文档"
	case "single_html":
		return "单 HTML"
	case "static_site":
		return "多文件静态站点"
	case "zip_site":
		return "ZIP 静态站点"
	case "html":
		if strings.TrimSpace(root) != "" {
			return "ZIP 静态站点"
		}
		if fileCount <= 1 {
			return "单 HTML"
		}
		return "多文件静态站点"
	default:
		if kind == "" {
			return "未知 Bundle"
		}
		return kind
	}
}

func bundleEntryNote(kind, root, entry string, fileCount int) string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		entry = "未识别"
	}
	switch kind {
	case "markdown":
		return fmt.Sprintf("Markdown 入口为 %s，使用严格渲染策略并隔离用户脚本。", entry)
	case "single_html":
		return fmt.Sprintf("单文件 HTML 入口为 %s。", entry)
	case "static_site":
		return fmt.Sprintf("多文件静态站点入口为 %s，相对资源按站点目录解析。", entry)
	case "zip_site":
		return fmt.Sprintf("ZIP 包已识别站点根目录 %s，入口文件为 %s。", rootOrCurrent(root), entry)
	case "html":
		if strings.TrimSpace(root) != "" {
			return fmt.Sprintf("ZIP 包已识别站点根目录 %s，入口文件为 %s。", rootOrCurrent(root), entry)
		}
		if fileCount <= 1 {
			return fmt.Sprintf("单文件 HTML 入口为 %s。", entry)
		}
		return fmt.Sprintf("多文件静态站点入口为 %s，相对资源按站点目录解析。", entry)
	default:
		return fmt.Sprintf("入口文件为 %s。", entry)
	}
}

func rootOrCurrent(root string) string {
	if strings.TrimSpace(root) == "" {
		return "."
	}
	return strings.TrimSpace(root)
}

func (s *Server) reuseDetailForContent(r *http.Request, code, title string, content *GetContentResponse, policy detailReusePolicy) *ReuseDetail {
	if content == nil {
		return nil
	}
	base := s.requestBaseURL(r)
	escapedCode := url.QueryEscape(code)
	downloadURL := fmt.Sprintf("%s/api/deploy/content?code=%s&version=%d&download=1", base, escapedCode, content.Version)
	detailURL := fmt.Sprintf("%s/api/deploys/%s", base, url.PathEscape(code))
	allowDownload, allowReuse, policyNote := reusePolicy(policy)
	if !allowReuse {
		return &ReuseDetail{
			DetailURL:     detailURL,
			AllowReuse:    false,
			AllowDownload: allowDownload,
			PolicyNote:    policyNote,
		}
	}
	if !allowDownload {
		downloadURL = ""
	}
	displayTitle := strings.TrimSpace(title)
	if displayTitle == "" {
		displayTitle = strings.TrimSpace(content.Title)
	}
	if displayTitle == "" {
		displayTitle = code
	}
	outputDir := "pagepilot-template"
	deployPath := "./pagepilot-template"
	if len(content.Files) <= 1 {
		deployPath = "./pagepilot-template/index.html"
	} else {
		deployPath = fmt.Sprintf("./pagepilot-template/%s-v%d.zip", code, content.Version)
	}
	mcp := map[string]any{
		"tool":                    "deploy_site",
		"template_source_code":    code,
		"template_source_version": content.Version,
		"reuse_mode":              "new",
	}
	return &ReuseDetail{
		DownloadURL: downloadURL,
		DetailURL:   detailURL,
		CLI: fmt.Sprintf(
			"pagep market show %s\npagep get %s --version %d --download --output ./%s\npagep deploy %s --description \"基于 %s 二次创作\" --title \"新作品标题\" --template-source-code %s --template-source-version %d",
			code,
			code,
			content.Version,
			outputDir,
			deployPath,
			displayTitle,
			code,
			content.Version,
		),
		AgentPrompt: fmt.Sprintf(
			"请参考 PagePilot 作品《%s》（code: %s，version: %d）的文件结构和交互方式，基于用户的新需求重新创作。发布新作品时传 templateSourceCode=%s、templateSourceVersion=%d，让 PagePilot 记录模板来源和复用计数。不要直接复制敏感内容。",
			displayTitle,
			code,
			content.Version,
			code,
			content.Version,
		),
		MCP:                   mcp,
		AllowReuse:            true,
		AllowDownload:         allowDownload,
		PolicyNote:            policyNote,
		TemplateSourceCode:    code,
		TemplateSourceVersion: content.Version,
	}
}

func reusePolicy(policy detailReusePolicy) (allowDownload, allowReuse bool, note string) {
	if policy.CanManage {
		return true, true, "你是站点所有者或管理员，可以下载源码并复用为新发布。"
	}
	if strings.TrimSpace(policy.Status) != "" && policy.Status != "active" {
		return false, false, "站点当前未上架，源码下载和模板复用仅限所有者或管理员。"
	}
	if policy.AccessProtected {
		return false, false, "加密作品不提供源码下载和模板复用；如需复用，请先关闭访问密码后再下载。"
	}
	if !policy.Authenticated {
		return false, false, "源码下载和模板复用需要先登录；已登录用户或用户 Token 可按站点策略下载。"
	}
	sourcePolicy := normalizeReusePolicy(policy.SourceDownloadPolicy)
	reusePolicyValue := normalizeReusePolicy(policy.ReusePolicy)
	sourceAllowed := sourcePolicy == "allow" || (sourcePolicy == "auto" && policy.Visibility == "public" && !policy.AccessProtected)
	reuseAllowed := reusePolicyValue == "allow" || (reusePolicyValue == "auto" && sourceAllowed)
	if sourcePolicy == "deny" {
		sourceAllowed = false
	}
	if reusePolicyValue == "deny" {
		reuseAllowed = false
	}
	if sourceAllowed && reuseAllowed {
		if sourcePolicy == "allow" || reusePolicyValue == "allow" {
			return true, true, "站点策略已允许源码下载和模板复用。"
		}
		return true, true, "公开且未加密的作品可以下载源码并作为模板复用。"
	}
	if sourceAllowed {
		return true, false, "站点策略允许源码下载，但未开放模板复用。"
	}
	if policy.Visibility != "public" {
		return false, false, "不公开站点只允许通过链接浏览，源码下载和模板复用仅限所有者或管理员。"
	}
	return false, false, "站点策略未开放源码下载和模板复用。"
}

func authenticatedUserActor(actor string) bool {
	userID := strings.TrimPrefix(strings.TrimSpace(actor), "user:")
	return strings.HasPrefix(strings.TrimSpace(actor), "user:") && userID != ""
}

func normalizeReusePolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "allow", "deny":
		return strings.ToLower(strings.TrimSpace(policy))
	default:
		return "auto"
	}
}

func isReusePolicyInput(policy string) bool {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "auto", "allow", "deny":
		return true
	default:
		return false
	}
}

func normalizeSiteSecurityMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "strict", "compatible", "trusted":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "auto"
	}
}

func isSiteSecurityModeInput(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "auto", "strict", "compatible", "trusted":
		return true
	default:
		return false
	}
}

func effectiveSiteSecurityMode(siteMode, bundleMode string) string {
	siteMode = normalizeSiteSecurityMode(siteMode)
	if siteMode != "auto" {
		return siteMode
	}
	bundleMode = strings.ToLower(strings.TrimSpace(bundleMode))
	switch bundleMode {
	case "strict", "compatible", "trusted", "standard":
		return bundleMode
	default:
		return "standard"
	}
}

func splitSiteTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		value := strings.TrimSpace(strings.Trim(part, "# "))
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
		if len(out) >= 6 {
			break
		}
	}
	return out
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
	versionNumber, apiErr := siteAccessLoginVersion(r, site)
	if apiErr != nil {
		s.recordSiteAccessLoginAudit(r, code, 0, "failed", apiErr.Stage)
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	if auth.VerifyPassword(req.Password, site.AccessPasswordHash) {
		setSiteAccessCookie(w, code, site.AccessPasswordHash, versionNumber, s.cookieSecureForRequest(r))
		s.recordSiteAccessLoginAudit(r, code, versionNumber, "success", "")
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	s.recordSiteAccessLoginAudit(r, code, versionNumber, "failed", "incorrect_password")
	writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "access_password", "password is incorrect"), reqID))
}

func (s *Server) recordSiteAccessLoginAudit(r *http.Request, code string, versionNumber int64, result string, reason string) {
	detail := map[string]any{
		"versionNumber": versionNumber,
	}
	if strings.TrimSpace(reason) != "" {
		detail["reason"] = strings.TrimSpace(reason)
	}
	s.recordAuditLogWithResult(r, "browser", "", "public", "site.access_login", code, "site", code, result, detail)
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
	actorType, actorID, actorRole := s.auditActorFromRequest(r)
	accessPasswordAuditDetail := func() map[string]any {
		return map[string]any{
			"accessProtected": password != "",
		}
	}
	writeAccessPasswordError := func(apiErr *APIError) {
		s.recordAuditLogWithResult(r, actorType, actorID, actorRole, "site.access_password", code, "site", code, "failed",
			mergeAPIErrorAuditDetail(accessPasswordAuditDetail(), apiErr))
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	if password != "" && len(password) < 8 {
		writeAccessPasswordError(NewError(CodeInvalidInput, "access_password", "password must be at least 8 characters"))
		return
	}
	if err := s.deployer.SetSiteAccessPassword(r.Context(), code, password); err != nil {
		writeAccessPasswordError(NewError(CodeInternal, "access_password", err.Error()))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "site.access_password", code, "site", code, accessPasswordAuditDetail())
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
	rawVisibility := strings.TrimSpace(req.Visibility)
	visibility := normalizeSiteVisibility(req.Visibility)
	actorType, actorID, actorRole := s.auditActorFromRequest(r)
	visibilityAuditDetail := func() map[string]any {
		value := visibility
		if value == "" {
			value = rawVisibility
		}
		return map[string]any{
			"visibility": value,
		}
	}
	writeVisibilityError := func(apiErr *APIError) {
		s.recordAuditLogWithResult(r, actorType, actorID, actorRole, "site.visibility", code, "site", code, "failed",
			mergeAPIErrorAuditDetail(visibilityAuditDetail(), apiErr))
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	if visibility == "" {
		writeVisibilityError(NewError(CodeInvalidInput, "visibility", "visibility must be public or unlisted"))
		return
	}
	if err := s.deployer.SetSiteVisibility(r.Context(), code, visibility); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeVisibilityError(NewError(CodeNotFound, "site", "site not found"))
			return
		}
		writeVisibilityError(NewError(CodeInternal, "visibility", err.Error()))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "site.visibility", code, "site", code, visibilityAuditDetail())
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

func siteAccessLoginVersion(r *http.Request, site store.Site) (int64, *APIError) {
	raw := strings.TrimSpace(r.URL.Query().Get("version"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("v"))
	}
	if raw == "" {
		return siteAccessCookieVersion(site, nil), nil
	}
	version, err := parseInt64(raw)
	if err != nil || version <= 0 {
		return 0, NewError(CodeInvalidInput, "version", "version must be a positive integer")
	}
	return version, nil
}

func siteAccessCookieVersion(site store.Site, versionPtr *int64) int64 {
	if versionPtr != nil && *versionPtr > 0 {
		return *versionPtr
	}
	if site.CurrentVersion != nil && *site.CurrentVersion > 0 {
		return *site.CurrentVersion
	}
	return 0
}

func setSiteAccessCookie(w http.ResponseWriter, code, passwordHash string, version int64, secure bool) {
	expiresAt := time.Now().UTC().Add(siteAccessCookieTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     siteAccessCookieName(code),
		Value:    siteAccessCookieValue(code, passwordHash, version, expiresAt),
		Path:     "/",
		MaxAge:   int(siteAccessCookieTTL.Seconds()),
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func siteAccessCookieName(code string) string {
	return "pagepilot_access_" + code
}

func siteAccessCookieValue(code, passwordHash string, version int64, expiresAt time.Time) string {
	versionText := strconv.FormatInt(version, 10)
	expiresUnix := strconv.FormatInt(expiresAt.Unix(), 10)
	return strings.Join([]string{
		versionText,
		expiresUnix,
		siteAccessCookieSignature(code, passwordHash, versionText, expiresUnix),
	}, ".")
}

func siteAccessCookieSignature(code, passwordHash, version, expiresUnix string) string {
	mac := hmac.New(sha256.New, []byte(passwordHash))
	_, _ = mac.Write([]byte(code))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(version))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(expiresUnix))
	return hex.EncodeToString(mac.Sum(nil))
}

func validSiteAccessCookie(value, code, passwordHash string, version int64, now time.Time) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return false
	}
	if parts[0] != strconv.FormatInt(version, 10) {
		return false
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || expiresUnix <= now.Unix() {
		return false
	}
	expected := siteAccessCookieSignature(code, passwordHash, parts[0], parts[1])
	return subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expected)) == 1
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

func parseOptionalRFC3339(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
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

	actorType, actorID, actorRole := s.auditActorFromRequest(r)
	targetID := strconv.FormatInt(version, 10)
	detail := map[string]any{"locked": req.Locked}
	resp, apiErr := s.deployer.LockVersion(r.Context(), code, version, req.Locked)
	if apiErr != nil {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "version.lock", code, "version", targetID, detail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "version.lock", code, "version", targetID, detail)
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

	actorType, actorID, actorRole := s.auditActorFromRequest(r)
	targetID := ""
	if req.VersionNumber != nil {
		targetID = strconv.FormatInt(*req.VersionNumber, 10)
	} else if strings.TrimSpace(req.VersionID) != "" {
		targetID = strings.TrimSpace(req.VersionID)
	}
	detail := map[string]any{
		"versionNumber": req.VersionNumber,
		"versionId":     req.VersionID,
	}
	writeCurrentError := func(apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "version.current", code, "version", targetID, detail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}

	var resp *SetCurrentResponse
	var apiErr *APIError
	switch {
	case req.VersionNumber != nil && *req.VersionNumber > 0:
		resp, apiErr = s.deployer.SwitchCurrent(r.Context(), code, *req.VersionNumber)
	case req.VersionID != "":
		resp, apiErr = s.deployer.SwitchCurrentByUUID(r.Context(), code, req.VersionID)
	default:
		writeCurrentError(NewError(CodeInvalidInput, "validate",
			"provide either 'versionNumber' or 'versionId'"))
		return
	}
	if apiErr != nil {
		writeCurrentError(apiErr)
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "version.current", code, "version", strconv.FormatInt(resp.CurrentVersion, 10), detail)
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
	actorType, actorID, actorRole := s.auditActorFromRequest(r)
	targetID := strconv.FormatInt(version, 10)

	r.Body = http.MaxBytesReader(w, r.Body, s.maxRequestBodyBytes())

	var req OverwriteRequest
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(ct, "application/json") {
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "parse_json",
				fmt.Sprintf("invalid JSON body: %v", err)), reqID))
			return
		}

		if statusVal, hasStatus := raw["status"]; hasStatus {
			status, ok := statusVal.(string)
			detail := map[string]any{"status": status}
			if !ok {
				detail["statusType"] = fmt.Sprintf("%T", statusVal)
				apiErr := NewError(CodeInvalidInput, "validate", "'status' must be a string")
				s.recordFailedAuditLog(r, actorType, actorID, actorRole, "version.status", code, "version", targetID, detail, apiErr)
				writeError(w, apiErrWithReqID(apiErr, reqID))
				return
			}
			resp, apiErr := s.deployer.SetVersionStatus(r.Context(), code, version, status)
			if apiErr != nil {
				s.recordFailedAuditLog(r, actorType, actorID, actorRole, "version.status", code, "version", targetID, detail, apiErr)
				writeError(w, apiErrWithReqID(apiErr, reqID))
				return
			}
			s.recordAuditLog(r, actorType, actorID, actorRole, "version.status", code, "version", targetID, detail)
			writeJSON(w, http.StatusOK, resp)
			return
		}

		// 覆盖模式：重新解析成结构化对象
		body, _ := json.Marshal(raw)
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "parse_json",
				fmt.Sprintf("invalid OverwriteRequest: %v", err)), reqID))
			return
		}
	} else if strings.HasPrefix(ct, "multipart/form-data") {
		parsed, apiErr := s.decodeDeployMultipart(r)
		if apiErr != nil {
			writeError(w, apiErrWithReqID(apiErr, reqID))
			return
		}
		req = OverwriteRequest{
			Description: parsed.Description,
			Title:       parsed.Title,
			Filename:    parsed.Filename,
			Content:     parsed.Content,
			Files:       parsed.Files,
		}
	} else {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "content_type",
			"Content-Type must be application/json or multipart/form-data"), reqID))
		return
	}

	overwriteDetail := map[string]any{
		"description":  req.Description,
		"title":        req.Title,
		"filename":     req.Filename,
		"fileCount":    len(req.Files),
		"contentBytes": len(req.Content),
	}
	resp, apiErr := s.deployer.OverwriteVersion(r.Context(), code, version, req)
	if apiErr != nil {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "version.overwrite", code, "version", targetID, overwriteDetail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	s.rewriteDeployResponseURLs(r, resp)
	s.recordAuditLog(r, actorType, actorID, actorRole, "version.overwrite", code, "version", strconv.FormatInt(version, 10), map[string]any{
		"newVersionNumber": resp.VersionNumber,
		"versionId":        resp.VersionID,
	})
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
	actorType, actorID, actorRole := s.auditActorFromRequest(r)
	targetID := strconv.FormatInt(version, 10)
	resp, apiErr := s.deployer.DeleteVersion(r.Context(), code, version)
	if apiErr != nil {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "version.delete", code, "version", targetID, nil, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "version.delete", code, "version", targetID, nil)
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
	download, _ := parseBoolParam(q.Get("download"))
	downloadDetail := sourceDownloadAuditDetail(download, versionPtr)
	actorType, actorID, actorRole := s.auditActorFromRequest(r)

	if apiErr := s.authorizeSourceContentRead(r, code); apiErr != nil {
		if download {
			s.recordFailedAuditLog(r, actorType, actorID, actorRole, "source_download", code, "site", code, downloadDetail, apiErr)
		}
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}

	// download 模式
	if download {
		if apiErr := s.deployer.StreamDownload(r.Context(), code, versionPtr, w); apiErr != nil {
			s.recordFailedAuditLog(r, actorType, actorID, actorRole, "source_download", code, "site", code, downloadDetail, apiErr)
			// 此时响应头可能已写，但出错时一般没写。
			writeError(w, apiErrWithReqID(apiErr, reqID))
			return
		}
		s.recordAuditLog(r, actorType, actorID, actorRole, "source_download", code, "site", code, downloadDetail)
		return
	}

	resp, apiErr := s.deployer.GetContent(r.Context(), code, versionPtr)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func sourceDownloadAuditDetail(download bool, versionPtr *int64) map[string]any {
	detail := map[string]any{"download": download}
	if versionPtr != nil {
		detail["version"] = *versionPtr
	}
	return detail
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
	actorOwner := ""
	if requesterUserID != "" {
		actorOwner = "user:" + requesterUserID
	}
	actorType, actorID, actorRole := auditActorFromOwner(actorOwner, isRequesterAdmin)
	tokenCreateDetail := func() map[string]any {
		return map[string]any{
			"label":       strings.TrimSpace(req.Label),
			"isAdmin":     req.IsAdmin,
			"ownerUserId": ownerUserID,
			"expiresAt":   strings.TrimSpace(req.ExpiresAt),
			"ttlSeconds":  req.TTLSeconds,
		}
	}
	writeTokenCreateError := func(apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "token.create", "", "token", "", tokenCreateDetail(), apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	if !isRequesterAdmin {
		if req.IsAdmin {
			writeTokenCreateError(NewError(CodeForbidden, "auth", "admin account required to create admin tokens"))
			return
		}
		if ownerUserID == "" {
			writeTokenCreateError(NewError(CodeForbidden, "auth", "token must belong to your user account"))
			return
		}
		if ownerUserID != requesterUserID {
			writeTokenCreateError(NewError(CodeForbidden, "owner", "you can only create tokens for your own user account"))
			return
		}
	}
	if ownerUserID != "" {
		if user, err := s.auth.GetUser(r.Context(), ownerUserID); err != nil || !user.IsActive {
			writeTokenCreateError(NewError(CodeInvalidInput, "owner", "ownerUserId is not an active user"))
			return
		}
	}
	if ownerUserID == "" {
		writeTokenCreateError(NewError(CodeInvalidInput, "owner", "token must belong to a user"))
		return
	}
	expiresAt, parseErr := parseTokenExpiresAt(req)
	if parseErr != nil {
		writeTokenCreateError(NewError(CodeInvalidInput, "expiresAt", parseErr.Error()))
		return
	}

	gen, err := s.auth.Generate(r.Context(), strings.TrimSpace(req.Label), req.IsAdmin, ownerUserID, expiresAt)
	if err != nil {
		writeTokenCreateError(NewError(CodeInvalidInput, "create_token",
			fmt.Sprintf("failed to create token: %v", err)))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "token.create", "", "token", gen.ID, map[string]any{
		"label":       gen.Label,
		"isAdmin":     gen.IsAdmin,
		"ownerUserId": gen.OwnerUserID,
		"expiresAt":   gen.ExpiresAt,
	})
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
	actorType, actorID, actorRole := auditActorFromOwner(actor, isAdmin)
	tokenRevokeDetail := func(target store.Token) map[string]any {
		return map[string]any{
			"ownerUserId": target.OwnerUserID,
			"isAdmin":     target.IsAdmin,
		}
	}
	if !isAdmin {
		tok, err := s.auth.GetToken(r.Context(), id)
		if err != nil || tok.OwnerUserID != actorUserID {
			apiErr := NewError(CodeForbidden, "auth", "you can only revoke your own tokens")
			s.recordFailedAuditLog(r, actorType, actorID, actorRole, "token.revoke", "", "token", id, tokenRevokeDetail(tok), apiErr)
			writeError(w, apiErrWithReqID(apiErr, reqID))
			return
		}
	}
	targetToken, _ := s.auth.GetToken(r.Context(), id)
	if err := s.auth.Revoke(r.Context(), id); err != nil {
		apiErr := NewError(CodeNotFound, "revoke_token",
			fmt.Sprintf("token %q not found or already revoked", id))
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "token.revoke", "", "token", id, tokenRevokeDetail(targetToken), apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "token.revoke", "", "token", id, map[string]any{
		"ownerUserId": targetToken.OwnerUserID,
		"isAdmin":     targetToken.IsAdmin,
	})
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
	versionNumber := siteAccessCookieVersion(site, versionPtr)
	if c, err := r.Cookie(siteAccessCookieName(code)); err == nil &&
		validSiteAccessCookie(c.Value, code, site.AccessPasswordHash, versionNumber, time.Now()) {
		return true
	}
	if s.validScreenAccessCookie(r, code, versionPtr, time.Now()) {
		return true
	}
	if s.registeredOwnerAccessAllowed(r, site) {
		return true
	}
	return false
}

func (s *Server) registeredOwnerAccessAllowed(r *http.Request, site store.Site) bool {
	actor, isAdmin, authErr := s.optionalActor(r)
	if authErr != nil {
		return false
	}
	if isAdmin {
		return true
	}
	return authenticatedUserActor(actor) && actor == site.OwnerTokenID
}

func (s *Server) renderAccessPasswordPage(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>需要访问密码 - PagePilot</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f6fbff;color:#0f172a;font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.card{width:min(420px,calc(100%% - 32px));border:1px solid #dbeafe;border-radius:16px;background:#fff;box-shadow:0 22px 60px rgba(14,116,144,.12);padding:24px}h1{margin:0;font-size:22px}.muted{color:#64748b;line-height:1.7}input{width:100%%;height:42px;border:1px solid #cbd5e1;border-radius:10px;padding:0 12px;font:inherit}button{width:100%%;height:42px;margin-top:12px;border:0;border-radius:10px;background:#0284c7;color:white;font-weight:800;cursor:pointer}.err{min-height:20px;color:#be123c;font-size:13px}</style></head><body><form class="card" id="f"><h1>这个网页已加密</h1><p class="muted">请输入访问密码后继续查看。</p><input id="p" type="password" autocomplete="current-password" placeholder="访问密码" autofocus><button type="submit">进入网页</button><p class="err" id="e"></p></form><script>document.getElementById("f").addEventListener("submit",async e=>{e.preventDefault();const r=await fetch("/api/deploys/%s/access",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({password:document.getElementById("p").value})});if(r.ok) location.reload(); else document.getElementById("e").textContent="密码不正确";});</script></body></html>`, code)
}

func (s *Server) renderAccessPasswordPageV2(w http.ResponseWriter, code string, versionPtr *int64) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	accessPath := fmt.Sprintf("/api/deploys/%s/access", url.PathEscape(code))
	if versionPtr != nil && *versionPtr > 0 {
		accessPath += "?version=" + strconv.FormatInt(*versionPtr, 10)
	}
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
<p>请输入访问密码后继续查看。验证通过后，本浏览器会在 5 分钟内记住访问权限。</p>
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
    const r=await fetch("%s",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({password:document.getElementById("p").value})});
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
</html>`, accessPath)
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

func (s *Server) authorizeSourceContentRead(r *http.Request, code string) *APIError {
	site, err := s.deployer.GetSite(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		return NewError(CodeNotFound, "site", "site not found")
	}
	if err != nil {
		return NewError(CodeInternal, "site", err.Error())
	}
	actor, isAdmin, authErr := s.sourceDownloadActor(r)
	if authErr != nil {
		return authErr
	}
	if isAdmin || (actor != "" && actor == site.OwnerTokenID) {
		return nil
	}
	if strings.TrimSpace(site.AccessPasswordHash) != "" {
		return NewError(CodeForbidden, "source_download",
			"encrypted sites only allow source download for owner or admin").
			WithHint("源码下载需要站点所有者或管理员权限；访问密码只允许浏览页面，不授予源码下载权限。")
	}
	allowDownload, _, _ := reusePolicy(detailReusePolicy{
		Authenticated:        true,
		AccessProtected:      strings.TrimSpace(site.AccessPasswordHash) != "",
		Visibility:           site.Visibility,
		Status:               site.Status,
		ReusePolicy:          site.ReusePolicy,
		SourceDownloadPolicy: site.SourceDownloadPolicy,
		SecurityMode:         site.SecurityMode,
	})
	if allowDownload {
		return nil
	}
	return NewError(CodeForbidden, "source_download",
		"source download is disabled by site policy").
		WithHint("源码下载需要登录，并且站点策略必须允许源码下载；访问密码只允许浏览页面，不授予源码下载权限。")
}

func (s *Server) sourceDownloadActor(r *http.Request) (string, bool, *APIError) {
	if user, ok := s.adminUserFromCookie(r); ok {
		if !user.IsActive {
			return "", false, NewError(CodeForbidden, "auth", "user is inactive")
		}
		return "user:" + user.ID, user.IsAdmin, nil
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		return "", false, NewError(CodeUnauthorized, "auth", "login or bearer token required").
			WithHint("源码下载需要先登录；Skill、CLI 或 MCP 请使用已绑定注册用户的 Token。")
	}
	tok, authErr := s.authenticateToken(r)
	if authErr != nil {
		return "", false, authErr
	}
	if strings.TrimSpace(tok.OwnerUserID) == "" {
		return "", false, NewError(CodeUnauthorized, "auth", "user-bound token required").
			WithHint("源码下载需要用户归属的 Token。")
	}
	user, err := s.auth.GetUser(r.Context(), tok.OwnerUserID)
	if err != nil || !user.IsActive {
		return "", false, NewError(CodeForbidden, "user", "token owner is inactive or missing")
	}
	return "user:" + strings.TrimSpace(tok.OwnerUserID), tok.IsAdmin || user.IsAdmin, nil
}

func auditActorFromToken(tok store.Token) (actorType, actorID, actorRole string) {
	actorRole = "user"
	if tok.IsAdmin {
		actorRole = "admin"
	}
	if strings.TrimSpace(tok.OwnerUserID) != "" {
		return "user", strings.TrimSpace(tok.OwnerUserID), actorRole
	}
	if strings.HasPrefix(tok.ID, "admin:") {
		return "user", strings.TrimPrefix(tok.ID, "admin:"), actorRole
	}
	if strings.TrimSpace(tok.ID) != "" {
		return "token", strings.TrimSpace(tok.ID), actorRole
	}
	return "unknown", "", actorRole
}

func auditActorFromOwner(owner string, isAdmin bool) (actorType, actorID, actorRole string) {
	actorRole = "user"
	if isAdmin {
		actorRole = "admin"
	}
	owner = strings.TrimSpace(owner)
	if strings.HasPrefix(owner, "user:") {
		return "user", strings.TrimPrefix(owner, "user:"), actorRole
	}
	if strings.HasPrefix(owner, "anon:") {
		return "anonymous", strings.TrimPrefix(owner, "anon:"), "anonymous"
	}
	if owner != "" {
		return "token", owner, actorRole
	}
	return "unknown", "", actorRole
}

func (s *Server) recordAuditLog(r *http.Request, actorType, actorID, actorRole, action, siteCode, targetType, targetID string, detail any) {
	s.recordAuditLogWithResult(r, actorType, actorID, actorRole, action, siteCode, targetType, targetID, "success", detail)
}

func (s *Server) recordAuditLogWithResult(r *http.Request, actorType, actorID, actorRole, action, siteCode, targetType, targetID, result string, detail any) {
	writer, ok := s.deployer.(auditLogWriter)
	if !ok {
		return
	}
	result = strings.TrimSpace(result)
	if result == "" {
		result = "success"
	}
	detailJSON := "{}"
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil && json.Valid(b) {
			detailJSON = string(b)
		}
	}
	if err := writer.RecordAuditLog(r.Context(), store.AuditLog{
		ActorType:  actorType,
		ActorID:    actorID,
		ActorRole:  actorRole,
		Action:     action,
		Result:     result,
		SiteCode:   siteCode,
		TargetType: targetType,
		TargetID:   targetID,
		IP:         clientIPFromRequest(r),
		UserAgent:  strings.TrimSpace(r.UserAgent()),
		DetailJSON: detailJSON,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		s.logger.Printf("failed to record audit log action=%s target=%s/%s: %v", action, targetType, targetID, err)
	}
}

func deployFailureAuditDetail(req DeployRequest, apiErr *APIError) map[string]any {
	detail := apiErrorAuditDetail(apiErr)
	if filename := strings.TrimSpace(req.Filename); filename != "" {
		detail["filename"] = filename
	}
	if title := strings.TrimSpace(req.Title); title != "" {
		detail["title"] = title
	}
	if description := strings.TrimSpace(req.Description); description != "" {
		detail["description"] = description
	}
	if customCode := strings.TrimSpace(req.CustomCode); customCode != "" {
		detail["customCode"] = customCode
	}
	if visibility := strings.TrimSpace(req.Visibility); visibility != "" {
		detail["visibility"] = visibility
	}
	if source := strings.TrimSpace(req.Source); source != "" {
		detail["source"] = source
	}
	detail["createVersion"] = req.CreateVersion
	detail["enableCustomCode"] = req.EnableCustomCode
	detail["fileCount"] = len(req.Files)
	return detail
}

func apiErrorAuditDetail(apiErr *APIError) map[string]any {
	detail := map[string]any{}
	if apiErr == nil {
		detail["errorCode"] = string(CodeInternal)
		detail["detail"] = "unknown error"
		return detail
	}
	detail["errorCode"] = string(apiErr.ErrorCode)
	if apiErr.Stage != "" {
		detail["stage"] = apiErr.Stage
	}
	if apiErr.Detail != "" {
		detail["detail"] = apiErr.Detail
	}
	if apiErr.Hint != "" {
		detail["hint"] = apiErr.Hint
	}
	if apiErr.RetryAfterSeconds != nil {
		detail["retryAfterSeconds"] = *apiErr.RetryAfterSeconds
	}
	return detail
}

func mergeAPIErrorAuditDetail(detail map[string]any, apiErr *APIError) map[string]any {
	if detail == nil {
		detail = map[string]any{}
	}
	for k, v := range apiErrorAuditDetail(apiErr) {
		detail[k] = v
	}
	return detail
}

func (s *Server) recordFailedAuditLog(
	r *http.Request,
	actorType string,
	actorID string,
	actorRole string,
	action string,
	siteCode string,
	targetType string,
	targetID string,
	detail map[string]any,
	apiErr *APIError,
) {
	s.recordAuditLogWithResult(r, actorType, actorID, actorRole, action, siteCode, targetType, targetID, "failed",
		mergeAPIErrorAuditDetail(detail, apiErr))
}

const cspReportMaxBytes = 64 << 10

type cspViolationReport struct {
	DocumentURI        string `json:"document-uri"`
	BlockedURI         string `json:"blocked-uri"`
	ViolatedDirective  string `json:"violated-directive"`
	EffectiveDirective string `json:"effective-directive"`
	OriginalPolicy     string `json:"original-policy"`
	SourceFile         string `json:"source-file"`
	LineNumber         int    `json:"line-number"`
	ColumnNumber       int    `json:"column-number"`
	StatusCode         int    `json:"status-code"`
	Referrer           string `json:"referrer"`
	Disposition        string `json:"disposition"`
	Sample             string `json:"script-sample"`
}

type reportingAPICSPReport struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Body struct {
		DocumentURL        string `json:"documentURL"`
		BlockedURL         string `json:"blockedURL"`
		EffectiveDirective string `json:"effectiveDirective"`
		ViolatedDirective  string `json:"violatedDirective"`
		OriginalPolicy     string `json:"originalPolicy"`
		SourceFile         string `json:"sourceFile"`
		LineNumber         int    `json:"lineNumber"`
		ColumnNumber       int    `json:"columnNumber"`
		StatusCode         int    `json:"statusCode"`
		Referrer           string `json:"referrer"`
		Disposition        string `json:"disposition"`
		Sample             string `json:"sample"`
	} `json:"body"`
}

func (s *Server) handleCSPReport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, cspReportMaxBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	report, ok := parseCSPViolationReport(body)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	siteCode := s.siteCodeFromCSPDocumentURI(report.DocumentURI)
	targetID := firstNonEmpty(report.EffectiveDirective, report.ViolatedDirective, "unknown")
	s.recordAuditLogWithResult(r, "browser", "", "public", "security.csp_report", siteCode, "csp", targetID, "reported", report.auditDetail())
	w.WriteHeader(http.StatusNoContent)
}

func parseCSPViolationReport(body []byte) (cspViolationReport, bool) {
	var envelope struct {
		Report cspViolationReport `json:"csp-report"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && !envelope.Report.empty() {
		return envelope.Report.normalized(), true
	}
	var report cspViolationReport
	if err := json.Unmarshal(body, &report); err == nil && !report.empty() {
		return report.normalized(), true
	}
	var reports []reportingAPICSPReport
	if err := json.Unmarshal(body, &reports); err == nil {
		for _, item := range reports {
			if strings.TrimSpace(item.Type) != "" && strings.TrimSpace(item.Type) != "csp-violation" {
				continue
			}
			report := cspViolationReport{
				DocumentURI:        firstNonEmpty(item.Body.DocumentURL, item.URL),
				BlockedURI:         item.Body.BlockedURL,
				EffectiveDirective: item.Body.EffectiveDirective,
				ViolatedDirective:  item.Body.ViolatedDirective,
				OriginalPolicy:     item.Body.OriginalPolicy,
				SourceFile:         item.Body.SourceFile,
				LineNumber:         item.Body.LineNumber,
				ColumnNumber:       item.Body.ColumnNumber,
				StatusCode:         item.Body.StatusCode,
				Referrer:           item.Body.Referrer,
				Disposition:        item.Body.Disposition,
				Sample:             item.Body.Sample,
			}
			if !report.empty() {
				return report.normalized(), true
			}
		}
	}
	return cspViolationReport{}, false
}

func (r cspViolationReport) empty() bool {
	return strings.TrimSpace(r.DocumentURI) == "" &&
		strings.TrimSpace(r.BlockedURI) == "" &&
		strings.TrimSpace(r.ViolatedDirective) == "" &&
		strings.TrimSpace(r.EffectiveDirective) == ""
}

func (r cspViolationReport) normalized() cspViolationReport {
	r.DocumentURI = auditText(r.DocumentURI, 2048)
	r.BlockedURI = auditText(r.BlockedURI, 2048)
	r.ViolatedDirective = auditText(r.ViolatedDirective, 256)
	r.EffectiveDirective = auditText(r.EffectiveDirective, 256)
	r.OriginalPolicy = auditText(r.OriginalPolicy, 4096)
	r.SourceFile = auditText(r.SourceFile, 2048)
	r.Referrer = auditText(r.Referrer, 2048)
	r.Disposition = auditText(r.Disposition, 64)
	r.Sample = auditText(r.Sample, 1024)
	return r
}

func (r cspViolationReport) auditDetail() map[string]any {
	detail := map[string]any{
		"documentUri":        r.DocumentURI,
		"blockedUri":         r.BlockedURI,
		"violatedDirective":  r.ViolatedDirective,
		"effectiveDirective": r.EffectiveDirective,
		"disposition":        r.Disposition,
	}
	if r.OriginalPolicy != "" {
		detail["originalPolicy"] = r.OriginalPolicy
	}
	if r.SourceFile != "" {
		detail["sourceFile"] = r.SourceFile
	}
	if r.LineNumber > 0 {
		detail["lineNumber"] = r.LineNumber
	}
	if r.ColumnNumber > 0 {
		detail["columnNumber"] = r.ColumnNumber
	}
	if r.StatusCode > 0 {
		detail["statusCode"] = r.StatusCode
	}
	if r.Referrer != "" {
		detail["referrer"] = r.Referrer
	}
	if r.Sample != "" {
		detail["sample"] = r.Sample
	}
	return detail
}

func (s *Server) siteCodeFromCSPDocumentURI(documentURI string) string {
	u, err := url.Parse(strings.TrimSpace(documentURI))
	if err != nil {
		return ""
	}
	if strings.HasPrefix(u.Path, "/agent/") {
		rest := strings.TrimPrefix(u.Path, "/agent/")
		code := strings.Split(rest, "/")[0]
		if routeCodeRe.MatchString(code) {
			return code
		}
	}
	appURL := s.appURLConfig()
	suffix := strings.TrimSpace(appURL.AppDomainSuffix)
	if suffix == "" || appURL.AppURLMode == AppURLModePath {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(stripHostPort(u.Host), "."))
	needle := "." + suffix
	if !strings.HasSuffix(host, needle) {
		return ""
	}
	code := strings.TrimSuffix(host, needle)
	if strings.Contains(code, ".") || !routeCodeRe.MatchString(code) {
		return ""
	}
	return code
}

func auditText(value string, limit int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) auditActorFromRequest(r *http.Request) (string, string, string) {
	actor, isAdmin, err := s.optionalActor(r)
	if err != nil {
		return "unknown", "", "unknown"
	}
	return auditActorFromOwner(actor, isAdmin)
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
	// 匿名 actor 永远不是管理员（SEC-06：避免匿名用户被误标 admin 绕过下游权限检查）
	return "anon:" + id, false, nil
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
	s.setAdminUISecurityHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(web.AdminHTML())
}

func (s *Server) handleUserAppUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	body, err := fs.ReadFile(web.UserSubFS(), "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	html := injectHTMLSnippets(string(body), s.mainHTMLInjectionSnippets(""))
	_, _ = io.WriteString(w, html)
}

func (s *Server) userAppFileServer() http.Handler {
	fileServer := http.FileServer(http.FS(web.UserSubFS()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			s.handleUserAppUI(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) handleAPIDocsRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin?tab=apiDocs", http.StatusFound)
}

func (s *Server) handleMarketRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/market", http.StatusFound)
}

func (s *Server) handleAgentGuideUI(w http.ResponseWriter, r *http.Request) {
	s.handleUserAppUI(w, r)
}

func (s *Server) handleScreenGuideUI(w http.ResponseWriter, r *http.Request) {
	s.handleUserAppUI(w, r)
}

func (s *Server) handleSkillDownload(w http.ResponseWriter, r *http.Request) {
	filename := "pagep.zip"
	path := s.managedSkillZipPath()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		data, embedErr := web.SkillPackage()
		if embedErr != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		http.ServeContent(w, r, filename, time.Time{}, bytes.NewReader(data))
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	http.ServeFile(w, r, path)
}

type adminSkillResponse struct {
	Success bool              `json:"success"`
	Package adminSkillPackage `json:"package"`
}

type adminSkillPackage struct {
	Exists    bool   `json:"exists"`
	Path      string `json:"path"`
	Source    string `json:"source"`
	Size      int64  `json:"size"`
	Sha256    string `json:"sha256,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func (s *Server) handleAdminGetSkill(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, adminSkillResponse{
		Success: true,
		Package: s.skillPackageInfo(),
	})
}

func (s *Server) handleAdminUploadSkillPackage(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	tok, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	actorType, actorID, actorRole := auditActorFromToken(tok)
	detail := map[string]any{}
	writeSkillPackageError := func(apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "skill.package_upload", "", "skill_package", "hostctl-deploy.zip", detail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	if err := r.ParseMultipartForm(21 << 20); err != nil {
		writeSkillPackageError(NewError(CodeInvalidInput, "skill_package", "multipart file is required"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeSkillPackageError(NewError(CodeInvalidInput, "skill_package", "file field is required"))
		return
	}
	defer file.Close()
	name := strings.ToLower(strings.TrimSpace(header.Filename))
	detail["filename"] = name
	if !strings.HasSuffix(name, ".zip") {
		writeSkillPackageError(NewError(CodeInvalidInput, "skill_package", "skill package must be a .zip file"))
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		writeSkillPackageError(NewError(CodeInternal, "skill_package", err.Error()))
		return
	}
	detail["size"] = len(data)
	if len(data) == 0 || len(data) > 20<<20 {
		writeSkillPackageError(NewError(CodeInvalidInput, "skill_package", "skill package size must be between 1 byte and 20 MB"))
		return
	}
	if err := validateSkillZip(data); err != nil {
		writeSkillPackageError(NewError(CodeInvalidInput, "skill_package", err.Error()))
		return
	}
	path := s.managedSkillZipPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeSkillPackageError(NewError(CodeInternal, "skill_package", err.Error()))
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hostctl-deploy-*.zip")
	if err != nil {
		writeSkillPackageError(NewError(CodeInternal, "skill_package", err.Error()))
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		writeSkillPackageError(NewError(CodeInternal, "skill_package", err.Error()))
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		writeSkillPackageError(NewError(CodeInternal, "skill_package", err.Error()))
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		writeSkillPackageError(NewError(CodeInternal, "skill_package", err.Error()))
		return
	}
	sum := sha256.Sum256(data)
	s.recordAuditLog(r, actorType, actorID, actorRole, "skill.package_upload", "", "skill_package", "hostctl-deploy.zip", map[string]any{
		"filename": name,
		"size":     len(data),
		"sha256":   hex.EncodeToString(sum[:]),
		"path":     path,
	})
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
		if data, embedErr := web.SkillPackage(); embedErr == nil {
			sum := sha256.Sum256(data)
			info.Exists = true
			info.Source = "built-in"
			info.Size = int64(len(data))
			info.Sha256 = hex.EncodeToString(sum[:])
		}
		return info
	}
	info.Exists = true
	info.Source = "uploaded"
	info.Size = st.Size()
	info.UpdatedAt = st.ModTime().UTC().Format(time.RFC3339)
	if data, err := os.ReadFile(path); err == nil {
		sum := sha256.Sum256(data)
		info.Sha256 = hex.EncodeToString(sum[:])
	}
	return info
}

// handleAdminSession validates the admin UI bearer token or admin cookie.
func (s *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	mode := "prod"
	if !s.requireAuth {
		mode = "dev"
	}
	optional := r.URL.Query().Get("optional") == "1"
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
		if optional {
			writeJSON(w, http.StatusOK, AdminSessionResponse{
				Success:    false,
				Mode:       mode,
				NeedsSetup: !hasAdmin,
			})
			return
		}
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

const captchaMaxEntries = 5000

func (s *Server) handleCaptcha(w http.ResponseWriter, r *http.Request) {
	id := "cap_" + randomHex(12)
	answer := fmt.Sprintf("%04d", randomIntRange(0, 9999))
	s.captchaMu.Lock()
	now := time.Now()
	for key, item := range s.captchas {
		if now.After(item.ExpiresAt) {
			delete(s.captchas, key)
		}
	}
	// SEC-05：限制 captcha map 大小，防止 OOM DoS
	if len(s.captchas) >= captchaMaxEntries {
		s.captchaMu.Unlock()
		writeError(w, apiErrWithReqID(NewError(CodeRateLimited, "captcha", "too many pending captchas; retry later"), requestIDFromContext(r.Context())))
		return
	}
	s.captchas[id] = captchaChallenge{
		Answer:    answer,
		ExpiresAt: now.Add(5 * time.Minute),
	}
	s.captchaMu.Unlock()
	writeJSON(w, http.StatusOK, CaptchaResponse{
		Success: true,
		ID:      id,
		Prompt:  "输入图片中的 4 位数字",
		Image:   captchaSVGDataURL(answer),
	})
}

// startCaptchaCleanup 启动后台清理协程（5 分钟一次）。
func (s *Server) startCaptchaCleanup() {
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.captchaMu.Lock()
			now := time.Now()
			for k, item := range s.captchas {
				if now.After(item.ExpiresAt) {
					delete(s.captchas, k)
				}
			}
			s.captchaMu.Unlock()
		}
	}()
}

func captchaSVGDataURL(answer string) string {
	// Lightweight SVG captcha: enough to stop casual automated form posts
	// without adding image dependencies to the server binary.
	escaped := html.EscapeString(answer)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="132" height="46" viewBox="0 0 132 46">
<rect width="132" height="46" rx="12" fill="#ecfeff"/>
<path d="M4 34 C32 12, 52 52, 82 18 S112 28, 128 12" fill="none" stroke="#38bdf8" stroke-width="3" opacity=".55"/>
<path d="M8 15 H124 M12 28 H120" stroke="#0f172a" stroke-width="1" opacity=".08"/>
<text x="66" y="31" text-anchor="middle" font-family="ui-monospace,Consolas,monospace" font-size="25" font-weight="900" letter-spacing="7" fill="#0f172a" transform="rotate(-3 66 23)">%s</text>
<circle cx="24" cy="12" r="2" fill="#f472b6"/><circle cx="106" cy="35" r="2.5" fill="#22c55e"/><circle cx="118" cy="14" r="1.8" fill="#f59e0b"/>
</svg>`, escaped)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
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

func (s *Server) handleEmailVerificationCode(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if !s.cfg.AllowRegistration {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "register", "user registration is disabled"), reqID))
		return
	}
	if !s.cfg.EmailVerificationEnabled {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "email", "email verification is not enabled"), reqID))
		return
	}
	if !s.emailConfig().SMTPConfigured {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "email", "SMTP is not configured"), reqID))
		return
	}
	var req EmailVerificationRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if !s.verifyCaptcha(req.CaptchaID, req.Captcha) {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "captcha", "captcha is incorrect or expired"), reqID))
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "email", "please enter a valid email address"), reqID))
		return
	}
	code := fmt.Sprintf("%06d", randomIntRange(0, 999999))
	expiresAt := time.Now().Add(10 * time.Minute)
	if err := s.sendEmailVerificationCode(email, code); err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "email", "failed to send verification email: "+err.Error()), reqID))
		return
	}
	s.emailMu.Lock()
	s.pruneExpiredEmailCodesLocked(time.Now())
	s.emailCodes[email] = emailVerificationChallenge{Code: code, ExpiresAt: expiresAt}
	s.emailMu.Unlock()
	writeJSON(w, http.StatusOK, EmailVerificationResponse{Success: true, Email: email, ExpiresIn: int(time.Until(expiresAt).Seconds())})
}

func normalizeEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || !emailRe.MatchString(email) || len(email) > 254 {
		return ""
	}
	return email
}

func (s *Server) verifyEmailCode(email, code string) bool {
	email = normalizeEmail(email)
	code = strings.TrimSpace(code)
	if email == "" || code == "" {
		return false
	}
	s.emailMu.Lock()
	defer s.emailMu.Unlock()
	s.pruneExpiredEmailCodesLocked(time.Now())
	item, ok := s.emailCodes[email]
	if !ok {
		return false
	}
	if time.Now().After(item.ExpiresAt) || !subtleConstantTimeString(item.Code, code) {
		return false
	}
	delete(s.emailCodes, email)
	return true
}

func (s *Server) pruneExpiredEmailCodesLocked(now time.Time) {
	for email, item := range s.emailCodes {
		if now.After(item.ExpiresAt) {
			delete(s.emailCodes, email)
		}
	}
}

func (s *Server) sendEmailVerificationCode(email, code string) error {
	host := strings.TrimSpace(s.cfg.SMTPHost)
	from := strings.TrimSpace(s.cfg.SMTPFrom)
	if host == "" || from == "" {
		return fmt.Errorf("SMTP host and from are required")
	}
	port := strings.TrimSpace(s.cfg.SMTPPort)
	if port == "" {
		port = "587"
	}
	addr := net.JoinHostPort(host, port)
	message := strings.Join([]string{
		"From: " + from,
		"To: " + email,
		"Subject: PagePilot verification code",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"你的 PagePilot 注册验证码是：" + code,
		"",
		"验证码 10 分钟内有效。如果不是你本人操作，可以忽略这封邮件。",
	}, "\r\n")
	var auth smtp.Auth
	if s.cfg.SMTPUsername != "" || s.cfg.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, host)
	}
	switch strings.ToLower(strings.TrimSpace(s.cfg.SMTPSecure)) {
	case "ssl", "tls", "true", "465":
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer client.Close()
		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
		if err := client.Mail(from); err != nil {
			return err
		}
		if err := client.Rcpt(email); err != nil {
			return err
		}
		wc, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := wc.Write([]byte(message)); err != nil {
			_ = wc.Close()
			return err
		}
		if err := wc.Close(); err != nil {
			return err
		}
		return client.Quit()
	default:
		return smtp.SendMail(addr, auth, from, []string{email}, []byte(message))
	}
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if !s.cfg.AllowRegistration {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "register", "user registration is disabled"), reqID))
		return
	}
	var req RegisterRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	registerDetail := func(email string, emailVerified bool) map[string]any {
		return map[string]any{
			"username":      strings.TrimSpace(req.Username),
			"email":         email,
			"emailVerified": emailVerified,
		}
	}
	writeRegisterError := func(apiErr *APIError, email string, emailVerified bool) {
		s.recordFailedAuditLog(r, "unknown", "", "public", "auth.register", "", "user", strings.TrimSpace(req.Username),
			registerDetail(email, emailVerified), apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	if !s.verifyCaptcha(req.CaptchaID, req.Captcha) {
		writeRegisterError(NewError(CodeInvalidInput, "captcha", "captcha is incorrect or expired"), "", false)
		return
	}
	email := normalizeEmail(req.Email)
	emailVerified := false
	if s.cfg.EmailVerificationEnabled {
		if email == "" {
			writeRegisterError(NewError(CodeInvalidInput, "email", "email is required when email verification is enabled"), email, false)
			return
		}
		if !s.verifyEmailCode(email, req.EmailCode) {
			writeRegisterError(NewError(CodeInvalidInput, "email", "email verification code is incorrect or expired"), email, false)
			return
		}
		emailVerified = true
	}
	if err := validatePasswordStrength(req.Password); err != nil {
		writeRegisterError(err, email, emailVerified)
		return
	}
	user, err := s.auth.CreateUserWithEmail(r.Context(), req.Username, email, emailVerified, req.Password, false, 20)
	if err != nil {
		writeRegisterError(NewError(CodeInvalidInput, "register", "username already exists or password is invalid"), email, emailVerified)
		return
	}
	anonymousClaimed := s.claimCurrentAnonymousSession(r, user.ID, user.IsAdmin, "register")
	if anonymousClaimed {
		clearAnonymousSessionCookie(w, s.cookieSecureForRequest(r))
	}
	actorType, actorID, actorRole := auditActorFromOwner("user:"+user.ID, user.IsAdmin)
	s.recordAuditLog(r, actorType, actorID, actorRole, "auth.register", "", "user", user.ID, map[string]any{
		"userId":           user.ID,
		"username":         user.Username,
		"email":            user.Email,
		"emailVerified":    user.EmailVerified,
		"anonymousClaimed": anonymousClaimed,
	})
	writeJSON(w, http.StatusOK, RegisterResponse{Success: true, UserID: user.ID, Username: user.Username, Email: user.Email, EmailVerified: user.EmailVerified})
}

func (s *Server) handleAccountPassword(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	user, ok := s.adminUserFromCookie(r)
	if !ok {
		writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "auth", "login required"), reqID))
		return
	}
	actorType, actorID, actorRole := auditActorFromOwner("user:"+user.ID, user.IsAdmin)
	auditDetail := map[string]any{
		"userId":   user.ID,
		"username": user.Username,
		"self":     true,
		"isAdmin":  user.IsAdmin,
	}
	var req AccountPasswordRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if apiErr := validatePasswordStrength(req.NewPassword); apiErr != nil {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "account.password", "", "user", user.ID, auditDetail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	if err := s.auth.ChangeUserPassword(r.Context(), user.ID, req.OldPassword, req.NewPassword); err != nil {
		apiErr := NewError(CodeUnauthorized, "password", "old password is incorrect")
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "account.password", "", "user", user.ID, auditDetail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "account.password", "", "user", user.ID, auditDetail)
	writeJSON(w, http.StatusOK, AccountPasswordResponse{Success: true})
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	var req AdminLoginRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	loginDetail := func() map[string]any {
		return map[string]any{
			"username": strings.TrimSpace(req.Username),
		}
	}
	writeLoginError := func(apiErr *APIError) {
		s.recordFailedAuditLog(r, "unknown", "", "public", "auth.login", "", "user", strings.TrimSpace(req.Username), loginDetail(), apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	clientIP := clientIPFromRequest(r)
	loginKey := s.loginKey(req.Username, clientIP)
	if remain := s.loginCheckLocked(loginKey); remain > 0 {
		apiErr := NewError(CodeRateLimited, "rate_limit", fmt.Sprintf("too many failed attempts; retry in %d seconds", int(remain.Seconds()))).
			WithRetryAfter(int(remain.Seconds()))
		writeLoginError(apiErr)
		return
	}
	if !s.verifyCaptcha(req.CaptchaID, req.Captcha) {
		s.loginRecordFail(loginKey)
		writeLoginError(NewError(CodeInvalidInput, "captcha", "captcha is incorrect or expired"))
		return
	}
	res, err := s.auth.LoginAdmin(r.Context(), req.Username, req.Password, 7*24*time.Hour)
	if err != nil {
		s.loginRecordFail(loginKey)
		writeLoginError(NewError(CodeUnauthorized, "auth", "username or password is incorrect"))
		return
	}
	s.loginReset(loginKey)
	anonymousClaimed := s.claimCurrentAnonymousSession(r, res.User.ID, res.User.IsAdmin, "login")
	if anonymousClaimed {
		clearAnonymousSessionCookie(w, s.cookieSecureForRequest(r))
	}
	actorType, actorID, actorRole := auditActorFromOwner("user:"+res.User.ID, res.User.IsAdmin)
	s.recordAuditLog(r, actorType, actorID, actorRole, "auth.login", "", "user", res.User.ID, map[string]any{
		"userId":           res.User.ID,
		"username":         res.User.Username,
		"isAdmin":          res.User.IsAdmin,
		"anonymousClaimed": anonymousClaimed,
	})
	setAdminSessionCookie(w, res.Plaintext, int((7 * 24 * time.Hour).Seconds()), s.cookieSecureForRequest(r))
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

func (s *Server) claimCurrentAnonymousSession(r *http.Request, userID string, isAdmin bool, source string) bool {
	sessionID := anonymousSessionIDFromRequest(r)
	if sessionID == "" {
		return false
	}
	actorType, actorID, actorRole := auditActorFromOwner("user:"+userID, isAdmin)
	detail := map[string]any{
		"userId":    userID,
		"sessionId": sessionID,
		"source":    source,
		"auto":      true,
	}
	result, err := s.deployer.ClaimAnonymousSession(r.Context(), sessionID, userID)
	if err != nil {
		s.logger.Printf("failed to claim anonymous session %s for user %s: %v", sessionID, userID, err)
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "anonymous.claim", "", "anonymous_session", sessionID, detail,
			NewError(CodeInvalidInput, "anonymous_session", err.Error()))
		return false
	}
	detail["siteCount"] = result.SiteCount
	detail["deployCount"] = result.DeployCount
	detail["alreadyClaimed"] = result.AlreadyClaimed
	s.recordAuditLog(r, actorType, actorID, actorRole, "anonymous.claim", "", "anonymous_session", result.SessionID, detail)
	return true
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("hostctl_admin_session"); err == nil {
		if user, _, verifyErr := s.auth.VerifyAdminSession(r.Context(), c.Value); verifyErr == nil {
			actorType, actorID, actorRole := auditActorFromOwner("user:"+user.ID, user.IsAdmin)
			detail := map[string]any{
				"userId":   user.ID,
				"username": user.Username,
				"isAdmin":  user.IsAdmin,
			}
			if revokeErr := s.auth.RevokeAdminSession(r.Context(), c.Value); revokeErr != nil {
				s.recordFailedAuditLog(r, actorType, actorID, actorRole, "auth.logout", "", "user", user.ID, detail,
					NewError(CodeInternal, "auth", revokeErr.Error()))
			} else {
				s.recordAuditLog(r, actorType, actorID, actorRole, "auth.logout", "", "user", user.ID, detail)
			}
		} else {
			_ = s.auth.RevokeAdminSession(r.Context(), c.Value)
		}
	}
	setAdminSessionCookie(w, "", -1, s.cookieSecureForRequest(r))
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
		ContentInjection:  s.contentInjectionForConfigResponse(r),
		CooldownSeconds:   s.cfg.CooldownSeconds,
		Limits: Limits{
			MaxSingleFileBytes: s.cfg.MaxSingleFileBytes,
			MaxSiteTotalBytes:  s.cfg.MaxSiteTotalBytes,
			MaxFilesPerSite:    s.cfg.MaxFilesPerSite,
		},
		AnonymousPolicy: AnonymousPolicy{
			DeployLimit: s.cfg.AnonymousDeployLimit,
		},
		RegistrationAllowed: s.cfg.AllowRegistration,
		Email:               s.emailConfig(),
		Storage:             s.storageConfig(),
		Version:             s.version,
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

func (s *Server) handleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	if _, authErr := s.authenticateAdmin(r); authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	reader, ok := s.deployer.(auditLogReader)
	if !ok {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "audit_logs", "audit log store is unavailable"), reqID))
		return
	}
	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}
	pageSize := parseIntDefault(r.URL.Query().Get("pageSize"), 50)
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	filter := store.AuditLogFilter{
		ActorType:  strings.TrimSpace(r.URL.Query().Get("actorType")),
		ActorID:    strings.TrimSpace(r.URL.Query().Get("actorId")),
		ActorRole:  strings.TrimSpace(r.URL.Query().Get("actorRole")),
		Action:     strings.TrimSpace(r.URL.Query().Get("action")),
		Result:     strings.TrimSpace(r.URL.Query().Get("result")),
		SiteCode:   strings.TrimSpace(r.URL.Query().Get("siteCode")),
		TargetType: strings.TrimSpace(r.URL.Query().Get("targetType")),
		TargetID:   strings.TrimSpace(r.URL.Query().Get("targetId")),
		Query:      strings.TrimSpace(r.URL.Query().Get("q")),
		Limit:      pageSize,
		Offset:     (page - 1) * pageSize,
	}
	if rawSince := strings.TrimSpace(r.URL.Query().Get("since")); rawSince != "" {
		since, err := time.Parse(time.RFC3339, rawSince)
		if err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "audit_logs", "since must be RFC3339"), reqID))
			return
		}
		filter.Since = &since
	}
	if rawUntil := strings.TrimSpace(r.URL.Query().Get("until")); rawUntil != "" {
		until, err := time.Parse(time.RFC3339, rawUntil)
		if err != nil {
			writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "audit_logs", "until must be RFC3339"), reqID))
			return
		}
		filter.Until = &until
	}
	logs, total, err := reader.ListAuditLogs(r.Context(), filter)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "audit_logs", err.Error()), reqID))
		return
	}
	items := make([]AuditLogListItem, 0, len(logs))
	for _, log := range logs {
		detail := json.RawMessage(strings.TrimSpace(log.DetailJSON))
		if len(detail) == 0 || !json.Valid(detail) {
			detail = json.RawMessage(`{}`)
		}
		items = append(items, AuditLogListItem{
			ID:         log.ID,
			ActorType:  log.ActorType,
			ActorID:    log.ActorID,
			ActorRole:  log.ActorRole,
			Action:     log.Action,
			Result:     log.Result,
			SiteCode:   log.SiteCode,
			TargetType: log.TargetType,
			TargetID:   log.TargetID,
			IP:         log.IP,
			UserAgent:  log.UserAgent,
			Detail:     detail,
			CreatedAt:  log.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, AuditLogListResponse{
		Success:  true,
		Logs:     items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// handlePutConfig 更新可热修改的运行时配置。
func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())

	// 鉴权：运行配置属于后台管理能力，dev 模式也必须登录管理员。
	tok, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}

	var req ConfigUpdateRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	actorType, actorID, actorRole := auditActorFromToken(tok)
	configUpdateAuditDetail := func() map[string]any {
		detail := map[string]any{
			"appURL":               req.AppURLMode != nil || req.AppDomainSuffix != nil || req.AppURLScheme != nil || req.AppURLPort != nil,
			"anonymousDeployLimit": req.AnonymousDeployLimit != nil,
			"cooldownSeconds":      req.CooldownSeconds != nil,
			"uploadLimits":         req.MaxSingleFileBytes != nil || req.MaxSiteTotalBytes != nil || req.MaxFilesPerSite != nil,
			"corsAllowOrigins":     req.CORSAllowOrigins != nil,
			"embedPolicy":          req.EmbedPolicy != nil || req.EmbedAllowOrigins != nil,
			"contentInjection":     req.ContentInjection != nil,
		}
		if req.ContentInjection != nil {
			detail["contentInjectionSummary"] = contentInjectionAuditSummary(config.NormalizeContentInjection(*req.ContentInjection))
		}
		return detail
	}
	writeConfigError := func(apiErr *APIError) {
		detail := mergeAPIErrorAuditDetail(configUpdateAuditDetail(), apiErr)
		s.recordAuditLogWithResult(r, actorType, actorID, actorRole, "config.update", "", "config", "runtime", "failed", detail)
		writeError(w, apiErrWithReqID(apiErr, reqID))
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
				writeConfigError(NewError(CodeInvalidInput, "validate",
					"appURLPort must be a number between 1 and 65535"))
				return
			}
		}
		if (next.AppURLMode == AppURLModeDomain || next.AppURLMode == AppURLModeDual) && next.AppDomainSuffix == "" {
			writeConfigError(NewError(CodeInvalidInput, "validate",
				"appDomainSuffix is required when appURLMode is domain or dual"))
			return
		}
		setter, ok := s.deployer.(appURLConfigSetter)
		if !ok {
			writeConfigError(NewError(CodeInternal, "set_config",
				"deployer does not support app URL config"))
			return
		}
		if err := setter.SetAppURLConfig(r.Context(), next); err != nil {
			writeConfigError(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist app URL config: %v", err)))
			return
		}
		s.cfg.AppURLMode = next.AppURLMode
		s.cfg.AppDomainSuffix = next.AppDomainSuffix
		s.cfg.AppURLScheme = next.AppURLScheme
		s.cfg.AppURLPort = next.AppURLPort
	}
	if req.AnonymousDeployLimit != nil {
		if *req.AnonymousDeployLimit < -1 {
			writeConfigError(NewError(CodeInvalidInput, "validate",
				"anonymousDeployLimit must be -1 or greater"))
			return
		}
		if err := s.deployer.SetAnonymousDeployLimit(r.Context(), *req.AnonymousDeployLimit); err != nil {
			writeConfigError(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist anonymous limit: %v", err)))
			return
		}
		s.cfg.AnonymousDeployLimit = *req.AnonymousDeployLimit
	}
	if req.CooldownSeconds != nil {
		if *req.CooldownSeconds < 0 || *req.CooldownSeconds > 3600 {
			writeConfigError(NewError(CodeInvalidInput, "validate",
				"cooldownSeconds must be between 0 and 3600"))
			return
		}
		if err := s.deployer.SetCooldownSeconds(r.Context(), *req.CooldownSeconds); err != nil {
			writeConfigError(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist cooldown: %v", err)))
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
			writeConfigError(NewError(CodeInvalidInput, "validate",
				"upload limits must be positive"))
			return
		}
		if single > total {
			writeConfigError(NewError(CodeInvalidInput, "validate",
				"maxSingleFileBytes cannot exceed maxSiteTotalBytes"))
			return
		}
		if err := s.deployer.SetUploadLimits(r.Context(), single, total, files); err != nil {
			writeConfigError(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist upload limits: %v", err)))
			return
		}
		s.cfg.MaxSingleFileBytes = single
		s.cfg.MaxSiteTotalBytes = total
		s.cfg.MaxFilesPerSite = files
	}
	if req.CORSAllowOrigins != nil {
		origins := strings.TrimSpace(*req.CORSAllowOrigins)
		if origins == "*" {
			writeConfigError(NewError(CodeInvalidInput, "validate",
				"corsAllowOrigins no longer supports wildcard *; leave it blank to disable CORS"))
			return
		}
		origins = config.NormalizeCORSAllowOrigins(origins)
		if origins != "" && !corsOriginsLookValid(origins) {
			writeConfigError(NewError(CodeInvalidInput, "validate",
				"corsAllowOrigins must be empty or a comma/newline separated list of http(s) origins"))
			return
		}
		if err := s.deployer.SetCORSAllowOrigins(r.Context(), origins); err != nil {
			writeConfigError(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist CORS origins: %v", err)))
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
			writeConfigError(NewError(CodeInvalidInput, "validate",
				"embedAllowOrigins is required when embedPolicy is allowlist"))
			return
		}
		if origins != "" && !corsOriginsLookValid(origins) {
			writeConfigError(NewError(CodeInvalidInput, "validate",
				"embedAllowOrigins must be empty or a comma/newline separated list of http(s) origins"))
			return
		}
		if err := s.deployer.SetEmbedPolicy(r.Context(), policy, origins); err != nil {
			writeConfigError(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist embed policy: %v", err)))
			return
		}
		s.cfg.EmbedPolicy = policy
		s.cfg.EmbedAllowOrigins = origins
	}
	if req.ContentInjection != nil {
		next := config.NormalizeContentInjection(*req.ContentInjection)
		if apiErr := validateContentInjectionConfig(next); apiErr != nil {
			writeConfigError(apiErr)
			return
		}
		if err := s.deployer.SetContentInjection(r.Context(), next); err != nil {
			writeConfigError(NewError(CodeInternal, "set_config",
				fmt.Sprintf("failed to persist content injection: %v", err)))
			return
		}
		s.cfg.ContentInjection = next
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "config.update", "", "config", "runtime", configUpdateAuditDetail())
	currentBase := s.requestBaseURL(r)
	writeJSON(w, http.StatusOK, ConfigUpdateResponse{
		Success:           true,
		CurrentBaseURL:    currentBase,
		AppURL:            s.appURLConfigForRequest(r),
		CORSAllowOrigins:  s.cfg.CORSAllowOrigins,
		EmbedPolicy:       config.NormalizeEmbedPolicy(s.cfg.EmbedPolicy),
		EmbedAllowOrigins: s.cfg.EmbedAllowOrigins,
		ContentInjection:  config.NormalizeContentInjection(s.cfg.ContentInjection),
		CooldownSeconds:   s.cfg.CooldownSeconds,
		Limits: Limits{
			MaxSingleFileBytes: s.cfg.MaxSingleFileBytes,
			MaxSiteTotalBytes:  s.cfg.MaxSiteTotalBytes,
			MaxFilesPerSite:    s.cfg.MaxFilesPerSite,
		},
		AnonymousPolicy: AnonymousPolicy{
			DeployLimit: s.cfg.AnonymousDeployLimit,
		},
		RegistrationAllowed: s.cfg.AllowRegistration,
		Email:               s.emailConfig(),
		Storage:             s.storageConfig(),
	})
}

const (
	maxContentInjectionSnippetBytes = 128 << 10
	maxContentInjectionTotalBytes   = 512 << 10
)

func validateContentInjectionConfig(cfg config.ContentInjectionConfig) *APIError {
	total := 0
	for _, item := range []struct {
		name string
		code string
	}{
		{"main.headCode", cfg.Main.HeadCode},
		{"main.bodyStartCode", cfg.Main.BodyStartCode},
		{"main.bodyEndCode", cfg.Main.BodyEndCode},
		{"app.headCode", cfg.App.HeadCode},
		{"app.bodyStartCode", cfg.App.BodyStartCode},
		{"app.bodyEndCode", cfg.App.BodyEndCode},
	} {
		size := len([]byte(item.code))
		if size > maxContentInjectionSnippetBytes {
			return NewError(CodeInvalidInput, "validate",
				item.name+" must be at most 128 KB")
		}
		total += size
	}
	if total > maxContentInjectionTotalBytes {
		return NewError(CodeInvalidInput, "validate",
			"contentInjection total size must be at most 512 KB")
	}
	return nil
}

func contentInjectionAuditSummary(cfg config.ContentInjectionConfig) map[string]any {
	targetSummary := func(target config.InjectionTargetConfig) map[string]any {
		return map[string]any{
			"enabled":        target.Enabled,
			"headBytes":      len([]byte(target.HeadCode)),
			"bodyStartBytes": len([]byte(target.BodyStartCode)),
			"bodyEndBytes":   len([]byte(target.BodyEndCode)),
		}
	}
	return map[string]any{
		"main": targetSummary(cfg.Main),
		"app":  targetSummary(cfg.App),
	}
}

func (s *Server) contentInjectionForConfigResponse(r *http.Request) config.ContentInjectionConfig {
	cfg := config.NormalizeContentInjection(s.cfg.ContentInjection)
	if _, authErr := s.authenticateAdmin(r); authErr == nil {
		return cfg
	}
	return config.ContentInjectionConfig{
		Main: config.InjectionTargetConfig{Enabled: injectionTargetHasContent(cfg.Main)},
		App:  config.InjectionTargetConfig{Enabled: injectionTargetHasContent(cfg.App)},
	}
}

func (s *Server) emailConfig() EmailConfig {
	return EmailConfig{
		VerificationEnabled: s.cfg.EmailVerificationEnabled,
		SMTPConfigured:      s.cfg.SMTPHost != "" && s.cfg.SMTPFrom != "",
		SMTPHost:            s.cfg.SMTPHost,
		SMTPFrom:            s.cfg.SMTPFrom,
		SMTPSecure:          s.cfg.SMTPSecure,
	}
}

func (s *Server) storageConfig() StorageConfig {
	return StorageConfig{
		Backend:          s.cfg.StorageBackend,
		HostedDir:        s.cfg.HostedDir,
		OSSProvider:      s.cfg.OSSProvider,
		OSSEndpoint:      s.cfg.OSSEndpoint,
		OSSBucket:        s.cfg.OSSBucket,
		OSSPrefix:        s.cfg.OSSPrefix,
		OSSPublicBaseURL: s.cfg.OSSPublicBaseURL,
		OSSConfigured:    s.cfg.OSSEndpoint != "" && s.cfg.OSSBucket != "" && s.cfg.OSSAccessKeyID != "" && s.cfg.OSSAccessKeySecret != "",
	}
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
	tok, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req UserCreateRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	actorType, actorID, actorRole := auditActorFromToken(tok)
	userCreateDetail := func() map[string]any {
		return map[string]any{
			"username":      strings.TrimSpace(req.Username),
			"email":         strings.TrimSpace(req.Email),
			"emailVerified": req.EmailVerified != nil,
			"isAdmin":       req.IsAdmin,
			"deployLimit":   req.DeployLimit,
		}
	}
	writeUserCreateError := func(apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "user.create", "", "user", "", userCreateDetail(), apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	email := normalizeEmail(req.Email)
	if strings.TrimSpace(req.Email) != "" && email == "" {
		writeUserCreateError(NewError(CodeInvalidInput, "users", "invalid email address"))
		return
	}
	emailVerified := email != ""
	if req.EmailVerified != nil {
		emailVerified = *req.EmailVerified
	}
	if email == "" {
		emailVerified = false
	}
	if apiErr := validatePasswordStrength(req.Password); apiErr != nil {
		writeUserCreateError(apiErr)
		return
	}
	user, err := s.auth.CreateUserWithEmail(r.Context(), req.Username, email, emailVerified, req.Password, req.IsAdmin, req.DeployLimit)
	if err != nil {
		writeUserCreateError(NewError(CodeInvalidInput, "users", err.Error()))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "user.create", "", "user", user.ID, map[string]any{
		"username":    user.Username,
		"isAdmin":     user.IsAdmin,
		"isActive":    user.IsActive,
		"deployLimit": user.DeployLimit,
	})
	writeJSON(w, http.StatusOK, UserCreateResponse{Success: true, User: toUserListItem(user)})
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	tok, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	id := r.PathValue("id")
	actorType, actorID, actorRole := auditActorFromToken(tok)
	userUpdateDetail := func(req *UserUpdateRequest) map[string]any {
		detail := map[string]any{}
		if req == nil {
			return detail
		}
		detail["username"] = req.Username != nil
		detail["email"] = req.Email != nil
		detail["emailVerified"] = req.EmailVerified != nil
		detail["isAdmin"] = req.IsAdmin != nil
		detail["isActive"] = req.IsActive != nil
		detail["deployLimit"] = req.DeployLimit != nil
		return detail
	}
	writeUserUpdateError := func(apiErr *APIError, req *UserUpdateRequest) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "user.update", "", "user", id, userUpdateDetail(req), apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	user, err := s.auth.GetUser(r.Context(), id)
	if err != nil {
		writeUserUpdateError(NewError(CodeNotFound, "users", "user not found"), nil)
		return
	}
	var req UserUpdateRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	if req.Username != nil {
		user.Username = strings.TrimSpace(*req.Username)
	}
	if req.Email != nil {
		email := normalizeEmail(*req.Email)
		if strings.TrimSpace(*req.Email) != "" && email == "" {
			writeUserUpdateError(NewError(CodeInvalidInput, "users", "invalid email address"), &req)
			return
		}
		user.Email = email
		if email == "" {
			user.EmailVerified = false
		}
	}
	if req.EmailVerified != nil {
		user.EmailVerified = *req.EmailVerified && user.Email != ""
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
		writeUserUpdateError(NewError(CodeInvalidInput, "users", err.Error()), &req)
		return
	}
	user, _ = s.auth.GetUser(r.Context(), id)
	s.recordAuditLog(r, actorType, actorID, actorRole, "user.update", "", "user", user.ID, map[string]any{
		"username":      req.Username != nil,
		"email":         req.Email != nil,
		"emailVerified": req.EmailVerified != nil,
		"isAdmin":       req.IsAdmin != nil,
		"isActive":      req.IsActive != nil,
		"deployLimit":   req.DeployLimit != nil,
	})
	writeJSON(w, http.StatusOK, UserUpdateResponse{Success: true, User: toUserListItem(user)})
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	tok, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	actorType, actorID, actorRole := auditActorFromToken(tok)
	userDeleteDetail := func(user store.AdminUser) map[string]any {
		return map[string]any{
			"username": user.Username,
			"isAdmin":  user.IsAdmin,
		}
	}
	writeUserDeleteError := func(apiErr *APIError, user store.AdminUser) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "user.delete", "", "user", id, userDeleteDetail(user), apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	if id == "" {
		writeUserDeleteError(NewError(CodeInvalidInput, "users", "missing user id"), store.AdminUser{})
		return
	}
	if id == tok.OwnerUserID || id == strings.TrimPrefix(tok.ID, "admin:") {
		writeUserDeleteError(NewError(CodeInvalidInput, "users", "cannot delete your own account"), store.AdminUser{})
		return
	}
	user, err := s.auth.GetUser(r.Context(), id)
	if err != nil {
		writeUserDeleteError(NewError(CodeNotFound, "users", "user not found"), store.AdminUser{})
		return
	}
	if user.IsAdmin {
		users, err := s.auth.ListUsers(r.Context())
		if err != nil {
			writeUserDeleteError(NewError(CodeInternal, "users", err.Error()), user)
			return
		}
		activeAdmins := 0
		for _, u := range users {
			if u.IsAdmin && u.IsActive {
				activeAdmins++
			}
		}
		if activeAdmins <= 1 {
			writeUserDeleteError(NewError(CodeInvalidInput, "users", "cannot delete the last active admin"), user)
			return
		}
	}
	if err := s.auth.DeleteUser(r.Context(), id); err != nil {
		writeUserDeleteError(NewError(CodeInvalidInput, "users", err.Error()), user)
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "user.delete", "", "user", id, map[string]any{
		"username": user.Username,
		"isAdmin":  user.IsAdmin,
	})
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
		ID:            user.ID,
		Username:      user.Username,
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		IsAdmin:       user.IsAdmin,
		IsActive:      user.IsActive,
		DeployLimit:   user.DeployLimit,
		DeployCount:   user.DeployCount,
		Remaining:     remaining,
		CreatedAt:     user.CreatedAt,
		LastLoginAt:   user.LastLoginAt,
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
			Code:                 site.Code,
			PublicID:             site.PublicID,
			OwnerTokenID:         site.OwnerTokenID,
			OwnerUsername:        ownerUsername,
			CurrentVersion:       site.CurrentVersion,
			VersionCount:         site.VersionCount,
			TotalSize:            site.TotalSize,
			ViewCount:            site.ViewCount,
			LikeCount:            site.LikeCount,
			Status:               site.Status,
			Visibility:           site.Visibility,
			ReusePolicy:          normalizeReusePolicy(site.ReusePolicy),
			SourceDownloadPolicy: normalizeReusePolicy(site.SourceDownloadPolicy),
			SecurityMode:         normalizeSiteSecurityMode(site.SecurityMode),
			Category:             site.Category,
			Tags:                 splitSiteTags(site.Tags),
			Title:                site.Title,
			Description:          site.Description,
			Filename:             site.MainEntry,
			AccessProtected:      site.AccessProtected,
			IsPinned:             site.IsPinned,
			PinnedAt:             site.PinnedAt,
			CreatedAt:            site.CreatedAt,
			Source:               site.Source,
			LastVersionAt:        site.LastVersionAt,
		})
	}
	writeJSON(w, http.StatusOK, SiteListResponse{Success: true, Sites: items})
}

func (s *Server) handleAdminGetSiteDetail(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path", "missing code in path"), reqID))
		return
	}
	actor, isAdmin, authErr := s.optionalActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	if actor == "" {
		writeError(w, apiErrWithReqID(NewError(CodeUnauthorized, "auth", "login or bearer token required"), reqID))
		return
	}
	site, err := s.deployer.GetSite(r.Context(), code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "site", "site not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "site", err.Error()), reqID))
		return
	}
	if !isAdmin && site.OwnerTokenID != actor {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "owner", "you can only view sites you own"), reqID))
		return
	}

	versionsResp, apiErr := s.deployer.ListVersions(r.Context(), code)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	bundle, files, reuse, apiErr := s.deployDetailExtras(r, code, code, site.CurrentVersion, detailReusePolicy{
		CanManage:            true,
		AccessProtected:      strings.TrimSpace(site.AccessPasswordHash) != "",
		Visibility:           site.Visibility,
		Status:               site.Status,
		ReusePolicy:          site.ReusePolicy,
		SourceDownloadPolicy: site.SourceDownloadPolicy,
	})
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	item := siteListItemFromSite(site, s.siteOwnerUsername(r.Context(), site.OwnerTokenID))
	item.VersionCount = len(versionsResp.Versions)
	if bundle != nil {
		item.Filename = bundle.MainEntry
		item.TotalSize = bundle.TotalSize
	}
	if len(versionsResp.Versions) > 0 {
		last := versionsResp.Versions[len(versionsResp.Versions)-1].CreatedAt
		item.LastVersionAt = &last
	}
	writeJSON(w, http.StatusOK, SiteDetailResponse{
		Success:  true,
		Site:     item,
		Versions: versionsResp.Versions,
		Bundle:   bundle,
		Files:    files,
		Reuse:    reuse,
	})
}

func siteListItemFromSite(site store.Site, ownerUsername string) SiteListItem {
	return SiteListItem{
		Code:                  site.Code,
		PublicID:              site.PublicID,
		OwnerTokenID:          site.OwnerTokenID,
		OwnerUsername:         ownerUsername,
		CurrentVersion:        site.CurrentVersion,
		ViewCount:             site.ViewCount,
		LikeCount:             site.LikeCount,
		ReuseCount:            site.ReuseCount,
		Status:                site.Status,
		Visibility:            site.Visibility,
		ReusePolicy:           normalizeReusePolicy(site.ReusePolicy),
		SourceDownloadPolicy:  normalizeReusePolicy(site.SourceDownloadPolicy),
		SecurityMode:          normalizeSiteSecurityMode(site.SecurityMode),
		TemplateSourceCode:    site.TemplateSourceCode,
		TemplateSourceVersion: site.TemplateSourceVersion,
		Category:              site.Category,
		Tags:                  splitSiteTags(site.Tags),
		AccessProtected:       strings.TrimSpace(site.AccessPasswordHash) != "",
		IsPinned:              site.IsPinned,
		PinnedAt:              site.PinnedAt,
		CreatedAt:             site.CreatedAt,
		Source:                site.Source,
	}
}

func (s *Server) siteOwnerUsername(ctx context.Context, ownerTokenID string) string {
	if !strings.HasPrefix(ownerTokenID, "user:") {
		return ""
	}
	return s.ownerUsername(ctx, strings.TrimPrefix(ownerTokenID, "user:"))
}

// handleAdminSetSitePin 允许管理员置顶或取消置顶创作市场站点。
func (s *Server) handleAdminSetSitePin(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			"missing code in path"), reqID))
		return
	}
	tok, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req SitePinRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	actorType, actorID, actorRole := auditActorFromToken(tok)
	pinDetail := map[string]any{}
	writePinError := func(apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "site.pin", code, "site", code, pinDetail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	if req.Pinned == nil {
		writePinError(NewError(CodeInvalidInput, "pinned", "pinned is required"))
		return
	}
	pinDetail["pinned"] = *req.Pinned
	if err := s.deployer.SetSitePinned(r.Context(), code, *req.Pinned); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writePinError(NewError(CodeNotFound, "site", "site not found"))
			return
		}
		writePinError(NewError(CodeInternal, "site_pin", err.Error()))
		return
	}
	site, err := s.deployer.GetSite(r.Context(), code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writePinError(NewError(CodeNotFound, "site", "site not found"))
			return
		}
		writePinError(NewError(CodeInternal, "site_pin", err.Error()))
		return
	}
	resp := SitePinResponse{Success: true, Code: code, IsPinned: site.IsPinned}
	if site.PinnedAt != nil {
		t := site.PinnedAt.UTC().Format(time.RFC3339Nano)
		resp.PinnedAt = &t
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "site.pin", code, "site", code, map[string]any{
		"pinned": *req.Pinned,
	})
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminSetSiteReusePolicy(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path", "missing code in path"), reqID))
		return
	}
	tok, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req SiteReusePolicyRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	actorType, actorID, actorRole := auditActorFromToken(tok)
	reuseDetail := map[string]any{
		"reusePolicy":          strings.TrimSpace(req.ReusePolicy),
		"sourceDownloadPolicy": strings.TrimSpace(req.SourceDownloadPolicy),
	}
	writeReusePolicyError := func(apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "site.reuse_policy", code, "site", code, reuseDetail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	if !isReusePolicyInput(req.ReusePolicy) {
		writeReusePolicyError(NewError(CodeInvalidInput, "reusePolicy", "reusePolicy must be one of auto, allow, deny"))
		return
	}
	if !isReusePolicyInput(req.SourceDownloadPolicy) {
		writeReusePolicyError(NewError(CodeInvalidInput, "sourceDownloadPolicy", "sourceDownloadPolicy must be one of auto, allow, deny"))
		return
	}
	reusePolicyValue := normalizeReusePolicy(req.ReusePolicy)
	sourcePolicyValue := normalizeReusePolicy(req.SourceDownloadPolicy)
	reuseDetail["reusePolicy"] = reusePolicyValue
	reuseDetail["sourceDownloadPolicy"] = sourcePolicyValue
	if err := s.deployer.SetSiteReusePolicy(r.Context(), code, reusePolicyValue, sourcePolicyValue); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeReusePolicyError(NewError(CodeNotFound, "site", "site not found"))
			return
		}
		writeReusePolicyError(NewError(CodeInternal, "site_reuse_policy", err.Error()))
		return
	}
	site, err := s.deployer.GetSite(r.Context(), code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeReusePolicyError(NewError(CodeNotFound, "site", "site not found"))
			return
		}
		writeReusePolicyError(NewError(CodeInternal, "site_reuse_policy", err.Error()))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "site.reuse_policy", code, "site", code, map[string]any{
		"reusePolicy":          reusePolicyValue,
		"sourceDownloadPolicy": sourcePolicyValue,
	})
	writeJSON(w, http.StatusOK, SiteReusePolicyResponse{
		Success: true,
		Site:    siteListItemFromSite(site, s.siteOwnerUsername(r.Context(), site.OwnerTokenID)),
	})
}

func (s *Server) handleAdminSetSiteSecurityMode(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path", "missing code in path"), reqID))
		return
	}
	tok, authErr := s.authenticateAdmin(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req SiteSecurityModeRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	actorType, actorID, actorRole := auditActorFromToken(tok)
	securityDetail := map[string]any{"securityMode": strings.TrimSpace(req.SecurityMode)}
	writeSecurityModeError := func(apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "site.security_mode", code, "site", code, securityDetail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	if !isSiteSecurityModeInput(req.SecurityMode) {
		writeSecurityModeError(NewError(CodeInvalidInput, "securityMode", "securityMode must be one of auto, strict, compatible, trusted"))
		return
	}
	mode := normalizeSiteSecurityMode(req.SecurityMode)
	securityDetail["securityMode"] = mode
	if err := s.deployer.SetSiteSecurityMode(r.Context(), code, mode); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeSecurityModeError(NewError(CodeNotFound, "site", "site not found"))
			return
		}
		writeSecurityModeError(NewError(CodeInternal, "site_security_mode", err.Error()))
		return
	}
	site, err := s.deployer.GetSite(r.Context(), code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeSecurityModeError(NewError(CodeNotFound, "site", "site not found"))
			return
		}
		writeSecurityModeError(NewError(CodeInternal, "site_security_mode", err.Error()))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "site.security_mode", code, "site", code, map[string]any{
		"securityMode": mode,
	})
	writeJSON(w, http.StatusOK, SiteSecurityModeResponse{
		Success: true,
		Site:    siteListItemFromSite(site, s.siteOwnerUsername(r.Context(), site.OwnerTokenID)),
	})
}

func (s *Server) handleAdminSetSiteCategory(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path", "missing code in path"), reqID))
		return
	}
	actor, isAdmin, authErr := s.authenticateActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	actorType, actorID, actorRole := auditActorFromOwner(actor, isAdmin)
	writeCategoryError := func(detail map[string]any, apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "site.category", code, "site", code, detail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	site, err := s.deployer.GetSite(r.Context(), code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCategoryError(nil, NewError(CodeNotFound, "site", "site not found"))
			return
		}
		writeCategoryError(nil, NewError(CodeInternal, "site_category", err.Error()))
		return
	}
	if !isAdmin && site.OwnerTokenID != actor {
		writeCategoryError(nil, NewError(CodeForbidden, "site_category", "you can only update your own site category"))
		return
	}
	var req SiteCategoryRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	category := strings.TrimSpace(req.Category)
	categoryDetail := map[string]any{"category": category}
	if category != "" && !marketCategorySlugRe.MatchString(category) {
		writeCategoryError(categoryDetail, NewError(CodeInvalidInput, "site_category", "category slug is invalid"))
		return
	}
	if err := s.deployer.SetSiteCategory(r.Context(), code, category); err != nil {
		writeCategoryError(categoryDetail, NewError(CodeInternal, "site_category", err.Error()))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "site.category", code, "site", code, map[string]any{
		"category": category,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "code": code, "category": category})
}

func (s *Server) handleAdminSetSiteTags(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path", "missing code in path"), reqID))
		return
	}
	actor, isAdmin, authErr := s.authenticateActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	actorType, actorID, actorRole := auditActorFromOwner(actor, isAdmin)
	writeTagsError := func(detail map[string]any, apiErr *APIError) {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "site.tags", code, "site", code, detail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
	}
	site, err := s.deployer.GetSite(r.Context(), code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeTagsError(nil, NewError(CodeNotFound, "site", "site not found"))
			return
		}
		writeTagsError(nil, NewError(CodeInternal, "site_tags", err.Error()))
		return
	}
	if !isAdmin && site.OwnerTokenID != actor {
		writeTagsError(nil, NewError(CodeForbidden, "site_tags", "you can only update your own site tags"))
		return
	}
	var req SiteTagsRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	tagsDetail := map[string]any{"tags": req.Tags}
	if err := s.deployer.SetSiteTags(r.Context(), code, req.Tags); err != nil {
		writeTagsError(tagsDetail, NewError(CodeInternal, "site_tags", err.Error()))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "site.tags", code, "site", code, map[string]any{
		"tags": req.Tags,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "code": code, "tags": splitSiteTags(strings.Join(req.Tags, ","))})
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
	actorType, actorID, actorRole := s.auditActorFromRequest(r)
	if apiErr := s.deployer.DeleteSite(r.Context(), code); apiErr != nil {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "site.delete", code, "site", code, nil, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "site.delete", code, "site", code, nil)
	writeJSON(w, http.StatusOK, SiteDeleteResponse{Success: true, Code: code})
}

// handleAdminRevealSiteAccessPassword 返回站点访问密码的明文（仅管理员可用）。
func (s *Server) handleAdminRevealSiteAccessPassword(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	code := r.PathValue("code")
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "path",
			"missing code in path"), reqID))
		return
	}
	// 仅管理员可调用
	actor, isAdmin, authErr := s.authenticateActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	if !isAdmin {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "admin", "admin privileges required"), reqID))
		return
	}
	password, err := s.deployer.RevealSiteAccessPassword(r.Context(), code)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeNotFound, "access_password", err.Error()), reqID))
		return
	}
	s.recordAuditLog(r, "admin", actor, "admin", "site.access_reveal", code, "site", code, map[string]any{"accessRevealed": true})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "password": password})
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
	actorType, actorID, actorRole := s.auditActorFromRequest(r)
	strategyDetail := map[string]any{"strategy": req.PrimaryVersionStrategy}
	resp, apiErr := s.deployer.SetPrimaryStrategy(r.Context(), code, req.PrimaryVersionStrategy)
	if apiErr != nil {
		s.recordFailedAuditLog(r, actorType, actorID, actorRole, "site.primary_strategy", code, "site", code, strategyDetail, apiErr)
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	s.recordAuditLog(r, actorType, actorID, actorRole, "site.primary_strategy", code, "site", code, map[string]any{
		"strategy": resp.PrimaryVersionStrategy,
	})
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
	actor, actorIsAdmin, actorErr := s.authenticateActor(r)
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
		actorIsAdmin = user.IsAdmin
		ownerTokenID = "user:" + user.ID
		userID = user.ID
	} else if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		tok, authErr := s.authenticateToken(r)
		if authErr != nil {
			writeError(w, apiErrWithReqID(authErr, reqID))
			return
		}
		actorIsAdmin = tok.IsAdmin
		ownerTokenID = ownerForToken(tok)
		if tok.OwnerUserID != "" {
			user, userErr := s.auth.GetUser(r.Context(), tok.OwnerUserID)
			if userErr != nil || !user.IsActive {
				writeError(w, apiErrWithReqID(NewError(CodeForbidden, "user", "token owner is inactive or missing"), reqID))
				return
			}
			userID = user.ID
		}
	} else {
		ownerTokenID = actor
	}
	clientIP := clientIPFromRequest(r)

	// 转换成 DeployRequest，强制 createVersion=true。filename 只是入口提示，可留空交给部署层识别。
	filename := strings.TrimSpace(req.Filename)
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
	if resp.Created && userID != "" {
		if _, err := s.auth.IncrementUserDeployCount(r.Context(), userID); err != nil {
			s.logger.Printf("failed to increment user deploy count %s: %v", userID, err)
		}
	}
	actorType, actorID, actorRole := auditActorFromOwner(actor, actorIsAdmin)
	s.recordAuditLog(r, actorType, actorID, actorRole, "deploy.version.create", resp.Code, "site", resp.Code, map[string]any{
		"versionNumber": resp.VersionNumber,
		"versionId":     resp.VersionID,
		"filename":      filename,
		"title":         req.Title,
		"legacyPatch":   true,
	})

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
		s.renderAccessPasswordPageV2(w, code, versionPtr)
		return
	}

	var mainEntry string
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
	if !hostedSubPathSafe(sub) {
		http.NotFound(w, r)
		return
	}

	if versionPtr == nil && sub == mainEntry {
		go func(c string) {
			_ = s.deployer.IncrementViewCount(context.Background(), c)
		}(code)
	}

	body, modTime, apiErr := s.deployer.ReadAppFile(r.Context(), code, versionPtr, sub)
	if apiErr != nil {
		http.NotFound(w, r)
		return
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
	securityMode := s.effectiveSecurityModeForServe(r.Context(), code, versionPtr)
	s.serveHostedFile(w, r, code, versionPtr, sub, body, modTime, routePrefix, securityMode)
}

func (s *Server) effectiveSecurityModeForServe(ctx context.Context, code string, versionPtr *int64) string {
	site, err := s.deployer.GetSite(ctx, code)
	if err != nil {
		return "standard"
	}
	siteMode := normalizeSiteSecurityMode(site.SecurityMode)
	if siteMode != "auto" {
		return siteMode
	}
	versionNumber := int64(0)
	if versionPtr != nil {
		versionNumber = *versionPtr
	} else if site.CurrentVersion != nil {
		versionNumber = *site.CurrentVersion
	}
	if versionNumber <= 0 {
		return "standard"
	}
	if reader, ok := s.deployer.(bundleMetadataReader); ok {
		if meta, err := reader.GetVersionBundle(ctx, code, versionNumber); err == nil {
			return effectiveSiteSecurityMode(siteMode, meta.SecurityMode)
		}
	}
	return "standard"
}

func (s *Server) serveHostedFile(w http.ResponseWriter, r *http.Request, code string, versionPtr *int64, sub string, body []byte, modTime time.Time, routePrefix string, securityMode string) {
	setHostedContentCORSHeaders(w, r)
	lowerSub := strings.ToLower(sub)
	injectAppSnippets := !hostedPreviewRequest(r)
	if strings.HasSuffix(lowerSub, ".md") || strings.HasSuffix(lowerSub, ".markdown") {
		nonce := markdownCSPNonce()
		theme := render.NormalizeMarkdownTheme(r.URL.Query().Get("theme"))
		s.setHostedMarkdownSecurityHeaders(w, nonce, injectAppSnippets && injectionTargetHasContent(s.cfg.ContentInjection.App))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		rendered := s.renderHostedMarkdown(r.Context(), code, versionPtr, sub, body, theme)
		rendered = render.ApplyMarkdownNonce(rendered, nonce)
		if injectAppSnippets {
			rendered = injectHTMLSnippets(rendered, s.appHTMLInjectionSnippets(nonce))
		}
		_, _ = io.WriteString(w, rendered)
		return
	}
	s.setHostedContentSecurityHeaders(w, securityMode)
	if !strings.HasSuffix(lowerSub, ".html") && !strings.HasSuffix(lowerSub, ".htm") {
		http.ServeContent(w, r, filepath.Base(sub), modTime, bytes.NewReader(body))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := injectHostedHTMLCompat(body, routePrefix)
	if injectAppSnippets {
		html = injectHTMLSnippets(html, s.appHTMLInjectionSnippets(""))
	}
	http.ServeContent(w, r, filepath.Base(sub), modTime, strings.NewReader(html))
}

func hostedPreviewRequest(r *http.Request) bool {
	v := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("preview")))
	return v == "1" || v == "true" || v == "yes"
}

func hostedSubPathSafe(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) {
		return false
	}
	path = filepath.ToSlash(path)
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func (s *Server) renderHostedMarkdown(ctx context.Context, code string, versionPtr *int64, sub string, body []byte, theme string) string {
	theme = render.NormalizeMarkdownTheme(theme)
	sum := sha256.Sum256(body)
	contentSHA := hex.EncodeToString(sum[:])
	versionNumber := s.markdownRenderVersionNumber(ctx, code, versionPtr)
	cacheKey := markdownRenderCacheKey(code, versionNumber, sub, contentSHA, theme)
	cache, ok := s.deployer.(markdownRenderCache)
	if ok {
		if entry, hit, err := cache.GetRenderCache(ctx, cacheKey); err == nil && hit {
			return entry.HTML
		}
	}
	html := render.MarkdownToHTMLWithTheme(body, theme)
	if ok {
		_ = cache.PutRenderCache(ctx, store.RenderCacheEntry{
			CacheKey:      cacheKey,
			SiteCode:      code,
			VersionNumber: versionNumber,
			MainEntry:     sub,
			ContentSHA256: contentSHA,
			Theme:         theme,
			HTML:          html,
			CreatedAt:     time.Now().UTC(),
		})
	}
	return html
}

func (s *Server) markdownRenderVersionNumber(ctx context.Context, code string, versionPtr *int64) int64 {
	if versionPtr != nil && *versionPtr > 0 {
		return *versionPtr
	}
	site, err := s.deployer.GetSite(ctx, code)
	if err != nil || site.CurrentVersion == nil || *site.CurrentVersion <= 0 {
		return 0
	}
	return *site.CurrentVersion
}

func markdownRenderCacheKey(code string, versionNumber int64, entry string, contentSHA string, theme string) string {
	return strings.Join([]string{
		code,
		strconv.FormatInt(versionNumber, 10),
		filepath.ToSlash(entry),
		contentSHA,
		render.NormalizeMarkdownTheme(theme),
		render.MarkdownRendererVersion,
	}, ":")
}

func (s *Server) setHostedContentSecurityHeaders(w http.ResponseWriter, securityMode string) {
	csp := hostedContentCSP(securityMode)
	if frameAncestors := s.frameAncestorsDirective(); frameAncestors != "" {
		csp += "; " + frameAncestors
	}
	csp = withCSPReportEndpoint(csp)
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func hostedContentCSP(securityMode string) string {
	switch strings.ToLower(strings.TrimSpace(securityMode)) {
	case "strict":
		return "sandbox allow-scripts allow-forms allow-popups allow-downloads allow-modals; " +
			"default-src 'self' data: blob:; " +
			"img-src 'self' data: blob: https: http:; media-src 'self' data: blob: https: http:; font-src 'self' data:; " +
			"style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; " +
			"connect-src 'self' https: http: wss: ws:; frame-src 'self' https: http:; child-src 'self' https: http:"
	case "trusted":
		return "default-src * data: blob: 'unsafe-inline' 'unsafe-eval'; " +
			"img-src * data: blob:; media-src * data: blob:; font-src * data: blob:; style-src * 'unsafe-inline'; script-src * 'unsafe-inline' 'unsafe-eval'; " +
			"connect-src *; frame-src *; child-src *"
	default:
		return "sandbox allow-scripts allow-forms allow-popups allow-downloads allow-modals allow-pointer-lock allow-top-navigation-by-user-activation; " +
			"default-src * data: blob: 'unsafe-inline' 'unsafe-eval'; " +
			"img-src * data: blob:; media-src * data: blob:; font-src * data: blob:; style-src * 'unsafe-inline'; script-src * 'unsafe-inline' 'unsafe-eval'; " +
			"connect-src *; frame-src *; child-src *"
	}
}

func (s *Server) setHostedMarkdownSecurityHeaders(w http.ResponseWriter, nonce string, allowInjectedSnippets bool) {
	scriptSrc := "script-src 'nonce-" + nonce + "'"
	styleSrc := "style-src 'self' 'nonce-" + nonce + "'"
	styleElemSrc := "style-src-elem 'self' 'unsafe-inline'"
	connectSrc := "connect-src 'self'"
	if allowInjectedSnippets {
		scriptSrc = "script-src 'nonce-" + nonce + "' https: http:"
		styleSrc = "style-src 'self' 'nonce-" + nonce + "' https: http:"
		styleElemSrc = "style-src-elem 'self' https: http: 'unsafe-inline'"
		connectSrc = "connect-src 'self' https: http: wss: ws:"
	}
	csp := "default-src 'self'; " +
		scriptSrc + "; " +
		styleSrc + "; " +
		styleElemSrc + "; " +
		"style-src-attr 'unsafe-inline'; " +
		"img-src 'self' data: blob: https: http:; media-src 'self' data: blob: https: http:; font-src 'self' data:; " +
		connectSrc + "; " +
		"object-src 'none'; base-uri 'none'; form-action 'none'"
	if frameAncestors := s.frameAncestorsDirective(); frameAncestors != "" {
		csp += "; " + frameAncestors
	}
	csp = withCSPReportEndpoint(csp)
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func (s *Server) setAdminUISecurityHeaders(w http.ResponseWriter) {
	setAdminSecurityHeaders(w)
	w.Header().Set("Cache-Control", "no-cache")
}

func setAdminSecurityHeaders(w http.ResponseWriter) {
	csp := "default-src 'self'; " +
		"script-src 'self'; style-src 'self'; " +
		"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; " +
		"object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	csp = withCSPReportEndpoint(csp)
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}

func withCSPReportEndpoint(csp string) string {
	csp = strings.TrimSpace(csp)
	if csp == "" || strings.Contains(csp, "report-uri ") {
		return csp
	}
	return csp + "; report-uri /api/security/csp-report"
}

func markdownCSPNonce() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
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

func noSniffFileServer(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})
}

func markdownAssetFileServer(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		if strings.TrimSpace(r.Header.Get("Origin")) == "null" {
			w.Header().Set("Access-Control-Allow-Origin", "null")
			w.Header().Add("Vary", "Origin")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func adminAssetFileServer(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setAdminSecurityHeaders(w)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})
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

type htmlInjectionSnippets struct {
	Head      string
	BodyStart string
	BodyEnd   string
}

func (s *Server) mainHTMLInjectionSnippets(nonce string) htmlInjectionSnippets {
	return htmlInjectionSnippetsFromTarget(s.cfg.ContentInjection.Main, nonce)
}

func (s *Server) appHTMLInjectionSnippets(nonce string) htmlInjectionSnippets {
	return htmlInjectionSnippetsFromTarget(s.cfg.ContentInjection.App, nonce)
}

func htmlInjectionSnippetsFromTarget(target config.InjectionTargetConfig, nonce string) htmlInjectionSnippets {
	if !injectionTargetHasContent(target) {
		return htmlInjectionSnippets{}
	}
	return htmlInjectionSnippets{
		Head:      addNonceToSnippet(target.HeadCode, nonce),
		BodyStart: addNonceToSnippet(target.BodyStartCode, nonce),
		BodyEnd:   addNonceToSnippet(target.BodyEndCode, nonce),
	}
}

func injectionTargetHasContent(target config.InjectionTargetConfig) bool {
	if !target.Enabled {
		return false
	}
	return strings.TrimSpace(target.HeadCode) != "" ||
		strings.TrimSpace(target.BodyStartCode) != "" ||
		strings.TrimSpace(target.BodyEndCode) != ""
}

func injectHTMLSnippets(htmlText string, snippets htmlInjectionSnippets) string {
	if strings.TrimSpace(snippets.Head) == "" &&
		strings.TrimSpace(snippets.BodyStart) == "" &&
		strings.TrimSpace(snippets.BodyEnd) == "" {
		return htmlText
	}
	if strings.TrimSpace(snippets.Head) != "" {
		htmlText = injectBeforeClosingTag(htmlText, "</head>", snippets.Head, true)
	}
	if strings.TrimSpace(snippets.BodyStart) != "" {
		htmlText = injectAfterOpeningBody(htmlText, snippets.BodyStart)
	}
	if strings.TrimSpace(snippets.BodyEnd) != "" {
		htmlText = injectBeforeClosingTag(htmlText, "</body>", snippets.BodyEnd, false)
	}
	return htmlText
}

func injectBeforeClosingTag(htmlText, closingTag, snippet string, prependWhenMissing bool) string {
	insert := "\n" + snippet + "\n"
	idx := strings.LastIndex(strings.ToLower(htmlText), strings.ToLower(closingTag))
	if idx >= 0 {
		return htmlText[:idx] + insert + htmlText[idx:]
	}
	if prependWhenMissing {
		return insert + htmlText
	}
	return htmlText + insert
}

func injectAfterOpeningBody(htmlText, snippet string) string {
	insert := "\n" + snippet + "\n"
	lower := strings.ToLower(htmlText)
	idx := strings.Index(lower, "<body")
	if idx < 0 {
		return insert + htmlText
	}
	end := strings.Index(htmlText[idx:], ">")
	if end < 0 {
		return insert + htmlText
	}
	insertAt := idx + end + 1
	return htmlText[:insertAt] + insert + htmlText[insertAt:]
}

func addNonceToSnippet(snippet, nonce string) string {
	if strings.TrimSpace(nonce) == "" || strings.TrimSpace(snippet) == "" {
		return snippet
	}
	snippet = addNonceToOpeningTags(snippet, "script", nonce)
	snippet = addNonceToOpeningTags(snippet, "style", nonce)
	return snippet
}

func addNonceToOpeningTags(snippet, tagName, nonce string) string {
	needle := "<" + strings.ToLower(tagName)
	lower := strings.ToLower(snippet)
	var out strings.Builder
	pos := 0
	for {
		rel := strings.Index(lower[pos:], needle)
		if rel < 0 {
			out.WriteString(snippet[pos:])
			break
		}
		start := pos + rel
		afterName := start + len(needle)
		if afterName < len(lower) {
			next := lower[afterName]
			if next != '>' && next != ' ' && next != '\t' && next != '\n' && next != '\r' && next != '/' {
				out.WriteString(snippet[pos:afterName])
				pos = afterName
				continue
			}
		}
		endRel := strings.Index(snippet[start:], ">")
		if endRel < 0 {
			out.WriteString(snippet[pos:])
			break
		}
		end := start + endRel
		out.WriteString(snippet[pos:start])
		tag := snippet[start : end+1]
		if strings.Contains(strings.ToLower(tag), "nonce") {
			out.WriteString(tag)
			pos = end + 1
			continue
		}
		insertAt := end
		if end > start && snippet[end-1] == '/' {
			insertAt = end - 1
		}
		out.WriteString(snippet[start:insertAt])
		out.WriteString(` nonce="`)
		out.WriteString(html.EscapeString(nonce))
		out.WriteString(`"`)
		out.WriteString(snippet[insertAt : end+1])
		pos = end + 1
	}
	return out.String()
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
