package scanner

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
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

func TestGetLocalIPTargetTypesAndSSHHosts(t *testing.T) {
	if ip := GetLocalIP(); ip == "" {
		t.Fatal("GetLocalIP returned empty string")
	}
	if targetType("router") != TargetRouter || targetType("zerotier") != TargetZeroTier || targetType("ssh-host") != TargetSSHHost || targetType("other") != TargetServer {
		t.Fatal("unexpected targetType mapping")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sshConfig := "Host workstation homelab-router *.wildcard github.com\n  HostName 192.168.1.10\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(sshConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts := GetKnownSSHHosts()
	if len(hosts) != 2 {
		t.Fatalf("GetKnownSSHHosts() = %v, want 2 hosts", hosts)
	}
}

func TestScanIosSimulatorsAndroidAndScanAllWithMockTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell script mock skipped on Windows")
	}
	binDir := t.TempDir()
	xcrunScript := `#!/bin/sh
cat <<'EOF'
{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-18-0":[{"name":"iPhone 16","udid":"SIM-1","state":"Booted","isAvailable":true},{"name":"iPhone SE","udid":"SIM-2","state":"Shutdown","isAvailable":true}]}}
EOF
`
	adbScript := `#!/bin/sh
cat <<'EOF'
List of devices attached
emulator-5554          device product:sdk_gphone64_arm64 model:sdk_gphone64_arm64
192.168.1.88:5555      device product:pixel
dead-device            offline
EOF
`
	if err := os.WriteFile(filepath.Join(binDir, "xcrun"), []byte(xcrunScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "adb"), []byte(adbScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	sims := ScanIosSimulators()
	if len(sims) != 1 || sims[0].UDID != "SIM-1" || sims[0].Type != TargetIosSim {
		t.Fatalf("ScanIosSimulators() = %#v", sims)
	}

	androids := ScanAndroidDevices()
	if len(androids) != 2 || androids[0].Type != TargetAndroid {
		t.Fatalf("ScanAndroidDevices() = %#v", androids)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	defaults := []string{"WOL_NETWORK_PROBE_TARGETS=edge=127.0.0.1:" + portOf(listener.Addr().String())}
	all, err := ScanAllWithEnv(t.TempDir(), defaults)
	if err != nil {
		t.Fatalf("ScanAllWithEnv() failed: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("ScanAllWithEnv() count = %d, want 4 (%#v)", len(all), all)
	}
}
