package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/store"
)

func (m *WakeModel) renderInspector(devices []store.Device, width int) string {
	if len(devices) == 0 {
		return m.theme.muted().Render("add a machine to start")
	}
	device, _ := m.selectedDevice(devices)
	if m.actionPicker {
		return m.renderActionPicker(devices, width)
	}
	active := m.waking || m.opening || m.shuttingDown
	rowWidth := max(1, width)
	power := powerWord(m.deviceState(device))
	wake := m.wakeCapability(device)
	action := m.actionWord(device)
	if active && (m.actionTargetID == "" || device.ID == m.actionTargetID) {
		spinner := "●"
		if m.motion.Enabled {
			if m.theme.ASCII {
				asciiSpinners := []string{"-", "\\", "|", "/"}
				spinner = asciiSpinners[(int(m.frame)/2)%len(asciiSpinners)]
			} else {
				spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
				spinner = spinners[(int(m.frame)/2)%len(spinners)]
			}
		}
		label := "waking"
		if m.shuttingDown {
			label = "stopping"
		} else if m.opening && !m.waking {
			label = "connecting"
		}
		power = spinner + " " + label
	}
	lock := ""
	if active {
		lock = "locked · "
	}
	divider := strings.Repeat("─", min(rowWidth, 24))
	if m.theme.ASCII {
		divider = strings.Repeat("-", min(rowWidth, 24))
	}
	lines := []string{
		m.theme.accent().Render(fitText(lock+"INSPECTOR", rowWidth)),
		m.theme.title().Render(fitText(device.Name, rowWidth)),
		fitText(power+"  "+action, rowWidth),
		m.theme.muted().Render(fitText(wake.detail, rowWidth)),
	}
	if active {
		lines = append(lines, m.renderSignalPath(device, rowWidth), m.theme.muted().Render("esc cancel"))
	} else {
		lines = append(lines, m.theme.muted().Render("enter  wake or stream"))
	}
	lines = append(lines,
		m.theme.muted().Render(divider),
		m.theme.muted().Render("IP   : ")+fitText(device.IPAddress, max(1, rowWidth-7)),
		m.theme.muted().Render("Seen : ")+fitText(m.presenceDetail(device), max(1, rowWidth-7)),
		m.theme.muted().Render("MAC  : ")+fitText(device.MACAddress, max(1, rowWidth-7)),
		m.theme.muted().Render("Route: ")+fitText(m.routeText(device), max(1, rowWidth-7)),
	)
	return strings.Join(lines, "\n")
}

func (m *WakeModel) renderActionPicker(devices []store.Device, width int) string {
	if len(devices) == 0 {
		return "no machine selected"
	}
	device := devices[min(m.selected, len(devices)-1)]
	remoteLabel := "remote"
	if profile, ok := m.profiles[device.ID]; ok && streamProfile(profile) {
		remoteLabel = "stream"
	} else if _, ok := m.profiles[device.ID]; !ok {
		remoteLabel = "stream"
	}
	items := []struct{ key, label string }{
		{"w", "wake"},
		{"c", remoteLabel},
		{"s", "check"},
		{"P", "power off"},
		{"esc", "cancel"},
	}
	rows := []string{m.theme.title().Render(fitText(device.Name, max(1, width))), ""}
	for i, item := range items {
		marker := " "
		label := item.label
		if i == m.pickerSelected {
			marker = m.theme.Glyph("arrow")
			label = m.theme.accent().Render(label)
		}
		rows = append(rows, fitText(fmt.Sprintf("%s %s", marker, label), max(1, width)))
	}
	return strings.Join(rows, "\n")
}

func (m *WakeModel) renderSignalPath(device store.Device, width int) string {
	if m.action == "shutdown-wait" {
		return m.renderActionPath([]string{"DESK", "SSH", strings.ToUpper(fitText(device.Name, 12))}, width)
	}
	if m.action == "wake-remote" {
		last := "REMOTE"
		if profile, ok := m.profiles[device.ID]; ok && streamProfile(profile) {
			last = "STREAM"
		}
		return m.renderActionPath([]string{"WAKE", "WAIT", last}, width)
	}
	route := "LAN"
	if strings.EqualFold(device.WakeStrategy, "relay") || device.WakeRelayID != "" {
		route = "RELAY"
	}
	return m.renderActionPath([]string{"DESK", route, strings.ToUpper(fitText(device.Name, 12))}, width)
}

func (m *WakeModel) pathPositionAt(now time.Time) int {
	if !m.motion.Enabled {
		return 0
	}
	if m.action == "wake-wait" || m.action == "wake-remote" || m.action == "shutdown-wait" {
		if !m.motion.Active(now) {
			return 2
		}
		cycleDuration := 900 * time.Millisecond
		stepDuration := cycleDuration / 3
		elapsed := now.Sub(m.motion.Started)
		if elapsed < 0 {
			elapsed = 0
		}
		return int((elapsed % cycleDuration) / stepDuration)
	}
	if m.waking || m.opening || m.shuttingDown {
		if !m.motion.Active(now) {
			return 2
		}
		t := m.motion.T(now)
		if t < 1.0/3 {
			return 0
		}
		if t < 2.0/3 {
			return 1
		}
		return 2
	}
	if m.motion.Active(now) {
		t := m.motion.T(now)
		if t < 1.0/3 {
			return 0
		}
		if t < 2.0/3 {
			return 1
		}
		return 2
	}
	return 0
}

func signalPath(steps []string, position int, ascii bool) string {
	connector, pulse := "──", "●"
	if ascii {
		connector, pulse = "--", "*"
	}
	left, right := connector+">", connector+">"
	name := steps[2]
	switch position {
	case 1:
		right = pulse + connector + ">"
	case 2:
		name = pulse + name
	default:
		left = pulse + connector + ">"
	}
	return steps[0] + " " + left + " " + steps[1] + " " + right + " " + name
}

func (m *WakeModel) renderActionPath(steps []string, width int) string {
	path := signalPath(steps, m.pathPositionAt(time.Now()), m.theme.ASCII)
	return fitText(m.theme.accent().Render(path), width)
}
