package wake

import (
	"context"
	"errors"
	"net"
	"path/filepath"
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
