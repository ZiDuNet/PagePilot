package api

import "time"

// PrimaryVersionStrategy 是 main URL 选版本策略。
//   - "likes"：主 URL 跟随"最高锁定（替代点赞）"的 active 版本（默认）
//   - "latest"：主 URL 始终跟随最新 active 版本（适合日报/迭代项目）
type PrimaryVersionStrategy string

const (
	StrategyLikes  PrimaryVersionStrategy = "likes"
	StrategyLatest PrimaryVersionStrategy = "latest"
)

// DeployRequest 是 POST /api/deploy 的请求体。
// 严格对齐项目 OpenAPI 3.1：
//   - filename 必填且必须 `\.html?$`（主入口文件名）
//   - description 必填，≤240 字符
//   - code pattern: ^[a-z0-9](?:[a-z0-9-]{2,30}[a-z0-9])?$
//
// 多文件扩展（hostctl 独有，OpenAPI 兼容）：
//   - files[]: 可选；提供时与 content 二选一
//   - 每个文件 path 走严格白名单（HTML/CSS/JS/字体/图片/任意静态资源均可）
type DeployRequest struct {
	// filename 必填；主入口 HTML 文件名，必须 .html 或 .htm
	Filename string `json:"filename"`

	// description 必填，≤240 字符
	Description string `json:"description"`

	// title 可选
	Title string `json:"title,omitempty"`

	// 单 HTML 模式：主入口的全文内容
	Content string `json:"content,omitempty"`

	// 多文件模式：HTML + JS + CSS + 字体 + 图片等所有静态资源
	Files []DeployFile `json:"files,omitempty"`

	// 短码
	EnableCustomCode bool   `json:"enableCustomCode,omitempty"`
	CustomCode       string `json:"customCode,omitempty"`

	// 版本控制
	CreateVersion bool `json:"createVersion,omitempty"`

	// Source 标记本次部署来源，仅用于审计。默认 "api"。
	Source string `json:"source,omitempty"`

	// accessPassword 设置站点访问密码。仅注册用户 / 绑定 Agent 可用，匿名部署不允许。
	AccessPassword string `json:"accessPassword,omitempty"`
}

// DeployFile 是多文件部署里的单条文件。
// content 用于文本；contentBase64 用于二进制（图片等）。两者必居其一。
type DeployFile struct {
	Path          string `json:"path"`
	Content       string `json:"content,omitempty"`
	ContentBase64 string `json:"contentBase64,omitempty"`
}

// DeployResponse 是 POST /api/deploy 成功响应。
// 对齐项目 OpenAPI 3.1 DeployResponse 全部字段。
type DeployResponse struct {
	Success bool `json:"success"`

	// id 是 deploy 的稳定 ID（= 首版本 ID，等同 versionId 在新部署时）
	ID string `json:"id"`

	Code string `json:"code"`
	URL  string `json:"url"`

	// detailUrl 是应用本体的访问 URL（/agent/{code} 前缀）
	DetailURL string `json:"detailUrl"`

	// versionUrl 指向具体版本的预览（?v=N）
	VersionURL string `json:"versionUrl"`

	// qrCode 是 base64 data URL（PNG）
	QRCode string `json:"qrCode,omitempty"`

	Description string `json:"description,omitempty"`

	// versionId 是本次新建/覆盖的版本 UUID
	VersionID string `json:"versionId"`

	// versionNumber 是本次部署的版本号
	VersionNumber int `json:"versionNumber"`

	// currentVersionId 是当前对外服务的版本 UUID（部署成功后通常 = versionId）
	CurrentVersionID string `json:"currentVersionId"`

	// preserveHint 仅新建短链时返回。提示 Agent 告诉用户：
	// 用 lock 锁定该版本以防自动清理。
	PreserveHint string `json:"preserveHint,omitempty"`

	// agentGuideUrl 仅新建短链时返回。稳定 API 文档链接。
	AgentGuideURL string `json:"agentGuideUrl,omitempty"`

	// primaryVersionStrategy 该 site 的主 URL 选版本策略
	PrimaryVersionStrategy PrimaryVersionStrategy `json:"primaryVersionStrategy"`

	RequestID string `json:"requestId,omitempty"`

	// cooldownSeconds 本次部署启动的全局冷却秒数
	CooldownSeconds int `json:"cooldownSeconds"`

	// nextAvailableAt 下次可部署时间（RFC3339）
	NextAvailableAt time.Time `json:"nextAvailableAt"`

	// versionCount 部署完成后该 site 累计版本数
	VersionCount int `json:"versionCount"`

	// 兼容字段：旧版 admin / CLI 用的简短形式
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
}

