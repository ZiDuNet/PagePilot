package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TryAcquireSiteMutationLease acquires a short-lived, code-specific lease.
// The UPSERT is one SQLite write, so independent server processes cannot both
// start a destructive mutation for the same application.
func (s *SQLiteStore) TryAcquireSiteMutationLease(ctx context.Context, code, holder string, ttl time.Duration) (bool, error) {
	code = strings.TrimSpace(code)
	holder = strings.TrimSpace(holder)
	if code == "" || holder == "" || ttl <= 0 {
		return false, fmt.Errorf("lease code, holder, and positive TTL are required")
	}
	now := time.Now()
	expiresAt := now.Add(ttl).UnixNano()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO site_mutation_leases (code, holder, expires_at)
		VALUES (?, ?, ?)
		ON CONFLICT(code) DO UPDATE SET holder = excluded.holder, expires_at = excluded.expires_at
		WHERE site_mutation_leases.expires_at <= ?
	`, code, holder, expiresAt, now.UnixNano())
	if err != nil {
		return false, fmt.Errorf("acquire site mutation lease: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check site mutation lease: %w", err)
	}
	return n > 0, nil
}

// RenewSiteMutationLease extends a lease only while the caller still owns it.
func (s *SQLiteStore) RenewSiteMutationLease(ctx context.Context, code, holder string, ttl time.Duration) (bool, error) {
	code = strings.TrimSpace(code)
	holder = strings.TrimSpace(holder)
	if code == "" || holder == "" || ttl <= 0 {
		return false, fmt.Errorf("lease code, holder, and positive TTL are required")
	}
	now := time.Now()
	expiresAt := now.Add(ttl).UnixNano()
	res, err := s.db.ExecContext(ctx, `
		UPDATE site_mutation_leases
		SET expires_at = ?
		WHERE code = ? AND holder = ? AND expires_at > ?
	`, expiresAt, code, holder, now.UnixNano())
	if err != nil {
		return false, fmt.Errorf("renew site mutation lease: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check renewed site mutation lease: %w", err)
	}
	return n > 0, nil
}

// ReleaseSiteMutationLease releases only the caller's lease. A delayed
// cleanup can therefore never release a lease acquired by another process.
func (s *SQLiteStore) ReleaseSiteMutationLease(ctx context.Context, code, holder string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM site_mutation_leases WHERE code = ? AND holder = ?
	`, strings.TrimSpace(code), strings.TrimSpace(holder))
	if err != nil {
		return fmt.Errorf("release site mutation lease: %w", err)
	}
	return nil
}
