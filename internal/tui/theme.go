package tui

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/aklkbqx/wol/internal/ui"
)

// Palette defines semantic color tokens for the Wake Desk TUI.
type Palette struct {
	Ink       color.Color
	Panel     color.Color
	PanelHot  color.Color
	Text      color.Color
	Muted     color.Color
	Border    color.Color
	Network   color.Color
	Attention color.Color
	Success   color.Color
	Error     color.Color

	Cyan     color.Color
	Indigo   color.Color
	Purple   color.Color
	Emerald  color.Color
	Amber    color.Color
	Rose     color.Color
	Slate    color.Color
	CardBg   color.Color
	CardLine color.Color
	// Track is the unfilled part of progress bars; CardLine is used when nil.
	Track color.Color
}

// PalettePreset describes a named, high-contrast color scheme selectable via
// WOL_TUI_THEME or cycled at runtime with the 't' shortcut.
type PalettePreset struct {
	ID      string
	Name    string
	Palette Palette
}

type themeStyles struct {
	ready       bool
	base        lipgloss.Style
	title       lipgloss.Style
	muted       lipgloss.Style
	accent      lipgloss.Style
	amber       lipgloss.Style
	cardLine    lipgloss.Style
	cardLineHot lipgloss.Style
	cyan        lipgloss.Style
	indigo      lipgloss.Style
	emerald     lipgloss.Style
	success     lipgloss.Style
	danger      lipgloss.Style
	ribbon      lipgloss.Style
	tabActive   lipgloss.Style
	keycap      lipgloss.Style
	spot        spotStyles
}

// Theme contains the terminal-safe styles and active palette preset.
type Theme struct {
	Palette    Palette
	Colors     bool
	ASCII      bool
	presetIdx  int
	presetID   string
	presetName string
	styles     themeStyles
}

