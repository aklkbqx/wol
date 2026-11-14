package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/aklkbqx/wol/internal/buildinfo"
	"github.com/aklkbqx/wol/internal/fleet"
	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/remoteflow"
	"github.com/aklkbqx/wol/internal/remoteopen"
	"github.com/aklkbqx/wol/internal/scanner"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/termimg"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
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
	operationID uint64
	targetID    string
	targetName  string
	result      wakeservice.Result
	err         error
}

type wakeVerifyMsg struct {
	operationID uint64
	targetID    string
	targetName  string
	status      string
}

type remoteResultMsg struct {
	operationID uint64
	targetID    string
	deviceName  string
	err         error
}

type disconnectResultMsg struct {
	targetID   string
	deviceName string
	err        error
}

type probeResultMsg struct {
	message   string
	requestID uint64
	deviceID  string
	status    string
	method    string
	err       error
}

type probeBatchMsg struct {
	requestID uint64
	statuses  map[string]string
	methods   map[string]string
	summary   presence.Summary
	err       error
}

type lanDiscoverMsg struct {
	id       uint64
	ifaces   []string
	devices  []store.Device
	profiles []store.RemoteProfile
	known    []scanner.LANHost
	moved    []scanner.LANHost
	unknown  []presence.Neighbor
	updated  int
	err      error
}

// clockTickMsg redraws the wake countdown once a second when motion is off.
type clockTickMsg struct{ id uint64 }

type wakeTickMsg struct {
	id uint64
}

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
	selectedID string
	message    string
	keep       bool
}

type shutdownInitiatedMsg struct {
	deviceID   string
	deviceName string
}

type shutdownVerifyMsg struct {
	operationID uint64
	targetID    string
	targetName  string
	status      string
}

