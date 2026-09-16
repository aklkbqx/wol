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
		for i, relay := range m.relays {
			marker := " "
			if i == m.selected {
				marker = m.theme.Glyph("arrow")
			}
			state := "ready"
			if !relay.Enabled {
				state = "off"
			}
			rows = append(rows, fitText(fmt.Sprintf("%s %s  %s  %s:%d", marker, fitText(relay.Name, max(4, min(18, width/3))), state, fitText(relay.Address, max(4, min(16, width/3))), relay.Port), width))
		}
	}
	return strings.Join(rows, "\n")
}

func (m *WakeModel) renderActivity(width int) string {
	rows := []string{m.theme.muted().Render(fitText(fmt.Sprintf("%d recent", len(m.history)), width))}
	if len(m.history) == 0 {
		rows = append(rows, fitText("no wake activity yet", width))
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
			rows = append(rows, fitText(fmt.Sprintf("%s %s  %s  %s", marker, when, fitText(attempt.TargetName, max(4, min(16, width/3))), strings.ToLower(attempt.PacketStatus)), width))
		}
	}
	return strings.Join(rows, "\n")
}

func (m *WakeModel) footer(width int) string {
	return fitText(m.theme.muted().Render("enter choose   w wake   c stream   s check   x stop   ? help   q quit"), width)
}
