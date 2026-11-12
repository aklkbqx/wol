package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/aklkbqx/wol/internal/store"
)

func spotlightModel() *WakeModel {
	m := NewWakeModel(nil, "0.4.20-dev", "test")
	m.phase, m.loading = phaseReady, false
	m.motion = NewMotion(false)
	m.status = ""
	m.width, m.height = 100, 28
	m.theme = NewTheme(true, false)
	m.devices = []store.Device{
		{ID: "a", Name: "studio-ws", IPAddress: "198.51.100.10", MACAddress: "02:00:00:00:01:01", BroadcastAddress: "198.51.100.255", Port: 9, Enabled: true},
		{ID: "b", Name: "nas-vault", IPAddress: "198.51.100.20", MACAddress: "02:00:00:00:01:02", BroadcastAddress: "198.51.100.255", Port: 9, VerifyPort: 445, Enabled: true},
		{ID: "c", Name: "render-node", IPAddress: "198.51.100.30", MACAddress: "02:00:00:00:01:03", BroadcastAddress: "198.51.100.255", Port: 9, Enabled: true},
		{ID: "d", Name: "lab-mini", IPAddress: "198.51.100.40", MACAddress: "02:00:00:00:01:04", BroadcastAddress: "198.51.100.255", Port: 9, Enabled: true},
		{ID: "e", Name: "print-srv", IPAddress: "198.51.100.50", MACAddress: "02:00:00:00:01:05", BroadcastAddress: "198.51.100.255", Port: 9, Enabled: true},
	}
	m.presence = map[string]string{"a": "online", "b": "offline", "c": "offline", "d": "online"}
	m.marked = map[string]bool{"c": true}
	m.selected = 1
	return m
}

func assertExactFrame(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Fatalf("frame has %d lines, want %d", len(lines), height)
	}
	for i, line := range lines {
		if got := lipgloss.Width(stripANSI(line)); got != width {
			t.Fatalf("line %d is %d cells, want %d: %q", i, got, width, stripANSI(line))
		}
	}
}

func TestSpotlightReadyStage(t *testing.T) {
	m := spotlightModel()
	view := m.render()
	assertExactFrame(t, view, 100, 28)
	plain := stripANSI(view)
	for _, want := range []string{
		"wol", "all sites", "MACHINES", "studio-ws", "print-srv", "✓",
		"Machines", "Routes", "Activity", "Sites",
		"asleep", "address", "198.51.100.20", "hardware", "02:00:00:00:01:02",
		"wakes by", "broadcast 198.51.100.255:9", "checks", "tcp:445",
		"Wake nas-vault", "wake and connect", "check",
		"2 awake", "2 asleep", "1 marked", "move", "mark", "filter", "all keys", "quit",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("spotlight missing %q:\n%s", want, plain)
		}
	}
	// The focused name is drawn in block letters.
	if !strings.Contains(plain, "▄████▄") || !strings.Contains(plain, "█") {
		t.Fatalf("spotlight lamp or block name missing:\n%s", plain)
	}
}

func TestSpotlightOnlineOffersConnectOrSetup(t *testing.T) {
	m := spotlightModel()
	m.selected = 0
	plain := stripANSI(m.render())
	for _, want := range []string{"awake", "Check studio-ws", "set up remote", "shut down"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("online stage missing %q:\n%s", want, plain)
		}
	}
	m.profiles = map[string]store.RemoteProfile{"a": {DeviceID: "a", Enabled: true}}
	if plain := stripANSI(m.render()); !strings.Contains(plain, "Connect studio-ws") {
		t.Fatalf("online stage with a remote profile should offer connect:\n%s", plain)
	}
}

func TestSpotlightBlockedWakeExplainsWhy(t *testing.T) {
	m := spotlightModel()
	m.devices[1].MACAddress = "not-a-mac"
	plain := stripANSI(m.render())
	if !strings.Contains(plain, "can't wake") || !strings.Contains(plain, "invalid MAC address") || strings.Contains(plain, "Wake nas-vault") {
		t.Fatalf("blocked stage should explain the reason instead of offering wake:\n%s", plain)
	}
}

