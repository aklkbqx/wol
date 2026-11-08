package doctor

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunDoctorReportsInvalidNetworkConfig(t *testing.T) {
	t.Setenv("WOL_DB", filepath.Join(t.TempDir(), "wol.db"))
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
	t.Setenv("WOL_DB", filepath.Join(t.TempDir(), "wol.db"))
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

func TestRunDoctorWithMockToolsNetworkTargetsAndRenderReport(t *testing.T) {
	t.Setenv("WOL_DB", filepath.Join(t.TempDir(), "wol.db"))

	if _, err := findRemoteEtherwake("-oProxyCommand=bad"); err == nil {
		t.Fatal("expected findRemoteEtherwake to reject unsafe host starting with '-'")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	if runtime.GOOS != "windows" {
		binDir := t.TempDir()
		ztScript := "#!/bin/sh\necho '200 info abcdef0123 1.12.2 ONLINE'\n"
		if err := os.WriteFile(filepath.Join(binDir, "zerotier-cli"), []byte(ztScript), 0o755); err != nil {
			t.Fatal(err)
		}
		sshScript := "#!/bin/sh\necho '/usr/bin/etherwake'\n"
		if err := os.WriteFile(filepath.Join(binDir, "ssh"), []byte(sshScript), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	targetsJSON := fmt.Sprintf(`{"targets":[
		{"name":"local-router","type":"router","host":"127.0.0.1","port":%d,"ssh_host":"admin@127.0.0.1","wol_relay":true},
		{"name":"zt-peer","type":"zerotier","host":"127.0.0.1","port":1}
	]}`, port)
	targetsFile := filepath.Join(t.TempDir(), "targets.json")
	if err := os.WriteFile(targetsFile, []byte(targetsJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOL_NETWORK_TARGETS_FILE", targetsFile)

	report := RunDoctorWithEnv(t.TempDir(), nil)
	rendered := report.RenderReport()
	if !strings.Contains(rendered, "WOL LOCAL NETWORK DOCTOR") || !strings.Contains(rendered, "local-router") {
		t.Fatalf("unexpected rendered report:\n%s", rendered)
	}
}
