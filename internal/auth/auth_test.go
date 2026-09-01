package auth

import (
	"context"
	"testing"
	"time"

	"github.com/yourorg/hostctl/internal/store"
)

func TestPasswordMeetsStrength(t *testing.T) {
	tests := []struct {
		password string
		want     bool
	}{
		{password: "", want: false},
		{password: "        ", want: false},
		{password: "short1", want: false},
		{password: "onlyletters", want: false},
		{password: "12345678", want: false},
		{password: "password1", want: true},
		{password: " password1 ", want: true},
	}
	for _, tt := range tests {
		if got := PasswordMeetsStrength(tt.password); got != tt.want {
			t.Errorf("PasswordMeetsStrength(%q) = %v, want %v", tt.password, got, tt.want)
		}
	}
}

func TestAccountPasswordOperationsRejectWeakPasswords(t *testing.T) {
	svc := New(&memoryStore{})
	for _, password := range []string{"", "short1", "onlyletters", "12345678", "   "} {
		if _, err := svc.CreateFirstAdmin(context.Background(), "admin", password); err != ErrInvalid {
			t.Fatalf("CreateFirstAdmin(%q) error = %v; want ErrInvalid", password, err)
		}
		if _, err := svc.CreateUser(context.Background(), "user", password, false, 1); err != ErrInvalid {
			t.Fatalf("CreateUser(%q) error = %v; want ErrInvalid", password, err)
		}
		if password != "" {
			if err := svc.EnsureBootstrapAdmin(context.Background(), "admin", password); err != ErrInvalid {
				t.Fatalf("EnsureBootstrapAdmin(%q) error = %v; want ErrInvalid", password, err)
			}
		}
	}
}

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

func TestVerifyTokenRejectsInactiveOwner(t *testing.T) {
	svc, plaintext := newTestAuthServiceWithUser(t, store.Token{OwnerUserID: "user-1"}, store.AdminUser{ID: "user-1", IsActive: false})
	if _, err := svc.VerifyToken(context.Background(), "Bearer "+plaintext); err != ErrRevoked {
		t.Fatalf("VerifyToken error = %v; want ErrRevoked", err)
	}
}

func TestVerifyTokenRejectsDemotedAdminToken(t *testing.T) {
	svc, plaintext := newTestAuthServiceWithUser(t, store.Token{OwnerUserID: "user-1", IsAdmin: true}, store.AdminUser{ID: "user-1", IsActive: true, IsAdmin: false})
	if _, err := svc.VerifyToken(context.Background(), "Bearer "+plaintext); err != ErrRevoked {
		t.Fatalf("VerifyToken error = %v; want ErrRevoked", err)
	}
}

func TestGenerateRejectsInactiveOwner(t *testing.T) {
	svc, _ := newTestAuthServiceWithUser(t, store.Token{OwnerUserID: "user-1"}, store.AdminUser{ID: "user-1", IsActive: false})
	if _, err := svc.Generate(context.Background(), "inactive", false, "user-1", nil); err != ErrInvalid {
		t.Fatalf("Generate error = %v; want ErrInvalid", err)
	}
}

func TestGenerateRejectsAdminTokenForNonAdminOwner(t *testing.T) {
	svc, _ := newTestAuthServiceWithUser(t, store.Token{OwnerUserID: "user-1"}, store.AdminUser{ID: "user-1", IsActive: true, IsAdmin: false})
	if _, err := svc.Generate(context.Background(), "admin", true, "user-1", nil); err != ErrInvalid {
		t.Fatalf("Generate error = %v; want ErrInvalid", err)
	}
}

func newTestAuthService(t *testing.T, tok store.Token) (*Service, string) {
	return newTestAuthServiceWithUser(t, tok, store.AdminUser{ID: tok.OwnerUserID, IsActive: true, IsAdmin: tok.IsAdmin})
}

func newTestAuthServiceWithUser(t *testing.T, tok store.Token, user store.AdminUser) (*Service, string) {
	t.Helper()
	plaintext := "plain-token"
	tok.ID = "tok-1"
	tok.TokenHash = HashToken(plaintext)
	tok.CreatedAt = time.Now()
	s := &memoryStore{token: tok, user: user}
	return New(s), plaintext
}

type memoryStore struct {
	store.Store
	token store.Token
	user  store.AdminUser
}

func (m *memoryStore) GetTokenByHash(context.Context, string) (store.Token, error) {
	return m.token, nil
}

func (m *memoryStore) TouchTokenLastUsed(context.Context, string) error {
	return nil
}

func (m *memoryStore) GetAdminUserByID(context.Context, string) (store.AdminUser, error) {
	if m.user.ID == "" {
		return store.AdminUser{}, store.ErrNotFound
	}
	return m.user, nil
}
