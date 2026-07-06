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

func TestSQLiteMigrationAddsAdminEmailColumnsBeforeIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostctl.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	now := time.Now().UTC()
	_, err = db.Exec(`
		CREATE TABLE admin_users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			last_login_at DATETIME
		);
		INSERT INTO admin_users (id, username, password_hash, is_active, created_at)
		VALUES ('admin-1', 'admin', 'hash', 1, ?);
	`, now)
	if err != nil {
		t.Fatalf("seed legacy admin users: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	defer store.Close()

	var email string
	var verified bool
	if err := store.db.QueryRow(`SELECT email, email_verified FROM admin_users WHERE id = 'admin-1'`).Scan(&email, &verified); err != nil {
		t.Fatalf("query migrated email columns: %v", err)
	}
	if email != "" || verified {
		t.Fatalf("email = %q verified = %v, want empty/false", email, verified)
	}
	var indexCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_admin_users_email'`).Scan(&indexCount); err != nil {
		t.Fatalf("query email index: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("idx_admin_users_email count = %d, want 1", indexCount)
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

func TestMarketplacePinnedDeploysStayAboveLikeRanking(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-3 * time.Hour)

	seedMarketplaceSite(t, store, "popular", 50, base)
	seedMarketplaceSite(t, store, "pinned-high", 30, base.Add(time.Hour))
	seedMarketplaceSite(t, store, "pinned-low", 1, base.Add(90*time.Minute))
	seedMarketplaceSite(t, store, "fresh", 10, base.Add(2*time.Hour))

	if err := store.SetSitePinned(ctx, "pinned-high", true); err != nil {
		t.Fatalf("pin high site: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := store.SetSitePinned(ctx, "pinned-low", true); err != nil {
		t.Fatalf("pin low site: %v", err)
	}

	deploys, total, err := store.ListMarketplaceDeploys(ctx, "", "", "likes_desc", "", "", "", "", 1, 10)
	if err != nil {
		t.Fatalf("list marketplace: %v", err)
	}
	if total != 4 {
		t.Fatalf("total = %d, want 4", total)
	}
	got := []string{deploys[0].Code, deploys[1].Code, deploys[2].Code, deploys[3].Code}
	want := []string{"pinned-high", "pinned-low", "popular", "fresh"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if !deploys[0].IsPinned || deploys[0].PinnedAt == nil {
		t.Fatalf("first deploy pin fields = isPinned:%v pinnedAt:%v, want pinned", deploys[0].IsPinned, deploys[0].PinnedAt)
	}
}

func TestCreateSiteDefaultsToUnlistedAndStaysOutOfMarketplace(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.CreateSite(ctx, Site{
		Code:         "hidden-by-default",
		PublicID:     "hidden-by-default-public-id",
		OwnerTokenID: "anon:anon-default",
		CreatedAt:    now,
		UpdatedAt:    now,
		Source:       "api",
	}); err != nil {
		t.Fatalf("create default site: %v", err)
	}
	if err := store.CreateVersion(ctx, Version{
		ID:            "hidden-by-default-version-id",
		SiteCode:      "hidden-by-default",
		VersionNumber: 1,
		Title:         "Hidden by default",
		Description:   "This site should not enter marketplace by default.",
		MainEntry:     "index.html",
		TotalSize:     100,
		FileCount:     1,
		ContentSha256: "hidden-by-default-sha",
		Status:        "active",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create default version: %v", err)
	}
	v := int64(1)
	if err := store.SetCurrentVersion(ctx, "hidden-by-default", &v); err != nil {
		t.Fatalf("set current version: %v", err)
	}

	site, err := store.GetSite(ctx, "hidden-by-default")
	if err != nil {
		t.Fatalf("get default site: %v", err)
	}
	if site.Visibility != "unlisted" {
		t.Fatalf("visibility = %q, want unlisted", site.Visibility)
	}
	deploys, total, err := store.ListMarketplaceDeploys(ctx, "", "", "likes_desc", "", "", "", "", 1, 10)
	if err != nil {
		t.Fatalf("list marketplace: %v", err)
	}
	if total != 0 || len(deploys) != 0 {
		t.Fatalf("marketplace deploys = total:%d items:%#v, want hidden by default", total, deploys)
	}
}

func TestSetCurrentVersionUpdatesSiteUpdatedAt(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	createdAt := time.Now().UTC().Add(-2 * time.Hour)

	seedMarketplaceSite(t, store, "demo", 0, createdAt)
	if err := store.CreateVersion(ctx, Version{
		ID:            "demo-version-2",
		SiteCode:      "demo",
		VersionNumber: 2,
		Title:         "demo v2",
		Description:   "demo v2 description",
		MainEntry:     "index.html",
		TotalSize:     120,
		FileCount:     1,
		ContentSha256: "demo-v2-sha",
		Status:        "active",
		CreatedAt:     createdAt.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create version 2: %v", err)
	}

	v := int64(2)
	if err := store.SetCurrentVersion(ctx, "demo", &v); err != nil {
		t.Fatalf("set current version: %v", err)
	}

	site, err := store.GetSite(ctx, "demo")
	if err != nil {
		t.Fatalf("get site: %v", err)
	}
	if !site.UpdatedAt.After(createdAt) {
		t.Fatalf("updated_at = %s, want after %s", site.UpdatedAt, createdAt)
	}
}

func TestSQLiteMigrationAddsSitePinColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostctl.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE sites (
			code TEXT PRIMARY KEY,
			public_id TEXT NOT NULL UNIQUE,
			owner_token_id TEXT NOT NULL,
			current_version INTEGER,
			primary_version_strategy TEXT NOT NULL DEFAULT 'likes',
			view_count INTEGER NOT NULL DEFAULT 0,
			like_count INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			access_password_hash TEXT NOT NULL DEFAULT '',
			expires_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			source TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("seed old sites table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	defer store.Close()

	cols := map[string]bool{}
	rows, err := store.db.Query(`PRAGMA table_info(sites)`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		cols[name] = true
	}
	for _, name := range []string{"is_pinned", "pinned_at"} {
		if !cols[name] {
			t.Fatalf("column %s was not added by migration", name)
		}
	}
}

func seedMarketplaceSite(t *testing.T, store *SQLiteStore, code string, likes int64, createdAt time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := store.CreateSite(ctx, Site{
		Code:         code,
		PublicID:     code + "-public-id",
		OwnerTokenID: "user:owner",
		Visibility:   "public",
		LikeCount:    likes,
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
		Source:       "api",
	}); err != nil {
		t.Fatalf("create site %s: %v", code, err)
	}
	if err := store.CreateVersion(ctx, Version{
		ID:            code + "-version-id",
		SiteCode:      code,
		VersionNumber: 1,
		Title:         code,
		Description:   code + " description",
		MainEntry:     "index.html",
		TotalSize:     100,
		FileCount:     1,
		ContentSha256: code + "-sha",
		Status:        "active",
		CreatedAt:     createdAt,
	}); err != nil {
		t.Fatalf("create version %s: %v", code, err)
	}
	v := int64(1)
	if err := store.SetCurrentVersion(ctx, code, &v); err != nil {
		t.Fatalf("set current version %s: %v", code, err)
	}
	if likes > 0 {
		if _, err := store.db.ExecContext(ctx, `UPDATE sites SET like_count = ? WHERE code = ?`, likes, code); err != nil {
			t.Fatalf("set like count %s: %v", code, err)
		}
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
