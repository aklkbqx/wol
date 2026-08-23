package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderPanel(theme Theme, title, subtitle, body string, width int) string {
	inner := clampWidth(width-2, 18, 0)
	header := theme.title().Render(title)
	if subtitle != "" {
		header += "  " + theme.muted().Render(subtitle)
	}
	content := header
	if body != "" {
		content += "\n\n" + body
	}
	style := theme.panel(inner)
	if !theme.Colors {
		style = lipgloss.NewStyle().Padding(0, 1).Width(inner)
	}
	return style.Render(content)
}

func renderTabs(theme Theme, active int, labels []string, width int) string {
	parts := make([]string, 0, len(labels))
	for i, label := range labels {
		prefix := fmt.Sprintf("%d ", i+1)
		if i == active {
			style := theme.accent()
			if theme.Colors {
				style = style.Underline(true)
			}
			parts = append(parts, style.Render(prefix+label))
		} else {
			parts = append(parts, theme.muted().Render(prefix+label))
		}
	}
	result := strings.Join(parts, "   ")
	return fitText(result, max(1, width))
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
