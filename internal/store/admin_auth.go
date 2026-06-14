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
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count admin users: %w", err)
	}
	return n, nil
}

func (s *SQLiteStore) CreateAdminUser(ctx context.Context, user AdminUser) error {
	if user.DeployLimit == 0 {
		user.DeployLimit = 20
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_users (id, username, password_hash, is_admin, is_active, can_like, deploy_limit, deploy_count, created_at, last_login_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, user.ID, user.Username, user.PasswordHash, user.IsAdmin, user.IsActive, user.CanLike, user.DeployLimit, user.DeployCount, user.CreatedAt.UTC(), nilTime(user.LastLoginAt))
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateAdminUser(ctx context.Context, user AdminUser) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE admin_users
		SET username = ?, is_admin = ?, is_active = ?, can_like = ?, deploy_limit = ?
		WHERE id = ?
	`, user.Username, user.IsAdmin, user.IsActive, user.CanLike, user.DeployLimit, user.ID)
	if err != nil {
		return fmt.Errorf("update admin user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) UpdateAdminUserPassword(ctx context.Context, id, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `
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
	return nil
}

func (s *SQLiteStore) DeleteAdminUser(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete admin user: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE tokens SET is_revoked = 1 WHERE owner_user_id = ?`, id); err != nil {
		return fmt.Errorf("revoke user tokens: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE admin_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, time.Now().UTC(), id); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_binding_codes WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("delete user binding codes: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM admin_users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete admin user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete admin user: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetAdminUserByUsername(ctx context.Context, username string) (AdminUser, error) {
	return s.scanAdminUser(s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, is_admin, is_active, can_like, deploy_limit, deploy_count, created_at, last_login_at
		FROM admin_users WHERE username = ?
	`, username))
}

func (s *SQLiteStore) GetAdminUserByID(ctx context.Context, id string) (AdminUser, error) {
	return s.scanAdminUser(s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, is_admin, is_active, can_like, deploy_limit, deploy_count, created_at, last_login_at
		FROM admin_users WHERE id = ?
	`, id))
}

func (s *SQLiteStore) scanAdminUser(row scanner) (AdminUser, error) {
	var user AdminUser
	var lastLogin sql.NullTime
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin, &user.IsActive, &user.CanLike, &user.DeployLimit, &user.DeployCount, &user.CreatedAt, &lastLogin)
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
		SELECT id, username, password_hash, is_admin, is_active, can_like, deploy_limit, deploy_count, created_at, last_login_at
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
