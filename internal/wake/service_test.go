package wake

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/wol"
)

func TestWakeDeviceUsesSQLiteRouteAndRecordsAttempt(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	device, err := repository.CreateDevice(context.Background(), store.Device{Name: "office", MACAddress: "02:00:00:00:00:5d", BroadcastAddress: "192.168.50.255", Port: 9, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	var request wol.SendRequest
	service := NewService(repository, Hooks{
		Direct: func(_ context.Context, got wol.SendRequest) (wol.SendResult, error) {
			request = got
			return wol.SendResult{Destination: got.Destination.String(), Port: got.Port, Packets: got.Repeat, Bytes: 102}, nil
		},
	})
	result, err := service.WakeDevice(context.Background(), device.ID, Options{Repeat: 2, Interval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if request.Destination.String() != "192.168.50.255" || request.Repeat != 2 {
		t.Fatalf("unexpected direct request: %+v", request)
	}
	if result.Attempt.PacketStatus != "sent" || result.Attempt.Packets != 2 {
		t.Fatalf("unexpected attempt: %+v", result.Attempt)
	}
	history, err := repository.ListWakeAttempts(context.Background(), 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("history = %#v, err=%v", history, err)
	}
}

func TestWakeDeviceRecordsFailure(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	device, err := repository.CreateDevice(context.Background(), store.Device{Name: "broken", MACAddress: "02:00:00:00:00:5e", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repository, Hooks{Direct: func(context.Context, wol.SendRequest) (wol.SendResult, error) {
		return wol.SendResult{}, errors.New("socket unavailable")
	}})
	result, err := service.WakeDevice(context.Background(), device.ID, Options{})
	if err == nil || result.Attempt.PacketStatus != "failed" {
		t.Fatalf("expected failed wake, result=%+v err=%v", result, err)
	}
}

func TestWakeDeviceForceBypassesDisabledFlag(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	device, err := repository.CreateDevice(context.Background(), store.Device{Name: "disabled", MACAddress: "02:00:00:00:00:60", BroadcastAddress: "192.168.50.255", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repository, Hooks{Direct: func(_ context.Context, request wol.SendRequest) (wol.SendResult, error) {
		return wol.SendResult{Destination: request.Destination.String(), Packets: 1}, nil
	}})
	if _, err := service.WakeDevice(context.Background(), device.ID, Options{}); err == nil {
		t.Fatal("disabled device should fail without force")
	}
	result, err := service.WakeDevice(context.Background(), device.ID, Options{Force: true})
	if err != nil || result.Attempt.PacketStatus != "sent" {
		t.Fatalf("forced wake = %+v, err=%v", result.Attempt, err)
	}
}

func TestSendEtherwakeRejectsUnsafeRelay(t *testing.T) {
	_, err := SendEtherwake(context.Background(), store.WakeRelay{Address: "router;bad", Interface: "br-lan"}, net.HardwareAddr{0, 1, 2, 3, 4, 5})
	if err == nil {
		t.Fatal("expected unsafe relay to be rejected")
	}
}

func TestWakeDeviceUsesRelayRoute(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	relay, err := repository.CreateWakeRelay(context.Background(), store.WakeRelay{Name: "router", Address: "198.51.100.1", Port: 2222, Interface: "br-lan", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	device, err := repository.CreateDevice(context.Background(), store.Device{Name: "relay-target", MACAddress: "02:00:00:00:00:5f", Port: 9, WakeStrategy: "relay", WakeRelayID: relay.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	var gotRelay store.WakeRelay
	var gotMAC net.HardwareAddr
	service := NewService(repository, Hooks{Relay: func(_ context.Context, got store.WakeRelay, mac net.HardwareAddr) (RelayResult, error) {
		gotRelay, gotMAC = got, mac
		return RelayResult{Packets: 1, Detail: "relay accepted"}, nil
	}})
	result, err := service.WakeDevice(context.Background(), device.ID, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Route.Kind != "relay" || result.Route.Port != 9 || gotRelay.Port != 2222 || gotMAC.String() != device.MACAddress {
		t.Fatalf("relay route/result = %+v relay=%+v mac=%s", result.Route, gotRelay, gotMAC)
	}
}

func TestResolveRouteSiteFallbackAndValidationErrors(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	ctx := context.Background()

	site, err := repository.CreateSite(ctx, store.Site{
		Name:             "branch",
		BroadcastAddress: "10.10.20.255",
		DefaultPort:      7,
		DefaultInterface: "en0",
	})
	if err != nil {
		t.Fatal(err)
	}

	service := NewService(repository, Hooks{})

	route, err := service.ResolveRoute(ctx, store.Device{
		Name:   "site-inherited",
		SiteID: site.ID,
	})
	if err != nil {
		t.Fatalf("ResolveRoute with site defaults: %v", err)
	}
	if route.Kind != "broadcast" || route.Destination.String() != "10.10.20.255" || route.Port != 7 || route.Interface != "en0" {
		t.Fatalf("unexpected site route: %+v", route)
	}

	if _, err := service.ResolveRoute(ctx, store.Device{
		Name:             "bad-bcast",
		BroadcastAddress: "not-an-ip",
	}); err == nil {
		t.Fatal("expected invalid broadcast address error")
	}

	if _, err := service.ResolveRoute(ctx, store.Device{
		Name:         "missing-relay-id",
		WakeStrategy: "relay",
	}); err == nil {
		t.Fatal("expected missing relay route error")
	}

	disabledRelay, err := repository.CreateWakeRelay(ctx, store.WakeRelay{
		Name:    "disabled-router",
		Address: "192.0.2.1",
		Port:    22,
		Enabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveRoute(ctx, store.Device{
		Name:         "disabled-relay-dev",
		WakeStrategy: "relay",
		WakeRelayID:  disabledRelay.ID,
	}); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled relay error, got %v", err)
	}
}

func TestWakeDeviceVerificationAndMissingDevicePaths(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	ctx := context.Background()

	devNoIP, err := repository.CreateDevice(ctx, store.Device{
		Name:       "no-ip",
		MACAddress: "02:00:00:00:00:71",
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	devWithIP, err := repository.CreateDevice(ctx, store.Device{
		Name:       "with-ip",
		MACAddress: "02:00:00:00:00:72",
		IPAddress:  "192.168.1.72",
		VerifyPort: 22,
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var verifyErr error
	service := NewService(repository, Hooks{
		Direct: func(_ context.Context, req wol.SendRequest) (wol.SendResult, error) {
			return wol.SendResult{Destination: req.Destination.String(), Port: req.Port, Packets: req.Repeat, Warning: "fallback broadcast"}, nil
		},
		Verify: func(_ context.Context, _ string, _ int, _ time.Duration) error {
			return verifyErr
		},
	})

	if res, err := service.WakeDevice(ctx, "missing-id", Options{TargetType: "batch"}); err == nil || res.Attempt.PacketStatus != "failed" || res.Attempt.TargetType != "batch" {
		t.Fatalf("missing device wake = %+v, err=%v", res, err)
	}

	resNoIP, err := service.WakeDevice(ctx, devNoIP.ID, Options{Verify: true})
	if err != nil || resNoIP.Attempt.VerificationStatus != "unavailable" {
		t.Fatalf("expected unavailable verification, got %+v err=%v", resNoIP.Attempt, err)
	}

	verifyErr = nil
	resReachable, err := service.WakeDevice(ctx, devWithIP.ID, Options{Verify: true, Timeout: 50 * time.Millisecond})
	if err != nil || resReachable.Attempt.VerificationStatus != "reachable" {
		t.Fatalf("expected reachable verification, got %+v err=%v", resReachable.Attempt, err)
	}

	verifyErr = errors.New("verify timed out")
	resTimeout, err := service.WakeDevice(ctx, devWithIP.ID, Options{Verify: true, VerifyPort: 3389, Timeout: 50 * time.Millisecond})
	if err != nil || resTimeout.Attempt.VerificationStatus != "timeout" {
		t.Fatalf("expected timeout verification, got %+v err=%v", resTimeout.Attempt, err)
	}
}

func TestSendEtherwakeValidationAndMockSSH(t *testing.T) {
	ctx := context.Background()
	validMAC := net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}

	invalidCases := []struct {
		name  string
		relay store.WakeRelay
		mac   net.HardwareAddr
	}{
		{
			name:  "empty address",
			relay: store.WakeRelay{Name: "r1"},
			mac:   validMAC,
		},
		{
			name:  "unsupported transport",
			relay: store.WakeRelay{Name: "r1", Address: "192.0.2.1", Transport: "telnet"},
			mac:   validMAC,
		},
		{
			name:  "invalid mac length",
			relay: store.WakeRelay{Name: "r1", Address: "192.0.2.1"},
			mac:   net.HardwareAddr{0x02, 0x00},
		},
		{
			name:  "unsafe ssh user",
			relay: store.WakeRelay{Name: "r1", Address: "192.0.2.1", SSHUser: "-oProxyCommand=bad"},
			mac:   validMAC,
		},
		{
			name:  "unsafe interface",
			relay: store.WakeRelay{Name: "r1", Address: "192.0.2.1", Interface: "br-lan;id"},
			mac:   validMAC,
		},
		{
			name:  "invalid port",
			relay: store.WakeRelay{Name: "r1", Address: "192.0.2.1", Port: 70000},
			mac:   validMAC,
		},
	}

	for _, tc := range invalidCases {
		if _, err := SendEtherwake(ctx, tc.relay, tc.mac); err == nil {
			t.Fatalf("%s: expected error, got nil", tc.name)
		}
	}

	if runtime.GOOS == "windows" {
		return
	}

	binDir := t.TempDir()
	sshPath := filepath.Join(binDir, "ssh")
	script := "#!/bin/sh\necho \"mock-ssh:$*\"\n"
	if err := os.WriteFile(sshPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	res, err := SendEtherwake(ctx, store.WakeRelay{
		Name:      "edge",
		Address:   "198.51.100.10",
		SSHUser:   "admin",
		Port:      2222,
		Interface: "",
		Enabled:   true,
	}, validMAC)
	if err != nil {
		t.Fatalf("SendEtherwake with mock ssh failed: %v", err)
	}
	if res.Packets != 1 || !strings.Contains(res.Detail, "-p 2222 -- admin@198.51.100.10 /usr/bin/etherwake -b -i br-lan 02:00:00:00:00:01") {
		t.Fatalf("unexpected relay result: %+v", res)
	}
}
