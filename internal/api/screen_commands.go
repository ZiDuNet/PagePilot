package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

var supportedScreenCommands = map[string]string{
	"refresh":  "刷新 WebView 内容",
	"sleep":    "进入黑屏待机",
	"wake":     "唤醒并恢复播放",
	"shutdown": "软关机或黑屏待机",
}

func (s *Server) handleRequestScreenCommand(w http.ResponseWriter, r *http.Request) {
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
	var req ScreenCommandRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	commandType := normalizeScreenCommandType(req.Type)
	if commandType == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "type", "type must be one of refresh, sleep, wake, shutdown"), reqID))
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
		writeError(w, apiErrWithReqID(NewError(CodeForbidden, "screen", "you can only control your own screens"), reqID))
		return
	}
	commandID := "cmd_" + randomHex(12)
	updated, err := s.deployer.RequestScreenCommand(r.Context(), screenID, commandID, commandType, normalizeScreenCommandPayload(req.Payload))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeNotFound, "screen", "screen not found"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen_command", err.Error()), reqID))
		return
	}
	resp := ScreenCommandResponse{Success: true, Screen: s.toScreenItem(r.Context(), updated)}
	if updated.CommandRequestedAt != nil {
		resp.Command = &ScreenDeviceCommand{
			RequestID:   commandID,
			Type:        commandType,
			Payload:     rawDeviceInfo(updated.CommandPayload),
			RequestedAt: *updated.CommandRequestedAt,
		}
	}
	s.sendScreenWSCommand(screenID, resp.Command)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeviceCommandAck(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	screen, authErr := s.authenticateDevice(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, reqID))
		return
	}
	var req DeviceCommandAckRequest
	if err := decodeJSONBody(w, r, &req, reqID); err != nil {
		return
	}
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		writeError(w, apiErrWithReqID(NewError(CodeInvalidInput, "requestId", "requestId is required"), reqID))
		return
	}
	if strings.TrimSpace(screen.CommandRequestID) == "" {
		writeError(w, apiErrWithReqID(NewError(CodeConflict, "command", "no pending screen command"), reqID))
		return
	}
	if screen.CommandRequestID != requestID {
		writeError(w, apiErrWithReqID(NewError(CodeConflict, "command", "stale screen command"), reqID))
		return
	}
	if req.Type != "" && normalizeScreenCommandType(req.Type) != screen.CommandType {
		writeError(w, apiErrWithReqID(NewError(CodeConflict, "command", "screen command type mismatch"), reqID))
		return
	}
	completedAt := time.Now().UTC()
	if _, err := s.deployer.CompleteScreenCommand(r.Context(), screen.ID, requestID, completedAt); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apiErrWithReqID(NewError(CodeConflict, "command", "screen command is no longer pending"), reqID))
			return
		}
		writeError(w, apiErrWithReqID(NewError(CodeInternal, "screen_command", err.Error()), reqID))
		return
	}
	writeJSON(w, http.StatusOK, DeviceCommandAckResponse{
		Success:     true,
		ScreenID:    screen.ID,
		CompletedAt: completedAt,
	})
}

func normalizeScreenCommandType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := supportedScreenCommands[value]; !ok {
		return ""
	}
	return value
}

func normalizeScreenCommandPayload(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "{}"
	}
	if len(raw) > 4096 || !json.Valid(raw) {
		return "{}"
	}
	return string(raw)
}
