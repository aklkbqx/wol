package ui

import (
	"image/color"
	"os"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

// adaptive picks a light or dark variant at first use. The terminal is only
// queried when both stdin and stdout are terminals and NO_COLOR is unset, so
// pipes, tests and scripts never get a background query written to them.
type adaptive struct{ light, dark color.Color }

var (
	darkOnce sync.Once
	darkBg   = true
)

func hasDarkBackground() bool {
	darkOnce.Do(func() {
		if os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
			return
		}
		darkBg = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	})
	return darkBg
}

// RGBA satisfies color.Color.
func (a adaptive) RGBA() (r, g, b, alpha uint32) {
	if hasDarkBackground() {
		return a.dark.RGBA()
	}
	return a.light.RGBA()
}

func adapt(light, dark string) color.Color {
	return adaptive{light: lipgloss.Color(light), dark: lipgloss.Color(dark)}
}

// Color tokens for Cyber/Midnight theme
var (
	ColorPrimary   = adapt("#0284c7", "#38bdf8") // Sky Blue
	ColorSecondary = adapt("#6366f1", "#818cf8") // Indigo
	ColorAccent    = adapt("#7c3aed", "#a855f7") // Purple
	ColorSuccess   = adapt("#059669", "#34d399") // Emerald Green
	ColorWarning   = adapt("#d97706", "#fbbf24") // Amber
	ColorDanger    = adapt("#dc2626", "#f87171") // Ruby Red
	ColorMuted     = adapt("#64748b", "#94a3b8") // Slate
	ColorSubtle    = adapt("#94a3b8", "#475569") // Darker Slate
	ColorBg        = adapt("#f8fafc", "#0f172a") // Midnight Dark
	ColorBorder    = adapt("#cbd5e1", "#334155") // Dark Slate
)

// Lip Gloss Base Styles
var (
	StyleBold = lipgloss.NewStyle().Bold(true)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Padding(0, 1)

	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	StyleSuccess = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSuccess)

	StyleWarning = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWarning)

	StyleDanger = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorDanger)

	StyleMuted = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StyleKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	StyleVal = lipgloss.NewStyle().
			Foreground(adapt("#0f172a", "#f8fafc"))

	StyleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	StyleBadgeSuccess = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#ffffff")).
				Background(lipgloss.Color("#059669")).
				Padding(0, 1)

	StyleBadgeWarning = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#000000")).
				Background(lipgloss.Color("#fbbf24")).
				Padding(0, 1)

	StyleBadgeDanger = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#ffffff")).
				Background(lipgloss.Color("#dc2626")).
				Padding(0, 1)

	StyleBadgeInfo = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#0284c7")).
			Padding(0, 1)
)

// Paint returns styled string based on ANSI escape codes or simple lipgloss style
func Paint(c color.Color, text string) string {
	return lipgloss.NewStyle().Foreground(c).Render(text)
}

// Badge formats a text badge
func Badge(status string, text string) string {
	switch strings.ToLower(status) {
	case "success", "ok", "ready", "online":
		return StyleBadgeSuccess.Render(text)
	case "warning", "pending", "building":
		return StyleBadgeWarning.Render(text)
	case "danger", "error", "failed", "offline":
		return StyleBadgeDanger.Render(text)
	default:
		return StyleBadgeInfo.Render(text)
	}
}
