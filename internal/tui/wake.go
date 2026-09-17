package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/aklkbqx/wol/internal/buildinfo"
	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/remoteflow"
	"github.com/aklkbqx/wol/internal/remoteopen"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
	tea "github.com/charmbracelet/bubbletea"
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

// WakeModel is the standalone Wake Desk. It opens SQLite directly and never
// starts an HTTP server, Vite, or a child service supervisor.
type WakeModel struct {
	repository    *store.Store
	service       *wakeservice.Service
	wakeAndRemote func(context.Context, store.Device, store.RemoteProfile) error
	stopRemote    func(string) error
	streaming     map[string]bool
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
	checkedDevice map[string]time.Time
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
	actionContext  context.Context
	actionID       uint64
	actionTargetID string
	actionTarget   string
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
		repository:    repository,
		service:       wakeservice.NewService(repository, wakeservice.Hooks{}),
		version:       version,
		credit:        credit,
		theme:         DetectTheme(),
		motion:        NewMotion(MotionEnabled()),
		width:         80,
		height:        24,
		presence:      make(map[string]string),
		profiles:      make(map[string]store.RemoteProfile),
		streaming:     make(map[string]bool),
		checkedDevice: make(map[string]time.Time),
		detector:      presence.NewDetector(),
		status:        "Loading local inventory...",
		loading:       true,
		phase:         phaseBootLoading,
		loadingKind:   loadingBoot,
		loadingStage:  stageInventory,
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
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func (m *WakeModel) Init() tea.Cmd {
	return m.beginRefresh(loadingBoot)
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
		if value.operationID != m.actionID || value.targetID != m.actionTargetID {
			return m, nil
		}
		if value.err == nil {
			m.prependWakeAttempt(value.result.Attempt)
			m.status = fmt.Sprintf("%s · packet sent via %s (%d). Waiting for power...", value.targetName, routeLabel(value.result.Route), value.result.Attempt.Packets)
			m.action = "wake-wait"
			return m, m.verifyWake(value.operationID, value.result.Device)
		}
		m.waking = false
		m.prependWakeAttempt(value.result.Attempt)
		m.finishAction()
		m.motion.Until = time.Time{}
		m.status = value.targetName + " · wake failed: " + value.err.Error()
		return m, nil
	case wakeVerifyMsg:
		if value.operationID != m.actionID || value.targetID != m.actionTargetID {
			return m, nil
		}
		m.waking = false
		m.finishAction()
		m.motion.Until = time.Time{}
		m.ensurePresence()
		if value.status == "online" {
			m.presence[value.targetID] = "online"
			m.ensureCheckedDevice()
			m.checkedDevice[value.targetID] = time.Now()
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
		m.motion.Until = time.Time{}
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
			m.checkedDevice[value.targetID] = time.Now()
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
		m.motion.Until = time.Time{}
		m.finishLoadContext()
		if value.err != nil {
			m.status = "Status check failed: " + value.err.Error()
		} else {
			m.ensurePresence()
			m.presence[value.deviceID] = value.status
			if m.checkedDevice == nil {
				m.checkedDevice = make(map[string]time.Time)
			}
			m.checkedDevice[value.deviceID] = time.Now()
			m.status = m.deviceName(value.deviceID) + " · power " + strings.ToUpper(value.status) + "."
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
			m.form.saving = false
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
		return m.handleFormKey(msg)
	}
	if (m.waking || m.opening) && (keyName == "q" || keyName == "ctrl+c") {
		if m.actionCancel != nil {
			m.actionCancel()
		}
		m.finishLoadContext()
		return tea.Quit
	}
	if m.actionPicker {
		return m.handleActionPicker(keyName)
	}
	if keyName == "esc" && (m.waking || m.opening) {
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
			runes := []rune(m.filterInput)
			if len(runes) > 0 {
				m.filterInput = string(runes[:len(runes)-1])
			}
		default:
			if runes := []rune(keyName); len(runes) == 1 && unicode.IsPrint(runes[0]) {
				m.filterInput += keyName
			}
		}
		return nil
	}
	if m.waking || m.opening {
		switch keyName {
		case "tab", "1", "2", "3", "m", "h", "j", "k", "up", "down", "enter", "w", "c", "f", "s", "r", "a", "e", "p", "d", "x", "/":
			m.status = m.actionTarget + " · action in progress; selection is locked. Press Esc to cancel."
			return nil
		}
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
	m.motion.TriggerStage(time.Now(), StageSelect, 180*time.Millisecond, from, m.selected)
	return m.motionTick()
}

func (m *WakeModel) motionTick() tea.Cmd {
	now := time.Now()
	if !m.motion.Active(now) {
		return nil
	}
	interval := m.motion.TickInterval()
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg { return wakeTickMsg{} })
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
	if m.opening {
		return m.renderRemoteLoadingView(inner, mode)
	}
	var builder strings.Builder
	var headerParts []string
	if m.theme.ASCII {
		headerParts = append(headerParts, m.theme.title().Render("wol"))
	} else {
		headerParts = append(headerParts, m.theme.cyan().Render("⚡ wol command center"))
	}
	if strings.Contains(m.version, "dev") {
		if m.theme.ASCII {
			headerParts = append(headerParts, m.theme.accent().Render("dev"))
		} else {
			headerParts = append(headerParts, m.theme.accent().Render("[dev]"))
		}
	}
	if m.stale {
		headerParts = append(headerParts, m.theme.danger().Render("STALE"))
	}
	headerParts = append(headerParts, m.theme.muted().Render(m.freshnessText()))
	builder.WriteString("\n" + fitText(strings.Join(headerParts, "  "), inner) + "\n")

	showTabs := inner >= 36 && (m.height <= 0 || m.height >= 20 || m.tab != 0 || m.form != nil)
	if showTabs {
		builder.WriteString(renderTabs(m.theme, m.tab, []string{"machines", "routes", "activity"}, inner) + "\n")
	}

	if m.form != nil {
		builder.WriteString("\n" + m.renderForm(inner))
	} else {
		builder.WriteString("\n")
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
		if m.motion.Active(time.Now()) && m.motion.Stage == StageSignal {
			frames := []string{"·", "o", "O", "o"}
			if m.theme.ASCII {
				frames = []string{".", "..", "...", "...."}
			}
			status = frames[int(m.frame)%len(frames)] + " " + status
		}
		builder.WriteString("\n" + renderNotice(m.theme, fitText(status, inner), inner))
	}
	if m.filtering {
		builder.WriteString("\n" + m.theme.accent().Render("filter  "+m.filterInput+"_"))
	}
	if m.confirm != "" {
		builder.WriteString("\n" + renderPanel(m.theme, "delete", "", "y confirm   n cancel", inner))
	}
	if m.showHelp {
		builder.WriteString("\n" + renderPanel(m.theme, "keys", "v"+m.version+"  "+m.credit, strings.Join([]string{
			"enter   choose wake, stream, or check",
			"w       wake",
			"c       wake and stream / remote",
			"x       disconnect stream",
			"s       check power",
			"p       remote setup",
			"r       refresh",
			"a e d   add, edit, delete",
			"tab     machines / routes / activity",
			"/       filter",
			"q       quit",
		}, "\n"), inner))
	}
	if inner >= 48 && (m.height <= 0 || m.height >= 22) {
		div := strings.Repeat("─", inner)
		if m.theme.ASCII {
			div = strings.Repeat("-", inner)
		}
		builder.WriteString("\n" + m.theme.muted().Render(div) + "\n" + m.footer(inner) + "\n")
	} else {
		builder.WriteString("\n\n" + m.footer(inner) + "\n")
	}
	return builder.String()
}
