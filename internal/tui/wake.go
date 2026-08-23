package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/buildinfo"
	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/remoteflow"
	"github.com/aklkbqx/wol/internal/remoteopen"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
	"github.com/aklkbqx/wol/internal/wol"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type wakeDataMsg struct {
	requestID uint64
	kind      loadingKind
	devices   []store.Device
	sites     []store.Site
	relays    []store.WakeRelay
	profiles  []store.RemoteProfile
	history   []store.WakeAttempt
	err       error
}

type wakeResultMsg struct {
	result wakeservice.Result
	err    error
}

type remoteResultMsg struct {
	deviceName string
	err        error
}

type probeResultMsg struct {
	requestID uint64
	deviceID  string
	status    string
	err       error
}

type probeBatchMsg struct {
	requestID uint64
	kind      loadingKind
	statuses  map[string]string
	summary   presence.Summary
	err       error
}

type wakeTickMsg struct{}

type viewPhase uint8

const (
	phaseReady viewPhase = iota
	phaseBootLoading
	phaseRefreshing
	phaseCheckingMachine
	phaseLoadError
)

type loadingKind string

const (
	loadingBoot    loadingKind = "boot"
	loadingRefresh loadingKind = "refresh"
)

type loadingStage string

const (
	stageInventory loadingStage = "inventory"
	stagePresence  loadingStage = "presence"
)

type formSavedMsg struct {
	message string
	keep    bool
}

type wakeFormKind string

const (
	deviceForm        wakeFormKind = "device"
	relayForm         wakeFormKind = "relay"
	remoteProfileForm wakeFormKind = "remote-profile"
)

type wakeForm struct {
	kind     wakeFormKind
	id       string
	labels   []string
	values   []string
	selected int
	error    string
}

// WakeModel is the standalone Wake Desk. It opens SQLite directly and never
// starts an HTTP server, Vite, or a child service supervisor.
type WakeModel struct {
	repository    *store.Store
	service       *wakeservice.Service
	wakeAndRemote func(context.Context, store.Device, store.RemoteProfile) error
	version       string
	credit        string
	theme         Theme
	motion        Motion

	width    int
	height   int
	tab      int
	selected int
	devices  []store.Device
	sites    []store.Site
	relays   []store.WakeRelay
	history  []store.WakeAttempt
	profiles map[string]store.RemoteProfile
	presence map[string]string
	detector *presence.Detector
	pending  *wakeDataMsg

	phase         viewPhase
	loadingKind   loadingKind
	loadingStage  loadingStage
	loadingTarget string
	loadingError  string
	requestID     uint64
	loadContext   context.Context
	loadCancel    context.CancelFunc
	checkedAt     time.Time
	stale         bool

	loading        bool
	waking         bool
	opening        bool
	action         string
	checking       bool
	status         string
	filtering      bool
	filter         string
	filterInput    string
	showHelp       bool
	form           *wakeForm
	confirm        string
	actionPicker   bool
	pickerSelected int
	actionCancel   context.CancelFunc
	frame          uint64
}

// NewWakeModel creates the standalone TUI model around an already-open SQLite
// store. Keeping the store outside the model makes lifecycle and tests clear.
func NewWakeModel(repository *store.Store, version, credit string) *WakeModel {
	if strings.TrimSpace(version) == "" {
		version = buildinfo.Version
	}
	if strings.TrimSpace(credit) == "" {
		credit = buildinfo.Credit
	}
	return &WakeModel{
		repository:   repository,
		service:      wakeservice.NewService(repository, wakeservice.Hooks{}),
		version:      version,
		credit:       credit,
		theme:        DetectTheme(),
		motion:       NewMotion(MotionEnabled()),
		width:        80,
		height:       24,
		presence:     make(map[string]string),
		profiles:     make(map[string]store.RemoteProfile),
		detector:     presence.NewDetector(),
		status:       "Loading local inventory...",
		loading:      true,
		phase:        phaseBootLoading,
		loadingKind:  loadingBoot,
		loadingStage: stageInventory,
	}
}

// RunWakeDesk opens the alternate screen and runs the standalone Wake Desk.
func RunWakeDesk(dbPath string) error {
	if strings.TrimSpace(dbPath) == "" {
		dbPath = store.DefaultDatabasePath()
	}
	repository, err := store.Open(filepath.Clean(dbPath))
	if err != nil {
		return errors.New("could not open local inventory")
	}
	defer repository.Close()
	model := NewWakeModel(repository, buildinfo.Version, buildinfo.Credit)
	remoteManager := remoteflow.New(repository, remoteopen.Open)
	defer remoteManager.Close()
	model.wakeAndRemote = func(ctx context.Context, device store.Device, profile store.RemoteProfile) error {
		_, err := remoteManager.Open(ctx, device, profile, true)
		return err
	}
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func (m *WakeModel) Init() tea.Cmd {
	return m.beginRefresh(loadingBoot)
}

func (m *WakeModel) beginRefresh(kind loadingKind) tea.Cmd {
	if m.loadCancel != nil {
		m.loadCancel()
	}
	m.requestID++
	m.loadingKind = kind
	m.loadingStage = stageInventory
	m.loadingTarget = ""
	m.loadingError = ""
	m.pending = nil
	m.loading = true
	m.checking = false
	if kind == loadingBoot {
		m.phase = phaseBootLoading
	} else {
		m.phase = phaseRefreshing
	}
	m.status = "Reading local inventory..."
	m.motion.Trigger(time.Now(), 30*time.Minute)
	m.loadContext, m.loadCancel = context.WithCancel(context.Background())
	return tea.Batch(m.loadData(m.loadContext, m.requestID, kind), m.motionTick())
}

func (m *WakeModel) loadData(parent context.Context, requestID uint64, kind loadingKind) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		devices, err := m.repository.ListDevices(ctx)
		if err != nil {
			return wakeDataMsg{requestID: requestID, kind: kind, err: err}
		}
		sites, err := m.repository.ListSites(ctx)
		if err != nil {
			return wakeDataMsg{requestID: requestID, kind: kind, err: err}
		}
		relays, err := m.repository.ListWakeRelays(ctx)
		if err != nil {
			return wakeDataMsg{requestID: requestID, kind: kind, err: err}
		}
		history, err := m.repository.ListWakeAttempts(ctx, 80)
		if err != nil {
			return wakeDataMsg{requestID: requestID, kind: kind, err: err}
		}
		profiles, err := m.repository.ListRemoteProfiles(ctx)
		return wakeDataMsg{requestID: requestID, kind: kind, devices: devices, sites: sites, relays: relays, profiles: profiles, history: history, err: err}
	}
}

