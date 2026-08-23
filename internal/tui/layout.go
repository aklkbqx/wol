package tui

import "github.com/charmbracelet/lipgloss"

// LayoutMode is selected from the current terminal dimensions.
type LayoutMode string

const (
	LayoutWide    LayoutMode = "wide"
	LayoutCompact LayoutMode = "compact"
	LayoutNarrow  LayoutMode = "narrow"
)

func ResolveLayout(width, height int) LayoutMode {
	if width >= 110 && height >= 24 {
		return LayoutWide
	}
	if width >= 76 && height >= 20 {
		return LayoutCompact
	}
	return LayoutNarrow
}

func contentWidth(width int) int {
	if width <= 0 {
		return 76
	}
	if width < 12 {
		return width
	}
	return width - 2
}

func clampWidth(width, minimum, maximum int) int {
	if width < minimum {
		return minimum
	}
	if maximum > 0 && width > maximum {
		return maximum
	}
	return width
}

func fitText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	return truncateANSI(text, width)
}

// truncateANSI is deliberately conservative: TUI labels are generated from
// redacted strings and styles, so preserving the visible prefix is preferable
// to attempting to parse arbitrary terminal control sequences here.
func truncateANSI(text string, width int) string {
	if width <= 0 {
		return ""
	}
	plain := stripANSI(text)
	runes := []rune(plain)
	if len(runes) <= width {
		return text
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}
