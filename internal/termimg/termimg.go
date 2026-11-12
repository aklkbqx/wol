// Package termimg turns pictures into terminal output: kitty graphics with
// Unicode placeholders where the terminal supports them, and truecolor
// half-block cells everywhere else. It only builds strings; the caller
// decides when to write them.
package termimg

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strconv"
	"strings"
)

// Mode is how pictures are drawn.
type Mode int

const (
	None   Mode = iota // no pictures
	Blocks             // half-block cells, two pixels per cell
	Kitty              // kitty graphics protocol with Unicode placeholders
)

// EnvVar selects the picture mode: auto, kitty, blocks or off.
const EnvVar = "WOL_TUI_IMAGES"

func (m Mode) String() string {
	switch m {
	case Blocks:
		return "blocks"
	case Kitty:
		return "kitty"
	default:
		return "off"
	}
}

// ParseMode reads a WOL_TUI_IMAGES value. auto (or empty) reports false so
// the caller detects the terminal instead.
func ParseMode(value string) (Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "none", "no", "0", "false":
		return None, true
	case "blocks", "block", "halfblock", "half-block":
		return Blocks, true
	case "kitty", "graphics":
		return Kitty, true
	default:
		return None, false
	}
}

// Detect chooses the picture mode for the terminal described by getenv.
// Kitty graphics are used in Kitty and Ghostty outside tmux and screen,
// half blocks in other truecolor terminals, and nothing when colour is off.
func Detect(getenv func(string) string) Mode {
	if getenv("NO_COLOR") != "" {
		return None
	}
	if m, ok := ParseMode(getenv(EnvVar)); ok {
		return m
	}
	term := strings.ToLower(getenv("TERM"))
	if term == "dumb" || term == "" && getenv("TERM_PROGRAM") == "" {
		return None
	}
	multiplexed := getenv("TMUX") != "" || getenv("STY") != "" || strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux")
	program := strings.ToLower(getenv("TERM_PROGRAM"))
	if !multiplexed && (program == "ghostty" || strings.Contains(term, "kitty") || strings.Contains(term, "ghostty") || getenv("KITTY_WINDOW_ID") != "") {
		return Kitty
	}
	switch strings.ToLower(getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return Blocks
	}
	switch program {
	case "ghostty", "iterm.app", "wezterm", "vscode", "rio", "alacritty":
		return Blocks
	}
	if getenv("WT_SESSION") != "" { // Windows Terminal
		return Blocks
	}
	return None
}

// BlockLines draws img with "▀" cells: each cell shows two pixels, the top
// one in the foreground colour and the bottom one in the background. Every
// line is exactly img width cells wide and ends by resetting colours.
func BlockLines(img *image.RGBA) []string {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rows := (h + 1) / 2
	lines := make([]string, 0, rows)
	var sb strings.Builder
	for row := 0; row < rows; row++ {
		sb.Reset()
		sb.Grow(w * 12)
		var lastFG, lastBG [3]uint8
		haveFG, haveBG := false, false
		for x := 0; x < w; x++ {
			top := pixel(img, b.Min.X+x, b.Min.Y+2*row)
			bottom := top
			if 2*row+1 < h {
				bottom = pixel(img, b.Min.X+x, b.Min.Y+2*row+1)
			}
			if !haveBG || bottom != lastBG {
				writeSGR(&sb, 48, bottom)
				lastBG, haveBG = bottom, true
			}
			if top == bottom {
				sb.WriteByte(' ')
				continue
			}
			if !haveFG || top != lastFG {
				writeSGR(&sb, 38, top)
				lastFG, haveFG = top, true
			}
			sb.WriteString("▀")
		}
		sb.WriteString("\x1b[m")
		lines = append(lines, sb.String())
	}
	return lines
}

func pixel(img *image.RGBA, x, y int) [3]uint8 {
	i := img.PixOffset(x, y)
	return [3]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}
}

func writeSGR(sb *strings.Builder, kind int, c [3]uint8) {
	sb.WriteString("\x1b[")
	sb.WriteString(strconv.Itoa(kind))
	sb.WriteString(";2;")
	sb.WriteString(strconv.Itoa(int(c[0])))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(int(c[1])))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(int(c[2])))
	sb.WriteByte('m')
}

