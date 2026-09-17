package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *WakeModel) beginWake(force bool) tea.Cmd {
	devices := m.filteredDevices()
	if len(devices) == 0 {
		m.status = "No machine selected. Press a to add one."
		return nil
	}
	if m.waking || m.opening || m.shuttingDown {
		m.status = "Finish the current action before starting another."
		return nil
	}
	device := devices[min(m.selected, len(devices)-1)]
	m.actionID++
	operationID := m.actionID
	m.actionTargetID, m.actionTarget = device.ID, device.Name
	m.waking = true
	m.action = "wake"
	m.status = fmt.Sprintf("Sending wake packet to %s...", device.Name)
	motionCmd := m.triggerMotion(StageSignal, 800*time.Millisecond, 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	m.actionContext = ctx
	m.actionCancel = cancel
	wakeCmd := func() tea.Msg {
		timedCtx, timeoutCancel := context.WithTimeout(ctx, 35*time.Second)
		defer timeoutCancel()
		result, err := m.service.WakeDevice(timedCtx, device.ID, wakeservice.Options{Force: force, Repeat: 3, Interval: 200 * time.Millisecond, Verify: false})
		return wakeResultMsg{operationID: operationID, targetID: device.ID, targetName: device.Name, result: result, err: err}
	}
	return tea.Batch(wakeCmd, motionCmd)
}

func (m *WakeModel) beginWakeAndRemote() tea.Cmd {
	devices := m.filteredDevices()
	if len(devices) == 0 {
		m.status = "No machine selected. Press a to add one."
		return nil
	}
	if m.opening || m.waking || m.shuttingDown {
		m.status = "Finish the current action before starting another."
		return nil
	}
	device := devices[min(m.selected, len(devices)-1)]
	profile, ok := m.profiles[device.ID]
	if !ok || !profile.Enabled {
		m.status = "Local remote needs setup for " + device.Name + ". Press p to configure it."
		return nil
	}
	if err := validateRemoteProfile(profile); err != nil {
		m.status = "Local remote profile is incomplete: " + err.Error() + ". Press p to fix it."
		return nil
	}
	if m.wakeAndRemote == nil {
		m.status = "Local remote runtime is unavailable. Run wol remote doctor, then try again."
		return nil
	}
	m.opening = true
	m.actionID++
	operationID := m.actionID
	m.actionTargetID, m.actionTarget = device.ID, device.Name
	m.action = "wake-remote"
	if streamProfile(profile) {
		m.status = "Wake & stream. Esc cancels."
	} else {
		m.status = "Wake & remote. Esc cancels."
	}
	motionCmd := m.triggerMotion(StageSignal, 95*time.Second, 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	m.actionContext = ctx
	m.actionCancel = cancel
	openCmd := func() tea.Msg {
		defer cancel()
		err := m.wakeAndRemote(ctx, device, profile)
		return remoteResultMsg{operationID: operationID, targetID: device.ID, deviceName: device.Name, err: err}
	}
	return tea.Batch(openCmd, motionCmd)
}

func (m *WakeModel) beginDisconnect() tea.Cmd {
	devices := m.filteredDevices()
	if len(devices) == 0 {
		m.status = "No machine selected."
		return nil
	}
	device := devices[min(m.selected, len(devices)-1)]
	if m.stopRemote == nil {
		m.status = "Disconnect is unavailable."
		return nil
	}
	if !m.streaming[device.ID] {
		m.status = device.Name + " is not streaming."
		return nil
	}
	name := device.Name
	id := device.ID
	return func() tea.Msg {
		err := m.stopRemote(id)
		return disconnectResultMsg{targetID: id, deviceName: name, err: err}
	}
}

func streamProfile(profile store.RemoteProfile) bool {
	return profile.Protocol == "sunshine" || profile.Mode == "native-moonlight"
}

func (m *WakeModel) ensureStreaming() {
	if m.streaming == nil {
		m.streaming = make(map[string]bool)
	}
}

func (m *WakeModel) finishAction() {
	if m.actionCancel != nil {
		m.actionCancel()
	}
	m.actionCancel = nil
	m.actionContext = nil
	m.actionTargetID = ""
	m.actionTarget = ""
	m.action = ""
}

func (m *WakeModel) cancelAction() {
	m.actionID++
	m.waking = false
	m.opening = false
	m.shuttingDown = false
	m.stopMotion()
	m.finishAction()
}

func (m *WakeModel) verifyWake(operationID uint64, device store.Device) tea.Cmd {
	targetID, targetName := device.ID, device.Name
	port := device.VerifyPort
	if profile, ok := m.profiles[device.ID]; port == 0 && ok {
		port = profile.VerifyPort
	}
	parent := m.actionContext
	if parent == nil {
		parent = context.Background()
	}
	detector := m.presenceDetector()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 90*time.Second)
		defer cancel()
		delays := []time.Duration{0, time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second, 8 * time.Second, 10 * time.Second}
		for attempt := 0; ; attempt++ {
			delay := delays[min(attempt, len(delays)-1)]
			if delay > 0 {
				select {
				case <-ctx.Done():
					return wakeVerifyMsg{operationID: operationID, targetID: targetID, targetName: targetName, status: "timeout"}
				case <-time.After(delay):
				}
			}
			result := detector.Probe(ctx, presence.Target{DeviceID: targetID, IPAddress: device.IPAddress, VerifyPort: port}, 1500*time.Millisecond)
			if result.Status == presence.StatusOnline {
				return wakeVerifyMsg{operationID: operationID, targetID: targetID, targetName: targetName, status: "online"}
			}
			if ctx.Err() != nil {
				return wakeVerifyMsg{operationID: operationID, targetID: targetID, targetName: targetName, status: "timeout"}
			}
		}
	}
}

