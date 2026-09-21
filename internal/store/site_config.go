package store

import (
	"context"
	"fmt"
	"github.com/aklkbqx/wol/internal/netutil"
	"net"
	"strings"
	"time"
)

func normalizeSite(item Site) (Site, error) {
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		return Site{}, fmt.Errorf("site name is required")
	}
	if item.Subnet != "" {
		b, err := netutil.Broadcast(item.Subnet)
		if err != nil {
			return Site{}, err
		}
		_, n, _ := net.ParseCIDR(item.Subnet)
		item.Subnet = n.String()
		if item.BroadcastAddress == "" {
			item.BroadcastAddress = b
		}
	}
	if item.BroadcastAddress != "" && net.ParseIP(item.BroadcastAddress).To4() == nil {
		return Site{}, fmt.Errorf("broadcast must be IPv4")
	}
	if item.DefaultPort == 0 {
		item.DefaultPort = 9
	}
	if item.TimeoutMS == 0 {
		item.TimeoutMS = 2500
	}
	if item.Concurrency == 0 {
		item.Concurrency = 4
	}
	if item.DefaultPort < 1 || item.DefaultPort > 65535 {
		return Site{}, fmt.Errorf("port must be 1–65535")
	}
	if item.TimeoutMS < 100 || item.TimeoutMS > 60000 {
		return Site{}, fmt.Errorf("timeout must be 100–60000 ms")
	}
	if item.Concurrency < 1 || item.Concurrency > 16 {
		return Site{}, fmt.Errorf("concurrency must be 1–16")
	}
	return item, nil
}

// SyncDeviceAddress atomically follows a discovered address without overwriting
// unrelated edits or an explicit remote host override.
func (s *Store) SyncDeviceAddress(ctx context.Context, id, oldIP, newIP string) (bool, error) {
	if net.ParseIP(newIP).To4() == nil {
		return false, fmt.Errorf("discovered address must be IPv4")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	r, err := tx.ExecContext(ctx, `UPDATE devices SET ip_address=?, updated_at=? WHERE id=? AND ip_address=?`, newIP, now, id, oldIP)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE remote_profiles SET host=?, updated_at=? WHERE device_id=? AND host=?`, newIP, now, id, oldIP)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
