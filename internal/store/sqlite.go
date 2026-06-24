package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// SQLiteStore 是 Store 接口的 SQLite 实现。
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 打开（必要时创建）SQLite 数据库并初始化 schema。
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	// _pragma: 用 DSN 形式设置 WAL 模式、忙等待、外键约束
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite 适合单写连接，限制连接池为 1 避免锁竞争
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := migrateDropLegacyAgentBindingCodes(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("drop legacy agent binding codes: %w", err)
	}

	// 兼容老库：补齐 marketplace 相关字段（CREATE TABLE IF NOT EXISTS 不会改老表）
	if err := migrateSitesAddMarketplaceColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sites marketplace: %w", err)
	}
	if err := migrateAdminUsersAddPolicyColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate admin users policy: %w", err)
	}
	if err := migrateTokensAddOwnerUserColumn(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate tokens owner user: %w", err)
	}
	if err := migrateDeleteUnownedTokens(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("delete unowned tokens: %w", err)
	}
	if err := migrateTokensAddExpiresAtColumn(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate tokens expires at: %w", err)
	}
	if err := migrateAnonymousSessionsAddAgentColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate anonymous sessions agent meta: %w", err)
	}

	// 给老 site 补 public_id（NULL 的填上）
	if err := backfillSitesPublicID(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("backfill public_id: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func migrateDropLegacyAgentBindingCodes(db *sql.DB) error {
	if _, err := db.Exec(`DROP TABLE IF EXISTS agent_binding_codes`); err != nil {
		return fmt.Errorf("drop legacy agent binding codes: %w", err)
	}
	return nil
}

func migrateDeleteUnownedTokens(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM tokens WHERE owner_user_id IS NULL OR owner_user_id = ''`)
	if err != nil && !contains(err.Error(), "no such table") && !contains(err.Error(), "no such column") {
		return fmt.Errorf("delete unowned tokens: %w", err)
	}
	return nil
}

func migrateAnonymousSessionsAddAgentColumns(db *sql.DB) error {
	cols := []struct {
		name string
		ddl  string
	}{
		{"agent_id", "TEXT"},
		{"agent_label", "TEXT"},
		{"device_ip", "TEXT"},
		{"user_agent", "TEXT"},
		{"claimed_by_user_id", "TEXT"},
		{"claimed_at", "DATETIME"},
	}
	for _, c := range cols {
		_, err := db.Exec(fmt.Sprintf(`ALTER TABLE anonymous_sessions ADD COLUMN %s %s`, c.name, c.ddl))
		if err != nil && !contains(err.Error(), "duplicate column") && !contains(err.Error(), "no such table") {
			return fmt.Errorf("alter anonymous_sessions add %s: %w", c.name, err)
		}
	}
	return nil
}

func migrateTokensAddOwnerUserColumn(db *sql.DB) error {
	_, err := db.Exec(`ALTER TABLE tokens ADD COLUMN owner_user_id TEXT`)
	if err != nil && !contains(err.Error(), "duplicate column") && !contains(err.Error(), "no such table") {
		return fmt.Errorf("alter tokens add owner_user_id: %w", err)
	}
	return nil
}

func migrateTokensAddExpiresAtColumn(db *sql.DB) error {
	_, err := db.Exec(`ALTER TABLE tokens ADD COLUMN expires_at DATETIME`)
	if err != nil && !contains(err.Error(), "duplicate column") && !contains(err.Error(), "no such table") {
		return fmt.Errorf("alter tokens add expires_at: %w", err)
	}
	return nil
}

func migrateAdminUsersAddPolicyColumns(db *sql.DB) error {
	cols := []struct {
		name string
		ddl  string
	}{
		{"is_admin", "BOOLEAN NOT NULL DEFAULT 0"},
		{"can_like", "BOOLEAN NOT NULL DEFAULT 1"},
		{"deploy_limit", "INTEGER NOT NULL DEFAULT 20"},
		{"deploy_count", "INTEGER NOT NULL DEFAULT 0"},
	}
	for _, c := range cols {
		_, err := db.Exec(fmt.Sprintf(`ALTER TABLE admin_users ADD COLUMN %s %s`, c.name, c.ddl))
		if err != nil && !contains(err.Error(), "duplicate column") && !contains(err.Error(), "no such table") {
			return fmt.Errorf("alter admin_users add %s: %w", c.name, err)
		}
	}
	_, _ = db.Exec(`UPDATE admin_users SET is_admin = 1 WHERE username = 'admin' AND is_admin = 0`)
	return nil
}

// migrateSitesAddMarketplaceColumns 给老 sites 表补齐 marketplace 所需列。
// 新建的库已经有这些列，会因 "duplicate column" 错误被忽略。
func migrateSitesAddMarketplaceColumns(db *sql.DB) error {
	type colDef struct {
		name string
		ddl  string
	}
	cols := []colDef{
		{"public_id", "TEXT"},
		{"view_count", "INTEGER NOT NULL DEFAULT 0"},
		{"like_count", "INTEGER NOT NULL DEFAULT 0"},
		{"status", "TEXT NOT NULL DEFAULT 'active'"},
		{"access_password_hash", "TEXT NOT NULL DEFAULT ''"},
		{"is_pinned", "BOOLEAN NOT NULL DEFAULT 0"},
		{"pinned_at", "DATETIME"},
		{"expires_at", "DATETIME"},
		{"updated_at", "DATETIME"},
		{"primary_version_strategy", "TEXT NOT NULL DEFAULT 'likes'"},
	}
	for _, c := range cols {
		_, err := db.Exec(fmt.Sprintf(`ALTER TABLE sites ADD COLUMN %s %s`, c.name, c.ddl))
		if err != nil {
			msg := err.Error()
			if !contains(msg, "duplicate column") {
				return fmt.Errorf("alter add %s: %w", c.name, err)
			}
		}
	}
	return nil
}

// backfillSitesPublicID 给 public_id 为 NULL 的 site 生成 UUID（裸 32 字符无连字符，方便 URL）。
func backfillSitesPublicID(db *sql.DB) error {
	rows, err := db.Query(`SELECT code FROM sites WHERE public_id IS NULL`)
	if err != nil {
		return err
	}
	var codes []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			_ = rows.Close()
			return err
		}
		codes = append(codes, c)
	}
	_ = rows.Close()

	for _, code := range codes {
		id, err := newUUID()
		if err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE sites SET public_id = ? WHERE code = ?`, id, code); err != nil {
			return err
		}
	}
	return nil
}

