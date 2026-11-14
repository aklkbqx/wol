package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aklkbqx/wol/internal/store"
)

func TestPatchbaySplitLayoutRendersShearedFramesAndBatchMarks(t *testing.T) {
	model := &WakeModel{
		width:  88, // classic split; Spotlight starts at 90 columns
		height: 32,
		phase:  phaseReady,
		theme:  NewTheme(true, false),
		motion: NewMotion(false),
		devices: []store.Device{
			{ID: "one", Name: "studio-ws", MACAddress: "02:00:00:00:01:01", IPAddress: "198.51.100.10", BroadcastAddress: "198.51.100.255", Port: 9, Interface: "en0", SiteID: "site-1", Enabled: true},
			{ID: "two", Name: "nas-vault", MACAddress: "02:00:00:00:01:02", IPAddress: "198.51.100.20", BroadcastAddress: "198.51.100.255", Port: 9, SiteID: "site-1", Enabled: true},
		},
		sites:    []store.Site{{ID: "site-1", Name: "homelab", BroadcastAddress: "198.51.100.255", DefaultPort: 9}},
		presence: map[string]string{"one": "online", "two": "offline"},
		marked:   map[string]bool{"two": true},
	}

	assertViewFits(t, model, 88, 32)
	plain := stripANSI(model.renderClassic(model.width))
	for _, want := range []string{"FLEET", "PATCHBAY", "[✓]", "[ ]", "1 marked", "W", "S"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("patchbay split view missing %q:\n%s", want, plain)
		}
	}

	model.theme = NewTheme(false, true)
	assertViewFits(t, model, 88, 32)
	asciiPlain := stripANSI(model.renderClassic(model.width))
	for _, want := range []string{"[x]", "[ ]", "1 marked"} {
		if !strings.Contains(asciiPlain, want) {
			t.Fatalf("ASCII patchbay split view missing %q:\n%s", want, asciiPlain)
		}
	}
}

func TestPatchbayInspectorShowsTopologySparklineAndMachineHistory(t *testing.T) {
	model := &WakeModel{
		width:  88, // classic inspector; Spotlight starts at 90 columns
		height: 34,
		phase:  phaseReady,
		theme:  NewTheme(true, false),
		motion: NewMotion(false),
		devices: []store.Device{
			{ID: "one", Name: "render-node", MACAddress: "02:00:00:00:02:10", IPAddress: "198.51.100.42", BroadcastAddress: "198.51.100.255", Port: 9, Interface: "en0", Enabled: true},
		},
		presence:       map[string]string{"one": "online"},
		presenceMethod: map[string]string{"one": "tcp:22"},
		history: []store.WakeAttempt{
			{TargetID: "one", TargetName: "render-node", PacketStatus: "sent", CreatedAt: "2026-11-11T10:15:00Z"},
			{TargetID: "one", TargetName: "render-node", PacketStatus: "failed", CreatedAt: "2026-11-11T09:00:00Z"},
		},
	}

	assertViewFits(t, model, 88, 34)
	plain := stripANSI(model.renderClassic(model.width))
	for _, want := range []string{"en0", "198.51.100.255:9", "02:00:00:00:02:10", "PULSE", "sent"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("inspector missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "*-->") || strings.Contains(plain, "●──>") {
		t.Fatalf("idle inspector rendered active motion signal path:\n%s", plain)
	}

	// Verify sparkline helper directly in Unicode and ASCII modes.
	uniSpark := renderSparkline([]int{1, 3, 5, 8}, 4, false)
	if uniSpark == "" || len([]rune(uniSpark)) != 4 {
		t.Fatalf("unexpected unicode sparkline %q", uniSpark)
	}
	asciiSpark := renderSparkline([]int{1, 3, 5, 8}, 4, true)
	if !isPlainASCII(asciiSpark) || len(asciiSpark) != 4 {
		t.Fatalf("unexpected ASCII sparkline %q", asciiSpark)
	}

	// Privacy redaction must hide NIC, broadcast, IP, MAC, and target name in new inspector fields.
	model.redactIdentifiers = true
	redacted := strings.ToLower(stripANSI(model.render()))
	for _, secret := range []string{"render-node", "198.51.100", "02:00:00", "en0"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted inspector leaked %q:\n%s", secret, redacted)
		}
	}
}

func TestRuntimeThemeCycleShortcutPreservesState(t *testing.T) {
	model := &WakeModel{
		width:    120,
		height:   32,
		phase:    phaseReady,
		theme:    NewTheme(true, false),
		motion:   NewMotion(false),
		devices:  []store.Device{{ID: "one", Name: "workstation", MACAddress: "02:00:00:00:03:01", IPAddress: "198.51.100.55", Enabled: true}},
		presence: map[string]string{"one": "online"},
	}

	initialID := model.theme.PresetID()
	presets := PalettePresets()
	for i := 0; i < len(presets); i++ {
		cmd := model.handleKey(tea.KeyPressMsg{Code: 't', Text: "t"})
		if cmd != nil {
			t.Fatalf("theme cycle triggered unexpected async command on step %d", i)
		}
		assertViewFits(t, model, 120, 32)
		if !strings.Contains(model.status, model.theme.PresetName()) {
			t.Fatalf("status %q did not announce preset %q", model.status, model.theme.PresetName())
		}
	}
	if model.theme.PresetID() != initialID {
		t.Fatalf("theme cycle did not wrap around to %q, got %q", initialID, model.theme.PresetID())
	}
	if model.devices[0].Name != "workstation" || model.waking || model.opening || model.checking {
		t.Fatalf("theme cycle mutated machine or action state")
	}
}
