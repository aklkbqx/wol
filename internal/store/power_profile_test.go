package store

import (
	"path/filepath"
	"testing"
)

func TestPowerProfileAndAttempts(t *testing.T) {
	ctx := t.Context()
	dbPath := filepath.Join(t.TempDir(), "wol_power_test.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	// Create device
	dev, err := st.CreateDevice(ctx, Device{
		Name:       "win-desktop",
		MACAddress: "11:22:33:44:55:66",
		IPAddress:  "192.168.8.200",
		Platform:   "windows",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateDevice failed: %v", err)
	}

	// Test UpsertPowerProfile (Insert)
	profile, err := st.UpsertPowerProfile(ctx, PowerProfile{
		DeviceID: dev.ID,
		SSHUser:  "admin",
		SSHPort:  22,
		Platform: "windows",
		UseSudo:  false,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("UpsertPowerProfile insert failed: %v", err)
	}
	if profile.SSHUser != "admin" || profile.Platform != "windows" {
		t.Errorf("unexpected profile: %+v", profile)
	}

	// Test GetPowerProfile
	fetched, err := st.GetPowerProfile(ctx, dev.ID)
	if err != nil {
		t.Fatalf("GetPowerProfile failed: %v", err)
	}
	if fetched.ID != profile.ID || fetched.SSHUser != "admin" {
		t.Errorf("GetPowerProfile mismatch: got %+v, want %+v", fetched, profile)
	}

	// Test UpsertPowerProfile (Update)
	updated, err := st.UpsertPowerProfile(ctx, PowerProfile{
		DeviceID: dev.ID,
		SSHUser:  "aklkbqx",
		SSHPort:  2222,
		Platform: "windows",
		UseSudo:  true,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("UpsertPowerProfile update failed: %v", err)
	}
	if updated.SSHUser != "aklkbqx" || updated.SSHPort != 2222 || !updated.UseSudo {
		t.Errorf("unexpected updated profile: %+v", updated)
	}

	// Test RecordPowerAttempt
	attempt, err := st.RecordPowerAttempt(ctx, PowerAttempt{
		DeviceID:     dev.ID,
		DeviceName:   dev.Name,
		Action:       "schedule",
		DelaySeconds: 1800,
		Status:       "sent",
		Message:      "shutdown /s /t 1800",
	})
	if err != nil {
		t.Fatalf("RecordPowerAttempt failed: %v", err)
	}
	if attempt.ID == "" || attempt.Action != "schedule" {
		t.Errorf("unexpected attempt: %+v", attempt)
	}

	// Test ListPowerAttempts
	attempts, err := st.ListPowerAttempts(ctx, 10)
	if err != nil {
		t.Fatalf("ListPowerAttempts failed: %v", err)
	}
	if len(attempts) != 1 || attempts[0].ID != attempt.ID {
		t.Errorf("unexpected attempts list: %+v", attempts)
	}

	// Test DeletePowerProfile
	if err := st.DeletePowerProfile(ctx, dev.ID); err != nil {
		t.Fatalf("DeletePowerProfile failed: %v", err)
	}
	_, err = st.GetPowerProfile(ctx, dev.ID)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}
