package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/aklkbqx/wol/internal/store"
)

func (m *WakeModel) bodyLineBudget() int {
	if m.height <= 0 {
		return 24
	}
	inner := contentWidth(m.width)
	chrome := 1 // top header ribbon
	showTabs := inner >= 36 && (m.height >= 20 || m.tab != 0 || m.form != nil)
	if showTabs {
		chrome++
	}
	if m.height >= 30 {
		chrome++ // breathing spacer above main deck on tall terminals
	}
	showStatus := m.height >= 22 || m.loading || m.waking || m.opening || m.shuttingDown || m.checking || statusNeedsAttention(m.status)
	if m.status != "" && showStatus {
		chrome++
	}
	if m.filtering {
		chrome++
	}
	if m.confirm != "" {
		chrome += 4
	}
	if m.showHelp {
		chrome += 19
	}
	if inner >= 48 && m.height >= 22 {
		chrome += 2 // divider + keycap footer
	} else {
		chrome += 2 // blank + compact footer
	}
	return max(4, m.height-chrome)
}

func (m *WakeModel) renderMachines(width int, mode LayoutMode) string {
	devices := m.filteredDevices()
	budget := m.bodyLineBudget()
	if m.actionPicker {
		if width >= 44 && budget >= 9 {
			return renderShearedBox(m.theme, m.theme.Glyph("diamond")+" ACTION PICKER", "enter confirm", m.renderActionPicker(devices, width-4), width, 0, true)
		}
		return m.renderActionPicker(devices, width)
	}

	showSplit := width >= 76 && m.height >= 20 && budget >= 8
	if showSplit {
		leftWidth := (width * 60) / 100
		if leftWidth < 38 {
			leftWidth = 38
		}
		rightWidth := width - leftWidth - 1
		if rightWidth < 34 {
			rightWidth = 34
			leftWidth = max(36, width-rightWidth-1)
			rightWidth = width - leftWidth - 1
		}
		cardBudget := max(6, budget-2)
		leftContent := m.renderMachineListBounded(devices, leftWidth-4, cardBudget)
		rightContent := m.renderInspectorBounded(devices, rightWidth-4, cardBudget)
		leftLines := strings.Count(leftContent, "\n") + 1
		rightLines := strings.Count(rightContent, "\n") + 1
		sharedHeight := min(cardBudget, max(leftLines, rightLines))

		online, offline, _ := m.machineSummary(devices)
		fleetMeta := fmt.Sprintf("%d hosts / %d up / %d down", len(devices), online, offline)
		inspTitle := m.theme.Glyph("diamond") + " PATCHBAY INSPECTOR"
		inspMeta := "idle"
		active := m.waking || m.opening || m.shuttingDown
		if active {
			inspMeta = "locked"
		} else if sel, ok := m.selectedDevice(devices); ok {
			inspMeta = powerWord(m.deviceState(sel))
		}

		leftCard := renderShearedBox(m.theme, m.theme.Glyph("diamond")+" FLEET LEDGER", fleetMeta, leftContent, leftWidth, sharedHeight, !active)
		rightCard := renderShearedBox(m.theme, inspTitle, inspMeta, rightContent, rightWidth, sharedHeight, active)
		return lipgloss.JoinHorizontal(lipgloss.Top, leftCard, " ", rightCard)
	}

	listBudget := budget
	hasActiveSignal := false
	if _, ok := m.actionDevice(); ok && (m.waking || m.opening || m.shuttingDown) {
		hasActiveSignal = true
		listBudget = max(4, budget-1)
	}
	list := m.renderMachineListBounded(devices, width, listBudget)
	if hasActiveSignal {
		if target, ok := m.actionDevice(); ok {
			tName := m.privateField(target.Name)
			sig := m.renderSignalPath(target, max(1, width-visibleWidth(tName)-2))
			return list + "\n" + m.theme.accent().Render(fitText(tName+"  "+sig, width))
		}
	}
	if mode == LayoutWide && budget >= 14 {
		return list + "\n" + m.renderInspectorBounded(devices, width, max(5, budget-strings.Count(list, "\n")-2))
	}
	return list
}

func (m *WakeModel) renderMachineList(devices []store.Device, width int) string {
	return m.renderMachineListBounded(devices, width, max(6, m.bodyLineBudget()-2))
}

