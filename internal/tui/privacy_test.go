package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aklkbqx/wol/internal/fleet"
	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
	"github.com/aklkbqx/wol/internal/wol"
	tea "github.com/charmbracelet/bubbletea"
)

// These tests catch identifiers escaping through any renderer, including values
// truncated before rendering and endpoint details absent from the inventory.
func TestPrivacyRenderedSurfaces(t *testing.T) {
	t.Setenv("WOL_TUI_REDACT", "1")
	cases := []struct {
		name, word string
		setup      func(*WakeModel)
	}{
		{"selected", "unreachable", func(m *WakeModel) {}},
		{"actions", "wake", func(m *WakeModel) { m.actionPicker = true }},
		{"waking", "waking", func(m *WakeModel) {
			m.waking = true
			m.actionTargetID = "one"
			m.actionTarget = "hush-device"
			m.action = "wake"
		}},
		{"online", "online", func(m *WakeModel) { m.presence["one"] = "online" }},
		{"failed", "failed", func(m *WakeModel) { m.status = "hush-device · wake failed: dial unlisted-error.internal 198.51.100.2" }},
		{"cancelled", "cancelled", func(m *WakeModel) { m.status = "hush-device · action cancelled." }},
		{"remote", "wake", func(m *WakeModel) { m.opening = true; m.actionTarget = "hush-device" }},
		{"checking", "checking", func(m *WakeModel) { m.phase = phaseCheckingMachine; m.loadingTarget = "hush-device" }},
		{"load-error", "could not load", func(m *WakeModel) {
			m.phase = phaseLoadError
			m.loadingError = "open /private/hush-database: dial unlisted-error.internal"
		}},
		{"routes", "ready", func(m *WakeModel) { m.tab = 1 }},
		{"activity", "failed", func(m *WakeModel) {
			m.tab = 2
			m.history = []store.WakeAttempt{{TargetName: "hush-device", PacketStatus: "failed"}}
		}},
		{"sites", "relay", func(m *WakeModel) { m.tab = 3 }},
		{"site-filter", "unreachable", func(m *WakeModel) { m.devices[0].SiteID = "site"; m.siteFilter = "site" }},
		{"relay-detail", "unreachable", func(m *WakeModel) { m.devices[0].WakeRelayID = "relay" }},
		{"presence-method", "online", func(m *WakeModel) { m.presence["one"] = "online"; m.presenceMethod["one"] = "hush-method" }},
		{"batch-preview", "selected", func(m *WakeModel) { m.batchKind = "wake"; m.batchTargets = m.devices }},
		{"batch", "failed", func(m *WakeModel) {
			m.showBatchResults = true
			m.batchResults = []fleet.Result{{Name: "hush-device", Status: "failed", Method: "hush-method", Message: "dial unlisted-error.internal"}}
		}},
		{"discovery", "new", func(m *WakeModel) {
			m.lanOpen = true
			m.lanIface = "hush-interface"
			m.lanUnknown = []presence.Neighbor{{IP: "198.51.100.2", MAC: "02:00:00:00:00:5d"}}
		}},
		{"form", "saves", func(m *WakeModel) {
			m.form = newWakeForm(deviceForm, "one", []string{"Name", "IP"}, []string{"hush-device", "198.51.100.2"}, m.theme)
			m.form.error = "unlisted-error.internal invalid"
		}},
		{"filter", "filter", func(m *WakeModel) { m.filtering = true; m.filterInput = "hush-filter" }},
	}
	for _, size := range [][2]int{{120, 32}, {40, 28}} {
		for _, color := range []bool{true, false} {
			for _, tc := range cases {
				t.Run(fmt.Sprintf("%dx%d/color=%t/%s", size[0], size[1], color, tc.name), func(t *testing.T) {
					m := privacyTestModel()
					m.width, m.height = size[0], size[1]
					m.theme = NewTheme(color, !color)
					tc.setup(m)
					view := strings.ToLower(stripANSI(m.View()))
					for _, secret := range []string{"hush-", "198.51.100", "02:00:00", "unlisted-error", "/private/"} {
						if strings.Contains(view, secret) {
							t.Errorf("identifier %q leaked:\n%s", secret, view)
						}
					}
					if !strings.Contains(view, tc.word) {
						t.Errorf("truthful state %q missing:\n%s", tc.word, view)
					}
				})
			}
		}
	}
}

func privacyTestModel() *WakeModel {
	m := NewWakeModel(nil, "0.4.18-dev", "test")
	m.phase, m.loading = phaseReady, false
	m.motion = NewMotion(false)
	m.status = ""
	m.devices = []store.Device{{ID: "one", Name: "hush-device", IPAddress: "198.51.100.2", MACAddress: "02:00:00:00:00:5d", BroadcastAddress: "198.51.100.255", Port: 9, Enabled: true}}
	m.presence["one"] = "offline"
	m.relays = []store.WakeRelay{{ID: "relay", Name: "hush-relay", Address: "198.51.100.1", Port: 22, Enabled: true}}
	m.sites = []store.Site{{ID: "site", Name: "hush-site", Subnet: "198.51.100.0/24", WakeRelayID: "relay"}}
	return m
}

func TestPrivacyDefaultDisplayAndFormState(t *testing.T) {
	t.Setenv("WOL_TUI_REDACT", "")
	m := privacyTestModel()
	m.width, m.height = 120, 32
	view := stripANSI(m.View())
	for _, want := range []string{"hush-device", "198.51.100.2", "02:00:00:00:00:5d"} {
		if !strings.Contains(view, want) {
			t.Errorf("default display missing %q", want)
		}
	}
	t.Setenv("WOL_TUI_REDACT", "1")
	m = privacyTestModel()
	m.form = newWakeForm(deviceForm, "one", []string{"Name"}, []string{"hush-device"}, m.theme)
	_ = m.View()
	if m.devices[0].Name != "hush-device" || m.form.inputs[0].Value() != "hush-device" || m.form.values[0] != "hush-device" {
		t.Fatal("presentation mutated inventory or form")
	}
}

func TestPrivacyWakeUsesOriginalNetworkValues(t *testing.T) {
	t.Setenv("WOL_TUI_REDACT", "1")
	repo, err := store.Open(t.TempDir() + "/wol.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	d, err := repo.CreateDevice(context.Background(), store.Device{Name: "hush-device", MACAddress: "02:00:00:00:00:5d", IPAddress: "198.51.100.2", BroadcastAddress: "198.51.100.255", Port: 9, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	m := NewWakeModel(repo, "0.4.18-dev", "test")
	m.phase, m.loading = phaseReady, false
	m.devices = []store.Device{d}
	var got wol.SendRequest
	m.service = wakeservice.NewService(repo, wakeservice.Hooks{Direct: func(_ context.Context, r wol.SendRequest) (wol.SendResult, error) {
		got = r
		return wol.SendResult{Destination: r.Destination.String(), Packets: r.Repeat}, nil
	}})
	batch := m.beginWake(false)().(tea.BatchMsg)
	msg := batch[0]().(wakeResultMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if got.MAC.String() != "02:00:00:00:00:5d" || got.Destination.String() != "198.51.100.255" || msg.result.Device.IPAddress != "198.51.100.2" {
		t.Fatalf("network values changed: request=%+v device=%+v", got, msg.result.Device)
	}
	if strings.Contains(strings.ToLower(stripANSI(m.View())), "hush-") {
		t.Fatal("action rendered real name")
	}
	if m.actionCancel != nil {
		m.actionCancel()
	}
}
