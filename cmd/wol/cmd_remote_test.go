package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aklkbqx/wol/internal/store"
)

type fakeRemoteManager struct {
	opened bool
	wake   bool
	url    string
}

func (m *fakeRemoteManager) Open(_ context.Context, _ store.Device, _ store.RemoteProfile, wake bool) (string, error) {
	m.opened, m.wake = true, wake
	return m.url, nil
}
func (*fakeRemoteManager) Close() error { return nil }

func TestRunRemoteConfigureAndOpenLocalhost(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "wol.db")
	repository, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	device, err := repository.CreateDevice(t.Context(), store.Device{Name: "windows", MACAddress: "02:00:00:00:00:5d", IPAddress: "192.168.50.200", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	repository.Close()

	if code := runRemote([]string{"configure", "--db", databasePath, "--protocol", "rdp", "windows"}); code != 0 {
		t.Fatalf("remote configure exit code = %d", code)
	}
	repository, err = store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := repository.GetRemoteProfile(t.Context(), device.ID)
	repository.Close()
	if err != nil || profile.Host != "192.168.50.200" || profile.Port != 3389 || profile.Mode != "browser-local" {
		t.Fatalf("profile = %+v, err = %v", profile, err)
	}

	fake := &fakeRemoteManager{url: "http://127.0.0.1:43210/s/token"}
	previousFactory, previousWait := newRemoteManager, waitForRemoteStop
	t.Cleanup(func() { newRemoteManager, waitForRemoteStop = previousFactory, previousWait })
	newRemoteManager = func(*store.Store) remoteManager { return fake }
	waitForRemoteStop = func(context.Context) {}
	if code := runRemote([]string{"--db", databasePath, "windows"}); code != 0 {
		t.Fatalf("remote open exit code = %d", code)
	}
	if !fake.opened || !fake.wake {
		t.Fatalf("open = %v, auto wake = %v", fake.opened, fake.wake)
	}
}

func TestRunRemoteNoWakeIsExplicit(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "wol.db")
	repository, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	device, _ := repository.CreateDevice(t.Context(), store.Device{Name: "pc", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.1.10", Enabled: true})
	_, _ = repository.UpsertRemoteProfile(t.Context(), store.RemoteProfile{DeviceID: device.ID, Protocol: "rdp", Host: device.IPAddress, Port: 3389, VerifyPort: 3389, Mode: "browser-local", Enabled: true})
	repository.Close()
	fake := &fakeRemoteManager{url: "http://127.0.0.1:1/s/token"}
	previousFactory, previousWait := newRemoteManager, waitForRemoteStop
	t.Cleanup(func() { newRemoteManager, waitForRemoteStop = previousFactory, previousWait })
	newRemoteManager = func(*store.Store) remoteManager { return fake }
	waitForRemoteStop = func(context.Context) {}
	if code := runRemote([]string{"--db", databasePath, "--no-wake", "pc"}); code != 0 || fake.wake {
		t.Fatalf("exit = %d, auto wake = %v", code, fake.wake)
	}
}