// WakeModel is the standalone Wake Desk. It opens SQLite directly and never
// starts an HTTP server, Vite, or a child service supervisor.
type WakeModel struct {
	lastBatchKind     string
	showBatchResults  bool
	batchPreviewIndex int
	selectAfterLoad   string
	siteFilter        string
	stateFilter       string
	marked            map[string]bool
	batchKind         string
	batchTargets      []store.Device
	batchResults      []fleet.Result
	batchID           uint64
	batchCancel       context.CancelFunc
	batchRunning      bool
	scanDone          int
	scanTotal         int
	presenceMessages  map[string]string
	scanResults       <-chan fleet.Result
	fixedNow          time.Time
	fleetRun          func(context.Context, []store.Device, []store.Site, int, fleet.Operation) <-chan fleet.Result
	repository        *store.Store
	service           *wakeservice.Service
	wakeAndRemote     func(context.Context, store.Device, store.RemoteProfile) error
	stopRemote        func(string) error
	streaming         map[string]bool
	version           string
	credit            string
	theme             Theme
	redactIdentifiers bool
	motion            Motion
	art               *artCache // pictures; nil or mode None draws none

	width          int
	height         int
	tab            int
	selected       int
	devices        []store.Device
	sites          []store.Site
	relays         []store.WakeRelay
	history        []store.WakeAttempt
	profiles       map[string]store.RemoteProfile
	presence       map[string]string
	presenceMethod map[string]string
	detector       *presence.Detector
	pending        *wakeDataMsg
	lanOpen        bool
	lanHosts       []scanner.LANHost
	lanUnknown     []presence.Neighbor
	lanSelected    int
	lanUpdated     int
	lanID          uint64
	lanIface       string
	lanIfaces      []string
	lanMarked      map[string]bool
	lanQueue       []presence.Neighbor
	lanPrepareID   uint64
	lanPreparing   bool

	phase         viewPhase
	loadingKind   loadingKind
	loadingStage  loadingStage
	loadingTarget string
	loadingError  string
	requestID     uint64
	loadContext   context.Context
	loadCancel    context.CancelFunc
	checkedAt     time.Time
	checkedDevice map[string]time.Time
	stale         bool

	loading        bool
	waking         bool
	opening        bool
	shuttingDown   bool
	action         string
	checking       bool
	status         string
	filtering      bool
	filter         string
	filterInput    string
	showHelp       bool
	helpPage       int  // page of the Spotlight key list on a small window
	listView       bool // l: the list alone on a window too narrow for the pane
	form           *wakeForm
	confirm        string
	actionPicker   bool
	pickerSelected int
	actionCancel   context.CancelFunc
	actionContext  context.Context
	rootCtx        context.Context
	actionID       uint64
	actionTargetID string
	actionTarget   string
	motionID       uint64
	frame          uint64
	wakeSentAt     time.Time
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
		repository:        repository,
		service:           wakeservice.NewService(repository, wakeservice.Hooks{}),
		version:           version,
		credit:            credit,
		theme:             DetectTheme(),
		redactIdentifiers: envOn("WOL_TUI_REDACT"),
		motion:            NewMotion(MotionEnabled()),
		width:             80,
		height:            24,
		presence:          make(map[string]string),
		presenceMethod:    make(map[string]string),
		profiles:          make(map[string]store.RemoteProfile),
		streaming:         make(map[string]bool),
		checkedDevice:     make(map[string]time.Time),
		detector:          presence.NewDetector(),
		status:            "Loading local inventory...",
		loading:           true,
		phase:             phaseBootLoading,
		loadingKind:       loadingBoot,
		loadingStage:      stageInventory,
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
	model.stopRemote = func(deviceID string) error {
		return remoteManager.Stop(deviceID)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	model.rootCtx = ctx
	defer model.cancelAll()
	model.art = newArtCache(termimg.Detect(os.Getenv))
	program := tea.NewProgram(model, tea.WithContext(ctx))
	_, err = program.Run()
	if release := model.art.release(); release != "" {
		_, _ = os.Stdout.WriteString(release)
	}
	if err != nil && !errors.Is(err, tea.ErrInterrupted) && !errors.Is(err, tea.ErrProgramKilled) {
		return err
	}
	return nil
}

// now is the model's clock: the wall clock, or a fixed time where the
// screens must not change between runs (the website demo).
func (m *WakeModel) now() time.Time {
	if m != nil && !m.fixedNow.IsZero() {
		return m.fixedNow
	}
	return time.Now()
}

func (m *WakeModel) baseContext() context.Context {
	if m != nil && m.rootCtx != nil {
		return m.rootCtx
	}
	return context.Background()
}

func (m *WakeModel) cancelAll() {
	if m.actionCancel != nil {
		m.actionCancel()
	}
	if m.batchCancel != nil {
		m.batchCancel()
	}
	m.finishLoadContext()
}

func (m *WakeModel) Init() tea.Cmd {
	return m.beginRefresh(loadingBoot)
}

// Update handles one message, then sends any pictures the next frame needs.
func (m *WakeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	return model, m.withArt(cmd)
}

func (m *WakeModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch value := msg.(type) {
	case photoLoadedMsg:
		if m.art != nil {
			m.art.photoLoaded(value)
		}
		return m, nil
	case tea.InterruptMsg:
		m.cancelAll()
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width, m.height = value.Width, value.Height
		return m, nil
	case tea.KeyPressMsg:
		return m, m.handleKey(value)
	case wakeDataMsg:
		if value.requestID != m.requestID {
			return m, nil
		}
		if value.err != nil {
			return m, m.failLoading("Could not read the local inventory.")
		}
		m.pending = &value
		m.commitPending(m.presence, m.presenceMethod, presence.Summary{})
		return m, m.startPresenceScan(m.devices, value.requestID, value.kind, nil)
	case siteCheckMsg:
		m.status = value.message
		return m, nil
	case fleetMsg:
		return m, m.applyFleetMsg(value)
	case wakeResultMsg:
		if value.operationID != m.actionID || value.targetID != m.actionTargetID {
			return m, nil
		}
		if value.err == nil {
			m.prependWakeAttempt(value.result.Attempt)
			m.status = fmt.Sprintf("%s · packet sent via %s (%d). Waiting for power...", value.targetName, routeLabel(value.result.Route), value.result.Attempt.Packets)
			m.action = "wake-wait"
			m.wakeSentAt = m.now()
			motionCmd := m.triggerMotion(StageSignal, wakeAnswerWindow+5*time.Second, 0, 0)
			return m, tea.Batch(m.verifyWake(value.operationID, value.result.Device), motionCmd, m.clockTick())
		}
		m.waking = false
		m.prependWakeAttempt(value.result.Attempt)
		m.finishAction()
		m.stopMotion()
		m.status = value.targetName + " · wake failed: " + value.err.Error()
		return m, nil
	case wakeVerifyMsg:
		if value.operationID != m.actionID || value.targetID != m.actionTargetID {
			return m, nil
		}
		m.waking = false
		m.finishAction()
		m.stopMotion()
		m.ensurePresence()
		if value.status == "online" {
			m.presence[value.targetID] = "online"
			m.ensureCheckedDevice()
			m.checkedDevice[value.targetID] = m.now()
			m.status = value.targetName + " · ONLINE after wake."
		} else {
			m.status = value.targetName + " · packet sent, but the machine is not online yet. Press s to check again."
		}
		return m, nil
	case remoteResultMsg:
		if value.operationID != m.actionID || value.targetID != m.actionTargetID {
			return m, nil
		}
		m.opening = false
		m.finishAction()
		m.stopMotion()
		if value.err != nil {
			if errors.Is(value.err, context.Canceled) {
				m.status = value.deviceName + " · cancelled."
			} else {
				m.status = value.deviceName + " · failed: " + value.err.Error()
			}
		} else {
			m.ensurePresence()
			m.presence[value.targetID] = "online"
			m.ensureCheckedDevice()
			m.checkedDevice[value.targetID] = m.now()
			if profile, ok := m.profiles[value.targetID]; ok && streamProfile(profile) {
				m.ensureStreaming()
				m.streaming[value.targetID] = true
				m.status = value.deviceName + " · streaming"
			} else {
				m.status = value.deviceName + " · remote opened"
			}
		}
		return m, nil
	case disconnectResultMsg:
		m.ensureStreaming()
		delete(m.streaming, value.targetID)
		if value.err != nil {
			m.status = value.deviceName + " · disconnect failed: " + value.err.Error()
		} else {
			m.status = value.deviceName + " · disconnected"
		}
		return m, nil
	case probeResultMsg:
		if value.requestID != m.requestID {
			return m, nil
		}
		m.checking = false
		m.loading = false
		m.phase = phaseReady
		m.stopMotion()
		m.finishLoadContext()
		if value.err != nil {
			m.status = "Status check failed: " + value.err.Error()
		} else {
			m.ensurePresence()
			m.presence[value.deviceID] = value.status
			if m.presenceMethod == nil {
				m.presenceMethod = make(map[string]string)
			}
			if value.method != "" {
				m.presenceMethod[value.deviceID] = value.method
			}
			if m.checkedDevice == nil {
				m.checkedDevice = make(map[string]time.Time)
			}
			m.checkedDevice[value.deviceID] = m.now()
			status := strings.ToUpper(value.status)
			if value.method != "" && value.method != "none" {
				status += " via " + value.method
			}
			m.status = m.deviceName(value.deviceID) + " · power " + status + "."
			if value.message != "" {
				m.status += " " + value.message
			}
		}
		return m, nil
	case probeBatchMsg:
		if value.requestID != m.requestID {
			return m, nil
		}
		if value.err != nil {
			return m, m.failLoading("Power check failed.")
		}
		m.commitPending(value.statuses, value.methods, value.summary)
		return m, nil
	case lanDiscoverMsg:
		if value.id != m.lanID {
			return m, nil
		}
		return m, m.applyLANDiscover(value)
	case lanPrepareMsg:
		if value.id != m.lanPrepareID {
			return m, nil
		}
		m.lanPreparing = false
		m.showNeighborForm(value)
		return m, nil
	case clockTickMsg:
		if value.id != m.actionID || !m.waking {
			return m, nil
		}
		return m, m.clockTick()
	case wakeTickMsg:
		if value.id != 0 && value.id != m.motionID {
			return m, nil
		}
		if m.motion.Step(m.now()) {
			m.frame = m.motion.Frame
			return m, m.motionTick()
		}
		return m, nil
	case shutdownInitiatedMsg:
		m.form = nil
		m.actionID++
		operationID := m.actionID
		m.actionTargetID = value.deviceID
		m.actionTarget = value.deviceName
		m.action = "shutdown-wait"
		m.shuttingDown = true
		device, ok := m.actionDevice()
		if !ok {
			m.shuttingDown = false
			m.finishAction()
			m.stopMotion()
			m.status = value.deviceName + " · machine not found in active inventory."
			return m, nil
		}
		motionCmd := m.triggerMotion(StageSignal, 90*time.Second, 0, 0)
		if m.actionCancel != nil {
			m.actionCancel()
		}
		ctx, cancel := context.WithCancel(m.baseContext())
		m.actionContext = ctx
		m.actionCancel = cancel
		return m, tea.Batch(m.verifyShutdown(operationID, device), motionCmd)
	case shutdownVerifyMsg:
		if value.operationID != m.actionID || value.targetID != m.actionTargetID {
			return m, nil
		}
		m.shuttingDown = false
		m.finishAction()
		m.stopMotion()
		m.ensurePresence()
		if value.status == "offline" {
			m.presence[value.targetID] = "offline"
			m.ensureCheckedDevice()
			m.checkedDevice[value.targetID] = m.now()
			m.status = value.targetName + " · unreachable after shutdown; power-off is unconfirmed."
		} else {
			m.status = value.targetName + " · shutdown command sent, but machine is still responding. Press s to check again."
		}
		return m, nil
	case formSavedMsg:
		if value.selectedID != "" {
			m.selectAfterLoad = value.selectedID
		}
		if value.keep && m.form != nil {
			m.form.error = value.message
			m.form.saving = false
		} else {
			m.form = nil
		}
		m.status = value.message
		if !value.keep && len(m.lanQueue) > 0 {
			next := m.lanQueue[0]
			m.lanQueue = m.lanQueue[1:]
			return m, m.beginAddFromLAN(next)
		}
		if value.keep {
			return m, nil
		}
		return m, m.beginRefresh(loadingRefresh)
	}
	return m, nil
}

func (m *WakeModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	keyName := msg.String()
	if keyName == "ctrl+c" {
		m.cancelAll()
		return tea.Quit
	}
	if m.phase != phaseReady {
		switch keyName {
		case "q", "ctrl+c":
			m.cancelAll()
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
	if keyName == "esc" && m.batchRunning {
		m.batchCancel()
		m.status = "Cancelling batch; completed results are retained."
		return nil
	}
	if m.batchKind != "" && !m.batchRunning {
		switch keyName {
		case "enter", "y":
			return m.startBatch()
		case "j", "down":
			m.batchPreviewIndex = min(m.batchPreviewIndex+1, len(m.batchTargets)-1)
		case "k", "up":
			m.batchPreviewIndex = max(0, m.batchPreviewIndex-1)
		case "esc", "n":
			m.batchKind = ""
			m.batchTargets = nil
			m.status = "Batch cancelled."
		}
		return nil
	}
	if m.showBatchResults {
		switch keyName {
		case "esc", "b", "q":
			m.showBatchResults = false
		case "j", "down":
			m.batchPreviewIndex = min(m.batchPreviewIndex+1, len(m.batchResults)-1)
		case "k", "up":
			m.batchPreviewIndex = max(0, m.batchPreviewIndex-1)
		}
		return nil
	}
	if keyName == "esc" && m.lanPreparing {
		m.lanPrepareID++
		m.lanPreparing = false
		m.lanQueue = nil
		m.status = "Discovery form cancelled."
		return nil
	}
	if keyName == "esc" && m.checking {
		m.cancelLoading()
		return nil
	}
	if m.form != nil {
		return m.handleFormKey(msg)
	}
	if (m.waking || m.opening || m.shuttingDown) && (keyName == "q" || keyName == "ctrl+c") {
		m.cancelAll()
		return tea.Quit
	}
	if m.actionPicker {
		return m.handleActionPicker(keyName)
	}
	if m.lanOpen {
		return m.handleLANKey(keyName)
	}
	if keyName == "esc" && (m.waking || m.opening || m.shuttingDown) {
		target := m.actionTarget
		m.cancelAction()
		m.status = target + " · action cancelled."
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
		switch keyName {
		case "?", "esc", "q":
			m.showHelp, m.helpPage = false, 0
		case " ", "space", "j", "down":
			m.helpPage++
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
			runes := []rune(m.filterInput)
			if len(runes) > 0 {
				m.filterInput = string(runes[:len(runes)-1])
			}
		default:
			for _, r := range msg.Key().Text {
				if unicode.IsPrint(r) {
					m.filterInput += string(r)
				}
			}
		}
		return nil
	}
	if m.waking || m.opening || m.shuttingDown {
		m.status = m.actionTarget + " · action in progress; selection is locked. Press Esc to cancel."
		return nil
	}

	if m.batchRunning && strings.Contains("wcfPedanWrsiESR", keyName) && len(keyName) == 1 {
		m.status = "Finish or cancel the batch before another machine action."
		return nil
	}
	switch keyName {
	case "q", "ctrl+c":
		m.cancelAll()
		return tea.Quit
	case "tab":
		m.tab = (m.tab + 1) % 4
		m.selected = 0
	case "1":
		m.tab, m.selected = 0, 0
	case "2", "m":
		m.tab, m.selected = 1, 0
	case "3", "h":
		m.tab, m.selected = 2, 0
	case "b":
		m.showBatchResults = true
		m.batchPreviewIndex = 0
		return nil
	case "4":
		m.tab, m.selected = 3, 0
	case "[", "]":
		m.cycleSite()
		return nil
	case "v":
		m.cycleState()
		return nil
	case " ", "space":
		m.toggleMarked()
		return nil
	case "W":
		m.prepareBatch("wake")
		return nil
	case "S":
		m.prepareBatch("check")
		return nil
	case "R":
		m.retryBatch()
		return nil
	case "i":
		m.beginInventoryFile(false)
		return nil
	case "E":
		m.beginInventoryFile(true)
		return nil
	case "j", "down":
		return m.move(1)
	case "k", "up":
		return m.move(-1)
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
		if m.tab == 3 {
			return m.checkSelectedSite()
		}
		if m.tab == 0 {
			return m.probeSelected()
		}
	case "r":
		return m.beginRefresh(loadingRefresh)
	case "n":
		if m.tab == 0 {
			return m.beginLANDiscover()
		}
	case "a":
		m.beginAdd()
	case "e":
		m.beginEdit()
	case "p":
		m.beginRemoteProfile()
	case "P":
		if m.tab == 0 {
			m.beginShutdownForm()
		}
	case "x":
		if m.tab == 0 {
			return m.beginDisconnect()
		}
	case "d":
		m.beginDelete()
	case "/":
		m.filtering = true
		m.filterInput = m.filter
		m.status = "Type a filter, then press Enter."
	case "?":
		m.showHelp = true
	case "t":
		m.theme = m.theme.NextPreset()
		m.status = "Theme: " + m.theme.PresetName()
		return nil
	case "l":
		if m.tab == 0 {
			m.listView = !m.listView
		}
	case "esc":
		if m.listView {
			m.listView = false
			return nil
		}
		m.marked = make(map[string]bool)
		m.filter = ""
		m.selected = 0
		m.status = "Filter and selection cleared."
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
	m.status = "Choose an action."
}

func (m *WakeModel) handleActionPicker(keyName string) tea.Cmd {
	switch keyName {
	case "esc", "q", "5":
		m.actionPicker = false
		m.status = "Action cancelled."
	case "j", "down":
		m.pickerSelected = (m.pickerSelected + 1) % 5
	case "k", "up":
		m.pickerSelected = (m.pickerSelected + 4) % 5
	case "1", "w":
		m.actionPicker = false
		return m.beginWake(false)
	case "2", "c":
		m.actionPicker = false
		return m.beginWakeAndRemote()
	case "3", "s":
		m.actionPicker = false
		return m.probeSelected()
	case "4", "p", "P":
		m.actionPicker = false
		m.beginShutdownForm()
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
		case 3:
			m.beginShutdownForm()
		default:
			m.status = "Action cancelled."
		}
	}
	return nil
}

func (m *WakeModel) move(delta int) tea.Cmd {
	from := m.selected
	count := 0
	switch m.tab {
	case 0:
		count = len(m.filteredDevices())
	case 3:
		count = len(m.sites)
	case 1:
		count = len(m.relayList())
	default:
		count = len(m.history)
	}
	if count == 0 {
		m.selected = 0
		return nil
	}
	m.selected = (m.selected + delta + count) % count
	if m.tab != 0 || m.actionPicker || from == m.selected {
		return nil
	}
	devices := m.filteredDevices()
	visible, start := m.machineViewport(devices, contentWidth(m.width))
	if from < start || from >= start+len(visible) {
		return nil
	}
	return m.triggerMotion(StageSelect, 180*time.Millisecond, from, m.selected)
}

func (m *WakeModel) triggerMotion(stage Stage, duration time.Duration, origin, dest int) tea.Cmd {
	m.motionID++
	m.motion.TriggerStage(m.now(), stage, duration, origin, dest)
	return m.motionTick()
}

func (m *WakeModel) stopMotion() {
	m.motionID++
	m.motion.Until = time.Time{}
}

// clockTick keeps the countdown moving when motion ticks are not running.
// It stops on its own once the wake finishes or another action starts.
func (m *WakeModel) clockTick() tea.Cmd {
	if !m.waking || m.motion.Active(m.now()) {
		return nil
	}
	id := m.actionID
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return clockTickMsg{id: id} })
}

func (m *WakeModel) motionTick() tea.Cmd {
	now := m.now()
	if !m.motion.Active(now) {
		return nil
	}
	interval := m.motion.TickInterval()
	if interval <= 0 {
		return nil
	}
	id := m.motionID
	return tea.Tick(interval, func(time.Time) tea.Msg { return wakeTickMsg{id: id} })
}

func (m *WakeModel) versionLabel() string {
	version := strings.TrimSpace(m.version)
	if version == "" {
		return ""
	}
	if version[0] == 'v' || version[0] == 'V' {
		return version
	}
	return "v" + version
}

func (m *WakeModel) versionMark() string {
	label := m.versionLabel()
	if label == "" {
		return ""
	}
	style := m.theme.muted()
	if strings.Contains(strings.ToLower(m.version), "dev") {
		style = m.theme.accent()
	}
	return style.Render(label)
}

func (m *WakeModel) brandMark() string {
	if m.theme.ASCII || !m.theme.Colors {
		return m.theme.title().Render("wol")
	}
	return m.theme.ribbon().Render(" ◆ wol · wake desk ")
}

func (m *WakeModel) renderHelpPanel(width int) string {
	helpLines := []string{
		"enter   choose wake, stream, check, or power off",
		"w / c   wake only / wake and stream",
		"P / x   power off / disconnect stream",
		"s / r   check selected power / refresh fleet",
		"n       discover LAN neighbors via ARP",
		"a e d   add, edit, delete record",
		"p       configure remote profile",
		"tab     switch machines / routes / activity / sites",
		"[ ] / v cycle site filter / cycle status filter",
		"space   mark host; W batch wake; S batch check",
		"R / b   retry failed batch / view batch results",
		"i / E   import / export inventory JSON",
		"t       cycle color theme (" + m.theme.PresetName() + ")",
		"/       search filter",
		"q       quit",
	}
	maxHelp := len(helpLines)
	if m.height > 0 {
		avail := max(4, m.height-10)
		if avail < maxHelp {
			helpLines = helpLines[:avail]
		}
	}
	return renderPanel(m.theme, m.theme.Glyph("diamond")+" KEYS", "v"+m.version+"  "+m.credit, strings.Join(helpLines, "\n"), width)
}

// View wraps the rendered frame in a full-screen Bubble Tea view.
func (m *WakeModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "wol"
	return v
}

// render draws the current frame as a styled string.
// spotView draws the Spotlight frame for the current state, or reports
// false when the classic layout draws it.
func (m *WakeModel) spotView(width, height int) (string, bool) {
	if kind := m.spotPanelKind(width, height); kind != "" {
		return m.renderSpotPanel(kind, width, height), true
	}
	if m.opening {
		return "", false
	}
	if m.spotlightEligible(width, height) {
		return m.renderSpotlight(width, height), true
	}
	if m.spotFormEligible(width, height) {
		return m.renderSpotForm(width, height), true
	}
	return "", false
}

func (m *WakeModel) render() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	inner := contentWidth(width)
	mode := ResolveLayout(width, m.height)
	if m.phase != phaseReady {
		return m.renderLoadingView(inner, mode)
	}
	if view, ok := m.spotView(width, m.height); ok {
		return view
	}
	if m.opening {
		return m.renderRemoteLoadingView(inner, mode)
	}
	return m.renderClassic(width)
}

