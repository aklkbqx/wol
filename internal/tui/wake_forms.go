package tui

import (
	"context"
	"errors"
	"fmt"
	"github.com/aklkbqx/wol/internal/netutil"
	"github.com/aklkbqx/wol/internal/wol"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/power"
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
	powerForm         wakeFormKind = "power"
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
	if m.tab == 3 {
		m.beginSite(false)
		return
	}
	if m.tab == 0 {
		values := make([]string, 10)
		if m.siteFilter != "" && m.siteFilter != "unassigned" {
			values[9] = m.siteName(m.siteFilter)
		}
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
	if m.tab == 3 {
		m.beginSite(true)
		return
	}
	if m.tab == 0 {
		devices := m.filteredDevices()
		if len(devices) == 0 {
			m.status = "No machine selected."
			return
		}
		device := devices[min(m.selected, len(devices)-1)]
		m.form = newWakeForm(deviceForm, device.ID, deviceFormLabels(), []string{
			device.Name, device.MACAddress, device.IPAddress, device.BroadcastAddress,
			strconv.Itoa(device.Port), device.Interface, strconv.Itoa(device.VerifyPort),
			device.WakeStrategy, device.WakeRelayID, m.siteName(device.SiteID),
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
		relay := relays[min(m.selected, len(relays)-1)]
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
			protocol, port = "rdp", 3389
		}
		mode := "native"
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

func (m *WakeModel) beginShutdownForm() {
	if m.tab != 0 {
		return
	}
	devices := m.filteredDevices()
	if len(devices) == 0 {
		m.status = "No machine selected."
		return
	}
	device := devices[min(m.selected, len(devices)-1)]
	sshUser := ""
	sshPort := "22"
	platform := device.Platform
	if platform == "" || platform == "unknown" {
		platform = "windows"
	}
	useSudo := "no"

	if m.repository != nil {
		if profile, err := m.repository.GetPowerProfile(context.Background(), device.ID); err == nil {
			if profile.SSHUser != "" {
				sshUser = profile.SSHUser
			}
			if profile.SSHPort > 0 {
				sshPort = strconv.Itoa(profile.SSHPort)
			}
			if profile.Platform != "" {
				platform = profile.Platform
			}
			if profile.UseSudo {
				useSudo = "yes"
			}
		} else if rProfile, err := m.repository.GetRemoteProfile(context.Background(), device.ID); err == nil {
			if rProfile.UsernameHint != "" {
				sshUser = rProfile.UsernameHint
			}
		}
	}

	m.form = newWakeForm(powerForm, device.ID, powerFormLabels(), []string{
		"now", sshUser, sshPort, platform, useSudo,
	}, m.theme)
	m.status = "Power off " + device.Name + ". Enter action/delay (now, 15m, 30m, 1h, cancel)."
}

func (m *WakeModel) beginDelete() {
	if m.tab == 3 && len(m.sites) > 0 {
		s := m.sites[min(m.selected, len(m.sites)-1)]
		m.confirm = "site:" + s.ID
		m.status = "Delete site " + s.Name + "? Reassign its machines first. y confirms."
		return
	}
	if m.tab == 0 {
		devices := m.filteredDevices()
		if len(devices) > 0 {
			dev := devices[min(m.selected, len(devices)-1)]
			m.confirm = "machine:" + dev.ID
			m.status = "Delete " + dev.Name + "? press y/Enter to confirm."
		}
	} else if m.tab == 1 {
		relays := m.relayList()
		if len(relays) > 0 {
			rel := relays[min(m.selected, len(relays)-1)]
			m.confirm = "relay:" + rel.ID
			m.status = "Delete route " + rel.Name + "? press y/Enter to confirm."
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
		} else if parts[0] == "site" {
			err = m.repository.DeleteSite(ctx, parts[1])
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
	if name == "ctrl+n" {
		choices := []string{""}
		if form.kind == deviceForm && form.selected == 9 {
			for _, s := range m.sites {
				choices = append(choices, s.Name)
			}
		} else if (form.kind == siteForm && form.selected == 4) || (form.kind == deviceForm && form.selected == 8) {
			for _, r := range m.relays {
				choices = append(choices, r.Name)
			}
		} else {
			return nil
		}
		i := 0
		for j, c := range choices {
			if c == form.values[form.selected] {
				i = j
			}
		}
		value := choices[(i+1)%len(choices)]
		form.values[form.selected] = value
		form.inputs[form.selected].SetValue(value)
		return nil
	}
	if name == "ctrl+c" {
		m.finishLoadContext()
		return tea.Quit
	}
	if name == "esc" {
		m.lanQueue = nil
		m.form = nil
		m.status = "Edit cancelled."
		return nil
	}
	if name == "up" || name == "shift+tab" {
		form.setSelected(form.selected - 1)
		return nil
	}
	if name == "tab" || name == "down" || name == "enter" || name == "ctrl+s" {
		if err := validateFormField(form); err != nil {
			form.error = err.Error()
			return nil
		}
	}
	if name == "down" || name == "tab" {
		form.setSelected(form.selected + 1)
		return nil
	}
	if name == "ctrl+s" {
		return m.saveForm()
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
	sites := append([]store.Site(nil), m.sites...)
	relays := append([]store.WakeRelay(nil), m.relays...)
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
			mode := "native"
			if proto == "sunshine" {
				mode = "native-moonlight"
			} else if proto != "rdp" && proto != "ssh" && proto != "vnc" {
				mode = "browser-local"
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

		if form.kind == powerForm {
			device, err := m.repository.GetDevice(ctx, form.id)
			if err != nil {
				return formSavedMsg{message: "Power operation failed: machine no longer exists.", keep: true}
			}
			actionInput := strings.ToLower(strings.TrimSpace(values[0]))
			sshUser := strings.TrimSpace(values[1])
			sshPort, err := parseFormInt(values[2], 22)
			if err != nil {
				return formSavedMsg{message: "Power operation failed: valid port is required.", keep: true}
			}
			platform := strings.TrimSpace(values[3])
			useSudo := strings.EqualFold(strings.TrimSpace(values[4]), "yes") || strings.EqualFold(strings.TrimSpace(values[4]), "true") || strings.TrimSpace(values[4]) == "1"

			var delay time.Duration
			cancel := false
			if actionInput == "cancel" {
				cancel = true
			} else if actionInput != "now" && actionInput != "0" && actionInput != "" {
				parsed, err := time.ParseDuration(actionInput)
				if err != nil || parsed < 0 {
					return formSavedMsg{message: "Invalid action/delay: enter 'now', 'cancel', or duration like '15m', '30m', '1h'.", keep: true}
				}
				delay = parsed
			}

			sshKey := ""
			if existingProfile, err := m.repository.GetPowerProfile(ctx, device.ID); err == nil {
				sshKey = existingProfile.SSHKey
			}

			_, _ = m.repository.UpsertPowerProfile(ctx, store.PowerProfile{
				DeviceID: device.ID,
				SSHUser:  sshUser,
				SSHPort:  sshPort,
				SSHKey:   sshKey,
				Platform: platform,
				UseSudo:  useSudo,
				Enabled:  true,
			})

			target := power.Target{
				DeviceID:   device.ID,
				DeviceName: device.Name,
				Host:       device.IPAddress,
				Port:       sshPort,
				User:       sshUser,
				KeyPath:    sshKey,
				Platform:   platform,
				UseSudo:    useSudo,
			}

			svc := power.NewService(nil)
			callCtx, cancelFn := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancelFn()

			res, execErr := svc.Execute(callCtx, power.Request{
				Target: target,
				Delay:  delay,
				Cancel: cancel,
			})

			statusStr := "sent"
			msg := ""
			if execErr != nil {
				statusStr = "failed"
				msg = execErr.Error()
			}
			recordCtx, recordCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer recordCancel()
			_, _ = m.repository.RecordPowerAttempt(recordCtx, store.PowerAttempt{
				DeviceID:     device.ID,
				DeviceName:   device.Name,
				Action:       string(res.Action),
				DelaySeconds: int(delay.Seconds()),
				Status:       statusStr,
				Message:      msg,
			})

			if execErr != nil {
				return formSavedMsg{message: "Power command failed: " + execErr.Error(), keep: true}
			}

			if cancel {
				return formSavedMsg{message: device.Name + " · shutdown cancelled."}
			} else if delay > 0 {
				return formSavedMsg{message: fmt.Sprintf("%s · shutdown scheduled in %s (at %s)", device.Name, delay, res.ScheduledTime.Format("15:04:05"))}
			}
			return shutdownInitiatedMsg{deviceID: device.ID, deviceName: device.Name}
		}

		if form.kind == importForm || form.kind == exportForm {
			if err := inventoryFile(ctx, m.repository, form.kind, values[0]); err != nil {
				return formSavedMsg{message: err.Error(), keep: true}
			}
			return formSavedMsg{message: "Inventory " + string(form.kind) + " complete."}
		}
		if form.kind == siteForm {
			port, e1 := strconv.Atoi(values[5])
			timeout, e2 := strconv.Atoi(values[6])
			limit, e3 := strconv.Atoi(values[7])
			if e1 != nil || e2 != nil || e3 != nil {
				return formSavedMsg{message: "Port, timeout and concurrency must be numbers.", keep: true}
			}
			relayID := ""
			for _, r := range relays {
				if r.ID == values[4] || strings.EqualFold(r.Name, values[4]) {
					relayID = r.ID
				}
			}
			if values[4] != "" && relayID == "" {
				return formSavedMsg{message: "Relay not found. Add it in Routes first.", keep: true}
			}
			item := store.Site{Name: values[0], Subnet: values[1], BroadcastAddress: values[2], DefaultInterface: values[3], WakeRelayID: relayID, DefaultPort: port, TimeoutMS: timeout, Concurrency: limit}
			var err error
			if form.id == "" {
				_, err = m.repository.CreateSite(ctx, item)
			} else {
				_, err = m.repository.UpdateSite(ctx, form.id, item)
			}
			if err != nil {
				return formSavedMsg{message: err.Error(), keep: true}
			}
			return formSavedMsg{message: "Site saved. Review inherited routes before waking machines."}
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
		if verifyPort == 3389 {
			item.Platform = "windows"
		} else if verifyPort == 22 {
			item.Platform = "linux"
		}
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
		for _, r := range relays {
			if strings.EqualFold(r.Name, item.WakeRelayID) {
				item.WakeRelayID = r.ID
			}
		}
		if len(values) > 9 {
			oldSiteID := item.SiteID
			item.SiteID = ""
			name := strings.TrimSpace(values[9])
			for _, s := range sites {
				if s.ID == name || strings.EqualFold(s.Name, name) {
					item.SiteID = s.ID
				}
			}
			if name == oldSiteID {
				item.SiteID = oldSiteID
			}
			if name != "" && name != "Unassigned" && item.SiteID == "" {
				return formSavedMsg{message: "Site not found. Add it in Sites first.", keep: true}
			}
		}
		var saveErr error
		if form.id == "" {
			item, saveErr = m.repository.CreateDevice(ctx, item)
		} else {
			item, saveErr = m.repository.UpdateDevice(ctx, form.id, item)
		}
		if saveErr != nil {
			return formSavedMsg{message: "Machine save failed: " + saveErr.Error(), keep: true}
		}
		return formSavedMsg{message: "Machine saved.", selectedID: item.ID}
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
	return []string{"Name", "MAC address", "IP address", "Broadcast", "UDP port", "Interface", "Verify port", "Wake strategy", "Relay name (Ctrl+N)", "Site name (Ctrl+N)"}
}

func relayFormLabels() []string {
	return []string{"Name", "SSH address", "SSH port", "Router interface", "SSH user"}
}

func remoteProfileFormLabels() []string {
	return []string{"Protocol (sunshine/rdp/vnc/ssh)", "Host", "Port", "Verify port", "FPS (60/120)", "Resolution (e.g. 1920x1080)", "App name (Desktop)"}
}

func powerFormLabels() []string {
	return []string{"Action (now/15m/30m/1h/cancel)", "SSH user", "SSH port", "Platform (windows/linux/darwin)", "Use sudo (yes/no)"}
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
	rows = append(rows, m.theme.muted().Render("ctrl+s saves   tab moves   esc cancels"), "")
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
	} else if form.kind == powerForm {
		title = "power off"
	}
	return m.theme.title().Render(title) + "\n" + strings.Join(rows, "\n")
}

func validateFormField(form *wakeForm) error {
	if form.selected < 0 || form.selected >= len(form.values) {
		return nil
	}
	value := strings.TrimSpace(form.values[form.selected])
	if form.selected == 0 && value == "" {
		return fmt.Errorf("%s is required", form.labels[0])
	}
	if form.kind == siteForm && form.selected == 1 && value != "" {
		_, err := netutil.Broadcast(value)
		return err
	}
	if form.kind != deviceForm {
		return nil
	}
	switch form.selected {
	case 1:
		if _, err := wol.ParseMAC(value); err != nil {
			return fmt.Errorf("enter a valid unicast MAC address")
		}
	case 2, 3:
		if value != "" && net.ParseIP(value).To4() == nil {
			return fmt.Errorf("enter an IPv4 address, or leave blank")
		}
	case 4, 6:
		if _, err := parseFormInt(value, 0); err != nil {
			return fmt.Errorf("port must be 0–65535; 0 inherits defaults")
		}
	}
	return nil
}