var paletteRegistry = []PalettePreset{
	{
		ID:   "night",
		Name: "Night Room",
		Palette: Palette{
			Ink:       lipgloss.Color("#080C14"),
			Panel:     lipgloss.Color("#10172A"),
			PanelHot:  lipgloss.Color("#1B2542"),
			Text:      lipgloss.Color("#F3F5F8"),
			Muted:     lipgloss.Color("#8E99B3"),
			Border:    lipgloss.Color("#F5A524"),
			Network:   lipgloss.Color("#C6CEDE"),
			Attention: lipgloss.Color("#F5A524"),
			Success:   lipgloss.Color("#22C55E"),
			Error:     lipgloss.Color("#FB7185"),

			Cyan:     lipgloss.Color("#C6CEDE"),
			Indigo:   lipgloss.Color("#C6CEDE"),
			Purple:   lipgloss.Color("#C6CEDE"),
			Emerald:  lipgloss.Color("#22C55E"),
			Amber:    lipgloss.Color("#F5A524"),
			Rose:     lipgloss.Color("#FB7185"),
			Slate:    lipgloss.Color("#C6CEDE"),
			CardBg:   lipgloss.Color("#10172A"),
			CardLine: lipgloss.Color("#26314F"),
			Track:    lipgloss.Color("#3A4668"),
		},
	},
	{
		ID:   "brass",
		Name: "Brass Night",
		Palette: Palette{
			Ink:       lipgloss.Color(ui.Night.Ink),
			Panel:     lipgloss.Color("#161B14"),
			PanelHot:  lipgloss.Color("#1F261D"),
			Text:      lipgloss.Color("#F2EDE4"),
			Muted:     lipgloss.Color("#9DA497"),
			Border:    lipgloss.Color("#E5A93B"),
			Network:   lipgloss.Color("#67E8F9"),
			Attention: lipgloss.Color(ui.Night.Amber),
			Success:   lipgloss.Color("#4ADE80"),
			Error:     lipgloss.Color("#FB7185"),

			Cyan:     lipgloss.Color("#67E8F9"),
			Indigo:   lipgloss.Color("#A5B4FC"),
			Purple:   lipgloss.Color("#D8B4FE"),
			Emerald:  lipgloss.Color("#4ADE80"),
			Amber:    lipgloss.Color("#FBBF24"),
			Rose:     lipgloss.Color("#FB7185"),
			Slate:    lipgloss.Color("#A8B0A2"),
			CardBg:   lipgloss.Color("#161B14"),
			CardLine: lipgloss.Color("#364133"),
		},
	},
	{
		ID:   "catppuccin",
		Name: "Catppuccin Mocha",
		Palette: Palette{
			Ink:       lipgloss.Color("#11111B"),
			Panel:     lipgloss.Color("#181825"),
			PanelHot:  lipgloss.Color("#24273A"),
			Text:      lipgloss.Color("#CDD6F4"),
			Muted:     lipgloss.Color("#A6ADC8"),
			Border:    lipgloss.Color("#CBA6F7"),
			Network:   lipgloss.Color("#89DCEB"),
			Attention: lipgloss.Color("#F9E2AF"),
			Success:   lipgloss.Color("#A6E3A1"),
			Error:     lipgloss.Color("#F38BA8"),

			Cyan:     lipgloss.Color("#89DCEB"),
			Indigo:   lipgloss.Color("#B4BEFE"),
			Purple:   lipgloss.Color("#CBA6F7"),
			Emerald:  lipgloss.Color("#A6E3A1"),
			Amber:    lipgloss.Color("#F9E2AF"),
			Rose:     lipgloss.Color("#F38BA8"),
			Slate:    lipgloss.Color("#BAC2DE"),
			CardBg:   lipgloss.Color("#181825"),
			CardLine: lipgloss.Color("#45475A"),
		},
	},
	{
		ID:   "tokyonight",
		Name: "Tokyo Night",
		Palette: Palette{
			Ink:       lipgloss.Color("#16161E"),
			Panel:     lipgloss.Color("#1A1B26"),
			PanelHot:  lipgloss.Color("#24283B"),
			Text:      lipgloss.Color("#C0CAF5"),
			Muted:     lipgloss.Color("#9AA5CE"),
			Border:    lipgloss.Color("#7AA2F7"),
			Network:   lipgloss.Color("#7DCFFF"),
			Attention: lipgloss.Color("#E0AF68"),
			Success:   lipgloss.Color("#9ECE6A"),
			Error:     lipgloss.Color("#FF859D"),

			Cyan:     lipgloss.Color("#7DCFFF"),
			Indigo:   lipgloss.Color("#9ABDF5"),
			Purple:   lipgloss.Color("#BB9AF7"),
			Emerald:  lipgloss.Color("#9ECE6A"),
			Amber:    lipgloss.Color("#E0AF68"),
			Rose:     lipgloss.Color("#FF859D"),
			Slate:    lipgloss.Color("#A9B1D6"),
			CardBg:   lipgloss.Color("#1A1B26"),
			CardLine: lipgloss.Color("#414868"),
		},
	},
	{
		ID:   "gruvbox",
		Name: "Gruvbox Dark",
		Palette: Palette{
			Ink:       lipgloss.Color("#1D2021"),
			Panel:     lipgloss.Color("#242424"),
			PanelHot:  lipgloss.Color("#2E2C2A"),
			Text:      lipgloss.Color("#EBDBB2"),
			Muted:     lipgloss.Color("#BDAE93"),
			Border:    lipgloss.Color("#FABD2F"),
			Network:   lipgloss.Color("#8EC07C"),
			Attention: lipgloss.Color("#FE8019"),
			Success:   lipgloss.Color("#B8BB26"),
			Error:     lipgloss.Color("#FF7A6B"),

			Cyan:     lipgloss.Color("#8EC07C"),
			Indigo:   lipgloss.Color("#98C1B9"),
			Purple:   lipgloss.Color("#E096AA"),
			Emerald:  lipgloss.Color("#B8BB26"),
			Amber:    lipgloss.Color("#FABD2F"),
			Rose:     lipgloss.Color("#FF7A6B"),
			Slate:    lipgloss.Color("#D5C4A1"),
			CardBg:   lipgloss.Color("#242424"),
			CardLine: lipgloss.Color("#504945"),
		},
	},
	{
		ID:   "daylight",
		Name: "Daylight Paper",
		Palette: Palette{
			Ink:       lipgloss.Color("#FAF8F3"),
			Panel:     lipgloss.Color("#F4F1EA"),
			PanelHot:  lipgloss.Color("#EAE5DA"),
			Text:      lipgloss.Color("#1C1917"),
			Muted:     lipgloss.Color("#524D47"),
			Border:    lipgloss.Color("#7C3A0A"),
			Network:   lipgloss.Color("#0E5A6F"),
			Attention: lipgloss.Color("#7C3A0A"),
			Success:   lipgloss.Color("#166534"),
			Error:     lipgloss.Color("#991B1B"),

			Cyan:     lipgloss.Color("#0E5A6F"),
			Indigo:   lipgloss.Color("#3730A3"),
			Purple:   lipgloss.Color("#6B21A8"),
			Emerald:  lipgloss.Color("#166534"),
			Amber:    lipgloss.Color("#7C3A0A"),
			Rose:     lipgloss.Color("#9F1239"),
			Slate:    lipgloss.Color("#44403C"),
			CardBg:   lipgloss.Color("#F4F1EA"),
			CardLine: lipgloss.Color("#B8B0A0"),
		},
	},
}

