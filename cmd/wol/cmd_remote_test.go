package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aklkbqx/wol/internal/moonlight"
	"github.com/aklkbqx/wol/internal/store"
)

type fakeRemoteManager struct {
	opened  bool
	wake    bool
	url     string
	profile store.RemoteProfile
}

func (m *fakeRemoteManager) Open(_ context.Context, _ store.Device, profile store.RemoteProfile, wake bool) (string, error) {
	m.opened, m.wake = true, wake
	m.profile = profile
	return m.url, nil
}
func (*fakeRemoteManager) Close() error { return nil }

func TestRunRemoteConfigureAndChooseNativeOrBrowser(t *testing.T) {
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
	if err != nil || profile.Host != "192.168.50.200" || profile.Port != 3389 || profile.Mode != "native" {
		t.Fatalf("profile = %+v, err = %v", profile, err)
	}

	fake := &fakeRemoteManager{url: "http://127.0.0.1:43210/s/token"}
	previousFactory, previousWait := newRemoteManager, waitForRemoteStop
	t.Cleanup(func() { newRemoteManager, waitForRemoteStop = previousFactory, previousWait })
	newRemoteManager = func(*store.Store) remoteManager { return fake }
	waited := false
	waitForRemoteStop = func(context.Context) { waited = true }
	if code := runRemote([]string{"--db", databasePath, "windows"}); code != 0 {
		t.Fatalf("remote open exit code = %d", code)
	}
	if !fake.opened || !fake.wake {
		t.Fatalf("open = %v, auto wake = %v", fake.opened, fake.wake)
	}
	if waited || fake.profile.Mode != "native" {
		t.Fatalf("native launch waited=%v mode=%s", waited, fake.profile.Mode)
	}
	if code := runRemote([]string{"--db", databasePath, "--browser", "windows"}); code != 0 {
		t.Fatalf("browser open exit code = %d", code)
	}
	if !waited || fake.profile.Mode != "browser-local" {
		t.Fatalf("browser launch waited=%v mode=%s", waited, fake.profile.Mode)
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

func TestRunRemoteConfigureSunshine(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "wol.db")
	repository, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	device, err := repository.CreateDevice(t.Context(), store.Device{Name: "gaming-rig", MACAddress: "02:00:00:00:00:99", IPAddress: "192.168.1.100", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	repository.Close()

	if code := runRemote([]string{"configure", "--db", databasePath, "--protocol", "sunshine", "--fps", "120", "--res", "2560x1440", "--app", "Desktop", "gaming-rig"}); code != 0 {
		t.Fatalf("remote configure exit code = %d", code)
	}

	repository, err = store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := repository.GetRemoteProfile(t.Context(), device.ID)
	repository.Close()
	if err != nil || profile.Protocol != "sunshine" || profile.Port != 47989 || profile.Mode != "native-moonlight" || profile.FPS != 120 || profile.Resolution != "2560x1440" || profile.AppName != "Desktop" {
		t.Fatalf("unexpected profile: %+v, err: %v", profile, err)
	}
}

func TestRunRemotePairOnlySendsCredentialsToPinnedSunshine(t *testing.T) {
	var pinRequests atomic.Int32
	var user, pass string
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pin" {
			pinRequests.Add(1)
			user, pass, _ = r.BasicAuth()
		}
		w.WriteHeader(http.StatusOK)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	address, _ := url.Parse(server.URL)

	// Take the Sunshine path even when this computer has Moonlight installed.
	previousLookPath, previousExists := moonlight.LookPathFunc, moonlight.FileExistsFunc
	t.Cleanup(func() { moonlight.LookPathFunc, moonlight.FileExistsFunc = previousLookPath, previousExists })
	moonlight.LookPathFunc = func(string) (string, error) { return "", exec.ErrNotFound }
	moonlight.FileExistsFunc = func(string) bool { return false }

	databasePath := filepath.Join(t.TempDir(), "wol.db")
	repository, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateDevice(t.Context(), store.Device{Name: "rig", MACAddress: "02:00:00:00:00:98", IPAddress: address.Hostname(), Enabled: true}); err != nil {
		t.Fatal(err)
	}
	repository.Close()
	t.Setenv("SUNSHINE_USER", "admin")
	t.Setenv("SUNSHINE_PASS", "secret")
	pair := func(extra ...string) int {
		arguments := append([]string{"pair", "--db", databasePath, "--pin", "1234", "--port", address.Port()}, extra...)
		return runRemote(append(arguments, "rig"))
	}

	if code := pair(); code != 3 || pinRequests.Load() != 0 {
		t.Fatalf("unpinned self-signed pair: exit %d with %d PIN requests, want exit 3 and none", code, pinRequests.Load())
	}
	if code := pair("--tls-fingerprint", "not-hex"); code != 2 || pinRequests.Load() != 0 {
		t.Fatalf("malformed fingerprint: exit %d with %d PIN requests, want exit 2 and none", code, pinRequests.Load())
	}
	sum := sha256.Sum256(server.Certificate().Raw)
	if code := pair("--tls-fingerprint", strings.ToUpper(hex.EncodeToString(sum[:]))); code != 0 || user != "admin" || pass != "secret" {
		t.Fatalf("pinned pair: exit %d sent %q/%q, want exit 0 with admin/secret", code, user, pass)
	}
}

func TestRunRemotePairUsageDoesNotPrintSunshinePassword(t *testing.T) {
	t.Setenv("SUNSHINE_PASS", "hunter2-from-env")
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = write
	code := runRemote([]string{"pair", "--no-such-flag"})
	os.Stderr = previous
	write.Close()
	output, _ := io.ReadAll(read)
	if code != 2 || !strings.Contains(string(output), "-pass") {
		t.Fatalf("exit %d, usage %q; want exit 2 with flag usage", code, output)
	}
	if strings.Contains(string(output), "hunter2-from-env") {
		t.Fatalf("usage printed the SUNSHINE_PASS value: %s", output)
	}
}
