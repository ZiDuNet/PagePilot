package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	screenWSPongWait   = 70 * time.Second
	screenWSPingPeriod = 25 * time.Second
	screenWSWriteWait  = 8 * time.Second
	screenWSReadLimit  = 64 << 10
	screenWSMaxClients = 4
)

type screenHub struct {
	mu      sync.RWMutex
	clients map[string]map[*screenWSClient]struct{}
}

type screenWSClient struct {
	screenID string
	conn     *websocket.Conn
	send     chan ScreenWSMessage
	baseURL  string
}

func newScreenHub() *screenHub {
	return &screenHub{clients: map[string]map[*screenWSClient]struct{}{}}
}

func (h *screenHub) register(client *screenWSClient) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.screenID] == nil {
		h.clients[client.screenID] = map[*screenWSClient]struct{}{}
	}
	if len(h.clients[client.screenID]) >= screenWSMaxClients {
		return false
	}
	h.clients[client.screenID][client] = struct{}{}
	return true
}

func (h *screenHub) unregister(client *screenWSClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients := h.clients[client.screenID]; clients != nil {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.clients, client.screenID)
		}
	}
}

func (h *screenHub) clientsFor(screenID string) []*screenWSClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := make([]*screenWSClient, 0, len(h.clients[screenID]))
	for client := range h.clients[screenID] {
		clients = append(clients, client)
	}
	return clients
}

func (h *screenHub) pushClient(client *screenWSClient, msg ScreenWSMessage) bool {
	if msg.ScreenID == "" {
		msg.ScreenID = client.screenID
	}
	if msg.ServerTime.IsZero() {
		msg.ServerTime = time.Now().UTC()
	}
	select {
	case client.send <- msg:
		return true
	default:
		h.unregister(client)
		_ = client.conn.Close()
		return false
	}
}

func (h *screenHub) push(screenID string, msg ScreenWSMessage) bool {
	clients := h.clientsFor(screenID)
	if len(clients) == 0 {
		return false
	}
	delivered := false
	for _, client := range clients {
		if h.pushClient(client, msg) {
			delivered = true
		}
	}
	return delivered
}

func (s *Server) handleDeviceWebSocket(w http.ResponseWriter, r *http.Request) {
	screen, authErr := s.authenticateDevice(r)
	if authErr != nil {
		writeError(w, apiErrWithReqID(authErr, requestIDFromContext(r.Context())))
		return
	}
	upgrader := websocket.Upgrader{
		CheckOrigin: screenWSOriginAllowed,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Printf("screen websocket upgrade failed for %s: %v", screen.ID, err)
		return
	}
	client := &screenWSClient{
		screenID: screen.ID,
		conn:     conn,
		send:     make(chan ScreenWSMessage, 8),
		baseURL:  s.requestBaseURL(r),
	}
	if !s.screenHub.register(client) {
		_ = conn.Close()
		return
	}
	defer s.screenHub.unregister(client)

	if err := s.sendScreenWSManifest(r.Context(), screen.ID, r); err != nil {
		s.logger.Printf("screen websocket initial manifest failed for %s: %v", screen.ID, err)
	}

	done := make(chan struct{})
	go client.readLoop(done)
	client.writeLoop(done)
}

func (c *screenWSClient) readLoop(done chan<- struct{}) {
	defer close(done)
	c.conn.SetReadLimit(screenWSReadLimit)
	_ = c.conn.SetReadDeadline(time.Now().Add(screenWSPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(screenWSPongWait))
	})
	for {
		if _, _, err := c.conn.NextReader(); err != nil {
			return
		}
	}
}

// screenWSOriginAllowed accepts native clients (which omit Origin) and
// same-origin browser handshakes. Cross-origin browser requests must not be
// able to reuse a device token, even when a proxy requires query fallback.
func screenWSOriginAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if !strings.EqualFold(u.Scheme, requestScheme(r)) {
		return false
	}
	if strings.EqualFold(u.Host, strings.TrimSpace(r.Host)) {
		return true
	}
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if forwarded != "" {
		forwarded, _, _ = strings.Cut(forwarded, ",")
		return strings.EqualFold(u.Host, strings.TrimSpace(forwarded))
	}
	return false
}

func (c *screenWSClient) writeLoop(done <-chan struct{}) {
	ticker := time.NewTicker(screenWSPingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(screenWSWriteWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(screenWSWriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

func (s *Server) sendScreenWSManifest(ctx context.Context, screenID string, r *http.Request) error {
	screen, err := s.deployer.GetScreen(ctx, screenID)
	if err != nil {
		return err
	}
	clients := s.screenHub.clientsFor(screenID)
	if len(clients) == 0 {
		manifest, apiErr := s.screenManifest(ctx, screen, r)
		if apiErr != nil {
			return errors.New(apiErr.Detail)
		}
		s.screenHub.push(screenID, ScreenWSMessage{
			Type:     "manifest",
			ScreenID: screenID,
			Manifest: &manifest,
		})
		return nil
	}
	for _, client := range clients {
		req := r
		if client.baseURL != "" {
			req = requestWithPublicOrigin(client.baseURL)
		}
		manifest, apiErr := s.screenManifest(ctx, screen, req)
		if apiErr != nil {
			return errors.New(apiErr.Detail)
		}
		s.screenHub.pushClient(client, ScreenWSMessage{
			Type:     "manifest",
			ScreenID: screenID,
			Manifest: &manifest,
		})
	}
	return nil
}

func requestWithPublicOrigin(baseURL string) *http.Request {
	return &http.Request{
		Header: http.Header{"X-Hostctl-Current-Origin": []string{baseURL}},
	}
}

func (s *Server) sendScreenWSScreenshot(screenID string, shot *ScreenScreenshotCommand) {
	if shot == nil {
		return
	}
	s.screenHub.push(screenID, ScreenWSMessage{
		Type:       "screenshot",
		ScreenID:   screenID,
		Screenshot: shot,
	})
}

func (s *Server) sendScreenWSCommand(screenID string, command *ScreenDeviceCommand) {
	if command == nil {
		return
	}
	s.screenHub.push(screenID, ScreenWSMessage{
		Type:     "command",
		ScreenID: screenID,
		Command:  command,
	})
}
