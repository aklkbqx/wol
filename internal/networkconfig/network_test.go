package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNetworkTargetsFromFile(t *testing.T) {
	t.Setenv("WOL_NETWORK_PROBE_TARGETS", "")
	root := t.TempDir()
	path := filepath.Join(root, defaultNetworkTargetsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	contents := `{"targets":[{"name":"ZeroTier Gateway","host":"198.51.100.1","port":22,"type":"zerotier","sshHost":"root@198.51.100.1","wolRelay":true}]}`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	targets, err := LoadNetworkTargets(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Type != "zerotier" || !targets[0].WOLRelay {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}

func TestLoadNetworkTargetsEnvironmentOverride(t *testing.T) {
	t.Setenv("WOL_NETWORK_PROBE_TARGETS", "lan-gateway=192.168.50.1:22,zt-gateway=198.51.100.1:22")
	targets, err := LoadNetworkTargets(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].Type != "router" || targets[1].Type != "zerotier" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}

func TestLoadNetworkTargetsRejectsDuplicates(t *testing.T) {
	t.Setenv("WOL_NETWORK_PROBE_TARGETS", "one=192.168.50.1:22,two=192.168.50.1:22")
	if _, err := LoadNetworkTargets(t.TempDir()); err == nil {
		t.Fatal("expected duplicate address to fail")
	}
}