func (m *WakeModel) renderMachineListBounded(devices []store.Device, width int, maxLines int) string {
	rowWidth := max(1, width)
	online, offline, unknown := m.machineSummary(devices)

	overhead := 2 // summary + blank/header line
	showColHeader := width >= 44 && maxLines >= 7 && len(devices) > 0
	if !showColHeader && maxLines < 6 {
		overhead = 1
	}
	hasBatchDock := len(m.marked) > 0 && maxLines >= 6
	if hasBatchDock {
		overhead++
	}
	showCompactFooter := false
	showSplit := m.width >= 76 && m.height >= 20 && m.bodyLineBudget() >= 8
	if _, ok := m.selectedDevice(devices); ok && width >= 40 && !showSplit && maxLines >= 7 {
		showCompactFooter = true
		overhead += 2
	}

	visible, start := m.machineViewportBounded(devices, width, max(2, maxLines-overhead))
	rows := make([]string, 0, len(visible)+6)
	rows = append(rows,
		m.theme.muted().Render(fitText(fmt.Sprintf("%d machines  %d online  %d unreachable  %d unknown", len(devices), online, offline, unknown), rowWidth)),
	)
	if len(devices) == 0 {
		if maxLines >= 3 {
			rows = append(rows, "")
		}
		rows = append(rows, fitText("No machines. n discover  a add  i import", rowWidth))
		return strings.Join(rows, "\n")
	}

	nameWidth := 16
	if width >= 54 {
		nameWidth = 20
	}
	if width < 28 {
		nameWidth = max(8, width-10)
	}

	if showColHeader {
		if width >= 44 {
			hdr := "  " + padVisible("HOST", nameWidth) + "  MARK  " + padVisible("STATE", 13) + "  ACTION"
			if width >= 62 {
				hdr += "     SITE"
			}
			rows = append(rows, m.theme.muted().Render(fitText(hdr, rowWidth)))
		} else {
			rows = append(rows, "")
		}
	} else if maxLines >= 6 {
		rows = append(rows, "")
	}

	if start > 0 {
		rows = append(rows, m.theme.muted().Render(fitText(fmt.Sprintf("↑ %d above", start), rowWidth)))
	}

	now := time.Now()
	motionSelectActive := m.motion.Active(now) && m.motion.Stage == StageSelect
	arrowStyled := m.theme.accent().Render(m.theme.Glyph("arrow"))
	afterimageStyled := m.theme.accent().Render(m.theme.Glyph("afterimage"))
	unmarkedStyled := m.theme.muted().Render("[ ]")
	markedGlyph := "[✓]"
	if m.theme.ASCII {
		markedGlyph = "[x]"
	}
	markedStyled := m.theme.amber().Render(markedGlyph)
	markedShortStyled := m.theme.amber().Render("*")

	onlineStyled := stateStyle(m.theme, "ONLINE").Render(stateGlyph(m.theme, "ONLINE") + " " + padVisible("online", 11))
	offlineStyled := stateStyle(m.theme, "OFFLINE").Render(stateGlyph(m.theme, "OFFLINE") + " " + padVisible("unreachable", 11))
	unknownStyled := stateStyle(m.theme, "UNKNOWN").Render(stateGlyph(m.theme, "UNKNOWN") + " " + padVisible("unknown", 11))
	setupStyled := stateStyle(m.theme, "UNKNOWN").Render(padVisible("setup", 7))
	blockedStyled := stateStyle(m.theme, "BLOCKED").Render(padVisible("blocked", 7))
	streamStyled := stateStyle(m.theme, "ONLINE").Render(padVisible("stream", 7))
	remoteStyled := stateStyle(m.theme, "ONLINE").Render(padVisible("remote", 7))

	var siteLabels map[string]string
	if width >= 62 && len(m.sites) > 0 {
		siteLabels = make(map[string]string, len(m.sites))
		maxSiteW := max(4, width-54)
		for _, s := range m.sites {
			siteLabels[s.ID] = m.theme.muted().Render(fitText(m.privateField(s.Name), maxSiteW))
		}
	}

	for i, device := range visible {
		globalIndex := start + i
		highlighted := globalIndex == m.selected
		if (m.waking || m.opening || m.shuttingDown) && m.actionTargetID != "" {
			highlighted = device.ID == m.actionTargetID
		}
		marker := " "
		if motionSelectActive && globalIndex == m.motion.Origin {
			marker = afterimageStyled
		}
		if highlighted {
			marker = arrowStyled
		}

		isMarked := m.marked[device.ID]
		devState := m.deviceState(device)
		power := powerWord(devState)
		action := m.actionWord(device)
		name := padVisible(fitText(m.privateField(device.Name), nameWidth), nameWidth)
		if highlighted {
			name = m.theme.accent().Render(name)
		}

		var powerStyled string
		switch devState {
		case "ONLINE":
			powerStyled = onlineStyled
		case "OFFLINE":
			powerStyled = offlineStyled
		case "UNKNOWN":
			powerStyled = unknownStyled
		default:
			powerStyled = stateStyle(m.theme, devState).Render(stateGlyph(m.theme, devState) + " " + padVisible(power, 11))
		}

		var actionStyled string
		switch action {
		case "setup":
			actionStyled = setupStyled
		case "blocked":
			actionStyled = blockedStyled
		case "stream":
			actionStyled = streamStyled
		case "remote":
			actionStyled = remoteStyled
		default:
			actionStyled = stateStyle(m.theme, actionState(action)).Render(padVisible(action, 7))
		}

		if (m.waking || m.opening || m.shuttingDown) && device.ID == m.actionTargetID {
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
				label = "stop"
			} else if m.opening && !m.waking {
				label = "remote"
			}
			power = spinner + " " + label
			powerStyled = m.theme.accent().Render(padVisible(power, 13))
		}

		if width < 36 {
			prefix := marker
			if isMarked && !highlighted {
				prefix = "*"
			}
			rows = append(rows, fitText(prefix+" "+fitText(m.privateField(device.Name), max(1, rowWidth-2)), rowWidth))
			rows = append(rows, fitText("  "+power+"  "+action, rowWidth))
		} else if width < 44 {
			markShort := " "
			if isMarked {
				markShort = markedShortStyled
			}
			rows = append(rows, fitText(marker+" "+name+" "+markShort+" "+powerStyled+"  "+actionStyled, rowWidth))
		} else {
			markCell := unmarkedStyled
			if isMarked {
				markCell = markedStyled
			}
			line := marker + " " + name + "  " + markCell + "  " + powerStyled + "  " + actionStyled
			if width >= 62 && device.SiteID != "" {
				if pre, ok := siteLabels[device.SiteID]; ok {
					line += "  " + pre
				} else {
					siteLabel := m.privateField(m.siteName(device.SiteID))
					line += "  " + m.theme.muted().Render(fitText(siteLabel, max(4, width-54)))
				}
			}
			rows = append(rows, fitText(line, rowWidth))
		}
	}

	if below := len(devices) - (start + len(visible)); below > 0 {
		rows = append(rows, m.theme.muted().Render(fitText(fmt.Sprintf("↓ %d below", below), rowWidth)))
	}

	if hasBatchDock {
		badge := "[✓]"
		sep := " │ "
		if m.theme.ASCII {
			badge = "[x]"
			sep = " | "
		}
		dock := fmt.Sprintf("%s %d marked%sW batch wake%sS batch check%sEsc clear", badge, len(m.marked), sep, sep, sep)
		rows = append(rows, m.theme.amber().Render(fitText(dock, rowWidth)))
	}

	if showCompactFooter {
		if selected, ok := m.selectedDevice(devices); ok {
			rows = append(rows, "", m.theme.muted().Render(fitText(m.privateField(selected.IPAddress)+"  "+m.privateField(selected.MACAddress)+"  "+m.privateField(m.routeText(selected)), rowWidth)))
		}
	}

	if maxLines > 0 && len(rows) > maxLines {
		rows = rows[:maxLines]
	}
	return strings.Join(rows, "\n")
}

