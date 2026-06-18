package api

import "time"

// ===== 设置（GET/PUT /api/config） =====

// ConfigResponse 是 GET /api/config 响应。包含可读的运行时配置。
type ConfigResponse struct {
	Success          bool            `json:"success"`
	PublicBaseURL    string          `json:"publicBaseURL"`
	Mode             string          `json:"mode"` // "dev" | "prod"
	CORSAllowOrigins string          `json:"corsAllowOrigins"`
	CooldownSeconds  int             `json:"cooldownSeconds"`
	Limits           Limits          `json:"limits"`
	AnonymousPolicy  AnonymousPolicy `json:"anonymousPolicy"`
	Version          string          `json:"version"`
}

type AnonymousPolicy struct {
	DeployLimit int `json:"deployLimit"`
}

// Limits 是配额描述。
type Limits struct {
	MaxSingleFileBytes int64 `json:"maxSingleFileBytes"`
	MaxSiteTotalBytes  int64 `json:"maxSiteTotalBytes"`
	MaxFilesPerSite    int   `json:"maxFilesPerSite"`
}

// ConfigUpdateRequest 是 PUT /api/config 请求体。任一非空字段会被更新。
type ConfigUpdateRequest struct {
	PublicBaseURL        *string `json:"publicBaseURL,omitempty"`
	AnonymousDeployLimit *int    `json:"anonymousDeployLimit,omitempty"`
	CooldownSeconds      *int    `json:"cooldownSeconds,omitempty"`
	MaxSingleFileBytes   *int64  `json:"maxSingleFileBytes,omitempty"`
	MaxSiteTotalBytes    *int64  `json:"maxSiteTotalBytes,omitempty"`
	MaxFilesPerSite      *int    `json:"maxFilesPerSite,omitempty"`
	CORSAllowOrigins     *string `json:"corsAllowOrigins,omitempty"`
}

// ConfigUpdateResponse 是 PUT /api/config 响应。
type ConfigUpdateResponse struct {
	Success          bool            `json:"success"`
	PublicBaseURL    string          `json:"publicBaseURL"`
	CORSAllowOrigins string          `json:"corsAllowOrigins"`
	CooldownSeconds  int             `json:"cooldownSeconds"`
	Limits           Limits          `json:"limits"`
	AnonymousPolicy  AnonymousPolicy `json:"anonymousPolicy"`
}

// ===== 站点列表（GET /api/admin/sites） =====

// SiteListItem 是 admin 站点列表的一项。
type SiteListItem struct {
	Code            string     `json:"code"`
	PublicID        string     `json:"publicId"`
	OwnerTokenID    string     `json:"ownerTokenId"`
	OwnerUsername   string     `json:"ownerUsername,omitempty"`
	CurrentVersion  *int64     `json:"currentVersion,omitempty"`
	VersionCount    int        `json:"versionCount"`
	TotalSize       int64      `json:"totalSize"`
	ViewCount       int64      `json:"viewCount"`
	LikeCount       int64      `json:"likeCount"`
	Status          string     `json:"status"`
	AccessProtected bool       `json:"accessProtected"`
	IsPinned        bool       `json:"isPinned"`
	PinnedAt        *time.Time `json:"pinnedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	Source          string     `json:"source"`
	LastVersionAt   *time.Time `json:"lastVersionAt,omitempty"`
}

type UserListItem struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	IsAdmin     bool       `json:"isAdmin"`
	IsActive    bool       `json:"isActive"`
	CanLike     bool       `json:"canLike"`
	DeployLimit int        `json:"deployLimit"`
	DeployCount int        `json:"deployCount"`
	Remaining   int        `json:"remaining"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
}

type UserListResponse struct {
	Success bool           `json:"success"`
	Users   []UserListItem `json:"users"`
}

type UserCreateRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	IsAdmin     bool   `json:"isAdmin"`
	CanLike     bool   `json:"canLike"`
	DeployLimit int    `json:"deployLimit"`
}

type UserCreateResponse struct {
	Success bool         `json:"success"`
	User    UserListItem `json:"user"`
}

type UserUpdateRequest struct {
	Username    *string `json:"username,omitempty"`
	IsAdmin     *bool   `json:"isAdmin,omitempty"`
	IsActive    *bool   `json:"isActive,omitempty"`
	CanLike     *bool   `json:"canLike,omitempty"`
	DeployLimit *int    `json:"deployLimit,omitempty"`
}

type UserUpdateResponse struct {
	Success bool         `json:"success"`
	User    UserListItem `json:"user"`
}

type UserDeleteResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
}

// SiteListResponse 是 GET /api/admin/sites 响应。
type SiteListResponse struct {
	Success bool           `json:"success"`
	Sites   []SiteListItem `json:"sites"`
}

// SiteDeleteResponse 是 DELETE /api/admin/sites/{code} 响应。
type SiteDeleteResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
}

type SitePinRequest struct {
	Pinned *bool `json:"pinned"`
}

type SitePinResponse struct {
	Success  bool    `json:"success"`
	Code     string  `json:"code"`
	IsPinned bool    `json:"isPinned"`
	PinnedAt *string `json:"pinnedAt,omitempty"`
}

type AnonymousSessionResponse struct {
	Success     bool   `json:"success"`
	SessionID   string `json:"sessionId"`
	AgentID     string `json:"agentId,omitempty"`
	AgentLabel  string `json:"agentLabel,omitempty"`
	DeployCount int    `json:"deployCount"`
	DeployLimit int    `json:"deployLimit"`
	Remaining   int    `json:"remaining"`
}

type AnonymousSessionListItem struct {
	ID              string     `json:"id"`
	AgentID         string     `json:"agentId,omitempty"`
	AgentLabel      string     `json:"agentLabel,omitempty"`
	DeviceIP        string     `json:"deviceIp,omitempty"`
	UserAgent       string     `json:"userAgent,omitempty"`
	DeployCount     int        `json:"deployCount"`
	Remaining       int        `json:"remaining"`
	ClaimedByUserID string     `json:"claimedByUserId,omitempty"`
	ClaimedAt       *time.Time `json:"claimedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	LastUsedAt      time.Time  `json:"lastUsedAt"`
}

type AnonymousSessionListResponse struct {
	Success     bool                       `json:"success"`
	DeployLimit int                        `json:"deployLimit"`
	Sessions    []AnonymousSessionListItem `json:"sessions"`
}
