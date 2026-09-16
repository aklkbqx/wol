package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/store"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	inputs   []textinput.Model
	selected int
	error    string
	saving   bool
}

func newWakeForm(kind wakeFormKind, id string, labels []string, initialValues []string, theme Theme) *wakeForm {
	inputs := make([]textinput.Model, len(labels))
	values := make([]string, len(labels))
	for i := range labels {
		ti := textinput.New()
		ti.Prompt = ""
		ti.CharLimit = 128
		val := ""
		if i < len(initialValues) {
			val = initialValues[i]
		}
		ti.SetValue(val)
		values[i] = val
		if theme.Colors {
			ti.TextStyle = theme.base()
			ti.Cursor.Style = theme.accent()
		} else {
			ti.TextStyle = lipgloss.NewStyle()
			ti.Cursor.Style = lipgloss.NewStyle().Reverse(true)
		}
		if i == 0 {
			ti.Focus()
		} else {
			ti.Blur()
		}
		inputs[i] = ti
	}
	return &wakeForm{
		kind:     kind,
		id:       id,
		labels:   labels,
		values:   values,
		inputs:   inputs,
		selected: 0,
	}
}

func (f *wakeForm) ensureInputs(theme Theme) {
	if len(f.inputs) != len(f.labels) {
		f.inputs = make([]textinput.Model, len(f.labels))
		for i := range f.labels {
			ti := textinput.New()
			ti.Prompt = ""
			ti.CharLimit = 128
			val := ""
			if i < len(f.values) {
				val = f.values[i]
			}
			ti.SetValue(val)
			if theme.Colors {
				ti.TextStyle = theme.base()
				ti.Cursor.Style = theme.accent()
			} else {
				ti.TextStyle = lipgloss.NewStyle()
				ti.Cursor.Style = lipgloss.NewStyle().Reverse(true)
			}
			if i == f.selected {
				ti.Focus()
			} else {
				ti.Blur()
			}
			f.inputs[i] = ti
		}
	} else {
		for i := range f.inputs {
			if i < len(f.values) && f.inputs[i].Value() != f.values[i] {
				f.inputs[i].SetValue(f.values[i])
			}
			if i == f.selected {
				f.inputs[i].Focus()
			} else {
				f.inputs[i].Blur()
			}
		}
	}
}

func (f *wakeForm) setSelected(index int) {
	if len(f.labels) == 0 {
		f.selected = 0
		return
	}
	f.selected = (index + len(f.labels)) % len(f.labels)
	for i := range f.inputs {
		if i == f.selected {
			f.inputs[i].Focus()
		} else {
			f.inputs[i].Blur()
		}
	}
}

func (m *WakeModel) beginAdd() {
	if m.tab == 0 {
		values := make([]string, 9)
		values[4] = "9"
		m.form = newWakeForm(deviceForm, "", deviceFormLabels(), values, m.theme)
		m.status = "Add machine: fill each field and press Enter."
		return
	}
	if m.tab == 1 {
		m.form = newWakeForm(relayForm, "", relayFormLabels(), []string{"", "", "22", "br-lan", ""}, m.theme)
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
		m.form = newWakeForm(deviceForm, device.ID, deviceFormLabels(), []string{
			device.Name, device.MACAddress, device.IPAddress, device.BroadcastAddress,
			strconv.Itoa(device.Port), device.Interface, strconv.Itoa(device.VerifyPort),
			device.WakeStrategy, device.WakeRelayID,
		}, m.theme)
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
		m.form = newWakeForm(relayForm, relay.ID, relayFormLabels(), []string{
			relay.Name, relay.Address, strconv.Itoa(relay.Port), relay.Interface, relay.SSHUser,
		}, m.theme)
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
			protocol, port = "sunshine", 47989
		}
		mode := "browser-local"
		if protocol == "sunshine" {
			mode = "native-moonlight"
		}
		profile = store.RemoteProfile{
			DeviceID: device.ID, Protocol: protocol, Host: device.IPAddress,
			Port: port, VerifyPort: port, Mode: mode, AppName: "Desktop",
			FPS: 60, Resolution: "1920x1080", Enabled: true,
		}
	}
	m.form = newWakeForm(remoteProfileForm, device.ID, remoteProfileFormLabels(), []string{
		profile.Protocol, profile.Host, strconv.Itoa(profile.Port),
		strconv.Itoa(profile.VerifyPort), strconv.Itoa(profile.FPS),
		profile.Resolution, profile.AppName,
	}, m.theme)
	m.status = "Remote profile. Passwords are never stored."
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

