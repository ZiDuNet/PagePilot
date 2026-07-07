//go:build ignore

package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yourorg/hostctl/internal/auth"
	_ "modernc.org/sqlite"
)

const (
	legacyDemoCode   = "legacy-demo"
	legacySecretCode = "legacy-secret"
	legacyUserID     = "user-1"
	legacyTokenID    = "owned-token"
)

func main() {
	mode := flag.String("mode", "", "seed or verify")
	dbPath := flag.String("db", "", "SQLite database path")
	hostedDir := flag.String("hosted", "", "hosted file directory")
	adminPassword := flag.String("admin-password", "legacy_admin_Pass123!", "legacy admin password")
	secretPassword := flag.String("secret-password", "legacy-secret", "legacy site access password")
	tokenPlaintext := flag.String("token", "legacy-token-plaintext", "legacy token plaintext")
	flag.Parse()

	if *dbPath == "" {
		fatalf("--db is required")
	}
	switch *mode {
	case "seed":
		if *hostedDir == "" {
			fatalf("--hosted is required in seed mode")
		}
		if err := seed(*dbPath, *hostedDir, *adminPassword, *secretPassword, *tokenPlaintext); err != nil {
			fatalf("seed legacy database: %v", err)
		}
	case "verify":
		if err := verify(*dbPath); err != nil {
			fatalf("verify upgraded database: %v", err)
		}
	default:
		fatalf("--mode must be seed or verify")
	}
}

func seed(dbPath, hostedDir, adminPassword, secretPassword, tokenPlaintext string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(hostedDir, 0o755); err != nil {
		return err
	}
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	adminHash, err := auth.HashPassword(adminPassword)
	if err != nil {
		return err
	}
	accessHash, err := auth.HashPassword(secretPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Add(-2 * time.Hour)

	demoHTML := []byte(`<!doctype html><html><head><meta charset="utf-8"><title>Legacy Demo</title><script src="assets/app.js"></script></head><body><main><h1>legacy demo ok</h1></main></body></html>`)
	demoJS := []byte(`document.body.dataset.legacyDemo = "ok";`)
	secretHTML := []byte(`<!doctype html><html><head><meta charset="utf-8"><title>Legacy Secret</title></head><body><main><h1>legacy secret ok</h1></main></body></html>`)

	if err := writeHostedFile(hostedDir, legacyDemoCode, 1, "index.html", demoHTML); err != nil {
		return err
	}
	if err := writeHostedFile(hostedDir, legacyDemoCode, 1, "assets/app.js", demoJS); err != nil {
		return err
	}
	if err := writeHostedFile(hostedDir, legacySecretCode, 1, "index.html", secretHTML); err != nil {
		return err
	}

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
		CREATE TABLE admin_users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			last_login_at DATETIME
		);
		CREATE TABLE sites (
			code TEXT PRIMARY KEY,
			owner_token_id TEXT NOT NULL,
			current_version INTEGER,
			visibility TEXT NOT NULL DEFAULT 'unlisted',
			status TEXT NOT NULL DEFAULT 'active',
			category TEXT NOT NULL DEFAULT '',
			tags TEXT NOT NULL DEFAULT '',
			view_count INTEGER NOT NULL DEFAULT 0,
			like_count INTEGER NOT NULL DEFAULT 0,
			access_password_hash TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			source TEXT NOT NULL
		);
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
		CREATE TABLE anonymous_sessions (
			id TEXT PRIMARY KEY,
			deploy_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			last_used_at DATETIME NOT NULL
		);
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
	`)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO admin_users (id, username, password_hash, is_active, created_at)
		VALUES (?, 'admin', ?, 1, ?)
	`, legacyUserID, adminHash, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO tokens (id, token_hash, label, is_admin, is_revoked, owner_user_id, created_at)
		VALUES
			(?, ?, 'legacy owned token', 0, 0, ?, ?),
			('legacy-system-token', ?, 'legacy unowned token', 0, 0, '', ?)
	`, legacyTokenID, auth.HashToken(tokenPlaintext), legacyUserID, now, auth.HashToken("unowned-token"), now); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO sites (
			code, owner_token_id, current_version, visibility, status, category, tags,
			view_count, like_count, access_password_hash, created_at, source
		) VALUES
			(?, ?, 1, 'public', 'active', 'docs', 'legacy,upgrade', 12, 4, '', ?, 'cli'),
			(?, ?, 1, 'public', 'active', 'internal', 'secret', 3, 1, ?, ?, 'api')
	`, legacyDemoCode, "user:"+legacyUserID, now, legacySecretCode, "user:"+legacyUserID, accessHash, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO versions (
			id, site_code, version_number, title, description, main_entry,
			total_size, file_count, content_sha256, is_locked, status, created_at
		) VALUES
			('version-demo-1', ?, 1, 'Legacy Demo', 'Legacy demo upgraded from old SQLite data', 'index.html', ?, 2, ?, 0, 'active', ?),
			('version-secret-1', ?, 1, 'Legacy Secret', 'Password protected legacy site', 'index.html', ?, 1, ?, 0, 'active', ?)
	`, legacyDemoCode, int64(len(demoHTML)+len(demoJS)), aggregateSHA(demoHTML, demoJS), now,
		legacySecretCode, int64(len(secretHTML)), aggregateSHA(secretHTML), now); err != nil {
		return err
	}
	for _, file := range []struct {
		code string
		path string
		body []byte
	}{
		{legacyDemoCode, "index.html", demoHTML},
		{legacyDemoCode, "assets/app.js", demoJS},
		{legacySecretCode, "index.html", secretHTML},
	} {
		if _, err := tx.Exec(`
			INSERT INTO files (site_code, version_number, file_path, size, sha256, is_binary)
			VALUES (?, 1, ?, ?, ?, 0)
		`, file.code, file.path, len(file.body), sha256Hex(file.body)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO anonymous_sessions (id, deploy_count, created_at, last_used_at)
		VALUES ('anon-legacy', 5, ?, ?)
	`, now, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO screens (
			id, owner_user_id, name, device_name, device_token_hash, status,
			current_site_code, current_version, app_version, runtime, created_at, updated_at
		) VALUES (
			'screen-legacy', ?, 'Legacy Lobby', 'ABR-AL80', 'screen-token-hash',
			'online', ?, 1, '1.0.0', 'android', ?, ?
		)
	`, legacyUserID, legacyDemoCode, now, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO audit_logs (actor_type, actor_id, action, site_code, target_type, target_id, ip, user_agent, detail_json, created_at)
		VALUES ('user', ?, 'site.update', ?, 'site', ?, '127.0.0.1', 'legacy-upgrade-qa', '{"from":"legacy"}', ?)
	`, legacyUserID, legacyDemoCode, legacyDemoCode, now); err != nil {
		return err
	}

	return tx.Commit()
}

