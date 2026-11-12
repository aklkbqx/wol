package termimg

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		vars map[string]string
		want Mode
	}{
		{"ghostty", map[string]string{"TERM": "xterm-ghostty", "TERM_PROGRAM": "ghostty"}, Kitty},
		{"kitty", map[string]string{"TERM": "xterm-kitty"}, Kitty},
		{"kitty window id", map[string]string{"TERM": "xterm-256color", "KITTY_WINDOW_ID": "1"}, Kitty},
		{"ghostty in tmux", map[string]string{"TERM": "tmux-256color", "TERM_PROGRAM": "ghostty", "TMUX": "/tmp/x", "COLORTERM": "truecolor"}, Blocks},
		{"kitty in screen", map[string]string{"TERM": "screen", "STY": "1.x", "KITTY_WINDOW_ID": "1"}, None},
		{"truecolor", map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}, Blocks},
		{"iterm", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app"}, Blocks},
		{"windows terminal", map[string]string{"WT_SESSION": "x", "TERM": "xterm-256color"}, Blocks},
		{"apple terminal", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "Apple_Terminal"}, None},
		{"dumb", map[string]string{"TERM": "dumb", "COLORTERM": "truecolor"}, None},
		{"no colour wins", map[string]string{"TERM": "xterm-kitty", "NO_COLOR": "1", EnvVar: "kitty"}, None},
		{"override off", map[string]string{"TERM": "xterm-kitty", EnvVar: "off"}, None},
		{"override blocks", map[string]string{"TERM": "xterm-kitty", EnvVar: "blocks"}, Blocks},
		{"override kitty", map[string]string{"TERM": "xterm-256color", EnvVar: "kitty"}, Kitty},
		{"auto detects", map[string]string{"TERM": "xterm-kitty", EnvVar: "auto"}, Kitty},
	}
	for _, tc := range cases {
		if got := Detect(env(tc.vars)); got != tc.want {
			t.Errorf("%s: Detect = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// cells counts the printed cells of a line, skipping escape sequences.
func cells(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := strings.IndexByte(s[i:], 'm')
			if j < 0 {
				return -1
			}
			i += j + 1
			continue
		}
		r := []rune(s[i:])[0]
		i += len(string(r))
		if r >= 0x300 && r < 0x370 || r >= 0x480 && r < 0x2000 && r != 0x2580 {
			continue // combining marks of the placeholder table
		}
		n++
	}
	return n
}

func TestBlockLines(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 7, 5))
	for y := 0; y < 5; y++ {
		for x := 0; x < 7; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 50), B: 9, A: 255})
		}
	}
	lines := BlockLines(img)
	if len(lines) != 3 {
		t.Fatalf("5 pixel rows gave %d lines, want 3", len(lines))
	}
	for i, l := range lines {
		if got := cells(l); got != 7 {
			t.Errorf("line %d is %d cells, want 7: %q", i, got, l)
		}
		if !strings.HasSuffix(l, "\x1b[m") {
			t.Errorf("line %d does not reset colours", i)
		}
	}
	if !strings.Contains(lines[0], "\x1b[38;2;0;0;9m") || !strings.Contains(lines[0], "▀") {
		t.Errorf("first line misses the top pixel colour: %q", lines[0])
	}
}

func TestTransmitChunks(t *testing.T) {
	data := make([]byte, 7000) // 9336 base64 bytes: three chunks
	out := Transmit(42, data, 12, 4)
	parts := strings.Split(strings.TrimSuffix(out, "\x1b\\"), "\x1b\\")
	if len(parts) != 3 {
		t.Fatalf("got %d chunks, want 3", len(parts))
	}
	if !strings.HasPrefix(parts[0], "\x1b_Ga=T,U=1,f=100,t=d,q=2,i=42,c=12,r=4,m=1;") {
		t.Errorf("first chunk header = %q", parts[0][:60])
	}
	if !strings.HasPrefix(parts[1], "\x1b_Gq=2,m=1;") || !strings.HasPrefix(parts[2], "\x1b_Gq=2,m=0;") {
		t.Errorf("later chunks must keep q=2 and end with m=0")
	}
	for i, p := range parts[:2] {
		if payload := p[strings.IndexByte(p, ';')+1:]; len(payload) != chunk {
			t.Errorf("chunk %d carries %d bytes, want %d", i, len(payload), chunk)
		}
	}
	if one := Transmit(1, nil, 1, 1); strings.Count(one, "\x1b_G") != 1 || !strings.Contains(one, "m=0;") {
		t.Errorf("empty data should still send one final chunk: %q", one)
	}
	if got := Delete(42); got != "\x1b_Ga=d,d=I,q=2,i=42\x1b\\" {
		t.Errorf("Delete = %q", got)
	}
}

func TestPlaceholderLines(t *testing.T) {
	lines := PlaceholderLines(17, 5, 3)
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	for r, l := range lines {
		if !strings.HasPrefix(l, "\x1b[38;5;17m") || !strings.HasSuffix(l, "\x1b[m") {
			t.Errorf("line %d colours = %q", r, l)
		}
		runes := []rune(strings.TrimSuffix(strings.TrimPrefix(l, "\x1b[38;5;17m"), "\x1b[m"))
		if len(runes) != 15 {
			t.Fatalf("line %d has %d runes, want 5 cells of 3", r, len(runes))
		}
		for c := 0; c < 5; c++ {
			if runes[3*c] != placeholder || runes[3*c+1] != diacritics[r] || runes[3*c+2] != diacritics[c] {
				t.Errorf("cell %d,%d = %q", r, c, string(runes[3*c:3*c+3]))
			}
		}
	}
	if got := PlaceholderLines(1, 500, 500); len(got) != MaxPlaceholder {
		t.Errorf("size is not capped: %d rows", len(got))
	}
}

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{"OFF": None, " blocks ": Blocks, "kitty": Kitty} {
		if m, ok := ParseMode(in); !ok || m != want {
			t.Errorf("ParseMode(%q) = %s %v", in, m, ok)
		}
	}
	if _, ok := ParseMode("auto"); ok {
		t.Error("auto must mean detect")
	}
}
