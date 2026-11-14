package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aklkbqx/wol/internal/fleet"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/termimg"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
)

// Demo is a recorded walk through the Wake Desk for the website: every
// screen the demo keys reach from a sample inventory, drawn by the real
// model and renderer. Commands the model returns are never run, so nothing
// is sent on the network; wake, check and connect answers are played back
// instead.
type Demo struct {
	Width  int         `json:"width"`
	Height int         `json:"height"`
	Rows   []string    `json:"rows"`   // distinct screen rows with ANSI colour
	States []DemoState `json:"states"` // state 0 is the first screen
}

// DemoState is one screen: its rows by index into Demo.Rows, the state
// each key leads to, and an optional state that follows on its own.
type DemoState struct {
	Rows []int          `json:"rows"`
	Keys map[string]int `json:"keys,omitempty"`
	Next *DemoNext      `json:"next,omitempty"`
}

// DemoNext moves to State after After milliseconds unless a key comes first.
type DemoNext struct {
	After int `json:"after"`
	State int `json:"state"`
}

// DemoOptions sizes the demo. MaxStates bounds the walk; keys that would
// lead past it do nothing.
type DemoOptions struct {
	Width, Height int
	MaxStates     int
	Version       string
}

// DemoKeys are the keys the website sends. "char" is any printable key
// while the filter is open; it types the next letter of demoFilterWord.
var DemoKeys = []string{
	"up", "down", "enter", "esc", "tab", "space", "?", "/", "l", "w", "c", "s",
	"x", "d", "W", "S", "e", "p", "P", "char", "backspace", "shift+tab",
}

const demoFilterWord = "nas"

// demoAuto is the event that follows a state on its own, played back in
// place of a command the demo does not run.
const demoAuto = "auto"

// BuildDemo walks the demo keys breadth first from the first screen. Each
// state is rebuilt by replaying its path on a fresh model, so no model is
// shared between states. The keys of one state replay in parallel on at
// most demoWorkers goroutines, all of which finish before the next state;
// screens are numbered in key order, so the result is the same every run.
func BuildDemo(ctx context.Context, opts DemoOptions) (*Demo, error) {
	if opts.Width < spotListWidth || opts.Height < spotListHeight {
		return nil, fmt.Errorf("demo size %dx%d is below %dx%d", opts.Width, opts.Height, spotListWidth, spotListHeight)
	}
	if opts.MaxStates <= 0 {
		opts.MaxStates = 2000
	}
	d := &Demo{Width: opts.Width, Height: opts.Height}
	rowIndex := map[string]int{}
	seen := map[string]int{}
	var paths [][]string

	// Picture caches are not safe to share between goroutines; each
	// worker borrows one.
	caches := make(chan *artCache, demoWorkers)
	for range demoWorkers {
		caches <- newArtCache(termimg.Blocks)
	}
	replay := func(path []string) demoStep {
		art := <-caches
		defer func() { caches <- art }()
		m, err := demoReplay(opts, art, path)
		if err != nil {
			return demoStep{}
		}
		return demoStep{m: m, view: m.render(), ok: true}
	}

	first := replay(nil)
	if !first.ok {
		return nil, errors.New("demo: first screen failed")
	}
	base := first.m.demoWorld()

	// add records a replayed screen. A key that would start a wake or
	// connect in a world already changed by one does nothing, so the walk
	// stays at one changed machine and the site offers a reset.
	add := func(step demoStep, path []string, parent *WakeModel) (int, bool) {
		m := step.m
		if !step.ok {
			return 0, false
		}
		if parent != nil && parent.demoWorld() != base {
			if !parent.waking && !parent.opening && (m.waking || m.opening) {
				return 0, false
			}
			// Forms, filters and marks are walked in the first world only.
			opens := (m.form != nil && parent.form == nil) || (m.filtering && !parent.filtering) ||
				(m.filter != "" && parent.filter == "") || len(m.marked) > len(parent.marked) ||
				(m.confirm != "" && parent.confirm == "")
			if !parent.demoBusy() && opens {
				return 0, false
			}
		}
		id := step.view + "\x00" + m.demoHidden()
		if i, ok := seen[id]; ok {
			return i, true
		}
		if len(d.States) >= opts.MaxStates {
			return 0, false
		}
		lines := demoPad(step.view, d.Width, d.Height)
		rows := make([]int, len(lines))
		for i, line := range lines {
			r, ok := rowIndex[line]
			if !ok {
				r = len(d.Rows)
				rowIndex[line] = r
				d.Rows = append(d.Rows, line)
			}
			rows[i] = r
		}
		seen[id] = len(d.States)
		d.States = append(d.States, DemoState{Rows: rows})
		paths = append(paths, path)
		return len(d.States) - 1, true
	}

	add(first, nil, nil)
	for i := 0; i < len(paths); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := paths[i]
		parent := replay(path)
		if !parent.ok {
			continue
		}
		events := []string{}
		after, timed := parent.m.demoNextAfter()
		if timed {
			events = append(events, demoAuto)
		}
		for _, key := range DemoKeys {
			if parent.m.demoKeyAllowed(key) {
				events = append(events, key)
			}
		}
		steps := make([]demoStep, len(events))
		var wg sync.WaitGroup
		for j, event := range events {
			wg.Add(1)
			go func() {
				defer wg.Done()
				steps[j] = replay(demoExtend(path, event))
			}()
		}
		wg.Wait()
		for j, event := range events {
			next, ok := add(steps[j], demoExtend(path, event), parent.m)
			if !ok || next == i {
				continue
			}
			if event == demoAuto {
				d.States[i].Next = &DemoNext{After: int(after / time.Millisecond), State: next}
				continue
			}
			if d.States[i].Keys == nil {
				d.States[i].Keys = map[string]int{}
			}
			d.States[i].Keys[event] = next
		}
	}
	return d, nil
}