// newUUID 生成 32 字符的 UUID（无连字符），用 crypto/rand。
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// RFC 4122 v4 设置版本和变体位
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x", b), nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Close 关闭数据库。
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// CreateSite 创建一个新 site。
func (s *SQLiteStore) CreateSite(ctx context.Context, site Site) error {
	strategy := site.PrimaryVersionStrategy
	if strategy == "" {
		strategy = "likes"
	}
	publicID := site.PublicID
	if publicID == "" {
		if id, err := newUUID(); err == nil {
			publicID = id
		}
	}
	status := site.Status
	if status == "" {
		status = "active"
	}
	var expiresAt any
	if site.ExpiresAt != nil {
		expiresAt = site.ExpiresAt.UTC()
	}
	var pinnedAt any
	if site.PinnedAt != nil {
		pinnedAt = site.PinnedAt.UTC()
	}
	now := time.Now().UTC()
	updatedAt := site.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sites (code, public_id, owner_token_id, current_version, primary_version_strategy,
		                   view_count, like_count, status, access_password_hash, is_pinned, pinned_at, expires_at, created_at, updated_at, source)
		VALUES (?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, site.Code, publicID, site.OwnerTokenID, strategy, site.ViewCount, site.LikeCount, status, site.AccessPasswordHash, site.IsPinned, pinnedAt, expiresAt, site.CreatedAt.UTC(), updatedAt.UTC(), site.Source)
	if err != nil {
		return fmt.Errorf("insert site: %w", err)
	}
	return nil
}

