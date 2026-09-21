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
	rows := []string{m.theme.muted().Render(fitText(fmt.Sprintf("%d routes", len(m.relays)), width))}
	if len(m.relays) == 0 {
		rows = append(rows, fitText("no routes. press a to add a relay.", width))
	} else {
		start, end := m.listWindow(len(m.relays))
		for i := start; i < end; i++ {
			relay := m.relays[i]
			marker := " "
			name := fitText(relay.Name, max(4, min(18, width/3)))
			if i == m.selected {
				marker = m.theme.accent().Render(m.theme.Glyph("arrow"))
				name = m.theme.accent().Render(name)
			}
			state := "ready"
			if !relay.Enabled {
				state = "off"
			}
			rows = append(rows, fitText(fmt.Sprintf("%s %s  %s  %s:%d", marker, name, state, fitText(relay.Address, max(4, min(16, width/3))), relay.Port), width))
		}
	}
	return strings.Join(rows, "\n")
}

func (m *WakeModel) renderActivity(width int) string {
	rows := []string{m.theme.muted().Render(fitText(fmt.Sprintf("%d recent", len(m.history)), width))}
	if len(m.history) == 0 {
		rows = append(rows, fitText("no wake activity yet", width))
	} else {
		start, end := m.listWindow(len(m.history))
		for i := start; i < end; i++ {
			attempt := m.history[i]
			marker := " "
			target := fitText(attempt.TargetName, max(4, min(16, width/3)))
			if i == m.selected {
				marker = m.theme.accent().Render(m.theme.Glyph("arrow"))
				target = m.theme.accent().Render(target)
			}
			when := attempt.CreatedAt
			if parsed, err := time.Parse(time.RFC3339Nano, attempt.CreatedAt); err == nil {
				when = parsed.Local().Format("15:04:05")
			}
			rows = append(rows, fitText(fmt.Sprintf("%s %s  %s  %s", marker, when, target, strings.ToLower(attempt.PacketStatus)), width))
		}
	}
	return strings.Join(rows, "\n")
}

func (m *WakeModel) footer(width int) string {
	if m.theme.ASCII || width < 48 {
		return fitText(m.theme.muted().Render("enter choose   space select   / search   ? help   q quit"), width)
	}
	shortcuts := []struct{ key, label string }{
		{"enter", "choose"},
		{"space", "select"},
		{"/", "search"},
		{"?", "help"},
		{"q", "quit"},
	}
	parts := make([]string, 0, len(shortcuts))
	for _, s := range shortcuts {
		k := m.theme.accent().Render(s.key)
		l := m.theme.muted().Render(s.label)
		parts = append(parts, k+" "+l)
	}
	return fitText(strings.Join(parts, "   "), width)
}
