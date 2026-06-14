package api

import "time"

// TokenCreateRequest 是 POST /api/token 请求体。
type TokenCreateRequest struct {
	Label       string `json:"label,omitempty"`       // 可选标签
	IsAdmin     bool   `json:"isAdmin,omitempty"`     // 是否管理员
	OwnerUserID string `json:"ownerUserId,omitempty"` // 归属用户；管理员可指定
}

// TokenCreateResponse 是 POST /api/token 响应。Plaintext 仅此一次返回。
type TokenCreateResponse struct {
	Success     bool      `json:"success"`
	ID          string    `json:"id"`
	Token       string    `json:"token"` // 明文 token，仅此一次可见
	Label       string    `json:"label,omitempty"`
	IsAdmin     bool      `json:"isAdmin"`
	OwnerUserID string    `json:"ownerUserId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// TokenListItem 是 GET /api/tokens 响应里的一项。
// 不包含明文（只存 hash）。
type TokenListItem struct {
	ID            string     `json:"id"`
	Label         string     `json:"label,omitempty"`
	IsAdmin       bool       `json:"isAdmin"`
	IsRevoked     bool       `json:"isRevoked"`
	OwnerUserID   string     `json:"ownerUserId,omitempty"`
	OwnerUsername string     `json:"ownerUsername,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastUsedAt    *time.Time `json:"lastUsedAt,omitempty"`
}

// TokenListResponse 是 GET /api/tokens 响应。
type TokenListResponse struct {
	Success bool            `json:"success"`
	Tokens  []TokenListItem `json:"tokens"`
}

// TokenRevokeResponse 是 DELETE /api/tokens/{id} 响应。
type TokenRevokeResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
}

type AdminLoginRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	CaptchaID string `json:"captchaId,omitempty"`
	Captcha   string `json:"captcha,omitempty"`
}

type AdminSetupRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	CaptchaID string `json:"captchaId,omitempty"`
	Captcha   string `json:"captcha,omitempty"`
}

type RegisterRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	CaptchaID string `json:"captchaId"`
	Captcha   string `json:"captcha"`
}

type RegisterResponse struct {
	Success  bool   `json:"success"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

type AccountPasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type AccountPasswordResponse struct {
	Success bool `json:"success"`
}

type CaptchaResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
	Prompt  string `json:"prompt"`
}

type AdminLoginResponse struct {
	Success  bool   `json:"success"`
	Mode     string `json:"mode"`
	UserID   string `json:"userId,omitempty"`
	Username string `json:"username,omitempty"`
	IsAdmin  bool   `json:"isAdmin"`
}

type AdminLogoutResponse struct {
	Success bool `json:"success"`
}

type AdminSetupResponse struct {
	Success  bool   `json:"success"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

// AdminSessionResponse is returned by GET /api/admin/session.
type AdminSessionResponse struct {
	Success     bool   `json:"success"`
	Mode        string `json:"mode"`
	TokenID     string `json:"tokenId,omitempty"`
	UserID      string `json:"userId,omitempty"`
	Username    string `json:"username,omitempty"`
	Label       string `json:"label,omitempty"`
	IsAdmin     bool   `json:"isAdmin,omitempty"`
	NeedsSetup  bool   `json:"needsSetup,omitempty"`
	LoginMethod string `json:"loginMethod,omitempty"`
}

type AgentBindingCreateRequest struct {
	Label string `json:"label,omitempty"`
}

type AgentBindingCodeItem struct {
	Code       string     `json:"code,omitempty"`
	Label      string     `json:"label,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	ConsumedAt *time.Time `json:"consumedAt,omitempty"`
	ConsumedBy string     `json:"consumedBy,omitempty"`
}

type AgentBindingCreateResponse struct {
	Success   bool                 `json:"success"`
	Binding   AgentBindingCodeItem `json:"binding"`
	ExpiresAt time.Time            `json:"expiresAt"`
}

type AgentBindingListResponse struct {
	Success  bool                   `json:"success"`
	Bindings []AgentBindingCodeItem `json:"bindings"`
}

type AgentBindRequest struct {
	Code       string `json:"code"`
	AgentLabel string `json:"agentLabel,omitempty"`
	AgentID    string `json:"agentId,omitempty"`
}

type AgentBindResponse struct {
	Success   bool      `json:"success"`
	Token     string    `json:"token"`
	TokenID   string    `json:"tokenId"`
	Username  string    `json:"username"`
	AgentID   string    `json:"agentId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