func (m *WakeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch value := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = value.Width, value.Height
		return m, nil
	case tea.KeyMsg:
		return m, m.handleKey(value)
	case wakeDataMsg:
		if value.requestID != m.requestID {
			return m, nil
		}
		if value.err != nil {
			return m, m.failLoading("Could not read the local inventory.")
		}
		m.pending = &value
		if len(value.devices) == 0 {
			m.commitPending(nil, presence.Summary{})
			return m, nil
		}
		m.loadingStage = stagePresence
		m.checking = true
		m.status = fmt.Sprintf("Checking power for %d machine(s)...", len(value.devices))
		return m, m.startPresenceScan(value.devices, value.requestID, value.kind, m.loadContext)
	case wakeResultMsg:
		m.waking = false
		m.actionCancel = nil
		m.motion.Until = time.Time{}
		if value.err != nil {
			m.status = "Wake failed: " + value.err.Error()
		} else {
			m.status = fmt.Sprintf("Wake sent to %s via %s (%d packet(s)).", value.result.Device.Name, routeLabel(value.result.Route), value.result.Attempt.Packets)
		}
		return m, m.beginRefresh(loadingRefresh)
	case remoteResultMsg:
		m.opening = false
		m.actionCancel = nil
		m.motion.Until = time.Time{}
		if value.err != nil {
			if errors.Is(value.err, context.Canceled) {
				m.status = "Wake & Remote cancelled."
			} else {
				m.status = "Wake & Remote failed: " + value.err.Error()
			}
		} else {
			m.status = "Local remote ready for " + value.deviceName + "."
		}
		return m, nil
	case probeResultMsg:
		if value.requestID != m.requestID {
			return m, nil
		}
		m.checking = false
		m.loading = false
		m.phase = phaseReady
		m.motion.Until = time.Time{}
		m.finishLoadContext()
		if value.err != nil {
			m.status = "Status check failed: " + value.err.Error()
		} else {
			m.ensurePresence()
			m.presence[value.deviceID] = value.status
			m.checkedAt = time.Now()
			m.stale = false
			m.status = "Status updated: " + strings.ToUpper(value.status) + "."
		}
		return m, nil
	case probeBatchMsg:
		if value.requestID != m.requestID {
			return m, nil
		}
		if value.err != nil {
			return m, m.failLoading("Power check failed.")
		}
		m.commitPending(value.statuses, value.summary)
		return m, nil
	case wakeTickMsg:
		if m.motion.Step(time.Now()) {
			m.frame = m.motion.Frame
			return m, m.motionTick()
		}
		return m, nil
	case formSavedMsg:
		if value.keep && m.form != nil {
			m.form.error = value.message
		} else {
			m.form = nil
		}
		m.status = value.message
		if value.keep {
			return m, nil
		}
		return m, m.beginRefresh(loadingRefresh)
	}
	return m, nil
}

func (m *WakeModel) commitPending(statuses map[string]string, summary presence.Summary) {
	if m.pending == nil {
		return
	}
	data := m.pending
	m.devices = append([]store.Device(nil), data.devices...)
	m.sites = append([]store.Site(nil), data.sites...)
	m.relays = append([]store.WakeRelay(nil), data.relays...)
	m.history = append([]store.WakeAttempt(nil), data.history...)
	m.profiles = make(map[string]store.RemoteProfile, len(data.profiles))
	for _, profile := range data.profiles {
		m.profiles[profile.DeviceID] = profile
	}
	m.presence = make(map[string]string, len(statuses))
	for deviceID, status := range statuses {
		m.presence[deviceID] = status
	}
	if m.selected >= len(m.filteredDevices()) {
		m.selected = max(0, len(m.filteredDevices())-1)
	}
	m.pending = nil
	m.phase = phaseReady
	m.loading = false
	m.checking = false
	m.loadingError = ""
	m.loadingTarget = ""
	m.checkedAt = time.Now()
	m.stale = false
	m.motion.Until = time.Time{}
	m.finishLoadContext()
	if len(m.devices) == 0 {
		m.status = fmt.Sprintf("Inventory ready: %d machine(s), %d route(s).", len(m.devices), len(m.relays))
		return
	}
	m.status = fmt.Sprintf("Latest state ready: %d online · %d offline · %d unknown. Wake readiness is shown separately.", summary.Online, summary.Offline, summary.Unknown)
}

func (m *WakeModel) failLoading(message string) tea.Cmd {
	wasBoot := m.phase == phaseBootLoading && len(m.devices) == 0
	m.pending = nil
	m.loading = false
	m.checking = false
	m.motion.Until = time.Time{}
	m.finishLoadContext()
	if wasBoot {
		m.phase = phaseLoadError
		m.loadingError = message
		m.status = message
		return nil
	}
	m.phase = phaseReady
	m.stale = true
	m.status = message + " Showing the last verified state. Press r to retry."
	return nil
}

func (m *WakeModel) finishLoadContext() {
	if m.loadCancel != nil {
		m.loadCancel()
	}
	m.loadCancel = nil
	m.loadContext = nil
}

func (m *WakeModel) cancelLoading() {
	m.finishLoadContext()
	m.requestID++
	m.pending = nil
	m.loading = false
	m.checking = false
	m.phase = phaseReady
	m.motion.Until = time.Time{}
	m.status = "Check cancelled. Showing the last verified state."
}

func (m *WakeModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	keyName := msg.String()
	if m.phase != phaseReady {
		switch keyName {
		case "q", "ctrl+c":
			m.finishLoadContext()
			return tea.Quit
		case "r":
			if m.phase == phaseLoadError {
				return m.beginRefresh(loadingBoot)
			}
		case "esc":
			if m.phase == phaseRefreshing || m.phase == phaseCheckingMachine {
				m.cancelLoading()
			}
		}
		return nil
	}
	if m.form != nil {
		return m.handleFormKey(keyName)
	}
	if m.actionPicker {
		return m.handleActionPicker(keyName)
	}
	if keyName == "esc" && (m.waking || m.opening) {
		if m.actionCancel != nil {
			m.actionCancel()
		}
		m.status = "Cancelling the current action..."
		return nil
	}
	if m.confirm != "" {
		switch strings.ToLower(keyName) {
		case "y", "enter":
			return m.deleteConfirmed()
		case "n", "esc", "q":
			m.confirm = ""
			m.status = "Delete cancelled."
		}
		return nil
	}
	if m.showHelp {
		if keyName == "?" || keyName == "esc" || keyName == "q" {
			m.showHelp = false
		}
		return nil
	}
	if m.filtering {
		switch keyName {
		case "enter":
			m.filter = strings.TrimSpace(m.filterInput)
			m.filtering = false
			m.selected = 0
			m.status = filterMessage(m.filter)
		case "esc":
			m.filtering = false
			m.filterInput = ""
		case "backspace":
			if len(m.filterInput) > 0 {
				m.filterInput = m.filterInput[:len(m.filterInput)-1]
			}
		default:
			if len([]rune(keyName)) == 1 && keyName >= " " && keyName <= "~" {
				m.filterInput += keyName
			}
		}
		return nil
	}

	switch keyName {
	case "q", "ctrl+c":
		return tea.Quit
	case "tab":
		m.tab = (m.tab + 1) % 3
		m.selected = 0
	case "1":
		m.tab, m.selected = 0, 0
	case "2", "m":
		m.tab, m.selected = 1, 0
	case "3", "h":
		m.tab, m.selected = 2, 0
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "enter":
		if m.tab == 0 {
			m.openActionPicker()
		}
	case "w":
		if m.tab == 0 {
			return m.beginWake(false)
		}
	case "c":
		if m.tab == 0 {
			return m.beginWakeAndRemote()
		}
	case "f":
		if m.tab == 0 {
			return m.beginWake(true)
		}
	case "s":
		if m.tab == 0 {
			return m.probeSelected()
		}
	case "r":
		return m.beginRefresh(loadingRefresh)
	case "a":
		m.beginAdd()
	case "e":
		m.beginEdit()
	case "p":
		m.beginRemoteProfile()
	case "d":
		m.beginDelete()
	case "/":
		m.filtering = true
		m.filterInput = m.filter
		m.status = "Type a filter, then press Enter."
	case "?":
		m.showHelp = true
	case "esc":
		m.filter = ""
		m.selected = 0
		m.status = "Filter cleared."
	}
	return nil
}

