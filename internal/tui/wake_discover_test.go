package tui

import (
	"strings"
	"testing"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/charmbracelet/lipgloss"
)

func TestLANDiscoverRefreshesRemoteHost(t *testing.T) {
	m := NewWakeModel(nil, "test", "test")
	m.phase, m.loading = phaseReady, false
	m.devices = []store.Device{{ID: "pc", IPAddress: "192.0.2.10"}}
	m.profiles["pc"] = store.RemoteProfile{DeviceID: "pc", Host: "192.0.2.10"}
	m.applyLANDiscover(lanDiscoverMsg{
		updated:  1,
		devices:  []store.Device{{ID: "pc", IPAddress: "192.0.2.20"}},
		profiles: []store.RemoteProfile{{DeviceID: "pc", Host: "192.0.2.20"}},
	})
	m.handleLANKey("esc")
	if m.lanOpen || m.devices[0].IPAddress != "192.0.2.20" || m.profiles["pc"].Host != "192.0.2.20" {
		t.Fatalf("discover left stale machine or remote state: %+v %+v", m.devices, m.profiles)
	}
}

func TestLANDiscoverFitsNarrowTerminal(t *testing.T) {
	m := NewWakeModel(nil, "test", "test")
	m.phase, m.loading, m.lanOpen = phaseReady, false, true
	m.theme = NewTheme(false, true)
	for _, count := range []int{0, 1} {
		m.lanUnknown = nil
		if count > 0 {
			m.lanUnknown = []presence.Neighbor{{IP: "192.0.2.20", MAC: "02:00:00:00:00:20"}}
		}
		for _, width := range []int{8, 18, 24, 40, 80} {
			m.width = width
			for _, line := range strings.Split(m.View(), "\n") {
				if lipgloss.Width(stripANSI(line)) > width {
					t.Fatalf("width %d count %d overflow: %q", width, count, line)
				}
			}
		}
	}
}
