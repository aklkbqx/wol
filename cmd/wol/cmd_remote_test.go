package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aklkbqx/wol/internal/store"
)

func TestRunRemoteConfigureAndOpen(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "wol.db")
	repository, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	device, err := repository.CreateDevice(t.Context(), store.Device{Name: "windows", MACAddress: "02:00:00:00:00:5d", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	repository.Close()

	target := "https://wol.example.test/remote/" + device.ID
	if code := runRemote([]string{"set", "--db", databasePath, "windows", target}); code != 0 {
		t.Fatalf("remote set exit code = %d", code)
	}

	previous := openRemoteURL
	t.Cleanup(func() { openRemoteURL = previous })
	var opened string
	openRemoteURL = func(_ context.Context, target string) error {
		opened = target
		return nil
	}
	if code := runRemote([]string{"--db", databasePath, "windows"}); code != 0 {
		t.Fatalf("remote open exit code = %d", code)
	}
	if opened != target {
		t.Fatalf("opened %q, want %q", opened, target)
	}
}