// VersionCreatedResponse 是 PATCH /api/deploy/content（兼容追加）的成功响应。
// 对齐 OpenAPI 中的 VersionCreatedResponse schema。
type VersionCreatedResponse struct {
	Success                bool                   `json:"success"`
	Code                   string                 `json:"code"`
	VersionID              string                 `json:"versionId"`
	VersionNumber          int                    `json:"versionNumber"`
	URL                    string                 `json:"url"`
	DetailURL              string                 `json:"detailUrl"`
	VersionURL             string                 `json:"versionUrl"`
	CurrentVersionID       string                 `json:"currentVersionId"`
	PreserveHint           string                 `json:"preserveHint,omitempty"`
	PrimaryVersionStrategy PrimaryVersionStrategy `json:"primaryVersionStrategy"`
}

// ContentPatchRequest 是 PATCH /api/deploy/content（兼容追加版本）的请求体。
type ContentPatchRequest struct {
	Code        string `json:"code,omitempty"`
	URL         string `json:"url,omitempty"`
	Content     string `json:"content"`
	Description string `json:"description"`
	Title       string `json:"title,omitempty"`
	Filename    string `json:"filename,omitempty"`
}

// PrimaryStrategyRequest 是 PATCH /api/deploys/{code}/primary-strategy 的请求体。
type PrimaryStrategyRequest struct {
	PrimaryVersionStrategy PrimaryVersionStrategy `json:"primaryVersionStrategy"`
}

// PrimaryStrategyResponse 是 GET/PATCH primary-strategy 的响应。
type PrimaryStrategyResponse struct {
	Success                bool                   `json:"success"`
	Code                   string                 `json:"code"`
	PrimaryVersionStrategy PrimaryVersionStrategy `json:"primaryVersionStrategy"`
	PrimaryVersionID       string                 `json:"primaryVersionId"`
	PrimaryVersionNumber   int64                  `json:"primaryVersionNumber"`
	CurrentVersionID       string                 `json:"currentVersionId"`
}

// VersionUpdatedResponse 是 PATCH /api/deploys/{code}/versions/{version} 成功响应。
// 对齐 OpenAPI 中的 VersionUpdatedResponse schema。
type VersionUpdatedResponse struct {
	Success          bool   `json:"success"`
	Code             string `json:"code"`
	ID               string `json:"id"`
	VersionID        string `json:"versionId"`
	VersionNumber    int    `json:"versionNumber"`
	Status           string `json:"status"`
	URL              string `json:"url"`
	DetailURL        string `json:"detailUrl"`
	VersionURL       string `json:"versionUrl"`
	CurrentVersionID string `json:"currentVersionId"`
	FileSize         int64  `json:"fileSize"`
	Description      string `json:"description"`
}

// VersionDeletedResponse 是 DELETE /api/deploys/{code}/versions/{version} 成功响应。
type VersionDeletedResponse struct {
	Success              bool   `json:"success"`
	Code                 string `json:"code"`
	ID                   string `json:"id"`
	DeletedVersionID     string `json:"deletedVersionId"`
	DeletedVersionNumber int    `json:"deletedVersionNumber"`
	CurrentVersionID     string `json:"currentVersionId"`
}