// GetSite 按 code 取 site。
func (s *SQLiteStore) GetSite(ctx context.Context, code string) (Site, error) {
	var site Site
	var cur sql.NullInt64
	var expiresAt sql.NullString
	var pinnedAt sql.NullString
	var updatedAt sql.NullString
	var isPinned int
	err := s.db.QueryRowContext(ctx, `
		SELECT code, public_id, owner_token_id, current_version, primary_version_strategy,
		       view_count, like_count, status, access_password_hash, is_pinned, pinned_at, expires_at, created_at, updated_at, source
		FROM sites WHERE code = ?
	`, code).Scan(
		&site.Code, &site.PublicID, &site.OwnerTokenID, &cur, &site.PrimaryVersionStrategy,
		&site.ViewCount, &site.LikeCount, &site.Status, &site.AccessPasswordHash, &isPinned, &pinnedAt, &expiresAt, &site.CreatedAt, &updatedAt, &site.Source,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Site{}, ErrNotFound
	}
	if err != nil {
		return Site{}, fmt.Errorf("get site: %w", err)
	}
	if cur.Valid {
		v := cur.Int64
		site.CurrentVersion = &v
	}
	if site.PrimaryVersionStrategy == "" {
		site.PrimaryVersionStrategy = "likes"
	}
	if site.Status == "" {
		site.Status = "active"
	}
	site.IsPinned = isPinned != 0
	if pinnedAt.Valid && pinnedAt.String != "" {
		if t, perr := parseSQLiteTime(pinnedAt.String); perr == nil {
			t = t.Local()
			site.PinnedAt = &t
		}
	}
	if expiresAt.Valid && expiresAt.String != "" {
		if t, perr := parseSQLiteTime(expiresAt.String); perr == nil {
			t = t.Local()
			site.ExpiresAt = &t
		}
	}
	if updatedAt.Valid && updatedAt.String != "" {
		if t, perr := parseSQLiteTime(updatedAt.String); perr == nil {
			site.UpdatedAt = t.Local()
		}
	}
	if site.UpdatedAt.IsZero() {
		site.UpdatedAt = site.CreatedAt
	}
	site.CreatedAt = site.CreatedAt.Local()
	return site, nil
}

// SetPrimaryStrategy 更新 main URL 选版本策略。
func (s *SQLiteStore) SetPrimaryStrategy(ctx context.Context, code, strategy string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sites SET primary_version_strategy = ? WHERE code = ?`,
		strategy, code)
	if err != nil {
		return fmt.Errorf("update strategy: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SiteExists 判断 code 是否存在。
func (s *SQLiteStore) SiteExists(ctx context.Context, code string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM sites WHERE code = ? LIMIT 1`, code).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("site exists: %w", err)
	}
	return true, nil
}

// CreateVersion 写一条版本记录。
func (s *SQLiteStore) CreateVersion(ctx context.Context, v Version) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO versions
		  (id, site_code, version_number, title, description, main_entry,
		   total_size, file_count, content_sha256, is_locked, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, v.ID, v.SiteCode, v.VersionNumber, nullString(v.Title), v.Description, v.MainEntry,
		v.TotalSize, v.FileCount, v.ContentSha256, v.IsLocked, v.Status, v.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert version: %w", err)
	}
	return nil
}