// EncodePNG encodes img for kitty transmission.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MaxPlaceholder is the largest number of rows or columns a placeholder
// image can span.
const MaxPlaceholder = len(diacritics)

const chunk = 4096

// Transmit returns the escape sequence that sends a PNG to the terminal as
// image id and creates a virtual placement cols × rows cells big, to be
// shown by placeholder cells. q=2 keeps the terminal from replying, so no
// answer arrives as keyboard input.
func Transmit(id uint8, pngData []byte, cols, rows int) string {
	enc := base64.StdEncoding.EncodeToString(pngData)
	var sb strings.Builder
	sb.Grow(len(enc) + len(enc)/chunk*16 + 64)
	first := true
	for len(enc) > 0 || first {
		n := min(chunk, len(enc))
		part := enc[:n]
		enc = enc[n:]
		more := "0"
		if len(enc) > 0 {
			more = "1"
		}
		sb.WriteString("\x1b_G")
		if first {
			sb.WriteString("a=T,U=1,f=100,t=d,q=2,i=")
			sb.WriteString(strconv.Itoa(int(id)))
			sb.WriteString(",c=")
			sb.WriteString(strconv.Itoa(cols))
			sb.WriteString(",r=")
			sb.WriteString(strconv.Itoa(rows))
			sb.WriteByte(',')
			first = false
		} else {
			sb.WriteString("q=2,")
		}
		sb.WriteString("m=")
		sb.WriteString(more)
		sb.WriteByte(';')
		sb.WriteString(part)
		sb.WriteString("\x1b\\")
	}
	return sb.String()
}

// Delete returns the escape sequence that removes image id and frees its
// data in the terminal.
func Delete(id uint8) string {
	return "\x1b_Ga=d,d=I,q=2,i=" + strconv.Itoa(int(id)) + "\x1b\\"
}

// PlaceholderLines returns the cells that show image id, cols × rows cells.
// The foreground colour carries the image id; each cell carries its row and
// column as combining marks on U+10EEEE.
func PlaceholderLines(id uint8, cols, rows int) []string {
	cols = min(max(cols, 0), MaxPlaceholder)
	rows = min(max(rows, 0), MaxPlaceholder)
	lead := "\x1b[38;5;" + strconv.Itoa(int(id)) + "m"
	lines := make([]string, rows)
	var sb strings.Builder
	for r := 0; r < rows; r++ {
		sb.Reset()
		sb.Grow(len(lead) + cols*8 + 4)
		sb.WriteString(lead)
		for c := 0; c < cols; c++ {
			sb.WriteRune(placeholder)
			sb.WriteRune(diacritics[r])
			sb.WriteRune(diacritics[c])
		}
		sb.WriteString("\x1b[m")
		lines[r] = sb.String()
	}
	return lines
}

const placeholder = '\U0010EEEE'

// diacritics are the first combining marks of the kitty placeholder table;
// the n-th mark stands for row or column n.
var diacritics = [...]rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F, 0x0346, 0x034A,
	0x034B, 0x034C, 0x0350, 0x0351, 0x0352, 0x0357, 0x035B, 0x0363, 0x0364, 0x0365,
	0x0366, 0x0367, 0x0368, 0x0369, 0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F,
	0x0483, 0x0484, 0x0485, 0x0486, 0x0487, 0x0592, 0x0593, 0x0594, 0x0595, 0x0597,
	0x0598, 0x0599, 0x059C, 0x059D, 0x059E, 0x059F, 0x05A0, 0x05A1, 0x05A8, 0x05A9,
	0x05AB, 0x05AC, 0x05AF, 0x05C4, 0x0610, 0x0611, 0x0612, 0x0613, 0x0614, 0x0615,
	0x0616, 0x0617, 0x0657, 0x0658, 0x0659, 0x065A, 0x065B, 0x065D, 0x065E, 0x06D6,
	0x06D7, 0x06D8, 0x06D9, 0x06DA, 0x06DB, 0x06DC, 0x06DF, 0x06E0, 0x06E1, 0x06E2,
	0x06E4, 0x06E7, 0x06E8, 0x06EB, 0x06EC, 0x0730, 0x0732, 0x0733, 0x0735, 0x0736,
	0x073A, 0x073D, 0x073F, 0x0740, 0x0741, 0x0743, 0x0745, 0x0747, 0x0749, 0x074A,
}