func (m *WakeModel) openActionPicker() {
	if len(m.filteredDevices()) == 0 {
		m.status = "No machine selected. Press a to add one."
		return
	}
	if m.waking || m.opening || m.checking {
		m.status = "Finish the current action before choosing another."
		return
	}
	m.actionPicker = true
	m.pickerSelected = 0
	m.status = "Choose exactly what to do. Nothing runs until you confirm."
}

func (m *WakeModel) handleActionPicker(keyName string) tea.Cmd {
	switch keyName {
	case "esc", "q", "4":
		m.actionPicker = false
		m.status = "Action cancelled."
	case "j", "down":
		m.pickerSelected = (m.pickerSelected + 1) % 4
	case "k", "up":
		m.pickerSelected = (m.pickerSelected + 3) % 4
	case "1", "w":
		m.actionPicker = false
		return m.beginWake(false)
	case "2", "c":
		m.actionPicker = false
		return m.beginWakeAndRemote()
	case "3", "s":
		m.actionPicker = false
		return m.probeSelected()
	case "enter":
		selected := m.pickerSelected
		m.actionPicker = false
		switch selected {
		case 0:
			return m.beginWake(false)
		case 1:
			return m.beginWakeAndRemote()
		case 2:
			return m.probeSelected()
		default:
			m.status = "Action cancelled."
		}
	}
	return nil
}

func (m *WakeModel) move(delta int) {
	count := 0
	if m.tab == 0 {
		count = len(m.filteredDevices())
	} else if m.tab == 1 {
		count = len(m.relayList())
	} else {
		count = len(m.history)
	}
	if count == 0 {
		m.selected = 0
		return
	}
	m.selected = (m.selected + delta + count) % count
}

func (m *WakeModel) beginWake(force bool) tea.Cmd {
	devices := m.filteredDevices()
	if len(devices) == 0 {
		m.status = "No machine selected. Press a to add one."
		return nil
	}
	if m.waking || m.opening {
		m.status = "Finish the current action before starting another."
		return nil
	}
	device := devices[m.selected]
	m.waking = true
	m.action = "wake"
	m.status = fmt.Sprintf("Sending wake packet to %s...", device.Name)
	m.motion.Trigger(time.Now(), 1200*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	m.actionCancel = cancel
	wakeCmd := func() tea.Msg {
		defer cancel()
		timedCtx, timeoutCancel := context.WithTimeout(ctx, 35*time.Second)
		defer timeoutCancel()
		result, err := m.service.WakeDevice(timedCtx, device.ID, wakeservice.Options{Force: force, Repeat: 3, Interval: 200 * time.Millisecond, Verify: false})
		return wakeResultMsg{result: result, err: err}
	}
	return tea.Batch(wakeCmd, m.motionTick())
}

func (m *WakeModel) beginWakeAndRemote() tea.Cmd {
	devices := m.filteredDevices()
	if len(devices) == 0 {
		m.status = "No machine selected. Press a to add one."
		return nil
	}
	if m.opening || m.waking {
		m.status = "Finish the current action before starting another."
		return nil
	}
	device := devices[min(m.selected, len(devices)-1)]
	profile, ok := m.profiles[device.ID]
	if !ok || !profile.Enabled {
		m.status = "Local remote needs setup for " + device.Name + ". Press p to configure it."
		return nil
	}
	if err := validateRemoteProfile(profile); err != nil {
		m.status = "Local remote profile is incomplete: " + err.Error() + ". Press p to fix it."
		return nil
	}
	if m.wakeAndRemote == nil {
		m.status = "Local remote runtime is unavailable. Run wol remote doctor, then try again."
		return nil
	}
	m.opening = true
	m.action = "wake-remote"
	m.status = "Wake & Remote: WAKE → WAIT → LOCAL. Press Esc to cancel."
	m.motion.Trigger(time.Now(), 30*time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	m.actionCancel = cancel
	openCmd := func() tea.Msg {
		defer cancel()
		err := m.wakeAndRemote(ctx, device, profile)
		return remoteResultMsg{deviceName: device.Name, err: err}
	}
	return tea.Batch(openCmd, m.motionTick())
}

func (m *WakeModel) ensurePresence() {
	if m.presence == nil {
		m.presence = make(map[string]string)
	}
}

func (m *WakeModel) startPresenceScan(devices []store.Device, requestID uint64, kind loadingKind, parent context.Context) tea.Cmd {
	if len(devices) == 0 {
		return nil
	}
	targets := make([]presence.Target, 0, len(devices))
	for _, device := range devices {
		targets = append(targets, presence.Target{
			DeviceID:   device.ID,
			IPAddress:  device.IPAddress,
			VerifyPort: device.VerifyPort,
		})
	}
	detector := m.presenceDetector()
	if parent == nil {
		parent = context.Background()
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 4*time.Second)
		defer cancel()
		result := detector.ProbeBatch(ctx, targets, 2500*time.Millisecond)
		statuses := make(map[string]string, len(result.Results))
		for _, item := range result.Results {
			statuses[item.DeviceID] = string(item.Status)
		}
		return probeBatchMsg{requestID: requestID, kind: kind, statuses: statuses, summary: result.Summary}
	}
}

func (m *WakeModel) presenceDetector() *presence.Detector {
	if m.detector == nil {
		m.detector = presence.NewDetector()
	}
	return m.detector
}

func (m *WakeModel) motionTick() tea.Cmd {
	if !m.motion.Active(time.Now()) {
		return nil
	}
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return wakeTickMsg{} })
}

func (m *WakeModel) probeSelected() tea.Cmd {
	devices := m.filteredDevices()
	if len(devices) == 0 {
		m.status = "No machine selected."
		return nil
	}
	if m.checking {
		m.status = "Power scan is already running."
		return nil
	}
	device := devices[m.selected]
	if strings.TrimSpace(device.IPAddress) == "" {
		m.status = "Status unavailable: machine has no IP address."
		return nil
	}
	port := device.VerifyPort
	if port == 0 {
		port = 3389
	}
	if m.loadCancel != nil {
		m.loadCancel()
	}
	m.requestID++
	m.phase = phaseCheckingMachine
	m.loading = true
	m.loadingStage = stagePresence
	m.loadingTarget = device.Name
	m.status = fmt.Sprintf("Checking power at %s:%d...", device.IPAddress, port)
	detector := m.presenceDetector()
	m.checking = true
	m.motion.Trigger(time.Now(), 30*time.Minute)
	m.loadContext, m.loadCancel = context.WithCancel(context.Background())
	requestID := m.requestID
	parent := m.loadContext
	checkCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		result := detector.Probe(ctx, presence.Target{DeviceID: device.ID, IPAddress: device.IPAddress, VerifyPort: port}, 2500*time.Millisecond)
		return probeResultMsg{requestID: requestID, deviceID: device.ID, status: string(result.Status)}
	}
	return tea.Batch(checkCmd, m.motionTick())
}