// MaxVersionNumber 返回某 site 当前最大版本号；新 site 返回 0。
func (s *SQLiteStore) MaxVersionNumber(ctx context.Context, code string) (int64, error) {
	var max sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(version_number) FROM versions WHERE site_code = ?`, code).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("max version: %w", err)
	}
	if !max.Valid {
		return 0, nil
	}
	return max.Int64, nil
}

// ListVersions 列出某 site 所有版本，升序。
func (s *SQLiteStore) ListVersions(ctx context.Context, code string) ([]Version, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, site_code, version_number, title, description, main_entry,
		       total_size, file_count, content_sha256, is_locked, status, created_at
		FROM versions WHERE site_code = ?
		ORDER BY version_number ASC
	`, code)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		if err := scanVersion(rows, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateVersionLock 设置/解除锁定。
func (s *SQLiteStore) UpdateVersionLock(ctx context.Context, code string, version int64, locked bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE versions SET is_locked = ? WHERE site_code = ? AND version_number = ?`,
		locked, code, version)
	if err != nil {
		return fmt.Errorf("update lock: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateVersionStatus 设置状态。
func (s *SQLiteStore) UpdateVersionStatus(ctx context.Context, code string, version int64, status string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE versions SET status = ? WHERE site_code = ? AND version_number = ?`,
		status, code, version)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteVersion 删除版本记录 + 关联文件元数据（事务包裹）。
func (s *SQLiteStore) DeleteVersion(ctx context.Context, code string, version int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM files WHERE site_code = ? AND version_number = ?`,
		code, version); err != nil {
		return fmt.Errorf("delete files: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`DELETE FROM versions WHERE site_code = ? AND version_number = ?`,
		code, version)
	if err != nil {
		return fmt.Errorf("delete version: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete: %w", err)
	}
	return nil
}

// ListFiles 列出某版本的文件元数据。
func (s *SQLiteStore) ListFiles(ctx context.Context, code string, version int64) ([]FileMeta, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT site_code, version_number, file_path, size, sha256, is_binary
		FROM files WHERE site_code = ? AND version_number = ?
		ORDER BY file_path ASC
	`, code, version)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()
	out := []FileMeta{}
	for rows.Next() {
		var f FileMeta
		if err := rows.Scan(&f.SiteCode, &f.VersionNumber, &f.Path, &f.Size, &f.Sha256, &f.IsBinary); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SetCurrentVersion 切换当前版本，nil 表示下线。
func (s *SQLiteStore) SetCurrentVersion(ctx context.Context, code string, version *int64) error {
	var arg any
	if version != nil {
		arg = *version
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET current_version = ?, updated_at = ? WHERE code = ?`,
		arg, time.Now().UTC(), code)
	if err != nil {
		return fmt.Errorf("set current version: %w", err)
	}
	return nil
}

// CreateFiles 批量写文件元数据。事务包裹。
func (s *SQLiteStore) CreateFiles(ctx context.Context, files []FileMeta) error {
	if len(files) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO files (site_code, version_number, file_path, size, sha256, is_binary)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert file: %w", err)
	}
	defer stmt.Close()

	for _, f := range files {
		if _, err := stmt.ExecContext(ctx, f.SiteCode, f.VersionNumber, f.Path, f.Size, f.Sha256, f.IsBinary); err != nil {
			return fmt.Errorf("insert file %s: %w", f.Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit files: %w", err)
	}
	return nil
}

// GetVersion 按 code + version_number 取版本。
func (s *SQLiteStore) GetVersion(ctx context.Context, code string, version int64) (Version, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, site_code, version_number, title, description, main_entry,
		       total_size, file_count, content_sha256, is_locked, status, created_at
		FROM versions WHERE site_code = ? AND version_number = ?
	`, code, version)
	var v Version
	if err := scanVersion(row, &v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Version{}, ErrNotFound
		}
		return Version{}, fmt.Errorf("get version: %w", err)
	}
	return v, nil
}

// GetVersionByUUID 按 versions.id 取版本。
func (s *SQLiteStore) GetVersionByUUID(ctx context.Context, id string) (Version, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, site_code, version_number, title, description, main_entry,
		       total_size, file_count, content_sha256, is_locked, status, created_at
		FROM versions WHERE id = ?
	`, id)
	var v Version
	if err := scanVersion(row, &v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Version{}, ErrNotFound
		}
		return Version{}, fmt.Errorf("get version by uuid: %w", err)
	}
	return v, nil
}

// UpdateVersionContent 替换版本内容：删旧 files + 插新 files + 更新 versions 元数据。
// 事务包裹，任一步失败全部回滚。
func (s *SQLiteStore) UpdateVersionContent(ctx context.Context, code string, version int64, meta Version, files []FileMeta) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 删旧 files
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM files WHERE site_code = ? AND version_number = ?`,
		code, version); err != nil {
		return fmt.Errorf("delete old files: %w", err)
	}

	// 2. 插新 files
	if len(files) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO files (site_code, version_number, file_path, size, sha256, is_binary)
			VALUES (?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("prepare insert file: %w", err)
		}
		for _, f := range files {
			if _, err := stmt.ExecContext(ctx, f.SiteCode, f.VersionNumber, f.Path, f.Size, f.Sha256, f.IsBinary); err != nil {
				_ = stmt.Close()
				return fmt.Errorf("insert file %s: %w", f.Path, err)
			}
		}
		_ = stmt.Close()
	}

	// 3. 更新 versions 行
	res, err := tx.ExecContext(ctx, `
		UPDATE versions
		SET title = ?, description = ?, main_entry = ?,
		    total_size = ?, file_count = ?, content_sha256 = ?
		WHERE site_code = ? AND version_number = ?
	`, nullString(meta.Title), meta.Description, meta.MainEntry,
		meta.TotalSize, meta.FileCount, meta.ContentSha256,
		code, version)
	if err != nil {
		return fmt.Errorf("update version row: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update version content: %w", err)
	}
	return nil
}

// CreateToken 写一条 token 记录。
func (s *SQLiteStore) CreateToken(ctx context.Context, t Token) error {
	var lastUsed any
	if t.LastUsedAt != nil {
		lastUsed = (*t.LastUsedAt).UTC()
	}
	var expiresAt any
	if t.ExpiresAt != nil {
		expiresAt = (*t.ExpiresAt).UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tokens (id, token_hash, label, is_admin, is_revoked, owner_user_id, expires_at, created_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.TokenHash, nullString(t.Label), t.IsAdmin, t.IsRevoked, nullString(t.OwnerUserID), expiresAt, t.CreatedAt.UTC(), lastUsed)
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}
	return nil
}