// demoPad fits a frame to the terminal the way the terminal shows it: the
// classic layouts used by some tabs on small windows draw only the rows
// they need, and anything past the last row or column is cut.
func demoPad(view string, width, height int) []string {
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		w := visibleWidth(line)
		if w > width {
			line, w = truncateANSI(line, width), width
		}
		lines[i] = line + strings.Repeat(" ", width-w)
	}
	return lines
}

// demoWorkers bounds the replays running at once.
const demoWorkers = 8

type demoStep struct {
	m    *WakeModel
	view string
	ok   bool
}

func demoExtend(path []string, event string) []string {
	out := make([]string, len(path), len(path)+1)
	copy(out, path)
	return append(out, event)
}

// demoReplay builds the first screen and plays path on it. A step that
// panics (an action the demo model cannot take) fails the whole path.
func demoReplay(opts DemoOptions, art *artCache, path []string) (m *WakeModel, err error) {
	defer func() {
		if r := recover(); r != nil {
			m, err = nil, fmt.Errorf("demo step failed: %v", r)
		}
	}()
	m = newDemoModel(opts)
	m.art = art
	for n, event := range path {
		if event == demoAuto {
			m.demoAdvance()
		} else {
			stream, streaming := m.demoStreamingTarget()
			before, said := m.demoHidden(), m.status
			m.Update(demoKey(m, event))
			if event == "x" && streaming {
				// Disconnect answers at once; the stub stops nothing.
				m.Update(disconnectResultMsg{targetID: stream.ID, deviceName: stream.Name})
			}
			// On a plain screen a key's status ("Edit cancelled.") is
			// dropped, so it does not multiply every later screen. A key
			// whose only answer is its status ("needs setup") keeps it
			// until the next key. Answers that arrive on their own keep
			// theirs until the next key too.
			only := n == len(path)-1 && m.status != said && m.demoHidden() == before
			if m.demoIdle() && !only {
				m.status = ""
			}
		}
		m.demoNormalize()
	}
	return m, nil
}

// demoNormalize drops what the model keeps but never shows again, so
// screens that look and behave the same are one state.
func (m *WakeModel) demoNormalize() {
	if !m.showHelp {
		m.helpPage = 0
	}
	if !m.actionPicker {
		m.pickerSelected = 0
	}
	if !m.filtering {
		m.filterInput = m.filter // "/" starts from the applied filter
	}
	if m.batchKind == "" && !m.batchRunning && !m.showBatchResults {
		m.batchTargets, m.batchResults, m.lastBatchKind = nil, nil, ""
	}
	// Activity keeps the newest attempt per machine. A played-back wake
	// leaves none when it was cancelled or the machine was already awake
	// (connecting to it), so both return to the first world.
	seen := map[string]bool{}
	kept := m.history[:0]
	for _, a := range m.history {
		demo := strings.HasPrefix(a.ID, "demo-") && !(m.waking && m.actionTargetID == a.TargetID)
		cancelled := demo && (m.presence[a.TargetID] != "online" || demoAwake[a.TargetID])
		if !seen[a.TargetID] && !cancelled {
			seen[a.TargetID] = true
			kept = append(kept, a)
		}
	}
	m.history = kept
}