func (m *WakeModel) beginAdd() {
	if m.tab == 0 {
		m.form = &wakeForm{kind: deviceForm, labels: deviceFormLabels(), values: make([]string, 9)}
		m.form.values[4] = "9"
		m.status = "Add machine: fill each field and press Enter."
		return
	}
	if m.tab == 1 {
		m.form = &wakeForm{kind: relayForm, labels: relayFormLabels(), values: []string{"", "", "22", "br-lan", ""}}
		m.status = "Add route: fill each field and press Enter."
	}
}

func (m *WakeModel) beginEdit() {
	if m.tab == 0 {
		devices := m.filteredDevices()
		if len(devices) == 0 {
			m.status = "No machine selected."
			return
		}
		device := devices[m.selected]
		m.form = &wakeForm{kind: deviceForm, id: device.ID, labels: deviceFormLabels(), values: []string{device.Name, device.MACAddress, device.IPAddress, device.BroadcastAddress, strconv.Itoa(device.Port), device.Interface, strconv.Itoa(device.VerifyPort), device.WakeStrategy, device.WakeRelayID}}
		m.status = "Edit machine: press Enter to advance and save."
		return
	}
	if m.tab == 1 {
		relays := m.relayList()
		if len(relays) == 0 {
			m.status = "No route selected."
			return
		}
		relay := relays[m.selected]
		m.form = &wakeForm{kind: relayForm, id: relay.ID, labels: relayFormLabels(), values: []string{relay.Name, relay.Address, strconv.Itoa(relay.Port), relay.Interface, relay.SSHUser}}
		m.status = "Edit route: press Enter to advance and save."
	}
}

func (m *WakeModel) beginRemoteProfile() {
	if m.tab != 0 {
		return
	}
	devices := m.filteredDevices()
	if len(devices) == 0 {
		m.status = "No machine selected."
		return
	}
	device := devices[min(m.selected, len(devices)-1)]
	profile, ok := m.profiles[device.ID]
	if !ok {
		protocol, port := "ssh", 22
		if strings.EqualFold(device.Platform, "windows") {
			protocol, port = "rdp", 3389
		}
		profile = store.RemoteProfile{DeviceID: device.ID, Protocol: protocol, Host: device.IPAddress, Port: port, VerifyPort: port, Mode: "browser-local", Enabled: true}
	}
	m.form = &wakeForm{
		kind:   remoteProfileForm,
		id:     device.ID,
		labels: remoteProfileFormLabels(),
		values: []string{profile.Protocol, profile.Host, strconv.Itoa(profile.Port), strconv.Itoa(profile.VerifyPort), profile.UsernameHint},
	}
	m.status = "Local remote profile: no password is stored."
}

func (m *WakeModel) beginDelete() {
	if m.tab == 0 {
		devices := m.filteredDevices()
		if len(devices) > 0 {
			m.confirm = "machine:" + devices[m.selected].ID
			m.status = "Delete " + devices[m.selected].Name + "? press y/Enter to confirm."
		}
	} else if m.tab == 1 {
		relays := m.relayList()
		if len(relays) > 0 {
			m.confirm = "relay:" + relays[m.selected].ID
			m.status = "Delete route " + relays[m.selected].Name + "? press y/Enter to confirm."
		}
	}
}

func (m *WakeModel) deleteConfirmed() tea.Cmd {
	confirm := m.confirm
	m.confirm = ""
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		parts := strings.SplitN(confirm, ":", 2)
		if len(parts) != 2 {
			return formSavedMsg{message: "Delete failed: invalid selection."}
		}
		var err error
		if parts[0] == "machine" {
			err = m.repository.DeleteDevice(ctx, parts[1])
		} else {
			err = m.repository.DeleteWakeRelay(ctx, parts[1])
		}
		if err != nil {
			return formSavedMsg{message: "Delete failed: " + err.Error()}
		}
		return formSavedMsg{message: "Deleted successfully."}
	}
}

func (m *WakeModel) handleFormKey(name string) tea.Cmd {
	form := m.form
	if form == nil {
		return nil
	}
	if name == "esc" {
		m.form = nil
		m.status = "Edit cancelled."
		return nil
	}
	if name == "up" || name == "shift+tab" {
		form.selected = (form.selected + len(form.labels) - 1) % len(form.labels)
		return nil
	}
	if name == "down" || name == "tab" {
		form.selected = (form.selected + 1) % len(form.labels)
		return nil
	}
	if name == "backspace" {
		if len(form.values[form.selected]) > 0 {
			form.values[form.selected] = form.values[form.selected][:len(form.values[form.selected])-1]
		}
		return nil
	}
	if name == "enter" {
		if form.selected < len(form.labels)-1 {
			form.selected++
			return nil
		}
		return m.saveForm()
	}
	if len([]rune(name)) == 1 && name >= " " && name <= "~" {
		form.values[form.selected] += name
		form.error = ""
	}
	return nil
}