// GetTokenByHash 按 token_hash 取 token。
func (s *SQLiteStore) GetTokenByHash(ctx context.Context, hash string) (Token, error) {
	return s.scanToken(s.db.QueryRowContext(ctx, `
		SELECT id, token_hash, label, is_admin, is_revoked, owner_user_id, expires_at, created_at, last_used_at
		FROM tokens WHERE token_hash = ?
	`, hash))
}

func (s *SQLiteStore) GetTokenByID(ctx context.Context, id string) (Token, error) {
	return s.scanToken(s.db.QueryRowContext(ctx, `
		SELECT id, token_hash, label, is_admin, is_revoked, owner_user_id, expires_at, created_at, last_used_at
		FROM tokens WHERE id = ?
	`, id))
}

func (s *SQLiteStore) scanToken(sc scanner) (Token, error) {
	var t Token
	var label sql.NullString
	var ownerUserID sql.NullString
	var expiresAt sql.NullTime
	var lastUsed sql.NullTime
	err := sc.Scan(&t.ID, &t.TokenHash, &label, &t.IsAdmin, &t.IsRevoked, &ownerUserID, &expiresAt, &t.CreatedAt, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, ErrNotFound
	}
	if err != nil {
		return Token{}, fmt.Errorf("scan token: %w", err)
	}
	if label.Valid {
		t.Label = label.String
	}
	if ownerUserID.Valid {
		t.OwnerUserID = ownerUserID.String
	}
	if expiresAt.Valid {
		ea := expiresAt.Time
		t.ExpiresAt = &ea
	}
	if lastUsed.Valid {
		lu := lastUsed.Time
		t.LastUsedAt = &lu
	}
	t.CreatedAt = t.CreatedAt.Local()
	return t, nil
}

