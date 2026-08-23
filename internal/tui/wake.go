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
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
	"github.com/aklkbqx/wol/internal/wol"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type wakeDataMsg struct {
	devices []store.Device
	sites   []store.Site
	relays  []store.WakeRelay
	history []store.WakeAttempt
	err     error
}

type wakeResultMsg struct {
	result wakeservice.Result
	err    error
}

type probeResultMsg struct {
	deviceID string
	status   string
	err      error
}

type probeBatchMsg struct {
	statuses map[string]string
	summary  presence.Summary
	err      error
}

type wakeTickMsg struct{}

type formSavedMsg struct {
	message string
	keep    bool
}

type wakeFormKind string

const (
	deviceForm wakeFormKind = "device"
	relayForm  wakeFormKind = "relay"
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
	repository *store.Store
	service    *wakeservice.Service
	version    string
	credit     string
	theme      Theme
	motion     Motion

	width    int
	height   int
	tab      int
	selected int
	devices  []store.Device
	sites    []store.Site
	relays   []store.WakeRelay
	history  []store.WakeAttempt
	presence map[string]string
	detector *presence.Detector

	loading     bool
	waking      bool
	checking    bool
	status      string
	filtering   bool
	filter      string
	filterInput string
	showHelp    bool
	form        *wakeForm
	confirm     string
	frame       uint64
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
		repository: repository,
		service:    wakeservice.NewService(repository, wakeservice.Hooks{}),
		version:    version,
		credit:     credit,
		theme:      DetectTheme(),
		motion:     NewMotion(MotionEnabled()),
		width:      80,
		height:     24,
		presence:   make(map[string]string),
		detector:   presence.NewDetector(),
		status:     "Loading local inventory...",
		loading:    true,
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
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func (m *WakeModel) Init() tea.Cmd {
	return m.loadData()
}

func (m *WakeModel) loadData() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		devices, err := m.repository.ListDevices(ctx)
		if err != nil {
			return wakeDataMsg{err: err}
		}
		sites, err := m.repository.ListSites(ctx)
		if err != nil {
			return wakeDataMsg{err: err}
		}
		relays, err := m.repository.ListWakeRelays(ctx)
		if err != nil {
			return wakeDataMsg{err: err}
		}
		history, err := m.repository.ListWakeAttempts(ctx, 80)
		return wakeDataMsg{devices: devices, sites: sites, relays: relays, history: history, err: err}
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
		m.loading = false
		if value.err != nil {
			m.status = "Could not load the local inventory."
			return m, nil
		}
		m.devices, m.sites, m.relays, m.history = value.devices, value.sites, value.relays, value.history
		if m.selected >= len(m.filteredDevices()) {
			m.selected = max(0, len(m.filteredDevices())-1)
		}
		if len(m.devices) == 0 {
			m.status = fmt.Sprintf("Inventory refreshed: %d machine(s), %d route(s).", len(m.devices), len(m.relays))
			return m, nil
		}
		m.status = fmt.Sprintf("Inventory refreshed: %d machine(s), %d route(s). Checking power status...", len(m.devices), len(m.relays))
		return m, m.startPresenceScan(m.devices)
	case wakeResultMsg:
		m.waking = false
		if value.err != nil {
			m.status = "Wake failed: " + value.err.Error()
		} else {
			m.status = fmt.Sprintf("Wake sent to %s via %s (%d packet(s)).", value.result.Device.Name, routeLabel(value.result.Route), value.result.Attempt.Packets)
		}
		return m, m.loadData()
	case probeResultMsg:
		m.checking = false
		if value.err != nil {
			m.status = "Status check failed: " + value.err.Error()
		} else {
			m.ensurePresence()
			m.presence[value.deviceID] = value.status
			m.status = "Status updated: " + strings.ToUpper(value.status) + "."
		}
		return m, nil
	case probeBatchMsg:
		m.checking = false
		if value.err != nil {
			m.status = "Power scan failed: " + value.err.Error()
			return m, nil
		}
		m.ensurePresence()
		for deviceID, status := range value.statuses {
			m.presence[deviceID] = status
		}
		m.status = fmt.Sprintf("Power scan complete: %d online · %d offline · %d unknown. Wake readiness is shown separately.", value.summary.Online, value.summary.Offline, value.summary.Unknown)
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
		return m, m.loadData()
	}
	return m, nil
}

