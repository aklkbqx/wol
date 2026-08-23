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

func TestRemoteResultStaysBoundToOriginalTargetAfterSelectionMoves(t *testing.T) {
	model := &WakeModel{
		width: 120, height: 34, theme: NewTheme(false, true), motion: NewMotion(false), phase: phaseReady,
		devices:  []store.Device{{ID: "windows", Name: "windows", Enabled: true}, {ID: "private2", Name: "private2", Enabled: true}},
		presence: map[string]string{"windows": "offline", "private2": "offline"}, profiles: map[string]store.RemoteProfile{},
		selected: 1, opening: true, actionID: 7, actionTargetID: "windows", actionTarget: "windows", action: "wake-remote",
	}
	model.Update(remoteResultMsg{operationID: 7, targetID: "windows", deviceName: "windows"})
	if model.presence["windows"] != "online" || model.presence["private2"] != "offline" || !strings.Contains(model.status, "windows · local sign-in opened") {
		t.Fatalf("result moved to current selection: presence=%v status=%q", model.presence, model.status)
	}

	before := model.status
	model.Update(remoteResultMsg{operationID: 6, targetID: "private2", deviceName: "private2"})
	if model.status != before || model.presence["private2"] != "offline" {
		t.Fatalf("stale operation changed state: presence=%v status=%q", model.presence, model.status)
	}
}

func TestActiveActionPanelNamesOriginalTarget(t *testing.T) {
	model := &WakeModel{
		width: 120, height: 34, theme: NewTheme(false, true), motion: NewMotion(false), phase: phaseReady,
		devices:  []store.Device{{ID: "windows", Name: "windows", Enabled: true}, {ID: "private2", Name: "private2", Enabled: true}},
		presence: map[string]string{}, profiles: map[string]store.RemoteProfile{}, selected: 1,
		waking: true, actionID: 4, actionTargetID: "windows", actionTarget: "windows", action: "wake-wait",
	}
	view := stripANSI(model.View())
	if !strings.Contains(view, "ACTIVE FOR windows") || !strings.Contains(view, "selection changed") {
		t.Fatalf("active action was not target-bound after navigation:\n%s", view)
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
	if !strings.Contains(view, "WOL WAKE DESK") || !strings.Contains(view, "1 Machines") || !strings.Contains(view, "POWER") || !strings.Contains(view, "WAKE") || !strings.Contains(view, "REMOTE") {
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
	if !strings.Contains(view, "[Enter] choose") {
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
		if !strings.Contains(view, "device-19") || !strings.Contains(view, "machine(s) above") {
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
	for _, want := range []string{"POWER", "WAKE", "REMOTE", "SETUP", "ONLINE", "UNKNOWN", "OFFLINE", "READY", "BLOCKED", "192.168.50.200"} {
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
	message, ok := cmd().(probeBatchMsg)
	if !ok || message.requestID != 7 || message.kind != loadingRefresh || message.summary.Online != 1 || message.statuses["one"] != "online" {
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
	for _, want := range []string{"CHOOSE ACTION", "Wake only", "Wake & Remote", "Check power", "Cancel", "Nothing runs until you confirm"} {
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
	model.pickerSelected = 3
	if cmd := model.handleActionPicker("enter"); cmd != nil || model.actionPicker {
		t.Fatalf("Cancel option started work")
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
		if strings.Contains(line, "192.168.50.") {
			index := strings.Index(line, "POWER")
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
	if !strings.Contains(view, "Relay ID") || !strings.Contains(view, "earlier field") {
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
	if profile.Protocol != "rdp" || profile.Host != device.IPAddress || profile.Mode != "browser-local" || profile.Port != 3389 {
		t.Fatalf("saved profile = %+v", profile)
	}
}

func TestActionPickerFitsResponsiveViewports(t *testing.T) {
	device := store.Device{ID: "one", Name: "a-very-long-workstation-name", MACAddress: "00:11:22:33:44:55", IPAddress: "192.168.50.200", Enabled: true}
	for _, size := range []struct{ width, height int }{{18, 20}, {40, 20}, {80, 24}, {120, 30}} {
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

func TestStartupHidesFleetUntilAtomicSnapshotIsVerified(t *testing.T) {
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
	if model.phase != phaseBootLoading || len(model.devices) != 0 || model.pending == nil {
		t.Fatalf("inventory leaked before verification: phase=%v devices=%d pending=%v", model.phase, len(model.devices), model.pending != nil)
	}
	if view := model.View(); strings.Contains(view, "FLEET  select a machine") || strings.Contains(view, "MACHINES  ") || !strings.Contains(view, "CHECKING LATEST STATE") {
		t.Fatalf("startup exposed the dashboard before verification:\n%s", view)
	}
	result, ok := scan().(probeBatchMsg)
	if !ok {
		t.Fatalf("scan returned %T", scan())
	}
	model.Update(result)
	if model.phase != phaseReady || len(model.devices) != 1 || model.presence[model.devices[0].ID] != "online" || model.checkedAt.IsZero() {
		t.Fatalf("verified snapshot was not committed: phase=%v devices=%d presence=%v checked=%v", model.phase, len(model.devices), model.presence, model.checkedAt)
	}
	if view := model.View(); !strings.Contains(view, "FLEET") || strings.Contains(view, "CHECKING LATEST STATE") {
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

func TestSinglePowerCheckUsesFocusedLoadingWithoutMutatingVisibleStatus(t *testing.T) {
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
	if model.phase != phaseCheckingMachine || model.presence["one"] != "offline" {
		t.Fatalf("focused check changed visible state early: phase=%v presence=%v", model.phase, model.presence)
	}
	if view := model.View(); !strings.Contains(view, "CHECKING POWER") || !strings.Contains(view, "windows") || strings.Contains(view, "FLEET") {
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
	for _, size := range []struct{ width, height int }{{18, 20}, {40, 20}, {80, 24}, {120, 30}} {
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
	if model.phase != phaseLoadError || !strings.Contains(view, "[r] Retry") || strings.Contains(view, "FLEET  select a machine") {
		t.Fatalf("boot recovery view is wrong:\n%s", view)
	}
}
