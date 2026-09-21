package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
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
	model.phase, model.loading = phaseReady, false
	model.checkedAt = time.Date(2026, 8, 24, 5, 27, 4, 0, time.Local)
	model.theme = NewTheme(false, true)
	model.devices = []store.Device{device}
	model.relays, err = repository.ListWakeRelays(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{4, 8, 12, 18, 24, 40, 60, 80, 110, 140} {
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
	if !strings.Contains(view, "wol") {
		t.Fatalf("wake desk header missing:\n%s", view)
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

func TestRemoteResultStaysBoundToOriginalTargetAfterSelectionMoves(t *testing.T) {
	model := &WakeModel{
		width: 120, height: 34, theme: NewTheme(false, true), motion: NewMotion(false), phase: phaseReady,
		devices:  []store.Device{{ID: "windows", Name: "windows", Enabled: true}, {ID: "private2", Name: "private2", Enabled: true}},
		presence: map[string]string{"windows": "offline", "private2": "offline"}, profiles: map[string]store.RemoteProfile{},
		selected: 1, opening: true, actionID: 7, actionTargetID: "windows", actionTarget: "windows", action: "wake-remote",
	}
	model.Update(remoteResultMsg{operationID: 7, targetID: "windows", deviceName: "windows"})
	if model.presence["windows"] != "online" || model.presence["private2"] != "offline" || !strings.Contains(model.status, "windows · remote opened") {
		t.Fatalf("result moved to current selection: presence=%v status=%q", model.presence, model.status)
	}

	before := model.status
	model.Update(remoteResultMsg{operationID: 6, targetID: "private2", deviceName: "private2"})
	if model.status != before || model.presence["private2"] != "offline" {
		t.Fatalf("stale operation changed state: presence=%v status=%q", model.presence, model.status)
	}
}

func TestActiveActionPinsInspectorAndLocksNavigation(t *testing.T) {
	model := &WakeModel{
		width: 120, height: 34, theme: NewTheme(false, true), motion: NewMotion(false), phase: phaseReady,
		devices:  []store.Device{{ID: "windows", Name: "windows", Enabled: true}, {ID: "private2", Name: "private2", Enabled: true}},
		presence: map[string]string{}, profiles: map[string]store.RemoteProfile{}, selected: 1,
		waking: true, actionID: 4, actionTargetID: "windows", actionTarget: "windows", action: "wake-wait",
	}
	view := stripANSI(model.View())
	if !strings.Contains(view, "locked") || !strings.Contains(view, "windows") || strings.Contains(view, "> private2") {
		t.Fatalf("active action was not target-bound after navigation:\n%s", view)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if model.selected != 1 || !strings.Contains(model.status, "selection is locked") {
		t.Fatalf("active action allowed navigation: selected=%d status=%q", model.selected, model.status)
	}
}

func TestWakeAndRemoteUsesFullScreenLoadingUntilBrowserOpens(t *testing.T) {
	for _, size := range []struct{ width, height int }{{18, 20}, {40, 20}, {80, 24}, {120, 34}} {
		model := &WakeModel{
			width: size.width, height: size.height, theme: NewTheme(false, true), motion: NewMotion(false), phase: phaseReady,
			devices: []store.Device{{ID: "windows", Name: "windows", Enabled: true}}, presence: map[string]string{"windows": "offline"},
			profiles: map[string]store.RemoteProfile{}, opening: true, actionTargetID: "windows", actionTarget: "windows", action: "wake-remote",
		}
		view := stripANSI(model.View())
		if !strings.Contains(view, "windows") || !strings.Contains(view, "WAKE") || !strings.Contains(view, "WAIT") || !(strings.Contains(view, "REMOTE") || strings.Contains(view, "STREAM")) {
			t.Fatalf("%dx%d remote loading lost essential state:\n%s", size.width, size.height, view)
		}
		if strings.Contains(view, "machines") && strings.Contains(view, "enter choose") {
			t.Fatalf("%dx%d remote loading exposed interactive dashboard:\n%s", size.width, size.height, view)
		}
		for lineNo, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > size.width {
				t.Fatalf("%dx%d loading line %d overflows at %d: %q", size.width, size.height, lineNo, got, line)
			}
		}
		if lines := len(strings.Split(strings.TrimSuffix(view, "\n"), "\n")); lines > size.height {
			t.Fatalf("%dx%d loading uses %d lines:\n%s", size.width, size.height, lines, view)
		}
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
	if !strings.Contains(view, "wol") || !strings.Contains(view, "windows") || !strings.Contains(view, "online") || !strings.Contains(view, "unreachable") {
		t.Fatalf("compact view lost essential context:\n%s", view)
	}
}

func TestShortWideTerminalUsesCompactLayout(t *testing.T) {
	model := &WakeModel{
		width: 120, height: 24, theme: NewTheme(false, true), motion: NewMotion(false),
		devices: []store.Device{
			{ID: "one", Name: "private", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.5", Enabled: true},
			{ID: "two", Name: "windows", MACAddress: "00:11:22:33:44:66", IPAddress: "192.168.50.200", Enabled: true},
		},
		profiles: map[string]store.RemoteProfile{"one": {DeviceID: "one", Protocol: "ssh", Host: "192.168.50.5", Port: 22, VerifyPort: 22, Mode: "browser-local", Enabled: true}},
		presence: map[string]string{"one": "online", "two": "offline"},
		status:   "Power scan complete.",
	}
	view := model.View()
	if ResolveLayout(model.width, model.height) != LayoutCompact || strings.Contains(view, "ACTION DECK") {
		t.Fatalf("short terminal did not choose compact layout:\n%s", view)
	}
	if lines := len(strings.Split(strings.TrimSuffix(view, "\n"), "\n")); lines > model.height {
		t.Fatalf("short wide view uses %d lines in a %d-line terminal:\n%s", lines, model.height, view)
	}
	if !strings.Contains(strings.ToLower(view), "enter choose") {
		t.Fatalf("compact footer does not keep Enter explicit:\n%s", view)
	}
}

func TestMachineViewportFitsLargeInventories(t *testing.T) {
	devices := make([]store.Device, 20)
	presenceStates := make(map[string]string, len(devices))
	for i := range devices {
		id := fmt.Sprintf("device-%02d", i)
		devices[i] = store.Device{ID: id, Name: id, MACAddress: fmt.Sprintf("00:11:22:33:44:%02x", i), IPAddress: fmt.Sprintf("192.168.50.%d", i+10), Enabled: true}
		presenceStates[id] = "offline"
	}
	for _, size := range []struct{ width, height int }{{40, 20}, {80, 20}, {120, 30}} {
		model := &WakeModel{width: size.width, height: size.height, theme: NewTheme(false, true), motion: NewMotion(false), devices: devices, presence: presenceStates, selected: len(devices) - 1, status: "Power scan complete."}
		view := model.View()
		if lines := len(strings.Split(strings.TrimSuffix(view, "\n"), "\n")); lines > size.height {
			t.Fatalf("%dx%d large inventory uses %d lines:\n%s", size.width, size.height, lines, view)
		}
		for lineNo, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(stripANSI(line)); got > size.width {
				t.Fatalf("%dx%d line %d overflows at %d: %q", size.width, size.height, lineNo, got, line)
			}
		}
		if !strings.Contains(view, "device-19") || !strings.Contains(view, "above") {
			t.Fatalf("%dx%d viewport lost selection/context:\n%s", size.width, size.height, view)
		}
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
		profiles: map[string]store.RemoteProfile{"online": {DeviceID: "online", Protocol: "rdp", Host: "192.168.50.200", Port: 3389, VerifyPort: 3389, Mode: "browser-local", Enabled: true}},
		status:   "ready",
	}
	view := model.View()
	for _, want := range []string{"online", "unknown", "unreachable", "setup", "blocked", "remote", "192.168.50.200"} {
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
	cmd := model.startPresenceScan(devices, 7, loadingRefresh, context.Background())
	if model.presence["one"] != "" {
		t.Fatalf("scan changed the visible snapshot before completion: %#v", model.presence)
	}
	message, ok := cmd().(fleetMsg)
	if !ok || message.id != 7 || message.result.Status != "online" {
		t.Fatalf("scan message = %#v, want one online result", message)
	}
}

func TestEnterOnlyOpensActionPicker(t *testing.T) {
	model := &WakeModel{
		width: 120, height: 32, theme: NewTheme(false, true), motion: NewMotion(false),
		devices:  []store.Device{{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", Enabled: true}},
		presence: map[string]string{"one": "online"},
	}
	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !model.actionPicker || model.opening || model.waking || model.checking {
		t.Fatalf("Enter executed work: picker=%v opening=%v waking=%v checking=%v", model.actionPicker, model.opening, model.waking, model.checking)
	}
	view := model.View()
	for _, want := range []string{"wake", "stream", "check", "power off", "cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker missing %q:\n%s", want, view)
		}
	}
}

func TestActionPickerOptionsAreDeterministic(t *testing.T) {
	profile := store.RemoteProfile{DeviceID: "one", Protocol: "rdp", Host: "192.168.50.200", Port: 3389, VerifyPort: 3389, Mode: "browser-local", Enabled: true}
	remoteCalls := 0
	model := &WakeModel{
		width: 80, height: 32, theme: NewTheme(false, true), motion: NewMotion(false),
		devices:       []store.Device{{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", BroadcastAddress: "192.168.50.255", Port: 9, Enabled: true}},
		profiles:      map[string]store.RemoteProfile{"one": profile},
		presence:      map[string]string{"one": "offline"},
		wakeAndRemote: func(context.Context, store.Device, store.RemoteProfile) error { remoteCalls++; return nil },
	}

	model.openActionPicker()
	if cmd := model.handleActionPicker("c"); cmd == nil || !model.opening || model.waking || model.action != "wake-remote" {
		t.Fatalf("c did not select wake+remote: opening=%v waking=%v action=%q", model.opening, model.waking, model.action)
	} else {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, batched := range batch {
				if batched != nil {
					if result, ok := batched().(remoteResultMsg); ok {
						model.Update(result)
					}
				}
			}
		}
	}
	if remoteCalls != 1 {
		t.Fatalf("wake+remote callback calls = %d, want 1", remoteCalls)
	}

	model.openActionPicker()
	model.pickerSelected = 4
	if cmd := model.handleActionPicker("enter"); cmd != nil || model.actionPicker {
		t.Fatalf("Cancel option started work")
	}

	// Test option 3 opens shutdown form
	model.openActionPicker()
	model.pickerSelected = 3
	_ = model.handleActionPicker("enter")
	if model.form == nil || model.form.kind != powerForm {
		t.Fatalf("Power off option did not open power form: %+v", model.form)
	}
}

func TestFixedShortcutsNeverChangeMeaning(t *testing.T) {
	device := store.Device{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", BroadcastAddress: "192.168.50.255", Port: 9, Enabled: true}
	profile := store.RemoteProfile{DeviceID: "one", Protocol: "rdp", Host: device.IPAddress, Port: 3389, VerifyPort: 3389, Mode: "browser-local", Enabled: true}
	newModel := func() *WakeModel {
		return &WakeModel{devices: []store.Device{device}, profiles: map[string]store.RemoteProfile{"one": profile}, presence: map[string]string{}, motion: NewMotion(false), service: &wakeservice.Service{}, wakeAndRemote: func(context.Context, store.Device, store.RemoteProfile) error { return nil }}
	}
	if model := newModel(); model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}}) == nil || !model.waking || model.opening {
		t.Fatalf("w did not mean wake only")
	}
	if model := newModel(); model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}) == nil || !model.opening || model.waking {
		t.Fatalf("c did not mean wake and remote")
	}
	if model := newModel(); model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}) == nil || !model.checking || model.waking || model.opening {
		t.Fatalf("s did not mean check power")
	}
}

