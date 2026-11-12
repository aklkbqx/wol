package tui

import (
	"fmt"
	"strings"
)

var (
	unicodeSparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	asciiSparkBlocks   = []byte{'.', '_', '-', '=', '+', '*', '#', '%'}
)

func renderPanel(theme Theme, title, subtitle, body string, width int) string {
	if width >= 40 {
		return renderShearedBox(theme, title, subtitle, body, width, 0, true)
	}
	inner := clampWidth(width, 8, 0)
	header := theme.title().Render(title)
	if subtitle != "" {
		header += "  " + theme.muted().Render(subtitle)
	}
	content := header
	if body != "" {
		content += "\n" + body
	}
	return fitLines(theme.panel(inner).Render(content), width)
}

func renderTabRibbon(theme Theme, active int, labels []string, rightMeta string, width int) string {
	if width <= 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	numbered := width >= 58
	for i, label := range labels {
		prefix := ""
		if numbered {
			prefix = fmt.Sprintf("%d ", i+1)
		}
		if i == active {
			if theme.ASCII {
				parts = append(parts, "["+prefix+label+"]")
			} else if theme.Colors {
				parts = append(parts, theme.tabActive().Render(" "+prefix+"◆ "+label+" "))
			} else {
				parts = append(parts, theme.accent().Render("◆ "+prefix+label))
			}
			continue
		}
		if theme.ASCII {
			parts = append(parts, " "+prefix+label+" ")
		} else {
			parts = append(parts, theme.muted().Render(" "+prefix+label+" "))
		}
	}
	separator := " "
	if !theme.ASCII && width >= 64 {
		separator = theme.cardLine().Render(" │ ")
	}
	left := strings.Join(parts, separator)
	if rightMeta != "" && width >= 72 {
		leftW := visibleWidth(left)
		rightW := visibleWidth(rightMeta)
		if leftW+3+rightW <= width {
			gap := width - leftW - rightW
			return left + strings.Repeat(" ", gap) + rightMeta
		}
	}
	return fitText(left, max(1, width))
}

// renderHeaderRibbon draws the Crush-inspired top command header with brand
// ribbon, diagonal slash fill on wide terminals, and right-aligned status meta.
func renderHeaderRibbon(theme Theme, leftParts []string, rightMeta string, width int) string {
	if width <= 0 {
		return ""
	}
	left := strings.Join(leftParts, "  ")
	if rightMeta == "" || width < 68 {
		return fitText(left, width)
	}
	leftW := visibleWidth(left)
	rightW := visibleWidth(rightMeta)
	if leftW+2+rightW > width {
		combined := left + "  " + rightMeta
		return fitText(combined, width)
	}
	slack := width - leftW - rightW
	if !theme.ASCII && slack >= 8 {
		slashes := strings.Repeat("/", slack-4)
		return left + "  " + theme.cardLine().Render(slashes) + "  " + rightMeta
	}
	return left + strings.Repeat(" ", slack) + rightMeta
}