func stateGlyph(theme Theme, state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "ONLINE":
		return theme.Glyph("signal-ready")
	case "OFFLINE":
		return theme.Glyph("signal-stopped")
	case "DISABLED", "BLOCKED", "FAILED":
		return theme.Glyph("signal-failed")
	default:
		return theme.Glyph("signal-unknown")
	}
}

func (m *WakeModel) machineViewport(devices []store.Device, width int) ([]store.Device, int) {
	return m.machineViewportBounded(devices, width, max(2, m.bodyLineBudget()-4))
}

func (m *WakeModel) machineViewportBounded(devices []store.Device, width int, rowBudget int) ([]store.Device, int) {
	if len(devices) == 0 || m.height <= 0 {
		return devices, 0
	}
	baseRows := 1
	if width < 36 {
		baseRows = 2
	}
	capByHeight := max(baseRows+1, m.height-12)
	budget := max(baseRows, min(rowBudget, capByHeight))
	if len(devices)*baseRows <= budget {
		return devices, 0
	}
	// Reserve up to 2 lines for above/below scroll indicators when paging.
	pagedBudget := max(baseRows, budget-2)
	count := max(1, pagedBudget/baseRows)
	if count >= len(devices) {
		return devices, 0
	}
	start := m.selected - count/2
	start = max(0, min(start, len(devices)-count))
	return devices[start : start+count], start
}

func (m *WakeModel) selectedDevice(devices []store.Device) (store.Device, bool) {
	if len(devices) == 0 {
		return store.Device{}, false
	}
	if target, ok := m.actionDevice(); ok && (m.waking || m.opening || m.shuttingDown) {
		return target, true
	}
	return devices[min(m.selected, len(devices)-1)], true
}
