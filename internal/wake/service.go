// Package wake contains the storage-backed standalone Wake-on-LAN use cases.
package wake

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/wol"
)

// Repository is the SQLite-backed contract required by the wake service. It
// deliberately excludes HTTP concerns so a terminal client can use it
// directly.
type Repository interface {
	store.Repository
	GetWakeRelay(context.Context, string) (store.WakeRelay, error)
}

type RelayResult struct {
	Packets int
	Detail  string
}

type Hooks struct {
	Direct func(context.Context, wol.SendRequest) (wol.SendResult, error)
	Relay  func(context.Context, store.WakeRelay, net.HardwareAddr) (RelayResult, error)
	Verify func(context.Context, string, int, time.Duration) error
	Now    func() time.Time
}

type Options struct {
	Repeat     int
	Interval   time.Duration
	Verify     bool
	VerifyPort int
	Timeout    time.Duration
	Force      bool
	TargetType string
}

type Route struct {
	Kind        string
	Name        string
	Destination net.IP
	Port        int
	Interface   string
	Relay       store.WakeRelay
}

type Result struct {
	Attempt store.WakeAttempt
	Device  store.Device
	Route   Route
	Detail  string
}

type Service struct {
	repository Repository
	hooks      Hooks
}

func NewService(repository Repository, hooks Hooks) *Service {
	if hooks.Direct == nil {
		hooks.Direct = wol.Send
	}
	if hooks.Relay == nil {
		hooks.Relay = SendEtherwake
	}
	if hooks.Verify == nil {
		hooks.Verify = wol.WaitForTCP
	}
	if hooks.Now == nil {
		hooks.Now = time.Now
	}
	return &Service{repository: repository, hooks: hooks}
}

func (s *Service) ResolveRoute(ctx context.Context, device store.Device) (Route, error) {
	destination := strings.TrimSpace(device.BroadcastAddress)
	port := device.Port
	interfaceName := strings.TrimSpace(device.Interface)
	if device.SiteID != "" {
		site, err := s.repository.GetSite(ctx, device.SiteID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return Route{}, err
		}
		if site.ID != "" {
			if destination == "" {
				destination = strings.TrimSpace(site.BroadcastAddress)
			}
			if port == 0 {
				port = site.DefaultPort
			}
			if interfaceName == "" {
				interfaceName = strings.TrimSpace(site.DefaultInterface)
			}
		}
	}
	if port == 0 {
		port = 9
	}

	if strings.EqualFold(strings.TrimSpace(device.WakeStrategy), "relay") || strings.TrimSpace(device.WakeRelayID) != "" {
		if strings.TrimSpace(device.WakeRelayID) == "" {
			return Route{}, errors.New("relay wake strategy requires a relay route")
		}
		relay, err := s.repository.GetWakeRelay(ctx, device.WakeRelayID)
		if err != nil {
			return Route{}, fmt.Errorf("load relay route: %w", err)
		}
		if !relay.Enabled {
			return Route{}, fmt.Errorf("relay route %q is disabled", relay.Name)
		}
		return Route{Kind: "relay", Name: relay.Name, Port: port, Interface: relay.Interface, Relay: relay}, nil
	}

	if destination == "" {
		destination = "255.255.255.255"
	}
	ip := net.ParseIP(destination)
	if ip == nil || ip.To4() == nil {
		return Route{}, fmt.Errorf("broadcast address %q is not a valid IPv4 address", destination)
	}
	return Route{Kind: "broadcast", Name: "direct broadcast", Destination: ip.To4(), Port: port, Interface: interfaceName}, nil
}

