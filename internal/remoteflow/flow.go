package remoteflow

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aklkbqx/wol/internal/localremote"
	"github.com/aklkbqx/wol/internal/moonlight"
	"github.com/aklkbqx/wol/internal/remoteopen"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
)

type sessionStarter func(context.Context, localremote.Config) (*localremote.Session, error)
type targetProbe func(context.Context, string, int) bool
type streamStarter func(context.Context, string, string, int, string, int) (*moonlight.Session, error)
type desktopStarter func(context.Context, store.RemoteProfile) error

// Manager owns the short-lived Docker sidecars, brokers, and streaming flows it starts.
type Manager struct {
	repository     *store.Store
	service        *wakeservice.Service
	opener         localremote.Opener
	ctx            context.Context
	cancel         context.CancelFunc
	start          sessionStarter
	probe          targetProbe
	startMoonlight streamStarter
	startDesktop   desktopStarter

	mu       sync.Mutex
	closed   bool
	sessions map[string]*localremote.Session
	streams  map[string]*moonlight.Session
}

func defaultMoonlightStart(ctx context.Context, host, appName string, fps int, resolution string, bitrateKbps int) (*moonlight.Session, error) {
	client, err := moonlight.Detect()
	if err != nil {
		return nil, err
	}
	return client.Start(ctx, host, appName, fps, resolution, bitrateKbps)
}

// New creates a localhost remote manager. Close must be called before exit.
func New(repository *store.Store, opener localremote.Opener) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		repository:     repository,
		service:        wakeservice.NewService(repository, wakeservice.Hooks{}),
		opener:         opener,
		ctx:            ctx,
		cancel:         cancel,
		start:          localremote.Start,
		probe:          probeTarget,
		startMoonlight: defaultMoonlightStart,
		startDesktop:   remoteopen.OpenDesktop,
		sessions:       make(map[string]*localremote.Session),
		streams:        make(map[string]*moonlight.Session),
	}
}

// Open checks the target, optionally wakes it, then opens a native desktop
// client, Moonlight stream, or loopback-only browser session.
func (m *Manager) Open(ctx context.Context, device store.Device, profile store.RemoteProfile, autoWake bool) (string, error) {
	if m == nil || m.repository == nil || m.service == nil {
		return "", errors.New("local remote manager is unavailable")
	}
	if !profile.Enabled || profile.DeviceID != device.ID {
		return "", errors.New("local remote profile is not enabled for this machine")
	}

	if err := m.ensureReachable(ctx, device, profile, autoWake); err != nil {
		return "", err
	}

	if remoteopen.NativeDesktop(profile) {
		if m.startDesktop == nil {
			m.startDesktop = remoteopen.OpenDesktop
		}
		if err := m.startDesktop(ctx, profile); err != nil {
			return "", fmt.Errorf("open %s client: %w", profile.Protocol, err)
		}
		return fmt.Sprintf("Opened %s for %s (%s)", strings.ToUpper(profile.Protocol), device.Name, profile.Host), nil
	}

	if profile.Mode == "native-moonlight" || profile.Protocol == "sunshine" {
		session, err := m.startMoonlight(ctx, profile.Host, profile.AppName, profile.FPS, profile.Resolution, profile.BitrateKbps)
		if err != nil {
			return "", fmt.Errorf("launch moonlight: %w", err)
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			_ = session.Stop()
			return "", errors.New("local remote manager is closed")
		}
		previous := m.streams[device.ID]
		m.streams[device.ID] = session
		m.mu.Unlock()
		if previous != nil {
			_ = previous.Stop()
		}
		return fmt.Sprintf("Moonlight stream launched for %s (%s)", device.Name, profile.Host), nil
	}

	if profile.Mode != "browser-local" {
		return "", fmt.Errorf("remote mode %q is not supported; use native, browser-local, or native-moonlight", profile.Mode)
	}

	sessionCtx, sessionCancel := context.WithCancel(m.ctx)
	stopActionCancel := context.AfterFunc(ctx, sessionCancel)
	session, err := m.start(sessionCtx, localremote.Config{
		Name: device.Name, Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port,
		UsernameHint: profile.UsernameHint, DomainHint: profile.DomainHint,
		CertificatePolicy: profile.CertificatePolicy,
		Vault:             localremote.OSVault(),
		Opener:            m.opener, OpenBrowser: true,
	})
	if err != nil {
		stopActionCancel()
		sessionCancel()
		return "", err
	}
	// Once startup succeeds, the manager—not the one-shot action—owns lifetime.
	stopActionCancel()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = session.Close()
		return "", errors.New("local remote manager is closed")
	}
	previous := m.sessions[device.ID]
	m.sessions[device.ID] = session
	m.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	return session.URL, nil
}

func (m *Manager) ensureReachable(ctx context.Context, device store.Device, profile store.RemoteProfile, autoWake bool) error {
	if m.probe(ctx, profile.Host, profile.VerifyPort) {
		return nil
	}
	if !autoWake {
		return fmt.Errorf("%s is not reachable; run without --no-wake to wake it first", device.Name)
	}
	if _, err := m.service.WakeDevice(ctx, device.ID, wakeservice.Options{Repeat: 3, Interval: 200 * time.Millisecond, Verify: false}); err != nil {
		return fmt.Errorf("wake %s: %w", device.Name, err)
	}
	if err := m.waitUntilReachable(ctx, profile.Host, profile.VerifyPort, 60*time.Second); err != nil {
		return fmt.Errorf("wait for %s: %w", device.Name, err)
	}
	return nil
}

func (m *Manager) waitUntilReachable(ctx context.Context, host string, port int, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if m.probe(ctx, host, port) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("remote service did not become reachable within 60 seconds")
		case <-ticker.C:
		}
	}
}

func probeTarget(ctx context.Context, host string, port int) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil {
		_ = conn.Close()
		return true
	}
	return false
}

// Stop ends the Moonlight stream or browser session for one machine.
func (m *Manager) Stop(deviceID string) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	stream := m.streams[deviceID]
	delete(m.streams, deviceID)
	session := m.sessions[deviceID]
	delete(m.sessions, deviceID)
	m.mu.Unlock()
	var result error
	if stream != nil {
		result = errors.Join(result, stream.Stop())
	}
	if session != nil {
		result = errors.Join(result, session.Close())
	}
	return result
}

// Streaming reports whether wol still owns a Moonlight process for the machine.
func (m *Manager) Streaming(deviceID string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.streams[deviceID]
	return session != nil && session.Alive()
}

// Close tears down every broker, container, stream, and private Docker network.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	m.mu.Lock()
	sessions := make([]*localremote.Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = make(map[string]*localremote.Session)
	streams := make([]*moonlight.Session, 0, len(m.streams))
	for _, session := range m.streams {
		streams = append(streams, session)
	}
	m.streams = make(map[string]*moonlight.Session)
	m.mu.Unlock()
	var result error
	for _, session := range sessions {
		result = errors.Join(result, session.Close())
	}
	for _, session := range streams {
		result = errors.Join(result, session.Stop())
	}
	return result
}
