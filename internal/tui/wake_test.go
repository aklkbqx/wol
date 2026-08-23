package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestWakeDeskViewFitsTerminalWidths(t *testing.T) {
	repository, err := store.Open(t.TempDir() + "/wol.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	device, err := repository.CreateDevice(context.Background(), store.Device{
		Name: "office-workstation-with-a-long-name", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200",
		BroadcastAddress: "192.168.50.255", Port: 9, Interface: "br-lan", VerifyPort: 3389,
		WakeStrategy: "broadcast", DeviceType: "desktop", Platform: "windows", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.CreateWakeRelay(context.Background(), store.WakeRelay{Name: "router-relay-with-a-long-name", Address: "198.51.100.1", Port: 22, Interface: "br-lan", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	model := NewWakeModel(repository, "0.3.0", "aklkbqx")
	model.theme = NewTheme(false, true)
	model.devices = []store.Device{device}
	model.relays, err = repository.ListWakeRelays(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{18, 24, 40, 60, 80, 110, 140} {
		model.width, model.height = width, 32
		for _, colors := range []bool{false, true} {
			model.theme = NewTheme(colors, !colors)
			for _, tab := range []int{0, 1, 2} {
				model.tab = tab
				view := model.View()
				for lineNo, line := range strings.Split(view, "\n") {
					if got := lipgloss.Width(stripANSI(line)); got > width {
						t.Fatalf("width %d tab %d line %d overflows at %d: %q", width, tab, lineNo, got, line)
					}
				}
			}
		}
	}
	view := model.View()
	if !strings.Contains(view, "WOL WAKE DESK") || !strings.Contains(view, "Credit: aklkbqx") {
		t.Fatalf("wake desk header missing version/credit:\n%s", view)
	}
	if strings.Contains(view, ".data/dev/wol.db") || strings.Contains(view, "SQLite") {
		t.Fatalf("wake desk exposed storage implementation details:\n%s", view)
	}
}

func TestWakeDeskSignalPathOnlyAppearsDuringWake(t *testing.T) {
	model := &WakeModel{
		width: 80, height: 32, theme: NewTheme(false, true), motion: NewMotion(true),
		devices:  []store.Device{{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", BroadcastAddress: "192.168.50.255", Enabled: true}},
		presence: map[string]string{"one": "offline"},
	}
	if view := model.View(); strings.Contains(view, "DESK *-->") {
		t.Fatalf("idle view contains an animated signal path:\n%s", view)
	}
	model.waking = true
	model.motion.Trigger(time.Now(), time.Second)
	view := model.View()
	if !strings.Contains(view, "DESK *--> LAN --> WINDOWS") {
		t.Fatalf("wake view missing signal path:\n%s", view)
	}
}

func TestCompactWakeDeskFitsShortTerminalHeight(t *testing.T) {
	model := &WakeModel{
		width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(false),
		devices: []store.Device{
			{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", Enabled: true},
			{ID: "two", Name: "private", MACAddress: "00:11:22:33:44:66", IPAddress: "192.168.50.5", Enabled: true},
			{ID: "three", Name: "private2", MACAddress: "00:11:22:33:44:77", IPAddress: "192.168.50.6", Enabled: true},
		},
		presence: map[string]string{"one": "online", "two": "unknown", "three": "offline"},
		status:   "Power scan complete.",
	}
	view := model.View()
	if lines := len(strings.Split(strings.TrimSuffix(view, "\n"), "\n")); lines > model.height {
		t.Fatalf("compact view uses %d lines in a %d-line terminal:\n%s", lines, model.height, view)
	}
	if !strings.Contains(view, "WOL WAKE DESK") || !strings.Contains(view, "1 Machines") || !strings.Contains(view, "P:") || !strings.Contains(view, "W:") {
		t.Fatalf("compact view lost essential context:\n%s", view)
	}
}

func TestWakeDeskKeyboardNavigationAndFilter(t *testing.T) {
	model := &WakeModel{
		width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(false),
		devices:  []store.Device{{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", Enabled: true}, {ID: "two", Name: "private", MACAddress: "00:11:22:33:44:66", Enabled: true}},
		presence: map[string]string{}, status: "ready",
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if model.selected != 1 {
		t.Fatalf("j selected %d, want 1", model.selected)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.filter != "p" || model.filtering {
		t.Fatalf("filter state = %q/%v, want p/false", model.filter, model.filtering)
	}
	if len(model.filteredDevices()) != 1 || model.filteredDevices()[0].Name != "private" {
		t.Fatalf("filtered devices = %#v", model.filteredDevices())
	}
}

func TestWakeDeskShowsPowerAndWakeStatesSeparately(t *testing.T) {
	model := &WakeModel{
		width:  60,
		height: 40,
		theme:  NewTheme(false, true),
		devices: []store.Device{
			{ID: "online", Name: "windows", MACAddress: "02:00:00:00:00:5d", IPAddress: "192.168.50.200", BroadcastAddress: "192.168.50.255", Port: 9, Enabled: true},
			{ID: "unknown", Name: "private", MACAddress: "00:11:22:33:44:66", IPAddress: "192.168.50.5", Enabled: true},
			{ID: "blocked", Name: "broken", MACAddress: "not-a-mac", IPAddress: "192.168.50.6", Enabled: true},
		},
		presence: map[string]string{"online": "online", "unknown": "unknown", "blocked": "offline"},
		status:   "ready",
	}
	view := model.View()
	for _, want := range []string{"POWER", "WAKE", "ONLINE", "UNKNOWN", "OFFLINE", "READY", "BLOCKED", "192.168.50.200"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	for lineNo, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(stripANSI(line)); got > model.width {
			t.Fatalf("line %d overflows width %d: %d %q", lineNo, model.width, got, line)
		}
	}
	t.Logf("rendered narrow fleet:\n%s", view)
}

func TestWakeCapabilityExplainsReadiness(t *testing.T) {
	model := &WakeModel{
		sites: []store.Site{{ID: "site-a", Name: "lab", BroadcastAddress: "10.0.0.255", DefaultPort: 7}},
		relays: []store.WakeRelay{
			{ID: "ready-relay", Name: "router-a", Address: "198.51.100.1", Port: 22, Transport: "ssh_etherwake", Enabled: true},
			{ID: "off-relay", Name: "router-b", Enabled: false},
		},
	}
	tests := []struct {
		name, wantState, wantDetail string
		device                      store.Device
	}{
		{name: "direct default", wantState: "READY", wantDetail: "255.255.255.255:9", device: store.Device{MACAddress: "00:11:22:33:44:55", Enabled: true}},
		{name: "site route", wantState: "READY", wantDetail: "10.0.0.255:7", device: store.Device{MACAddress: "00:11:22:33:44:55", SiteID: "site-a", Enabled: true}},
		{name: "invalid mac", wantState: "BLOCKED", wantDetail: "invalid MAC", device: store.Device{MACAddress: "bad", Enabled: true}},
		{name: "missing relay", wantState: "BLOCKED", wantDetail: "not found", device: store.Device{MACAddress: "00:11:22:33:44:55", WakeStrategy: "relay", WakeRelayID: "missing", Enabled: true}},
		{name: "disabled relay", wantState: "BLOCKED", wantDetail: "disabled", device: store.Device{MACAddress: "00:11:22:33:44:55", WakeStrategy: "relay", WakeRelayID: "off-relay", Enabled: true}},
		{name: "ready relay", wantState: "READY", wantDetail: "router-a", device: store.Device{MACAddress: "00:11:22:33:44:55", WakeStrategy: "relay", WakeRelayID: "ready-relay", Enabled: true}},
		{name: "disabled machine", wantState: "BLOCKED", wantDetail: "disabled", device: store.Device{MACAddress: "00:11:22:33:44:55", Enabled: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.wakeCapability(tt.device)
			if got.state != tt.wantState || !strings.Contains(got.detail, tt.wantDetail) {
				t.Fatalf("capability = %#v, want %s containing %q", got, tt.wantState, tt.wantDetail)
			}
		})
	}
}

func TestWakeDeskRefreshStartsPresenceScan(t *testing.T) {
	model := &WakeModel{
		presence: map[string]string{},
		detector: presence.NewDetector(
			presence.WithTCPPorts(nil),
			presence.WithPing(func(context.Context, string, time.Duration) (time.Duration, error) {
				return time.Millisecond, nil
			}),
		),
	}
	devices := []store.Device{{ID: "one", IPAddress: "192.168.50.200", VerifyPort: 3389, Enabled: true}}
	cmd := model.startPresenceScan(devices)
	if !model.checking || model.presence["one"] != "checking" {
		t.Fatalf("scan did not mark device checking: %#v/%v", model.presence, model.checking)
	}
	message, ok := cmd().(probeBatchMsg)
	if !ok || message.summary.Online != 1 || message.statuses["one"] != "online" {
		t.Fatalf("scan message = %#v, want one online result", message)
	}
}