// PalettePresets returns a copy of all built-in TUI color presets.
func PalettePresets() []PalettePreset {
	out := make([]PalettePreset, len(paletteRegistry))
	copy(out, paletteRegistry)
	return out
}

// NewTheme creates the default Night Room theme, which matches the website.
func NewTheme(colors, ascii bool) Theme {
	return NewThemeWithPreset("night", colors, ascii)
}

// NewThemeWithPreset creates a Theme using the named preset ID, falling back
// to Night Room when the preset name is empty or unrecognized.
func NewThemeWithPreset(presetID string, colors, ascii bool) Theme {
	idx := 0
	needle := strings.ToLower(strings.TrimSpace(presetID))
	for i, p := range paletteRegistry {
		if p.ID == needle || strings.EqualFold(p.Name, needle) {
			idx = i
			break
		}
	}
	chosen := paletteRegistry[idx]
	t := Theme{
		Palette:    chosen.Palette,
		Colors:     colors,
		ASCII:      ascii,
		presetIdx:  idx,
		presetID:   chosen.ID,
		presetName: chosen.Name,
	}
	t.styles = buildThemeStyles(t.Palette, colors)
	return t
}

func buildThemeStyles(p Palette, colors bool) themeStyles {
	if !colors {
		bold := lipgloss.NewStyle().Bold(true)
		plain := lipgloss.NewStyle()
		return themeStyles{
			ready:       true,
			base:        plain,
			title:       bold,
			muted:       plain,
			accent:      bold,
			amber:       bold,
			cardLine:    plain,
			cardLineHot: bold,
			cyan:        bold,
			indigo:      plain,
			emerald:     bold,
			success:     plain,
			danger:      plain,
			ribbon:      bold,
			tabActive:   bold,
			keycap:      bold,
			spot:        buildSpotStyles(p, false),
		}
	}
	return themeStyles{
		ready:       true,
		base:        lipgloss.NewStyle().Foreground(p.Text),
		title:       lipgloss.NewStyle().Bold(true).Foreground(p.Text),
		muted:       lipgloss.NewStyle().Foreground(p.Muted),
		accent:      lipgloss.NewStyle().Bold(true).Foreground(p.Amber),
		amber:       lipgloss.NewStyle().Bold(true).Foreground(p.Amber),
		cardLine:    lipgloss.NewStyle().Foreground(p.CardLine),
		cardLineHot: lipgloss.NewStyle().Foreground(p.Border),
		cyan:        lipgloss.NewStyle().Bold(true).Foreground(p.Cyan),
		indigo:      lipgloss.NewStyle().Foreground(p.Indigo),
		emerald:     lipgloss.NewStyle().Bold(true).Foreground(p.Emerald),
		success:     lipgloss.NewStyle().Foreground(p.Success),
		danger:      lipgloss.NewStyle().Foreground(p.Error),
		ribbon:      lipgloss.NewStyle().Bold(true).Foreground(p.Ink).Background(p.Amber),
		tabActive:   lipgloss.NewStyle().Bold(true).Foreground(p.Amber).Background(p.PanelHot),
		keycap:      lipgloss.NewStyle().Bold(true).Foreground(p.Amber).Background(p.PanelHot),
		spot:        buildSpotStyles(p, true),
	}
}