func (m *WakeModel) saveForm() tea.Cmd {
	form := *m.form
	values := append([]string(nil), form.values...)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if form.kind == relayForm {
			port, err := parseFormInt(values[2], 22)
			if err != nil || strings.TrimSpace(values[0]) == "" || strings.TrimSpace(values[1]) == "" {
				return formSavedMsg{message: "Route save failed: name, address, and valid port are required.", keep: true}
			}
			item := store.WakeRelay{Name: strings.TrimSpace(values[0]), Address: strings.TrimSpace(values[1]), Port: port, Interface: strings.TrimSpace(values[3]), SSHUser: strings.TrimSpace(values[4]), Transport: "ssh_etherwake", Enabled: true}
			var saveErr error
			if form.id == "" {
				_, saveErr = m.repository.CreateWakeRelay(ctx, item)
			} else {
				_, saveErr = m.repository.UpdateWakeRelay(ctx, form.id, item)
			}
			if saveErr != nil {
				return formSavedMsg{message: "Route save failed: " + saveErr.Error(), keep: true}
			}
			return formSavedMsg{message: "Route saved."}
		}
		if form.kind == remoteProfileForm {
			port, err := parseFormInt(values[2], 0)
			if err != nil {
				return formSavedMsg{message: "Profile save failed: port must be between 1 and 65535.", keep: true}
			}
			verifyPort, err := parseFormInt(values[3], port)
			if err != nil {
				return formSavedMsg{message: "Profile save failed: verify port must be between 1 and 65535.", keep: true}
			}
			profile := store.RemoteProfile{DeviceID: form.id, Protocol: strings.ToLower(strings.TrimSpace(values[0])), Host: strings.TrimSpace(values[1]), Port: port, VerifyPort: verifyPort, UsernameHint: strings.TrimSpace(values[4]), Mode: "browser-local", Enabled: true}
			if err := validateRemoteProfile(profile); err != nil {
				return formSavedMsg{message: "Profile save failed: " + err.Error() + ".", keep: true}
			}
			if _, err := m.repository.UpsertRemoteProfile(ctx, profile); err != nil {
				return formSavedMsg{message: "Profile save failed: " + err.Error(), keep: true}
			}
			return formSavedMsg{message: "Local remote profile saved."}
		}

		port, err := parseFormInt(values[4], 9)
		if err != nil || strings.TrimSpace(values[0]) == "" || strings.TrimSpace(values[1]) == "" {
			return formSavedMsg{message: "Machine save failed: name, MAC, and valid port are required.", keep: true}
		}
		verifyPort, err := parseFormInt(values[6], 0)
		if err != nil {
			return formSavedMsg{message: "Machine save failed: verify port must be numeric.", keep: true}
		}
		strategy := strings.TrimSpace(values[7])
		if strategy == "" {
			strategy = "broadcast"
		}
		item := store.Device{DeviceType: "unknown", Platform: "unknown", Enabled: true}
		if form.id != "" {
			item, err = m.repository.GetDevice(ctx, form.id)
			if err != nil {
				return formSavedMsg{message: "Machine save failed: machine no longer exists.", keep: true}
			}
		}
		item.Name = strings.TrimSpace(values[0])
		item.MACAddress = strings.TrimSpace(values[1])
		item.IPAddress = strings.TrimSpace(values[2])
		item.BroadcastAddress = strings.TrimSpace(values[3])
		item.Port = port
		item.Interface = strings.TrimSpace(values[5])
		item.VerifyPort = verifyPort
		item.WakeStrategy = strategy
		item.WakeRelayID = strings.TrimSpace(values[8])
		var saveErr error
		if form.id == "" {
			_, saveErr = m.repository.CreateDevice(ctx, item)
		} else {
			_, saveErr = m.repository.UpdateDevice(ctx, form.id, item)
		}
		if saveErr != nil {
			return formSavedMsg{message: "Machine save failed: " + saveErr.Error(), keep: true}
		}
		return formSavedMsg{message: "Machine saved."}
	}
}

func parseFormInt(value string, fallback int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 || parsed > 65535 {
		return 0, fmt.Errorf("invalid port")
	}
	return parsed, nil
}

func deviceFormLabels() []string {
	return []string{"Name", "MAC address", "IP address", "Broadcast", "UDP port", "Interface", "Verify port", "Wake strategy", "Relay ID"}
}

func relayFormLabels() []string {
	return []string{"Name", "SSH address", "SSH port", "Router interface", "SSH user"}
}

func remoteProfileFormLabels() []string {
	return []string{"Protocol", "Host", "Port", "Verify port", "Username hint"}
}

func validateRemoteProfile(profile store.RemoteProfile) error {
	switch strings.ToLower(strings.TrimSpace(profile.Protocol)) {
	case "rdp", "vnc", "ssh":
	default:
		return errors.New("protocol must be rdp, vnc, or ssh")
	}
	if strings.TrimSpace(profile.Host) == "" {
		return errors.New("host is required")
	}
	if profile.Port < 1 || profile.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if profile.VerifyPort < 1 || profile.VerifyPort > 65535 {
		return errors.New("verify port must be between 1 and 65535")
	}
	if strings.ToLower(strings.TrimSpace(profile.Mode)) != "browser-local" {
		return errors.New("mode must be browser-local")
	}
	return nil
}

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

func (m *WakeModel) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	inner := contentWidth(width)
	mode := ResolveLayout(width, m.height)
	if m.phase != phaseReady {
		return m.renderLoadingView(inner, mode)
	}
	var builder strings.Builder
	header := m.theme.title().Render("WOL WAKE DESK") + "  " + m.theme.muted().Render("v"+m.version+"  ·  Credit: "+m.credit)
	builder.WriteString("\n" + fitText(header, inner))
	meta := m.theme.muted().Render("LOCAL INVENTORY  ·  " + modeLabel(mode) + "  ·  " + m.freshnessText())
	builder.WriteString("\n" + fitText(meta, inner) + "\n\n")
	builder.WriteString(renderTabs(m.theme, m.tab, []string{"Machines", "Routes", "Activity"}, inner) + "\n")
	builder.WriteString(m.renderRouteRail(inner) + "\n")

	if m.form != nil {
		builder.WriteString(m.renderForm(inner))
	} else {
		switch m.tab {
		case 1:
			builder.WriteString(m.renderRoutes(inner))
		case 2:
			builder.WriteString(m.renderActivity(inner))
		default:
			builder.WriteString(m.renderMachines(inner, mode))
		}
	}
	showStatus := m.height <= 0 || m.height >= 22 || m.loading || m.waking || m.opening || m.checking || statusNeedsAttention(m.status)
	if m.status != "" && showStatus {
		status := m.status
		if m.motion.Active(time.Now()) {
			frames := []string{"◐", "◓", "◑", "◒"}
			if m.theme.ASCII {
				frames = []string{".", "..", "...", "...."}
			}
			status = frames[int(m.frame)%len(frames)] + " " + status
		}
		builder.WriteString("\n" + renderNotice(m.theme, fitText(status, inner), inner))
	}
	if m.filtering {
		builder.WriteString("\n" + m.theme.accent().Render("filter: "+m.filterInput+"_"))
	}
	if m.confirm != "" {
		builder.WriteString("\n" + renderPanel(m.theme, "CONFIRM DELETE", "irreversible inventory change", "Press y/Enter to confirm or n/Esc to cancel.", inner))
	}
	if m.showHelp {
		builder.WriteString("\n" + renderPanel(m.theme, "WAKE DESK KEYS", "actions stay local", strings.Join([]string{
			"j/k or arrows   move selection",
			"Enter           choose an action; nothing runs yet",
			"w               wake only",
			"c               wake and open local remote",
			"f               force wake",
			"s               check selected power status",
			"p               configure local remote profile",
			"r               refresh inventory + power scan",
			"a/e/d           add, edit, or delete",
			"1/2/3 or Tab    switch machines, routes, activity",
			"/               filter machines",
			"q / Ctrl+C      quit",
			"NO_COLOR=1      disable terminal colors",
		}, "\n"), inner))
	}
	builder.WriteString("\n\n" + m.footer(inner) + "\n")
	return builder.String()
}

func (m *WakeModel) freshnessText() string {
	if m.checkedAt.IsZero() {
		return "not checked yet"
	}
	if m.stale {
		return "STALE · last checked " + m.checkedAt.Format("15:04:05")
	}
	return "checked " + m.checkedAt.Format("15:04:05")
}

