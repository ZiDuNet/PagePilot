package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *SQLiteStore) CreateAnonymousSession(ctx context.Context, session AnonymousSession) error {
	now := time.Now().UTC()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.LastUsedAt.IsZero() {
		session.LastUsedAt = session.CreatedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO anonymous_sessions (id, agent_id, agent_label, device_ip, user_agent, deploy_count, claimed_by_user_id, claimed_at, created_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, nullString(session.AgentID), nullString(session.AgentLabel), nullString(session.DeviceIP), nullString(session.UserAgent), session.DeployCount, nullString(session.ClaimedByUserID), nullableTime(session.ClaimedAt), session.CreatedAt.UTC(), session.LastUsedAt.UTC())
	if err != nil {
		return fmt.Errorf("create anonymous session: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetAnonymousSession(ctx context.Context, id string) (AnonymousSession, error) {
	var out AnonymousSession
	var agentID sql.NullString
	var agentLabel sql.NullString
	var deviceIP sql.NullString
	var userAgent sql.NullString
	var claimedByUserID sql.NullString
	var claimedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, agent_label, device_ip, user_agent, deploy_count, claimed_by_user_id, claimed_at, created_at, last_used_at
		FROM anonymous_sessions WHERE id = ?
	`, id).Scan(&out.ID, &agentID, &agentLabel, &deviceIP, &userAgent, &out.DeployCount, &claimedByUserID, &claimedAt, &out.CreatedAt, &out.LastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AnonymousSession{}, ErrNotFound
	}
	if err != nil {
		return AnonymousSession{}, fmt.Errorf("get anonymous session: %w", err)
	}
	if agentID.Valid {
		out.AgentID = agentID.String
	}
	if agentLabel.Valid {
		out.AgentLabel = agentLabel.String
	}
	if deviceIP.Valid {
		out.DeviceIP = deviceIP.String
	}
	if userAgent.Valid {
		out.UserAgent = userAgent.String
	}
	if claimedByUserID.Valid {
		out.ClaimedByUserID = claimedByUserID.String
	}
	if claimedAt.Valid {
		ca := claimedAt.Time
		out.ClaimedAt = &ca
	}
	out.CreatedAt = out.CreatedAt.Local()
	out.LastUsedAt = out.LastUsedAt.Local()
	return out, nil
}

func (s *SQLiteStore) UpdateAnonymousSessionMeta(ctx context.Context, id, agentID, agentLabel, deviceIP, userAgent string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE anonymous_sessions
		SET agent_id = COALESCE(NULLIF(?, ''), agent_id),
		    agent_label = COALESCE(NULLIF(?, ''), agent_label),
		    device_ip = COALESCE(NULLIF(?, ''), device_ip),
		    user_agent = COALESCE(NULLIF(?, ''), user_agent),
		    last_used_at = ?
		WHERE id = ?
	`, agentID, agentLabel, deviceIP, userAgent, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update anonymous session meta: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) IncrementAnonymousSessionDeployCount(ctx context.Context, id string) (AnonymousSession, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE anonymous_sessions
		SET deploy_count = deploy_count + 1, last_used_at = ?
		WHERE id = ? AND claimed_by_user_id IS NULL
	`, now, id)
	if err != nil {
		return AnonymousSession{}, fmt.Errorf("increment anonymous session deploy count: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return AnonymousSession{}, ErrNotFound
	}
	return s.GetAnonymousSession(ctx, id)
}

func (s *SQLiteStore) ClaimAnonymousSession(ctx context.Context, id, userID string) (AnonymousSessionClaimResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AnonymousSessionClaimResult{}, fmt.Errorf("begin claim anonymous session: %w", err)
	}
	defer tx.Rollback()

	var sess AnonymousSession
	var claimedByUserID sql.NullString
	var deployCount int
	err = tx.QueryRowContext(ctx, `
		SELECT id, deploy_count, claimed_by_user_id
		FROM anonymous_sessions WHERE id = ?
	`, id).Scan(&sess.ID, &deployCount, &claimedByUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return AnonymousSessionClaimResult{}, ErrNotFound
	}
	if err != nil {
		return AnonymousSessionClaimResult{}, fmt.Errorf("load anonymous session for claim: %w", err)
	}

	result := AnonymousSessionClaimResult{
		SessionID:      id,
		UserID:         userID,
		DeployCount:    deployCount,
		AlreadyClaimed: claimedByUserID.Valid && claimedByUserID.String == userID,
	}
	if claimedByUserID.Valid && claimedByUserID.String != "" && claimedByUserID.String != userID {
		return AnonymousSessionClaimResult{}, fmt.Errorf("anonymous session already claimed")
	}

	ownerAnon := "anon:" + id
	ownerUser := "user:" + userID
	res, err := tx.ExecContext(ctx, `
		UPDATE sites SET owner_token_id = ? WHERE owner_token_id = ?
	`, ownerUser, ownerAnon)
	if err != nil {
		return AnonymousSessionClaimResult{}, fmt.Errorf("claim anonymous sites: %w", err)
	}
	siteCount, _ := res.RowsAffected()
	result.SiteCount = int(siteCount)

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		UPDATE anonymous_sessions
		SET claimed_by_user_id = ?, claimed_at = COALESCE(claimed_at, ?), last_used_at = ?
		WHERE id = ?
	`, userID, now, now, id)
	if err != nil {
		return AnonymousSessionClaimResult{}, fmt.Errorf("mark anonymous session claimed: %w", err)
	}
	if !result.AlreadyClaimed && deployCount > 0 {
		_, err = tx.ExecContext(ctx, `
			UPDATE admin_users SET deploy_count = deploy_count + ? WHERE id = ?
		`, deployCount, userID)
		if err != nil {
			return AnonymousSessionClaimResult{}, fmt.Errorf("carry anonymous deploy count: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return AnonymousSessionClaimResult{}, fmt.Errorf("commit anonymous claim: %w", err)
	}
	return result, nil
}

func (s *SQLiteStore) ListAnonymousSessions(ctx context.Context, limit int) ([]AnonymousSession, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.agent_id, a.agent_label, a.device_ip, a.user_agent,
		       a.deploy_count, a.claimed_by_user_id, a.claimed_at, a.created_at, a.last_used_at
		FROM anonymous_sessions a
		WHERE a.deploy_count > 0
		   OR EXISTS (SELECT 1 FROM sites s WHERE s.owner_token_id = 'anon:' || a.id)
		ORDER BY a.last_used_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list anonymous sessions: %w", err)
	}
	defer rows.Close()
	out := []AnonymousSession{}
	for rows.Next() {
		var item AnonymousSession
		var agentID sql.NullString
		var agentLabel sql.NullString
		var deviceIP sql.NullString
		var userAgent sql.NullString
		var claimedByUserID sql.NullString
		var claimedAt sql.NullTime
		if err := rows.Scan(&item.ID, &agentID, &agentLabel, &deviceIP, &userAgent, &item.DeployCount, &claimedByUserID, &claimedAt, &item.CreatedAt, &item.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan anonymous session: %w", err)
		}
		if agentID.Valid {
			item.AgentID = agentID.String
		}
		if agentLabel.Valid {
			item.AgentLabel = agentLabel.String
		}
		if deviceIP.Valid {
			item.DeviceIP = deviceIP.String
		}
		if userAgent.Valid {
			item.UserAgent = userAgent.String
		}
		if claimedByUserID.Valid {
			item.ClaimedByUserID = claimedByUserID.String
		}
		if claimedAt.Valid {
			ca := claimedAt.Time
			item.ClaimedAt = &ca
		}
		item.CreatedAt = item.CreatedAt.Local()
		item.LastUsedAt = item.LastUsedAt.Local()
		out = append(out, item)
	}
	return out, rows.Err()
}

func nullableTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC()
}
