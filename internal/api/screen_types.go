package api

import "time"

type ScreenItem struct {
	ID              string     `json:"id"`
	OwnerUserID     string     `json:"ownerUserId,omitempty"`
	Name            string     `json:"name"`
	DeviceName      string     `json:"deviceName"`
	Status          string     `json:"status"`
	CurrentSiteCode string     `json:"currentSiteCode,omitempty"`
	CurrentVersion  *int64     `json:"currentVersion,omitempty"`
	LastSeenAt      *time.Time `json:"lastSeenAt,omitempty"`
	AppVersion      string     `json:"appVersion,omitempty"`
	Runtime         string     `json:"runtime,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
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
	DeviceName string `json:"deviceName,omitempty"`
	AppVersion string `json:"appVersion,omitempty"`
	Runtime    string `json:"runtime,omitempty"`
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
	Success     bool                  `json:"success"`
	ScreenID    string                `json:"screenId"`
	Mode        string                `json:"mode"`
	BaseURL     string                `json:"baseUrl"`
	EntryURL    string                `json:"entryUrl,omitempty"`
	SiteCode    string                `json:"siteCode,omitempty"`
	Version     int64                 `json:"version,omitempty"`
	MainEntry   string                `json:"mainEntry,omitempty"`
	Title       string                `json:"title,omitempty"`
	Description string                `json:"description,omitempty"`
	Assets      []ScreenManifestAsset `json:"assets,omitempty"`
	UpdatedAt   time.Time             `json:"updatedAt"`
}

type DeviceHeartbeatRequest struct {
	AppVersion string `json:"appVersion,omitempty"`
	Runtime    string `json:"runtime,omitempty"`
}

type DeviceHeartbeatResponse struct {
	Success bool       `json:"success"`
	Screen  ScreenItem `json:"screen"`
}
