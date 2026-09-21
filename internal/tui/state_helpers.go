package tui

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
	"github.com/aklkbqx/wol/internal/wol"
	"github.com/charmbracelet/lipgloss"
)

func (m *WakeModel) filteredDevices() []store.Device {
	filter := strings.ToLower(strings.TrimSpace(m.filter))
	if filter == "" {
		return append([]store.Device(nil), m.devices...)
	}
	items := make([]store.Device, 0)
	for _, item := range m.devices {
		text := strings.ToLower(strings.Join([]string{item.Name, item.MACAddress, item.IPAddress, item.WakeRelayID}, " "))
		if strings.Contains(text, filter) {
			items = append(items, item)
		}
	}
	return items
}

func (m *WakeModel) relayList() []store.WakeRelay { return append([]store.WakeRelay(nil), m.relays...) }

func filterMessage(value string) string {
	if value == "" {
		return "Showing all machines."
	}
	return "Filtering machines by " + value + "."
}

func statusNeedsAttention(value string) bool {
	value = strings.ToLower(value)
	for _, word := range []string{"failed", "invalid", "required", "unavailable", "could not"} {
		if strings.Contains(value, word) {
			return true
		}
	}
	return false
}

func routeLabel(route wakeservice.Route) string {
	if route.Kind == "relay" {
		return "relay " + route.Name
	}
	if route.Destination == nil {
		return "direct"
	}
	return route.Destination.String()
}

func (m *WakeModel) deviceState(device store.Device) string {
	if !device.Enabled {
		return "DISABLED"
	}
	if status := m.presence[device.ID]; status != "" {
		return strings.ToUpper(status)
	}
	return "UNKNOWN"
}

type wakeCapability struct {
	state  string
	detail string
}

func (m *WakeModel) wakeCapability(device store.Device) wakeCapability {
	if !device.Enabled {
		return wakeCapability{state: "BLOCKED", detail: "machine disabled (use f to force)"}
	}
	if _, err := wol.ParseMAC(device.MACAddress); err != nil {
		return wakeCapability{state: "BLOCKED", detail: "invalid MAC address"}
	}
	if strings.EqualFold(strings.TrimSpace(device.WakeStrategy), "relay") || strings.TrimSpace(device.WakeRelayID) != "" {
		if strings.TrimSpace(device.WakeRelayID) == "" {
			return wakeCapability{state: "BLOCKED", detail: "relay route is missing"}
		}
		for _, relay := range m.relays {
			if relay.ID != device.WakeRelayID {
				continue
			}
			if !relay.Enabled {
				return wakeCapability{state: "BLOCKED", detail: "relay " + fitText(relay.Name, 18) + " is disabled"}
			}
			if strings.TrimSpace(relay.Address) == "" {
				return wakeCapability{state: "BLOCKED", detail: "relay has no SSH address"}
			}
			if relay.Port < 0 || relay.Port > 65535 {
				return wakeCapability{state: "BLOCKED", detail: "relay has invalid SSH port"}
			}
			if transport := strings.TrimSpace(relay.Transport); transport != "" && !strings.EqualFold(transport, "ssh_etherwake") {
				return wakeCapability{state: "BLOCKED", detail: "relay transport is unsupported"}
			}
			return wakeCapability{state: "READY", detail: "relay " + fitText(relay.Name, 18)}
		}
		return wakeCapability{state: "BLOCKED", detail: "relay route not found"}
	}

	destination, port := m.routeTarget(device)
	if ip := net.ParseIP(destination); ip == nil || ip.To4() == nil {
		return wakeCapability{state: "BLOCKED", detail: "invalid broadcast address"}
	}
	if port < 1 || port > 65535 {
		return wakeCapability{state: "BLOCKED", detail: "invalid UDP port"}
	}
	return wakeCapability{state: "READY", detail: fmt.Sprintf("direct broadcast %s:%d", destination, port)}
}

func (m *WakeModel) remoteCapability(device store.Device) (state, detail string) {
	if m.streaming[device.ID] {
		return "STREAMING", "x to disconnect"
	}
	profile, ok := m.profiles[device.ID]
	if !ok || !profile.Enabled {
		return "SETUP", "press p to set up stream"
	}
	if err := validateRemoteProfile(profile); err != nil {
		return "SETUP", "profile incomplete; press p"
	}
	if streamProfile(profile) {
		return "READY", "sunshine · moonlight"
	}
	return "READY", strings.ToUpper(profile.Protocol) + " · native"
}