// ListTokens 列出所有未吊销的 token，按创建时间升序。
func (s *SQLiteStore) ListTokens(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, token_hash, label, is_admin, is_revoked, owner_user_id, expires_at, created_at, last_used_at
		FROM tokens ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token
		var label sql.NullString
		var ownerUserID sql.NullString
		var expiresAt sql.NullTime
		var lastUsed sql.NullTime
		if err := rows.Scan(&t.ID, &t.TokenHash, &label, &t.IsAdmin, &t.IsRevoked, &ownerUserID, &expiresAt, &t.CreatedAt, &lastUsed); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		if label.Valid {
			t.Label = label.String
		}
		if ownerUserID.Valid {
			t.OwnerUserID = ownerUserID.String
		}
		if expiresAt.Valid {
			ea := expiresAt.Time
			t.ExpiresAt = &ea
		}
		if lastUsed.Valid {
			lu := lastUsed.Time
			t.LastUsedAt = &lu
		}
		t.CreatedAt = t.CreatedAt.Local()
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken 标记 token 已吊销。
func (s *SQLiteStore) RevokeToken(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tokens SET is_revoked = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchTokenLastUsed 更新 token 的 last_used_at 为当前时间。
func (s *SQLiteStore) TouchTokenLastUsed(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tokens SET last_used_at = ? WHERE id = ?`, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("touch token last_used: %w", err)
	}
	return nil
}

// scanner 抽象 sql.Row 和 sql.Rows 的共同 Scan 接口。
type scanner interface {
	Scan(dest ...any) error
}

// scanVersion 把一行版本数据扫描到 Version。
// title 在 schema 中可为 NULL（v.Title 是 string），用 NullString 中转。
func scanVersion(sc scanner, v *Version) error {
	var title sql.NullString
	err := sc.Scan(
		&v.ID, &v.SiteCode, &v.VersionNumber, &title, &v.Description, &v.MainEntry,
		&v.TotalSize, &v.FileCount, &v.ContentSha256, &v.IsLocked, &v.Status, &v.CreatedAt,
	)
	if err != nil {
		return err
	}
	if title.Valid {
		v.Title = title.String
	} else {
		v.Title = ""
	}
	v.CreatedAt = v.CreatedAt.Local()
	return nil
}

// 辅助：将空字符串转 NULL
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// GetSetting 读取一个键值；不存在返回 ("", nil)。
func (s *SQLiteStore) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting %s: %w", key, err)
	}
	return value, nil
}

// SetSetting 写入 / 更新一个键值。
func (s *SQLiteStore) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}
	return nil
}

// ListSites 列出所有 site，附版本统计：版本数 + 总大小 + 最新版本时间。
// 按 created_at DESC 排序。
func (s *SQLiteStore) ListSites(ctx context.Context) ([]SiteWithMeta, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			s.code,
			s.public_id,
			s.owner_token_id,
			s.current_version,
			s.created_at,
			COALESCE(s.updated_at, s.created_at) AS updated_at,
			s.source,
			s.view_count,
			s.like_count,
			COALESCE(s.status, 'active') AS status,
			CASE WHEN COALESCE(s.access_password_hash, '') <> '' THEN 1 ELSE 0 END AS access_protected,
			COALESCE(s.is_pinned, 0) AS is_pinned,
			s.pinned_at,
			COUNT(DISTINCT v.version_number) AS version_count,
			COALESCE(SUM(v.total_size), 0) AS total_size,
			MAX(v.created_at) AS last_version_at
		FROM sites s
		LEFT JOIN versions v ON v.site_code = s.code
		GROUP BY s.code
		ORDER BY COALESCE(s.is_pinned, 0) DESC, s.pinned_at DESC, s.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}
	defer rows.Close()

	out := []SiteWithMeta{}
	for rows.Next() {
		var item SiteWithMeta
		var cur sql.NullInt64
		var updatedAt sql.NullString
		var accessProtected int
		var isPinned int
		var pinnedAt sql.NullString
		var lastVStr sql.NullString // SQLite 的 MAX() 返回字符串，扫不进 *time.Time
		if err := rows.Scan(
			&item.Code, &item.PublicID, &item.OwnerTokenID, &cur, &item.CreatedAt, &updatedAt, &item.Source,
			&item.ViewCount, &item.LikeCount, &item.Status,
			&accessProtected, &isPinned, &pinnedAt, &item.VersionCount, &item.TotalSize, &lastVStr,
		); err != nil {
			return nil, fmt.Errorf("scan site row: %w", err)
		}
		item.AccessProtected = accessProtected != 0
		item.IsPinned = isPinned != 0
		if cur.Valid {
			v := cur.Int64
			item.CurrentVersion = &v
		}
		if updatedAt.Valid && updatedAt.String != "" {
			if t, perr := parseSQLiteTime(updatedAt.String); perr == nil {
				item.UpdatedAt = t.Local()
			}
		}
		if pinnedAt.Valid && pinnedAt.String != "" {
			if t, perr := parseSQLiteTime(pinnedAt.String); perr == nil {
				t = t.Local()
				item.PinnedAt = &t
			}
		}
		if lastVStr.Valid && lastVStr.String != "" {
			if t, perr := parseSQLiteTime(lastVStr.String); perr == nil {
				t = t.Local()
				item.LastVersionAt = &t
			}
		}
		if item.Status == "" {
			item.Status = "active"
		}
		if item.UpdatedAt.IsZero() {
			item.UpdatedAt = item.CreatedAt
		}
		item.CreatedAt = item.CreatedAt.Local()
		out = append(out, item)
	}
	return out, rows.Err()
}

// DeleteSite 删除整个 site（事务：先 files，再 versions，再 sites）。
// 磁盘文件清理由调用方负责。
func (s *SQLiteStore) DeleteSite(ctx context.Context, code string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM files WHERE site_code = ?`, code); err != nil {
		return fmt.Errorf("delete files: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM versions WHERE site_code = ?`, code); err != nil {
		return fmt.Errorf("delete versions: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM likes WHERE site_code = ?`, code); err != nil {
		// 老库可能没有 likes 表，忽略
		if !strings.Contains(err.Error(), "no such table") {
			return fmt.Errorf("delete likes: %w", err)
		}
	}
	res, err := tx.ExecContext(ctx,
		`DELETE FROM sites WHERE code = ?`, code)
	if err != nil {
		return fmt.Errorf("delete site: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete site: %w", err)
	}
	return nil
}

// ===== Marketplace =====

// marketplaceSelectCols 是 marketplace 列表/单条查询共用的列。
// current version 通过 LEFT JOIN 一次拿到，避免 N+1。
const marketplaceSelectCols = `
	s.public_id              AS id,
	s.code                   AS code,
	s.owner_token_id         AS owner_token_id,
	s.current_version        AS current_version,
	cv.id                    AS current_version_id,
	cv.title                 AS title,
	cv.description           AS description,
	cv.main_entry            AS main_entry,
	cv.total_size            AS file_size,
	s.primary_version_strategy AS primary_version_strategy,
	s.view_count             AS view_count,
	s.like_count             AS like_count,
	s.status                 AS status,
	CASE WHEN COALESCE(s.access_password_hash, '') <> '' THEN 1 ELSE 0 END AS access_protected,
	COALESCE(s.is_pinned, 0) AS is_pinned,
	s.pinned_at              AS pinned_at,
	s.expires_at             AS expires_at,
	s.created_at             AS created_at,
	COALESCE(s.updated_at, s.created_at) AS updated_at,
	(SELECT COUNT(*) FROM versions v2 WHERE v2.site_code = s.code) AS version_count
`

// marketplaceJoins 是 site LEFT JOIN 当前版本的 FROM/JOIN 子句。
// 当前版本定义：current_version 不为空时按它取，否则按 max(version_number) 兜底。
const marketplaceFrom = `
	FROM sites s
	LEFT JOIN versions cv ON cv.site_code = s.code
		AND cv.version_number = COALESCE(s.current_version, (
			SELECT MAX(version_number) FROM versions WHERE site_code = s.code
		))
`

func scanMarketplaceRow(sc scanner) (MarketplaceDeploy, error) {
	var d MarketplaceDeploy
	var curVer sql.NullInt64
	var curVerID sql.NullString
	var title sql.NullString
	var desc sql.NullString
	var mainEntry sql.NullString
	var fileSize sql.NullInt64
	var expiresAt sql.NullString
	var updatedAt sql.NullString
	var strategy sql.NullString
	var status sql.NullString
	var accessProtected int
	var isPinned int
	var pinnedAt sql.NullString
	if err := sc.Scan(
		&d.ID, &d.Code, &d.OwnerTokenID, &curVer, &curVerID, &title, &desc, &mainEntry, &fileSize,
		&strategy, &d.ViewCount, &d.LikeCount, &status, &accessProtected, &isPinned, &pinnedAt, &expiresAt, &d.CreatedAt, &updatedAt, &d.VersionCount,
	); err != nil {
		return d, err
	}
	d.AccessProtected = accessProtected != 0
	d.IsPinned = isPinned != 0
	if curVer.Valid {
		v := curVer.Int64
		d.CurrentVersion = &v
	}
	if curVerID.Valid {
		d.CurrentVersionID = curVerID.String
	}
	if title.Valid {
		d.Title = title.String
	}
	if desc.Valid {
		d.Description = desc.String
	}
	if mainEntry.Valid {
		d.MainEntry = mainEntry.String
		d.Filename = mainEntry.String
	}
	if fileSize.Valid {
		d.FileSize = fileSize.Int64
	}
	if strategy.Valid {
		d.PrimaryVersionStrategy = strategy.String
	}
	if status.Valid {
		d.Status = status.String
	}
	if d.Status == "" {
		d.Status = "active"
	}
	if d.PrimaryVersionStrategy == "" {
		d.PrimaryVersionStrategy = "likes"
	}
	if pinnedAt.Valid && pinnedAt.String != "" {
		if t, perr := parseSQLiteTime(pinnedAt.String); perr == nil {
			t = t.Local()
			d.PinnedAt = &t
		}
	}
	if expiresAt.Valid && expiresAt.String != "" {
		if t, perr := parseSQLiteTime(expiresAt.String); perr == nil {
			t = t.Local()
			d.ExpiresAt = &t
		}
	}
	if updatedAt.Valid && updatedAt.String != "" {
		if t, perr := parseSQLiteTime(updatedAt.String); perr == nil {
			d.UpdatedAt = t.Local()
		}
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = d.CreatedAt
	}
	d.CreatedAt = d.CreatedAt.Local()
	return d, nil
}

// ListMarketplaceDeploys 分页 + 搜索 + 排序 + 状态过滤。
// 返回 deploys 列表 + total（用于分页显示）。
func (s *SQLiteStore) ListMarketplaceDeploys(ctx context.Context, q, status, sort string, page, pageSize int) ([]MarketplaceDeploy, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 24
	}

	var where []string
	var args []any
	if q != "" {
		where = append(where, `(s.code LIKE ? OR cv.title LIKE ? OR cv.description LIKE ? OR cv.main_entry LIKE ?)`)
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	if status == "active" || status == "inactive" {
		where = append(where, `s.status = ?`)
		args = append(args, status)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	orderSQL := "s.created_at DESC"
	switch sort {
	case "oldest":
		orderSQL = "s.created_at ASC"
	case "views_desc":
		orderSQL = "s.view_count DESC, s.created_at DESC"
	case "views_asc":
		orderSQL = "s.view_count ASC, s.created_at DESC"
	case "likes_desc":
		orderSQL = "s.like_count DESC, s.created_at DESC"
	case "likes_asc":
		orderSQL = "s.like_count ASC, s.created_at DESC"
	}
	orderSQL = "COALESCE(s.is_pinned, 0) DESC, " + orderSQL

	// 总数
	var total int
	countSQL := `SELECT COUNT(*) FROM sites s LEFT JOIN versions cv ON cv.site_code = s.code AND cv.version_number = COALESCE(s.current_version, (SELECT MAX(version_number) FROM versions WHERE site_code = s.code)) ` + whereSQL
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count marketplace: %w", err)
	}

	listSQL := `SELECT ` + marketplaceSelectCols + marketplaceFrom + whereSQL + `
		ORDER BY ` + orderSQL + `
		LIMIT ? OFFSET ?`
	listArgs := append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list marketplace: %w", err)
	}
	defer rows.Close()

	out := []MarketplaceDeploy{}
	for rows.Next() {
		d, err := scanMarketplaceRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan marketplace row: %w", err)
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

// GetMarketplaceDeploy 按 code 取单条 marketplace 数据。
func (s *SQLiteStore) GetMarketplaceDeploy(ctx context.Context, code string) (MarketplaceDeploy, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+marketplaceSelectCols+marketplaceFrom+` WHERE s.code = ?`, code)
	d, err := scanMarketplaceRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return MarketplaceDeploy{}, ErrNotFound
	}
	if err != nil {
		return MarketplaceDeploy{}, fmt.Errorf("get marketplace deploy by code: %w", err)
	}
	return d, nil
}

// GetMarketplaceDeployByUUID 按 public_id 取单条 marketplace 数据。
func (s *SQLiteStore) GetMarketplaceDeployByUUID(ctx context.Context, publicID string) (MarketplaceDeploy, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+marketplaceSelectCols+marketplaceFrom+` WHERE s.public_id = ?`, publicID)
	d, err := scanMarketplaceRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return MarketplaceDeploy{}, ErrNotFound
	}
	if err != nil {
		return MarketplaceDeploy{}, fmt.Errorf("get marketplace deploy by uuid: %w", err)
	}
	return d, nil
}

