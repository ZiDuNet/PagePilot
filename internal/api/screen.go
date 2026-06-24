package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yourorg/hostctl/internal/auth"
	"github.com/yourorg/hostctl/internal/store"
)

const screenPairingTTL = 5 * time.Minute

func (s *Server) handleListScreens(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	userID, isAdmin, authErr := s.screenActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var (
		screens []store.Screen
		err     error
	)
	if isAdmin {
		screens, err = s.deployer.ListScreens(r.Context())
	} else {
		screens, err = s.deployer.ListScreensByUser(r.Context(), userID)
	}
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screens", err.Error()), reqID))
		return
	}
	items := make([]ScreenItem, 0, len(screens))
	for _, screen := range screens {
		items = append(items, toScreenItem(screen))
	}
	writeJSON(w, http.StatusOK, ScreenListResponse{Success: true, Screens: items})
}

func (s *Server) handleBindScreen(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	userID, _, authErr := s.screenActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req ScreenBindRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	code := strings.TrimSpace(req.PairingCode)
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "pairingCode", "pairingCode is required"), reqID))
		return
	}
	screen, err := s.deployer.BindScreenPairing(r.Context(), code, userID, strings.TrimSpace(req.Name))
	if err != nil {
		statusCode := CodeInvalidInput
		if errors.Is(err, store.ErrNotFound) {
			statusCode = CodeNotFound
		}
		writeError(w, apiErrWithReqID(NewError(statusCode, "screen_pairing", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, ScreenBindResponse{Success: true, Screen: toScreenItem(screen)})
}

func (s *Server) handlePublishScreen(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	userID, isAdmin, authErr := s.screenActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	screenID := strings.TrimSpace(r.PathValue("screenId"))
	if screenID == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "screen", "screen id is required"), reqID))
		return
	}
	var req ScreenPublishRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "code", "code is required"), reqID))
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
	if !isAdmin && site.OwnerTokenID != "user:"+userID {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "site", "you can only publish your own apps to screens"), reqID))
		return
	}
	publishOwnerID := userID
	if isAdmin {
		screen, err := s.deployer.GetScreen(r.Context(), screenID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screen", "screen not found"), reqID))
				return
			}
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen", err.Error()), reqID))
			return
		}
		publishOwnerID = screen.OwnerUserID
	}
	version := req.VersionNumber
	if version == nil {
		version = site.CurrentVersion
	}
	if version == nil || *version <= 0 {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "version", "site has no current version"), reqID))
		return
	}
	if err := s.deployer.PublishScreen(r.Context(), screenID, publishOwnerID, code, version); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screen", "screen not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen", err.Error()), reqID))
		return
	}
	screen, err := s.deployer.GetScreen(r.Context(), screenID)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, ScreenPublishResponse{Success: true, Screen: toScreenItem(screen)})
}

func (s *Server) handleUnbindScreen(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	userID, isAdmin, authErr := s.screenActor(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	screenID := strings.TrimSpace(r.PathValue("screenId"))
	if screenID == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "screen", "screen id is required"), reqID))
		return
	}
	unbindOwnerID := userID
	if isAdmin {
		screen, err := s.deployer.GetScreen(r.Context(), screenID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screen", "screen not found"), reqID))
				return
			}
			writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen", err.Error()), reqID))
			return
		}
		unbindOwnerID = screen.OwnerUserID
	}
	if err := s.deployer.UnbindScreen(r.Context(), screenID, unbindOwnerID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screen", "screen not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, ScreenDeleteResponse{Success: true, ID: screenID})
}

func (s *Server) handleDevicePairingStart(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	var req DevicePairingStartRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	now := time.Now().UTC()
	pairingSecret := randomHex(32)
	deviceName := strings.TrimSpace(req.DeviceName)
	if deviceName == "" {
		deviceName = "未命名屏幕"
	}
	var pairing store.ScreenPairing
	var lastErr error
	for i := 0; i < 5; i++ {
		pairing = store.ScreenPairing{
			ID:                "pair_" + randomHex(12),
			Code:              screenPairingCode(),
			PairingSecretHash: auth.HashToken(pairingSecret),
			ScreenID:          "screen_" + randomHex(12),
			DeviceName:        deviceName,
			ExpiresAt:         now.Add(screenPairingTTL),
			CreatedAt:         now,
		}
		lastErr = s.deployer.CreateScreenPairing(r.Context(), pairing)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen_pairing", lastErr.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, DevicePairingStartResponse{
		Success:       true,
		ScreenID:      pairing.ScreenID,
		PairingID:     pairing.ID,
		PairingCode:   pairing.Code,
		PairingSecret: pairingSecret,
		ExpiresAt:     pairing.ExpiresAt,
		ServerTime:    now,
	})
}

func (s *Server) screenActor(r *http.Request) (string, bool, *APIError) {
	if user, ok := s.adminUserFromCookie(r); ok {
		if !user.IsActive {
			return "", false, NewError(CodeForbidden, "auth", "user is inactive")
		}
		return user.ID, user.IsAdmin, nil
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		return "", false, NewError(CodeUnauthorized, "auth", "registered user token required")
	}
	tok, authErr := s.authenticateToken(r)
	if authErr != nil {
		return "", false, authErr
	}
	if strings.TrimSpace(tok.OwnerUserID) == "" {
		return "", false, NewError(CodeUnauthorized, "auth", "registered user token required")
	}
	user, err := s.auth.GetUser(r.Context(), tok.OwnerUserID)
	if err != nil || !user.IsActive {
		return "", false, NewError(CodeForbidden, "user", "token owner is inactive or missing")
	}
	return user.ID, tok.IsAdmin || user.IsAdmin, nil
}