func (m *WakeModel) renderLoadingView(width int, mode LayoutMode) string {
	var builder strings.Builder
	header := m.theme.title().Render("WOL WAKE DESK") + "  " + m.theme.muted().Render("v"+m.version+"  ·  Credit: "+m.credit)
	builder.WriteString("\n" + fitText(header, width))
	if m.phase == phaseLoadError {
		builder.WriteString("\n" + fitText(m.theme.danger().Render("LOCAL INVENTORY  ·  LOAD FAILED"), width) + "\n\n")
		builder.WriteString(fitText(m.theme.danger().Render(m.theme.Glyph("signal-failed")+" COULD NOT LOAD WAKE DESK"), width) + "\n\n")
		message := m.loadingError
		if message == "" {
			message = "The local inventory could not be read."
		}
		builder.WriteString(fitText(message, width) + "\n")
		builder.WriteString(fitText(m.theme.muted().Render("Your saved machines were not changed."), width) + "\n\n")
		builder.WriteString(fitText(m.theme.accent().Render("[r] Retry")+m.theme.muted().Render("   [q] Quit"), width) + "\n")
		return builder.String()
	}

	title := "CHECKING LATEST STATE"
	contextLabel := "LOCAL INVENTORY"
	if m.phase == phaseCheckingMachine {
		title = "CHECKING POWER"
		contextLabel = "LOCAL POWER CHECK"
	} else if m.phase == phaseRefreshing {
		contextLabel = "REFRESHING LOCAL INVENTORY"
	}
	builder.WriteString("\n" + fitText(m.theme.muted().Render(contextLabel+"  ·  "+modeLabel(mode)), width) + "\n\n")
	builder.WriteString(fitText(m.theme.title().Render(title), width) + "\n")
	if m.loadingTarget != "" {
		builder.WriteString(fitText(m.theme.accent().Render(m.loadingTarget), width) + "\n")
	}
	builder.WriteString("\n" + fitText(m.loadingSignal(width), width) + "\n\n")

	count := 0
	if m.pending != nil {
		count = len(m.pending.devices)
	}
	if mode != LayoutNarrow && width >= 36 {
		inventoryGlyph, inventoryStyle := m.theme.Glyph("signal-busy"), m.theme.accent()
		powerGlyph, powerStyle := m.theme.Glyph("signal-stopped"), m.theme.muted()
		if m.loadingStage == stagePresence || m.phase == phaseCheckingMachine {
			inventoryGlyph, inventoryStyle = m.theme.Glyph("check"), m.theme.success()
			powerGlyph, powerStyle = m.theme.Glyph("signal-busy"), m.theme.accent()
		}
		builder.WriteString(fitText(inventoryStyle.Render(inventoryGlyph+" Local inventory"), width) + "\n")
		powerText := "Power status"
		if count > 0 {
			powerText = fmt.Sprintf("Power status · %d machine(s)", count)
		}
		builder.WriteString(fitText(powerStyle.Render(powerGlyph+" "+powerText), width) + "\n")
		builder.WriteString(fitText(m.theme.muted().Render(m.theme.Glyph("signal-stopped")+" Latest view waits for verification"), width) + "\n\n")
	} else {
		stage := "Reading inventory"
		if m.loadingStage == stagePresence || m.phase == phaseCheckingMachine {
			stage = "Checking power"
			if count > 0 {
				stage = fmt.Sprintf("Checking %d machines", count)
			}
		}
		builder.WriteString(fitText(m.theme.accent().Render(m.theme.Glyph("signal-busy")+" "+stage), width) + "\n\n")
	}

	controls := "[q] Quit"
	if m.phase == phaseRefreshing || m.phase == phaseCheckingMachine {
		controls = "[Esc] Cancel   [q] Quit"
	}
	builder.WriteString(fitText(m.theme.muted().Render(controls), width) + "\n")
	return builder.String()
}

func (m *WakeModel) loadingSignal(width int) string {
	left, right := "LOCAL", "FLEET"
	if m.phase == phaseCheckingMachine {
		right = "TARGET"
	}
	railWidth := width - len(left) - len(right) - 2
	if railWidth < 1 {
		return m.theme.accent().Render(m.theme.Glyph("signal-busy") + " CHECKING")
	}
	railWidth = min(railWidth, 30)
	railRune, signalRune := "─", "●"
	if m.theme.ASCII {
		railRune, signalRune = "-", "*"
	}
	position := railWidth / 2
	if m.motion.Enabled && railWidth > 1 {
		cycle := (railWidth - 1) * 2
		position = int(m.frame % uint64(cycle))
		if position >= railWidth {
			position = cycle - position
		}
	}
	rail := strings.Repeat(railRune, position) + signalRune + strings.Repeat(railRune, railWidth-position-1)
	return m.theme.muted().Render(left+" ") + m.theme.accent().Render(rail) + m.theme.muted().Render(" "+right)
}

func (m *WakeModel) renderRouteRail(width int) string {
	if len(m.relayList()) == 0 {
		return fitText(m.theme.muted().Render("ROUTES  0 relay routes  ·  direct broadcast is the default"), width)
	}
	parts := make([]string, 0, len(m.relays))
	for i, relay := range m.relays {
		glyph, style := m.theme.Glyph("signal-stopped"), m.theme.muted()
		if relay.Enabled {
			glyph, style = m.theme.Glyph("signal-ready"), m.theme.success()
		}
		parts = append(parts, style.Render(fmt.Sprintf("%s #%d %s %s", glyph, i+1, fitText(relay.Name, 18), strings.ToUpper(relay.Transport))))
	}
	return fitText(m.theme.muted().Render("ROUTES  ")+strings.Join(parts, "   "), width)
}

func (m *WakeModel) renderMachines(width int, mode LayoutMode) string {
	devices := m.filteredDevices()
	if m.actionPicker && (mode == LayoutCompact || m.height < 34) {
		return m.renderActionPicker(devices, width)
	}
	if mode == LayoutWide {
		leftWidth := (width * 58) / 100
		if leftWidth < 36 {
			leftWidth = 36
		}
		rightWidth := width - leftWidth - 1
		if rightWidth < 28 {
			rightWidth = 28
			leftWidth = width - rightWidth - 1
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, m.renderMachineList(devices, leftWidth), m.renderInspector(devices, rightWidth))
	}
	if mode == LayoutCompact || m.height < 34 {
		fleet := m.renderMachineList(devices, width)
		if len(devices) > 0 && (m.waking || m.opening || m.motion.Active(time.Now())) {
			return fleet + "\n" + m.renderSignalPath(devices[min(m.selected, len(devices)-1)], width)
		}
		return fleet
	}
	return m.renderMachineList(devices, width) + "\n" + m.renderInspector(devices, width)
}