func (m *WakeModel) handleKey(msg tea.KeyMsg) tea.Cmd {
	keyName := msg.String()
	if m.form != nil {
		return m.handleFormKey(keyName)
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
			return m.beginWake(false)
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
		m.loading = true
		m.status = "Refreshing local inventory..."
		return m.loadData()
	case "a":
		m.beginAdd()
	case "e":
		m.beginEdit()
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
	if len(devices) == 0 || m.waking {
		m.status = "No machine selected. Press a to add one."
		return nil
	}
	device := devices[m.selected]
	m.waking = true
	m.status = fmt.Sprintf("Sending wake packet to %s...", device.Name)
	m.motion.Trigger(time.Now(), 1200*time.Millisecond)
	wakeCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		result, err := m.service.WakeDevice(ctx, device.ID, wakeservice.Options{Force: force, Repeat: 3, Interval: 200 * time.Millisecond, Verify: false})
		return wakeResultMsg{result: result, err: err}
	}
	return tea.Batch(wakeCmd, m.motionTick())
}

func (m *WakeModel) ensurePresence() {
	if m.presence == nil {
		m.presence = make(map[string]string)
	}
}

func (m *WakeModel) startPresenceScan(devices []store.Device) tea.Cmd {
	m.ensurePresence()
	if len(devices) == 0 {
		m.checking = false
		return nil
	}
	m.checking = true
	targets := make([]presence.Target, 0, len(devices))
	for _, device := range devices {
		targets = append(targets, presence.Target{
			DeviceID:   device.ID,
			IPAddress:  device.IPAddress,
			VerifyPort: device.VerifyPort,
		})
		m.presence[device.ID] = "checking"
	}
	detector := m.presenceDetector()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		result := detector.ProbeBatch(ctx, targets, 2500*time.Millisecond)
		statuses := make(map[string]string, len(result.Results))
		for _, item := range result.Results {
			statuses[item.DeviceID] = string(item.Status)
		}
		return probeBatchMsg{statuses: statuses, summary: result.Summary}
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
	m.ensurePresence()
	m.presence[device.ID] = "checking"
	m.status = fmt.Sprintf("Checking power at %s:%d...", device.IPAddress, port)
	detector := m.presenceDetector()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result := detector.Probe(ctx, presence.Target{DeviceID: device.ID, IPAddress: device.IPAddress, VerifyPort: port}, 2500*time.Millisecond)
		return probeResultMsg{deviceID: device.ID, status: string(result.Status)}
	}
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
		item := store.Device{Name: strings.TrimSpace(values[0]), MACAddress: strings.TrimSpace(values[1]), IPAddress: strings.TrimSpace(values[2]), BroadcastAddress: strings.TrimSpace(values[3]), Port: port, Interface: strings.TrimSpace(values[5]), VerifyPort: verifyPort, WakeStrategy: strategy, WakeRelayID: strings.TrimSpace(values[8]), DeviceType: "unknown", Platform: "unknown", Enabled: true}
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
	var builder strings.Builder
	header := m.theme.title().Render("WOL WAKE DESK") + "  " + m.theme.muted().Render("v"+m.version+"  ·  Credit: "+m.credit)
	builder.WriteString("\n" + fitText(header, inner))
	meta := m.theme.muted().Render("LOCAL INVENTORY  ·  " + modeLabel(mode) + "  ·  " + time.Now().Format("15:04:05"))
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
	if m.status != "" {
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
			"Enter           wake selected machine",
			"f               force wake",
			"s               check selected power status",
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
		if len(devices) > 0 && (m.waking || m.motion.Active(time.Now())) {
			return fleet + "\n" + m.renderSignalPath(devices[min(m.selected, len(devices)-1)], width)
		}
		return fleet
	}
	return m.renderMachineList(devices, width) + "\n" + m.renderInspector(devices, width)
}

