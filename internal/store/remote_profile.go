package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

type RemoteProfile struct {
	ID           string `json:"id"`
	DeviceID     string `json:"deviceId"`
	Protocol     string `json:"protocol"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	VerifyPort   int    `json:"verifyPort"`
	UsernameHint string `json:"usernameHint,omitempty"`
	Mode         string `json:"mode"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

var hostnamePattern = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`)

func normalizeRemoteProfile(item RemoteProfile) (RemoteProfile, error) {
	item.DeviceID = strings.TrimSpace(item.DeviceID)
	item.Protocol = strings.ToLower(strings.TrimSpace(item.Protocol))
	item.Host = strings.TrimSpace(item.Host)
	item.UsernameHint = strings.TrimSpace(item.UsernameHint)
	item.Mode = strings.ToLower(strings.TrimSpace(item.Mode))
	if item.DeviceID == "" {
		return RemoteProfile{}, errors.New("remote profile device is required")
	}
	defaultPort := 0
	switch item.Protocol {
	case "rdp":
		defaultPort = 3389
	case "vnc":
		defaultPort = 5900
	case "ssh":
		defaultPort = 22
	default:
		return RemoteProfile{}, errors.New("remote protocol must be rdp, vnc, or ssh")
	}
	if item.Host == "" {
		return RemoteProfile{}, errors.New("remote host is required")
	}
	if ip := net.ParseIP(item.Host); ip != nil {
		ipv4 := ip.To4()
		if ipv4 == nil || (!ipv4.IsPrivate() && !ipv4.IsLoopback() && !ipv4.IsLinkLocalUnicast()) {
			return RemoteProfile{}, errors.New("remote host must be a private IPv4 address or hostname")
		}
		item.Host = ipv4.String()
	} else {
		looksLikeIPv4 := strings.Count(item.Host, ".") == 3 && strings.Trim(item.Host, "0123456789.") == ""
		localHostname := !strings.Contains(item.Host, ".") || strings.HasSuffix(strings.ToLower(item.Host), ".local")
		if looksLikeIPv4 || !localHostname || len(item.Host) > 253 || !hostnamePattern.MatchString(item.Host) {
			return RemoteProfile{}, errors.New("remote host must be a private IPv4 address or local hostname")
		}
		item.Host = strings.ToLower(item.Host)
	}
	if item.Port == 0 {
		item.Port = defaultPort
	}
	if item.Port < 1 || item.Port > 65535 {
		return RemoteProfile{}, errors.New("remote port must be between 1 and 65535")
	}
	if item.VerifyPort == 0 {
		item.VerifyPort = item.Port
	}
	if item.VerifyPort < 1 || item.VerifyPort > 65535 {
		return RemoteProfile{}, errors.New("remote verify port must be between 1 and 65535")
	}
	if item.Mode == "" {
		item.Mode = "browser-local"
	}
	if item.Mode != "browser-local" {
		return RemoteProfile{}, errors.New("remote mode must be browser-local")
	}
	if len(item.UsernameHint) > 128 || strings.ContainsAny(item.UsernameHint, "\r\n\x00") {
		return RemoteProfile{}, errors.New("remote username hint is invalid")
	}
	return item, nil
}

func (s *Store) GetRemoteProfile(ctx context.Context, deviceID string) (RemoteProfile, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, device_id, protocol, host, port, verify_port, username_hint, mode, enabled, created_at, updated_at FROM remote_profiles WHERE device_id = ?`, strings.TrimSpace(deviceID))
	return scanRemoteProfile(row)
}

func (s *Store) ListRemoteProfiles(ctx context.Context) ([]RemoteProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, device_id, protocol, host, port, verify_port, username_hint, mode, enabled, created_at, updated_at FROM remote_profiles ORDER BY device_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]RemoteProfile, 0)
	for rows.Next() {
		item, err := scanRemoteProfile(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanRemoteProfile(scanner interface{ Scan(...any) error }) (RemoteProfile, error) {
	var item RemoteProfile
	var enabled int
	err := scanner.Scan(&item.ID, &item.DeviceID, &item.Protocol, &item.Host, &item.Port, &item.VerifyPort, &item.UsernameHint, &item.Mode, &enabled, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RemoteProfile{}, ErrNotFound
	}
	item.Enabled = enabled == 1
	return item, err
}

func (s *Store) UpsertRemoteProfile(ctx context.Context, item RemoteProfile) (RemoteProfile, error) {
	item.DeviceID = strings.TrimSpace(item.DeviceID)
	device, err := s.GetDevice(ctx, item.DeviceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RemoteProfile{}, fmt.Errorf("remote profile device: %w", ErrNotFound)
		}
		return RemoteProfile{}, err
	}
	if strings.TrimSpace(item.Host) == "" {
		item.Host = device.IPAddress
	}
	item, err = normalizeRemoteProfile(item)
	if err != nil {
		return RemoteProfile{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	existing, err := s.GetRemoteProfile(ctx, item.DeviceID)
	switch {
	case errors.Is(err, ErrNotFound):
		if item.ID == "" {
			item.ID = newID("remote")
		}
		item.CreatedAt = now
		item.UpdatedAt = now
		_, err = s.db.ExecContext(ctx, `INSERT INTO remote_profiles (id, device_id, protocol, host, port, verify_port, username_hint, mode, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.DeviceID, item.Protocol, item.Host, item.Port, item.VerifyPort, item.UsernameHint, item.Mode, boolInt(item.Enabled), item.CreatedAt, item.UpdatedAt)
	case err != nil:
		return RemoteProfile{}, err
	default:
		item.ID = existing.ID
		item.CreatedAt = existing.CreatedAt
		item.UpdatedAt = now
		_, err = s.db.ExecContext(ctx, `UPDATE remote_profiles SET protocol = ?, host = ?, port = ?, verify_port = ?, username_hint = ?, mode = ?, enabled = ?, updated_at = ? WHERE device_id = ?`, item.Protocol, item.Host, item.Port, item.VerifyPort, item.UsernameHint, item.Mode, boolInt(item.Enabled), item.UpdatedAt, item.DeviceID)
	}
	if err != nil {
		return RemoteProfile{}, normalizeDBError(err)
	}
	return s.GetRemoteProfile(ctx, item.DeviceID)
}

func (s *Store) DeleteRemoteProfile(ctx context.Context, deviceID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM remote_profiles WHERE device_id = ?`, strings.TrimSpace(deviceID))
	if err != nil {
		return normalizeDBError(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}
