package tui

import (
	"os"
	"strings"

	"github.com/aklkbqx/wol/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// Palette is the night-desk color vocabulary. Amber is reserved for the
// selected machine and the action in progress — a NIC link LED, not chrome.
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

// NewTheme creates the wake desk theme: warm paper on a dark chassis, one
// amber signal, teal only for a machine that is actually online.
func NewTheme(colors, ascii bool) Theme {
	return Theme{
		Palette: Palette{
			Ink:       lipgloss.Color(ui.Night.Ink),
			Panel:     lipgloss.Color(ui.Night.Ink),
			PanelHot:  lipgloss.Color(ui.Night.Panel),
			Text:      lipgloss.Color(ui.Night.Paper),
			Muted:     lipgloss.Color(ui.Night.Muted),
			Border:    lipgloss.Color(ui.Night.Line),
			Network:   lipgloss.Color(ui.Night.Amber),
			Attention: lipgloss.Color(ui.Night.Amber),
			Success:   lipgloss.Color(ui.Night.Live),
			Error:     lipgloss.Color(ui.Night.Danger),
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
		style = style.Bold(true).Foreground(t.Palette.Attention)
	} else {
		style = style.Bold(true)
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