func verify(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	checks := []struct {
		name string
		sql  string
		want int
	}{
		{"migrated sites", `SELECT COUNT(*) FROM sites WHERE public_id IS NOT NULL AND public_id <> '' AND source_download_policy = 'auto' AND security_mode = 'auto'`, 2},
		{"owned token preserved", `SELECT COUNT(*) FROM tokens WHERE id = 'owned-token' AND owner_user_id = 'user-1' AND expires_at IS NULL`, 1},
		{"unowned token removed", `SELECT COUNT(*) FROM tokens WHERE id = 'legacy-system-token'`, 0},
		{"admin policy backfilled", `SELECT COUNT(*) FROM admin_users WHERE id = 'user-1' AND username = 'admin' AND is_admin = 1 AND can_like = 1`, 1},
		{"anonymous columns backfilled", `SELECT COUNT(*) FROM anonymous_sessions WHERE id = 'anon-legacy' AND deploy_count = 5 AND agent_id IS NULL AND claimed_by_user_id IS NULL`, 1},
		{"screen columns backfilled", `SELECT COUNT(*) FROM screens WHERE id = 'screen-legacy' AND device_info = '{}' AND screenshot_request_id = '' AND command_payload = '{}'`, 1},
		{"audit result backfilled", `SELECT COUNT(*) FROM audit_logs WHERE site_code = 'legacy-demo' AND result = 'success' AND actor_role = ''`, 1},
		{"fts backfilled", `SELECT COUNT(*) FROM site_search_fts WHERE code = 'legacy-demo'`, 1},
	}
	for _, check := range checks {
		var got int
		if err := db.QueryRow(check.sql).Scan(&got); err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
		if got != check.want {
			return fmt.Errorf("%s: got %d, want %d", check.name, got, check.want)
		}
	}
	for _, table := range []string{"favorites", "render_cache", "version_bundles", "screen_pairings"} {
		var got int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table', 'view') AND name = ?`, table).Scan(&got); err != nil {
			return err
		}
		if got != 1 {
			return fmt.Errorf("table %s exists = %d, want 1", table, got)
		}
	}
	return nil
}

func writeHostedFile(hostedDir, code string, version int, name string, body []byte) error {
	full := filepath.Join(hostedDir, code, "versions", fmt.Sprintf("%d", version), filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, body, 0o644)
}

func aggregateSHA(parts ...[]byte) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(sha256Hex(part)))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
