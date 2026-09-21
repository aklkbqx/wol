package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/scanner"
	"github.com/aklkbqx/wol/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *WakeModel) beginLANDiscover() tea.Cmd {
	m.status = "Scanning LAN neighbors..."
	repo := m.repository
	devices := append([]store.Device(nil), m.devices...)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		neighbors, err := scanner.ScanLAN(ctx)
		if err != nil {
			return lanDiscoverMsg{err: err}
		}
		known, moved, unknown := scanner.ClassifyLAN(devices, neighbors)
		updated, err := scanner.SyncDeviceIPs(ctx, repo, devices, neighbors)
		if err != nil {
			return lanDiscoverMsg{err: err}
		}
		devices, err = repo.ListDevices(ctx)
		if err != nil {
			return lanDiscoverMsg{err: err}
		}
		profiles, err := repo.ListRemoteProfiles(ctx)
		return lanDiscoverMsg{devices: devices, profiles: profiles, known: known, moved: moved, unknown: unknown, updated: updated, err: err}
	}
}

func (m *WakeModel) applyLANDiscover(msg lanDiscoverMsg) tea.Cmd {
	if msg.err != nil {
		m.status = "LAN scan failed: " + msg.err.Error()
		return nil
	}
	m.devices = msg.devices
	m.profiles = make(map[string]store.RemoteProfile, len(msg.profiles))
	for _, profile := range msg.profiles {
		m.profiles[profile.DeviceID] = profile
	}
	m.lanOpen = true
	m.lanSelected = 0
	m.lanUpdated = msg.updated
	m.lanUnknown = msg.unknown
	m.lanHosts = append(append([]scanner.LANHost(nil), msg.moved...), msg.known...)
	m.status = fmt.Sprintf("LAN: %d known · %d new · %d IP updates. Enter adds a new host.", len(msg.known)+len(msg.moved), len(msg.unknown), msg.updated)
	return nil
}

func (m *WakeModel) handleLANKey(keyName string) tea.Cmd {
	switch keyName {
	case "esc", "q":
		m.lanOpen = false
		m.status = "Closed LAN discover."
	case "n", "r":
		return m.beginLANDiscover()
	case "j", "down":
		if max := m.lanCount() - 1; m.lanSelected < max {
			m.lanSelected++
		}
	case "k", "up":
		if m.lanSelected > 0 {
			m.lanSelected--
		}
	case "enter":
		return m.addSelectedLANHost()
	}
	return nil
}

func (m *WakeModel) lanCount() int {
	return len(m.lanUnknown) + len(m.lanHosts)
}

func (m *WakeModel) addSelectedLANHost() tea.Cmd {
	if m.lanSelected < len(m.lanUnknown) {
		neighbor := m.lanUnknown[m.lanSelected]
		m.lanOpen = false
		m.beginAddFromLAN(neighbor)
		return nil
	}
	idx := m.lanSelected - len(m.lanUnknown)
	if idx >= 0 && idx < len(m.lanHosts) && m.lanHosts[idx].Device != nil {
		id := m.lanHosts[idx].Device.ID
		for i, device := range m.filteredDevices() {
			if device.ID == id {
				m.selected = i
				break
			}
		}
		m.lanOpen = false
		m.status = m.lanHosts[idx].Device.Name + " is already in inventory."
	}
	return nil
}

func (m *WakeModel) beginAddFromLAN(neighbor presence.Neighbor) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	platform, verifyPort := scanner.GuessIdentity(ctx, neighbor.IP)
	name := scanner.UniqueDeviceName(m.devices, scanner.SuggestName(neighbor.IP, neighbor.MAC))
	values := []string{
		name,
		neighbor.MAC,
		neighbor.IP,
		scanner.BroadcastOf(neighbor.IP),
		"9",
		neighbor.Iface,
		strconv.Itoa(verifyPort),
		"broadcast",
		"",
	}
	m.form = newWakeForm(deviceForm, "", deviceFormLabels(), values, m.theme)
	if platform != "unknown" {
		m.status = fmt.Sprintf("Add %s (%s). Enter saves.", name, platform)
	} else {
		m.status = "Add discovered machine: fill each field and press Enter."
	}
}

func (m *WakeModel) renderLANDiscover(width int) string {
	rowWidth := max(1, width)
	rows := []string{
		m.theme.muted().Render(fitText(fmt.Sprintf("lan  %d new  %d known  %d ip updates", len(m.lanUnknown), len(m.lanHosts), m.lanUpdated), rowWidth)),
		"",
	}
	if m.lanCount() == 0 {
		rows = append(rows, m.theme.muted().Render(fitText("no neighbors with complete ARP entries", rowWidth)))
		return strings.Join(rows, "\n")
	}
	index := 0
	for _, neighbor := range m.lanUnknown {
		rows = append(rows, m.lanLine(index, neighbor.IP, neighbor.MAC, "new", rowWidth))
		index++
	}
	for _, host := range m.lanHosts {
		state := "known"
		if host.Moved {
			state = "moved"
		}
		name := host.Neighbor.IP
		if host.Device != nil {
			name = host.Device.Name + "  " + host.Neighbor.IP
		}
		rows = append(rows, m.lanLine(index, name, host.Neighbor.MAC, state, rowWidth))
		index++
	}
	rows = append(rows, "", m.theme.muted().Render(fitText("enter add/select   n rescan   esc back", rowWidth)))
	return strings.Join(rows, "\n")
}

func (m *WakeModel) lanLine(index int, name, mac, state string, width int) string {
	marker := " "
	if index == m.lanSelected {
		marker = m.theme.accent().Render(m.theme.Glyph("arrow"))
	}
	line := marker + " " + padVisible(fitText(name, 28), 28) + "  " + padVisible(state, 8) + "  " + mac
	return fitText(line, width)
}
