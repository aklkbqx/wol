package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Color tokens for Cyber/Midnight theme
var (
	ColorPrimary   = lipgloss.AdaptiveColor{Light: "#0284c7", Dark: "#38bdf8"} // Sky Blue
	ColorSecondary = lipgloss.AdaptiveColor{Light: "#6366f1", Dark: "#818cf8"} // Indigo
	ColorAccent    = lipgloss.AdaptiveColor{Light: "#7c3aed", Dark: "#a855f7"} // Purple
	ColorSuccess   = lipgloss.AdaptiveColor{Light: "#059669", Dark: "#34d399"} // Emerald Green
	ColorWarning   = lipgloss.AdaptiveColor{Light: "#d97706", Dark: "#fbbf24"} // Amber
	ColorDanger    = lipgloss.AdaptiveColor{Light: "#dc2626", Dark: "#f87171"} // Ruby Red
	ColorMuted     = lipgloss.AdaptiveColor{Light: "#64748b", Dark: "#94a3b8"} // Slate
	ColorSubtle    = lipgloss.AdaptiveColor{Light: "#94a3b8", Dark: "#475569"} // Darker Slate
	ColorBg        = lipgloss.AdaptiveColor{Light: "#f8fafc", Dark: "#0f172a"} // Midnight Dark
	ColorBorder    = lipgloss.AdaptiveColor{Light: "#cbd5e1", Dark: "#334155"} // Dark Slate
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
			Foreground(lipgloss.AdaptiveColor{Light: "#0f172a", Dark: "#f8fafc"})

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
func Paint(color lipgloss.TerminalColor, text string) string {
	return lipgloss.NewStyle().Foreground(color).Render(text)
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
