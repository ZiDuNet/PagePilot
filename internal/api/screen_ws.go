package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	screenWSPongWait   = 70 * time.Second
	screenWSPingPeriod = 25 * time.Second
	screenWSWriteWait  = 8 * time.Second
)

type screenHub struct {
	mu      sync.RWMutex
	clients map[string]map[*screenWSClient]struct{}
}

type screenWSClient struct {
	screenID string
	conn     *websocket.Conn
	send     chan ScreenWSMessage
}

func newScreenHub() *screenHub {
	return &screenHub{clients: map[string]map[*screenWSClient]struct{}{}}
}

func (h *screenHub) register(client *screenWSClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.screenID] == nil {
		h.clients[client.screenID] = map[*screenWSClient]struct{}{}
	}
	h.clients[client.screenID][client] = struct{}{}
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

func (h *screenHub) push(screenID string, msg ScreenWSMessage) bool {
	h.mu.RLock()
	clients := make([]*screenWSClient, 0, len(h.clients[screenID]))
	for client := range h.clients[screenID] {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	if len(clients) == 0 {
		return false
	}
	if msg.ScreenID == "" {
		msg.ScreenID = screenID
	}
	if msg.ServerTime.IsZero() {
		msg.ServerTime = time.Now().UTC()
	}
	delivered := false
	for _, client := range clients {
		select {
		case client.send <- msg:
			delivered = true
		default:
			h.unregister(client)
			_ = client.conn.Close()
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
		CheckOrigin: func(*http.Request) bool { return true },
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
	}
	s.screenHub.register(client)
	defer s.screenHub.unregister(client)

	if err := s.sendScreenWSManifest(r.Context(), screen.ID); err != nil {
		s.logger.Printf("screen websocket initial manifest failed for %s: %v", screen.ID, err)
	}

	done := make(chan struct{})
	go client.readLoop(done)
	client.writeLoop(done)
}

func (c *screenWSClient) readLoop(done chan<- struct{}) {
	defer close(done)
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

func (s *Server) sendScreenWSManifest(ctx context.Context, screenID string) error {
	screen, err := s.deployer.GetScreen(ctx, screenID)
	if err != nil {
		return err
	}
	manifest, apiErr := s.screenManifest(ctx, screen, nil)
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
