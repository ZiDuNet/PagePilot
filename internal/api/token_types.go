package api

import "time"

// TokenCreateRequest 是 POST /api/token 请求体。
type TokenCreateRequest struct {
	Label       string `json:"label,omitempty"`
	IsAdmin     bool   `json:"isAdmin,omitempty"`
	OwnerUserID string `json:"ownerUserId,omitempty"`
	ExpiresAt   string `json:"expiresAt,omitempty"`
	TTLSeconds  *int64 `json:"ttlSeconds,omitempty"`
}

// TokenCreateResponse 是 POST /api/token 响应。Plaintext 仅此一次返回。
type TokenCreateResponse struct {
	Success     bool       `json:"success"`
	ID          string     `json:"id"`
	Token       string     `json:"token"`
	Label       string     `json:"label,omitempty"`
	IsAdmin     bool       `json:"isAdmin"`
	OwnerUserID string     `json:"ownerUserId,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// TokenListItem 是 GET /api/tokens 响应里的一项，不包含明文 token。
type TokenListItem struct {
	ID            string     `json:"id"`
	Label         string     `json:"label,omitempty"`
	IsAdmin       bool       `json:"isAdmin"`
	IsRevoked     bool       `json:"isRevoked"`
	OwnerUserID   string     `json:"ownerUserId,omitempty"`
	OwnerUsername string     `json:"ownerUsername,omitempty"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
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
	Email     string `json:"email,omitempty"`
	Password  string `json:"password"`
	CaptchaID string `json:"captchaId"`
	Captcha   string `json:"captcha"`
	EmailCode string `json:"emailCode,omitempty"`
}

type RegisterResponse struct {
	Success       bool   `json:"success"`
	UserID        string `json:"userId"`
	Username      string `json:"username"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"emailVerified"`
}

type EmailVerificationRequest struct {
	Email     string `json:"email"`
	CaptchaID string `json:"captchaId"`
	Captcha   string `json:"captcha"`
}

type EmailVerificationResponse struct {
	Success   bool   `json:"success"`
	Email     string `json:"email"`
	ExpiresIn int    `json:"expiresIn"`
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
	Image   string `json:"image,omitempty"`
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

type SessionClaimRequest struct {
	SessionID string `json:"sessionId,omitempty"`
}

type SessionClaimResponse struct {
	Success        bool   `json:"success"`
	SessionID      string `json:"sessionId"`
	UserID         string `json:"userId"`
	SiteCount      int    `json:"siteCount"`
	DeployCount    int    `json:"deployCount"`
	AlreadyClaimed bool   `json:"alreadyClaimed"`
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