// renderShearedBox renders a hardware-styled rounded box with border-embedded
// title and optional right-hand metadata on the top rail.
func renderShearedBox(theme Theme, title, rightMeta, body string, width, minHeight int, focused bool) string {
	if width < 14 {
		return fitLines(body, width)
	}
	innerWidth := max(1, width-4)
	tl, tr, bl, br, hz, vt, vtR := "╭─ ", "─╮", "╰─", "─╯", "─", "│ ", " │"
	if theme.ASCII {
		tl, tr, bl, br, hz, vt, vtR = "+- ", "-+", "+-", "-+", "-", "| ", " |"
	}
	borderStyle := theme.cardLine()
	if focused {
		borderStyle = theme.cardLineHot()
	}

	maxTitleW := max(1, width-8)
	plainTitle := fitText(stripANSI(title), maxTitleW)
	titleRendered := plainTitle
	if plainTitle != "" {
		if focused || (theme.Colors && !theme.ASCII) {
			titleRendered = theme.accent().Render(plainTitle)
		} else {
			titleRendered = theme.title().Render(plainTitle)
		}
	}
	rawTitleWidth := visibleWidth(plainTitle)

	var topLine string
	plainRight := strings.TrimSpace(stripANSI(rightMeta))
	rawRightWidth := visibleWidth(plainRight)
	// Layout with right metadata: "╭─ " (3) + title + " " (1) + dashes (>=1) + " " (1) + right + " ─╮" (3)
	if plainRight != "" && 3+rawTitleWidth+3+rawRightWidth+3 <= width {
		dashCount := width - 9 - rawTitleWidth - rawRightWidth
		rightStyled := theme.muted().Render(plainRight)
		topLine = borderStyle.Render(tl) + titleRendered + borderStyle.Render(" "+strings.Repeat(hz, max(1, dashCount))+" ") + rightStyled + borderStyle.Render(" "+tr)
	} else if plainTitle != "" {
		topFillWidth := max(0, width-6-rawTitleWidth)
		topLine = borderStyle.Render(tl) + titleRendered + borderStyle.Render(" "+strings.Repeat(hz, topFillWidth)+tr)
	} else {
		topLine = borderStyle.Render("╭" + strings.Repeat(hz, max(0, width-2)) + "╮")
		if theme.ASCII {
			topLine = borderStyle.Render("+" + strings.Repeat(hz, max(0, width-2)) + "+")
		}
	}
	topLine = fitText(topLine, width)

	bodyLines := strings.Split(body, "\n")
	contentCount := len(bodyLines)
	if minHeight > 0 && contentCount < minHeight {
		contentCount = minHeight
	}
	leftBorder := borderStyle.Render(vt)
	rightBorder := borderStyle.Render(vtR)

	var builder strings.Builder
	builder.Grow((width + 16) * (contentCount + 2))
	builder.WriteString(topLine)
	builder.WriteByte('\n')

	for i := 0; i < contentCount; i++ {
		line := ""
		if i < len(bodyLines) {
			line = bodyLines[i]
		}
		rawLineWidth := visibleWidth(line)
		if rawLineWidth > innerWidth {
			line = fitText(line, innerWidth)
			rawLineWidth = visibleWidth(line)
		}
		fill := max(0, innerWidth-rawLineWidth)
		builder.WriteString(leftBorder)
		builder.WriteString(line)
		if fill > 0 {
			builder.WriteString(strings.Repeat(" ", fill))
		}
		builder.WriteString(rightBorder)
		builder.WriteByte('\n')
	}

	bottomLine := borderStyle.Render(bl + strings.Repeat(hz, max(0, width-4)) + br)
	builder.WriteString(fitText(bottomLine, width))
	return builder.String()
}

// renderSparkline renders an 8-level Unicode block sparkline (with ASCII
// fallback) clamped to width. Values are normalized across their positive max.
func renderSparkline(values []int, width int, ascii bool) string {
	if width <= 0 {
		return ""
	}
	if len(values) == 0 {
		if ascii {
			return strings.Repeat(".", width)
		}
		return strings.Repeat("▁", width)
	}
	sampled := values
	if len(sampled) > width {
		sampled = sampled[len(sampled)-width:]
	}
	peak := 1
	for _, v := range sampled {
		if v > peak {
			peak = v
		}
	}
	var b strings.Builder
	if len(sampled) < width {
		padCount := width - len(sampled)
		for i := 0; i < padCount; i++ {
			if ascii {
				b.WriteByte('.')
			} else {
				b.WriteRune('▁')
			}
		}
	}
	for _, v := range sampled {
		if v < 0 {
			v = 0
		}
		idx := (v * 7) / peak
		if idx > 7 {
			idx = 7
		}
		if ascii {
			b.WriteByte(asciiSparkBlocks[idx])
		} else {
			b.WriteRune(unicodeSparkBlocks[idx])
		}
	}
	return b.String()
}

func fitLines(text string, width int) string {
	if width <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = fitText(line, width)
	}
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
