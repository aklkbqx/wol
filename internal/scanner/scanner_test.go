package scanner

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckTCPPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	if !CheckTCPPort(listener.Addr().String(), time.Second) {
		t.Fatal("CheckTCPPort() reported a listening port as offline")
	}
	if CheckTCPPort("127.0.0.1:1", 20*time.Millisecond) {
		t.Fatal("CheckTCPPort() reported an unused port as online")
	}
}

func TestScanNetworkTargetsUsesConfiguredFile(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	configPath := filepath.Join(t.TempDir(), "targets.json")
	content := []byte(`{"targets":[{"name":"offline","host":"127.0.0.1","port":1,"type":"server"},{"name":"local","host":"127.0.0.1","port":` + portOf(listener.Addr().String()) + `,"type":"server"}]}`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOL_NETWORK_TARGETS_FILE", configPath)

	got, err := ScanNetworkTargets(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "local" || got[0].Status != "online" || got[1].Status != "offline" {
		t.Fatalf("ScanNetworkTargets() = %#v", got)
	}
}

func TestScanNetworkTargetsUsesEnvironmentDefaults(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	defaults := []string{"WOL_NETWORK_PROBE_TARGETS=dev=127.0.0.1:" + portOf(listener.Addr().String())}
	got, err := ScanNetworkTargetsWithEnv(t.TempDir(), defaults)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "dev" || got[0].Status != "online" {
		t.Fatalf("ScanNetworkTargetsWithEnv() = %#v", got)
	}
}

func portOf(address string) string {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		panic(err)
	}
	return port
}
