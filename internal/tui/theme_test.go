package tui

import (
	"testing"

	"github.com/aklkbqx/wol/internal/ui"
)

func TestNightPaletteMatchesDeskTokens(t *testing.T) {
	if got := string(NewTheme(true, false).Palette.Attention); got != ui.Night.Amber {
		t.Fatalf("Attention = %q, want %q", got, ui.Night.Amber)
	}
}
