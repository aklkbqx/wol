package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunDoctorReportsInvalidNetworkConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "targets.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOL_NETWORK_TARGETS_FILE", path)

	report := RunDoctor(t.TempDir())
	for _, item := range report.Items {
		if item.Category == "Configuration" && item.Name == "Network Targets" {
			if item.Status != "FAIL" {
				t.Fatalf("network configuration status = %q", item.Status)
			}
			return
		}
	}
	t.Fatal("doctor report did not include network configuration")
}

func TestRunDoctorOmitsUnusedRelayAndZeroTierNoise(t *testing.T) {
	report := RunDoctor(t.TempDir())
	for _, item := range report.Items {
		if item.Name == "Local etherwake" {
			t.Fatalf("etherwake should not appear unless an SSH relay is configured: %+v", item)
		}
		if item.Category == "ZeroTier" {
			t.Fatalf("ZeroTier should not appear without a configured zerotier target: %+v", item)
		}
		if item.Name == "Magic packet" && item.Status != "OK" {
			t.Fatalf("magic packet status = %q", item.Status)
		}
	}
}
