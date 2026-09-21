package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
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
	encoded, err := json.Marshal(device)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "remoteUrl") {
		t.Fatalf("legacy remote URL leaked into device JSON: %s", encoded)
	}
	columns, err := migrated.db.QueryContext(t.Context(), `PRAGMA table_info(devices)`)
	if err != nil {
		t.Fatal(err)
	}
	defer columns.Close()
	for columns.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := columns.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "remote_url" {
			t.Fatal("legacy hosted remote storage was not removed")
		}
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
	if _, err := reopened.GetDevice(t.Context(), device.ID); err != nil {
		t.Fatalf("device after reopen: %v", err)
	}
	if _, err := reopened.GetRemoteProfile(t.Context(), device.ID); err != ErrNotFound {
		t.Fatalf("legacy public URL must not become a remote profile: %v", err)
	}
}

func TestOpenMigratesLegacyRemoteProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-remote.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE devices (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, mac_address TEXT NOT NULL UNIQUE, ip_address TEXT NOT NULL DEFAULT '', broadcast_address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 0, interface_name TEXT NOT NULL DEFAULT '', site_id TEXT NOT NULL DEFAULT '', device_type TEXT NOT NULL DEFAULT 'unknown', platform TEXT NOT NULL DEFAULT 'unknown', wake_strategy TEXT NOT NULL DEFAULT 'broadcast', wake_relay_id TEXT NOT NULL DEFAULT '', verify_port INTEGER NOT NULL DEFAULT 0, remote_url TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
INSERT INTO devices VALUES ('device-old','windows','00:11:22:33:44:55','192.168.50.200','192.168.50.255',9,'','','desktop','windows','broadcast','',3389,'https://legacy.invalid/session','',1,'now','now');
CREATE TABLE remote_profiles (id TEXT PRIMARY KEY, device_id TEXT NOT NULL, name TEXT NOT NULL, protocol TEXT NOT NULL, host TEXT NOT NULL, port INTEGER NOT NULL, verify_port INTEGER NOT NULL DEFAULT 0, username_hint TEXT NOT NULL DEFAULT '', domain TEXT NOT NULL DEFAULT '', credential_mode TEXT NOT NULL DEFAULT 'prompt', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE INDEX remote_profiles_device_idx ON remote_profiles(device_id);
INSERT INTO remote_profiles VALUES ('remote-old','device-old','Windows desktop','rdp','192.168.50.200',3389,3389,'','WORKGROUP','prompt',1,'now','now');
`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	repository, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	var legacyColumns int
	if err := repository.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name = 'remote_url'`).Scan(&legacyColumns); err != nil || legacyColumns != 0 {
		t.Fatalf("legacy remote URL column count = %d, err = %v", legacyColumns, err)
	}
	profile, err := repository.GetRemoteProfile(t.Context(), "device-old")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Mode != "browser-local" || profile.Protocol != "rdp" || profile.Host != "192.168.50.200" || profile.Port != 3389 || profile.DomainHint != "WORKGROUP" || profile.CertificatePolicy != "strict" {
		t.Fatalf("migrated profile = %+v", profile)
	}
	if _, err := repository.UpsertRemoteProfile(t.Context(), profile); err != nil {
		t.Fatalf("upsert after migration: %v", err)
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
	device, err := source.CreateDevice(t.Context(), Device{Name: "windows", MACAddress: "02:00:00:00:00:5d", IPAddress: "192.168.50.200", WakeStrategy: "relay", WakeRelayID: relay.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: "rdp", Host: "192.168.50.200", Mode: "browser-local", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	data, err := source.Export(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if data.Version != currentExportVersion || len(data.Devices) != 1 || len(data.WakeRelays) != 1 || len(data.RemoteProfiles) != 1 {
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
	profiles, err := target.ListRemoteProfiles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || len(relays) != 1 || len(profiles) != 1 || devices[0].WakeRelayID != relays[0].ID || profiles[0].DeviceID != devices[0].ID || profiles[0].Port != 3389 || profiles[0].VerifyPort != 3389 {
		t.Fatalf("relay reference was not remapped: devices=%+v relays=%+v", devices, relays)
	}
}

func TestPortableExportImportsSitesAndPowerProfiles(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	site, err := source.CreateSite(t.Context(), Site{Name: "lan", BroadcastAddress: "192.168.50.255"})
	if err != nil {
		t.Fatal(err)
	}
	device, err := source.CreateDevice(t.Context(), Device{Name: "box", MACAddress: "02:00:00:00:00:5e", IPAddress: "192.168.50.20", SiteID: site.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.UpsertPowerProfile(t.Context(), PowerProfile{DeviceID: device.ID, SSHUser: "admin", SSHPort: 2222, Platform: "linux", UseSudo: true, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	data, err := source.Export(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if data.Version != currentExportVersion || len(data.Sites) != 1 || len(data.PowerProfiles) != 1 {
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
	sites, err := target.ListSites(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	devices, err := target.ListDevices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := target.ListPowerProfiles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || len(devices) != 1 || len(profiles) != 1 {
		t.Fatalf("imported counts: sites=%d devices=%d profiles=%d", len(sites), len(devices), len(profiles))
	}
	if devices[0].SiteID != sites[0].ID {
		t.Fatalf("site id was not remapped: device=%+v site=%+v", devices[0], sites[0])
	}
	if profiles[0].DeviceID != devices[0].ID || profiles[0].SSHUser != "admin" || profiles[0].SSHPort != 2222 || profiles[0].Platform != "linux" || !profiles[0].UseSudo {
		t.Fatalf("power profile was not imported: %+v", profiles[0])
	}
}

func TestImportRollsBackOnInvalidRemoteProfile(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err := source.CreateDevice(t.Context(), Device{Name: "box", MACAddress: "02:00:00:00:00:5f", IPAddress: "192.168.50.21", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	data, err := source.Export(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	data.RemoteProfiles = []RemoteProfile{{
		DeviceID: "missing-device",
		Protocol: "rdp",
		Host:     "192.168.50.21",
		Mode:     "browser-local",
		Enabled:  true,
	}}

	target, err := Open(filepath.Join(t.TempDir(), "target.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := target.Import(t.Context(), data); err == nil {
		t.Fatal("expected import to fail")
	}
	devices, err := target.ListDevices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 0 {
		t.Fatalf("partial import was committed: %+v", devices)
	}
}

func TestRemoteProfileCRUDValidationAndDeviceCleanup(t *testing.T) {
	repository, err := Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	device, err := repository.CreateDevice(t.Context(), Device{Name: "desktop", MACAddress: "aa:bb:cc:dd:ee:33", IPAddress: "192.168.50.33", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: " RDP ", Host: "192.168.008.033", UsernameHint: "DOMAIN\\user", Enabled: true})
	if err == nil {
		t.Fatalf("non-canonical IPv4 should not be accepted: %+v", created)
	}
	created, err = repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: " RDP ", UsernameHint: "DOMAIN\\user", DomainHint: "WORK", CertificatePolicy: "trust-local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if created.Protocol != "rdp" || created.Host != device.IPAddress || created.Port != 3389 || created.VerifyPort != 3389 || created.Mode != "native" || created.DomainHint != "WORK" || created.CertificatePolicy != "trust-local" {
		t.Fatalf("profile defaults were not normalized: %+v", created)
	}
	updated, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: "vnc", Host: "10.0.0.8", Port: 5901, Mode: "browser-local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.CreatedAt != created.CreatedAt || updated.Port != 5901 || updated.VerifyPort != 5901 {
		t.Fatalf("profile upsert did not preserve identity: before=%+v after=%+v", created, updated)
	}
	profiles, err := repository.ListRemoteProfiles(t.Context())
	if err != nil || len(profiles) != 1 {
		t.Fatalf("list remote profiles: %+v, %v", profiles, err)
	}
	if _, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: "rdp", Host: "8.8.8.8", Enabled: true}); err == nil {
		t.Fatal("public IPv4 remote host must be rejected")
	}
	if _, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: "rdp", Host: "remote.example.test", Enabled: true}); err == nil {
		t.Fatal("public remote hostname must be rejected")
	}
	if _, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: "rdp", Host: device.IPAddress, Mode: "hosted", Enabled: true}); err == nil {
		t.Fatal("unsupported hosted mode must be rejected")
	}
	if _, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: "https", Host: "desktop.local", Enabled: true}); err == nil {
		t.Fatal("web URL protocol must be rejected")
	}
	if _, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: "rdp", Host: device.IPAddress, CertificatePolicy: "ignore-all", Enabled: true}); err == nil {
		t.Fatal("unknown certificate policy must be rejected")
	}
	sunshineProfile, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{
		DeviceID:    device.ID,
		Protocol:    "sunshine",
		FPS:         120,
		Resolution:  "2560x1440",
		BitrateKbps: 50000,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("sunshine profile upsert failed: %v", err)
	}
	if sunshineProfile.Protocol != "sunshine" || sunshineProfile.Port != 47989 || sunshineProfile.VerifyPort != 47989 || sunshineProfile.Mode != "native-moonlight" || sunshineProfile.AppName != "Desktop" || sunshineProfile.FPS != 120 || sunshineProfile.Resolution != "2560x1440" || sunshineProfile.BitrateKbps != 50000 {
		t.Fatalf("sunshine profile defaults were not normalized: %+v", sunshineProfile)
	}
	if _, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: device.ID, Protocol: "sunshine", Resolution: "invalid-res", Enabled: true}); err == nil {
		t.Fatal("invalid resolution format must be rejected")
	}
	if err := repository.DeleteDevice(t.Context(), device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetRemoteProfile(t.Context(), device.ID); err != ErrNotFound {
		t.Fatalf("profile survived device deletion: %v", err)
	}
}

func TestDeleteRemoteProfileAndOlderImports(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			repository, err := Open(filepath.Join(t.TempDir(), "wol.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repository.Close()
			data := ExportData{Version: version, Devices: []Device{{ID: "old-device", Name: "legacy", MACAddress: "aa:bb:cc:dd:ee:44", Enabled: true}}}
			if err := repository.Import(t.Context(), data); err != nil {
				t.Fatalf("import version %d: %v", version, err)
			}
			devices, err := repository.ListDevices(t.Context())
			if err != nil || len(devices) != 1 {
				t.Fatalf("devices after version %d import: %+v, %v", version, devices, err)
			}
			if _, err := repository.UpsertRemoteProfile(t.Context(), RemoteProfile{DeviceID: devices[0].ID, Protocol: "ssh", Host: "nas.local", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			if err := repository.DeleteRemoteProfile(t.Context(), devices[0].ID); err != nil {
				t.Fatal(err)
			}
			if err := repository.DeleteRemoteProfile(t.Context(), devices[0].ID); err != ErrNotFound {
				t.Fatalf("second delete = %v, want ErrNotFound", err)
			}
		})
	}
}
