package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PowerProfile holds the SSH credentials and preferences for remote power actions.
type PowerProfile struct {
	ID        string `json:"id"`
	DeviceID  string `json:"deviceId"`
	SSHUser   string `json:"sshUser"`
	SSHPort   int    `json:"sshPort"`
	SSHKey    string `json:"sshKey,omitempty"`
	Platform  string `json:"platform"`
	UseSudo   bool   `json:"useSudo"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// PowerAttempt records an event where a power operation was sent.
type PowerAttempt struct {
	ID           string `json:"id"`
	DeviceID     string `json:"deviceId"`
	DeviceName   string `json:"deviceName"`
	Action       string `json:"action"` // shutdown_now, schedule, cancel
	DelaySeconds int    `json:"delaySeconds"`
	Status       string `json:"status"` // sent, failed
	Message      string `json:"message,omitempty"`
	CreatedAt    string `json:"createdAt"`
}

func normalizePowerProfile(item *PowerProfile) {
	item.DeviceID = strings.TrimSpace(item.DeviceID)
	item.SSHUser = strings.TrimSpace(item.SSHUser)
	item.SSHKey = strings.TrimSpace(item.SSHKey)
	item.Platform = strings.ToLower(strings.TrimSpace(item.Platform))
	if item.Platform == "" {
		item.Platform = "windows"
	}
	if item.SSHPort <= 0 || item.SSHPort > 65535 {
		item.SSHPort = 22
	}
}

// GetPowerProfile returns the stored power profile for a device.
func (s *Store) GetPowerProfile(ctx context.Context, deviceID string) (PowerProfile, error) {
	return scanPowerProfile(s.db.QueryRowContext(ctx, `SELECT id, device_id, ssh_user, ssh_port, ssh_key, platform, use_sudo, enabled, created_at, updated_at FROM power_profiles WHERE device_id = ?`, strings.TrimSpace(deviceID)))
}

func (s *Store) ListPowerProfiles(ctx context.Context) ([]PowerProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, device_id, ssh_user, ssh_port, ssh_key, platform, use_sudo, enabled, created_at, updated_at FROM power_profiles ORDER BY device_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]PowerProfile, 0)
	for rows.Next() {
		item, err := scanPowerProfile(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanPowerProfile(scanner interface{ Scan(...any) error }) (PowerProfile, error) {
	var item PowerProfile
	var useSudo, enabled int
	err := scanner.Scan(&item.ID, &item.DeviceID, &item.SSHUser, &item.SSHPort, &item.SSHKey, &item.Platform, &useSudo, &enabled, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PowerProfile{}, ErrNotFound
	}
	if err != nil {
		return PowerProfile{}, err
	}
	item.UseSudo = useSudo == 1
	item.Enabled = enabled == 1
	return item, nil
}

// UpsertPowerProfile creates or updates the power profile for a device.
func (s *Store) UpsertPowerProfile(ctx context.Context, item PowerProfile) (PowerProfile, error) {
	item.DeviceID = strings.TrimSpace(item.DeviceID)
	if item.DeviceID == "" {
		return PowerProfile{}, errors.New("power profile device ID is required")
	}
	_, err := s.GetDevice(ctx, item.DeviceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return PowerProfile{}, fmt.Errorf("power profile device: %w", ErrNotFound)
		}
		return PowerProfile{}, err
	}
	normalizePowerProfile(&item)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	existing, err := s.GetPowerProfile(ctx, item.DeviceID)
	switch {
	case errors.Is(err, ErrNotFound):
		if item.ID == "" {
			item.ID = newID("power")
		}
		item.CreatedAt = now
		item.UpdatedAt = now
		_, err = s.db.ExecContext(ctx, `INSERT INTO power_profiles (id, device_id, ssh_user, ssh_port, ssh_key, platform, use_sudo, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.DeviceID, item.SSHUser, item.SSHPort, item.SSHKey, item.Platform, boolInt(item.UseSudo), boolInt(item.Enabled), item.CreatedAt, item.UpdatedAt)
	case err != nil:
		return PowerProfile{}, err
	default:
		item.ID = existing.ID
		item.CreatedAt = existing.CreatedAt
		item.UpdatedAt = now
		_, err = s.db.ExecContext(ctx, `UPDATE power_profiles SET ssh_user = ?, ssh_port = ?, ssh_key = ?, platform = ?, use_sudo = ?, enabled = ?, updated_at = ? WHERE device_id = ?`, item.SSHUser, item.SSHPort, item.SSHKey, item.Platform, boolInt(item.UseSudo), boolInt(item.Enabled), item.UpdatedAt, item.DeviceID)
	}
	if err != nil {
		return PowerProfile{}, normalizeDBError(err)
	}
	return s.GetPowerProfile(ctx, item.DeviceID)
}

