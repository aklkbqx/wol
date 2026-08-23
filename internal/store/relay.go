package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// WakeRelay is a router reachable over SSH that can run etherwake.
type WakeRelay struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Address          string `json:"address"`
	Port             int    `json:"port"`
	Transport        string `json:"transport"`
	Interface        string `json:"interface"`
	SSHUser          string `json:"sshUser,omitempty"`
	SharedSecretHash string `json:"-"`
	Enabled          bool   `json:"enabled"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

func (s *Store) ListWakeRelays(ctx context.Context) ([]WakeRelay, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, address, port, transport, interface_name, ssh_user, shared_secret_hash, enabled, created_at, updated_at FROM wake_relays ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WakeRelay, 0)
	for rows.Next() {
		item, err := scanWakeRelay(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetWakeRelay(ctx context.Context, id string) (WakeRelay, error) {
	return scanWakeRelay(s.db.QueryRowContext(ctx, `SELECT id, name, address, port, transport, interface_name, ssh_user, shared_secret_hash, enabled, created_at, updated_at FROM wake_relays WHERE id = ?`, id))
}

func scanWakeRelay(scanner interface{ Scan(...any) error }) (WakeRelay, error) {
	var item WakeRelay
	var enabled int
	err := scanner.Scan(&item.ID, &item.Name, &item.Address, &item.Port, &item.Transport, &item.Interface, &item.SSHUser, &item.SharedSecretHash, &enabled, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return WakeRelay{}, ErrNotFound
	}
	item.Enabled = enabled == 1
	if item.Transport == "" {
		item.Transport = "ssh_etherwake"
	}
	if item.Interface == "" {
		item.Interface = "br-lan"
	}
	return item, err
}

func (s *Store) CreateWakeRelay(ctx context.Context, item WakeRelay) (WakeRelay, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.ID, item.CreatedAt, item.UpdatedAt = newID("relay"), now, now
	normalizeWakeRelay(&item)
	_, err := s.db.ExecContext(ctx, `INSERT INTO wake_relays (id, name, address, port, transport, interface_name, ssh_user, shared_secret_hash, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, strings.TrimSpace(item.Name), strings.TrimSpace(item.Address), item.Port, item.Transport, item.Interface, item.SSHUser, item.SharedSecretHash, boolInt(item.Enabled), item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return WakeRelay{}, normalizeDBError(err)
	}
	return item, nil
}

func (s *Store) upsertWakeRelay(ctx context.Context, item WakeRelay) (WakeRelay, error) {
	var existingID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM wake_relays WHERE name = ?`, strings.TrimSpace(item.Name)).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		return s.CreateWakeRelay(ctx, item)
	}
	if err != nil {
		return WakeRelay{}, err
	}
	return s.UpdateWakeRelay(ctx, existingID, item)
}

func (s *Store) UpdateWakeRelay(ctx context.Context, id string, item WakeRelay) (WakeRelay, error) {
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	normalizeWakeRelay(&item)
	result, err := s.db.ExecContext(ctx, `UPDATE wake_relays SET name = ?, address = ?, port = ?, transport = ?, interface_name = ?, ssh_user = ?, shared_secret_hash = ?, enabled = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(item.Name), strings.TrimSpace(item.Address), item.Port, item.Transport, item.Interface, item.SSHUser, item.SharedSecretHash, boolInt(item.Enabled), item.UpdatedAt, id)
	if err != nil {
		return WakeRelay{}, normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return WakeRelay{}, ErrNotFound
	}
	return s.GetWakeRelay(ctx, id)
}

func (s *Store) DeleteWakeRelay(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE devices SET wake_relay_id = '' WHERE wake_relay_id = ?`, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM wake_relays WHERE id = ?`, id)
	if err != nil {
		_ = tx.Rollback()
		return normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		_ = tx.Rollback()
		return ErrNotFound
	}
	return tx.Commit()
}

func normalizeWakeRelay(item *WakeRelay) {
	if item.Port == 0 {
		item.Port = 22
	}
	if item.Transport == "" {
		item.Transport = "ssh_etherwake"
	}
	if item.Interface == "" {
		item.Interface = "br-lan"
	}
}
