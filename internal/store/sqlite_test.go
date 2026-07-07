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

func TestSQLiteMigrationAddsAuditResultColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostctl.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	now := time.Now().UTC()
	_, err = db.Exec(`
		CREATE TABLE audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_type TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			site_code TEXT NOT NULL DEFAULT '',
			target_type TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL
		);
		INSERT INTO audit_logs (
			actor_type, actor_id, action, site_code, target_type,
			target_id, ip, user_agent, detail_json, created_at
		) VALUES (
			'user', 'u1', 'legacy.action', 'demo', 'site',
			'demo', '127.0.0.1', 'legacy-agent', '{}', ?
		);
	`, now)
	if err != nil {
		t.Fatalf("seed old audit table: %v", err)
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
	rows, err := store.db.Query(`PRAGMA table_info(audit_logs)`)
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
	for _, name := range []string{"actor_role", "result"} {
		if !cols[name] {
			t.Fatalf("column %s was not added by migration", name)
		}
	}

	var actorRole, result, action string
	if err := store.db.QueryRow(`
		SELECT actor_role, result, action FROM audit_logs WHERE actor_id = 'u1'
	`).Scan(&actorRole, &result, &action); err != nil {
		t.Fatalf("query migrated audit log: %v", err)
	}
	if actorRole != "" || result != "success" || action != "legacy.action" {
		t.Fatalf("migrated audit log = role:%q result:%q action:%q", actorRole, result, action)
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

func TestSQLiteMigrationAddsAuditResultBeforeIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostctl.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	now := time.Now().UTC()
	_, err = db.Exec(`
		CREATE TABLE audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_type TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			actor_role TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			site_code TEXT NOT NULL DEFAULT '',
			target_type TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL
		);
		INSERT INTO audit_logs
			(actor_type, actor_id, actor_role, action, site_code, target_type, target_id, created_at)
		VALUES
			('user', 'admin-1', 'admin', 'site.pin', 'demo', 'site', 'demo', ?);
	`, now)
	if err != nil {
		t.Fatalf("seed legacy audit logs: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	defer store.Close()

	var result string
	if err := store.db.QueryRow(`SELECT result FROM audit_logs WHERE id = 1`).Scan(&result); err != nil {
		t.Fatalf("query migrated result column: %v", err)
	}
	if result != "success" {
		t.Fatalf("result = %q, want success", result)
	}
	var indexCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_audit_logs_result'`).Scan(&indexCount); err != nil {
		t.Fatalf("query audit result index: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("idx_audit_logs_result count = %d, want 1", indexCount)
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

func TestSetSiteSecurityModePersists(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.CreateSite(ctx, Site{
		Code:         "secure-demo",
		PublicID:     "secure-demo-public-id",
		OwnerTokenID: "user:owner",
		CreatedAt:    now,
		UpdatedAt:    now,
		Source:       "api",
	}); err != nil {
		t.Fatalf("create site: %v", err)
	}
	site, err := store.GetSite(ctx, "secure-demo")
	if err != nil {
		t.Fatalf("get site: %v", err)
	}
	if site.SecurityMode != "auto" {
		t.Fatalf("security mode = %q, want auto", site.SecurityMode)
	}
	if err := store.SetSiteSecurityMode(ctx, "secure-demo", "compatible"); err != nil {
		t.Fatalf("set security mode: %v", err)
	}
	site, err = store.GetSite(ctx, "secure-demo")
	if err != nil {
		t.Fatalf("get updated site: %v", err)
	}
	if site.SecurityMode != "compatible" {
		t.Fatalf("security mode = %q, want compatible", site.SecurityMode)
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

func TestSQLiteMigrationAddsPublicIDBeforeIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostctl.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	now := time.Now().UTC()
	_, err = db.Exec(`
		CREATE TABLE sites (
			code TEXT PRIMARY KEY,
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
		INSERT INTO sites (
			code, owner_token_id, current_version, created_at, updated_at, source
		) VALUES (
			'legacy-demo', 'anon:old', NULL, ?, ?, 'api'
		);
	`, now, now)
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

	site, err := store.GetSite(context.Background(), "legacy-demo")
	if err != nil {
		t.Fatalf("get migrated site: %v", err)
	}
	if site.PublicID == "" {
		t.Fatal("expected migrated site to receive public_id")
	}
	var indexCount int
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_sites_public_id'
	`).Scan(&indexCount); err != nil {
		t.Fatalf("query public_id index: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("idx_sites_public_id count = %d, want 1", indexCount)
	}
}

func TestSQLiteMigrationPreservesProductionLikeLegacyData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostctl.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	_, err = db.Exec(`
		CREATE TABLE tokens (
			id TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL UNIQUE,
			label TEXT,
			is_admin BOOLEAN NOT NULL DEFAULT 0,
			is_revoked BOOLEAN NOT NULL DEFAULT 0,
			owner_user_id TEXT,
			created_at DATETIME NOT NULL,
			last_used_at DATETIME
		);
		INSERT INTO tokens (id, token_hash, label, is_admin, is_revoked, owner_user_id, created_at)
		VALUES
			('owned-token', 'owned-hash', 'owned', 0, 0, 'user-1', ?),
			('legacy-system-token', 'legacy-hash', 'legacy', 0, 0, '', ?);

		CREATE TABLE admin_users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			last_login_at DATETIME
		);
		INSERT INTO admin_users (id, username, password_hash, is_active, created_at)
		VALUES ('user-1', 'alice', 'hash', 1, ?);

		CREATE TABLE sites (
			code TEXT PRIMARY KEY,
			owner_token_id TEXT NOT NULL,
			current_version INTEGER,
			view_count INTEGER NOT NULL DEFAULT 0,
			like_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			source TEXT NOT NULL
		);
		INSERT INTO sites (code, owner_token_id, current_version, view_count, like_count, created_at, source)
		VALUES ('legacy-demo', 'user:user-1', 1, 7, 3, ?, 'api');

		CREATE TABLE versions (
			id TEXT PRIMARY KEY,
			site_code TEXT NOT NULL,
			version_number INTEGER NOT NULL,
			title TEXT,
			description TEXT NOT NULL,
			main_entry TEXT NOT NULL DEFAULT 'index.html',
			total_size INTEGER NOT NULL,
			file_count INTEGER NOT NULL,
			content_sha256 TEXT NOT NULL,
			is_locked BOOLEAN NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			UNIQUE(site_code, version_number)
		);
		INSERT INTO versions (
			id, site_code, version_number, title, description, main_entry,
			total_size, file_count, content_sha256, is_locked, status, created_at
		) VALUES (
			'version-1', 'legacy-demo', 1, '旧站点', '旧版本描述', 'index.html',
			128, 1, 'sha-legacy', 0, 'active', ?
		);

		CREATE TABLE files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			site_code TEXT NOT NULL,
			version_number INTEGER NOT NULL,
			file_path TEXT NOT NULL,
			size INTEGER NOT NULL,
			sha256 TEXT NOT NULL,
			is_binary BOOLEAN NOT NULL,
			UNIQUE(site_code, version_number, file_path)
		);
		INSERT INTO files (site_code, version_number, file_path, size, sha256, is_binary)
		VALUES ('legacy-demo', 1, 'index.html', 128, 'sha-file', 0);

		CREATE TABLE anonymous_sessions (
			id TEXT PRIMARY KEY,
			deploy_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			last_used_at DATETIME NOT NULL
		);
		INSERT INTO anonymous_sessions (id, deploy_count, created_at, last_used_at)
		VALUES ('anon-1', 2, ?, ?);

		CREATE TABLE screens (
			id TEXT PRIMARY KEY,
			owner_user_id TEXT,
			name TEXT NOT NULL DEFAULT '',
			device_name TEXT NOT NULL DEFAULT '',
			device_token_hash TEXT UNIQUE,
			status TEXT NOT NULL DEFAULT 'pairing',
			current_site_code TEXT NOT NULL DEFAULT '',
			current_version INTEGER,
			last_seen_at DATETIME,
			app_version TEXT NOT NULL DEFAULT '',
			runtime TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			revoked_at DATETIME
		);
		INSERT INTO screens (
			id, owner_user_id, name, device_name, device_token_hash, status,
			current_site_code, current_version, app_version, runtime, created_at, updated_at
		) VALUES (
			'screen-1', 'user-1', '大厅屏', 'ABR-AL80', 'screen-hash', 'online',
			'legacy-demo', 1, '1.0.0', 'android', ?, ?
		);

		CREATE TABLE audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_type TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			site_code TEXT NOT NULL DEFAULT '',
			target_type TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL
		);
		INSERT INTO audit_logs (actor_type, actor_id, action, site_code, target_type, target_id, created_at)
		VALUES ('user', 'user-1', 'site.update', 'legacy-demo', 'site', 'legacy-demo', ?);
	`, now, now, now, now, now, now, now, now, now, now, now)
	if err != nil {
		t.Fatalf("seed production-like legacy db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	site, err := store.GetSite(ctx, "legacy-demo")
	if err != nil {
		t.Fatalf("get migrated site: %v", err)
	}
	if site.PublicID == "" || site.CurrentVersion == nil || *site.CurrentVersion != 1 ||
		site.Visibility != "unlisted" || site.Status != "active" ||
		site.PrimaryVersionStrategy != "likes" || site.UpdatedAt.IsZero() {
		t.Fatalf("migrated site = %+v; want defaults and preserved current version", site)
	}

	version, err := store.GetVersion(ctx, "legacy-demo", 1)
	if err != nil {
		t.Fatalf("get migrated version: %v", err)
	}
	if version.TemplateSourceCode != "" || version.TemplateSourceVersion != nil ||
		version.Title != "旧站点" || version.Description != "旧版本描述" {
		t.Fatalf("migrated version = %+v; want old metadata preserved and template defaults", version)
	}
	files, err := store.ListFiles(ctx, "legacy-demo", 1)
	if err != nil {
		t.Fatalf("list migrated files: %v", err)
	}
	if len(files) != 1 || files[0].Path != "index.html" || files[0].Size != 128 {
		t.Fatalf("migrated files = %+v; want index.html preserved", files)
	}

	user, err := store.GetAdminUserByID(ctx, "user-1")
	if err != nil {
		t.Fatalf("get migrated user: %v", err)
	}
	if user.Email != "" || user.EmailVerified || user.IsAdmin || !user.CanLike || user.DeployLimit != 20 {
		t.Fatalf("migrated user = %+v; want policy/email defaults", user)
	}
	tokens, err := store.ListTokens(ctx)
	if err != nil {
		t.Fatalf("list migrated tokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != "owned-token" || tokens[0].OwnerUserID != "user-1" || tokens[0].ExpiresAt != nil {
		t.Fatalf("migrated tokens = %+v; want owned token preserved and legacy unowned token removed", tokens)
	}

	session, err := store.GetAnonymousSession(ctx, "anon-1")
	if err != nil {
		t.Fatalf("get migrated anonymous session: %v", err)
	}
	if session.DeployCount != 2 || session.AgentID != "" || session.ClaimedByUserID != "" {
		t.Fatalf("migrated anonymous session = %+v; want deploy count and empty new fields", session)
	}
	screens, err := store.ListScreensByUser(ctx, "user-1")
	if err != nil {
		t.Fatalf("list migrated screens: %v", err)
	}
	if len(screens) != 1 || screens[0].ID != "screen-1" || screens[0].DeviceInfo != "{}" ||
		screens[0].ScreenshotRequestID != "" || screens[0].CommandPayload != "{}" {
		t.Fatalf("migrated screens = %+v; want device info and command defaults", screens)
	}

	var ftsCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM site_search_fts WHERE code = 'legacy-demo'`).Scan(&ftsCount); err != nil {
		t.Fatalf("query search index: %v", err)
	}
	if ftsCount != 1 {
		t.Fatalf("search index count = %d, want 1", ftsCount)
	}
	for _, table := range []string{"favorites", "render_cache", "version_bundles"} {
		var tableCount int
		if err := store.db.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master
			WHERE type IN ('table', 'view') AND name = ?
		`, table).Scan(&tableCount); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if tableCount != 1 {
			t.Fatalf("table %s count = %d, want 1", table, tableCount)
		}
	}
	var auditResult, auditRole string
	if err := store.db.QueryRow(`SELECT result, actor_role FROM audit_logs WHERE id = 1`).Scan(&auditResult, &auditRole); err != nil {
		t.Fatalf("query migrated audit log: %v", err)
	}
	if auditResult != "success" || auditRole != "" {
		t.Fatalf("audit result=%q role=%q, want success and empty role", auditResult, auditRole)
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