func (s *Store) upsertPowerProfileOn(ctx context.Context, q queryable, item PowerProfile) (PowerProfile, error) {
	item.DeviceID = strings.TrimSpace(item.DeviceID)
	if item.DeviceID == "" {
		return PowerProfile{}, errors.New("power profile device ID is required")
	}
	normalizePowerProfile(&item)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var existingID, createdAt string
	err := q.QueryRowContext(ctx, `SELECT id, created_at FROM power_profiles WHERE device_id = ?`, item.DeviceID).Scan(&existingID, &createdAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if item.ID == "" {
			item.ID = newID("power")
		}
		item.CreatedAt = now
		item.UpdatedAt = now
		_, err = q.ExecContext(ctx, `INSERT INTO power_profiles (id, device_id, ssh_user, ssh_port, ssh_key, platform, use_sudo, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.DeviceID, item.SSHUser, item.SSHPort, item.SSHKey, item.Platform, boolInt(item.UseSudo), boolInt(item.Enabled), item.CreatedAt, item.UpdatedAt)
	case err != nil:
		return PowerProfile{}, err
	default:
		item.ID = existingID
		item.CreatedAt = createdAt
		item.UpdatedAt = now
		_, err = q.ExecContext(ctx, `UPDATE power_profiles SET ssh_user = ?, ssh_port = ?, ssh_key = ?, platform = ?, use_sudo = ?, enabled = ?, updated_at = ? WHERE device_id = ?`, item.SSHUser, item.SSHPort, item.SSHKey, item.Platform, boolInt(item.UseSudo), boolInt(item.Enabled), item.UpdatedAt, item.DeviceID)
	}
	if err != nil {
		return PowerProfile{}, normalizeDBError(err)
	}
	return scanPowerProfile(q.QueryRowContext(ctx, `SELECT id, device_id, ssh_user, ssh_port, ssh_key, platform, use_sudo, enabled, created_at, updated_at FROM power_profiles WHERE device_id = ?`, item.DeviceID))
}

// DeletePowerProfile removes the power profile for a device.
func (s *Store) DeletePowerProfile(ctx context.Context, deviceID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM power_profiles WHERE device_id = ?`, strings.TrimSpace(deviceID))
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordPowerAttempt logs a power action attempt.
func (s *Store) RecordPowerAttempt(ctx context.Context, item PowerAttempt) (PowerAttempt, error) {
	if item.ID == "" {
		item.ID = newID("power_attempt")
	}
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO power_attempts (id, device_id, device_name, action, delay_seconds, status, message, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, strings.TrimSpace(item.DeviceID), strings.TrimSpace(item.DeviceName), strings.TrimSpace(item.Action), item.DelaySeconds, strings.TrimSpace(item.Status), strings.TrimSpace(item.Message), item.CreatedAt)
	if err != nil {
		return PowerAttempt{}, normalizeDBError(err)
	}
	return item, nil
}

// ListPowerAttempts retrieves recent power attempts up to the specified limit.
func (s *Store) ListPowerAttempts(ctx context.Context, limit int) ([]PowerAttempt, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, device_id, device_name, action, delay_seconds, status, message, created_at FROM power_attempts ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []PowerAttempt
	for rows.Next() {
		var item PowerAttempt
		if err := rows.Scan(&item.ID, &item.DeviceID, &item.DeviceName, &item.Action, &item.DelaySeconds, &item.Status, &item.Message, &item.CreatedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, item)
	}
	return attempts, rows.Err()
}
