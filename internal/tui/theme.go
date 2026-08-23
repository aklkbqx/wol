package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette is the shared Signal Desk color vocabulary. Each token has one
// semantic job so status remains understandable without relying on decoration.
type Palette struct {
	Ink       lipgloss.Color
	Panel     lipgloss.Color
	PanelHot  lipgloss.Color
	Text      lipgloss.Color
	Muted     lipgloss.Color
	Border    lipgloss.Color
	Network   lipgloss.Color
	Attention lipgloss.Color
	Success   lipgloss.Color
	Error     lipgloss.Color
}

// Theme contains the terminal-safe styles used by both interactive surfaces.
type Theme struct {
	Palette Palette
	Colors  bool
	ASCII   bool
}

// NewTheme creates the WOL Signal Desk theme. The palette intentionally avoids
// gradients and glow effects: contrast and a single signal rail carry status.
func NewTheme(colors, ascii bool) Theme {
	return Theme{
		Palette: Palette{
			Ink:       lipgloss.Color("#0D131B"),
			Panel:     lipgloss.Color("#16212B"),
			PanelHot:  lipgloss.Color("#1D2C38"),
			Text:      lipgloss.Color("#E7EEF5"),
			Muted:     lipgloss.Color("#8FA2B3"),
			Border:    lipgloss.Color("#304452"),
			Network:   lipgloss.Color("#4FD1C5"),
			Attention: lipgloss.Color("#F4B942"),
			Success:   lipgloss.Color("#9ED47A"),
			Error:     lipgloss.Color("#F26D6D"),
		},
		Colors: colors,
		ASCII:  ascii,
	}
}

// DetectTheme reads the documented terminal controls once at startup.
func DetectTheme() Theme {
	colors := os.Getenv("NO_COLOR") == "" && !strings.EqualFold(strings.TrimSpace(os.Getenv("WOL_TUI_COLOR")), "off")
	ascii := envOn("WOL_TUI_ASCII")
	return NewTheme(colors, ascii)
}

// MotionEnabled is kept separate from the theme so a no-color terminal can
// still choose whether to see motion cues.
func MotionEnabled() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("WOL_TUI_MOTION")), "off") && !envOn("WOL_TUI_REDUCED_MOTION")
}

func envOn(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (t Theme) base() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Colors {
		style = style.Foreground(t.Palette.Text)
	}
	return style
}

func (t Theme) title() lipgloss.Style {
	style := t.base()
	if t.Colors {
		style = style.Bold(true)
		style = style.Foreground(t.Palette.Network)
	}
	return style
}

func (t Theme) muted() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Colors {
		style = style.Foreground(t.Palette.Muted)
	}
	return style
}

func (t Theme) accent() lipgloss.Style {
	style := t.base()
	if t.Colors {
		style = style.Bold(true)
		style = style.Foreground(t.Palette.Attention)
	}
	return style
}

func (t Theme) success() lipgloss.Style {
	style := t.base()
	if t.Colors {
		style = style.Bold(true)
		style = style.Foreground(t.Palette.Success)
	}
	return style
}

func (t Theme) danger() lipgloss.Style {
	style := t.base()
	if t.Colors {
		style = style.Bold(true)
		style = style.Foreground(t.Palette.Error)
	}
	return style
}

func (t Theme) panel(width int) lipgloss.Style {
	style := lipgloss.NewStyle().Padding(0, 1)
	if width > 0 {
		style = style.Width(width)
	}
	if t.Colors {
		style = style.Border(lipgloss.NormalBorder()).BorderForeground(t.Palette.Border)
	}
	return style
}

// Glyph returns a semantic symbol with an ASCII fallback for narrow or
// automation terminals.
func (t Theme) Glyph(name string) string {
	if t.ASCII {
		switch name {
		case "signal-ready":
			return "*"
		case "signal-busy":
			return ">"
		case "signal-failed":
			return "!"
		case "signal-stopped":
			return "-"
		case "bullet":
			return "-"
		case "arrow":
			return ">"
		case "check":
			return "OK"
		case "cross":
			return "!!"
		}
	}
	switch name {
	case "signal-ready":
		return "●"
	case "signal-busy":
		return "◐"
	case "signal-failed":
		return "!"
	case "signal-stopped":
		return "○"
	case "bullet":
		return "•"
	case "arrow":
		return "›"
	case "check":
		return "✓"
	case "cross":
		return "×"
	default:
		return " "
	}
}