// demoWorld is what a wake, connect or disconnect changes.
func (m *WakeModel) demoWorld() string {
	var b strings.Builder
	for _, d := range m.devices {
		fmt.Fprintf(&b, "%s=%s/%t,", d.ID, m.presence[d.ID], m.streaming[d.ID])
	}
	for _, a := range m.history {
		b.WriteString(a.ID + ",")
	}
	return b.String()
}

func (m *WakeModel) demoStreamingTarget() (store.Device, bool) {
	if m.tab != 0 {
		return store.Device{}, false
	}
	d, ok := m.selectedDevice(m.filteredDevices())
	return d, ok && m.streaming[d.ID]
}

func (m *WakeModel) demoBusy() bool {
	_, busy := m.demoNextAfter()
	return busy
}

func (m *WakeModel) demoIdle() bool {
	return !m.demoBusy() && !m.filtering && m.form == nil && m.confirm == "" && !m.actionPicker &&
		!m.showHelp && !m.showBatchResults && len(m.batchTargets) == 0
}

// demoKeyAllowed is whether the site offers key on this screen. Forms
// move between fields but never save, delete only asks, and nothing
// quits, so every screen shows something the real program does.
func (m *WakeModel) demoKeyAllowed(key string) bool {
	if m.demoBusy() {
		// Answers come on their own within seconds; a wake or connect can
		// be cancelled, the short checks and batches cannot.
		return key == "esc" && (m.waking || m.opening)
	}
	switch {
	case m.filtering:
		if key == "char" {
			return len(m.filterInput) < len(demoFilterWord)
		}
		if key == "enter" {
			return m.filterInput == "" || m.filterInput == demoFilterWord
		}
		return key == "backspace" || key == "esc"
	case key == "char" || key == "backspace":
		return false
	case m.form != nil:
		return key == "tab" || key == "shift+tab" || key == "up" || key == "down" || key == "esc"
	case m.confirm != "":
		return key == "esc"
	case len(m.marked) > 0 && m.batchKind == "" && !m.showBatchResults:
		// While machines are marked the demo stays on the batch path.
		switch key {
		case "space":
			d, ok := m.selectedDevice(m.filteredDevices())
			return ok && (m.marked[d.ID] || len(m.marked) < 2)
		case "up", "down":
			return len(m.marked) < 2 // two marked is enough for a batch
		case "W", "S", "esc":
			return true
		}
		return false
	case key == "s" && m.tab != 0:
		return false // a site check has no played-back answer
	case key == "w" && m.tab == 0:
		// Waking an awake machine again only grows the history.
		if d, ok := m.selectedDevice(m.filteredDevices()); ok && m.presence[d.ID] == "online" {
			return false
		}
	}
	return true
}

func demoKey(m *WakeModel, key string) tea.KeyPressMsg {
	switch key {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "char":
		r := rune(demoFilterWord[len(demoFilterWord)-1])
		if n := len([]rune(m.filterInput)); n < len(demoFilterWord) && strings.HasPrefix(demoFilterWord, m.filterInput) {
			r = rune(demoFilterWord[n])
		}
		return tea.KeyPressMsg{Code: r, Text: string(r)}
	}
	r := []rune(key)[0]
	return tea.KeyPressMsg{Code: r, Text: key}
}

// demoHidden is the state a screen does not show but the next key reads.
func (m *WakeModel) demoHidden() string {
	marked := make([]string, 0, len(m.marked))
	for id, on := range m.marked {
		if on {
			marked = append(marked, id)
		}
	}
	sort.Strings(marked)
	// helpPage is left out: it only grows, the frame shows it modulo the
	// page count, and the frame is already part of a state.
	return fmt.Sprintf("%d|%d|%t|%t|%t|%d|%q|%t|%q|%q|%q|%v|%t|%q|%t|%t|%t|%q|%d|%t|%d|%s",
		m.selected, m.tab, m.listView, m.showHelp, m.actionPicker, m.pickerSelected, m.confirm,
		m.filtering, m.filterInput, m.filter, m.stateFilter, marked, m.waking, m.action, m.opening,
		m.checking, m.batchRunning, m.batchKind, len(m.batchResults), m.showBatchResults, m.batchPreviewIndex, m.demoWorld())
}

