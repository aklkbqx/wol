package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aklkbqx/wol/internal/store"
)

func TestBatchSelectionAndWakePreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wol.db")
	repo, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateDevice(t.Context(), store.Device{Name: "demo", MACAddress: "02:00:00:00:00:01", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	repo.Close()
	for _, args := range [][]string{{"wake", "--db", path, "--all"}, {"check", "--db", path}, {"check", "--db", path, "--all", "demo"}, {"check", "--db", path, "--site", "missing"}} {
		if code := runBatch(args); code != 2 {
			t.Fatalf("%v = %d", args, code)
		}
	}
	if code := runBatch([]string{"check", "--db", path, "--all", "--json", "--no-input"}); code != 1 {
		t.Fatalf("missing IP = %d", code)
	}
	repo, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	attempts, err := repo.ListWakeAttempts(t.Context(), 10)
	if err != nil || len(attempts) != 0 {
		t.Fatalf("preview sent packets: %v %v", attempts, err)
	}
}

func TestBatchAcceptsFlagsAfterMachineNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wol.db")
	// A parser that stops at the machine name would open WOL_DB instead of --db.
	t.Setenv("WOL_DB", filepath.Join(t.TempDir(), "unused.db"))
	repo, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDevice(t.Context(), store.Device{Name: "demo", MACAddress: "02:00:00:00:00:01", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	// The machine has no IP, so the check fails (1) instead of reporting an unknown machine (2).
	if code := runBatch([]string{"check", "demo", "--db", path, "--json", "--no-input"}); code != 1 {
		t.Fatalf("check demo --db = %d, want 1", code)
	}
	if code := runBatch([]string{"check", "--db", path, "--", "--json"}); code != 2 {
		t.Fatalf("argument after -- was not treated as a machine name: %d", code)
	}
}

func TestCommandsAcceptFlagsAfterPositionals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wol.db")
	// A parser that stops at the positional argument would use WOL_DB instead of --db.
	t.Setenv("WOL_DB", filepath.Join(dir, "unused.db"))
	inventory := filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(inventory, []byte(`{"version":6,"sites":[],"devices":[{"name":"demo","macAddress":"02:00:00:00:00:01","enabled":true}],"groups":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if code := runImport([]string{inventory, "--db", path}); code != 0 {
		t.Fatalf("import FILE --db = %d, want 0", code)
	}
	repo, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := repo.ListDevices(t.Context())
	repo.Close()
	if err != nil || len(devices) != 1 || devices[0].Name != "demo" {
		t.Fatalf("import did not reach --db: %v %v", devices, err)
	}
	// The machine has no IP: status reports a failed check rather than a usage error.
	if code := runStatus([]string{"demo", "--db", path, "--timeout", "1s"}); code == 2 {
		t.Fatal("status demo --db treated the flags as extra machines")
	}
	if code := runRemoteConfigure([]string{"demo", "--db", path, "--protocol", "ssh", "--host", "10.0.0.10"}); code != 0 {
		t.Fatalf("remote configure demo --db --protocol = %d, want 0", code)
	}
	if code := runRemoteClear([]string{"demo", "--db", path}); code != 0 {
		t.Fatalf("remote clear demo --db = %d, want 0", code)
	}
}
