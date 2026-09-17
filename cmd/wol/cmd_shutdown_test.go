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