// Demo pacing: how long each played-back answer takes.
const (
	demoSendAfter   = 700 * time.Millisecond
	demoTickAfter   = time.Second
	demoAnswerAfter = 6 // seconds of countdown before the machine answers
	demoCheckAfter  = 900 * time.Millisecond
	demoOpenAfter   = 2500 * time.Millisecond
	demoBatchAfter  = 450 * time.Millisecond
)

func (m *WakeModel) demoNextAfter() (time.Duration, bool) {
	switch {
	case m.waking && m.action == "wake":
		return demoSendAfter, true
	case m.waking && m.action == "wake-wait":
		return demoTickAfter, true
	case m.opening:
		return demoOpenAfter, true
	case m.checking:
		return demoCheckAfter, true
	case m.batchRunning:
		return demoBatchAfter, true
	}
	return 0, false
}

// demoAdvance plays the answer a command would have brought.
func (m *WakeModel) demoAdvance() {
	switch {
	case m.waking && m.action == "wake":
		device, _ := m.demoDevice(m.actionTargetID)
		route := wakeservice.Route{Kind: "broadcast", Destination: net.ParseIP(device.BroadcastAddress), Port: device.Port}
		attempt := store.WakeAttempt{
			ID: fmt.Sprintf("demo-%d", len(m.history)+1), TargetType: "device", TargetID: device.ID, TargetName: device.Name,
			MACAddress: device.MACAddress, Destination: device.BroadcastAddress, Port: device.Port,
			PacketStatus: "sent", VerificationStatus: "pending", Packets: 3, CreatedAt: m.now().UTC().Format(time.RFC3339),
		}
		m.Update(wakeResultMsg{operationID: m.actionID, targetID: device.ID, targetName: device.Name,
			result: wakeservice.Result{Attempt: attempt, Device: device, Route: route}})
	case m.waking && m.action == "wake-wait":
		// One second passes; the machine answers after demoAnswerAfter.
		// The clock stays put so screens after the wake match the ones
		// before; the send time moves back instead.
		m.wakeSentAt = m.wakeSentAt.Add(-time.Second)
		if m.now().Sub(m.wakeSentAt) >= demoAnswerAfter*time.Second {
			m.Update(wakeVerifyMsg{operationID: m.actionID, targetID: m.actionTargetID, targetName: m.actionTarget, status: "online"})
		}
	case m.opening:
		m.Update(remoteResultMsg{operationID: m.actionID, targetID: m.actionTargetID, deviceName: m.actionTarget})
	case m.checking:
		device, _ := m.selectedDevice(m.filteredDevices())
		status, method := m.demoPower(device.ID)
		m.Update(probeResultMsg{requestID: m.requestID, deviceID: device.ID, status: status, method: method})
	case m.batchRunning:
		if len(m.batchResults) >= len(m.batchTargets) {
			m.Update(fleetMsg{id: m.batchID, batch: true, done: true})
			m.showBatchResults, m.batchPreviewIndex = true, 0
			return
		}
		device := m.batchTargets[len(m.batchResults)]
		status, method := "sent", ""
		if m.batchKind == "check" {
			status, method = m.demoPower(device.ID)
		}
		m.Update(fleetMsg{id: m.batchID, batch: true, result: fleet.Result{DeviceID: device.ID, Name: device.Name, Status: status, Method: method}})
	}
}

func (m *WakeModel) demoDevice(id string) (store.Device, bool) {
	for _, d := range m.devices {
		if d.ID == id {
			return d, true
		}
	}
	return store.Device{}, false
}

// demoPower is what a check finds: the machine's sample power, or online
// once the demo has woken it.
func (m *WakeModel) demoPower(id string) (string, string) {
	if m.presence[id] == "online" || demoAwake[id] {
		port := 0
		if d, ok := m.demoDevice(id); ok {
			port = m.verifyPort(d)
		}
		if port > 0 {
			return "online", fmt.Sprintf("tcp:%d", port)
		}
		return "online", "ping"
	}
	return "offline", "none"
}

var demoAwake = map[string]bool{"a": true, "d": true}