// IncrementViewCount 给 site.view_count + 1。
func (s *SQLiteStore) IncrementViewCount(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET view_count = view_count + 1 WHERE code = ?`, code)
	if err != nil {
		return fmt.Errorf("increment view_count: %w", err)
	}
	return nil
}

// AddLike 给 site 加一次点赞；user_fingerprint 已存在则忽略。
// 返回加完后的 like_count。
func (s *SQLiteStore) AddLike(ctx context.Context, code, userFingerprint string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO likes (site_code, user_fingerprint, created_at) VALUES (?, ?, ?)`,
		code, userFingerprint, time.Now().UTC()); err != nil {
		return 0, fmt.Errorf("insert like: %w", err)
	}

	// 只在实际插入时（fingerprint 未存在）才给 site.like_count + 1
	res, err := tx.ExecContext(ctx,
		`UPDATE sites
		 SET like_count = like_count + (SELECT changes())
		 WHERE code = ?`, code)
	if err != nil {
		return 0, fmt.Errorf("bump like_count: %w", err)
	}
	_ = res

	var newCount int64
	if err := tx.QueryRowContext(ctx,
		`SELECT like_count FROM sites WHERE code = ?`, code).Scan(&newCount); err != nil {
		return 0, fmt.Errorf("read like_count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit like: %w", err)
	}
	return newCount, nil
}

