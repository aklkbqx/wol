package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/fleet"
	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *WakeModel) ensurePresence() {
	if m.presence == nil {
		m.presence = make(map[string]string)
	}
}

func (m *WakeModel) ensureCheckedDevice() {
	if m.checkedDevice == nil {
		m.checkedDevice = make(map[string]time.Time)
	}
}

func (m *WakeModel) startPresenceScan(devices []store.Device, requestID uint64, kind loadingKind, parent context.Context) tea.Cmd {
	devices = append([]store.Device(nil), devices...)
	if m.tab == 0 {
		if d, ok := m.selectedDevice(m.filteredDevices()); ok {
			for i, item := range devices {
				if item.ID == d.ID {
					devices[0], devices[i] = devices[i], devices[0]
					break
				}
			}
		}
	}
	if len(devices) == 0 {
		return nil
	}
	if parent == nil {
		parent = context.Background()
	}
	m.loadContext, m.loadCancel = context.WithCancel(parent)
	profiles := make([]store.RemoteProfile, 0, len(m.profiles))
	for _, p := range m.profiles {
		profiles = append(profiles, p)
	}
	m.checking = true
	m.scanDone = 0
	m.scanTotal = len(devices)
	m.scanResults = fleet.Run(m.loadContext, devices, m.sites, 16, fleet.Check(m.presenceDetector(), profiles))
	m.status = fmt.Sprintf("Checking 0/%d machines. Esc cancels.", len(devices))
	return nextFleet(m.scanResults, requestID, false)
}

func (m *WakeModel) presenceDetector() *presence.Detector {
	if m.detector == nil {
		m.detector = presence.NewDetector()
	}
	return m.detector
}

func (m *WakeModel) probeSelected() tea.Cmd {
	devices := m.filteredDevices()
	if len(devices) == 0 {
		m.status = "No machine selected."
		return nil
	}
	if m.checking {
		m.status = "Power scan is already running."
		return nil
	}
	device := devices[max(0, min(m.selected, len(devices)-1))]
	if strings.TrimSpace(device.IPAddress) == "" {
		m.status = "Status unavailable: machine has no IP address."
		return nil
	}
	port := device.VerifyPort
	if profile, ok := m.profiles[device.ID]; port == 0 && ok {
		port = profile.VerifyPort
	}
	if m.loadCancel != nil {
		m.loadCancel()
	}
	m.requestID++
	m.phase = phaseReady
	m.loading = false
	m.loadingStage = stagePresence
	m.loadingTarget = device.Name
	if port > 0 {
		m.status = fmt.Sprintf("Checking power at %s:%d...", device.IPAddress, port)
	} else {
		m.status = fmt.Sprintf("Checking power for %s with automatic local probes...", device.IPAddress)
	}
	detector := m.presenceDetector()
	m.checking = true
	motionCmd := m.triggerMotion(StageSignal, 15*time.Second, 0, 0)
	m.loadContext, m.loadCancel = context.WithCancel(context.Background())
	requestID := m.requestID
	parent := m.loadContext
	checkCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		result := detector.Probe(ctx, presence.Target{DeviceID: device.ID, IPAddress: device.IPAddress, VerifyPort: port}, 2500*time.Millisecond)
		return probeResultMsg{requestID: requestID, deviceID: device.ID, status: string(result.Status), method: string(result.Method), message: result.Message}
	}
	return tea.Batch(checkCmd, motionCmd)
}