// newDemoModel is the first screen: a sample inventory on documentation
// addresses, every machine checked a moment ago, pictures as half blocks.
func newDemoModel(opts DemoOptions) *WakeModel {
	m := NewWakeModel(nil, opts.Version, "")
	m.redactIdentifiers = false
	m.theme = NewTheme(true, false)
	m.motion = NewMotion(false)
	m.art = newArtCache(termimg.Blocks)
	m.phase, m.loading, m.loadingStage = phaseReady, false, ""
	m.status = ""
	m.width, m.height = opts.Width, opts.Height
	m.rootCtx = context.Background()
	m.wakeAndRemote = func(context.Context, store.Device, store.RemoteProfile) error { return nil }
	m.stopRemote = func(string) error { return nil }
	// Batches and scans start their workers inside Update; the demo's
	// runner starts none, and answers come from demoAdvance.
	m.fleetRun = func(context.Context, []store.Device, []store.Site, int, fleet.Operation) <-chan fleet.Result {
		done := make(chan fleet.Result)
		close(done)
		return done
	}
	m.sites = []store.Site{
		{ID: "s1", Name: "Home lab", Subnet: "198.51.100.0/24", BroadcastAddress: "198.51.100.255", DefaultPort: 9},
		{ID: "s2", Name: "Studio", Subnet: "203.0.113.0/24", BroadcastAddress: "203.0.113.255", DefaultPort: 9, WakeRelayID: "r1"},
	}
	m.relays = []store.WakeRelay{{ID: "r1", Name: "studio-router", Address: "203.0.113.1", Port: 9, Transport: "udp", Enabled: true}}
	m.devices = []store.Device{
		{ID: "a", Name: "studio-ws", IPAddress: "203.0.113.10", MACAddress: "02:00:00:00:01:01", BroadcastAddress: "203.0.113.255", Port: 9, SiteID: "s2", Enabled: true, DeviceType: "desktop"},
		{ID: "b", Name: "nas-vault", IPAddress: "198.51.100.20", MACAddress: "02:00:00:00:01:02", BroadcastAddress: "198.51.100.255", Port: 9, VerifyPort: 445, SiteID: "s1", Enabled: true, DeviceType: "nas"},
		{ID: "c", Name: "render-node", IPAddress: "198.51.100.30", MACAddress: "02:00:00:00:01:03", BroadcastAddress: "198.51.100.255", Port: 9, VerifyPort: 47989, SiteID: "s1", Enabled: true, DeviceType: "server"},
		{ID: "d", Name: "lab-mini", IPAddress: "198.51.100.40", MACAddress: "02:00:00:00:01:04", BroadcastAddress: "198.51.100.255", Port: 9, SiteID: "s1", Enabled: true, DeviceType: "mini"},
		{ID: "e", Name: "print-srv", IPAddress: "198.51.100.50", MACAddress: "02:00:00:00:01:05", BroadcastAddress: "198.51.100.255", Port: 9, SiteID: "s1", Enabled: true, DeviceType: "server"},
	}
	m.profiles = map[string]store.RemoteProfile{
		"a": {ID: "p1", DeviceID: "a", Protocol: "sunshine", Host: "203.0.113.10", Port: 47989, VerifyPort: 47989, Mode: "native-moonlight", Enabled: true},
		"c": {ID: "p2", DeviceID: "c", Protocol: "sunshine", Host: "198.51.100.30", Port: 47989, VerifyPort: 47989, Mode: "native-moonlight", Enabled: true},
	}
	// A fixed clock keeps every screen the same between runs.
	now := time.Date(2026, time.November, 13, 21, 40, 0, 0, time.Local)
	m.fixedNow = now
	m.checkedAt = now
	for _, d := range m.devices {
		status, method := m.demoPower(d.ID)
		m.presence[d.ID] = status
		m.presenceMethod[d.ID] = method
		m.checkedDevice[d.ID] = now
	}
	m.history = []store.WakeAttempt{
		{ID: "h1", TargetType: "device", TargetID: "d", TargetName: "lab-mini", MACAddress: "02:00:00:00:01:04", Destination: "198.51.100.255", Port: 9, PacketStatus: "sent", VerificationStatus: "online", Packets: 3, CreatedAt: now.Add(-26 * time.Minute).UTC().Format(time.RFC3339)},
		{ID: "h2", TargetType: "device", TargetID: "a", TargetName: "studio-ws", MACAddress: "02:00:00:00:01:01", Destination: "203.0.113.255", Port: 9, PacketStatus: "sent", VerificationStatus: "online", Packets: 3, CreatedAt: now.Add(-3 * time.Hour).UTC().Format(time.RFC3339)},
	}
	m.selected = 1
	return m
}