func (m *WakeModel) handleFormKey(msg tea.KeyMsg) tea.Cmd {
	form := m.form
	if form == nil {
		return nil
	}
	form.ensureInputs(m.theme)
	name := msg.String()
	if name == "esc" {
		m.form = nil
		m.status = "Edit cancelled."
		return nil
	}
	if name == "up" || name == "shift+tab" {
		form.setSelected(form.selected - 1)
		return nil
	}
	if name == "down" || name == "tab" {
		form.setSelected(form.selected + 1)
		return nil
	}
	if name == "enter" {
		if form.saving {
			return nil
		}
		if form.selected < len(form.labels)-1 {
			form.setSelected(form.selected + 1)
			return nil
		}
		return m.saveForm()
	}
	if form.selected >= 0 && form.selected < len(form.inputs) {
		var cmd tea.Cmd
		form.inputs[form.selected], cmd = form.inputs[form.selected].Update(msg)
		form.values[form.selected] = form.inputs[form.selected].Value()
		form.error = ""
		return cmd
	}
	return nil
}

func (m *WakeModel) saveForm() tea.Cmd {
	if m.form == nil || m.form.saving {
		return nil
	}
	m.form.saving = true
	form := *m.form
	form.ensureInputs(m.theme)
	values := append([]string(nil), form.values...)
	for i := range form.inputs {
		if i < len(values) {
			values[i] = form.inputs[i].Value()
		}
	}
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
			fps, _ := parseFormInt(values[4], 0)
			proto := strings.ToLower(strings.TrimSpace(values[0]))
			mode := "browser-local"
			if proto == "sunshine" {
				mode = "native-moonlight"
			}
			profile := store.RemoteProfile{
				DeviceID:   form.id,
				Protocol:   proto,
				Host:       strings.TrimSpace(values[1]),
				Port:       port,
				VerifyPort: verifyPort,
				FPS:        fps,
				Resolution: strings.TrimSpace(values[5]),
				AppName:    strings.TrimSpace(values[6]),
				Mode:       mode,
				Enabled:    true,
			}
			if err := validateRemoteProfile(profile); err != nil {
				return formSavedMsg{message: "Profile save failed: " + err.Error() + ".", keep: true}
			}
			if _, err := m.repository.UpsertRemoteProfile(ctx, profile); err != nil {
				return formSavedMsg{message: "Profile save failed: " + err.Error(), keep: true}
			}
			return formSavedMsg{message: "Remote profile saved."}
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
	return []string{"Protocol (sunshine/rdp/vnc/ssh)", "Host", "Port", "Verify port", "FPS (60/120)", "Resolution (e.g. 1920x1080)", "App name (Desktop)"}
}

func validateRemoteProfile(profile store.RemoteProfile) error {
	switch strings.ToLower(strings.TrimSpace(profile.Protocol)) {
	case "sunshine", "rdp", "vnc", "ssh":
	default:
		return errors.New("protocol must be sunshine, rdp, vnc, or ssh")
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
	return nil
}

func (m *WakeModel) renderForm(width int) string {
	form := m.form
	if form == nil {
		return ""
	}
	form.ensureInputs(m.theme)
	start, end := 0, len(form.labels)
	if m.height > 0 && m.height < 32 && len(form.labels) > 6 {
		start = max(0, form.selected-3)
		end = min(len(form.labels), start+6)
		start = max(0, end-6)
	}
	rows := make([]string, 0, end-start+4)
	rows = append(rows, m.theme.muted().Render("enter saves   tab moves   esc cancels"), "")
	if start > 0 {
		rows = append(rows, m.theme.muted().Render(fmt.Sprintf("%d earlier field(s)", start)))
	}
	for i := start; i < end; i++ {
		label := form.labels[i]
		marker := " "
		if i == form.selected {
			marker = m.theme.Glyph("arrow")
		}
		var val string
		if i == form.selected && i < len(form.inputs) {
			val = form.inputs[i].View()
		} else if i < len(form.values) {
			val = form.values[i]
		}
		rows = append(rows, fmt.Sprintf("%s %-18s %s", marker, label, fitText(val, max(12, width-26))))
	}
	if end < len(form.labels) {
		rows = append(rows, m.theme.muted().Render(fmt.Sprintf("%d more field(s)", len(form.labels)-end)))
	}
	if form.error != "" {
		rows = append(rows, "", m.theme.danger().Render(form.error))
	}
	title := "machine"
	if form.kind == relayForm {
		title = "route"
	} else if form.kind == remoteProfileForm {
		title = "remote"
	}
	return m.theme.title().Render(title) + "\n" + strings.Join(rows, "\n")
}
