package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m *WakeModel) freshnessText() string {
	if m.checkedAt.IsZero() {
		return "not checked yet"
	}
	if m.stale {
		return "STALE · last checked " + m.checkedAt.Format("15:04:05")
	}
	return "checked " + m.checkedAt.Format("15:04:05")
}

func (m *WakeModel) renderRoutes(width int) string {
	budget := m.bodyLineBudget()
	framed := width >= 44 && budget >= 7
	innerW := width
	maxRows := budget
	if framed {
		innerW = width - 4
		maxRows = max(3, budget-2)
	}

	rows := make([]string, 0, maxRows)
	rows = append(rows, m.theme.muted().Render(fitText(fmt.Sprintf("%d relay routes   a add   e edit   d delete", len(m.relays)), innerW)))
	if len(m.relays) == 0 {
		rows = append(rows, fitText("no routes. press a to add a relay.", innerW))
	} else {
		limit := max(1, maxRows-1)
		start := max(0, min(m.selected-limit/2, max(0, len(m.relays)-limit)))
		end := min(len(m.relays), start+limit)
		for i := start; i < end; i++ {
			relay := m.relays[i]
			marker := " "
			nameW := max(6, min(20, innerW/3))
			name := padVisible(fitText(m.privateField(relay.Name), nameW), nameW)
			if i == m.selected {
				marker = m.theme.accent().Render(m.theme.Glyph("arrow"))
				name = m.theme.accent().Render(name)
			}
			state := "ready"
			stateStyled := m.theme.success().Render(m.theme.Glyph("signal-ready") + " ready")
			if !relay.Enabled {
				state = "off"
				stateStyled = m.theme.muted().Render(m.theme.Glyph("signal-stopped") + " off  ")
			}
			endpoint := fmt.Sprintf("%s:%d", fitText(m.privateField(relay.Address), max(4, min(20, innerW/3))), relay.Port)
			if innerW < 36 {
				rows = append(rows, fitText(fmt.Sprintf("%s %s  %s  %s", marker, name, state, endpoint), innerW))
			} else {
				rows = append(rows, fitText(fmt.Sprintf("%s %s  %s  %s", marker, name, stateStyled, endpoint), innerW))
			}
		}
	}
	body := strings.Join(rows, "\n")
	if framed {
		meta := fmt.Sprintf("%d routes", len(m.relays))
		return renderShearedBox(m.theme, m.theme.Glyph("diamond")+" RELAY ROUTES", meta, body, width, 0, true)
	}
	return body
}

func (m *WakeModel) renderActivity(width int) string {
	budget := m.bodyLineBudget()
	framed := width >= 44 && budget >= 7
	innerW := width
	maxRows := budget
	if framed {
		innerW = width - 4
		maxRows = max(3, budget-2)
	}

	rows := make([]string, 0, maxRows)
	rows = append(rows, m.theme.muted().Render(fitText(fmt.Sprintf("%d recent wake events", len(m.history)), innerW)))
	if len(m.history) == 0 {
		rows = append(rows, fitText("no wake activity yet", innerW))
	} else {
		limit := max(1, maxRows-1)
		start := max(0, min(m.selected-limit/2, max(0, len(m.history)-limit)))
		end := min(len(m.history), start+limit)
		for i := start; i < end; i++ {
			attempt := m.history[i]
			marker := " "
			targetW := max(6, min(20, innerW/3))
			target := padVisible(fitText(m.privateField(attempt.TargetName), targetW), targetW)
			if i == m.selected {
				marker = m.theme.accent().Render(m.theme.Glyph("arrow"))
				target = m.theme.accent().Render(target)
			}
			when := attempt.CreatedAt
			if parsed, err := time.Parse(time.RFC3339Nano, attempt.CreatedAt); err == nil {
				when = parsed.Local().Format("15:04:05")
			}
			status := strings.ToLower(strings.TrimSpace(attempt.PacketStatus))
			statusStyled := m.theme.muted().Render(status)
			if status == "sent" || status == "ok" {
				statusStyled = m.theme.success().Render(status)
			} else if status == "failed" || status == "error" {
				statusStyled = m.theme.danger().Render(status)
			}
			rows = append(rows, fitText(fmt.Sprintf("%s %s  %s  %s", marker, m.theme.muted().Render(when), target, statusStyled), innerW))
		}
	}
	body := strings.Join(rows, "\n")
	if framed {
		meta := fmt.Sprintf("%d events", len(m.history))
		return renderShearedBox(m.theme, m.theme.Glyph("diamond")+" WAKE ACTIVITY", meta, body, width, 0, true)
	}
	return body
}

func (m *WakeModel) footer(width int) string {
	if m.theme.ASCII || width < 48 {
		if width >= 72 {
			return fitText(m.theme.muted().Render("enter choose   w wake   c stream   space select   / search   t theme   ? help   q quit"), width)
		}
		return fitText(m.theme.muted().Render("enter choose   space select   / search   ? help   q quit"), width)
	}
	shortcuts := []struct{ key, label string }{
		{"enter", "choose"},
		{"space", "select"},
		{"/", "search"},
		{"?", "help"},
		{"q", "quit"},
	}
	if width >= 88 {
		shortcuts = []struct{ key, label string }{
			{"enter", "choose"},
			{"w", "wake"},
			{"c", "stream"},
			{"space", "select"},
			{"/", "search"},
			{"t", "theme"},
			{"?", "help"},
			{"q", "quit"},
		}
	} else if width >= 66 {
		shortcuts = []struct{ key, label string }{
			{"enter", "choose"},
			{"space", "select"},
			{"/", "search"},
			{"t", "theme"},
			{"?", "help"},
			{"q", "quit"},
		}
	}
	parts := make([]string, 0, len(shortcuts))
	for _, s := range shortcuts {
		k := m.theme.keycap().Render(s.key)
		l := m.theme.muted().Render(s.label)
		parts = append(parts, k+" "+l)
	}
	left := strings.Join(parts, "   ")
	if width >= 108 {
		presetTag := m.theme.muted().Render("◐ " + m.theme.PresetName())
		leftW := visibleWidth(stripANSI(left))
		tagW := visibleWidth(stripANSI(presetTag))
		if leftW+3+tagW <= width {
			return left + strings.Repeat(" ", width-leftW-tagW) + presetTag
		}
	}
	return fitText(left, width)
}