// PresetID returns the active palette preset identifier (for example "brass").
func (t Theme) PresetID() string {
	if t.presetID == "" {
		return paletteRegistry[0].ID
	}
	return t.presetID
}

// PresetName returns the human-readable palette preset title.
func (t Theme) PresetName() string {
	if t.presetName == "" {
		return paletteRegistry[0].Name
	}
	return t.presetName
}

// NextPreset advances to the next built-in palette while preserving terminal
// color and ASCII mode flags.
func (t Theme) NextPreset() Theme {
	next := (t.presetIdx + 1) % len(paletteRegistry)
	return NewThemeWithPreset(paletteRegistry[next].ID, t.Colors, t.ASCII)
}

// DetectTheme reads the documented terminal controls once at startup.
func DetectTheme() Theme {
	colors := os.Getenv("NO_COLOR") == "" && !strings.EqualFold(strings.TrimSpace(os.Getenv("WOL_TUI_COLOR")), "off")
	ascii := envOn("WOL_TUI_ASCII")
	return NewThemeWithPreset(os.Getenv("WOL_TUI_THEME"), colors, ascii)
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

func (t Theme) ensureStyles() themeStyles {
	if t.styles.ready {
		return t.styles
	}
	return buildThemeStyles(t.Palette, t.Colors)
}

func (t Theme) base() lipgloss.Style        { return t.ensureStyles().base }
func (t Theme) title() lipgloss.Style       { return t.ensureStyles().title }
func (t Theme) muted() lipgloss.Style       { return t.ensureStyles().muted }
func (t Theme) accent() lipgloss.Style      { return t.ensureStyles().accent }
func (t Theme) amber() lipgloss.Style       { return t.ensureStyles().amber }
func (t Theme) cardLine() lipgloss.Style    { return t.ensureStyles().cardLine }
func (t Theme) cardLineHot() lipgloss.Style { return t.ensureStyles().cardLineHot }
func (t Theme) cyan() lipgloss.Style        { return t.ensureStyles().cyan }
func (t Theme) indigo() lipgloss.Style      { return t.ensureStyles().indigo }
func (t Theme) emerald() lipgloss.Style     { return t.ensureStyles().emerald }
func (t Theme) success() lipgloss.Style     { return t.ensureStyles().success }
func (t Theme) danger() lipgloss.Style      { return t.ensureStyles().danger }
func (t Theme) ribbon() lipgloss.Style      { return t.ensureStyles().ribbon }
func (t Theme) tabActive() lipgloss.Style   { return t.ensureStyles().tabActive }
func (t Theme) keycap() lipgloss.Style      { return t.ensureStyles().keycap }

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
		case "signal-unknown":
			return "?"
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
		case "diamond":
			return "#"
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
	case "signal-unknown":
		return "◌"
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
	case "diamond":
		return "◆"
	default:
		return " "
	}
}
