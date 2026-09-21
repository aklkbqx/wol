package main

import (
	"github.com/aklkbqx/wol/internal/store"
	"path/filepath"
	"testing"
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