func (m *WakeModel) presenceDetail(device store.Device) string {
	state := powerWord(m.deviceState(device))
	if method := strings.TrimSpace(m.presenceMethod[device.ID]); method != "" && method != "none" {
		return state + " via " + method
	}
	return state
}

func powerWord(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "ONLINE":
		return "online"
	case "OFFLINE":
		return "asleep"
	case "DISABLED":
		return "off"
	case "CHECKING":
		return "check"
	default:
		return "unknown"
	}
}

func (m *WakeModel) actionWord(device store.Device) string {
	if m.streaming[device.ID] {
		return "streaming"
	}
	if m.wakeCapability(device).state == "BLOCKED" {
		return "blocked"
	}
	if streamProfile(m.profiles[device.ID]) {
		return "stream"
	}
	if _, ok := m.profiles[device.ID]; ok {
		return "remote"
	}
	return "setup"
}

func actionState(word string) string {
	switch word {
	case "streaming", "stream", "remote", "online":
		return "ONLINE"
	case "blocked":
		return "BLOCKED"
	case "setup":
		return "UNKNOWN"
	default:
		return "READY"
	}
}

func (m *WakeModel) remoteSummary(devices []store.Device) (configured, setup int) {
	for _, device := range devices {
		state, _ := m.remoteCapability(device)
		if state == "READY" {
			configured++
		} else {
			setup++
		}
	}
	return configured, setup
}

func (m *WakeModel) routeText(device store.Device) string {
	if strings.EqualFold(device.WakeStrategy, "relay") || device.WakeRelayID != "" {
		if device.WakeRelayID == "" {
			return "relay missing"
		}
		for _, relay := range m.relays {
			if relay.ID == device.WakeRelayID {
				return "relay " + relay.Name
			}
		}
		return "relay unavailable"
	}
	destination, port := m.routeTarget(device)
	return fmt.Sprintf("broadcast %s:%d", destination, port)
}

func (m *WakeModel) verifyText(device store.Device) string {
	if strings.TrimSpace(device.IPAddress) == "" {
		return "no IP address"
	}
	port := device.VerifyPort
	if profile, ok := m.profiles[device.ID]; port == 0 && ok {
		port = profile.VerifyPort
	}
	if port == 0 {
		return device.IPAddress + " (automatic local probes)"
	}
	if port < 1 || port > 65535 {
		return "invalid TCP port"
	}
	return net.JoinHostPort(device.IPAddress, strconv.Itoa(port))
}

func (m *WakeModel) routeTarget(device store.Device) (destination string, port int) {
	destination = strings.TrimSpace(device.BroadcastAddress)
	port = device.Port
	if device.SiteID != "" {
		for _, site := range m.sites {
			if site.ID != device.SiteID {
				continue
			}
			if destination == "" {
				destination = strings.TrimSpace(site.BroadcastAddress)
			}
			if port == 0 {
				port = site.DefaultPort
			}
			break
		}
	}
	if destination == "" {
		destination = "255.255.255.255"
	}
	if port == 0 {
		port = 9
	}
	return destination, port
}

func (m *WakeModel) machineSummary(devices []store.Device) (online, offline, unknown, disabled, checking, ready, blocked int) {
	for _, device := range devices {
		switch m.deviceState(device) {
		case "ONLINE":
			online++
		case "OFFLINE":
			offline++
		case "CHECKING":
			checking++
		case "DISABLED":
			disabled++
		default:
			unknown++
		}
		if m.wakeCapability(device).state == "READY" {
			ready++
		} else {
			blocked++
		}
	}
	return
}

func stateStyle(theme Theme, state string) lipgloss.Style {
	switch strings.ToUpper(state) {
	case "ONLINE", "READY", "SENT", "REACHABLE", "CONFIGURED", "STREAMING":
		return theme.success()
	case "FAILED", "DISABLED", "BLOCKED", "TIMEOUT", "INVALID":
		return theme.danger()
	case "OFFLINE":
		return theme.muted()
	case "SENDING", "CHECKING":
		return theme.accent()
	default:
		return theme.muted()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
