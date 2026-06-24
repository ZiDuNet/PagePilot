package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *SQLiteStore) CreateScreenPairing(ctx context.Context, pairing ScreenPairing) error {
	now := time.Now().UTC()
	if pairing.CreatedAt.IsZero() {
		pairing.CreatedAt = now
	}
	if pairing.ExpiresAt.IsZero() {
		pairing.ExpiresAt = now.Add(5 * time.Minute)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin screen pairing: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM screen_pairings WHERE expires_at <= ? AND consumed_at IS NULL
	`, now); err != nil {
		return fmt.Errorf("cleanup expired screen pairings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM screens
		WHERE status = 'pairing'
		  AND (owner_user_id IS NULL OR owner_user_id = '')
		  AND id NOT IN (SELECT screen_id FROM screen_pairings)
	`); err != nil {
		return fmt.Errorf("cleanup expired pairing screens: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO screens (id, name, device_name, status, device_info, created_at, updated_at)
		VALUES (?, ?, ?, 'pairing', ?, ?, ?)
	`, pairing.ScreenID, pairing.DeviceName, pairing.DeviceName, normalizeDeviceInfo(pairing.DeviceInfo), pairing.CreatedAt.UTC(), pairing.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert screen: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO screen_pairings
		  (id, code, pairing_secret_hash, screen_id, device_name, expires_at, consumed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, pairing.ID, pairing.Code, pairing.PairingSecretHash, pairing.ScreenID, pairing.DeviceName,
		pairing.ExpiresAt.UTC(), nilTime(pairing.ConsumedAt), pairing.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert screen pairing: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit screen pairing: %w", err)
	}
	return nil
}

func (s *SQLiteStore) BindScreenPairing(ctx context.Context, code, ownerUserID, name string) (Screen, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Screen{}, fmt.Errorf("begin bind screen pairing: %w", err)
	}
	defer tx.Rollback()

	var pairing ScreenPairing
	err = tx.QueryRowContext(ctx, `
		SELECT id, code, pairing_secret_hash, screen_id, device_name, expires_at, consumed_at, created_at
		FROM screen_pairings
		WHERE code = ? AND consumed_at IS NULL AND expires_at > ?
	`, code, now).Scan(
		&pairing.ID,
		&pairing.Code,
		&pairing.PairingSecretHash,
		&pairing.ScreenID,
		&pairing.DeviceName,
		&pairing.ExpiresAt,
		new(sql.NullTime),
		&pairing.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Screen{}, ErrNotFound
	}
	if err != nil {
		return Screen{}, fmt.Errorf("load screen pairing: %w", err)
	}
	if name == "" {
		name = pairing.DeviceName
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE screens
		SET owner_user_id = ?, name = ?, status = 'bound', updated_at = ?
		WHERE id = ? AND (owner_user_id IS NULL OR owner_user_id = '') AND revoked_at IS NULL
	`, ownerUserID, name, now, pairing.ScreenID)
	if err != nil {
		return Screen{}, fmt.Errorf("bind screen owner: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Screen{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE screen_pairings SET consumed_at = ? WHERE id = ?
	`, now, pairing.ID); err != nil {
		return Screen{}, fmt.Errorf("consume screen pairing: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Screen{}, fmt.Errorf("commit bind screen pairing: %w", err)
	}
	return s.GetScreen(ctx, pairing.ScreenID)
}

func (s *SQLiteStore) CompleteScreenPairing(ctx context.Context, pairingID, pairingSecretHash, deviceTokenHash string) error {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin complete screen pairing: %w", err)
	}
	defer tx.Rollback()

	var screenID string
	var ownerUserID sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT p.screen_id, sc.owner_user_id
		FROM screen_pairings p
		JOIN screens sc ON sc.id = p.screen_id
		WHERE p.id = ? AND p.pairing_secret_hash = ? AND p.consumed_at IS NOT NULL
	`, pairingID, pairingSecretHash).Scan(&screenID, &ownerUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load completed screen pairing: %w", err)
	}
	if !ownerUserID.Valid || ownerUserID.String == "" {
		return ErrNotFound
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE screens
		SET device_token_hash = ?, status = 'online', updated_at = ?, last_seen_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, deviceTokenHash, now, now, screenID)
	if err != nil {
		return fmt.Errorf("store screen device token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit complete screen pairing: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetScreen(ctx context.Context, id string) (Screen, error) {
	return s.scanScreen(s.db.QueryRowContext(ctx, screenSelectSQL()+` WHERE id = ? AND revoked_at IS NULL`, id))
}

func (s *SQLiteStore) GetScreenByDeviceTokenHash(ctx context.Context, hash string) (Screen, error) {
	return s.scanScreen(s.db.QueryRowContext(ctx, screenSelectSQL()+` WHERE device_token_hash = ? AND revoked_at IS NULL`, hash))
}

func (s *SQLiteStore) ListScreensByUser(ctx context.Context, ownerUserID string) ([]Screen, error) {
	rows, err := s.db.QueryContext(ctx, screenSelectSQL()+`
		WHERE owner_user_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list user screens: %w", err)
	}
	defer rows.Close()
	return scanScreens(rows)
}

func (s *SQLiteStore) ListScreens(ctx context.Context) ([]Screen, error) {
	rows, err := s.db.QueryContext(ctx, screenSelectSQL()+`
		WHERE revoked_at IS NULL
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list screens: %w", err)
	}
	defer rows.Close()
	return scanScreens(rows)
}

func (s *SQLiteStore) PublishScreen(ctx context.Context, screenID, ownerUserID, siteCode string, version *int64) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE screens
		SET current_site_code = ?, current_version = ?, updated_at = ?
		WHERE id = ? AND owner_user_id = ? AND revoked_at IS NULL
	`, siteCode, nilInt64(version), now, screenID, ownerUserID)
	if err != nil {
		return fmt.Errorf("publish screen: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) TouchScreenHeartbeat(ctx context.Context, screenID, appVersion, runtime, deviceInfo string) (Screen, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE screens
		SET status = 'online', last_seen_at = ?, app_version = ?, runtime = ?, device_info = ?, updated_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, now, appVersion, runtime, normalizeDeviceInfo(deviceInfo), now, screenID)
	if err != nil {
		return Screen{}, fmt.Errorf("touch screen heartbeat: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Screen{}, ErrNotFound
	}
	return s.GetScreen(ctx, screenID)
}

func (s *SQLiteStore) RequestScreenScreenshot(ctx context.Context, screenID, requestID string) (Screen, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE screens
		SET screenshot_request_id = ?, screenshot_requested_at = ?, updated_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, requestID, now, now, screenID)
	if err != nil {
		return Screen{}, fmt.Errorf("request screen screenshot: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Screen{}, ErrNotFound
	}
	return s.GetScreen(ctx, screenID)
}

func (s *SQLiteStore) CompleteScreenScreenshot(ctx context.Context, screenID, requestID string, screenshotAt time.Time) (Screen, error) {
	now := screenshotAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE screens
		SET screenshot_request_id = '', screenshot_requested_at = NULL, screenshot_at = ?, updated_at = ?
		WHERE id = ? AND screenshot_request_id = ? AND revoked_at IS NULL
	`, now, now, screenID, requestID)
	if err != nil {
		return Screen{}, fmt.Errorf("complete screen screenshot: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Screen{}, ErrNotFound
	}
	return s.GetScreen(ctx, screenID)
}

func (s *SQLiteStore) RequestScreenCommand(ctx context.Context, screenID, requestID, commandType, payload string) (Screen, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE screens
		SET command_request_id = ?, command_type = ?, command_payload = ?, command_requested_at = ?, updated_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, requestID, commandType, normalizeDeviceInfo(payload), now, now, screenID)
	if err != nil {
		return Screen{}, fmt.Errorf("request screen command: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Screen{}, ErrNotFound
	}
	return s.GetScreen(ctx, screenID)
}

func (s *SQLiteStore) CompleteScreenCommand(ctx context.Context, screenID, requestID string, completedAt time.Time) (Screen, error) {
	now := completedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE screens
		SET command_request_id = '', command_type = '', command_payload = '{}',
		    command_completed_at = ?, updated_at = ?
		WHERE id = ? AND command_request_id = ? AND revoked_at IS NULL
	`, now, now, screenID, requestID)
	if err != nil {
		return Screen{}, fmt.Errorf("complete screen command: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Screen{}, ErrNotFound
	}
	return s.GetScreen(ctx, screenID)
}

func (s *SQLiteStore) UnbindScreen(ctx context.Context, screenID, ownerUserID string) error {
	now := time.Now().UTC()
	if ownerUserID == "" {
		res, err := s.db.ExecContext(ctx, `
			UPDATE screens
			SET revoked_at = ?, status = 'revoked', device_token_hash = NULL, updated_at = ?
			WHERE id = ? AND (owner_user_id IS NULL OR owner_user_id = '') AND revoked_at IS NULL
		`, now, now, screenID)
		if err != nil {
			return fmt.Errorf("unbind unowned screen: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrNotFound
		}
		return nil
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE screens
		SET revoked_at = ?, status = 'revoked', device_token_hash = NULL, updated_at = ?
		WHERE id = ? AND owner_user_id = ? AND revoked_at IS NULL
	`, now, now, screenID, ownerUserID)
	if err != nil {
		return fmt.Errorf("unbind screen: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func screenSelectSQL() string {
	return `
		SELECT id, owner_user_id, name, device_name, device_token_hash, status,
		       current_site_code, current_version, last_seen_at, app_version, runtime,
		       device_info, screenshot_request_id, screenshot_requested_at, screenshot_at,
		       command_request_id, command_type, command_payload, command_requested_at,
		       command_completed_at,
		       created_at, updated_at, revoked_at
		FROM screens`
}

func (s *SQLiteStore) scanScreen(row scanner) (Screen, error) {
	var screen Screen
	err := scanScreenRow(row, &screen)
	if errors.Is(err, sql.ErrNoRows) {
		return Screen{}, ErrNotFound
	}
	if err != nil {
		return Screen{}, err
	}
	return screen, nil
}

func scanScreens(rows *sql.Rows) ([]Screen, error) {
	var out []Screen
	for rows.Next() {
		var screen Screen
		if err := scanScreenRow(rows, &screen); err != nil {
			return nil, err
		}
		out = append(out, screen)
	}
	return out, rows.Err()
}

func scanScreenRow(row scanner, screen *Screen) error {
	var ownerUserID sql.NullString
	var deviceTokenHash sql.NullString
	var screenshotRequestID sql.NullString
	var commandRequestID sql.NullString
	var commandType sql.NullString
	var commandPayload sql.NullString
	var currentVersion sql.NullInt64
	var lastSeenAt sql.NullTime
	var screenshotRequestedAt sql.NullTime
	var screenshotAt sql.NullTime
	var commandRequestedAt sql.NullTime
	var commandCompletedAt sql.NullTime
	var revokedAt sql.NullTime
	err := row.Scan(
		&screen.ID,
		&ownerUserID,
		&screen.Name,
		&screen.DeviceName,
		&deviceTokenHash,
		&screen.Status,
		&screen.CurrentSiteCode,
		&currentVersion,
		&lastSeenAt,
		&screen.AppVersion,
		&screen.Runtime,
		&screen.DeviceInfo,
		&screenshotRequestID,
		&screenshotRequestedAt,
		&screenshotAt,
		&commandRequestID,
		&commandType,
		&commandPayload,
		&commandRequestedAt,
		&commandCompletedAt,
		&screen.CreatedAt,
		&screen.UpdatedAt,
		&revokedAt,
	)
	if err != nil {
		return err
	}
	if ownerUserID.Valid {
		screen.OwnerUserID = ownerUserID.String
	}
	if deviceTokenHash.Valid {
		screen.DeviceTokenHash = deviceTokenHash.String
	}
	if screenshotRequestID.Valid {
		screen.ScreenshotRequestID = screenshotRequestID.String
	}
	if commandRequestID.Valid {
		screen.CommandRequestID = commandRequestID.String
	}
	if commandType.Valid {
		screen.CommandType = commandType.String
	}
	if commandPayload.Valid {
		screen.CommandPayload = commandPayload.String
	}
	if currentVersion.Valid {
		v := currentVersion.Int64
		screen.CurrentVersion = &v
	}
	if lastSeenAt.Valid {
		t := lastSeenAt.Time.Local()
		screen.LastSeenAt = &t
	}
	if screenshotRequestedAt.Valid {
		t := screenshotRequestedAt.Time.Local()
		screen.ScreenshotRequestedAt = &t
	}
	if screenshotAt.Valid {
		t := screenshotAt.Time.Local()
		screen.ScreenshotAt = &t
	}
	if commandRequestedAt.Valid {
		t := commandRequestedAt.Time.Local()
		screen.CommandRequestedAt = &t
	}
	if commandCompletedAt.Valid {
		t := commandCompletedAt.Time.Local()
		screen.CommandCompletedAt = &t
	}
	if revokedAt.Valid {
		t := revokedAt.Time.Local()
		screen.RevokedAt = &t
	}
	screen.CreatedAt = screen.CreatedAt.Local()
	screen.UpdatedAt = screen.UpdatedAt.Local()
	if screen.DeviceInfo == "" {
		screen.DeviceInfo = "{}"
	}
	if screen.CommandPayload == "" {
		screen.CommandPayload = "{}"
	}
	return nil
}

func normalizeDeviceInfo(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 8192 || !json.Valid([]byte(value)) {
		return "{}"
	}
	return value
}

func nilInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
