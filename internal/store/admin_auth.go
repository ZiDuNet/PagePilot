package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *SQLiteStore) CountAdminUsers(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users WHERE is_admin = 1 AND is_active = 1`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count admin users: %w", err)
	}
	return n, nil
}

func (s *SQLiteStore) CreateAdminUser(ctx context.Context, user AdminUser) error {
	if user.DeployLimit == 0 {
		user.DeployLimit = 20
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_users (id, username, email, email_verified, password_hash, is_admin, is_active, can_like, deploy_limit, deploy_count, created_at, last_login_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, user.ID, user.Username, user.Email, user.EmailVerified, user.PasswordHash, user.IsAdmin, user.IsActive, user.CanLike, user.DeployLimit, user.DeployCount, user.CreatedAt.UTC(), nilTime(user.LastLoginAt))
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	return nil
}

// CreateFirstAdmin atomically creates the first active administrator. The
// conditional INSERT is evaluated while SQLite holds the write lock, so
// concurrent setup requests cannot both observe an uninitialized database and
// create administrators.
func (s *SQLiteStore) CreateFirstAdmin(ctx context.Context, user AdminUser) error {
	if user.DeployLimit == 0 {
		user.DeployLimit = 20
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create first admin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO admin_users (id, username, email, email_verified, password_hash, is_admin, is_active, can_like, deploy_limit, deploy_count, created_at, last_login_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM admin_users WHERE is_admin = 1 AND is_active = 1)
	`, user.ID, user.Username, user.Email, user.EmailVerified, user.PasswordHash, user.IsAdmin, user.IsActive, user.CanLike, user.DeployLimit, user.DeployCount, user.CreatedAt.UTC(), nilTime(user.LastLoginAt))
	if err != nil {
		return fmt.Errorf("create first admin: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check first admin insert: %w", err)
	}
	if n == 0 {
		return ErrAlreadyExists
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit first admin: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateAdminUser(ctx context.Context, user AdminUser) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update admin user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentAdmin, currentActive bool
	if err := tx.QueryRowContext(ctx, `SELECT is_admin, is_active FROM admin_users WHERE id = ?`, user.ID).Scan(&currentAdmin, &currentActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load admin user for update: %w", err)
	}
	removesActiveAdmin := currentAdmin && currentActive && (!user.IsAdmin || !user.IsActive)
	if removesActiveAdmin {
		var activeAdmins int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users WHERE is_admin = 1 AND is_active = 1`).Scan(&activeAdmins); err != nil {
			return fmt.Errorf("count active admins: %w", err)
		}
		if activeAdmins <= 1 {
			return ErrLastActiveAdmin
		}
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE admin_users
		SET username = ?, email = ?, email_verified = ?, is_admin = ?, is_active = ?, can_like = ?, deploy_limit = ?
		WHERE id = ?
	`, user.Username, user.Email, user.EmailVerified, user.IsAdmin, user.IsActive, user.CanLike, user.DeployLimit, user.ID)
	if err != nil {
		return fmt.Errorf("update admin user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if !user.IsActive || (currentAdmin && !user.IsAdmin) {
		if _, err := tx.ExecContext(ctx, `UPDATE tokens SET is_revoked = 1 WHERE owner_user_id = ? AND is_revoked = 0`, user.ID); err != nil {
			return fmt.Errorf("revoke user tokens: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE admin_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, time.Now().UTC(), user.ID); err != nil {
			return fmt.Errorf("revoke user sessions: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update admin user: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateAdminUserPassword(ctx context.Context, id, passwordHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update admin user password: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		UPDATE admin_users
		SET password_hash = ?
		WHERE id = ?
	`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("update admin user password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tokens SET is_revoked = 1 WHERE owner_user_id = ? AND is_revoked = 0`, id); err != nil {
		return fmt.Errorf("revoke user tokens after password change: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE admin_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, time.Now().UTC(), id); err != nil {
		return fmt.Errorf("revoke user sessions after password change: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update admin user password: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteAdminUser(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete admin user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentAdmin, currentActive bool
	if err := tx.QueryRowContext(ctx, `SELECT is_admin, is_active FROM admin_users WHERE id = ?`, id).Scan(&currentAdmin, &currentActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load admin user for delete: %w", err)
	}
	if currentAdmin && currentActive {
		var activeAdmins int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users WHERE is_admin = 1 AND is_active = 1`).Scan(&activeAdmins); err != nil {
			return fmt.Errorf("count active admins: %w", err)
		}
		if activeAdmins <= 1 {
			return ErrLastActiveAdmin
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE tokens SET is_revoked = 1 WHERE owner_user_id = ?`, id); err != nil {
		return fmt.Errorf("revoke user tokens: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE admin_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, time.Now().UTC(), id); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_users WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete admin user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete admin user: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetAdminUserByUsername(ctx context.Context, username string) (AdminUser, error) {
	return s.scanAdminUser(s.db.QueryRowContext(ctx, `
		SELECT id, username, email, email_verified, password_hash, is_admin, is_active, can_like, deploy_limit, deploy_count, created_at, last_login_at
		FROM admin_users WHERE username = ?
	`, username))
}

func (s *SQLiteStore) GetAdminUserByID(ctx context.Context, id string) (AdminUser, error) {
	return s.scanAdminUser(s.db.QueryRowContext(ctx, `
		SELECT id, username, email, email_verified, password_hash, is_admin, is_active, can_like, deploy_limit, deploy_count, created_at, last_login_at
		FROM admin_users WHERE id = ?
	`, id))
}

func (s *SQLiteStore) scanAdminUser(row scanner) (AdminUser, error) {
	var user AdminUser
	var lastLogin sql.NullTime
	err := row.Scan(&user.ID, &user.Username, &user.Email, &user.EmailVerified, &user.PasswordHash, &user.IsAdmin, &user.IsActive, &user.CanLike, &user.DeployLimit, &user.DeployCount, &user.CreatedAt, &lastLogin)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminUser{}, ErrNotFound
	}
	if err != nil {
		return AdminUser{}, fmt.Errorf("scan admin user: %w", err)
	}
	user.CreatedAt = user.CreatedAt.Local()
	if lastLogin.Valid {
		t := lastLogin.Time.Local()
		user.LastLoginAt = &t
	}
	return user, nil
}

func (s *SQLiteStore) ListAdminUsers(ctx context.Context) ([]AdminUser, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, email, email_verified, password_hash, is_admin, is_active, can_like, deploy_limit, deploy_count, created_at, last_login_at
		FROM admin_users ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list admin users: %w", err)
	}
	defer rows.Close()
	var users []AdminUser
	for rows.Next() {
		user, err := s.scanAdminUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *SQLiteStore) TouchAdminUserLastLogin(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE admin_users SET last_login_at = ? WHERE id = ?`, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("touch admin user last login: %w", err)
	}
	return nil
}

func (s *SQLiteStore) IncrementAdminUserDeployCount(ctx context.Context, id string) (AdminUser, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE admin_users SET deploy_count = deploy_count + 1 WHERE id = ?
	`, id)
	if err != nil {
		return AdminUser{}, fmt.Errorf("increment admin user deploy count: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return AdminUser{}, ErrNotFound
	}
	return s.GetAdminUserByID(ctx, id)
}

// TryConsumeAdminUserDeployQuota atomically reserves one new-site deployment
// for an active user. The quota predicate is part of the UPDATE so concurrent
// requests cannot all pass a read-then-increment check.
func (s *SQLiteStore) TryConsumeAdminUserDeployQuota(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE admin_users
		SET deploy_count = deploy_count + 1
		WHERE id = ? AND is_active = 1
		  AND (deploy_limit < 0 OR deploy_count < deploy_limit)
	`, id)
	if err != nil {
		return false, fmt.Errorf("consume admin user deploy quota: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check admin user deploy quota: %w", err)
	}
	return n > 0, nil
}

// ReleaseAdminUserDeployQuota rolls back one reservation made by
// TryConsumeAdminUserDeployQuota after a deployment fails or creates no site.
func (s *SQLiteStore) ReleaseAdminUserDeployQuota(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_users
		SET deploy_count = CASE WHEN deploy_count > 0 THEN deploy_count - 1 ELSE 0 END
		WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("release admin user deploy quota: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CreateAdminSession(ctx context.Context, session AdminSession) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_sessions (id, user_id, session_hash, created_at, last_used_at, expires_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.UserID, session.SessionHash, session.CreatedAt.UTC(), session.LastUsedAt.UTC(), session.ExpiresAt.UTC(), nilTime(session.RevokedAt))
	if err != nil {
		return fmt.Errorf("create admin session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetAdminSessionByHash(ctx context.Context, hash string) (AdminSession, error) {
	var session AdminSession
	var revoked sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, session_hash, created_at, last_used_at, expires_at, revoked_at
		FROM admin_sessions WHERE session_hash = ?
	`, hash).Scan(&session.ID, &session.UserID, &session.SessionHash, &session.CreatedAt, &session.LastUsedAt, &session.ExpiresAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminSession{}, ErrNotFound
	}
	if err != nil {
		return AdminSession{}, fmt.Errorf("get admin session: %w", err)
	}
	session.CreatedAt = session.CreatedAt.Local()
	session.LastUsedAt = session.LastUsedAt.Local()
	session.ExpiresAt = session.ExpiresAt.Local()
	if revoked.Valid {
		t := revoked.Time.Local()
		session.RevokedAt = &t
	}
	return session, nil
}

func (s *SQLiteStore) TouchAdminSessionLastUsed(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE admin_sessions SET last_used_at = ? WHERE id = ?`, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("touch admin session last used: %w", err)
	}
	return nil
}

func (s *SQLiteStore) RevokeAdminSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE admin_sessions SET revoked_at = ? WHERE id = ?`, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("revoke admin session: %w", err)
	}
	return nil
}

func nilTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}
