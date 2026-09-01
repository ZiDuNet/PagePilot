package api

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/config"
	"github.com/yourorg/hostctl/internal/store"
)

func TestAnonymousSessionEndpointRateLimitsNewSessions(t *testing.T) {
	stub := &anonymousSessionLimiterStub{sessions: make(map[string]store.AnonymousSession)}
	srv := New(config.Default(), stub, nil, false, log.New(bytes.NewBuffer(nil), "", 0))

	firstReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	firstReq.RemoteAddr = "198.51.100.40:1234"
	firstRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(firstRR, firstReq)
	if firstRR.Code != http.StatusOK {
		t.Fatalf("first session status = %d, body = %s; want 200", firstRR.Code, firstRR.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, cookie := range firstRR.Result().Cookies() {
		if cookie.Name == "hostctl_anon_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("first session response omitted anonymous-session cookie")
	}

	for attempt := 1; attempt < anonymousSessionPerIP; attempt++ {
		req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		req.RemoteAddr = "198.51.100.40:1234"
		rr := httptest.NewRecorder()
		srv.mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("session attempt %d status = %d, body = %s; want 200", attempt+1, rr.Code, rr.Body.String())
		}
	}

	limitedReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	limitedReq.RemoteAddr = "198.51.100.40:1234"
	limitedRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(limitedRR, limitedReq)
	if limitedRR.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited session status = %d, body = %s; want 429", limitedRR.Code, limitedRR.Body.String())
	}
	if limitedRR.Header().Get("Retry-After") == "" {
		t.Fatal("rate-limited session response omitted Retry-After")
	}

	// Refreshing a known session remains available while new-session creation is locked.
	refreshReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	refreshReq.RemoteAddr = "198.51.100.40:1234"
	refreshReq.AddCookie(sessionCookie)
	refreshRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(refreshRR, refreshReq)
	if refreshRR.Code != http.StatusOK {
		t.Fatalf("session refresh status = %d, body = %s; want 200", refreshRR.Code, refreshRR.Body.String())
	}
	if got := len(stub.sessions); got != anonymousSessionPerIP {
		t.Fatalf("created session rows = %d, want %d", got, anonymousSessionPerIP)
	}
	if stub.pruneCalls != 1 {
		t.Fatalf("prune calls = %d, want one call per cleanup interval", stub.pruneCalls)
	}
}

func TestAnonymousSessionLimiterEnforcesGlobalWindow(t *testing.T) {
	srv := &Server{}
	now := time.Now()
	for i := 0; i < anonymousSessionGlobal; i++ {
		if ok, _ := srv.allowAnonymousSessionStart(fmt.Sprintf("198.51.100.%d", i%250+1), now); !ok {
			t.Fatalf("global allowance denied at attempt %d", i+1)
		}
	}
	if ok, retry := srv.allowAnonymousSessionStart("203.0.113.1", now); ok || retry <= 0 {
		t.Fatalf("global limit result = ok %v, retry %s; want denial with retry", ok, retry)
	}
	if ok, _ := srv.allowAnonymousSessionStart("203.0.113.1", now.Add(anonymousSessionWindow)); !ok {
		t.Fatal("global allowance did not reset after window")
	}
}

type anonymousSessionLimiterStub struct {
	DeployerPort
	sessions   map[string]store.AnonymousSession
	pruneCalls int
}

func (s *anonymousSessionLimiterStub) CreateAnonymousSession(_ context.Context, id string) (store.AnonymousSession, error) {
	now := time.Now()
	sess := store.AnonymousSession{ID: id, CreatedAt: now, LastUsedAt: now}
	s.sessions[id] = sess
	return sess, nil
}

func (s *anonymousSessionLimiterStub) GetAnonymousSession(_ context.Context, id string) (store.AnonymousSession, error) {
	sess, ok := s.sessions[id]
	if !ok {
		return store.AnonymousSession{}, store.ErrNotFound
	}
	return sess, nil
}

func (s *anonymousSessionLimiterStub) UpdateAnonymousSessionMeta(_ context.Context, id, agentID, agentLabel, deviceIP, userAgent string) error {
	sess, ok := s.sessions[id]
	if !ok {
		return store.ErrNotFound
	}
	if agentID != "" {
		sess.AgentID = agentID
	}
	if agentLabel != "" {
		sess.AgentLabel = agentLabel
	}
	if deviceIP != "" {
		sess.DeviceIP = deviceIP
	}
	if userAgent != "" {
		sess.UserAgent = userAgent
	}
	sess.LastUsedAt = time.Now()
	s.sessions[id] = sess
	return nil
}

func (s *anonymousSessionLimiterStub) PruneAnonymousSessions(context.Context, time.Time) error {
	s.pruneCalls++
	return nil
}
