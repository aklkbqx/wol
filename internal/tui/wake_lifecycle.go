package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/scanner"
	"github.com/aklkbqx/wol/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *WakeModel) beginRefresh(kind loadingKind) tea.Cmd {
	if m.loadCancel != nil {
		m.loadCancel()
	}
	m.requestID++
	m.loadingKind = kind
	m.loadingStage = stageInventory
	m.loadingTarget = ""
	m.loadingError = ""
	m.pending = nil
	m.loading = true
	m.checking = false
	if kind == loadingBoot {
		m.phase = phaseBootLoading
	} else {
		m.phase = phaseRefreshing
	}
	m.status = "Reading local inventory..."
	motionCmd := m.triggerMotion(StageSignal, 15*time.Second, 0, 0)
	m.loadContext, m.loadCancel = context.WithCancel(context.Background())
	return tea.Batch(m.loadData(m.loadContext, m.requestID, kind), motionCmd)
}

func (m *WakeModel) loadData(parent context.Context, requestID uint64, kind loadingKind) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		devices, err := m.repository.ListDevices(ctx)
		if err != nil {
			return wakeDataMsg{requestID: requestID, kind: kind, err: err}
		}
		if neighbors, nerr := scanner.ScanLAN(ctx); nerr == nil {
			if _, err := scanner.SyncDeviceIPs(ctx, m.repository, devices, neighbors); err == nil {
				if refreshed, err := m.repository.ListDevices(ctx); err == nil {
					devices = refreshed
				}
			}
		}
		sites, err := m.repository.ListSites(ctx)
		if err != nil {
			return wakeDataMsg{requestID: requestID, kind: kind, err: err}
		}
		relays, err := m.repository.ListWakeRelays(ctx)
		if err != nil {
			return wakeDataMsg{requestID: requestID, kind: kind, err: err}
		}
		history, err := m.repository.ListWakeAttempts(ctx, 80)
		if err != nil {
			return wakeDataMsg{requestID: requestID, kind: kind, err: err}
		}
		profiles, err := m.repository.ListRemoteProfiles(ctx)
		return wakeDataMsg{requestID: requestID, kind: kind, devices: devices, sites: sites, relays: relays, profiles: profiles, history: history, err: err}
	}
}

func (m *WakeModel) commitPending(statuses, methods map[string]string, summary presence.Summary) {
	if m.pending == nil {
		return
	}
	data := m.pending
	m.devices = append([]store.Device(nil), data.devices...)
	m.sites = append([]store.Site(nil), data.sites...)
	m.relays = append([]store.WakeRelay(nil), data.relays...)
	m.history = append([]store.WakeAttempt(nil), data.history...)
	m.profiles = make(map[string]store.RemoteProfile, len(data.profiles))
	for _, profile := range data.profiles {
		m.profiles[profile.DeviceID] = profile
	}
	m.presence = make(map[string]string, len(statuses))
	for deviceID, status := range statuses {
		m.presence[deviceID] = status
	}
	m.presenceMethod = make(map[string]string, len(methods))
	for deviceID, method := range methods {
		m.presenceMethod[deviceID] = method
	}
	count := 0
	switch m.tab {
	case 0:
		count = len(m.filteredDevices())
	case 1:
		count = len(m.relayList())
	default:
		count = len(m.history)
	}
	if count == 0 {
		m.selected = 0
	} else if m.selected >= count {
		m.selected = count - 1
	}
	m.pending = nil
	m.phase = phaseReady
	m.loading = false
	m.checking = false
	m.loadingError = ""
	m.loadingTarget = ""
	m.checkedAt = time.Now()
	m.stale = false
	m.stopMotion()
	m.finishLoadContext()
	if len(m.devices) == 0 {
		m.status = fmt.Sprintf("Inventory ready: %d machine(s), %d route(s).", len(m.devices), len(m.relays))
		return
	}
	m.status = fmt.Sprintf("Latest state ready: %d online · %d offline · %d unknown. Wake readiness is shown separately.", summary.Online, summary.Offline, summary.Unknown)
}

func (m *WakeModel) failLoading(message string) tea.Cmd {
	wasBoot := m.phase == phaseBootLoading && len(m.devices) == 0
	m.pending = nil
	m.loading = false
	m.checking = false
	m.stopMotion()
	m.finishLoadContext()
	if wasBoot {
		m.phase = phaseLoadError
		m.loadingError = message
		m.status = message
		return nil
	}
	m.phase = phaseReady
	m.stale = true
	m.status = message + " Showing the last verified state. Press r to retry."
	return nil
}

func (m *WakeModel) finishLoadContext() {
	if m.loadCancel != nil {
		m.loadCancel()
	}
	m.loadCancel = nil
	m.loadContext = nil
}

func (m *WakeModel) cancelLoading() {
	m.finishLoadContext()
	m.requestID++
	m.pending = nil
	m.loading = false
	m.checking = false
	m.phase = phaseReady
	m.stopMotion()
	m.status = "Check cancelled. Showing the last verified state."
}