// renderClassic draws the classic layout: the other tabs, ASCII and
// windows too small for any Spotlight frame.
func (m *WakeModel) renderClassic(width int) string {
	inner := contentWidth(width)
	mode := ResolveLayout(width, m.height)
	var builder strings.Builder
	var headerParts []string
	headerParts = append(headerParts, m.brandMark())
	if version := m.versionMark(); version != "" {
		headerParts = append(headerParts, version)
	}
	if m.stale {
		headerParts = append(headerParts, m.theme.danger().Render("STALE"))
	}
	if m.redactIdentifiers {
		headerParts = append(headerParts, m.theme.amber().Render("[REDACT]"))
	}
	if len(m.marked) > 0 {
		headerParts = append(headerParts, m.theme.amber().Render(fmt.Sprintf("%d selected", len(m.marked))))
	}

	var filterChips []string
	if m.siteFilter != "" {
		filterChips = append(filterChips, "site:"+m.privateField(m.siteName(m.siteFilter)))
	}
	if m.stateFilter != "" {
		filterChips = append(filterChips, "state:"+m.stateFilter)
	}
	if m.filter != "" {
		filterChips = append(filterChips, "/"+m.privateField(m.filter))
	}
	tabMeta := ""
	if len(filterChips) > 0 {
		if inner >= 72 {
			tabMeta = m.theme.amber().Render(strings.Join(filterChips, "  "))
		} else {
			headerParts = append(headerParts, strings.Join(filterChips, " "))
		}
	}

	rightFresh := m.theme.muted().Render(m.freshnessText())
	builder.WriteString(renderHeaderRibbon(m.theme, headerParts, rightFresh, inner) + "\n")

	showTabs := inner >= 36 && (m.height <= 0 || m.height >= 20 || m.tab != 0 || m.form != nil)
	if showTabs {
		builder.WriteString(renderTabRibbon(m.theme, m.tab, []string{"machines", "routes", "activity", "sites"}, tabMeta, inner) + "\n")
	}
	if m.height <= 0 || m.height >= 30 {
		builder.WriteByte('\n')
	}

	if m.showBatchResults {
		builder.WriteString(m.renderBatchResults(inner))
	} else if m.batchKind != "" && !m.batchRunning {
		builder.WriteString(m.renderBatchPreview(inner))
	} else if m.form != nil {
		builder.WriteString(m.renderForm(inner))
	} else if m.lanOpen && m.tab == 0 {
		builder.WriteString(m.renderLANDiscover(inner))
	} else {
		switch m.tab {
		case 1:
			builder.WriteString(m.renderRoutes(inner))
		case 2:
			builder.WriteString(m.renderActivity(inner))
		case 3:
			builder.WriteString(m.renderSites(inner))
		default:
			builder.WriteString(m.renderMachines(inner, mode))
		}
	}

	showStatus := m.height <= 0 || m.height >= 22 || m.loading || m.waking || m.opening || m.shuttingDown || m.checking || statusNeedsAttention(m.status)
	if m.status != "" && showStatus {
		status := m.privateMessage(m.status)
		if m.motion.Active(m.now()) && m.motion.Stage == StageSignal {
			frames := []string{"·", "o", "O", "o"}
			if m.theme.ASCII {
				frames = []string{".", "..", "...", "...."}
			}
			status = frames[(int(m.frame)/3)%len(frames)] + " " + status
		}
		builder.WriteString("\n" + renderNotice(m.theme, fitText(status, inner), inner))
	}
	if m.filtering {
		builder.WriteString("\n" + fitText(m.theme.accent().Render("filter  "+m.privateField(m.filterInput)+"_"), inner))
	}
	if m.confirm != "" {
		builder.WriteString("\n" + renderPanel(m.theme, "delete", "", "y confirm   n cancel", inner))
	}
	if m.showHelp {
		builder.WriteString("\n" + m.renderHelpPanel(inner))
	}
	if inner >= 48 && (m.height <= 0 || m.height >= 22) {
		div := strings.Repeat("─", inner)
		if m.theme.ASCII {
			div = strings.Repeat("-", inner)
		}
		builder.WriteString("\n" + m.theme.cardLine().Render(div) + "\n" + m.footer(inner) + "\n")
	} else {
		builder.WriteString("\n\n" + m.footer(inner) + "\n")
	}
	return builder.String()
}
