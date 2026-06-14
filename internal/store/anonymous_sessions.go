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
		INSERT INTO anonymous_sessions (id, agent_id, agent_label, device_ip, user_agent, deploy_count, created_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, nullString(session.AgentID), nullString(session.AgentLabel), nullString(session.DeviceIP), nullString(session.UserAgent), session.DeployCount, session.CreatedAt.UTC(), session.LastUsedAt.UTC())
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
	err := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, agent_label, device_ip, user_agent, deploy_count, created_at, last_used_at
		FROM anonymous_sessions WHERE id = ?
	`, id).Scan(&out.ID, &agentID, &agentLabel, &deviceIP, &userAgent, &out.DeployCount, &out.CreatedAt, &out.LastUsedAt)
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
		WHERE id = ?
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

func (s *SQLiteStore) ListAnonymousSessions(ctx context.Context, limit int) ([]AnonymousSession, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, agent_label, device_ip, user_agent, deploy_count, created_at, last_used_at
		FROM anonymous_sessions
		ORDER BY last_used_at DESC
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
		if err := rows.Scan(&item.ID, &agentID, &agentLabel, &deviceIP, &userAgent, &item.DeployCount, &item.CreatedAt, &item.LastUsedAt); err != nil {
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
		item.CreatedAt = item.CreatedAt.Local()
		item.LastUsedAt = item.LastUsedAt.Local()
		out = append(out, item)
	}
	return out, rows.Err()
}
