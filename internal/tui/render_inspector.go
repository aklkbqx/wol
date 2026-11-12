package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/store"
)

func (m *WakeModel) renderInspectorBounded(devices []store.Device, width int, maxLines int) string {
	if len(devices) == 0 {
		return m.theme.muted().Render("add a machine to start")
	}
	device, _ := m.selectedDevice(devices)
	if m.actionPicker {
		return m.renderActionPicker(devices, width)
	}
	active := m.waking || m.opening || m.shuttingDown
	rowWidth := max(1, width)
	devState := m.deviceState(device)
	power := powerWord(devState)
	wake := m.wakeCapability(device)
	action := m.actionWord(device)

	powerStyled := stateStyle(m.theme, devState).Render(stateGlyph(m.theme, devState) + " " + power)
	if active && (m.actionTargetID == "" || device.ID == m.actionTargetID) {
		spinner := "●"
		if m.theme.ASCII {
			spinner = "*"
		}
		if m.motion.Enabled {
			if m.theme.ASCII {
				asciiSpinners := []string{"-", "\\", "|", "/"}
				spinner = asciiSpinners[(int(m.frame)/2)%len(asciiSpinners)]
			} else {
				spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
				spinner = spinners[(int(m.frame)/2)%len(spinners)]
			}
		}
		label := "waking"
		if m.shuttingDown {
			label = "stopping"
		} else if m.opening && !m.waking {
			label = "connecting"
		}
		power = spinner + " " + label
		powerStyled = m.theme.accent().Render(power)
	}

	actionStyled := stateStyle(m.theme, actionState(action)).Render(m.theme.Glyph("diamond") + " " + action)

	headerLabel := "PATCHBAY · " + wake.state
	if m.theme.ASCII {
		headerLabel = "PATCHBAY / " + wake.state
	}
	if active {
		headerLabel = "locked · PATCHBAY"
		if m.theme.ASCII {
			headerLabel = "locked / PATCHBAY"
		}
	}

	heroLine := m.theme.title().Render(fitText(m.privateField(device.Name), rowWidth))
	if device.SiteID != "" {
		siteBadge := "[" + m.privateField(m.siteName(device.SiteID)) + "]"
		plainHero := m.privateField(device.Name)
		if visibleWidth(plainHero)+1+visibleWidth(siteBadge) <= rowWidth {
			heroLine = m.theme.title().Render(plainHero) + " " + m.theme.muted().Render(siteBadge)
		}
	}

	divider := strings.Repeat("─", min(rowWidth, max(12, rowWidth)))
	kvSep := " │ "
	if m.theme.ASCII {
		divider = strings.Repeat("-", min(rowWidth, max(12, rowWidth)))
		kvSep = " : "
	}

	lines := make([]string, 0, 16)
	lines = append(lines,
		m.theme.accent().Render(fitText(headerLabel, rowWidth)),
		heroLine,
		fitText(powerStyled+"   "+actionStyled, rowWidth),
		m.theme.muted().Render(fitText(m.privateMessage(wake.detail), rowWidth)),
	)

	if active {
		lines = append(lines,
			m.renderSignalPath(device, rowWidth),
			m.theme.muted().Render(fitText("esc cancel", rowWidth)),
		)
	} else {
		lines = append(lines, m.renderTopologyLine(device, rowWidth))
	}

	valWidth := max(1, rowWidth-8)
	ipText := m.privateField(device.IPAddress)
	if ipText == "" {
		ipText = "-"
	}
	macText := m.privateField(device.MACAddress)
	if macText == "" {
		macText = "-"
	}
	lines = append(lines,
		m.theme.cardLine().Render(divider),
		m.theme.muted().Render("IP   "+kvSep)+fitText(ipText, valWidth),
		m.theme.muted().Render("SEEN "+kvSep)+fitText(m.presenceDetail(device), valWidth),
		m.theme.muted().Render("MAC  "+kvSep)+fitText(macText, valWidth),
		m.theme.muted().Render("ROUTE"+kvSep)+fitText(m.privateField(m.routeText(device)), valWidth),
	)

	// Telemetry sparkline + recent per-machine activity when vertical space permits.
	if maxLines <= 0 || len(lines)+2 <= maxLines {
		series, sentCount, failCount, recent := m.deviceTelemetry(device)
		sparkW := min(10, max(4, valWidth-18))
		spark := renderSparkline(series, sparkW, m.theme.ASCII)
		sparkStyled := m.theme.emerald().Render(spark)
		if failCount > sentCount {
			sparkStyled = m.theme.danger().Render(spark)
		} else if devState != "ONLINE" && sentCount == 0 {
			sparkStyled = m.theme.cyan().Render(spark)
		}
		summary := fmt.Sprintf(" %d ok / %d err", sentCount, failCount)
		lines = append(lines,
			m.theme.cardLine().Render(divider),
			fitText(m.theme.muted().Render("PULSE"+kvSep)+sparkStyled+m.theme.muted().Render(summary), rowWidth),
		)
		for _, entry := range recent {
			if maxLines > 0 && len(lines) >= maxLines-1 {
				break
			}
			lines = append(lines, fitText(entry, rowWidth))
		}
	}

	if !active && (maxLines <= 0 || len(lines) < maxLines) {
		lines = append(lines, m.theme.muted().Render(fitText("enter  wake or stream", rowWidth)))
	}

	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func (m *WakeModel) renderTopologyLine(device store.Device, width int) string {
	nic := m.topologyNIC(device)
	hop := m.privateField(m.topologyHop(device))
	mac := m.privateField(device.MACAddress)
	if mac == "" {
		mac = "no-mac"
	}
	conn := " ──▸ "
	if m.theme.ASCII {
		conn = " :: "
	}
	plain := nic + conn + hop + conn + mac
	if visibleWidth(plain) > width {
		// Gracefully shorten to 2-hop when narrow while keeping NIC + hop visible.
		short := nic + conn + hop
		if visibleWidth(short) <= width {
			return m.theme.cyan().Render(nic) + m.theme.muted().Render(conn) + m.theme.amber().Render(hop)
		}
		return m.theme.cyan().Render(fitText(plain, width))
	}
	return m.theme.cyan().Render(nic) + m.theme.muted().Render(conn) + m.theme.indigo().Render(hop) + m.theme.muted().Render(conn) + m.theme.amber().Render(mac)
}

func (m *WakeModel) topologyNIC(device store.Device) string {
	iface := strings.TrimSpace(device.Interface)
	if iface == "" && device.SiteID != "" {
		for _, s := range m.sites {
			if s.ID == device.SiteID && strings.TrimSpace(s.DefaultInterface) != "" {
				iface = strings.TrimSpace(s.DefaultInterface)
				break
			}
		}
	}
	if iface == "" {
		return "local"
	}
	return m.privateField(iface)
}

func (m *WakeModel) topologyHop(device store.Device) string {
	relayID := strings.TrimSpace(device.WakeRelayID)
	if relayID == "" && device.SiteID != "" {
		for _, s := range m.sites {
			if s.ID == device.SiteID {
				relayID = strings.TrimSpace(s.WakeRelayID)
				break
			}
		}
	}
	if strings.EqualFold(device.WakeStrategy, "relay") || relayID != "" {
		for _, r := range m.relays {
			if r.ID == relayID {
				return "relay " + r.Name
			}
		}
		return "relay"
	}
	dest, port := m.routeTarget(device)
	return fmt.Sprintf("%s:%d", dest, port)
}

func (m *WakeModel) deviceTelemetry(device store.Device) (series []int, sentCount, failCount int, recent []string) {
	var matched []store.WakeAttempt
	for _, h := range m.history {
		if (device.ID != "" && h.TargetID == device.ID) || (device.Name != "" && strings.EqualFold(h.TargetName, device.Name)) {
			matched = append(matched, h)
			if len(matched) >= 8 {
				break
			}
		}
	}
	// History is newest-first; build chronological sparkline oldest-to-newest.
	series = make([]int, 0, len(matched)+1)
	for i := len(matched) - 1; i >= 0; i-- {
		status := strings.ToLower(strings.TrimSpace(matched[i].PacketStatus))
		switch status {
		case "sent", "ok", "online", "ready":
			series = append(series, 7)
		case "failed", "error", "blocked":
			series = append(series, 2)
		default:
			series = append(series, 4)
		}
	}
	switch m.deviceState(device) {
	case "ONLINE":
		series = append(series, 8)
	case "OFFLINE":
		series = append(series, 1)
	default:
		series = append(series, 3)
	}

	for i, h := range matched {
		status := strings.ToLower(strings.TrimSpace(h.PacketStatus))
		if status == "sent" || status == "ok" {
			sentCount++
		} else if status != "" {
			failCount++
		}
		if i < 2 {
			when := h.CreatedAt
			if parsed, err := time.Parse(time.RFC3339Nano, h.CreatedAt); err == nil {
				when = parsed.Local().Format("15:04:05")
			} else if len(when) > 8 {
				when = when[len(when)-8:]
			}
			if status == "" {
				status = "logged"
			}
			styledStatus := m.theme.muted().Render(status)
			if status == "sent" || status == "ok" {
				styledStatus = m.theme.success().Render(status)
			} else if status == "failed" || status == "error" {
				styledStatus = m.theme.danger().Render(status)
			}
			recent = append(recent, "  "+m.theme.muted().Render(when)+"  "+styledStatus)
		}
	}
	return series, sentCount, failCount, recent
}

func (m *WakeModel) renderActionPicker(devices []store.Device, width int) string {
	if len(devices) == 0 {
		return "no machine selected"
	}
	device := devices[min(m.selected, len(devices)-1)]
	remoteLabel := "remote"
	if profile, ok := m.profiles[device.ID]; ok && streamProfile(profile) {
		remoteLabel = "stream"
	} else if _, ok := m.profiles[device.ID]; !ok {
		remoteLabel = "stream"
	}
	items := []struct{ key, label string }{
		{"w", "wake"},
		{"c", remoteLabel},
		{"s", "check"},
		{"P", "power off"},
		{"esc", "cancel"},
	}
	rows := []string{m.theme.title().Render(fitText(m.privateField(device.Name), max(1, width))), ""}
	for i, item := range items {
		marker := " "
		keyBadge := m.theme.muted().Render(padVisible(item.key, 3))
		label := item.label
		if i == m.pickerSelected {
			marker = m.theme.accent().Render(m.theme.Glyph("arrow"))
			keyBadge = m.theme.keycap().Render(padVisible(item.key, 3))
			label = m.theme.accent().Render(label)
		}
		rows = append(rows, fitText(fmt.Sprintf("%s %s  %s", marker, keyBadge, label), max(1, width)))
	}
	return strings.Join(rows, "\n")
}

func (m *WakeModel) renderSignalPath(device store.Device, width int) string {
	if m.action == "shutdown-wait" {
		return m.renderActionPath([]string{"DESK", "SSH", strings.ToUpper(fitText(m.privateField(device.Name), 12))}, width)
	}
	if m.action == "wake-remote" {
		last := "REMOTE"
		if profile, ok := m.profiles[device.ID]; ok && streamProfile(profile) {
			last = "STREAM"
		}
		return m.renderActionPath([]string{"WAKE", "WAIT", last}, width)
	}
	route := "LAN"
	if strings.EqualFold(device.WakeStrategy, "relay") || device.WakeRelayID != "" {
		route = "RELAY"
	}
	return m.renderActionPath([]string{"DESK", route, strings.ToUpper(fitText(m.privateField(device.Name), 12))}, width)
}

func (m *WakeModel) pathPositionAt(now time.Time) int {
	if !m.motion.Enabled {
		return 0
	}
	if m.action == "wake-wait" || m.action == "wake-remote" || m.action == "shutdown-wait" {
		if !m.motion.Active(now) {
			return 2
		}
		cycleDuration := 900 * time.Millisecond
		stepDuration := cycleDuration / 3
		elapsed := now.Sub(m.motion.Started)
		if elapsed < 0 {
			elapsed = 0
		}
		return int((elapsed % cycleDuration) / stepDuration)
	}
	if m.waking || m.opening || m.shuttingDown {
		if !m.motion.Active(now) {
			return 2
		}
		t := m.motion.T(now)
		if t < 1.0/3 {
			return 0
		}
		if t < 2.0/3 {
			return 1
		}
		return 2
	}
	if m.motion.Active(now) {
		t := m.motion.T(now)
		if t < 1.0/3 {
			return 0
		}
		if t < 2.0/3 {
			return 1
		}
		return 2
	}
	return 0
}

func signalPath(steps []string, position int, ascii bool) string {
	connector, pulse := "──", "●"
	if ascii {
		connector, pulse = "--", "*"
	}
	left, right := connector+">", connector+">"
	name := steps[2]
	switch position {
	case 1:
		right = pulse + connector + ">"
	case 2:
		name = pulse + name
	default:
		left = pulse + connector + ">"
	}
	return steps[0] + " " + left + " " + steps[1] + " " + right + " " + name
}

func (m *WakeModel) renderActionPath(steps []string, width int) string {
	path := signalPath(steps, m.pathPositionAt(time.Now()), m.theme.ASCII)
	return fitText(m.theme.accent().Render(path), width)
}
