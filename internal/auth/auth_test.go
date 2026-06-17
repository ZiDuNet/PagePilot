package auth

import (
	"context"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

func TestVerifyTokenRequiresOwner(t *testing.T) {
	svc, plaintext := newTestAuthService(t, store.Token{OwnerUserID: ""})

	_, err := svc.VerifyToken(context.Background(), "Bearer "+plaintext)
	if err == nil {
		t.Fatal("expected unowned token to be rejected")
	}
}

func TestVerifyTokenRejectsExpiredToken(t *testing.T) {
	expired := time.Now().Add(-time.Minute)
	svc, plaintext := newTestAuthService(t, store.Token{
		OwnerUserID: "user-1",
		ExpiresAt:   &expired,
	})

	_, err := svc.VerifyToken(context.Background(), "Bearer "+plaintext)
	if err != ErrExpired {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestVerifyTokenAcceptsPermanentUserToken(t *testing.T) {
	svc, plaintext := newTestAuthService(t, store.Token{OwnerUserID: "user-1"})

	tok, err := svc.VerifyToken(context.Background(), "Bearer "+plaintext)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if tok.OwnerUserID != "user-1" {
		t.Fatalf("owner = %q, want user-1", tok.OwnerUserID)
	}
}

func newTestAuthService(t *testing.T, tok store.Token) (*Service, string) {
	t.Helper()
	plaintext := "plain-token"
	tok.ID = "tok-1"
	tok.TokenHash = HashToken(plaintext)
	tok.CreatedAt = time.Now()
	s := &memoryStore{token: tok}
	return New(s), plaintext
}

type memoryStore struct {
	store.Store
	token store.Token
}

func (m *memoryStore) GetTokenByHash(context.Context, string) (store.Token, error) {
	return m.token, nil
}

func (m *memoryStore) TouchTokenLastUsed(context.Context, string) error {
	return nil
}
