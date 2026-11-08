package scanner

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	_ "modernc.org/sqlite"
)

func TestClassifyLANAndBroadcast(t *testing.T) {
	devices := []store.Device{
		{ID: "d1", Name: "windows", MACAddress: "30:56:0f:7f:c4:5d", IPAddress: "192.168.8.10"},
		{ID: "d2", Name: "same-ip", MACAddress: "02:00:00:00:00:22", IPAddress: "192.168.8.22"},
		{ID: "d3", Name: "bad-mac", MACAddress: "invalid", IPAddress: "192.168.8.23"},
	}
	neighbors := []presence.Neighbor{
		{IP: "192.168.8.200", MAC: "30:56:0f:7f:c4:5d", Iface: "en0"},
		{IP: "192.168.8.22", MAC: "02:00:00:00:00:22", Iface: "en0"},
		{IP: "192.168.8.40", MAC: "aa:bb:cc:dd:ee:01", Iface: "en0"},
	}
	known, moved, unknown := ClassifyLAN(devices, neighbors)
	if len(known) != 1 || len(moved) != 1 || len(unknown) != 1 {
		t.Fatalf("known=%d moved=%d unknown=%d", len(known), len(moved), len(unknown))
	}
	if moved[0].Neighbor.IP != "192.168.8.200" {
		t.Fatalf("moved IP = %s", moved[0].Neighbor.IP)
	}

	if UniqueDeviceName(devices, "windows") == "windows" {
		t.Fatal("expected unique name to avoid colliding with windows")
	}
	if got := UniqueDeviceName(devices, ""); got != "lan-host" {
		t.Fatalf("UniqueDeviceName empty = %q, want lan-host", got)
	}
	if bcast := BroadcastOf("192.168.8.10"); bcast == "" {
		t.Fatal("BroadcastOf returned empty string")
	}
}

func TestSyncDeviceIPsToleratesBadSubnetAndUpdatesValidMoves(t *testing.T) {
	ctx := context.Background()
	if n, err := SyncDeviceIPs(ctx, nil, nil, nil); err != nil || n != 0 {
		t.Fatalf("SyncDeviceIPs(nil) = %d, %v", n, err)
	}

	dbPath := filepath.Join(t.TempDir(), "wol.db")
	repository, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	validSite, err := repository.CreateSite(ctx, store.Site{
		Name:   "valid-lan",
		Subnet: "192.168.10.0/24",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherSite, err := repository.CreateSite(ctx, store.Site{
		Name:   "other-subnet",
		Subnet: "192.168.20.0/24",
	})
	if err != nil {
		t.Fatal(err)
	}
	badSubnetSite, err := repository.CreateSite(ctx, store.Site{
		Name: "corrupt-subnet-site",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Inject a malformed subnet directly into SQLite to verify SyncDeviceIPs
	// never crashes or updates devices when an existing DB row has a bad subnet.
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.ExecContext(ctx, `UPDATE sites SET subnet = 'not-a-valid-cidr/99' WHERE id = ?`, badSubnetSite.ID); err != nil {
		_ = rawDB.Close()
		t.Fatal(err)
	}
	_ = rawDB.Close()

	devValid, err := repository.CreateDevice(ctx, store.Device{
		Name:       "valid-move",
		MACAddress: "02:00:00:00:01:10",
		IPAddress:  "192.168.10.10",
		SiteID:     validSite.ID,
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	devOutOfSubnet, err := repository.CreateDevice(ctx, store.Device{
		Name:       "out-of-subnet-move",
		MACAddress: "02:00:00:00:01:11",
		IPAddress:  "192.168.20.10",
		SiteID:     otherSite.ID,
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	devBadSubnet, err := repository.CreateDevice(ctx, store.Device{
		Name:       "bad-subnet-move",
		MACAddress: "02:00:00:00:01:12",
		IPAddress:  "192.168.30.10",
		SiteID:     badSubnetSite.ID,
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	devices := []store.Device{devValid, devOutOfSubnet, devBadSubnet}
	neighbors := []presence.Neighbor{
		{IP: "192.168.10.55", MAC: "02:00:00:00:01:10", Iface: "en0"},
		{IP: "192.168.99.55", MAC: "02:00:00:00:01:11", Iface: "en0"},
		{IP: "192.168.30.55", MAC: "02:00:00:00:01:12", Iface: "en0"},
	}

	updated, err := SyncDeviceIPs(ctx, repository, devices, neighbors)
	if err != nil {
		t.Fatalf("SyncDeviceIPs failed: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}
	reloaded, err := repository.GetDevice(ctx, devValid.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.IPAddress != "192.168.10.55" {
		t.Fatalf("reloaded IP = %q, want 192.168.10.55", reloaded.IPAddress)
	}
	reloadedBad, err := repository.GetDevice(ctx, devBadSubnet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedBad.IPAddress != "192.168.30.10" {
		t.Fatalf("bad-subnet device IP should not have changed, got %q", reloadedBad.IPAddress)
	}
	if got := BroadcastOf("not-an-ip"); got != "" {
		t.Fatalf("BroadcastOf(bad IP) = %q, want empty", got)
	}
}

func TestSuggestNameAndGuessIdentityWithCanceledContext(t *testing.T) {
	if got := SuggestName("invalid-ip", "02:00:00:ab:cd:ef"); got != "lan-abcdef" {
		t.Fatalf("SuggestName MAC fallback = %q, want lan-abcdef", got)
	}
	if got := SuggestName("invalid-ip", "invalid-mac"); got != "lan-host" {
		t.Fatalf("SuggestName default = %q, want lan-host", got)
	}
	if got := SuggestName("198.51.100.42", "02:00:00:00:00:01"); strings.TrimSpace(got) == "" || strings.Contains(got, "in-addr.arpa") {
		t.Fatalf("SuggestName IPv4 = %q", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	platform, port := GuessIdentity(ctx, "127.0.0.1")
	if platform != "unknown" || port != 0 {
		t.Fatalf("GuessIdentity canceled = (%q, %d), want (unknown, 0)", platform, port)
	}
}