func (s *Server) handleDevicePairingComplete(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	var req DevicePairingCompleteRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	pairingID := strings.TrimSpace(req.PairingID)
	pairingSecret := strings.TrimSpace(req.PairingSecret)
	if pairingID == "" || pairingSecret == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "pairing", "pairingId and pairingSecret are required"), reqID))
		return
	}
	deviceToken := randomHex(32)
	if err := s.deployer.CompleteScreenPairing(r.Context(), pairingID, auth.HashToken(pairingSecret), auth.HashToken(deviceToken)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusAccepted, DevicePairingCompleteResponse{Success: true, Paired: false})
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen_pairing", err.Error()), reqID))
		return
	}
	screen, err := s.deployer.GetScreenByDeviceTokenHash(r.Context(), auth.HashToken(deviceToken))
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen", err.Error()), reqID))
		return
	}
	item := toScreenItem(screen)
	writeJSON(w, http.StatusOK, DevicePairingCompleteResponse{
		Success:     true,
		Paired:      true,
		DeviceToken: deviceToken,
		Screen:      &item,
	})
}

func (s *Server) handleDeviceManifest(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	screen, authErr := s.authenticateDevice(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	resp, apiErr := s.screenManifest(r.Context(), screen)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeviceHeartbeat(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	screen, authErr := s.authenticateDevice(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req DeviceHeartbeatRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	updated, err := s.deployer.TouchScreenHeartbeat(r.Context(), screen.ID, strings.TrimSpace(req.AppVersion), strings.TrimSpace(req.Runtime))
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "heartbeat", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, DeviceHeartbeatResponse{Success: true, Screen: toScreenItem(updated)})
}

func (s *Server) authenticateDevice(r *http.Request) (store.Screen, *APIError) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Device "
	if !strings.HasPrefix(header, prefix) {
		return store.Screen{}, NewError(CodeUnauthorized, "device", "device token required")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return store.Screen{}, NewError(CodeUnauthorized, "device", "device token required")
	}
	screen, err := s.deployer.GetScreenByDeviceTokenHash(r.Context(), auth.HashToken(token))
	if err != nil {
		return store.Screen{}, NewError(CodeUnauthorized, "device", "invalid device token")
	}
	return screen, nil
}

func (s *Server) screenManifest(ctx context.Context, screen store.Screen) (ScreenManifestResponse, *APIError) {
	now := time.Now().UTC()
	base := strings.TrimRight(s.deployer.PublicBaseURL(), "/")
	resp := ScreenManifestResponse{
		Success:   true,
		ScreenID:  screen.ID,
		Mode:      "idle",
		BaseURL:   base,
		UpdatedAt: now,
	}
	if strings.TrimSpace(screen.CurrentSiteCode) == "" {
		return resp, nil
	}
	content, apiErr := s.deployer.GetContent(ctx, screen.CurrentSiteCode, screen.CurrentVersion)
	if apiErr != nil {
		return ScreenManifestResponse{}, apiErr
	}
	entryURL := fmt.Sprintf("%s/agent/%s/", base, screen.CurrentSiteCode)
	if screen.CurrentVersion != nil && *screen.CurrentVersion > 0 {
		entryURL = fmt.Sprintf("%s/agent/%s/versions/%d/", base, screen.CurrentSiteCode, *screen.CurrentVersion)
	}
	resp.Mode = "webapp"
	resp.EntryURL = entryURL
	resp.SiteCode = screen.CurrentSiteCode
	resp.Version = content.Version
	resp.MainEntry = content.MainEntry
	resp.Title = content.Title
	resp.Description = content.Description
	resp.UpdatedAt = screen.UpdatedAt
	resp.Assets = make([]ScreenManifestAsset, 0, len(content.Files))
	for _, file := range content.Files {
		resp.Assets = append(resp.Assets, ScreenManifestAsset{
			Path:   file.Path,
			URL:    fmt.Sprintf("%s/agent/%s/versions/%d/%s", base, screen.CurrentSiteCode, content.Version, escapeManifestAssetPath(file.Path)),
			Size:   file.Size,
			Sha256: file.Sha256,
		})
	}
	return resp, nil
}

func toScreenItem(screen store.Screen) ScreenItem {
	return ScreenItem{
		ID:              screen.ID,
		OwnerUserID:     screen.OwnerUserID,
		Name:            screen.Name,
		DeviceName:      screen.DeviceName,
		Status:          screen.Status,
		CurrentSiteCode: screen.CurrentSiteCode,
		CurrentVersion:  screen.CurrentVersion,
		LastSeenAt:      screen.LastSeenAt,
		AppVersion:      screen.AppVersion,
		Runtime:         screen.Runtime,
		CreatedAt:       screen.CreatedAt,
		UpdatedAt:       screen.UpdatedAt,
	}
}

func screenPairingCode() string {
	return fmt.Sprintf("%06d", randomIntRange(0, 999999))
}

func escapeManifestAssetPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
