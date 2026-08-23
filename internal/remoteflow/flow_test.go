package remoteflow

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/aklkbqx/wol/internal/localremote"
	"github.com/aklkbqx/wol/internal/store"
)

func TestOpenUsesOnlyGeneratedLocalSession(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	device, err := repository.CreateDevice(t.Context(), store.Device{Name: "windows", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	profile := store.RemoteProfile{DeviceID: device.ID, Protocol: "rdp", Host: device.IPAddress, Port: 3389, VerifyPort: 3389, Mode: "browser-local", Enabled: true}
	manager := New(repository, func(context.Context, string) error { return nil })
	defer manager.Close()
	manager.probe = func(context.Context, string, int) bool { return true }
	var config localremote.Config
	manager.start = func(_ context.Context, received localremote.Config) (*localremote.Session, error) {
		config = received
		return &localremote.Session{URL: "http://127.0.0.1:43210/s/one-time"}, nil
	}
	url, err := manager.Open(t.Context(), device, profile, false)
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://127.0.0.1:43210/s/one-time" || config.Host != device.IPAddress || !config.OpenBrowser {
		t.Fatalf("url = %q, config = %+v", url, config)
	}
}

func TestProbeRequiresAcceptingRemoteService(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if !probeTarget(t.Context(), "127.0.0.1", port) {
		t.Fatal("listening remote service was not ready")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if probeTarget(t.Context(), "127.0.0.1", port) {
		t.Fatalf("refused remote service %s was treated as ready", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	}
}

func TestNoWakeStopsBeforeRuntimeWhenOffline(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	device, _ := repository.CreateDevice(t.Context(), store.Device{Name: "pc", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.1.20", Enabled: true})
	profile := store.RemoteProfile{DeviceID: device.ID, Protocol: "rdp", Host: device.IPAddress, Port: 3389, VerifyPort: 3389, Mode: "browser-local", Enabled: true}
	manager := New(repository, func(context.Context, string) error { return nil })
	defer manager.Close()
	manager.probe = func(context.Context, string, int) bool { return false }
	started := false
	manager.start = func(context.Context, localremote.Config) (*localremote.Session, error) {
		started = true
		return nil, nil
	}
	if _, err := manager.Open(t.Context(), device, profile, false); err == nil || started {
		t.Fatalf("error = %v, runtime started = %v", err, started)
	}
}