func TestSpotlightWakingCountdown(t *testing.T) {
	m := spotlightModel()
	m.waking, m.action = true, "wake-wait"
	m.actionTargetID, m.actionTarget = "b", "nas-vault"
	m.wakeSentAt = time.Now().Add(-7 * time.Second)
	view := m.render()
	assertExactFrame(t, view, 100, 28)
	plain := stripANSI(view)
	for _, want := range []string{"◐", "waking", "packet sent 7 s ago", "waiting for tcp:445 to answer", "seconds left of 90", "sent by broadcast", "waiting for tcp:445 to answer", "cancel"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("waking stage missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Wake nas-vault") {
		t.Fatalf("waking stage must not offer another wake:\n%s", plain)
	}
}

func TestSpotlightFallsBackToClassicLayout(t *testing.T) {
	cases := map[string]func(*WakeModel){
		"narrow": func(m *WakeModel) { m.width = 89 },
		"short":  func(m *WakeModel) { m.height = 23 },
		"ascii":  func(m *WakeModel) { m.theme = NewTheme(false, true) },
		"routes": func(m *WakeModel) { m.tab = 1 },
		"help":   func(m *WakeModel) { m.showHelp = true },
		"form": func(m *WakeModel) {
			m.form = newWakeForm(deviceForm, "b", []string{"Name"}, []string{"nas-vault"}, m.theme)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			m := spotlightModel()
			setup(m)
			if m.spotlightEligible(m.width, m.height) {
				t.Fatalf("%s should use the classic layout", name)
			}
		})
	}
}

func TestSpotlightWithoutColorUsesNoColorCodes(t *testing.T) {
	m := spotlightModel()
	m.theme = NewTheme(false, false)
	view := m.render()
	assertExactFrame(t, view, 100, 28)
	if strings.Contains(view, "38;2;") || strings.Contains(view, "48;2;") {
		t.Fatalf("no-color spotlight emitted color codes")
	}
}

func TestSpotlightScrollsToSelection(t *testing.T) {
	m := spotlightModel()
	for i := 0; i < 20; i++ {
		m.devices = append(m.devices, store.Device{ID: string(rune('f' + i)), Name: "host-" + string(rune('a'+i)), MACAddress: "02:00:00:00:01:09", Enabled: true})
	}
	m.selected = len(m.devices) - 1
	view := m.render()
	assertExactFrame(t, view, 100, 28)
	if plain := stripANSI(view); !strings.Contains(plain, "host-t") {
		t.Fatalf("selected machine scrolled out of view:\n%s", plain)
	}
}

func TestBlockText(t *testing.T) {
	rows, ok := blockText("a1", 4)
	if !ok {
		t.Fatal("blockText(a1) not rendered")
	}
	want := [3]string{"▄▀▄ ▄█ ", "█▀█  █ ", "▀ ▀ ▀▀▀"}
	if rows != want {
		t.Fatalf("blockText(a1) = %q, want %q", rows, want)
	}
	for _, tc := range []struct {
		text string
		max  int
	}{{"jet", 10}, {"too-long", 3}, {"", 3}, {"a.b", 5}} {
		if _, ok := blockText(tc.text, tc.max); ok {
			t.Fatalf("blockText(%q, %d) should fall back", tc.text, tc.max)
		}
	}
}

func BenchmarkRenderSpotlight(b *testing.B) {
	m := spotlightModel()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.render()
	}
}

func BenchmarkRenderClassic100x28(b *testing.B) {
	m := spotlightModel()
	m.width = 89
	b.ReportAllocs()
	for b.Loop() {
		_ = m.render()
	}
}

// Block letters cannot be caught by a plain substring search, so check the
// redacted frame for the block rendering of the real name too.
func TestSpotlightRedactsBlockName(t *testing.T) {
	m := spotlightModel()
	m.devices[1].Name = "vault"
	m.redactIdentifiers = true
	plain := stripANSI(m.render())
	real, ok := blockText("vault", 20)
	if !ok {
		t.Fatal("test name should render in block letters")
	}
	for _, row := range real {
		if strings.Contains(plain, row) {
			t.Fatalf("redacted spotlight drew the real name in block letters:\n%s", plain)
		}
	}
	for _, secret := range []string{"vault", "198.51.100", "02:00:00"} {
		if strings.Contains(plain, secret) {
			t.Fatalf("redacted spotlight leaked %q:\n%s", secret, plain)
		}
	}
}
