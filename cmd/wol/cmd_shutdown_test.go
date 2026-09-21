package main

import (
	"path/filepath"
	"testing"

	"github.com/aklkbqx/wol/internal/store"
)

func TestShutdownCLI(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "wol_test.db")

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	_, err = st.CreateDevice(t.Context(), store.Device{
		Name:       "test-win",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		IPAddress:  "192.168.8.200",
		Platform:   "windows",
		Enabled:    true,
	})
	if err != nil {
		st.Close()
		t.Fatalf("CreateDevice failed: %v", err)
	}
	st.Close()

	// Test missing argument
	if code := runShutdown([]string{"--db", dbPath}); code != 2 {
		t.Errorf("expected code 2 for missing machine, got %d", code)
	}

	// Test unknown machine
	if code := runShutdown([]string{"--db", dbPath, "non-existent"}); code != 2 {
		t.Errorf("expected code 2 for unknown machine, got %d", code)
	}

	// Test shutdown configure
	if code := runShutdownConfigure([]string{"--db", dbPath, "--user", "admin", "--port", "22", "--platform", "windows", "test-win"}); code != 0 {
		t.Errorf("expected code 0 for configure, got %d", code)
	}

	// Verify profile saved
	st2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("re-open store failed: %v", err)
	}
	dev, _ := st2.GetDevice(t.Context(), "device_1")
	devices, _ := st2.ListDevices(t.Context())
	if len(devices) > 0 {
		dev = devices[0]
	}
	prof, err := st2.GetPowerProfile(t.Context(), dev.ID)
	if err != nil {
		t.Fatalf("GetPowerProfile failed: %v", err)
	}
	if prof.SSHUser != "admin" || prof.Platform != "windows" {
		t.Errorf("unexpected profile: %+v", prof)
	}
	st2.Close()

	// Test shutdown status command
	if code := runShutdownStatus([]string{"--db", dbPath}); code != 0 {
		t.Errorf("expected code 0 for status, got %d", code)
	}

	// Test invalid delay flag
	if code := runShutdown([]string{"--db", dbPath, "--delay", "invalid", "test-win"}); code != 2 {
		t.Errorf("expected code 2 for invalid delay, got %d", code)
	}
}

func TestShutdownConfigureMergesUnspecifiedFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wol_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDevice(t.Context(), store.Device{Name: "box", MACAddress: "aa:bb:cc:dd:ee:11", IPAddress: "192.168.8.10", Enabled: true}); err != nil {
		st.Close()
		t.Fatal(err)
	}
	st.Close()

	if code := runShutdownConfigure([]string{"--db", dbPath, "--user", "alice", "--key", "/tmp/id_ed25519", "--platform", "linux", "--sudo", "box"}); code != 0 {
		t.Fatalf("configure exit = %d", code)
	}
	if code := runShutdownConfigure([]string{"--db", dbPath, "--port", "2222", "box"}); code != 0 {
		t.Fatalf("merge configure exit = %d", code)
	}

	st, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	devices, err := st.ListDevices(t.Context())
	if err != nil || len(devices) != 1 {
		t.Fatalf("devices = %+v err = %v", devices, err)
	}
	profile, err := st.GetPowerProfile(t.Context(), devices[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.SSHUser != "alice" || profile.SSHPort != 2222 || profile.SSHKey != "/tmp/id_ed25519" || profile.Platform != "linux" || !profile.UseSudo {
		t.Fatalf("merged profile = %+v", profile)
	}
}