func (m *WakeModel) verifyShutdown(operationID uint64, device store.Device) tea.Cmd {
	targetID, targetName := device.ID, device.Name
	port := device.VerifyPort
	if profile, ok := m.profiles[device.ID]; port == 0 && ok {
		port = profile.VerifyPort
	}
	parent := m.actionContext
	if parent == nil {
		parent = context.Background()
	}
	detector := m.presenceDetector()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 90*time.Second)
		defer cancel()
		delays := []time.Duration{time.Second, 2 * time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second, 5 * time.Second}
		for attempt := 0; ; attempt++ {
			delay := delays[min(attempt, len(delays)-1)]
			select {
			case <-ctx.Done():
				return shutdownVerifyMsg{operationID: operationID, targetID: targetID, targetName: targetName, status: "timeout"}
			case <-time.After(delay):
			}
			result := detector.Probe(ctx, presence.Target{DeviceID: targetID, IPAddress: device.IPAddress, VerifyPort: port}, 1500*time.Millisecond)
			if result.Status == presence.StatusOffline {
				return shutdownVerifyMsg{operationID: operationID, targetID: targetID, targetName: targetName, status: "offline"}
			}
			if ctx.Err() != nil {
				return shutdownVerifyMsg{operationID: operationID, targetID: targetID, targetName: targetName, status: "timeout"}
			}
		}
	}
}

func (m *WakeModel) deviceName(deviceID string) string {
	for _, device := range m.devices {
		if device.ID == deviceID {
			return device.Name
		}
	}
	return "Machine"
}

func (m *WakeModel) prependWakeAttempt(attempt store.WakeAttempt) {
	if attempt.ID == "" {
		return
	}
	for _, existing := range m.history {
		if existing.ID == attempt.ID {
			return
		}
	}
	m.history = append([]store.WakeAttempt{attempt}, m.history...)
	if len(m.history) > 80 {
		m.history = m.history[:80]
	}
}

func (m *WakeModel) actionDevice() (store.Device, bool) {
	if m.actionTargetID == "" {
		devices := m.filteredDevices()
		if (m.waking || m.opening || m.shuttingDown) && len(devices) > 0 {
			return devices[min(m.selected, len(devices)-1)], true
		}
		return store.Device{}, false
	}
	for _, device := range m.devices {
		if device.ID == m.actionTargetID {
			return device, true
		}
	}
	return store.Device{}, false
}