// UpdateSiteStatus 设置 site.status。
func (s *SQLiteStore) UpdateSiteStatus(ctx context.Context, code, status string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sites SET status = ?, updated_at = ? WHERE code = ?`,
		status, time.Now().UTC(), code)
	if err != nil {
		return fmt.Errorf("update site status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSitePinned 设置或取消首页应用商城置顶。
func (s *SQLiteStore) SetSitePinned(ctx context.Context, code string, pinned bool) error {
	now := time.Now().UTC()
	var pinnedAt any
	if pinned {
		pinnedAt = now
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sites SET is_pinned = ?, pinned_at = ?, updated_at = ? WHERE code = ?`,
		pinned, pinnedAt, now, code)
	if err != nil {
		return fmt.Errorf("set site pinned: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchSiteUpdated 把 updated_at 更新为当前时间。
func (s *SQLiteStore) TouchSiteUpdated(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET updated_at = ? WHERE code = ?`, time.Now().UTC(), code)
	if err != nil {
		return fmt.Errorf("touch updated_at: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SetSiteAccessPasswordHash(ctx context.Context, code, hash string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sites SET access_password_hash = ?, updated_at = ? WHERE code = ?`,
		hash, time.Now().UTC(), code)
	if err != nil {
		return fmt.Errorf("set site access password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", value)
}
