package tui

import (
	"context"
	"fmt"
	"github.com/aklkbqx/wol/internal/netutil"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/scanner"
	"github.com/aklkbqx/wol/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *WakeModel) beginLANDiscover() tea.Cmd {
	m.lanID++
	id := m.lanID
	m.lanOpen = true
	iface := m.lanIface
	m.status = "Scanning LAN neighbors..."
	repo := m.repository
	devices := append([]store.Device(nil), m.devices...)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		neighbors, err := scanner.ScanLAN(ctx)
		if err != nil {
			return lanDiscoverMsg{id: id, err: err}
		}
		ifaces := []string{}
		seen := map[string]bool{}
		filtered := neighbors[:0]
		for _, n := range neighbors {
			if !seen[n.Iface] {
				seen[n.Iface] = true
				ifaces = append(ifaces, n.Iface)
			}
			if iface == "" || n.Iface == iface {
				filtered = append(filtered, n)
			}
		}
		sort.Strings(ifaces)
		neighbors = filtered
		known, moved, unknown := scanner.ClassifyLAN(devices, neighbors)
		updated, err := scanner.SyncDeviceIPs(ctx, repo, devices, neighbors)
		if err != nil {
			return lanDiscoverMsg{id: id, err: err}
		}
		devices, err = repo.ListDevices(ctx)
		if err != nil {
			return lanDiscoverMsg{id: id, err: err}
		}
		profiles, err := repo.ListRemoteProfiles(ctx)
		return lanDiscoverMsg{id: id, ifaces: ifaces, devices: devices, profiles: profiles, known: known, moved: moved, unknown: unknown, updated: updated, err: err}
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
	m.lanIfaces = msg.ifaces
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
		m.lanID++
		m.status = "Closed LAN discover."
	case "f":
		choices := append([]string{""}, m.lanIfaces...)
		idx := 0
		for i, v := range choices {
			if v == m.lanIface {
				idx = i
			}
		}
		m.lanIface = choices[(idx+1)%len(choices)]
		return m.beginLANDiscover()
	case " ":
		if m.lanSelected < len(m.lanUnknown) {
			if m.lanMarked == nil {
				m.lanMarked = map[string]bool{}
			}
			key := m.lanUnknown[m.lanSelected].MAC
			m.lanMarked[key] = !m.lanMarked[key]
		}
		return nil
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
	selected := []presence.Neighbor{}
	for _, n := range m.lanUnknown {
		if m.lanMarked[n.MAC] {
			selected = append(selected, n)
		}
	}
	if len(selected) > 0 {
		m.lanOpen = false
		m.lanMarked = nil
		m.lanQueue = selected[1:]
		return m.beginAddFromLAN(selected[0])
	}
	if m.lanSelected < len(m.lanUnknown) {
		neighbor := m.lanUnknown[m.lanSelected]
		m.lanOpen = false
		return m.beginAddFromLAN(neighbor)
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

type lanPrepareMsg struct {
	id       uint64
	neighbor presence.Neighbor
	platform string
	port     int
}

func (m *WakeModel) beginAddFromLAN(neighbor presence.Neighbor) tea.Cmd {
	m.lanPrepareID++
	id := m.lanPrepareID
	m.lanPreparing = true
	m.status = "Checking discovered services. Esc cancels."
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		platform, port := scanner.GuessIdentity(ctx, neighbor.IP)
		return lanPrepareMsg{id: id, neighbor: neighbor, platform: platform, port: port}
	}
}
func (m *WakeModel) showNeighborForm(msg lanPrepareMsg) {
	neighbor := msg.neighbor
	name := scanner.UniqueDeviceName(m.devices, scanner.SuggestName(neighbor.IP, neighbor.MAC))
	site := ""
	if m.siteFilter != "" && m.siteFilter != "unassigned" {
		site = m.siteName(m.siteFilter)
	}
	values := []string{name, neighbor.MAC, neighbor.IP, netutil.LocalBroadcast(neighbor.IP, neighbor.Iface), "9", neighbor.Iface, strconv.Itoa(msg.port), "broadcast", "", site}
	m.form = newWakeForm(deviceForm, "", deviceFormLabels(), values, m.theme)
	m.status = fmt.Sprintf("Review %s · suggested %s · %d more selected. Ctrl+S saves; Esc cancels remaining.", name, msg.platform, len(m.lanQueue))
}

func (m *WakeModel) renderLANDiscover(width int) string {
	rowWidth := max(1, width)
	rows := []string{
		m.theme.muted().Render(fitText(fmt.Sprintf("lan %s · %d new · %d known · %d ip updates", m.lanIface, len(m.lanUnknown), len(m.lanHosts), m.lanUpdated), rowWidth)),
		"",
	}
	if m.lanCount() == 0 {
		rows = append(rows, m.theme.muted().Render(fitText("no neighbors with complete ARP entries", rowWidth)))
		return strings.Join(rows, "\n")
	}
	start := max(0, m.lanSelected-max(1, m.height-12)+1)
	end := start + max(1, m.height-12)
	index := 0
	for _, neighbor := range m.lanUnknown {
		if index >= start && index < end {
			state := "new"
			if m.lanMarked[neighbor.MAC] {
				state = "selected"
			}
			rows = append(rows, m.lanLine(index, neighbor.IP, neighbor.MAC, state, rowWidth))
		}
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
		if index >= start && index < end {
			rows = append(rows, m.lanLine(index, name, host.Neighbor.MAC, state, rowWidth))
		}
		index++
	}
	rows = append(rows, "", m.theme.muted().Render(fitText("space select · enter review · f interface · n scan · esc back", rowWidth)))
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
