package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	if len(devices) == 0 {
		return nil
	}
	targets := make([]presence.Target, 0, len(devices))
	for _, device := range devices {
		targets = append(targets, presence.Target{
			DeviceID:   device.ID,
			IPAddress:  device.IPAddress,
			VerifyPort: device.VerifyPort,
		})
	}
	detector := m.presenceDetector()
	if parent == nil {
		parent = context.Background()
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 4*time.Second)
		defer cancel()
		result := detector.ProbeBatch(ctx, targets, 2500*time.Millisecond)
		statuses := make(map[string]string, len(result.Results))
		for _, item := range result.Results {
			statuses[item.DeviceID] = string(item.Status)
		}
		return probeBatchMsg{requestID: requestID, kind: kind, statuses: statuses, summary: result.Summary}
	}
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
	device := devices[m.selected]
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
	m.phase = phaseCheckingMachine
	m.loading = true
	m.loadingStage = stagePresence
	m.loadingTarget = device.Name
	if port > 0 {
		m.status = fmt.Sprintf("Checking power at %s:%d...", device.IPAddress, port)
	} else {
		m.status = fmt.Sprintf("Checking power for %s with automatic local probes...", device.IPAddress)
	}
	detector := m.presenceDetector()
	m.checking = true
	m.motion.TriggerStage(time.Now(), StageSignal, 15*time.Second, 0, 0)
	m.loadContext, m.loadCancel = context.WithCancel(context.Background())
	requestID := m.requestID
	parent := m.loadContext
	checkCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		result := detector.Probe(ctx, presence.Target{DeviceID: device.ID, IPAddress: device.IPAddress, VerifyPort: port}, 2500*time.Millisecond)
		return probeResultMsg{requestID: requestID, deviceID: device.ID, status: string(result.Status)}
	}
	return tea.Batch(checkCmd, m.motionTick())
}
