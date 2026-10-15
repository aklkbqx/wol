package tui

import (
	"math"
	"strconv"
	"testing"

	"github.com/aklkbqx/wol/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

func TestNightPaletteMatchesDeskTokens(t *testing.T) {
	if got := string(NewTheme(true, false).Palette.Attention); got != ui.Night.Amber {
		t.Fatalf("Attention = %q, want %q", got, ui.Night.Amber)
	}
}

func TestRecordingTextContrast(t *testing.T) {
	if got := contrastRatio("#000000", "#ffffff"); math.Abs(got-21) > 1e-9 {
		t.Fatalf("black/white contrast = %f, want 21", got)
	}
	p := NewTheme(true, false).Palette
	for _, fg := range []lipgloss.Color{p.Text, p.Muted, p.Slate, p.Error, p.Rose, p.Purple} {
		for _, bg := range []lipgloss.Color{p.Ink, p.Panel, p.PanelHot, p.CardBg} {
			if ratio := contrastRatio(string(fg), string(bg)); ratio < 4.5 {
				t.Errorf("normal text contrast %.2f for %s on %s, want at least 4.5", ratio, fg, bg)
			}
		}
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