func TestEscapeCancelsWakeAndRemoteContext(t *testing.T) {
	device := store.Device{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", Enabled: true}
	profile := store.RemoteProfile{DeviceID: "one", Protocol: "rdp", Host: device.IPAddress, Port: 3389, VerifyPort: 3389, Mode: "browser-local", Enabled: true}
	started := make(chan struct{})
	model := &WakeModel{devices: []store.Device{device}, profiles: map[string]store.RemoteProfile{"one": profile}, presence: map[string]string{}, motion: NewMotion(false), wakeAndRemote: func(ctx context.Context, _ store.Device, _ store.RemoteProfile) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	cmd := model.beginWakeAndRemote()
	done := make(chan tea.Msg, 1)
	go func() {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			done <- batch[0]()
			return
		}
		done <- msg
	}()
	<-started
	model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	select {
	case msg := <-done:
		model.Update(msg)
	case <-time.After(time.Second):
		t.Fatal("Esc did not cancel callback context")
	}
	if model.opening || !strings.Contains(strings.ToLower(model.status), "cancel") {
		t.Fatalf("cancel state = opening %v status %q", model.opening, model.status)
	}
}

func TestWakeAndRemoteActionsCannotOverlap(t *testing.T) {
	device := store.Device{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", BroadcastAddress: "192.168.50.255", Enabled: true}
	profile := store.RemoteProfile{DeviceID: "one", Protocol: "rdp", Host: "192.168.50.200", Port: 3389, VerifyPort: 3389, Mode: "browser-local", Enabled: true}
	model := &WakeModel{devices: []store.Device{device}, profiles: map[string]store.RemoteProfile{"one": profile}, presence: map[string]string{}, motion: NewMotion(false), waking: true, wakeAndRemote: func(context.Context, store.Device, store.RemoteProfile) error { return nil }}
	if cmd := model.beginWakeAndRemote(); cmd != nil || model.opening {
		t.Fatalf("remote started during wake: cmd=%v opening=%v", cmd != nil, model.opening)
	}
	model.waking, model.opening = false, true
	if cmd := model.beginWake(false); cmd != nil || model.waking {
		t.Fatalf("wake started during remote: cmd=%v waking=%v", cmd != nil, model.waking)
	}
}

func TestColoredWideRowsKeepStatusColumnsAligned(t *testing.T) {
	model := &WakeModel{
		theme: NewTheme(true, false), selected: 0,
		devices: []store.Device{
			{ID: "one", Name: "a", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.5", Enabled: true},
			{ID: "two", Name: "a-much-longer-name", MACAddress: "00:11:22:33:44:66", IPAddress: "192.168.50.6", Enabled: true},
		},
		presence: map[string]string{"one": "online", "two": "offline"},
	}
	view := stripANSI(model.renderMachineList(model.devices, 120))
	columns := make([]int, 0, 2)
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "machines") {
			continue
		}
		index := strings.Index(line, "online")
		if index < 0 {
			index = strings.Index(line, "unreachable")
		}
		if index > 0 {
			columns = append(columns, lipgloss.Width(line[:index]))
		}
	}
	if len(columns) != 2 || columns[0] != columns[1] {
		t.Fatalf("colored rows are not aligned: columns=%v\n%s", columns, view)
	}
}

func TestMachineEditPreservesMetadataWithoutHostedRemoteField(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	device, err := repository.CreateDevice(t.Context(), store.Device{
		Name: "windows", MACAddress: "00:11:22:33:44:55", SiteID: "site-private",
		DeviceType: "desktop", Platform: "windows", Description: "keep me", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	model := NewWakeModel(repository, "test", "test")
	model.devices = []store.Device{device}
	model.beginEdit()
	message, ok := model.saveForm()().(formSavedMsg)
	if !ok || message.keep {
		t.Fatalf("save message = %#v", message)
	}
	updated, err := repository.GetDevice(t.Context(), device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SiteID != device.SiteID || updated.DeviceType != "desktop" || updated.Platform != "windows" || updated.Description != "keep me" || !updated.Enabled {
		t.Fatalf("machine edit lost metadata: %+v", updated)
	}
	for _, label := range model.form.labels {
		if strings.Contains(strings.ToLower(label), "url") {
			t.Fatalf("machine form still exposes remote URL: %v", model.form.labels)
		}
	}
}

func TestMachineFormFitsShortTerminal(t *testing.T) {
	model := &WakeModel{width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(false), presence: map[string]string{}}
	model.beginAdd()
	view := model.View()
	if lines := len(strings.Split(strings.TrimSuffix(view, "\n"), "\n")); lines > model.height {
		t.Fatalf("machine form uses %d lines in a %d-line terminal:\n%s", lines, model.height, view)
	}
	if !strings.Contains(view, "more field") {
		t.Fatalf("short form does not explain hidden fields:\n%s", view)
	}
	model.form.selected = len(model.form.labels) - 1
	view = model.View()
	if !strings.Contains(view, "Site name") || !strings.Contains(view, "earlier field") {
		t.Fatalf("short form did not scroll to selected final field:\n%s", view)
	}
}

func TestWakeAndRemoteRequiresLocalProfileAndRuntime(t *testing.T) {
	device := store.Device{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", Enabled: true}
	model := &WakeModel{devices: []store.Device{device}, profiles: map[string]store.RemoteProfile{}, presence: map[string]string{}, motion: NewMotion(false)}
	if cmd := model.beginWakeAndRemote(); cmd != nil || !strings.Contains(model.status, "Press p") {
		t.Fatalf("missing profile guidance = %q", model.status)
	}
	model.profiles[device.ID] = store.RemoteProfile{DeviceID: device.ID, Protocol: "rdp", Host: device.IPAddress, Port: 3389, VerifyPort: 3389, Mode: "browser-local", Enabled: true}
	if cmd := model.beginWakeAndRemote(); cmd != nil || !strings.Contains(model.status, "remote doctor") {
		t.Fatalf("missing runtime guidance = %q", model.status)
	}
	if strings.Contains(strings.ToLower(model.View()), "http://") || strings.Contains(strings.ToLower(model.View()), "https://") || strings.Contains(strings.ToLower(model.View()), "aklkbqx.com") {
		t.Fatalf("TUI exposed an external URL/domain:\n%s", model.View())
	}
}

func TestRemoteProfileFormSavesProtocolWithoutPassword(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	device, err := repository.CreateDevice(t.Context(), store.Device{Name: "windows", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", Platform: "windows", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	model := NewWakeModel(repository, "test", "test")
	model.devices = []store.Device{device}
	model.beginRemoteProfile()
	if model.form == nil || model.form.kind != remoteProfileForm {
		t.Fatal("p did not open local remote profile form")
	}
	for _, label := range model.form.labels {
		if strings.Contains(strings.ToLower(label), "password") || strings.Contains(strings.ToLower(label), "url") {
			t.Fatalf("unsafe field in profile form: %q", label)
		}
	}
	message, ok := model.saveForm()().(formSavedMsg)
	if !ok || message.keep {
		t.Fatalf("save message = %#v", message)
	}
	profile, err := repository.GetRemoteProfile(t.Context(), device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Protocol != "rdp" || profile.Host != device.IPAddress || profile.Mode != "native" || profile.Port != 3389 {
		t.Fatalf("saved profile = %+v", profile)
	}
}

func TestActionPickerFitsResponsiveViewports(t *testing.T) {
	device := store.Device{ID: "one", Name: "a-very-long-workstation-name", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", Enabled: true}
	for _, size := range []struct{ width, height int }{{18, 20}, {40, 20}, {80, 24}, {120, 30}, {120, 34}} {
		model := &WakeModel{width: size.width, height: size.height, theme: NewTheme(false, true), motion: NewMotion(false), devices: []store.Device{device}, presence: map[string]string{}, actionPicker: true}
		view := model.View()
		if lines := len(strings.Split(strings.TrimSuffix(view, "\n"), "\n")); lines > size.height {
			t.Fatalf("%dx%d picker uses %d lines:\n%s", size.width, size.height, lines, view)
		}
		for lineNo, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(stripANSI(line)); got > size.width {
				t.Fatalf("%dx%d picker line %d overflows at %d: %q", size.width, size.height, lineNo, got, line)
			}
		}
	}
}

func TestStartupShowsInventoryWhileProbesStream(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, err = repository.CreateDevice(t.Context(), store.Device{Name: "windows", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	model := NewWakeModel(repository, "test", "aklkbqx")
	model.theme = NewTheme(false, true)
	model.motion = NewMotion(false)
	model.detector = presence.NewDetector(
		presence.WithTCPPorts(nil),
		presence.WithPing(func(context.Context, string, time.Duration) (time.Duration, error) { return time.Millisecond, nil }),
	)
	data, ok := model.Init()().(wakeDataMsg)
	if !ok {
		t.Fatalf("initial command returned %T", model.Init()())
	}
	_, scan := model.Update(data)
	if model.phase != phaseReady || len(model.devices) != 1 || model.pending != nil {
		t.Fatalf("inventory leaked before verification: phase=%v devices=%d pending=%v", model.phase, len(model.devices), model.pending != nil)
	}
	if view := model.View(); !strings.Contains(view, "windows") || !strings.Contains(view, "Checking") {
		t.Fatalf("startup exposed the dashboard before verification:\n%s", view)
	}
	result, ok := scan().(fleetMsg)
	if !ok {
		t.Fatalf("scan returned %T", scan())
	}
	_, next := model.Update(result)
	if next != nil {
		model.Update(next())
	}
	if model.phase != phaseReady || len(model.devices) != 1 || model.presence[model.devices[0].ID] != "online" || model.checkedAt.IsZero() {
		t.Fatalf("verified snapshot was not committed: phase=%v devices=%d presence=%v checked=%v", model.phase, len(model.devices), model.presence, model.checkedAt)
	}
	if view := model.View(); !strings.Contains(view, "windows") || strings.Contains(view, "reading inventory") {
		t.Fatalf("verified dashboard did not replace loading:\n%s", view)
	}
}

func TestRefreshFailureKeepsLastVerifiedSnapshot(t *testing.T) {
	device := store.Device{ID: "old", Name: "saved", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.5", Enabled: true}
	checked := time.Date(2026, 8, 24, 5, 27, 4, 0, time.Local)
	model := &WakeModel{
		width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(false),
		phase: phaseReady, devices: []store.Device{device}, presence: map[string]string{"old": "online"}, profiles: map[string]store.RemoteProfile{}, checkedAt: checked,
	}
	model.requestID = 10
	model.phase, model.loadingKind, model.loadingStage, model.loading = phaseRefreshing, loadingRefresh, stageInventory, true
	model.Update(wakeDataMsg{requestID: 10, kind: loadingRefresh, err: fmt.Errorf("disk busy")})
	if model.phase != phaseReady || len(model.devices) != 1 || model.devices[0].ID != "old" || model.presence["old"] != "online" || !model.stale || !model.checkedAt.Equal(checked) {
		t.Fatalf("refresh failure damaged snapshot: phase=%v devices=%v presence=%v stale=%v checked=%v", model.phase, model.devices, model.presence, model.stale, model.checkedAt)
	}
	if view := model.View(); !strings.Contains(view, "STALE") || !strings.Contains(view, "saved") {
		t.Fatalf("stale recovery is not visible:\n%s", view)
	}
}

func TestStaleAsyncResponseCannotOverwriteCurrentRequest(t *testing.T) {
	model := &WakeModel{phase: phaseRefreshing, requestID: 12, loading: true, devices: []store.Device{{ID: "current", Name: "current"}}, presence: map[string]string{"current": "online"}}
	model.Update(wakeDataMsg{requestID: 11, kind: loadingRefresh, devices: []store.Device{{ID: "stale", Name: "stale"}}})
	if len(model.devices) != 1 || model.devices[0].ID != "current" || model.pending != nil || model.phase != phaseRefreshing {
		t.Fatalf("stale response changed current state: devices=%v pending=%v phase=%v", model.devices, model.pending, model.phase)
	}
}

func TestSinglePowerCheckKeepsInventoryInteractive(t *testing.T) {
	device := store.Device{ID: "one", Name: "windows", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", Enabled: true}
	model := &WakeModel{
		width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(false), phase: phaseReady,
		devices: []store.Device{device}, presence: map[string]string{"one": "offline"}, profiles: map[string]store.RemoteProfile{},
		detector: presence.NewDetector(
			presence.WithTCPPorts(nil),
			presence.WithPing(func(context.Context, string, time.Duration) (time.Duration, error) { return time.Millisecond, nil }),
		),
	}
	cmd := model.probeSelected()
	if model.phase != phaseReady || model.presence["one"] != "offline" {
		t.Fatalf("focused check changed visible state early: phase=%v presence=%v", model.phase, model.presence)
	}
	if view := model.View(); !strings.Contains(view, "Checking power") || !strings.Contains(view, "windows") || !strings.Contains(view, "enter choose") {
		t.Fatalf("focused check view is unclear:\n%s", view)
	}
	message, ok := cmd().(probeResultMsg)
	if !ok {
		t.Fatalf("check returned %T", cmd())
	}
	model.Update(message)
	if model.phase != phaseReady || model.presence["one"] != "online" || model.checkedDevice["one"].IsZero() || !model.checkedAt.IsZero() {
		t.Fatalf("focused result was not committed independently: phase=%v presence=%v deviceChecked=%v fleetChecked=%v", model.phase, model.presence, model.checkedDevice, model.checkedAt)
	}
}

func TestLoadingAndErrorViewsFitResponsiveTerminals(t *testing.T) {
	for _, size := range []struct{ width, height int }{{18, 20}, {40, 20}, {80, 24}, {120, 30}, {120, 34}} {
		for _, phase := range []viewPhase{phaseBootLoading, phaseRefreshing, phaseCheckingMachine, phaseLoadError} {
			model := &WakeModel{
				width: size.width, height: size.height, theme: NewTheme(false, true), motion: NewMotion(false),
				phase: phase, loading: phase != phaseLoadError, loadingStage: stagePresence, loadingTarget: "a-very-long-workstation-name", loadingError: "Could not read the local inventory.",
				devices: []store.Device{{ID: "one", Name: "saved"}}, pending: &wakeDataMsg{devices: []store.Device{{ID: "pending"}}},
			}
			view := model.View()
			if lines := len(strings.Split(strings.TrimSuffix(view, "\n"), "\n")); lines > size.height {
				t.Fatalf("%dx%d phase %v uses %d lines:\n%s", size.width, size.height, phase, lines, view)
			}
			for lineNo, line := range strings.Split(view, "\n") {
				if got := lipgloss.Width(stripANSI(line)); got > size.width {
					t.Fatalf("%dx%d phase %v line %d overflows at %d: %q", size.width, size.height, phase, lineNo, got, line)
				}
			}
			if strings.Contains(view, "SQLite") || strings.Contains(view, "/Library/") {
				t.Fatalf("loading view exposed storage details:\n%s", view)
			}
		}
	}
}

func TestReducedMotionLoadingSignalIsStable(t *testing.T) {
	model := &WakeModel{width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(false), phase: phaseBootLoading, loadingStage: stagePresence}
	first := model.loadingSignal(60)
	model.frame = 99
	if second := model.loadingSignal(60); first != second {
		t.Fatalf("reduced-motion signal changed: %q != %q", first, second)
	}
}

func TestEscapeCancelsRefreshAndRejectsItsLateResult(t *testing.T) {
	device := store.Device{ID: "verified", Name: "verified", IPAddress: "192.168.50.5", Enabled: true}
	ctx, cancel := context.WithCancel(context.Background())
	model := &WakeModel{
		phase: phaseRefreshing, loading: true, requestID: 7, loadContext: ctx, loadCancel: cancel,
		devices: []store.Device{device}, presence: map[string]string{"verified": "online"},
	}

	model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if model.phase != phaseReady || model.loading || model.requestID != 8 || !strings.Contains(model.status, "cancelled") {
		t.Fatalf("refresh cancel state is wrong: phase=%v loading=%v request=%d status=%q", model.phase, model.loading, model.requestID, model.status)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("refresh context was not cancelled")
	}

	model.Update(probeBatchMsg{requestID: 7, statuses: map[string]string{"late": "offline"}})
	if len(model.devices) != 1 || model.devices[0].ID != "verified" || model.presence["late"] != "" {
		t.Fatalf("late refresh result changed verified state: devices=%v presence=%v", model.devices, model.presence)
	}
}

func TestBootFailureOffersRetryWithoutExposingDashboard(t *testing.T) {
	model := &WakeModel{
		width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(false),
		phase: phaseBootLoading, loading: true, loadingKind: loadingBoot, requestID: 3,
	}
	model.Update(wakeDataMsg{requestID: 3, kind: loadingBoot, err: fmt.Errorf("database unavailable")})
	view := model.View()
	if model.phase != phaseLoadError || !strings.Contains(view, "r retry") || strings.Contains(view, "windows") {
		t.Fatalf("boot recovery view is wrong:\n%s", view)
	}
}

func TestFleetAfterimageRespectsMotion(t *testing.T) {
	devices := []store.Device{
		{ID: "one", Name: "windows", Enabled: true},
		{ID: "two", Name: "private", Enabled: true},
	}
	offASCII := &WakeModel{width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(false), devices: devices, selected: 1}
	ascii := stripANSI(offASCII.renderMachineList(devices, 80))
	if strings.Contains(ascii, "·") || strings.Count(ascii, ">") != 1 {
		t.Fatalf("reduced-motion ASCII caret/afterimage wrong:\n%s", ascii)
	}
	offUnicode := &WakeModel{width: 80, height: 24, theme: NewTheme(true, false), motion: NewMotion(false), devices: devices, selected: 1}
	unicode := stripANSI(offUnicode.renderMachineList(devices, 80))
	if strings.Contains(unicode, "·") || strings.Count(unicode, "›") != 1 {
		t.Fatalf("reduced-motion unicode caret/afterimage wrong:\n%s", unicode)
	}

	on := &WakeModel{width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(true), devices: devices, selected: 1}
	on.motion.TriggerStage(time.Now(), StageSelect, 180*time.Millisecond, 0, 1)
	live := stripANSI(on.renderMachineList(devices, 80))
	if !strings.Contains(live, ". windows") || !strings.Contains(live, "> private") {
		t.Fatalf("select afterimage missing:\n%s", live)
	}
}

func TestSignalPathFollowsMotionTime(t *testing.T) {
	started := time.Unix(1_700_000_000, 0)
	model := &WakeModel{
		theme: NewTheme(false, true),
		motion: Motion{
			Enabled: true, Stage: StageSignal,
			Started: started, Duration: 800 * time.Millisecond,
			Until: started.Add(800 * time.Millisecond),
		},
		waking: true,
	}
	if got := model.pathPositionAt(started); got != 0 {
		t.Fatalf("T=0 position = %d", got)
	}
	if got := signalPath([]string{"DESK", "LAN", "WINDOWS"}, 0, true); got != "DESK *--> LAN --> WINDOWS" {
		t.Fatalf("T=0 path = %q", got)
	}
	mid := started.Add(400 * time.Millisecond)
	if got := model.pathPositionAt(mid); got != 1 {
		t.Fatalf("mid position = %d", got)
	}
	if got := signalPath([]string{"DESK", "LAN", "WINDOWS"}, 1, true); got != "DESK --> LAN *--> WINDOWS" {
		t.Fatalf("mid path = %q", got)
	}
	if got := signalPath([]string{"DESK", "RELAY", "WINDOWS"}, 1, true); got != "DESK --> RELAY *--> WINDOWS" {
		t.Fatalf("relay mid path = %q", got)
	}
	if got := model.pathPositionAt(started.Add(800 * time.Millisecond)); got != 2 {
		t.Fatalf("dwell at Until = %d", got)
	}
	if got := model.pathPositionAt(started.Add(900 * time.Millisecond)); got != 2 {
		t.Fatalf("dwell after Until = %d", got)
	}
	if got := signalPath([]string{"DESK", "LAN", "WINDOWS"}, 2, true); got != "DESK --> LAN --> *WINDOWS" {
		t.Fatalf("dwell path = %q", got)
	}

	model.motion.Frame = 0
	a := model.pathPositionAt(mid)
	model.motion.Frame = 12
	b := model.pathPositionAt(mid)
	if a != b || a != 1 {
		t.Fatalf("path depended on Frame: %d vs %d", a, b)
	}
	if tNow := model.motion.T(mid); tNow != 0.5 {
		t.Fatalf("T(400ms of 800ms) = %v, want 0.5", tNow)
	}
	posA := railPosition(mid, model.motion, 21)
	model.motion.Frame = 0
	posB := railPosition(mid, model.motion, 21)
	if posA != posB {
		t.Fatalf("rail depended on Frame: %d vs %d", posA, posB)
	}

	still := NewMotion(false)
	if railPosition(mid, still, 21) != 10 {
		t.Fatalf("reduced-motion rail is not centered: %d", railPosition(mid, still, 21))
	}
}

func TestHandleKeyJStartsSelectMotionTick(t *testing.T) {
	model := &WakeModel{
		width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(true), phase: phaseReady,
		devices: []store.Device{{ID: "one", Name: "windows", Enabled: true}, {ID: "two", Name: "private", Enabled: true}},
	}
	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if cmd == nil || model.selected != 1 || model.motion.Stage != StageSelect || !model.motion.Active(time.Now()) {
		t.Fatalf("j did not start select motion: selected=%d stage=%v cmd=%v", model.selected, model.motion.Stage, cmd)
	}
	model.motion = NewMotion(false)
	model.selected = 0
	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}); cmd != nil {
		t.Fatalf("reduced-motion j returned a tick")
	}
}

func TestNightDeskFleetFitsOverflowMatrix(t *testing.T) {
	devices := []store.Device{
		{ID: "one", Name: "windows-workstation-with-a-long-name", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", BroadcastAddress: "192.168.50.255", Enabled: true},
		{ID: "two", Name: "private", MACAddress: "00:11:22:33:44:66", IPAddress: "192.168.50.5", Enabled: true},
	}
	presenceStates := map[string]string{"one": "online", "two": "offline"}
	for _, size := range []struct{ width, height int }{{18, 20}, {40, 20}, {80, 24}, {120, 30}, {120, 34}} {
		model := &WakeModel{
			width: size.width, height: size.height, theme: NewTheme(true, false), motion: NewMotion(true),
			phase: phaseReady, devices: devices, selected: 1, presence: presenceStates,
		}
		model.motion.TriggerStage(time.Now(), StageSelect, 180*time.Millisecond, 0, 1)
		assertViewFits(t, model, size.width, size.height)
		model.waking = true
		model.actionTargetID = "one"
		model.actionTarget = devices[0].Name
		model.action = "wake"
		model.motion.TriggerStage(time.Now(), StageSignal, 800*time.Millisecond, 0, 0)
		assertViewFits(t, model, size.width, size.height)
	}
}

func TestMachineFormInteractiveTextInput(t *testing.T) {
	model := &WakeModel{width: 80, height: 24, theme: NewTheme(false, true), motion: NewMotion(false), presence: map[string]string{}}
	model.beginAdd()
	if model.form == nil {
		t.Fatal("form is nil after beginAdd")
	}
	for _, ch := range "node-1" {
		model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if got := model.form.values[0]; got != "node-1" {
		t.Fatalf("expected form.values[0] = 'node-1', got %q", got)
	}
	model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if model.form.selected != 1 {
		t.Fatalf("expected form.selected = 1, got %d", model.form.selected)
	}
	model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if model.form.selected != 0 {
		t.Fatalf("expected form.selected = 0, got %d", model.form.selected)
	}
	model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.form != nil {
		t.Fatal("expected form to be closed after Esc")
	}
}

func TestWakeWaitSignalPathAnimatesContinuously(t *testing.T) {
	started := time.Unix(1_700_000_000, 0)
	model := &WakeModel{
		theme:  NewTheme(false, true),
		action: "wake-wait",
		waking: true,
		motion: Motion{
			Enabled: true, Stage: StageSignal,
			Started: started, Duration: 95 * time.Second,
			Until: started.Add(95 * time.Second),
		},
	}
	// Cycle duration is 900ms (300ms per step: 0 -> 1 -> 2 -> 0 -> 1 -> 2...)
	if got := model.pathPositionAt(started); got != 0 {
		t.Fatalf("at 0ms, want position 0, got %d", got)
	}
	if got := model.pathPositionAt(started.Add(350 * time.Millisecond)); got != 1 {
		t.Fatalf("at 350ms, want position 1, got %d", got)
	}
	if got := model.pathPositionAt(started.Add(650 * time.Millisecond)); got != 2 {
		t.Fatalf("at 650ms, want position 2, got %d", got)
	}
	// After 900ms, it should cycle back to 0!
	if got := model.pathPositionAt(started.Add(950 * time.Millisecond)); got != 0 {
		t.Fatalf("at 950ms, want position 0 (cycled), got %d", got)
	}
	if got := model.pathPositionAt(started.Add(1250 * time.Millisecond)); got != 1 {
		t.Fatalf("at 1250ms, want position 1 (cycled), got %d", got)
	}

	// When motion is disabled, returns position 0
	still := model
	still.motion.Enabled = false
	if got := still.pathPositionAt(started.Add(950 * time.Millisecond)); got != 0 {
		t.Fatalf("with motion disabled, want position 0, got %d", got)
	}
}

func TestWakeWaitFleetRowShowsAnimatedSpinner(t *testing.T) {
	started := time.Unix(1_700_000_000, 0)
	model := &WakeModel{
		width: 100, height: 30,
		theme:          NewTheme(true, false),
		action:         "wake-wait",
		actionTargetID: "win-1",
		actionTarget:   "windows",
		waking:         true,
		devices: []store.Device{
			{ID: "win-1", Name: "windows", Enabled: true, IPAddress: "192.168.1.100", MACAddress: "00:11:22:33:44:55"},
		},
		presence: map[string]string{"win-1": "offline"},
		motion: Motion{
			Enabled: true, Stage: StageSignal,
			Started: started, Duration: 95 * time.Second,
			Until: started.Add(95 * time.Second),
		},
	}
	view0 := model.View()
	if !strings.Contains(view0, "waking") {
		t.Fatalf("fleet view expected to contain 'waking' during wake-wait:\n%s", view0)
	}
	if !strings.Contains(view0, "⠋ waking") {
		t.Fatalf("fleet view expected to contain initial spinner frame '⠋ waking':\n%s", view0)
	}

	// Advance frame
	model.frame = 4
	view4 := model.View()
	if !strings.Contains(view4, "⠹ waking") {
		t.Fatalf("fleet view expected to contain rotated spinner frame '⠹ waking':\n%s", view4)
	}

	// Check reduced motion (ASCII)
	asciiModel := model
	asciiModel.theme = NewTheme(false, true)
	asciiModel.frame = 0
	asciiView := asciiModel.View()
	if !strings.Contains(asciiView, "waking") {
		t.Fatalf("ASCII fleet view expected to contain 'waking':\n%s", asciiView)
	}
}

func assertViewFits(t *testing.T, model *WakeModel, width, height int) {
	t.Helper()
	view := model.View()
	if lines := len(strings.Split(strings.TrimSuffix(view, "\n"), "\n")); lines > height {
		t.Fatalf("%dx%d uses %d lines:\n%s", width, height, lines, view)
	}
	for lineNo, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(stripANSI(line)); got > width {
			t.Fatalf("%dx%d line %d overflows at %d: %q", width, height, lineNo, got, line)
		}
	}
}

func TestShutdownWaitSignalPathAndAutoOffline(t *testing.T) {
	device := store.Device{
		ID:         "win-dev-1",
		Name:       "windows-box",
		MACAddress: "00:11:22:33:44:55",
		IPAddress:  "192.168.1.100",
		Enabled:    true,
	}
	started := time.Now()
	model := &WakeModel{
		width:   80,
		height:  24,
		theme:   NewTheme(true, false),
		devices: []store.Device{device},
		presence: map[string]string{
			"win-dev-1": "online",
		},
		motion: Motion{
			Enabled: true,
			Stage:   StageSignal,
			Started: started,
			Until:   started.Add(90 * time.Second),
		},
	}

	// 1. Initiate shutdown
	model.Update(shutdownInitiatedMsg{
		deviceID:   "win-dev-1",
		deviceName: "windows-box",
	})

	if !model.shuttingDown {
		t.Fatalf("expected shuttingDown to be true")
	}
	if model.action != "shutdown-wait" {
		t.Fatalf("expected action to be shutdown-wait, got %q", model.action)
	}

	// Check view during shutdown
	view := model.View()
	if !strings.Contains(view, "stop") && !strings.Contains(view, "stopping") {
		t.Fatalf("expected view to contain stop/stopping animation, got:\n%s", view)
	}
	if !strings.Contains(view, "SSH") || !strings.Contains(view, "WINDOWS-BOX") {
		t.Fatalf("expected signal path to show SSH and destination, got:\n%s", view)
	}

	// 2. Key lockout during shuttingDown
	for _, key := range []string{"p", "P", "a", "e", "d", "r", "s", "enter", "?"} {
		cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd != nil {
			t.Fatalf("expected key %q to be locked out during shutdown, but got cmd", key)
		}
		if !model.shuttingDown {
			t.Fatalf("key %q erroneously broke out of shuttingDown state", key)
		}
	}

	// 3. Auto offline without reload
	model.Update(shutdownVerifyMsg{
		operationID: model.actionID,
		targetID:    "win-dev-1",
		targetName:  "windows-box",
		status:      "offline",
	})

	if model.shuttingDown {
		t.Fatalf("expected shuttingDown to be false after offline verification")
	}
	if model.presence["win-dev-1"] != "offline" {
		t.Fatalf("expected presence to be offline, got %q", model.presence["win-dev-1"])
	}
	if !strings.Contains(model.status, "unreachable after shutdown") {
		t.Fatalf("expected status to mention unreachable after shutdown, got %q", model.status)
	}

	offlineView := model.View()
	if !strings.Contains(offlineView, "unreachable") {
		t.Fatalf("expected offline view to display 'unreachable', got:\n%s", offlineView)
	}
	if !strings.Contains(offlineView, "1 unreachable") {
		t.Fatalf("expected fleet summary to count 1 unreachable, got:\n%s", offlineView)
	}
}
