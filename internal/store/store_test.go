package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenMigratesLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE sites (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, broadcast_address TEXT NOT NULL DEFAULT '', default_port INTEGER NOT NULL DEFAULT 9, default_interface TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE devices (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, mac_address TEXT NOT NULL UNIQUE, ip_address TEXT NOT NULL DEFAULT '', broadcast_address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, interface_name TEXT NOT NULL DEFAULT '', site_id TEXT NOT NULL DEFAULT '', device_type TEXT NOT NULL DEFAULT 'unknown', verify_port INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE groups_table (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE group_members (group_id TEXT NOT NULL, device_id TEXT NOT NULL, position INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (group_id, device_id));
CREATE TABLE wake_attempts (id TEXT PRIMARY KEY, target_type TEXT NOT NULL, target_id TEXT NOT NULL, target_name TEXT NOT NULL, mac_address TEXT NOT NULL, destination TEXT NOT NULL, port INTEGER NOT NULL, packet_status TEXT NOT NULL, verification_status TEXT NOT NULL, message TEXT NOT NULL DEFAULT '', packets INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	device, err := migrated.CreateDevice(t.Context(), Device{Name: "legacy-target", MACAddress: "aa:bb:cc:dd:ee:11", Platform: "linux", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if device.Platform != "linux" || device.WakeStrategy != "broadcast" {
		t.Fatalf("unexpected migrated device: %+v", device)
	}
	if device.RemoteURL != "" {
		t.Fatalf("legacy remote URL default = %q, want empty", device.RemoteURL)
	}
	device.RemoteURL = "https://wol.example.test/remote/legacy-target"
	if _, err := migrated.UpdateDevice(t.Context(), device.ID, device); err != nil {
		t.Fatal(err)
	}
	if relays, err := migrated.ListWakeRelays(t.Context()); err != nil || len(relays) != 0 {
		t.Fatalf("unexpected relay inventory after migration: %d, %v", len(relays), err)
	}
	if err := migrated.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.GetDevice(t.Context(), device.ID)
	if err != nil || persisted.RemoteURL != device.RemoteURL {
		t.Fatalf("remote URL after reopen = %q, %v", persisted.RemoteURL, err)
	}
}

func TestListGroupsLoadsMembersWithSingleSQLiteConnection(t *testing.T) {
	repository, err := Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	device, err := repository.CreateDevice(t.Context(), Device{
		Name:       "group-target",
		MACAddress: "aa:bb:cc:dd:ee:22",
		Platform:   "linux",
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.CreateGroup(t.Context(), Group{
		Name:        "core-machines",
		Description: "Machines used by the core test fixture",
		DeviceIDs:   []string{device.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	groups, err := repository.ListGroups(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected one group, got %d", len(groups))
	}
	if groups[0].ID != created.ID || len(groups[0].DeviceIDs) != 1 || groups[0].DeviceIDs[0] != device.ID {
		t.Fatalf("unexpected group members: %+v", groups[0])
	}
}

func TestPortableExportImportsRelayReferences(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	relay, err := source.CreateWakeRelay(t.Context(), WakeRelay{Name: "router", Address: "192.168.50.1", Port: 22, Interface: "br-lan", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	remoteURL := "https://wol.example.test/remote/windows"
	if _, err := source.CreateDevice(t.Context(), Device{Name: "windows", MACAddress: "02:00:00:00:00:5d", IPAddress: "192.168.50.200", WakeStrategy: "relay", WakeRelayID: relay.ID, RemoteURL: remoteURL, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	data, err := source.Export(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if data.Version != 2 || len(data.Devices) != 1 || len(data.WakeRelays) != 1 {
		t.Fatalf("unexpected export: %+v", data)
	}

	target, err := Open(filepath.Join(t.TempDir(), "target.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := target.Import(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	devices, err := target.ListDevices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	relays, err := target.ListWakeRelays(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || len(relays) != 1 || devices[0].WakeRelayID != relays[0].ID || devices[0].RemoteURL != remoteURL {
		t.Fatalf("relay reference was not remapped: devices=%+v relays=%+v", devices, relays)
	}
}
