package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

const screenScreenshotMaxBytes = 3 << 20

type screenScreenshotMeta struct {
	ScreenID  string    `json:"screenId"`
	MimeType  string    `json:"mimeType"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *Server) handleDeviceScreenshot(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	screen, authErr := s.authenticateDevice(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req DeviceScreenshotRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "requestId", "requestId is required"), reqID))
		return
	}
	if strings.TrimSpace(screen.ScreenshotRequestID) == "" {
		writeError(w, apiErrWithReqID(NewError(CodeConflict, "screenshot", "no pending screenshot request"), reqID))
		return
	}
	if screen.ScreenshotRequestID != requestID {
		writeError(w, apiErrWithReqID(NewError(CodeConflict, "screenshot", "stale screenshot request"), reqID))
		return
	}
	data, mimeType, apiErr := parseScreenshotPayload(req)
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	meta, err := s.storeScreenScreenshot(screen.ID, data, mimeType, req.Width, req.Height)
	if err != nil {
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screenshot", err.Error()), reqID))
		return
	}
	if _, err := s.deployer.CompleteScreenScreenshot(r.Context(), screen.ID, requestID, meta.UpdatedAt); err != nil {
		s.removeScreenScreenshot(screen.ID)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeConflict, "screenshot", "screenshot request is no longer pending"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screenshot", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, DeviceScreenshotResponse{
		Success:   true,
		ScreenID:  screen.ID,
		UpdatedAt: meta.UpdatedAt,
	})
}

func (s *Server) handleRequestScreenScreenshot(w http.ResponseWriter, r *http.Request) {
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
	screen, err := s.deployer.GetScreen(r.Context(), screenID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screen", "screen not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen", err.Error()), reqID))
		return
	}
	if !isAdmin && screen.OwnerUserID != userID {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "screen", "you can only request screenshots for your own screens"), reqID))
		return
	}
	requestID := "shot_" + randomHex(12)
	updated, err := s.deployer.RequestScreenScreenshot(r.Context(), screenID, requestID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screen", "screen not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screenshot", err.Error()), reqID))
		return
	}
	item := s.toScreenItem(r.Context(), updated)
	resp := ScreenScreenshotResponse{Success: true, Screen: item}
	if updated.ScreenshotRequestedAt != nil {
		resp.Screenshot = &ScreenScreenshotCommand{
			RequestID:   requestID,
			RequestedAt: *updated.ScreenshotRequestedAt,
		}
	}
	s.sendScreenWSScreenshot(screenID, resp.Screenshot)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetScreenScreenshot(w http.ResponseWriter, r *http.Request) {
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
	screen, err := s.deployer.GetScreen(r.Context(), screenID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screen", "screen not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen", err.Error()), reqID))
		return
	}
	if !isAdmin && screen.OwnerUserID != userID {
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "screen", "you can only view screenshots of your own screens"), reqID))
		return
	}
	meta, path, err := s.loadScreenScreenshot(screenID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screenshot", "screenshot not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screenshot", err.Error()), reqID))
		return
	}
	after, hasAfter, apiErr := parseScreenshotAfter(r.URL.Query().Get("after"))
	if apiErr != nil {
		writeError(w, apiErrWithReqID(apiErr, reqID))
		return
	}
	if hasAfter && !meta.UpdatedAt.After(after) {
		writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screenshot", "screenshot not ready"), reqID))
		return
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screenshot", "screenshot not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screenshot", err.Error()), reqID))
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", meta.MimeType)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(path), meta.UpdatedAt, f)
}

func parseScreenshotAfter(raw string) (time.Time, bool, *APIError) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	after, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, true, NewError(CodeInvalidInput, "after", "after must be an RFC3339 timestamp")
	}
	return after.UTC(), true, nil
}

func (s *Server) loadScreenScreenshot(screenID string) (screenScreenshotMeta, string, error) {
	root := s.screenScreenshotDir()
	stem := screenScreenshotStem(screenID)
	metaPath := filepath.Join(root, stem+".json")
	metaBytes, err := os.ReadFile(metaPath)
	if err == nil {
		var meta screenScreenshotMeta
		if jsonErr := json.Unmarshal(metaBytes, &meta); jsonErr != nil {
			return screenScreenshotMeta{}, "", jsonErr
		}
		path := filepath.Join(root, stem+screenScreenshotExt(meta.MimeType))
		if meta.UpdatedAt.IsZero() {
			if info, statErr := os.Stat(path); statErr == nil {
				meta.UpdatedAt = info.ModTime()
				meta.Size = info.Size()
			}
		}
		if meta.MimeType == "" {
			meta.MimeType = "image/jpeg"
		}
		return meta, path, nil
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		path := filepath.Join(root, stem+ext)
		info, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}
		meta := screenScreenshotMeta{
			ScreenID:  screenID,
			MimeType:  mimeFromExt(ext),
			Size:      info.Size(),
			UpdatedAt: info.ModTime(),
		}
		return meta, path, nil
	}
	return screenScreenshotMeta{}, "", fs.ErrNotExist
}

func (s *Server) storeScreenScreenshot(screenID string, data []byte, mimeType string, width, height int) (screenScreenshotMeta, error) {
	root := s.screenScreenshotDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return screenScreenshotMeta{}, err
	}
	stem := screenScreenshotStem(screenID)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		_ = os.Remove(filepath.Join(root, stem+ext))
	}
	mimeType = normalizeScreenshotMime(mimeType)
	if mimeType == "" {
		return screenScreenshotMeta{}, NewError(CodeInvalidInput, "mimeType", "mimeType must be image/jpeg, image/png, or image/webp")
	}
	imagePath := filepath.Join(root, stem+screenScreenshotExt(mimeType))
	meta := screenScreenshotMeta{
		ScreenID:  screenID,
		MimeType:  mimeType,
		Width:     width,
		Height:    height,
		Size:      int64(len(data)),
		UpdatedAt: time.Now().UTC(),
	}
	if err := writeAtomicFile(imagePath, data, 0o644); err != nil {
		return screenScreenshotMeta{}, err
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return screenScreenshotMeta{}, err
	}
	if err := writeAtomicFile(filepath.Join(root, stem+".json"), metaBytes, 0o644); err != nil {
		return screenScreenshotMeta{}, err
	}
	return meta, nil
}

func (s *Server) screenScreenshotDir() string {
	return filepath.Join(filepath.Dir(s.cfg.DBPath), "screenshots")
}

func (s *Server) screenScreenshotAt(screenID string) *time.Time {
	meta, _, err := s.loadScreenScreenshot(screenID)
	if err != nil {
		return nil
	}
	t := meta.UpdatedAt
	return &t
}

func (s *Server) removeScreenScreenshot(screenID string) {
	root := s.screenScreenshotDir()
	stem := screenScreenshotStem(screenID)
	_ = os.Remove(filepath.Join(root, stem+".json"))
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		_ = os.Remove(filepath.Join(root, stem+ext))
	}
}

func screenScreenshotStem(screenID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(screenID)))
	return hex.EncodeToString(sum[:])
}

func screenScreenshotExt(mimeType string) string {
	switch normalizeScreenshotMime(mimeType) {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func normalizeScreenshotMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "", "image/jpg", "image/jpeg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	case "image/webp":
		return "image/webp"
	default:
		return ""
	}
}

func parseScreenshotPayload(req DeviceScreenshotRequest) ([]byte, string, *APIError) {
	raw := strings.TrimSpace(req.ContentBase64)
	if raw == "" {
		return nil, "", NewError(CodeInvalidInput, "contentBase64", "contentBase64 is required")
	}
	if idx := strings.Index(raw, ","); idx >= 0 && strings.Contains(strings.ToLower(raw[:idx]), "base64") {
		raw = raw[idx+1:]
	}
	raw = strings.ReplaceAll(raw, "\n", "")
	raw = strings.ReplaceAll(raw, "\r", "")
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, "", NewError(CodeInvalidInput, "contentBase64", "contentBase64 must be valid base64")
	}
	if len(data) == 0 {
		return nil, "", NewError(CodeInvalidInput, "contentBase64", "screenshot payload is empty")
	}
	if len(data) > screenScreenshotMaxBytes {
		return nil, "", NewError(CodeContentTooLarge, "contentBase64",
			fmt.Sprintf("screenshot exceeds max size (%d bytes)", screenScreenshotMaxBytes))
	}
	mimeType := normalizeScreenshotMime(req.MimeType)
	if mimeType == "" {
		detected := http.DetectContentType(data)
		if mimeType = normalizeScreenshotMime(detected); mimeType == "" {
			mimeType = "image/jpeg"
		}
	}
	return data, mimeType, nil
}

func writeAtomicFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if _, err := io.Copy(tmp, bytes.NewReader(data)); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
