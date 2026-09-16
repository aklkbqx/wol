package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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
			if theme.ASCII {
				parts = append(parts, "["+label+"]")
			} else {
				parts = append(parts, theme.accent().Render("◆ "+label))
			}
			continue
		}
		if theme.ASCII {
			parts = append(parts, " "+label+" ")
		} else {
			parts = append(parts, theme.muted().Render("  "+label))
		}
	}
	separator := "    "
	if !theme.ASCII && theme.Colors && width >= 48 {
		separator = theme.cardLine().Render("   │   ")
	} else if width < 48 {
		separator = "  "
	}
	return fitText(strings.Join(parts, separator), max(1, width))
}

// renderCardBox creates a framed box container with a title on the top border
func renderCardBox(theme Theme, title string, body string, width, minHeight int) string {
	if width < 14 {
		return fitText(body, width)
	}
	innerWidth := max(1, width-4)
	tl, tr, bl, br, hz, vt, vtR := "╭─ ", "─╮", "╰─", "─╯", "─", "│ ", " │"
	if theme.ASCII {
		tl, tr, bl, br, hz, vt, vtR = "+- ", "-+", "+-", "-+", "-", "| ", " |"
	}
	titleRendered := ""
	if title != "" {
		if theme.Colors && !theme.ASCII {
			titleRendered = theme.accent().Render(title)
		} else {
			titleRendered = title
		}
	}
	rawTitleWidth := lipgloss.Width(stripANSI(titleRendered))
	topFillWidth := max(0, width-6-rawTitleWidth)
	topLine := theme.cardLine().Render(tl) + titleRendered + theme.cardLine().Render(" "+strings.Repeat(hz, topFillWidth)+tr)
	topLine = fitText(topLine, width)

	bodyLines := strings.Split(body, "\n")
	contentCount := len(bodyLines)
	if minHeight > 0 && contentCount < minHeight {
		contentCount = minHeight
	}
	lines := make([]string, 0, contentCount+2)
	lines = append(lines, topLine)
	for i := 0; i < contentCount; i++ {
		line := ""
		if i < len(bodyLines) {
			line = bodyLines[i]
		}
		rawLineWidth := lipgloss.Width(stripANSI(line))
		fill := max(0, innerWidth-rawLineWidth)
		midLine := theme.cardLine().Render(vt) + line + strings.Repeat(" ", fill) + theme.cardLine().Render(vtR)
		lines = append(lines, fitText(midLine, width))
	}
	bottomLine := theme.cardLine().Render(bl + strings.Repeat(hz, max(0, width-4)) + br)
	lines = append(lines, fitText(bottomLine, width))
	return strings.Join(lines, "\n")
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
