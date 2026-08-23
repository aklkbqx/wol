// Package remoteflow coordinates Wake-on-LAN with an ephemeral localhost-only
// browser remote. It owns all sessions so callers can shut them down cleanly.
package remoteflow

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/aklkbqx/wol/internal/localremote"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
)

type sessionStarter func(context.Context, localremote.Config) (*localremote.Session, error)
type targetProbe func(context.Context, string, int) bool

// Manager owns the short-lived Docker sidecars and localhost brokers it starts.
type Manager struct {
	repository *store.Store
	service    *wakeservice.Service
	opener     localremote.Opener
	ctx        context.Context
	cancel     context.CancelFunc
	start      sessionStarter
	probe      targetProbe

	mu       sync.Mutex
	sessions map[string]*localremote.Session
}

// New creates a localhost remote manager. Close must be called before exit.
func New(repository *store.Store, opener localremote.Opener) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		repository: repository,
		service:    wakeservice.NewService(repository, wakeservice.Hooks{}),
		opener:     opener,
		ctx:        ctx,
		cancel:     cancel,
		start:      localremote.Start,
		probe:      probeTarget,
		sessions:   make(map[string]*localremote.Session),
	}
}

// Open checks the target, optionally wakes it, then starts and opens a
// loopback-only browser session. No external URL is accepted or persisted.
func (m *Manager) Open(ctx context.Context, device store.Device, profile store.RemoteProfile, autoWake bool) (string, error) {
	if m == nil || m.repository == nil || m.service == nil {
		return "", errors.New("local remote manager is unavailable")
	}
	if !profile.Enabled || profile.DeviceID != device.ID {
		return "", errors.New("local remote profile is not enabled for this machine")
	}
	if profile.Mode != "browser-local" {
		return "", fmt.Errorf("remote mode %q is not supported yet; use browser-local", profile.Mode)
	}
	if !m.probe(ctx, profile.Host, profile.VerifyPort) {
		if !autoWake {
			return "", fmt.Errorf("%s is not reachable; run without --no-wake to wake it first", device.Name)
		}
		if _, err := m.service.WakeDevice(ctx, device.ID, wakeservice.Options{Repeat: 3, Interval: 200 * time.Millisecond, Verify: false}); err != nil {
			return "", fmt.Errorf("wake %s: %w", device.Name, err)
		}
		if err := m.waitUntilReachable(ctx, profile.Host, profile.VerifyPort, 60*time.Second); err != nil {
			return "", fmt.Errorf("wait for %s: %w", device.Name, err)
		}
	}

	sessionCtx, sessionCancel := context.WithCancel(m.ctx)
	stopActionCancel := context.AfterFunc(ctx, sessionCancel)
	session, err := m.start(sessionCtx, localremote.Config{
		Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port,
		UsernameHint: profile.UsernameHint, DomainHint: profile.DomainHint,
		CertificatePolicy: profile.CertificatePolicy,
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
	previous := m.sessions[device.ID]
	m.sessions[device.ID] = session
	m.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	return session.URL, nil
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

// Close tears down every broker, container, and private Docker network.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.cancel()
	m.mu.Lock()
	sessions := make([]*localremote.Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = make(map[string]*localremote.Session)
	m.mu.Unlock()
	var result error
	for _, session := range sessions {
		result = errors.Join(result, session.Close())
	}
	return result
}