func (m *WakeModel) renderMachineList(devices []store.Device, width int) string {
	online, offline, unknown, disabled, checking, ready, blocked := m.machineSummary(devices)
	remoteConfigured, remoteSetup := m.remoteSummary(devices)
	rowWidth := max(1, width-4)
	rows := []string{
		fmt.Sprintf("MACHINES  %d total", len(devices)),
		fitText(fmt.Sprintf("POWER  %d online · %d offline · %d unknown%s%s", online, offline, unknown, summaryCount("checking", checking), summaryCount("disabled", disabled)), rowWidth),
		fitText(fmt.Sprintf("WAKE   %d ready · %d blocked", ready, blocked), rowWidth),
		fitText(fmt.Sprintf("REMOTE %d configured · %d setup required", remoteConfigured, remoteSetup), rowWidth),
	}
	if len(devices) == 0 {
		rows = append(rows, "No machines yet. Press a to add one.")
	} else {
		visible, start := m.machineViewport(devices, width)
		if start > 0 {
			rows = append(rows, m.theme.muted().Render(fmt.Sprintf("↑ %d machine(s) above", start)))
		}
		for i, device := range visible {
			globalIndex := start + i
			marker := " "
			if globalIndex == m.selected {
				marker = m.theme.accent().Render(m.theme.Glyph("arrow"))
			}
			power := m.deviceState(device)
			wake := m.wakeCapability(device)
			remote, _ := m.remoteCapability(device)
			name := fitText(device.Name, max(1, rowWidth-2))
			if globalIndex == m.selected {
				name = m.theme.accent().Render(name)
			}
			if width < 42 {
				rows = append(rows, fitText(marker+" "+name, rowWidth))
				rows = append(rows, fitText("  POWER  "+statusBadge(m.theme, power), rowWidth))
				rows = append(rows, fitText("  WAKE   "+statusBadge(m.theme, wake.state), rowWidth))
				rows = append(rows, fitText("  REMOTE "+statusBadge(m.theme, remote), rowWidth))
			} else if width < 88 {
				rows = append(rows, fitText(fmt.Sprintf("%s %-18s %s", marker, name, device.IPAddress), rowWidth))
				rows = append(rows, fitText("  POWER "+statusBadge(m.theme, power)+" · WAKE "+statusBadge(m.theme, wake.state)+" · REMOTE "+statusBadge(m.theme, remote), rowWidth))
			} else {
				row := marker + " " + padVisible(name, 18) + " POWER " + padVisible(statusBadge(m.theme, power), 10) + " WAKE " + padVisible(statusBadge(m.theme, wake.state), 9) + " REMOTE " + padVisible(statusBadge(m.theme, remote), 13) + " " + device.IPAddress
				rows = append(rows, fitText(row, rowWidth))
			}
			if i < len(visible)-1 && width < 70 {
				rows = append(rows, "")
			}
		}
		if below := len(devices) - (start + len(visible)); below > 0 {
			rows = append(rows, m.theme.muted().Render(fmt.Sprintf("↓ %d machine(s) below", below)))
		}
	}
	return wakePanel(m.theme, "FLEET", "select a machine", strings.Join(rows, "\n"), width)
}

func (m *WakeModel) machineViewport(devices []store.Device, width int) ([]store.Device, int) {
	if len(devices) == 0 || m.height <= 0 {
		return devices, 0
	}
	baseRows, separator := 1, 0
	if width < 42 {
		baseRows, separator = 4, 1
	} else if width < 70 {
		baseRows, separator = 2, 1
	} else if width < 88 {
		baseRows = 2
	}
	budget := m.height - 16
	if m.height < 22 {
		budget = m.height - 15
	}
	budget = max(baseRows+1, budget)
	count := len(devices)
	for count > 1 {
		rowCount := count*baseRows + max(0, count-1)*separator
		if rowCount+2 <= budget {
			break
		}
		count--
	}
	if count >= len(devices) {
		return devices, 0
	}
	start := m.selected - count/2
	start = max(0, min(start, len(devices)-count))
	return devices[start : start+count], start
}

func (m *WakeModel) renderInspector(devices []store.Device, width int) string {
	if len(devices) == 0 {
		return wakePanel(m.theme, "INSPECTOR", "ready for inventory", "Add a machine to see its route and wake controls.", width)
	}
	device := devices[min(m.selected, len(devices)-1)]
	if m.actionPicker {
		return m.renderActionPicker(devices, width)
	}
	rowWidth := max(1, width-4)
	power := m.deviceState(device)
	wake := m.wakeCapability(device)
	remote, remoteDetail := m.remoteCapability(device)
	powerHint := ""
	if power == "UNKNOWN" {
		powerHint = "  · press s to check"
	}
	lines := []string{
		m.theme.muted().Render("SELECTED"),
		m.theme.title().Render(fitText(m.theme.Glyph("arrow")+" "+device.Name, rowWidth)),
		fitText("POWER  "+statusBadge(m.theme, power)+powerHint, rowWidth),
		fitText("WAKE   "+statusBadge(m.theme, wake.state)+"  ·  "+wake.detail, rowWidth),
		fitText("REMOTE "+statusBadge(m.theme, remote)+"  ·  "+remoteDetail, rowWidth),
		"",
		m.theme.accent().Render(fitText("[Enter] Choose action", rowWidth)),
		m.theme.muted().Render(fitText("w wake only · c wake & remote · s check · p setup", rowWidth)),
		"",
		fitText("IP     "+device.IPAddress, rowWidth),
		fitText("MAC    "+device.MACAddress, rowWidth),
		fitText("PATH   "+m.routeText(device), rowWidth),
		fitText("CHECK  "+verifyText(device), rowWidth),
	}
	if m.waking || m.opening || m.motion.Active(time.Now()) {
		lines = append(lines[:5], append([]string{m.renderSignalPath(device, rowWidth), ""}, lines[5:]...)...)
	}
	return wakePanel(m.theme, "ACTION DECK", "selected machine", strings.Join(lines, "\n"), width)
}

func (m *WakeModel) renderActionPicker(devices []store.Device, width int) string {
	if len(devices) == 0 {
		return wakePanel(m.theme, "CHOOSE ACTION", "no machine selected", "Press Esc to return.", width)
	}
	device := devices[min(m.selected, len(devices)-1)]
	items := []struct{ key, label, detail string }{
		{"w", "Wake only", "Send a wake packet; do not open anything"},
		{"c", "Wake & Remote", "Wake, wait, then open a localhost session"},
		{"s", "Check power", "Refresh this machine's power state"},
		{"Esc", "Cancel", "Return without running an action"},
	}
	compact := width < 30 || (m.height > 0 && m.height <= 20)
	rows := []string{m.theme.title().Render(fitText(device.Name, max(1, width-4)))}
	if !compact {
		rows = append(rows, "Nothing runs until you confirm.", "")
	}
	for i, item := range items {
		marker := " "
		label := item.label
		if i == m.pickerSelected {
			marker = m.theme.Glyph("arrow")
			label = m.theme.accent().Render(label)
		}
		rows = append(rows, fitText(fmt.Sprintf("%s [%s] %s", marker, item.key, label), max(1, width-4)))
		if width >= 48 && !compact {
			rows = append(rows, m.theme.muted().Render(fitText("    "+item.detail, max(1, width-4))))
		}
	}
	title, subtitle := "CHOOSE ACTION", "explicit and local"
	if compact {
		title, subtitle = "ACTION", ""
	}
	return wakePanel(m.theme, title, subtitle, strings.Join(rows, "\n"), width)
}

func (m *WakeModel) renderSignalPath(device store.Device, width int) string {
	if m.action == "wake-remote" {
		return m.renderActionPath([]string{"WAKE", "WAIT", "LOCAL"}, width)
	}
	route := "LAN"
	if strings.EqualFold(device.WakeStrategy, "relay") || device.WakeRelayID != "" {
		route = "RELAY"
	}
	return m.renderActionPath([]string{"DESK", route, strings.ToUpper(fitText(device.Name, 12))}, width)
}

