package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSQLiteMigrationDropsLegacyBindingsAndUnownedTokens(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostctl.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	now := time.Now().UTC()
	_, err = db.Exec(`
		CREATE TABLE tokens (
			id TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL UNIQUE,
			label TEXT,
			is_admin BOOLEAN NOT NULL DEFAULT 0,
			is_revoked BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			last_used_at DATETIME
		);
		INSERT INTO tokens (id, token_hash, label, is_admin, is_revoked, created_at)
		VALUES ('legacy-token', 'hash-1', 'old', 0, 0, ?);
		CREATE TABLE agent_binding_codes (
			code TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			label TEXT,
			created_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			consumed_at DATETIME,
			consumed_by TEXT
		);
	`, now)
	if err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	defer store.Close()

	var tokenCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM tokens`).Scan(&tokenCount); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if tokenCount != 0 {
		t.Fatalf("token count = %d, want 0", tokenCount)
	}

	var legacyTableCount int
	err = store.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'agent_binding_codes'
	`).Scan(&legacyTableCount)
	if err != nil {
		t.Fatalf("check legacy table: %v", err)
	}
	if legacyTableCount != 0 {
		t.Fatalf("legacy binding table count = %d, want 0", legacyTableCount)
	}
}

func TestClaimAnonymousSessionMigratesSitesAndStats(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.CreateAdminUser(ctx, AdminUser{
		ID:           "user-1",
		Username:     "alice",
		PasswordHash: "hash",
		IsActive:     true,
		DeployLimit:  20,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.CreateAnonymousSession(ctx, AnonymousSession{ID: "anon-1", DeployCount: 2, CreatedAt: now, LastUsedAt: now}); err != nil {
		t.Fatalf("create anonymous session: %v", err)
	}
	if err := store.CreateAnonymousSession(ctx, AnonymousSession{ID: "empty", CreatedAt: now, LastUsedAt: now}); err != nil {
		t.Fatalf("create empty anonymous session: %v", err)
	}
	if err := store.CreateSite(ctx, Site{
		Code:         "demo",
		OwnerTokenID: "anon:anon-1",
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create site: %v", err)
	}

	result, err := store.ClaimAnonymousSession(ctx, "anon-1", "user-1")
	if err != nil {
		t.Fatalf("claim anonymous session: %v", err)
	}
	if result.SiteCount != 1 || result.DeployCount != 2 || result.AlreadyClaimed {
		t.Fatalf("unexpected claim result: %+v", result)
	}

	site, err := store.GetSite(ctx, "demo")
	if err != nil {
		t.Fatalf("get site: %v", err)
	}
	if site.OwnerTokenID != "user:user-1" {
		t.Fatalf("site owner = %q, want user:user-1", site.OwnerTokenID)
	}

	user, err := store.GetAdminUserByID(ctx, "user-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.DeployCount != 2 {
		t.Fatalf("user deploy count = %d, want 2", user.DeployCount)
	}

	again, err := store.ClaimAnonymousSession(ctx, "anon-1", "user-1")
	if err != nil {
		t.Fatalf("claim anonymous session again: %v", err)
	}
	if !again.AlreadyClaimed || again.SiteCount != 0 {
		t.Fatalf("unexpected repeated claim result: %+v", again)
	}
	user, err = store.GetAdminUserByID(ctx, "user-1")
	if err != nil {
		t.Fatalf("get user after repeated claim: %v", err)
	}
	if user.DeployCount != 2 {
		t.Fatalf("user deploy count after repeated claim = %d, want 2", user.DeployCount)
	}

	sessions, err := store.ListAnonymousSessions(ctx, 100)
	if err != nil {
		t.Fatalf("list anonymous sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "anon-1" || sessions[0].ClaimedByUserID != "user-1" {
		t.Fatalf("sessions = %+v, want only claimed anon-1", sessions)
	}
}

func TestClaimAnonymousSessionRejectsNewAnonymousDeploysAfterClaim(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.CreateAdminUser(ctx, AdminUser{
		ID:           "user-1",
		Username:     "alice",
		PasswordHash: "hash",
		IsActive:     true,
		DeployLimit:  20,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := store.CreateAnonymousSession(ctx, AnonymousSession{ID: "anon-1", DeployCount: 1, CreatedAt: now, LastUsedAt: now}); err != nil {
		t.Fatalf("create anonymous session: %v", err)
	}
	if _, err := store.ClaimAnonymousSession(ctx, "anon-1", "user-1"); err != nil {
		t.Fatalf("claim anonymous session: %v", err)
	}

	if _, err := store.IncrementAnonymousSessionDeployCount(ctx, "anon-1"); err == nil {
		t.Fatal("expected claimed anonymous session to reject new anonymous deploys")
	}
}

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "hostctl.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	return store
}
