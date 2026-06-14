package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *SQLiteStore) CreateAgentBindingCode(ctx context.Context, code AgentBindingCode) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_binding_codes (code, user_id, label, created_at, expires_at, consumed_at, consumed_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, code.Code, code.UserID, nullString(code.Label), code.CreatedAt.UTC(), code.ExpiresAt.UTC(), nilTime(code.ConsumedAt), nullString(code.ConsumedBy))
	if err != nil {
		return fmt.Errorf("create agent binding code: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ConsumeAgentBindingCode(ctx context.Context, code, consumedBy string) (AgentBindingCode, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentBindingCode{}, fmt.Errorf("begin consume binding: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		SELECT code, user_id, label, created_at, expires_at, consumed_at, consumed_by
		FROM agent_binding_codes WHERE code = ?
	`, code)
	b, err := scanAgentBindingCode(row)
	if err != nil {
		return AgentBindingCode{}, err
	}
	if b.ConsumedAt != nil || time.Now().After(b.ExpiresAt) {
		return AgentBindingCode{}, ErrNotFound
	}
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE agent_binding_codes
		SET consumed_at = ?, consumed_by = ?
		WHERE code = ? AND consumed_at IS NULL AND expires_at > ?
	`, now, consumedBy, code, now)
	if err != nil {
		return AgentBindingCode{}, fmt.Errorf("consume agent binding code: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return AgentBindingCode{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return AgentBindingCode{}, fmt.Errorf("commit consume binding: %w", err)
	}
	b.ConsumedAt = &now
	b.ConsumedBy = consumedBy
	return b, nil
}

func (s *SQLiteStore) ListAgentBindingCodes(ctx context.Context, userID string, limit int) ([]AgentBindingCode, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT code, user_id, label, created_at, expires_at, consumed_at, consumed_by
		FROM agent_binding_codes
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list agent binding codes: %w", err)
	}
	defer rows.Close()
	var out []AgentBindingCode
	for rows.Next() {
		b, err := scanAgentBindingCode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanAgentBindingCode(sc scanner) (AgentBindingCode, error) {
	var b AgentBindingCode
	var label sql.NullString
	var consumedAt sql.NullTime
	var consumedBy sql.NullString
	err := sc.Scan(&b.Code, &b.UserID, &label, &b.CreatedAt, &b.ExpiresAt, &consumedAt, &consumedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentBindingCode{}, ErrNotFound
	}
	if err != nil {
		return AgentBindingCode{}, fmt.Errorf("scan agent binding code: %w", err)
	}
	if label.Valid {
		b.Label = label.String
	}
	if consumedAt.Valid {
		t := consumedAt.Time.Local()
		b.ConsumedAt = &t
	}
	if consumedBy.Valid {
		b.ConsumedBy = consumedBy.String
	}
	b.CreatedAt = b.CreatedAt.Local()
	b.ExpiresAt = b.ExpiresAt.Local()
	return b, nil
}