func (s *Service) WakeDevice(ctx context.Context, deviceID string, options Options) (Result, error) {
	device, err := s.repository.GetDevice(ctx, deviceID)
	if err != nil {
		targetType := strings.TrimSpace(options.TargetType)
		if targetType == "" {
			targetType = "device"
		}
		return Result{Attempt: store.WakeAttempt{TargetType: targetType, TargetID: deviceID, PacketStatus: "failed", VerificationStatus: "not_requested", Message: err.Error()}}, err
	}
	if !device.Enabled && !options.Force {
		return s.recordFailure(ctx, device, Route{}, options, errors.New("device is disabled"))
	}
	route, err := s.ResolveRoute(ctx, device)
	if err != nil {
		return s.recordFailure(ctx, device, route, options, err)
	}
	mac, err := wol.ParseMAC(device.MACAddress)
	if err != nil {
		return s.recordFailure(ctx, device, route, options, err)
	}

	repeat := options.Repeat
	if repeat < 1 {
		repeat = 3
	}
	interval := options.Interval
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	targetType := strings.TrimSpace(options.TargetType)
	if targetType == "" {
		targetType = "device"
	}
	attempt := store.WakeAttempt{
		TargetType:         targetType,
		TargetID:           device.ID,
		TargetName:         device.Name,
		MACAddress:         device.MACAddress,
		PacketStatus:       "sending",
		VerificationStatus: "not_requested",
		Port:               route.Port,
	}
	if route.Destination != nil {
		attempt.Destination = route.Destination.String()
	}

	var packets int
	var detail string
	if route.Kind == "relay" {
		result, sendErr := s.hooks.Relay(ctx, route.Relay, mac)
		packets = result.Packets
		detail = result.Detail
		if sendErr != nil {
			attempt.PacketStatus = "failed"
			attempt.Message = sendErr.Error()
			attempt.Packets = packets
			stored, recordErr := s.repository.RecordWakeAttempt(ctx, attempt)
			if recordErr != nil {
				return Result{Attempt: attempt, Device: device, Route: route, Detail: detail}, recordErr
			}
			return Result{Attempt: stored, Device: device, Route: route, Detail: detail}, sendErr
		}
	} else {
		sendResult, sendErr := s.hooks.Direct(ctx, wol.SendRequest{MAC: mac, Destination: route.Destination, Port: route.Port, Interface: route.Interface, Repeat: repeat, Interval: interval})
		packets = sendResult.Packets
		if sendErr != nil {
			attempt.PacketStatus = "failed"
			attempt.Message = sendErr.Error()
			attempt.Packets = packets
			stored, recordErr := s.repository.RecordWakeAttempt(ctx, attempt)
			if recordErr != nil {
				return Result{Attempt: attempt, Device: device, Route: route}, recordErr
			}
			return Result{Attempt: stored, Device: device, Route: route}, sendErr
		}
	}

	attempt.PacketStatus = "sent"
	attempt.Packets = packets
	attempt.Message = detail
	if options.Verify {
		verifyPort := options.VerifyPort
		if verifyPort == 0 {
			verifyPort = device.VerifyPort
		}
		if strings.TrimSpace(device.IPAddress) == "" || verifyPort < 1 || verifyPort > 65535 {
			attempt.VerificationStatus = "unavailable"
			if attempt.Message == "" {
				attempt.Message = "verification requires an IP address and TCP port"
			}
		} else {
			attempt.VerificationStatus = "checking"
			verifyErr := s.hooks.Verify(ctx, device.IPAddress, verifyPort, options.Timeout)
			if verifyErr != nil {
				attempt.VerificationStatus = "timeout"
				attempt.Message = verifyErr.Error()
			} else {
				attempt.VerificationStatus = "reachable"
			}
		}
	}
	stored, err := s.repository.RecordWakeAttempt(ctx, attempt)
	if err != nil {
		return Result{Attempt: attempt, Device: device, Route: route, Detail: detail}, err
	}
	return Result{Attempt: stored, Device: device, Route: route, Detail: detail}, nil
}

func (s *Service) recordFailure(ctx context.Context, device store.Device, route Route, options Options, cause error) (Result, error) {
	targetType := strings.TrimSpace(options.TargetType)
	if targetType == "" {
		targetType = "device"
	}
	attempt := store.WakeAttempt{TargetType: targetType, TargetID: device.ID, TargetName: device.Name, MACAddress: device.MACAddress, PacketStatus: "failed", VerificationStatus: "not_requested", Message: cause.Error()}
	if route.Destination != nil {
		attempt.Destination = route.Destination.String()
		attempt.Port = route.Port
	}
	stored, err := s.repository.RecordWakeAttempt(ctx, attempt)
	if err != nil {
		return Result{Attempt: attempt, Device: device, Route: route}, err
	}
	return Result{Attempt: stored, Device: device, Route: route}, cause
}