func (m *WakeModel) renderMachineList(devices []store.Device, width int) string {
	online, offline, unknown, disabled, checking, ready, blocked := m.machineSummary(devices)
	rowWidth := max(1, width-4)
	rows := []string{
		fmt.Sprintf("MACHINES  %d total", len(devices)),
		fitText(fmt.Sprintf("POWER  %d online · %d offline · %d unknown%s%s", online, offline, unknown, summaryCount("checking", checking), summaryCount("disabled", disabled)), rowWidth),
		fitText(fmt.Sprintf("WAKE   %d ready · %d blocked", ready, blocked), rowWidth),
	}
	if len(devices) == 0 {
		rows = append(rows, "No machines yet. Press a to add one.")
	} else {
		for i, device := range devices {
			marker := " "
			if i == m.selected {
				marker = m.theme.accent().Render(m.theme.Glyph("arrow"))
			}
			power := m.deviceState(device)
			wake := m.wakeCapability(device)
			name := fitText(device.Name, max(1, rowWidth-2))
			if i == m.selected {
				name = m.theme.accent().Render(name)
			}
			if width < 42 {
				rows = append(rows, fitText(marker+" "+name, rowWidth))
				rows = append(rows, fitText("  P "+statusBadge(m.theme, power)+" · W "+statusBadge(m.theme, wake.state), rowWidth))
			} else if width < 70 {
				rows = append(rows, fitText(marker+" "+name, rowWidth))
				rows = append(rows, fitText("  POWER "+statusBadge(m.theme, power)+"  WAKE "+statusBadge(m.theme, wake.state), rowWidth))
			} else {
				row := fmt.Sprintf("%s %-18s  P:%-8s  W:%-8s  %s", marker, name, statusBadge(m.theme, power), statusBadge(m.theme, wake.state), fitText(device.IPAddress, max(1, rowWidth-52)))
				rows = append(rows, fitText(row, rowWidth))
			}
			if i < len(devices)-1 && width < 70 {
				rows = append(rows, "")
			}
		}
	}
	return wakePanel(m.theme, "FLEET", "select a machine", strings.Join(rows, "\n"), width)
}

func (m *WakeModel) renderInspector(devices []store.Device, width int) string {
	if len(devices) == 0 {
		return wakePanel(m.theme, "INSPECTOR", "ready for inventory", "Add a machine to see its route and wake controls.", width)
	}
	device := devices[min(m.selected, len(devices)-1)]
	rowWidth := max(1, width-4)
	power := m.deviceState(device)
	wake := m.wakeCapability(device)
	powerHint := ""
	if power == "UNKNOWN" {
		powerHint = "  · press s to check"
	}
	lines := []string{
		m.theme.muted().Render("SELECTED"),
		m.theme.title().Render(fitText(m.theme.Glyph("arrow")+" "+device.Name, rowWidth)),
		fitText("POWER  "+statusBadge(m.theme, power)+powerHint, rowWidth),
		fitText("WAKE   "+statusBadge(m.theme, wake.state)+"  ·  "+wake.detail, rowWidth),
		"",
		fitText("IP     "+device.IPAddress, rowWidth),
		fitText("MAC    "+device.MACAddress, rowWidth),
		fitText("PATH   "+m.routeText(device), rowWidth),
		fitText("CHECK  "+verifyText(device), rowWidth),
		"",
		m.theme.muted().Render("Enter wake  ·  s check power  ·  f force"),
	}
	if m.waking || m.motion.Active(time.Now()) {
		lines = append(lines[:4], append([]string{m.renderSignalPath(device, rowWidth), ""}, lines[4:]...)...)
	}
	return wakePanel(m.theme, "INSPECTOR", "selected machine", strings.Join(lines, "\n"), width)
}

func (m *WakeModel) renderSignalPath(device store.Device, width int) string {
	route := "LAN"
	if strings.EqualFold(device.WakeStrategy, "relay") || device.WakeRelayID != "" {
		route = "RELAY"
	}
	steps := []string{"DESK", route, strings.ToUpper(fitText(device.Name, 12))}
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
	rows := make([]string, 0, len(form.labels)+2)
	rows = append(rows, "Enter saves the final field · Tab/↑↓ moves · Esc cancels", "")
	for i, label := range form.labels {
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
	if form.error != "" {
		rows = append(rows, "", m.theme.danger().Render(form.error))
	}
	title := "EDIT MACHINE"
	if form.kind == relayForm {
		title = "EDIT ROUTE"
	}
	return wakePanel(m.theme, title, "local inventory", strings.Join(rows, "\n"), width)
}

func (m *WakeModel) footer(width int) string {
	return fitText(m.theme.muted().Render("[j/k] move  [Enter] wake  [s] check power  [r] refresh  [a/e/d] manage  [1-3] views  [/] filter  [?] help  [q] quit"), width)
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
	case "ONLINE", "READY", "SENT", "REACHABLE":
		return theme.Glyph("signal-ready")
	case "OFFLINE":
		return theme.Glyph("signal-stopped")
	case "CHECKING", "SENDING":
		return theme.Glyph("signal-busy")
	case "FAILED", "DISABLED", "BLOCKED", "TIMEOUT":
		return theme.Glyph("signal-failed")
	default:
		return "?"
	}
}

func stateStyle(theme Theme, state string) lipgloss.Style {
	switch strings.ToUpper(state) {
	case "ONLINE", "READY", "SENT", "REACHABLE":
		return theme.success()
	case "FAILED", "DISABLED", "BLOCKED", "TIMEOUT", "OFFLINE":
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
