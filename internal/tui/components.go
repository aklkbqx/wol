package tui

import (
	"strings"
)

func renderPanel(theme Theme, title, subtitle, body string, width int) string {
	inner := clampWidth(width, 8, 0)
	header := theme.title().Render(title)
	if subtitle != "" {
		header += "  " + theme.muted().Render(subtitle)
	}
	content := header
	if body != "" {
		content += "\n\n" + body
	}
	return theme.panel(inner).Render(content)
}

func renderTabs(theme Theme, active int, labels []string, width int) string {
	parts := make([]string, 0, len(labels))
	for i, label := range labels {
		if i == active {
			parts = append(parts, theme.accent().Render(label))
			continue
		}
		parts = append(parts, theme.muted().Render(label))
	}
	return fitText(strings.Join(parts, "   "), max(1, width))
}

func renderNotice(theme Theme, message string, width int) string {
	if message == "" {
		return ""
	}
	return theme.accent().Render(fitText(message, width))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
