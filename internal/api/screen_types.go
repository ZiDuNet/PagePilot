package api

import (
	"encoding/json"
	"time"
)

type ScreenItem struct {
	ID                    string          `json:"id"`
	OwnerUserID           string          `json:"ownerUserId,omitempty"`
	Name                  string          `json:"name"`
	DeviceName            string          `json:"deviceName"`
	Status                string          `json:"status"`
	CurrentSiteCode       string          `json:"currentSiteCode,omitempty"`
	CurrentVersion        *int64          `json:"currentVersion,omitempty"`
	LastSeenAt            *time.Time      `json:"lastSeenAt,omitempty"`
	AppVersion            string          `json:"appVersion,omitempty"`
	Runtime               string          `json:"runtime,omitempty"`
	DeviceInfo            json.RawMessage `json:"deviceInfo,omitempty"`
	ScreenshotRequestedAt *time.Time      `json:"screenshotRequestedAt,omitempty"`
	ScreenshotAt          *time.Time      `json:"screenshotAt,omitempty"`
	CommandType           string          `json:"commandType,omitempty"`
	CommandRequestedAt    *time.Time      `json:"commandRequestedAt,omitempty"`
	CommandCompletedAt    *time.Time      `json:"commandCompletedAt,omitempty"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
	OwnerUsername         string          `json:"ownerUsername,omitempty"`
}

type ScreenListResponse struct {
	Success bool         `json:"success"`
	Screens []ScreenItem `json:"screens"`
}

type ScreenBindRequest struct {
	PairingCode string `json:"pairingCode"`
	Name        string `json:"name,omitempty"`
}

type ScreenBindResponse struct {
	Success bool       `json:"success"`
	Screen  ScreenItem `json:"screen"`
}

type ScreenAssignRequest struct {
	OwnerUserID string `json:"ownerUserId"`
	Name        string `json:"name,omitempty"`
}

type ScreenAssignResponse struct {
	Success bool       `json:"success"`
	Screen  ScreenItem `json:"screen"`
}

type ScreenPublishRequest struct {
	Code          string `json:"code"`
	VersionNumber *int64 `json:"versionNumber,omitempty"`
}

type ScreenPublishResponse struct {
	Success bool       `json:"success"`
	Screen  ScreenItem `json:"screen"`
}

type ScreenDeleteResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
}

type DevicePairingStartRequest struct {
	DeviceName string          `json:"deviceName,omitempty"`
	AppVersion string          `json:"appVersion,omitempty"`
	Runtime    string          `json:"runtime,omitempty"`
	DeviceInfo json.RawMessage `json:"deviceInfo,omitempty"`
}

type DevicePairingStartResponse struct {
	Success       bool      `json:"success"`
	ScreenID      string    `json:"screenId"`
	PairingID     string    `json:"pairingId"`
	PairingCode   string    `json:"pairingCode"`
	PairingSecret string    `json:"pairingSecret"`
	ExpiresAt     time.Time `json:"expiresAt"`
	ServerTime    time.Time `json:"serverTime"`
}

type DevicePairingCompleteRequest struct {
	PairingID     string `json:"pairingId"`
	PairingSecret string `json:"pairingSecret"`
}

type DevicePairingCompleteResponse struct {
	Success     bool        `json:"success"`
	Paired      bool        `json:"paired"`
	DeviceToken string      `json:"deviceToken,omitempty"`
	Screen      *ScreenItem `json:"screen,omitempty"`
}

type ScreenManifestAsset struct {
	Path   string `json:"path"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	Sha256 string `json:"sha256"`
}

type ScreenManifestResponse struct {
	Success       bool                     `json:"success"`
	ScreenID      string                   `json:"screenId"`
	ScreenName    string                   `json:"screenName,omitempty"`
	OwnerUserID   string                   `json:"ownerUserId,omitempty"`
	OwnerUsername string                   `json:"ownerUsername,omitempty"`
	Mode          string                   `json:"mode"`
	BaseURL       string                   `json:"baseUrl"`
	EntryURL      string                   `json:"entryUrl,omitempty"`
	SiteCode      string                   `json:"siteCode,omitempty"`
	Version       int64                    `json:"version,omitempty"`
	MainEntry     string                   `json:"mainEntry,omitempty"`
	Title         string                   `json:"title,omitempty"`
	Description   string                   `json:"description,omitempty"`
	Assets        []ScreenManifestAsset    `json:"assets,omitempty"`
	AccessCookie  *ScreenAccessCookie      `json:"accessCookie,omitempty"`
	Screenshot    *ScreenScreenshotCommand `json:"screenshot,omitempty"`
	Command       *ScreenDeviceCommand     `json:"command,omitempty"`
	UpdatedAt     time.Time                `json:"updatedAt"`
}

type DeviceHeartbeatRequest struct {
	AppVersion string          `json:"appVersion,omitempty"`
	Runtime    string          `json:"runtime,omitempty"`
	DeviceInfo json.RawMessage `json:"deviceInfo,omitempty"`
}

type DeviceHeartbeatResponse struct {
	Success bool       `json:"success"`
	Screen  ScreenItem `json:"screen"`
}

type DeviceScreenshotRequest struct {
	ContentBase64 string `json:"contentBase64"`
	MimeType      string `json:"mimeType,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	RequestID     string `json:"requestId"`
}

type DeviceScreenshotResponse struct {
	Success   bool      `json:"success"`
	ScreenID  string    `json:"screenId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ScreenAccessCookie struct {
	Name          string    `json:"name"`
	Value         string    `json:"value"`
	Path          string    `json:"path"`
	MaxAgeSeconds int       `json:"maxAgeSeconds"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type ScreenScreenshotCommand struct {
	RequestID   string    `json:"requestId"`
	RequestedAt time.Time `json:"requestedAt"`
}

type ScreenScreenshotResponse struct {
	Success    bool                     `json:"success"`
	Screen     ScreenItem               `json:"screen"`
	Screenshot *ScreenScreenshotCommand `json:"screenshot,omitempty"`
}

type ScreenCommandRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type ScreenDeviceCommand struct {
	RequestID   string          `json:"requestId"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	RequestedAt time.Time       `json:"requestedAt"`
}

type ScreenCommandResponse struct {
	Success bool                 `json:"success"`
	Screen  ScreenItem           `json:"screen"`
	Command *ScreenDeviceCommand `json:"command,omitempty"`
}

type ScreenWSMessage struct {
	Type       string                   `json:"type"`
	ScreenID   string                   `json:"screenId,omitempty"`
	ServerTime time.Time                `json:"serverTime,omitempty"`
	Manifest   *ScreenManifestResponse  `json:"manifest,omitempty"`
	Screenshot *ScreenScreenshotCommand `json:"screenshot,omitempty"`
	Command    *ScreenDeviceCommand     `json:"command,omitempty"`
}

type DeviceCommandAckRequest struct {
	RequestID string `json:"requestId"`
	Type      string `json:"type,omitempty"`
}

type DeviceCommandAckResponse struct {
	Success     bool      `json:"success"`
	ScreenID    string    `json:"screenId"`
	CompletedAt time.Time `json:"completedAt"`
}