func (m *WakeModel) renderActionPath(steps []string, width int) string {
	connector := "──"
	pulse := "●"
	if m.theme.ASCII {
		connector, pulse = "--", "*"
	}
	active := int(m.frame % 2)
	left, right := connector+">", connector+">"
	if active == 0 {
		left = pulse + connector + ">"
	} else {
		right = pulse + connector + ">"
	}
	return fitText(m.theme.accent().Render(steps[0]+" "+left+" "+steps[1]+" "+right+" "+steps[2]), width)
}

func (m *WakeModel) renderRoutes(width int) string {
	rows := []string{fmt.Sprintf("ROUTES  %d configured", len(m.relays))}
	if len(m.relays) == 0 {
		rows = append(rows, "No routes yet. Press a to add an SSH etherwake relay.")
	} else {
		for i, relay := range m.relays {
			marker := " "
			if i == m.selected {
				marker = m.theme.Glyph("arrow")
			}
			state := "READY"
			if !relay.Enabled {
				state = "DISABLED"
			}
			rows = append(rows, fmt.Sprintf("%s %-20s %-8s %s:%d  iface=%s", marker, fitText(relay.Name, 20), stateStyle(m.theme, state).Render(state), fitText(relay.Address, 18), relay.Port, relay.Interface))
		}
	}
	return wakePanel(m.theme, "ROUTE MANAGER", "SSH etherwake relays", strings.Join(rows, "\n"), width)
}

func (m *WakeModel) renderActivity(width int) string {
	rows := []string{fmt.Sprintf("RECENT SIGNALS  %d stored", len(m.history))}
	if len(m.history) == 0 {
		rows = append(rows, "No wake activity yet.")
	} else {
		for i, attempt := range m.history {
			marker := " "
			if i == m.selected {
				marker = m.theme.Glyph("arrow")
			}
			when := attempt.CreatedAt
			if parsed, err := time.Parse(time.RFC3339Nano, attempt.CreatedAt); err == nil {
				when = parsed.Local().Format("15:04:05")
			}
			rows = append(rows, fmt.Sprintf("%s %-8s %-18s %-8s %s", marker, when, fitText(attempt.TargetName, 18), stateStyle(m.theme, strings.ToUpper(attempt.PacketStatus)).Render(strings.ToUpper(attempt.PacketStatus)), fitText(attempt.Message, max(12, width-56))))
		}
	}
	return wakePanel(m.theme, "ACTIVITY", "local wake history", strings.Join(rows, "\n"), width)
}

func (m *WakeModel) renderForm(width int) string {
	form := m.form
	start, end := 0, len(form.labels)
	if m.height > 0 && m.height < 32 && len(form.labels) > 6 {
		start = max(0, form.selected-3)
		end = min(len(form.labels), start+6)
		start = max(0, end-6)
	}
	rows := make([]string, 0, end-start+4)
	rows = append(rows, "Enter saves the final field · Tab/↑↓ moves · Esc cancels", "")
	if start > 0 {
		rows = append(rows, m.theme.muted().Render(fmt.Sprintf("↑ %d earlier field(s)", start)))
	}
	for i := start; i < end; i++ {
		label := form.labels[i]
		marker := " "
		if i == form.selected {
			marker = m.theme.Glyph("arrow")
		}
		value := form.values[i]
		if i == form.selected {
			value += "_"
		}
		rows = append(rows, fmt.Sprintf("%s %-18s %s", marker, label, fitText(value, max(12, width-26))))
	}
	if end < len(form.labels) {
		rows = append(rows, m.theme.muted().Render(fmt.Sprintf("↓ %d more field(s)", len(form.labels)-end)))
	}
	if form.error != "" {
		rows = append(rows, "", m.theme.danger().Render(form.error))
	}
	title := "EDIT MACHINE"
	if form.kind == relayForm {
		title = "EDIT ROUTE"
	} else if form.kind == remoteProfileForm {
		title = "LOCAL REMOTE PROFILE"
	}
	return wakePanel(m.theme, title, "local inventory", strings.Join(rows, "\n"), width)
}

func (m *WakeModel) footer(width int) string {
	return fitText(m.theme.muted().Render("[j/k] move  [Enter] choose  [w] wake  [c] wake+remote  [s] power  [p] remote setup  [r] refresh  [?] help  [q] quit"), width)
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
	profile, ok := m.profiles[device.ID]
	if !ok || !profile.Enabled {
		return "SETUP", "press p to configure localhost remote"
	}
	if err := validateRemoteProfile(profile); err != nil {
		return "SETUP", "profile incomplete; press p"
	}
	return "READY", strings.ToUpper(profile.Protocol) + " · " + profile.Mode
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

func verifyText(device store.Device) string {
	if strings.TrimSpace(device.IPAddress) == "" {
		return "no IP address"
	}
	port := device.VerifyPort
	if port == 0 {
		return net.JoinHostPort(device.IPAddress, "3389") + " (auto fallback)"
	}
	if port < 1 || port > 65535 {
		return "invalid TCP port"
	}
	return net.JoinHostPort(device.IPAddress, strconv.Itoa(device.VerifyPort))
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

func summaryCount(label string, count int) string {
	if count == 0 {
		return ""
	}
	return fmt.Sprintf(" · %d %s", count, label)
}

func statusBadge(theme Theme, state string) string {
	state = strings.ToUpper(strings.TrimSpace(state))
	if state == "" {
		state = "UNKNOWN"
	}
	return stateStyle(theme, state).Render(statusGlyph(theme, state) + " " + state)
}

func statusGlyph(theme Theme, state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "ONLINE", "READY", "SENT", "REACHABLE", "CONFIGURED":
		return theme.Glyph("signal-ready")
	case "OFFLINE":
		return theme.Glyph("signal-stopped")
	case "CHECKING", "SENDING":
		return theme.Glyph("signal-busy")
	case "FAILED", "DISABLED", "BLOCKED", "TIMEOUT", "INVALID":
		return theme.Glyph("signal-failed")
	default:
		return "?"
	}
}

func stateStyle(theme Theme, state string) lipgloss.Style {
	switch strings.ToUpper(state) {
	case "ONLINE", "READY", "SENT", "REACHABLE", "CONFIGURED":
		return theme.success()
	case "FAILED", "DISABLED", "BLOCKED", "TIMEOUT", "OFFLINE", "INVALID":
		return theme.danger()
	case "SENDING", "CHECKING":
		return theme.accent()
	default:
		return theme.muted()
	}
}

func wakePanel(theme Theme, title, subtitle, body string, width int) string {
	if width < 12 {
		width = 12
	}
	inner := width - 2
	header := theme.title().Render(title)
	if subtitle != "" {
		header += "  " + theme.muted().Render(subtitle)
	}
	content := header
	if body != "" {
		content += "\n\n" + body
	}
	style := lipgloss.NewStyle().Width(inner).Padding(0, 1)
	if theme.Colors {
		style = style.Border(lipgloss.NormalBorder()).BorderForeground(theme.Palette.Border)
	}
	return style.Render(content)
}

func modeLabel(mode LayoutMode) string { return string(mode) }

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
