package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Site struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	BroadcastAddress string `json:"broadcastAddress"`
	DefaultPort      int    `json:"defaultPort"`
	DefaultInterface string `json:"defaultInterface"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

type Device struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	MACAddress       string `json:"macAddress"`
	IPAddress        string `json:"ipAddress"`
	BroadcastAddress string `json:"broadcastAddress"`
	Port             int    `json:"port"`
	Interface        string `json:"interface"`
	SiteID           string `json:"siteId"`
	DeviceType       string `json:"deviceType"`
	Platform         string `json:"platform"`
	WakeStrategy     string `json:"wakeStrategy"`
	WakeRelayID      string `json:"wakeRelayId,omitempty"`
	VerifyPort       int    `json:"verifyPort"`
	Description      string `json:"description"`
	Enabled          bool   `json:"enabled"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

type Group struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	DeviceIDs   []string `json:"deviceIds"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

type WakeAttempt struct {
	ID                 string `json:"id"`
	TargetType         string `json:"targetType"`
	TargetID           string `json:"targetId"`
	TargetName         string `json:"targetName"`
	MACAddress         string `json:"macAddress"`
	Destination        string `json:"destination"`
	Port               int    `json:"port"`
	PacketStatus       string `json:"packetStatus"`
	VerificationStatus string `json:"verificationStatus"`
	Message            string `json:"message"`
	Packets            int    `json:"packets"`
	CreatedAt          string `json:"createdAt"`
}

type ExportData struct {
	Version    int         `json:"version"`
	Sites      []Site      `json:"sites"`
	Devices    []Device    `json:"devices"`
	Groups     []Group     `json:"groups"`
	WakeRelays []WakeRelay `json:"wakeRelays,omitempty"`
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultDatabasePath()
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS sites (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  broadcast_address TEXT NOT NULL DEFAULT '',
  default_port INTEGER NOT NULL DEFAULT 9,
  default_interface TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  mac_address TEXT NOT NULL UNIQUE,
  ip_address TEXT NOT NULL DEFAULT '',
  broadcast_address TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 0,
  interface_name TEXT NOT NULL DEFAULT '',
  site_id TEXT NOT NULL DEFAULT '',
	device_type TEXT NOT NULL DEFAULT 'unknown',
	platform TEXT NOT NULL DEFAULT 'unknown',
	wake_strategy TEXT NOT NULL DEFAULT 'broadcast',
	wake_relay_id TEXT NOT NULL DEFAULT '',
  verify_port INTEGER NOT NULL DEFAULT 0,
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE SET NULL
);
CREATE TABLE IF NOT EXISTS groups_table (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS group_members (
  group_id TEXT NOT NULL,
  device_id TEXT NOT NULL,
  position INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (group_id, device_id),
  FOREIGN KEY (group_id) REFERENCES groups_table(id) ON DELETE CASCADE,
  FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS wake_attempts (
  id TEXT PRIMARY KEY,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  target_name TEXT NOT NULL,
  mac_address TEXT NOT NULL,
  destination TEXT NOT NULL,
  port INTEGER NOT NULL,
  packet_status TEXT NOT NULL,
  verification_status TEXT NOT NULL,
  message TEXT NOT NULL DEFAULT '',
  packets INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS wake_attempts_created_idx ON wake_attempts(created_at DESC);
CREATE TABLE IF NOT EXISTS wake_relays (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  address TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 22,
  shared_secret_hash TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
`)
	if err != nil {
		return err
	}
	// Earlier releases had fewer inventory and relay columns. Keep
	// upgrades additive so an existing wol.db can be opened without a reset.
	for _, column := range []struct {
		table string
		name  string
		def   string
	}{
		{table: "devices", name: "platform", def: "TEXT NOT NULL DEFAULT 'unknown'"},
		{table: "devices", name: "wake_strategy", def: "TEXT NOT NULL DEFAULT 'broadcast'"},
		{table: "devices", name: "wake_relay_id", def: "TEXT NOT NULL DEFAULT ''"},
		{table: "wake_relays", name: "transport", def: "TEXT NOT NULL DEFAULT 'ssh_etherwake'"},
		{table: "wake_relays", name: "interface_name", def: "TEXT NOT NULL DEFAULT 'br-lan'"},
		{table: "wake_relays", name: "ssh_user", def: "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.ensureColumn(ctx, column.table, column.name, column.def); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var found bool
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
	return err
}

func (s *Store) ListSites(ctx context.Context) ([]Site, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, broadcast_address, default_port, default_interface, created_at, updated_at FROM sites ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Site, 0)
	for rows.Next() {
		var item Site
		if err := rows.Scan(&item.ID, &item.Name, &item.BroadcastAddress, &item.DefaultPort, &item.DefaultInterface, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetSite(ctx context.Context, id string) (Site, error) {
	var item Site
	err := s.db.QueryRowContext(ctx, `SELECT id, name, broadcast_address, default_port, default_interface, created_at, updated_at FROM sites WHERE id = ?`, id).Scan(&item.ID, &item.Name, &item.BroadcastAddress, &item.DefaultPort, &item.DefaultInterface, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Site{}, ErrNotFound
	}
	return item, err
}

func (s *Store) CreateSite(ctx context.Context, item Site) (Site, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.ID = newID("site")
	item.CreatedAt = now
	item.UpdatedAt = now
	if item.DefaultPort == 0 {
		item.DefaultPort = 9
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sites (id, name, broadcast_address, default_port, default_interface, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, item.ID, strings.TrimSpace(item.Name), item.BroadcastAddress, item.DefaultPort, item.DefaultInterface, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return Site{}, normalizeDBError(err)
	}
	return item, nil
}

func (s *Store) UpdateSite(ctx context.Context, id string, item Site) (Site, error) {
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE sites SET name = ?, broadcast_address = ?, default_port = ?, default_interface = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(item.Name), item.BroadcastAddress, item.DefaultPort, item.DefaultInterface, item.UpdatedAt, id)
	if err != nil {
		return Site{}, normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Site{}, ErrNotFound
	}
	return s.GetSite(ctx, id)
}

func (s *Store) DeleteSite(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE devices SET site_id = '' WHERE site_id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM sites WHERE id = ?`, id)
	if err != nil {
		tx.Rollback()
		return normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		tx.Rollback()
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) ListDevices(ctx context.Context) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, mac_address, ip_address, broadcast_address, port, interface_name, site_id, device_type, platform, wake_strategy, wake_relay_id, verify_port, description, enabled, created_at, updated_at FROM devices ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Device, 0)
	for rows.Next() {
		item, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetDevice(ctx context.Context, id string) (Device, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, mac_address, ip_address, broadcast_address, port, interface_name, site_id, device_type, platform, wake_strategy, wake_relay_id, verify_port, description, enabled, created_at, updated_at FROM devices WHERE id = ?`, id)
	return scanDevice(row)
}

func scanDevice(scanner interface{ Scan(...any) error }) (Device, error) {
	var item Device
	var enabled int
	err := scanner.Scan(&item.ID, &item.Name, &item.MACAddress, &item.IPAddress, &item.BroadcastAddress, &item.Port, &item.Interface, &item.SiteID, &item.DeviceType, &item.Platform, &item.WakeStrategy, &item.WakeRelayID, &item.VerifyPort, &item.Description, &enabled, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	item.Enabled = enabled == 1
	return item, err
}

func (s *Store) CreateDevice(ctx context.Context, item Device) (Device, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.ID = newID("device")
	item.CreatedAt = now
	item.UpdatedAt = now
	if item.Port == 0 {
		item.Port = 0
	}
	if item.DeviceType == "" {
		item.DeviceType = "unknown"
	}
	if item.Platform == "" {
		item.Platform = "unknown"
	}
	if item.WakeStrategy == "" {
		item.WakeStrategy = "broadcast"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO devices (id, name, mac_address, ip_address, broadcast_address, port, interface_name, site_id, device_type, platform, wake_strategy, wake_relay_id, verify_port, description, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, strings.TrimSpace(item.Name), strings.ToLower(item.MACAddress), item.IPAddress, item.BroadcastAddress, item.Port, item.Interface, item.SiteID, item.DeviceType, item.Platform, item.WakeStrategy, item.WakeRelayID, item.VerifyPort, item.Description, boolInt(item.Enabled), item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return Device{}, normalizeDBError(err)
	}
	return item, nil
}

func (s *Store) UpdateDevice(ctx context.Context, id string, item Device) (Device, error) {
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if item.Platform == "" {
		item.Platform = "unknown"
	}
	if item.WakeStrategy == "" {
		item.WakeStrategy = "broadcast"
	}
	result, err := s.db.ExecContext(ctx, `UPDATE devices SET name = ?, mac_address = ?, ip_address = ?, broadcast_address = ?, port = ?, interface_name = ?, site_id = ?, device_type = ?, platform = ?, wake_strategy = ?, wake_relay_id = ?, verify_port = ?, description = ?, enabled = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(item.Name), strings.ToLower(item.MACAddress), item.IPAddress, item.BroadcastAddress, item.Port, item.Interface, item.SiteID, item.DeviceType, item.Platform, item.WakeStrategy, item.WakeRelayID, item.VerifyPort, item.Description, boolInt(item.Enabled), item.UpdatedAt, id)
	if err != nil {
		return Device{}, normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Device{}, ErrNotFound
	}
	return s.GetDevice(ctx, id)
}

func (s *Store) DeleteDevice(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_members WHERE device_id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM devices WHERE id = ?`, id)
	if err != nil {
		tx.Rollback()
		return normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		tx.Rollback()
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, created_at, updated_at FROM groups_table ORDER BY name`)
	if err != nil {
		return nil, err
	}
	groups := make([]Group, 0)
	for rows.Next() {
		var item Group
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.CreatedAt, &item.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		groups = append(groups, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	// SQLite is intentionally configured with one open connection. Close the
	// parent result set before loading each group's members, otherwise the
	// nested query waits forever for the only available connection.
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range groups {
		groups[index].DeviceIDs, err = s.groupDeviceIDs(ctx, groups[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return groups, nil
}

func (s *Store) GetGroup(ctx context.Context, id string) (Group, error) {
	var item Group
	err := s.db.QueryRowContext(ctx, `SELECT id, name, description, created_at, updated_at FROM groups_table WHERE id = ?`, id).Scan(&item.ID, &item.Name, &item.Description, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	if err != nil {
		return Group{}, err
	}
	item.DeviceIDs, err = s.groupDeviceIDs(ctx, id)
	return item, err
}

func (s *Store) groupDeviceIDs(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device_id FROM group_members WHERE group_id = ? ORDER BY position, device_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) CreateGroup(ctx context.Context, item Group) (Group, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.ID = newID("group")
	item.CreatedAt = now
	item.UpdatedAt = now
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Group{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO groups_table (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, item.ID, strings.TrimSpace(item.Name), item.Description, item.CreatedAt, item.UpdatedAt); err != nil {
		tx.Rollback()
		return Group{}, normalizeDBError(err)
	}
	if err := replaceMembers(ctx, tx, item.ID, item.DeviceIDs); err != nil {
		tx.Rollback()
		return Group{}, err
	}
	if err := tx.Commit(); err != nil {
		return Group{}, err
	}
	return item, nil
}

func (s *Store) UpdateGroup(ctx context.Context, id string, item Group) (Group, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Group{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE groups_table SET name = ?, description = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(item.Name), item.Description, now, id)
	if err != nil {
		tx.Rollback()
		return Group{}, normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		tx.Rollback()
		return Group{}, ErrNotFound
	}
	if err := replaceMembers(ctx, tx, id, item.DeviceIDs); err != nil {
		tx.Rollback()
		return Group{}, err
	}
	if err := tx.Commit(); err != nil {
		return Group{}, err
	}
	return s.GetGroup(ctx, id)
}

func (s *Store) DeleteGroup(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_members WHERE group_id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM groups_table WHERE id = ?`, id)
	if err != nil {
		tx.Rollback()
		return normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		tx.Rollback()
		return ErrNotFound
	}
	return tx.Commit()
}

func replaceMembers(ctx context.Context, tx *sql.Tx, groupID string, deviceIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_members WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	for position, deviceID := range deviceIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO group_members (group_id, device_id, position) VALUES (?, ?, ?)`, groupID, deviceID, position); err != nil {
			return normalizeDBError(err)
		}
	}
	return nil
}

func (s *Store) RecordWakeAttempt(ctx context.Context, item WakeAttempt) (WakeAttempt, error) {
	if item.ID == "" {
		item.ID = newID("wake")
	}
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO wake_attempts (id, target_type, target_id, target_name, mac_address, destination, port, packet_status, verification_status, message, packets, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.TargetType, item.TargetID, item.TargetName, item.MACAddress, item.Destination, item.Port, item.PacketStatus, item.VerificationStatus, item.Message, item.Packets, item.CreatedAt)
	return item, normalizeDBError(err)
}

func (s *Store) ListWakeAttempts(ctx context.Context, limit int) ([]WakeAttempt, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, target_type, target_id, target_name, mac_address, destination, port, packet_status, verification_status, message, packets, created_at FROM wake_attempts ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WakeAttempt, 0)
	for rows.Next() {
		var item WakeAttempt
		if err := rows.Scan(&item.ID, &item.TargetType, &item.TargetID, &item.TargetName, &item.MACAddress, &item.Destination, &item.Port, &item.PacketStatus, &item.VerificationStatus, &item.Message, &item.Packets, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Export(ctx context.Context) (ExportData, error) {
	sites, err := s.ListSites(ctx)
	if err != nil {
		return ExportData{}, err
	}
	devices, err := s.ListDevices(ctx)
	if err != nil {
		return ExportData{}, err
	}
	groups, err := s.ListGroups(ctx)
	if err != nil {
		return ExportData{}, err
	}
	relays, err := s.ListWakeRelays(ctx)
	if err != nil {
		return ExportData{}, err
	}
	return ExportData{Version: 1, Sites: sites, Devices: devices, Groups: groups, WakeRelays: relays}, nil
}

func (s *Store) Import(ctx context.Context, data ExportData) error {
	if data.Version == 0 {
		data.Version = 1
	}
	if data.Version != 1 {
		return fmt.Errorf("unsupported export version %d", data.Version)
	}
	relayIDs := make(map[string]string, len(data.WakeRelays))
	for _, relay := range data.WakeRelays {
		imported, err := s.upsertWakeRelay(ctx, relay)
		if err != nil {
			return err
		}
		relayIDs[relay.ID] = imported.ID
	}
	for _, site := range data.Sites {
		if err := s.upsertSite(ctx, site); err != nil {
			return err
		}
	}
	for _, device := range data.Devices {
		if mapped, ok := relayIDs[device.WakeRelayID]; ok {
			device.WakeRelayID = mapped
		}
		if err := s.upsertDevice(ctx, device); err != nil {
			return err
		}
	}
	for _, group := range data.Groups {
		if err := s.upsertGroup(ctx, group); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) upsertSite(ctx context.Context, item Site) error {
	var existingID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM sites WHERE name = ?`, item.Name).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.CreateSite(ctx, item)
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.UpdateSite(ctx, existingID, item)
	return err
}

func (s *Store) upsertDevice(ctx context.Context, item Device) error {
	var existingID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM devices WHERE mac_address = ?`, strings.ToLower(item.MACAddress)).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.CreateDevice(ctx, item)
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.UpdateDevice(ctx, existingID, item)
	return err
}

func (s *Store) upsertGroup(ctx context.Context, item Group) error {
	var existingID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM groups_table WHERE name = ?`, item.Name).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.CreateGroup(ctx, item)
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.UpdateGroup(ctx, existingID, item)
	return err
}

var ErrNotFound = errors.New("not found")

func newID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeDBError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") || strings.Contains(message, "constraint") {
		return fmt.Errorf("record conflicts with an existing item")
	}
	return err
}

func EncodeExport(data ExportData) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}
