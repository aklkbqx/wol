package tui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"testing"

	"github.com/aklkbqx/wol/internal/ui"
)

func TestNightPaletteMatchesDeskTokens(t *testing.T) {
	if got := hexOf(NewThemeWithPreset("brass", true, false).Palette.Attention); got != ui.Night.Amber {
		t.Fatalf("Attention = %q, want %q", got, ui.Night.Amber)
	}
}

// The default preset uses the same tokens as the website.
func TestNightRoomMatchesWebsiteTokens(t *testing.T) {
	p := NewTheme(true, false).Palette
	for name, got := range map[string]string{
		"ink": hexOf(p.Ink), "pane": hexOf(p.Panel), "focus": hexOf(p.PanelHot), "cap": hexOf(p.CardLine),
		"track": hexOf(p.Track), "text": hexOf(p.Text), "soft": hexOf(p.Slate), "muted": hexOf(p.Muted),
		"amber": hexOf(p.Amber), "green": hexOf(p.Emerald), "rose": hexOf(p.Rose),
	} {
		want := map[string]string{
			"ink": "#080C14", "pane": "#10172A", "focus": "#1B2542", "cap": "#26314F", "track": "#3A4668",
			"text": "#F3F5F8", "soft": "#C6CEDE", "muted": "#8E99B3", "amber": "#F5A524", "green": "#22C55E", "rose": "#FB7185",
		}[name]
		if got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
}

func TestRecordingTextContrast(t *testing.T) {
	if got := contrastRatio("#000000", "#ffffff"); math.Abs(got-21) > 1e-9 {
		t.Fatalf("black/white contrast = %f, want 21", got)
	}
	for _, preset := range PalettePresets() {
		t.Run(preset.ID, func(t *testing.T) {
			p := NewThemeWithPreset(preset.ID, true, false).Palette
			for _, fg := range []color.Color{p.Text, p.Muted, p.Slate, p.Error, p.Rose, p.Purple, p.Cyan, p.Emerald, p.Amber} {
				for _, bg := range []color.Color{p.Ink, p.Panel, p.PanelHot, p.CardBg} {
					if ratio := contrastRatio(hexOf(fg), hexOf(bg)); ratio < 4.5 {
						t.Errorf("preset %s contrast %.2f for %s on %s, want at least 4.5", preset.ID, ratio, hexOf(fg), hexOf(bg))
					}
				}
			}
		})
	}
}

func TestPalettePresetsAndRuntimeCycling(t *testing.T) {
	presets := PalettePresets()
	if len(presets) < 5 {
		t.Fatalf("PalettePresets() count = %d, want at least 5", len(presets))
	}
	if presets[0].ID != "night" {
		t.Fatalf("default preset ID = %q, want night", presets[0].ID)
	}

	t.Setenv("NO_COLOR", "")
	t.Setenv("WOL_TUI_COLOR", "")
	t.Setenv("WOL_TUI_THEME", "tokyonight")
	detected := DetectTheme()
	if detected.PresetID() != "tokyonight" || detected.PresetName() != "Tokyo Night" {
		t.Fatalf("DetectTheme() = (%q, %q), want (tokyonight, Tokyo Night)", detected.PresetID(), detected.PresetName())
	}

	t.Setenv("WOL_TUI_THEME", "unknown-theme")
	fallback := DetectTheme()
	if fallback.PresetID() != "night" {
		t.Fatalf("unknown WOL_TUI_THEME fell back to %q, want night", fallback.PresetID())
	}

	cycled := NewTheme(true, true)
	seen := make(map[string]bool)
	for i := 0; i < len(presets); i++ {
		seen[cycled.PresetID()] = true
		cycled = cycled.NextPreset()
		if !cycled.Colors || !cycled.ASCII {
			t.Fatalf("NextPreset() lost terminal flags: Colors=%v ASCII=%v", cycled.Colors, cycled.ASCII)
		}
	}
	if len(seen) != len(presets) {
		t.Fatalf("cycled through %d unique presets, want %d", len(seen), len(presets))
	}
	if cycled.PresetID() != "night" {
		t.Fatalf("full cycle landed on %q, want night", cycled.PresetID())
	}
}

func contrastRatio(a, b string) float64 {
	luminance := func(hex string) float64 {
		var channels [3]float64
		for i := range channels {
			n, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
			if err != nil {
				panic(err)
			}
			v := float64(n) / 255
			if v <= 0.04045 {
				channels[i] = v / 12.92
			} else {
				channels[i] = math.Pow((v+0.055)/1.055, 2.4)
			}
		}
		return channels[0]*0.2126 + channels[1]*0.7152 + channels[2]*0.0722
	}
	x, y := luminance(a), luminance(b)
	return (math.Max(x, y) + 0.05) / (math.Min(x, y) + 0.05)
}

func hexOf(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}
