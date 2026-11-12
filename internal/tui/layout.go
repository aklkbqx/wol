package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// LayoutMode is selected from the current terminal dimensions.
type LayoutMode string

const (
	LayoutWide    LayoutMode = "wide"
	LayoutCompact LayoutMode = "compact"
	LayoutNarrow  LayoutMode = "narrow"
)

func ResolveLayout(width, height int) LayoutMode {
	if width >= 110 && height >= 30 {
		return LayoutWide
	}
	if width >= 76 && height >= 20 {
		return LayoutCompact
	}
	return LayoutNarrow
}

func isPlainASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] >= 0x7f {
			return false
		}
	}
	return true
}

func asciiANSIWidth(s string) (int, bool) {
	w := 0
	inEsc := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if inEsc {
			if b == 'm' {
				inEsc = false
			} else if b >= 0x80 {
				return 0, false
			}
			continue
		}
		if b == 0x1b {
			inEsc = true
			continue
		}
		if b < 0x20 || b >= 0x7f {
			return 0, false
		}
		w++
	}
	return w, !inEsc
}

func visibleWidth(text string) int {
	if w, ok := asciiANSIWidth(text); ok {
		return w
	}
	return lipgloss.Width(text)
}

func padVisible(text string, width int) string {
	text = fitText(text, width)
	w := visibleWidth(text)
	if w >= width {
		return text
	}
	return text + strings.Repeat(" ", width-w)
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
	if visibleWidth(text) <= width {
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
	if lipgloss.Width(plain) <= width {
		return text
	}
	if width <= 3 {
		return visiblePrefix(runes, width)
	}
	return visiblePrefix(runes, width-3) + "..."
}

func visiblePrefix(runes []rune, width int) string {
	if width <= 0 {
		return ""
	}
	used := 0
	end := 0
	for index, value := range runes {
		cellWidth := lipgloss.Width(string(value))
		if used+cellWidth > width {
			break
		}
		used += cellWidth
		end = index + 1
	}
	return string(runes[:end])
}
