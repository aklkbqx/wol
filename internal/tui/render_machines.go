package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/store"
	"github.com/charmbracelet/lipgloss"
)

func (m *WakeModel) renderMachines(width int, mode LayoutMode) string {
	devices := m.filteredDevices()
	if m.actionPicker {
		return m.renderActionPicker(devices, width)
	}
	showSplit := width >= 76 && m.height >= 18
	if showSplit {
		leftWidth := (width * 55) / 100
		if leftWidth < 36 {
			leftWidth = 36
		}
		rightWidth := width - leftWidth - 1
		if rightWidth < 26 {
			rightWidth = 26
			leftWidth = width - rightWidth - 1
		}
		leftContent := m.renderMachineList(devices, leftWidth-4)
		rightContent := m.renderInspector(devices, rightWidth-4)
		leftLines := len(strings.Split(leftContent, "\n"))
		rightLines := len(strings.Split(rightContent, "\n"))
		sharedHeight := max(leftLines, rightLines)
		if m.height >= 26 {
			sharedHeight = max(sharedHeight, min(12, m.height-14))
		}
		leftCard := renderCardBox(m.theme, "◆ FLEET", leftContent, leftWidth, sharedHeight)
		rightCard := renderCardBox(m.theme, "◆ INSPECTOR", rightContent, rightWidth, sharedHeight)
		return lipgloss.JoinHorizontal(lipgloss.Top, leftCard, " ", rightCard)
	}
	list := m.renderMachineList(devices, width)
	if target, ok := m.actionDevice(); ok && (m.waking || m.opening) {
		return list + "\n" + m.theme.accent().Render(fitText(target.Name+"  "+m.renderSignalPath(target, max(1, width-len(target.Name)-4)), width))
	}
	if mode == LayoutWide {
		return list + "\n" + m.renderInspector(devices, width)
	}
	return list
}

func joinPanels(left, right, sep string, leftWidth int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	maxLines := max(len(leftLines), len(rightLines))
	out := make([]string, maxLines)
	for i := 0; i < maxLines; i++ {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		pad := max(0, leftWidth-lipgloss.Width(stripANSI(l)))
		out[i] = l + strings.Repeat(" ", pad) + sep + r
	}
	return strings.Join(out, "\n")
}

func (m *WakeModel) renderMachineList(devices []store.Device, width int) string {
	rowWidth := max(1, width)
	online, offline, unknown, _, _, _, _ := m.machineSummary(devices)
	rows := []string{
		m.theme.muted().Render(fitText(fmt.Sprintf("%d machines  %d online  %d asleep  %d unknown", len(devices), online, offline, unknown), rowWidth)),
		"",
	}
	if len(devices) == 0 {
		rows = append(rows, "no machines. press a to add one.")
		return strings.Join(rows, "\n")
	}
	visible, start := m.machineViewport(devices, width)
	if start > 0 {
		rows = append(rows, m.theme.muted().Render(fmt.Sprintf("%d above", start)))
	}
	nameWidth := 16
	if width >= 48 {
		nameWidth = 20
	}
	if width < 28 {
		nameWidth = max(8, width-10)
	}
	for i, device := range visible {
		globalIndex := start + i
		highlighted := globalIndex == m.selected
		if (m.waking || m.opening) && m.actionTargetID != "" {
			highlighted = device.ID == m.actionTargetID
		}
		marker := " "
		now := time.Now()
		if m.motion.Active(now) && m.motion.Stage == StageSelect && globalIndex == m.motion.Origin {
			marker = m.theme.accent().Render(m.theme.Glyph("afterimage"))
		}
		if highlighted {
			marker = m.theme.accent().Render(m.theme.Glyph("arrow"))
		}
		power := powerWord(m.deviceState(device))
		action := m.actionWord(device)
		name := padVisible(fitText(device.Name, nameWidth), nameWidth)
		if highlighted {
			name = m.theme.accent().Render(name)
		}
		powerStyled := stateStyle(m.theme, m.deviceState(device)).Render(padVisible(power, 8))
		actionStyled := stateStyle(m.theme, actionState(action)).Render(action)
		if width < 36 {
			rows = append(rows, fitText(marker+" "+fitText(device.Name, max(1, rowWidth-2)), rowWidth))
			rows = append(rows, fitText("  "+power+"  "+action, rowWidth))
		} else {
			rows = append(rows, fitText(marker+" "+name+"  "+powerStyled+"  "+actionStyled, rowWidth))
		}
	}
	if below := len(devices) - (start + len(visible)); below > 0 {
		rows = append(rows, m.theme.muted().Render(fmt.Sprintf("%d below", below)))
	}
	showSplit := m.width >= 76 && m.height >= 18
	if selected, ok := m.selectedDevice(devices); ok && width >= 40 && !showSplit {
		rows = append(rows, "", m.theme.muted().Render(fitText(selected.IPAddress+"  "+selected.MACAddress+"  "+m.routeText(selected), rowWidth)))
	}
	return strings.Join(rows, "\n")
}

func (m *WakeModel) machineViewport(devices []store.Device, width int) ([]store.Device, int) {
	if len(devices) == 0 || m.height <= 0 {
		return devices, 0
	}
	baseRows := 1
	if width < 36 {
		baseRows = 2
	}
	budget := m.height - 10
	if m.showHelp {
		budget -= 8
	}
	budget = max(baseRows+1, budget)
	count := len(devices)
	for count > 1 {
		if count*baseRows+2 <= budget {
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

func (m *WakeModel) selectedDevice(devices []store.Device) (store.Device, bool) {
	if len(devices) == 0 {
		return store.Device{}, false
	}
	if target, ok := m.actionDevice(); ok && (m.waking || m.opening) {
		return target, true
	}
	return devices[min(m.selected, len(devices)-1)], true
}
