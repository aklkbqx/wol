package ui

import (
	"strings"
	"testing"
)

func TestNightTokensDoNotCollideWithCyberNames(t *testing.T) {
	_ = ColorMuted
	_ = ColorDanger
	if Night.Muted == "" || Night.Danger == "" || Night.Amber != "#C9842A" {
		t.Fatalf("Night tokens = %+v", Night)
	}
	css := DeskCSS()
	for _, part := range []string{Night.Amber, "--panel", "prefers-reduced-motion", "*{box-sizing:border-box}"} {
		if !strings.Contains(css, part) {
			t.Fatalf("DeskCSS missing %q: %s", part, css)
		}
	}
}
