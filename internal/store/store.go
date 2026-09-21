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

type queryable interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Site struct {
	Subnet           string `json:"subnet,omitempty"`
	WakeRelayID      string `json:"wakeRelayId,omitempty"`
	TimeoutMS        int    `json:"timeoutMs,omitempty"`
	Concurrency      int    `json:"concurrency,omitempty"`
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

const currentExportVersion = 6

type ExportData struct {
	Version        int             `json:"version"`
	Sites          []Site          `json:"sites"`
	Devices        []Device        `json:"devices"`
	Groups         []Group         `json:"groups"`
	WakeRelays     []WakeRelay     `json:"wakeRelays,omitempty"`
	RemoteProfiles []RemoteProfile `json:"remoteProfiles,omitempty"`
	PowerProfiles  []PowerProfile  `json:"powerProfiles,omitempty"`
}

func sqliteDSN(path string) string {
	if path == ":memory:" || strings.Contains(path, "?") {
		return path
	}
	return path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
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
	db, err := sql.Open("sqlite", sqliteDSN(path))
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
CREATE TABLE IF NOT EXISTS remote_profiles (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL UNIQUE,
  protocol TEXT NOT NULL,
  host TEXT NOT NULL,
  port INTEGER NOT NULL,
  verify_port INTEGER NOT NULL,
  username_hint TEXT NOT NULL DEFAULT '',
  domain_hint TEXT NOT NULL DEFAULT '',
  certificate_policy TEXT NOT NULL DEFAULT 'strict',
  mode TEXT NOT NULL DEFAULT 'browser-local',
  app_name TEXT NOT NULL DEFAULT '',
  fps INTEGER NOT NULL DEFAULT 0,
  resolution TEXT NOT NULL DEFAULT '',
  bitrate_kbps INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS power_profiles (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL UNIQUE,
  ssh_user TEXT NOT NULL DEFAULT '',
  ssh_port INTEGER NOT NULL DEFAULT 22,
  ssh_key TEXT NOT NULL DEFAULT '',
  platform TEXT NOT NULL DEFAULT 'windows',
  use_sudo INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS power_attempts (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  device_name TEXT NOT NULL,
  action TEXT NOT NULL,
  delay_seconds INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS power_attempts_created_idx ON power_attempts(created_at DESC);
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
		{table: "sites", name: "subnet", def: "TEXT NOT NULL DEFAULT ''"},
		{table: "sites", name: "wake_relay_id", def: "TEXT NOT NULL DEFAULT ''"},
		{table: "sites", name: "timeout_ms", def: "INTEGER NOT NULL DEFAULT 2500"},
		{table: "sites", name: "concurrency", def: "INTEGER NOT NULL DEFAULT 4"},
		{table: "devices", name: "platform", def: "TEXT NOT NULL DEFAULT 'unknown'"},
		{table: "devices", name: "wake_strategy", def: "TEXT NOT NULL DEFAULT 'broadcast'"},
		{table: "devices", name: "wake_relay_id", def: "TEXT NOT NULL DEFAULT ''"},
		{table: "wake_relays", name: "transport", def: "TEXT NOT NULL DEFAULT 'ssh_etherwake'"},
		{table: "wake_relays", name: "interface_name", def: "TEXT NOT NULL DEFAULT 'br-lan'"},
		{table: "wake_relays", name: "ssh_user", def: "TEXT NOT NULL DEFAULT ''"},
		{table: "remote_profiles", name: "domain_hint", def: "TEXT NOT NULL DEFAULT ''"},
		{table: "remote_profiles", name: "certificate_policy", def: "TEXT NOT NULL DEFAULT 'strict'"},
		{table: "remote_profiles", name: "app_name", def: "TEXT NOT NULL DEFAULT ''"},
		{table: "remote_profiles", name: "fps", def: "INTEGER NOT NULL DEFAULT 0"},
		{table: "remote_profiles", name: "resolution", def: "TEXT NOT NULL DEFAULT ''"},
		{table: "remote_profiles", name: "bitrate_kbps", def: "INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.ensureColumn(ctx, column.table, column.name, column.def); err != nil {
			return err
		}
	}
	if err := s.dropLegacyRemoteURL(ctx); err != nil {
		return err
	}
	return s.migrateRemoteProfiles(ctx)
}

func (s *Store) dropLegacyRemoteURL(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(devices)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		found = found || strings.EqualFold(name, "remote_url")
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !found {
		return nil
	}
	// Erase legacy hosted endpoints before removing the obsolete column so a
	// failed ALTER can never leave private URLs behind in the local database.
	if _, err := s.db.ExecContext(ctx, `UPDATE devices SET remote_url = ''`); err != nil {
		return fmt.Errorf("erase legacy remote URLs: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE devices DROP COLUMN remote_url`); err != nil {
		return fmt.Errorf("remove legacy remote URL storage: %w", err)
	}
	return nil
}

func (s *Store) migrateRemoteProfiles(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(remote_profiles)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if columns["mode"] && columns["domain_hint"] && columns["certificate_policy"] && columns["app_name"] && !columns["name"] && !columns["domain"] && !columns["credential_mode"] {
		return nil
	}
	domainSource := "domain_hint"
	if columns["domain"] {
		domainSource = "domain"
	} else if !columns["domain_hint"] {
		domainSource = "''"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, fmt.Sprintf(`
DROP INDEX IF EXISTS remote_profiles_device_idx;
ALTER TABLE remote_profiles RENAME TO remote_profiles_legacy_v032;
CREATE TABLE remote_profiles (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL UNIQUE,
  protocol TEXT NOT NULL,
  host TEXT NOT NULL,
  port INTEGER NOT NULL,
  verify_port INTEGER NOT NULL,
  username_hint TEXT NOT NULL DEFAULT '',
  domain_hint TEXT NOT NULL DEFAULT '',
  certificate_policy TEXT NOT NULL DEFAULT 'strict',
  mode TEXT NOT NULL DEFAULT 'browser-local',
  app_name TEXT NOT NULL DEFAULT '',
  fps INTEGER NOT NULL DEFAULT 0,
  resolution TEXT NOT NULL DEFAULT '',
  bitrate_kbps INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);
INSERT INTO remote_profiles (id, device_id, protocol, host, port, verify_port, username_hint, domain_hint, certificate_policy, mode, app_name, fps, resolution, bitrate_kbps, enabled, created_at, updated_at)
SELECT id, device_id, protocol, host, port,
       CASE WHEN verify_port > 0 THEN verify_port ELSE port END,
       username_hint, %s, 'strict', 'browser-local', '', 0, '', 0, enabled, created_at, updated_at
FROM remote_profiles_legacy_v032
WHERE rowid IN (SELECT MAX(rowid) FROM remote_profiles_legacy_v032 GROUP BY device_id);
DROP TABLE remote_profiles_legacy_v032;
`, domainSource)); err != nil {
		return fmt.Errorf("migrate remote profiles: %w", err)
	}
	return tx.Commit()
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, broadcast_address, default_port, default_interface, created_at, updated_at, subnet, wake_relay_id, timeout_ms, concurrency FROM sites ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Site, 0)
	for rows.Next() {
		var item Site
		if err := rows.Scan(&item.ID, &item.Name, &item.BroadcastAddress, &item.DefaultPort, &item.DefaultInterface, &item.CreatedAt, &item.UpdatedAt, &item.Subnet, &item.WakeRelayID, &item.TimeoutMS, &item.Concurrency); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetSite(ctx context.Context, id string) (Site, error) {
	var item Site
	err := s.db.QueryRowContext(ctx, `SELECT id, name, broadcast_address, default_port, default_interface, created_at, updated_at, subnet, wake_relay_id, timeout_ms, concurrency FROM sites WHERE id = ?`, id).Scan(&item.ID, &item.Name, &item.BroadcastAddress, &item.DefaultPort, &item.DefaultInterface, &item.CreatedAt, &item.UpdatedAt, &item.Subnet, &item.WakeRelayID, &item.TimeoutMS, &item.Concurrency)
	if errors.Is(err, sql.ErrNoRows) {
		return Site{}, ErrNotFound
	}
	return item, err
}

func (s *Store) CreateSite(ctx context.Context, item Site) (Site, error) {
	normalized, validationErr := normalizeSite(item)
	if validationErr != nil {
		return Site{}, validationErr
	}
	item = normalized
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.ID = newID("site")
	item.CreatedAt = now
	item.UpdatedAt = now
	if item.DefaultPort == 0 {
		item.DefaultPort = 9
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sites (id, name, broadcast_address, default_port, default_interface, created_at, updated_at, subnet, wake_relay_id, timeout_ms, concurrency) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, strings.TrimSpace(item.Name), item.BroadcastAddress, item.DefaultPort, item.DefaultInterface, item.CreatedAt, item.UpdatedAt, item.Subnet, item.WakeRelayID, item.TimeoutMS, item.Concurrency)
	if err != nil {
		return Site{}, normalizeDBError(err)
	}
	return item, nil
}

func (s *Store) UpdateSite(ctx context.Context, id string, item Site) (Site, error) {
	normalized, validationErr := normalizeSite(item)
	if validationErr != nil {
		return Site{}, validationErr
	}
	item = normalized
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE sites SET name = ?, broadcast_address = ?, default_port = ?, default_interface = ?, updated_at = ?, subnet = ?, wake_relay_id = ?, timeout_ms = ?, concurrency = ? WHERE id = ?`, strings.TrimSpace(item.Name), item.BroadcastAddress, item.DefaultPort, item.DefaultInterface, item.UpdatedAt, item.Subnet, item.WakeRelayID, item.TimeoutMS, item.Concurrency, id)
	if err != nil {
		return Site{}, normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Site{}, ErrNotFound
	}
	return s.GetSite(ctx, id)
}

func (s *Store) DeleteSite(ctx context.Context, id string) error {
	var used int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM devices WHERE site_id=?", id).Scan(&used); err != nil {
		return err
	}
	if used > 0 {
		return fmt.Errorf("site is assigned to %d machines; reassign them before deleting", used)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
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
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_members WHERE device_id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM remote_profiles WHERE device_id = ?`, id); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM power_profiles WHERE device_id = ?`, id); err != nil {
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
	defer tx.Rollback()
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
	defer tx.Rollback()
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
	defer tx.Rollback()
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

func replaceMembers(ctx context.Context, q queryable, groupID string, deviceIDs []string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM group_members WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	for position, deviceID := range deviceIDs {
		if _, err := q.ExecContext(ctx, `INSERT INTO group_members (group_id, device_id, position) VALUES (?, ?, ?)`, groupID, deviceID, position); err != nil {
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
	profiles, err := s.ListRemoteProfiles(ctx)
	if err != nil {
		return ExportData{}, err
	}
	powerProfiles, err := s.ListPowerProfiles(ctx)
	if err != nil {
		return ExportData{}, err
	}
	return ExportData{Version: currentExportVersion, Sites: sites, Devices: devices, Groups: groups, WakeRelays: relays, RemoteProfiles: profiles, PowerProfiles: powerProfiles}, nil
}

func (s *Store) Import(ctx context.Context, data ExportData) error {
	if data.Version == 0 {
		data.Version = 1
	}
	if data.Version < 1 || data.Version > currentExportVersion {
		return fmt.Errorf("unsupported export version %d", data.Version)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	relayIDs := make(map[string]string, len(data.WakeRelays))
	for _, relay := range data.WakeRelays {
		originalID := relay.ID
		imported, err := s.upsertWakeRelayOn(ctx, tx, relay)
		if err != nil {
			return err
		}
		if originalID != "" {
			relayIDs[originalID] = imported.ID
		}
	}

	siteIDs := make(map[string]string, len(data.Sites))
	for _, site := range data.Sites {
		if site.WakeRelayID != "" {
			mapped, ok := relayIDs[site.WakeRelayID]
			if !ok {
				return fmt.Errorf("site references unknown relay %q", site.WakeRelayID)
			}
			site.WakeRelayID = mapped
		}
		originalID := site.ID
		imported, err := s.upsertSiteOn(ctx, tx, site)
		if err != nil {
			return err
		}
		if originalID != "" {
			siteIDs[originalID] = imported.ID
		}
	}

	deviceIDs := make(map[string]string, len(data.Devices))
	for _, device := range data.Devices {
		originalID := device.ID
		if mapped, ok := relayIDs[device.WakeRelayID]; ok {
			device.WakeRelayID = mapped
		}
		if mapped, ok := siteIDs[device.SiteID]; ok {
			device.SiteID = mapped
		} else if strings.TrimSpace(device.SiteID) != "" {
			device.SiteID = ""
		}
		imported, err := s.upsertDeviceOn(ctx, tx, device)
		if err != nil {
			return err
		}
		if originalID != "" {
			deviceIDs[originalID] = imported.ID
		}
	}

	for _, group := range data.Groups {
		for index, oldID := range group.DeviceIDs {
			if mapped, ok := deviceIDs[oldID]; ok {
				group.DeviceIDs[index] = mapped
			}
		}
		if err := s.upsertGroupOn(ctx, tx, group); err != nil {
			return err
		}
	}

	for _, profile := range data.RemoteProfiles {
		mapped, ok := deviceIDs[profile.DeviceID]
		if !ok {
			return fmt.Errorf("remote profile references unknown device %q", profile.DeviceID)
		}
		profile.ID = ""
		profile.DeviceID = mapped
		if _, err := s.upsertRemoteProfileOn(ctx, tx, profile); err != nil {
			return err
		}
	}

	for _, profile := range data.PowerProfiles {
		mapped, ok := deviceIDs[profile.DeviceID]
		if !ok {
			return fmt.Errorf("power profile references unknown device %q", profile.DeviceID)
		}
		profile.ID = ""
		profile.DeviceID = mapped
		if _, err := s.upsertPowerProfileOn(ctx, tx, profile); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) upsertSite(ctx context.Context, item Site) (Site, error) {
	return s.upsertSiteOn(ctx, s.db, item)
}

func (s *Store) upsertDevice(ctx context.Context, item Device) (Device, error) {
	return s.upsertDeviceOn(ctx, s.db, item)
}

func (s *Store) upsertGroup(ctx context.Context, item Group) error {
	return s.upsertGroupOn(ctx, s.db, item)
}

func (s *Store) upsertSiteOn(ctx context.Context, q queryable, item Site) (Site, error) {
	normalized, validationErr := normalizeSite(item)
	if validationErr != nil {
		return Site{}, validationErr
	}
	item = normalized
	var existingID string
	err := q.QueryRowContext(ctx, `SELECT id FROM sites WHERE name = ?`, item.Name).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		item.ID = newID("site")
		item.CreatedAt = now
		item.UpdatedAt = now
		if item.DefaultPort == 0 {
			item.DefaultPort = 9
		}
		_, err = q.ExecContext(ctx, `INSERT INTO sites (id, name, broadcast_address, default_port, default_interface, created_at, updated_at, subnet, wake_relay_id, timeout_ms, concurrency) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, strings.TrimSpace(item.Name), item.BroadcastAddress, item.DefaultPort, item.DefaultInterface, item.CreatedAt, item.UpdatedAt, item.Subnet, item.WakeRelayID, item.TimeoutMS, item.Concurrency)
		if err != nil {
			return Site{}, normalizeDBError(err)
		}
		return item, nil
	}
	if err != nil {
		return Site{}, err
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if item.DefaultPort == 0 {
		item.DefaultPort = 9
	}
	result, err := q.ExecContext(ctx, `UPDATE sites SET name = ?, broadcast_address = ?, default_port = ?, default_interface = ?, updated_at = ?, subnet = ?, wake_relay_id = ?, timeout_ms = ?, concurrency = ? WHERE id = ?`, strings.TrimSpace(item.Name), item.BroadcastAddress, item.DefaultPort, item.DefaultInterface, item.UpdatedAt, item.Subnet, item.WakeRelayID, item.TimeoutMS, item.Concurrency, existingID)
	if err != nil {
		return Site{}, normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Site{}, ErrNotFound
	}
	item.ID = existingID
	return item, nil
}

func (s *Store) upsertDeviceOn(ctx context.Context, q queryable, item Device) (Device, error) {
	var existingID string
	err := q.QueryRowContext(ctx, `SELECT id FROM devices WHERE mac_address = ?`, strings.ToLower(item.MACAddress)).Scan(&existingID)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		item.ID = newID("device")
		item.CreatedAt = now
		item.UpdatedAt = now
		if item.DeviceType == "" {
			item.DeviceType = "unknown"
		}
		if item.Platform == "" {
			item.Platform = "unknown"
		}
		if item.WakeStrategy == "" {
			item.WakeStrategy = "broadcast"
		}
		_, err = q.ExecContext(ctx, `INSERT INTO devices (id, name, mac_address, ip_address, broadcast_address, port, interface_name, site_id, device_type, platform, wake_strategy, wake_relay_id, verify_port, description, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, strings.TrimSpace(item.Name), strings.ToLower(item.MACAddress), item.IPAddress, item.BroadcastAddress, item.Port, item.Interface, item.SiteID, item.DeviceType, item.Platform, item.WakeStrategy, item.WakeRelayID, item.VerifyPort, item.Description, boolInt(item.Enabled), item.CreatedAt, item.UpdatedAt)
		if err != nil {
			return Device{}, normalizeDBError(err)
		}
		return item, nil
	}
	if err != nil {
		return Device{}, err
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if item.Platform == "" {
		item.Platform = "unknown"
	}
	if item.WakeStrategy == "" {
		item.WakeStrategy = "broadcast"
	}
	result, err := q.ExecContext(ctx, `UPDATE devices SET name = ?, mac_address = ?, ip_address = ?, broadcast_address = ?, port = ?, interface_name = ?, site_id = ?, device_type = ?, platform = ?, wake_strategy = ?, wake_relay_id = ?, verify_port = ?, description = ?, enabled = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(item.Name), strings.ToLower(item.MACAddress), item.IPAddress, item.BroadcastAddress, item.Port, item.Interface, item.SiteID, item.DeviceType, item.Platform, item.WakeStrategy, item.WakeRelayID, item.VerifyPort, item.Description, boolInt(item.Enabled), item.UpdatedAt, existingID)
	if err != nil {
		return Device{}, normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return Device{}, ErrNotFound
	}
	return scanDevice(q.QueryRowContext(ctx, `SELECT id, name, mac_address, ip_address, broadcast_address, port, interface_name, site_id, device_type, platform, wake_strategy, wake_relay_id, verify_port, description, enabled, created_at, updated_at FROM devices WHERE id = ?`, existingID))
}

func (s *Store) upsertGroupOn(ctx context.Context, q queryable, item Group) error {
	var existingID string
	err := q.QueryRowContext(ctx, `SELECT id FROM groups_table WHERE name = ?`, item.Name).Scan(&existingID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		item.ID = newID("group")
		item.CreatedAt = now
		item.UpdatedAt = now
		if _, err := q.ExecContext(ctx, `INSERT INTO groups_table (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, item.ID, strings.TrimSpace(item.Name), item.Description, item.CreatedAt, item.UpdatedAt); err != nil {
			return normalizeDBError(err)
		}
		return replaceMembers(ctx, q, item.ID, item.DeviceIDs)
	case err != nil:
		return err
	default:
		result, err := q.ExecContext(ctx, `UPDATE groups_table SET name = ?, description = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(item.Name), item.Description, now, existingID)
		if err != nil {
			return normalizeDBError(err)
		}
		if count, _ := result.RowsAffected(); count == 0 {
			return ErrNotFound
		}
		return replaceMembers(ctx, q, existingID, item.DeviceIDs)
	}
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
