// Package store 提供元数据的持久化抽象。
package store

import (
	"context"
	"errors"
	"time"
)

// Site 是 sites 表的记录。
type Site struct {
	Code                   string
	PublicID               string // 对外暴露的 UUID
	OwnerTokenID           string
	CurrentVersion         *int64
	PrimaryVersionStrategy string // 'likes' | 'latest'
	ViewCount              int64
	LikeCount              int64
	Status                 string // 'active' | 'inactive'
	AccessPasswordHash     string
	IsPinned               bool
	PinnedAt               *time.Time
	ExpiresAt              *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
	Source                 string
}

// MarketplaceDeploy 是 marketplace 列表展示用的聚合记录（site + 当前版本元信息 + 统计）。
type MarketplaceDeploy struct {
	ID                     string // site.public_id
	Code                   string // site.code
	OwnerTokenID           string
	CurrentVersion         *int64
	CurrentVersionID       string
	Title                  string
	Description            string
	Filename               string // 当前版本主入口文件名
	MainEntry              string
	FileSize               int64 // 当前版本总大小
	PrimaryVersionStrategy string
	ViewCount              int64
	LikeCount              int64
	VersionCount           int
	Status                 string
	AccessProtected        bool
	IsPinned               bool
	PinnedAt               *time.Time
	ExpiresAt              *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// LikeRecord 是 likes 表的记录。
type LikeRecord struct {
	SiteCode        string
	UserFingerprint string
	CreatedAt       time.Time
}

// Version 是 versions 表的记录。
type Version struct {
	ID            string
	SiteCode      string
	VersionNumber int64
	Title         string
	Description   string
	MainEntry     string
	TotalSize     int64
	FileCount     int
	ContentSha256 string
	IsLocked      bool
	Status        string
	CreatedAt     time.Time
}

// FileMeta 是 files 表的记录。
type FileMeta struct {
	SiteCode      string
	VersionNumber int64
	Path          string
	Size          int64
	Sha256        string
	IsBinary      bool
}

// Token 是 tokens 表的记录。
type Token struct {
	ID          string
	TokenHash   string
	Label       string
	IsAdmin     bool
	IsRevoked   bool
	OwnerUserID string
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	LastUsedAt  *time.Time
}

type AnonymousSession struct {
	ID              string
	AgentID         string
	AgentLabel      string
	DeviceIP        string
	UserAgent       string
	DeployCount     int
	ClaimedByUserID string
	ClaimedAt       *time.Time
	CreatedAt       time.Time
	LastUsedAt      time.Time
}

type AnonymousSessionClaimResult struct {
	SessionID      string
	UserID         string
	SiteCount      int
	DeployCount    int
	AlreadyClaimed bool
}

type AdminUser struct {
	ID           string
	Username     string
	PasswordHash string
	IsAdmin      bool
	IsActive     bool
	CanLike      bool
	DeployLimit  int
	DeployCount  int
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

type AdminSession struct {
	ID          string
	UserID      string
	SessionHash string
	CreatedAt   time.Time
	LastUsedAt  time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
}

// SiteWithMeta 用于 admin UI 列表：site 主表 + 版本统计聚合。
type SiteWithMeta struct {
	Code            string
	PublicID        string
	OwnerTokenID    string
	CurrentVersion  *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Source          string
	ViewCount       int64
	LikeCount       int64
	Status          string
	AccessProtected bool
	IsPinned        bool
	PinnedAt        *time.Time
	VersionCount    int
	TotalSize       int64
	LastVersionAt   *time.Time
}

// ErrNotFound 表示记录不存在。
var ErrNotFound = errors.New("not found")

// Store 定义元数据访问接口。
type Store interface {
	Close() error

	// CreateSite 创建一个新 site 记录。current_version 初始为 NULL。
	CreateSite(ctx context.Context, s Site) error

	// GetSite 按 code 查 site。
	GetSite(ctx context.Context, code string) (Site, error)

	// SiteExists 判断 code 是否已存在。
	SiteExists(ctx context.Context, code string) (bool, error)

	// SetPrimaryStrategy 设置 main URL 选版本策略。
	SetPrimaryStrategy(ctx context.Context, code, strategy string) error

	// CreateVersion 写一条版本记录。
	CreateVersion(ctx context.Context, v Version) error

	// MaxVersionNumber 返回某 site 当前最大版本号；新 site 返回 0。
	MaxVersionNumber(ctx context.Context, code string) (int64, error)

	// ListVersions 列出某 site 所有版本，按 version_number 升序。
	ListVersions(ctx context.Context, code string) ([]Version, error)

	// UpdateVersionLock 设置/解除版本锁定。
	UpdateVersionLock(ctx context.Context, code string, version int64, locked bool) error

	// UpdateVersionStatus 设置版本状态（active/inactive）。
	UpdateVersionStatus(ctx context.Context, code string, version int64, status string) error

	// DeleteVersion 删除版本记录（不含磁盘文件，磁盘清理由调用方负责）。
	DeleteVersion(ctx context.Context, code string, version int64) error

	// ListFiles 列出某版本的所有文件元数据。
	ListFiles(ctx context.Context, code string, version int64) ([]FileMeta, error)

	// SetCurrentVersion 切换 site 的当前版本。传 nil 表示下线。
	SetCurrentVersion(ctx context.Context, code string, version *int64) error

	// CreateFiles 批量写入文件元数据。
	CreateFiles(ctx context.Context, files []FileMeta) error

	// GetVersion 按 site_code + version_number 取版本。
	GetVersion(ctx context.Context, code string, version int64) (Version, error)

	// GetVersionByUUID 按 versions.id 取版本。
	GetVersionByUUID(ctx context.Context, id string) (Version, error)

	// UpdateVersionContent 更新版本内容元数据 + 替换文件清单。
	UpdateVersionContent(ctx context.Context, code string, version int64, meta Version, files []FileMeta) error

	// ===== Token 管理（Day 5） =====

	CreateToken(ctx context.Context, t Token) error
	GetTokenByHash(ctx context.Context, hash string) (Token, error)
	GetTokenByID(ctx context.Context, id string) (Token, error)
	ListTokens(ctx context.Context) ([]Token, error)
	RevokeToken(ctx context.Context, id string) error
	TouchTokenLastUsed(ctx context.Context, id string) error

	CreateAnonymousSession(ctx context.Context, session AnonymousSession) error
	GetAnonymousSession(ctx context.Context, id string) (AnonymousSession, error)
	UpdateAnonymousSessionMeta(ctx context.Context, id, agentID, agentLabel, deviceIP, userAgent string) error
	IncrementAnonymousSessionDeployCount(ctx context.Context, id string) (AnonymousSession, error)
	ClaimAnonymousSession(ctx context.Context, id, userID string) (AnonymousSessionClaimResult, error)
	ListAnonymousSessions(ctx context.Context, limit int) ([]AnonymousSession, error)

	CountAdminUsers(ctx context.Context) (int, error)
	CreateAdminUser(ctx context.Context, user AdminUser) error
	UpdateAdminUser(ctx context.Context, user AdminUser) error
	UpdateAdminUserPassword(ctx context.Context, id, passwordHash string) error
	DeleteAdminUser(ctx context.Context, id string) error
	GetAdminUserByUsername(ctx context.Context, username string) (AdminUser, error)
	GetAdminUserByID(ctx context.Context, id string) (AdminUser, error)
	ListAdminUsers(ctx context.Context) ([]AdminUser, error)
	TouchAdminUserLastLogin(ctx context.Context, id string) error
	IncrementAdminUserDeployCount(ctx context.Context, id string) (AdminUser, error)
	CreateAdminSession(ctx context.Context, session AdminSession) error
	GetAdminSessionByHash(ctx context.Context, hash string) (AdminSession, error)
	TouchAdminSessionLastUsed(ctx context.Context, id string) error
	RevokeAdminSession(ctx context.Context, id string) error

	// ===== 设置（Day 7：管理后台可写 baseURL） =====

	// GetSetting 读取一个键值；不存在返回 ("", nil)。
	GetSetting(ctx context.Context, key string) (string, error)

	// SetSetting 写入 / 更新一个键值。
	SetSetting(ctx context.Context, key, value string) error

	// ===== 站点列表（Day 7：admin UI） =====

	// ListSites 列出所有 site（含聚合统计），按创建时间降序。
	ListSites(ctx context.Context) ([]SiteWithMeta, error)

	// DeleteSite 删除整个 site（含所有版本）。
	// 注意：磁盘文件清理由调用方负责。
	DeleteSite(ctx context.Context, code string) error

	// ===== 应用商城（marketplace） =====

	// ListMarketplaceDeploys 分页 + 搜索 + 排序 + 状态过滤，返回 marketplace 卡片数据。
	// sort: "newest" | "oldest" | "views_desc" | "views_asc" | "likes_desc" | "likes_asc"
	// status: "" | "active" | "inactive"
	ListMarketplaceDeploys(ctx context.Context, q, status, sort string, page, pageSize int) ([]MarketplaceDeploy, int, error)

	// GetMarketplaceDeploy 按 code 取单条 marketplace 卡片数据（含当前版本元信息）。
	GetMarketplaceDeploy(ctx context.Context, code string) (MarketplaceDeploy, error)

	// GetMarketplaceDeployByUUID 按 public_id 取单条 marketplace 卡片数据。
	GetMarketplaceDeployByUUID(ctx context.Context, publicID string) (MarketplaceDeploy, error)

	// IncrementViewCount 给 site.view_count + 1。
	IncrementViewCount(ctx context.Context, code string) error

	// AddLike 给 site 加一次点赞（user_fingerprint 已存在则忽略）。
	// 返回加完后 like_count 总数。
	AddLike(ctx context.Context, code, userFingerprint string) (int64, error)

	// UpdateSiteStatus 设置 site.status（active/inactive）。
	UpdateSiteStatus(ctx context.Context, code, status string) error

	// SetSitePinned 设置或取消首页应用商城置顶。
	SetSitePinned(ctx context.Context, code string, pinned bool) error

	// TouchSiteUpdated 把 site.updated_at 更新为当前时间。
	TouchSiteUpdated(ctx context.Context, code string) error

	// SetSiteAccessPasswordHash sets or clears the site access password hash.
	SetSiteAccessPasswordHash(ctx context.Context, code, hash string) error
}
