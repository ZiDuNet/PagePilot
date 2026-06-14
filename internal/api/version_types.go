package api

import "time"

// VersionItem 是 list versions 响应里的一项。
type VersionItem struct {
	VersionNumber int64     `json:"versionNumber"`
	ID            string    `json:"id"`
	Title         string    `json:"title,omitempty"`
	Description   string    `json:"description"`
	Size          int64     `json:"size"`
	FileCount     int       `json:"fileCount"`
	IsLocked      bool      `json:"isLocked"`
	IsCurrent     bool      `json:"isCurrent"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
}

// ListVersionsResponse 是 GET /api/deploys/{code}/versions 响应。
type ListVersionsResponse struct {
	Success        bool          `json:"success"`
	Code           string        `json:"code"`
	CurrentVersion *int64        `json:"currentVersion,omitempty"`
	Versions       []VersionItem `json:"versions"`
}

// LockRequest 是 POST .../lock 请求体。
type LockRequest struct {
	Locked bool `json:"locked"`
}

// LockResponse 是锁定响应。
type LockResponse struct {
	Success       bool   `json:"success"`
	Code          string `json:"code"`
	VersionNumber int64  `json:"versionNumber"`
	IsLocked      bool   `json:"isLocked"`
}

// SetCurrentRequest 是 PATCH .../current 请求体。
type SetCurrentRequest struct {
	VersionNumber *int64 `json:"versionNumber,omitempty"`
	VersionID     string `json:"versionId,omitempty"`
}

// SetCurrentResponse 是切换当前版本响应。
type SetCurrentResponse struct {
	Success        bool   `json:"success"`
	Code           string `json:"code"`
	CurrentVersion int64  `json:"currentVersion"`
}

// GetContentResponse 是 GET /api/deploy/content（JSON 模式）响应。
type GetContentResponse struct {
	Success     bool          `json:"success"`
	Code        string        `json:"code"`
	Version     int64         `json:"version"`
	Title       string        `json:"title,omitempty"`
	Description string        `json:"description"`
	MainEntry   string        `json:"mainEntry"`
	TotalSize   int64         `json:"totalSize"`
	IsLocked    bool          `json:"isLocked"`
	Files       []ContentFile `json:"files"`
	Content     string        `json:"content,omitempty"`
	CreatedAt   time.Time     `json:"createdAt"`
}

// ContentFile 是 GET /api/deploy/content 返回的文件清单。
type ContentFile struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Sha256   string `json:"sha256"`
	IsBinary bool   `json:"isBinary"`
	Content  string `json:"content,omitempty"`
}

// OverwriteRequest 是 PATCH /api/deploys/{code}/versions/{version} 的覆盖模式请求体。
// 对齐项目 OpenAPI VersionUpdateRequest：传 content 时 description 必填。
type OverwriteRequest struct {
	Description string       `json:"description"` // content 模式必填
	Title       string       `json:"title,omitempty"`
	Filename    string       `json:"filename,omitempty"`
	Content     string       `json:"content,omitempty"`
	Files       []DeployFile `json:"files,omitempty"`

	// 状态切换模式（与 content/files 互斥）
	Status *string `json:"status,omitempty"`
}
