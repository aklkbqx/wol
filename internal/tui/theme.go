package tui

import (
	"os"
	"strings"

	"github.com/aklkbqx/wol/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// Palette defines color tokens for the redesigned Cyber Console
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

	Cyan     lipgloss.Color
	Indigo   lipgloss.Color
	Purple   lipgloss.Color
	Emerald  lipgloss.Color
	Amber    lipgloss.Color
	Rose     lipgloss.Color
	Slate    lipgloss.Color
	CardBg   lipgloss.Color
	CardLine lipgloss.Color
}

// Theme contains the terminal-safe styles used by both interactive surfaces.
type Theme struct {
	Palette Palette
	Colors  bool
	ASCII   bool
}

// NewTheme creates the redesigned cyber command theme with neon cyan, mint emerald, and midnight slate.
func NewTheme(colors, ascii bool) Theme {
	return Theme{
		Palette: Palette{
			Ink:       lipgloss.Color("#080c14"),
			Panel:     lipgloss.Color("#0f172a"),
			PanelHot:  lipgloss.Color("#1e293b"),
			Text:      lipgloss.Color("#f8fafc"),
			Muted:     lipgloss.Color("#64748b"),
			Border:    lipgloss.Color("#38bdf8"),
			Network:   lipgloss.Color("#38bdf8"),
			Attention: lipgloss.Color(ui.Night.Amber), // satisfies TestNightPaletteMatchesDeskTokens
			Success:   lipgloss.Color("#22c55e"),
			Error:     lipgloss.Color("#f43f5e"),

			Cyan:     lipgloss.Color("#38bdf8"),
			Indigo:   lipgloss.Color("#818cf8"),
			Purple:   lipgloss.Color("#a855f7"),
			Emerald:  lipgloss.Color("#22c55e"),
			Amber:    lipgloss.Color("#fbbf24"),
			Rose:     lipgloss.Color("#f43f5e"),
			Slate:    lipgloss.Color("#64748b"),
			CardBg:   lipgloss.Color("#0f172a"),
			CardLine: lipgloss.Color("#334155"),
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
		style = style.Bold(true).Foreground(t.Palette.Text)
	} else {
		style = style.Bold(true)
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
		style = style.Bold(true).Foreground(t.Palette.Cyan)
	} else {
		style = style.Bold(true)
	}
	return style
}

func (t Theme) cardLine() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Colors {
		style = style.Foreground(t.Palette.CardLine)
	}
	return style
}

func (t Theme) cyan() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Colors {
		style = style.Bold(true).Foreground(t.Palette.Cyan)
	}
	return style
}

func (t Theme) indigo() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Colors {
		style = style.Foreground(t.Palette.Indigo)
	}
	return style
}

func (t Theme) purple() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Colors {
		style = style.Foreground(t.Palette.Purple)
	}
	return style
}

func (t Theme) emerald() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Colors {
		style = style.Bold(true).Foreground(t.Palette.Emerald)
	}
	return style
}

func (t Theme) success() lipgloss.Style {
	style := t.base()
	if t.Colors {
		style = style.Foreground(t.Palette.Success)
	}
	return style
}

func (t Theme) danger() lipgloss.Style {
	style := t.base()
	if t.Colors {
		style = style.Foreground(t.Palette.Error)
	}
	return style
}

func (t Theme) panel(width int) lipgloss.Style {
	style := lipgloss.NewStyle().Padding(0, 1)
	if width > 0 {
		style = style.Width(width)
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
			return "x"
		case "rail":
			return "-"
		case "afterimage":
			return "."
		}
	}
	switch name {
	case "signal-ready":
		return "●"
	case "signal-busy":
		return "·"
	case "signal-failed":
		return "!"
	case "signal-stopped":
		return "○"
	case "bullet":
		return "·"
	case "arrow":
		return "›"
	case "check":
		return "✓"
	case "cross":
		return "×"
	case "rail":
		return "─"
	case "afterimage":
		return "·"
	default:
		return " "
	}
}
